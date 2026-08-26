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
		// THE SHADOWING MESSAGE, not "not an .html file". This case is named for
		// the fixed-layout guard and used to assert the suffix check, which is to
		// say it pinned the guard's UNREACHABILITY: every fixedPaths key is
		// caught by the suffix check first, so the arm could never fire. The
		// distinction is what the reader does next -- "not an .html file" invites
		// renaming app.js to app.html, which shadows nothing and is still wrong.
		{"a path that shadows an asset", []export.View{ok, {Path: "app.js",
			Template: export.SankeyTemplate, Projection: "sankey"}},
			"part of the fixed site layout"},
		// And a genuine non-html path still gets the suffix message, so moving
		// the arm has not swallowed the case it used to answer.
		{"a path that shadows nothing and is not html", []export.View{ok,
			{Path: "notes.txt", Template: export.SankeyTemplate, Projection: "sankey"}},
			"not an .html file"},
		// A lede on a template that renders none was set, exported and dropped in
		// silence -- the field is a trap for the next caller unless it fails.
		{"a lede the template cannot render", []export.View{ok,
			{Path: "extra.html", Template: export.SankeyTemplate, Projection: "sankey",
				Lede: "a sentence that would go nowhere"}},
			"has no {{.Lede}}"},
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
	// AND SAY SO IN THE DOCUMENT'S OWN COUNTS, which is not bookkeeping. A
	// series short a column is a state this project supports; a document whose
	// counts.points disagrees with the points it carries is a different thing
	// and Write now refuses it (fisc-4j5). Leaving the count at 2 would make
	// this test assert the refusal instead of the layout it is named for.
	meta := doc["metadata"].(map[string]any)
	meta["counts"].(map[string]any)["points"] = 1
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

// writeTrends packages one trends document and returns whatever Write said.
//
// It exists so the refusal tests below differ only in the document they hand
// over, which is the whole of what each is about.
func writeTrends(t *testing.T, raw []byte, pages ...int) error {
	t.Helper()
	_, err := export.Write(export.Options{
		Dir: t.TempDir(),
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "revenue-trends": raw,
		},
		Views: []export.View{
			{Path: export.IndexPath, Template: export.SankeyTemplate, Projection: "sankey"},
			{Path: "revenue.html", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(pages...),
	})
	return err
}

// TestADocumentThatLosesAFigureBetweenProjectionAndPageIsRefused is fisc-4j5's
// exact case, built directly rather than reasoned about.
//
// buildCells drops a point whose (fiscal_year, basis) is in no published column,
// and its comment justified that by saying "the caller compares counts.points
// against what it rendered". THE CALLER DID NOT: counts.points was decoded and
// read nowhere. So a one-column document carrying a two-point series exported
// successfully, rendered ONE cell, lost the other figure with no error, and the
// page's own lede still said how many figures were in it.
//
// That is a published page whose prose contradicts what it draws -- a plausible
// wrong value with provenance disagreeing with it, which is the class this
// project exists to refuse.
//
// NOT REACHABLE THROUGH internal/project TODAY, because Trends.Document filters
// points on the same columns the metadata is built from. This is a packager that
// did not fail closed, so it is built here by hand, which is the only way to
// reach it at all.
func TestADocumentThatLosesAFigureBetweenProjectionAndPageIsRefused(t *testing.T) {
	// Two columns and two points, then the SECOND column removed -- leaving a
	// point that belongs to no column the page will draw.
	var doc map[string]any
	if err := json.Unmarshal(trendsDoc(127, 128), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := doc["metadata"].(map[string]any)
	cols := meta["columns"].([]any)
	meta["columns"] = cols[:1]
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	err = writeTrends(t, raw, 127, 128)
	if err == nil {
		t.Fatal("Write accepted a document whose second point lands in no column, want a refusal")
	}
	// BOTH NUMBERS, because a refusal that says only "these disagree" leaves the
	// reader to go and count.
	for _, want := range []string{"counts.points 2", "carry 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Write error = %q, want it to name %q", err, want)
		}
	}
}

