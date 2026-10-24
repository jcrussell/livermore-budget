package export_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
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
	"github.com/jcrussell/livermore-budget/internal/hint"
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

// TestThePlanWritesTheIndexLast holds Prepare's write order: the pages after
// every asset they fetch, with index.html, the completed-site sentinel, last
// of all. The index is declared first here so that writing the pages in
// declaration order fails. The manifest is not in the plan: Write writes it
// around the plan, listing it.
//
// Mutation: add the pages in declaration order without holding IndexPath back
// and this goes red with "the last planned file is \"caveats.html\"".
// Adding MarkerName to the plan goes red with "want the manifest left to
// Write".
func TestThePlanWritesTheIndexLast(t *testing.T) {
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
	if len(paths) < 2 {
		t.Fatalf("planned %q, want assets and pages", paths)
	}
	if slices.Contains(paths, export.MarkerName) {
		t.Errorf("planned %q, want the manifest left to Write, which lists the plan in it", export.MarkerName)
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

// cancelFromCall is a context whose Err() reports a cancellation from its
// nth call on.
type cancelFromCall struct {
	context.Context
	n, calls int
}

func (c *cancelFromCall) Err() error {
	c.calls++
	if c.calls >= c.n {
		return context.Canceled
	}
	return nil
}

// Write stops on a cancelled context: one already cancelled creates nothing,
// not even the output directory, and one cancelled once the directory exists
// writes no file into it.
//
// Mutation: drop the ctx check before MkdirAll and the first case goes red
// with "the output directory was created after cancellation"; drop the
// per-file check and the second goes red with "wrote [...] after
// cancellation".
func TestWriteStopsOnACancelledContext(t *testing.T) {
	type writer interface {
		Write(context.Context, bool) ([]string, export.Removal, error)
	}
	prepare := func(t *testing.T) (writer, string) {
		t.Helper()
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
		return p, dir
	}

	t.Run("already cancelled", func(t *testing.T) {
		p, dir := prepare(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		written, _, err := p.Write(ctx, true)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got error %v, want it to wrap context.Canceled", err)
		}
		if len(written) > 0 {
			t.Errorf("wrote %q after cancellation", written)
		}
		if _, serr := os.Stat(dir); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("the output directory was created after cancellation (stat: %v)", serr)
		}
	})

	t.Run("cancelled once the directory exists", func(t *testing.T) {
		p, dir := prepare(t)
		written, _, err := p.Write(&cancelFromCall{Context: context.Background(), n: 2}, true)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got error %v, want it to wrap context.Canceled", err)
		}
		entries, rerr := os.ReadDir(dir)
		if rerr != nil {
			t.Fatalf("the output directory was not created before the cancellation: %v", rerr)
		}
		if len(written) > 0 || len(entries) > 0 {
			t.Errorf("wrote %q (%d entries on disk) after cancellation", written, len(entries))
		}
	})
}

// A write over an earlier export that stops partway leaves none of that
// export's pages beside the mix of old and new data. Without --clean, Err is
// asked before the directory, before the manifest, and before each file: the
// fourth call stops the write after its first file.
//
// Mutation: drop Write's removal of the earlier export's pages and this goes
// red naming index.html.
func TestAWriteStoppedOverAnEarlierExportLeavesNoPage(t *testing.T) {
	dir, written := writeGolden(t)
	pages := slices.DeleteFunc(slices.Clone(written), func(rel string) bool { return !strings.HasSuffix(rel, ".html") })
	if !slices.Contains(pages, export.IndexPath) {
		t.Fatalf("the earlier export wrote %q, want index.html among them", written)
	}
	p, err := export.Prepare(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views:       spineOnly(),
		Docs:        budgetDocs(),
		PageText:    pageTextFS(),
		GeneratedBy: "fisc test",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, _, err := p.Write(&cancelFromCall{Context: context.Background(), n: 4}, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want it to wrap context.Canceled", err)
	}
	for _, rel := range pages {
		if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); serr == nil {
			t.Errorf("%s, the earlier export's page, is still served beside a partial write", rel)
		}
	}
	if _, _, err := p.Write(context.Background(), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, rel := range pages {
		if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); serr != nil {
			t.Errorf("a completed re-run did not write %s: %v", rel, serr)
		}
	}
}

