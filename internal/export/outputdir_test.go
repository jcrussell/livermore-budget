package export_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

func TestResolveOutputDirCanonicalises(t *testing.T) {
	dir := t.TempDir()
	got, err := export.ResolveOutputDir(filepath.Join(dir, "a", "..", "site"))
	if err != nil {
		t.Fatalf("ResolveOutputDir: %v", err)
	}
	// t.TempDir hands back a path under /tmp, which is a symlink on macOS, so
	// resolve the expectation the same way rather than comparing raw strings.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if got != filepath.Join(want, "site") {
		t.Errorf("got %q, want %q", got, filepath.Join(want, "site"))
	}
}

func TestResolveOutputDirRefusesDangerousTargets(t *testing.T) {
	if _, err := export.ResolveOutputDir(""); err == nil {
		t.Error("got nil error for an empty path, want a refusal")
	}
	if _, err := export.ResolveOutputDir(string(os.PathSeparator)); err == nil {
		t.Error("got nil error for the filesystem root, want a refusal")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory to test against")
	}
	if _, err := export.ResolveOutputDir(home); err == nil {
		t.Error("got nil error for the home directory, want a refusal")
	}
}

func TestWritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := export.WritableDir(dir); err != nil {
		t.Errorf("existing directory: %v", err)
	}
	// A path that does not exist yet is answered by its nearest existing
	// ancestor, because that is where the first mkdir will land.
	if err := export.WritableDir(filepath.Join(dir, "not", "yet")); err != nil {
		t.Errorf("missing directory: %v", err)
	}

	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := export.WritableDir(file); err == nil {
		t.Error("got nil error for a file, want a refusal")
	}

	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not apply")
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := export.WritableDir(locked)
	if err == nil {
		t.Fatal("got nil error for a read-only directory, want a refusal")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("got error %v, want it to wrap os.ErrPermission", err)
	}
}

// TestRefuseForeignDir: export writes only into a directory that is
// missing, empty, or carries a valid manifest. A directory holding anything
// else -- `fisc export -o data` names one holding YAML and subdirectories,
// and `-o site` one holding Go -- is refused, and so is one holding a
// finished site's index.html with no manifest, or with a marker that is not
// one: the bare "fisc export" an older binary wrote, or any listing that
// could name a path outside the directory or name one twice. Every refusal
// says how to recover: remove the directory and export again.
//
// Mutation: make RefuseForeignDir return nil for any directory and every
// refused case goes red with "got nil error, want a refusal"; make
// parseManifest accept any marker and the invalid-marker cases go red with
// "got nil error, want a refusal"; drop the header check and "the older
// binary's marker" goes red the same way; drop the order check and
// "unsorted" and "repeated" do; drop the hint and the refused cases go red
// with "want it to say how to recover"; stop disregarding a stranded write
// probe and "only a stale write probe" goes red with "want the directory
// accepted"; stop disregarding a stranded manifest temp file and "only a
// stranded manifest temp file" goes red the same way; disregard every entry
// cmdutil.IsTempName accepts whatever its type and "a directory named like a
// temp file" goes red with "got nil error, want a refusal".
func TestRefuseForeignDir(t *testing.T) {
	const h = export.ManifestHeader + "\n"
	noMarker := "carries no " + export.MarkerName
	notManifest := "is not a manifest this fisc reads"
	cases := []struct {
		name   string
		marker string // the marker's text, written when not empty
		files  []string
		dirs   []string
		refuse string // what the refusal says; empty means accepted
	}{
		{name: "a YAML file and a subdirectory", files: []string{"sources.yaml"}, dirs: []string{"pdf"}, refuse: noMarker},
		{name: "source files", files: []string{"embed.go", "app.js"}, refuse: noMarker},
		{name: "an index.html with no marker", files: []string{"index.html"}, refuse: noMarker},
		{name: "a file named like a write probe", files: []string{".fisc-write-probe-123"}, refuse: noMarker},
		{name: "a directory named like the marker", dirs: []string{export.MarkerName}, refuse: "not a regular file"},
		{name: "the older binary's marker", marker: "fisc export\n", files: []string{"index.html"}, refuse: notManifest},
		{name: "an empty marker", marker: "\n", refuse: notManifest},
		{name: "no final newline", marker: export.ManifestHeader, refuse: notManifest},
		{name: "a later version", marker: "fisc export manifest 2\n", refuse: notManifest},
		{name: "an absolute entry", marker: h + "/etc/passwd\n", refuse: notManifest},
		{name: "a parent entry", marker: h + "../escape\n", refuse: notManifest},
		{name: "a parent within", marker: h + "data/../../escape\n", refuse: notManifest},
		{name: "an empty line", marker: h + "\nindex.html\n", refuse: notManifest},
		{name: "an unclean entry", marker: h + "./index.html\n", refuse: notManifest},
		{name: "a dot entry", marker: h + ".\n", refuse: notManifest},
		{name: "a backslash", marker: h + "data\\x.json\n", refuse: notManifest},
		{name: "a carriage return", marker: h + "index.html\r\n", refuse: notManifest},
		{name: "the marker listed", marker: h + export.MarkerName + "\n", refuse: notManifest},
		{name: "unsorted", marker: h + "vendor/d3.js\nindex.html\n", refuse: notManifest},
		{name: "repeated", marker: h + "index.html\nindex.html\n", refuse: notManifest},
		{name: "empty"},
		{name: "only a stale write probe", dirs: []string{".fisc-write-probe-123"}},
		{name: "a partial export and a stale write probe", files: []string{export.MarkerName}, dirs: []string{".fisc-write-probe-123"}},
		{name: "a partial export", files: []string{export.MarkerName}},
		{name: "only a stranded manifest temp file", files: []string{".fisc-tmp-123"}},
		{name: "a partial export and a stranded manifest temp file", files: []string{export.MarkerName, ".fisc-tmp-123"}},
		{name: "a file named like a temp file but not shaped as one", files: []string{".fisc-tmp-abc"}, refuse: noMarker},
		{name: "a directory named like a temp file", dirs: []string{".fisc-tmp-123"}, refuse: noMarker},
		{name: "another tool's temp file", files: []string{".tmp-123"}, refuse: noMarker},
		{name: "a finished export", files: []string{export.MarkerName, "index.html"}, dirs: []string{"data"}},
		{name: "a manifest listing files", marker: h + "index.html\nvendor/d3.js\n", files: []string{"index.html"}},
		// The directory fisc-3s5d.65 is about: an older marker over nothing
		// but subdirectories holding source. Only a manifest says fisc's.
		{name: "the older binary's marker over source below", marker: "fisc export\n", files: []string{"cmd/fisc/main.go", "src/x/x.go"}, refuse: notManifest},
		// A manifest is the whole rule: what else is there is never at risk,
		// because --clean removes no file the manifest does not list but a
		// stranded manifest temp file.
		{name: "a manifest beside files no export wrote", files: []string{export.MarkerName, "notes.md", "sub/keep.go", ".git/HEAD"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFiles(t, dir, tc.files...)
			if tc.marker != "" {
				if err := os.WriteFile(filepath.Join(dir, export.MarkerName), []byte(tc.marker), 0o600); err != nil {
					t.Fatalf("seed marker: %v", err)
				}
			}
			for _, d := range tc.dirs {
				if err := os.Mkdir(filepath.Join(dir, d), 0o750); err != nil {
					t.Fatalf("seed %s: %v", d, err)
				}
			}
			err := export.RefuseForeignDir(dir)
			switch {
			case tc.refuse != "" && err == nil:
				t.Fatal("got nil error, want a refusal")
			case tc.refuse != "" && !errors.Is(err, export.ErrForeignDir):
				t.Errorf("got error %v, want a refusal matching ErrForeignDir", err)
			case tc.refuse != "" && !strings.Contains(err.Error(), tc.refuse):
				t.Errorf("got error %q, want it to say %q", err, tc.refuse)
			case tc.refuse != "" && !strings.Contains(err.Error(), "remove the directory and export again, or name a new one"):
				t.Errorf("got error %q, want it to say how to recover", err)
			case tc.refuse == "" && err != nil:
				t.Errorf("got %v, want the directory accepted", err)
			}
		})
	}
	if err := export.RefuseForeignDir(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("missing directory: %v", err)
	}
}

