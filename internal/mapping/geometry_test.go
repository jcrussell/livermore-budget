package mapping

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/geom"
)

// word is one word of an inline geometry fixture.
type word struct {
	x0, y0, x1, y1 float64
	text           string
}

// geomJSON renders words the way tools/extract.py writes them.
func geomJSON(docID string, page int, words []word) string {
	rows := make([]string, len(words))
	for i, w := range words {
		rows[i] = fmt.Sprintf("  [%g,%g,%g,%g,%s]", w.x0, w.y0, w.x1, w.y1, quote(w.text))
	}
	return fmt.Sprintf("{\n \"doc_id\": %s,\n \"height\": 792.0,\n \"page\": %d,\n"+
		" \"schema_version\": 1,\n \"width\": 612.0,\n \"words\": [\n%s\n ]\n}\n",
		quote(docID), page, strings.Join(rows, ",\n"))
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// spineHeaders is what a part of Budget Book p67 declares: the page prints
// "FY 2025-26 / FY 2026-27" once over each of four fund groups.
var spineHeaders = []string{
	"FY 2025-26", "FY 2026-27", "FY 2025-26", "FY 2026-27",
	"FY 2025-26", "FY 2026-27", "FY 2025-26", "FY 2026-27",
}

// p67Rule writes a rule reading the ten revenue rows of Budget Book p67 with the
// given column headers, so a test can vary only the grid the rule claims.
func p67Rule(t *testing.T, headers []string) (*Resolver, *Rule, *Part) {
	t.Helper()
	cols := make([]string, 8)
	for i := range cols {
		cols[i] = fmt.Sprintf("{fund_group: g%d, fiscal_year: 202%d}", i, i%8)
	}
	var rows strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&rows, "      - {label: \"row%d\", category: c}\n", i)
	}
	src := "schema_version: 1\ndoc_id: livermore-budget-fy2026-2027\nrules:\n" +
		"  - id: p67\n    kind: revenue\n    basis: adopted\n    units: dollars\n" +
		"    parts:\n      - page: 67\n" +
		"        labels_from: 66\n" +
		"        section: \"FY 2026-27\"\n        section_ordinal: 4\n" +
		"        stop_at: \"$\"\n" +
		"        columns: [" + strings.Join(cols, ", ") + "]\n" +
		headerYAML(headers) +
		"      - page: 66\n" +
		"        section: \"REVENUES:\"\n        section_ordinal: 1\n" +
		"        stop_at: \"TOTAL REVENUES:\"\n" +
		"        columns: [{fund_group: general, fiscal_year: 2026}]\n" +
		"    rows:\n" + rows.String()

	f, err := parse(strings.NewReader(src), "p67.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(budgetDoc(t, 66, 67), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return res, &f.Rules[0], &f.Rules[0].Parts[0]
}

func headerYAML(headers []string) string {
	if len(headers) == 0 {
		return ""
	}
	q := make([]string, len(headers))
	for i, h := range headers {
		q[i] = quote(h)
	}
	return "        column_headers: [" + strings.Join(q, ", ") + "]\n"
}

// TestColumnGuardReadsTheSpine is the positive control the mutations below are
// mutations OF: with the grid the page actually prints, all eighty of p67's
// revenue figures place in the columns the rule assigned them.
func TestColumnGuardReadsTheSpine(t *testing.T) {
	res, rule, part := p67Rule(t, spineHeaders)
	values, _, err := res.Values(rule, part)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if got, want := len(values), 80; got != want {
		t.Errorf("read %d values, want %d (10 rows x 8 columns)", got, want)
	}
}

