package export_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
	"github.com/jcrussell/livermore-budget/site"
)

// goldenPath is the frozen worked example of the sankey contract: real FY2026
// figures with real fact ids. Testing against it rather than a toy fixture is
// the point — the numbers a reviewer checks by eye are the numbers the page
// has to show.
const goldenPath = "../../testdata/sankey.golden.json"

func goldenSankey(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden projection: %v", err)
	}
	return b
}

func budgetDocs() []export.Doc {
	return []export.Doc{{
		ID:        "livermore-budget-fy2026-2027",
		Title:     "FY 2025-2027 Budget Book",
		Publisher: "City of Livermore, California",
		PDFURL:    "https://www.livermoreca.gov/home/showpublisheddocument/12813",
	}}
}

// budgetDocID is the one document the golden projection cites.
const budgetDocID = "livermore-budget-fy2026-2027"

// pageTextFS stands in for data/extracted/. It holds more than the golden
// projection cites — an uncited page of the same document and a whole second
// document — because "only the cited pages ship" is the claim, and a tree
// holding exactly the cited pages could not fail it.
//
// The bytes are synthetic rather than copied from data/extracted/: this package
// does not read page text, it copies it, so what is asserted is that the bytes
// arrive unchanged. A real fixture here would be one more file to re-copy when
// the extraction changes (AGENTS.md, "Testing") and would prove nothing
// extra.
func pageTextFS() fstest.MapFS {
	return fstest.MapFS{
		budgetDocID + "/pages/p0066.txt":        {Data: []byte("REVENUE    123,456    789\n")},
		budgetDocID + "/pages/p0067.txt":        {Data: []byte("EXPENDITURE    987,654\n")},
		budgetDocID + "/pages/p0100.txt":        {Data: []byte("a page nothing cites\n")},
		"livermore-acfr-fy2025/pages/p0177.txt": {Data: []byte("another document\n")},
		budgetDocID + "/geometry/p0066.json":    {Data: []byte("{}")},
		budgetDocID + "/" + "manifest.json":     {Data: []byte("{}")},
	}
}

// shippedPageText is where a cited page lands in the output tree.
func shippedPageText(docID string, page int) string {
	return fmt.Sprintf("%s/%s/pages/p%04d.txt", export.PageTextDir, docID, page)
}

func writeGolden(t *testing.T) (dir string, written []string) {
	t.Helper()
	return writeGoldenOpts(t, export.Options{PageText: pageTextFS()})
}

// writeGoldenOpts writes the golden projection with opts' provenance settings.
// Dir, Projections, Docs and GeneratedBy are this helper's.
func writeGoldenOpts(t *testing.T, opts export.Options) (dir string, written []string) {
	t.Helper()
	dir = t.TempDir()
	opts.Dir = dir
	opts.Projections = map[string][]byte{"sankey": goldenSankey(t)}
	opts.Docs = budgetDocs()
	opts.GeneratedBy = "fisc test"
	written, err := writeSite(opts)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir, written
}

func TestWriteProducesTheSiteLayout(t *testing.T) {
	dir, got := writeGolden(t)

	want := []string{
		".fisc-export",
		"app.js",
		"core.js",
		// Every page's client module ships whatever views the site carries,
		// as sankey.js does: verbatimAssets is not per view.
		"department.js",
		"capital.js",
		"debt.js",
		"glance.js",
		"sides.js",
		"ledger.js",
		"change.js",
		"format.js",
		"theme.js",
		// One document per published column, not also one per schedule.
		"fy2026-adopted.json",
		"index.html",
		"sankey.js",
		"style.css",
		"vendor/d3-sankey.LICENSE",
		"vendor/d3-sankey.min.js",
		"vendor/d3.LICENSE",
		"vendor/d3.min.js",
		".nojekyll",
		// The two pages the golden projection cites, and nothing else out of
		// the extraction tree.
		shippedPageText(budgetDocID, 66),
		shippedPageText(budgetDocID, 67),
	}
	// The returned list is sorted; sort the expectation the same way by
	// comparing sets of names rather than trusting two orderings to agree.
	if diff := cmp.Diff(sorted(want), got); diff != "" {
		t.Errorf("written files (-want +got):\n%s", diff)
	}
	for _, rel := range got {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("reported %q but: %v", rel, err)
		}
	}

	// .nojekyll has to be there and has to be empty: GitHub Pages only reads
	// its presence, and //go:embed silently skips dotfiles unless they are
	// named, so an empty-but-present file is the thing worth asserting.
	info, err := os.Stat(filepath.Join(dir, ".nojekyll"))
	if err != nil {
		t.Fatalf("stat .nojekyll: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("got .nojekyll of %d bytes, want 0", info.Size())
	}
}

func TestEveryAssetThePageAsksForWasWritten(t *testing.T) {
	// A renamed asset is a 404 in the browser and nowhere else: the export
	// still succeeds, the page still loads, and the chart silently never
	// arrives. Walk the page's own references instead of trusting a list.
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	var refs []string
	for _, attr := range []string{`src="`, `href="`} {
		rest := page
		for {
			i := strings.Index(rest, attr)
			if i < 0 {
				break
			}
			rest = rest[i+len(attr):]
			j := strings.Index(rest, `"`)
			if j < 0 {
				break
			}
			ref := rest[:j]
			rest = rest[j:]
			// THE FRAGMENT IS STRIPPED, NOT SKIPPED, and the difference is the
			// whole reason this walk can see the caveat links at all. A bare
			// "#anchor" is same-page and rightly ignored; "caveats.html#x"
			// names a FILE and this loop used to stat it verbatim, fragment
			// and all, so the first cross-page anchor the site shipped would
			// have been reported as a missing asset. Cutting first means the
			// file half is checked and the fragment half is left to
			// TestEveryCaveatSummaryLinksToAnAnchorThatExists, which is the
			// only check that can resolve it.
			if before, _, ok := strings.Cut(ref, "#"); ok {
				ref = before
			}
			// Only the relative ones are this export's problem; the absolute
			// ones are citations and are checked elsewhere.
			if ref == "" || strings.Contains(ref, "://") {
				continue
			}
			refs = append(refs, ref)
		}
	}
	if len(refs) < 4 {
		t.Fatalf("got %d relative references in the page, want at least the script, style and data files: %v", len(refs), refs)
	}
	for _, ref := range refs {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ref))); err != nil {
			t.Errorf("the page references %q, which was not written: %v", ref, err)
		}
	}
}

