package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// goldenSankey is the hand-derived worked example of the spine document. The
// pipeline has to reproduce it.
func goldenSankey(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "sankey.golden.json"))
	if err != nil {
		t.Fatalf("read golden projection: %v", err)
	}
	return b
}

// goldenFunding is a department-funding document of the golden spine's column,
// its stem and bytes, which the glance page reads beside the spine; its p67
// departments tie to the spine as glanceTies holds them.
func goldenFunding(t *testing.T) (string, []byte) {
	t.Helper()
	var spine struct {
		Metadata struct {
			GeneratedBy string `json:"generated_by"`
			FiscalYear  int    `json:"fiscal_year"`
			Label       string `json:"fiscal_year_label"`
			Basis       string `json:"basis"`
			Headline    struct {
				Spent int64 `json:"all_funds_gross_expenditure_cents"`
			} `json:"headline"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(goldenSankey(t), &spine); err != nil {
		t.Fatal(err)
	}
	m := spine.Metadata
	stems := publishedInto(export.PrimaryProjection, project.DepartmentFundingProjection)
	if len(stems) != 1 {
		t.Fatalf("department-funding is published %d times over the spine's column, want once", len(stems))
	}
	apart, err := departmentsApartFromSpine()
	if err != nil {
		t.Fatal(err)
	}
	rows := m.Headline.Spent
	for _, a := range apart {
		if a.FiscalYear == m.FiscalYear && a.Basis == m.Basis {
			rows -= a.ResidualCents
		}
	}
	police := rows * 4 / 5
	link := func(to string, cents int64, id string) string {
		return fmt.Sprintf(`{"source":"fund-group/general","target":%q,"value_cents":%d,"kind":"external",`+
			`"fact_ids":[%q],"locators":[{"doc_id":%q,"pages":[67]}]}`, to, cents, id, budgetDocID)
	}
	doc := fmt.Sprintf(`{"schema_version":1,"projection":%q,"nodes":[`+
		`{"id":"fund-group/general","label":"General Fund","tier":2,"role":"fund_group"},`+
		`{"id":"department/police-department","label":"Police Department","tier":4,"role":"whole_department"},`+
		`{"id":"department/city-council","label":"City Council","tier":4,"role":"whole_department"}],`+
		`"links":[%s,%s],"metadata":{"generated_by":%q,"fiscal_year":%d,"fiscal_year_label":%q,"basis":%q,`+
		`"scopes":["department-funding-sources"],"currency":"USD","units":"cents",`+
		`"sources":[{"doc_id":%q,"pages":[67]}],"counts":{"facts":2,"nodes":3,"links":2},"caveats":[]}}`,
		project.DepartmentFundingProjection,
		link("department/police-department", police, "fisc-f-000000000001"),
		link("department/city-council", rows-police, "fisc-f-000000000002"),
		m.GeneratedBy, m.FiscalYear, m.Label, m.Basis, budgetDocID)
	return stems[0], []byte(doc)
}

// goldenProjections is the golden spine and goldenFunding beside it, the
// fewest documents the committed views open a site on.
func goldenProjections(t *testing.T) map[string][]byte {
	t.Helper()
	stem, funding := goldenFunding(t)
	return map[string][]byte{"sankey": goldenSankey(t), stem: funding}
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
	// The vocabulary and the views are the repository's own: views() reads
	// both, and a fake of either would be a second declaration of what the
	// site renders.
	for _, name := range []string{"funds.yaml", "taxonomy.yaml", "departments.yaml", viewsFile} {
		b, err := os.ReadFile(filepath.Join(repoRootForTest(t), "data", name)) // #nosec G304 -- the committed data registries.
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "data", name), b, 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
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
			return result{Projections: goldenProjections(t)}, nil
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
		{name: "clean dry run", args: []string{"--clean", "--dry-run"}, want: Options{OutputDir: "dist", Clean: true, DryRun: true}},
		{name: "dry run without clean", args: []string{"--dry-run"}, wantErr: "--dry-run requires --clean"},
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
		{
			name:    "browse url with a query",
			args:    []string{"--source-browse-url", "https://example.test/blob/main?plain=1"},
			wantErr: "carries a query",
		},
		{
			name:    "browse url with an empty query",
			args:    []string{"--source-browse-url", "https://example.test/blob/main?"},
			wantErr: "carries a query",
		},
		{
			name:    "browse url with a fragment",
			args:    []string{"--source-browse-url", "https://example.test/blob/main#top"},
			wantErr: "carries a fragment",
		},
		{
			name:    "browse url with an empty fragment",
			args:    []string{"--source-browse-url", "https://example.test/blob/main#"},
			wantErr: "carries a fragment",
		},
		{
			name:    "browse url with a user and password",
			args:    []string{"--source-browse-url", "https://user:pass@example.test/blob/main"},
			wantErr: `carries an "@"`,
		},
		{
			name:    "browse url with an empty userinfo",
			args:    []string{"--source-browse-url", "https://@example.test/blob/main"},
			wantErr: `carries an "@"`,
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
			if got.DryRun != tc.want.DryRun {
				t.Errorf("got --dry-run %v, want %v", got.DryRun, tc.want.DryRun)
			}
			if got.SourceBrowseURL != tc.want.SourceBrowseURL {
				t.Errorf("got --source-browse-url %q, want %q", got.SourceBrowseURL, tc.want.SourceBrowseURL)
			}
			// Validate canonicalises the path, so compare against the
			// resolved form of what was asked for.
			want, err := export.ResolveOutputDir(tc.want.OutputDir)
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

	if err := exportRun(context.Background(), opts); err != nil {
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

// TestEveryCaveatLinkLandsOnAnAnchorThatExists holds every caveats.html link
// the corpus writes, in markup and in FISC_CONFIG, to an id caveats.html carries.
// Mutation: append "-typo" to the "caveats-line" template's anchor, or to
// caveatIndex.href's fragment, and that spelling goes red.
func TestEveryCaveatLinkLandsOnAnAnchorThatExists(t *testing.T) {
	built := builtStemsForTest(t)
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return repoRootForTest(t), nil }
	opts.Build = func(string) (result, error) { return result{Projections: built}, nil }
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(readFile(t, opts.OutputDir, "caveats.html"), -1) {
		ids[m[1]] = true
	}
	spellings := []*regexp.Regexp{
		regexp.MustCompile(`href="caveats\.html#([^"]+)"`),
		regexp.MustCompile(`"href":"caveats\.html#([^"]+)"`),
	}
	links := make([]int, len(spellings))
	pages, err := filepath.Glob(filepath.Join(opts.OutputDir, "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range pages {
		page := readFile(t, opts.OutputDir, filepath.Base(path))
		for i, re := range spellings {
			for _, m := range re.FindAllStringSubmatch(page, -1) {
				links[i]++
				if !ids[m[1]] {
					t.Errorf("%s links caveats.html#%s, which carries no such anchor", filepath.Base(path), m[1])
				}
			}
		}
	}
	if links[0] == 0 || links[1] == 0 {
		t.Fatalf("the site carries %v caveats links of the two spellings, so a spelling asserts nothing", links)
	}
}

