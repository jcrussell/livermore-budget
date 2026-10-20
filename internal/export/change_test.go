package export_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// changeDocs are a ten-year changes document of two audited years and a
// revenue-trends document of two budget columns: revenue, expenditure and the
// printed excess on the audited side, a revenue line and a transfer in on the
// budget side. `extra` adds a series of a kind the page draws nowhere.
func changeDocs(extra bool) map[string][]byte {
	point := func(year int, basis string, cents int64, page int) string {
		return fmt.Sprintf(`{"fiscal_year":%d,"basis":%q,"amount_cents":%d,"fact_id":"fisc-f-%012x",`+
			`"doc_id":"livermore-acfr-fy2025","page":%d,"offset":1,"token":"1","derived":false}`, year, basis, cents, year*100+page+int(cents%7), page)
	}
	series := func(label, kind, category string, fund string, points ...string) string {
		return fmt.Sprintf(`{"series_id":"fisc-s-%012x","label":%q,"fund":%s,"fund_name":"","fund_group":"","kind":%q,`+
			`"category":"c","category_label":%q,"points":[%s]}`, len(label)*31+len(kind), label, fund, kind, category, strings.Join(points, ","))
	}
	audited := []string{
		series("Property taxes", "revenue", "Property Taxes", "null", point(2024, "audited", 100_00, 168), point(2025, "audited", 120_00, 168)),
		series("Police", "expenditure", "Police", "null", point(2024, "audited", 70_00, 168), point(2025, "audited", 90_00, 168)),
		series("over (under) expenditures", "fund_balance", "Excess", "null", point(2024, "audited", 30_00, 168), point(2025, "audited", 30_00, 168)),
	}
	n := 6
	if extra {
		audited = append(audited, series("Other financing", "other", "Other", "null", point(2024, "audited", 1_00, 169), point(2025, "audited", 1_00, 169)))
		n = 8
	}
	changes := fmt.Sprintf(`{"schema_version":1,"projection":%q,"metadata":{"generated_by":"fisc (test)","scope":"acfr",`+
		`"sources":[{"doc_id":"livermore-acfr-fy2025","pages":[168]}],"columns":[`+
		`{"fiscal_year":2024,"fiscal_year_label":"FY 2023-24","basis":"audited","comparable_group":"audited"},`+
		`{"fiscal_year":2025,"fiscal_year_label":"FY 2024-25","basis":"audited","comparable_group":"audited"}],`+
		`"counts":{"facts":%d,"series":%d,"points":%d},"caveats":[]},"series":[%s]}`,
		project.ChangesProjection, n, len(audited), n, strings.Join(audited, ","))
	bpoint := func(year int, basis string, cents int64) string {
		return fmt.Sprintf(`{"fiscal_year":%d,"basis":%q,"amount_cents":%d,"fact_id":"fisc-f-%012x",`+
			`"doc_id":"livermore-budget-fy2026-2027","page":127,"offset":1,"token":"1","derived":false}`, year, basis, cents, year*7+int(cents))
	}
	budget := fmt.Sprintf(`{"schema_version":1,"projection":%q,"metadata":{"generated_by":"fisc (test)","scope":"revenue-by-fund",`+
		`"sources":[{"doc_id":"livermore-budget-fy2026-2027","pages":[127]}],"columns":[`+
		`{"fiscal_year":2026,"fiscal_year_label":"FY 2025-26","basis":"adopted","comparable_group":"adopted"},`+
		`{"fiscal_year":2027,"fiscal_year_label":"FY 2026-27","basis":"adopted","comparable_group":"adopted"}],`+
		`"counts":{"facts":4,"series":2,"points":4},"caveats":[]},"series":[%s,%s]}`,
		project.TrendsProjection,
		series("Current Year - Secured", "revenue", "Property Taxes", "100", bpoint(2026, "adopted", 500_00), bpoint(2027, "adopted", 520_00)),
		series("Transfer from General Fund", "transfer_in", "Transfers In", "100", bpoint(2026, "adopted", 5_00), bpoint(2027, "adopted", 6_00)))
	return map[string][]byte{project.ChangesProjection: []byte(changes), project.TrendsProjection: []byte(budget)}
}