// TestWriteCopiesAColumnlessProjectionVerbatim: revenue-trends and the two
// balance documents have no column and no schema, so they must ship byte for
// byte. A document with a column is re-encoded and held by encodeColumn's
// schema check instead.
func TestWriteCopiesAColumnlessProjectionVerbatim(t *testing.T) {
	series := []byte(`{"schema_version":1,"projection":"revenue-trends",` +
		`"metadata":{"generated_by":"fisc test","sources":[]},"series":[]}` + "\n")
	dir := t.TempDir()
	written, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t), "revenue-trends": series},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if slices.Contains(written, "data/sankey.json") {
		t.Error("sankey folds into a column and must not also ship at data/sankey.json")
	}
	got, rerr := os.ReadFile(filepath.Join(dir, "data", "revenue-trends.json"))
	if rerr != nil {
		t.Fatalf("read exported projection: %v", rerr)
	}
	if diff := cmp.Diff(string(series), string(got)); diff != "" {
		t.Errorf("exported projection differs from its input (-want +got):\n%s", diff)
	}
}

// indexSite writes the index beside the caveats page, over the golden spine.
func indexSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate,
				Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "caveats.html", Nav: "Caveats", Template: export.CaveatsTemplate},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

// TestThePlanWritesTheMarkerFirstAndTheIndexLast holds Prepare's write order:
// the marker before anything else, so a run that dies midway leaves a
// directory --clean recognises, and the pages after every asset they fetch,
// with index.html, the completed-site sentinel, last of all. The index is
// declared first here so that writing the pages in declaration order fails.
//
// Mutation: add the pages in declaration order without holding IndexPath back
// and this goes red with "the last planned file is \"caveats.html\"".
// Moving the marker's add below the verbatim assets goes red with "the first
// planned file is \"app.js\"".
func TestThePlanWritesTheMarkerFirstAndTheIndexLast(t *testing.T) {
	p, err := export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows", Template: export.SankeyTemplate,
				Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "caveats.html", Nav: "Caveats", Template: export.CaveatsTemplate},
		},
		Files:       map[string][]byte{"extra/asset.txt": []byte("x")},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	paths := export.PlannedPaths(p)
	if len(paths) < 3 {
		t.Fatalf("planned %q, want the marker, assets and pages", paths)
	}
	if paths[0] != export.MarkerName {
		t.Errorf("the first planned file is %q, want %q", paths[0], export.MarkerName)
	}
	if last := paths[len(paths)-1]; last != export.IndexPath {
		t.Errorf("the last planned file is %q, want %q", last, export.IndexPath)
	}
	firstPage := -1
	for i, rel := range paths {
		isPage := strings.HasSuffix(rel, ".html")
		if isPage && firstPage < 0 {
			firstPage = i
		}
		if !isPage && firstPage >= 0 {
			t.Errorf("%q is planned after the page %q", rel, paths[firstPage])
		}
	}
	if firstPage < 0 {
		t.Fatalf("planned %q, want pages", paths)
	}
}