// p66Rule writes a labelled rule over the ten revenue rows of Budget Book p66,
// with one column per declared header.
func p66Rule(t *testing.T, headers []string) (*Resolver, *Rule, *Part) {
	t.Helper()
	cols := make([]string, len(headers))
	for i := range cols {
		cols[i] = fmt.Sprintf("{fund_group: g%d, fiscal_year: 202%d}", i, i%8)
	}
	labels := []string{"Property Taxes", "Other Taxes", "Intergovernmental",
		"Charges for Services", "Use of Money And Property", "Contributions Outsourced",
		"Miscellaneous Revenue", "Sales Taxes", "Fines & Forfeitures", "Licenses & Permits"}
	var rows strings.Builder
	for _, l := range labels {
		fmt.Fprintf(&rows, "      - {label: %s, category: c}\n", quote(l))
	}
	src := "schema_version: 1\ndoc_id: livermore-budget-fy2026-2027\nrules:\n" +
		"  - id: p66\n    kind: revenue\n    basis: adopted\n    units: dollars\n" +
		"    parts:\n      - page: 66\n" +
		"        section: \"REVENUES:\"\n        section_ordinal: 1\n" +
		"        stop_at: \"TOTAL REVENUES:\"\n" +
		"        columns: [" + strings.Join(cols, ", ") + "]\n" +
		headerYAML(headers) +
		"    rows:\n" + rows.String()
	f, err := parse(strings.NewReader(src), "p66.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(budgetDoc(t, 66), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return res, &f.Rules[0], &f.Rules[0].Parts[0]
}

// TestColumnGuardRejectsAGridThePageDoesNotPrint is what keeps the guard from
// being inert. Every case reads a real page with its real figures and differs
// only in what the rule CLAIMS about where the columns are; each must be
// refused, and the three arms fail for three different reasons.
func TestColumnGuardRejectsAGridThePageDoesNotPrint(t *testing.T) {
	swapped := append([]string{}, spineHeaders...)
	swapped[0], swapped[1] = swapped[1], swapped[0]

	t.Run("a rule that forgot its first column", func(t *testing.T) {
		// PLACEMENT. Naming only the last three of p66's four printed headers
		// matches, in order, against a grid whose first band starts at x=374.04
		// -- and "Property Taxes"'s first figure ends at x=363.60, in the area
		// the grid calls row labels. Nothing about the token count says so:
		// the row is followed by four figures and the rule wanted three.
		res, rule, part := p66Rule(t, []string{"FY 2026-27", "FY 2025-26", "FY 2026-27"})
		_, _, err := res.Values(rule, part)
		if err == nil {
			t.Fatal("Values succeeded against a grid missing the page's first column")
		}
		got := diagnosis(t, err)
		for _, want := range []string{
			`row "Property Taxes" column 1`, `"64,143,762" ends at x 363.64`,
			"the row-label area, left of the first column", "reads it as column 1",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("error = %s\nwant it to mention %q", got, want)
			}
		}
	})

	t.Run("two headers in the wrong order", func(t *testing.T) {
		// ORDER. column_headers is a claim about the order the page prints the
		// columns in, and it is enforced by requiring each header to occur after
		// the one before it. Swapping a pair makes the sequence unmatchable
		// rather than matchable-and-shifted, which is the point: a page that
		// dropped a column fails here instead of shifting every figure one place.
		res, rule, part := p67Rule(t, swapped)
		_, _, err := res.Values(rule, part)
		if err == nil {
			t.Fatal("Values succeeded against headers in an order the page does not print")
		}
		got := diagnosis(t, err)
		if want := `does not occur after header`; !strings.Contains(got, want) {
			t.Errorf("error = %s\nwant it to mention %q", got, want)
		}
	})

	t.Run("one column short of the page", func(t *testing.T) {
		// AN ADDED COLUMN. In-order matching alone cannot catch a page that
		// GAINED a column: every declared header still matches, and the new
		// column's figures quietly join the last band. What catches it is that
		// the header line then carries a printed header no column claims.
		res, rule, part := p67RuleWithColumns(t, spineHeaders[:7])
		_, _, err := res.Values(rule, part)
		if err == nil {
			t.Fatal("Values succeeded against a page printing a column the rule does not declare")
		}
		got := diagnosis(t, err)
		for _, want := range []string{
			"is printed on the header line", "not one of this part's columns",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("error = %s\nwant it to mention %q", got, want)
			}
		}
	})

	t.Run("a header the page does not print", func(t *testing.T) {
		res, rule, part := p67RuleWithColumns(t,
			append(append([]string{}, spineHeaders[:7]...), "FY 2099-00"))
		_, _, err := res.Values(rule, part)
		if err == nil {
			t.Fatal("Values succeeded against a header the page does not print")
		}
		if want := `column header 8 ("FY 2099-00") does not occur after header 7`; !strings.Contains(
			diagnosis(t, err), want) {
			t.Errorf("error = %s\nwant it to mention %q", diagnosis(t, err), want)
		}
	})
}

