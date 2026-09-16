package mapping

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
    grain: fund-group-by-category
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

func readTwoAnchor(t *testing.T, src string) ([]Value, []Omission, error) {
	t.Helper()
	f, err := parse(strings.NewReader(src), "two-anchor.yaml")
	if err != nil {
		return nil, nil, err
	}
	r, err := NewResolver(inlineDoc(t, "livermore-budget-fy2026-2027",
		map[int]string{76: twoAnchorPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r.Values(&f.Rules[0], &f.Rules[0].Parts[0])
}

func resolveTwoAnchor(t *testing.T, src string) ([]Value, error) {
	t.Helper()
	vals, _, err := readTwoAnchor(t, src)
	return vals, err
}

// omissionRule lists THREE transfers out of one fund against the two-row page
// above, so the middle one -- "to Wastewater" -- is a row the rule carries and
// the page does not print. All three share a Label, which is the point: the
// declaration has to name one of them without naming the other two.
func omissionRule(omitted string) string {
	return `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: two-anchor
    kind: transfer_in
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    rows:
      - {label: "Transfer From General Fund", label_tail: "to Stormwater", category: "transfers/in"}
      - {label: "Transfer From General Fund", label_tail: "to Wastewater", category: "transfers/in"}
      - {label: "Transfer From General Fund", label_tail: "to Airport", category: "transfers/in"}
    parts:
      - page: 76
        section: "Header A   B"
        omitted_rows: [` + omitted + `]
        columns:
          - {fund_group: general, fiscal_year: 2026}
          - {fund_group: general, fiscal_year: 2027}
`
}

const omitWastewater = `{label: "Transfer From General Fund", label_tail: "to Wastewater"}`

// TestATwoAnchorRowIsOmittedByNamingBothAnchors is fisc-gtv (1) and (2).
//
// The row index is keyed on Row.Identity(), which joins the two anchors on a
// \x1f. While omitted_rows was a list of bare labels, NO value an author could
// type matched a two-anchor row: the declaration OmittedRows calls mandatory
// could not be written at all for the pages label_tail exists to map.
func TestATwoAnchorRowIsOmittedByNamingBothAnchors(t *testing.T) {
	vals, omitted, err := readTwoAnchor(t, omissionRule(omitWastewater))
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(vals) != 4 {
		t.Fatalf("got %d values, want 4 (2 printed rows × 2 columns)", len(vals))
	}
	for _, want := range []struct {
		at, rowIndex int
		label        string
	}{
		{0, 0, "Transfer From General Fund to Stormwater"},
		{2, 2, "Transfer From General Fund to Airport"},
	} {
		v := vals[want.at]
		if got := v.Row.PrintedLabel(); got != want.label {
			t.Errorf("value %d is row %q, want %q", want.at, got, want.label)
		}
		// RowIndex indexes the RULE's rows, so the surviving second row is 2
		// and not 1. Pairing active rows to rule rows on the bare Label gives
		// it the OMITTED row's position, and a consumer laying values and
		// omissions out together then puts two rows in one slot.
		if v.RowIndex != want.rowIndex {
			t.Errorf("value %d has RowIndex %d, want %d", want.at, v.RowIndex, want.rowIndex)
		}
	}
	if len(omitted) != 1 {
		t.Fatalf("got %d omissions, want 1: %+v", len(omitted), omitted)
	}
	if got := omitted[0].Row.PrintedLabel(); got != "Transfer From General Fund to Wastewater" {
		t.Errorf("omitted row is %q", got)
	}
	// The omission indexes the RULE's rows, and the omitted row is the middle
	// one. A Label-keyed pairing in canonicalRows would have handed the
	// surviving rows the wrong indices too.
	if omitted[0].RowIndex != 1 {
		t.Errorf("omitted RowIndex = %d, want 1", omitted[0].RowIndex)
	}
}

// TestActiveRowsDropOnlyTheRowNamed is the trap under the obvious fix for the
// test above: ActiveRows filtered on the bare Label, so one declaration would
// have dropped every row sharing it -- here, the whole rule.
func TestActiveRowsDropOnlyTheRowNamed(t *testing.T) {
	f, err := parse(strings.NewReader(omissionRule(omitWastewater)), "two-anchor.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rule := &f.Rules[0]
	var got []string
	for _, row := range rule.ActiveRows(&rule.Parts[0]) {
		got = append(got, row.PrintedLabel())
	}
	want := []string{
		"Transfer From General Fund to Stormwater",
		"Transfer From General Fund to Airport",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ActiveRows (-want +got):\n%s", diff)
	}
}

// TestOmittedRowMustNameExactlyOneRow is the fail-closed direction. An entry
// that names no row has always been refused; an entry that names more than one
// has to be refused for the same reason, since silently dropping the rows an
// author did not mean to omit is the mismapping the declaration exists to
// prevent.
func TestOmittedRowMustNameExactlyOneRow(t *testing.T) {
	tests := []struct {
		name    string
		omitted string
		want    []string
	}{
		{
			name:    "the bare label names all three",
			omitted: `"Transfer From General Fund"`,
			want:    []string{"names 3 rows", "label_tail"},
		},
		{
			name:    "a pair that is not a row",
			omitted: `{label: "Transfer From General Fund", label_tail: "to Nowhere"}`,
			want:    []string{"Transfer From General Fund to Nowhere", "not one of this rule's rows"},
		},
		{
			name:    "declared twice",
			omitted: omitWastewater + ", " + omitWastewater,
			want:    []string{"declared twice"},
		},
		{
			name:    "a typo'd key is not silently dropped",
			omitted: `{label: "Transfer From General Fund", label_tial: "to Wastewater"}`,
			want:    []string{"label_tial"},
		},
		{
			name:    "an entry with no label",
			omitted: `{label_tail: "to Wastewater"}`,
			want:    []string{"has no label"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(omissionRule(tt.omitted)), "two-anchor.yaml")
			if err == nil {
				t.Fatal("parsed; an omitted_rows entry must name exactly one row")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not mention %q:\n%v", want, err)
				}
			}
		})
	}
}

// TestTotalRowInRowsIsRefusedForATwoAnchorRow is fisc-gtv (3). The guard that
// stops a printed total being mapped as a data row tested the row index, which
// the label_tail work re-keyed on Identity() -- so the guard stopped firing for
// exactly the rows whose Label the total_row could still collide with. total_row
// is matched against the page text as a plain substring, so the Label is what
// it collides with.
func TestTotalRowInRowsIsRefusedForATwoAnchorRow(t *testing.T) {
	src := strings.Replace(omissionRule(omitWastewater),
		"    units: dollars\n",
		"    units: dollars\n    total_row: \"Transfer From General Fund\"\n", 1)
	_, err := parse(strings.NewReader(src), "two-anchor.yaml")
	if err == nil {
		t.Fatal("parsed; a total_row that is also a data row double-counts it")
	}
	if got := err.Error(); !strings.Contains(got, "also listed in rows") {
		t.Errorf("error does not report the collision:\n%s", got)
	}
}

// TestBlankLabelTailIsReportedAsBlank: a whitespace-only tail is blank, and
// saying it has "leading or trailing whitespace" sends the author looking for
// a character to trim rather than for the anchor they meant to name.
func TestBlankLabelTailIsReportedAsBlank(t *testing.T) {
	_, err := parse(strings.NewReader(
		twoAnchorRule("Transfer From General Fund", "   ", "to Airport")), "two-anchor.yaml")
	if err == nil {
		t.Fatal("parsed; a blank label_tail names nothing")
	}
	if got := err.Error(); !strings.Contains(got, "label_tail is blank") {
		t.Errorf("error does not say the tail is blank:\n%s", got)
	}
}

// TestTwoRowsCannotPrintTheSameLabel is what makes fisc-28h's choice safe. A
// fact's id is hashed from the row_label it publishes, which is
// Row.PrintedLabel(); two rows with distinct identities may still print the
// same label, and those two facts would be indistinguishable to every reader
// of facts.jsonl whatever their ids were.
func TestTwoRowsCannotPrintTheSameLabel(t *testing.T) {
	src := `schema_version: 1
doc_id: d
rules:
  - id: r
    kind: revenue
    basis: adopted
    grain: category
    units: dollars
    parts: [{page: 1, columns: [{fiscal_year: 2026}]}]
    rows:
      - {label: "Transfer From Water to Water", category: a}
      - {label: "Transfer From Water", label_tail: "to Water", category: a}
`
	_, err := parse(strings.NewReader(src), "printed.yaml")
	if err == nil {
		t.Fatal("parsed; two rows cannot publish one row_label")
	}
	if got := err.Error(); !strings.Contains(got, "two rows print as") {
		t.Errorf("error does not report the printed collision:\n%s", got)
	}
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
