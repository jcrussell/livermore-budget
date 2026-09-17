package check

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestScopeConstantsNameDeclaredCuts holds this package's scope and cut-name
// constants to internal/structure's declarations, so the two spellings cannot
// drift: a check comparing facts by scope string and a cut admitting facts by
// the same string must mean the same schedule.
func TestScopeConstantsNameDeclaredCuts(t *testing.T) {
	byName := map[string]structure.Cut{}
	for _, c := range structure.AllCuts() {
		byName[c.Name] = c
	}
	for _, tt := range []struct{ cut, scope string }{
		{spineCut, spineScope},
		{revenueDetailCut, revenueDetailScope},
		{departmentwideCut, "departmentwide-expenditures"},
		{"funding-sources", fundingSourcesScope},
	} {
		c, ok := byName[tt.cut]
		if !ok {
			t.Errorf("no declared cut is named %q", tt.cut)
			continue
		}
		if c.Scope != tt.scope {
			t.Errorf("cut %q reads scope %q and this package spells it %q", tt.cut, c.Scope, tt.scope)
		}
	}
}
