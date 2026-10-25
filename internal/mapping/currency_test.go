package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// dollarPerToken is ACFR p177's FY2016 row shape, spacing copied from the
// page: "$" as its own token before each figure, and a "$ -" published zero.
const dollarPerToken = `     Fiscal      Column A          Column B          Column C          Column D
     2016      $ 60,193,384     $    2,566,738    $            -   $     619,257
     2017        56,386,950          2,440,343                 -         512,946
`

const acfrP177DebtRow = `schema_version: 1
doc_id: livermore-acfr-fy2025

rules:
  - id: acfr-debt
    kind: expenditure
    basis: actual
    scope: acfr-statistical
    grain: fund-group-by-category
    units: dollars
    rows:
      - {label: "2016", category: debt-services}
      - {label: "2017", category: debt-services}
    parts:
      - page: 177
        section: "Column D"
        columns:
          - {fund_group: general, fiscal_year: 2016}
          - {fund_group: general, fiscal_year: 2017}
          - {fund_group: general, fiscal_year: 2018}
          - {fund_group: general, fiscal_year: 2019}
`

func dollarTokenResolver(t *testing.T) (*Resolver, *Rule) {
	t.Helper()
	f, err := parse(strings.NewReader(acfrP177DebtRow), "acfr-debt.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, "livermore-acfr-fy2025",
		map[int]string{177: dollarPerToken}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// TestStandaloneDollarRowResolves: rows printing "$ 1,234" resolve and read
// identically to the row without marks; "$            -" is a published zero,
// not an absent cell.
func TestStandaloneDollarRowResolves(t *testing.T) {
	r, ru := dollarTokenResolver(t)
	vals, _, err := r.Values(ru, &ru.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(vals) != 8 {
		t.Fatalf("got %d values, want 8 (two rows x four columns)", len(vals))
	}
	for i, want := range []amount.Cents{
		6019338400, 256673800, 0, 61925700,
		5638695000, 244034300, 0, 51294600,
	} {
		if got := vals[i].Cents; got != want {
			t.Errorf("value %d = %d cents, want %d", i, got, want)
		}
	}
}

// TestOffsetStillPointsAtTheFigureWhenTheDollarIsItsOwnToken: the mark is
// dropped rather than joined, so the figure's token still sits at its own
// offset and fact-offset-points-at-token holds.
func TestOffsetStillPointsAtTheFigureWhenTheDollarIsItsOwnToken(t *testing.T) {
	r, ru := dollarTokenResolver(t)
	vals, _, err := r.Values(ru, &ru.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if vals[0].Token != "60,193,384" {
		t.Fatalf("Token = %q, want %q -- the mark must be dropped, never joined",
			vals[0].Token, "60,193,384")
	}
	for _, v := range vals {
		got := dollarPerToken[v.Offset : v.Offset+len(v.Token)]
		if got != v.Token {
			t.Errorf("page[%d:%d] = %q, want %q; the offset must point at the figure",
				v.Offset, v.Offset+len(v.Token), got, v.Token)
		}
	}
}

// TestCurrencyMarkWithNoFigureIsAnError: a trailing mark means the read ran
// off the row, and is refused rather than dropped.
func TestCurrencyMarkWithNoFigureIsAnError(t *testing.T) {
	for _, tt := range []struct {
		name string
		toks []string
	}{
		{"mark at the end of the row", []string{"1,234", "$"}},
		{"two marks in a row", []string{"$", "$", "1,234"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := make([]token, len(tt.toks))
			for i, s := range tt.toks {
				in[i] = token{text: s, off: i}
			}
			if _, err := dropCurrencyMarks(in); err == nil {
				t.Fatal("dropCurrencyMarks succeeded; a mark with no figure after it must fail")
			}
		})
	}
}