// manifestOf is the listing dir's manifest holds, failing the test where it
// holds none a later export would accept.
func manifestOf(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, export.MarkerName))
	if err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	if err := export.RefuseForeignDir(dir); err != nil {
		t.Fatalf("the manifest %q is not one a later export accepts: %v", b, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	return lines[1:]
}

// seedExport makes dir look like an earlier export's output: a manifest
// listing listed, and a file at each of present.
func seedExport(t *testing.T, dir string, listed, present []string) {
	t.Helper()
	for _, rel := range present {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("earlier export\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	body := export.ManifestHeader + "\n" + strings.Join(listed, "\n")
	if len(listed) > 0 {
		body += "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, export.MarkerName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The manifest a finished export leaves lists exactly the files it wrote,
// itself excluded, in the order a later export reads them.
//
// Mutation: write the final manifest from the previous listing only and this
// goes red with a diff naming every planned path.
func TestTheManifestListsWhatTheExportWrote(t *testing.T) {
	dir, written := writeGolden(t)
	want := slices.DeleteFunc(slices.Clone(written), func(rel string) bool { return rel == export.MarkerName })
	if len(want) == len(written) {
		t.Fatalf("wrote %q, want the manifest among them", written)
	}
	if diff := cmp.Diff(want, manifestOf(t, dir)); diff != "" {
		t.Errorf("manifest (-written +listed):\n%s", diff)
	}
}

// A write stopped after its first file leaves a manifest listing everything
// the previous one did and everything this export plans, so whatever is on
// disk is listed whichever export put it there.
//
// Mutation: write the first manifest from the plan alone and this goes red
// with "old/gone.txt" missing from the listing.
func TestAStoppedWriteLeavesAManifestOfBothExports(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	seedExport(t, dir, []string{"old/gone.txt"}, []string{"old/gone.txt"})
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
	planned := export.PlannedPaths(p)
	// Err is asked before the directory, before the manifest, before the
	// clean's removal, and before each file: the fifth call stops the write
	// after its first file.
	_, _, err = p.Write(&cancelFromCall{Context: context.Background(), n: 5}, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want it to wrap context.Canceled", err)
	}
	if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(planned[0]))); serr != nil {
		t.Fatalf("the first planned file was not written: %v", serr)
	}
	if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(planned[1]))); serr == nil {
		t.Fatal("the second planned file was written, so the write did not stop where this test needs")
	}
	want := append(slices.Clone(planned), "old/gone.txt")
	slices.Sort(want)
	if diff := cmp.Diff(want, manifestOf(t, dir)); diff != "" {
		t.Errorf("manifest (-earlier and planned +listed):\n%s", diff)
	}
}

// --clean removes before it writes only what stands in the way of a planned
// path, and the rest of the earlier export's leftovers after the last file,
// so a served site keeps what it links to while the write runs: a run
// stopped after its first file has removed the leftover in the way and left
// the other on disk and in the manifest, and a full run removes both.
//
// Mutation: remove every leftover before the write loop and this goes red
// with "old/gone.txt was removed before the write finished".
func TestCleanRemovesOnlyWhatIsInTheWayBeforeItWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	seedExport(t, dir, []string{"facts", "old/gone.txt"}, []string{"facts", "old/gone.txt"})
	p := planFiles(t, dir, map[string][]byte{"facts/d/p0001.jsonl": []byte("now\n")})
	// Err is asked before the directory, before the manifest, before the
	// removal of what is in the way, and before each file: the fifth call
	// stops the write after its first file.
	_, _, err := p.Write(&cancelFromCall{Context: context.Background(), n: 5}, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want it to wrap context.Canceled", err)
	}
	if info, serr := os.Lstat(filepath.Join(dir, "facts")); serr == nil && !info.IsDir() {
		t.Error("facts, in the way of facts/d/p0001.jsonl, was not removed before the write")
	}
	if _, serr := os.Stat(filepath.Join(dir, "old", "gone.txt")); serr != nil {
		t.Errorf("old/gone.txt was removed before the write finished: %v", serr)
	}
	if !slices.Contains(manifestOf(t, dir), "old/gone.txt") {
		t.Error("the manifest no longer lists old/gone.txt, which is still on disk")
	}
	_, removed, err := p.Write(context.Background(), true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if diff := cmp.Diff(export.Removal{Files: []string{"old/gone.txt"}, Dirs: []string{"old"}}, removed); diff != "" {
		t.Errorf("removed (-want +got):\n%s", diff)
	}
	if _, serr := os.Stat(filepath.Join(dir, "old")); serr == nil {
		t.Error("old survived a full --clean")
	}
}

// A write without --clean removes nothing, and its manifest keeps listing what
// the earlier export wrote and is still there, so a later --clean can still
// remove it. A listed path that is gone, or is now a directory, drops out.
//
// Mutation: write the final manifest from the plan alone and this goes red
// with "old/gone.txt" missing from the listing; carry forward every earlier
// entry and it goes red listing "vanished.txt".
func TestAWriteKeepsListingWhatTheEarlierExportLeft(t *testing.T) {
	dir := t.TempDir()
	seedExport(t, dir, []string{"now-a-dir", "old/gone.txt", "vanished.txt"}, []string{"now-a-dir/inside.txt", "old/gone.txt"})
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
	if _, _, err := p.Write(context.Background(), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old", "gone.txt")); err != nil {
		t.Errorf("a write without --clean removed a file: %v", err)
	}
	want := append(export.PlannedPaths(p), "old/gone.txt")
	slices.Sort(want)
	if diff := cmp.Diff(want, manifestOf(t, dir)); diff != "" {
		t.Errorf("manifest (-want +listed):\n%s", diff)
	}
}

// --clean removes what the earlier export's manifest lists and this one does
// not write, and the directories that leaves empty, and nothing else: not a
// file the manifest never listed, wherever it sits, not a directory still
// holding one, not a file reached through a symlinked directory, and not the
// output directory itself.
//
// Mutation: make leftover return every earlier entry the plan does not
// write, unchecked, and this goes red with "outside/x.txt was removed";
// remove each leftover's parent directories with RemoveAll rather than only
// when empty and it goes red with "old/mixed/mine.txt was removed"; skip the
// pruning and it goes red with "old/sub was left behind empty"; leave the
// directories out of Write's Removal and it goes red with a removed diff.
func TestCleanRemovesOnlyWhatTheEarlierExportListed(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "out")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "x.txt"), []byte("not the export's"), 0o600); err != nil {
		t.Fatal(err)
	}
	unlisted := []string{"notes.md", "sub/keep.go", ".git/HEAD", "old/mixed/mine.txt"}
	listed := []string{"link/x.txt", "old/mixed/gone.txt", "old/sub/gone.txt", "top.txt"}
	seedExport(t, dir, listed, append(slices.Clone(unlisted), "old/mixed/gone.txt", "old/sub/gone.txt", "top.txt"))
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
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
	_, removed, err := p.Write(context.Background(), true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := export.Removal{Files: []string{"old/mixed/gone.txt", "old/sub/gone.txt", "top.txt"}, Dirs: []string{"old/sub"}}
	if diff := cmp.Diff(want, removed); diff != "" {
		t.Errorf("removed (-want +got):\n%s", diff)
	}
	for _, rel := range unlisted {
		if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); serr != nil {
			t.Errorf("%s was removed: %v", rel, serr)
		}
	}
	if _, serr := os.Stat(filepath.Join(outside, "x.txt")); serr != nil {
		t.Errorf("outside/x.txt was removed: %v", serr)
	}
	for _, rel := range []string{"old/mixed/gone.txt", "old/sub/gone.txt", "top.txt"} {
		if _, serr := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel))); serr == nil {
			t.Errorf("%s survived --clean", rel)
		}
	}
	if _, serr := os.Stat(filepath.Join(dir, "old", "sub")); serr == nil {
		t.Error("old/sub was left behind empty")
	}
	planned := export.PlannedPaths(p)
	slices.Sort(planned)
	if diff := cmp.Diff(planned, manifestOf(t, dir)); diff != "" {
		t.Errorf("manifest (-planned +listed):\n%s", diff)
	}
}

