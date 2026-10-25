package mapping

import (
	"strings"
	"testing"
)

// leadingGapPage prints an all-dash line between the section anchor and the
// first mapped row. The dashes are printed zeros, so the line is a row the
// rule has not mapped, and the totals tie without it.
const leadingGapPage = `HEAD
Unmapped Line - -
Alpha 10 20
Beta 1 2
Total 11 22
`

const leadingGapRule = `schema_version: 1
doc_id: leading-gap-doc

rules:
  - id: leading-gap
    kind: revenue
    basis: adopted
    scope: all-funds-gross
    grain: fund-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Alpha", category: charges-for-services}
      - {label: "Beta", category: intergovernmental}
    parts:
      - page: 1
        section: "HEAD"
        stop_at: "Total"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2026}
          - {fund_group: general, fund: 100, fiscal_year: 2027}
`

func leadingGapValues(t *testing.T, page string) error {
	t.Helper()
	f, err := parse(strings.NewReader(leadingGapRule), "leading-gap.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{1: page}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	rule := &f.Rules[0]
	_, _, err = r.Values(rule, &rule.Parts[0])
	return err
}

// TestAnAllDashLineBeforeTheFirstRowIsRefused: a line of printed zeros is a
// row. Mutation: delete checkGap's dash arm and this goes red.
func TestAnAllDashLineBeforeTheFirstRowIsRefused(t *testing.T) {
	if err := leadingGapValues(t, strings.Replace(leadingGapPage, "Unmapped Line - -\n", "", 1)); err != nil {
		t.Fatalf("the page without the dash line must resolve, or the refusal below is vacuous: %v", err)
	}
	for _, line := range []string{"Unmapped Line - -", "Unmapped Line $ -", "Unmapped Line —"} {
		err := leadingGapValues(t, strings.Replace(leadingGapPage, "Unmapped Line - -", line, 1))
		if err == nil {
			t.Errorf("%q before the first row resolved; a printed zero line was dropped", line)
			continue
		}
		if !strings.Contains(err.Error(), "before the first row") {
			t.Errorf("%q refused for some other reason: %v", line, err)
		}
	}
	// A header of words is still the leading gap's to carry.
	if err := leadingGapValues(t, strings.Replace(leadingGapPage, "Unmapped Line - -", "Actual Revised", 1)); err != nil {
		t.Errorf("a header line of words was refused: %v", err)
	}
}

// TestAnUnguardedLabelledReadNeedsWholeWordsAndItsOwnLine. Mutation: search
// the label with strings.Index and Alpha reads "Alphabet"'s figures; drop the
// same-line arm and Alpha takes Beta's first figure.
func TestAnUnguardedLabelledReadNeedsWholeWordsAndItsOwnLine(t *testing.T) {
	base := strings.Replace(leadingGapPage, "Unmapped Line - -\n", "", 1)
	if err := leadingGapValues(t, base); err != nil {
		t.Fatalf("the base page must resolve, or the refusals below are vacuous: %v", err)
	}
	for _, tt := range []struct{ name, page, want string }{
		{"a label inside a longer word",
			strings.Replace(base, "Alpha 10 20", "Alphabet 10 20", 1),
			`row "Alpha" does not occur`},
		{"a short row",
			strings.NewReplacer("Alpha 10 20\nBeta 1 2", "Alpha 10\n20 Beta 1 2").Replace(base),
			`row "Alpha" prints fewer than 2 values on its own line`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := leadingGapValues(t, tt.page)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
	// A label ending in punctuation still matches where a word follows it.
	if got := indexWord("x Total: 5", 0, "Total:"); got != 2 {
		t.Errorf("indexWord found %q at %d, want 2", "Total:", got)
	}
}
