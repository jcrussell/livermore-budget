package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestFundGroupSourcesEqualUsesIsFailable plants over the committed corpus.
func TestFundGroupSourcesEqualUsesIsFailable(t *testing.T) {
	t.Parallel()
	s := mutable(t)
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

	// only keeps, of general's FY2027 column, the facts keep says.
	only := func(keep func(fact.Fact) bool) func() []fact.Fact {
		return func() []fact.Fact {
			return slices.DeleteFunc(slices.Clone(orig), func(f fact.Fact) bool { return general2027(f) && !keep(f) })
		}
	}
	nudge := func(by int64) func() []fact.Fact {
		return func() []fact.Fact {
			facts := slices.Clone(orig)
			i := slices.IndexFunc(facts, func(f fact.Fact) bool {
				return general2027(f) && f.Kind == vocab.KindRevenue
			})
			facts[i].AmountCents += by
			return facts
		}
	}
	isChange := func(f fact.Fact) bool { return f.Category == project.CategoryFundBalanceChange }
	const missing = "all-funds-gross general FY2027 adopted: prints a line of sources = uses and is missing "
	const noChange = missing + "fund-balance/beginning, fund-balance/change, fund-balance/ending"

	for _, tc := range []struct {
		name  string
		facts func() []fact.Fact
		want  string
	}{
		{"the FY2027 general fund balance column skipped", func() []fact.Fact {
			return slices.DeleteFunc(slices.Clone(orig), func(f fact.Fact) bool {
				return general2027(f) && f.RuleID == "spine-fund-balance"
			})
		}, noChange},
		{"only revenue printed", only(func(f fact.Fact) bool { return f.Kind == vocab.KindRevenue }), noChange},
		{"only expenditure printed", only(func(f fact.Fact) bool { return f.Kind == vocab.KindExpenditure }), noChange},
		{"only transfers in printed", only(func(f fact.Fact) bool { return f.Kind == vocab.KindTransferIn }), noChange},
		{"only transfers out printed", only(func(f fact.Fact) bool { return f.Kind == vocab.KindTransferOut }), noChange},
		{"only the reserve increase printed", only(func(f fact.Fact) bool { return f.Category == project.CategoryFundBalanceReserveIncrease }), noChange},
		{"one cent moved into a revenue row", nudge(1), "(off by $0.01)"},
		{"one cent moved out of a revenue row", nudge(-1), "(off by -$0.01)"},
		{"a category that is no term", func() []fact.Fact {
			facts := slices.Clone(orig)
			i := slices.IndexFunc(facts, func(f fact.Fact) bool { return general2027(f) && isChange(f) })
			facts[i].Category = "fund-balance/other"
			return facts
		}, "which is no term of sources = uses"},
		{"the column's terms from another document than its change", func() []fact.Fact {
			facts := slices.Clone(orig)
			for i := range facts {
				if general2027(facts[i]) && !isChange(facts[i]) {
					facts[i].DocID = "other-doc"
				}
			}
			return facts
		}, "other-doc " + missing + "fund-balance/change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.Facts = tc.facts()
			res, err := c.Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			var said []string
			for _, f := range res.Findings {
				said = append(said, f.Subject+": "+f.Detail)
			}
			if res.Status != StatusFail || !strings.Contains(strings.Join(said, "\n"), tc.want) {
				t.Fatalf("status %s, findings %v; want a fail saying %q", res.Status, res.Findings, tc.want)
			}
		})
	}
}
