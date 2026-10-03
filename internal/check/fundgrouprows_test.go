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
	load := func() *Subject {
		t.Helper()
		s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return s
	}
	s := load()
	res := resultFor(t, runOne(t, s, &fundGroupsAreTheirPrintedRows{}), "fund-groups-are-their-printed-rows")
	if res.Status != StatusPass || res.Subjects != 4*7*8 {
		t.Fatalf("status %s over %d cells (%s), want a pass over 4 columns x 7 groups x 8 lines",
			res.Status, res.Subjects, res.Summary)
	}

	t.Run("selecting without the cuts", func(t *testing.T) {
		s := load()
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
		s := load()
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

func hasFinding(res Result, prefix string) bool {
	for _, f := range res.Findings {
		if strings.HasPrefix(f.Subject+": "+f.Detail, prefix) {
			return true
		}
	}
	return false
}
