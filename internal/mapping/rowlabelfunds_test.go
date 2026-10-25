package mapping

import (
	"strings"
	"testing"
)

// rowLabelFundsRule is the pp.85-125 shape: (year, basis) columns carrying no
// fund, the fund on the row. #DECL and #ROWS are substituted per case.
const rowLabelFundsRule = `schema_version: 1
doc_id: livermore-budget-fy2026-2027
rules:
  - id: funding-example
    kind: expenditure
    basis: adopted
    scope: department-funding-sources
    grain: fund-by-category
    units: dollars
    total_row: "Total Department Funding Sources"
#DECL
    parts:
      - page: 93
        section: "Department Funding Sources\n"
        stop_at: "Total Department Funding Sources"
        column_headers: ["FY 2023-24", "FY 2024-25", "FY 2025-26", "FY 2026-27"]
        columns:
          - {fiscal_year: 2024, basis: actual}
          - {fiscal_year: 2025, basis: revised}
          - {fiscal_year: 2026}
          - {fiscal_year: 2027}
    rows:
#ROWS
`

func parseRowLabelFunds(t *testing.T, decl, rows string) error {
	t.Helper()
	src := strings.Replace(rowLabelFundsRule, "#DECL", decl, 1)
	src = strings.Replace(src, "#ROWS", rows, 1)
	_, err := parse(strings.NewReader(src), "funding.yaml")
	return err
}

const fundedRows = `      - {label: "General Fund", category: department-funding-sources, fund: 100, fund_group: general}
      - {label: "General Liability", category: department-funding-sources, fund: 700, fund_group: internal-service}`

// TestRowLabelsNameFundsRefusesARowItWouldSayNothingAbout: the declaration must
// assert something, so a single unfunded row refuses it. Whether the label
// names that fund is row-funds-match-their-anchors'.
func TestRowLabelsNameFundsRefusesARowItWouldSayNothingAbout(t *testing.T) {
	const decl = "    row_labels_name_funds: true"

	t.Run("every row declares a fund", func(t *testing.T) {
		if err := parseRowLabelFunds(t, decl, fundedRows); err != nil {
			t.Fatalf("refused a rule whose rows all declare a fund: %v", err)
		}
	})

	t.Run("one row declares none", func(t *testing.T) {
		rows := fundedRows + "\n      - {label: \"Grants\", category: department-funding-sources}"
		err := parseRowLabelFunds(t, decl, rows)
		if err == nil {
			t.Fatal("accepted a declaration that says nothing about one of its rows")
		}
		for _, want := range []string{`row "Grants" declares no fund`, "says nothing about it"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want it to contain %q", err, want)
			}
		}
	})

	// A skipped row is not read, so it needs no fund.
	t.Run("a skipped row declares none", func(t *testing.T) {
		rows := fundedRows + "\n      - {label: \"Grants\", skip: true}"
		if err := parseRowLabelFunds(t, decl, rows); err != nil {
			t.Fatalf("refused a rule whose only unfunded row is skipped: %v", err)
		}
	})

	// Every row skipped leaves the declaration over zero checked rows.
	t.Run("every row is skipped", func(t *testing.T) {
		rows := `      - {label: "General Fund", skip: true}
      - {label: "General Liability", skip: true}`
		err := parseRowLabelFunds(t, decl, rows)
		if err == nil {
			t.Fatal("accepted a declaration covering no row at all")
		}
		if !strings.Contains(err.Error(), "covers none of them") {
			t.Errorf("error = %v, want it to say the declaration covers no row", err)
		}
	})

	// The declaration speaks for the row's own fund; a counterpart's fund is at
	// the far end, which a bare label cannot name.
	t.Run("a row declares a counterpart", func(t *testing.T) {
		rows := fundedRows + "\n      - {label: \"Transfers In\", category: \"transfers/in\", " +
			"kind: transfer_in, fund: 100, fund_group: general, " +
			"counterpart: {category: \"transfers/out\", kind: transfer_out, fund: 200, " +
			"fund_group: special-revenue}}"
		err := parseRowLabelFunds(t, decl, rows)
		if err == nil {
			t.Fatal("accepted a declaration over a row with two ends")
		}
		for _, want := range []string{
			`row "Transfers In" declares a counterpart`,
			"printed label cannot name",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want it to contain %q", err, want)
			}
		}
	})

	// Without the declaration the same file is fine: most schedules have rows
	// with no fund.
	t.Run("no declaration, unfunded rows", func(t *testing.T) {
		rows := fundedRows + "\n      - {label: \"Grants\", category: department-funding-sources}"
		if err := parseRowLabelFunds(t, "", rows); err != nil {
			t.Fatalf("refused an undeclared rule with an unfunded row: %v", err)
		}
	})
}
