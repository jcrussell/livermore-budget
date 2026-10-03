package mapping

import (
	"strings"
	"testing"
)

// rowLabelFundsRule is a schedule whose rows are labelled with printed fund
// names, which is the shape Budget Book pp.85-125 prints: the columns are
// (year, basis) pairs carrying no fund at all, and the fund dimension is on the
// row. #DECL and #ROWS are substituted per case.
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

// TestRowLabelsNameFundsRefusesARowItWouldSayNothingAbout is the precondition
// the parser can check without the registry or the pages.
//
// The claim itself -- that "Water" is fund 640, and that the page prints that
// label on that line -- belongs to row-funds-match-their-anchors, which has both.
// What is checkable here is that the declaration ASSERTS something, and that is
// validateTotalRowKinds' discipline applied one field over: a declaration that
// cannot fail records an author's belief rather than a property of the page.
//
// A SINGLE UNFUNDED ROW IS ENOUGH TO REFUSE, and that is the point rather than
// strictness for its own sake. The check reads every row of the rule, so one row
// with no fund is one row the declaration silently does not cover -- and the
// author, having declared it, has no way to tell which.
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

	// A SKIPPED ROW IS NOT READ, so the declaration is not about it. Refusing
	// one would force an author to put a fund number on a row the resolver never
	// reaches, which is a figure typed to satisfy a validator -- the shape
	// Part.OmittedCells' comment refuses by name.
	t.Run("a skipped row declares none", func(t *testing.T) {
		rows := fundedRows + "\n      - {label: \"Grants\", skip: true}"
		if err := parseRowLabelFunds(t, decl, rows); err != nil {
			t.Fatalf("refused a rule whose only unfunded row is skipped: %v", err)
		}
	})

	// EVERY ROW SKIPPED IS THE STRONGER VERSION OF "one row declares none", and
	// it was accepted while that one was refused. A skipped row is never read
	// from the page, so the check drops it from every arm -- not a subject, not
	// in the omission counters -- and the declaration stood over zero checked
	// rows with the rule named nowhere in the summary.
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

	// A COUNTERPART IS AT THE FAR END AND NO BARE LABEL NAMES IT. The
	// declaration speaks for the row's OWN fund; a counterpart's fund sits at
	// the other end of the movement and a bare fund name carries no direction to
	// reach it. Leaving it unchecked was not neutral: the counterpart fell into
	// row-funds-match-their-anchors' unphrased counter, whose clause then named
	// the rule as one that does NOT declare row_labels_name_funds while it did.
	// A false sentence in a string fisc verify prints every run.
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

	// AND WITHOUT THE DECLARATION THE SAME FILE IS FINE. Every schedule mapped
	// before pp.85-125 has rows with no fund, so a guard that fired regardless
	// of the declaration would refuse most of the corpus.
	t.Run("no declaration, unfunded rows", func(t *testing.T) {
		rows := fundedRows + "\n      - {label: \"Grants\", category: department-funding-sources}"
		if err := parseRowLabelFunds(t, "", rows); err != nil {
			t.Fatalf("refused an undeclared rule with an unfunded row: %v", err)
		}
	})
}
