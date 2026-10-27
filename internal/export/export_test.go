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
	"github.com/jcrussell/livermore-budget/internal/project/projecttest"
	"github.com/jcrussell/livermore-budget/schema"
	"github.com/jcrussell/livermore-budget/site"
)

// goldenSankey is the published spine document over projecttest's
// transcription of pp.66-67, FY2026, built in memory.
func goldenSankey(t *testing.T) []byte {
	t.Helper()
	for _, p := range project.Registry(projecttest.SpineLabels, nil) {
		if p.Name() != project.PublishedProjection {
			continue
		}
		b, err := p.Build(projecttest.WithCIPLeg(projecttest.SpineFacts(projecttest.Year)), project.Options{
			Columns: []project.Column{{FiscalYear: projecttest.Year, Basis: projecttest.Basis}},
			Scopes:  []string{projecttest.Scope},
			Version: projecttest.Version,
		})
		if err != nil {
			t.Fatalf("build the spine: %v", err)
		}
		return b
	}
	t.Fatalf("no %s projection is registered", project.PublishedProjection)
	return nil
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

// pageTextFS stands in for data/extracted/, holding an uncited page and a
// second document so "only the cited pages ship" can fail. The bytes are
// synthetic: this package copies page text and never reads it.
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
	// The returned list is sorted.
	if diff := cmp.Diff(sorted(want), got); diff != "" {
		t.Errorf("written files (-want +got):\n%s", diff)
	}
	for _, rel := range got {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("reported %q but: %v", rel, err)
		}
	}

	// .nojekyll must be present and empty; //go:embed skips unnamed dotfiles.
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

// TestThePlanWritesTheIndexLast holds Prepare's order: pages after every
// asset, index.html (the completed-site sentinel) last, the manifest unplanned.
//
// Mutation: plan pages in declaration order and this goes red with "the last
// planned file is \"caveats.html\""; plan MarkerName and it goes red with
// "want the manifest left to Write".
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

// Write on a cancelled context creates nothing, not even the output directory.
//
// Mutation: drop the ctx check before MkdirAll and the first case goes red
// with "the output directory was created"; drop the per-file check and the
// second goes red with "wrote [...] after cancellation".
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

// A write over an earlier export that stops partway leaves none of its pages
// beside the mixed data. The fourth Err call stops it after its first file.
//
// Mutation: drop Write's removal of the earlier pages; red naming index.html.
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

// A write stopped after its first file leaves a manifest listing both the
// previous export and this plan, so whatever is on disk is listed.
//
// Mutation: write the first manifest from the plan alone; red with
// "old/gone.txt" missing.
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
	// The fifth Err call stops the write after its first file.
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

// --clean removes only what is in a planned path's way before writing, and
// the other leftovers after the last file, so a served site keeps its links.
//
// Mutation: remove every leftover before the write loop; red with
// "old/gone.txt was removed before the write finished".
func TestCleanRemovesOnlyWhatIsInTheWayBeforeItWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	seedExport(t, dir, []string{"facts", "old/gone.txt"}, []string{"facts", "old/gone.txt"})
	p := planFiles(t, dir, map[string][]byte{"facts/d/p0001.jsonl": []byte("now\n")})
	// The fifth Err call stops the write after its first file.
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

// A write without --clean removes nothing and keeps listing the earlier
// export's surviving files; a listed path gone or now a directory drops out.
//
// Mutation: final manifest from the plan alone goes red missing
// "old/gone.txt"; carrying every earlier entry goes red listing "vanished.txt".
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

// --clean removes listed, unplanned files and the directories that leaves
// empty, and nothing else: no unlisted file, nothing through a symlink.
//
// Mutation: leftover returning every unplanned entry unchecked goes red with
// "outside/x.txt was removed"; RemoveAll on parents goes red with
// "old/mixed/mine.txt was removed"; skipping the pruning goes red with
// "old/sub was left behind empty"; dirs left out of Removal, a removed diff.
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

// A path turning between file and directory converges under --clean; without
// it the write is refused up front with a hint naming --clean.
//
// Mutation: remove leftovers after the write loop and the "with --clean" cases
// go red with "not a directory"/"is a directory"; drop the refusal and the
// "without --clean" cases go red with an I/O error not matching ErrForeignDir.
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

// A directory a planned file lands in is kept: neither listed by the dry run
// nor removed and recreated by --clean.
//
// Mutation: drop the keep check from prunedBy; red with Obsolete dirs naming
// facts/doc/pages, facts/doc and facts.
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