// p67RuleWithColumns is p67Rule with the column COUNT matched to the header
// count, so the parser's own count check does not pre-empt the guard.
func p67RuleWithColumns(t *testing.T, headers []string) (*Resolver, *Rule, *Part) {
	t.Helper()
	if len(headers) == 8 {
		return p67Rule(t, headers)
	}
	cols := make([]string, len(headers))
	for i := range cols {
		cols[i] = fmt.Sprintf("{fund_group: g%d, fiscal_year: 202%d}", i, i%8)
	}
	src := "schema_version: 1\ndoc_id: livermore-budget-fy2026-2027\nrules:\n" +
		"  - id: p67\n    kind: revenue\n    basis: adopted\n    units: dollars\n" +
		"    parts:\n      - page: 67\n" +
		"        section: \"FY 2026-27\"\n        section_ordinal: 4\n" +
		"        stop_at: \"$\"\n" +
		"        columns: [" + strings.Join(cols, ", ") + "]\n" +
		headerYAML(headers) +
		"    rows:\n      - {label: \"row0\", category: c}\n"
	f, err := parse(strings.NewReader(src), "p67.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(budgetDoc(t, 67), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return res, &f.Rules[0], &f.Rules[0].Parts[0]
}

// TestOverlapFilingLosesZeroDashes is the arithmetic behind the filing rule, over
// a real page.
//
// Budget Book p167 right-aligns its figures to a grid offset right of the header
// text, so a value's right edge identifies its column and a value's OVERLAP with
// its header does not. Filing by overlap does not merely place fewer tokens: the
// tokens it fails to place are zero dashes, which this corpus spells "-" and
// must never lose.
func TestOverlapFilingLosesZeroDashes(t *testing.T) {
	b, err := os.ReadFile("../../testdata/geometry/budget-p0167.json")
	if err != nil {
		t.Fatalf("read geometry fixture: %v", err)
	}
	g, err := geom.ParsePage(b)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}

	var header geom.Line
	for _, l := range g.Lines() {
		if countText(l, "FY") >= 2 {
			header = l
			break
		}
	}
	m := matchHeaders(header, []string{"FY 2023-24", "FY 2024-25", "FY 2025-26", "FY 2026-27"})
	if !m.ok {
		t.Fatal("p167's four fiscal-year headers were not found on its header line")
	}
	grid, err := geom.NewGrid(m.spans, m.spans[0].Lo)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}

	var placedByRight, lostByOverlap int
	var lostTexts []string
	for _, w := range g.Words {
		if grid.Index(w.Right()) < 0 {
			continue // the row-label area
		}
		placedByRight++
		overlaps := false
		for i := 0; i < grid.Len(); i++ {
			if w.X0 < m.spans[i].Hi && w.X1 > m.spans[i].Lo {
				overlaps = true
				break
			}
		}
		if !overlaps {
			lostByOverlap++
			lostTexts = append(lostTexts, w.Text)
		}
	}

	if placedByRight == 0 {
		t.Fatal("no word placed at all, so this test asserts nothing")
	}
	if lostByOverlap == 0 {
		t.Fatalf("overlap filing placed all %d words, so the two rules do not "+
			"differ on this page and it is the wrong fixture for this claim",
			placedByRight)
	}
	for _, s := range lostTexts {
		if s != "-" {
			t.Errorf("overlap filing loses %q; every token it loses on this page "+
				"should be a zero dash", s)
		}
	}
	t.Logf("p167: right-edge filing places %d words; overlap filing loses %d of them, "+
		"all zero dashes", placedByRight, lostByOverlap)
}

