package export_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/site"
)

// trendsDoc is a minimal revenue-trends document citing pages the SPINE does
// not, which is the whole point: it is the shape fisc-fjy was filed about.
//
// It is hand-written rather than built by internal/project: the packager
// consumes projections as bytes and knows nothing about how they were built,
// and a test that reached for the producer would quietly retire that seam.
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
			// applies_to IS NON-EMPTY, and that is what makes
			// TestTheCaveatsPagePromisesAChartFlagOnlyWhereThereIsAChart able
			// to fail. caveats.html prints its chart-flag promise inside
			// {{if .AppliesTo}}, so a document whose caveats all carry an empty
			// one never reaches that branch -- and the check asserting the
			// promise is absent from the trends section passed whether the
			// predicate behind it worked or not. No real revenue-trends caveat
			// names a node today; this fixture is where that case lives.
			"caveats": []map[string]any{{
				"id":         "a-caveat-this-document-carries",
				"summary":    "a caveat this document carries",
				"text":       "a caveat this document carries, at length",
				"applies_to": []string{"fund/100"},
			}},
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
	_, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey":         goldenSankey(t),
			"revenue-trends": trendsDoc(trendsPages...),
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "trends.html", Nav: "Revenue tables",
				Template: export.TrendsTemplate, Projection: "revenue-trends",
				Title: "Revenue tables", Lede: "A lede."},
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
// A view citing pages another does not must still get them shipped. Composing
// the page-text set from the primary projection's metadata alone renders
// citation links styled exactly like the working ones beside them, resolving in
// the template and 404ing in the browser -- invisible to every other check,
// because fact-offset-points-at-token reads the committed corpus and nothing
// else walks the built tree.
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
	trends := readFile(t, dir, "trends.html")

	if !strings.Contains(spine, "p66 (PDF)") || strings.Contains(spine, "p127 (PDF)") {
		t.Error("the spine page's Sources list is not its own")
	}
	if !strings.Contains(trends, "p127 (PDF)") || strings.Contains(trends, "p66 (PDF)") {
		t.Error("the trends page's Sources list is not its own")
	}
}

// TestEveryCitationTheClientComposesResolves is the check fisc-fjy asks for, and
// it is NOT the check that already existed.
//
// TestEveryAssetThePageAsksForWasWritten walks src= and href= out of the rendered
// markup, and it is green -- but the citation links fisc-fjy is about are not IN
// the markup. site/app.js reads them at runtime, per cited fact, from
// CONFIG.docs[d].pages[n].text, and only the footer's
// handful ever reach the HTML. So a walk of the page cannot see the class of
// link that broke.
//
// This crosses the client's own config against every document it can fetch, which
// is the set of paths the browser will actually construct.
//
// It walks the years' columns, so it reaches a source cited only by a schedule
// behind a click.
func TestEveryCitationTheClientComposesResolves(t *testing.T) {
	dir := twoViews(t, 127, 128)
	cfg := clientConfigOf(t, readFile(t, dir, "index.html"))

	checked := 0
	for _, y := range cfg.Years {
		var column struct {
			Schedules map[string]struct {
				Sources []struct {
					DocID string `json:"doc_id"`
					Pages []int  `json:"pages"`
				} `json:"sources"`
			} `json:"schedules"`
		}
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(y.Path)))
		if err != nil {
			t.Fatalf("the client fetches year %s at %s, and it is not there: %v", y.Stem, y.Path, err)
		}
		if err := json.Unmarshal(raw, &column); err != nil {
			t.Fatalf("decode %s: %v", y.Path, err)
		}
		if len(column.Schedules) == 0 {
			t.Fatalf("%s carries no schedule, so this loop covers nothing", y.Path)
		}
		for key, sched := range column.Schedules {
			for _, src := range sched.Sources {
				d, ok := cfg.Docs[src.DocID]
				if !ok {
					// Not a failure: the client composes no link for a document it has no entry for.
					t.Logf("%s's %s schedule cites %s, which the client config does not describe",
						y.Path, key, src.DocID)
					continue
				}
				for _, p := range src.Pages {
					checked++
					href := d.Pages[strconv.Itoa(p)].Text
					if href == "" {
						t.Errorf("the config carries no text link for %s page %d, which a schedule cites", src.DocID, p)
						continue
					}
					if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(href))); err != nil {
						t.Errorf("the config links %q for %s page %d, and it is not in "+
							"the output: %v", href, src.DocID, p, err)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no citation path was checked; this test is asserting nothing")
	}
}

// chartView is a well-formed chart view with one thing broken.
//
// A CONSTRUCTOR RATHER THAN TEN LITERALS, because the point of each case is the
// ONE field it breaks: written out in full, a case that stopped breaking
// anything -- a field renamed, a default filled in -- would still name a
// message and still pass, and nothing would say which arm had gone quiet.
//
// It is a second SankeyTemplate view, at a path the site does not open on.
func chartView(breaks func(*export.View)) export.View {
	v := export.View{
		Path: "extra.html", Template: export.SankeyTemplate, Projection: "sankey",
		Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{Tiers: []int{0, 2}}},
	}
	breaks(&v)
	return v
}

// drawsView is a well-formed view drawing another schedule of its column, as
// the index draws the expansion: the spine's figures and years, fund-flows'
// chart at tiers it draws a column of.
// TestAViewDrawsAnotherScheduleOfItsColumn is its control.
func drawsView(breaks func(*export.View)) export.View {
	v := export.View{
		Path: "other.html", Nav: "Other", Template: export.SankeyTemplate, Projection: "sankey",
		Draws: "fund-flows",
		Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
			Tiers: []int{0, 3, 4, 5},
		}},
		Description: "Every flow at once.",
	}
	breaks(&v)
	return v
}

// TestAViewDrawsAnotherScheduleOfItsColumn is the control for the drawn-
// schedule refusals: the page ships fund-flows as its primary schedule with
// the overview's hints under the Sankey's key, its counts line is fund-flows'
// and its description replaces the spine's.
func TestAViewDrawsAnotherScheduleOfItsColumn(t *testing.T) {
	fundFlows, err := os.ReadFile("../../testdata/fund-flows.golden.json")
	if err != nil {
		t.Fatalf("read fund-flows golden: %v", err)
	}
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t), "fund-flows": builtLike(t, goldenSankey(t), fundFlows)},
		Views: []export.View{{Path: export.IndexPath, Nav: "Budget flows",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			drawsView(func(*export.View) {})},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write refused a view drawing another schedule: %v", err)
	}
	page := readFile(t, dir, "other.html")
	for _, want := range []string{
		`"primary":"fund-flows"`,
		`"overview":{"form":"sankey","sankey":{"tiers":[0,3,4,5]}}`,
		`<desc id="chart-desc">` + "\n        Every flow at once. ",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page carries no %s", want)
		}
	}
	var doc struct {
		Metadata struct {
			Counts struct{ Links int } `json:"counts"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(fundFlows, &doc); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(`"links":%d`, doc.Metadata.Counts.Links); !strings.Contains(page, want) {
		t.Errorf("the page's year counts no %s, fund-flows' own link count", want)
	}
	if strings.Contains(page, "Revenue categories flow into fund groups") {
		t.Error("the page describes the spine's chart")
	}
}

func TestTheCaveatsPageRefusesWhatWouldRender(t *testing.T) {
	// THE CAVEATS VIEW IS THE INDEX HERE, which is not how the site is
	// configured and is the smallest thing that reaches the builder. Options
	// requires exactly one view at IndexPath, and pairing this with a spine
	// view means the spine's own "no sankey projection to build the page from"
	// fires first and the caveats page is never built -- which is how the first
	// draft of this test passed five refusals without reaching any of them.
	caveatsView := export.View{
		Path: export.IndexPath, Nav: "Caveats", Template: export.CaveatsTemplate,
		Title: "What these figures do not say", Lede: "A lede.",
	}
	// A minimal document of a shape this package has never been taught -- no
	// nodes, no links, no series. The aggregator is shape-blind and building
	// the fixture that way is what asserts it.
	//
	// STEMMED "sankey" BECAUSE Options.validate REQUIRES THAT ONE to be among
	// the projections, whatever the views are. It is not the spine document and
	// nothing here treats it as one; it is the stem the packager insists exists.
	doc := func(caveats string) map[string][]byte {
		return map[string][]byte{export.PrimaryProjection: []byte(
			`{"schema_version":1,"projection":"sankey","metadata":{` +
				`"fiscal_year_label":"FY 2025-26","basis":"adopted","sources":[],` +
				`"caveats":` + caveats + `}}`)}
	}
	const good = `[{"id":"a","summary":"s","text":"t","applies_to":[]}]`

	for _, tc := range []struct {
		name        string
		projections map[string][]byte
		wantErr     string
	}{
		{"a well-formed document renders", doc(good), ""},
		{
			"no document carries a caveat",
			doc(`[]`),
			"no published document carries a caveat",
		},
		{
			"a caveat with no id",
			doc(`[{"id":"","summary":"s","text":"t","applies_to":[]}]`),
			"carries a caveat with no id",
		},
		{
			"a caveat with no summary",
			doc(`[{"id":"a","summary":"","text":"t","applies_to":[]}]`),
			"has no summary",
		},
		{
			"a caveat with no text",
			doc(`[{"id":"a","summary":"s","text":"","applies_to":[]}]`),
			"has no text",
		},
		{
			// TWO DOCUMENTS, ONE ANCHOR. Not two entries in one document --
			// that is ValidateCaveats' case, one package over, and it cannot
			// see across documents at all. The anchor embeds the stem, so this
			// needs two documents whose stems collide, which the map cannot
			// express; what it CAN express is the same document twice under
			// one stem, so the collision is forced by giving one document two
			// entries the decoder will hand over as-is.
			"two caveats claiming one anchor",
			doc(`[{"id":"a","summary":"s","text":"t","applies_to":[]},` +
				`{"id":"a","summary":"s2","text":"t2","applies_to":[]}]`),
			"is claimed by both",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := writeSite(export.Options{
				Dir:         t.TempDir(),
				Projections: tc.projections,
				Views:       []export.View{caveatsView},
				Docs:        budgetDocs(),
				GeneratedBy: "fisc test",
			})
			switch {
			case tc.wantErr == "":
				if err != nil {
					t.Errorf("Write = %v, want a page", err)
				}
			case err == nil:
				t.Fatalf("Write = nil, want a refusal naming %q", tc.wantErr)
			case !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("Write = %q, want it to name %q", err, tc.wantErr)
			}
		})
	}
}

// TestTheCaveatsPagePromisesAChartFlagOnlyWhereThereIsAChart pins caveatDocument
// .Drawn, which had no test at all.
//
// THE PAGE SAYS "the charts flag these marks where they draw them" under each
// document, and three of the seven the site publishes have no page -- the
// fund-flows columns unviewedDocuments declares. Flipping Drawn to !drawn[stem]
// publishes that promise on exactly those three and left `go test ./...`
// entirely green.
//
// "A VIEW NAMES THIS STEM" IS ALSO THE WRONG TEST, and this pins the right one.
// trends.html names revenue-trends as its projection and ships no app.js: its
// figures are a server-rendered table, so a caveat on that document can be
// listed and can never be chipped on a mark.
func TestTheCaveatsPagePromisesAChartFlagOnlyWhereThereIsAChart(t *testing.T) {
	dir := t.TempDir()
	_, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			export.PrimaryProjection: goldenSankey(t),
			"revenue-trends":         trendsDoc(127),
			// A second schedule of the column, which no view renders.
			"unviewed": bytes.Replace(goldenSankey(t), []byte(`"projection": "sankey"`), []byte(`"projection": "fund-flows"`), 1),
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: export.PrimaryProjection},
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate,
				Projection: "revenue-trends", Title: "Trends", Lede: "A lede."},
			{Path: "caveats.html", Nav: "Caveats", Template: export.CaveatsTemplate,
				Title: "Caveats", Lede: "A lede."},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(127),
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	page := readFile(t, dir, "caveats.html")

	const promise = "the charts flag these marks"
	section := func(stem string) string {
		t.Helper()
		i := strings.Index(page, "<h2>"+stem)
		if i < 0 {
			t.Fatalf("caveats.html carries no section for %q", stem)
		}
		rest := page[i+1:]
		if j := strings.Index(rest, "<section"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	if !strings.Contains(section(export.PrimaryProjection), promise) {
		t.Errorf("the spine's section does not promise a chart flag, and index.html draws it")
	}
	if strings.Contains(section("revenue-trends"), promise) {
		t.Error("the revenue-trends section promises a chart flag; trends.html renders that " +
			"document as a table and ships no app.js, so nothing on it can be flagged")
	}
	if strings.Contains(section("unviewed"), promise) {
		t.Error("a document no view renders promises a chart flag; there is no chart")
	}
}

// TestASingleViewSiteLinksNoCaveatsPage: with no caveats page the index
// links none, rather than a page that 404s.
func TestASingleViewSiteLinksNoCaveatsPage(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readFile(t, dir, "index.html")

	if _, err := os.Stat(filepath.Join(dir, "caveats.html")); err == nil {
		t.Fatal("a single-view export wrote caveats.html; this test is about the site " +
			"that has none, and no longer describes one")
	}
	if strings.Contains(page, `href="caveats.html`) {
		t.Error("index.html links to caveats.html, which this site does not carry")
	}
}

