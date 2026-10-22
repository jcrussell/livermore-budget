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

func TestSafeCleanDirRefusesADirectoryItDidNotWrite(t *testing.T) {
	// The failure this guards against: a typo'd --output naming a directory
	// full of somebody's work.
	dir := t.TempDir()
	keep := filepath.Join(dir, "thesis.txt")
	if err := os.WriteFile(keep, []byte("years of work"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := export.SafeCleanDir(dir)
	if err == nil {
		t.Fatal("got nil error, want a refusal to delete")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("got error %q, want it to say it is refusing", err)
	}
	if _, serr := os.Stat(keep); serr != nil {
		t.Errorf("the file was removed anyway: %v", serr)
	}
}

func TestSafeCleanDirEmptiesASiteItRecognises(t *testing.T) {
	cases := map[string][]string{
		"a finished site":  {export.MarkerName, "index.html", "stale.json"},
		"a partial export": {export.MarkerName, "stale.json"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			if err := export.SafeCleanDir(dir); err != nil {
				t.Fatalf("SafeCleanDir: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read cleaned dir: %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("got %d entries after cleaning, want 0", len(entries))
			}
		})
	}
}

func TestSafeCleanDirAcceptsEmptyAndMissingDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := export.SafeCleanDir(dir); err != nil {
		t.Errorf("empty directory: %v", err)
	}
	missing := filepath.Join(dir, "nested", "site")
	if err := export.SafeCleanDir(missing); err != nil {
		t.Errorf("missing directory: %v", err)
	}
	info, err := os.Stat(missing)
	if err != nil {
		t.Fatalf("stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("SafeCleanDir did not leave a directory behind")
	}
}

func TestSafeCleanDirRefusesAFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := export.SafeCleanDir(file)
	if err == nil {
		t.Fatal("got nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("got error %q, want it to say the path is not a directory", err)
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
// missing, empty, or carries the marker. A directory holding anything else
// -- `fisc export -o data` names one holding YAML and subdirectories, and
// `-o site` one holding Go -- is refused, and so is one holding a finished
// site's index.html with no marker, since nothing but the marker says fisc
// wrote it. Such a site is what an export made before the marker left, so
// the refusal says how to recover: remove the directory or name a new one.
//
// Mutation: make RefuseForeignDir return nil for any directory and every
// refused case goes red with "got nil error, want a refusal"; make the
// marker loop match nothing and the marker cases go red with "want the
// directory accepted"; drop the hint and the refused cases go red with "want
// it to say how to recover a site exported before the marker"; stop
// disregarding a stranded write probe and "only a stale write probe" goes
// red with "want the directory accepted".
func TestRefuseForeignDir(t *testing.T) {
	cases := []struct {
		name   string
		files  []string
		dirs   []string
		refuse bool
	}{
		{name: "a YAML file and a subdirectory", files: []string{"sources.yaml"}, dirs: []string{"pdf"}, refuse: true},
		{name: "source files", files: []string{"embed.go", "app.js"}, refuse: true},
		{name: "an index.html with no marker", files: []string{"index.html"}, refuse: true},
		{name: "a directory named like the marker", dirs: []string{export.MarkerName}, refuse: true},
		{name: "a file named like a write probe", files: []string{".fisc-write-probe-123"}, refuse: true},
		{name: "empty"},
		{name: "only a stale write probe", dirs: []string{".fisc-write-probe-123"}},
		{name: "a partial export and a stale write probe", files: []string{export.MarkerName}, dirs: []string{".fisc-write-probe-123"}},
		{name: "a partial export", files: []string{export.MarkerName}},
		{name: "a finished export", files: []string{export.MarkerName, "index.html"}, dirs: []string{"data"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
					t.Fatalf("seed %s: %v", f, err)
				}
			}
			for _, d := range tc.dirs {
				if err := os.Mkdir(filepath.Join(dir, d), 0o750); err != nil {
					t.Fatalf("seed %s: %v", d, err)
				}
			}
			err := export.RefuseForeignDir(dir)
			switch {
			case tc.refuse && err == nil:
				t.Fatal("got nil error, want a refusal")
			case tc.refuse && !strings.Contains(err.Error(), "carries no "+export.MarkerName):
				t.Errorf("got error %q, want it to name the missing marker", err)
			case tc.refuse && !strings.Contains(err.Error(), "remove the directory or name a new one"):
				t.Errorf("got error %q, want it to say how to recover a site exported before the marker", err)
			case !tc.refuse && err != nil:
				t.Errorf("got %v, want the directory accepted", err)
			}
			if !tc.refuse {
				return
			}
			if cerr := export.SafeCleanDir(dir); cerr == nil {
				t.Error("SafeCleanDir accepted a directory RefuseForeignDir refuses")
			}
			for _, f := range append(tc.files, tc.dirs...) {
				if _, serr := os.Stat(filepath.Join(dir, f)); serr != nil {
					t.Errorf("%s was removed: %v", f, serr)
				}
			}
		})
	}
	if err := export.RefuseForeignDir(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("missing directory: %v", err)
	}
}

// A refusal matches ErrForeignDir; failing to read the directory does not.
//
// Mutation: return a plain fmt.Errorf from the unmarked-directory arm and the
// first case goes red with "does not match ErrForeignDir".
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

// A marker is not proof by itself: one a binary wrote into data/ or site/
// before it refused those, or one copied by hand, sits beside source, and
// --clean would RemoveAll the source with it. RefuseForeignDir refuses such a
// directory for writing, and SafeCleanDir, which goes through it, leaves
// every file in place.
//
// Mutation: make looksLikeSource report nothing and every case goes red with
// "got nil error, want a refusal of a marked directory holding source".
func TestSafeCleanDirRefusesASourceDirectory(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		dirs  []string
	}{
		{name: "a Go package", files: []string{"embed.go", "index.html", export.MarkerName}},
		{name: "data", files: []string{"sources.yaml", export.MarkerName}, dirs: []string{"pdf"}},
		{name: "a checkout", files: []string{export.MarkerName}, dirs: []string{".git"}},
		{name: "the fact store", files: []string{"facts.jsonl", export.MarkerName}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
					t.Fatalf("seed %s: %v", f, err)
				}
			}
			for _, d := range tc.dirs {
				if err := os.Mkdir(filepath.Join(dir, d), 0o750); err != nil {
					t.Fatalf("seed %s: %v", d, err)
				}
			}
			err := export.RefuseForeignDir(dir)
			if err == nil {
				t.Fatal("got nil error, want a refusal of a marked directory holding source")
			}
			if !strings.Contains(err.Error(), "no export writes") {
				t.Errorf("error %q does not say why it refused", err)
			}
			if err := export.SafeCleanDir(dir); err == nil {
				t.Error("SafeCleanDir on a marked directory holding source = nil error, want a refusal")
			}
			for _, f := range append(tc.files, tc.dirs...) {
				if _, serr := os.Stat(filepath.Join(dir, f)); serr != nil {
					t.Errorf("%s was removed anyway: %v", f, serr)
				}
			}
		})
	}
}
