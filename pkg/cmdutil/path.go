package cmdutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ExportMarkerName is the sentinel file `fisc export` writes at the very start
// of a run to mark the output directory as fisc's, and therefore safe to
// clean. index.html is written last, so a run that fails midway would
// otherwise leave a non-empty tree with no index.html — which SafeCleanDir
// refuses to delete, dead-ending the retry. The early marker keeps a partial
// export recoverable with --clean.
const ExportMarkerName = ".fisc-export"

// ResolveOutputDir abs-resolves a user-supplied output directory, follows
// symlinks on its existing prefix, and refuses the two locations where a
// mistyped flag does the most damage: the filesystem root and the user's home
// directory. This is byob-input-validation.1 for the CLI output-dir case,
// where — unlike an archive entry or a template include — the user may
// legitimately name an absolute path outside the working directory, so
// containment inside a base is the wrong check.
func ResolveOutputDir(outputDir string) (string, error) {
	if outputDir == "" {
		return "", errors.New("output directory is required")
	}
	abs, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", outputDir, err)
	}
	resolved, err := evalExistingSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", outputDir, err)
	}
	if resolved == string(os.PathSeparator) {
		return "", fmt.Errorf("refusing to use filesystem root %q as output directory", resolved)
	}
	// Both sides are symlink-resolved before comparing. Comparing a resolved
	// target against a raw $HOME silently disarms the guard wherever the home
	// directory traverses a symlink, which is the normal case on NFS-mounted
	// homes and on macOS -- path_test.go already works around the same thing
	// for /tmp.
	if home, herr := os.UserHomeDir(); herr == nil && home != "" {
		if rhome, rerr := evalExistingSymlinks(home); rerr == nil && resolved == rhome {
			return "", fmt.Errorf("refusing to use home directory %q as output directory", resolved)
		}
	}
	return resolved, nil
}

// looksLikeSource reports whether dir holds files a build produced from a
// human's work rather than files a generator wrote.
//
// The marker sentinel makes a directory reusable, but it cannot tell an
// exported site from a source directory that was once exported into: after one
// run, `--output site` leaves index.html and the marker beside site/embed.go,
// and the NEXT --clean deletes the Go package. That happened to be spelled out
// in this command's own Example. A generated site contains no source, so the
// presence of any is the signal.
func looksLikeSource(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".go", ".tmpl", ".mod", ".sum":
			return true
		}
	}
	return false
}

// SafeCleanDir removes outputDir and recreates it empty, but only when the
// directory is empty or looks like one fisc generated — one holding either a
// finished site's index.html or the ExportMarkerName sentinel. It refuses to
// delete a non-empty directory with neither, so `fisc export --clean -o
// ~/documents` cannot wipe the documents. The path is canonicalised and
// screened by ResolveOutputDir first.
//
// Checking the sentinel means reading the directory before removing it, so
// this is a read-then-RemoveAll rather than a bare RemoveAll. The TOCTOU
// window that opens is worth the guard for a single-user CLI: the failure it
// prevents is unrecoverable data loss, and the failure it admits requires
// someone to swap the directory mid-run.
func SafeCleanDir(outputDir string) error {
	resolved, err := ResolveOutputDir(outputDir)
	if err != nil {
		return err
	}

	info, err := os.Stat(resolved)
	if errors.Is(err, os.ErrNotExist) {
		// #nosec G301 -- the caller is building a web root; 0750 would hide
		// the published site from a server running as another user.
		return os.MkdirAll(resolved, 0o755)
	}
	if err != nil {
		return fmt.Errorf("stat %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path %q exists but is not a directory", resolved)
	}

	if looksLikeSource(resolved) {
		return fmt.Errorf("output directory %q holds source files; refusing to delete", resolved)
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return fmt.Errorf("read output dir: %w", err)
	}
	if len(entries) > 0 {
		managed := false
		for _, e := range entries {
			if e.Name() == "index.html" || e.Name() == ExportMarkerName {
				managed = true
				break
			}
		}
		if !managed {
			return fmt.Errorf(
				"output directory %q is not empty and does not look like a generated site (no index.html and no %s marker); refusing to delete it",
				resolved, ExportMarkerName)
		}
	}

	if err := os.RemoveAll(resolved); err != nil {
		return fmt.Errorf("clean output dir: %w", err)
	}
	// #nosec G301 -- as above.
	return os.MkdirAll(resolved, 0o755)
}

// WritableDir reports whether path — or, if it does not exist yet, its
// nearest existing ancestor — is a directory this process can write to. It
// probes by creating and removing a hidden temporary directory, because the
// permission bits alone do not answer the question on a read-only mount, over
// NFS, or under a container's user mapping. It does not create path itself.
func WritableDir(path string) error {
	target := path
	for {
		info, err := os.Stat(target)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%q is not a directory", target)
			}
			tmp, terr := os.MkdirTemp(target, ".fisc-write-probe-*")
			if terr != nil {
				return fmt.Errorf("directory %q is not writable: %w", target, terr)
			}
			return os.Remove(tmp)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %q: %w", target, err)
		}
		parent := filepath.Dir(target)
		if parent == target {
			return fmt.Errorf("no existing ancestor for %q", path)
		}
		target = parent
	}
}

// evalExistingSymlinks resolves symlinks in the longest existing prefix of p
// and rejoins the components that do not exist yet. filepath.EvalSymlinks
// rejects a path that is not there, which is exactly the case a CLI cares
// about: "I am about to create this, but canonicalise it first."
func evalExistingSymlinks(p string) (string, error) {
	p = filepath.Clean(p)
	var trailing []string
	cur := p
	for {
		if _, err := os.Lstat(cur); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		dir, last := filepath.Split(cur)
		trailing = append([]string{last}, trailing...)
		if dir == "" {
			cur = "."
			break
		}
		next := filepath.Clean(dir)
		if next == cur {
			break
		}
		cur = next
	}
	resolved, err := filepath.EvalSymlinks(cur)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{resolved}, trailing...)...), nil
}