// TestEveryViewsMarkupResolves extends the existing asset walk to every page
// rather than only the one the site opens on.
func TestEveryViewsMarkupResolves(t *testing.T) {
	dir := twoViews(t, 127, 128)
	ref := regexp.MustCompile(`(?:src|href)="([^"]+)"`)

	for _, page := range []string{"index.html", "trends.html"} {
		found := 0
		for _, m := range ref.FindAllStringSubmatch(readFile(t, dir, page), -1) {
			target := m[1]
			// Fragment stripped rather than skipped, for the reason spelled
			// out in export_test.go's walk: "caveats.html#x" names a file this
			// check exists to stat, and only a bare "#x" is same-page.
			if before, _, ok := strings.Cut(target, "#"); ok {
				target = before
			}
			if target == "" || strings.Contains(target, "://") {
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
		{"index.html", "Budget flows", "trends.html"},
		{"trends.html", "Revenue tables", export.IndexPath},
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
	if _, err := os.Stat(filepath.Join(dir, "trends.html")); err == nil {
		t.Error("a one-view site wrote a second page")
	}
}

// TestWriteRefusesAnUnrenderableViewSet covers every way a caller can hand this
// package a set of pages it must not write. Each one would fail in the browser
// and nowhere else.
func TestWriteRefusesAnUnrenderableViewSet(t *testing.T) {
	fundFlows, err := os.ReadFile("../../testdata/fund-flows.golden.json")
	if err != nil {
		t.Fatalf("read fund-flows golden: %v", err)
	}
	ok := export.View{Path: export.IndexPath, Nav: "Budget flows",
		Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}
	cases := []struct {
		name  string
		views []export.View
		want  string
	}{
		{"no index", []export.View{{Path: "trends.html",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}}, "opens on exactly one"},
		{"two indexes", []export.View{ok, ok}, "two views claim the output path"},
		{"a path that is not html", []export.View{ok, {Path: "revenue.txt",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}}, "not an .html file"},
		{"a path in a subdirectory", []export.View{ok, {Path: "views/trends.html",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}}, "not at the site root"},
		// THE SHADOWING MESSAGE, not "not an .html file". This case is named for
		// the fixed-layout guard and used to assert the suffix check, which is to
		// say it pinned the guard's UNREACHABILITY: every fixedPaths key is
		// caught by the suffix check first, so the arm could never fire. The
		// distinction is what the reader does next -- "not an .html file" invites
		// renaming app.js to app.html, which shadows nothing and is still wrong.
		{"a path that shadows an asset", []export.View{ok, {Path: "app.js",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}},
			"part of the fixed site layout"},
		// And a genuine non-html path still gets the suffix message, so moving
		// the arm has not swallowed the case it used to answer.
		{"a path that shadows nothing and is not html", []export.View{ok,
			{Path: "notes.txt", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}},
			"not an .html file"},
		// A lede on a template that renders none was set, exported and dropped in
		// silence -- the field is a trap for the next caller unless it fails.
		{"a lede the template cannot render", []export.View{ok,
			{Path: "extra.html", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey",
				Lede: "a sentence that would go nowhere"}},
			"has no {{.Lede}}"},
		{"an overview in a form no renderer draws", []export.View{ok,
			chartView(func(v *export.View) { v.Overview.Form = "stub"; v.Overview.Sankey = nil })},
			"declares chart form \"stub\", which is not one of"},
		// Against the documents: a tier no node of the document stands at is a
		// column drawn empty.
		{"a chart drawing a tier its document has no node at", []export.View{ok,
			chartView(func(v *export.View) { v.Overview.Sankey.Tiers = []int{0, 7} })},
			"chart draws tier 7, and [sankey] carries nodes at tiers [0 2 5]"},
		{"a projection that was not built", []export.View{ok, {Path: "trends.html",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "nope"}}, "which was not built"},
		{"no template", []export.View{ok, {Path: "trends.html", Projection: "sankey"}},
			"names no template"},
		{"a year stem that was not built", []export.View{{Path: export.IndexPath,
			Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey",
			YearStems: []string{"sankey", "sankey-2099"}}}, "names no projection that was built"},
		// The Lede trap one field over: year stems dropped in silence lose whole
		// documents. Only the chart template renders a year control.
		{"year stems a template cannot render", []export.View{ok,
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate,
				Projection: "sankey", YearStems: []string{"sankey"}}},
			"has no year control"},
		// buildSite falls back Nav -> Title and has nothing after that, so this
		// set shipped <a href="trends.html"></a> on every page of the site.
		{"a view with neither a nav label nor a title", []export.View{ok,
			{Path: "trends.html", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}},
			"empty link"},
		// The third field of the same family. Only the chart template publishes
		// an overview; on any other the form would be dropped in silence.
		{"a chart form a template does not publish", []export.View{ok,
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate,
				Projection: "sankey", Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{Tiers: []int{0, 2, 4}}}}},
			"draws none"},
		{"sankey hints on a chart declaring no form", []export.View{ok,
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate,
				Projection: "sankey", Overview: export.Chart{Sankey: &export.SankeyHints{Tiers: []int{0, 2, 4}}}}},
			"carries sankey hints and declares a \"\" chart"},
		// The sections family, both directions: headings dropped in silence
		// lose the only thing telling p167's two same-labelled blocks apart,
		// and a history table with none puts every row under no heading.
		{"sections a template cannot group", []export.View{ok,
			{Path: "extra.html", Nav: "Extra", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
				Projection: "sankey",
				Sections:   []export.Section{{Heading: "Revenues", Kind: "revenue"}}}},
			"groups nothing"},
		{"a history view with no sections", []export.View{ok,
			{Path: "extra.html", Nav: "Extra", Template: export.HistoryTemplate,
				Projection: "sankey"}},
			"under no printed heading"},
		// The growth and the split caps are the expansion's: chartView's
		// overview breaks one thing each.
		{"a canvas growing by no px", []export.View{ok,
			chartView(func(v *export.View) {
				v.Overview.Sankey.Grow = &export.CanvasGrowth{NodePx: 0, Floor: 820, Ceiling: 4800}
			})},
			"grows its canvas 0px a node"},
		{"a canvas ceiling below its floor", []export.View{ok,
			chartView(func(v *export.View) {
				v.Overview.Sankey.Grow = &export.CanvasGrowth{NodePx: 40, Floor: 820, Ceiling: 819}
			})},
			"819px ceiling below its 820px floor"},
		{"a split cap of no member", []export.View{ok,
			chartView(func(v *export.View) {
				v.Overview.Sankey.SplitCaps = []export.SplitCap{{Split: "funds", Cap: 0, Tail: "funds"}}
			})},
			`caps split "funds" at 0`},
		{"a split cap naming no tail", []export.View{ok,
			chartView(func(v *export.View) {
				v.Overview.Sankey.SplitCaps = []export.SplitCap{{Split: "funds", Cap: 8}}
			})},
			`caps split "funds" with no tail`},
		{"a split cap on a split the stitch does not declare", []export.View{ok,
			chartView(func(v *export.View) {
				v.Overview.Sankey.SplitCaps = []export.SplitCap{{Split: "fund", Cap: 8, Tail: "funds"}}
			})},
			`caps split "fund", and the expansion schedule declares splits`},
		{"a split capped twice", []export.View{ok,
			chartView(func(v *export.View) {
				v.Overview.Sankey.SplitCaps = []export.SplitCap{{Split: "funds", Cap: 8, Tail: "funds"}, {Split: "funds", Cap: 9, Tail: "funds"}}
			})},
			`caps split "funds" twice`},
		{"a schedule no year's column carries", []export.View{ok,
			drawsView(func(v *export.View) { v.Draws = "revenue-trends" })},
			"is of a column that carries none"},
		{"a schedule drawn with no chart", []export.View{ok,
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate,
				Projection: "sankey", Draws: "fund-flows"}},
			"declares no chart to draw it in"},
		{"a description on a template that draws no chart", []export.View{ok,
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate,
				Projection: "sankey", Description: "Words."}},
			"describes its chart"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := writeSite(export.Options{
				Dir: t.TempDir(),
				Projections: map[string][]byte{
					"sankey": goldenSankey(t), "fund-flows": builtLike(t, goldenSankey(t), fundFlows),
				},
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
// was the only page; an asset at trends.html would have replaced a view in
// silence, and the site would have exported cleanly.
func TestAnAssetCannotShadowAView(t *testing.T) {
	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc(127)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Files:       map[string][]byte{"trends.html": []byte("not the view")},
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
	html := readFile(t, dir, "trends.html")

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
	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc()},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate, Projection: "revenue-trends"},
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

// TestTheSpineTemplatePublishesTheColumnOrderItDeclares: the chart template
// publishes the overview's hints, and only a rendered page can say it does. A
// spine whose order was dropped places no root of the expansion, and nothing
// reports it.
//
// The mutation: drop Overview from buildSankeyPage's clientConfig.
func TestTheSpineTemplatePublishesTheColumnOrderItDeclares(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{{Path: export.IndexPath, Nav: "Budget flows",
			Template: export.SankeyTemplate, Projection: "sankey",
			Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{Tiers: []int{0, 2, 5}}}}},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	cfg := clientConfigOf(t, readFile(t, dir, "index.html"))
	if diff := cmp.Diff([]int{0, 2, 5}, cfg.Overview.Sankey.Tiers); diff != "" {
		t.Errorf("index.html's FISC_CONFIG.overview.sankey.tiers (-want +got):\n%s\n"+
			"site/app.js aligns every column on this list, and reads an absent key as "+
			"\"let d3 decide\"", diff)
	}
}

