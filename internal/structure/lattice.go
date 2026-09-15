// Package structure reads the fact store as one containment hierarchy rather
// than as a set of unrelated schedules.
//
// A FIGURE'S GRAIN IS WHICH AXES IT IS A TOTAL OVER, and until it is stated a
// total cannot be checked. pp.66-67 print revenue per fund group; pp.127-140
// print the same revenue per fund. Summing both is a doubling, and summing
// either alone is right -- so "the total" is a question about a set of cells,
// not about the store. A [Level] names one such set, and [Refines] says which
// level decomposes which.
//
// THE LEVEL SET AND ITS EDGES ARE DECLARED, NOT INFERRED FROM AXIS INCLUSION.
// Two levels can share an axis set and mean different things, and a refinement
// that holds arithmetically need not hold in the documents -- pp.167-170
// decompose General Fund expenditure by department, which is a refinement, and
// the ACFR's fund-balance roll-forward shares axes with it and is not. An
// undeclared edge is refused rather than assumed.
package structure

import (
	"fmt"
	"sort"
)

// Axis is one coordinate a published figure can be a total over.
//
// The four here are the coordinate axes of [fact.Fact]. Kind, basis and fiscal
// year are not axes: they select which facts are comparable at all, and a cut
// spanning two of them would be comparing different money rather than the same
// money at a different grain.
type Axis string

// The four coordinate axes of a published figure, named as [fact.Fact] names
// them so a level's axis set can be read against a record without a second
// vocabulary to keep in step.
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
	// LevelFundByDepartmentByCategory is the finest the store carries.
	LevelFundByDepartmentByCategory Level = "fund-by-department-by-category"
)

// levelAxes is every declared level and the axes it totals over.
//
// A LEVEL IS ITS AXES PLUS ITS NAME, and the name is load-bearing: see the
// package comment on why two levels may share an axis set.
var levelAxes = map[Level][]Axis{
	LevelCategory:                   {AxisCategory},
	LevelFundGroup:                  {AxisFundGroup},
	LevelFundGroupByCategory:        {AxisFundGroup, AxisCategory},
	LevelFundByCategory:             {AxisFundGroup, AxisFund, AxisCategory},
	LevelDepartmentByCategory:       {AxisDepartment, AxisCategory},
	LevelFundByDepartment:           {AxisFundGroup, AxisFund, AxisDepartment},
	LevelFundByDepartmentByCategory: {AxisFundGroup, AxisFund, AxisDepartment, AxisCategory},
}

// refinements are the declared parent edges: refinements[fine] are the levels
// fine decomposes.
//
// EDGES ARE DECLARED ONE WAY AND READ TRANSITIVELY. Declaring only the
// immediate parent keeps the table short enough to check by eye against the
// documents; [Refines] closes it.
var refinements = map[Level][]Level{
	LevelFundGroupByCategory:        {LevelFundGroup, LevelCategory},
	LevelFundByCategory:             {LevelFundGroupByCategory},
	LevelDepartmentByCategory:       {LevelCategory},
	LevelFundByDepartment:           {LevelFundGroup},
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

// Axes are the axes a level totals over, in a stable order. It returns nil for
// a level this package does not declare, which callers must treat as an error
// rather than as a level with no axes.
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

// Refines says whether fine decomposes coarse: whether coarse is reachable
// from fine by declared parent edges.
//
// A LEVEL DOES NOT REFINE ITSELF. Two cuts at one level are peers, and the
// containment comparison is between a coarse cut and a strictly finer one --
// so an equal pair must fall out of the comparison rather than tie trivially.
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

// LevelsComparable says whether two levels sit on one chain of the lattice, in
// either direction. Levels that do not are incomparable, which is what makes a
// set of them an antichain.
func LevelsComparable(a, b Level) bool {
	return a == b || Refines(a, b) || Refines(b, a)
}

// IsAntichain says whether no level in the set refines another, which is the
// condition for their cells to be summable without double counting.
//
// A REPEATED LEVEL IS NOT A VIOLATION. Two cuts at one level are peers -- the
// General Fund's departments and the other funds' departments are both
// department-by-category -- and summing them is exactly what a cut is for.
// What is refused is a pair where one decomposes the other.
func IsAntichain(levels []Level) (Level, Level, bool) {
	for i, a := range levels {
		for _, b := range levels[i+1:] {
			if Refines(a, b) {
				return a, b, false
			}
			if Refines(b, a) {
				return b, a, false
			}
		}
	}
	return "", "", true
}

// validateLattice refuses a table that cannot be read: an edge naming an
// undeclared level, a cycle, or a parent whose axes are not a subset of its
// child's. It runs from an init so a malformed table cannot reach a check.
func validateLattice() error {
	for fine, ups := range refinements {
		if !Declared(fine) {
			return fmt.Errorf("refinements declares an edge from %q, which is not a declared level", fine)
		}
		for _, coarse := range ups {
			if !Declared(coarse) {
				return fmt.Errorf("level %q refines %q, which is not a declared level", fine, coarse)
			}
			// A PARENT'S AXES ARE A SUBSET OF ITS CHILD'S. The edge is
			// declared, but an edge whose direction contradicts the axes is a
			// typo rather than a judgement, and summing along it would total a
			// coordinate the coarse level does not carry.
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

// Meet is the finest level both a and b decompose: the grain at which two cuts
// state figures about the same money and can be compared cell by cell.
//
// IT IS NOT ALWAYS ONE OF THE TWO. pp.66-67 carry a fund group and an object
// category; pp.85-125 carry a department and an object category and no fund
// axis at all. Neither decomposes the other, and the level they agree at is the
// object category alone -- which is where departmentwide expenditure reconciles
// to the spine, and where a comparison keyed on either input's own axes would
// have found no shared key.
//
// AMBIGUITY IS REFUSED RATHER THAN BROKEN BY ORDER. Two common ancestors
// neither of which refines the other mean the lattice does not say which grain
// the pair agrees at, and picking one would publish a comparison whose subject
// depends on map iteration.
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

// Drop is the level whose axes are l's without a, or "" when the lattice names
// no such level.
//
// It exists for a cut that declares a placeholder: the store puts the cut at
// one level and the pages put it a level coarser, and the difference has to be
// resolved against the declared level set rather than by subtracting axes into
// a level nothing names.
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
