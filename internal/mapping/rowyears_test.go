package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// yearRows is Budget Book p80's DEBT SERVICE REQUIREMENTS in miniature: a row
// per year ending June 30, a band printed as one figure, and two fund types
// each printing "Principal" and "Interest", the second of which stops before
// the band, so its blanks are named by column_ordinal.
const yearRows = `schema_version: 1
doc_id: years-doc

rules:
  - id: years
    kind: expenditure
    basis: projected
    scope: probe
    grain: category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "2026", fiscal_year: 2026}
      - {label: "2027", fiscal_year: 2027}
      - {label: "2028-2032", fiscal_year: 2028, fiscal_year_through: 2032}
    parts:
      - page: 1
        section: "June 30"
        stop_at: "Total"
        omitted_cells:
          - {label: "2028-2032", column: "Principal", column_ordinal: 2, note: "the second fund type prints nothing for the band"}
          - {label: "2028-2032", column: "Interest", column_ordinal: 2, note: "the second fund type prints nothing for the band"}
        column_headers: ["Principal", "Interest", "Principal", "Interest"]
        columns:
          - {category: debt-services/principal}
          - {category: debt-services/interest}
          - {category: debt-services/principal, fund_group: internal-service}
          - {category: debt-services/interest, fund_group: internal-service}
`

func yearRowsPage() map[int]string {
	row := func(label string, v ...string) string {
		s := fmt.Sprintf("%-14s", label)
		for _, x := range v {
			s += fmt.Sprintf("%11s", x)
		}
		return s + "\n"
	}
	return map[int]string{1: row("June 30", "Principal", "Interest", "Principal", "Interest") +
		row("2026", "100", "10", "7", "1") +
		row("2027", "200", "20", "8", "2") +
		row("2028-2032", "500", "50") +
		row("Total", "800", "80", "15", "3")}
}