// clientConfigOf decodes window.FISC_CONFIG out of a rendered page.
type clientCfg struct {
	Years []struct {
		Stem string `json:"stem"`
		Path string `json:"path"`
	} `json:"years"`
	Docs     map[string]testDoc `json:"docs"`
	Overview struct {
		Form   string `json:"form"`
		Sankey struct {
			Tiers []int `json:"tiers"`
		} `json:"sankey"`
	} `json:"overview"`
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
	// AND counts.facts WITH IT, because that is what the producer would have
	// emitted. trends.go appends exactly one Point per selected fact and counts
	// facts as len(selected), so a series short a column is short a fact and a
	// point alike -- the two diverge only in a document somebody edited. Write
	// reconciles the rendered cells against BOTH now, since facts is the number
	// the lede prints (fisc-5tu).
	meta["counts"].(map[string]any)["facts"] = 1
	short, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "revenue-trends": short,
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(127, 128),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	html := readFile(t, dir, "trends.html")
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
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc(pages...)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate, Projection: "revenue-trends"},
		},
		Docs:        budgetDocs(),
		PageText:    twoViewPageText(pages...),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "trends.html"))
	if err != nil {
		t.Fatalf("read trends.html: %v", err)
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
	_, err := writeSite(export.Options{
		Dir: t.TempDir(),
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "revenue-trends": raw,
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "trends.html", Nav: "Revenue tables", Template: export.TrendsTemplate, Projection: "revenue-trends"},
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

// TestALedeClaimingMoreFiguresThanTheTableShowsIsRefused is fisc-5tu: the
// figure-loss above, reached through the OTHER count.
//
// The guard beside this one reconciles the rendered cells against
// counts.points, and the sentence it exists to protect prints a DIFFERENT
// number -- trends.html.tmpl renders {{.Facts}}, "N figures in all", from
// counts.facts. project.TrendCounts' doc comment states the two are computed
// independently, facts off the selection and points off the series actually
// built, so they are free to disagree.
//
// So this document -- facts 2, points 1, one point carried, one cell rendered
// -- exported cleanly and published a lede claiming two figures over a table
// showing one. Every guard was green; the page contradicted itself in prose.
//
// UNREACHABLE THROUGH internal/project, like its neighbour and for a sharper
// reason: trends.go appends exactly one Point per selected fact and counts
// facts as len(selected), so the producer cannot emit facts != points at all.
// That is also why reconciling against facts costs nothing legitimate -- a
// series short a column is short a fact and a point alike.
func TestALedeClaimingMoreFiguresThanTheTableShowsIsRefused(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(trendsDoc(127, 128), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	series := doc["series"].([]any)[0].(map[string]any)
	series["points"] = []any{series["points"].([]any)[1]}
	// counts.points FOLLOWS the document, so the arm beside this one stays
	// green and only the lede's number is left disagreeing. Without this line
	// the test would pass for the wrong reason.
	doc["metadata"].(map[string]any)["counts"].(map[string]any)["points"] = 1
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	err = writeTrends(t, raw, 127, 128)
	if err == nil {
		t.Fatal("Write accepted a lede claiming more figures than the table shows, want a refusal")
	}
	// BOTH NUMBERS, so the reader does not have to go and count.
	for _, want := range []string{"counts.facts 2", "carry 1 cells"} {
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
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "sankey-2027": raw,
		},
		Views: []export.View{{
			Path: export.IndexPath, Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
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
		Docs map[string]testDoc `json:"docs"`
	}
	if err := json.Unmarshal([]byte(cfg), &config); err != nil {
		t.Fatalf("decode FISC_CONFIG: %v", err)
	}
	for _, id := range []string{budgetDocID, acfr} {
		if len(config.Docs[id].Pages) == 0 {
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
	if view.Overview.Form == "" {
		view.Overview = export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}
	}
	view.Projection = "sankey"
	view.YearStems = []string{"sankey", "sankey-2027"}

	dir := t.TempDir()
	if _, err = writeSite(export.Options{
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

// TestBothChartTemplatesAcceptYearStems pins the OTHER half of
// templateRendersAYearControl, and the reason is the failure the helper it
// mirrors was written to document.
//
// A refusal test alone pins only that the set is non-empty. templateRendersLede
// exists because its predecessor was spelled `v.Template != TrendsTemplate`
// while exactly one template rendered a lede, and that spelling went stale in
// silence the next time a template landed -- refusing a lede on a page that
// would have rendered one perfectly well (fisc-5miz.5). A year-control guard
// spelled `v.Template == SankeyTemplate` would have been born with that defect
// already in it: the bead naming this trap was filed BEFORE the drill-down grew
// a year control, so the obvious reading of it is wrong.
//
// So both arms are asserted, and by rendering rather than by calling the
// unexported helper: what matters is that a caller can hand either chart
// template a year list and get a page.
func TestBothChartTemplatesAcceptYearStems(t *testing.T) {
	fundFlows, err := os.ReadFile("../../testdata/fund-flows.golden.json")
	if err != nil {
		t.Fatalf("read fund-flows golden: %v", err)
	}
	second := reyeared(t, goldenSankey(t), 2027, "FY 2026-27")

	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "sankey-2027": second, "fund-flows": fundFlows,
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
				Projection: "sankey", YearStems: []string{"sankey", "sankey-2027"}},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write refused year stems on a template that renders them: %v", err)
	}

	// AND THE YEARS REACHED THE PAGE, not merely past the guard. A validate arm
	// that accepts a field the builder then ignores is the defect this whole
	// commit is about, one layer down.
	if got := len(yearsIn(t, readFile(t, dir, export.IndexPath))); got == 0 {
		t.Errorf("%s renders %d years, want its stems", export.IndexPath, got)
	}
}

// headlined stamps a headline onto a drill-down document, so a view test can
// render its tier structure as a page. fund-flows publishes no headline on
// purpose and decodeSankey refuses one without; the figure is a literal
// because nothing asserts it.
func headlined(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta, ok := doc["metadata"].(map[string]any)
	if !ok {
		t.Fatal("the document carries no metadata block to stamp a headline into")
	}
	meta["headline"] = map[string]any{
		"all_funds_gross_revenue_cents": 1, "all_funds_gross_expenditure_cents": 1,
		"external_revenue_cents": 1, "external_expenditure_cents": 1,
		"internal_transfer_in_cents": 1, "internal_transfer_out_cents": 1,
		"naive_expenditure_cents": 1, "transfer_residual_cents": 1,
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return out
}

// reyeared restamps a projection's fiscal year, so one golden document can serve
// as two published years.
func reyeared(t *testing.T, raw []byte, year int, label string) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := doc["metadata"].(map[string]any)
	meta["fiscal_year"] = year
	meta["fiscal_year_label"] = label
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return out
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
// with the client test that paints it. If the span is dropped, maybeEl returns
// null, the guard swallows it, and the footer silently stops following the
// year. Only a Go assertion on the shipped markup closes that.
func TestEachYearCarriesItsOwnBasisAndTitle(t *testing.T) {
	page, err := twoYearSankey(t, export.View{}, func(meta map[string]any) {
		meta["basis"] = "revised"
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
	if years[0].Basis != "adopted" || years[1].Basis != "revised" {
		t.Errorf("bases are %q and %q, want adopted and revised", years[0].Basis, years[1].Basis)
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

// TestACallersOwnTitleSurvivesEveryYear pins the half of fisc-rn0 that a
// client test cannot see: it is about what the PACKAGER hands over.
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
		meta["scopes"] = []string{"revenue-by-fund"}
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
// The basis fix left its two sentence-mates unguarded, and fixing one value of
// three while leaving the others is how the original defect got in.
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
// lede contradicting its own footer, with every guard green.
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

// TestATemplateWithNoArmIsRefusedRatherThanRenderedAsASpine is the fail-closed
// half of buildSite's dispatch.
//
// THE ARM THAT USED TO BE THERE WAS `default: buildSankeyPage`, and its failure
// mode is silent by construction. A template name that is not the trends one --
// a typo, or a third template added to the asset tree without a matching arm --
// was handed pageData and rendered as a spine. A template that reads none of the
// fields it is given renders a page of BLANKS, not an error: nothing in the
// pipeline compares a template against the shape of the data it received, and
// renderPage cannot, because the whole point of html/template is that a missing
// field is an empty string.
//
// The asset tree here CARRIES the third template, which is what makes the test
// about the dispatch rather than about a missing file. Under the old default the
// page below renders successfully and ships a title and nothing else.
func TestATemplateWithNoArmIsRefusedRatherThanRenderedAsASpine(t *testing.T) {
	// A NAME NO TEMPLATE IN THE TREE HAS, and it has to stay that way. This
	// read "drilldown.html.tmpl" until that became a real template with a real
	// arm, at which point the test asserted the opposite of reality and said so
	// by going red. Whatever this is renamed to next, check first that
	// buildSite has no case for it -- a test about an unhandled template is
	// worthless the moment its template is handled.
	const orphan = "unclaimed.html.tmpl"
	assets := fstest.MapFS{
		"index.html.tmpl":     {Data: []byte(`<!doctype html><title>{{.Title}}</title>`)},
		"trends.html.tmpl":    {Data: []byte(`<!doctype html><title>{{.Title}}</title>`)},
		export.SourcesPartial: {Data: []byte(`{{define "sources"}}{{end}}`)},
		// Reads nothing. That is the point: it is what a page of blanks is.
		orphan:             {Data: []byte(`<!doctype html><title>unclaimed</title>`)},
		"app.js":           {Data: []byte(`/* app */`)},
		"style.css":        {Data: []byte(`body{}`)},
		".nojekyll":        {Data: []byte{}},
		"vendor/d3.min.js": {Data: []byte(`/* d3 */`)},
	}
	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Assets:      assets,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "unclaimed.html", Nav: "Unclaimed",
				Template: orphan, Projection: "sankey"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err == nil {
		t.Fatal("Write = nil, want a refusal naming the template with no builder")
	}
	// "has no builder for" IS THE DISPATCH'S OWN WORDING, and asserting it is
	// the point rather than pedantry. This test used to check only that the
	// error named the view and the template, which any refusal mentioning both
	// satisfies -- and one did: an arm in View.validate began catching an
	// unknown template first, so restoring the historical
	// `default: buildSankeyPage` bug left this test GREEN. Pinning the message
	// that only the dispatch produces is what makes it test its own name again.
	for _, want := range []string{"unclaimed.html", orphan, "has no builder for"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got error %q, want it to name %q", err, want)
		}
	}
}

// TestAProjectionWithNoViewStillShipsItsCitedPages is the guarantee every other
// document already has, extended to the one shape that did not have it.
//
// buildSite collected citations by walking VIEWS, so a projection written to
// data/<stem>.json but rendered on no page contributed none -- and the pages its
// metadata.sources names were absent from dist/extracted/. That document is not
// hypothetical: it is what a projection looks like between the commit that
// builds it and the commit that gives it a page, which is exactly the state the
// drill-down ships in (fisc-f75 owns the view).
//
// The consequence is not a blank page. It is a published document whose
// provenance links resolve to nothing -- 404s from the one part of this site
// that exists to be checkable -- and no check sees it, because
// fact-offset-points-at-token reads the committed corpus and nothing walks the
// built tree.
func TestAProjectionWithNoViewStillShipsItsCitedPages(t *testing.T) {
	dir := t.TempDir()
	_, err := writeSite(export.Options{
		Dir: dir,
		// Two documents, ONE view. The trends document is published and
		// unviewed, and it is the only one citing p127.
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": trendsDoc(127)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(127),
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := shippedPageText(budgetDocID, 127)
	if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
		t.Errorf("the unviewed document cites p127 and %s was not written: %v", want, err)
	}
	// And the viewed document's own pages are still there, in case the new
	// pass replaced the old one rather than extending it.
	for _, p := range []int{66, 67} {
		s := shippedPageText(budgetDocID, p)
		if _, err := os.Stat(filepath.Join(dir, s)); err != nil {
			t.Errorf("the spine's own page %d stopped shipping: %v", p, err)
		}
	}
}

// provenanceSite writes a site whose third view is the provenance index, over a
// hand-built page index of two locators.
func provenanceSite(t *testing.T, edit func(*export.Options)) (string, error) {
	t.Helper()
	dir := t.TempDir()
	shard66 := []byte(`{"id":"a","doc_id":"` + budgetDocID + `","page":66}` + "\n")
	shard67 := []byte(`{"id":"c","doc_id":"` + budgetDocID + `","page":67}` + "\n")
	shard127 := []byte(`{"id":"b","doc_id":"` + budgetDocID + `","page":127}` + "\n")
	base := export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "provenance.html", Nav: "Sources and data",
				Template: export.ProvenanceTemplate, Title: "Every figure"},
		},
		Files: map[string][]byte{
			"facts/d/pages/p0066.jsonl": shard66,
			"facts/d/pages/p0067.jsonl": shard67,
			"facts/d/pages/p0127.jsonl": shard127,
			"facts/facts.csv":           []byte("id\na\n"),
		},
		PageIndex: []export.PageIndexEntry{
			{Citation: export.Citation{DocID: budgetDocID, Page: 66}, Records: 1,
				Data: "facts/d/pages/p0066.jsonl", Bytes: len(shard66), Note: "the spine"},
			{Citation: export.Citation{DocID: budgetDocID, Page: 67}, Records: 1,
				Data: "facts/d/pages/p0067.jsonl", Bytes: len(shard67), Note: "the spine, continued"},
			{Citation: export.Citation{DocID: budgetDocID, Page: 127}, Records: 1,
				Data: "facts/d/pages/p0127.jsonl", Bytes: len(shard127), Note: "property taxes"},
		},
		Downloads: []export.Download{
			{Path: "facts/facts.csv", Label: "Every figure as CSV", Bytes: 5},
		},
		RecordsBase: map[string]string{budgetDocID: "facts/d/pages/"},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(66, 67, 127),
	}
	if edit != nil {
		edit(&base)
	}
	_, err := writeSite(base)
	return base.Dir, err
}

// TestTheProvenanceViewNeedsNoProjection is the weakening this page required,
// asserted in both directions so it stays narrow.
//
// View.validate refused a view naming no projection, unconditionally. The
// provenance index renders none -- it is built from Options.PageIndex and
// decodes nothing -- so the guard had to become conditional on the template.
// The mirror arm is what keeps that from being a hole: a template that DOES
// render a document must still name one, and a projection named on a template
// that renders none is refused rather than ignored.
func TestTheProvenanceViewNeedsNoProjection(t *testing.T) {
	dir, err := provenanceSite(t, nil)
	if err != nil {
		t.Fatalf("Write refused a provenance view with no projection: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(dir, "provenance.html")); serr != nil {
		t.Fatalf("the provenance page was not written: %v", serr)
	}

	_, err = writeSite(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "provenance.html", Nav: "Sources and data",
				Template: export.ProvenanceTemplate, Projection: "sankey"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err == nil {
		t.Fatal("Write accepted a projection on a template that renders none")
	}
	if !strings.Contains(err.Error(), "would be ignored") {
		t.Errorf("got %v, want the refusal naming the ignored projection", err)
	}
}

// TestTheIndexLinksTheSourcesPageInPlaceOfItsList: beside a provenance
// index, the index page lists no cited page and links there, and Write
// refuses a page the index cites and the provenance index does not list.
// Mutation: drop listedBy's call in buildSankeyPage -- the second arm goes
// red; render the footer list unconditionally -- the first goes red.
func TestTheIndexLinksTheSourcesPageInPlaceOfItsList(t *testing.T) {
	dir, err := provenanceSite(t, nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	page := readerVisible(t, readPage(t, dir))
	if strings.Contains(page, `id="sources-view"`) || strings.Contains(page, "extracted/") {
		t.Error("index.html lists the pages it cites beside a sources page that lists them")
	}
	if !strings.Contains(page, `<a href="provenance.html">`) {
		t.Error("index.html does not link the sources page")
	}
	listed := readFile(t, dir, "provenance.html")
	for _, p := range []string{"p0066.txt", "p0067.txt"} {
		if !strings.Contains(listed, "extracted/"+budgetDocID+"/pages/"+p) {
			t.Errorf("provenance.html does not list %s, which index.html cites", p)
		}
	}

	_, err = provenanceSite(t, func(o *export.Options) {
		o.PageIndex = slices.DeleteFunc(o.PageIndex, func(e export.PageIndexEntry) bool { return e.Page == 67 })
		delete(o.Files, "facts/d/pages/p0067.jsonl")
	})
	if err == nil {
		t.Fatal("Write linked index.html's sources to a page that does not list p67")
	}
	if !strings.Contains(err.Error(), "p67") || !strings.Contains(err.Error(), "does not list it") {
		t.Errorf("got %v, want the refusal naming p67", err)
	}
}

// TestWriteRefusesAProvenanceRowWithNoShard: the page's whole claim is that its
// citations resolve, so an entry naming a file the caller did not supply would
// publish a link that 404s -- worse than publishing nothing. And nothing is
// created, because validate runs before Write makes a directory.
func TestWriteRefusesAProvenanceRowWithNoShard(t *testing.T) {
	var dir string
	// ONE FILE DELETED, everything else intact -- so the refusal under test is
	// the missing shard and not a byte count that no longer matches.
	_, err := provenanceSite(t, func(o *export.Options) {
		dir = o.Dir
		delete(o.Files, "facts/d/pages/p0127.jsonl")
	})
	if err == nil {
		t.Fatal("Write accepted a page index entry naming no file")
	}
	if !strings.Contains(err.Error(), "not among the files to be written") {
		t.Errorf("got %v, want the missing-shard refusal", err)
	}
	if entries, rerr := os.ReadDir(dir); rerr == nil && len(entries) > 0 {
		t.Errorf("the refusal wrote %d entries; validate must precede any output", len(entries))
	}
}

// TestRecordsBaseStaysSiteRelativeUnderSourceBrowseURL is the asymmetry the
// clientDoc doc comment warns about, asserted rather than described.
//
// --source-browse-url ships no page text and cites a remote URL, so
// a page's text link goes ABSOLUTE. The shards are written into the output tree
// on every export, so its records link must stay SITE-RELATIVE -- two keys that
// look alike, sit beside each other in one clientPage, and differ in kind.
// Making records follow text would publish a browse URL for 21 files
// the remote does not have, which is the mistake facts.go's index comment
// records having already been made once.
func TestRecordsBaseStaysSiteRelativeUnderSourceBrowseURL(t *testing.T) {
	dir, err := provenanceSite(t, func(o *export.Options) {
		o.SourceBrowseURL = "https://example.invalid/tree/main/"
		o.PageText = nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	b, rerr := os.ReadFile(filepath.Join(dir, export.IndexPath))
	if rerr != nil {
		t.Fatalf("read index: %v", rerr)
	}
	for id, doc := range configDocs(t, string(b)) {
		for n, p := range doc.Pages {
			if !strings.HasPrefix(p.Text, "https://example.invalid/") {
				t.Fatalf("%s p%s's text link %q did not go remote, so this test is not exercising "+
					"the asymmetry it is named for", id, n, p.Text)
			}
			if p.Records != "" && !strings.HasPrefix(p.Records, "facts/d/pages/") {
				t.Errorf("%s p%s's records link %q is not the site-relative shard; shards ship "+
					"locally whatever the page text does", id, n, p.Records)
			}
		}
	}
}

// TestWriteRefusesARecordsBaseTheEntriesContradict covers the MIS-WIRED CALLER
// and is claimed as no more than that.
//
// A base and the entries' Data both come from the same producer, so this
// cannot witness a wrong path RULE -- what it catches is a caller filling
// RecordsBase from a different source than PageIndex, which is how the config
// would come to carry a URL for a file nobody wrote. That failure is
// otherwise completely silent: every shard is present, every byte count
// matches, the provenance page's own links work, and only the chart's records
// links 404.
func TestWriteRefusesARecordsBaseTheEntriesContradict(t *testing.T) {
	_, err := provenanceSite(t, func(o *export.Options) {
		o.RecordsBase = map[string]string{budgetDocID: "facts/somewhere-else/pages/"}
	})
	if err == nil {
		t.Fatal("Write accepted a records base that no page index entry is under")
	}
	if !strings.Contains(err.Error(), "has its records in") {
		t.Errorf("got %v, want the base-contradicts-entry refusal", err)
	}
}

// TestWriteRefusesARecordsBaseWithNoTrailingSeparator is the string-prefix
// trap, and it is the likeliest way a caller gets this wrong.
//
// Now covered by the exact-directory rule rather than by a separator arm of
// its own, and kept as its own case because the CAUSE is distinct: this one is
// path.Join dropping the separator, and TestWriteRefusesAnAncestorRecordsBase
// is a caller naming the wrong directory entirely.
//
// "facts/d/pages" is a clean STRING prefix of "facts/d/pages/p0066.jsonl", so
// a prefix test alone accepts it -- and the client, which appends a filename,
// composes "facts/d/pagesp0066.jsonl". Every records anchor 404s from a base
// that validated. Options.Build is a public seam and path.Join drops the
// trailing separator, so this is one reach for the wrong helper away.
func TestWriteRefusesARecordsBaseWithNoTrailingSeparator(t *testing.T) {
	_, err := provenanceSite(t, func(o *export.Options) {
		o.RecordsBase = map[string]string{budgetDocID: "facts/d/pages"}
	})
	if err == nil {
		t.Fatal("Write accepted a records base the client cannot append a filename to")
	}
	if !strings.Contains(err.Error(), "has its records in") {
		t.Errorf("got %v, want the wrong-directory refusal", err)
	}
}

// TestWriteRefusesAnAncestorRecordsBase is the hole a prefix test leaves open
// even WITH a trailing separator.
//
// "facts/" ends in "/" and is a clean prefix of "facts/d/pages/p0066.jsonl",
// so both earlier forms of this guard accepted it -- and the client, which
// appends only a filename, composes "facts/p0066.jsonl". The base has to BE
// the entry's directory, which is the relationship the client actually relies
// on, so that is what is asserted rather than a containment approximation.
func TestWriteRefusesAnAncestorRecordsBase(t *testing.T) {
	_, err := provenanceSite(t, func(o *export.Options) {
		o.RecordsBase = map[string]string{budgetDocID: "facts/"}
	})
	if err == nil {
		t.Fatal("Write accepted a records base that is an ancestor of where the shards are")
	}
	if !strings.Contains(err.Error(), "has its records in") {
		t.Errorf("got %v, want the wrong-directory refusal", err)
	}
}

// TestWriteRefusesARecordsBaseForADocumentItPublishesNothingOf is the other
// direction: a base the client would build links from and nothing would answer.
func TestWriteRefusesARecordsBaseForADocumentItPublishesNothingOf(t *testing.T) {
	_, err := provenanceSite(t, func(o *export.Options) {
		o.RecordsBase["acfr-fy2024"] = "facts/acfr-fy2024/pages/"
	})
	if err == nil {
		t.Fatal("Write accepted a records base for a document with no page index entry")
	}
	if !strings.Contains(err.Error(), "which no page index entry does") {
		t.Errorf("got %v, want the unknown-document refusal", err)
	}
}

// TestAnAbsentRecordsBaseIsNotAnError pins the absent-is-not-zero half. A
// caller that publishes no records for a document is not a caller publishing a
// base pointing at nothing, and the client must render no records link rather
// than a broken one.
func TestAnAbsentRecordsBaseIsNotAnError(t *testing.T) {
	dir, err := provenanceSite(t, func(o *export.Options) {
		o.RecordsBase = nil
	})
	if err != nil {
		t.Fatalf("Write with no records base: %v", err)
	}
	b, rerr := os.ReadFile(filepath.Join(dir, export.IndexPath))
	if rerr != nil {
		t.Fatalf("read index: %v", rerr)
	}
	for id, doc := range configDocs(t, string(b)) {
		for n, p := range doc.Pages {
			if p.Records != "" {
				t.Errorf("%s p%s carries records link %q with no records published; the key "+
					"must be present and empty", id, n, p.Records)
			}
		}
	}
	if !strings.Contains(string(b), `"records":""`) {
		t.Error("the page config carries no empty records link; the key must be present and " +
			"empty rather than absent, as every other link is")
	}
}

// TestTheProvenancePageShipsThePageTextOfEveryLocatorItPublishes is the p76 case
// in miniature, and the reason cited is seeded from PageIndex rather than from
// a second mechanism.
//
// The shipped extraction used to be whatever the projections' own
// metadata.sources named. The fact store covers a page the charts do not, so a
// provenance link resolved to a shard beside a 404. Routing the index's pages
// through the same cited set means withPageText's existing refusal covers them:
// a published locator whose page text is not in the extraction tree is an
// error, not a dangling link.
func TestTheProvenancePageShipsThePageTextOfEveryLocatorItPublishes(t *testing.T) {
	// The page text carries 66 and 67 only; the index publishes 127 too.
	_, err := provenanceSite(t, func(o *export.Options) { o.PageText = twoViewPageText(66, 67) })
	if err == nil {
		t.Fatal("Write shipped a locator whose page text it does not carry")
	}
	if !strings.Contains(err.Error(), "p0127") {
		t.Errorf("got %v, want it to name the page that is missing", err)
	}

	// And the positive: a page cited by NO projection still ships, because the
	// index cites it.
	dir, err := provenanceSite(t, nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, page := range []string{"p0066.txt", "p0127.txt"} {
		p := filepath.Join(dir, "extracted", budgetDocID, "pages", page)
		if _, serr := os.Stat(p); serr != nil {
			t.Errorf("%s was published as a locator and its text was not shipped: %v", page, serr)
		}
	}
}

// TestTheProvenancePageLinksBothHalvesOfEveryCitation. The page exists to be
// followed, so every link it draws for a row has to land on something the site
// wrote or on the city's own document -- and the PDF link must be the city's
// canonical URL, never a forge's raw host, which serves LFS pointer text rather
// than a PDF.
func TestTheProvenancePageLinksBothHalvesOfEveryCitation(t *testing.T) {
	dir, err := provenanceSite(t, nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	page := readFile(t, dir, "provenance.html")
	for _, want := range []string{
		"facts/d/pages/p0066.jsonl",
		"facts/d/pages/p0127.jsonl",
		"extracted/" + budgetDocID + "/pages/p0066.txt",
		"#page=66",
		"#page=127",
		"the spine",
		"property taxes",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the provenance page does not carry %q", want)
		}
	}
	if strings.Contains(page, "raw.githubusercontent.com") {
		t.Error("a citation points at the raw host, which serves LFS pointer text")
	}

	// Every relative reference resolves, the same walk the other views get.
	ref := regexp.MustCompile(`(?:src|href)="([^"]+)"`)
	found := 0
	for _, m := range ref.FindAllStringSubmatch(page, -1) {
		target := m[1]
		if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
			continue
		}
		found++
		if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(target))); serr != nil {
			t.Errorf("provenance.html references %q, which was not written: %v", target, serr)
		}
	}
	if found < 6 {
		t.Errorf("provenance.html has %d relative references, want its shards, its "+
			"page text and its stylesheet", found)
	}
}

// TestTheChartsAccessibleNameIsTheYearViewsOwnString pins the server-rendered
// <title id="chart-title"> on both chart-bearing templates to the string the
// client repaints on a year switch: the config blob's years[0].chart_title.
//
// THE ELEMENT IS AN ACCESSIBILITY LABEL, so drift here is the one wording
// defect no sighted reader can see: a template composing its own copy of the
// sentence renders one wording, and the first year toggle repaints the other,
// announced only through a screen reader (fisc-rn0; this element is
// fisc-m1uu). Comparing the markup against the blob fails whichever side
// moves, including a template that regrows a hand-composed literal.
func TestTheChartsAccessibleNameIsTheYearViewsOwnString(t *testing.T) {
	fundFlows, err := os.ReadFile("../../testdata/fund-flows.golden.json")
	if err != nil {
		t.Fatalf("read fund-flows golden: %v", err)
	}
	second := reyeared(t, goldenSankey(t), 2027, "FY 2026-27")

	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "sankey-2027": second, "fund-flows": fundFlows,
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
				Projection: "sankey", YearStems: []string{"sankey", "sankey-2027"}},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	for _, page := range []string{export.IndexPath} {
		src := readFile(t, dir, page)
		var config struct {
			Years []struct {
				ChartTitle string `json:"chart_title"`
			} `json:"years"`
		}
		if err := json.Unmarshal(configBlob(t, src), &config); err != nil {
			t.Fatalf("%s: decode FISC_CONFIG: %v", page, err)
		}
		// The opening year is the last: buildSankeyPage renders years[len-1].
		if len(config.Years) == 0 || config.Years[len(config.Years)-1].ChartTitle == "" {
			t.Fatalf("%s: the config carries no opening chart_title to compare against", page)
		}
		const open = `<title id="chart-title">`
		got := strings.TrimPrefix(between(t, readerVisible(t, src), open, "</title>"), open)
		if want := template.HTMLEscapeString(config.Years[len(config.Years)-1].ChartTitle); got != want {
			t.Errorf("%s renders chart title %q, but the client repaints %q on a year switch",
				page, got, want)
		}
	}
}

// chartAndSpine writes the template that draws a chart: the spine's index.html.
func chartAndSpine(t *testing.T) string {
	t.Helper()
	fundFlows, err := os.ReadFile("../../testdata/fund-flows.golden.json")
	if err != nil {
		t.Fatalf("read fund-flows golden: %v", err)
	}
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t), "fund-flows": headlined(t, fundFlows)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
				Projection: "sankey"},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

// TestTheApparatusShipsClosedOnEveryChartPage. A <details> and a <details open>
// render identically to whoever wrote them -- the difference only shows on a
// reader's first visit -- so the attribute needs an assertion rather than an
// eye.
//
// IT IS A RULE OVER EVERY DISCLOSURE ON THE PAGE, not a list of three ids. A
// list would say nothing about the next panel somebody folds, which is the one
// likely to ship open by accident.
func TestTheApparatusShipsClosedOnEveryChartPage(t *testing.T) {
	dir := chartAndSpine(t)
	// `open`, `open>` and `open=""` are one boolean attribute.
	open := regexp.MustCompile(`<details[^>]*\sopen(?:[\s>]|="")`)

	for _, page := range []string{export.IndexPath} {
		html := readFile(t, dir, page)
		if n := strings.Count(html, "<details"); n < 2 {
			t.Errorf("%s renders %d <details>, want the apparatus folded", page, n)
		}
		if m := open.FindString(html); m != "" {
			t.Errorf("%s ships a disclosure open: %s", page, m)
		}
		// Every disclosure the page renders: the regex above says only that
		// nothing is open. #sources-view is here because this site has no
		// sources page.
		for _, id := range []string{"derived-view", "table-view", "sources-view"} {
			if !strings.Contains(html, `<details class="apparatus" id="`+id+`">`) {
				t.Errorf("%s does not fold #%s into an .apparatus disclosure", page, id)
			}
		}
	}
}

// TestAClosedFlowTableIsNotDescribedAsListedBelow is the a11y claim
// index.html.tmpl's disclosure comment argues, enforced rather than restated: a
// closed <details> is collapsed for assistive technology as well as visually,
// so its content is out of the accessibility tree and out of in-page find until
// the reader opens it. A <desc> telling a screen-reader user that the figures
// are "listed below" then sends them looking for a table that is not, from
// their position, there.
//
// WRITTEN AS A RULE OVER THE PAGES THAT HAVE BOTH, so it covers whichever
// template folds its table next rather than the two that already have. The
// condition is the fold: a page whose #table-view ships open may say "below",
// because there it is true.
func TestAClosedFlowTableIsNotDescribedAsListedBelow(t *testing.T) {
	dir := chartAndSpine(t)
	desc := regexp.MustCompile(`(?s)<desc id="chart-desc">(.*?)</desc>`)
	closedTable := regexp.MustCompile(`<details[^>]*id="table-view"[^>]*>`)

	checked := 0
	var withTable []string
	for _, page := range []string{export.IndexPath} {
		html := readFile(t, dir, page)
		tag := closedTable.FindString(html)
		if tag == "" {
			continue
		}
		withTable = append(withTable, page)
		if strings.Contains(tag, " open") {
			continue
		}
		m := desc.FindStringSubmatch(html)
		if m == nil {
			t.Errorf("%s folds its flow table but renders no chart <desc>", page)
			continue
		}
		// The template writes the whole <desc> itself, so there is no caller-supplied
		// half to strip. Read as a browser does: entities decoded, whitespace
		// collapsed because the template wraps it.
		suffix := strings.TrimSpace(strings.Join(strings.Fields(stdhtml.UnescapeString(m[1])), " "))
		if suffix == "" {
			t.Errorf("%s renders an empty <desc>, so it says nothing about where "+
				"the table is", page)
			continue
		}
		checked++
		if strings.Contains(suffix, "below") && !strings.Contains(suffix, "opens from") {
			t.Errorf("%s folds its flow table and its <desc> still says the figures are "+
				"below without saying what opens them:\n%s", page, suffix)
			continue
		}
		// AND THE HEADING IT NAMES HAS TO EXIST. The sentence navigates a
		// screen-reader user by quoting the table's own <h2> verbatim, and two
		// copies of one string in one file is how a renamed heading ships a
		// pointer to nothing -- green, because "opens from" is still there.
		_, afterQuote, opened := strings.Cut(suffix, `"`)
		quoted, _, shut := strings.Cut(afterQuote, `"`)
		if !opened || !shut || quoted == "" {
			t.Errorf("%s's <desc> promises a heading without naming one: %s", page, suffix)
			continue
		}
		if !strings.Contains(html, "<h2>"+quoted+"</h2>") {
			t.Errorf("%s's <desc> sends a reader to a heading %q that the page does not "+
				"render", page, quoted)
		}
	}
	// Anti-vacuity, without contradicting the skip above: the page must render a
	// #table-view at all, or the loop is green over nothing.
	if len(withTable) != 1 {
		t.Errorf("%d of 1 page renders a #table-view: %v; the assertion above is about "+
			"the pages that have one", len(withTable), withTable)
	}
	if checked == 0 {
		t.Error("no page folds its flow table, so nothing above was checked")
	}
}

// TestTheFooterSourcesFoldWithoutTakingTheDocumentOnScreenWithThem covers the
// footer's two halves, which are two different claims and belong on two
// different sides of the disclosure.
//
// The source list is two links per cited page and grows with the corpus, so it
// folds. The sentence under it names the scope, the basis and the file the page
// is drawn from -- which is to say WHICH DOCUMENT THE READER IS LOOKING AT --
// and a reader must not have to open anything to learn that. On index.html and
// the chart pages the basis half of it is repainted per year, so folding it
// would hide a string the client is still writing.
//
// TestEachViewsFooterCitesItsOwnSources stays green through the fold, which is
// why this exists beside it rather than inside it: it asks what the footer
// names, not what a reader arrives to.
func TestTheFooterSourcesFoldWithoutTakingTheDocumentOnScreenWithThem(t *testing.T) {
	dir := twoViews(t, 127, 128)
	// Three spellings, as above.
	open := regexp.MustCompile(`<details[^>]*id="sources-view"[^>]*\sopen(?:[\s>]|="")`)

	for _, page := range []string{export.IndexPath, "trends.html"} {
		html := readFile(t, dir, page)
		start := strings.Index(html, "<footer>")
		if start < 0 {
			t.Fatalf("%s renders no footer", page)
		}
		foot := html[start:]
		summary := strings.Index(foot, "<summary>")
		summaryEnd := strings.Index(foot, "</summary>")
		heading := strings.Index(foot, "<h3>Sources")
		basis := strings.Index(foot, "Projection:")
		closed := strings.Index(foot, "</details>")
		for name, at := range map[string]int{
			"<summary>": summary, "</summary>": summaryEnd, "<h3>Sources": heading,
			"Projection:": basis, "</details>": closed,
		} {
			if at < 0 {
				t.Fatalf("%s's footer renders no %s", page, name)
			}
		}
		if !strings.Contains(foot, `<details class="apparatus" id="sources-view">`) {
			t.Errorf("%s does not fold its source list into an .apparatus disclosure", page)
		}
		// THE SUMMARY DESCRIBES THE WHOLE PANEL. It counts documents AND files
		// because the fold swept the data-file list in beside the sources, and
		// a summary naming only half of what is behind it gives a reader no
		// reason to open it for the other half.
		folded := foot[summary:closed]
		if !strings.Contains(folded, "Every data file the site publishes") {
			t.Errorf("%s's summary counts data files that are not inside the panel", page)
		}
		// THE SUMMARY ALONE, and the first draft of this arm read the whole
		// folded panel -- which contains the phrase "data file" in the list
		// itself, so a summary that stopped mentioning files stayed green.
		// Caught by the mutation, not by reading it.
		head := foot[summary:summaryEnd]
		if !strings.Contains(head, "data file") {
			t.Errorf("%s's sources summary does not say the panel also holds the data "+
				"files it hides: %s", page, strings.Join(strings.Fields(head), " "))
		}
		if open.MatchString(foot) {
			t.Errorf("%s ships its source list open", page)
		}
		// BOTH BOUNDS. Asserting only that the heading comes AFTER <summary>
		// leaves it free to sit anywhere below, including inside the collapsed
		// panel -- which takes it out of the heading outline a screen reader
		// navigates by, the one thing keeping the folded list findable.
		// Measured: moved past </summary>, this test stayed green.
		if summary > heading || heading > summaryEnd {
			t.Errorf("%s's Sources heading is at %d, not between <summary> at %d and "+
				"</summary> at %d", page, heading, summary, summaryEnd)
		}
		// The disclosure closes BEFORE the scope sentence, which is what puts
		// that sentence on the reader's side of the fold.
		if closed > basis {
			t.Errorf("%s's scope-and-basis sentence is inside the folded source list", page)
		}
	}
}

// TestTheCaveatsPageFoldsItsFileListAndNotItsReason. caveats.html is the one
// page whose footer holds no source list -- it shows no figures, so a union of
// every document's pages would be the broadest false provenance claim on the
// site -- so its disclosure holds only the data files and carries a different
// id saying so.
//
// THE PARAGRAPH BELOW IT MUST STAY OUT. It says this page draws no projection
// and publishes no figure, which is the sentence that makes the missing source
// list read as deliberate rather than as an omission.
func TestTheCaveatsPageFoldsItsFileListAndNotItsReason(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			export.PrimaryProjection: goldenSankey(t),
			"revenue-trends":         trendsDoc(127),
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: export.PrimaryProjection},
			{Path: "caveats.html", Nav: "Caveats", Template: export.CaveatsTemplate,
				Title: "Caveats", Lede: "A lede."},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(127),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "caveats.html")

	start := strings.Index(html, "<footer>")
	if start < 0 {
		t.Fatal("caveats.html renders no footer")
	}
	foot := html[start:]

	if !strings.Contains(foot, `<details class="apparatus" id="data-view">`) {
		t.Error("caveats.html does not fold its data-file list into an .apparatus " +
			"disclosure")
	}
	if regexp.MustCompile(`<details[^>]*\sopen(?:[\s>]|="")`).MatchString(foot) {
		t.Error("caveats.html ships its footer disclosure open")
	}
	// The id is the claim: this one holds no sources, and naming it sources-view
	// would say it does.
	if strings.Contains(foot, `id="sources-view"`) {
		t.Error("caveats.html's footer claims a sources-view; that page cites no pages, " +
			"which is what the comment above its footer is about")
	}
	if !strings.Contains(foot, "Every data file the site publishes") {
		t.Error("caveats.html's disclosure holds no data-file list, so it folds nothing")
	}
	// The heading is in the outline here for the same reason it is on every
	// other page: it is what makes a folded panel findable without opening it.
	head := strings.Index(foot, "<summary>")
	headEnd := strings.Index(foot, "</summary>")
	title := strings.Index(foot, "<h3>The data</h3>")
	if head < 0 || headEnd < 0 || title < 0 {
		t.Fatalf("caveats.html's footer is missing its summary (%d, %d) or its "+
			"heading (%d)", head, headEnd, title)
	}
	if title < head || title > headEnd {
		t.Errorf("caveats.html's data heading is at %d, not between <summary> at %d "+
			"and </summary> at %d", title, head, headEnd)
	}
	shut := strings.Index(foot, "</details>")
	reason := strings.Index(foot, "publishes no figure")
	if shut < 0 || reason < 0 {
		t.Fatalf("caveats.html's footer is missing its disclosure (%d) or its "+
			"reason (%d)", shut, reason)
	}
	if shut > reason {
		t.Error("caveats.html folded the sentence saying it publishes no figure, which " +
			"is what makes the absent source list read as deliberate")
	}
}

// TestEveryFooterDisclosureKeepsItsHeadingInTheOutline asserts the rule on the
// TEMPLATES rather than on rendered pages: the pages that cite a figure
// execute one sources partial, and caveats.html.tmpl folds its own file list,
// so two files hold every footer disclosure the site renders. A page that
// grows its own copy of the footer again is refused here, because a copy is
// one this walk would have to find to hold to the rule.
//
// WHY IT MATTERS AT ALL. A <summary> stays in the accessibility tree when the
// panel is closed and its content does not, so a heading inside the summary is
// the only thing that keeps a folded list findable by heading navigation
// without opening it. Below the summary it is inside the fold with everything
// else.
func TestEveryFooterDisclosureKeepsItsHeadingInTheOutline(t *testing.T) {
	entries, err := fs.ReadDir(site.FS(), ".")
	if err != nil {
		t.Fatalf("read embedded site: %v", err)
	}
	var folding, calling []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".html.tmpl") {
			continue
		}
		b, err := fs.ReadFile(site.FS(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(b)
		if strings.Contains(src, `{{template "sources" .}}`) {
			calling = append(calling, name)
		}
		disclosure := strings.Index(src, `<details class="apparatus" id="sources-view">`)
		if disclosure < 0 {
			disclosure = strings.Index(src, `<details class="apparatus" id="data-view">`)
		}
		if disclosure < 0 {
			continue
		}
		folding = append(folding, name)
		rest := src[disclosure:]
		summary := strings.Index(rest, "<summary>")
		summaryEnd := strings.Index(rest, "</summary>")
		heading := strings.Index(rest, "<h3>")
		if summary < 0 || summaryEnd < 0 || heading < 0 {
			t.Errorf("%s's footer disclosure has no summary (%d, %d) or no heading (%d)",
				name, summary, summaryEnd, heading)
			continue
		}
		if heading < summary || heading > summaryEnd {
			t.Errorf("%s puts its footer heading outside the <summary>, which takes it "+
				"out of the outline while the panel is closed", name)
		}
	}
	// Anti-vacuity, and the one-copy rule: the partial and caveats.html.tmpl
	// hold the only disclosures, and every page that cites a figure reaches
	// the partial's.
	if diff := cmp.Diff([]string{export.CaveatsTemplate, export.SourcesPartial}, folding); diff != "" {
		t.Errorf("templates folding a footer list (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{export.CapitalTemplate, export.DebtTemplate,
		export.DepartmentTemplate, export.GlanceTemplate, export.HistoryTemplate, export.SankeyTemplate,
		export.LedgerTemplate, export.ProvenanceTemplate, export.TotalsTemplate, export.TreemapTemplate,
		export.TrendsTemplate}, calling); diff != "" {
		t.Errorf("templates executing the sources partial (-want +got):\n%s", diff)
	}
}

// TestTheSankeyPageNamesItsSourcesForEveryYear: index.html.tmpl's
// "sources-heading" replaces the partial's default only because renderPage
// parses the partial first. Parsed the other way round, the partial's block
// replaces the page's and every page reads "Sources" with no error.
func TestTheSankeyPageNamesItsSourcesForEveryYear(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)
	_, footer, ok := strings.Cut(page, `<details class="apparatus" id="sources-view">`)
	if !ok {
		t.Fatal("index.html renders no sources disclosure")
	}
	summary, _, ok := strings.Cut(footer, "</summary>")
	if !ok {
		t.Fatal("index.html's sources disclosure has no summary")
	}
	if want := "<h3>Sources for every year this page can show</h3>"; !strings.Contains(summary, want) {
		t.Errorf("index.html's sources summary = %q, want it to carry %q", summary, want)
	}
}

// detailsDepth is how many <details> elements are open at byte i of page.
func detailsDepth(page string, i int) int {
	return strings.Count(page[:i], "<details") - strings.Count(page[:i], "</details>")
}

// TestTheTablePagesFoldTheirApparatusAndNotTheirWarnings holds trends.html,
// history.html and provenance.html to fisc-yj4w.16's line: each page's
// apparatus ships in a CLOSED disclosure whose heading stays in the <summary>
// and whose content is inside it, the caveats sit below the table they
// qualify, and the text a reader needs before the first mark -- the Shape
// paragraph, the locator rule, the downloads -- is inside no <details> at all.
func TestTheTablePagesFoldTheirApparatusAndNotTheirWarnings(t *testing.T) {
	trends := readFile(t, twoViews(t, 127, 128), "trends.html")

	var hdoc map[string]any
	if err := json.Unmarshal(historyDoc(balancesFixture()...), &hdoc); err != nil {
		t.Fatal(err)
	}
	hdoc["metadata"].(map[string]any)["caveats"] = []map[string]any{{
		"id": "a-caveat-the-balances-carry", "summary": "a caveat the balances carry",
		"text": "a caveat the balances carry, at length",
	}}
	hbytes, err := json.Marshal(hdoc)
	if err != nil {
		t.Fatal(err)
	}
	hdir, err := writeHistorySite(t, hbytes, balancesSections())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	history := readFile(t, hdir, "balances.html")

	pdir, err := provenanceSite(t, nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	provenance := readFile(t, pdir, "provenance.html")

	for _, tc := range []struct {
		page, html, id, heading, folded string
		// below is what the disclosure must follow; "" for none.
		below string
		open  []string
	}{
		{"trends.html", trends, "caveats-view", "What this table does not say",
			"a caveat this document carries", "</table>",
			[]string{"marks in different rows are not comparable"}},
		{"balances.html", history, "caveats-view", "What this table does not say",
			"a caveat the balances carry", "</table>",
			[]string{"<strong>Shape</strong> column"}},
		{"provenance.html", provenance, "pages-view", "By page",
			"property taxes", "",
			[]string{"<strong>locator</strong>", "<h2>Download</h2>", "Every figure as CSV"}},
	} {
		tag := `<details class="apparatus" id="` + tc.id + `">`
		start := strings.Index(tc.html, tag)
		if start < 0 {
			t.Errorf("%s has no closed %s disclosure", tc.page, tc.id)
			continue
		}
		end := start + strings.Index(tc.html[start:], "</details>")
		panel := tc.html[start:end]
		summary, _, _ := strings.Cut(panel, "</summary>")
		if !strings.Contains(summary, "<h2>"+tc.heading+"</h2>") {
			t.Errorf("%s's %s summary does not carry <h2>%s</h2>, so the heading leaves "+
				"the outline while the panel is closed", tc.page, tc.id, tc.heading)
		}
		if !strings.Contains(panel, tc.folded) {
			t.Errorf("%s's %s disclosure does not hold %q", tc.page, tc.id, tc.folded)
		}
		if n := strings.Count(tc.html, tc.folded); n != 1 {
			t.Errorf("%s prints %q %d times, want once, inside the disclosure", tc.page, tc.folded, n)
		}
		if tc.below != "" && strings.Index(tc.html, tc.below) > start {
			t.Errorf("%s's %s opens above %s; it belongs below the table", tc.page, tc.id, tc.below)
		}
		for _, want := range tc.open {
			i := strings.Index(tc.html, want)
			switch {
			case i < 0:
				t.Errorf("%s does not print %q", tc.page, want)
			case detailsDepth(tc.html, i) != 0:
				t.Errorf("%s folds %q behind a <details>; it is stated where a reader meets it",
					tc.page, want)
			}
		}
	}
}

// TestTheSpineShipsItsBulkButtonsHiddenAndItsHintEmpty: the page config
// carries no steps key, Expand all and Collapse all are in the toolbar hidden
// and disabled for app.js to offer, the hint is empty for app.js to fill, and
// no chip row is shipped.
func TestTheSpineShipsItsBulkButtonsHiddenAndItsHintEmpty(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{{Path: export.IndexPath, Nav: "Budget flows",
			Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"}},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	page := readPage(t, dir)
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(configBlob(t, page), &cfg); err != nil {
		t.Fatalf("decode window.FISC_CONFIG: %v", err)
	}
	if _, ok := cfg["steps"]; ok {
		t.Errorf("the spine ships steps %s, want no steps key at all", cfg["steps"])
	}
	visible := readerVisible(t, page)
	for _, want := range []string{
		`<button type="button" id="chart-expand-all" disabled hidden>Expand all</button>`,
		`<button type="button" id="chart-collapse-all" disabled hidden>Collapse all</button>`,
		`<p class="subtle chart-hint" id="chart-hint"></p>`,
	} {
		if !strings.Contains(visible, want) {
			t.Errorf("the spine does not carry %s", want)
		}
	}
	if strings.Contains(page, `id="breadcrumb"`) {
		t.Errorf("the spine ships a chip row above the chart")
	}
}

// builtLike restamps a document's generated_by to another's, so two committed
// goldens can be paired on one page: the footer credits a single builder.
func builtLike(t *testing.T, like, raw []byte) []byte {
	t.Helper()
	var src, doc map[string]any
	if err := json.Unmarshal(like, &src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	doc["metadata"].(map[string]any)["generated_by"] =
		src["metadata"].(map[string]any)["generated_by"]
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return out
}

// TestADeclaredTierIsCarriedInEveryYearTheViewLists: a chart is one year's,
// so a column it declares is held against that year's document. One year of
// two missing the tier passes an any-year check, and the client then lays
// that year's chart out around an empty column.
func TestADeclaredTierIsCarriedInEveryYearTheViewLists(t *testing.T) {
	spine := goldenSankey(t)
	projections := func(stem string, tier int) map[string][]byte {
		p := map[string][]byte{
			"sankey":      spine,
			"sankey-2027": reyeared(t, spine, 2027, "FY 2026-27"),
		}
		if stem != "" {
			p[stem] = withoutTier(t, p[stem], tier)
		}
		return p
	}
	view := export.View{
		Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate,
		Projection: "sankey", YearStems: []string{"sankey", "sankey-2027"},
		Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{Tiers: []int{0, 2, 5}}},
	}
	write := func(projections map[string][]byte) error {
		_, err := writeSite(export.Options{
			Dir: t.TempDir(), Projections: projections, Views: []export.View{view},
			Docs: budgetDocs(), GeneratedBy: "fisc test",
		})
		return err
	}
	if err := write(projections("", 0)); err != nil {
		t.Fatalf("the control was refused, so the case below is evidence of nothing: %v", err)
	}
	err := write(projections("sankey-2027", 5))
	if want := `chart draws tier 5, and "sankey-2027", its document in that year`; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("the view's own tier the second year's document lacks: err = %v, want it to say %q", err, want)
	}
}

// withoutTier drops every node at one tier from a projection document, and
// every link touching one.
func withoutTier(t *testing.T, raw []byte, tier int) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("withoutTier: %v", err)
	}
	gone := map[string]bool{}
	var nodes []any
	for _, n := range doc["nodes"].([]any) {
		node := n.(map[string]any)
		if int(node["tier"].(float64)) == tier {
			gone[node["id"].(string)] = true
			continue
		}
		nodes = append(nodes, n)
	}
	if len(gone) == 0 {
		t.Fatalf("withoutTier: the document has no node at tier %d, so dropping it is no evidence", tier)
	}
	var links []any
	for _, l := range doc["links"].([]any) {
		link := l.(map[string]any)
		if !gone[link["source"].(string)] && !gone[link["target"].(string)] {
			links = append(links, l)
		}
	}
	doc["nodes"], doc["links"] = nodes, links
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("withoutTier: %v", err)
	}
	return out
}

// The years list oldest first and the page opens on the last, read off the
// rendered page.
func TestThePageOpensOnTheYearItDeclaresWhileListingThemOldestFirst(t *testing.T) {
	var second map[string]any
	if err := json.Unmarshal(goldenSankey(t), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := second["metadata"].(map[string]any)
	meta["fiscal_year"] = 2027
	meta["fiscal_year_label"] = "FY 2026-27"
	raw, marshalErr := json.Marshal(second)
	if marshalErr != nil {
		t.Fatalf("encode: %v", marshalErr)
	}

	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey": goldenSankey(t), "sankey-2027": raw,
		},
		Views: []export.View{{
			Path: export.IndexPath, Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
			Projection: "sankey",
			YearStems:  []string{"sankey", "sankey-2027"},
		}},
		Docs:        budgetDocs(),
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

	// The list keeps its ascending order: the control is a time axis.
	iFirst := strings.Index(page, `id="year-sankey"`)
	iSecond := strings.Index(page, `id="year-sankey-2027"`)
	if iFirst < 0 || iSecond < 0 {
		t.Fatalf("index.html is missing a year radio: sankey at %d, sankey-2027 at %d", iFirst, iSecond)
	}
	if iSecond < iFirst {
		t.Error("the year radios render newest first; the list is the order a reader " +
			"meets them in and must stay ascending, which is what Opens exists to allow")
	}

	// And the newest is already selected, with the page's words agreeing.
	if !strings.Contains(page, `value="sankey-2027"`+"\n             checked") {
		t.Error("index.html does not mark sankey-2027 checked, so the page opens on " +
			"the year it lists first rather than the year it declares")
	}
	if strings.Contains(page, `value="sankey"`+"\n             checked") {
		t.Error("index.html marks sankey checked as well; two checked radios in one " +
			"group is a page that cannot say which year it is showing")
	}
	if !strings.Contains(page, ">FY 2026-27 adopted<") {
		t.Error("index.html's lede does not name FY 2026-27, so its tiles and its " +
			"checked radio describe different years")
	}
	if !strings.Contains(page, "<title>City of Livermore budget flows — FY 2026-27</title>") {
		t.Error("the document title is not the opening year's")
	}
}
