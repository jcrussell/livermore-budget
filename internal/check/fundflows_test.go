package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// fundFlowsSubject is a drill-down and the facts it claims to be of: two facts
// behind one link, plus a printed zero behind none.
func fundFlowsSubject() *Subject {
	col := project.Column{FiscalYear: 2026, Basis: mapping.BasisAdopted}
	mk := func(id, scope string, cents int64) fact.Fact {
		return fact.Fact{ID: id, Scope: scope, Kind: mapping.KindRevenue,
			Category: "taxes/property", FundGroup: "general", Fund: 100,
			FiscalYear: col.FiscalYear, Basis: col.Basis, AmountCents: cents}
	}
	facts := []fact.Fact{
		mk("a", project.ScopeRevenueByFund, 100),
		mk("b", project.ScopeRevenueByFund, 200),
		mk("c", project.ScopeRevenueByFund, 0),
	}
	doc := &project.FundFlowsDocument{
		Nodes: []project.Node{{ID: "revenue/taxes/property"}, {ID: "fund/100"}},
		Links: []project.Link{{
			Source: "revenue/taxes/property", Target: "fund/100",
			ValueCents: 300, FactIDs: []string{"a", "b"},
		}},
		Metadata: project.FundFlowsMetadata{Counts: project.FundFlowsCounts{
			Facts: 3, FactsCited: 2, FactsUncited: 1, FactsCitedTwice: 0, Nodes: 2, Links: 1,
		}},
	}
	return &Subject{
		Facts: facts,
		Projections: []Projection{{
			Name: project.FundFlowsProjection,
			Options: project.Options{Columns: []project.Column{col},
				Scopes: project.FundFlowsScopes(), Version: testVersion},
			FundFlows: doc,
		}},
	}
}

// TestFundFlowsCountsReconcileIsFailable damages each published count in turn.
//
// EVERY CASE IS A NUMBER THE DOCUMENT PUBLISHES ABOUT ITSELF, and the check's
// whole value is that it re-derives them from the LINKS rather than by re-running
// the projection's arithmetic. A check that re-netted the same facts through the
// same rule would agree with the projection whatever the file said; this one
// disagrees the moment a citation is wrong, which is the case the spine's
// counts-reconcile cannot reach at all.
func TestFundFlowsCountsReconcileIsFailable(t *testing.T) {
	c := &fundFlowsCountsReconcile{}

	if res, err := c.Run(t.Context(), fundFlowsSubject()); err != nil {
		t.Fatalf("Run: %v", err)
	} else if res.Status != StatusPass {
		t.Fatalf("the undamaged document is %s, want pass: %v", res.Status, res.Findings)
	}

	cases := []struct {
		name   string
		damage func(*project.FundFlowsDocument)
		want   string
	}{
		{"facts", func(d *project.FundFlowsDocument) { d.Metadata.Counts.Facts = 4 },
			"counts.facts is 4 and re-derives to 3"},
		{"facts_cited", func(d *project.FundFlowsDocument) { d.Metadata.Counts.FactsCited = 3 },
			"counts.facts_cited is 3 and re-derives to 2"},
		{"facts_uncited", func(d *project.FundFlowsDocument) { d.Metadata.Counts.FactsUncited = 0 },
			"counts.facts_uncited is 0 and re-derives to 1"},
		{"facts_cited_twice", func(d *project.FundFlowsDocument) {
			d.Metadata.Counts.FactsCitedTwice = 1
		}, "counts.facts_cited_twice is 1 and re-derives to 0"},
		{"nodes", func(d *project.FundFlowsDocument) { d.Metadata.Counts.Nodes = 9 },
			"counts.nodes is 9"},
		{"links", func(d *project.FundFlowsDocument) { d.Metadata.Counts.Links = 9 },
			"counts.links is 9"},
		{
			// The case that matters most: a citation the document dropped. The
			// link's VALUE is untouched and link-values-tie-to-facts would go
			// red on it -- but only because the two are of one set, which is
			// itself the property under test here.
			name: "a dropped citation",
			damage: func(d *project.FundFlowsDocument) {
				d.Links[0].FactIDs = []string{"a"}
			},
			want: "counts.facts_cited is 2 and re-derives to 1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fundFlowsSubject()
			tc.damage(s.Projections[0].FundFlows)
			res, err := c.Run(t.Context(), s)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want fail", res.Status)
			}
			if !strings.Contains(findingDetails(res), tc.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tc.want)
			}
		})
	}
}

// TestTheFundFlowsIdentityIsCheckedAgainstItsOwnTwoNumbers is the arm that is
// about the document's INTERNAL consistency rather than about the corpus.
//
// The comparisons above say whether each count is right; this says whether the
// two the document publishes agree with each other, which is the claim a reader
// subtracting one from the other is relying on. Damaging both by the same amount
// keeps each wrong-but-consistent, so only the re-derivation catches it -- and
// damaging one alone leaves the identity broken, so only this arm does.
func TestTheFundFlowsIdentityIsCheckedAgainstItsOwnTwoNumbers(t *testing.T) {
	s := fundFlowsSubject()
	d := s.Projections[0].FundFlows
	// Facts says 4, cited + uncited still says 3. Both the facts comparison and
	// the identity fire, and the identity's message is the one about the claim.
	d.Metadata.Counts.Facts = 4
	res, err := (&fundFlowsCountsReconcile{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(findingDetails(res), "published identity") {
		t.Errorf("findings %v do not report the identity itself", res.Findings)
	}
}
