package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// scheduleSubject is one document of each of the four schedule shapes, each
// built over two facts of its own scope: one behind a link and one uncited
// printed zero. It is assembled by hand rather than through the producers,
// because every producer refuses a non-zero uncited fact at build time and a
// test that went through one would be green because the gate fired earlier.
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
	return &Subject{
		Facts: facts,
		Projections: []projection{
			{Name: project.FundFlowsProjection, Options: options(project.FundFlowsScopes()),
				FundFlows: &project.FundFlowsDocument{Nodes: nodes, Links: link("ff-a"),
					Metadata: project.FundFlowsMetadata{Counts: project.FundFlowsCounts{
						Facts: 2, FactsCited: 1, FactsUncited: 1, Nodes: 2, Links: 1}}}},
			{Name: project.DepartmentSpendingProjection, Options: options(project.DepartmentSpendingScopes()),
				DepartmentSpending: &project.DepartmentSpendingDocument{Nodes: nodes, Links: link("ds-a")}},
			{Name: project.DepartmentFundingProjection, Options: options(project.DepartmentFundingScopes()),
				DepartmentFunding: &project.DepartmentFundingDocument{Nodes: nodes, Links: link("df-a")}},
			{Name: project.TransfersByFundProjection, Options: options(project.TransfersByFundScopes()),
				TransfersByFund: &project.TransfersByFundDocument{Nodes: nodes, Links: link("tb-a")}},
		},
	}
}

// TestUncitedFactsArePrintedZerosIsFailable plants one non-zero uncited fact
// in each of the four shapes in turn.
//
// EACH CASE IS ONE THE COUNTS CHECKS CANNOT SEE: the document's facts,
// facts_cited and facts_uncited are all still right, because uncited is
// counted and not valued. The only thing that has moved is what the dropped
// fact is worth.
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

	// THE COUNTS CHECK IS GREEN ON THE SAME DAMAGE, which is the claim this
	// check exists for: the drill-down's facts, facts_cited and facts_uncited
	// all still re-derive, because the dropped fact is still uncited and
	// uncited is a count.
	s := scheduleSubject()
	for i := range s.Facts {
		if s.Facts[i].ID == "ff-z" {
			s.Facts[i].AmountCents = 500
		}
	}
	res, err = (&fundFlowsCountsReconcile{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Errorf("fund-flows-counts-reconcile is %s on a non-zero uncited fact, want pass -- "+
			"if it can see this, the check under test is a second spelling: %v",
			res.Status, res.Findings)
	}
}

// TestUncitedFactsArePrintedZerosSkipsTheSpine pins that the spine is not read:
// its fund balance rows are facts behind no link and are stocks, not zeros,
// and counts-reconcile is the check of that identity.
func TestUncitedFactsArePrintedZerosSkipsTheSpine(t *testing.T) {
	s := testSubject(t)
	res, err := (&uncitedFactsArePrintedZeros{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Errorf("over the spine-only fixture the check is %s, want vacuous: %s",
			res.Status, res.Summary)
	}
}
