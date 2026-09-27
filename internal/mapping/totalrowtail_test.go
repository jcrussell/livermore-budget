package mapping

import (
	"fmt"
	"strings"
	"testing"
)

// countyMeasBBProbe is Budget Book p0175's first fund, whose printed total
// wraps: "Total County Meas BB-" carries the figures and "Bike/Pedestrian"
// sits alone on the line beneath.
const countyMeasBBProbe = `schema_version: 1
doc_id: livermore-budget-fy2026-2027
rules:
  - id: fund-exp-county-meas-bb-bike-pedestrian
    kind: expenditure
    basis: adopted
    scope: expenditure-by-fund
    grain: fund-by-category
    units: dollars
    total_row: "Total County Meas BB-"
    total_row_tail: "Bike/Pedestrian"
    parts:
      - page: 175
        section: "County Meas BB-Bike/Pedestrian\n"
        stop_at: "Total County Meas BB-"
        column_headers: ["FY 2023-24", "FY 2024-25", "FY 2025-26", "FY 2026-27"]
        columns:
          - {fund_group: capital, fund: 551, fiscal_year: 2024, basis: actual}
          - {fund_group: capital, fund: 551, fiscal_year: 2025, basis: revised}
          - {fund_group: capital, fund: 551, fiscal_year: 2026}
          - {fund_group: capital, fund: 551, fiscal_year: 2027}
    rows:
      - {label: "Services & Supplies", category: services-and-supplies}
`

// checkCountyMeasBB parses the probe with one substitution and returns what
// CheckTotals concluded against the real page.
func checkCountyMeasBB(t *testing.T, old, new string) error {
	t.Helper()
	if !strings.Contains(countyMeasBBProbe, old) {
		t.Fatalf("the probe no longer contains %q, so this mutation changes nothing", old)
	}
	f, err := parse(strings.NewReader(strings.Replace(countyMeasBBProbe, old, new, 1)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 175), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	rule := &f.Rules[0]
	_, err = r.CheckTotals(rule, &rule.Parts[0])
	return err
}

// TestTotalRowTailIsReadOffThePage is the good case and the mutation that
// proves the tail is compared rather than trusted: one letter added to the
// declaration refuses the rule, quoting what the page prints.
func TestTotalRowTailIsReadOffThePage(t *testing.T) {
	if err := checkCountyMeasBB(t, `total_row_tail: "Bike/Pedestrian"`, `total_row_tail: "Bike/Pedestrian"`); err != nil {
		t.Fatalf("p0175's wrapped total was refused: %v", err)
	}
	err := checkCountyMeasBB(t, `total_row_tail: "Bike/Pedestrian"`, `total_row_tail: "Bike/Pedestrians"`)
	if err == nil {
		t.Fatal("a tail the page does not print was accepted")
	}
	if want := `prints "Bike/Pedestrian", not "Bike/Pedestrians"`; !strings.Contains(err.Error(), want) {
		t.Errorf("refused for some other reason, want %q: %v", want, err)
	}
}

// TestWrappedTotalLabelJoinsAtAHyphenWithoutASpace is the one choice the join
// makes: p0175 breaks "County Meas BB-Bike/Pedestrian" after the hyphen, and
// every other wrap in pp.172-183 breaks between words.
func TestWrappedTotalLabelJoinsAtAHyphenWithoutASpace(t *testing.T) {
	for _, tc := range []struct{ row, tail, want string }{
		{"Total County Meas BB-", "Bike/Pedestrian", "Total County Meas BB-Bike/Pedestrian"},
		{"Total Wastewater Connection", "Fees", "Total Wastewater Connection Fees"},
		{"Total Airport", "", "Total Airport"},
	} {
		r := &Rule{TotalRow: tc.row, TotalRowTail: tc.tail}
		if got := r.WrappedTotalLabel(); got != tc.want {
			t.Errorf("WrappedTotalLabel(%q, %q) = %q, want %q", tc.row, tc.tail, got, tc.want)
		}
	}
}

// TestTotalRowTailRefusals holds each refusal to its own arm by calling the
// validator directly: through parse, total_row_above's own guard fires first.
func TestTotalRowTailRefusals(t *testing.T) {
	errf := func(ruleID, field, format string, args ...any) error {
		return &parseError{RuleID: ruleID, Field: field, Msg: fmt.Sprintf(format, args...)}
	}
	for _, tc := range []struct {
		name string
		rule Rule
		want string
	}{
		{"no total_row", Rule{TotalRowTail: "Fees"}, "declared without a total_row"},
		{"above its rows", Rule{TotalRow: "Total", TotalRowAbove: true, TotalRowTail: "Fees"},
			"declared with total_row_above"},
		{"a label-less part", Rule{TotalRow: "Total", TotalRowTail: "Fees", Parts: []Part{{Page: 67, LabelsFrom: 66}}},
			"declared on a rule with a labels_from part"},
		{"padded", Rule{TotalRow: "Total", TotalRowTail: " Fees"}, "the one printed line after the total"},
		{"two lines", Rule{TotalRow: "Total", TotalRowTail: "Fees\nMore"}, "the one printed line after the total"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTotalRowTail(&tc.rule, errf)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused for some other reason, want %q: %v", tc.want, err)
			}
		})
	}
	if err := validateTotalRowTail(&Rule{TotalRow: "Total", TotalRowTail: "Fees"}, errf); err != nil {
		t.Errorf("a well-formed tail was refused: %v", err)
	}
}
