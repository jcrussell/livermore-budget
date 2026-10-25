package cmdutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TempPrefix begins every WriteFile temp file name, followed by
// os.CreateTemp's decimal suffix. A dotfile, so no glob picks up a
// half-written facts.jsonl, and names fisc so IsTempName claims no other
// tool's residue.
const TempPrefix = ".fisc-tmp-"

const tempPattern = TempPrefix + "*"

// IsTempName reports whether name has the shape WriteFile gives its temp
// files: TempPrefix, then the one to ten ASCII digits of the decimal uint32
// os.CreateTemp puts in place of the "*".
func IsTempName(name string) bool {
	suffix, ok := strings.CutPrefix(name, TempPrefix)
	if !ok || len(suffix) == 0 || len(suffix) > 10 {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// dirPerm has no world bit (gosec G301).
const dirPerm fs.FileMode = 0o750

// WriteFile replaces the file at path with whatever fn writes, atomically: a
// temp file in the same directory is fsynced and renamed over the target, so a
// reader sees old or new, never torn, and a failure leaves no temp behind. The
// same directory matters: os.Rename is atomic only within a filesystem.
func WriteFile(path string, perm fs.FileMode, fn func(io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	name := tmp.Name()

	// Every path out before the rename removes the temp file.
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(name)
		}
	}()

	if err := fn(tmp); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	// Sync before the rename, or a crash can publish a name pointing at
	// nothing.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %q: %w", path, err)
	}
	// Chmod through the handle so perm lands on this file, not whatever the
	// name refers to by then.
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set mode on %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %q: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("rename %q to %q: %w", name, path, err)
	}
	renamed = true
	return nil
}
