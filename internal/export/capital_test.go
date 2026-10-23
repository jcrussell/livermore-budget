package export_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// capitalSeries is one project line of a capital fixture document: its fund,
// its printed label, and one (cents, token, page) per column.
type capitalSeries struct {
	fund   int
	label  string
	points []capitalPoint
}

type capitalPoint struct {
	cents int64
	token string
	page  int
}

// capitalColumns are the listing's five columns as the document declares
// them: two adopted, three projected.
var capitalColumns = []struct {
	year  int
	basis string
}{{2026, "adopted"}, {2027, "adopted"}, {2028, "projected"}, {2029, "projected"}, {2030, "projected"}}

// capitalDoc is a minimal cip-projects document. Hand-written rather than
// built by internal/project, for historyDoc's reason. counts is the counts
// block's points and facts, or -1 for the true number.
func capitalDoc(counts int, series ...capitalSeries) []byte {
	cols := make([]map[string]any, 0, len(capitalColumns))
	for _, c := range capitalColumns {
		cols = append(cols, map[string]any{
			"fiscal_year": c.year, "fiscal_year_label": fmt.Sprintf("FY %d-%02d", c.year-1, c.year%100),
			"basis": c.basis, "comparable_group": c.basis,
		})
	}
	out := make([]map[string]any, 0, len(series))
	points := 0
	for i, s := range series {
		pts := make([]map[string]any, 0, len(s.points))
		for j, p := range s.points {
			points++
			pts = append(pts, map[string]any{
				"fiscal_year": capitalColumns[j].year, "basis": capitalColumns[j].basis,
				"amount_cents": p.cents, "fact_id": fmt.Sprintf("fisc-f-%06d%06d", i, j),
				"doc_id": budgetDocID, "page": p.page, "offset": 10*i + j, "token": p.token,
				"derived": false,
			})
		}
		fund := s.fund
		out = append(out, map[string]any{
			"series_id": fmt.Sprintf("fisc-s-%012d", i), "label": s.label,
			"fund": &fund, "fund_name": fmt.Sprintf("Fund name %d", s.fund), "fund_group": "capital",
			"kind": "expenditure", "category": "capital-projects", "category_label": "Capital Projects",
			"points": pts,
		})
	}
	if counts < 0 {
		counts = points
	}
	doc := map[string]any{
		"schema_version": 1,
		"projection":     "cip-projects",
		"metadata": map[string]any{
			"generated_by": "fisc test", "scope": "cip-project-listing", "currency": "USD", "units": "cents",
			"columns": cols,
			"sources": []map[string]any{{"doc_id": budgetDocID, "pages": []int{225, 227}}},
			"counts":  map[string]any{"facts": counts, "series": len(series), "points": counts},
			"caveats": []map[string]any{{"id": "a-caveat", "summary": "A caveat summary.", "theme": "left-out",
				"text": "A caveat's whole text.", "applies_to": []string{}}},
		},
		"series": out,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return b
}

// capitalFixture is two funds, the first with two projects, one of which
// prints a dash, on two pages.
func capitalFixture() []capitalSeries {
	five := func(page int, cents ...int64) []capitalPoint {
		out := make([]capitalPoint, 0, len(cents))
		for _, c := range cents {
			tok := "-"
			if c != 0 {
				tok = fmt.Sprintf("%d", c/100)
			}
			out = append(out, capitalPoint{cents: c, token: tok, page: page})
		}
		return out
	}
	return []capitalSeries{
		{101, "101 BL201518 Carpet Replacement", five(225, 10000000, 0, 500000, 0, 0)},
		{101, "101 PS202316 Police Building Renovations", five(225, 20000000, 30000000, 0, 0, 0)},
		{510, "510 TI199238 Isabel Avenue/I-580 Interchange", five(227, 40000000, 0, 0, 0, 100000)},
	}
}

// writeCapitalSite exports the capital view over the given document.
func writeCapitalSite(t *testing.T, doc []byte) (string, error) {
	t.Helper()
	dir := t.TempDir()
	_, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t), "cip-projects": doc},
		Views: append(spineOnly(), export.View{Path: "capital.html", Nav: "Capital",
			Template: export.CapitalTemplate, Projection: "cip-projects",
			Title: "A capital title", Lede: "A capital lede."}),
		Docs:     budgetDocs(),
		PageText: twoViewPageText(225, 227),
	})
	return dir, err
}

