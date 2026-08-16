package export

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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
	return root
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
		{name: "empty output", args: []string{"--output", ""}, wantErr: "--output requires a directory"},
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
	if diff := cmp.Diff([]string{"sankey"}, keys(got)); diff != "" {
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

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