func countText(l geom.Line, s string) int {
	n := 0
	for _, w := range l.Words {
		if w.Text == s {
			n++
		}
	}
	return n
}

// sparsePage is the shape the acceptance criterion names and no page of this
// corpus prints in a form a rule can currently map: a row whose SECOND column is
// blank and whose third carries a figure. -layout emits two tokens for it and
// says nothing about which columns they are.
const sparsePage = `          FY 2024   FY 2025   FY 2026
REVENUES:
Alpha       100                 300
Beta        400       500       600
TOTAL
`

func sparseGeometry(docID string) string {
	return geomJSON(docID, 1, []word{
		{100, 10, 112, 18, "FY"}, {114, 10, 130, 18, "2024"},
		{200, 10, 212, 18, "FY"}, {214, 10, 230, 18, "2025"},
		{300, 10, 312, 18, "FY"}, {314, 10, 330, 18, "2026"},
		{10, 30, 60, 38, "REVENUES:"},
		{10, 50, 40, 58, "Alpha"}, {120, 50, 135, 58, "100"}, {320, 50, 335, 58, "300"},
		{10, 70, 35, 78, "Beta"}, {120, 70, 135, 78, "400"},
		{220, 70, 235, 78, "500"}, {320, 70, 335, 78, "600"},
		{10, 90, 45, 98, "TOTAL"},
	})
}

const sparseRule = `schema_version: 1
doc_id: sparse-doc
rules:
  - id: sparse
    kind: revenue
    basis: adopted
    units: dollars
    parts:
      - page: 1
        section: "REVENUES:"
        stop_at: "TOTAL"
        columns: [{fund_group: g, fiscal_year: 2024},
                  {fund_group: g, fiscal_year: 2025},
                  {fund_group: g, fiscal_year: 2026}]
        column_headers: ["FY 2024", "FY 2025", "FY 2026"]
    rows:
      - {label: "Alpha", category: c}
      - {label: "Beta", category: c}
`

// TestASparseRowIsRefusedRatherThanMisfiled is the acceptance criterion itself.
//
// Without the guard, the labelled read takes the first three tokens after
// "Alpha" and files 300 -- a figure the page prints in the THIRD column -- as
// the value of the second, then carries on. That is a plausible wrong value with
// working-looking provenance, which is the thing this project refuses. The
// refusal has to name the column the page actually prints it in, or a reader
// cannot tell a mis-declared rule from a moved column.
func TestASparseRowIsRefusedRatherThanMisfiled(t *testing.T) {
	d := inlineDocWithGeometry(t, "sparse-doc",
		map[int]string{1: sparsePage},
		map[int]string{1: sparseGeometry("sparse-doc")})
	f, err := parse(strings.NewReader(sparseRule), "sparse.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(d, f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	_, _, err = res.Values(&f.Rules[0], &f.Rules[0].Parts[0])
	if err == nil {
		t.Fatal("Values succeeded on a row whose second column is blank; " +
			"the third column's figure would have been published as the second's")
	}
	got := diagnosis(t, err)
	for _, want := range []string{
		`row "Alpha" column 2`,
		`"300" ends at x 335.00`,
		"which is column 3",
		"reads it as column 2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error = %s\nwant it to mention %q", got, want)
		}
	}
}

