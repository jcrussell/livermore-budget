package export

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// MarkerName is the manifest `fisc export` keeps in the output directory: the
// header line ManifestHeader, then the sorted, slash-separated relative path of
// every file the export wrote, one per line, itself excluded. A directory is
// fisc's only while it carries a valid one, and --clean removes nothing it
// does not list.
const MarkerName = ".fisc-export"

// ManifestHeader is the manifest's first line. A marker with any other first
// line -- the bare "fisc export" an older binary wrote included -- is not a
// manifest, and the directory carrying it is not fisc's.
const ManifestHeader = "fisc export manifest 1"

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

func (e *foreignDirError) Error() string      { return e.msg }
func (*foreignDirError) Is(target error) bool { return target == ErrForeignDir }

func refuse(format string, a ...any) error {
	return &foreignDirError{msg: fmt.Sprintf(format, a...)}
}

// RefuseForeignDir refuses an output directory fisc does not own: one that
// exists, holds anything but a stranded write probe, and carries no valid
// MarkerName manifest. Export writes only into a directory that is missing,
// empty, or carries an earlier export's manifest; writing anywhere else would
// overwrite files fisc did not write. An unreadable directory or manifest is
// an I/O error rather than a refusal, and neither is assumed empty.
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
	_, err = readManifest(dir)
	var invalid *invalidManifestError
	switch {
	case errors.Is(err, errNoManifest):
		return refuse(
			"output directory %q is not empty and carries no %s manifest from an earlier export; refusing to write a site into it. "+
				"If an earlier export wrote it, remove the directory and export again, or name a new one",
			dir, MarkerName)
	case errors.As(err, &invalid):
		return refuse(
			"output directory %q carries a %s that is not a manifest this fisc reads (%s); refusing to write a site into it. "+
				"If an earlier export wrote it, remove the directory and export again, or name a new one",
			dir, MarkerName, invalid.reason)
	case err != nil:
		return err
	}
	if name, ok := looksLikeSource(entries); ok {
		return refuse(
			"output directory %q carries a %s marker and also holds %s, which no export writes; refusing to write a site into it or clean it",
			dir, MarkerName, name)
	}
	return nil
}

// errNoManifest is readManifest's answer for a directory with no MarkerName.
var errNoManifest = errors.New("no export manifest")

// invalidManifestError is readManifest's answer for a MarkerName that is
// there and is not a manifest: the directory is then nobody's export.
type invalidManifestError struct{ reason string }

func (e *invalidManifestError) Error() string { return "invalid export manifest: " + e.reason }

// readManifest returns the paths dir's manifest lists. It fails with
// errNoManifest when there is none, with an *invalidManifestError when the
// marker is not a regular file or does not parse, and with anything else
// only on I/O.
func readManifest(dir string) ([]string, error) {
	name := filepath.Join(dir, MarkerName)
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoManifest
	}
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, &invalidManifestError{reason: "it is not a regular file"}
	}
	b, err := os.ReadFile(name) // #nosec G304 -- the marker in the output directory the caller named.
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", name, err)
	}
	return parseManifest(b)
}

// parseManifest is the one reading of a manifest. Every entry must be a clean,
// local, slash-separated path other than the marker, and the entries strictly
// ascending, so no listing can name a file outside the directory or name one
// twice.
func parseManifest(b []byte) ([]string, error) {
	s := string(b)
	if !strings.HasSuffix(s, "\n") {
		return nil, &invalidManifestError{reason: "it does not end in a newline"}
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if lines[0] != ManifestHeader {
		return nil, &invalidManifestError{reason: fmt.Sprintf("its first line is %q, not %q", lines[0], ManifestHeader)}
	}
	paths := lines[1:]
	for i, rel := range paths {
		if !manifestEntry(rel) {
			return nil, &invalidManifestError{reason: fmt.Sprintf("line %d, %q, is not a clean relative path", i+2, rel)}
		}
		if i > 0 && rel <= paths[i-1] {
			return nil, &invalidManifestError{reason: fmt.Sprintf("line %d, %q, is out of order or repeated", i+2, rel)}
		}
	}
	return paths, nil
}

// manifestEntry reports whether rel may be listed: a path naming a file
// strictly inside the directory, spelled one way only.
func manifestEntry(rel string) bool {
	return rel != "" && rel != "." && rel != MarkerName &&
		!strings.ContainsAny(rel, "\\\x00\r") &&
		path.Clean(rel) == rel && filepath.IsLocal(filepath.FromSlash(rel))
}

// encodeManifest is the manifest listing paths, which must be sorted.
func encodeManifest(paths []string) []byte {
	var b strings.Builder
	b.WriteString(ManifestHeader)
	b.WriteByte('\n')
	for _, rel := range paths {
		b.WriteString(rel)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// writeManifest replaces dir's manifest with one listing paths, atomically,
// so a run stopped at any point leaves the old manifest or the new one. It
// refuses a listing parseManifest would refuse, so no export can write a
// manifest that dead-ends the next one.
func writeManifest(dir string, paths []string) error {
	b := encodeManifest(paths)
	if _, err := parseManifest(b); err != nil {
		return fmt.Errorf("write %s: %w", MarkerName, err)
	}
	// #nosec G306 -- it sits in a published web root beside files a server
	// running as another user must read.
	return cmdutil.WriteFile(filepath.Join(dir, MarkerName), 0o644, func(w io.Writer) error {
		_, err := w.Write(b)
		return err
	})
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
