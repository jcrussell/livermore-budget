package structure

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// Key is one cell two cuts must agree on: a column, plus a value for each axis
// of the level they are compared at.
//
// THE AXES ARE THE MEET'S AND NOT EITHER INPUT'S. A comparison keyed on the
// finer cut's axes finds no counterpart on the coarser side; keyed on the
// coarser cut's own axes it can still name an axis the other side does not
// carry. Only the meet is a set of coordinates both sides state.
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
// about the key at all.
//
// Present is separate from a zero total because they are different claims. "The
// schedule prints this cell and it is zero" ties against a zero on the other
// side; "the schedule has no such cell" is a rule that was dropped.
type Sum struct {
	Cents   int64
	Present bool
}

// project sums a cut's facts into the cells of a level, inside a restriction
// that applies to BOTH sides of a comparison.
//
// THE RESTRICTION IS THE PAIR'S, NOT THE CUT'S. A cut's own footprint says what
// its pages cover; what a comparison may look at is the INTERSECTION of the two
// footprints, applied to each side. Applying it only to the narrower cut leaves
// every cell the wider one covers alone on its side of the union, and the
// comparison reports a schedule that never claimed to print them as having
// dropped them.
//
// A KEY IS NOT UNIQUE WITHIN A CUT, and summing is what makes it one cell. p76
// prints two transfers into Stormwater in one column, from the General Fund and
// from Wastewater, and the fund's transfer in is their sum.
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

// coordOf reads one axis off a fact.
//
// AN ABSENT COORDINATE IS SPELLED, NOT BLANK. A fund the pages do not name --
// p76's LAVWMA row, which is a joint powers authority and no City fund -- must
// not share a cell with a fund that happens to sort first, and must not read as
// the same thing as a level that carries no fund axis at all. The two are
// different claims and the second is said by the LEVEL, not by this value.
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
	// at the grain both decompose. pp.85-125 by department and pp.66-67 by
	// fund group are both decompositions of citywide expenditure by object.
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

// Difference is Against minus Cut: what the reference side carries that the
// other does not.
func (c Cell) Difference() int64 { return c.Against.Cents - c.Cut.Cents }

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
	// OneSided is how many of those only one cut produced. They tie when the
	// other side is zero, which is a real agreement -- the spine prints a dash
	// where a detail schedule prints no row -- and a reader cannot recover the
	// count from the total, so it is reported rather than folded in.
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

// Comparable says whether two cuts can be compared at a level at all, and why
// not when they cannot. It is the whole of the answer a caller outside this
// package can use: what a comparison may look at is decided inside Compare.
func Comparable(a, b Cut, at Level) error {
	_, err := restrict(a, b, at)
	return err
}

// restrict is the slice of the store a pair may be compared over at a level.
//
// THREE WAYS A PAIR IS NOT A COMPARISON, each refused by name rather than
// compared to an empty or a partial answer:
//
// THE KINDS DO NOT MEET. pp.167-170 print expenditure and pp.127-140 print
// revenue. There is no money both describe, so every cell would be one-sided
// and the comparison would report a schedule as having dropped rows it never
// had.
//
// A FOOTPRINT CANNOT BE APPLIED TO A CUT WITH NO FUND GROUP AXIS. pp.167-170
// are of the General Fund; pp.85-125 span every fund and print no fund axis
// anywhere. So there is no way to ask pp.85-125 for their General Fund part,
// and a comparison of the whole against the part would report the difference
// as a defect. The spine can answer that question -- it carries the fund group
// -- which is why pp.167-170 reconcile there and not here.
//
// NO COLUMN BOTH PRINT. The ACFR prints audited figures and the Budget Book
// prints adopted ones, so pp.127-140 by fund and p41's General Fund summary
// are a lattice containment with no column in common, and comparing them
// would report every one of p41's cells as a rule the finer schedule dropped.
// A pair sharing no basis is not a comparison and is refused by name.
//
// THE DEPARTMENT AXIS IS TWO VOCABULARIES. pp.167-170 and pp.85-125's upper
// block name DIVISIONS; pp.171-176 name DEPARTMENTS, one tier up. Compared at a
// level carrying the department axis the two sides share no key, and the
// comparison reports every division as a cell the department schedule dropped.
// Measured over the committed store at the one pair that reaches this arm,
// pp.167-170 against pp.171-176 at fund-by-department: 104 of 116 cells
// disagree, on a schedule pair whose money is the same. Folding divisions into
// their departments would make the pair comparable and is not done here.
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
// pair it cannot relate.
//
// A PAIR AT ONE LEVEL IS NOT THIS FUNCTION'S. Two cuts at one level are peers,
// and whether they agree on the cells they share is a different question with
// a different failure mode; see Peers.
//
// A PAIR THAT MEETS STRICTLY BELOW BOTH IS COMPARED AT THE MEET, and only when
// one side is the reference. pp.85-125 by department and pp.66-67 by fund group
// both decompose citywide expenditure by object, and the object category is
// where they must agree. Which side decides the columns is the question a
// containment answers by direction and an agreement cannot, so the cut declared
// as the reference decides, and a pair with no reference in it is refused
// rather than compared on an intersection either side could shrink.
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

// Contain compares a finer cut against a coarser one at the level they meet.
//
// THE COARSE SIDE DECIDES THE COLUMNS. A coarse column with no fine column
// behind it is a gap in the mapping, which this check exists to find; a fine
// column with no coarse column is a gap in the documents -- pp.66-67 print no
// actual or revised column -- and is not this comparison's business. An
// intersection would let the fine side opt out of a column by dropping it.
//
// THE UNION OF KEYS, NOT THE FINE SIDE'S. Iterating only what the fine cut
// produced makes a dropped rule invisible: delete a block and its cells vanish
// from that side entirely, so the loop compares nothing and reports clean.
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
	for _, k := range unionKeys(cells, ref) {
		if !columns[k.Column()] {
			continue
		}
		cell := Cell{Key: k, Cut: cells[k], Against: ref[k]}
		out.Cells = append(out.Cells, cell)
		out.Subjects++
		if !cell.Cut.Present || !cell.Against.Present {
			out.OneSided++
		}
		if !cell.Ties() {
			out.Findings = append(out.Findings, out.Finding(cell))
		}
	}
	for col := range columns {
		out.Columns = append(out.Columns, col)
	}
	sort.Strings(out.Columns)
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

// unionKeys is every key either side produced, in a stable order so two runs
// report the same findings in the same sequence.
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
