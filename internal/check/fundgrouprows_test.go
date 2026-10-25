package check

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestFundGroupsAreTheirPrintedRowsCanFail runs the check over the committed
// corpus, then without the cuts (letting in the CIP funds block), then with one
// declared rounding dollar taken off a block total.
func TestFundGroupsAreTheirPrintedRowsCanFail(t *testing.T) {
	t.Parallel()
	s := committed(t)
	res := runCheck(t, &fundGroupsAreTheirPrintedRows{}, s)
	wantPass(t, res, 4*7*8)

	t.Run("selecting without the cuts", func(t *testing.T) {
		s := mutable(t)
		for i := range s.Projections {
			if s.Projections[i].Name == project.FundSourcesUsesProjection {
				s.Projections[i].Options.ThroughCuts = false
			}
		}
		res := runCheck(t, &fundGroupsAreTheirPrintedRows{}, s)
		want := "FY2026 adopted enterprise revenues: the document's enterprise funds sum to $71,573,173.00 " +
			"and pp.186-209 print $67,514,261.00"
		wantFail(t, res, want)
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
		res := runCheck(t, &fundGroupsAreTheirPrintedRows{}, s)
		wantFail(t, res, "FY2026 adopted capital fund-balance/beginning")
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
		return runCheck(t, &fundGroupsAreTheirPrintedRows{}, s)
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
		wantFail(t, res, "fund/200 is parented to")
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
