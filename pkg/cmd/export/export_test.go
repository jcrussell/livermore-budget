package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// goldenSankey is the hand-derived worked example of the contract
// (docs/sankey-contract.md). The pipeline has to reproduce it.
func goldenSankey(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "sankey.golden.json"))
	if err != nil {
		t.Fatalf("read golden projection: %v", err)
	}
	return b
}

// fakeRepo is a repository root with only the file the export reads out of it:
// the source registry. Pointing the test at the real tree would make it fail
// for reasons that have nothing to do with the command.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	registry := `schema_version: 1
sources:
  - id: livermore-budget-fy2026-2027
    title: "FY 2025-2027 Budget Book"
    publisher: "City of Livermore, California"
    url: "https://www.livermoreca.gov/home/showpublisheddocument/12813"
`
	if err := os.WriteFile(filepath.Join(root, "data", "sources.yaml"), []byte(registry), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedExtraction(t, root)
	return root
}

// seedExtraction writes the extraction artifacts the export copies into the
// site: the committed text of the two pages the golden projection cites, plus
// a page it does not, so "only the cited pages ship" can fail.
func seedExtraction(t *testing.T, root string) {
	t.Helper()
	pages := filepath.Join(root, "data", "extracted", budgetDocID, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for name, body := range map[string]string{
		"p0066.txt": pageText(66),
		"p0067.txt": pageText(67),
		"p0100.txt": "a page nothing cites\n",
	} {
		if err := os.WriteFile(filepath.Join(pages, name), []byte(body), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// budgetDocID is the document the golden projection cites.
const budgetDocID = "livermore-budget-fy2026-2027"

// pageText is the seeded body of one extracted page. The runs of spaces are
// the printed column grid, so they are part of what has to arrive unchanged.
func pageText(page int) string {
	return fmt.Sprintf("PAGE %d\nREVENUE      123,456      789\n", page)
}

func testOptions(t *testing.T) (*Options, *iostreams.IOStreams, func() string, func() string) {
	t.Helper()
	io, _, out, errOut := iostreams.Test()
	root := fakeRepo(t)
	opts := &Options{
		IO:        io,
		RepoRoot:  func() (string, error) { return root, nil },
		OutputDir: filepath.Join(t.TempDir(), "dist"),
		Build: func(string) (result, error) {
			return result{Projections: map[string][]byte{"sankey": goldenSankey(t)}}, nil
		},
	}
	return opts, io, out.String, errOut.String
}

func TestNewCmdExportFlags(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    Options
		wantErr string
	}{
		{name: "defaults", want: Options{OutputDir: "dist"}},
		{name: "output", args: []string{"--output", "site"}, want: Options{OutputDir: "site"}},
		{name: "short output", args: []string{"-o", "site"}, want: Options{OutputDir: "site"}},
		{name: "clean", args: []string{"--clean"}, want: Options{OutputDir: "dist", Clean: true}},
		{
			name: "source browse url",
			args: []string{"--source-browse-url", "https://example.test/blob/main"},
			want: Options{OutputDir: "dist", SourceBrowseURL: "https://example.test/blob/main"},
		},
		{name: "empty output", args: []string{"--output", ""}, wantErr: "--output requires a directory"},
		{
			name:    "browse url with no scheme",
			args:    []string{"--source-browse-url", "example.test/blob/main"},
			wantErr: "--source-browse-url",
		},
		{
			name:    "browse url on another scheme",
			args:    []string{"--source-browse-url", "ftp://example.test/blob/main"},
			wantErr: "must be an http or https URL",
		},
		{
			name:    "browse url with no host",
			args:    []string{"--source-browse-url", "https:///blob/main"},
			wantErr: "has no host",
		},
		{name: "positional args", args: []string{"dist"}, wantErr: `unknown command "dist"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: io, RepoRoot: func() (string, error) { return "", nil }}

			var got *Options
			cmd := NewCmdExport(f, func(o *Options) error {
				got = o
				return nil
			})
			// The command runs in whatever directory the test runs in, so
			// relative flag values resolve against it; the assertions below
			// compare the pre-resolution intent.
			cmd.SetArgs(tc.args)
			cmd.SetOut(os.Stderr)
			cmd.SetErr(os.Stderr)
			err := cmd.Execute()

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("got nil error, want %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %q, want it to contain %q", err, tc.wantErr)
				}
				var flagErr *cmdutil.FlagError
				if !errors.As(err, &flagErr) && strings.HasPrefix(tc.name, "browse url") {
					t.Errorf("got %T, want a *cmdutil.FlagError so the runner exits 2", err)
				}
				if strings.HasPrefix(tc.wantErr, "--") && !errors.As(err, &flagErr) {
					t.Errorf("got %T, want a *cmdutil.FlagError so the runner exits 2", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got == nil {
				t.Fatal("runF was not called")
			}
			if got.Clean != tc.want.Clean {
				t.Errorf("got --clean %v, want %v", got.Clean, tc.want.Clean)
			}
			if got.SourceBrowseURL != tc.want.SourceBrowseURL {
				t.Errorf("got --source-browse-url %q, want %q", got.SourceBrowseURL, tc.want.SourceBrowseURL)
			}
			// Validate canonicalises the path, so compare against the
			// resolved form of what was asked for.
			want, err := cmdutil.ResolveOutputDir(tc.want.OutputDir)
			if err != nil {
				t.Fatalf("resolve expectation: %v", err)
			}
			if got.OutputDir != want {
				t.Errorf("got output %q, want %q", got.OutputDir, want)
			}
		})
	}
}

func TestExportRunWritesASiteAndSaysHowToServeIt(t *testing.T) {
	opts, _, out, errOut := testOptions(t)

	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}

	// A document stating a year and basis ships as its column, not as data/sankey.json.
	for _, rel := range []string{"index.html", "app.js", "core.js", "sankey.js", "style.css", ".nojekyll",
		"vendor/d3.min.js", "fy2026-adopted.json"} {
		if _, err := os.Stat(filepath.Join(opts.OutputDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}

	// Data on Out: the paths are what a caller pipes somewhere.
	lines := strings.Split(strings.TrimSpace(out()), "\n")
	if len(lines) < 6 {
		t.Errorf("got %d paths on Out, want one per written file:\n%s", len(lines), out())
	}
	if !strings.HasSuffix(lines[len(lines)-1], filepath.Join(opts.OutputDir, "vendor", "d3.min.js")) &&
		!strings.Contains(out(), filepath.Join(opts.OutputDir, "index.html")) {
		t.Errorf("Out does not list the written files:\n%s", out())
	}

	// Chatter on ErrOut, including the one instruction without which the
	// output looks broken: fetch() is blocked on file:// URLs.
	if !strings.Contains(errOut(), "python3 -m http.server -d") {
		t.Errorf("ErrOut does not tell the user how to serve the site:\n%s", errOut())
	}
	if strings.Contains(out(), "python3") {
		t.Error("the serve instruction is chatter and belongs on ErrOut")
	}
}

// TestEveryCaveatSummaryLinksToAnAnchorThatExists is the check that makes the
// caveats page a feature rather than a hope.
//
// EVERY OTHER GUARD STOPS AT THE FILE. The asset walks stat "caveats.html" and
// are satisfied; they cut the fragment off before asking, because the
// filesystem has no opinion about it. So a summary linking to
// #caveat-sankey--typo would pass every existing check, render as a working
// link, and drop the reader at the top of a long page with no indication that
// anything went wrong -- which is the quietest failure this change can produce
// and the only one a reader would blame themselves for.
//
// IT WALKS THE RENDERED PAGES, not the view list or the Go structs, for the
// reason the asset walk does: the failure is a string composed in one place and
// consumed in another, and the page is where the two meet. The anchors are read
// out of caveats.html's own markup rather than recomposed with caveatAnchor,
// because recomposing them would assert that one function agrees with itself.
func TestEveryCaveatSummaryLinksToAnAnchorThatExists(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	read := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(opts.OutputDir, name))
		if err != nil {
			t.Fatalf("read the exported %s: %v", name, err)
		}
		return string(raw)
	}

	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="(caveat-[^"]+)"`).FindAllStringSubmatch(read("caveats.html"), -1) {
		if ids[m[1]] {
			t.Errorf("caveats.html carries the anchor %q twice; a link to it lands on "+
				"whichever the browser finds first", m[1])
		}
		ids[m[1]] = true
	}
	if len(ids) == 0 {
		t.Fatal("caveats.html carries no caveat anchors, so this test asserts nothing")
	}

	// TWO SPELLINGS, AND THE SECOND ONE IS THE LARGER SET. A page carries its
	// opening year's links as markup, href="caveats.html#...", and every OTHER
	// year's inside the FISC_CONFIG blob as JSON, "href":"caveats.html#...",
	// which app.js injects on a year switch. Matching markup alone left the
	// blob unchecked -- and the blob is where the anchors that differ live,
	// since FY2026-27 carries a contested-total caveat FY2025-26 does not.
	// Mutation-proved: appending "-typo" to the stem in buildSankeyPage's years
	// loop leaves every other test in both packages green while breaking every
	// caveat link a reader reaches by switching year.
	spellings := []*regexp.Regexp{
		regexp.MustCompile(`href="caveats\.html#([^"]+)"`),
		regexp.MustCompile(`"href":"caveats\.html#([^"]+)"`),
	}

	// Every page the site wrote, not a list: a page added without its links
	// checked is exactly what this is for.
	entries, err := os.ReadDir(opts.OutputDir)
	if err != nil {
		t.Fatalf("read the exported site: %v", err)
	}
	linked, injected := 0, 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		page := read(e.Name())
		for i, re := range spellings {
			for _, m := range re.FindAllStringSubmatch(page, -1) {
				if i == 0 {
					linked++
				} else {
					injected++
				}
				if !ids[m[1]] {
					t.Errorf("%s links to caveats.html#%s, and caveats.html carries no such "+
						"anchor; the link works and lands the reader nowhere in particular",
						e.Name(), m[1])
				}
			}
		}
	}
	if linked == 0 {
		t.Error("no page links to a caveat anchor in its markup, so the summaries are " +
			"truncations rather than pointers")
	}
	if injected == 0 {
		t.Error("no page carries a caveat anchor in its FISC_CONFIG blob, so the year " +
			"switch has none to inject and this test's second spelling checks nothing")
	}
}