// Write stops before a file once its context is cancelled.
//
// Mutation: drop the ctx check from Write and this goes red with "wrote
// [...] after cancellation".
func TestWriteStopsOnACancelledContext(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	p, err := export.Prepare(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views:       spineOnly(),
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	written, err := p.Write(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got error %v, want it to wrap context.Canceled", err)
	}
	if entries, _ := os.ReadDir(dir); len(written) > 0 || len(entries) > 0 {
		t.Errorf("wrote %q (%d entries on disk) after cancellation", written, len(entries))
	}
}

// TestTheIndexLeadsWithWhatTheCitySpends: the index's one headline figure is
// the golden spine's all-funds gross expenditure, in the hero. Mutation: drop
// {{.Hero.Value}} from site/flows.html.tmpl, and this goes red.
func TestTheIndexLeadsWithWhatTheCitySpends(t *testing.T) {
	index := readerVisible(t, readPage(t, indexSite(t)))
	hero := strings.Index(index, `id="hero"`)
	if hero < 0 {
		t.Fatal("index.html renders no hero")
	}
	span, _, _ := strings.Cut(index[hero:], "</strong>")
	if !strings.Contains(span, "$254,095,412") {
		t.Error("index.html's hero does not state what the city spends")
	}
}

// TestTheChartComesBeforeAnyApparatus pins the reading order: the title in
// the page head, then the chart card, the chart and its captions, then the
// lede and everything that explains them. Above the chart is only what
// operates it, so the opening chart fits the window app.js's roomHeight lays
// it out to: no paragraph but the filter line, which carries its Reset. Every marker is Fatalf-guarded, because an absent one is -1 and
// before everything. Mutation: put the lede back in the page head -- red;
// a hint paragraph under the title -- red; put an <h2> or a <details> back
// above the card -- red.
func TestTheChartComesBeforeAnyApparatus(t *testing.T) {
	page := readerVisible(t, readPage(t, indexSite(t)))
	markers := []string{`<h1`, `id="chart-card"`, `id="chart"`,
		`id="splits-line"`, `id="hairline-line"`, `id="hero"`, "Revenue on the left", `class="apparatus"`}
	at := make([]int, len(markers))
	for i, m := range markers {
		at[i] = strings.Index(page, m)
		if at[i] < 0 {
			t.Fatalf("page renders no %s", m)
		}
	}
	for i := 1; i < len(markers); i++ {
		if at[i-1] >= at[i] {
			t.Errorf("%s is at %d, which is not before %s at %d", markers[i-1], at[i-1], markers[i], at[i])
		}
	}
	card, chart := at[1], at[2]
	for _, tag := range []string{"<h2", "<details", "<section"} {
		if i := strings.Index(page, tag); i >= 0 && i < card {
			t.Errorf("%s at %d comes before the chart card at %d", tag, i, card)
		}
	}
	for _, p := range regexp.MustCompile(`<p[ >][^\n]{0,60}`).FindAllString(page[:chart], -1) {
		if !strings.HasPrefix(p, `<p class="filter-line"`) {
			t.Errorf("a paragraph comes before the chart: %q", p)
		}
	}
}

// TestTheIndexLinksItsCaveatsAndPrintsNone: caveats.html carries every
// caveat whole, and a chart mark carries its own, so the index prints none
// of them and links the page. Mutation: drop the link -- red; render the
// summaries back -- red.
func TestTheIndexLinksItsCaveatsAndPrintsNone(t *testing.T) {
	dir := indexSite(t)
	visible := readerVisible(t, readPage(t, dir))

	var doc struct {
		Metadata struct {
			Caveats []struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
			} `json:"caveats"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(goldenSankey(t), &doc); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(doc.Metadata.Caveats) == 0 {
		t.Fatal("golden projection has no caveats, so nothing below is checked")
	}
	for _, c := range doc.Metadata.Caveats {
		if strings.Contains(visible, template.HTMLEscapeString(c.Summary)) {
			t.Errorf("index.html prints caveat %q, which caveats.html carries", c.ID)
		}
	}
	if !strings.Contains(visible, `<a href="caveats.html">`) {
		t.Error("index.html does not link the caveats page")
	}
}

// TestAnIndexWithNoCaveatsPageListsTheCaveats: a site with no caveats view
// (writeGolden's views) links no caveats page, so the index lists every
// caveat's summary itself, as the footer lists the cited pages with no
// sources page. Mutation: drop the template's fallback list -- red.
func TestAnIndexWithNoCaveatsPageListsTheCaveats(t *testing.T) {
	dir, _ := writeGolden(t)
	visible := readerVisible(t, readPage(t, dir))

	var doc struct {
		Metadata struct {
			Caveats []struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
			} `json:"caveats"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(goldenSankey(t), &doc); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(doc.Metadata.Caveats) == 0 {
		t.Fatal("golden projection has no caveats, so nothing below is checked")
	}
	for _, c := range doc.Metadata.Caveats {
		if !strings.Contains(visible, template.HTMLEscapeString(c.Summary)) {
			t.Errorf("index.html with no caveats page does not list caveat %q", c.ID)
		}
	}
	if strings.Contains(visible, `href="caveats.html"`) {
		t.Error("index.html links a caveats page the site does not write")
	}
}

// The acceptance test for the whole change: a site exported with the extraction
// tree in hand resolves BOTH classes of citation — the city's PDF and the
// committed page text — and reaches github.com for neither.
//
// The two classes are not symmetric and the asymmetry is deliberate. The PDF is
// the city's document at the city's URL: 1.6 GB of it lives in Git LFS and
// republishing it would be a copy of somebody else's publication. The extracted
// text is ours, it is small, and it is the artifact every figure on the page is
// actually traced to — so it ships.
func TestPageCitesThePDFPageAndTheTextTheSiteShips(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	for _, want := range []string{
		"https://www.livermoreca.gov/home/showpublisheddocument/12813#page=66",
		"https://www.livermoreca.gov/home/showpublisheddocument/12813#page=67",
		shippedPageText(budgetDocID, 66),
		shippedPageText(budgetDocID, 67),
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not cite %q", want)
		}
	}

	// No forge, anywhere: not in the rendered citations and not in the config
	// the client composes its own links from. A citation that needs github.com
	// to be up, reachable and still hosting this repository is the defect this
	// change removes (fisc-ze7).
	// The footer's repository link is not a citation.
	if strings.Contains(strings.ReplaceAll(page, `href="`+export.RepositoryURL+`"`, ""), "github.com") {
		t.Error("the page still cites github.com; the shipped page text should have replaced it")
	}

	// Every page-text citation has to be a file in the output. Resolve them the
	// way a browser would — relative to the page — rather than trusting the
	// string.
	for _, doc := range configDocs(t, page) {
		for _, page := range []string{"66", "67"} {
			ref := doc.Pages[page].Text
			if ref == "" || strings.Contains(ref, "://") {
				t.Errorf("got text link %q for p%s, want a path relative to the site", ref, page)
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ref))); err != nil {
				t.Errorf("the config cites %q, which the site does not carry: %v", ref, err)
			}
		}
	}
}

// The copy has to be byte-for-byte: the runs of spaces ARE the printed column
// grid (AGENTS.md, "The extraction boundary"), so text that arrives reflowed is text a
// reader cannot check a figure against.
func TestCitedPageTextIsShippedVerbatimAndOnlyWhenCited(t *testing.T) {
	tree := pageTextFS()
	dir, written := writeGoldenOpts(t, export.Options{PageText: tree})

	for _, page := range []int{66, 67} {
		rel := shippedPageText(budgetDocID, page)
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read shipped page text: %v", err)
		}
		want := tree[fmt.Sprintf("%s/pages/p%04d.txt", budgetDocID, page)].Data
		if diff := cmp.Diff(string(want), string(got)); diff != "" {
			t.Errorf("%s differs from the extraction (-want +got):\n%s", rel, diff)
		}
	}

	// The corpus is 786 pages and the projection cites two. Shipping the whole
	// extraction would put it in every deploy to publish a handful of links.
	for _, rel := range written {
		if !strings.HasPrefix(rel, export.PageTextDir+"/") {
			continue
		}
		if rel != shippedPageText(budgetDocID, 66) && rel != shippedPageText(budgetDocID, 67) {
			t.Errorf("the site ships %q, which nothing cites", rel)
		}
	}
}