// planFiles prepares the golden site into dir, shipping files beside it.
func planFiles(t *testing.T, dir string, files map[string][]byte) interface {
	Write(context.Context, bool) ([]string, export.Removal, error)
	Obsolete() ([]string, []string, error)
} {
	t.Helper()
	p, err := export.Prepare(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views:       spineOnly(),
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		Files:       files,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return p
}

// A path the earlier export wrote as a file and this one needs as a
// directory, or the reverse, converges under --clean, which removes the
// earlier export's leftovers before it writes anything. Without --clean the
// write is refused before anything is written, naming --clean, since only
// --clean may remove what is in the way.
//
// Mutation: remove the leftovers after the write loop rather than before it
// and both "with --clean" cases go red with "not a directory" and "is a
// directory"; drop the refusal without clean and both "without --clean" cases
// go red with an I/O error that does not match ErrForeignDir.
func TestCleanLetsAPathTurnBetweenFileAndDirectory(t *testing.T) {
	cases := map[string]struct {
		earlier, now string
	}{
		"file to directory": {earlier: "facts", now: "facts/d/p0001.jsonl"},
		"directory to file": {earlier: "facts/d/p0001.jsonl", now: "facts"},
	}
	for name, c := range cases {
		t.Run(name+" with --clean", func(t *testing.T) {
			dir := t.TempDir()
			seedExport(t, dir, []string{c.earlier}, []string{c.earlier})
			p := planFiles(t, dir, map[string][]byte{c.now: []byte("now\n")})
			_, removed, err := p.Write(context.Background(), true)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if diff := cmp.Diff([]string{c.earlier}, removed.Files); diff != "" {
				t.Errorf("removed (-want +got):\n%s", diff)
			}
			if b, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.now))); rerr != nil || string(b) != "now\n" {
				t.Errorf("%s holds %q, %v; want this export's bytes", c.now, b, rerr)
			}
			if slices.Contains(manifestOf(t, dir), c.earlier) {
				t.Errorf("the manifest still lists %s", c.earlier)
			}
		})
		t.Run(name+" without --clean", func(t *testing.T) {
			dir := t.TempDir()
			seedExport(t, dir, []string{c.earlier}, []string{c.earlier})
			before, rerr := os.ReadFile(filepath.Join(dir, export.MarkerName))
			if rerr != nil {
				t.Fatal(rerr)
			}
			p := planFiles(t, dir, map[string][]byte{c.now: []byte("now\n")})
			_, _, err := p.Write(context.Background(), false)
			if !errors.Is(err, export.ErrForeignDir) {
				t.Fatalf("got error %v, want a refusal matching ErrForeignDir", err)
			}
			if !strings.Contains(err.Error(), strconv.Quote(c.now)) {
				t.Errorf("the refusal %q does not name %s", err, c.now)
			}
			var h *hint.ErrHint
			if !errors.As(err, &h) || !strings.Contains(h.Hint, "--clean") {
				t.Errorf("the refusal %q carries no hint naming --clean", err)
			}
			if b, serr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.earlier))); serr != nil || string(b) != "earlier export\n" {
				t.Errorf("%s was touched: %q, %v", c.earlier, b, serr)
			}
			if after, _ := os.ReadFile(filepath.Join(dir, export.MarkerName)); !bytes.Equal(before, after) {
				t.Errorf("the manifest was rewritten before the refusal: %q", after)
			}
		})
	}
}

