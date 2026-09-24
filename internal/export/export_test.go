package export_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
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
		// ONE DOCUMENT PER PUBLISHED COLUMN AND NOT ALSO ONE PER SCHEDULE.
		// data/sankey.json was here: sankey folds into this column, so
		// shipping it too was the same figures at two paths, and the second
		// was fetched by nothing.
		"fy2026-adopted.json",
		"index.html",
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

// TestWriteCopiesAColumnlessProjectionVerbatim is the verbatim-copy contract,
// narrowed to where it still holds and said so.
//
// IT USED TO BE ASKED OF sankey, and that is no longer a copy: a document
// stating a fiscal year and a basis folds into its column and is re-encoded
// there. What replaces the guarantee for those is stronger and is held
// elsewhere -- encodeColumn refuses bytes that do not match
// schema/column.schema.json, and pkg/cmd/export/column_test.go holds every
// column's figures to facts/facts.jsonl.
//
// What has no column and no schema is revenue-trends and the two balance
// documents, which carry a series and state no year. Those ship as themselves
// and must ship BYTE FOR BYTE, because nothing else looks at them at all.
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

func TestPageShowsTheHeadlineAndTheErrorItAvoids(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	for _, want := range []string{
		"$254,095,412", // what the city spends
		"$313,708,146", // the naive column total, published so the page can argue with it
		"$59,612,734",  // out
		"$21,525,997",  // in
		"$38,086,737",  // the unmatched residual
		"FY 2025-26",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
}

// TestTheHeroLeadsAndTheRestOfTheTileRowIsDisclosed pins WHICH SIDE of the
// #figures-view disclosure each headline figure is on, which the test above
// cannot see: it asks only whether the page contains a figure, and a page that
// tiled all seven above the chart and a page that hid all seven behind a
// summary both satisfy it.
//
// The slice is taken between the disclosure's own markers and every marker is
// Fatalf-guarded. Without that guard a renamed marker yields an empty slice,
// and "the naive total is not above the chart" is then true of nothing.
func TestTheHeroLeadsAndTheRestOfTheTileRowIsDisclosed(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	// THE MARKER STOPS BEFORE THE CLOSING ANGLE BRACKET, and that is not
	// tidiness. Spelled with the `>` on the end, an `open` attribute makes this
	// Index miss the element altogether -- so the Fatalf below fires with
	// "page renders no ..." and the ships-closed arm underneath can never be
	// reached, let alone fail. Measured by mutation: the arm was dead.
	const tag = `<details class="apparatus" id="figures-view"`
	start := strings.Index(page, tag)
	if start < 0 {
		t.Fatalf("page renders no %s", tag)
	}
	shut := strings.Index(page[start:], ">")
	if shut < 0 {
		t.Fatalf("the %s tag is unterminated", tag)
	}
	end := strings.Index(page[start:], "</details>")
	if end < 0 {
		t.Fatalf("the %s disclosure is never closed", tag)
	}
	disclosed := page[start : start+end]
	above := page[:start]

	// The disclosure ships closed. `<details ... open>` and `<details ...>` are
	// one attribute apart and render identically until a reader arrives.
	if strings.Contains(page[start:start+shut], " open") {
		t.Errorf("the tile row disclosure ships open")
	}
	if !strings.Contains(above, "$254,095,412") {
		t.Errorf("the hero figure is not above the disclosure")
	}
	if strings.Contains(disclosed, "$254,095,412") {
		t.Errorf("the hero figure is inside the disclosure as well as above it")
	}
	if !strings.Contains(disclosed, "$313,708,146") {
		t.Errorf("the naive column total is not inside the disclosure")
	}
	if strings.Contains(above, "$313,708,146") {
		t.Errorf("the naive column total is still above the chart")
	}
}

// TestTheFigureLeadsAndTheApparatusFollows pins the reading order the owner
// asked for: one number, the chart, then everything that explains them.
//
// EVERY MARKER IS Fatalf-GUARDED ON -1, because strings.Index returns -1 for
// absent and -1 is less than every offset -- so a renamed marker turns each
// comparison below into a true statement about a string that is not there.
func TestTheFigureLeadsAndTheApparatusFollows(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	// Ordered as the reader meets them. The prose reading of the chart is in
	// the list because moving it back above the card is the exact regression
	// this test exists to catch, and the three structural markers around it
	// would not notice.
	markers := []string{
		`id="hero"`,
		"<h2>The flow</h2>",
		`id="chart"`,
		"Revenue categories on the left",
		`class="apparatus"`,
	}
	at := make([]int, len(markers))
	for i, m := range markers {
		at[i] = strings.Index(page, m)
		if at[i] < 0 {
			t.Fatalf("page renders no %s", m)
		}
	}
	for i := 1; i < len(markers); i++ {
		if at[i-1] >= at[i] {
			t.Errorf("%s is at %d, which is not before %s at %d",
				markers[i-1], at[i-1], markers[i], at[i])
		}
	}
}

func TestPageRendersCaveatsWithoutJavaScript(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	var doc struct {
		Metadata struct {
			Caveats []struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
				Text    string `json:"text"`
			} `json:"caveats"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(goldenSankey(t), &doc); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(doc.Metadata.Caveats) == 0 {
		t.Fatal("golden projection has no caveats to render")
	}
	// THE CONFIG BLOB IS REMOVED BEFORE ANYTHING IS SEARCHED, and without that
	// this test is vacuous. index.html ships the whole projection as JSON in
	// window.FISC_CONFIG, so every caveat's text is in the file whatever the
	// markup does -- measured: replace the entire <ul class="caveats"> range
	// with an empty <ul> and the substring form of this test still PASSED.
	// That is the same defect as the p76 guard's, one file over, and it is why
	// "is the string in the page" is never the right question here.
	visible := readerVisible(t, page)

	for _, caveat := range doc.Metadata.Caveats {
		// THE SUMMARY, because that is what this page renders without
		// JavaScript. It used to be the text, which was right when the page
		// printed the paragraph and became unfalsifiable when it stopped:
		// the text is still in the file, in the blob just removed.
		//
		// ESCAPED AND COMPARED WHOLE. This inherited a truncate-at-the-first-
		// escapable-character rule written when the subject was the text, and
		// over summaries it was doing two wrong things: cutting
		// transfer-legs-unpaired's assertion down to a 24-character prefix, and
		// -- for any summary whose first such character falls before index 21 --
		// comparing an unescaped string against escaped HTML, which fails for a
		// reason that has nothing to do with the page. Escaping instead of
		// truncating removes both, and asserts the whole sentence.
		want := template.HTMLEscapeString(caveat.Summary)
		if !strings.Contains(visible, want) {
			t.Errorf("caveat %q's summary is missing from index.html: %q", caveat.ID, want)
		}
		// AND THE PARAGRAPH IS NOT PRINTED HERE. Showing a summary is only
		// honest as a pointer; a page that showed both would be the wall this
		// change removed, with a link added to it.
		//
		// ESCAPED BEFORE COMPARING. html/template rewrites ' " & < > as it
		// renders, so a raw comparison cannot fire for most of the corpus.
		// Measured against the real export: on caveats.html, which DOES print
		// every text, a raw Contains is False for transfer-legs-unpaired and
		// internal-service-is-outside-the-external-headline and True only for
		// the two texts with no escapable character.
		//
		// THIS ARM IS THE SECOND LINE AND NOT THE FIRST. index.html cannot
		// print the text today whatever the template does, because caveatRef
		// carries no such field -- so what it really guards is somebody putting
		// one back. The structural form of that claim is
		// TestTheSchemaStatesWhatThePageConfigCarries, which walks caveatRef
		// against schema/page.schema.json's additionalProperties: false and so
		// fails the moment the field exists rather than when a template happens
		// to render it.
		if len(caveat.Text) > 80 && strings.Contains(visible, template.HTMLEscapeString(caveat.Text)) {
			t.Errorf("caveat %q's full text is on index.html; the summary is meant to "+
				"stand in for it, not to precede it", caveat.ID)
		}
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
	if strings.Contains(page, "github.com") {
		t.Error("the page still cites github.com; the shipped page text should have replaced it")
	}

	// Every page-text citation has to be a file in the output. Resolve them the
	// way a browser would — relative to the page — rather than trusting the
	// string.
	for _, doc := range configDocs(t, page) {
		if strings.Contains(doc.PageTextBase, "://") {
			t.Errorf("got page_text_base %q, want a path relative to the site", doc.PageTextBase)
			continue
		}
		for _, page := range []int{66, 67} {
			ref := fmt.Sprintf("%sp%04d.txt", doc.PageTextBase, page)
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
		SchemaVersion int             `json:"schema_version"`
		Primary       string          `json:"primary"`
		Metadata      json.RawMessage `json:"metadata"`
		Docs          map[string]struct {
			PDFURL       string `json:"pdf_url"`
			PageTextBase string `json:"page_text_base"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(cfg, &got); err != nil {
		t.Fatalf("decode window.FISC_CONFIG: %v", err)
	}

	if got.Primary != "sankey" {
		t.Errorf("got primary %q, want %q", got.Primary, "sankey")
	}
	// NO PROJECTIONS MAP IS ASSERTED, because the config carries none. A year
	// names its document by its column and a step names its schedule, so a
	// stem -> path map on the wire would be a third name for the same file
	// and a second thing to keep in step.
	if got.SchemaVersion != 1 {
		t.Errorf("got schema_version %d, want 1", got.SchemaVersion)
	}

	// The page must not reassemble metadata the projection already published:
	// a second copy of the fiscal year or the headline is a second thing to
	// keep in step. Compare the whole block, semantically.
	var wantMeta, gotMeta any
	var golden struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(goldenSankey(t), &golden); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if err := json.Unmarshal(golden.Metadata, &wantMeta); err != nil {
		t.Fatalf("decode golden metadata: %v", err)
	}
	if err := json.Unmarshal(got.Metadata, &gotMeta); err != nil {
		t.Fatalf("decode config metadata: %v", err)
	}
	if diff := cmp.Diff(wantMeta, gotMeta); diff != "" {
		t.Errorf("config metadata differs from the projection's (-want +got):\n%s", diff)
	}

	doc, ok := got.Docs["livermore-budget-fy2026-2027"]
	if !ok {
		t.Fatal("config carries no doc entry for the budget book")
	}
	if want := "https://www.livermoreca.gov/home/showpublisheddocument/12813"; doc.PDFURL != want {
		t.Errorf("got pdf_url %q, want %q", doc.PDFURL, want)
	}
	// The base moved off github.com with fisc-ze7: the site ships the cited
	// pages, so the client composes a same-origin path and the provenance
	// resolves with no network access to a forge.
	if want := export.LocalPageTextBase(budgetDocID); doc.PageTextBase != want {
		t.Errorf("got page_text_base %q, want %q", doc.PageTextBase, want)
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
		Projections: map[string][]byte{"treemap": []byte(`{}`)},
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

// TestPrimaryProjectionIsPinnedToTheProducer keeps the packager's idea of which
// document drives the page in step with the producer's.
//
// internal/export does not import internal/project, deliberately, so nothing
// compiles the two names against each other. They stopped being decorative when
// each projection began declaring its own slices: published-projection-built now
// asserts that the projection NAMED here was built at the published triple, so a
// drift between these two constants would make that check green over a document
// the page does not render.
func TestPrimaryProjectionIsPinnedToTheProducer(t *testing.T) {
	if export.PrimaryProjection != project.PublishedProjection {
		t.Errorf("export.PrimaryProjection is %q but project.PublishedProjection is %q; "+
			"internal/export does not import internal/project, so this test is the only "+
			"thing keeping the page's document in step with the one verify checks -- "+
			"move both", export.PrimaryProjection, project.PublishedProjection)
	}
}

// The packager's schema constant is a deliberate second copy of the producer's:
// internal/export consumes projections as bytes and does not import
// internal/project, so nothing but an assertion holds the two together. This
// test is that assertion, and the reason the duplication is safe rather than
// merely tolerated. Without it the constants drift the first time
// project.SchemaVersion moves, and buildSankeyPage then accepts exactly the
// document its gate exists to refuse — silently, because a schema bump changes
// what the graph means and not what its keys are called.
//
// When project.SchemaVersion moves, move export.SchemaVersion and
// SCHEMA_VERSION in site/app.js in the same change.
func TestSchemaVersionIsPinnedToTheProducer(t *testing.T) {
	if export.SchemaVersion != project.SchemaVersion {
		t.Errorf("export.SchemaVersion is %d but project.SchemaVersion is %d; "+
			"internal/export does not import internal/project, so this test is the only "+
			"thing keeping the packager's gate in step with the producer's stamp — move both",
			export.SchemaVersion, project.SchemaVersion)
	}
}

// The client's copy is the one nothing compiles against, so it is the one that
// would drift in silence. Read it out of the same embedded asset tree the
// packager ships and pin the literal.
func TestClientSchemaVersionIsPinnedToTheProducer(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	want := fmt.Sprintf("const SCHEMA_VERSION = %d;", project.SchemaVersion)
	if !bytes.Contains(b, []byte(want)) {
		t.Errorf("site/app.js does not declare %q; the browser gate has drifted from "+
			"project.SchemaVersion %d, so the page would draw a document it does not understand",
			want, project.SchemaVersion)
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
		"index.html.tmpl":  {Data: []byte(`<!doctype html><title>{{.Title}}</title>`)},
		"style.css":        {Data: []byte(`body{}`)},
		".nojekyll":        {Data: []byte{}},
		"vendor/d3.min.js": {Data: []byte(`/* d3 */`)},
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

// configDocs decodes window.FISC_CONFIG's docs block.
func configDocs(t *testing.T, page string) map[string]struct {
	PDFURL       string `json:"pdf_url"`
	PageTextBase string `json:"page_text_base"`
} {
	t.Helper()
	var cfg struct {
		Docs map[string]struct {
			PDFURL       string `json:"pdf_url"`
			PageTextBase string `json:"page_text_base"`
		} `json:"docs"`
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

// TestTheDisabledYearToggleKeepsItsSelectionUnderTheCursor is fisc-36r, pinned
// against the shipped stylesheet.
//
// `.year-toggle:disabled label:hover { background: none }` has specificity
// (0,3,1) and outranked `.year-toggle input:checked + label` at (0,2,2). The
// fieldset SHIPS DISABLED — the template renders it that way and app.js removes
// the attribute as its enhancement — so with JavaScript off, or during the
// opening fetch, hovering the currently selected year erased the fill that says
// it is selected, leaving only the box-shadow ring that the checked rule's own
// comment says it deliberately does not rely on alone.
//
// WHY THIS IS A STRING ASSERTION AND NOT A SPECIFICITY CALCULATOR. Computing
// specificity in Go was the first plan and is the wrong shape: it is what
// tools/jscheck refuses to do for selectors one layer up ("a selector engine
// here would be a second implementation of a thing the browser already has"),
// and it would model only ONE axis of the cascade — source order, !important,
// @layer and whether both rules match the same element are the others — so
// asserting (0,2,2) > (0,3,1) would prove the arithmetic and not the outcome.
// Parsing a thousand lines of hand-formatted CSS with the standard library
// alone would also redden on a reformat.
//
// This costs five lines and fails exactly when the bug comes back, which is the
// property that matters for a fix that is one careless edit from returning.
// TestTheDisabledYearToggleDoesNotDimItsOwnSelectionRing is fisc-b7k's third
// defect, pinned in the same shape and for the same reason as its sibling below.
//
// `.year-toggle:disabled { opacity: 0.55 }` composited the WHOLE fieldset, and
// the fieldset ships disabled — so that was a no-JS reader's state for the whole
// visit, not a flicker. It silently invalidated the measured contrast table 40
// lines further down, whose ratios (ring vs fill, 7.11:1 light / 8.26:1 dark
// against WCAG 1.4.11's 3:1) are for OPAQUE elements: at 0.55 the ring falls to
// roughly 2.51:1 in light, below the threshold. That ring is not one signal
// among several — the same block establishes that the fill is identical between
// checked and unchecked (its own 1.000:1 finding), so for a no-JS reader in
// light mode it is the only thing left saying which year is on screen.
//
// The fix dims the legend and the UNCHECKED pills instead, so the selected one
// stays opaque and every number in that table remains a true statement about
// what ships. Deliberately NOT a recomputed ratio: nothing in this tree parses
// or renders CSS (fisc-6at), so a second hand-computed figure would double the
// unpinned surface rather than close it.
func TestTheDisabledYearToggleDoesNotDimItsOwnSelectionRing(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	css := string(b)

	// As a RULE, with the brace: the selector also appears in prose above its
	// replacement, which is where the reasoning lives.
	if strings.Contains(css, ".year-toggle:disabled {") {
		t.Error("style.css composites the whole disabled fieldset again; that drops the " +
			"selection ring below WCAG 1.4.11's 3:1 for every reader without JavaScript, " +
			"and falsifies the measured contrast table in this same file (fisc-b7k)")
	}
	if !strings.Contains(css, ".year-toggle:disabled input:not(:checked) + label {") {
		t.Error("style.css no longer dims the unchecked pills, so the disabled group " +
			"either looks live or was rewritten; if rewritten, this test must name " +
			"whatever now carries the not-yet-live signal")
	}
	// AND NOT THE LEGEND. `.year-toggle legend` is a screen-reader caption --
	// absolutely positioned, 1x1, clip-path inset(50%) -- so dimming it changes
	// nothing anyone can see, so pinning that selector would assert a signal the
	// page does not carry.
	if strings.Contains(css, ".year-toggle:disabled legend") {
		t.Error("style.css dims the year toggle's legend, which is visually hidden; " +
			"that is an inert rule dressed as a state indicator")
	}
}

func TestTheDisabledYearToggleKeepsItsSelectionUnderTheCursor(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	css := string(b)

	// The over-reaching rule, as a RULE — the string also appears in the comment
	// above its replacement, which is where the reasoning lives.
	if strings.Contains(css, ".year-toggle:disabled label:hover {") {
		t.Error("style.css suppresses the hover fill on EVERY disabled pill, including " +
			"the checked one; with JavaScript off the selected year loses its fill " +
			"under the cursor (fisc-36r)")
	}
	if !strings.Contains(css, ".year-toggle:disabled input:not(:checked) + label:hover {") {
		t.Error("style.css no longer narrows the disabled hover suppression to the " +
			"unchecked pills; either the fix was reverted or it was rewritten, and if " +
			"rewritten this test needs to name whatever replaced it")
	}
	// The rule the narrowing exists to protect must still be there, or the test
	// above passes over a stylesheet that marks no selection at all.
	if !strings.Contains(css, ".year-toggle input:checked + label {") {
		t.Error("style.css no longer gives the checked year its own treatment")
	}
}

// TestTheStylesheetHasOneTextMeasure witnesses ONE DECLARATION, NOT ONE
// RENDERING, and the distinction is the whole of what this test is worth.
//
// Nothing in this tree parses or renders CSS (fisc-6at): tools/jscheck reads
// style.css to harvest custom-property names and to read .chart-wrap's width
// allowance as text, and the two tests below this one match selector strings.
// So this cannot say that --measure applies to any element a template renders,
// that a line comes out at 68 characters, or that the wide tables stayed at the
// container width. Those are read in a browser after `make site` and nowhere
// else.
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
	// 1180px -- which squeezes the page's tiles and tables, and squeezed the
	// chart too until .chart-wrap took a width of its own (fisc-5e2b).
	// Mutation-measured: that split was green.
	const container = "main,\n.page-head {\n  max-width: 1180px;"
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
		t.Error("style.css no longer sets main and .page-head together to 1180px, so the " +
			"measure has been applied to the page's own column rather than to the " +
			"prose in it")
	}
}

// writeSite renders o into o.Dir in one step, which is what most of these
// tests want and what the binary deliberately does not do: pkg/cmd/export
// takes Prepare, then cleans the directory, then plan.Write, so a render that
// fails does not destroy the previous site first. Keeping the one-step form
// here rather than in the package means that ordering has no shortcut around
// it in production.
// TestAPageWithNoBuildStampIsRefused is the one arm [writeSite]'s default is
// written around.
//
// AN EMPTY exported_by IS A GATE SWITCHED OFF, not a cosmetic gap. site/app.js
// refuses a fetched artifact whose generated_by disagrees with
// CONFIG.exported_by, and that comparison is the only thing left that catches a
// column a reader cached from before the last deploy -- the site publishes no
// cache-busting. Two empty strings pass it for every file, forever.
func TestAPageWithNoBuildStampIsRefused(t *testing.T) {
	_, err := export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
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

// writeSite prepares and writes one site, stamping the build where the caller
// did not.
//
// THE STAMP IS FILLED IN AND THE TEST THAT IS ABOUT IT PLANTS ITS OWN. An empty
// GeneratedBy is refused by page.schema.json, because the client's one surviving
// cache gate compares a fetched artifact's generated_by with CONFIG.exported_by
// and two empty strings pass it trivially -- so a page with no stamp is a page
// with that gate switched off. Every test above is about something else and the
// real command has never produced one (generatedBy() is "fisc " plus the build),
// so defaulting here keeps those arms about their own subject.
// TestAPageWithNoBuildStampIsRefused is the one that asserts the refusal, and it
// calls Prepare directly rather than coming through here.
func writeSite(o export.Options) ([]string, error) {
	if o.GeneratedBy == "" {
		o.GeneratedBy = "fisc (test)"
	}
	p, err := export.Prepare(o)
	if err != nil {
		return nil, err
	}
	return p.Write()
}

// What style.css DECLARES; nothing here renders CSS (fisc-6at).
//
// Mutation: move the focus block above `.node.derived rect` -- equal
// specificity, so the later wins -- and a focused derived mark keeps the dashed
// stroke with every other test green. Dropping the dasharray reset is the same
// defect one property in.
func TestTheChartsFocusRingIsDeclaredOnTheMarkAndAfterTheDerivedRule(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	css := string(b)

	// The global ring is what put a box round the whole <g>. It has to stay for
	// every other focusable thing on the page, so the chart turns it off rather
	// than the file dropping it.
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

	// Without this the indicator inherits the derived rule's dashes and reads as
	// a different kind of mark rather than as focus.
	focusBlock := css[iFocused:]
	if end := strings.Index(focusBlock, "}"); end >= 0 {
		focusBlock = focusBlock[:end]
	}
	if !strings.Contains(focusBlock, "stroke-dasharray: none") {
		t.Error("the focused-mark rule does not reset stroke-dasharray, so a focused " +
			"derived mark draws its focus indicator dashed")
	}

	// A ribbon cannot take the node's fix -- its stroke and its dashes are both
	// published distinctions -- and it cannot keep the box either: a <path>'s
	// bounding box is the whole bezier's rectangle, so any outline on one is a
	// ring around mostly empty space. It takes neither, and a drawn echo
	// instead.
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
