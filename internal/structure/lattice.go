// Package structure reads the fact store as one containment hierarchy rather
// than as a set of unrelated schedules.
//
// A figure's grain is which axes it is a total over: pp.66-67 print revenue
// per fund group and pp.127-140 the same revenue per fund, so summing both
// doubles it. A [Level] names a grain and [Refines] says which decomposes
// which. Levels and edges are declared, not inferred from axis inclusion: two
// levels can share an axis set and mean different things.
package structure

import (
	"fmt"
	"sort"
)

// Axis is one coordinate a published figure can be a total over. Kind, basis
// and fiscal year are not axes: they select which facts are comparable at all.
type Axis string

// The coordinate axes, spelled as [fact.Fact]'s JSON keys.
const (
	AxisFundGroup  Axis = "fund_group"
	AxisFund       Axis = "fund"
	AxisDepartment Axis = "department"
	AxisCategory   Axis = "category"
)

// Level is a named point in the grain lattice.
type Level string

const (
	// LevelCategory is a figure with no fund and no department: a category and
	// nothing else.
	LevelCategory Level = "category"
	// LevelFundGroup is a figure with no category axis: a schedule that
	// decomposes a group's money by something other than what it was spent on.
	LevelFundGroup Level = "fund-group"
	// LevelFundGroupByCategory is the spine's grain, pp.66-67.
	LevelFundGroupByCategory Level = "fund-group-by-category"
	// LevelFundByCategory is pp.127-140 and p76: the same money, per fund.
	LevelFundByCategory Level = "fund-by-category"
	// LevelDepartmentByCategory is pp.85-125, which name a department and
	// carry no fund axis at all.
	LevelDepartmentByCategory Level = "department-by-category"
	// LevelFundByDepartment is pp.171-176, which reach a fund and a department
	// and print no object category to reach.
	LevelFundByDepartment Level = "fund-by-department"
	// LevelDepartment is a department's total, the grain pp.85-125 print it at
	// twice: Total Department Expenditures and Total Department Funding Sources.
	LevelDepartment Level = "department"
	// LevelFundByDepartmentByCategory is the finest the store carries.
	LevelFundByDepartmentByCategory Level = "fund-by-department-by-category"
)

// levelAxes is every declared level and the axes it totals over.
var levelAxes = map[Level][]Axis{
	LevelCategory:                   {AxisCategory},
	LevelFundGroup:                  {AxisFundGroup},
	LevelFundGroupByCategory:        {AxisFundGroup, AxisCategory},
	LevelFundByCategory:             {AxisFundGroup, AxisFund, AxisCategory},
	LevelDepartmentByCategory:       {AxisDepartment, AxisCategory},
	LevelFundByDepartment:           {AxisFundGroup, AxisFund, AxisDepartment},
	LevelDepartment:                 {AxisDepartment},
	LevelFundByDepartmentByCategory: {AxisFundGroup, AxisFund, AxisDepartment, AxisCategory},
}

// refinements are the declared immediate parent edges: refinements[fine] are
// the levels fine decomposes. [Refines] closes them transitively.
var refinements = map[Level][]Level{
	LevelFundGroupByCategory:        {LevelFundGroup, LevelCategory},
	LevelFundByCategory:             {LevelFundGroupByCategory},
	LevelDepartmentByCategory:       {LevelCategory, LevelDepartment},
	LevelFundByDepartment:           {LevelFundGroup, LevelDepartment},
	LevelFundByDepartmentByCategory: {LevelFundByCategory, LevelDepartmentByCategory, LevelFundByDepartment},
}

