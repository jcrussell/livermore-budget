package check

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestDebtServiceTiesToFactsGoesRed edits the committed debt-service document
// one way at a time and holds the check to naming each: a figure that is not
// its fact, a fact no figure publishes, a band read as a year, and a total
// that is not its facts' sum.
//
// Mutation: drop the slice sweep from debtServiceTiesToFacts.Run and "a
// figure dropped" goes green.
func TestDebtServiceTiesToFactsGoesRed(t *testing.T) {
	t.Parallel()
	const id = "debt-service-ties-to-facts"
	if res := resultFor(t, runOne(t, committed(t), &debtServiceTiesToFacts{}), id); res.Status != StatusPass {
		t.Fatalf("the committed document: status = %s, findings %v", res.Status, res.Findings)
	}
	for _, tt := range []struct {
		name string
		edit func(d *project.DebtServiceDocument)
		want string
	}{
		{"a figure that is not its fact", func(d *project.DebtServiceDocument) {
			d.Issues[0].Cells[0].Principal.AmountCents++
		}, "publishes"},
		{"a figure dropped", func(d *project.DebtServiceDocument) {
			d.Issues[0].Cells[0].Principal = nil
		}, "is published by 0 figures"},
		{"a band read as its first year", func(d *project.DebtServiceDocument) {
			for i := range d.Requirements.Periods {
				d.Requirements.Periods[i].FiscalYearThrough = nil
			}
		}, "sits in FY2031 projected"},
		{"a total that is not its facts' sum", func(d *project.DebtServiceDocument) {
			d.Requirements.Totals[0].PrincipalCents++
		}, "the facts it cites sum to"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := mutable(t)
			edited := false
			for i, p := range s.Projections {
				if p.Debt == nil {
					continue
				}
				raw, err := json.Marshal(p.Debt)
				if err != nil {
					t.Fatal(err)
				}
				var d project.DebtServiceDocument
				if err := json.Unmarshal(raw, &d); err != nil {
					t.Fatal(err)
				}
				tt.edit(&d)
				s.Projections[i].Debt = &d
				edited = true
			}
			if !edited {
				t.Fatal("the committed corpus builds no debt-service document")
			}
			res := resultFor(t, runOne(t, s, &debtServiceTiesToFacts{}), id)
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("status = %s, findings %v; want one containing %q", res.Status, res.Findings, tt.want)
			}
		})
	}
}
