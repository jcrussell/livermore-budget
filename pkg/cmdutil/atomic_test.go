package cmdutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// entries lists the names in dir, sorted, so a test can assert that a failed
// write left nothing behind.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(des))
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestWriteFile(t *testing.T) {
	// A nested path exercises the parent-directory creation: a first build in
	// a fresh checkout has no facts/ directory.
	path := filepath.Join(t.TempDir(), "facts", "facts.jsonl")

	err := WriteFile(path, 0o644, func(w io.Writer) error {
		_, err := io.WriteString(w, "one\ntwo\n")
		return err
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if diff := cmp.Diff("one\ntwo\n", readFile(t, path)); diff != "" {
		t.Errorf("contents mismatch (-want +got):\n%s", diff)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// os.CreateTemp makes a 0600 file, so the requested mode is only correct
	// if WriteFile applied it. Chmod ignores the umask, so this is exact.
	if got, want := info.Mode().Perm(), fs.FileMode(0o644); got != want {
		t.Errorf("mode = %v, want %v", got, want)
	}
	if got := entries(t, filepath.Dir(path)); len(got) != 1 {
		t.Errorf("directory holds %v, want only the target", got)
	}
}

// TestWriteFileKeepsItsTempFileBesideTheTarget pins the property the whole
// design rests on: rename is atomic only within a filesystem, so a temp file
// anywhere but the target's own directory would degrade to a copy across a
// mount boundary — and it would do so silently, on someone else's machine.
func TestWriteFileKeepsItsTempFileBesideTheTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "facts.jsonl")

	var duringWrite []string
	err := WriteFile(path, 0o644, func(w io.Writer) error {
		duringWrite = entries(t, dir)
		_, err := io.WriteString(w, "x\n")
		return err
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if len(duringWrite) != 1 || !strings.HasPrefix(duringWrite[0], ".tmp-") {
		t.Errorf("during the write the directory held %v, want one .tmp- file", duringWrite)
	}
	// The target must not exist until the rename, or a reader can see a
	// partial file.
	if duringWrite[0] == filepath.Base(path) {
		t.Error("the target existed before the rename")
	}
}

// TestWriteFileFailureLeavesTheTargetAlone is the reason a build calls this
// instead of os.WriteFile: a rule that fails half way through must not leave a
// truncated facts.jsonl that verify then reports as drift.
func TestWriteFileFailureLeavesTheTargetAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "facts.jsonl")
	if err := os.WriteFile(path, []byte("previous\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	boom := errors.New("rule failed")
	err := WriteFile(path, 0o644, func(w io.Writer) error {
		// Bytes already written must not survive either.
		if _, err := io.WriteString(w, "partial\n"); err != nil {
			return err
		}
		return fmt.Errorf("resolve: %w", boom)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WriteFile error = %v, want it to wrap the writer's error", err)
	}
	if diff := cmp.Diff("previous\n", readFile(t, path)); diff != "" {
		t.Errorf("target contents mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"facts.jsonl"}, entries(t, dir)); diff != "" {
		t.Errorf("directory contents mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteFileWithAnUnwritableParent(t *testing.T) {
	// A path whose parent is a regular file cannot be created, and the failure
	// must arrive before anything is opened.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "facts")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	called := false
	err := WriteFile(filepath.Join(blocker, "facts.jsonl"), 0o644, func(io.Writer) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("WriteFile = nil error, want a failure")
	}
	if called {
		t.Error("the writer ran although the directory could not be created")
	}
}
