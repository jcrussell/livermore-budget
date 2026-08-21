package mapping

import (
	"strings"
	"testing"
)

// twoAnchorPage prints one row whose identity is two fields separated by a run
// of spaces, and a second row that shares the first field and differs only in
// the second -- which is why neither field alone can identify a row.
const twoAnchorPage = `  Header A   B
     Transfer From General Fund          to Stormwater      100     200
     Transfer From General Fund               to Airport    300     400
`

func twoAnchorRule(first, tail, second string) string {
	return `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: two-anchor
    kind: transfer_in
    basis: adopted
    scope: all-funds-gross
    units: dollars
    rows:
      - {label: "` + first + `", label_tail: "` + tail + `", category: "transfers/in"}
      - {label: "Transfer From General Fund", label_tail: "` + second + `", category: "transfers/in"}
    parts:
      - page: 76
        section: "Header A   B"
        columns:
          - {fund_group: general, fiscal_year: 2026}
          - {fund_group: general, fiscal_year: 2027}
`
}

func resolveTwoAnchor(t *testing.T, src string) ([]Value, error) {
	t.Helper()
	f, err := Parse(strings.NewReader(src), "two-anchor.yaml")
	if err != nil {
		return nil, err
	}
	r, err := NewResolver(inlineDoc(t, "livermore-budget-fy2026-2027",
		map[int]string{76: twoAnchorPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	vals, _, err := r.Values(&f.Rules[0], &f.Rules[0].Parts[0])
	return vals, err
}

// TestTwoAnchorsDoNotAssertTheGap is fisc-ffy's acceptance criterion. The two
// rows are separated from their second field by ten spaces and fifteen; the
// rule says neither number, and both resolve.
func TestTwoAnchorsDoNotAssertTheGap(t *testing.T) {
	vals, err := resolveTwoAnchor(t,
		twoAnchorRule("Transfer From General Fund", "to Stormwater", "to Airport"))
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(vals) != 4 {
		t.Fatalf("got %d values, want 4", len(vals))
	}
	// The two rows share a first anchor, so identity must come from the pair.
	if got := vals[0].Row.PrintedLabel(); got != "Transfer From General Fund to Stormwater" {
		t.Errorf("row_label = %q, want the pair joined by one space", got)
	}
	if vals[0].Cents != 10000 || vals[2].Cents != 30000 {
		t.Errorf("rows read in the wrong order: %d, %d", vals[0].Cents, vals[2].Cents)
	}
}

// TestTheGapBetweenAnchorsStaysCheckable is why this is a two-anchor match and
// not a wildcard. Whitespace between the fields is typesetting; a WORD between
// them means the two anchors matched different rows, and the read would file
// one row's figures under another row's identity.
func TestTheGapBetweenAnchorsStaysCheckable(t *testing.T) {
	// "to Airport" occurs only on the second printed row, so pairing it with
	// the first row's anchor spans a row boundary -- and the intervening text
	// is the first row's figures plus the second row's first field.
	_, err := resolveTwoAnchor(t,
		twoAnchorRule("Transfer From General Fund", "to Airport", "to Stormwater"))
	if err == nil {
		t.Fatal("resolved; anchors that span a row boundary must fail closed")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"sits between", "to Airport"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}

// TestMissingSecondAnchorFailsClosed covers the anchor that is not on the page
// at all.
func TestMissingSecondAnchorFailsClosed(t *testing.T) {
	_, err := resolveTwoAnchor(t,
		twoAnchorRule("Transfer From General Fund", "to Wastewater", "to Airport"))
	if err == nil {
		t.Fatal("resolved; a second anchor that is not printed must fail closed")
	}
	if got := diagnosis(t, err); !strings.Contains(got, "does not occur after it") {
		t.Errorf("error does not name the missing anchor:\n%s", got)
	}
}

// TestRowIdentityIsThePairNotTheLabel: two rows sharing a first anchor are
// legitimate, so uniqueness cannot be enforced on Label alone -- but two rows
// sharing BOTH anchors are still a duplicate identity.
func TestRowIdentityIsThePairNotTheLabel(t *testing.T) {
	_, err := resolveTwoAnchor(t,
		twoAnchorRule("Transfer From General Fund", "to Stormwater", "to Stormwater"))
	if err == nil {
		t.Fatal("parsed; two rows with the same pair are one identity")
	}
	if got := err.Error(); !strings.Contains(got, "duplicate row label") {
		t.Errorf("error does not report a duplicate:\n%s", got)
	}
}
