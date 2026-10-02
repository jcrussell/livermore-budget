package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// blankPair is a labelled page and its label-less continuation, whose middle
// row prints FY B and TOTAL and leaves FY C blank -- the shape of Budget Book
// p207, where County Measure D prints five figures and nothing under Reserve
// Increase/(Use).
const blankPair = `schema_version: 1
doc_id: blank-doc

rules:
  - id: pair
    kind: revenue
    basis: adopted
    scope: probe
    grain: category
    units: dollars
    rows:
      - {label: "Alpha", category: taxes/property}
      - {label: "Beta", category: taxes/sales}
      - {label: "Gamma", category: taxes}
    parts:
      - page: 1
        section: "FY A\n"
        stop_at: "END"
        columns:
          - {fiscal_year: 2025}
      - page: 2
        labels_from: 1
        section: "TOTAL\n"
        stop_at: "END"
        omitted_cells:
          - {label: "Beta", column: "FY C", note: "the page prints FY B and TOTAL and leaves FY C blank"}
        column_headers: ["FY B", "FY C", "TOTAL"]
        columns:
          - {fiscal_year: 2026}
          - {fiscal_year: 2027}
          - {skip: true}
`

// blankEntry is blankPair's declaration, for the tests that change it.
const blankEntry = `          - {label: "Beta", column: "FY C", note: "the page prints FY B and TOTAL and leaves FY C blank"}
`

func blankPairPages() map[int]string {
	labelled := fmt.Sprintf("%22s\n", "FY A")
	for _, r := range [][2]string{{"Alpha", "1"}, {"Beta", "2"}, {"Gamma", "3"}} {
		labelled += fmt.Sprintf("%-12s%10s\n", r[0], r[1])
	}
	figures := ""
	for _, r := range [][3]string{{"FY B", "FY C", "TOTAL"}, {"10", "20", "30"},
		{"40", "", "40"}, {"-", "5", "5"}} {
		figures += fmt.Sprintf("%10s%10s%10s\n", r[0], r[1], r[2])
	}
	return map[int]string{1: labelled + "END\n", 2: figures + "END\n"}
}

// blankLabelled is a labelled page whose middle row leaves its FY B cell
// blank, with a printed total under each column.
const blankLabelled = `schema_version: 1
doc_id: blank-doc

rules:
  - id: labelled
    kind: revenue
    basis: adopted
    scope: probe
    grain: category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Alpha", category: taxes/property}
      - {label: "Beta", category: taxes/sales}
      - {label: "Gamma", category: taxes}
    parts:
      - page: 1
        section: "FY B\n"
        stop_at: "Total"
        omitted_cells:
          - {label: "Beta", column: "FY B", note: "printed blank"}
        column_headers: ["FY A", "FY B"]
        columns:
          - {fiscal_year: 2025}
          - {fiscal_year: 2026}
`

const blankLabelledEntry = `          - {label: "Beta", column: "FY B", note: "printed blank"}
`

func blankLabelledPage() map[int]string {
	row := func(label, a, b string) string { return fmt.Sprintf("%-12s%10s%10s\n", label, a, b) }
	return map[int]string{1: row("", "FY A", "FY B") + row("Alpha", "100", "200") +
		row("Beta", "300", "") + row("Gamma", "500", "600") + row("Total", "900", "800")}
}

