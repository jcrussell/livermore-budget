package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// countedSubject is scheduleSubject with the three newer documents' counts
// filled in as the producers would publish them over its facts.
func countedSubject() *Subject {
	s := scheduleSubject()
	for i := range s.Projections {
		p := &s.Projections[i]
		switch {
		case p.DepartmentSpending != nil:
			p.DepartmentSpending.Metadata.Counts = struct {
				Facts        int `json:"facts"`
				FactsCited   int `json:"facts_cited"`
				FactsUncited int `json:"facts_uncited"`
				Nodes        int `json:"nodes"`
				Links        int `json:"links"`
			}{2, 1, 1, 2, 1}
		case p.DepartmentFunding != nil:
			p.DepartmentFunding.Metadata.Counts = struct {
				Facts        int `json:"facts"`
				FactsCited   int `json:"facts_cited"`
				FactsUncited int `json:"facts_uncited"`
				Nodes        int `json:"nodes"`
				Links        int `json:"links"`
			}{2, 1, 1, 2, 1}
		case p.TransfersByFund != nil:
			// One link is half a movement; the fixture's transfers figure is
			// links/2 rounded down, which is what the check re-derives.
			p.TransfersByFund.Metadata.Counts = struct {
				Facts        int `json:"facts"`
				FactsCited   int `json:"facts_cited"`
				FactsUncited int `json:"facts_uncited"`
				Transfers    int `json:"transfers"`
				Nodes        int `json:"nodes"`
				Links        int `json:"links"`
			}{2, 1, 1, 0, 2, 1}
		}
	}
	return s
}

// TestScheduleCountsReconcileIsFailable damages each published count of each
// of the three shapes in turn, the way fund-flows-counts-reconcile's test does.
func TestScheduleCountsReconcileIsFailable(t *testing.T) {
	c := &scheduleCountsReconcile{}

	res, err := c.Run(t.Context(), countedSubject())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass || res.Subjects != 3 {
		t.Fatalf("the undamaged subject is %s over %d, want pass over 3 documents: %v",
			res.Status, res.Subjects, res.Findings)
	}

	cases := []struct {
		name   string
		damage func(s *Subject)
		want   string
	}{
		{"department-spending facts", func(s *Subject) {
			s.Projections[1].DepartmentSpending.Metadata.Counts.Facts = 4
		}, "counts.facts is 4 and re-derives to 2"},
		{"department-spending facts_cited", func(s *Subject) {
			s.Projections[1].DepartmentSpending.Metadata.Counts.FactsCited = 2
		}, "counts.facts_cited is 2 and re-derives to 1"},
		{"department-spending facts_uncited", func(s *Subject) {
			s.Projections[1].DepartmentSpending.Metadata.Counts.FactsUncited = 0
		}, "counts.facts_uncited is 0 and re-derives to 1"},
		{"department-funding nodes", func(s *Subject) {
			s.Projections[2].DepartmentFunding.Metadata.Counts.Nodes = 9
		}, "counts.nodes is 9 and re-derives to 2"},
		{"department-funding links", func(s *Subject) {
			s.Projections[2].DepartmentFunding.Metadata.Counts.Links = 9
		}, "counts.links is 9 and re-derives to 1"},
		{"transfers-by-fund transfers", func(s *Subject) {
			s.Projections[3].TransfersByFund.Metadata.Counts.Transfers = 7
		}, "counts.transfers is 7 and re-derives to 0"},
		{
			// The case that matters most: a citation the document dropped,
			// with the link's value untouched.
			name: "a dropped citation",
			damage: func(s *Subject) {
				s.Projections[3].TransfersByFund.Links[0].FactIDs = nil
			},
			want: "counts.facts_cited is 1 and re-derives to 0",
		},
		{
			// Facts says 3 and cited + uncited still says 2, so the facts
			// comparison and the identity both fire; the identity's is the
			// message about the claim.
			name: "the identity against the document's own two numbers",
			damage: func(s *Subject) {
				s.Projections[1].DepartmentSpending.Metadata.Counts.Facts = 3
			},
			want: "do not add up to the third",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := countedSubject()
			tc.damage(s)
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

// TestScheduleCountsReconcileReadsThreeShapes pins which documents the check
// is of: fund-flows has a counts check of its own and the spine has
// counts-reconcile, so a subject carrying only those two is vacuous here.
func TestScheduleCountsReconcileReadsThreeShapes(t *testing.T) {
	s := countedSubject()
	s.Projections = s.Projections[:1]
	if s.Projections[0].FundFlows == nil {
		t.Fatal("the first fixture projection is not the fund-flows one")
	}
	res, err := (&scheduleCountsReconcile{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Errorf("over a fund-flows document alone the check is %s, want vacuous", res.Status)
	}
	_ = project.FundFlowsProjection
}
