package structure

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// A RULE READS ONE TABLE AT ONE GRAIN, and says which in mappings/*.yaml as
// its `grain:`. The declaration is what the lattice reads; the derivation below
// is what makes it worth reading. Measured over the committed store: of the
// rules that produce facts, every one populates a single axis union, so the
// two can be compared rule by rule and a declaration the facts do not bear out
// is refused rather than trusted.
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

// LevelOfRule is each fact-publishing rule's declared grain, checked against
// the axes its facts populate.
//
// OVER THE WHOLE STORE AND EVERY RULE FILE, because four things are refused and
// a subset of either would hide one: a grain naming no declared level; a rule
// publishing facts with no grain declared; a declared grain the facts
// contradict, named with both levels; and a grain declared on a rule the store
// carries no fact for, which nothing could check. The parser refuses the last
// two shapes it can see -- a missing grain on a publishing rule and a grain on
// a rule whose every row or column is skipped -- and this is where the rest
// is settled, against the facts rather than the YAML.
//
// A RULE WHOSE AXIS UNION MATCHES NO DECLARED LEVEL IS AN ERROR, never a
// nearest match. A grain the lattice cannot name is a schedule nothing can
// compare, and silently rounding it to a neighbour would compare it against
// money it does not decompose.
func LevelOfRule(facts []fact.Fact, files []*mapping.File) (map[string]Level, error) {
	declared := map[string]Level{}
	for _, f := range files {
		for i := range f.Rules {
			r := &f.Rules[i]
			if r.Grain == "" {
				continue
			}
			l := Level(r.Grain)
			if !Declared(l) {
				return nil, fmt.Errorf("rule %q declares grain %q, which is not a declared "+
					"level; the levels are %v", r.ID, r.Grain, Levels())
			}
			declared[r.ID] = l
		}
	}

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
		derived, ok := byAxes[axisKey(axes)]
		if !ok {
			return nil, fmt.Errorf("rule %q populates axes %s, which no declared level names",
				rule, axisKey(axes))
		}
		want, ok := declared[rule]
		if !ok {
			return nil, fmt.Errorf("rule %q publishes facts at %q and declares no grain",
				rule, derived)
		}
		if want != derived {
			return nil, fmt.Errorf("rule %q declares grain %q, but its facts populate %s, "+
				"which is level %q", rule, want, axisKey(axes), derived)
		}
		out[rule] = want
	}
	for rule, l := range declared {
		if _, ok := out[rule]; !ok {
			return nil, fmt.Errorf("rule %q declares grain %q and the store carries no fact "+
				"of it; a grain over zero facts cannot be checked", rule, l)
		}
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
	// DepartmentTier says which tier of data/departments.yaml the pages name
	// on the department axis: "division" or "department". Required on a cut
	// whose level carries that axis, because the two tiers are two
	// vocabularies and a comparison across them shares no key.
	DepartmentTier string
	// Reference marks the cut whose columns every agreement is held to. It is
	// the spine, pp.66-67: the citywide control totals, printed for exactly
	// the columns the city adopted. At most one cut may carry it.
	Reference bool
	// Bases are the column bases the pages print. Required, and a claim about
	// the pages: the ACFR prints audited figures and the Budget Book prints
	// adopted ones, so a lattice containment between the two -- pp.127-140 by
	// fund decompose p41's General Fund summary by axes -- has no column both
	// print, and is refused rather than reported as every cell dropped.
	Bases []mapping.Basis
	// Rules selects the rules this cut reads, for a scope whose pages print
	// two grains: ACFR p167 prints the General Fund with its group named and
	// the other governmental funds aggregated, in one scope, and each is a
	// cut. Empty means every rule of the scope.
	Rules []string
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
	// key at all. ValidateCuts holds the declaration to both halves of that:
	// one value over the cut's facts, carried by no other scope.
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
	if len(c.Rules) > 0 && !contains(c.Rules, f.RuleID) {
		return false
	}
	if len(c.FundGroups) > 0 && !contains(c.FundGroups, f.FundGroup) {
		return false
	}
	return containsKind(c.Kinds, f.Kind)
}