// A citation the extraction cannot back is an error, not a link left dangling:
// the page renders the citation either way, and only one of the two outcomes is
// visible before somebody clicks it.
func TestWriteRefusesACitedPageMissingFromTheExtraction(t *testing.T) {
	tree := pageTextFS()
	delete(tree, budgetDocID+"/pages/p0067.txt")

	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Docs:        budgetDocs(),
		PageText:    tree,
	})
	if err == nil {
		t.Fatal("got nil error, want a refusal to cite text it cannot ship")
	}
	if got := err.Error(); !strings.Contains(got, "page 67") || !strings.Contains(got, budgetDocID) {
		t.Errorf("got error %q, want it to name the document and page", got)
	}
}

// Without an extraction tree the citation has to go somewhere, and the
// documented somewhere is the forge's blob view. This is the fallback, not the
// normal path: `fisc export` always passes the tree.
func TestPageTextFallsBackToTheBlobViewWithoutAnExtractionTree(t *testing.T) {
	dir, written := writeGoldenOpts(t, export.Options{})
	page := readPage(t, dir)

	want := export.DefaultSourceBrowseURL + "/data/extracted/" + budgetDocID + "/pages/p0066.txt"
	if !strings.Contains(page, want) {
		t.Errorf("page does not cite %q", want)
	}
	// raw.githubusercontent.com would serve these bytes correctly —
	// data/extracted/ is ordinary git, not LFS, and the artifacts are .txt, so
	// GitHub's blob view renders the runs of spaces that ARE the column grid
	// rather than collapsing them the way it would for .md (corpus.PagePath
	// says the same thing from the other end). So this is a policy choice, not
	// a correctness one: raw serves the bytes bare — no line numbers, no
	// history, no way to reach the rest of the document — and a provenance
	// citation should land somewhere a reader can navigate from.
	if strings.Contains(page, "raw.githubusercontent.com") {
		t.Error("page links to raw.githubusercontent.com rather than the github.com blob view")
	}
	for _, rel := range written {
		if strings.HasPrefix(rel, export.PageTextDir+"/") {
			t.Errorf("no extraction tree was given but the site shipped %q", rel)
		}
	}
}

// An explicit remote is the caller overruling the default, so it must not also
// ship the text: two published answers to "where is this page" is one more than
// the site can keep true.
func TestSourceBrowseURLCitesTheRemoteAndShipsNothing(t *testing.T) {
	const browse = "https://example.invalid/tree/main/"
	dir, written := writeGoldenOpts(t, export.Options{
		PageText:        pageTextFS(),
		SourceBrowseURL: browse,
	})
	page := readPage(t, dir)

	want := "https://example.invalid/tree/main/data/extracted/" + budgetDocID + "/pages/p0066.txt"
	if !strings.Contains(page, want) {
		t.Errorf("page does not cite %q", want)
	}
	for _, rel := range written {
		if strings.HasPrefix(rel, export.PageTextDir+"/") {
			t.Errorf("the export was told to cite %q but shipped %q as well", browse, rel)
		}
	}
}

// The channel fisc-4ua.8 inherits: an arbitrary path -> bytes, written verbatim
// beside the site. The page text is its first user; the fact store is next.
func TestFilesShipVerbatimBesideTheSite(t *testing.T) {
	facts := []byte(`{"id":"f1","amount_cents":1234}` + "\n")
	dir, written := writeGoldenOpts(t, export.Options{
		PageText: pageTextFS(),
		Files:    map[string][]byte{"provenance/facts.jsonl": facts},
	})

	got, err := os.ReadFile(filepath.Join(dir, "provenance", "facts.jsonl"))
	if err != nil {
		t.Fatalf("read shipped asset: %v", err)
	}
	if diff := cmp.Diff(string(facts), string(got)); diff != "" {
		t.Errorf("shipped asset differs from its input (-want +got):\n%s", diff)
	}
	if !slices.Contains(written, "provenance/facts.jsonl") {
		t.Errorf("Write did not report the asset it wrote:\n%v", written)
	}
	// The channel must not disturb the contract it sits beside.
	if !slices.Contains(written, "fy2026-adopted.json") {
		t.Errorf("the projection's column is no longer at fy2026-adopted.json:\n%v", written)
	}
}

// The output is a web root somebody will serve or rsync. A path that escapes it
// or lands on the fixed layout is refused at the door: shadowing index.html or
// data/sankey.json would export cleanly and break only in a browser.
func TestWriteRefusesAnAssetPathOutsideTheSite(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"absolute":           "/etc/passwd",
		"parent":             "../escape.txt",
		"parent within":      "assets/../../escape.txt",
		"unclean":            "./facts.jsonl",
		"backslash":          `assets\facts.jsonl`,
		"the page itself":    "index.html",
		"an embedded asset":  "app.js",
		"the export marker":  ".fisc-export",
		"a projection":       "data/sankey.json",
		"the projection dir": "data/anything.json",
		"vendored d3":        "vendor/d3.min.js",
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := writeSite(export.Options{
				Dir:         dir,
				Projections: map[string][]byte{"sankey": goldenSankey(t)},
				Docs:        budgetDocs(),
				Files:       map[string][]byte{rel: []byte("x")},
			})
			if err == nil {
				t.Fatalf("got nil error, want a refusal of %q", rel)
			}
			if _, serr := os.Stat(filepath.Join(dir, "index.html")); serr == nil {
				t.Error("the site was written anyway; the screen has to run before any side effect")
			}
		})
	}
}