// A refusal matches ErrForeignDir; failing to read the directory or its
// manifest does not.
//
// Mutation: return a plain fmt.Errorf from the unmarked-directory arm and the
// first case goes red with "does not match ErrForeignDir"; make readManifest
// report a read failure as an invalid manifest and the last goes red with
// "reading the manifest matches ErrForeignDir".
func TestRefuseForeignDirTellsARefusalFromAnIOError(t *testing.T) {
	refused := t.TempDir()
	if err := os.WriteFile(filepath.Join(refused, "sources.yaml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := export.RefuseForeignDir(refused); !errors.Is(err, export.ErrForeignDir) {
		t.Errorf("refusal %v does not match ErrForeignDir", err)
	}
	if os.Geteuid() == 0 {
		return
	}
	unreadable := filepath.Join(t.TempDir(), "out")
	if err := os.Mkdir(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o750) })
	err := export.RefuseForeignDir(unreadable)
	if err == nil {
		t.Fatal("got nil error for an unreadable directory")
	}
	if errors.Is(err, export.ErrForeignDir) {
		t.Errorf("I/O error %v matches ErrForeignDir", err)
	}

	// A manifest that is there and cannot be read is the same: nothing says
	// it is not a valid one.
	marked := t.TempDir()
	seedFiles(t, marked, export.MarkerName, "index.html")
	marker := filepath.Join(marked, export.MarkerName)
	if cerr := os.Chmod(marker, 0o000); cerr != nil {
		t.Fatal(cerr)
	}
	err = export.RefuseForeignDir(marked)
	if err == nil {
		t.Fatal("got nil error for an unreadable manifest")
	}
	if errors.Is(err, export.ErrForeignDir) {
		t.Errorf("I/O error %v reading the manifest matches ErrForeignDir", err)
	}
}

// A directory named like a write probe that holds something is not a
// stranded probe, and does not make an unmarked directory look empty.
//
// Mutation: drop strandedProbe's emptiness test and this goes red with "got
// nil error, want a refusal".
func TestRefuseForeignDirKeepsAProbeNamedDirectoryThatHoldsAFile(t *testing.T) {
	dir := t.TempDir()
	probe := filepath.Join(dir, ".fisc-write-probe-123")
	if err := os.Mkdir(probe, 0o750); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(probe, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := export.RefuseForeignDir(dir); err == nil {
		t.Fatal("got nil error, want a refusal")
	}
}

// seedFiles writes each of files into dir. A MarkerName among them is a valid
// manifest listing nothing; any other file holds "x".
func seedFiles(t *testing.T, dir string, files ...string) {
	t.Helper()
	for _, f := range files {
		body := "x"
		if f == export.MarkerName {
			body = export.ManifestHeader + "\n"
		}
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("seed %s: %v", f, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", f, err)
		}
	}
}
