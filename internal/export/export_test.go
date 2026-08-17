package export_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
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

func writeGolden(t *testing.T) (dir string, written []string) {
	t.Helper()
	dir = t.TempDir()
	written, err := export.Write(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
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
		"data/sankey.json",
		"index.html",
		"style.css",
		"vendor/d3-sankey.LICENSE",
		"vendor/d3-sankey.min.js",
		"vendor/d3.LICENSE",
		"vendor/d3.min.js",
		".nojekyll",
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
			// Only the relative ones are this export's problem; the absolute
			// ones are citations and are checked elsewhere.
			if ref == "" || strings.Contains(ref, "://") || strings.HasPrefix(ref, "#") {
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

func TestWriteCopiesProjectionVerbatim(t *testing.T) {
	dir, _ := writeGolden(t)
	got, err := os.ReadFile(filepath.Join(dir, "data", "sankey.json"))
	if err != nil {
		t.Fatalf("read exported projection: %v", err)
	}
	if diff := cmp.Diff(string(goldenSankey(t)), string(got)); diff != "" {
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

func TestPageRendersCaveatsWithoutJavaScript(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	var doc struct {
		Metadata struct {
			Caveats []string `json:"caveats"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(goldenSankey(t), &doc); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(doc.Metadata.Caveats) == 0 {
		t.Fatal("golden projection has no caveats to render")
	}
	for _, caveat := range doc.Metadata.Caveats {
		// html/template escapes as it renders, so compare against the escaped
		// form rather than asserting on a prefix that happens to be plain.
		head := caveat
		if i := strings.IndexAny(head, "&<>'\"$("); i > 20 {
			head = head[:i]
		}
		if !strings.Contains(page, head) {
			t.Errorf("caveat missing from the page: %q", head)
		}
	}
}

func TestPageCitesThePDFPageAndNeverTheLFSPointer(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	for _, want := range []string{
		"https://www.livermoreca.gov/home/showpublisheddocument/12813#page=66",
		"https://www.livermoreca.gov/home/showpublisheddocument/12813#page=67",
		"https://github.com/jcrussell/livermore-budget/blob/main/data/extracted/livermore-budget-fy2026-2027/pages/p0066.txt",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not cite %q", want)
		}
	}
	// raw.githubusercontent.com serves the LFS pointer instead of the file, so
	// a citation pointing there resolves to "oid sha256:..." and nothing else.
	if strings.Contains(page, "raw.githubusercontent.com") {
		t.Error("page links to raw.githubusercontent.com, which serves LFS pointer text rather than the document")
	}
}

func TestPageConfigCarriesTheProjectionMetadataVerbatim(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	cfg := configBlob(t, page)
	var got struct {
		SchemaVersion int               `json:"schema_version"`
		Primary       string            `json:"primary"`
		Projections   map[string]string `json:"projections"`
		Metadata      json.RawMessage   `json:"metadata"`
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
	if diff := cmp.Diff(map[string]string{"sankey": "data/sankey.json"}, got.Projections); diff != "" {
		t.Errorf("projection paths (-want +got):\n%s", diff)
	}
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
	if want := "https://github.com/jcrussell/livermore-budget/blob/main/data/extracted/livermore-budget-fy2026-2027/pages/"; doc.PageTextBase != want {
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
			"caveats":           []string{`</script><script>alert("pwned")</script>`},
			"sources":           []any{},
		},
		"nodes": []any{},
		"links": []any{},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	dir := t.TempDir()
	if _, err := export.Write(export.Options{
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
	_, err := export.Write(export.Options{
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
			if _, err := export.Write(opts); err == nil {
				t.Fatal("got nil error, want a refusal")
			}
		})
	}
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
	_, err := export.Write(export.Options{
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

func readPage(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	return string(b)
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
