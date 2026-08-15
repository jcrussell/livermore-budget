package cmdutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir moves into dir for the duration of the test. t.Chdir handles the
// restore and refuses to run in a parallel test, which manual save/restore
// does not.
func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}

// newRepo lays out a fake repository root containing the marker file.
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, repoMarker), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	// t.TempDir may sit behind a symlink (/tmp -> /private/tmp on macOS), and
	// Getwd resolves it, so compare against the resolved path.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	return resolved
}

func TestFindRepoRootFromRoot(t *testing.T) {
	root := newRepo(t)
	chdir(t, root)

	got, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

// TestFindRepoRootFromSubdirectory is the behavior that matters: fisc should
// work from anywhere in the tree, the way git does.
func TestFindRepoRootFromSubdirectory(t *testing.T) {
	root := newRepo(t)
	deep := filepath.Join(root, "internal", "mapping", "testdata")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	chdir(t, deep)

	got, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestFindRepoRootOutsideRepoIsHinted(t *testing.T) {
	chdir(t, t.TempDir())

	_, err := findRepoRoot()
	if err == nil {
		t.Fatal("got nil error, want a failure outside any repository")
	}
	var hint *ErrHint
	if !errors.As(err, &hint) {
		t.Fatalf("got %T, want an *ErrHint so the user is told what to do", err)
	}
	if !strings.Contains(err.Error(), repoMarker) {
		t.Errorf("got %q, want it to name the missing marker %q", err, repoMarker)
	}
}