// changeOptions is a site with the spine and the change page over changeDocs.
func changeOptions(t *testing.T, dir string, extra bool) export.Options {
	t.Helper()
	projections := map[string][]byte{export.PrimaryProjection: goldenSankey(t)}
	for k, v := range changeDocs(extra) {
		projections[k] = v
	}
	return export.Options{
		Dir:         dir,
		Projections: projections,
		Views: append(spineOnly(), export.View{Path: "change.html", Nav: "Over time",
			Template: export.ChangeTemplate, Projection: project.ChangesProjection,
			Title: "A change title", Lede: "A change lede."}),
		Docs: append(budgetDocs(), export.Doc{ID: "livermore-acfr-fy2025", Title: "An ACFR", PDFURL: "https://example.test/acfr.pdf"}),
	}
}

var changeCell = regexp.MustCompile(`<tr data-category="([^"]+)">\s*<th scope="row">([^<]+)</th>[\s\S]*?</tr>`)

// TestChangePagePrintsADocumentsCaveatsOnce holds the page to one statement
// of a document's caveats, on the page's line below every figure and inside
// none, where three figures are drawn from the audited document.
//
// Mutation: render the "caveats-line" template inside each figure and the
// audited caveat is printed five times. Mutation: move the line inside the
// last figure, before its closing tag, and it is printed once but inside it.
func TestChangePagePrintsADocumentsCaveatsOnce(t *testing.T) {
	dir := t.TempDir()
	o := changeOptions(t, dir, false)
	changes := string(o.Projections[project.ChangesProjection])
	const bare = `"caveats":[]},"series"`
	if strings.Count(changes, bare) != 1 {
		t.Fatal("the changes document's caveats are not where this test expects them")
	}
	o.Projections[project.ChangesProjection] = []byte(strings.Replace(changes, bare,
		`"caveats":[{"id":"ten-years-unaudited","summary":"Ten unaudited years.","text":"A text.","applies_to":[]}]},"series"`, 1))
	if _, err := writeSite(o); err != nil {
		t.Fatalf("Write: %v", err)
	}
	page := readFile(t, dir, "change.html")
	if got := strings.Count(page, "Ten unaudited years."); got != 1 {
		t.Errorf("the audited document's caveat is printed %d times, want once", got)
	}
	at := strings.Index(page, "Ten unaudited years.")
	rest, end := page, 0
	for {
		open := strings.Index(rest, `<section class="change-figure"`)
		if open < 0 {
			break
		}
		closing := strings.Index(rest[open:], "</section>")
		if closing < 0 {
			t.Fatalf("the change figure opening at %d is never closed", end+open)
		}
		closing += open + len("</section>")
		if at >= end+open && at < end+closing {
			t.Errorf("the caveat is printed at %d, inside the figure spanning %d-%d", at, end+open, end+closing)
		}
		rest, end = rest[closing:], end+closing
	}
	if end == 0 || at < end {
		t.Errorf("the caveat is printed at %d, not below the last figure, which closes at %d", at, end)
	}
}

// TestChangePageRendersEachKindAsItsOwnFigure holds the page to four figures
// in order, each the rows of one kind of one document with every printed
// cell on it, and the budget's transfers in left out.
//
// Mutation: drop the kind filter in changeRows and the audited revenue
// figure lists the police row.
func TestChangePageRendersEachKindAsItsOwnFigure(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(changeOptions(t, dir, false)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "change.html")
	for _, want := range []string{"<h1>A change title</h1>", "A change lede.", `import { boot } from "./change.js"`,
		`href="index.html"`, `data-figure="audited-revenue" data-chart="true"`, `data-figure="audited-excess" data-chart="false"`,
		`data-figure="budget-revenue" data-chart="true"`, `<div class="change-cells">`,
		`data-column="1" data-cents="12000">`, `href="https://example.test/acfr.pdf#page=168"`} {
		if !strings.Contains(html, want) {
			t.Errorf("change.html does not contain %q", want)
		}
	}
	parts := strings.Split(html, `<section class="change-figure" data-figure="`)
	if len(parts) != 5 {
		t.Fatalf("got %d figures, want four", len(parts)-1)
	}
	want := map[string][]string{
		"audited-revenue":  {"Property Taxes|Property taxes"},
		"audited-spending": {"Police|Police"},
		"audited-excess":   {"Excess|over (under) expenditures"},
		"budget-revenue":   {"Property Taxes|Current Year - Secured"},
	}
	for _, p := range parts[1:] {
		id, body, _ := strings.Cut(p, `"`)
		var got []string
		for _, m := range changeCell.FindAllStringSubmatch(body, -1) {
			got = append(got, m[1]+"|"+m[2])
		}
		if fmt.Sprint(got) != fmt.Sprint(want[id]) {
			t.Errorf("%s rows: got %v, want %v", id, got, want[id])
		}
	}
	if strings.Contains(html, "Transfer from General Fund") {
		t.Error("the budget figure lists a transfer in; the page draws revenue alone")
	}
}

