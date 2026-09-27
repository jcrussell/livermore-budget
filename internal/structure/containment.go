package structure

import (
	"fmt"
	"slices"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// Key is one cell two cuts must agree on: a column, plus a value for each axis
// of the meet -- not of either input, which the other side may not carry.
type Key struct {
	Year  int
	Basis string
	Level Level
	// Coords are the meet's axes in the order Axes returns them.
	Coords string
}

func (k Key) String() string {
	return fmt.Sprintf("FY%d %s %s[%s]", k.Year, k.Basis, k.Level, k.Coords)
}

// Column is the (year, basis) half of a Key, the way a finding names it.
func (k Key) Column() string { return fmt.Sprintf("FY%d %s", k.Year, k.Basis) }

// Sum is one side of a comparison: a total, and whether the cut said anything
// about the key at all. A printed zero and no such cell are different claims.
type Sum struct {
	Cents   int64
	Present bool
}

// project sums a cut's facts into the cells of a level, inside the pair's
// restriction -- applied to both sides, or the wider cut's extra cells would
// read as dropped by the narrower. A key is not unique within a cut: p76
// prints two transfers into Stormwater in one column.
func project(facts []fact.Fact, c Cut, at Level, r restriction) (map[Key]Sum, error) {
	axes := Axes(at)
	if axes == nil {
		return nil, fmt.Errorf("cut %q: %q is not a declared level", c.Name, at)
	}
	out := map[Key]Sum{}
	for i := range facts {
		f := &facts[i]
		if !c.admits(f) || !r.admits(f) {
			continue
		}
		k := KeyOf(f, at)
		s := out[k]
		s.Cents += f.AmountCents
		s.Present = true
		out[k] = s
	}
	return out, nil
}

// KeyOf is the cell a fact falls in at a level. The level must be declared;
// callers project through Axes first.
func KeyOf(f *fact.Fact, at Level) Key {
	axes := Axes(at)
	coords := make([]string, len(axes))
	for j, a := range axes {
		coords[j] = coordOf(f, a)
	}
	return Key{Year: f.FiscalYear, Basis: string(f.Basis), Level: at, Coords: joinCoords(axes, coords)}
}

// coordOf reads one axis off a fact. An absent coordinate is spelled, so
// p76's LAVWMA row, which names no City fund, is not read as a level with no
// fund axis.
func coordOf(f *fact.Fact, a Axis) string {
	switch a {
	case AxisFundGroup:
		return orAbsent(f.FundGroup)
	case AxisFund:
		return fact.FundString(f.Fund)
	case AxisDepartment:
		return orAbsent(f.Department)
	case AxisCategory:
		return orAbsent(f.Category)
	}
	return "(absent)"
}

func orAbsent(s string) string {
	if s == "" {
		return "(absent)"
	}
	return s
}

func joinCoords(axes []Axis, coords []string) string {
	out := ""
	for i := range axes {
		if i > 0 {
			out += " "
		}
		out += string(axes[i]) + "=" + coords[i]
	}
	return out
}

// Coords renders axis values the way Key.Coords spells them, so a declaration
// can name a cell without re-deriving the joining rule.
func Coords(at Level, values map[Axis]string) string {
	axes := Axes(at)
	coords := make([]string, len(axes))
	for i, a := range axes {
		coords[i] = orAbsent(values[a])
	}
	return joinCoords(axes, coords)
}

// Relation is how two cuts stand to each other in the lattice.
type Relation string

const (
	// Containment is a finer cut against the coarser one it decomposes: the
	// fine side must sum to the coarse side's own cells.
	Containment Relation = "containment"
	// Agreement is two cuts neither of which decomposes the other, compared
	// at the grain both decompose.
	Agreement Relation = "agreement"
)

// A Cell is one key both sides were asked about.
type Cell struct {
	Key Key
	// Cut is the side named first in the comparison, Against the side whose
	// columns decided what was compared.
	Cut, Against Sum
}

// Ties says whether the two sides agree. A one-sided cell ties when the side
// that printed it printed zero, which is a real agreement -- the spine prints a
// dash where a detail schedule prints no row.
func (c Cell) Ties() bool { return c.Cut.Cents == c.Against.Cents }

// A Comparison is what one pass produced.
type Comparison struct {
	Cut, Against Cut
	Relation     Relation
	At           Level
	// Cells is every key either side produced inside the columns compared,
	// in a stable order. Subjects, OneSided and Findings are read off it.
	Cells []Cell
	// Subjects is the number of cells compared.
	Subjects int
	// OneSided is how many of those only one cut produced.
	OneSided int
	// Columns is the (year, basis) set the comparison covered.
	Columns []string
	// Findings is one line per disagreeing cell.
	Findings []string
}

// Name is the pair as a finding names it.
func (c Comparison) Name() string {
	if c.Relation == Agreement {
		return fmt.Sprintf("%s ~ %s", c.Cut.Name, c.Against.Name)
	}
	return fmt.Sprintf("%s -> %s", c.Cut.Name, c.Against.Name)
}

// Finding is the line one disagreeing cell produces, with the three ways a
// cell can disagree told apart because they need different fixes.
func (c Comparison) Finding(cell Cell) string {
	d, sp := cell.Cut, cell.Against
	switch {
	case !d.Present:
		return fmt.Sprintf(
			"%s: %q publishes %s here and %q has no such cell at all; a cell the finer "+
				"schedule stopped printing is a rule that was dropped, not a cell that is empty",
			cell.Key, c.Against.Name, cents(sp.Cents), c.Cut.Name)
	case !sp.Present:
		return fmt.Sprintf(
			"%s: %q publishes %s here and %q has no such cell; a finer cut must decompose "+
				"the coarser one, never extend it",
			cell.Key, c.Cut.Name, cents(d.Cents), c.Against.Name)
	default:
		return fmt.Sprintf(
			"%s: %q sums to %s and %q publishes %s, a difference of %s; these are the same "+
				"money decomposed two ways and must tie to the cent",
			cell.Key, c.Cut.Name, cents(d.Cents), c.Against.Name, cents(sp.Cents), cents(d.Cents-sp.Cents))
	}
}

// restriction is the slice of the store one comparison may look at: the two
// cuts' footprints intersected.
type restriction struct {
	kinds      []mapping.Kind
	fundGroups []string
}

func (r restriction) admits(f *fact.Fact) bool {
	if len(r.fundGroups) > 0 && !contains(r.fundGroups, f.FundGroup) {
		return false
	}
	return containsKind(r.kinds, f.Kind)
}

// restrict is the slice of the store a pair may be compared over at a level.
// A pair is refused by name, never compared to a partial answer, when the
// kinds do not meet, when a fund-group footprint meets a cut with no fund
// group axis, when no column is printed by both, or when the two name the
// department axis at different tiers (HoldTie folds those).
func restrict(a, b Cut, at Level) (restriction, error) {
	var r restriction
	for _, k := range a.Kinds {
		if containsKind(b.Kinds, k) {
			r.kinds = append(r.kinds, k)
		}
	}
	if len(r.kinds) == 0 {
		return r, fmt.Errorf("%q prints %v and %q prints %v; no money is described by both",
			a.Name, a.Kinds, b.Name, b.Kinds)
	}
	r.fundGroups = a.FundGroups
	if len(b.FundGroups) > 0 {
		r.fundGroups = b.FundGroups
		if len(a.FundGroups) > 0 {
			r.fundGroups = nil
			for _, g := range a.FundGroups {
				if contains(b.FundGroups, g) {
					r.fundGroups = append(r.fundGroups, g)
				}
			}
			if len(r.fundGroups) == 0 {
				return r, fmt.Errorf("%q covers fund groups %v and %q covers %v; they share none",
					a.Name, a.FundGroups, b.Name, b.FundGroups)
			}
		}
	}
	if len(r.fundGroups) > 0 {
		for _, c := range []Cut{a, b} {
			if !hasAxis(c.Level, AxisFundGroup) {
				return r, fmt.Errorf("the pair covers only fund groups %v and %q is at %q, which "+
					"carries no fund group axis; there is no way to ask %q for that part of its money",
					r.fundGroups, c.Name, c.Level, c.Name)
			}
		}
	}
	shared := false
	for _, basis := range a.Bases {
		if b.prints(basis) {
			shared = true
			break
		}
	}
	if !shared {
		return r, fmt.Errorf("%q prints %v columns and %q prints %v; there is no column both print",
			a.Name, a.Bases, b.Name, b.Bases)
	}
	if hasAxis(at, AxisDepartment) && a.DepartmentTier != b.DepartmentTier {
		return r, fmt.Errorf("%q names departments at the %q tier and %q at the %q tier; compared "+
			"at %q the two would share no key, so the pair is not one vocabulary and is not compared",
			a.Name, a.DepartmentTier, b.Name, b.DepartmentTier, at)
	}
	return r, nil
}

// hasAxis says whether a level totals over an axis.
func hasAxis(l Level, a Axis) bool {
	for _, have := range levelAxes[l] {
		if have == a {
			return true
		}
	}
	return false
}

// Compare relates two cuts the way the lattice says they stand, and refuses a
// pair it cannot relate. Peers are Peers'. A pair meeting strictly below both
// is compared at the meet only when one side is the reference, which decides
// the columns; otherwise either side could shrink an intersection.
func Compare(facts []fact.Fact, a, b Cut) (Comparison, error) {
	switch {
	case a.Level == b.Level:
		return Comparison{}, fmt.Errorf("compare %q against %q: both are at %q, so this is a "+
			"pair of peers and not a containment", a.Name, b.Name, a.Level)
	case Refines(a.Level, b.Level):
		return Contain(facts, a, b)
	case Refines(b.Level, a.Level):
		return Contain(facts, b, a)
	}
	at, err := Meet(a.Level, b.Level)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %q against %q: %w", a.Name, b.Name, err)
	}
	switch {
	case b.Reference && !a.Reference:
		return agree(facts, a, b, at)
	case a.Reference && !b.Reference:
		return agree(facts, b, a, at)
	}
	return Comparison{}, fmt.Errorf("compare %q (%s) against %q (%s): neither decomposes the "+
		"other and neither is the reference, so nothing says whose columns the pair is held to",
		a.Name, a.Level, b.Name, b.Level)
}