// A directory a planned file lands in is kept, though removing a leftover
// beside it would leave it empty for a moment: the dry run does not list it,
// and --clean neither lists it as removed nor removes and recreates it.
//
// Mutation: drop the keep check from prunedBy and this goes red with an
// Obsolete dirs diff naming facts/doc/pages, facts/doc and facts.
func TestCleanKeepsTheDirectoriesAPlannedFileLandsIn(t *testing.T) {
	dir := t.TempDir()
	seedExport(t, dir, []string{"facts/doc/pages/p0001.txt"}, []string{"facts/doc/pages/p0001.txt"})
	pages := filepath.Join(dir, "facts", "doc", "pages")
	before, err := os.Stat(pages)
	if err != nil {
		t.Fatal(err)
	}
	p := planFiles(t, dir, map[string][]byte{"facts/doc/pages/p0002.txt": []byte("now\n")})
	files, dirs, err := p.Obsolete()
	if err != nil {
		t.Fatalf("Obsolete: %v", err)
	}
	if diff := cmp.Diff([]string{"facts/doc/pages/p0001.txt"}, files); diff != "" {
		t.Errorf("Obsolete files (-want +got):\n%s", diff)
	}
	if len(dirs) > 0 {
		t.Errorf("Obsolete dirs %q, want none: a planned file lands in each", dirs)
	}
	_, removed, err := p.Write(context.Background(), true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if diff := cmp.Diff(export.Removal{Files: files}, removed); diff != "" {
		t.Errorf("removed (-Obsolete +Write):\n%s", diff)
	}
	after, err := os.Stat(pages)
	if err != nil {
		t.Fatalf("facts/doc/pages is gone: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Error("facts/doc/pages was removed and made again")
	}
	if _, err := os.Stat(filepath.Join(pages, "p0001.txt")); err == nil {
		t.Error("p0001.txt survived --clean")
	}
}

// An empty directory at or under a path this export writes as a file holds
// no data, whoever left it: --clean removes it so the file can land, and a
// write without --clean refuses with a hint naming --clean, never calling it
// something no manifest lists.
//
// Mutation: drop the walk under planned paths from prunedBy and every case
// goes red, the --clean ones with a refusal and the others with a message
// saying no earlier export's manifest lists facts.
func TestAnEmptyDirectoryInTheWayIsCleanable(t *testing.T) {
	cases := map[string][]string{
		"its file removed by hand": {"facts/d/p0001.jsonl"},
		"listed by no manifest":    nil,
	}
	for name, listed := range cases {
		for _, clean := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s clean=%v", name, clean), func(t *testing.T) {
				dir := t.TempDir()
				seedExport(t, dir, listed, nil)
				if err := os.MkdirAll(filepath.Join(dir, "facts", "d"), 0o750); err != nil {
					t.Fatal(err)
				}
				p := planFiles(t, dir, map[string][]byte{"facts": []byte("now\n")})
				if !clean {
					_, _, err := p.Write(context.Background(), false)
					if !errors.Is(err, export.ErrForeignDir) {
						t.Fatalf("got error %v, want a refusal matching ErrForeignDir", err)
					}
					if strings.Contains(err.Error(), "manifest lists") {
						t.Errorf("the refusal %q says a manifest does not list an empty directory", err)
					}
					var h *hint.ErrHint
					if !errors.As(err, &h) || !strings.Contains(h.Hint, "--clean") {
						t.Errorf("the refusal %q carries no hint naming --clean", err)
					}
					if _, serr := os.Stat(filepath.Join(dir, "facts", "d")); serr != nil {
						t.Errorf("facts/d was removed without --clean: %v", serr)
					}
					return
				}
				_, dirs, err := p.Obsolete()
				if err != nil {
					t.Fatalf("Obsolete: %v", err)
				}
				if diff := cmp.Diff([]string{"facts/d", "facts"}, dirs); diff != "" {
					t.Errorf("Obsolete dirs (-want +got):\n%s", diff)
				}
				if _, _, err := p.Write(context.Background(), true); err != nil {
					t.Fatalf("Write: %v", err)
				}
				if b, rerr := os.ReadFile(filepath.Join(dir, "facts")); rerr != nil || string(b) != "now\n" {
					t.Errorf("facts holds %q, %v; want this export's bytes", b, rerr)
				}
			})
		}
	}
}