// An empty directory in a planned file's way is removed by --clean, and
// without --clean refused with a hint naming --clean, not as unlisted.
//
// Mutation: drop the walk under planned paths from prunedBy; --clean cases go
// red with a refusal, the others with "no earlier export's manifest lists".
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

// Write and Obsolete refuse up front, with or without --clean, a plan that
// would overwrite, write through or be blocked by anything no manifest lists.
//
// Mutation: drop the foreign refusal from screen; overwrites go red with "got
// error <nil>", blocked ones with an I/O error not matching ErrForeignDir.
// Treat a listed symlink as a file; "a symlink at a listed path" goes red.
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

// --clean removes, and Obsolete lists, a stranded top-level manifest temp
// file; a plain write leaves it, and nothing else is touched.
//
// Mutation: strandedTemps out of the removal goes red with ".fisc-tmp-42
// survived --clean"; out of Obsolete, with an Obsolete diff.
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

// A chosen directory something has since landed in is skipped, with those
// above it; one still empty is removed, and only removals are reported.
//
// Mutation: drop notEmpty from removeListed's skip; red with "directory not
// empty".
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
// Mutation: previousManifest trusting an invalid manifest turns "unsorted"
// red with "gone.txt was removed".
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

// TestTheChartComesBeforeAnyApparatus pins the reading order: title, chart
// card, chart and captions, then the lede; above the chart only the filter
// line. Every marker is Fatalf-guarded: an absent one is -1.
//
// Mutation: a lede in the page head, a hint paragraph under the title, or an
// <h2> or <details> above the card each go red.
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

// TestTheIndexLinksItsCaveatsAndPrintsNone: the index links caveats.html and
// prints no caveat. Mutation: drop the link, or render the summaries -- red.
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

// TestAnIndexWithNoCaveatsPageListsTheCaveats: with no caveats view the index
// lists every caveat summary itself. Mutation: drop the fallback list -- red.
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

// A site exported with the extraction tree cites the city's PDF at the city's
// URL and the page text it ships, and reaches github.com for neither.
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

	// No forge in the citations or the config; the footer's repository link is
	// not a citation.
	if strings.Contains(strings.ReplaceAll(page, `href="`+export.RepositoryURL+`"`, ""), "github.com") {
		t.Error("the page still cites github.com; the shipped page text should have replaced it")
	}

	// Every page-text citation resolves, relative to the page, to a file.
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

// Cited page text ships byte for byte, since its runs of spaces are the
// printed column grid, and uncited pages do not ship.
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

	// Only the two cited pages ship.
	for _, rel := range written {
		if !strings.HasPrefix(rel, export.PageTextDir+"/") {
			continue
		}
		if rel != shippedPageText(budgetDocID, 66) && rel != shippedPageText(budgetDocID, 67) {
			t.Errorf("the site ships %q, which nothing cites", rel)
		}
	}
}

// A citation the extraction cannot back is an error, not a dangling link.
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

// Without an extraction tree the page text is cited at the forge's blob view.
func TestPageTextFallsBackToTheBlobViewWithoutAnExtractionTree(t *testing.T) {
	dir, written := writeGoldenOpts(t, export.Options{})
	page := readPage(t, dir)

	want := export.DefaultSourceBrowseURL + "/data/extracted/" + budgetDocID + "/pages/p0066.txt"
	if !strings.Contains(page, want) {
		t.Errorf("page does not cite %q", want)
	}
	// The blob view, not raw: a citation lands where a reader can navigate.
	if strings.Contains(page, "raw.githubusercontent.com") {
		t.Error("page links to raw.githubusercontent.com rather than the github.com blob view")
	}
	for _, rel := range written {
		if strings.HasPrefix(rel, export.PageTextDir+"/") {
			t.Errorf("no extraction tree was given but the site shipped %q", rel)
		}
	}
}

// An explicit remote is cited and the page text is not also shipped.
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

// Options.Files ships an arbitrary path -> bytes verbatim beside the site.
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

// An asset path that escapes the web root or lands on the fixed layout is
// refused before any side effect.
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

// A view path joining the plan is screened by manifestEntry too, so one with
// a line break is refused.
//
// Mutation: drop the manifestEntry check from Prepare's add; red with
// "Prepare accepted the view path".
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

// Prepare refuses every plan Write could only fail on, leaving Write only I/O.
//
// Mutation: drop '\n' from manifestEntry's refused characters and "two
// manifest lines" goes red with "Prepare accepted"; drop the nesting check
// and "a file and a directory" does; drop the MarkerName check and "under the
// manifest" does.
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

