package mapping

import (
	"fmt"
	"testing"
)

// fundBalancePages is Budget Book pp.186-209, the fund-balance schedule's
// twelve page pairs, every one of which the published rules read.
func fundBalancePages() []int {
	var out []int
	for p := 186; p <= 209; p++ {
		out = append(out, p)
	}
	return out
}

func publishedFundBalances(t *testing.T) (*Resolver, *File) {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, fundBalancePages()...), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, f
}

// TestPublishedFundBalancesSummaryRowsAreTheirBlockTotals holds the relation a
// subtotal cannot: each year's summary block prints one row per fund group,
// and each is the printed "Total ..." of that group's detail block further
// down, in all ten columns. CheckSubtotals holds the summary rows to their own
// totals and each block total to its funds; this joins the two.
func TestPublishedFundBalancesSummaryRowsAreTheirBlockTotals(t *testing.T) {
	r, f := publishedFundBalances(t)
	type column struct{ part, col int }
	pairs := [][2]string{
		{"Special Revenue Funds", "Total Special Revenue Funds"},
		{"Debt Service Funds", "Total Debt Service Funds"},
		{"Permanent Funds", "Total Permanent Funds"},
		{"Capital Funds", "Total Capital Funds"},
		{"Enterprise Funds", "Total Enterprise Funds"},
		{"Internal Service", "Total Internal Service Funds"},
		{"Capital Improvement Program Funds", "Total Capital Improvement Program Funds"},
	}
	for year := 2024; year <= 2027; year++ {
		chain := fmt.Sprintf("fund-balances-fy%d", year)
		figures := map[string]map[column]Value{}
		rules := 0
		for i := range f.Rules {
			rule := &f.Rules[i]
			if rule.SubtotalChain != chain {
				continue
			}
			rules++
			for j := range rule.Parts {
				cells, err := r.Cells(rule, &rule.Parts[j])
				if err != nil {
					t.Fatalf("%s p%d: %v", rule.ID, rule.Parts[j].Page, err)
				}
				for _, c := range cells {
					label := c.Row.Label
					if label == "Internal Service Funds" {
						// FY2023-24 prints the summary row as "Internal Service
						// Funds", later years as "Internal Service".
						label = "Internal Service"
					}
					if figures[label] == nil {
						figures[label] = map[column]Value{}
					}
					figures[label][column{j, c.ColumnIndex}] = c
				}
			}
		}
		if rules != 4 {
			t.Errorf("chain %s has %d rules, want 4: one per page pair, the third read as "+
				"the funds above the Capital Improvement Program Funds and the block itself", chain, rules)
		}
		for _, pair := range pairs {
			summary, total := figures[pair[0]], figures[pair[1]]
			if len(summary) != 10 || len(total) != 10 {
				t.Errorf("FY%d: %q prints %d figures and %q %d, want 10 each",
					year, pair[0], len(summary), pair[1], len(total))
				continue
			}
			for k, s := range summary {
				if got := total[k]; got.Cents != s.Cents {
					t.Errorf("FY%d column %v: %q prints %s on p%d and %q prints %s on p%d",
						year, k, pair[0], s.Token, s.Page, pair[1], got.Token, got.Page)
				}
			}
		}
	}
}