// TestARowCarriesItsYearsAndABandStaysABand reads year rows: each figure is
// filed under its row's fiscal year, a band under its first and last year,
// the second fund type's blanks under the band are named by ordinal, and the
// columns tie to the printed Total.
//
// Mutation: drop the year arm from Row.EffectiveColumn and every figure comes
// back with no fiscal year.
func TestARowCarriesItsYearsAndABandStaysABand(t *testing.T) {
	r, rule := blankResolver(t, yearRows, yearRowsPage())
	p := &rule.Parts[0]
	vals, omitted, err := r.Values(rule, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	type cell struct {
		Token          string
		Year, Through  int
		Category, Fund string
	}
	var got []cell
	for _, v := range vals {
		c := v.Row.EffectiveColumn(v.Column)
		got = append(got, cell{v.Token, c.FiscalYear, c.FiscalYearThrough, v.Category(), c.FundGroup})
	}
	want := []cell{
		{"100", 2026, 0, "debt-services/principal", ""},
		{"10", 2026, 0, "debt-services/interest", ""},
		{"7", 2026, 0, "debt-services/principal", "internal-service"},
		{"1", 2026, 0, "debt-services/interest", "internal-service"},
		{"200", 2027, 0, "debt-services/principal", ""},
		{"20", 2027, 0, "debt-services/interest", ""},
		{"8", 2027, 0, "debt-services/principal", "internal-service"},
		{"2", 2027, 0, "debt-services/interest", "internal-service"},
		{"500", 2028, 2032, "debt-services/principal", ""},
		{"50", 2028, 2032, "debt-services/interest", ""},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("cells (-want +got):\n%s", diff)
	}
	if len(omitted) != 2 || omitted[0].ColumnIndex != 2 || omitted[1].ColumnIndex != 3 ||
		omitted[0].Header != "Principal #2" || omitted[1].Header != "Interest #2" {
		t.Errorf("omissions = %+v, want the band's cells in columns 2 and 3, named by ordinal", omitted)
	}
	if _, err := r.CheckTotals(rule, p); err != nil {
		t.Errorf("CheckTotals: %v", err)
	}
}

// TestParseRefusesARowsYearsItsLabelDoesNotPrint holds every way a row's years
// could say something its printed label does not.
func TestParseRefusesARowsYearsItsLabelDoesNotPrint(t *testing.T) {
	with := func(old, new string) string {
		s := strings.Replace(yearRows, old, new, 1)
		if s == yearRows {
			t.Fatalf("the replacement of %q changed nothing", old)
		}
		return s
	}
	for _, tt := range []struct{ name, src, want string }{
		{"a year its label does not print",
			with(`{label: "2027", fiscal_year: 2027}`, `{label: "2027", fiscal_year: 2028}`),
			`row "2027" declares the years "2028"`},
		{"a band one year short",
			with(`fiscal_year_through: 2032}`, `fiscal_year_through: 2031}`),
			`row "2028-2032" declares the years "2028-2031"`},
		{"a band ending where it begins",
			with(`fiscal_year_through: 2032}`, `fiscal_year_through: 2028}`),
			`fiscal_year_through 2028 is not after fiscal_year 2028`},
		{"a band end with no start",
			with(`{label: "2028-2032", fiscal_year: 2028, fiscal_year_through: 2032}`,
				`{label: "2028-2032", fiscal_year_through: 2032}`),
			`fiscal_year_through 2032 with no fiscal_year`},
		{"a publishing row with no year",
			with(`{label: "2027", fiscal_year: 2027}`, `{label: "2027"}`),
			`row "2027" publishes and declares no fiscal_year`},
		{"a skipped row with a year",
			with(`{label: "2027", fiscal_year: 2027}`, `{label: "2027", fiscal_year: 2027, skip: true}`),
			`row "2027" declares fiscal_year 2027 and publishes nothing`},
		{"a year on the column too",
			with(`- {category: debt-services/principal}`, `- {category: debt-services/principal, fiscal_year: 2026}`),
			`declares fiscal_year 2026, and this rule's rows carry its years`},
		{"an ordinal on a repeated header left out",
			with(`column: "Principal", column_ordinal: 2,`, `column: "Principal",`),
			`"Principal" heads more than one of this part's columns`},
		{"an ordinal past the headers",
			with(`column: "Principal", column_ordinal: 2,`, `column: "Principal", column_ordinal: 3,`),
			`column_ordinal 3 for "Principal", which heads 2 columns`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(tt.src), "years.yaml")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

// TestAColumnOrdinalOnAHeaderPrintedOnceIsRefused: an ordinal that could only
// restate its header is a declaration nothing needs.
func TestAColumnOrdinalOnAHeaderPrintedOnceIsRefused(t *testing.T) {
	src := strings.Replace(blankPair, `{label: "Beta", column: "FY C",`, `{label: "Beta", column: "FY C", column_ordinal: 1,`, 1)
	if src == blankPair {
		t.Fatal("the replacement changed nothing")
	}
	_, err := parse(strings.NewReader(src), "blank.yaml")
	if want := `column_ordinal 1 for "FY C", which heads one column`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want an error containing %q", err, want)
	}
}

// TestARowPrintingOnlyASkippedColumnIsRead is Budget Book p81's 2046-2050
// line: a figure under Grand Total, which the rule skips, and nothing under
// the fund types it publishes. The row is printed, publishes nothing on this
// part, and the read still demands its figure.
//
// Mutation: count only publishing columns in validateOmittedCells again and
// parse refuses the row as every column blank.
func TestARowPrintingOnlyASkippedColumnIsRead(t *testing.T) {
	src := strings.Replace(blankPair, blankEntry, blankEntry+
		`          - {label: "Beta", column: "FY B", note: "the page prints TOTAL alone"}`+"\n", 1)
	pages := blankPairPages()
	pages[2] = strings.Replace(pages[2], fmt.Sprintf("%10s%10s%10s\n", "40", "", "40"),
		fmt.Sprintf("%10s%10s%10s\n", "", "", "40"), 1)
	_, cells, _, err := readBlank(t, src, pages, 1)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var beta []string
	for _, c := range cells {
		if c.Row.Label == "Beta" {
			beta = append(beta, c.Token)
		}
	}
	if diff := cmp.Diff([]string{"40"}, beta); diff != "" {
		t.Errorf("Beta's cells (-want +got):\n%s", diff)
	}

	// And the count still refuses the row when the page prints nothing at
	// all on it.
	pages[2] = strings.Replace(pages[2], fmt.Sprintf("%10s%10s%10s\n", "", "", "40"), "\n", 1)
	if _, _, _, err := readBlank(t, src, pages, 1); err == nil {
		t.Error("a row printing no figure read clean")
	}
}

// TestTwoGridsOfOnePageAreNamedApart: a page printing two schedules under
// different headers reads each through its own named grid, two parts naming
// one grid still declare one list, and two names for one list are refused.
//
// Mutation: key checkColumnGrids by (doc, page) alone again and the two
// grids of pp.80-81 refuse the published file.
func TestTwoGridsOfOnePageAreNamedApart(t *testing.T) {
	two := func(gridA, headersA, gridB, headersB string) string {
		part := func(id, grid, headers string) string {
			return fmt.Sprintf(`  - id: %s
    kind: revenue
    basis: adopted
    scope: probe
    grain: category
    units: dollars
    rows:
      - {label: "Alpha", category: taxes}
    parts:
      - page: 1
        grid: %s
        section: "S"
        column_headers: %s
        columns:
          - {fiscal_year: 2025}
          - {fiscal_year: 2026}
`, id, grid, headers)
		}
		return "schema_version: 1\ndoc_id: grid-doc\n\nrules:\n" +
			part("a", gridA, headersA) + part("b", gridB, headersB)
	}
	for _, tt := range []struct{ name, src, want string }{
		{"two grids, two lists", two("budget", `["X", "Y"]`, "requirements", `["P", "Q"]`), ""},
		{"one grid, two lists", two("budget", `["X", "Y"]`, "budget", `["P", "Q"]`),
			`but rule "a" in grid.yaml declares`},
		{"two grids, one list", two("budget", `["X", "Y"]`, "requirements", `["X", "Y"]`),
			`one header list is one grid`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parse(strings.NewReader(tt.src), "grid.yaml")
			if err == nil {
				err = checkColumnGrids([]*File{f})
			}
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("got %v, want none", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}
