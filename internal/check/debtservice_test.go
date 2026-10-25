package check

import (
	"encoding/json"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestDebtServiceTiesToFactsGoesRed edits the debt-service document one way at
// a time and holds the check to naming each.
//
// Mutation: drop the slice sweep from debtServiceTiesToFacts.Run and "a figure
// dropped" goes green; drop the column-set comparison and "a total that leaves
// out the bands" does; drop the per-type count of totals and "a fund type with
// no total" and "a fund type with two totals" do.
func TestDebtServiceTiesToFactsGoesRed(t *testing.T) {
	t.Parallel()
	if res := runCheck(t, &debtServiceTiesToFacts{}, committed(t)); res.Status != StatusPass {
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
		// The projection mutation this arm was written for: the bands left out
		// of Governmental Funds' total, ids and cents both, so the total is
		// its own facts' sum and is still not its column's.
		{"a total that leaves out the bands", func(d *project.DebtServiceDocument) {
			t := &d.Requirements.Totals[0]
			drop := map[string]bool{}
			for _, per := range d.Requirements.Periods {
				if c := per.Cells[0]; per.FiscalYearThrough != nil {
					drop[c.Principal.FactID] = true
					drop[c.Interest.FactID] = true
					t.PrincipalCents -= c.Principal.AmountCents
					t.InterestCents -= c.Interest.AmountCents
				}
			}
			kept := t.FactIDs[:0]
			for _, id := range t.FactIDs {
				if !drop[id] {
					kept = append(kept, id)
				}
			}
			t.FactIDs = kept
		}, "leaves out"},
		{"a fund type with no total", func(d *project.DebtServiceDocument) {
			d.Requirements.Totals = d.Requirements.Totals[1:]
		}, "carries 0 totals for it, want 1"},
		{"a fund type with two totals", func(d *project.DebtServiceDocument) {
			d.Requirements.Totals = append(d.Requirements.Totals, d.Requirements.Totals[0])
		}, "carries 2 totals for it, want 1"},
		{"an issue's two adopted years swapped", func(d *project.DebtServiceDocument) {
			c := d.Issues[0].Cells
			c[1], c[2] = c[2], c[1]
		}, "sits in FY2026 adopted"},
		{"principal and interest swapped", func(d *project.DebtServiceDocument) {
			c := &d.Issues[0].Cells[0]
			c.Principal, c.Interest = c.Interest, c.Principal
		}, "sits in the debt-services/principal slot"},
		{"a figure under another issue", func(d *project.DebtServiceDocument) {
			d.Issues[0].Cells[0], d.Issues[1].Cells[0] = d.Issues[1].Cells[0], d.Issues[0].Cells[0]
		}, "sits in row"},
		{"Airport's interest in Water's slot", func(d *project.DebtServiceDocument) {
			c := d.Requirements.Periods[0].Cells
			c[2].Interest, c[3].Interest = c[3].Interest, c[2].Interest
		}, "sits under fund group"},
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
			res := runCheck(t, &debtServiceTiesToFacts{}, s)
			if res.Status != StatusFail || !hasFinding(res, tt.want) {
				t.Errorf("status = %s, findings %v; want one containing %q", res.Status, res.Findings, tt.want)
			}
		})
	}
}
