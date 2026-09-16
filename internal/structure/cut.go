package structure

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// A RULE READS ONE TABLE AT ONE GRAIN, which is what lets the level be derived
// rather than declared while the fact format still carries no grain field.
// Measured over the committed store: of the rules that produce facts, every one
// populates a single axis union.
//
// THE UNION IS OVER THE RULE'S FACTS, NOT PER FACT, and the difference is the
// whole point. p76-transfers-in-enterprise sets a fund on 22 of its 24 facts
// and omits it on the LAVWMA rows, which name a joint powers authority and no
// City fund. Read per fact those two look like a coarser grain; read as a union
// they are what they are -- an absent VALUE on an axis the rule carries.
// Conflating the two would put a $13.2M fund-group total and a printed dash at
// one address.

// ruleAxes is the axis union one rule's facts populate.
func ruleAxes(facts []fact.Fact) map[string]map[Axis]bool {
	out := map[string]map[Axis]bool{}
	for i := range facts {
		f := &facts[i]
		set := out[f.RuleID]
		if set == nil {
			set = map[Axis]bool{}
			out[f.RuleID] = set
		}
		if f.FundGroup != "" {
			set[AxisFundGroup] = true
		}
		if f.Fund != nil {
			set[AxisFund] = true
		}
		if f.Department != "" {
			set[AxisDepartment] = true
		}
		if f.Category != "" {
			set[AxisCategory] = true
		}
	}
	return out
}

// LevelOfRule is each rule's grain, derived from the axes its facts populate.
//
// A RULE WHOSE AXIS UNION MATCHES NO DECLARED LEVEL IS AN ERROR, never a
// nearest match. A grain the lattice cannot name is a schedule nothing can
// compare, and silently rounding it to a neighbour would compare it against
// money it does not decompose.
func LevelOfRule(facts []fact.Fact) (map[string]Level, error) {
	byAxes := map[string]Level{}
	for l, axes := range levelAxes {
		byAxes[axisKey(axes)] = l
	}
	out := map[string]Level{}
	for rule, set := range ruleAxes(facts) {
		axes := make([]Axis, 0, len(set))
		for a := range set {
			axes = append(axes, a)
		}
		l, ok := byAxes[axisKey(axes)]
		if !ok {
			return nil, fmt.Errorf("rule %q populates axes %s, which no declared level names",
				rule, axisKey(axes))
		}
		out[rule] = l
	}
	return out, nil
}

// axisKey renders an axis set in a stable order, for lookup and for messages.
func axisKey(axes []Axis) string {
	sorted := make([]string, len(axes))
	for i, a := range axes {
		sorted[i] = string(a)
	}
	sort.Strings(sorted)
	key := ""
	for i, a := range sorted {
		if i > 0 {
			key += "+"
		}
		key += a
	}
	if key == "" {
		return "(none)"
	}
	return key
}

// A Cut is one slice of the store that can be summed without double counting:
// a set of facts all at one level, inside a declared footprint.
//
// THE FOOTPRINT IS DECLARED AND NOT MEASURED, and that is the one thing here
// that must not be derived. A footprint read off the facts shrinks with them,
// so a schedule that loses a fund group loses it from its own footprint too and
// the cells it stopped printing are never looked at -- the dropped-rule case
// reported green. The footprint is a claim about the PAGES.
type Cut struct {
	// Name identifies the cut in a finding. It is the scope for a scope drawn
	// at one level, and the scope plus a qualifier where one scope prints two
	// grains -- the ACFR's fund-balance schedule prints the General Fund with
	// its group named and the other governmental funds aggregated.
	Name string
	// Scope is the fact scope this cut selects.
	Scope string
	// Level is the grain every fact of this cut is at. It is checked against
	// LevelOfRule rather than trusted.
	Level Level
	// Kinds are the fact kinds the pages print. Required: a cut admitting every
	// kind compares a revenue schedule against fund balances.
	Kinds []mapping.Kind
	// FundGroups pins the cut to the groups its pages cover. Empty means all
	// six, which is a claim about the pages and not a default.
	FundGroups []string
	// Placeholders are axes whose field the facts populate and whose pages
	// carry no such axis, each with the reason it is not one.
	//
	// A DEGENERATE AXIS IS NOT AUTOMATICALLY A PLACEHOLDER, and the two are
	// told apart by VOCABULARY rather than by cardinality. pp.167-170 carry
	// exactly one fund group, `general`, and that is a footprint: `general` is
	// one of the six values five other schedules also use, and the axis is real
	// wherever the pages could have named another. pp.171-176 carry exactly one
	// category, `department-funding-sources`, which is the scope's own name and
	// appears nowhere else in the store: nothing is being selected, the field
	// is standing in for an axis the pages do not have. Read as an axis it
	// makes a comparison against the spine's four object categories share no
	// key at all.
	Placeholders []Axis
}

// DerivedLevel is the level this cut's facts put it at, before its declared
// placeholders are removed. It exists so a declaration can be checked against
// the store rather than trusted.
func (c Cut) DerivedLevel(byRule map[string]Level, facts []fact.Fact) (Level, bool) {
	var got Level
	for i := range facts {
		f := &facts[i]
		if !c.admits(f) {
			continue
		}
		l, ok := byRule[f.RuleID]
		if !ok {
			return "", false
		}
		if got == "" {
			got = l
			continue
		}
		if got != l {
			return "", false
		}
	}
	return got, got != ""
}

// admits says whether a fact falls inside this cut.
func (c Cut) admits(f *fact.Fact) bool {
	if f.Scope != c.Scope {
		return false
	}
	if len(c.FundGroups) > 0 && !contains(c.FundGroups, f.FundGroup) {
		return false
	}
	return containsKind(c.Kinds, f.Kind)
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func containsKind(haystack []mapping.Kind, needle mapping.Kind) bool {
	for _, k := range haystack {
		if k == needle {
			return true
		}
	}
	return false
}