// TestFusedRowsAreCaughtByLineAccounting is what the line count is for, and it
// is a case no other guard can see.
//
// The part has ONE column, so its single band claims everything right of the
// gutter and the placement check has nothing to say. The value count is right:
// two rows, one column, two figures. What is wrong is that the page printed them
// on one line rather than two, which is precisely the corruption the previous
// extractor produced (fisc-gxt) and which mismaps every row below it.
func TestFusedRowsAreCaughtByLineAccounting(t *testing.T) {
	labels := "LABELS:\nAlpha       1\nBeta        2\nEND\n"
	fused := "          FY 2024\n       10       20\nEND\n"
	labelsGeom := geomJSON("fused-doc", 1, []word{
		{10, 10, 60, 18, "LABELS:"},
		{10, 30, 40, 38, "Alpha"}, {120, 30, 135, 38, "1"},
		{10, 50, 35, 58, "Beta"}, {120, 50, 135, 58, "2"},
		{10, 70, 40, 78, "END"},
	})
	fusedGeom := geomJSON("fused-doc", 2, []word{
		{100, 10, 112, 18, "FY"}, {114, 10, 130, 18, "2024"},
		{120, 30, 135, 38, "10"}, {220, 30, 235, 38, "20"},
		{10, 50, 40, 58, "END"},
	})
	src := `schema_version: 1
doc_id: fused-doc
rules:
  - id: fused
    kind: revenue
    basis: adopted
    units: dollars
    parts:
      - page: 1
        section: "LABELS:"
        stop_at: "END"
        columns: [{fund_group: g, fiscal_year: 2024}]
        column_headers: ["FY 2024"]
      - page: 2
        labels_from: 1
        section: "FY 2024"
        stop_at: "END"
        columns: [{fund_group: g, fiscal_year: 2024}]
        column_headers: ["FY 2024"]
    rows:
      - {label: "Alpha", category: c}
      - {label: "Beta", category: c}
`
	d := inlineDocWithGeometry(t, "fused-doc",
		map[int]string{1: labels, 2: fused},
		map[int]string{1: labelsGeom, 2: fusedGeom})
	f, err := parse(strings.NewReader(src), "fused.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(d, f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	rule := &f.Rules[0]
	// The count guard is satisfied, which is the point of the fixture.
	if got, want := rule.expectedValues(&rule.Parts[1]), 2; got != want {
		t.Fatalf("the fixture expects %d values, want %d; it no longer isolates "+
			"the line count from the value count", got, want)
	}
	_, _, err = res.Values(rule, &rule.Parts[1])
	if err == nil {
		t.Fatal("Values succeeded on two rows fused onto one printed line")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"covers 1 printed line ", "but the rule has 2 rows here"} {
		if !strings.Contains(got, want) {
			t.Errorf("error = %s\nwant it to mention %q", got, want)
		}
	}
}

// TestGuardRefusesWhatItCannotCheck covers the two ways a part that asked for the
// column guard cannot get one. Both must refuse: a part that declared
// column_headers and then read without them would be the silent degradation the
// required interface method exists to prevent.
func TestGuardRefusesWhatItCannotCheck(t *testing.T) {
	t.Run("the page has no geometry", func(t *testing.T) {
		d := inlineDoc(t, "sparse-doc", map[int]string{1: sparsePage})
		f, err := parse(strings.NewReader(sparseRule), "sparse.yaml")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		res, err := NewResolver(d, f)
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		_, _, err = res.Values(&f.Rules[0], &f.Rules[0].Parts[0])
		if err == nil {
			t.Fatal("Values succeeded without the geometry the part asked to be checked against")
		}
		got := diagnosis(t, err)
		for _, want := range []string{"geometry/p0001.json", "cannot carry the column guard"} {
			if !strings.Contains(got, want) {
				t.Errorf("error = %s\nwant it to mention %q", got, want)
			}
		}
	})

	t.Run("the two substrates disagree", func(t *testing.T) {
		// One word dropped from the geometry of a line the page text still
		// carries three tokens on. Neither substrate is authoritative, so the
		// only safe answer is to refuse the page and say what each one saw.
		short := geomJSON("sparse-doc", 1, []word{
			{100, 10, 112, 18, "FY"}, {114, 10, 130, 18, "2024"},
			{200, 10, 212, 18, "FY"}, {214, 10, 230, 18, "2025"},
			{300, 10, 312, 18, "FY"}, {314, 10, 330, 18, "2026"},
			{10, 30, 60, 38, "REVENUES:"},
			{10, 50, 40, 58, "Alpha"}, {320, 50, 335, 58, "300"},
			{10, 70, 35, 78, "Beta"}, {120, 70, 135, 78, "400"},
			{220, 70, 235, 78, "500"}, {320, 70, 335, 78, "600"},
			{10, 90, 45, 98, "TOTAL"},
		})
		d := inlineDocWithGeometry(t, "sparse-doc",
			map[int]string{1: sparsePage}, map[int]string{1: short})
		f, err := parse(strings.NewReader(sparseRule), "sparse.yaml")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		res, err := NewResolver(d, f)
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		_, _, err = res.Values(&f.Rules[0], &f.Rules[0].Parts[0])
		if err == nil {
			t.Fatal("Values succeeded on a page whose two substrates disagree")
		}
		got := diagnosis(t, err)
		for _, want := range []string{
			"line 3 has 3 tokens in the page text but 2 words in the geometry",
			`page text "Alpha 100 300"`, `geometry "Alpha 300"`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("error = %s\nwant it to mention %q", got, want)
			}
		}
	})
}

