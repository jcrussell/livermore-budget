package mapping

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// TestQuantitiesIsTheClosedSet pins the vocabulary, the same way
// TestKindsIsTheClosedSet pins kinds: one list, read by valid() and
// quantityList() both, so a fifth value cannot be added to one spelling and
// missed by the other.
func TestQuantitiesIsTheClosedSet(t *testing.T) {
	want := []Quantity{QuantityAmount, QuantityAmountPerUnit, QuantityPercentage, QuantityNumber}
	if !slices.Equal(quantities, want) {
		t.Errorf("quantities = %v, want %v", quantities, want)
	}
	for _, q := range quantities {
		if !q.valid() {
			t.Errorf("quantities offers %q, which valid() rejects", q)
		}
	}
	for _, q := range []Quantity{"", "ratio", "Amount", "per_unit"} {
		if q.valid() {
			t.Errorf("valid(%q) = true, want false", q)
		}
	}
	// The list an error message spells excludes the default, which the parser
	// refuses to see written.
	if got, want := quantityList(), "amount_per_unit, percentage, number"; got != want {
		t.Errorf("quantityList() = %q, want %q", got, want)
	}
}

// TestEveryQuantityHasAGrammar walks the closed set through the resolver's
// dispatch, so a fifth value cannot be added to the vocabulary and left
// unrecognizable at read time.
func TestEveryQuantityHasAGrammar(t *testing.T) {
	for _, q := range quantities {
		if q == QuantityAmount {
			continue // the amount grammar is amount.Parse, not recognize
		}
		// "no grammar recognizes" is the harness-error arm; any other outcome
		// means the quantity reached a real grammar.
		if err := recognize(q, "definitely-not-a-figure"); err != nil &&
			strings.Contains(err.Error(), "no grammar recognizes") {
			t.Errorf("recognize(%q) has no grammar arm", q)
		}
	}
	if err := recognize(Quantity("banana"), "1"); err == nil ||
		!strings.Contains(err.Error(), "no grammar recognizes") {
		t.Errorf("recognize on an unknown quantity = %v, want the harness-error arm", err)
	}
}

func TestEffectiveQuantityPrecedence(t *testing.T) {
	col := Column{Quantity: QuantityPercentage}
	if got := (Row{}).EffectiveQuantity(col); got != QuantityPercentage {
		t.Errorf("column quantity: got %q, want %q", got, QuantityPercentage)
	}
	if got := (Row{Quantity: QuantityNumber}).EffectiveQuantity(col); got != QuantityNumber {
		t.Errorf("row override: got %q, want %q", got, QuantityNumber)
	}
	if got := (Row{}).EffectiveQuantity(Column{}); got != QuantityAmount {
		t.Errorf("default: got %q, want %q", got, QuantityAmount)
	}
}

// parseRuleErr parses an inline rule file and returns the error.
func parseRuleErr(t *testing.T, src string) error {
	t.Helper()
	_, err := parse(strings.NewReader(src), "inline.yaml")
	return err
}

// quantityRuleYAML builds a minimal rule around one substitution point, so
// each refusal test states only the declaration it is about.
func quantityRuleYAML(rows, columns string) string {
	return `schema_version: 1
doc_id: quantity-doc

rules:
  - id: quantity-rule
    kind: revenue
    basis: audited
    scope: quantity-scope
    grain: category
    units: dollars
    rows:
` + rows + `
    parts:
      - page: 7
        section: "Header"
        columns:
` + columns + "\n"
}

