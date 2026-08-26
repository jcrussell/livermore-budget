package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
		Build: func(string) (map[string][]byte, error) {
			return map[string][]byte{"sankey": goldenSankey(t)}, nil
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

	for _, rel := range []string{"index.html", "app.js", "style.css", ".nojekyll", "vendor/d3.min.js", "data/sankey.json"} {
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
			if _, err := os.Stat(filepath.Join(opts.OutputDir, filepath.FromSlash(ref))); err != nil {
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

func TestExportRunCleanEmptiesItsOwnOutput(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(opts); err != nil {
		t.Fatalf("first export: %v", err)
	}
	stale := filepath.Join(opts.OutputDir, "data", "gone.json")
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

func TestExportRunReportsABuilderFailure(t *testing.T) {
	opts, _, _, errOut := testOptions(t)
	opts.Build = func(string) (map[string][]byte, error) {
		return nil, os.ErrNotExist
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
	// The stems are asserted in full, and the two rules that produce them are
	// visible in the list. A SINGLE-COLUMN document is a year of something and
	// goes through project.PublishedStem, so the spine's opening year keeps the
	// bare stem the contract promises and the next is suffixed. A document of
	// SEVERAL columns is a year of nothing and takes its projection's name
	// verbatim -- revenue-trends spans four columns, and putting it through
	// PublishedStem would write it twice, once per published year, as two
	// byte-identical files one of which claims a year it does not cover.
	if diff := cmp.Diff([]string{"revenue-trends", "sankey", "sankey-2027"}, keys(got)); diff != "" {
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

// TestStemForAsksHowManyDocumentsNotHowManyColumns pins both directions of the
// naming rule, and the second case is one an earlier version of stemFor got
// wrong.
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := stemFor("sankey", c.o, c.slices)
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
// The build loop used to be the cartesian product of the published fiscal years
// and the registry, and it hard-errored through slicesContain when a Sliced
// projection did not declare the published SPINE slice. A trends projection
// declares one slice of a different schedule, so `fisc export` failed on its
// first invocation, before writing a file. fisc-744 fixed the same shape in
// internal/check and left this copy behind.
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
		stem := project.PublishedStem(project.TrendsProjection, y)
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

// TestViewsNamesEveryDocumentTheSitePublishes is the composition root's half of
// the multi-view shell: internal/export lays out what it is handed and never
// guesses what a stem means, so this is the only place that knows the revenue
// trends are a view rather than a year of the Sankey.
func TestViewsNamesEveryDocumentTheSitePublishes(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	got := views(built)

	if len(got) != 2 {
		t.Fatalf("got %d views over %v, want the spine and the revenue trends", len(got), keys(built))
	}
	if got[0].Path != export.IndexPath || got[0].Projection != export.PrimaryProjection {
		t.Errorf("the site opens on %+v, want the spine at %s", got[0], export.IndexPath)
	}
	// The YEARS belong to the spine view and to no other. A site-wide year list
	// could not say that: the revenue trends are one document over four columns
	// and have no year to switch between.
	if len(got[0].YearStems) != len(project.PublishedFiscalYears()) {
		t.Errorf("the spine view lists %d year stems, want %d",
			len(got[0].YearStems), len(project.PublishedFiscalYears()))
	}
	if len(got[1].YearStems) != 0 {
		t.Errorf("the revenue view lists year stems %v; it is one document over four columns",
			got[1].YearStems)
	}
	if got[1].Projection != project.TrendsProjection {
		t.Errorf("the second view renders %q, want %q", got[1].Projection, project.TrendsProjection)
	}
	// Titles and ledes are the caller's words. A packager composing prose about
	// a document would be making a claim about figures it may not recompute.
	if got[1].Title == "" || got[1].Lede == "" {
		t.Error("the revenue view ships no title or lede, so the page would head itself")
	}
}

// TestAViewWhoseDocumentWasNotBuiltIsDropped: a nav entry pointing at a page
// that was not written is a 404 a reader can click, and it is the one failure
// this function must never produce. Dropping the view is what prevents it;
// `fisc verify` is what says the document is missing.
func TestAViewWhoseDocumentWasNotBuiltIsDropped(t *testing.T) {
	only := map[string][]byte{export.PrimaryProjection: {}}
	got := views(only)
	if len(got) != 1 {
		t.Fatalf("got %d views with only the spine built, want 1: %+v", len(got), got)
	}
	if got[0].Path != export.IndexPath {
		t.Errorf("the surviving view is %q, want the one the site opens on", got[0].Path)
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
			opts: project.Options{Columns: d.Columns, Scope: d.Scope},
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
				b.opts.Scope = project.PublishedScope
				m[project.TrendsProjection] = b
			},
			wants: []string{project.TrendsProjection, "missing"},
		},
		{
			// The spine is not special-cased, and this is what says so.
			name: "a published spine year missing",
			drop: func(m map[string]builtDoc) {
				delete(m, project.PublishedStem(project.PublishedProjection, 2027))
			},
			wants: []string{"sankey-2027", "no projection built it"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			built := map[string]builtDoc{}
			for stem, b := range full {
				built[stem] = builtDoc{name: b.name, opts: project.Options{
					Columns: slices.Clone(b.opts.Columns), Scope: b.opts.Scope,
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
				Scope      string `json:"scope"`
				Columns    []struct {
					FiscalYear int    `json:"fiscal_year"`
					Basis      string `json:"basis"`
				} `json:"columns"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Errorf("decode %s: %v", d.Stem, err)
			continue
		}
		if doc.Metadata.Scope != d.Scope {
			t.Errorf("%s ships scope %q, declared %q", d.Stem, doc.Metadata.Scope, d.Scope)
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

// TestStemForRefusesADocumentWithNoColumns covers the index that used to be
// taken blind.
//
// A zero-column Options is not reachable through project.Slices today, but a
// projection is an interface any future type can satisfy, and the failure mode
// of `o.Columns[0]` is a PANIC inside the command whose job is writing files --
// which is worse than any refusal, because it leaves a half-exported directory
// and no message a reader could act on. It is refused with the reason instead.
func TestStemForRefusesADocumentWithNoColumns(t *testing.T) {
	// slices > 1, or the one-document branch returns before the index.
	_, err := stemFor("sankey", project.Options{}, 2)
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
	}, 2)
	if err != nil || got != "sankey-2027" {
		t.Errorf("stemFor = %q, %v; want \"sankey-2027\", nil", got, err)
	}
}