// TestCapitalPageRendersEveryProjectCitedToItsPage holds the capital view to
// its document: one row per series under its fund, one fund option per fund,
// every figure a link to the page its point cites, a printed dash rendered as
// the dash, the projected columns saying so in their headers, and the
// footer's sum labelled as ours rather than as the listing's TOTAL.
//
// Mutation: render a zero cell as its formatted Value rather than its token
// and the dash assertion goes red ("$0" is printed); drop the {{$c.Basis}}
// span from the template's header and the projected assertion goes red.
func TestCapitalPageRendersEveryProjectCitedToItsPage(t *testing.T) {
	dir, err := writeCapitalSite(t, capitalDoc(-1, capitalFixture()...))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "capital.html")
	for _, want := range []string{
		"<h1>A capital title</h1>",
		"A capital lede.",
		`import { boot } from "./capital.js"; boot(document);`,
		`<span aria-current="page">Capital</span>`,
		`id="sources-view"`,
		`<option value="101">101 Fund name 101</option>`,
		`<option value="510">510 Fund name 510</option>`,
		`<th scope="row">BL201518 Carpet Replacement</th>`,
		"<li>A caveat summary.</li>",
		"◇ Our sum of the rows shown",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("capital.html does not carry %q", want)
		}
	}
	if got := strings.Count(html, `<tr data-fund="101">`); got != 2 {
		t.Errorf("fund 101 has %d rows, want 2", got)
	}
	if got := strings.Count(html, `<tr data-fund="510">`); got != 1 {
		t.Errorf("fund 510 has %d rows, want 1", got)
	}
	if strings.Contains(html, ">$0<") {
		t.Error("a zero renders as $0, a figure the page did not print; it prints a dash")
	}
	if got := strings.Count(html, `title="Read on page 225">-</a>`); got != 6 {
		t.Errorf("fund 101's dashes render %d times as linked dashes, want 6", got)
	}
	if strings.Contains(html, ">Total<") || strings.Contains(html, ">TOTAL<") {
		t.Error("the page labels a figure Total; its only sum is ours, of the rows shown")
	}
	// Every cell's link is the page its point cites.
	cell := regexp.MustCompile(`<a href="[^"]*p(\d{4})\.txt" title="Read on page (\d+)">([^<]*)</a>`)
	cells := cell.FindAllStringSubmatch(html, -1)
	if len(cells) != 15 {
		t.Fatalf("capital.html links %d cells, want 15", len(cells))
	}
	for _, m := range cells {
		if n, err := strconv.Atoi(m[2]); err != nil || fmt.Sprintf("%04d", n) != m[1] {
			t.Errorf("cell %q links page %s and says it is on page %s", m[3], m[1], m[2])
		}
	}
	if !strings.Contains(html, `title="Read on page 227">$400,000</a>`) {
		t.Error("fund 510's FY2025-26 figure does not link page 227")
	}
	head := between(t, html, "<thead>", "</thead>")
	if got := strings.Count(head, `<span class="basis">projected</span>`); got != 3 {
		t.Errorf("%d column headers say projected, want 3", got)
	}
	if strings.Contains(html, "publishes no figure yet") {
		t.Error("the footer still says the page publishes no figure")
	}
	if _, err := os.Stat(filepath.Join(dir, "capital.js")); err != nil {
		t.Errorf("capital.html loads ./capital.js and the site does not carry it: %v", err)
	}
}

// TestCapitalPageRefusesADocumentThatLostAFigure: counts that disagree with
// the cells laid out are refused, as on the history template's pages.
//
// Mutation: drop the reconcileSeriesCounts call from buildCapitalPage and
// this goes green on a page claiming 16 figures over 15.
func TestCapitalPageRefusesADocumentThatLostAFigure(t *testing.T) {
	_, err := writeCapitalSite(t, capitalDoc(16, capitalFixture()...))
	if err == nil || !strings.Contains(err.Error(), "counts.points 16") {
		t.Fatalf("Write = %v, want the counts refused", err)
	}
}

// TestCapitalPageRefusesARowNotOpeningWithItsFund: the Project cell is the
// printed label less its fund number, so a label that does not open with it
// is refused rather than cut wrong.
func TestCapitalPageRefusesARowNotOpeningWithItsFund(t *testing.T) {
	rows := capitalFixture()
	rows[2].label = "511 TI199238 Isabel Avenue/I-580 Interchange"
	_, err := writeCapitalSite(t, capitalDoc(-1, rows...))
	if err == nil || !strings.Contains(err.Error(), "does not open with its fund number 510") {
		t.Fatalf("Write = %v, want the label refused", err)
	}
}