// A page text and an asset claiming one path are refused, not a silent
// overwrite.
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
	// The shipped page text makes the text link same-origin.
	if !strings.HasPrefix(p66.Text, export.LocalPageTextBase(budgetDocID)) {
		t.Errorf("got p66's text link %q, want one under %q", p66.Text, export.LocalPageTextBase(budgetDocID))
	}
}

func TestConfigCannotCloseItsScriptElement(t *testing.T) {
	// Markup in a caveat must neither break the page nor execute.
	doc := map[string]any{
		"schema_version": 1,
		"projection":     "sankey",
		"metadata": map[string]any{
			"fiscal_year_label": "FY 2025-26",
			"basis":             "adopted",
			"headline":          map[string]any{"all_funds_gross_expenditure_cents": 25409541200},
			// Both fields carry the markup: text renders via html/template,
			// summary rides in the FISC_CONFIG <script> as JSON.
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

// The embedded core.js's schema literals are pinned to their producers'
// constants. Mutation: set PageConfigVersion back to 1 and the page arm goes red; set
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
// template imports boot from ./app.js after the three classic scripts whose
// globals it reads, and app.js exports boot.
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

// A projection at any schema version but this binary's, older or newer, is
// refused rather than rendered plausible and wrong.
func TestWriteRefusesASchemaVersionItDoesNotUnderstand(t *testing.T) {
	// A missing key and an explicit 0 both report "got 0".
	type badVersion struct {
		version int
		absent  bool
	}
	cases := map[string]badVersion{
		"newer than this binary": {version: export.SchemaVersion + 1},
		"an explicit zero":       {version: 0},
		"no schema_version key":  {absent: true},
	}
	// At version 1 there is no older non-zero version to test.
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

// The golden projection, at this binary's version, goes through.
func TestWriteAcceptsTheSchemaVersionItUnderstands(t *testing.T) {
	dir, _ := writeGolden(t)
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Fatalf("the current schema version did not produce a page: %v", err)
	}
}

// reversionedGolden is the golden projection with schema_version rewritten, or
// removed when absent is set, and nothing else touched.
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
	// An asset tree missing app.js stands in for a botched embed.
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

// The page carries one work-in-progress banner, about coverage, not accuracy.
func TestPageCarriesTheWorkInProgressBanner(t *testing.T) {
	dir, _ := writeGolden(t)
	page := readPage(t, dir)

	// Exactly once: a duplicate renders as two stacked bars, not an error.
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

// readerVisible is the page with the FISC_CONFIG script removed. Every "does
// the page say X" assertion needs it: that script holds the whole projection.
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

// TestAFragmentTargetLandsBelowTheStickyBanner: html declares a
// scroll-padding-top so a fragment target clears the sticky .wip-banner.
//
// Mutation: delete the html rule and this goes red.
func TestAFragmentTargetLandsBelowTheStickyBanner(t *testing.T) {
	b, err := fs.ReadFile(site.FS(), "style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	rule := regexp.MustCompile(`(?m)^html \{([^}]*)\}`).FindSubmatch(b)
	if rule == nil || !regexp.MustCompile(`scroll-padding-top:\s*[0-9.]*[1-9]`).Match(rule[1]) {
		t.Error("style.css gives html no scroll-padding-top, so a fragment lands under .wip-banner")
	}
	// One declaration, so no later rule or media query takes it back.
	if n := strings.Count(string(b), "scroll-padding-top:"); n != 1 {
		t.Errorf("style.css declares scroll-padding-top %d times, want the html rule's alone", n)
	}
}

// TestTheStylesheetHasOneTextMeasure holds style.css to one --measure token,
// a declaration and not a rendering (fisc-6at). The 1180px container arm is
// the anti-vacuity one: without it, clamping main to the measure passes.
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
	// The container rule itself, not the number anywhere. Mutation: split it
	// and clamp main to the measure; a bare 1180px grep stays green.
	const container = "main,\n.page-head,\n.year-bar,\nfooter {\n  max-width: 1180px;\n  margin: 0 auto;"
	// The print block opens what the page folds; declared only (fisc-yj4w.18).
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

// TestTheGateRunsAfterValidationAndBeforeAnyPageRenders: validation's refusal
// wins over the gate's, and the gate's over the unstamped page's render error.
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
// Sankey, with no hint and no year control.
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
// spineOnly where the caller did not.
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