// TestAWrappedFigureIsCaughtAndTheLineNumbersAreThePageS covers the case where a
// row's last figure is printed on the next line rather than its own -- the band
// is right, so placement has nothing to say, and the token count is right too.
//
// The fixture opens with blank lines on purpose. A message that numbered lines by
// their position among the NON-BLANK ones would report 2 and 3 here instead of 5
// and 6, sending a reader to the wrong rows of a page whose blank lines they
// cannot see in the error.
func TestAWrappedFigureIsCaughtAndTheLineNumbersAreThePageS(t *testing.T) {
	page := "\n\n\n          FY 2024   FY 2025   FY 2026\nAlpha       100       200\n" +
		"                                300\nBeta        400       500       600\nTOTAL\n"
	g := geomJSON("wrap-doc", 1, []word{
		{100, 10, 112, 18, "FY"}, {114, 10, 130, 18, "2024"},
		{200, 10, 212, 18, "FY"}, {214, 10, 230, 18, "2025"},
		{300, 10, 312, 18, "FY"}, {314, 10, 330, 18, "2026"},
		{10, 30, 40, 38, "Alpha"}, {120, 30, 135, 38, "100"}, {220, 30, 235, 38, "200"},
		{320, 50, 335, 58, "300"},
		{10, 70, 35, 78, "Beta"}, {120, 70, 135, 78, "400"},
		{220, 70, 235, 78, "500"}, {320, 70, 335, 78, "600"},
		{10, 90, 45, 98, "TOTAL"},
	})
	src := `schema_version: 1
doc_id: wrap-doc
rules:
  - id: wrap
    kind: revenue
    basis: adopted
    units: dollars
    parts:
      - page: 1
        section: "FY 2026"
        stop_at: "TOTAL"
        columns: [{fund_group: g, fiscal_year: 2024},
                  {fund_group: g, fiscal_year: 2025},
                  {fund_group: g, fiscal_year: 2026}]
        column_headers: ["FY 2024", "FY 2025", "FY 2026"]
    rows:
      - {label: "Alpha", category: c}
      - {label: "Beta", category: c}
`
	d := inlineDocWithGeometry(t, "wrap-doc", map[int]string{1: page}, map[int]string{1: g})
	f, err := parse(strings.NewReader(src), "wrap.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(d, f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	_, _, err = res.Values(&f.Rules[0], &f.Rules[0].Parts[0])
	if err == nil {
		t.Fatal("Values succeeded on a row whose last figure is printed on the next line")
	}
	got := diagnosis(t, err)
	for _, want := range []string{
		`row "Alpha" column 3`,
		`"300" is printed on line 6 but this row's earlier figures are on line 5`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error = %s\nwant it to mention %q", got, want)
		}
	}
}

// cipDoc builds an extraction of the CIP from the committed fixtures.
func cipDoc(t *testing.T, pages ...int) *corpus.Doc {
	t.Helper()
	return testDoc(t, cipFixtures, pages)
}

