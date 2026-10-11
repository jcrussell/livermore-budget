package mapping

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// Budget Book pp.80-81's DEBT SERVICE REQUIREMENTS, read through the
// committed rule against the committed fixtures, so the arithmetic below is
// over the tokens the published rule's own anchors produce.

// debtResolver loads the published Budget Book rules over pp.80-81 and
// returns the requirements rule and the by-issue principal and interest rules.
func debtResolver(t *testing.T) (r *Resolver, req, principal, interest *Rule) {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for i := range f.Rules {
		switch f.Rules[i].ID {
		case "debt-service-requirements":
			req = &f.Rules[i]
		case "debt-service-principal":
			principal = &f.Rules[i]
		case "debt-service-interest":
			interest = &f.Rules[i]
		}
	}
	if req == nil || principal == nil || interest == nil {
		t.Fatalf("%s lacks a debt-service rule", publishedSpine)
	}
	r, err = NewResolver(testDoc(t, budgetFixtures, []int{80, 81}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, req, principal, interest
}

// reqKey is one row of the schedule and one of its two measures.
type reqKey struct {
	Row      string
	Category string
}

// TestDebtServiceRequirementsCrossFootToTheGrandTotal holds the schedule to
// both of its printed totals: every published column to the Total under it
// (CheckTotals, which fisc verify runs), and every row's four fund types to
// the Grand Total p81 prints beside them, which the rule skips because it
// is their sum. The second is what catches a row of p81's positional read
// filed one line off: both pages' columns would still tie.
//
// Mutation: give row "2031-2035" skip: true and CheckTotals goes red on
// p80 by 12,521,661 and 9,563,191; drop the Water blanks under 2036-2040 and
// the read refuses p81's count.
func TestDebtServiceRequirementsCrossFootToTheGrandTotal(t *testing.T) {
	r, req, _, _ := debtResolver(t)
	funds := map[reqKey]amount.Cents{}
	grand := map[reqKey]amount.Cents{}
	for i := range req.Parts {
		p := &req.Parts[i]
		if _, err := r.CheckTotals(req, p); err != nil {
			t.Fatalf("p%d CheckTotals: %v", p.Page, err)
		}
		cells, err := r.Cells(req, p)
		if err != nil {
			t.Fatalf("p%d Cells: %v", p.Page, err)
		}
		for _, c := range cells {
			if c.Column.Skip {
				// p81's Grand Total pair: Principal then Interest.
				cat := "debt-services/principal"
				if c.ColumnIndex == 5 {
					cat = "debt-services/interest"
				}
				grand[reqKey{c.Row.Label, cat}] += c.Cents
				continue
			}
			funds[reqKey{c.Row.Label, c.Category()}] += c.Cents
		}
	}
	if len(grand) != 20 {
		t.Fatalf("p81 prints a Grand Total pair on 10 rows; read %d cells", len(grand))
	}
	if diff := cmp.Diff(grand, funds); diff != "" {
		t.Errorf("the fund types do not sum to the Grand Total (-grand +funds):\n%s", diff)
	}
}

// TestInternalServiceRequirementsAreTheLease: the Internal Service Funds'
// 2026 and 2027 requirements are the Lease/Purchase Loan's FY2025-26 and
// FY2026-27 debt service on p81, to the dollar -- the year ending June 30,
// 2026 is fiscal_year 2026 on both schedules.
func TestInternalServiceRequirementsAreTheLease(t *testing.T) {
	r, req, principal, interest := debtResolver(t)
	got := map[reqKey]amount.Cents{}
	for i := range req.Parts {
		vals, _, err := r.Values(req, &req.Parts[i])
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		for _, v := range vals {
			c := v.Row.EffectiveColumn(v.Column)
			if c.FundGroup == "internal-service" && (c.FiscalYear == 2026 || c.FiscalYear == 2027) {
				got[reqKey{YearLabel(c.FiscalYear, c.FiscalYearThrough), v.Category()}] = v.Cents
			}
		}
	}
	want := map[reqKey]amount.Cents{}
	for _, rule := range []*Rule{principal, interest} {
		vals, _, err := r.Values(rule, &rule.Parts[1])
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		for _, v := range vals {
			if v.Row.Label == "Lease/Purchase Loan" {
				want[reqKey{YearLabel(v.Column.FiscalYear, 0), v.Category()}] = v.Cents
			}
		}
	}
	if len(want) != 4 {
		t.Fatalf("read %d Lease/Purchase cells for FY2026-27, want 4", len(want))
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Internal Service requirements against the lease (-lease +requirements):\n%s", diff)
	}
}
