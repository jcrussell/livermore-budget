package structure

import (
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// A Residue is a set of facts no cut admits, declared with the reason the
// pages put it outside every decomposition. Every fact is in exactly one cut
// or one residue, and a residue matching no fact is refused.
type Residue struct {
	Scope  string
	Rule   string
	Kind   mapping.Kind
	Reason string
}

func (r Residue) matches(f *fact.Fact) bool {
	return f.Scope == r.Scope && f.RuleID == r.Rule && f.Kind == r.Kind
}

// BudgetBookResidue is every declared residue.
func BudgetBookResidue() []Residue {
	return []Residue{{
		Scope: "departmentwide-expenditures",
		Rule:  "dw-maintenance",
		Kind:  mapping.KindTransferOut,
		Reason: "pp.85-125 print one Transfers Out row, under Maintenance, and no other: 266,798 in " +
			"FY2023-24 actual and a printed dash in every later column. It is one division's " +
			"transfer and not a decomposition of pp.66-67's citywide TRANSFER OUT, so the " +
			"departmentwide cut prints expenditure only and this row sits in no cut",
	}}
}

// Covered holds a store to its cuts: every fact is admitted by exactly one
// cut, or by no cut and exactly one declared residue, and every residue
// matches at least one fact. It returns one line per fact or residue at
// fault and the number of facts the residue covers.
func Covered(facts []fact.Fact, cuts []Cut, residue []Residue) (findings []string, uncovered int) {
	matched := make([]int, len(residue))
	for i := range facts {
		f := &facts[i]
		var in []string
		for _, c := range cuts {
			if c.admits(f) {
				in = append(in, c.Name)
			}
		}
		var declared []int
		for j, r := range residue {
			if r.matches(f) {
				declared = append(declared, j)
			}
		}
		switch {
		case len(in) > 1:
			findings = append(findings, fmt.Sprintf("fact %s (scope %q, rule %q, %s) is admitted by %d cuts %v; "+
				"a fact in two cuts is counted twice by any view holding both", f.ID, f.Scope, f.RuleID, f.Kind, len(in), in))
		case len(in) == 1 && len(declared) > 0:
			findings = append(findings, fmt.Sprintf("fact %s (scope %q, rule %q, %s) is admitted by cut %q and "+
				"declared a residue; one of the two is a false claim about the pages", f.ID, f.Scope, f.RuleID, f.Kind, in[0]))
		case len(in) == 0 && len(declared) == 0:
			findings = append(findings, fmt.Sprintf("fact %s (scope %q, rule %q, %s) is admitted by no cut and "+
				"declared no residue, so it is in no comparison and no view and nothing says why",
				f.ID, f.Scope, f.RuleID, f.Kind))
		case len(in) == 0 && len(declared) > 1:
			findings = append(findings, fmt.Sprintf("fact %s is declared a residue %d times", f.ID, len(declared)))
		case len(in) == 0:
			matched[declared[0]]++
			uncovered++
		}
	}
	// A residue on a scope the store does not carry -- a fixture -- is not stale.
	scopes := map[string]bool{}
	for i := range facts {
		scopes[facts[i].Scope] = true
	}
	for j, r := range residue {
		if matched[j] == 0 && scopes[r.Scope] {
			findings = append(findings, fmt.Sprintf("residue (%s, %s, %s) matches no fact, so it excuses nothing; "+
				"remove it rather than leaving a declaration that has stopped describing the corpus",
				r.Scope, r.Rule, r.Kind))
		}
	}
	return findings, uncovered
}