// Write refuses, before it writes anything, a plan whose write would
// overwrite, write through or be blocked by anything the previous valid
// manifest does not list, with or without --clean, and so does the dry run's
// Obsolete: recording such a file in the first manifest would license a later
// --clean to delete it.
//
// Mutation: drop the foreign refusal from screen and every case goes red, the
// overwrites with "got error <nil>" and the blocked ones with an I/O error
// that does not match ErrForeignDir; treat a listed symlink as a regular file
// and "a symlink at a listed path" goes red with "got error <nil>".
func TestWriteRefusesToWriteOverWhatNoManifestLists(t *testing.T) {
	type seed func(t *testing.T, dir, outside string)
	put := func(t *testing.T, dir, rel, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name  string
		files map[string][]byte
		seed  seed
		keep  string // the path that must survive as "mine\n", relative to dir
	}{
		{
			name: "an unlisted file at a planned path beside a manifest",
			seed: func(t *testing.T, dir, _ string) {
				seedExport(t, dir, []string{"old.txt"}, []string{"old.txt"})
				put(t, dir, "app.js", "mine\n")
			},
			keep: "app.js",
		},
		{
			name: "an unlisted file at a planned path and no manifest",
			seed: func(t *testing.T, dir, _ string) { put(t, dir, "app.js", "mine\n") },
			keep: "app.js",
		},
		{
			name:  "an unlisted file above a planned path",
			files: map[string][]byte{"facts/d/p0001.jsonl": []byte("{}\n")},
			seed: func(t *testing.T, dir, _ string) {
				seedExport(t, dir, nil, nil)
				put(t, dir, "facts", "mine\n")
			},
			keep: "facts",
		},
		{
			name:  "a directory at a planned path holding an unlisted file",
			files: map[string][]byte{"facts": []byte("{}\n")},
			seed: func(t *testing.T, dir, _ string) {
				seedExport(t, dir, []string{"facts/old.txt"}, []string{"facts/old.txt"})
				put(t, dir, "facts/mine.txt", "mine\n")
			},
			keep: "facts/mine.txt",
		},
		{
			name: "a symlink at a listed path",
			seed: func(t *testing.T, dir, outside string) {
				seedExport(t, dir, []string{"app.js"}, nil)
				put(t, outside, "x.js", "mine\n")
				if err := os.Symlink(filepath.Join(outside, "x.js"), filepath.Join(dir, "app.js")); err != nil {
					t.Fatal(err)
				}
			},
			keep: "app.js",
		},
	}
	for _, c := range cases {
		for _, clean := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s clean=%v", c.name, clean), func(t *testing.T) {
				root := t.TempDir()
				dir, outside := filepath.Join(root, "out"), filepath.Join(root, "outside")
				for _, d := range []string{dir, outside} {
					if err := os.MkdirAll(d, 0o750); err != nil {
						t.Fatal(err)
					}
				}
				c.seed(t, dir, outside)
				marker, _ := os.ReadFile(filepath.Join(dir, export.MarkerName))
				p := planFiles(t, dir, c.files)
				if clean {
					if _, _, err := p.Obsolete(); !errors.Is(err, export.ErrForeignDir) {
						t.Errorf("Obsolete: got %v, want a refusal matching ErrForeignDir", err)
					}
				}
				_, _, err := p.Write(context.Background(), clean)
				if !errors.Is(err, export.ErrForeignDir) {
					t.Fatalf("got error %v, want a refusal matching ErrForeignDir", err)
				}
				if !strings.Contains(err.Error(), strconv.Quote(strings.SplitN(c.keep, "/", 2)[0])) {
					t.Errorf("the refusal %q does not name what is in the way", err)
				}
				if b, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.keep))); rerr != nil || string(b) != "mine\n" {
					t.Errorf("%s holds %q, %v; want it untouched", c.keep, b, rerr)
				}
				if after, _ := os.ReadFile(filepath.Join(dir, export.MarkerName)); !bytes.Equal(marker, after) {
					t.Errorf("the manifest was written before the refusal: %q", after)
				}
			})
		}
	}
}