// TestTheSiteDoesNotSayThePSeventySixScheduleIsUnmapped is a cross-package
// guard, and it lives HERE because here is the only place the two copies of
// that claim meet.
//
// WHAT WENT WRONG. The same claim was written twice, in two packages, about the
// same difference: internal/project's transferCaveat, built from the facts, and
// internal/export's "Unmatched transfers" tile note, typed into the packager.
// Both said the p76 transfer schedule was unmapped. Both stopped being true at
// ced45b4, when p76 was published. And neither was findable from the other,
// because one said "not yet mapped" and the other said "not mapped yet" -- a
// grep for either went green with the other still shipping to readers.
//
// So this asserts over the RENDERED PAGE rather than over either source. The
// page is where a reader meets both, and no word-order variant survives it.
//
// WHAT THIS TEST OBSERVES, EXACTLY, because the two halves reach the page by
// different routes and only one of them is live here. testOptions stubs Build
// to return the COMMITTED testdata/sankey.golden.json, so the caveat on this
// page is fixture bytes: mutate transferCaveat and this test stays green.
// TestSankeyReproducesGoldenFile is what fails then -- it rebuilds the document
// from the facts and compares those same bytes -- so the caveat is pinned, one
// package over. The tile note IS live code on this path and this test is its
// only guard: reverting internal/export/page.go's old wording reddens exactly
// this test and nothing else. Proved both ways by mutation, 2026-08-27.
//
// WHICH PAGE IT READS CHANGED, AND IT HAD TO. The caveat's full text moved to
// caveats.html when index.html started showing one-line summaries -- and this
// test went on passing, because index.html still ships the whole document as
// JSON inside window.FISC_CONFIG and "Transfers Out to CIP" was in that blob.
// Measured: strip the config script from the exported index.html and neither
// string is left anywhere a reader could see. So the test was green on a string
// no reader reads, which is the exact "green because the gate fired" shape --
// its own premise above says the page is where a reader MEETS the claim, and
// that had silently stopped being index.html.
//
// The two halves are therefore read from two pages now, each where a reader
// actually meets it: the correction on caveats.html, which carries the text,
// and the stale wording refused on BOTH, since either could carry it.
func TestTheSiteDoesNotSayThePSeventySixScheduleIsUnmapped(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	read := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(opts.OutputDir, name))
		if err != nil {
			t.Fatalf("read the exported %s: %v", name, err)
		}
		return string(raw)
	}
	pages := map[string]string{
		"index.html":   read("index.html"),
		"caveats.html": read("caveats.html"),
	}

	// Both word orders, plus the bead that never published anything -- its own
	// close reason reads "Not published: no facts, facts.jsonl byte-identical".
	// Refused on every page, because the claim could reappear on either.
	for name, page := range pages {
		for _, stale := range []string{
			"not yet mapped", "not mapped yet", "fisc-5gk.3",
			// p76's own document is published and Transfers In opens into it.
			"fisc-9gh", "Pairing the legs needs",
		} {
			if strings.Contains(page, stale) {
				t.Errorf("the exported %s promises work that has landed (%q); Budget Book "+
					"p76 is mapped, published at scope transfers-by-fund AND drawn with "+
					"its legs paired", name, stale)
			}
		}
	}

	// And the correction is present rather than merely the falsehood absent: a
	// caveat that dropped the sentence entirely would pass the loop above.
	//
	// caveats.html ships no FISC_CONFIG, so this cannot pass on JSON no reader reads.
	caveats := pages["caveats.html"]
	if strings.Contains(caveats, "FISC_CONFIG") {
		t.Fatal("caveats.html ships a config blob, so this test's substring search " +
			"could pass on JSON no reader reads")
	}
	for _, want := range []string{"p222", "Capital Improvement Program", "transfers-by-fund"} {
		if !strings.Contains(caveats, want) {
			t.Errorf("the exported caveats.html does not say %q, so the residual is "+
				"unexplained rather than cited", want)
		}
	}

	// AND index.html STILL POINTS AT IT. Moving the text is only honest if the
	// page a reader lands on offers a way to reach it; a summary with no link
	// is a truncation.
	if !strings.Contains(pages["index.html"], `href="caveats.html#caveat-`) {
		t.Error("index.html links to no caveat anchor, so the paragraph it stopped " +
			"printing is not reachable from the page that stopped printing it")
	}
}

// The acceptance test for fisc-ze7: the exported site resolves BOTH citation
// classes with no network access to github.com.
//
// It walks the page's own links rather than a list, because the failure being
// prevented is invisible to every other test — the export succeeds, the page
// renders, and the citation 404s only for the reader who follows it. Class one,
// the city's PDF, stays remote by design: it is the city's publication and its
// LFS copy is not ours to republish. Class two, the extracted text every figure
// is traced to, has to be a file inside the output.
func TestExportRunResolvesBothCitationClassesWithoutGitHub(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readPage(t, opts.OutputDir)

	var pdfCitations, textCitations int
	for _, ref := range pageRefs(page) {
		switch {
		case strings.HasPrefix(ref, "https://www.livermoreca.gov/"):
			// The city's own document: the source heading links the document,
			// each cited page links it again with a #page fragment.
			if strings.Contains(ref, "#page=") {
				pdfCitations++
			}
		case strings.Contains(ref, "://"):
			t.Errorf("the page reaches %q; every citation but the city's PDF has to resolve inside the site", ref)
		default:
			// The anchor comes off before the filesystem is asked:
			// "caveats.html#caveat-sankey--x" names a file plus a place in it,
			// and only the file half is a question about what was written. The
			// fragment half is TestEveryCaveatSummaryLinksToAnAnchorThatExists'.
			target := ref
			if before, _, ok := strings.Cut(target, "#"); ok {
				target = before
			}
			if _, err := os.Stat(filepath.Join(opts.OutputDir, filepath.FromSlash(target))); err != nil {
				t.Errorf("the page references %q, which the site does not carry: %v", ref, err)
			}
			if strings.HasSuffix(ref, ".txt") {
				textCitations++
			}
		}
	}
	if pdfCitations < 2 {
		t.Errorf("got %d PDF citations, want one per cited page", pdfCitations)
	}
	if textCitations < 2 {
		t.Errorf("got %d extracted-text citations resolving inside the site, want one per cited page", textCitations)
	}
	if strings.Contains(page, "github.com") {
		t.Error("the page cites github.com; the site is supposed to carry its own provenance")
	}

	// The bytes have to be the extraction's, spaces and all: the runs of
	// spaces ARE the printed column grid.
	got, err := os.ReadFile(filepath.Join(opts.OutputDir, "extracted", budgetDocID, "pages", "p0066.txt"))
	if err != nil {
		t.Fatalf("read the shipped page text: %v", err)
	}
	if diff := cmp.Diff(pageText(66), string(got)); diff != "" {
		t.Errorf("shipped page text differs from the extraction (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, "extracted", budgetDocID, "pages", "p0100.txt")); err == nil {
		t.Error("the site ships p0100.txt, which nothing cites")
	}
}

// --source-browse-url is the escape hatch for a deploy that would rather link
// to a browsable tree. It is opt-in precisely because it puts the provenance
// back on somebody else's server.
func TestExportRunCitesTheBrowseURLWhenAsked(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	opts.SourceBrowseURL = "https://example.test/blob/main"
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readPage(t, opts.OutputDir)

	want := "https://example.test/blob/main/data/extracted/" + budgetDocID + "/pages/p0066.txt"
	if !strings.Contains(page, want) {
		t.Errorf("page does not cite %q", want)
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, "extracted")); err == nil {
		t.Error("the export cited a remote and shipped the page text as well")
	}
}

// An extraction the repository does not have is a broken checkout, and the
// export has to say so rather than publish citations to files it never copied.
func TestExportRunNeedsTheExtractionItCites(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	root, err := opts.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	if rmErr := os.RemoveAll(filepath.Join(root, "data", "extracted")); rmErr != nil {
		t.Fatalf("seed: %v", rmErr)
	}

	err = exportRun(opts)
	if err == nil {
		t.Fatal("got nil error, want a refusal to cite text it cannot ship")
	}
	if got := err.Error(); !strings.Contains(got, "page 66") {
		t.Errorf("got error %q, want it to name the page it could not read", got)
	}
}

