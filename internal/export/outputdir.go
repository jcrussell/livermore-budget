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
// fisc's only while it carries a valid one.
const MarkerName = ".fisc-export"

// ManifestHeader is the manifest's first line; a marker with any other is not
// a manifest.
const ManifestHeader = "fisc export manifest 1"

// probePrefix names the directory WritableDir creates and removes. A process
// that dies between the two strands it empty, so RefuseForeignDir disregards
// an empty directory of this name rather than refusing every later export.
const probePrefix = ".fisc-write-probe-"

// ResolveOutputDir abs-resolves a user-supplied output directory, follows
// symlinks on its existing prefix, and refuses the filesystem root and the
// user's home directory (byob-input-validation.1; an output dir may lie
// outside the working directory, so containment is the wrong check).
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
	// $HOME is symlink-resolved too, or the guard is disarmed wherever home
	// traverses a symlink (NFS, macOS).
	if home, herr := os.UserHomeDir(); herr == nil && home != "" {
		if rhome, rerr := evalExistingSymlinks(home); rerr == nil && resolved == rhome {
			return "", fmt.Errorf("refusing to use home directory %q as output directory", resolved)
		}
	}
	return resolved, nil
}

// ErrForeignDir matches every refusal RefuseForeignDir makes and none of its
// I/O errors; the two exit differently.
var ErrForeignDir = errors.New("output directory is not one fisc may write into")

type foreignDirError struct{ msg string }

func (e *foreignDirError) Error() string      { return e.msg }
func (*foreignDirError) Is(target error) bool { return target == ErrForeignDir }

func refuse(format string, a ...any) error {
	return &foreignDirError{msg: fmt.Sprintf(format, a...)}
}

// RefuseForeignDir refuses an output directory fisc does not own: one that
// exists, holds anything but a stranded write probe or manifest temp file,
// and carries no valid MarkerName manifest. An unreadable directory or
// manifest is an I/O error, never assumed empty. Other files beside a valid
// manifest are safe: --clean and Write touch only what it lists.
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
		if !strandedProbe(dir, e) && !strandedTemp(e) {
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
				"If an earlier export wrote it, remove the directory and export again, or name a new one. "+
				"To publish into a checkout that holds other files, such as a GitHub Pages branch, export into a new "+
				"directory and copy its files across",
			dir, MarkerName)
	case errors.As(err, &invalid):
		return refuse(
			"output directory %q carries a %s that is not a manifest this fisc reads (%s); refusing to write a site into it. "+
				"If an earlier export wrote it, remove the directory and export again, or name a new one",
			dir, MarkerName, invalid.reason)
	case err != nil:
		return err
	}
	return nil
}

// errNoManifest is readManifest's answer for a directory with no MarkerName.
var errNoManifest = errors.New("no export manifest")

// invalidManifestError is readManifest's answer for a MarkerName that is
// there and is not a manifest: the directory is then nobody's export.
type invalidManifestError struct{ reason string }

func (e *invalidManifestError) Error() string { return "invalid export manifest: " + e.reason }

// readManifest returns the paths dir's manifest lists, or errNoManifest, an
// *invalidManifestError, or an I/O error.
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

// parseManifest is the one reading of a manifest: entries must pass
// manifestEntry and be strictly ascending.
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
// strictly inside the directory, spelled one way only, on one line. Prepare
// screens every planned path with it too.
func manifestEntry(rel string) bool {
	return rel != "" && rel != "." && rel != MarkerName &&
		!strings.ContainsAny(rel, "\\\x00\r\n") &&
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
// and refuses a listing parseManifest would refuse.
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

// strandedProbe reports whether e is a WritableDir probe left behind empty;
// one holding anything, or unreadable, is not.
func strandedProbe(dir string, e os.DirEntry) bool {
	if !e.IsDir() || !strings.HasPrefix(e.Name(), probePrefix) {
		return false
	}
	inside, err := os.ReadDir(filepath.Join(dir, e.Name()))
	return err == nil && len(inside) == 0
}

// strandedTemp reports whether e is a regular file named exactly as
// cmdutil.WriteFile names its temp files.
func strandedTemp(e os.DirEntry) bool {
	return e.Type().IsRegular() && cmdutil.IsTempName(e.Name())
}

// WritableDir reports whether path — or, if it does not exist yet, its
// nearest existing ancestor — is a directory this process can write to, by
// creating and removing a probe directory, since permission bits do not
// answer on a read-only mount, NFS or a user mapping.
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
// and rejoins the components that do not exist yet, which
// filepath.EvalSymlinks refuses.
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