// TestTheSourcesPageListsEveryPageTheSankeyCites: the sources page lists every
// page the Sankey's config cites, in every year. Mutation: render the footer's
// source list unconditionally -- the first arm goes red; drop p66 from
// provenance.html's table and footer -- the second does.
func TestTheSourcesPageListsEveryPageTheSankeyCites(t *testing.T) {
	dir := clientExport(t)
	read := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- the temp site.
		if err != nil {
			t.Fatalf("read the exported %s: %v", name, err)
		}
		return string(raw)
	}
	flows, sources := read("flows.html"), read("provenance.html")
	before, rest, ok := strings.Cut(flows, configOpen)
	if !ok {
		t.Fatal("flows.html carries no FISC_CONFIG")
	}
	blob, after, ok := strings.Cut(rest, configClose)
	if !ok {
		t.Fatal("flows.html never closes FISC_CONFIG")
	}
	if visible := before + after; strings.Contains(visible, `id="sources-view"`) ||
		strings.Contains(visible, "/pages/p0") {
		t.Error("flows.html lists the pages it cites beside the sources page")
	} else if !strings.Contains(visible, `<a href="provenance.html">`) {
		t.Error("flows.html does not link the sources page")
	}
	var cfg struct {
		Docs map[string]struct {
			Pages map[string]struct {
				Text string `json:"text"`
			} `json:"pages"`
		} `json:"docs"`
	}
	if err := json.Unmarshal([]byte(blob), &cfg); err != nil {
		t.Fatal(err)
	}
	cited := 0
	for id, doc := range cfg.Docs {
		for n, p := range doc.Pages {
			cited++
			if !strings.Contains(sources, `href="`+p.Text+`"`) {
				t.Errorf("provenance.html does not list %s p%s, which flows.html cites", id, n)
			}
		}
	}
	if cited == 0 {
		t.Fatal("flows.html's config cites no page, so nothing above was checked")
	}
	t.Logf("%d page(s) flows.html cites", cited)
}

// TestExportRunResolvesBothCitationClassesWithoutGitHub walks the page's own
// links: the city's PDF stays remote, and the extracted text every figure is
// traced to is a file inside the output.
func TestExportRunResolvesBothCitationClassesWithoutGitHub(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readFile(t, opts.OutputDir, "flows.html")

	var pdfCitations, textCitations int
	for _, ref := range pageRefs(page) {
		switch {
		case ref == export.RepositoryURL:
			// The footer's link to the code, which cites no figure.
		case strings.HasPrefix(ref, "https://www.livermoreca.gov/"):
			// The city's own document: the source heading links the document,
			// each cited page links it again with a #page fragment.
			if strings.Contains(ref, "#page=") {
				pdfCitations++
			}
		case strings.Contains(ref, "://"):
			t.Errorf("the page reaches %q; every citation but the city's PDF has to resolve inside the site", ref)
		default:
			// The fragment comes off before the filesystem is asked; the fragment half is
			// TestEveryCaveatLinkLandsOnAnAnchorThatExists'.
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
	// The footer's repository link is not a citation.
	if strings.Contains(strings.ReplaceAll(page, `href="`+export.RepositoryURL+`"`, ""), "github.com") {
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
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readFile(t, opts.OutputDir, "flows.html")

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

	err = exportRun(context.Background(), opts)
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
			// Fragments are kept here: the caller counts "#page=" on PDF links and strips
			// the anchor off a relative one itself.
			if ref == "" || strings.HasPrefix(ref, "#") {
				continue
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

// readFile is the exported file at name under dir.
func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))) // #nosec G304 -- a test's own site.
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// TestExportRunRefusesAnUnreachableDocumentBeforeTouchingTheSite: `fisc export
// --clean` over an undeclared published document refuses before it writes or
// removes anything.
func TestExportRunRefusesAnUnreachableDocumentBeforeTouchingTheSite(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("first export: %v", err)
	}
	index := filepath.Join(opts.OutputDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("no site to destroy: %v", err)
	}

	// A published document that is built, rendered by no view and declared by no
	// entry; the stem is read out of the map, and Fatalf fires if it empties.
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
		built := goldenProjections(t)
		built[stem] = doc
		return result{Projections: built}, nil
	}

	opts.Clean = true
	if err := exportRun(context.Background(), opts); err == nil {
		t.Fatal("exportRun accepted an undeclared unrendered document")
	} else if !strings.Contains(err.Error(), "no view renders it") {
		t.Fatalf("got %v, want the reachability refusal", err)
	}
	if _, err := os.Stat(index); err != nil {
		t.Errorf("the export touched the reader's site before refusing: %v", err)
	}
}

// treeOf is every entry under dir, its mode, modification time and, for a
// file, its bytes: what a run that writes nothing leaves exactly as it was.
func treeOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := fmt.Sprintf("%v %d", info.Mode(), info.ModTime().UnixNano())
		if d.Type().IsRegular() {
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			entry += " " + string(b)
		}
		tree[p] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return tree
}