// cipRule writes a one-part rule over a CIP page, with eight columns: the seven
// fiscal years the plan prints plus the row-wise TOTAL the page ends with.
func cipRule(t *testing.T, page int, section, stopAt, label string) (*Resolver, *Rule, *Part) {
	t.Helper()
	years := []string{"2024-25", "2025-26", "2026-27", "2027-28", "2028-29", "2029-30", "2030-45"}
	cols := make([]string, 0, len(years)+1)
	headers := make([]string, 0, len(years)+1)
	for i, y := range years {
		cols = append(cols, fmt.Sprintf("{fund_group: cip, fiscal_year: %d}", 2025+i))
		headers = append(headers, "FY "+y)
	}
	cols = append(cols, "{fund_group: cip, fiscal_year: 2045, skip: true}")
	headers = append(headers, "TOTAL")

	src := "schema_version: 1\ndoc_id: livermore-cip-fy2026-2030\nrules:\n" +
		"  - id: cip\n    kind: expenditure\n    basis: adopted\n    units: dollars\n" +
		"    parts:\n      - page: " + fmt.Sprint(page) + "\n" +
		"        section: " + quote(section) + "\n" +
		"        stop_at: " + quote(stopAt) + "\n" +
		"        columns: [" + strings.Join(cols, ", ") + "]\n" +
		headerYAML(headers) +
		"    rows:\n      - {label: " + quote(label) + ", category: c}\n"
	f, err := parse(strings.NewReader(src), "cip.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := NewResolver(cipDoc(t, page), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return res, &f.Rules[0], &f.Rules[0].Parts[0]
}

// TestCIPp29SplitFigureFailsClosed is fisc-j5p, and it disproves that bead's own
// hypothesis about how it would be caught.
//
// CIP p29 prints one figure of $21,130,083 as "$21130 083". The bead expected
// geometry to hold ONE word there, so that a count cross-check between the
// substrates would see one word against two tokens and refuse. It does not:
// geometry holds two words, at x1 280.45 and 299.67, the substrates agree
// token for token and line for line, and every count matches.
//
// What catches it is placement. Both right edges fall in the SAME band -- band 0
// ends at 301.54 -- so the token the rule reads as its second column is printed
// in its first, and the guard says so. Without that, amount.Parse("$21130")
// succeeds and $21,130 is published as the value of a printed $21,130,083, with
// the rest of the row shifted one place; splitDigits cannot see it because the
// split falls across a token boundary.
//
// The part must be LABELLED. A label-less part would be refused by the value
// count first -- 30 tokens where 8 were wanted -- and this test would be showing
// something other than what it claims.
func TestCIPp29SplitFigureFailsClosed(t *testing.T) {
	res, rule, part := cipRule(t, 29, "TOTAL - DOWNTOWN", "CITY OF", "REVITILIZATION")

	_, _, err := res.Values(rule, part)
	if err == nil {
		t.Fatal("Values succeeded on a row whose first figure is split across a space; " +
			"$21,130 would be published as the value of a printed $21,130,083")
	}
	got := diagnosis(t, err)
	for _, want := range []string{
		`row "REVITILIZATION" column 2`,
		`"083" ends at x 299.67`,
		"which is column 1",
		"reads it as column 2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error = %s\nwant it to mention %q", got, want)
		}
	}
}