// --clean removes a manifest temp file an interrupted export stranded at the
// top of the output directory, and Obsolete lists it; a plain write leaves
// it, and nothing of another shape or in a subdirectory is touched.
//
// Mutation: leave strandedTemps out of the clean's removal and this goes red
// with ".fisc-tmp-42 survived --clean" and a removed diff; leave it out of
// Obsolete and it goes red with an Obsolete diff.
func TestCleanRemovesAStrandedManifestTempFile(t *testing.T) {
	dir := t.TempDir()
	keep := []string{".fisc-tmp-x", "sub/.fisc-tmp-43"}
	seedExport(t, dir, nil, append(slices.Clone(keep), ".fisc-tmp-42"))
	p := planFiles(t, dir, nil)
	obsolete, _, err := p.Obsolete()
	if err != nil {
		t.Fatalf("Obsolete: %v", err)
	}
	if diff := cmp.Diff([]string{".fisc-tmp-42"}, obsolete); diff != "" {
		t.Errorf("Obsolete (-want +got):\n%s", diff)
	}
	if _, _, err = p.Write(context.Background(), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err = os.Stat(filepath.Join(dir, ".fisc-tmp-42")); err != nil {
		t.Errorf("a write without --clean removed the temp file: %v", err)
	}
	_, removed, err := p.Write(context.Background(), true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if diff := cmp.Diff(export.Removal{Temps: []string{".fisc-tmp-42"}}, removed); diff != "" {
		t.Errorf("removed (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(filepath.Join(dir, ".fisc-tmp-42")); err == nil {
		t.Error(".fisc-tmp-42 survived --clean")
	}
	for _, rel := range keep {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s was removed: %v", rel, err)
		}
	}
}

// A directory prunedBy chose that something has landed in since is skipped,
// not removed and not an error, and so is every directory above it; one
// still empty is removed, and only what was removed is reported.
//
// Mutation: drop notEmpty from removeListed's skip and this goes red with
// "directory not empty".
func TestRemovalSkipsADirectoryThatIsNoLongerEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"a/full", "b"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "full", "landed.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := export.RemoveListed(dir, nil, []string{"a/full", "a", "b"})
	if err != nil {
		t.Fatalf("RemoveListed: %v", err)
	}
	if diff := cmp.Diff([]string{"b"}, removed); diff != "" {
		t.Errorf("removed (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "full", "landed.txt")); err != nil {
		t.Errorf("what landed is gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b")); err == nil {
		t.Error("b, still empty, was not removed")
	}
}

// A file that took a chosen directory's place is listed by no manifest, and
// the prune leaves it. Mutation: prune with os.Remove again and the file is
// unlinked.
func TestRemovalLeavesAFileWhereADirectoryWas(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old"), []byte("not fisc's"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := export.RemoveListed(dir, nil, []string{"old"})
	if err != nil {
		t.Fatalf("RemoveListed: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed %v, want nothing", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "old")); err != nil {
		t.Errorf("the file in the directory's place is gone: %v", err)
	}
}

// A --clean removal that fails carries a hint saying how to get past it.
//
// Mutation: return removeListed's error without the hint and this goes red
// with "carries no hint".
func TestACleanRemovalFailureCarriesAHint(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes from a read-only directory")
	}
	dir := t.TempDir()
	seedExport(t, dir, []string{"old/gone.txt"}, []string{"old/gone.txt"})
	old := filepath.Join(dir, "old")
	if err := os.Chmod(old, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(old, 0o750) })
	_, _, err := planFiles(t, dir, nil).Write(context.Background(), true)
	if err == nil {
		t.Fatal("removing from a read-only directory succeeded")
	}
	var h *hint.ErrHint
	if !errors.As(err, &h) || !strings.Contains(h.Hint, "--output") {
		t.Errorf("the failure %q carries no hint", err)
	}
}