// Two writers on one path is a silent overwrite whose winner depends on map
// order. The page text and an asset are written by different code paths, so
// this is the collision that can actually happen.
func TestWriteRefusesTwoAssetsClaimingOnePath(t *testing.T) {
	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Docs:        budgetDocs(),
		PageText:    pageTextFS(),
		Files:       map[string][]byte{shippedPageText(budgetDocID, 66): []byte("mine now")},
	})
	if err == nil {
		t.Fatal("got nil error, want a refusal of the colliding path")
	}
	if got := err.Error(); !strings.Contains(got, shippedPageText(budgetDocID, 66)) {
		t.Errorf("got error %q, want it to name the contested path", got)
	}
}

func TestPageConfigCarriesTheProjectionMetadataVerbatim(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	cfg := configBlob(t, page)
	var got struct {
		SchemaVersion int                `json:"schema_version"`
		Primary       string             `json:"primary"`
		Docs          map[string]testDoc `json:"docs"`
	}
	if err := json.Unmarshal(cfg, &got); err != nil {
		t.Fatalf("decode window.FISC_CONFIG: %v", err)
	}

	if got.Primary != "sankey" {
		t.Errorf("got primary %q, want %q", got.Primary, "sankey")
	}
	// No projections map: a year names its document by its column, and the
	// page's chart names its schedule as primary.
	if got.SchemaVersion != export.PageConfigVersion {
		t.Errorf("got schema_version %d, want %d", got.SchemaVersion, export.PageConfigVersion)
	}

	doc, ok := got.Docs["livermore-budget-fy2026-2027"]
	if !ok {
		t.Fatal("config carries no doc entry for the budget book")
	}
	p66 := doc.Pages["66"]
	if want := "https://www.livermoreca.gov/home/showpublisheddocument/12813#page=66"; p66.PDF != want {
		t.Errorf("got p66's pdf link %q, want %q", p66.PDF, want)
	}
	// The site ships the cited pages, so the text link is same-origin and the
	// provenance resolves with no network access to a forge.
	if !strings.HasPrefix(p66.Text, export.LocalPageTextBase(budgetDocID)) {
		t.Errorf("got p66's text link %q, want one under %q", p66.Text, export.LocalPageTextBase(budgetDocID))
	}
}

func TestConfigCannotCloseItsScriptElement(t *testing.T) {
	// A caveat is free text out of a data file. If it ever contains markup,
	// the page must not break — or worse, execute it.
	doc := map[string]any{
		"schema_version": 1,
		"projection":     "sankey",
		"metadata": map[string]any{
			"fiscal_year_label": "FY 2025-26",
			"basis":             "adopted",
			"headline":          map[string]any{"all_funds_gross_expenditure_cents": 25409541200},
			// BOTH FIELDS CARRY THE MARKUP, because both now reach a page and
			// they reach it by different routes: text is rendered into the
			// caveat list by html/template, and summary rides in the
			// FISC_CONFIG blob as JSON inside a <script>. A fixture that
			// attacked only the field it used to have would leave the new one
			// untested while looking like it covered the case.
			"caveats": []map[string]any{{
				"id":         `</script><script>alert("pwned")</script>`,
				"summary":    `</script><script>alert("pwned")</script>`,
				"text":       `</script><script>alert("pwned")</script>`,
				"applies_to": []string{},
			}},
			"sources": []any{},
		},
		"nodes": []any{},
		"links": []any{},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": b},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	page := readPage(t, dir)
	if strings.Contains(page, `</script><script>alert`) {
		t.Error("the caveat's markup survived into the page unescaped")
	}
}

func TestWriteRefusesAProjectionSetWithoutTheSankey(t *testing.T) {
	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"not-the-spine": []byte(`{}`)},
	})
	if err == nil {
		t.Fatal("got nil error, want a refusal")
	}
	if got, want := err.Error(), export.ErrNoPrimary.Error(); got != want {
		t.Errorf("got error %q, want %q", got, want)
	}
}

func TestWriteRefusesBadInput(t *testing.T) {
	cases := map[string]export.Options{
		"no directory":     {Projections: map[string][]byte{"sankey": []byte(`{}`)}},
		"no projections":   {Dir: t.TempDir()},
		"path as a name":   {Dir: t.TempDir(), Projections: map[string][]byte{"sankey": []byte(`{}`), "../evil": []byte(`{}`)}},
		"unparseable json": {Dir: t.TempDir(), Projections: map[string][]byte{"sankey": []byte(`{`)}},
		"no metadata":      {Dir: t.TempDir(), Projections: map[string][]byte{"sankey": []byte(`{"schema_version":1}`)}},
		"no headline": {Dir: t.TempDir(), Projections: map[string][]byte{
			"sankey": []byte(`{"schema_version":1,"metadata":{"fiscal_year_label":"FY 2025-26"}}`),
		}},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := writeSite(opts); err == nil {
				t.Fatal("got nil error, want a refusal")
			}
		})
	}
}

// The client's copies are the ones nothing compiles against, so they are the
// ones that would drift in silence. Read them out of the same embedded asset
// tree the packager ships and pin each literal to its producer's constant.
// Mutation: set PageConfigVersion back to 1 and the page arm goes red; set
// PROJECTION_VERSION to 2 and the projection arm does.
func TestClientSchemaVersionIsPinnedToTheProducer(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "core.js")
	if err != nil {
		t.Fatalf("read embedded core.js: %v", err)
	}
	for _, c := range []struct {
		want, producer string
		version        int
	}{
		{"const SCHEMA_VERSION = %d;", "export.PageConfigVersion", export.PageConfigVersion},
		{"const PROJECTION_VERSION = %d;", "project.SchemaVersion", project.SchemaVersion},
	} {
		want := fmt.Sprintf(c.want, c.version)
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("site/core.js does not declare %q; the client has drifted from %s %d",
				want, c.producer, c.version)
		}
	}
}

