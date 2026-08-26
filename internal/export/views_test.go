package export_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// trendsDoc is a minimal revenue-trends document citing pages the SPINE does
// not, which is the whole point: it is the shape fisc-fjy was filed about.
//
// It is hand-written rather than built by internal/project, because this package
// deliberately does not import it -- the packager consumes projections as bytes
// and knows nothing about how they were built, and a test that reached for the
// producer would quietly retire that seam.
func trendsDoc(pages ...int) []byte {
	type point struct {
		FiscalYear  int    `json:"fiscal_year"`
		Basis       string `json:"basis"`
		AmountCents int64  `json:"amount_cents"`
		FactID      string `json:"fact_id"`
		DocID       string `json:"doc_id"`
		Page        int    `json:"page"`
		Offset      int    `json:"offset"`
		Token       string `json:"token"`
		Derived     bool   `json:"derived"`
	}
	points := make([]point, 0, len(pages))
	for i, p := range pages {
		points = append(points, point{
			FiscalYear: 2026 + i, Basis: "adopted", AmountCents: 100 + int64(i),
			FactID: fmt.Sprintf("fisc-f-%012d", i), DocID: budgetDocID, Page: p,
			Offset: 1, Token: "1.00",
		})
	}
	// The columns follow the points, so a fixture built with no pages is a
	// document with no columns -- which is the state
	// TestTheTrendsViewRefusesADocumentWithNoColumns is about, and it would
	// assert nothing against a hard-coded list.
	columns := make([]map[string]any, 0, len(points))
	for i := range points {
		columns = append(columns, map[string]any{
			"fiscal_year":       2026 + i,
			"fiscal_year_label": fmt.Sprintf("FY %d-%02d", 2025+i, (26+i)%100),
			"basis":             "adopted",
			"comparable_group":  "adopted",
		})
	}
	doc := map[string]any{
		"schema_version": 1,
		"projection":     "revenue-trends",
		"metadata": map[string]any{
			"generated_by": "fisc test",
			"scope":        "revenue-by-fund",
			"currency":     "USD",
			"units":        "cents",
			"columns":      columns,
			"sources":      []map[string]any{{"doc_id": budgetDocID, "pages": pages}},
			"counts":       map[string]any{"facts": len(points), "series": 1, "points": len(points)},
			"caveats":      []string{"a caveat this document carries"},
		},
		"series": []map[string]any{{
			"series_id": "fisc-s-000000000000", "label": "Property Taxes",
			"fund": 100, "fund_name": "General Fund", "fund_group": "general",
			"kind": "revenue", "category": "taxes/property", "category_label": "Property Taxes",
			"points": points,
		}},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return b
}

// twoViews writes a site with the spine page and a revenue page whose document
// cites DISJOINT pages, and returns the output directory.
func twoViews(t *testing.T, trendsPages ...int) string {
	t.Helper()
	dir := t.TempDir()
	tree := pageTextFS()
	for _, p := range trendsPages {
		tree[fmt.Sprintf("%s/pages/p%04d.txt", budgetDocID, p)] =
			&fstest.MapFile{Data: []byte(fmt.Sprintf("page %d\n", p))}
	}
	_, err := export.Write(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey":         goldenSankey(t),
			"revenue-trends": trendsDoc(trendsPages...),
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Projection: "sankey"},
			{Path: "revenue.html", Nav: "Revenue by fund",
				Template: export.TrendsTemplate, Projection: "revenue-trends",
				Title: "Revenue by fund", Lede: "A lede."},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(trendsPages...),
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

// TestTwoViewsCitingDisjointPagesBothShip is fisc-fjy, and it is the acceptance
// criterion that bead names.
//
// buildPage used to compose the shipped page-text set from the PRIMARY
// projection's metadata alone, which was the whole set while there was one view.
// A second view citing pages the first does not would have rendered citation
// links styled exactly like the working ones beside them, resolving in the
// template, and 404ing in the browser -- invisible to every check, because
// fact-offset-points-at-token reads the committed corpus and nothing walks the
// built tree.
func TestTwoViewsCitingDisjointPagesBothShip(t *testing.T) {
	dir := twoViews(t, 127, 128)

	// The spine's pages AND the trends view's, in one output tree.
	for _, want := range []string{
		shippedPageText(budgetDocID, 66),
		shippedPageText(budgetDocID, 67),
		shippedPageText(budgetDocID, 127),
		shippedPageText(budgetDocID, 128),
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(want))); err != nil {
			t.Errorf("%s was not shipped: %v", want, err)
		}
	}
	// And nothing else: only CITED pages are copied. p0100 is in the fixture
	// tree and cited by neither view.
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(shippedPageText(budgetDocID, 100)))); err == nil {
		t.Error("a page no view cites was shipped; the corpus is 786 pages and the site " +
			"ships the ones it names")
	}
}

