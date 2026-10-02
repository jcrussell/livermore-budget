package mapping

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
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

// TestP187CannotCarryTheColumnGuard is why no real page declares a blank cell
// yet. Budget Book p187 right-aligns each figure past the right edge of the
// header over it, and the gap between "Transfers Out" and "Transfers Out to
// CIP" is narrow enough that a Transfers Out figure wider than a "-" ends past
// the midpoint and files under the next column. So the guard omitted_cells
// requires refuses the page, and Community Benefit Fund's blank ending balance
// stays unreadable until the guard can place this grid (fisc-3eh2).
func TestP187CannotCarryTheColumnGuard(t *testing.T) {
	b, err := os.ReadFile("../../testdata/geometry/budget-p0187.json")
	if err != nil {
		t.Fatalf("read geometry fixture: %v", err)
	}
	g, err := geom.ParsePage(b)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	headers := []string{"Expenses", "Transfers Out", "Transfers Out to CIP",
		"Increase/(Use)", "Total Uses", "6/30/24"}
	var grid *geom.Grid
	for _, l := range g.Lines() {
		if m := matchHeaders(l, headers); m.ok {
			if grid, err = geom.NewGrid(m.spans, m.spans[0].Lo); err != nil {
				t.Fatalf("NewGrid: %v", err)
			}
		}
	}
	if grid == nil {
		t.Fatal("p187's header line was not found")
	}

	misfiled := map[string]int{}
	rows, short := 0, 0
	for _, l := range g.Lines() {
		var figures []geom.Word
		for _, w := range l.Words {
			if _, err := amount.Parse(w.Text, amount.Dollars); err == nil && grid.Index(w.Right()) >= 0 {
				figures = append(figures, w)
			}
		}
		switch len(figures) {
		case len(headers):
			rows++
			for k, w := range figures {
				if grid.Index(w.Right()) != k {
					misfiled[headers[k]]++
				}
			}
		case len(headers) - 1:
			short++
		}
	}
	if diff := cmp.Diff(map[string]int{"Transfers Out": 15}, misfiled); diff != "" {
		t.Errorf("figures filed outside their header's band (-want +got):\n%s", diff)
	}
	if short != 1 {
		t.Errorf("%d lines print five figures, want 1: Community Benefit Fund's", short)
	}
	t.Logf("p187: %d of %d six-figure lines file a Transfers Out figure under "+
		"Transfers Out to CIP", misfiled["Transfers Out"], rows)
}