// TestThePageBootsTheClientAsAModule pins both halves of the handshake: the
// template imports boot from ./app.js in a module script after the three
// classic scripts whose globals it reads, and app.js exports boot. A rename on
// either side draws nothing, and nothing in this tree runs the page.
func TestThePageBootsTheClientAsAModule(t *testing.T) {
	page, err := fs.ReadFile(site.FS(), export.SankeyTemplate)
	if err != nil {
		t.Fatalf("read embedded %s: %v", export.SankeyTemplate, err)
	}
	app, err := fs.ReadFile(site.FS(), "app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	const boot = `<script type="module">import { boot } from "./app.js"; boot();</script>`
	at := bytes.Index(page, []byte(boot))
	if at < 0 {
		t.Fatalf("%s does not boot the client with %q", export.SankeyTemplate, boot)
	}
	for _, classic := range []string{
		`<script src="vendor/d3.min.js"></script>`,
		`<script src="vendor/d3-sankey.min.js"></script>`,
		`<script>window.FISC_CONFIG = {{.ConfigJSON}};</script>`,
	} {
		i := bytes.Index(page, []byte(classic))
		if i < 0 || i > at {
			t.Errorf("%s loads %q after the module that reads what it sets, or not at all", export.SankeyTemplate, classic)
		}
	}
	if !bytes.Contains(app, []byte("\nexport function boot() {")) {
		t.Errorf("site/app.js exports no boot function; the template's import would resolve to undefined and the page would draw nothing")
	}
	if bytes.Contains(app, []byte("\nmain().catch(")) {
		t.Errorf("site/app.js still calls main() at file scope; importing it would boot a page, so a test could not reach a function without drawing")
	}
}

// A projection whose schema this binary does not know has to be refused, not
// rendered: the keys still decode, so the page would come out plausible and
// wrong. Both directions are errors — an older document is as unreadable as a
// newer one, because the version says what the numbers mean.
func TestWriteRefusesASchemaVersionItDoesNotUnderstand(t *testing.T) {
	// wantVersion is what the error has to report as "got": a missing key and
	// an explicit 0 both decode to 0, and the message says so either way.
	type badVersion struct {
		version int
		absent  bool
	}
	cases := map[string]badVersion{
		"newer than this binary": {version: export.SchemaVersion + 1},
		"an explicit zero":       {version: 0},
		"no schema_version key":  {absent: true},
	}
	// At schema version 1 there is no older non-zero version to hand it, and
	// faking one with 0 would just re-run the case above under another name.
	// The case appears on its own the first time there is a real one.
	if export.SchemaVersion > 1 {
		cases["older than this binary"] = badVersion{version: export.SchemaVersion - 1}
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := writeSite(export.Options{
				Dir:         t.TempDir(),
				Projections: map[string][]byte{"sankey": reversionedGolden(t, tc.version, tc.absent)},
				Docs:        budgetDocs(),
			})
			if err == nil {
				t.Fatal("got nil error, want a refusal")
			}
			if !errors.Is(err, export.ErrSchemaVersion) {
				t.Fatalf("got error %q, want one matching export.ErrSchemaVersion", err)
			}
			want := fmt.Sprintf("got %d, want %d", tc.version, export.SchemaVersion)
			if got := err.Error(); !strings.Contains(got, want) {
				t.Errorf("got error %q, want it to contain %q", got, want)
			}
		})
	}
}

// The gate must not be a blanket refusal: the golden projection carries the
// version this binary understands and has to go through.
func TestWriteAcceptsTheSchemaVersionItUnderstands(t *testing.T) {
	dir, _ := writeGolden(t)
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Fatalf("the current schema version did not produce a page: %v", err)
	}
}

// reversionedGolden is the golden projection with schema_version rewritten, or
// removed when absent is set, and nothing else touched — so a refusal cannot be
// mistaken for a reaction to some other malformation.
func reversionedGolden(t *testing.T, version int, absent bool) []byte {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(goldenSankey(t), &doc); err != nil {
		t.Fatalf("decode golden projection: %v", err)
	}
	if _, ok := doc["schema_version"]; !ok {
		t.Fatal("golden projection carries no schema_version to rewrite")
	}
	if absent {
		delete(doc, "schema_version")
	} else {
		doc["schema_version"] = json.RawMessage(strconv.Itoa(version))
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode golden projection: %v", err)
	}
	return b
}

func TestWriteReportsAMissingAsset(t *testing.T) {
	// A hand-built asset tree missing app.js stands in for a botched embed:
	// the failure has to be an error, not a site that 404s in the browser.
	assets := fstest.MapFS{
		"flows.html.tmpl":     {Data: []byte(`<!doctype html><title>{{.Title}}</title>`)},
		export.SourcesPartial: {Data: []byte(`{{define "sources"}}{{end}}`)},
		export.ChromePartial:  {Data: []byte(`{{define "theme-stamp"}}{{end}}{{define "topbar"}}{{end}}`)},
		"style.css":           {Data: []byte(`body{}`)},
		".nojekyll":           {Data: []byte{}},
		"vendor/d3.min.js":    {Data: []byte(`/* d3 */`)},
	}
	_, err := writeSite(export.Options{
		Dir:         t.TempDir(),
		Assets:      assets,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
	})
	if err == nil {
		t.Fatal("got nil error, want a missing-asset failure")
	}
	if !strings.Contains(err.Error(), "app.js") {
		t.Errorf("got error %q, want it to name the missing asset", err)
	}
}

