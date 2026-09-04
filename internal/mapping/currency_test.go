package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// dollarPerToken reproduces the shape of ACFR p177's FY2016 row: the dollar
// sign printed as its OWN whitespace-delimited token before each figure, with
// the mark and the figure separated by a run of spaces, and a "$ -" published
// zero among them.
//
// It is written inline rather than read from testdata/pages/acfr-p0177.txt for
// a reason that is not this bead's: p177's rows carry eleven columns and the
// tenth is a percentage ("2.5%"), which amount.Parse rejects as "not a
// recognized number". A rule cannot stop short of it either, because the text
// between one row's last mapped figure and the next row's label must be empty
// (checkGap). So the real page stays unreadable end to end until the percentage
// column has an answer -- recorded on fisc-4ua.4, which owns that page. The
// spacing below is copied from it, and the offset assertion is the same
// assertion either way.
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

func acfrDebtResolver(t *testing.T) (*Resolver, *Rule) {
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

// TestStandaloneDollarRowResolves is fisc-yun's acceptance criterion: a page
// whose rows print "$ 1,234" resolves, and the row printed WITHOUT the marks
// reads identically. Before this, the labelled read took [$, 60,193,384, $,
// 2,566,738] as its first four values and failed with `cannot parse amount
// "$"`.
//
// The "$            -" cell is the one worth naming: the mark is dropped and
// the dash is a PUBLISHED ZERO, not an absent cell. Conflating those is the
// invariant this project is built on.
func TestStandaloneDollarRowResolves(t *testing.T) {
	r, ru := acfrDebtResolver(t)
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

// TestOffsetStillPointsAtTheFigureWhenTheDollarIsItsOwnToken is the constraint
// that rules out the obvious fix.
//
// Joining "$" to the figure in the tokenizer would give the fact a Token of
// "$60,193,384" at the dollar sign's offset. The page bytes there are
// "$ 60,193,3", so fisc verify's fact-offset-points-at-token -- which asserts
// text[Offset:Offset+len(Token)] == Token -- would fail on every row of every
// one of the 236 pages that print the mark detached. Dropping the mark instead
// leaves the figure's own token pointing at itself, which it already did.
func TestOffsetStillPointsAtTheFigureWhenTheDollarIsItsOwnToken(t *testing.T) {
	r, ru := acfrDebtResolver(t)
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

// TestCurrencyMarkWithNoFigureIsAnError keeps the grammar fail-closed. A mark
// with nothing after it means the read has run off the end of the row, and
// laundering it into a shorter token list would hide exactly the overrun the
// column guard exists to catch.
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