// --clean --dry-run prints exactly the paths --clean would remove and leaves the
// directory as it was, down to its modification times; the --clean after it
// says what it removed.
// Mutation: fall through to Write after printing -- a tree diff; skip printing --
// stdout ""; keep Validate's write probe for a dry run -- a modification-time diff;
// drop the --clean summary -- "does not say what --clean removed".
func TestExportRunDryRunPrintsWhatCleanWouldRemoveAndWritesNothing(t *testing.T) {
	opts, _, out, _ := testOptions(t)
	build := opts.Build
	opts.Build = func(root string) (result, error) {
		r, err := build(root)
		if err != nil {
			return r, err
		}
		r.Files = map[string][]byte{"facts/gone/p0001.jsonl": []byte("{}\n")}
		return r, nil
	}
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "notes.md"), []byte("mine\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Back-dated, so a probe made and removed within the filesystem's clock
	// tick still moves the directory's modification time.
	past := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(opts.OutputDir, past, past); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := treeOf(t, opts.OutputDir)

	io, _, stdout, stderr := iostreams.Test()
	opts.IO = io
	opts.Build = build
	opts.Clean = true
	opts.DryRun = true
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	want := filepath.Join(opts.OutputDir, "facts", "gone", "p0001.jsonl") + "\n" +
		filepath.Join(opts.OutputDir, "facts", "gone") + string(filepath.Separator) + "\n" +
		filepath.Join(opts.OutputDir, "facts") + string(filepath.Separator) + "\n"
	if stdout.String() != want {
		t.Errorf("got stdout %q, want %q", stdout.String(), want)
	}
	if diff := cmp.Diff(before, treeOf(t, opts.OutputDir)); diff != "" {
		t.Errorf("the dry run changed the output directory (-before +after):\n%s", diff)
	}
	if out() == "" {
		t.Fatal("the first export reported nothing, so this test proves nothing about its stdout")
	}

	opts.DryRun = false
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("--clean: %v", err)
	}
	if want := "removed 1 files the earlier export wrote and this one does not, 0 stranded manifest temp files and 2 empty directories\n"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr %q does not say what --clean removed: want %q", stderr.String(), want)
	}
}

// A path an earlier export wrote as a file and this one writes under is a usage
// error without --clean, with a hint naming it.
// Mutation: return Write's error unchanged -- "got <error>, want a FlagError".
func TestExportRunNeedsCleanWhereAnEarlierFileIsInTheWay(t *testing.T) {
	opts, _, _, _ := testOptions(t)
	build := opts.Build
	withFile := func(rel string) builder {
		return func(root string) (result, error) {
			r, err := build(root)
			r.Files = map[string][]byte{rel: []byte("{}\n")}
			return r, err
		}
	}
	opts.Build = withFile("facts")
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("first export: %v", err)
	}
	opts.Build = withFile("facts/d/p0001.jsonl")
	err := exportRun(context.Background(), opts)
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) {
		t.Fatalf("got %v, want a FlagError", err)
	}
	var h *hint.ErrHint
	if !errors.As(err, &h) || !strings.Contains(h.Hint, "--clean") {
		t.Errorf("the refusal %q carries no hint naming --clean", err)
	}
}

// A dry run over a missing directory creates and prints nothing.
// Mutation: fall through to Write after printing -- "the dry run created the
// output directory".
func TestExportRunDryRunOverAMissingDirectoryCreatesNothing(t *testing.T) {
	opts, _, out, _ := testOptions(t)
	opts.Clean = true
	opts.DryRun = true
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if _, err := os.Stat(opts.OutputDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the dry run created the output directory (stat: %v)", err)
	}
	if out() != "" {
		t.Errorf("got stdout %q, want nothing to remove", out())
	}
}

func TestExportRunReportsABuilderFailure(t *testing.T) {
	opts, _, _, errOut := testOptions(t)
	opts.Build = func(string) (result, error) {
		return result{}, os.ErrNotExist
	}
	if err := exportRun(context.Background(), opts); err == nil {
		t.Fatal("got nil error, want the builder's failure")
	}
	if _, err := os.Stat(opts.OutputDir); err == nil {
		t.Error("the output directory was created before the data was ready")
	}
	if errOut() != "" {
		t.Errorf("got chatter on a failed run: %q", errOut())
	}
}