// The site is published before its coverage is complete, so the page has to say
// so. The claim is about COVERAGE, not accuracy: every figure on the page is
// traced to a printed schedule and `fisc verify` fails if one is not, so a
// banner calling the figures preliminary would contradict the thing the project
// exists to assert.
func TestPageCarriesTheWorkInProgressBanner(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	// Exactly once: a second copy means the template grew a duplicate block,
	// which renders as two stacked bars rather than as an error.
	if got := strings.Count(page, `class="wip-banner"`); got != 1 {
		t.Errorf("got %d wip-banner elements, want 1", got)
	}
	for _, want := range []string{
		"Work in progress",
		"coverage is incomplete",
		`role="status"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("banner does not carry %q", want)
		}
	}
}

// testDoc is one FISC_CONFIG docs entry as the tests read it.
type testDoc struct {
	Title string              `json:"title"`
	Pages map[string]testPage `json:"pages"`
}

// testPage is one cited page's links.
type testPage struct {
	PDF     string `json:"pdf"`
	Text    string `json:"text"`
	Records string `json:"records"`
}

// configDocs decodes window.FISC_CONFIG's docs block.
func configDocs(t *testing.T, page string) map[string]testDoc {
	t.Helper()
	var cfg struct {
		Docs map[string]testDoc `json:"docs"`
	}
	if err := json.Unmarshal(configBlob(t, page), &cfg); err != nil {
		t.Fatalf("decode window.FISC_CONFIG: %v", err)
	}
	if len(cfg.Docs) == 0 {
		t.Fatal("window.FISC_CONFIG carries no docs")
	}
	return cfg.Docs
}

func readPage(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	return string(b)
}

// readerVisible is the page with the FISC_CONFIG script removed.
//
// EVERY "does the page say X" ASSERTION NEEDS IT. index.html ships the whole
// projection as JSON in that one element, so every caveat's text, every
// figure's note and every source's title is in the file whatever the markup
// does. Two tests were measured passing on it -- one with the caveat list
// emptied to <ul></ul>, and the p76 guard after its paragraph moved to another
// page entirely -- so this is a live failure mode rather than a precaution.
func readerVisible(t *testing.T, page string) string {
	t.Helper()
	const open = "<script>window.FISC_CONFIG"
	i := strings.Index(page, open)
	if i < 0 {
		t.Fatal("page carries no window.FISC_CONFIG; if a page legitimately has none, " +
			"say so at the call site rather than searching the whole file")
	}
	rest := page[i:]
	j := strings.Index(rest, "</script>")
	if j < 0 {
		t.Fatal("the FISC_CONFIG script element is unterminated")
	}
	return page[:i] + rest[j+len("</script>"):]
}

// configBlob pulls the window.FISC_CONFIG assignment back out of the page.
func configBlob(t *testing.T, page string) []byte {
	t.Helper()
	const open = "window.FISC_CONFIG = "
	i := strings.Index(page, open)
	if i < 0 {
		t.Fatal("page carries no window.FISC_CONFIG")
	}
	rest := page[i+len(open):]
	j := strings.Index(rest, ";</script>")
	if j < 0 {
		t.Fatal("window.FISC_CONFIG assignment is unterminated")
	}
	return []byte(rest[:j])
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TestTheStylesheetHasOneTextMeasure witnesses ONE DECLARATION, NOT ONE
// RENDERING, and the distinction is the whole of what this test is worth.
//
// Nothing in this tree parses or renders CSS (fisc-6at), so this cannot say
// that --measure applies to any rendered element; that is read in a browser
// after `make site`.
//
// What it CAN say is that the three hand-written measures became one token and
// stay one: 68ch on .lede, 46em on .caveat and main's 1180px inherited by
// everything else were three answers to one question, and the way that comes
// back is a fourth literal added beside the token rather than the token being
// deleted.
//
// THE 1180px ARM IS THE ANTI-VACUITY ONE. Without it a stylesheet that clamped
// main itself to the reading measure -- squeezing the chart, which is the exact
// defect this rule is written to avoid -- satisfies every other arm here.
func TestTheStylesheetHasOneTextMeasure(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	css := string(b)

	if n := strings.Count(css, "--measure:"); n != 1 {
		t.Errorf("style.css declares --measure %d times, want exactly 1; a second "+
			"declaration is a second answer to the question the token replaced", n)
	}
	if n := strings.Count(css, "max-width: var(--measure)"); n < 3 {
		t.Errorf("style.css points %d rules at var(--measure), want at least the lede, "+
			"the caveat and the prose that had no measure at all", n)
	}
	for _, gone := range []string{"max-width: 68ch", "max-width: 46em"} {
		if strings.Contains(css, gone) {
			t.Errorf("style.css still hand-writes %q beside the token that replaced it", gone)
		}
	}
	// THE CONTAINER RULE ITSELF, not the number anywhere in the file. Grepping
	// for "max-width: 1180px" passes over a stylesheet that split this rule in
	// two and clamped `main` to the reading measure while leaving .page-head at
	// 1180px -- which squeezes the page's tiles and tables.
	// Mutation-measured: that split was green.
	const container = "main,\n.page-head,\n.site-nav,\nfooter {\n  max-width: 1180px;\n  margin: 0 auto;"
	// THE PRINT BLOCK OPENS WHAT THE PAGE FOLDS. Declared only -- this cannot
	// say a folded panel prints, which is fisc-yj4w.18 -- but a fold added
	// without it silently drops the caveats from a printed copy, and the print
	// block's own comment about the banner argues that is a defect.
	for _, rule := range []string{"details > *:not(summary)", "details::details-content"} {
		if !strings.Contains(css, rule) {
			t.Errorf("style.css no longer carries %q, so a printed copy loses whatever "+
				"the page folds", rule)
		}
	}
	if !strings.Contains(css, container) {
		t.Error("style.css no longer sets main, .page-head, the top bar and the footer " +
			"together to one centred 1180px frame, so the measure has been applied to the " +
			"page's own column rather than to the prose in it, or the bar and footer run " +
			"past the page's edges")
	}
	// A later rule's margin would undo the frame's auto margins.
	for _, rule := range []string{".site-nav {\n  display: flex;\n  align-items: center;\n  gap: 8px;\n  margin: 0 auto 14px;",
		"footer {\n  margin: 48px auto 0;"} {
		if !strings.Contains(css, rule) {
			t.Errorf("style.css's own rule for the bar or the footer no longer centres it: want %q", rule)
		}
	}
	if !strings.Contains(css, ".prose-page main,\n.prose-page .page-head {\n  max-width: var(--measure);") {
		t.Error("style.css no longer draws a prose page's column at the reading measure")
	}
}

// TestAPageWithNoBuildStampIsRefused: an empty exported_by switches off the
// client's cache gate, since two empty strings match every artifact's
// generated_by.
func TestAPageWithNoBuildStampIsRefused(t *testing.T) {
	_, err := export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views:       spineOnly(),
		Docs:        budgetDocs(),
		PageText:    pageTextFS(),
	})
	if err == nil {
		t.Fatal("got nil error, want a refusal of a page carrying no build stamp")
	}
	for _, want := range []string{schema.Page, "exported_by"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

// TestAnExportNamingNoViewIsRefused: the pages a site has are the caller's
// to state, so an Options naming none is refused rather than given a page
// this package would have to invent.
func TestAnExportNamingNoViewIsRefused(t *testing.T) {
	_, err := export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err == nil || !strings.Contains(err.Error(), "no views to export") {
		t.Fatalf("err = %v, want the refusal of an export naming no view", err)
	}
}

// TestTheGateRunsAfterValidationAndBeforeAnyPageRenders: a caller's refusal
// is never reported over a site validation refuses, whose refusal names the
// declaration, and is reported before a page renders. Rendering an unstamped
// page is refused (TestAPageWithNoBuildStampIsRefused), so a gate whose error
// wins over that one ran first.
func TestTheGateRunsAfterValidationAndBeforeAnyPageRenders(t *testing.T) {
	gated := errors.New("the caller's gate")
	called := false
	gate := func(export.ColumnIndex) error { called = true; return gated }
	_, err := export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		Gate:        gate,
	})
	if called || err == nil || !strings.Contains(err.Error(), "no views to export") {
		t.Errorf("gate called %t, err = %v; want validation's refusal and no gate", called, err)
	}
	_, err = export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views:       spineOnly(),
		Docs:        budgetDocs(),
		PageText:    pageTextFS(),
		Gate:        gate,
	})
	if !errors.Is(err, gated) {
		t.Errorf("err = %v; want the gate's refusal ahead of the unstamped page's", err)
	}
}

// spineOnly is the one-page site most of these tests write: the spine's
// Sankey, with no hint and no year control. data/views.yaml's index
// view is far more than this, and export.Prepare refuses an Options naming no
// view, so the fixture lives here and the tests that want it say so through
// writeSite.
func spineOnly() []export.View {
	return []export.View{{
		Path:       export.IndexPath,
		Nav:        "Budget flows",
		Template:   export.SankeyTemplate,
		Projection: export.PrimaryProjection,
		Overview:   export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}},
	}}
}

// writeSite prepares and writes one site, stamping the build and naming
// spineOnly where the caller did not. The binary cleans the directory between
// Prepare and Write, so a failed render does not destroy the previous site;
// the one-step form lives only here.
func writeSite(o export.Options) ([]string, error) {
	if o.GeneratedBy == "" {
		o.GeneratedBy = "fisc (test)"
	}
	if len(o.Views) == 0 {
		o.Views = spineOnly()
	}
	p, err := export.Prepare(o)
	if err != nil {
		return nil, err
	}
	return p.Write(context.Background())
}

// What style.css DECLARES; nothing here renders CSS (fisc-6at).
//
// Mutation: move the focus block above `.node.derived rect` (equal specificity)
// and a focused derived mark keeps the dashed stroke.
func TestTheChartsFocusRingIsDeclaredOnTheMarkAndAfterTheDerivedRule(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	css := string(b)

	// The global ring stays for everything else focusable; the chart turns it off.
	if !strings.Contains(css, "svg.sankey .node:focus-visible {\n  outline: none;\n}") {
		t.Error("style.css does not turn the global outline off on a chart node; the ring " +
			"follows the <g>'s union box, which holds the label out in the gutter")
	}

	const derived = "svg.sankey .node.derived rect {"
	const focused = "svg.sankey .node:focus-visible rect {"
	iDerived, iFocused := strings.Index(css, derived), strings.Index(css, focused)
	if iDerived < 0 || iFocused < 0 {
		t.Fatalf("style.css is missing one of the two rules this ordering is about: "+
			"derived at %d, focused at %d", iDerived, iFocused)
	}
	if iFocused < iDerived {
		t.Error("svg.sankey .node:focus-visible rect is declared BEFORE " +
			"svg.sankey .node.derived rect; they have equal specificity, so the derived " +
			"rule wins and a focused derived mark draws no indicator")
	}

	// Without this the indicator inherits the derived rule's dashes.
	focusBlock := css[iFocused:]
	if end := strings.Index(focusBlock, "}"); end >= 0 {
		focusBlock = focusBlock[:end]
	}
	if !strings.Contains(focusBlock, "stroke-dasharray: none") {
		t.Error("the focused-mark rule does not reset stroke-dasharray, so a focused " +
			"derived mark draws its focus indicator dashed")
	}

	// A ribbon's stroke and dashes are both published distinctions, and an
	// outline on a <path> rings its whole bounding box, so it takes a drawn echo.
	iLink := strings.Index(css, "svg.sankey .link:focus-visible {")
	if iLink < 0 {
		t.Fatal("style.css states no focus rule for a ribbon")
	}
	linkBlock := css[iLink:]
	if end := strings.Index(linkBlock, "}"); end >= 0 {
		linkBlock = linkBlock[:end]
	}
	if !strings.Contains(linkBlock, "outline: none") {
		t.Error("style.css leaves a focused ribbon taking the global outline, whose box " +
			"is the whole bezier's rectangle rather than the ribbon")
	}
	if !strings.Contains(linkBlock, "drop-shadow") {
		t.Error("style.css gives a focused ribbon no drawn echo, so dropping the outline " +
			"leaves focus on a ribbon invisible")
	}
}
