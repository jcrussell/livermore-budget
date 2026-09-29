package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// scheduleSubject is one document of each of the four schedule shapes, each
// over one cited fact and one uncited printed zero. Hand-assembled, because
// every producer refuses a non-zero uncited fact and would fire first.
func scheduleSubject() *Subject {
	col := project.Column{FiscalYear: 2026, Basis: mapping.BasisAdopted}
	mk := func(id, scope string, cents int64) fact.Fact {
		return fact.Fact{ID: id, Scope: scope, Kind: mapping.KindRevenue,
			RowLabel: "Police", Category: "wages-and-benefits", FiscalYear: col.FiscalYear,
			Basis: col.Basis, AmountCents: cents}
	}
	link := func(cited string) []project.Link {
		return []project.Link{{Source: "expenditure/wages-and-benefits", Target: "dept/police",
			ValueCents: 100, FactIDs: []string{cited}}}
	}
	nodes := []project.Node{{ID: "expenditure/wages-and-benefits"}, {ID: "dept/police"}}
	facts := []fact.Fact{
		mk("ff-a", project.TrendsScope, 100), mk("ff-z", project.TrendsScope, 0),
		mk("ds-a", project.DepartmentSpendingScope, 100), mk("ds-z", project.DepartmentSpendingScope, 0),
		mk("df-a", project.DepartmentFundingScope, 100), mk("df-z", project.DepartmentFundingScope, 0),
		mk("tb-a", project.TransfersByFundScope, 100), mk("tb-z", project.TransfersByFundScope, 0),
	}
	options := func(scopes []string) project.Options {
		return project.Options{Columns: []project.Column{col}, Scopes: scopes, Version: testVersion}
	}
	doc := func(cited string) *project.Document {
		return &project.Document{Nodes: nodes, Links: link(cited),
			Metadata: project.Metadata{Counts: project.Counts{
				Facts: 2, FactsCited: 1, FactsUncited: 1, Nodes: 2, Links: 1}}}
	}
	return &Subject{
		Facts: facts,
		Projections: []projection{
			{Name: project.FundFlowsProjection, Options: options(project.FundFlowsScopes()), Graph: doc("ff-a")},
			{Name: project.DepartmentSpendingProjection, Options: options(project.DepartmentSpendingScopes()), Graph: doc("ds-a")},
			{Name: project.DepartmentFundingProjection, Options: options(project.DepartmentFundingScopes()), Graph: doc("df-a")},
			{Name: project.TransfersByFundProjection, Options: options(project.TransfersByFundScopes()), Graph: doc("tb-a")},
		},
	}
}

// TestUncitedFactsArePrintedZerosIsFailable plants one non-zero uncited fact
// in each of the four shapes in turn; every count stays right.
func TestUncitedFactsArePrintedZerosIsFailable(t *testing.T) {
	c := &uncitedFactsArePrintedZeros{}

	res, err := c.Run(t.Context(), scheduleSubject())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass || res.Subjects != 4 {
		t.Fatalf("the undamaged subject is %s over %d, want pass over 4 uncited facts: %v",
			res.Status, res.Subjects, res.Findings)
	}

	for _, id := range []string{"ff-z", "ds-z", "df-z", "tb-z"} {
		t.Run(id, func(t *testing.T) {
			s := scheduleSubject()
			for i := range s.Facts {
				if s.Facts[i].ID == id {
					s.Facts[i].AmountCents = 500
				}
			}
			got, gotErr := c.Run(t.Context(), s)
			if gotErr != nil {
				t.Fatalf("Run: %v", gotErr)
			}
			if got.Status != StatusFail || len(got.Findings) != 1 {
				t.Fatalf("status %s with %d findings, want one failure: %v",
					got.Status, len(got.Findings), got.Findings)
			}
			if d := got.Findings[0].Detail; !strings.Contains(d, id) || !strings.Contains(d, "$5.00") ||
				!strings.Contains(d, "not a printed zero") {
				t.Errorf("the finding names neither the fact nor its amount: %s", d)
			}
		})
	}

	// The counts check is green on the same damage.
	s := scheduleSubject()
	for i := range s.Facts {
		if s.Facts[i].ID == "ff-z" {
			s.Facts[i].AmountCents = 500
		}
	}
	res, err = (&countsReconcile{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Errorf("counts-reconcile is %s on a non-zero uncited fact, want pass -- "+
			"if it can see this, the check under test is a second spelling: %v",
			res.Status, res.Findings)
	}
}

// TestUncitedFactsArePrintedZerosAllowsTheSpinesStocks: the spine's beginning
// and ending working capital rows are balances behind no link, and are the one
// non-zero fact allowed uncited; a stock row re-classified as a flow is not.
func TestUncitedFactsArePrintedZerosAllowsTheSpinesStocks(t *testing.T) {
	s := testSubject(t)
	res, err := (&uncitedFactsArePrintedZeros{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass || res.Subjects == 0 {
		t.Fatalf("over the spine fixture the check is %s over %d, want pass over its stocks "+
			"and zero cells: %s", res.Status, res.Subjects, res.Summary)
	}
	if !strings.Contains(res.Summary, "stock") {
		t.Errorf("the summary does not say a stock row may go uncited: %s", res.Summary)
	}
	// A stock row filed under a flow category is money the spine dropped.
	s = testSubject(t)
	moved := false
	for i := range s.Facts {
		if s.Facts[i].Category == project.CategoryFundBalanceBeginning && s.Facts[i].AmountCents != 0 {
			s.Facts[i].Category = "fund-balance/change"
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("the fixture carries no non-zero beginning balance to re-classify")
	}
	res, err = (&uncitedFactsArePrintedZeros{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Errorf("a non-zero fact that is neither a stock nor cited passes as %s: %s",
			res.Status, res.Summary)
	}
}