// TestEachViewsFooterCitesItsOwnSources is the other half of fisc-fjy's
// decision, and it is the half that is easy to get wrong by fixing the first.
//
// The SHIPPED FILE SET is unioned; the FOOTER is not. A page listing the union
// would claim provenance for figures it never showed, which is its own defect --
// so the spine page must not advertise pp.127-128 and the revenue page must not
// advertise pp.66-67.
func TestEachViewsFooterCitesItsOwnSources(t *testing.T) {
	dir := twoViews(t, 127, 128)

	spine := readFile(t, dir, "index.html")
	revenue := readFile(t, dir, "revenue.html")

	if !strings.Contains(spine, "p66 (PDF)") || strings.Contains(spine, "p127 (PDF)") {
		t.Error("the spine page's Sources list is not its own")
	}
	if !strings.Contains(revenue, "p127 (PDF)") || strings.Contains(revenue, "p66 (PDF)") {
		t.Error("the revenue page's Sources list is not its own")
	}
}

// TestEveryCitationTheClientComposesResolves is the check fisc-fjy asks for, and
// it is NOT the check that already existed.
//
// TestEveryAssetThePageAsksForWasWritten walks src= and href= out of the rendered
// markup, and it is green -- but the citation links fisc-fjy is about are not IN
// the markup. site/app.js composes them at runtime, per cited fact, as
// CONFIG.docs[d].page_text_base + "p" + padded + ".txt", and only the footer's
// handful ever reach the HTML. So a walk of the page cannot see the class of
// link that broke.
//
// This crosses the client's own config against every document it can fetch, which
// is the set of paths the browser will actually construct.
func TestEveryCitationTheClientComposesResolves(t *testing.T) {
	dir := twoViews(t, 127, 128)
	cfg := clientConfigOf(t, readFile(t, dir, "index.html"))

	checked := 0
	for stem, rel := range cfg.Projections {
		var doc struct {
			Metadata struct {
				Sources []struct {
					DocID string `json:"doc_id"`
					Pages []int  `json:"pages"`
				} `json:"sources"`
			} `json:"metadata"`
		}
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("the client can fetch %s at %s, and it is not there: %v", stem, rel, err)
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		for _, src := range doc.Metadata.Sources {
			d, ok := cfg.Docs[src.DocID]
			if !ok {
				// Not a failure of this check: the client skips a document it
				// has no entry for, so no link is composed. Recorded so a
				// reader knows the loop did not silently cover nothing.
				t.Logf("%s cites %s, which the client config does not describe", stem, src.DocID)
				continue
			}
			for _, p := range src.Pages {
				checked++
				href := d.PageTextBase + fmt.Sprintf("p%04d.txt", p)
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(href))); err != nil {
					t.Errorf("the client would compose %q for %s page %d, and it is not in "+
						"the output: %v", href, src.DocID, p, err)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no citation path was checked; this test is asserting nothing")
	}
}

// TestEveryViewsMarkupResolves extends the existing asset walk to every page
// rather than only the one the site opens on.
func TestEveryViewsMarkupResolves(t *testing.T) {
	dir := twoViews(t, 127, 128)
	ref := regexp.MustCompile(`(?:src|href)="([^"]+)"`)

	for _, page := range []string{"index.html", "revenue.html"} {
		found := 0
		for _, m := range ref.FindAllStringSubmatch(readFile(t, dir, page), -1) {
			target := m[1]
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
				continue
			}
			found++
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(target))); err != nil {
				t.Errorf("%s references %q, which was not written: %v", page, target, err)
			}
		}
		if found < 4 {
			t.Errorf("%s has %d relative references, want at least the stylesheet and its "+
				"own data file", page, found)
		}
	}
}