// --clean over a directory with no manifest, or one that does not parse,
// removes nothing: there is no record of what an export wrote there.
//
// Mutation: make previousManifest return the paths of an invalid manifest as
// though it were valid and the "unsorted" case goes red with "gone.txt was
// removed".
func TestCleanWithoutAValidManifestRemovesNothing(t *testing.T) {
	for name, marker := range map[string]string{
		"none":     "",
		"older":    "fisc export\n",
		"unsorted": export.ManifestHeader + "\nzz.txt\ngone.txt\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if marker != "" {
				if err := os.WriteFile(filepath.Join(dir, export.MarkerName), []byte(marker), 0o600); err != nil {
					t.Fatal(err)
				}
			}
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
			_, removed, err := p.Write(context.Background(), true)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if _, serr := os.Stat(filepath.Join(dir, "gone.txt")); serr != nil || !cmp.Equal(export.Removal{}, removed) {
				t.Errorf("gone.txt was removed (removed %q, stat %v)", removed, serr)
			}
		})
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
	page := readPage(t, dir)
	visible := readerVisible(t, page)
	if _, err := os.Stat(filepath.Join(dir, "caveats.html")); err == nil {
		t.Fatal("writeGolden wrote caveats.html; this test is about a site with none")
	}

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
	if strings.Contains(page, `href="caveats.html`) {
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
	// change removes.
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
		"carriage return":    "notes\r.txt",
		"nul":                "notes\x00.txt",
		"two manifest lines": "notes.txt\nindex.html",
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

// A planned path that does not come from Options.Files, which assetPath
// screens first, is still put through manifestEntry as it joins the plan: a
// view path is checked for its suffix and its directory, not for a line
// break, and a manifest could not list it.
//
// Mutation: drop the manifestEntry check from Prepare's add and this goes
// red with "Prepare accepted the view path".
func TestPrepareRefusesAViewPathAManifestCouldNotList(t *testing.T) {
	views := append(spineOnly(), export.View{Path: "notes\nindex.html", Nav: "Caveats", Template: export.CaveatsTemplate})
	_, err := export.Prepare(export.Options{
		Dir:         t.TempDir(),
		Projections: map[string][]byte{"sankey": goldenSankey(t)},
		Views:       views,
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
	})
	if err == nil {
		t.Fatal("Prepare accepted the view path \"notes\\nindex.html\"")
	}
	if !strings.Contains(err.Error(), "not a clean relative path on one line") {
		t.Errorf("got %v, want the refusal of a path a manifest could not list", err)
	}
}

// Prepare refuses every plan Write could only fail on, so that what is left
// for Write is I/O: a path its manifest could not list on one line, a path
// planned both as a file and as a directory above another, and a path under
// the manifest, which Write would need as a directory.
//
// Mutation: drop '\n' from manifestEntry's refused characters and "two
// manifest lines" goes red with "Prepare accepted"; drop the nesting check
// and "a file and a directory" goes red the same way; drop the MarkerName
// check and "under the manifest" does.
func TestPrepareRefusesWhatWriteCouldOnlyFailOn(t *testing.T) {
	cases := map[string]map[string][]byte{
		"two manifest lines":     {"notes.txt\nindex.html": []byte("x")},
		"a file and a directory": {"facts": []byte("x"), "facts/d/p0001.jsonl": []byte("x")},
		"under the manifest":     {export.MarkerName + "/notes.txt": []byte("x")},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := export.Prepare(export.Options{
				Dir:         t.TempDir(),
				Projections: map[string][]byte{"sankey": goldenSankey(t)},
				Views:       spineOnly(),
				Docs:        budgetDocs(),
				GeneratedBy: "fisc test",
				Files:       files,
			})
			if err == nil {
				t.Fatalf("Prepare accepted %q", slices.Sorted(maps.Keys(files)))
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
			// BOTH FIELDS CARRY THE MARKUP, because both reach a page by
			// different routes: text is rendered into the caveat list by
			// html/template, and summary rides in the FISC_CONFIG blob as JSON
			// inside a <script>.
			"caveats": []map[string]any{{
				"id":      `</script><script>alert("pwned")</script>`,
				"summary": `</script><script>alert("pwned")</script>`, "theme": "left-out",
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

// TestAFragmentTargetLandsBelowTheStickyBanner: .wip-banner is sticky at the
// top of every page, so html carries a scroll-padding-top or a link to a
// caveat on caveats.html scrolls its summary under the banner. Like the
// measure test below it witnesses a declaration, not a rendering; the
// rendering was read in Chromium at 1100px and 390px.
//
// Mutation: delete the html rule and this goes red.
func TestAFragmentTargetLandsBelowTheStickyBanner(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	if !regexp.MustCompile(`(?m)^html \{\s*scroll-padding-top: [1-9]`).Match(b) {
		t.Error("style.css gives html no scroll-padding-top, so a fragment lands under .wip-banner")
	}
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
	const container = "main,\n.page-head,\n.year-bar,\nfooter {\n  max-width: 1180px;\n  margin: 0 auto;"
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
		t.Error("style.css no longer sets main, .page-head, the year bar and the footer " +
			"together to one centred 1180px frame, so the measure has been applied to the " +
			"page's own column rather than to the prose in it, or the year bar and footer " +
			"run past the page's edges")
	}
	// The nav is full-bleed and keeps its items in that frame by its padding;
	// a later rule's margin would undo the footer's auto margins. Mutation:
	// drop the nav's padding-inline and its items run to the window's edges.
	for _, rule := range []string{".site-nav {\n  display: flex;\n  align-items: center;\n  gap: 8px;\n  margin: 0 -20px 14px;\n  padding-inline: max(20px, calc((100% - 1180px) / 2 + 20px));",
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
	written, _, err := p.Write(context.Background(), false)
	return written, err
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
