package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestFundGroupSourcesEqualUsesIsFailable plants over the committed corpus.
func TestFundGroupSourcesEqualUsesIsFailable(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatal(err)
	}
	c := &fundGroupSourcesEqualUses{}
	res, err := c.Run(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusPass {
		t.Fatalf("the committed corpus is %s: %v", res.Status, res.Findings)
	}
	orig := s.Facts
	t.Cleanup(func() { s.Facts = orig })
	general2027 := func(f fact.Fact) bool {
		return f.Scope == "all-funds-gross" && f.FundGroup == "general" && f.FiscalYear == 2027
	}

	for _, tc := range []struct {
		name  string
		facts func() []fact.Fact
		want  string
	}{
		{"the FY2027 general fund balance column skipped", func() []fact.Fact {
			return slices.DeleteFunc(slices.Clone(orig), func(f fact.Fact) bool {
				return general2027(f) && f.RuleID == "spine-fund-balance"
			})
		}, "general FY2027 adopted: prints revenue or expenditure and no fund-balance/change"},
		{"one cent moved into a revenue row", func() []fact.Fact {
			facts := slices.Clone(orig)
			i := slices.IndexFunc(facts, func(f fact.Fact) bool {
				return general2027(f) && f.Kind == mapping.KindRevenue
			})
			facts[i].AmountCents++
			return facts
		}, "(off by $0.01)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.Facts = tc.facts()
			res, err := c.Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != StatusFail || len(res.Findings) != 1 ||
				!strings.Contains(res.Findings[0].Subject+": "+res.Findings[0].Detail, tc.want) {
				t.Fatalf("status %s, findings %v; want one fail saying %q", res.Status, res.Findings, tc.want)
			}
		})
	}
}