// TestTwoPointsInOneColumnAreRefused is the same figure-loss from the other
// side: a plain map assignment kept the last point and dropped the first as
// quietly as an undrawn column did, and the two need not even agree. The counts
// still reconcile in that case, so this needs its own refusal rather than
// riding on the one above.
func TestTwoPointsInOneColumnAreRefused(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(trendsDoc(127, 128), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	series := doc["series"].([]any)[0].(map[string]any)
	points := series["points"].([]any)
	// Re-file the second point in the FIRST point's column, keeping the counts
	// honest at two so this cannot pass through the counts arm instead.
	second := points[1].(map[string]any)
	first := points[0].(map[string]any)
	second["fiscal_year"] = first["fiscal_year"]
	second["basis"] = first["basis"]
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	err = writeTrends(t, raw, 127, 128)
	if err == nil {
		t.Fatal("Write accepted two points in one column, want a refusal")
	}
	if !strings.Contains(err.Error(), "two points publish") {
		t.Errorf("Write error = %q, want it to name the collision", err)
	}
}

// TestASeriesCountThatDisagreesWithTheSeriesIsRefused covers the other half of
// the reconciliation. counts.series feeds the page's own "231 rows" line, so a
// document that carries a different number of series prints a figure about
// itself that is wrong.
func TestASeriesCountThatDisagreesWithTheSeriesIsRefused(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(trendsDoc(127), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := doc["metadata"].(map[string]any)
	meta["counts"].(map[string]any)["series"] = 2
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	err = writeTrends(t, raw, 127)
	if err == nil {
		t.Fatal("Write accepted counts.series 2 over one series, want a refusal")
	}
	if !strings.Contains(err.Error(), "counts.series 2 and carries 1") {
		t.Errorf("Write error = %q, want it to name both counts", err)
	}
}

// TestASecondYearsCitationsSurviveTheYearItDoesNotOpenOn is fisc-yi4.
//
// The footer's sources and clientConfig.Docs were built from ONE document's
// metadata -- the year the view opens on -- while CONFIG.years lists every
// published year and app.js will switch to any of them. citations() drops a
// fact whose doc_id is not in that map with `if (!doc) continue`, no fallback
// and no diagnostic, so a year citing a document the opening year does not
// would render its chart normally and show a provenance panel with fewer rows
// than the chart has facts. Nothing anywhere would say so.
//
// The second year here cites the ACFR, which the opening year does not. Before
// the union, FISC_CONFIG's docs map held only the budget book.
func TestASecondYearsCitationsSurviveTheYearItDoesNotOpenOn(t *testing.T) {
	const acfr = "livermore-acfr-fy2025"

	var second map[string]any
	if err := json.Unmarshal(goldenSankey(t), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := second["metadata"].(map[string]any)
	meta["fiscal_year"] = 2027
	meta["fiscal_year_label"] = "FY 2026-27"
	// A source the opening year does not carry, alongside one it does -- so the
	// test distinguishes a union from a replacement.
	meta["sources"] = []map[string]any{
		{"doc_id": budgetDocID, "pages": []int{66}},
		{"doc_id": acfr, "pages": []int{177}},
	}
	raw, marshalErr := json.Marshal(second)
	if marshalErr != nil {
		t.Fatalf("encode: %v", marshalErr)
	}

	dir := t.TempDir()
	if _, err := export.Write(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "sankey-2027": raw,
		},
		Views: []export.View{{
			Path: export.IndexPath, Template: export.SankeyTemplate,
			Projection: "sankey", YearStems: []string{"sankey", "sankey-2027"},
		}},
		Docs: append(budgetDocs(), export.Doc{
			ID: acfr, Title: "FY 2024-25 Annual Comprehensive Financial Report",
			Publisher: "City of Livermore, California",
		}),
		GeneratedBy: "fisc test",
		PageText:    pageTextFS(),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	markup, err := os.ReadFile(filepath.Join(dir, export.IndexPath))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	page := string(markup)

	// THE CLIENT'S MAP IS THE ONE THAT MATTERS: a doc_id missing from it is a
	// citation app.js drops in silence.
	// between() keeps its opening marker, so strip it to leave the JSON alone.
	cfg := strings.TrimPrefix(
		between(t, page, "window.FISC_CONFIG = ", ";</script>"), "window.FISC_CONFIG = ")
	var config struct {
		Docs map[string]struct {
			PageTextBase string `json:"page_text_base"`
		} `json:"docs"`
	}
	if err := json.Unmarshal([]byte(cfg), &config); err != nil {
		t.Fatalf("decode FISC_CONFIG: %v", err)
	}
	for _, id := range []string{budgetDocID, acfr} {
		if config.Docs[id].PageTextBase == "" {
			t.Errorf("FISC_CONFIG.docs has no entry for %q; every fact citing it "+
				"would lose its citation with no error (docs: %v)", id, config.Docs)
		}
	}

	// And the footer advertises both, rather than the opening year's alone.
	if !strings.Contains(page, "Annual Comprehensive Financial Report") {
		t.Error("the footer does not cite the second year's document")
	}
	// The second year's page text must ship, or the citation resolves to a 404.
	if _, err := os.Stat(filepath.Join(dir, shippedPageText(acfr, 177))); err != nil {
		t.Errorf("the second year's cited page text did not ship: %v", err)
	}
}

// twoYearSankey writes a two-year spine view whose second year is the golden
// document with `edit` applied to its metadata, and returns the rendered
// index.html.
//
// EVERY DEFECT BELOW IS LATENT IN THE COMMITTED CORPUS -- both published years
// are adopted, carry one scope and cite the same two pages -- so a test that
// only exported the real store would assert nothing. The state has to be built.
func twoYearSankey(t *testing.T, view export.View, edit func(meta map[string]any)) (string, error) {
	t.Helper()
	var second map[string]any
	if err := json.Unmarshal(goldenSankey(t), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := second["metadata"].(map[string]any)
	meta["fiscal_year"] = 2027
	meta["fiscal_year_label"] = "FY 2026-27"
	edit(meta)
	raw, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	view.Path = export.IndexPath
	view.Template = export.SankeyTemplate
	view.Projection = "sankey"
	view.YearStems = []string{"sankey", "sankey-2027"}

	dir := t.TempDir()
	if _, err = export.Write(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t), "sankey-2027": raw},
		Views:       []export.View{view},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    pageTextFS(),
	}); err != nil {
		return "", err
	}
	markup, err := os.ReadFile(filepath.Join(dir, export.IndexPath))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	return string(markup), nil
}

// yearsIn decodes CONFIG.years out of a rendered page.
func yearsIn(t *testing.T, page string) []struct {
	Label string `json:"label"`
	Basis string `json:"basis"`
	Title string `json:"title"`
} {
	t.Helper()
	cfg := strings.TrimPrefix(
		between(t, page, "window.FISC_CONFIG = ", ";</script>"), "window.FISC_CONFIG = ")
	var config struct {
		Years []struct {
			Label string `json:"label"`
			Basis string `json:"basis"`
			Title string `json:"title"`
		} `json:"years"`
	}
	if err := json.Unmarshal([]byte(cfg), &config); err != nil {
		t.Fatalf("decode FISC_CONFIG: %v", err)
	}
	return config.Years
}

// TestEachYearCarriesItsOwnBasisAndTitle is the packager half of fisc-iyt and
// fisc-rn0: the words a year switch paints have to differ per year, or
// repainting them changes nothing.
//
// THE FOOTER'S BASIS IS ASSERTED IN THE MARKUP TOO, and that is not redundant
// with the jscheck check that paints it. tools/jscheck's DOM stub fabricates
// any id in TEMPLATE_IDS, so a check that "paintYearWords writes #page-basis"
// passes whether or not index.html.tmpl renders the span. If the span is
// dropped, maybeEl returns null, the guard swallows it, and the footer silently
// stops following the year -- the original defect, reached through the harness
// that was supposed to catch it. Only a Go assertion on the shipped markup
// closes that.
func TestEachYearCarriesItsOwnBasisAndTitle(t *testing.T) {
	page, err := twoYearSankey(t, export.View{}, func(meta map[string]any) {
		meta["basis"] = "proposed"
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !strings.Contains(page, `<span id="page-basis">`) {
		t.Error("the footer's basis is not wrapped in #page-basis, so paintYearWords " +
			"has nothing to repaint and the sentence keeps the opening year's basis")
	}

	years := yearsIn(t, page)
	if len(years) != 2 {
		t.Fatalf("CONFIG.years has %d entries, want 2", len(years))
	}
	if years[0].Basis != "adopted" || years[1].Basis != "proposed" {
		t.Errorf("bases are %q and %q, want adopted and proposed", years[0].Basis, years[1].Basis)
	}
	if years[0].Title == years[1].Title {
		t.Errorf("both years carry the title %q; the client writes this straight into "+
			"document.title, so one string for two years is a page that will not "+
			"say which year it is showing", years[0].Title)
	}
	for i, y := range years {
		if !strings.Contains(y.Title, y.Label) {
			t.Errorf("year %d's title %q does not name %q", i, y.Title, y.Label)
		}
	}
}

// TestACallersOwnTitleSurvivesEveryYear pins the half of fisc-rn0 that the
// jscheck check cannot see: it is about what the PACKAGER hands over.
//
// The caller's title is carried verbatim onto every year rather than having a
// year appended, and refusing a Title on this template was rejected -- see
// sankeyTitle for both arguments. What must not happen is the packager
// composing over the top of a caller's words, which is the trap View.Title's
// own doc comment warns about.
func TestACallersOwnTitleSurvivesEveryYear(t *testing.T) {
	// No apostrophe: html/template escapes one to &#39; in the <title>, which is
	// correct and would make this assertion about escaping rather than about the
	// caller's words surviving.
	const chosen = "Where the money goes, drawn"
	page, err := twoYearSankey(t, export.View{Title: chosen}, func(map[string]any) {})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(page, "<title>"+chosen+"</title>") {
		t.Errorf("the server-rendered <title> is not the caller's %q", chosen)
	}
	for i, y := range yearsIn(t, page) {
		if y.Title != chosen {
			t.Errorf("year %d carries the title %q; the caller asked for %q, and the "+
				"client writes year.title into document.title on every switch",
				i, y.Title, chosen)
		}
	}
}

// TestAYearStemOnAnotherScopeIsRefused is the fail-closed half of fisc-iyt.
//
// The footer states one scope and the lede states the same claim again in prose
// ("all funds, gross") that is not repainted, so a view whose years disagreed
// about scope would have to repaint one of two copies. It cannot happen today --
// Sankey.Slices fixes the scope -- and the answer to a state nothing can emit is
// to refuse it, not to build a repaint and a test that can never go red.
func TestAYearStemOnAnotherScopeIsRefused(t *testing.T) {
	_, err := twoYearSankey(t, export.View{}, func(meta map[string]any) {
		meta["scope"] = "revenue-by-fund"
	})
	if err == nil {
		t.Fatal("a year stem on another scope was exported; the page would state " +
			"one scope in its footer and another in its lede")
	}
	for _, want := range []string{"sankey-2027", "revenue-by-fund", "all-funds-gross"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// TestAYearStemBuiltByAnotherProjectionIsRefused covers the footer sentence's
// third value.
//
// "Scope X, basis Y. Projection: Z." is one sentence and all three come from
// the opening year. Basis is repainted; scope and Z fail closed. Z is
// meta.GeneratedBy, a claim about the tool that built the document -- a page
// crediting one builder for figures drawn from two is not a wording problem a
// repaint fixes, because the documents disagree about their own provenance.
//
// Added because /code-review of the basis fix found its two sentence-mates
// unguarded: fixing one value of three and leaving the others is how the
// original defect got in.
func TestAYearStemBuiltByAnotherProjectionIsRefused(t *testing.T) {
	_, err := twoYearSankey(t, export.View{}, func(meta map[string]any) {
		meta["generated_by"] = "fisc some-other-build"
	})
	if err == nil {
		t.Fatal("a year stem built by another projection was exported; the footer " +
			"credits one builder for figures drawn from two documents")
	}
	for _, want := range []string{"sankey-2027", "fisc some-other-build"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// TestTheLedesProseNamesThePagesScope couples two statements of one claim that
// nothing else couples.
//
// index.html.tmpl's lede says the budget is "all funds, gross" in English; the
// footer prints "Scope {{.Scope}}", the slug, three screens down. Neither is
// repainted and the scope guard in buildSankeyPage only makes a view's YEARS
// agree with each other -- so a view built entirely on some other scope ships a
// lede contradicting its own footer, with every guard green. Found by
// /code-review of the fisc-iyt landing.
//
// A PIN RATHER THAN A GUARD, deliberately. Refusing a scope in internal/export
// would mean this package holding an opinion about which scopes exist, which is
// internal/project's to hold; and rendering {{.Scope}} into the lede would put
// "all-funds-gross" in a sentence a reader reads aloud. What is wrong today is
// that the coupling is invisible, so the fix is to make it fail when it breaks.
func TestTheLedesProseNamesThePagesScope(t *testing.T) {
	page, err := twoYearSankey(t, export.View{}, func(map[string]any) {})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	const (
		slug  = "all-funds-gross"
		prose = "all funds, gross"
	)
	if !strings.Contains(page, "Scope "+slug) {
		t.Fatalf("the footer no longer prints scope %q; if the published scope changed, "+
			"the lede's prose at index.html.tmpl:41 has to change with it", slug)
	}
	if !strings.Contains(page, prose) {
		t.Errorf("the lede no longer says %q while the footer says scope %q; "+
			"one page, one claim, two spellings that have drifted", prose, slug)
	}
}