// prints says whether the cut declares a basis.
func (c Cut) prints(b mapping.Basis) bool {
	for _, have := range c.Bases {
		if have == b {
			return true
		}
	}
	return false
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

// ValidateCuts holds a set of cuts to the store and to each other: every cut
// sits at the level its facts put it at once its placeholders are dropped,
// every cut at a level with a department axis says which tier it names, no two
// cuts share a name, and at most one is the reference.
//
// A CUT NO FACT FALLS IN IS RETURNED, NOT REFUSED, unless it selects by rule.
// A declared level over zero facts is a claim nothing can check, and a cut
// that lost its every rule would sit in a comparison as a side that prints
// nothing -- so the caller gets the names and decides, because a fixture that
// maps one schedule is not a corpus that lost five. A cut naming a RULE is
// different: the rule is a claim about the store, and one no fact carries is
// refused before the cut can be returned as empty. Measured: with
// acfr-p0167-general-fund-balances deleted from its mapping and its facts
// from the store, the cut naming it was returned as empty, and a caller
// deciding emptiness by scope instead counted it as compared.
func ValidateCuts(facts []fact.Fact, byRule map[string]Level, cuts []Cut) (empty []string, err error) {
	names := map[string]bool{}
	references := 0
	for _, c := range cuts {
		if names[c.Name] {
			return nil, fmt.Errorf("cut %q is declared twice", c.Name)
		}
		names[c.Name] = true
		if !Declared(c.Level) {
			return nil, fmt.Errorf("cut %q declares level %q, which is not declared", c.Name, c.Level)
		}
		if c.Reference {
			references++
		}
		switch {
		case hasAxis(c.Level, AxisDepartment) && c.DepartmentTier == "":
			return nil, fmt.Errorf("cut %q is at %q, which carries the department axis, and does not say "+
				"which tier of data/departments.yaml its pages name", c.Name, c.Level)
		case !hasAxis(c.Level, AxisDepartment) && c.DepartmentTier != "":
			return nil, fmt.Errorf("cut %q declares department tier %q at %q, which carries no department axis",
				c.Name, c.DepartmentTier, c.Level)
		case c.DepartmentTier != "" && c.DepartmentTier != "division" && c.DepartmentTier != "department":
			return nil, fmt.Errorf("cut %q declares department tier %q; the tiers are division and department",
				c.Name, c.DepartmentTier)
		}
		for _, g := range c.FundGroups {
			if !hasAxis(c.Level, AxisFundGroup) {
				return nil, fmt.Errorf("cut %q is pinned to fund group %q at %q, which carries no fund group axis",
					c.Name, g, c.Level)
			}
		}
		if len(c.Bases) == 0 {
			return nil, fmt.Errorf("cut %q declares no basis; which columns its pages print is a claim "+
				"about the pages and not a default", c.Name)
		}
		for _, r := range c.Rules {
			found := false
			for i := range facts {
				if facts[i].RuleID == r && facts[i].Scope == c.Scope {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("cut %q selects rule %q, which produces no fact in scope %q", c.Name, r, c.Scope)
			}
		}
		derived, ok := c.DerivedLevel(byRule, facts)
		if !ok {
			if c.admitsNone(facts) {
				empty = append(empty, c.Name)
				continue
			}
			return nil, fmt.Errorf("cut %q selects facts at no single level", c.Name)
		}
		// THE DECLARED BASES ARE HELD TO THE STORE FROM BOTH SIDES. A basis the
		// facts carry and the cut does not declare is a column the comparison
		// would silently leave out; a declared basis no fact carries is a claim
		// nothing bears out. Both are refused by name.
		seen := map[mapping.Basis]bool{}
		for i := range facts {
			f := &facts[i]
			if !c.admits(f) {
				continue
			}
			if !c.prints(f.Basis) {
				return nil, fmt.Errorf("cut %q carries a %q column and declares bases %v", c.Name, f.Basis, c.Bases)
			}
			seen[f.Basis] = true
		}
		for _, b := range c.Bases {
			if !seen[b] {
				return nil, fmt.Errorf("cut %q declares basis %q and carries no such column", c.Name, b)
			}
		}
		want := derived
		for _, a := range c.Placeholders {
			want = Drop(want, a)
		}
		if want != c.Level {
			return nil, fmt.Errorf("cut %q declares level %q and its facts put it at %q (placeholders %v dropped from %q)",
				c.Name, c.Level, want, c.Placeholders, derived)
		}
		// A PLACEHOLDER IS TOLD FROM A FOOTPRINT BY VOCABULARY, and that is
		// measured rather than trusted: the axis carries one value over the
		// cut's facts, and no fact of any other scope carries that value. A
		// second value is an axis the pages do have; a value another schedule
		// selects by is a footprint, and dropping it would sum away money the
		// comparison should key on.
		for _, a := range c.Placeholders {
			values := map[string]bool{}
			for i := range facts {
				if c.admits(&facts[i]) {
					values[coordOf(&facts[i], a)] = true
				}
			}
			if len(values) != 1 {
				return nil, fmt.Errorf("cut %q declares axis %q a placeholder and its facts carry %d values on it (%s); "+
					"a placeholder is one value standing in for an axis the pages do not have",
					c.Name, a, len(values), joinSorted(values))
			}
			var value string
			for v := range values {
				value = v
			}
			for i := range facts {
				f := &facts[i]
				if f.Scope == c.Scope || coordOf(f, a) != value {
					continue
				}
				return nil, fmt.Errorf("cut %q declares axis %q a placeholder carrying %q, and scope %q carries that value too (rule %q); "+
					"a value another schedule selects by is a footprint and not a placeholder",
					c.Name, a, value, f.Scope, f.RuleID)
			}
		}
	}
	if references > 1 {
		return nil, fmt.Errorf("%d cuts are declared the reference; the columns of an agreement are one cut's", references)
	}
	return empty, nil
}

// joinSorted renders a value set in a stable order, for a refusal.
func joinSorted(values map[string]bool) string {
	out := make([]string, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// EmptyCuts is the cuts no fact falls in, by the admission rule ValidateCuts
// early-outs on. It is for a caller that has no rule files to run ValidateCuts
// with and still has to know which cuts to leave out of a comparison.
func EmptyCuts(facts []fact.Fact, cuts []Cut) []string {
	var empty []string
	for _, c := range cuts {
		if c.admitsNone(facts) {
			empty = append(empty, c.Name)
		}
	}
	return empty
}

// admitsNone says whether no fact falls in the cut.
func (c Cut) admitsNone(facts []fact.Fact) bool {
	for i := range facts {
		if c.admits(&facts[i]) {
			return false
		}
	}
	return true
}

// TierMisfits is one line per (cut, department) pair where a fact in a cut
// declaring a DepartmentTier names a department inTier says that tier does not
// list. A slug the registry lists in both tiers fits either.
func TierMisfits(facts []fact.Fact, cuts []Cut, inTier func(tier, slug string) bool) []string {
	var out []string
	for _, c := range cuts {
		if c.DepartmentTier == "" {
			continue
		}
		misfits := map[string]int{}
		for i := range facts {
			f := &facts[i]
			if c.admits(f) && !inTier(c.DepartmentTier, f.Department) {
				misfits[orAbsent(f.Department)]++
			}
		}
		for _, slug := range sortedKeys(misfits) {
			out = append(out, fmt.Sprintf("cut %q names departments at the %s tier and %d of its "+
				"facts name %q, which data/departments.yaml does not list as a %s",
				c.Name, c.DepartmentTier, misfits[slug], slug, c.DepartmentTier))
		}
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
