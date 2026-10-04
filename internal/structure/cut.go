package structure

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// ruleAxes is the axis union each rule's facts populate. It is a union over
// the rule and not per fact: p76's LAVWMA rows name no City fund, and read per
// fact they would look like a coarser grain rather than an absent value.
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

// LevelOfRule is each fact-publishing rule's declared `grain:`, checked
// against the axes its facts populate. It needs the whole store and every rule
// file: a grain on a rule with no facts, and facts from a rule with no grain,
// are both refused. An axis union matching no declared level is an error,
// never a nearest match.
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
// a set of facts all at one level, inside a footprint declared as a claim
// about the pages. A footprint read off the facts would shrink with them and
// hide a dropped rule.
type Cut struct {
	// Name identifies the cut in a finding: the scope, qualified where one
	// scope prints two grains.
	Name string
	// Scope is the fact scope this cut selects.
	Scope string
	// Level is the grain every fact of this cut is at, checked against
	// LevelOfRule.
	Level Level
	// Kinds are the fact kinds the pages print. Required: a cut admitting every
	// kind compares a revenue schedule against fund balances.
	Kinds []vocab.Kind
	// FundGroups pins the cut to the groups its pages cover. Empty means all.
	FundGroups []string
	// DepartmentTier is the tier of data/departments.yaml the pages name on
	// the department axis, TierDivision or TierDepartment; required where the
	// level carries that axis, since the two tiers share no key.
	DepartmentTier string
	// Reference marks the cut whose columns every agreement is held to: the
	// spine, pp.66-67. At most one cut carries it.
	Reference bool
	// Bases are the column bases the pages print. Required: an ACFR cut and a
	// Budget Book cut can be a lattice containment with no column both print.
	Bases []vocab.Basis
	// Rules selects the rules this cut reads, for a scope whose pages print
	// two grains. Empty means every rule of the scope.
	Rules []string
	// Placeholders are axes whose field the facts populate and whose pages
	// carry no such axis. A single-valued axis is a placeholder only if no
	// other scope carries its value: pp.167-170's one fund group `general` is
	// a footprint, pp.171-176's one category `department-funding-sources` is
	// a placeholder.
	Placeholders []Axis
	// Outside is why the pages put this cut's money outside the reference's
	// totals, or empty. The lattice compares such a cut with no other (the
	// peers check still pairs it at its own level), and
	// ValidateOutside holds the claim to the store: every fact carries a fund,
	// and no fund it carries is carried by a fact of any other cut.
	Outside string
	// Lines are the lines the pages print, where they print fewer than a
	// coarser cut's: a lattice comparison then holds the coarser side only on
	// these. Empty means every line of the cut's kinds. A comparison refuses a
	// fact of this cut on no declared line, so a line cannot be dropped from
	// the comparison by being left off the list. Peers does not read Lines.
	Lines []Line
	// Unprinted are the lines a cut compared with this one prints and these
	// pages do not. A comparison refuses a fact of the other side on neither
	// list, so a line the other side adds is a refusal and not a cell that
	// leaves the comparison unseen.
	Unprinted []Line
}

// DerivedLevel is the level this cut's facts put it at, before its declared
// placeholders are removed.
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

// Admits says whether a fact falls inside this cut: its scope, its rules
// where the cut selects some, its fund groups where the cut is pinned to
// some, and its kinds.
func (c Cut) Admits(f *fact.Fact) bool { return c.admits(f) }

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
func (c Cut) prints(b vocab.Basis) bool {
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

func containsKind(haystack []vocab.Kind, needle vocab.Kind) bool {
	for _, k := range haystack {
		if k == needle {
			return true
		}
	}
	return false
}

// ValidateCuts holds a set of cuts to the store and to each other: level,
// department tier, bases, kinds and placeholders against the facts, unique
// names, at most one reference.
//
// A cut no fact falls in is returned, not refused -- a fixture that maps one
// schedule is not a corpus that lost five -- unless it names a Rule, which is
// a claim about the store and is refused when no fact carries it.
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
		case c.DepartmentTier != "" && c.DepartmentTier != TierDivision && c.DepartmentTier != TierDepartment:
			return nil, fmt.Errorf("cut %q declares department tier %q; the tiers are division and department",
				c.Name, c.DepartmentTier)
		}
		for _, l := range append(slices.Clone(c.Lines), c.Unprinted...) {
			if !containsKind(c.Kinds, l.Kind) {
				return nil, fmt.Errorf("cut %q declares line %s, whose kind it does not print", c.Name, l)
			}
		}
		if len(c.Unprinted) > 0 && len(c.Lines) == 0 {
			return nil, fmt.Errorf("cut %q declares lines it does not print and none it does", c.Name)
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
		// Bases are held from both sides: an undeclared one would be left out
		// silently, and a declared one no fact carries is unfounded.
		type column struct {
			year  int
			basis vocab.Basis
		}
		seen := map[vocab.Basis]bool{}
		kinds := map[column]map[vocab.Kind]bool{}
		var columns []column
		for i := range facts {
			f := &facts[i]
			if !c.admits(f) {
				continue
			}
			if !c.prints(f.Basis) {
				return nil, fmt.Errorf("cut %q carries a %q column and declares bases %v", c.Name, f.Basis, c.Bases)
			}
			seen[f.Basis] = true
			col := column{f.FiscalYear, f.Basis}
			if kinds[col] == nil {
				kinds[col] = map[vocab.Kind]bool{}
				columns = append(columns, col)
			}
			kinds[col][f.Kind] = true
		}
		for _, b := range c.Bases {
			if !seen[b] {
				return nil, fmt.Errorf("cut %q declares basis %q and carries no such column", c.Name, b)
			}
		}
		// A declared kind a column carries no fact of is a dropped rule the
		// cut would otherwise compare as present.
		for _, col := range columns {
			for _, k := range c.Kinds {
				if !kinds[col][k] {
					return nil, fmt.Errorf("cut %q declares kind %q and its %s column carries no fact of it",
						c.Name, k, fact.ColumnLabel(col.year, col.basis))
				}
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
		// A placeholder carries one value, and no other scope carries it.
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

// EmptyCuts is the cuts no fact falls in, for a caller with no rule files to
// run ValidateCuts with.
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