// blankResolver parses src and resolves it over pages, each with the geometry
// a monospaced page implies.
func blankResolver(t *testing.T, src string, pages map[int]string) (*Resolver, *Rule) {
	t.Helper()
	f, err := parse(strings.NewReader(src), "blank.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := NewResolver(inlineDocWithGeometry(t, f.DocID, pages, textGeometry(f.DocID, pages)), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// readBlank reads part i of src's first rule: its cells and its omissions.
func readBlank(t *testing.T, src string, pages map[int]string, i int) (*Part, []Value, []Omission, error) {
	t.Helper()
	r, rule := blankResolver(t, src, pages)
	p := &rule.Parts[i]
	_, omissions, err := r.Values(rule, p)
	if err != nil {
		return p, nil, nil, err
	}
	cells, err := r.Cells(rule, p)
	return p, cells, omissions, err
}

// placedCell is one read figure as a reader of the page names it.
type placedCell struct {
	Row, Header, Token string
	Cents              amount.Cents
}

func placedCells(p *Part, vals []Value) []placedCell {
	var out []placedCell
	for _, v := range vals {
		out = append(out, placedCell{v.Row.Label, p.ColumnHeaders[v.ColumnIndex].Text, v.Token, v.Cents})
	}
	return out
}

type omittedCellAt struct {
	Row    string
	Page   int
	Cell   bool
	Column int
	Header string
}

func omittedCellsAt(omissions []Omission) []omittedCellAt {
	var out []omittedCellAt
	for _, o := range omissions {
		out = append(out, omittedCellAt{o.Row.Label, o.Page, o.Cell, o.ColumnIndex, o.Header})
	}
	return out
}

// TestABlankCellIsReadAsAbsent is the positional path: Beta's two figures are
// filed under the two columns it prints, the blank yields no cell -- not a
// zero, and not a skipped cell either -- and the declaration is reported as an
// omission so a later check can tell the blank from a "-".
func TestABlankCellIsReadAsAbsent(t *testing.T) {
	p, cells, omissions, err := readBlank(t, blankPair, blankPairPages(), 1)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []placedCell{
		{"Alpha", "FY B", "10", 1000}, {"Alpha", "FY C", "20", 2000}, {"Alpha", "TOTAL", "30", 3000},
		{"Beta", "FY B", "40", 4000}, {"Beta", "TOTAL", "40", 4000},
		{"Gamma", "FY B", "-", 0}, {"Gamma", "FY C", "5", 500}, {"Gamma", "TOTAL", "5", 500},
	}
	if diff := cmp.Diff(want, placedCells(p, cells)); diff != "" {
		t.Errorf("cells (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]omittedCellAt{{"Beta", 2, true, 1, "FY C"}}, omittedCellsAt(omissions)); diff != "" {
		t.Errorf("omissions (-want +got):\n%s", diff)
	}
}

// TestALabelledBlankCellIsReadAsAbsent is the labelled path: Beta takes one
// figure, under FY A, and the FY B total the page prints is the sum of the two
// figures the column prints -- a total over a blank adds what is printed.
func TestALabelledBlankCellIsReadAsAbsent(t *testing.T) {
	pages := blankLabelledPage()
	p, cells, omissions, err := readBlank(t, blankLabelled, pages, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []placedCell{
		{"Alpha", "FY A", "100", 10000}, {"Alpha", "FY B", "200", 20000},
		{"Beta", "FY A", "300", 30000},
		{"Gamma", "FY A", "500", 50000}, {"Gamma", "FY B", "600", 60000},
	}
	if diff := cmp.Diff(want, placedCells(p, cells)); diff != "" {
		t.Errorf("cells (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]omittedCellAt{{"Beta", 1, true, 1, "FY B"}}, omittedCellsAt(omissions)); diff != "" {
		t.Errorf("omissions (-want +got):\n%s", diff)
	}

	r, rule := blankResolver(t, blankLabelled, pages)
	res, err := r.CheckTotals(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("CheckTotals: %v", err)
	}
	if res.Columns != 2 {
		t.Errorf("CheckTotals tied %d columns, want 2", res.Columns)
	}
}

// TestAnUndeclaredBlankCellIsRefused is the default the declaration opts out
// of: a short row is a shape nothing recognises, on either path.
func TestAnUndeclaredBlankCellIsRefused(t *testing.T) {
	_, _, _, err := readBlank(t, strings.Replace(blankPair, blankEntry, "", 1), blankPairPages(), 1)
	if !strings.Contains(fmt.Sprint(err), "read 8 values, want 9") {
		t.Errorf("positional: got %v, want the value count's refusal", err)
	}
	_, _, _, err = readBlank(t, strings.Replace(blankLabelled, blankLabelledEntry, "", 1),
		blankLabelledPage(), 0)
	if !strings.Contains(fmt.Sprint(err), `row "Beta" column 2: token "Gamma" is printed on line 4`) {
		t.Errorf("labelled: got %v, want the guard's same-line refusal", err)
	}
}

// TestADeclaredBlankThePagePrintsIsRefused covers each way a declaration can
// be untrue: a printed cell declared besides the real blank, the declaration
// on the wrong row, on the wrong column, and on a labelled row that prints
// every figure, where no count can see it.
func TestADeclaredBlankThePagePrintsIsRefused(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		pages     map[int]string
		part      int
		want      string
	}{
		{"a printed cell declared besides the blank",
			strings.Replace(blankPair, blankEntry, blankEntry+
				"          - {label: \"Gamma\", column: \"FY C\", note: stale}\n", 1),
			blankPairPages(), 1, "read 8 values, want 7"},
		{"the declaration on the wrong row",
			strings.Replace(blankPair, `{label: "Beta", column: "FY C"`, `{label: "Alpha", column: "FY C"`, 1),
			blankPairPages(), 1,
			`row "Alpha" column 2: "20" is printed under column 2 (FY C), which the rule declares blank on this row`},
		{"the declaration on the wrong column",
			strings.Replace(blankPair, `{label: "Beta", column: "FY C"`, `{label: "Beta", column: "FY B"`, 1),
			blankPairPages(), 1,
			`row "Beta" column 1: "40" is printed under column 1 (FY B), which the rule declares blank on this row`},
		{"a labelled row that prints every figure",
			strings.Replace(blankLabelled, blankLabelledEntry, blankLabelledEntry+
				"          - {label: \"Gamma\", column: \"FY B\", note: stale}\n", 1),
			blankLabelledPage(), 0,
			`row "Gamma" column 2: "600" is printed under column 2 (FY B), which the rule declares blank on this row`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := readBlank(t, tt.src, tt.pages, tt.part)
			if !strings.Contains(fmt.Sprint(err), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestParseRefusesABadOmittedCell(t *testing.T) {
	with := func(src, old, e string) string { return strings.Replace(src, old, e, 1) }
	pair := func(e string) string { return with(blankPair, blankEntry, e) }
	for _, tt := range []struct{ name, src, want string }{
		{"no note", pair(`          - {label: "Beta", column: "FY C"}` + "\n"),
			`"Beta" under "FY C" has no note`},
		{"no label", pair(`          - {column: "FY C", note: n}` + "\n"),
			"entry 0 has no label"},
		{"no such row", pair(`          - {label: "Delta", column: "FY C", note: n}` + "\n"),
			`"Delta" is not one of this rule's rows`},
		{"no column", pair(`          - {label: "Beta", note: n}` + "\n"),
			`"Beta" names no column`},
		{"no such header", pair(`          - {label: "Beta", column: "FY D", note: n}` + "\n"),
			`"FY D" is not one of this part's column_headers`},
		{"a header over two columns", with(blankPair, `column_headers: ["FY B", "FY C", "TOTAL"]`,
			`column_headers: ["FY C", "FY C", "TOTAL"]`),
			`"FY C" heads more than one of this part's columns`},
		{"a skipped column", pair(`          - {label: "Beta", column: "TOTAL", note: n}` + "\n"),
			`"TOTAL" is a skipped column`},
		{"declared twice", pair(blankEntry + blankEntry), `"Beta" under "FY C" is declared twice`},
		{"a row the part omits", with(blankPair, "        omitted_cells:\n",
			"        omitted_rows: [\"Beta\"]\n        omitted_cells:\n"),
			`"Beta" is omitted from this part by omitted_rows`},
		{"an unknown key", pair(`          - {label: "Beta", colum: "FY C", note: n}` + "\n"),
			"field colum not found"},
		{"no column_headers", with(blankPair,
			`        column_headers: ["FY B", "FY C", "TOTAL"]`+"\n", ""),
			"omitted_cells needs column_headers"},
		// blankPair skips TOTAL, which cannot be declared blank, so only a
		// part with no skipped column can name every one.
		{"every column of a row", with(blankLabelled, blankLabelledEntry, blankLabelledEntry+
			"          - {label: \"Beta\", column: \"FY A\", note: n}\n"),
			`declares every column of "Beta" blank`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.src == blankPair || tt.src == blankLabelled {
				t.Fatal("the replacement changed nothing")
			}
			_, err := parse(strings.NewReader(tt.src), "blank.yaml")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

// p207CapitalFunds reads Budget Book pp.206-207's FY2026-27 Capital Funds
// block, p207 positionally against p206's labels.
// Only County Measure D publishes, and its line on p207 prints nothing under
// Reserve Increase/(Use).
const p207CapitalFunds = `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: p207-capital-funds
    kind: fund_balance
    basis: adopted
    scope: probe
    grain: fund-by-category
    units: dollars
    rows:
      - {label: "General Fund CIP Reserves", skip: true}
      - {label: "Traffic Impact Fee (TIF)", skip: true}
      - {label: "TVTC 20% Fee", skip: true}
      - {label: "Park Fee - AB 1600", skip: true}
      - {label: "Solid Waste & Recyc Impact Fee", skip: true}
      - {label: "2022 COP Construction Fund", skip: true}
      - {label: "County Measure D", fund_group: capital}
      - {label: "County Meas BB-Bike/Pedestrian", skip: true}
      - {label: "County Meas BB-Local St & Rd", skip: true}
      - {label: "County Measure F Veh Reg Fee", skip: true}
      - {label: "State - Gas Tax", skip: true}
      - {label: "State - SB1", skip: true}
      - {label: "Developers Deposit", skip: true}
      - {label: "Public Utility Undergrounding", skip: true}
      - {label: "Transferable Development Cred", skip: true}
      - {label: "SoLivSpec Plan & AD Closeout", skip: true}
    parts:
      - page: 206
        section: "Capital Funds\n"
        stop_at: "Total Capital Funds"
        columns:
          - {fiscal_year: 2027, category: fund-balance/beginning}
          - {fiscal_year: 2027, kind: revenue, category: taxes}
          - {fiscal_year: 2027, kind: transfer_in, category: transfers/in}
          - {skip: true}
      - page: 207
        labels_from: 206
        # p207 prints no labels, and the five lines above the block print six
        # "$" each, so the 31st "$" opens General Fund CIP Reserves' line. Its
        # other five "$" come next, so the sixth after it opens Total Capital
        # Funds.
        section: "$"
        section_ordinal: 31
        stop_at: "$"
        stop_at_ordinal: 6
        omitted_cells:
          - {label: "County Measure D", column: "Increase/(Use)", note: "the page prints five figures and nothing under Reserve Increase/(Use)"}
        column_headers: ["Expenses", "Transfers Out", "Transfers Out to CIP", "Increase/(Use)",
                         "Total Uses", "6/30/27"]
        columns:
          - {fiscal_year: 2027, kind: expenditure, category: capital-projects}
          - {fiscal_year: 2027, kind: transfer_out, category: transfers/out}
          - {fiscal_year: 2027, kind: transfer_out, category: transfers/out-to-cip}
          - {fiscal_year: 2027, category: fund-balance/reserve-increase}
          - {skip: true}
          - {fiscal_year: 2027, category: fund-balance/ending}
`

// TestP207ReadsCountyMeasureDWithItsBlankReserveCell is omitted_cells on the
// page it exists for, guarded. County Measure D's p207 line prints 505,784 |
// - | - | (blank) | 505,784 | (858,450), and p206 prints its sources as
// (358,666) | 6,000 | - | (352,666): (352,666) - 505,784 = (858,450), so the
// line is read as the page prints it.
func TestP207ReadsCountyMeasureDWithItsBlankReserveCell(t *testing.T) {
	f, err := parse(strings.NewReader(p207CapitalFunds), "p207.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 206, 207), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	rule := &f.Rules[0]

	type cell struct {
		Page     int
		Category string
		Token    string
		Cents    amount.Cents
	}
	var got []cell
	var omitted []omittedCellAt
	for i := range rule.Parts {
		vals, omissions, err := r.Values(rule, &rule.Parts[i])
		if err != nil {
			t.Fatalf("Values(p%d): %v", rule.Parts[i].Page, err)
		}
		for _, v := range vals {
			got = append(got, cell{v.Page, v.Category(), v.Token, v.Cents})
		}
		omitted = append(omitted, omittedCellsAt(omissions)...)
	}
	want := []cell{
		{206, "fund-balance/beginning", "(358,666)", -35866600},
		{206, "taxes", "6,000", 600000},
		{206, "transfers/in", "-", 0},
		{207, "capital-projects", "505,784", 50578400},
		{207, "transfers/out", "-", 0},
		{207, "transfers/out-to-cip", "-", 0},
		{207, "fund-balance/ending", "(858,450)", -85845000},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("cells (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]omittedCellAt{{"County Measure D", 207, true, 3, "Increase/(Use)"}},
		omitted); diff != "" {
		t.Errorf("omissions (-want +got):\n%s", diff)
	}
}