// TestTheNavIsOnEveryPageAndMarksTheCurrentOne: the set of views has to be
// reachable from any one of them, and a reader has to be able to tell which one
// they are on without reading the URL.
func TestTheNavIsOnEveryPageAndMarksTheCurrentOne(t *testing.T) {
	dir := twoViews(t, 127)
	for _, c := range []struct{ page, current, other string }{
		{"index.html", "Budget flows", "revenue.html"},
		{"revenue.html", "Revenue by fund", export.IndexPath},
	} {
		html := readFile(t, dir, c.page)
		if !strings.Contains(html, `<a href="`+c.other+`"`) {
			t.Errorf("%s does not link to %s", c.page, c.other)
		}
		if !strings.Contains(html, `<span aria-current="page">`+c.current+`</span>`) {
			t.Errorf("%s does not mark %q as the current view", c.page, c.current)
		}
	}
}

// TestASingleViewSiteRendersNoNav: one view is not a set to navigate, and a nav
// listing one page is furniture. This is also what keeps the pre-views output
// unchanged for a caller that named no views at all.
func TestASingleViewSiteRendersNoNav(t *testing.T) {
	dir, _ := writeGolden(t)
	if strings.Contains(readPage(t, dir), "site-nav") {
		t.Error("a one-view site rendered a nav")
	}
	if _, err := os.Stat(filepath.Join(dir, "revenue.html")); err == nil {
		t.Error("a one-view site wrote a second page")
	}
}