// TestChangePageRefusesWhatItCannotDraw: the page is refused with no
// revenue-trends document, over another projection, and when a kind of the
// audited document is drawn in no figure, so a point can go missing in
// silence.
//
// Mutation: drop the placed-points check in buildChangePage and the "kind
// drawn nowhere" case writes a page short of the document.
func TestChangePageRefusesWhatItCannotDraw(t *testing.T) {
	o := changeOptions(t, t.TempDir(), false)
	delete(o.Projections, project.TrendsProjection)
	if _, err := writeSite(o); err == nil || !strings.Contains(err.Error(), "which was not built") {
		t.Errorf("no revenue-trends: got %v, want a refusal", err)
	}
	o = changeOptions(t, t.TempDir(), false)
	o.Views[1].Projection = project.TrendsProjection
	if _, err := writeSite(o); err == nil || !strings.Contains(err.Error(), "the page's own document is") {
		t.Errorf("another projection: got %v, want a refusal", err)
	}
	if _, err := writeSite(changeOptions(t, t.TempDir(), true)); err == nil || !strings.Contains(err.Error(), "a kind of its series is drawn nowhere") {
		t.Errorf("a kind drawn nowhere: got %v, want a refusal", err)
	}
	// The budget document's transfers in are left out by declaration and its
	// revenue placed; a series of any other kind is drawn nowhere.
	// Mutation: drop the budget document from the reconciliation and the
	// page renders with the series gone.
	o = changeOptions(t, t.TempDir(), false)
	budget := string(o.Projections[project.TrendsProjection])
	for _, edit := range []struct{ name, old, new string }{
		{"a budget series of a kind drawn nowhere", `"kind":"transfer_in"`, `"kind":"expenditure"`},
		{"a budget document counting a point it does not carry", `"counts":{"facts":4,"series":2,"points":4}`,
			`"counts":{"facts":4,"series":2,"points":5}`},
	} {
		if strings.Count(budget, edit.old) != 1 {
			t.Fatalf("%s: the budget document carries %q %d times, want once", edit.name, edit.old, strings.Count(budget, edit.old))
		}
		o = changeOptions(t, t.TempDir(), false)
		o.Projections[project.TrendsProjection] = []byte(strings.Replace(budget, edit.old, edit.new, 1))
		if _, err := writeSite(o); err == nil || !strings.Contains(err.Error(), "of revenue-trends's") ||
			!strings.Contains(err.Error(), "a kind of its series is drawn nowhere") {
			t.Errorf("%s: got %v, want a refusal", edit.name, err)
		}
	}
	// A budget point in a column the document does not declare has no cell
	// to land in, and the budget document is reconciled against nothing else.
	// Mutation: drop the placed-against-published refusal in changeRows and
	// the page renders with the point gone.
	o = changeOptions(t, t.TempDir(), false)
	stray := `"points":[{"fiscal_year":2025,"basis":"adopted","amount_cents":100,"fact_id":"fisc-f-000000000bad",` +
		`"doc_id":"livermore-budget-fy2026-2027","page":127,"offset":1,"token":"1","derived":false},{"fiscal_year":2026,"basis":"adopted","amount_cents":50000`
	if strings.Count(budget, `"points":[{"fiscal_year":2026,"basis":"adopted","amount_cents":50000`) != 1 {
		t.Fatal("the budget revenue series is not where this test expects it")
	}
	o.Projections[project.TrendsProjection] = []byte(strings.Replace(budget, `"points":[{"fiscal_year":2026,"basis":"adopted","amount_cents":50000`, stray, 1))
	if _, err := writeSite(o); err == nil || !strings.Contains(err.Error(), "fall in a declared column") {
		t.Errorf("a budget point in no declared column: got %v, want a refusal", err)
	}
}
