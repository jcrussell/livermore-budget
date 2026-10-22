package export

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MarkerName is the sentinel file `fisc export` writes at the very start
// of a run to mark the output directory as fisc's, and therefore safe to
// write into and to clean. It goes down first so a run that fails midway
// still leaves a directory RefuseForeignDir accepts, keeping a partial export
// recoverable with --clean.
const MarkerName = ".fisc-export"

// probePrefix names the directory WritableDir creates and removes. A process
// that dies between the two strands it empty, so RefuseForeignDir disregards
// an empty directory of this name rather than refusing every later export.
const probePrefix = ".fisc-write-probe-"

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
	// homes and on macOS -- outputdir_test.go already works around the same thing
	// for /tmp.
	if home, herr := os.UserHomeDir(); herr == nil && home != "" {
		if rhome, rerr := evalExistingSymlinks(home); rerr == nil && resolved == rhome {
			return "", fmt.Errorf("refusing to use home directory %q as output directory", resolved)
		}
	}
	return resolved, nil
}

// ErrForeignDir matches every refusal RefuseForeignDir makes, and none of
// the I/O errors it can meet reading the directory: a refusal means the
// caller named a directory fisc must not write into, an I/O error means the
// environment failed, and the two exit differently.
var ErrForeignDir = errors.New("output directory is not one fisc may write into")

type foreignDirError struct{ msg string }

func (e *foreignDirError) Error() string        { return e.msg }
func (e *foreignDirError) Is(target error) bool { return target == ErrForeignDir }

func refuse(format string, a ...any) error {
	return &foreignDirError{msg: fmt.Sprintf(format, a...)}
}

// RefuseForeignDir refuses an output directory fisc does not own: one that
// exists, holds anything but a stranded write probe, and carries no
// MarkerName. Export writes only into a directory that is missing, empty, or
// marked by an earlier export, because writing anywhere else leaves the
// marker beside files fisc did not write, and a later --clean would take the
// marker at its word and delete them --
// `fisc export -o data` followed by `--clean` would remove the PDFs and the
// extracted pages. An unreadable directory is refused rather than assumed
// empty.
//
// The marker is not the whole rule: a marked directory that also looks like
// source (looksLikeSource) is refused too, because a marker can be stale --
// written by a binary that accepted `-o data` -- or copied by hand, and
// --clean would RemoveAll whatever sits beside it.
func RefuseForeignDir(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return refuse("output path %q exists but is not a directory", dir)
	}
	all, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read output dir: %w", err)
	}
	entries := all[:0:0]
	for _, e := range all {
		if !strandedProbe(dir, e) {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil
	}
	marked := false
	for _, e := range entries {
		if e.Name() == MarkerName && e.Type().IsRegular() {
			marked = true
			break
		}
	}
	if !marked {
		return refuse(
			"output directory %q is not empty and carries no %s marker from an earlier export; refusing to write a site into it. "+
				"If it is a site an export wrote before exports carried the marker, remove the directory or name a new one",
			dir, MarkerName)
	}
	if name, ok := looksLikeSource(entries); ok {
		return refuse(
			"output directory %q carries a %s marker and also holds %s, which no export writes; refusing to write a site into it or clean it",
			dir, MarkerName, name)
	}
	return nil
}

// strandedProbe reports whether e is a WritableDir probe left behind empty.
// One holding anything, or unreadable, is not disregarded: only an empty
// directory says nothing but that a probe was interrupted.
func strandedProbe(dir string, e os.DirEntry) bool {
	if !e.IsDir() || !strings.HasPrefix(e.Name(), probePrefix) {
		return false
	}
	inside, err := os.ReadDir(filepath.Join(dir, e.Name()))
	return err == nil && len(inside) == 0
}

// looksLikeSource returns the first top-level entry of a directory that a
// human's work or a checkout put there and no export writes: a .git, or a
// file whose extension the site never carries. data/'s top level is YAML,
// site/'s is Go and templates, facts/' is the JSONL fact store, and the
// repository root's is a go.mod.
func looksLikeSource(entries []os.DirEntry) (string, bool) {
	for _, e := range entries {
		if e.Name() == ".git" {
			return e.Name(), true
		}
		if e.IsDir() {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".go", ".tmpl", ".mod", ".sum", ".mjs", ".yaml", ".yml", ".py", ".pdf", ".md", ".jsonl":
			return e.Name(), true
		}
	}
	return "", false
}

// SafeCleanDir removes outputDir and recreates it empty, but only when
// RefuseForeignDir accepts it, so `fisc export --clean -o ~/documents`
// cannot wipe the documents. The path is canonicalised and screened by
// ResolveOutputDir first.
//
// Checking the marker means reading the directory before removing it, so
// this is a read-then-RemoveAll rather than a bare RemoveAll. The TOCTOU
// window that opens is worth the guard for a single-user CLI: the failure it
// prevents is unrecoverable data loss, and the failure it admits requires
// someone to swap the directory mid-run.
func SafeCleanDir(outputDir string) error {
	resolved, err := ResolveOutputDir(outputDir)
	if err != nil {
		return err
	}
	if err := RefuseForeignDir(resolved); err != nil {
		return err
	}
	if err := os.RemoveAll(resolved); err != nil {
		return fmt.Errorf("clean output dir: %w", err)
	}
	// #nosec G301 -- the caller is building a web root; 0750 would hide
	// the published site from a server running as another user.
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
			tmp, terr := os.MkdirTemp(target, probePrefix+"*")
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