// TestWriteRefusesAnUnrenderableViewSet covers every way a caller can hand this
// package a set of pages it must not write. Each one would fail in the browser
// and nowhere else.
func TestWriteRefusesAnUnrenderableViewSet(t *testing.T) {
	ok := export.View{Path: export.IndexPath, Template: export.SankeyTemplate, Projection: "sankey"}
	cases := []struct {
		name  string
		views []export.View
		want  string
	}{
		{"no index", []export.View{{Path: "revenue.html",
			Template: export.SankeyTemplate, Projection: "sankey"}}, "opens on exactly one"},
		{"two indexes", []export.View{ok, ok}, "two views claim the output path"},
		{"a path that is not html", []export.View{ok, {Path: "revenue.txt",
			Template: export.SankeyTemplate, Projection: "sankey"}}, "not an .html file"},
		{"a path in a subdirectory", []export.View{ok, {Path: "views/revenue.html",
			Template: export.SankeyTemplate, Projection: "sankey"}}, "not at the site root"},
		{"a path that shadows an asset", []export.View{ok, {Path: "app.js",
			Template: export.SankeyTemplate, Projection: "sankey"}}, "not an .html file"},
		{"a projection that was not built", []export.View{ok, {Path: "revenue.html",
			Template: export.SankeyTemplate, Projection: "nope"}}, "which was not built"},
		{"no template", []export.View{ok, {Path: "revenue.html", Projection: "sankey"}},
			"names no template"},
		{"a year stem that was not built", []export.View{{Path: export.IndexPath,
			Template: export.SankeyTemplate, Projection: "sankey",
			YearStems: []string{"sankey", "sankey-2099"}}}, "names no projection that was built"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := export.Write(export.Options{
				Dir:         t.TempDir(),
				Projections: map[string][]byte{"sankey": goldenSankey(t)},
				Views:       c.views,
				Docs:        budgetDocs(),
				GeneratedBy: "fisc test",
			})
			if err == nil {
				t.Fatalf("Write accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
		})
	}
}

// TestAnAssetCannotShadowAView is the reason reservedPaths became a property of
// Options rather than a package variable. It knew about index.html because that
// was the only page; an asset at revenue.html would have replaced a view in
// silence, and the site would have exported cleanly.
func TestAnAssetCannotShadowAView(t *testing.T) {
	_, err := export.Write(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc(127)},
		Views: []export.View{
			{Path: export.IndexPath, Template: export.SankeyTemplate, Projection: "sankey"},
			{Path: "revenue.html", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Files:       map[string][]byte{"revenue.html": []byte("not the view")},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err == nil {
		t.Fatal("an asset at a view's path was accepted")
	}
	if !strings.Contains(err.Error(), "fixed site layout") {
		t.Errorf("error %q does not say the path is the site's", err)
	}
}

// TestTheTrendsViewRendersWhatTheDocumentPublishes, and computes nothing.
//
// The packager's standing rule is that it does not recompute what a projection
// published. A table is where that is most tempting to break -- a total column,
// a growth percentage and a subtotal are each one line -- so the assertion is
// that the figures on the page are the document's own and that no total appears.
func TestTheTrendsViewRendersWhatTheDocumentPublishes(t *testing.T) {
	dir := twoViews(t, 127, 128)
	html := readFile(t, dir, "revenue.html")

	for _, want := range []string{
		"Property Taxes", // the row label and the category label
		"General Fund",   // the fund name, because the label is not unique
		"FY 2025-26",     // the column, with its own basis beside it
		"adopted",        //
		"a caveat this document carries",
		"revenue-by-fund", // the scope, stated rather than assumed
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the revenue page does not render %q", want)
		}
	}
	// No headline, no year control, no chart: this document has none of those
	// and a page inventing them would be publishing figures nobody built.
	for _, unwanted := range []string{"FISC_CONFIG", "app.js", "year-toggle", "d3"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("the revenue page carries %q, which belongs to the Sankey view", unwanted)
		}
	}
}

// TestTheTrendsViewRefusesADocumentWithNoColumns: an empty column list would
// render a table with a Fund, a Line and a Category and no figures at all, which
// looks like a page rather than a defect.
func TestTheTrendsViewRefusesADocumentWithNoColumns(t *testing.T) {
	_, err := export.Write(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc()},
		Views: []export.View{
			{Path: export.IndexPath, Template: export.SankeyTemplate, Projection: "sankey"},
			{Path: "revenue.html", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err == nil || !strings.Contains(err.Error(), "publishes no columns") {
		t.Fatalf("Write = %v, want a refusal naming the missing columns", err)
	}
}

// TestTheTrendsViewIsNotRefusedForLackingAHeadline is the seam decodeSankey's
// split exists for.
//
// The two refusals -- no fiscal_year_label, no headline expenditure -- used to
// sit in the ONE decode path every document went through. They are correct for a
// spine document and wrong for this one, which carries neither and is not
// defective for it, so a trends view under the old path could not have been
// rendered at all.
func TestTheTrendsViewIsNotRefusedForLackingAHeadline(t *testing.T) {
	raw := trendsDoc(127)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := doc["metadata"].(map[string]any)
	for _, key := range []string{"fiscal_year_label", "headline", "basis", "fiscal_year"} {
		if _, ok := meta[key]; ok {
			t.Fatalf("the fixture carries %q; this test asserts nothing unless it does not", key)
		}
	}
	// It renders anyway. twoViews would have failed the Write if it did not.
	twoViews(t, 127)
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// clientConfigOf decodes window.FISC_CONFIG out of a rendered page.
type clientCfg struct {
	Projections map[string]string `json:"projections"`
	Docs        map[string]struct {
		PageTextBase string `json:"page_text_base"`
	} `json:"docs"`
}

func clientConfigOf(t *testing.T, page string) clientCfg {
	t.Helper()
	var cfg clientCfg
	if err := json.Unmarshal(configBlob(t, page), &cfg); err != nil {
		t.Fatalf("decode window.FISC_CONFIG: %v", err)
	}
	return cfg
}

// TestAShortSeriesDoesNotShiftItsNeighboursIntoTheWrongColumn is the defect the
// positional layout had, pinned.
//
// A series short a column is a state the project supports: internal/check
// declares one through incompleteSeries, and `fisc export` runs no checks, so a
// document with a gap can be packaged. Laid out positionally -- one cell per
// point, against headers built from metadata.columns -- every figure after the
// gap slides one column left, so FY2026's money prints under FY2025 with a
// citation link to FY2026's page. That is a plausible wrong value, published,
// with provenance that disagrees with it, which is the whole class of error this
// project exists to prevent.
func TestAShortSeriesDoesNotShiftItsNeighboursIntoTheWrongColumn(t *testing.T) {
	// Two columns; the series publishes only the SECOND.
	raw := trendsDoc(127, 128)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	series := doc["series"].([]any)[0].(map[string]any)
	points := series["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("fixture has %d points, want 2", len(points))
	}
	// Drop the FIRST point, keeping the second. Its own fiscal year says which
	// column it belongs in, and that is what the layout must honour.
	kept := points[1].(map[string]any)
	series["points"] = []any{kept}
	short, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	dir := t.TempDir()
	if _, err := export.Write(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "revenue-trends": short,
		},
		Views: []export.View{
			{Path: export.IndexPath, Template: export.SankeyTemplate, Projection: "sankey"},
			{Path: "revenue.html", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(127, 128),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	html := readFile(t, dir, "revenue.html")
	row := between(t, html, `<tr data-fund-group=`, "</tr>")
	cells := strings.Count(row, "<td class=\"num")
	if cells != 2 {
		t.Fatalf("the row renders %d numeric cells over 2 columns:\n%s", cells, row)
	}
	// The kept figure must be in the SECOND cell, under its own year, and the
	// first must be a rendered gap rather than the kept figure slid left.
	first := between(t, row, "<td class=\"num", "</td>")
	if !strings.Contains(first, "missing") {
		t.Errorf("the first cell is not a gap, so a later figure slid into it:\n%s", first)
	}
	if !strings.Contains(first, "not printed in this column") {
		t.Errorf("the gap does not say what it is:\n%s", first)
	}
	// And the citation on the surviving figure still points at its own page.
	want := fmt.Sprintf("p%04d.txt", int(kept["page"].(float64)))
	if !strings.Contains(row, want) {
		t.Errorf("the surviving figure does not cite %s:\n%s", want, row)
	}
}

// twoViewPageText is the extraction tree the fixtures cite.
func twoViewPageText(pages ...int) fstest.MapFS {
	tree := pageTextFS()
	for _, p := range pages {
		tree[fmt.Sprintf("%s/pages/p%04d.txt", budgetDocID, p)] =
			&fstest.MapFile{Data: []byte(fmt.Sprintf("page %d\n", p))}
	}
	return tree
}

// between is the slice of s from the first occurrence of open to the next
// close. A missing marker fails the test naming it, rather than panicking on a
// -1 index and leaving the reader to work out which of the two was absent.
func between(t *testing.T, s, open, close string) string {
	t.Helper()
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatalf("the markup contains no %q", open)
	}
	rest := s[i:]
	j := strings.Index(rest, close)
	if j < 0 {
		t.Fatalf("the markup contains no %q after %q", close, open)
	}
	return rest[:j]
}

// TestTheMarkReachesTheRenderedPage is the end-to-end half of the mark: the
// geometry tests in mark_test.go assert what buildMark computes, and this
// asserts that it survives the template and lands in the file a reader gets.
//
// IT ALSO PINS THE PROPERTY THAT LETS THE MARK EXIST AT ALL. The mark carries no
// citation, and that is only defensible because every figure it draws is already
// in the same row as linked text. So this checks the two together: the row has
// its marks AND it still has its links. If a future change ever drew a figure in
// the mark that is not in the row, this is where it should stop.
func TestTheMarkReachesTheRenderedPage(t *testing.T) {
	dir := t.TempDir()
	pages := []int{127, 128}
	if _, err := export.Write(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc(pages...)},
		Views: []export.View{
			{Path: export.IndexPath, Template: export.SankeyTemplate, Projection: "sankey"},
			{Path: "revenue.html", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Docs:        budgetDocs(),
		PageText:    twoViewPageText(pages...),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "revenue.html"))
	if err != nil {
		t.Fatalf("read revenue.html: %v", err)
	}
	got := string(page)

	// One mark for the fixture's one series, and one bar per printed column.
	if n := strings.Count(got, `<td class="mark">`); n != 1 {
		t.Errorf("the page renders %d marks over one series, want 1", n)
	}
	if n := strings.Count(got, `class="mark-bar`); n != len(pages) {
		t.Errorf("the page renders %d bars over %d printed columns, want one each",
			n, len(pages))
	}
	// The bars say which column and which figure they are, so a mark is
	// identifiable without labels it has no room for.
	if !strings.Contains(got, "<title>FY 2025-26 adopted: $1</title>") {
		t.Error("no bar carries the column and figure it draws")
	}
	// AND THE FIGURES ARE STILL CITED. This is the load-bearing half: a mark
	// with no provenance pointer is only honest while the figure it draws has
	// one somewhere in the same row.
	for _, p := range pages {
		want := fmt.Sprintf("p%04d.txt", p)
		if !strings.Contains(got, want) {
			t.Errorf("the row draws a bar for p%d and cites no %s", p, want)
		}
	}
	// The scale limit is stated on the page. A per-row scale that a reader
	// takes for a shared one is a comparison the document does not support.
	if !strings.Contains(got, "not comparable with each other") {
		t.Error("the page draws per-row marks and does not say they are per-row")
	}
}