// Contain compares a finer cut against a coarser one at the level they meet,
// over the union of keys in the coarse side's columns. An intersection of
// columns, or the fine side's keys alone, would let a dropped rule opt out.
func Contain(facts []fact.Fact, fine, coarse Cut) (Comparison, error) {
	at, err := Meet(fine.Level, coarse.Level)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %q against %q: %w", fine.Name, coarse.Name, err)
	}
	if !Refines(fine.Level, coarse.Level) {
		return Comparison{}, fmt.Errorf("compare %q (%s) against %q (%s): neither level decomposes "+
			"the other, so this is a pair of peers and not a containment",
			fine.Name, fine.Level, coarse.Name, coarse.Level)
	}
	out, err := compareAt(facts, fine, coarse, at)
	if err != nil {
		return Comparison{}, err
	}
	out.Relation = Containment
	return out, nil
}

// agree compares two cuts at a meet strictly below both, the reference's
// columns deciding.
func agree(facts []fact.Fact, c, ref Cut, at Level) (Comparison, error) {
	out, err := compareAt(facts, c, ref, at)
	if err != nil {
		return Comparison{}, err
	}
	out.Relation = Agreement
	return out, nil
}

func compareAt(facts []fact.Fact, c, against Cut, at Level) (Comparison, error) {
	r, err := restrict(c, against, at)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %q against %q: %w", c.Name, against.Name, err)
	}
	cells, err := project(facts, c, at, r)
	if err != nil {
		return Comparison{}, err
	}
	ref, err := project(facts, against, at, r)
	if err != nil {
		return Comparison{}, err
	}

	columns := map[string]bool{}
	for k := range ref {
		columns[k.Column()] = true
	}
	out := Comparison{Cut: c, Against: against, At: at}
	out.tally(cells, ref, func(k Key) bool { return columns[k.Column()] })
	return out, nil
}