func TestQuantityDeclarationsAreValidated(t *testing.T) {
	amountRow := `      - {label: "Taxes", category: taxes/other}` + "\n"
	amountCol := `          - {fiscal_year: 2025}`

	cases := []struct {
		name string
		src  string
		want string // "" means the file must parse
	}{
		{
			name: "a non-amount column needs no fiscal_year and no category on its cells",
			src:  quantityRuleYAML(amountRow, amountCol+"\n          - {quantity: percentage}"),
		},
		{
			name: "a non-amount row needs no category",
			src: quantityRuleYAML(amountRow+`      - {label: "Ratio", quantity: percentage}`+"\n",
				amountCol),
		},
		{
			name: "explicit amount on a column is the default wearing ink",
			src:  quantityRuleYAML(amountRow, amountCol+"\n          - {quantity: amount}"),
			want: `quantity "amount" is the default`,
		},
		{
			name: "explicit amount on a row likewise",
			src: quantityRuleYAML(amountRow+`      - {label: "Ratio", category: taxes/sales, quantity: amount}`+"\n",
				amountCol),
			want: `quantity "amount" is the default`,
		},
		{
			name: "an unknown column quantity names the three declarable values",
			src:  quantityRuleYAML(amountRow, amountCol+"\n          - {quantity: ratio}"),
			want: "want one of amount_per_unit, percentage, number",
		},
		{
			name: "an unknown row quantity likewise",
			src: quantityRuleYAML(amountRow+`      - {label: "Ratio", quantity: ratio}`+"\n",
				amountCol),
			want: "want one of amount_per_unit, percentage, number",
		},
		{
			name: "a counterpart on a non-amount row has no figure to fan out",
			src: quantityRuleYAML(amountRow+
				`      - {label: "Ratio", quantity: percentage, counterpart: {category: transfers/in, kind: transfer_in, fund: 100, fund_group: general}}`+"\n",
				amountCol),
			want: "with a counterpart",
		},
		{
			name: "total_row over a non-amount column has no totals line to read",
			src: strings.Replace(
				quantityRuleYAML(amountRow, amountCol+"\n          - {quantity: percentage}"),
				"units: dollars", "units: dollars\n    total_row: \"Total\"", 1),
			want: `parses percentage, but the rule declares total_row "Total"`,
		},
		{
			name: "a stated_total_delta cannot describe a non-amount column",
			src: strings.Replace(
				quantityRuleYAML(amountRow, amountCol+"\n          - {quantity: percentage}"),
				"columns:", "stated_total_deltas:\n          - {column: 2, delta_cents: 100, note: \"n\"}\n        columns:", 1),
			want: "column 2 parses percentage, not amounts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseRuleErr(t, tc.src)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("parse: %v, want accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parse: %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// The row arm on a years-across page: p169's shape, inline because
// testdata/ carries no p169 pair yet (that copy is batch one's own work).
// A percentage row spans every year column, is read so the block stays whole,
// and publishes nothing.
const ratioRowPage = `Header
Taxes 100 200
Debt ratio 6.6% 4.9%
`

const ratioRowRule = `schema_version: 1
doc_id: ratio-doc

rules:
  - id: ratio-rule
    kind: revenue
    basis: audited
    scope: ratio-scope
    grain: fund-group-by-category
    units: dollars
    rows:
      - {label: "Taxes", category: taxes/other}
      - {label: "Debt ratio", quantity: percentage}
    parts:
      - page: 7
        section: "Header"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fund_group: general, fiscal_year: 2026}
`

func TestRowQuantityOverrideReadsWithoutPublishing(t *testing.T) {
	r, rule := scriptedResolver(t, ratioRowRule, map[int][]pageRead{
		7: {{text: ratioRowPage}},
	})
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	// Only the amount row publishes. The percentage row was READ — it is the
	// last row, so a refusal would have surfaced as an unparseable cell — and
	// yields no Value.
	var got []amount.Cents
	for _, v := range values {
		if v.Row.Label != "Taxes" {
			t.Errorf("row %q published a value; a non-amount row must not", v.Row.Label)
		}
		got = append(got, v.Cents)
	}
	if diff := cmp.Diff([]amount.Cents{100_00, 200_00}, got); diff != "" {
		t.Errorf("published cents mismatch (-want +got):\n%s", diff)
	}
}

// TestRowQuantityOverrideIsLoadBearing is the mutation, kept: remove the
// override and the read fails on amount.Parse rejecting the percentage — the
// exact blocker fisc-9tn4 records, shown still present rather than remembered.
func TestRowQuantityOverrideIsLoadBearing(t *testing.T) {
	src := strings.Replace(ratioRowRule, `, quantity: percentage`, ", category: taxes/other", 1)
	r, rule := scriptedResolver(t, src, map[int][]pageRead{
		7: {{text: ratioRowPage}},
	})
	_, _, err := r.Values(rule, &rule.Parts[0])
	if err == nil || !strings.Contains(err.Error(), `cannot parse amount "6.6%"`) {
		t.Fatalf("Values without the override: %v, want amount.Parse rejecting \"6.6%%\"", err)
	}
}

// TestQuantityCellsAreStillCells: a cell that fits no grammar fails the read
// even in a column that publishes nothing — quantity is a grammar, not a skip.
func TestQuantityCellsAreStillCells(t *testing.T) {
	page := strings.Replace(ratioRowPage, "6.6%", "NA", 1)
	r, rule := scriptedResolver(t, ratioRowRule, map[int][]pageRead{
		7: {{text: page}},
	})
	_, _, err := r.Values(rule, &rule.Parts[0])
	if err == nil || !strings.Contains(err.Error(), `cannot read "NA" as a percentage`) {
		t.Fatalf("Values over an NA cell: %v, want the percentage grammar refusing it", err)
	}
}