// TestCIPp40SparseRowFailsClosedButDoesNotRead is the acceptance criterion's CIP
// half, and the name carries the half that is NOT satisfied.
//
// PB200654 prints two figures with six columns between them, and the page PRINTS
// a "-" in each of those six. Those dashes are in no substrate at all -- not
// -layout, not -raw, not the default mode, not -bbox -- because they are drawn
// as non-text, while PB200429 on the same page does carry its dashes. So the row
// cannot be read correctly by any amount of geometry, and this test does not
// claim it can (fisc-8ln).
//
// What it claims is the half that matters here: the read fails, and it fails
// NAMING THE ROW rather than filing the second 550,000 under a year the city
// never put it in.
//
// IT NOW ACTUALLY REACHES THE ROW READ, which is fisc-i0d9. It used to anchor
// the section on "PROJECT NAME" -- the column-header line's own label -- so the
// block began at the headers, and checkGap's LEADING-GAP arm refused before any
// row was read: "figures appear before the first row". The single assertion was
// Contains("PB200654"), and that message quotes the first row's ANCHOR NAME, so
// it passed on an error with nothing to do with the sparse row. The test was
// cited as evidence for row-read behaviour it never executed.
//
// The anchor now starts the block past the header line -- on its trailing
// "TOTAL", ordinal 1, since "TOTAL - PARKS & BEAUTIFICATION" below is the second
// -- and the rule declares the two rows above PB200654 so their figures are
// consumed rather than sitting in the leading gap. PB200654's wrapped label
// needs a wrapped_labels entry, which is the third thing this page demands
// before a read gets through at all.
//
// AND THE GUARD THAT FIRES IS NOT THE ONE THE BEAD EXPECTED. fisc-i0d9 predicted
// that reverting the geometry column guard would change the outcome. It does
// not: the value count refuses first, because the row yields 2 tokens against 8
// columns, and geometry never gets a chance to place them. Measured both ways --
// with column_headers declared and without, the error is identical. So on THIS
// row the value count is the whole defence, and it is load-bearing in a way
// worth stating: neutering it does not produce a wrong read, it panics on the
// `toks[:ncols]` two lines below ("slice bounds out of range [:8] with capacity
// 2").
func TestCIPp40SparseRowFailsClosedButDoesNotRead(t *testing.T) {
	src := `schema_version: 1
doc_id: livermore-cip-fy2026-2030
rules:
  - id: cip
    kind: expenditure
    basis: adopted
    units: dollars
    parts:
      - page: 40
        # The header line's trailing "TOTAL". Ordinal 1 because the page's
        # closing "TOTAL - PARKS & BEAUTIFICATION" row carries the word again.
        section: "TOTAL"
        section_ordinal: 1
        stop_at: "Hagemann"
        column_headers: ["FY 2024-25", "FY 2025-26", "FY 2026-27", "FY 2027-28",
                         "FY 2028-29", "FY 2029-30", "FY 2030-45", "TOTAL"]
        wrapped_labels: ["Decorative Wall Replacement -"]
        columns: [{fund_group: cip, fiscal_year: 2025}, {fund_group: cip, fiscal_year: 2026},
                  {fund_group: cip, fiscal_year: 2027}, {fund_group: cip, fiscal_year: 2028},
                  {fund_group: cip, fiscal_year: 2029}, {fund_group: cip, fiscal_year: 2030},
                  {fund_group: cip, fiscal_year: 2031},
                  {fund_group: cip, fiscal_year: 2045, skip: true}]
    rows:
      - {label: "PB200429 Rehabilitation", category: c}
      - {label: "PB200646 LARPD Pa rk Expans ion Projects", category: c}
      - {label: "PB200654 Holmes Street", category: c}
`
	f, err := parse(strings.NewReader(src), "cip.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rule := &f.Rules[0]
	res, err := NewResolver(cipDoc(t, 40), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	values, _, err := res.Values(rule, &rule.Parts[0])
	if err == nil {
		t.Fatalf("Values returned %d values for a row six of whose cells are in no "+
			"substrate; it cannot have read the row", len(values))
	}

	got := diagnosis(t, err)
	if !strings.Contains(got, "PB200654") {
		t.Errorf("error = %s\nwant it to name the row", got)
	}
	// THE ASSERTION THE OLD TEST WAS MISSING. Naming the row is not enough:
	// checkGap's leading-gap message quotes the first row's anchor too, which is
	// how this test passed for a month on an error about the column headers.
	if !strings.Contains(got, "is followed by 2 values, want 8") {
		t.Errorf("error = %s\nwant the ROW READ to refuse, reporting the value count", got)
	}
	if strings.Contains(got, "figures appear before the first row") {
		t.Errorf("error = %s\nthe block still starts at the column headers, so the "+
			"row read is not reached; this is fisc-i0d9 again", got)
	}
	// The figure is not filed anywhere either, but that is true BY CONSTRUCTION
	// rather than by assertion: Values and readPart both return (nil, nil, err)
	// on every error path, so `if len(values) != 0` here could not fail and
	// would read as a guard. What the Fatalf above already establishes is the
	// checkable half -- that the read did not succeed.
}
