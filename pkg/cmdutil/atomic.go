package cmdutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// tempPattern is what an interrupted WriteFile leaves behind. It is a dotfile
// so a half-written facts.jsonl is never picked up by a glob, and it is
// spelled once here so a future sweep can recognize the residue rather than
// hard-coding a second copy that can drift.
const tempPattern = ".tmp-*"

// dirPerm is the mode a created parent directory gets: owner and group, no
// world bit, which is what gosec's G301 asks for and costs nothing here.
const dirPerm fs.FileMode = 0o750

// WriteFile replaces the file at path with whatever fn writes, atomically: fn
// writes into a temporary file in the SAME directory, which is fsynced and
// then renamed over the target. A reader therefore sees the old file or the
// new one, never a torn one, and a failure leaves the target exactly as it was
// with no temp file behind it.
//
// The same directory is the load-bearing detail, not an implementation choice.
// os.Rename is atomic only within a filesystem; a temp file in os.TempDir()
// would silently degrade to a copy across a mount boundary, which is precisely
// the torn write this exists to prevent — and it would be invisible on the
// machine where it was written.
//
// It takes a writer func rather than a []byte so a caller can stream: facts is
// a JSON Lines file whose whole point is that it is written a record at a time.
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

	// Every path out of here before the rename must take the temp file with
	// it, including the ones fn causes.
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
	// Sync before the rename, not after: a rename that beats its own data to
	// disk publishes a name pointing at nothing after a crash.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %q: %w", path, err)
	}
	// Chmod through the handle rather than the name. os.CreateTemp makes the
	// file 0600, and the caller's perm must land on this file rather than on
	// whatever the name happens to refer to by the time the call runs.
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