// pageRefs are the page's own href and src values.
func pageRefs(page string) []string {
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
			// FRAGMENTS ARE KEPT HERE. This function reports what the page
			// says, and its caller needs the fragment on both classes of
			// reference for opposite reasons: it COUNTS "#page=" on the city's
			// PDF links, and it has to STRIP the anchor off a relative one
			// before asking the filesystem about it. Cutting in this loop
			// broke the first while fixing the second -- measured, "got 0 PDF
			// citations, want one per cited page".
			if ref == "" || strings.HasPrefix(ref, "#") {
				continue
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

func readPage(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	return string(b)
}

func TestExportRunCleanRefusesSomebodyElsesDirectory(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	opts.Clean = true
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	keep := filepath.Join(opts.OutputDir, "thesis.txt")
	if err := os.WriteFile(keep, []byte("years of work"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := exportRun(opts)
	if err == nil {
		t.Fatal("got nil error, want a refusal to delete")
	}
	var hint *cmdutil.ErrHint
	if !errors.As(err, &hint) {
		t.Errorf("got %T, want a *cmdutil.ErrHint so the user is told what to do", err)
	}
	if _, serr := os.Stat(keep); serr != nil {
		t.Errorf("the file was removed anyway: %v", serr)
	}
}

// TestExportRunRefusesBeforeCleanDestroysTheSite pins the ORDER of the
// reachability check against --clean, which is the whole of its consequence.
//
// The check needs nothing SafeCleanDir produces. While it sat between Clean and
// Write, `fisc export --clean` over a corpus that had just published an
// undeclared document emptied the reader's output directory and THEN refused --
// destroying a working site to report a fault that was detectable before
// anything was touched. A validation that runs after a destructive step is a
// validation in the wrong place, however correct its verdict.
func TestExportRunRefusesBeforeCleanDestroysTheSite(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(opts); err != nil {
		t.Fatalf("first export: %v", err)
	}
	index := filepath.Join(opts.OutputDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("no site to destroy: %v", err)
	}

	// A published document that is BUILT, rendered by no view, and declared by
	// no entry. The stub builds only the spine, and the check skips documents
	// that were not built (that is assertPublishedBuilt's finding), so the
	// builder has to produce this one for the state to be reachable at all.
	// A DECLARED STEM, READ OUT OF THE MAP RATHER THAN NAMED. This was
	// project.FundFlowsProjection until drilldown.html started rendering it, at
	// which point the entry was correctly deleted and this test went red naming
	// its own remedy. Taking whichever stem is declared means the next page to
	// land does not send it red again -- and if the map ever empties, the
	// Fatalf below still says so rather than the test quietly asserting nothing.
	stem := ""
	for k := range unviewedDocuments {
		if stem == "" || k < stem {
			stem = k
		}
	}
	reason, ok := unviewedDocuments[stem]
	if !ok {
		t.Fatalf("unviewedDocuments declares nothing, so the state this test is about " +
			"cannot be reached; delete it or give it a document rendered by no view")
	}
	delete(unviewedDocuments, stem)
	t.Cleanup(func() { unviewedDocuments[stem] = reason })
	// The document states the projection the published declaration gives it.
	projection := ""
	for _, d := range project.PublishedDocuments() {
		if d.Stem == stem {
			projection = d.Projection
		}
	}
	if projection == "" {
		t.Fatalf("no published document has stem %q", stem)
	}
	doc := bytes.Replace(goldenSankey(t), []byte(`"projection": "sankey"`), []byte(`"projection": "`+projection+`"`), 1)
	opts.Build = func(string) (result, error) {
		return result{Projections: map[string][]byte{
			"sankey": goldenSankey(t), stem: doc}}, nil
	}

	opts.Clean = true
	if err := exportRun(opts); err == nil {
		t.Fatal("exportRun accepted an undeclared unrendered document")
	} else if !strings.Contains(err.Error(), "no view renders it") {
		t.Fatalf("got %v, want the reachability refusal", err)
	}
	if _, err := os.Stat(index); err != nil {
		t.Errorf("--clean emptied the output directory and then refused, so the reader's "+
			"site is gone and no new one was written: %v", err)
	}
}

func TestExportRunCleanEmptiesItsOwnOutput(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(opts); err != nil {
		t.Fatalf("first export: %v", err)
	}
	// Seeded in a subdirectory, which is what --clean has to reach.
	stale := filepath.Join(opts.OutputDir, "data", "gone.json")
	if err := os.MkdirAll(filepath.Dir(stale), 0o750); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(stale, []byte("{}"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	opts.Clean = true
	if err := exportRun(opts); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a stale file survived --clean")
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, "index.html")); err != nil {
		t.Errorf("the site was not rewritten: %v", err)
	}
}

// TestExportRunShipsTheBuildersFiles is fisc-xgr's whole point, and nothing
// else asserts it.
//
// export.Options.Files has existed since fisc-ze7 and this command never set
// it, so the only thing that ever travelled the asset channel was the cited
// page text, which Write puts through it on its own behalf. A Builder could
// produce an asset and had no way to hand it over. TestFilesShipVerbatimBeside
// TheSite covers the channel, but it calls export.Write directly -- it would
// stay green with the two ends of this command still unconnected, which is the
// state that shipped.
//
// So this asserts the wiring rather than the writing: an asset the BUILDER
// returned reaches the output tree, at the path it asked for, byte for byte,
// and is reported to stdout with the rest.
func TestExportRunShipsTheBuildersFiles(t *testing.T) {
	opts, _, out, _ := testOptions(t)
	shard := []byte(`{"id":"fisc-f-000000000000","doc_id":"d","page":66}` + "\n")
	opts.Build = func(string) (result, error) {
		return result{
			Projections: map[string][]byte{"sankey": goldenSankey(t)},
			Files:       map[string][]byte{"facts/d/pages/p0066.jsonl": shard},
		}, nil
	}
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(opts.OutputDir, "facts", "d", "pages", "p0066.jsonl"))
	if err != nil {
		t.Fatalf("the builder's asset did not reach the site: %v", err)
	}
	if !bytes.Equal(got, shard) {
		t.Errorf("asset shipped as %q, want %q", got, shard)
	}
	// AND IT IS REPORTED, because the written paths are this command's data --
	// what a caller pipes into a deploy. An asset that ships without appearing
	// there is invisible to everything downstream.
	if !strings.Contains(out(), "facts/d/pages/p0066.jsonl") {
		t.Errorf("stdout %q does not name the asset that was written", out())
	}
}

// TestExportRunRefusesAnAssetThatEscapesTheSite is the other half: the channel
// now reaches a caller, so the guard on it has to be reachable too.
//
// It is assetPath's refusal, asserted through the command rather than through
// export.Write, because that is the path a Builder now takes.
//
// IT RUNS BOTH WITH AND WITHOUT --clean, and the second case is the one that
// matters. The first version of this test ran only without, and stayed green
// while `fisc export --clean` emptied the reader's site and THEN refused: Write
// validates, but Write runs after SafeCleanDir. The fix hoisted
// Options.Validate above the clean, which is the same ordering
// assertPublishedReachable argues for one screen up; this is what would have
// caught it.
func TestExportRunRefusesAnAssetThatEscapesTheSite(t *testing.T) {
	for _, clean := range []bool{false, true} {
		name := "without --clean"
		if clean {
			name = "with --clean"
		}
		t.Run(name, func(t *testing.T) {
			opts, _, _, _ := testOptions(t)
			opts.Clean = clean

			// A SITE THE REFUSAL COULD DESTROY, written by a good run first.
			// Without it --clean has nothing to empty and the test asserts
			// against an absence rather than against a survivor.
			if err := exportRun(opts); err != nil {
				t.Fatalf("seed export: %v", err)
			}
			index := filepath.Join(opts.OutputDir, "index.html")
			if _, err := os.Stat(index); err != nil {
				t.Fatalf("seed export wrote no index: %v", err)
			}

			opts.Build = func(string) (result, error) {
				return result{
					Projections: map[string][]byte{"sankey": goldenSankey(t)},
					Files:       map[string][]byte{"../escaped.jsonl": []byte("{}")},
				}, nil
			}
			err := exportRun(opts)
			if err == nil {
				t.Fatal("exportRun accepted an asset path outside the site")
			}
			if !strings.Contains(err.Error(), "clean relative path") {
				t.Errorf("got %v, want the asset-path refusal", err)
			}
			if _, serr := os.Stat(index); serr != nil {
				t.Errorf("the refusal destroyed the reader's site: %v", serr)
			}
			if _, serr := os.Stat(filepath.Join(opts.OutputDir, "escaped.jsonl")); serr == nil {
				t.Error("the escaping asset was written")
			}
		})
	}
}

func TestExportRunReportsABuilderFailure(t *testing.T) {
	opts, _, _, errOut := testOptions(t)
	opts.Build = func(string) (result, error) {
		return result{}, os.ErrNotExist
	}
	if err := exportRun(opts); err == nil {
		t.Fatal("got nil error, want the builder's failure")
	}
	if _, err := os.Stat(opts.OutputDir); err == nil {
		t.Error("the output directory was created before the data was ready")
	}
	if errOut() != "" {
		t.Errorf("got chatter on a failed run: %q", errOut())
	}
}

// TestBuildProjectionsRunsThePipeline is the check that the published site is
// built from the fact store rather than from anything else. It reads the real
// committed facts/facts.jsonl through the real projection and compares against
// testdata/sankey.golden.json, the hand-derived worked example — so it fails
// both if the pipeline drifts and if somebody edits the golden to match a bug.
//
// generated_by is the one key that legitimately differs: the golden records
// that a human derived it, and a build records the binary that did.
func TestBuildProjectionsRunsThePipeline(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	got, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	// A single-column document goes through project.PublishedStem: the opening
	// published column keeps the bare stem, others are suffixed by year, and by
	// basis where it is not the published one. A multi-column document keeps
	// its projection's name.
	if diff := cmp.Diff([]string{
		"changes-in-fund-balances",
		"department-funding", "department-funding-2024-actual",
		"department-funding-2025-revised", "department-funding-2027",
		"department-spending", "department-spending-2024-actual",
		"department-spending-2025-revised", "department-spending-2027",
		"fund-balances",
		"fund-flows", "fund-flows-2024-actual", "fund-flows-2025-revised", "fund-flows-2027",
		"fund-sources-uses", "fund-sources-uses-2024-actual", "fund-sources-uses-2025-revised",
		"fund-sources-uses-2027",
		"revenue-trends", "sankey", "sankey-2027",
		"transfers-by-fund", "transfers-by-fund-2027", "transfers-out", "transfers-out-2027",
	}, keys(got)); diff != "" {
		t.Errorf("projection names (-want +got):\n%s", diff)
	}

	var built, golden map[string]any
	if err := json.Unmarshal(got["sankey"], &built); err != nil {
		t.Fatalf("decode built projection: %v", err)
	}
	if err := json.Unmarshal(goldenSankey(t), &golden); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	delete(built["metadata"].(map[string]any), "generated_by")
	delete(golden["metadata"].(map[string]any), "generated_by")
	if diff := cmp.Diff(golden, built); diff != "" {
		t.Errorf("the pipeline does not reproduce the golden file (-want +got):\n%s", diff)
	}
}

// TestBuildProjectionsNeedsTheFactStore checks the remediation, not just the
// failure: without facts.jsonl the site cannot be built, and the error has to
// say which command produces it.
func TestBuildProjectionsNeedsTheFactStore(t *testing.T) {
	_, err := buildProjections(t.TempDir())
	if err == nil {
		t.Fatal("buildProjections with no fact store = nil error, want a failure")
	}
	var hint *cmdutil.ErrHint
	if !errors.As(err, &hint) || !strings.Contains(hint.Hint, "fisc build") {
		t.Errorf("error %v carries no hint naming `fisc build`", err)
	}
}

func TestLoadDocsReadsTheSourceRegistry(t *testing.T) {
	docs, err := loadDocs(fakeRepo(t))
	if err != nil {
		t.Fatalf("loadDocs: %v", err)
	}
	want := []struct {
		id, url string
	}{{"livermore-budget-fy2026-2027", "https://www.livermoreca.gov/home/showpublisheddocument/12813"}}
	if len(docs) != len(want) {
		t.Fatalf("got %d docs, want %d", len(docs), len(want))
	}
	if docs[0].ID != want[0].id || docs[0].PDFURL != want[0].url {
		t.Errorf("got %+v, want id %q url %q", docs[0], want[0].id, want[0].url)
	}
}

// keys is the stems in a map, SORTED.
//
// The sort is not cosmetic. Go randomises map iteration, so without it this
// helper returns a different order on different runs and any cmp.Diff against a
// literal list is flaky -- it passed only because two elements come back in the
// written order often enough to look stable. A third stem made it visible.
func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTheCommittedStemsAreUnchanged pins the document stems as typed literals
// (fisc-rmx): a list derived from the naming rule would agree with a rename.
// A stem is the schedule key and the year radio's value in site/app.js, so a
// rename moves both silently. Published paths are
// TestTheSitePublishesOneFilePerColumnAndNothingTwice's.
func TestTheCommittedStemsAreUnchanged(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	want := []string{
		"changes-in-fund-balances",
		"department-funding", "department-funding-2024-actual",
		"department-funding-2025-revised", "department-funding-2027",
		"department-spending", "department-spending-2024-actual",
		"department-spending-2025-revised", "department-spending-2027",
		"fund-balances",
		"fund-flows", "fund-flows-2024-actual", "fund-flows-2025-revised", "fund-flows-2027",
		"fund-sources-uses", "fund-sources-uses-2024-actual", "fund-sources-uses-2025-revised",
		"fund-sources-uses-2027",
		"revenue-trends", "sankey", "sankey-2027",
		"transfers-by-fund", "transfers-by-fund-2027", "transfers-out", "transfers-out-2027",
	}
	got := keys(built)
	sort.Strings(got)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("stems mismatch (-want +got):\n%s", diff)
	}
}

// TestEveryRecordsLinkTheConfigCarriesIsAShardTheExportWrote holds the page
// config's records links to the export's own output: the client opens each
// verbatim, so a link to a file the export did not write is a broken citation.
func TestEveryRecordsLinkTheConfigCarriesIsAShardTheExportWrote(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readPage(t, opts.OutputDir)
	const open = "window.FISC_CONFIG = "
	_, rest, ok := strings.Cut(page, open)
	if !ok {
		t.Fatal("the served page carries no FISC_CONFIG")
	}
	body, _, _ := strings.Cut(rest, ";</script>")
	var config struct {
		Docs map[string]struct {
			Pages map[string]struct {
				Records string `json:"records"`
			} `json:"pages"`
		} `json:"docs"`
	}
	if err := json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatalf("decode FISC_CONFIG: %v", err)
	}
	shards := 0
	for id, doc := range config.Docs {
		for n, p := range doc.Pages {
			if p.Records == "" {
				continue
			}
			shards++
			if _, err := os.Stat(filepath.Join(opts.OutputDir, filepath.FromSlash(p.Records))); err != nil {
				t.Errorf("%s p%s links records %q, which the export did not write: %v", id, n, p.Records, err)
			}
		}
	}
	if shards == 0 {
		t.Fatal("the config carries no records link, so this test asserts nothing")
	}
	t.Logf("%d records links, each a shard the export wrote", shards)
}

// TestTheSitePublishesOneFilePerColumnAndNothingTwice pins the file set
// `fisc export` lays down, as typed literals: deriving it from ColumnsOf would
// agree with any change to ColumnsOf. A document stating no fiscal year or
// basis ships as itself.
func TestTheSitePublishesOneFilePerColumnAndNothingTwice(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	var got []string
	walkErr := filepath.WalkDir(opts.OutputDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(opts.OutputDir, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		// The fact shards and the extracted page text are their own channels,
		// disclosed on their own panels and named by their own tests.
		if strings.HasSuffix(rel, ".json") && !strings.HasPrefix(rel, "facts/") &&
			!strings.HasPrefix(rel, "extracted/") && !strings.HasPrefix(rel, "provenance/") {
			got = append(got, rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	sort.Strings(got)
	want := []string{
		"data/changes-in-fund-balances.json",
		"data/fund-balances.json",
		"data/revenue-trends.json",
		"fy2024-actual.json",
		"fy2025-revised.json",
		"fy2026-adopted.json",
		"fy2027-adopted.json",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the JSON the site publishes (-want +got):\n%s", diff)
	}
}

// TestStemForAsksHowManyDocumentsNotHowManyColumns pins both directions of the
// naming rule; the second case is the one a column count gets wrong.
//
// The question is whether the projection publishes one document PER YEAR, which
// is answered by the number of SLICES it declared. Asking the column count
// instead looks equivalent -- today every multi-slice projection is
// single-column and the single-slice one is multi-column -- and is not: a store
// carrying one revenue-by-fund column makes the trends document single-column
// too, and the column test would then ship it as revenue-trends-2024.json while
// data/revenue-trends.json, the path the contract promises, did not exist.
func TestStemForAsksHowManyDocumentsNotHowManyColumns(t *testing.T) {
	year := func(y int) project.Options {
		return project.Options{
			Columns: []project.Column{{FiscalYear: y, Basis: project.PublishedBasis}},
		}
	}
	cases := []struct {
		name   string
		o      project.Options
		slices int
		want   string
	}{
		{"one of several years keeps the bare stem when it opens",
			year(project.PublishedFiscalYear), 2, "sankey"},
		{"one of several years is suffixed when it does not", year(2027), 2, "sankey-2027"},
		{"the only document takes the name verbatim, whatever its columns",
			project.Options{Columns: []project.Column{
				{FiscalYear: 2024, Basis: "actual"},
				{FiscalYear: 2025, Basis: "revised"},
			}}, 1, "sankey"},
		// The regression: ONE slice of ONE column is still one document, so it
		// keeps the bare stem even though the year is not the opening one.
		{"one document of one column is not a year of anything", year(2024), 1, "sankey"},
		// fisc-rmx. Two BASES for one fiscal year is the shape that used to
		// compute one stem twice and take the whole export down; the basis is
		// spelled out because a year alone cannot tell them apart. Not a
		// hypothetical: Sankey.Slices derives its slices from (year, basis), so
		// mapping pp.66-67's revised column produces exactly this.
		{"a second basis for one year is not the same document",
			project.Options{Columns: []project.Column{
				{FiscalYear: project.PublishedFiscalYear, Basis: "revised"},
			}}, 3, "sankey-2026-revised"},
		{"a non-opening year keeps its year and gains its basis",
			project.Options{Columns: []project.Column{
				{FiscalYear: 2027, Basis: "revised"},
			}}, 3, "sankey-2027-revised"},
		// Every column, not the first: two documents of several columns each
		// are told apart by all of them.
		{"a multi-column document among several is named by its whole list",
			project.Options{Columns: []project.Column{
				{FiscalYear: 2024, Basis: "actual"},
				{FiscalYear: 2025, Basis: "revised"},
			}}, 2, "sankey-2024-actual-2025-revised"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A stand-in declaration of the right LENGTH. Stem reads only
			// how many there are, and spelling out c.slices plausible
			// Options per case would say nothing the count does not.
			got, err := stemFor("sankey", c.o, make([]project.Options, c.slices))
			if err != nil {
				t.Fatalf("stemFor: %v", err)
			}
			if got != c.want {
				t.Errorf("stemFor = %q, want %q", got, c.want)
			}
		})
	}
}

// TestBuildProjectionsDoesNotRefuseASecondSchedule is fisc-neh, pinned.
//
// A Sliced projection is built over the slices it declares, not over the
// published fiscal years: a trends projection declares one slice of a
// different schedule, and refusing it for lacking the SPINE slice fails
// `fisc export` before it writes a file.
func TestBuildProjectionsDoesNotRefuseASecondSchedule(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	got, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	raw, ok := got[project.TrendsProjection]
	if !ok {
		t.Fatalf("no %q document was built; stems were %v",
			project.TrendsProjection, keys(got))
	}
	// It is not a year of anything, so neither suffixed stem may exist.
	for _, y := range project.PublishedFiscalYears() {
		stem, err := project.Stem(project.TrendsProjection, project.Options{
			Columns: []project.Column{{FiscalYear: y, Basis: project.PublishedBasis}},
		}, make([]project.Options, 2))
		if err != nil {
			t.Fatalf("Stem: %v", err)
		}
		if stem == project.TrendsProjection {
			continue
		}
		if _, ok := got[stem]; ok {
			t.Errorf("a second copy of the trends document was written at %q", stem)
		}
	}
	var doc struct {
		Projection string `json:"projection"`
		Metadata   struct {
			Scope   string `json:"scope"`
			Columns []struct {
				FiscalYear int `json:"fiscal_year"`
			} `json:"columns"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode the trends document: %v", err)
	}
	if doc.Metadata.Scope != project.TrendsScope {
		t.Errorf("scope = %q, want the trends schedule's own %q",
			doc.Metadata.Scope, project.TrendsScope)
	}
	if len(doc.Metadata.Columns) != 4 {
		t.Errorf("got %d columns, want the four pp.127-140 print", len(doc.Metadata.Columns))
	}
}

// TestEveryPublishedDocumentIsRenderedOrDeclaredUnrendered is the assertion
// whose absence let four documents ship unreachable.
//
// published-projection-built and assertPublishedBuilt assert every published
// document was built; neither asks whether a page renders it.
func TestEveryPublishedDocumentIsRenderedOrDeclaredUnrendered(t *testing.T) {
	built := builtStemsForTest(t)
	if err := assertPublishedReachable(mustViews(t, result{Projections: built}), built); err != nil {
		t.Fatalf("the committed corpus: %v", err)
	}

	// A published document that is neither rendered nor declared must be
	// refused. Driving it through a view set with the spine removed is the
	// cheapest way to reach that state without inventing a document.
	if err := assertPublishedReachable(nil, built); err == nil {
		t.Error("no views at all was accepted; every published document is then " +
			"unreachable and only the declared ones may be")
	} else if !strings.Contains(err.Error(), "no view renders it") {
		t.Errorf("got %v, want a refusal naming the unrendered document", err)
	}

	// And a declaration that has stopped being true must go red rather than
	// quiet, which is the property that retires an entry instead of leaving an
	// exemption for whoever forgets. Same standard internal/check's
	// staleDocumentDeclarations applies to uncheckedDocuments.
	// Built from the REAL view set plus one, so the only thing wrong with it is
	// the stale declaration -- starting from a bare slice would trip the
	// missing-view arm above instead and prove nothing about this one.
	stale := mustViews(t, result{Projections: built})
	for stem := range unviewedDocuments {
		stale = append(stale, export.View{Path: "x.html", Projection: stem})
		break
	}
	if err := assertPublishedReachable(stale, built); err == nil {
		t.Error("a view rendering a document unviewedDocuments still declares unrendered " +
			"was accepted; the declaration is then a false statement about the site")
	} else if !strings.Contains(err.Error(), "delete that entry") {
		t.Errorf("got %v, want a refusal telling the caller to delete the declaration", err)
	}

	// A leftover entry naming no published document is the other half of the
	// same property. A MISTYPED entry is already caught by the arm above -- the
	// real document goes undeclared -- but one left behind when a document stops
	// being published is silent, and silence is what this declaration exists to
	// refuse.
	unviewedDocuments["no-such-document"] = "left behind"
	t.Cleanup(func() { delete(unviewedDocuments, "no-such-document") })
	if err := assertPublishedReachable(mustViews(t, result{Projections: built}), built); err == nil {
		t.Error("a declaration naming no published document was accepted")
	} else if !strings.Contains(err.Error(), "publishes no such document") {
		t.Errorf("got %v, want a refusal naming the leftover entry", err)
	}
}

// builtStemsForTest is the real corpus's projections, for the tests that assert
// over what the repository actually publishes rather than over a fixture.
func builtStemsForTest(t *testing.T) map[string][]byte {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	return built
}

// TestViewsOpensOnTheSpineAndGivesYearsToItAlone pins the shape of the view
// list: which page the site opens on, that the year control belongs to the
// spine and to no other view, and that the spine is the one chart page.
func TestViewsOpensOnTheSpineAndGivesYearsToItAlone(t *testing.T) {
	built := builtStemsForTest(t)
	got := mustViews(t, result{Projections: built})

	if len(got) != 5 {
		t.Fatalf("got %d views over %v, want the spine, the revenue trends, the two ACFR "+
			"history tables and the caveats index", len(got), keys(built))
	}
	spine := got[0]
	if spine.Path != export.IndexPath || spine.Projection != export.PrimaryProjection {
		t.Errorf("the site opens on %+v, want the spine at %s", spine, export.IndexPath)
	}
	if len(spine.YearStems) != 2 {
		t.Errorf("the spine view lists %d year stems, want both adopted years", len(spine.YearStems))
	}

	// Ascending is load-bearing: the page opens on the last year stem, and
	// internal/export renders whatever order it is handed.
	years := map[string]int{}
	for _, d := range project.PublishedDocuments() {
		for _, c := range d.Columns {
			years[d.Stem] = c.FiscalYear
		}
	}
	for i, stem := range spine.YearStems {
		if _, ok := years[stem]; !ok {
			t.Errorf("the spine lists year stem %q, which names no published document", stem)
			continue
		}
		if i > 0 && years[stem] <= years[spine.YearStems[i-1]] {
			t.Errorf("the spine lists %q (FY%d) after %q (FY%d); the years must ascend, "+
				"because the page opens on the last of them",
				stem, years[stem], spine.YearStems[i-1], years[spine.YearStems[i-1]])
		}
	}

	// By path rather than index: the nav order is a design decision, not this test's.
	byPath := map[string]export.View{}
	for _, v := range got {
		byPath[v.Path] = v
	}

	// Titles and ledes are the caller's, not the packager's.
	for _, path := range []string{"trends.html", "history.html", "balances.html"} {
		v, ok := byPath[path]
		if !ok {
			paths := make([]string, 0, len(got))
			for _, x := range got {
				paths = append(paths, x.Path)
			}
			t.Fatalf("no view at %s; the nav is %v", path, paths)
		}
		if v.Title == "" || v.Lede == "" {
			t.Errorf("the %s view ships no title or lede, so the page would head itself", path)
		}
	}

	// fund-flows is drawn by opening the spine; a page of its own would publish
	// the same money twice.
	for _, path := range []string{"revenue.html", "spending.html", "drilldown.html"} {
		if _, ok := byPath[path]; ok {
			t.Errorf("%s is in the nav; the spine's chain is where that document is drawn now", path)
		}
	}
	for _, v := range got {
		if v.Projection == project.FundFlowsProjection {
			t.Errorf("view %q renders fund-flows as its own page; it is drawn by opening the spine", v.Path)
		}
	}

	// The chain's actual values, because the client's tests measure against
	// them. Which document each year draws is export.ColumnIndex's, measured by
	// TestOpensIntoJoinsOnColumnNotOnDeclaredOrder.
	want := []export.DrillStep{
		{
			Key:        "fund-group",
			After:      []string{""},
			From:       2,
			Projection: project.FundFlowsProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Keep:  []int{0},
				Tiers: []int{0, 2, 3, 4, 5},
				Widen: []int{4, 5},
				Caps: []export.TierCap{{Tier: 3, Cap: 8}, {Tier: 4, Cap: 24, Tail: "divisions"},
					{Tier: 5, Cap: 8, Tail: "object rows"}},
			}},
			Noun: "fund group",
			Back: "All fund groups",
			Tail: "funds",
			// Derived from the cuts and exceptions, which are the declaration.
			Residual:      must[map[string]string](t)(project.FundFlowsResidual()),
			ResidualGrain: "fund",
			Description: "The revenue categories on the left are the citywide chart's own " +
				"cells; this fund group is the mark in the middle, and its own funds are " +
				"on the right, rescaled to the group's total \u2014 the citywide chart " +
				"cannot show them, because the General Fund alone is half the fund column " +
				"and the smallest fund is less than a thirty-thousandth of it. Money " +
				"Budget Book pp.127-140 print for no fund at all passes the group's mark " +
				"to a node of its own beside the funds, so what the group takes in here " +
				"is what its funds take in. Every fund a department draws on opens " +
				"further: the General Fund into the divisions that spend it, from Budget " +
				"Book pp.167-170, and every other fund into the departments it pays for, " +
				"from pp.85-125. Any other fund ends the drill. " +
				"Where there is room for more columns, the General Fund's divisions " +
				"from pp.167-170 are drawn beyond its funds, and beyond them the " +
				"object categories each fund spends on: the General Fund's through " +
				"its divisions, every other fund's straight from pp.173-183, which " +
				"print no division.",
		},
		{
			Key:   "fund",
			After: []string{"fund-group"},
			From:  3,
			Role:  "general_fund",
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Keep:  []int{2},
				Tiers: []int{2, 3, 4, 5},
				Widen: []int{5},
				Caps:  []export.TierCap{{Tier: 4, Cap: 24}, {Tier: 5, Cap: 8, Tail: "object rows"}},
			}},
			Noun: "fund",
			Back: "All funds",
			Tail: "divisions",
			Description: "The fund group this fund belongs to is on the left and the " +
				"divisions that spend it are on the right \u2014 that fund's rows of " +
				"Budget Book pp.167-170, rescaled to its total. The two sides of the " +
				"fund in the middle are not one figure: what it takes in is its revenue " +
				"and what leaves it here is what its divisions spend. The difference is " +
				"what pp.66-67 print for the fund group as a whole \u2014 the money the city " +
				"transfers out and sets aside in its balances and reserves \u2014 less the " +
				"money the group takes in that no fund receives, which the chart above " +
				"carries to a node of its own beside the funds.",
		},
		{
			Key:   "division",
			After: []string{"fund"},
			From:  4,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Keep:  []int{3},
				Tiers: []int{3, 4, 5},
				Caps:  []export.TierCap{{Tier: 5, Cap: 8}},
			}},
			Noun: "division",
			Back: "All divisions",
			Tail: "categories",
			Description: "The fund that pays for this division is on the left and the " +
				"object categories it spends on are on the right \u2014 that division's " +
				"cells of Budget Book pp.167-170, rescaled to its total.",
		},
		{
			Key:        "revenue-category",
			After:      []string{""},
			From:       0,
			Role:       "revenue_source",
			Projection: project.FundFlowsProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Keep:  []int{2},
				Tiers: []int{1, 0, 2},
				Caps:  []export.TierCap{{Tier: 1, Cap: 8}},
			}},
			Noun: "revenue category",
			Back: "All revenue categories",
			Tail: "lines",
			Description: "The lines Budget Book pp.127-140 print under this revenue " +
				"category are on the left; the fund groups its money reaches are on the " +
				"right, as the citywide chart draws them. The category itself is the mark " +
				"in the middle, and the two sides of it are the same figure read from two " +
				"schedules. A line the schedule prints as a reduction is drawn in red at " +
				"its printed size and named as one, and the category's own mark is the " +
				"figure net of them \u2014 the same one the citywide chart labels it with.",
		},
		{
			Key:        "object-category",
			After:      []string{""},
			From:       5,
			Role:       "object_category",
			Projection: project.DepartmentSpendingProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Keep:  []int{2},
				Tiers: []int{2, 5, 4},
				Caps:  []export.TierCap{{Tier: 4, Cap: 8}},
			}},
			Noun: "object category",
			Back: "All object categories",
			Tail: "divisions",
			// Derived from the exceptions cuts-tie-along-the-lattice pins.
			Gaps: must[map[string][]project.Gap](t)(project.SpendingGaps()),
			Description: "The fund groups that pay for this object category are on the " +
				"left; the divisions that spend it are on the right \u2014 Budget Book " +
				"pp.85-125's rows for this category, every division in the city that " +
				"has one, rescaled to the category's total. The two columns are read " +
				"from different schedules, and the right-hand one prints what a " +
				"division spends whatever pays for it: it carries no fund at all, so " +
				"no division here takes the colour of a fund group.",
		},
		{
			// No flank because it opens a source: validateSteps refuses Keep
			// and Side together.
			Key:   "transfers",
			After: []string{""},
			From:  0,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Side:  export.SideSource,
				Tiers: []int{2, 3},
			}},
			Role:       "transfer_in",
			Projection: project.TransfersByFundProjection,
			Noun:       "money coming in",
			Back:       "All money coming in",
			Tail:       "funds",
			Description: "Budget Book p76, Summary of Transfers: the funds that pay each " +
				"transfer the city makes to itself are on the left, and the funds that " +
				"receive them are on the right. One ribbon is one figure the page prints, " +
				"and a fund that both pays and receives is drawn once on each side, under " +
				"the same name. This is the money coming IN, which is what the mark on " +
				"the citywide chart counts.",
		},
		{
			// Shares (After, From) with object-category, which validateSteps
			// admits because the roles differ.
			Key:        "transfers-out",
			After:      []string{""},
			From:       5,
			Role:       "transfer_out",
			Projection: project.TransfersOutProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Tiers: []int{3, 5},
				Caps:  []export.TierCap{{Tier: 3, Cap: 10}, {Tier: 5, Cap: 10}},
			}},
			Noun: "money going out",
			Back: "All money going out",
			Tail: "funds",
			Description: "Budget Book p76 and p222: the funds that pay each transfer the " +
				"city makes are on the left, and the funds that receive them are on the " +
				"right. p76 lists the transfers between operating funds and p222 the " +
				"transfers to the Capital Improvement Program, whose funds are not on the " +
				"citywide chart; the two lists together are the Transfers Out it counts. " +
				"One ribbon is one figure a page prints.",
		},
		{
			// Shares (After, From) with `fund`, which validateSteps admits
			// because the roles differ. No caps, measured: the widest fund it
			// opens draws 5 departments (fund/240, FY2023-24 actual).
			Key:        "fund-departments",
			After:      []string{"fund-group"},
			From:       3,
			Role:       "fund",
			Projection: project.DepartmentFundingProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Keep:  []int{2},
				Tiers: []int{2, 3, 4},
			}},
			Noun: "fund",
			Back: "All funds",
			Tail: "departments",
			Description: "The fund group this fund belongs to is on the left and the " +
				"city departments it pays for are on the right — that fund's rows " +
				"of Budget Book pp.85-125, rescaled to its total. The two sides of the " +
				"fund in the middle are read from two different schedules and are not " +
				"one figure: what it takes in, from pp.127-140, and what the " +
				"departments draw on it here. Either side may be the larger. " +
				"A department here is the WHOLE department across every fund that " +
				"pays it, which is a coarser thing than the divisions the General Fund " +
				"opens into — five names belong to both tiers, so do not read one " +
				"as the other.",
		},
		{
			// A source, as transfers is, and capped as the fund-group step's
			// fund column is.
			Key:        "balance-draw",
			After:      []string{""},
			From:       0,
			Role:       "fund_balance_draw",
			Projection: project.FundSourcesUsesProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Side:  export.SideSource,
				Tiers: []int{0, 3},
				Caps:  []export.TierCap{{Tier: 3, Cap: 8}},
			}},
			Noun: "draw on fund balances",
			Back: "All money coming in",
			Tail: "funds",
			Description: "Budget Book pp.186-209 print each fund's balance at the start and the " +
				"end of the year, and the funds on the right are those whose balance falls, each " +
				"drawn at its own fall: the beginning balance less the ending one, which the city " +
				"prints as two figures and not as one. These are each fund's own draw, gross, " +
				"where the citywide chart's Fund Balance Draw is net within each fund group, so " +
				"the funds here sum to more than the mark they were opened from; a caveat gives " +
				"their gross sum and their sum netted within each group. Every fund opens into where its own money comes from and goes.",
		},
		{
			// Shares (After, From) with object-category and transfers-out, told
			// apart by role.
			Key:        "balance-contribution",
			After:      []string{""},
			From:       5,
			Role:       "fund_balance_contribution",
			Projection: project.FundSourcesUsesProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Tiers: []int{3, 5},
				Caps:  []export.TierCap{{Tier: 3, Cap: 8}},
			}},
			Noun: "contribution to fund balances",
			Back: "All money going out",
			Tail: "funds",
			Description: "Budget Book pp.186-209 print each fund's balance at the start and the " +
				"end of the year, and the funds on the left are those whose balance rises, each " +
				"drawn at its own rise: the ending balance less the beginning one, which the city " +
				"prints as two figures and not as one. These are each fund's own contribution, " +
				"gross, where the citywide chart's Fund Balance Contribution is net within each " +
				"fund group, so the funds here sum to more than the mark they were opened from; " +
				"a caveat gives their gross sum and their sum netted within each group. Every fund opens into where its own money comes " +
				"from and goes.",
		},
		{
			// One ribbon, the General Fund's: no cap.
			Key:        "reserve",
			After:      []string{""},
			From:       5,
			Role:       "reserve_increase",
			Projection: project.FundSourcesUsesProjection,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Tiers: []int{3, 5},
			}},
			Noun: "addition to reserves",
			Back: "All money going out",
			Tail: "funds",
			Description: "The funds on the left are those Budget Book pp.186-209 print an " +
				"increase in reserves for, each at the figure its own page prints. Every fund " +
				"opens into where its own money comes from and goes.",
		},
		{
			// Both sides of the opened fund, off the document before it: no
			// projection, no role, no flank.
			Key:   "fund-balance",
			After: []string{"balance-draw", "balance-contribution", "reserve"},
			From:  3,
			Chart: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{
				Side:  export.SideBoth,
				Tiers: []int{0, 3, 5},
			}},
			Noun: "fund",
			Back: "All funds",
			Tail: "lines",
			Description: "Where this fund's money comes from is on the left and where it goes is " +
				"on the right, each ribbon one line its page of Budget Book pp.186-209 prints. " +
				"The page prints the fund's balance at the start and the end of the year rather " +
				"than the change between them, so the draw on its balance or the contribution to " +
				"it is the difference of those two printed figures, drawn as one ribbon and " +
				"marked as ours. That difference is what makes the two sides of the fund one " +
				"figure.",
		},
	}
	if diff := cmp.Diff(want, spine.Steps); diff != "" {
		t.Errorf("the spine's steps (-want +got):\n%s\nthe client's tests measure "+
			"against this", diff)
	}
	// Pinned by hand: every kept flank's adjacency is read against this order.
	if diff := cmp.Diff([]int{0, 2, 5}, spine.Overview.Sankey.Tiers); diff != "" {
		t.Errorf("the spine's render tiers (-want +got):\n%s", diff)
	}
}

// TestOpensIntoJoinsOnColumnNotOnDeclaredOrder (fisc-zojk): a year's step
// document is joined on Column, not on its position in PublishedDocuments. A
// join on declared order pairs sankey with fund-flows-2024-actual.
func TestOpensIntoJoinsOnColumnNotOnDeclaredOrder(t *testing.T) {
	built := builtStemsForTest(t)

	position := -1
	var declared []string
	for _, d := range project.PublishedDocuments() {
		if d.Projection != project.FundFlowsProjection {
			continue
		}
		if d.Stem == project.FundFlowsProjection {
			position = len(declared)
		}
		declared = append(declared, d.Stem)
	}
	if position != 2 {
		t.Errorf("fund-flows' bare stem is declared at position %d of %v, want 2; the "+
			"premise that yearStems would put it third has moved", position, declared)
	}

	if !opensInto(export.PrimaryProjection, project.FundFlowsProjection, built) {
		t.Error("opensInto says the spine's opening year has no fund-flows document to open into")
	}
	// The opening year joins to the bare stem, not the first declared.
	_, ix, err := export.ColumnsOf(built, "fisc test")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	col, folded := ix.Column(export.PrimaryProjection)
	if !folded {
		t.Fatalf("the spine's opening document folded into no column")
	}
	stem, ok := ix.Stem(col, project.FundFlowsProjection)
	if !ok || stem != declared[position] {
		t.Errorf("the opening year opens into %q (found %v), want the bare stem at position %d",
			stem, ok, position)
	}

	// A year whose step document was not built is refused by name, not dropped.
	short := map[string][]byte{}
	for k, v := range built {
		if k != "fund-flows-2027" {
			short[k] = v
		}
	}
	if !opensInto(export.PrimaryProjection, project.FundFlowsProjection, short) {
		t.Error("dropping the SECOND year's document must not undeclare the step")
	}
	opts, _, _, _ := testOptions(t)
	opts.Build = func(string) (result, error) { return result{Projections: short}, nil }
	if err := exportRun(opts); err == nil {
		t.Error("exportRun accepted a spine whose second year has no step document")
	} else if !strings.Contains(err.Error(), `carries no such schedule`) {
		t.Errorf("got %v, want the refusal naming the year's column", err)
	}

	// No drill into a document that was not built, guarded per projection.
	without := func(prefixes ...string) map[string][]byte {
		out := map[string][]byte{}
		for k, v := range built {
			if !slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(k, p) }) {
				out[k] = v
			}
		}
		return out
	}
	for _, tc := range []struct {
		name string
		drop []string
		want []string
	}{
		// fund-departments needs both pp.85-125 and the pp.127-140 chart it opens from.
		{"no fund-flows", []string{project.FundFlowsProjection},
			[]string{"object-category", "transfers", "transfers-out", "balance-draw", "balance-contribution", "reserve", "fund-balance"}},
		{"no department-spending", []string{project.DepartmentSpendingProjection},
			[]string{"fund-group", "fund", "division", "revenue-category", "transfers",
				"transfers-out", "fund-departments", "balance-draw", "balance-contribution", "reserve", "fund-balance"}},
		{"no transfers-by-fund", []string{project.TransfersByFundProjection},
			[]string{"fund-group", "fund", "division", "revenue-category", "object-category",
				"transfers-out", "fund-departments", "balance-draw", "balance-contribution", "reserve", "fund-balance"}},
		{"no transfers-out", []string{project.TransfersOutProjection},
			[]string{"fund-group", "fund", "division", "revenue-category", "object-category",
				"transfers", "fund-departments", "balance-draw", "balance-contribution", "reserve", "fund-balance"}},
		{"no department-funding", []string{project.DepartmentFundingProjection},
			[]string{"fund-group", "fund", "division", "revenue-category", "object-category",
				"transfers", "transfers-out", "balance-draw", "balance-contribution", "reserve", "fund-balance"}},
		{"neither detail document", []string{project.FundFlowsProjection,
			project.DepartmentSpendingProjection}, []string{"transfers", "transfers-out", "balance-draw", "balance-contribution", "reserve", "fund-balance"}},
		{"no fund-sources-uses", []string{project.FundSourcesUsesProjection},
			[]string{"fund-group", "fund", "division", "revenue-category", "object-category",
				"transfers", "transfers-out", "fund-departments"}},
		{"no step document at all", []string{project.FundFlowsProjection,
			project.DepartmentSpendingProjection, project.TransfersByFundProjection,
			project.TransfersOutProjection, project.DepartmentFundingProjection,
			project.FundSourcesUsesProjection}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stepKeys(mustViews(t, result{Projections: without(tc.drop...)})[0].Steps)
			if len(got) == 0 {
				got = nil
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("the spine's steps (-want +got):\n%s", diff)
			}
		})
	}
}

// stepKeys names a step list for a failure message.
func stepKeys(steps []export.DrillStep) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Key)
	}
	return out
}

// TestAPageThatDisclaimsAuditAssuranceDoesNotClaimItInItsProse couples the two
// statements of one claim that print on one page: the caveat and the lede.
//
// The two ACFR ten-year tables draw the section the ACFR itself labels
// (Unaudited), and each of their documents ships the
// statistical-section-unaudited caveat, whose text quotes the auditor: "we do
// not express an opinion or any form of assurance thereon." The Title and Lede
// are composed here, out of any document's sight -- nothing else stops them
// asserting the assurance the caveat, further down the same page, disclaims.
//
// A PIN RATHER THAN A BANNED-WORD LIST, for
// TestTheLedesProseNamesThePagesScope's reason (internal/export/views_test.go).
// The scan is conditional on the document's own declaration: a view whose
// document ships no such caveat may say "audited" freely, and "unaudited" is
// the caveat's own word, so it is lifted out before the scan. The anchor half
// fails first if the caveat itself disappears, so the scan cannot rot into a
// vacuous pass.
func TestAPageThatDisclaimsAuditAssuranceDoesNotClaimItInItsProse(t *testing.T) {
	built := builtStemsForTest(t)

	const disclaimer = project.UnauditedCaveatID
	shipping := map[string]bool{}
	for _, v := range mustViews(t, result{Projections: built}) {
		if v.Projection == "" {
			continue
		}
		doc, ok := built[v.Projection]
		if !ok {
			continue
		}
		// Every builder in internal/project nests its caveats in its metadata
		// struct, so this one shape reads them all.
		var d struct {
			Metadata struct {
				Caveats []struct {
					ID string `json:"id"`
				} `json:"caveats"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(doc, &d); err != nil {
			t.Fatalf("unmarshal %s's document %s: %v", v.Path, v.Projection, err)
		}
		for _, c := range d.Metadata.Caveats {
			if c.ID != disclaimer {
				continue
			}
			shipping[v.Path] = true
			prose := strings.ToLower(v.Title + " " + v.Lede)
			prose = strings.ReplaceAll(prose, "unaudited", "")
			if strings.Contains(prose, "audit") {
				t.Errorf("%s heads itself with an audit claim (title %q, lede %q) while "+
					"its document ships the %s caveat: the auditor's opinion does not "+
					"cover these schedules, and the page would disclaim its own headline",
					v.Path, v.Title, v.Lede, disclaimer)
			}
		}
	}
	// The anchor: the two ACFR tables' documents still declare themselves
	// unaudited. If this half fails, the scan above has lost its subject --
	// decide whether the prose may now claim assurance before deleting it.
	for _, path := range []string{"history.html", "balances.html"} {
		if !shipping[path] {
			t.Errorf("%s's document no longer ships the %s caveat; the prose scan above "+
				"and TestAPageDoesNotLabelAColumnWithAWordItsCaveatWithdraws are both "+
				"checking nothing on the page they were written for", path, disclaimer)
		}
	}
}

// basisChipPattern and markTitlePattern are the two surfaces a column's basis
// word reaches a reader through: the chip in the column head
// (site/history.html.tmpl, `<span class="basis">`) and the native SVG tooltip on
// every drawn mark, which internal/export composes as label + basis + value.
var (
	basisChipPattern = regexp.MustCompile(`<span class="basis">([^<]*)</span>`)
	markTitlePattern = regexp.MustCompile(`<(?:circle|rect)\b[^>]*>\s*<title>([^<]*)</title>`)
)

// TestAPageDoesNotLabelAColumnWithAWordItsCaveatWithdraws is the second half of
// the audit-claim refusal, and it exists because the first half could not see
// the defect it was written for.
//
// TestAPageThatDisclaimsAuditAssuranceDoesNotClaimItInItsProse reads each view's
// Title and Lede. Both ACFR ten-year pages passed it while heading all ten
// columns `audited` and repeating the word in every cell tooltip, because those
// are rendered from the document's column data and not from the view's prose --
// a page making two contradictory claims about the same ten columns, with the
// caveat quoting p161's "(Unaudited)" a few paragraphs below (fisc-97n8).
//
// IT WALKS THE RENDERED PAGES, for TestEveryCaveatSummaryLinksToAnAnchorThatExists'
// reason: the word is chosen in internal/export and printed by a template, and
// the page is the only place the two meet. Scanning the Go structs would let a
// template start printing the document's raw basis again with this green.
//
// The match counts are asserted because a regex that has stopped matching is
// indistinguishable by exit code from a page that has stopped offending.
func TestAPageDoesNotLabelAColumnWithAWordItsCaveatWithdraws(t *testing.T) {
	built := builtStemsForTest(t)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	io, _, _, _ := iostreams.Test()
	opts := &Options{
		IO:        io,
		RepoRoot:  func() (string, error) { return root, nil },
		OutputDir: filepath.Join(t.TempDir(), "dist"),
		Build:     func(string) (result, error) { return result{Projections: built}, nil },
	}
	if err = exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}

	scanned := 0
	for _, v := range mustViews(t, result{Projections: built}) {
		if v.Projection == "" {
			continue
		}
		doc, ok := built[v.Projection]
		if !ok {
			continue
		}
		var d struct {
			Metadata struct {
				Caveats []struct {
					ID string `json:"id"`
				} `json:"caveats"`
			} `json:"metadata"`
		}
		if err = json.Unmarshal(doc, &d); err != nil {
			t.Fatalf("unmarshal %s's document %s: %v", v.Path, v.Projection, err)
		}
		if !slices.ContainsFunc(d.Metadata.Caveats, func(c struct {
			ID string `json:"id"`
		},
		) bool {
			return c.ID == project.UnauditedCaveatID
		}) {
			continue
		}
		scanned++

		b, err := os.ReadFile(filepath.Join(opts.OutputDir, filepath.FromSlash(v.Path)))
		if err != nil {
			t.Fatalf("read rendered %s: %v", v.Path, err)
		}
		page := string(b)

		for _, surface := range []struct {
			what string
			re   *regexp.Regexp
		}{
			{what: "column chip", re: basisChipPattern},
			{what: "cell tooltip", re: markTitlePattern},
		} {
			hits := surface.re.FindAllStringSubmatch(page, -1)
			if len(hits) == 0 {
				t.Errorf("%s renders no %s this scan can read; the surface moved and "+
					"this half of the refusal is checking nothing", v.Path, surface.what)
				continue
			}
			for _, h := range hits {
				// "unaudited" contains "audited", so it is removed before the
				// question is asked rather than special-cased after it.
				if strings.Contains(strings.ReplaceAll(strings.ToLower(h[1]), "unaudited", ""), "audit") {
					t.Errorf("%s prints %q in a %s while its document ships the %s caveat: "+
						"the auditor's opinion does not cover these columns, and the page "+
						"withdraws its own label a few paragraphs below",
						v.Path, h[1], surface.what, project.UnauditedCaveatID)
					break
				}
			}
		}
	}
	// The anchor, for the prose scan's reason: if no page ships the caveat the
	// loop above never runs, and a scan over nothing passes.
	if scanned != 2 {
		t.Errorf("scanned %d pages shipping the %s caveat, want 2 (history.html and "+
			"balances.html); this refusal has lost its subject", scanned, project.UnauditedCaveatID)
	}
}

// TestAViewWhoseDocumentWasNotBuiltIsDropped: a nav entry pointing at a page
// that was not written is a 404 a reader can click, and it is the one failure
// this function must never produce. Dropping the view is what prevents it;
// `fisc verify` is what says the document is missing.
func TestAViewWhoseDocumentWasNotBuiltIsDropped(t *testing.T) {
	only := map[string][]byte{export.PrimaryProjection: {}}
	got := mustViews(t, result{Projections: only})

	// EVERY VIEW THAT NAMES A PROJECTION IS THE SPINE'S, and that is the
	// assertion rather than a count: a view naming NO projection, the caveats
	// index, has nothing that could fail to be built and so nothing to drop.
	// A count would fail this test for a reason it is not about, and the
	// shortest way to green would be to make the caveats page conditional on
	// a document it does not have.
	for _, v := range got {
		if v.Projection == "" {
			continue
		}
		if v.Projection != export.PrimaryProjection {
			t.Errorf("view %q renders %q, which was not built", v.Path, v.Projection)
		}
		for _, stem := range v.YearStems {
			if _, ok := only[stem]; !ok {
				t.Errorf("view %q lists year stem %q, which was not built", v.Path, stem)
			}
		}
	}
	if got[0].Path != export.IndexPath {
		t.Errorf("the site opens on %q, want %q", got[0].Path, export.IndexPath)
	}
	// The views whose documents were not built really are gone: the whole
	// point. Named by their CURRENT paths, because a path that no longer exists
	// is an arm that can never match -- and this comment made exactly that
	// claim while the line below still named drilldown.html, a page views()
	// stopped emitting in the same commit. Twice in two commits, which is why
	// the list is now derived from the views the full set produces rather than
	// typed out.
	dropped := map[string]bool{}
	for _, v := range mustViews(t, result{Projections: map[string][]byte{
		export.PrimaryProjection:       {},
		project.TrendsProjection:       {},
		project.FundFlowsProjection:    {},
		project.ChangesProjection:      {},
		project.FundBalancesProjection: {},
	}}) {
		if v.Projection != "" && v.Projection != export.PrimaryProjection {
			dropped[v.Path] = true
		}
	}
	if len(dropped) == 0 {
		t.Fatal("no view names a projection other than the spine, so this test asserts nothing")
	}
	for _, v := range got {
		if dropped[v.Path] {
			t.Errorf("view %q survived with its document unbuilt; a nav entry pointing at "+
				"a page that was not written is a 404 a reader can click", v.Path)
		}
	}
}

// TestExportRefusesAPublishedDocumentThatWasNotBuilt is the export half of
// fisc-w7d, and it is the half a reader meets first: `fisc verify` says a
// document is missing, but `fisc export` is what would otherwise ship a site
// without it.
//
// Both arms matter and they are different repairs. A stem absent is a projection
// that built nothing at all. A stem present but short a column is the worse one:
// the file lands at the path the contract promises, every downstream check
// compares each series against the columns THAT DOCUMENT declares, so the site
// agrees with itself about a chart that is missing a year.
func TestExportRefusesAPublishedDocumentThatWasNotBuilt(t *testing.T) {
	// The real published set, built exactly as declared, is the control: if this
	// does not pass, neither arm below is evidence of anything.
	full := map[string]builtDoc{}
	for _, d := range project.PublishedDocuments() {
		full[d.Stem] = builtDoc{
			name: d.Projection,
			opts: project.Options{Columns: d.Columns, Scopes: d.Scopes},
		}
	}
	if err := assertPublishedBuilt(full); err != nil {
		t.Fatalf("the declared published set is refused as built: %v", err)
	}

	for _, tc := range []struct {
		name  string
		drop  func(map[string]builtDoc)
		wants []string
	}{
		{
			name: "a whole document missing",
			drop: func(m map[string]builtDoc) { delete(m, project.TrendsProjection) },
			wants: []string{project.TrendsProjection, "no projection built it",
				project.PublishedProjection},
		},
		{
			name: "a document short one published column",
			drop: func(m map[string]builtDoc) {
				b := m[project.TrendsProjection]
				b.opts.Columns = b.opts.Columns[1:]
				m[project.TrendsProjection] = b
			},
			wants: []string{project.TrendsProjection, "missing",
				project.Describe(project.TrendsColumns()[:1])},
		},
		{
			name: "a document built over another schedule entirely",
			drop: func(m map[string]builtDoc) {
				b := m[project.TrendsProjection]
				b.opts.Scopes = []string{project.PublishedScope}
				m[project.TrendsProjection] = b
			},
			wants: []string{project.TrendsProjection, "missing"},
		},
		{
			// The spine is not special-cased, and this is what says so.
			name: "a published spine year missing",
			drop: func(m map[string]builtDoc) {
				delete(m, "sankey-2027")
			},
			wants: []string{"sankey-2027", "no projection built it"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			built := map[string]builtDoc{}
			for stem, b := range full {
				built[stem] = builtDoc{name: b.name, opts: project.Options{
					Columns: slices.Clone(b.opts.Columns), Scopes: slices.Clone(b.opts.Scopes),
				}}
			}
			tc.drop(built)

			err := assertPublishedBuilt(built)
			if err == nil {
				t.Fatal("no error; the site would ship without a document it publishes")
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// TestPublishedDocumentsAreWhatTheCorpusBuilds pins project.PublishedDocuments
// against the documents the committed corpus actually writes, read back out of
// the shipped BYTES rather than out of the Options they were built from.
//
// WHY THIS TEST HAS TO EXIST. project.TrendsColumns states four columns instead
// of deriving them, and that is deliberate: a published set derived from the
// facts agrees with the corpus by construction and goes quiet in exactly the
// state it exists to catch. The cost of stating them is that they can go stale,
// and nothing else would notice — TestRegistry compares projection NAMES, and
// published-projection-built only asks whether the corpus covers the
// declaration, so a fifth mapped column would build, ship, and sit outside the
// published set in silence.
//
// This is the other direction: the corpus may not cover MORE than the site says
// it publishes. When it legitimately does — a fifth column mapped, a third
// document — this test is what fails, and the repair is to widen the
// declaration in the commit that widened the corpus.
func TestPublishedDocumentsAreWhatTheCorpusBuilds(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}

	for _, d := range project.PublishedDocuments() {
		raw, ok := built[d.Stem]
		if !ok {
			t.Errorf("the site publishes %s and the corpus built no document at that stem; "+
				"stems were %v", d, keys(built))
			continue
		}
		var doc struct {
			Metadata struct {
				// The spine publishes ONE column and names it singularly;
				// the trends publish four and carry a list. A document of one
				// shape decoded through the other leaves its field zero, which
				// is why both are read and exactly one is expected to be set.
				FiscalYear int    `json:"fiscal_year"`
				Basis      string `json:"basis"`
				// The spine and the trends publish ONE schedule and name it
				// singularly; the drill-down publishes two and carries a list.
				// Exactly one of the two keys is present on any document, which
				// is what MultiScopeEnvelope exists to keep true -- a document
				// of two schedules writing Scopes[0] into a singular key would
				// name one as the whole of it.
				Scope   string   `json:"scope"`
				Scopes  []string `json:"scopes"`
				Columns []struct {
					FiscalYear int    `json:"fiscal_year"`
					Basis      string `json:"basis"`
				} `json:"columns"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Errorf("decode %s: %v", d.Stem, err)
			continue
		}
		shippedScopes := doc.Metadata.Scopes
		if doc.Metadata.Scope != "" {
			shippedScopes = []string{doc.Metadata.Scope}
		}
		if diff := cmp.Diff(d.Scopes, shippedScopes); diff != "" {
			t.Errorf("%s ships scopes (-declared +shipped):\n%s", d.Stem, diff)
		}
		shipped := make([]project.Column, 0, len(doc.Metadata.Columns))
		for _, c := range doc.Metadata.Columns {
			shipped = append(shipped, project.Column{
				FiscalYear: c.FiscalYear, Basis: mapping.Basis(c.Basis)})
		}
		if len(shipped) == 0 && doc.Metadata.FiscalYear != 0 {
			shipped = append(shipped, project.Column{
				FiscalYear: doc.Metadata.FiscalYear, Basis: mapping.Basis(doc.Metadata.Basis)})
		}
		if diff := cmp.Diff(d.Columns, shipped); diff != "" {
			t.Errorf("%s: the declared columns and the shipped ones differ (-declared +shipped)"+
				":\n%s\nIf the corpus legitimately grew, widen project.PublishedDocuments in "+
				"the same commit; the declaration is what the site PROMISES and it may not "+
				"trail what it serves", d.Stem, diff)
		}
	}

	// And nothing published is left over: a document the corpus builds at a stem
	// no declaration names is shipped and unguarded, which is the whole of
	// fisc-w7d from the other end.
	declared := map[string]bool{}
	for _, d := range project.PublishedDocuments() {
		declared[d.Stem] = true
	}
	for stem := range built {
		if !declared[stem] {
			t.Errorf("the corpus builds a document at %q that project.PublishedDocuments does "+
				"not name, so `fisc verify` asserts nothing about it", stem)
		}
	}
}

// TestStemForRefusesADocumentWithNoColumns covers `o.Columns[0]`, refused
// rather than indexed blind.
//
// A zero-column Options is not reachable through project.Slices today, but a
// projection is an interface any future type can satisfy, and the failure mode
// of `o.Columns[0]` is a PANIC inside the command whose job is writing files --
// which is worse than any refusal, because it leaves a half-exported directory
// and no message a reader could act on. It is refused with the reason instead.
func TestStemForRefusesADocumentWithNoColumns(t *testing.T) {
	// More than one document, or the one-document branch returns before the
	// index.
	_, err := stemFor("sankey", project.Options{}, make([]project.Options, 2))
	if err == nil {
		t.Fatal("stemFor accepted an Options with no columns, want a refusal")
	}
	for _, want := range []string{"sankey", "no columns"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("stemFor error = %q, want it to name %q", err, want)
		}
	}

	// And one column still works, so the guard has not swallowed the real case.
	got, err := stemFor("sankey", project.Options{
		Columns: []project.Column{{FiscalYear: 2027, Basis: "adopted"}},
	}, make([]project.Options, 2))
	if err != nil || got != "sankey-2027" {
		t.Errorf("stemFor = %q, %v; want \"sankey-2027\", nil", got, err)
	}
}

// TestTheFundFlowsFixtureIsTheDocumentTheSiteDraws keeps
// testdata/fund-flows.golden.json honest.
//
// THAT FIXTURE IS A CAPTURE, NOT A DERIVATION, and this test is what makes the
// difference bearable. testdata/sankey.golden.json is hand-derived from
// internal/project's spineRows fixture, so reproducing that golden proves the
// projection reads those rows correctly. Nothing comparable is affordable
// here: the drill-down is 280 facts over 18 pages, and a hand-authored table
// of them would be a transcription of the same schedules the mapping rules
// already read.
//
// So the fixture exists as the real drill-down internal/export's view tests
// take as input, and this test keeps it the one buildProjections writes:
// without it, a step could be proved against a document that stopped being
// ours.
func TestTheFundFlowsFixtureIsTheDocumentTheSiteDraws(t *testing.T) {
	fixtureIsTheDocumentExported(t, project.FundFlowsProjection, "fund-flows.golden.json")
}

// fixtureIsTheDocumentExported compares one committed capture line for line
// with what buildProjections writes at `stem`.
//
// metadata.generated_by is excluded because it carries the commit and the build
// time, which no committed file can match. Nothing else is excluded: the nodes,
// the links, the counts, the caveats and the key order are all compared.
func fixtureIsTheDocumentExported(t *testing.T, stem, fixture string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	got, ok := built[stem]
	if !ok {
		t.Fatalf("buildProjections did not build %q", stem)
	}
	want, err := os.ReadFile(filepath.Join(root, "testdata", fixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	stamp := regexp.MustCompile(`"generated_by": "[^"]*"`)
	blank := []byte(`"generated_by": ""`)
	if diff := cmp.Diff(
		strings.Split(string(stamp.ReplaceAll(want, blank)), "\n"),
		strings.Split(string(stamp.ReplaceAll(got, blank)), "\n"),
	); diff != "" {
		// fisc export folds the document into its column and writes no file of
		// its own, so the built bytes are written here with the fixture's stamp.
		rebuilt := filepath.Join(root, "bin", strings.TrimSuffix(fixture, ".json")+"-rebuilt.json")
		if err := os.MkdirAll(filepath.Dir(rebuilt), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rebuilt, stamp.ReplaceAll(got, stamp.Find(want)), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Errorf("testdata/%s is no longer what buildProjections builds at %s (-fixture +built); "+
			"regenerate with `cp -f %s testdata/%s` and read the diff:\n%s", fixture, stem, rebuilt, fixture, diff)
	}
}

// TestEveryServedStampIsTheExportsOwn holds one export's build stamps to each
// other: the client refuses a column whose stamp is not its page's.
func TestEveryServedStampIsTheExportsOwn(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if runErr := exportRun(opts); runErr != nil {
		t.Fatalf("exportRun: %v", runErr)
	}
	exported := regexp.MustCompile(`"exported_by":"([^"]*)"`)
	stamps := map[string]string{}
	walkErr := filepath.WalkDir(opts.OutputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(opts.OutputDir, path)
		switch filepath.Ext(path) {
		case ".html":
			for _, m := range exported.FindAllSubmatch(b, -1) {
				stamps[rel+" exported_by"] = string(m[1])
			}
		case ".json":
			var top map[string]json.RawMessage
			if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
				return nil
			}
			if err := json.Unmarshal(b, &top); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			if top["generated_by"] == nil {
				return nil
			}
			var s string
			if err := json.Unmarshal(top["generated_by"], &s); err != nil {
				return fmt.Errorf("%s: generated_by: %w", rel, err)
			}
			stamps[rel+" generated_by"] = s
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	want := stamps["index.html exported_by"]
	for _, must := range []string{"index.html exported_by", "fy2026-adopted.json generated_by"} {
		if _, ok := stamps[must]; !ok {
			t.Fatalf("the export wrote no %s, so the comparison below is missing the stamp it exists for", must)
		}
	}
	for where, got := range stamps {
		if got != want {
			t.Errorf("%s is %q and index.html's exported_by is %q", where, got, want)
		}
	}
}

// mustViews is views for a test, where a declaration it cannot derive is a
// fatal setup error.
func mustViews(t *testing.T, built result) []export.View {
	t.Helper()
	vs, err := views(built)
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	return vs
}

// must unwraps a declaration a test cannot proceed without.
func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

// TestEveryPageALinkCitesHasItsLinksInTheConfig holds the config's per-page
// links to every locator of every link the published columns carry: the
// client shows a figure's citations by looking its pages up, so a cited page
// with no entry is a figure shown without its provenance. A schedule
// unviewedDocuments declares no page renders shows no figure, and is skipped.
func TestEveryPageALinkCitesHasItsLinksInTheConfig(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readPage(t, opts.OutputDir)
	_, rest, ok := strings.Cut(page, "window.FISC_CONFIG = ")
	if !ok {
		t.Fatal("the served page carries no FISC_CONFIG")
	}
	body, _, _ := strings.Cut(rest, ";</script>")
	var config struct {
		Years []struct {
			Path string `json:"path"`
		} `json:"years"`
		Docs map[string]struct {
			Pages map[string]struct {
				Text string `json:"text"`
			} `json:"pages"`
		} `json:"docs"`
	}
	if err := json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatalf("decode FISC_CONFIG: %v", err)
	}
	cited := 0
	for _, y := range config.Years {
		raw, err := os.ReadFile(filepath.Join(opts.OutputDir, filepath.FromSlash(y.Path)))
		if err != nil {
			t.Fatalf("read %s: %v", y.Path, err)
		}
		var column struct {
			Column struct {
				FiscalYear int    `json:"fiscal_year"`
				Basis      string `json:"basis"`
			} `json:"column"`
			Schedules map[string]struct {
				Links []struct {
					Locators []struct {
						DocID string `json:"doc_id"`
						Pages []int  `json:"pages"`
					} `json:"locators"`
				} `json:"links"`
			} `json:"schedules"`
		}
		if err := json.Unmarshal(raw, &column); err != nil {
			t.Fatalf("decode %s: %v", y.Path, err)
		}
		for key, sched := range column.Schedules {
			stem := publishedStem(key, project.Column{FiscalYear: column.Column.FiscalYear,
				Basis: mapping.Basis(column.Column.Basis)})
			if _, unviewed := unviewedDocuments[stem]; unviewed {
				continue
			}
			for _, l := range sched.Links {
				for _, loc := range l.Locators {
					for _, p := range loc.Pages {
						cited++
						if config.Docs[loc.DocID].Pages[strconv.Itoa(p)].Text == "" {
							t.Errorf("%s's %s schedule cites %s p%d, and the config carries no links for it",
								y.Path, key, loc.DocID, p)
						}
					}
				}
			}
		}
	}
	if cited == 0 {
		t.Fatal("no link cites a page, so this test asserts nothing")
	}
	t.Logf("%d cited pages, each with its links in the config", cited)
}
