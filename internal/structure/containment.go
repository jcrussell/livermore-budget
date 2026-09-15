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
		coords := make([]string, len(axes))
		for j, a := range axes {
			coords[j] = coordOf(f, a)
		}
		k := Key{Year: f.FiscalYear, Basis: string(f.Basis), Level: at, Coords: joinCoords(axes, coords)}
		s := out[k]
		s.Cents += f.AmountCents
		s.Present = true
		out[k] = s
	}
	return out, nil
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
		if f.Fund == 0 {
			return "(absent)"
		}
		return fmt.Sprintf("%d", f.Fund)
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

// A Comparison is what one containment pass produced.
type Comparison struct {
	Fine, Coarse Cut
	At           Level
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

// Comparable says whether two cuts can be compared at all, and why not when
// they cannot.
//
// TWO WAYS A PAIR IS NOT A COMPARISON, both refused by name rather than
// compared to an empty or a partial answer:
//
// THE KINDS DO NOT MEET. pp.167-170 print expenditure and pp.127-140 print
// revenue. There is no money both describe, so every cell would be one-sided
// and the comparison would report a schedule as having dropped rows it never
// had.
//
// THE COARSE CUT CANNOT BE RESTRICTED TO THE FINE CUT'S FOOTPRINT. pp.167-170
// are of the General Fund; pp.85-125 span every fund and print no fund axis
// anywhere. So there is no way to ask pp.85-125 for their General Fund part,
// and a comparison of the whole against the part would report the difference as
// a defect. The spine can answer that question -- it carries the fund group --
// which is why pp.167-170 reconcile there and not here.
func Comparable(fine, coarse Cut) (restriction, error) {
	var r restriction
	for _, k := range fine.Kinds {
		if containsKind(coarse.Kinds, k) {
			r.kinds = append(r.kinds, k)
		}
	}
	if len(r.kinds) == 0 {
		return r, fmt.Errorf("%q prints %v and %q prints %v; no money is described by both",
			fine.Name, fine.Kinds, coarse.Name, coarse.Kinds)
	}
	r.fundGroups = fine.FundGroups
	if len(coarse.FundGroups) > 0 {
		r.fundGroups = coarse.FundGroups
		if len(fine.FundGroups) > 0 {
			r.fundGroups = nil
			for _, g := range fine.FundGroups {
				if contains(coarse.FundGroups, g) {
					r.fundGroups = append(r.fundGroups, g)
				}
			}
			if len(r.fundGroups) == 0 {
				return r, fmt.Errorf("%q covers fund groups %v and %q covers %v; they share none",
					fine.Name, fine.FundGroups, coarse.Name, coarse.FundGroups)
			}
		}
	}
	if len(r.fundGroups) > 0 && !hasAxis(coarse.Level, AxisFundGroup) {
		return r, fmt.Errorf("%q covers only fund groups %v and %q is at %q, which carries no "+
			"fund group axis; there is no way to ask %q for that part of its money",
			fine.Name, r.fundGroups, coarse.Name, coarse.Level, coarse.Name)
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
	r, err := Comparable(fine, coarse)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %q against %q: %w", fine.Name, coarse.Name, err)
	}
	fineCells, err := project(facts, fine, at, r)
	if err != nil {
		return Comparison{}, err
	}
	coarseCells, err := project(facts, coarse, at, r)
	if err != nil {
		return Comparison{}, err
	}

	columns := map[string]bool{}
	for k := range coarseCells {
		columns[fmt.Sprintf("FY%d %s", k.Year, k.Basis)] = true
	}

	out := Comparison{Fine: fine, Coarse: coarse, At: at}
	for _, k := range unionKeys(fineCells, coarseCells) {
		if !columns[fmt.Sprintf("FY%d %s", k.Year, k.Basis)] {
			continue
		}
		d, sp := fineCells[k], coarseCells[k]
		out.Subjects++
		if !d.Present || !sp.Present {
			out.OneSided++
		}
		if d.Cents == sp.Cents {
			continue
		}
		switch {
		case !d.Present:
			out.Findings = append(out.Findings, fmt.Sprintf(
				"%s: %q publishes %s here and %q has no such cell at all; a cell the finer "+
					"schedule stopped printing is a rule that was dropped, not a cell that is empty",
				k, coarse.Name, cents(sp.Cents), fine.Name))
		case !sp.Present:
			out.Findings = append(out.Findings, fmt.Sprintf(
				"%s: %q publishes %s here and %q has no such cell; a finer cut must decompose "+
					"the coarser one, never extend it",
				k, fine.Name, cents(d.Cents), coarse.Name))
		default:
			out.Findings = append(out.Findings, fmt.Sprintf(
				"%s: %q sums to %s and %q publishes %s, a difference of %s; these are the same "+
					"money decomposed two ways and must tie to the cent",
				k, fine.Name, cents(d.Cents), coarse.Name, cents(sp.Cents), cents(d.Cents-sp.Cents)))
		}
	}
	for c := range columns {
		out.Columns = append(out.Columns, c)
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
