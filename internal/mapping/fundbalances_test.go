package mapping

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
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

// fundBalanceCell is one published figure of a fund-balance row: the line its
// column carries and the token the page prints.
type fundBalanceCell struct {
	Page     int
	Category string
	Kind     Kind
	Token    string
}

// fundBalanceRow reads one row of one published pp.186-209 rule: every value
// it publishes, and the cells its rule declares blank.
func fundBalanceRow(t *testing.T, r *Resolver, f *File, ruleID, label string) (Row, []fundBalanceCell, []omittedCellAt) {
	t.Helper()
	var rule *Rule
	for i := range f.Rules {
		if f.Rules[i].ID == ruleID {
			rule = &f.Rules[i]
		}
	}
	if rule == nil {
		t.Fatalf("no published rule %s", ruleID)
	}
	var row Row
	found := false
	var cells []fundBalanceCell
	var omitted []omittedCellAt
	for i := range rule.Parts {
		vals, omissions, err := r.Values(rule, &rule.Parts[i])
		if err != nil {
			t.Fatalf("%s p%d: %v", ruleID, rule.Parts[i].Page, err)
		}
		for _, v := range vals {
			if v.Row.Label != label {
				continue
			}
			row, found = v.Row, true
			cells = append(cells, fundBalanceCell{v.Page, v.Category(), v.Kind(rule), v.Token})
		}
		for _, o := range omittedCellsAt(omissions) {
			if o.Row == label {
				omitted = append(omitted, o)
			}
		}
	}
	if !found {
		t.Fatalf("%s publishes no figure on %q", ruleID, label)
	}
	return row, cells, omitted
}

// fundBalanceLine spells a row's eight published figures in the order the
// page pair prints them: the even page's beginning balance, Revenues and
// Transfers In, then the odd page's Expenses, Transfers Out, Transfers Out to
// CIP, Reserve Increase/(Use) and ending balance. An empty token is a cell the
// page leaves blank, which publishes nothing.
func fundBalanceLine(even int, tokens ...string) []fundBalanceCell {
	lines := []struct {
		category string
		kind     Kind
	}{
		{"fund-balance/beginning", KindFundBalance},
		{"revenues", KindRevenue},
		{"transfers/in", KindTransferIn},
		{"expenses", KindExpenditure},
		{"transfers/out", KindTransferOut},
		{"transfers/out-to-cip", KindTransferOut},
		{"fund-balance/reserve-increase", KindFundBalance},
		{"fund-balance/ending", KindFundBalance},
	}
	var out []fundBalanceCell
	for i, tok := range tokens {
		if tok == "" {
			continue
		}
		page := even
		if i >= 3 {
			page = even + 1
		}
		out = append(out, fundBalanceCell{page, lines[i].category, lines[i].kind, tok})
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

// TestPublishedFundBalancesGeneralFund reads the General Fund off pp.186-187,
// the summary row that publishes because the General Fund has no detail block.
// Its line holds: 14,440,690 + 142,028,002 + 737,455 = 157,206,147 sources,
// less 142,465,367 uses, is the 14,740,780 the page prints as its ending.
func TestPublishedFundBalancesGeneralFund(t *testing.T) {
	r, f := publishedFundBalances(t)
	row, got, _ := fundBalanceRow(t, r, f, "fund-balances-fy2024-p0186", "General Fund")
	want := fundBalanceLine(186, "14,440,690", "142,028,002", "737,455",
		"123,228,190", "14,507,398", "440,846", "4,288,933", "14,740,780")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("General Fund FY2024 (-want +got):\n%s", diff)
	}
	if row.Fund != 100 || row.FundGroup != "general" {
		t.Errorf("General Fund publishes as fund %d of %q, want 100 of general", row.Fund, row.FundGroup)
	}
	sources := amount.Cents(1444069000 + 14202800200 + 73745500)
	uses := amount.Cents(12322819000 + 1450739800 + 44084600 + 428893300)
	if sources-uses != 1474078000 {
		t.Errorf("sources %s - uses %s = %s, want the printed 14,740,780", sources, uses, sources-uses)
	}
}

// TestPublishedFundBalancesOneFundPerBlock reads one fund of each FY2026-27
// detail block, each figure transcribed from pp.204-209. CIP Fleet & Equipment
// Svcs is fund 731, an internal service fund among the CIP funds, read through
// an alias.
func TestPublishedFundBalancesOneFundPerBlock(t *testing.T) {
	r, f := publishedFundBalances(t)
	for _, tc := range []struct {
		rule, label string
		fund        int
		group       string
		want        []fundBalanceCell
	}{
		{"fund-balances-fy2027-p0204", "Low Income Housing Fund", 200, "special-revenue",
			fundBalanceLine(204, "47,148,680", "9,308,228", "-", "2,591,476", "-", "-", "-", "53,865,432")},
		{"fund-balances-fy2027-p0206", "2022 COPS", 402, "debt-service",
			fundBalanceLine(206, "2", "-", "2,557,425", "2,557,425", "-", "-", "-", "2")},
		{"fund-balances-fy2027-p0206", "Doolan Canyon Preserve Endow", 470, "permanent",
			fundBalanceLine(206, "-", "-", "-", "-", "-", "-", "-", "-")},
		{"fund-balances-fy2027-p0206", "State - SB1", 561, "capital",
			fundBalanceLine(206, "1,159,427", "2,542,651", "-", "2,000", "-", "4,250,000", "-", "(549,922)")},
		{"fund-balances-fy2027-p0208-above-cip", "Water Connection Fees", 643, "enterprise",
			fundBalanceLine(208, "620,374", "946,000", "-", "13,637", "-", "-", "-", "1,552,737")},
		{"fund-balances-fy2027-p0208-above-cip", "Facilities Rehab Pgm", 740, "internal-service",
			fundBalanceLine(208, "1,677,100", "2,125,000", "-", "3,785,705", "-", "-", "-", "16,395")},
		{"fund-balances-fy2027-p0208", "CIP Fleet & Equipment Svcs", 731, "internal-service",
			fundBalanceLine(208, "-", "1,350,000", "5,074,000", "6,424,000", "-", "-", "-", "-")},
	} {
		t.Run(tc.label, func(t *testing.T) {
			row, got, omitted := fundBalanceRow(t, r, f, tc.rule, tc.label)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			if row.Fund != tc.fund || row.FundGroup != tc.group {
				t.Errorf("publishes as fund %d of %q, want %d of %q", row.Fund, row.FundGroup, tc.fund, tc.group)
			}
			if len(omitted) != 0 {
				t.Errorf("declares %v blank, and the page prints every cell", omitted)
			}
		})
	}
}

// TestPublishedFundBalancesCountyMeasureDHasNoReserve: p207 prints County
// Measure D's FY2026-27 line as 505,784 | - | - | (blank) | 505,784 |
// (858,450), so it publishes seven figures and no Reserve Increase/(Use),
// and the rule reports the blank. (358,666) + 6,000 - 505,784 = (858,450).
func TestPublishedFundBalancesCountyMeasureDHasNoReserve(t *testing.T) {
	r, f := publishedFundBalances(t)
	_, got, omitted := fundBalanceRow(t, r, f, "fund-balances-fy2027-p0206", "County Measure D")
	want := fundBalanceLine(206, "(358,666)", "6,000", "-", "505,784", "-", "-", "", "(858,450)")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("County Measure D FY2027 (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]omittedCellAt{{"County Measure D", 207, 3, "Increase/(Use)"}},
		omitted); diff != "" {
		t.Errorf("omissions (-want +got):\n%s", diff)
	}
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
