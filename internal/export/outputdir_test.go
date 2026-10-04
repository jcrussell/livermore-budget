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
	if !strings.Contains(err.Error(), "refusing to delete") {
		t.Errorf("got error %q, want it to say it is refusing", err)
	}
	if _, serr := os.Stat(keep); serr != nil {
		t.Errorf("the file was removed anyway: %v", serr)
	}
}

func TestSafeCleanDirEmptiesASiteItRecognises(t *testing.T) {
	cases := map[string]string{
		"a finished site":  "index.html",
		"a partial export": export.MarkerName,
	}
	for name, sentinel := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{sentinel, "stale.json"} {
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

// TestSafeCleanDirRefusesASourceDirectory covers the path that actually lost
// files: `fisc export -o site` (which this command's Example once suggested)
// drops index.html and the marker beside site/embed.go, so the marker check
// accepts the directory on the NEXT run and RemoveAll takes the Go package
// with it. A generated site contains no source, so any source is the signal.
func TestSafeCleanDirRefusesASourceDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"embed.go", "index.html", export.MarkerName} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	err := export.SafeCleanDir(dir)
	if err == nil {
		t.Fatal("SafeCleanDir on a directory holding .go files = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "source") {
		t.Errorf("error %q does not say why it refused", err)
	}
	if _, serr := os.Stat(filepath.Join(dir, "embed.go")); serr != nil {
		t.Errorf("the source file was removed anyway: %v", serr)
	}
}