// TestBuildProjectionsRunsThePipeline reads the committed facts/facts.jsonl
// through the real projection against the hand-derived
// testdata/sankey.golden.json; only generated_by may differ.
func TestBuildProjectionsRunsThePipeline(t *testing.T) {
	root := repoRootForTest(t)
	got, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	// A single-column document goes through project.PublishedStem: the opening
	// column keeps the bare stem, others carry year and non-published basis.
	if diff := cmp.Diff([]string{
		"changes-in-fund-balances",
		"cip-projects",
		"debt-service",
		"department-funding", "department-funding-2024-actual",
		"department-funding-2025-revised", "department-funding-2027",
		"department-spending", "department-spending-2024-actual",
		"department-spending-2025-revised", "department-spending-2027",
		"expansion", "expansion-2027",
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
	var h *hint.ErrHint
	if !errors.As(err, &h) || !strings.Contains(h.Hint, "fisc build") {
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

// keys is the stems in a map, sorted, so a cmp.Diff against a literal is stable.
func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTheCommittedStemsAreUnchanged pins the document stems as typed literals:
// a stem is the schedule key and a year's name in window.FISC_CONFIG.
func TestTheCommittedStemsAreUnchanged(t *testing.T) {
	root := repoRootForTest(t)
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	want := []string{
		"changes-in-fund-balances",
		"cip-projects",
		"debt-service",
		"department-funding", "department-funding-2024-actual",
		"department-funding-2025-revised", "department-funding-2027",
		"department-spending", "department-spending-2024-actual",
		"department-spending-2025-revised", "department-spending-2027",
		"expansion", "expansion-2027",
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
	root := repoRootForTest(t)
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page := readFile(t, opts.OutputDir, "flows.html")
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

// TestTheSitePublishesOneFilePerColumnAndNothingTwice pins `fisc export`'s file
// set as typed literals; a document stating no year or basis ships as itself.
func TestTheSitePublishesOneFilePerColumnAndNothingTwice(t *testing.T) {
	root := repoRootForTest(t)
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if err := exportRun(context.Background(), opts); err != nil {
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
		"data/cip-projects.json",
		"data/debt-service.json",
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

// TestStemAsksHowManyDocumentsNotHowManyColumns: a projection publishes one
// document per year by its declared slices, not its column count; a store with
// one revenue-by-fund column must still ship data/revenue-trends.json.
func TestStemAsksHowManyDocumentsNotHowManyColumns(t *testing.T) {
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
		// Two bases for one fiscal year: the basis is spelled out, as pp.66-67's
		// revised column would produce.
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
			got, err := project.Stem("sankey", c.o, make([]project.Options, c.slices))
			if err != nil {
				t.Fatalf("Stem: %v", err)
			}
			if got != c.want {
				t.Errorf("Stem = %q, want %q", got, c.want)
			}
		})
	}
}

// TestBuildProjectionsDoesNotRefuseASecondSchedule: a projection is built over
// the slices it declares, so a trends slice lacking the spine's is not refused.
func TestBuildProjectionsDoesNotRefuseASecondSchedule(t *testing.T) {
	root := repoRootForTest(t)
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

// TestEveryPublishedDocumentIsRenderedOrDeclaredUnrendered: every published
// document is rendered by a page or declared in unviewedDocuments.
func TestEveryPublishedDocumentIsRenderedOrDeclaredUnrendered(t *testing.T) {
	built := builtStemsForTest(t)
	_, ix, err := export.ColumnsOf(built, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := assertPublishedReachable(mustViews(t, result{Projections: built}), built, ix); err != nil {
		t.Fatalf("the committed corpus: %v", err)
	}

	// A published document that is neither rendered nor declared must be
	// refused. Driving it through a view set with the spine removed is the
	// cheapest way to reach that state without inventing a document.
	if err := assertPublishedReachable(nil, built, ix); err == nil {
		t.Error("no views at all was accepted; every published document is then " +
			"unreachable and only the declared ones may be")
	} else if !strings.Contains(err.Error(), "no view renders it") {
		t.Errorf("got %v, want a refusal naming the unrendered document", err)
	}

	// A declaration that stopped being true goes red. Built from the real view set
	// plus one, so the stale declaration is the only fault.
	stale := mustViews(t, result{Projections: built})
	for stem := range unviewedDocuments {
		stale = append(stale, export.View{Path: "x.html", Projection: stem})
		break
	}
	if err := assertPublishedReachable(stale, built, ix); err == nil {
		t.Error("a view rendering a document unviewedDocuments still declares unrendered " +
			"was accepted; the declaration is then a false statement about the site")
	} else if !strings.Contains(err.Error(), "delete that entry") {
		t.Errorf("got %v, want a refusal telling the caller to delete the declaration", err)
	}

	// A leftover entry naming no published document is refused too.
	unviewedDocuments["no-such-document"] = "left behind"
	t.Cleanup(func() { delete(unviewedDocuments, "no-such-document") })
	if err := assertPublishedReachable(mustViews(t, result{Projections: built}), built, ix); err == nil {
		t.Error("a declaration naming no published document was accepted")
	} else if !strings.Contains(err.Error(), "publishes no such document") {
		t.Errorf("got %v, want a refusal naming the leftover entry", err)
	}
}

// TestTheRevenueScheduleCaveatNamesThePageThatPrintsItsFourColumns: the page the
// pp.127-140 caveat names is the nav label of the view reaching revenue-trends.
// Mutation: restore "the Revenue tables print all four" in
// revenueSchedulePublishedTwiceCaveat, and every document carrying it goes red.
func TestTheRevenueScheduleCaveatNamesThePageThatPrintsItsFourColumns(t *testing.T) {
	built := builtStemsForTest(t)
	_, ix, err := export.ColumnsOf(built, "")
	if err != nil {
		t.Fatal(err)
	}
	var navs []string
	for _, v := range mustViews(t, result{Projections: built}) {
		reached, err := v.ReachedStems(ix)
		if err != nil {
			t.Fatalf("%s: %v", v.Path, err)
		}
		if slices.Contains(reached, project.TrendsProjection) {
			navs = append(navs, v.Nav)
		}
	}
	if len(navs) != 1 {
		t.Fatalf("views reaching %s are listed as %q, want exactly one", project.TrendsProjection, navs)
	}
	page := "the " + navs[0] + " page prints"
	carried := 0
	for _, stem := range keys(built) {
		var doc struct {
			Metadata struct {
				Caveats []project.Caveat `json:"caveats"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(built[stem], &doc); err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		for _, c := range doc.Metadata.Caveats {
			if c.ID != "the-revenue-schedule-is-published-twice" {
				continue
			}
			carried++
			for field, text := range map[string]string{"summary": c.Summary, "text": c.Text} {
				if !strings.Contains(text, page) {
					t.Errorf("%s's caveat %s %s names no %q: %q", stem, c.ID, field, page, text)
				}
			}
		}
	}
	if carried == 0 {
		t.Fatal("no built document carries the-revenue-schedule-is-published-twice, so this asserts nothing")
	}
}

// builtStemsForTest is the real corpus's projections, for the tests that assert
// over what the repository actually publishes rather than over a fixture.
func builtStemsForTest(t *testing.T) map[string][]byte {
	t.Helper()
	root := repoRootForTest(t)
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	return built
}

// TestViewsOpensOnTheSpineAndGivesYearsToItsCharts: the site opens on the glance
// page, the year control is the spine's alone, and the Sankey is at flows.html.
// Mutation: swap the two views' paths in data/views.yaml and the opening and
// flows arms go red.
func TestViewsOpensOnTheSpineAndGivesYearsToItsCharts(t *testing.T) {
	built := builtStemsForTest(t)
	got := mustViews(t, result{Projections: built})

	if len(got) != 9 {
		t.Fatalf("got %d views over %v, want the glance, the spine's Sankey, its bars, the change page, "+
			"the fund balances table, the department, capital and debt pages "+
			"and the caveats index", len(got), keys(built))
	}
	var spine export.View
	for _, v := range got {
		if v.Template == export.SankeyTemplate {
			spine = v
		}
	}
	if spine.Path != "flows.html" || spine.Nav != "Flows" || spine.Projection != export.PrimaryProjection {
		t.Errorf("the Sankey is %q (%q) over %q, want the spine at flows.html under Flows",
			spine.Path, spine.Nav, spine.Projection)
	}
	if opens := got[0]; opens.Path != export.IndexPath || opens.Template != export.GlanceTemplate ||
		opens.Projection != export.PrimaryProjection {
		t.Errorf("the nav opens with %q (%s over %q), want the glance page over the spine at %s",
			opens.Path, opens.Template, opens.Projection, export.IndexPath)
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
	for _, path := range []string{export.IndexPath, "ledger.html", "change.html", "balances.html",
		"department.html", "capital.html", "debt.html"} {
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

	// fund-flows is drawn inside the expansion, which is stitched from it; a
	// page of its own would publish the same money twice. Expanding node by
	// node reaches the whole budget, so no page draws it at once.
	for _, path := range []string{"revenue.html", "spending.html", "drilldown.html", "whole.html"} {
		if _, ok := byPath[path]; ok {
			t.Errorf("%s is in the nav; the expansion is where that document is drawn now", path)
		}
	}
	for _, v := range got {
		if v.Projection == project.FundFlowsProjection {
			t.Errorf("view %q renders fund-flows as its own page; it is drawn inside the expansion", v.Path)
		}
		if len(v.YearStems) > 0 && v.Projection != export.PrimaryProjection &&
			(v.Path != "department.html" || v.Projection != project.DepartmentFundingProjection) {
			t.Errorf("view %q lists years of %q; the year control is the spine's documents' and "+
				"the department page's funding documents'", v.Path, v.Projection)
		}
	}
	draws := map[string]string{}
	for _, v := range got {
		if v.Draws != "" {
			draws[v.Path] = v.Draws
		}
	}
	if diff := cmp.Diff(map[string]string{
		"flows.html":  project.ExpansionProjection,
		"ledger.html": project.ExpansionProjection,
	}, draws); diff != "" {
		t.Errorf("the schedules the views draw out of their columns (-want +got):\n%s", diff)
	}
	// THE SPINE'S CHART IS THE EXPANSION, read in the spine's column order:
	// the client places a root by these tiers and caps a split by its key.
	if spine.Overview.Sankey == nil {
		t.Fatal("the spine declares no Sankey hints")
	}
	if diff := cmp.Diff(export.SankeyHints{Tiers: []int{0, 2, 5},
		Grow: &export.CanvasGrowth{NodePx: 40, Floor: 820, Ceiling: 4800},
		SplitCaps: []export.SplitCap{{Split: "funds", Cap: 8, Tail: "funds"}, {Split: "divisions", Cap: 24, Tail: "divisions"},
			{Split: "object-rows", Cap: 8, Tail: "object rows"}, {Split: "lines", Cap: 8, Tail: "lines"},
			{Split: "receivers", Cap: 8, Tail: "funds"}}},
		*spine.Overview.Sankey); diff != "" {
		t.Errorf("the spine's Sankey hints (-want +got):\n%s", diff)
	}
}

// TestOpensIntoJoinsOnColumnNotOnDeclaredOrder: a year's schedule is joined on
// Column; a join on declared order pairs sankey with fund-flows-2024-actual.
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

	// A year whose drawn schedule was not built is refused by name, not dropped.
	// Mutation: have drawsBuilt check every year the view lists, and the
	// Sankey falls back to its own document and the export ships.
	short := map[string][]byte{}
	for k, v := range built {
		if k != "expansion-2027" {
			short[k] = v
		}
	}
	if !opensInto(export.PrimaryProjection, project.ExpansionProjection, short) {
		t.Error("dropping the SECOND year's expansion must not undeclare the Sankey's chart")
	}
	opts, _, _, _ := testOptions(t)
	opts.Build = func(string) (result, error) { return result{Projections: short}, nil }
	if err := exportRun(context.Background(), opts); err == nil {
		t.Error("exportRun accepted a spine whose second year has no expansion")
	} else if !strings.Contains(err.Error(), `carries none`) {
		t.Errorf("got %v, want the refusal naming the year's column", err)
	}

	// No chart draws an unbuilt document: without the expansion the Sankey draws
	// its own. Mutation: key views' fallback on export.IndexPath and "no expansion"
	// goes red, the Sankey dropped.
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
		want map[string]string
	}{
		{"everything built", nil,
			map[string]string{"flows.html": project.ExpansionProjection}},
		{"no expansion", []string{project.ExpansionProjection},
			map[string]string{"flows.html": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]string{}
			for _, v := range mustViews(t, result{Projections: without(tc.drop...)}) {
				if v.Template == export.SankeyTemplate {
					got[v.Path] = v.Draws
				}
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("the chart pages and the schedules they draw (-want +got):\n%s", diff)
			}
		})
	}
}

// TestADocumentOutsideTheColumnItIsPublishedForIsRefused: where publishedInto
// and export.ColumnIndex disagree, the write refuses by the column's name.
func TestADocumentOutsideTheColumnItIsPublishedForIsRefused(t *testing.T) {
	built := builtStemsForTest(t)
	var doc map[string]any
	if err := json.Unmarshal(built[project.ExpansionProjection], &doc); err != nil {
		t.Fatalf("decode %s: %v", project.ExpansionProjection, err)
	}
	doc["metadata"].(map[string]any)["fiscal_year"] = 2099
	moved, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	elsewhere := maps.Clone(built)
	elsewhere[project.ExpansionProjection] = moved
	if !opensInto(export.PrimaryProjection, project.ExpansionProjection, elsewhere) {
		t.Fatal("opensInto drops the Sankey's chart, so the refusal below is no evidence")
	}
	opts, _, _, _ := testOptions(t)
	opts.Build = func(string) (result, error) { return result{Projections: elsewhere}, nil }
	if err := exportRun(context.Background(), opts); err == nil {
		t.Error("exportRun accepted a spine whose opening column carries no expansion")
	} else if !strings.Contains(err.Error(), "carries none") {
		t.Errorf("got %v, want the refusal naming the opening year's column", err)
	}
}

// TestAViewWhoseDocumentWasNotBuiltIsDropped: a view over an unbuilt document
// is dropped rather than linked as a 404.
func TestAViewWhoseDocumentWasNotBuiltIsDropped(t *testing.T) {
	only := map[string][]byte{export.PrimaryProjection: {}, project.DepartmentFundingProjection: {}}
	got := mustViews(t, result{Projections: only})

	// Every view that names a projection is the spine's; the caveats index names
	// none and has nothing to drop.
	for _, v := range got {
		if v.Projection == "" {
			continue
		}
		if v.Projection != export.PrimaryProjection {
			t.Errorf("view %q renders %q, and only the spine and the index's read were built", v.Path, v.Projection)
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
	// The views whose documents were not built are gone, derived from the full
	// set's views rather than typed out.
	dropped := map[string]bool{}
	for _, v := range mustViews(t, result{Projections: map[string][]byte{
		export.PrimaryProjection:            {},
		project.DepartmentFundingProjection: {},
		project.TrendsProjection:            {},
		project.FundFlowsProjection:         {},
		project.ChangesProjection:           {},
		project.FundBalancesProjection:      {},
		project.CIPProjectsProjection:       {},
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

// TestTheIndexIsRefusedByNameWithoutItsRead: without department-funding the
// export refuses by the index view and its missing read, before writing.
// Mutation: drop the index arm of views' unbuilt-read branch, and the export
// refuses with "0 views are at index.html" instead; both arms go red.
func TestTheIndexIsRefusedByNameWithoutItsRead(t *testing.T) {
	_, err := views(repoRootForTest(t), result{Projections: map[string][]byte{export.PrimaryProjection: {}}})
	if err == nil {
		t.Fatal("views accepted a spine with no department-funding beside it")
	}
	for _, want := range []string{`index.html ("At a glance")`, "reads " + project.DepartmentFundingProjection,
		project.DepartmentFundingProjection + " was not built"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("views refused with %q, want it to name %q", err, want)
		}
	}

	opts, _, _, _ := testOptions(t)
	opts.Build = func(string) (result, error) {
		return result{Projections: map[string][]byte{"sankey": goldenSankey(t)}}, nil
	}
	err = exportRun(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), project.DepartmentFundingProjection+" was not built") {
		t.Errorf("exportRun over the spine alone returned %v, want the index's named refusal", err)
	}
	if _, serr := os.Stat(filepath.Join(opts.OutputDir, export.IndexPath)); serr == nil {
		t.Error("the refused export wrote index.html")
	}
}

// TestExportRefusesAPublishedDocumentThatWasNotBuilt: a stem absent, and a stem
// present but short a column, are each refused by `fisc export`.
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

// TestPublishedDocumentsAreWhatTheCorpusBuilds: the corpus, read back from the
// shipped bytes, builds no more than project.PublishedDocuments declares (its
// TrendsColumns are stated, not derived).
func TestPublishedDocumentsAreWhatTheCorpusBuilds(t *testing.T) {
	root := repoRootForTest(t)
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
				// The spine publishes one column, the trends a list; exactly one is set.
				FiscalYear int    `json:"fiscal_year"`
				Basis      string `json:"basis"`
				// One schedule is named singularly, two as a list; exactly one key is present
				// (MultiScopeEnvelope).
				Scope   string   `json:"scope"`
				Scopes  []string `json:"scopes"`
				Columns []struct {
					FiscalYear int `json:"fiscal_year"`
					// A band's last year, which the debt-service document
					// publishes beside its first; absent everywhere else.
					FiscalYearThrough int    `json:"fiscal_year_through"`
					Basis             string `json:"basis"`
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
				FiscalYear: c.FiscalYear, Basis: vocab.Basis(c.Basis), Through: c.FiscalYearThrough})
		}
		if len(shipped) == 0 && doc.Metadata.FiscalYear != 0 {
			shipped = append(shipped, project.Column{
				FiscalYear: doc.Metadata.FiscalYear, Basis: vocab.Basis(doc.Metadata.Basis)})
		}
		if diff := cmp.Diff(d.Columns, shipped); diff != "" {
			t.Errorf("%s: the declared columns and the shipped ones differ (-declared +shipped)"+
				":\n%s\nIf the corpus legitimately grew, widen project.PublishedDocuments in "+
				"the same commit; the declaration is what the site PROMISES and it may not "+
				"trail what it serves", d.Stem, diff)
		}
	}

	// And nothing published is left over: a document the corpus builds at a stem
	// no declaration names is shipped and unguarded.
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

// TestStemRefusesADocumentWithNoColumns: a zero-column Options is refused with
// a reason rather than panicking on `o.Columns[0]`.
func TestStemRefusesADocumentWithNoColumns(t *testing.T) {
	// More than one document, or the one-document branch returns before the
	// index.
	_, err := project.Stem("sankey", project.Options{}, make([]project.Options, 2))
	if err == nil {
		t.Fatal("Stem accepted an Options with no columns, want a refusal")
	}
	for _, want := range []string{"sankey", "no columns"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Stem error = %q, want it to name %q", err, want)
		}
	}

	// And one column still works, so the guard has not swallowed the real case.
	got, err := project.Stem("sankey", project.Options{
		Columns: []project.Column{{FiscalYear: 2027, Basis: "adopted"}},
	}, make([]project.Options, 2))
	if err != nil || got != "sankey-2027" {
		t.Errorf("Stem = %q, %v; want \"sankey-2027\", nil", got, err)
	}
}

// TestTheFundFlowsFixtureIsTheDocumentTheSiteDraws holds the captured
// testdata/fund-flows.golden.json to what buildProjections writes.
func TestTheFundFlowsFixtureIsTheDocumentTheSiteDraws(t *testing.T) {
	fixtureIsTheDocumentExported(t, project.FundFlowsProjection, "fund-flows.golden.json")
}

// fixtureIsTheDocumentExported compares one committed capture line for line
// with what buildProjections writes at `stem`, metadata.generated_by aside.
func fixtureIsTheDocumentExported(t *testing.T, stem, fixture string) {
	t.Helper()
	root := repoRootForTest(t)
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
	root := repoRootForTest(t)
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if runErr := exportRun(context.Background(), opts); runErr != nil {
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
	want := stamps["flows.html exported_by"]
	for _, must := range []string{"flows.html exported_by", "fy2026-adopted.json generated_by"} {
		if _, ok := stamps[must]; !ok {
			t.Fatalf("the export wrote no %s, so the comparison below is missing the stamp it exists for", must)
		}
	}
	for where, got := range stamps {
		if got != want {
			t.Errorf("%s is %q and flows.html's exported_by is %q", where, got, want)
		}
	}
}

// mustViews is views for a test, where a declaration it cannot derive is a
// fatal setup error.
func mustViews(t *testing.T, built result) []export.View {
	t.Helper()
	vs, err := views(repoRootForTest(t), built)
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	return vs
}

// TestEveryPageALinkCitesHasItsLinksInTheConfig: every locator of every link in
// a schedule a page reaches has an entry in that page's config's links.
// Mutation: make department.html's config carry the opening year's sources
// alone, and the department rows on other years' pages go red.
func TestEveryPageALinkCitesHasItsLinksInTheConfig(t *testing.T) {
	root := repoRootForTest(t)
	opts, _, _, _ := testOptions(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	var built result
	opts.Build = func(root string) (result, error) {
		r, buildErr := cachedBuildAll(root)
		built = r
		return r, buildErr
	}
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	_, ix, err := export.ColumnsOf(built.Projections, "")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	// Each page that shows a column's figures, and where its config sits.
	configs := map[string][2]string{
		"flows.html":      {"window.FISC_CONFIG = ", ";</script>"},
		"department.html": {`<script type="application/json" id="department-config">`, "</script>"},
	}
	cited := map[string]int{}
	for _, v := range mustViews(t, built) {
		cut, ok := configs[v.Path]
		if !ok {
			continue
		}
		reached, err := v.ReachedStems(ix)
		if err != nil {
			t.Fatalf("%s: %v", v.Path, err)
		}
		page, err := os.ReadFile(filepath.Join(opts.OutputDir, v.Path))
		if err != nil {
			t.Fatal(err)
		}
		_, rest, ok := strings.Cut(string(page), cut[0])
		if !ok {
			t.Fatalf("the served %s carries no config", v.Path)
		}
		body, _, _ := strings.Cut(rest, cut[1])
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
			t.Fatalf("decode %s's config: %v", v.Path, err)
		}
		for _, y := range config.Years {
			raw, err := os.ReadFile(filepath.Join(opts.OutputDir, filepath.FromSlash(y.Path)))
			if err != nil {
				t.Fatalf("read %s: %v", y.Path, err)
			}
			var column struct {
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
				stem, _ := ix.Stem(y.Path, key)
				if !slices.Contains(reached, stem) {
					continue
				}
				for _, l := range sched.Links {
					for _, loc := range l.Locators {
						for _, p := range loc.Pages {
							cited[v.Path]++
							if config.Docs[loc.DocID].Pages[strconv.Itoa(p)].Text == "" {
								t.Errorf("%s: %s's %s schedule cites %s p%d, and the config carries no links for it",
									v.Path, y.Path, key, loc.DocID, p)
							}
						}
					}
				}
			}
		}
	}
	for path := range configs {
		if cited[path] == 0 {
			t.Errorf("no link %s shows cites a page, so this test asserts nothing of it", path)
		}
	}
	t.Logf("cited pages, each with its links in its page's config: %v", cited)
}

// No refusal echoes a credential: a username is as secret as a password, and
// "/", "?", "#" or odd slashes move where a parser thinks userinfo ends.
// Mutation: drop the "@" arm -- "sec/ret" goes red with "repeats sec", "///"
// and "http:/" with "repeats secret", "p#x" with "repeats p#x".
func TestValidateBrowseURLEchoesNoUserinfo(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		secrets []string
	}{
		{"https://ghp_SECRET@github.com/you/livermore-budget/blob/main", []string{"ghp_SECRET"}},
		{"https://ghp_SECRET:hunter2@example.test/blob/main", []string{"ghp_SECRET", "hunter2"}},
		{"ftp://ghp_SECRET:hunter2@example.test/blob/main", []string{"ghp_SECRET", "hunter2"}},
		{"https://ghp_SECRET:hunter2@/blob/main", []string{"ghp_SECRET", "hunter2"}},
		{"https://ghp_SECRET:hunter2@example.test/blob/main?ref=x", []string{"ghp_SECRET", "hunter2"}},
		{"https://ghp_SECRET:hunter2@exa mple.test/blob/main", []string{"ghp_SECRET", "hunter2"}},
		{"https://ghp_SECRET:hunter2@example.test:port/blob/main", []string{"ghp_SECRET", "hunter2"}},
		{"ghp_SECRET:hunter2@example.test/blob/main", []string{"ghp_SECRET", "hunter2"}},
		{"ftp:ghp_SECRET:hunter2@example.test", []string{"ghp_SECRET", "hunter2"}},
		{"https://user:sec/ret@github.com/x", []string{"sec", "ret"}},
		{"https:///user:secret@host", []string{"secret"}},
		{"http:/user:secret@host", []string{"secret"}},
		{"https://u:p#x@host", []string{"p#x"}},
		{"https://u:p?x@host", []string{"p?x"}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			err := validateBrowseURL(tc.raw)
			if err == nil {
				t.Fatal("got nil error, want a refusal")
			}
			for _, secret := range tc.secrets {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error %q repeats %s", err, secret)
				}
			}
		})
	}
}

// Validate refuses an export into a directory fisc does not own -- `-o data`,
// `-o site` -- with or without --clean, before anything is written.
// Mutation: drop Validate's RefuseForeignDir call -- "got nil error, want a
// refusal of a directory fisc does not own".
func TestNewCmdExportRefusesADirectoryItDoesNotOwn(t *testing.T) {
	const edit = "// a local edit\n"
	dirs := map[string]map[string]string{
		"yaml-and-subdir": {"sources.yaml": "documents: []\n", "pdf/keep.txt": "x"},
		"source":          {"embed.go": "package site\n", "app.js": edit},
		// The marker an older binary wrote is not a manifest, so the site
		// beside it is nobody's export.
		"older-marker": {export.MarkerName: "fisc export\n", "index.html": "<html>\n"},
		// An older marker over nothing but
		// subdirectories holding source.
		"older-marker-over-source": {export.MarkerName: "fisc export\n", "cmd/fisc/main.go": "package main\n", "src/x.go": "package x\n"},
	}
	for name, seed := range dirs {
		for _, extra := range [][]string{nil, {"--clean"}} {
			t.Run(strings.Join(append([]string{name}, extra...), " "), func(t *testing.T) {
				dir := t.TempDir()
				for rel, body := range seed {
					path := filepath.Join(dir, filepath.FromSlash(rel))
					if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
						t.Fatalf("seed %s: %v", rel, err)
					}
					if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
						t.Fatalf("seed %s: %v", rel, err)
					}
				}
				io, _, _, _ := iostreams.Test()
				f := &cmdutil.Factory{IOStreams: io, RepoRoot: func() (string, error) { return "", nil }}
				ran := false
				cmd := NewCmdExport(f, func(*Options) error { ran = true; return nil })
				cmd.SetArgs(append([]string{"--output", dir}, extra...))
				cmd.SetOut(os.Stderr)
				cmd.SetErr(os.Stderr)
				err := cmd.Execute()

				if err == nil {
					t.Fatal("got nil error, want a refusal of a directory fisc does not own")
				}
				var flagErr *cmdutil.FlagError
				if !errors.As(err, &flagErr) {
					t.Errorf("got %T, want a *cmdutil.FlagError so the runner exits 2", err)
				}
				if !strings.Contains(err.Error(), "carries no") && !strings.Contains(err.Error(), "is not a manifest") {
					t.Errorf("got error %q, want it to say the directory carries no manifest", err)
				}
				if ran {
					t.Error("the run function was reached")
				}
				for rel, body := range seed {
					if b, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))); rerr != nil || string(b) != body {
						t.Errorf("%s = %q, %v; want it left alone", rel, b, rerr)
					}
				}
				if _, seeded := seed[export.MarkerName]; !seeded {
					if _, serr := os.Stat(filepath.Join(dir, export.MarkerName)); serr == nil {
						t.Error("the marker was written")
					}
				}
			})
		}
	}
}

// An unreadable output directory is not a FlagError: the runner exits 1.
// Mutation: wrap every RefuseForeignDir error in FlagErrorf -- "got
// *cmdutil.FlagError ... want an error the runner exits 1 on".
func TestNewCmdExportReportsAnUnreadableDirectoryAsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 directory")
	}
	dir := filepath.Join(t.TempDir(), "out")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
	io, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: io, RepoRoot: func() (string, error) { return "", nil }}
	cmd := NewCmdExport(f, func(*Options) error { return nil })
	cmd.SetArgs([]string{"--output", dir})
	cmd.SetOut(os.Stderr)
	cmd.SetErr(os.Stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("got nil error, want the unreadable directory reported")
	}
	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) {
		t.Errorf("got %T (%v), want an error the runner exits 1 on", err, err)
	}
}

// Validate accepts the directories export may write into: missing, empty,
// and one an earlier export marked.
func TestNewCmdExportAcceptsADirectoryItOwns(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"missing": func(t *testing.T) string { return filepath.Join(t.TempDir(), "site") },
		"empty":   func(t *testing.T) string { return t.TempDir() },
		"marked": func(t *testing.T) string {
			dir := t.TempDir()
			seedExport(t, dir, "index.html")
			return dir
		},
	}
	for name, mk := range cases {
		for _, extra := range [][]string{nil, {"--clean"}} {
			t.Run(strings.Join(append([]string{name}, extra...), " "), func(t *testing.T) {
				dir := mk(t)
				io, _, _, _ := iostreams.Test()
				f := &cmdutil.Factory{IOStreams: io, RepoRoot: func() (string, error) { return "", nil }}
				ran := false
				cmd := NewCmdExport(f, func(*Options) error { ran = true; return nil })
				cmd.SetArgs(append([]string{"--output", dir}, extra...))
				cmd.SetOut(os.Stderr)
				cmd.SetErr(os.Stderr)
				if err := cmd.Execute(); err != nil {
					t.Fatalf("got %v, want the directory accepted", err)
				}
				if !ran {
					t.Error("the run function was not reached")
				}
			})
		}
	}
}

// A cancellation during the build, with --clean over an earlier export, wraps
// context.Canceled (exit 130) and leaves the earlier site as it was.
// Mutation: drop exportRun's ctx checks and discard Write's error -- "got <nil>".
func TestExportRunStopsWhenCancelled(t *testing.T) {
	opts, _, out, _ := testOptions(t)
	opts.Clean = true
	if err := os.MkdirAll(opts.OutputDir, 0o750); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedExport(t, opts.OutputDir, "index.html")
	manifest := readFile(t, opts.OutputDir, export.MarkerName)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	build := opts.Build
	opts.Build = func(root string) (result, error) {
		cancel()
		return build(root)
	}

	if err := exportRun(ctx, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want it to wrap context.Canceled", err)
	}
	for name, want := range map[string]string{export.MarkerName: manifest, "index.html": "earlier export\n"} {
		if got := readFile(t, opts.OutputDir, name); got != want {
			t.Errorf("%s = %q, want the earlier export left alone", name, got)
		}
	}
	if out() != "" {
		t.Errorf("got stdout %q, want no paths reported", out())
	}
}

// seedExport makes dir an earlier export's output: each of files holding
// "earlier export", and a manifest listing them.
func seedExport(t *testing.T, dir string, files ...string) {
	t.Helper()
	listed := slices.Clone(files)
	slices.Sort(listed)
	body := export.ManifestHeader + "\n"
	for _, rel := range listed {
		body += rel + "\n"
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("seed %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("earlier export\n"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", rel, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, export.MarkerName), []byte(body), 0o600); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
}

// TestAPageThatDisclaimsAuditAssuranceDoesNotClaimItInItsProse: a view whose
// document ships statistical-section-unaudited claims no audit in its Title or
// Lede ("unaudited" lifted out); the anchor half fails if the caveat goes.
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
	// The anchor: the ACFR pages' documents still declare themselves
	// unaudited. If this half fails, the scan above has lost its subject --
	// decide whether the prose may now claim assurance before deleting it.
	for _, path := range []string{"change.html", "balances.html"} {
		if !shipping[path] {
			t.Errorf("%s's document no longer ships the %s caveat; the prose scan above "+
				"is checking nothing on the page it was written for", path, disclaimer)
		}
	}
}

// TestEveryProjectionBuildsToTheSameBytesTwice builds every projection twice
// and holds each to the same bytes.
// Mutation: range over a map where a builder emits its splits or caveats and
// this goes red on that projection.
func TestEveryProjectionBuildsToTheSameBytesTwice(t *testing.T) {
	first, second := builtStemsForTest(t), builtStemsForTest(t)
	if len(first) == 0 {
		t.Fatal("the committed store builds no projection, so nothing is compared")
	}
	if diff := cmp.Diff(slices.Sorted(maps.Keys(first)), slices.Sorted(maps.Keys(second))); diff != "" {
		t.Errorf("the two builds wrote different stems (-first +second):\n%s", diff)
	}
	for stem, b := range first {
		if !bytes.Equal(b, second[stem]) {
			t.Errorf("%s builds to different bytes on a second build", stem)
		}
	}
}