// Levels is every declared level, in a stable order.
func Levels() []Level {
	out := make([]Level, 0, len(levelAxes))
	for l := range levelAxes {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Axes are the axes a level totals over, in a stable order, or nil for an
// undeclared level -- which callers must treat as an error, not as no axes.
func Axes(l Level) []Axis {
	axes, ok := levelAxes[l]
	if !ok {
		return nil
	}
	out := make([]Axis, len(axes))
	copy(out, axes)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Declared says whether l is a level this package knows.
func Declared(l Level) bool {
	_, ok := levelAxes[l]
	return ok
}

// Refines says whether coarse is reachable from fine by declared parent edges.
// A level does not refine itself: two cuts at one level are peers, and must
// not tie trivially.
func Refines(fine, coarse Level) bool {
	if fine == coarse || !Declared(fine) || !Declared(coarse) {
		return false
	}
	seen := map[Level]bool{fine: true}
	queue := []Level{fine}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, up := range refinements[cur] {
			if up == coarse {
				return true
			}
			if !seen[up] {
				seen[up] = true
				queue = append(queue, up)
			}
		}
	}
	return false
}

// validateLattice refuses an edge naming an undeclared level, a cycle, or a
// parent whose axes are not a subset of its child's.
func validateLattice() error {
	for fine, ups := range refinements {
		if !Declared(fine) {
			return fmt.Errorf("refinements declares an edge from %q, which is not a declared level", fine)
		}
		for _, coarse := range ups {
			if !Declared(coarse) {
				return fmt.Errorf("level %q refines %q, which is not a declared level", fine, coarse)
			}
			// An edge whose direction contradicts the axes is a typo, not a
			// judgement.
			if missing := notSubset(levelAxes[coarse], levelAxes[fine]); missing != "" {
				return fmt.Errorf("level %q refines %q, but %q carries axis %q and %q does not",
					fine, coarse, coarse, missing, fine)
			}
		}
	}
	for _, l := range Levels() {
		if Refines(l, l) {
			return fmt.Errorf("level %q reaches itself through refinements: the lattice has a cycle", l)
		}
	}
	return nil
}

// notSubset returns an axis of sub that sup lacks, or "" when sub is a subset.
func notSubset(sub, sup []Axis) Axis {
	have := make(map[Axis]bool, len(sup))
	for _, a := range sup {
		have[a] = true
	}
	for _, a := range sub {
		if !have[a] {
			return a
		}
	}
	return ""
}

func init() {
	if err := validateLattice(); err != nil {
		panic("internal/structure: " + err.Error())
	}
}

// ancestors is l and every level reachable from it by parent edges.
func ancestors(l Level) map[Level]bool {
	out := map[Level]bool{l: true}
	queue := []Level{l}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, up := range refinements[cur] {
			if !out[up] {
				out[up] = true
				queue = append(queue, up)
			}
		}
	}
	return out
}

// Meet is the finest level both a and b decompose, the grain at which two cuts
// can be compared cell by cell. It need not be either: pp.66-67 and pp.85-125
// meet at category alone. Two finest common ancestors are refused rather than
// broken by map order.
func Meet(a, b Level) (Level, error) {
	if !Declared(a) || !Declared(b) {
		return "", fmt.Errorf("meet of %q and %q: at least one is not a declared level", a, b)
	}
	up, shared := ancestors(a), []Level{}
	for l := range ancestors(b) {
		if up[l] {
			shared = append(shared, l)
		}
	}
	if len(shared) == 0 {
		return "", fmt.Errorf("levels %q and %q share no common coarsening, so no grain compares them", a, b)
	}
	best := []Level{}
	for _, cand := range shared {
		finest := true
		for _, other := range shared {
			if other != cand && Refines(other, cand) {
				finest = false
				break
			}
		}
		if finest {
			best = append(best, cand)
		}
	}
	if len(best) != 1 {
		sort.Slice(best, func(i, j int) bool { return best[i] < best[j] })
		return "", fmt.Errorf("levels %q and %q have %d finest common coarsenings (%v); "+
			"the lattice does not say which grain they agree at", a, b, len(best), best)
	}
	return best[0], nil
}

// Drop is the level whose axes are l's without a, or "" when no declared level
// has them: a cut's placeholder axis is resolved against the declared levels,
// never into a level nothing names.
func Drop(l Level, a Axis) Level {
	want := map[Axis]bool{}
	for _, have := range levelAxes[l] {
		if have != a {
			want[have] = true
		}
	}
	for cand, axes := range levelAxes {
		if len(axes) != len(want) {
			continue
		}
		all := true
		for _, have := range axes {
			if !want[have] {
				all = false
				break
			}
		}
		if all {
			return cand
		}
	}
	return ""
}