// tally fills a comparison with every key either side produced that keep admits.
func (c *Comparison) tally(cells, ref map[Key]Sum, keep func(Key) bool) {
	columns := map[string]bool{}
	for _, k := range unionKeys(cells, ref) {
		if !keep(k) {
			continue
		}
		columns[k.Column()] = true
		cell := Cell{Key: k, Cut: cells[k], Against: ref[k]}
		c.Cells = append(c.Cells, cell)
		c.Subjects++
		if !cell.Cut.Present || !cell.Against.Present {
			c.OneSided++
		}
		if !cell.Ties() {
			c.Findings = append(c.Findings, c.Finding(cell))
		}
	}
	for col := range columns {
		c.Columns = append(c.Columns, col)
	}
	sort.Strings(c.Columns)
}

// A Tie is two cuts that print one money at a level the lattice cannot compare
// them at, because one names divisions and the other departments.
type Tie struct {
	Name string
	A, B string
	At   Level
}

// HoldTie compares a tie in every column both cuts print, a division-tier
// side's facts summed into the department data/departments.yaml puts them in.
func HoldTie(facts []fact.Fact, cuts []Cut, t Tie, department func(division string) string) (Comparison, error) {
	var a, b Cut
	for _, c := range cuts {
		switch c.Name {
		case t.A:
			a = c
		case t.B:
			b = c
		}
	}
	if a.Name == "" || b.Name == "" || !Refines(a.Level, t.At) || !Refines(b.Level, t.At) {
		return Comparison{}, fmt.Errorf("tie %q: %q and %q are not two declared cuts that both refine %q",
			t.Name, t.A, t.B, t.At)
	}
	folded := slices.Clone(facts)
	for _, c := range []*Cut{&a, &b} {
		if c.DepartmentTier != "division" {
			continue
		}
		for i := range folded {
			if c.admits(&folded[i]) {
				folded[i].Department = department(folded[i].Department)
			}
		}
		c.DepartmentTier = "department"
	}
	r, err := restrict(a, b, t.At)
	if err != nil {
		return Comparison{}, fmt.Errorf("tie %q: %w", t.Name, err)
	}
	as, err := project(folded, a, t.At, r)
	if err != nil {
		return Comparison{}, err
	}
	bs, err := project(folded, b, t.At, r)
	if err != nil {
		return Comparison{}, err
	}
	out := Comparison{Cut: a, Against: b, Relation: Agreement, At: t.At}
	out.tally(as, bs, func(k Key) bool {
		return a.prints(mapping.Basis(k.Basis)) && b.prints(mapping.Basis(k.Basis))
	})
	return out, nil
}

func cents(c int64) string {
	neg := ""
	if c < 0 {
		neg, c = "-", -c
	}
	return fmt.Sprintf("%s$%d.%02d", neg, c/100, c%100)
}

// Cents renders an amount the way a finding does.
func Cents(c int64) string { return cents(c) }

// unionKeys is every key either side produced, in a stable order.
func unionKeys(a, b map[Key]Sum) []Key {
	keys := make([]Key, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, dup := a[k]; !dup {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Year != keys[j].Year {
			return keys[i].Year < keys[j].Year
		}
		if keys[i].Basis != keys[j].Basis {
			return keys[i].Basis < keys[j].Basis
		}
		return keys[i].Coords < keys[j].Coords
	})
	return keys
}
