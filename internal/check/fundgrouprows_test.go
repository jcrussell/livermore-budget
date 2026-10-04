package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestFundGroupsAreTheirPrintedRowsCanFail runs the check over the committed
// corpus, then over the same documents selecting without the cuts -- which
// lets in the Capital Improvement Program Funds block, whose CIP funds are
// typed enterprise and internal-service -- and then with one declared
// rounding dollar taken off a block total.
func TestFundGroupsAreTheirPrintedRowsCanFail(t *testing.T) {
	t.Parallel()
	s := committed(t)
	res := resultFor(t, runOne(t, s, &fundGroupsAreTheirPrintedRows{}), "fund-groups-are-their-printed-rows")
	if res.Status != StatusPass || res.Subjects != 4*7*8 {
		t.Fatalf("status %s over %d cells (%s), want a pass over 4 columns x 7 groups x 8 lines",
			res.Status, res.Subjects, res.Summary)
	}

	t.Run("selecting without the cuts", func(t *testing.T) {
		s := mutable(t)
		for i := range s.Projections {
			if s.Projections[i].Name == project.FundSourcesUsesProjection {
				s.Projections[i].Options.ThroughCuts = false
			}
		}
		res := resultFor(t, runOne(t, s, &fundGroupsAreTheirPrintedRows{}), "fund-groups-are-their-printed-rows")
		want := "FY2026 adopted enterprise revenues: the document's enterprise funds sum to $71,573,173.00 " +
			"and pp.186-209 print $67,514,261.00"
		if res.Status != StatusFail || !hasFinding(res, want) {
			t.Errorf("status %s, findings %v; want a finding starting %q", res.Status, res.Findings, want)
		}
	})

	t.Run("a declared rounding dollar dropped", func(t *testing.T) {
		// The delta is dropped from the rule's own row, and the resolver
		// memoizes rows, so this is a fresh load rather than a copy.
		s := isolated(t)
		dropped := false
		for _, f := range s.Files {
			for i := range f.Rules {
				for j := range f.Rules[i].Rows {
					row := &f.Rules[i].Rows[j]
					if f.Rules[i].ID == "fund-balances-fy2026-p0200" && row.Label == "Total Capital Funds" &&
						len(row.SubtotalDeltas) > 0 {
						row.SubtotalDeltas = nil
						dropped = true
					}
				}
			}
		}
		if !dropped {
			t.Fatal("fund-balances-fy2026-p0200 declares no delta on Total Capital Funds; the mutation changes nothing")
		}
		res := resultFor(t, runOne(t, s, &fundGroupsAreTheirPrintedRows{}), "fund-groups-are-their-printed-rows")
		if res.Status != StatusFail || !hasFinding(res, "FY2026 adopted capital fund-balance/beginning") {
			t.Errorf("status %s, findings %v; want FY2026 capital's beginning named", res.Status, res.Findings)
		}
	})
}

// TestAFundGroupFoldIsHeldToItsParentAndEveryLine plants the two defects a
// comparison over the printed cells alone would miss: a fund node parented
// to another group, and a fund line the summary prints no figure for.
func TestAFundGroupFoldIsHeldToItsParentAndEveryLine(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, plant func(s *Subject)) Result {
		t.Helper()
		s := isolated(t)
		plant(s)
		return resultFor(t, runOne(t, s, &fundGroupsAreTheirPrintedRows{}), "fund-groups-are-their-printed-rows")
	}
	t.Run("a fund parented to another group", func(t *testing.T) {
		res := run(t, func(s *Subject) {
			for _, p := range s.Projections {
				if p.Name != project.FundSourcesUsesProjection || p.Graph == nil {
					continue
				}
				for i := range p.Graph.Nodes {
					if p.Graph.Nodes[i].ID == "fund/200" {
						p.Graph.Nodes[i].Parent = project.PrefixFundGroup + "general"
					}
				}
			}
		})
		if res.Status != StatusFail || !strings.Contains(findingDetails(res), "fund/200 is parented to") {
			t.Errorf("status %s, findings %v; want fund/200's parent refused", res.Status, res.Findings)
		}
	})
	t.Run("a fund line no group row prints", func(t *testing.T) {
		res := run(t, func(s *Subject) {
			for i := range s.Facts {
				f := &s.Facts[i]
				if f.Scope == "fund-balances-by-fund" && f.FiscalYear == 2026 && f.Fund != nil &&
					*f.Fund == 200 && f.Category == "revenues" {
					f.Category = "intergovernmental"
				}
			}
		})
		if res.Status != StatusFail || !hasFinding(res, "FY2026 adopted special-revenue intergovernmental: "+
			"the document's special-revenue funds carry") {
			t.Errorf("status %s, findings %v; want the unprinted line named", res.Status, res.Findings)
		}
	})
}

func hasFinding(res Result, prefix string) bool {
	for _, f := range res.Findings {
		if strings.HasPrefix(f.Subject+": "+f.Detail, prefix) {
			return true
		}
	}
	return false
}
