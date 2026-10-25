package export

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/jcrussell/livermore-budget/internal/hint"
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

// Removal is what a clean removed, relative to the output directory: Files
// (listed by the earlier manifest, not planned) and Temps (stranded manifest
// temp files), both sorted, and Dirs, the emptied or in-the-way empty
// directories in removal order.
type Removal struct{ Files, Temps, Dirs []string }

// Write puts a prepared Plan on disk, creating o.Dir if needed. It returns
// the slash-separated paths it wrote, the manifest included, sorted and
// relative to o.Dir, and with clean what it removed. It checks ctx before
// creating o.Dir, before each file, before removing what is in the way, and
// before the last manifest, and stops with an error wrapping ctx.Err(),
// leaving what is on disk in place.
//
// The manifest goes down first and last. The first lists the previous
// manifest's paths plus this plan's, so a run stopped anywhere leaves a
// manifest covering everything an export may have put there. The last lists
// this plan's paths plus, without clean, the earlier export's surviving
// leftovers; with clean those leftovers, stranded temp files and the
// directories prunedBy names are removed. What stands in the way of a planned
// path goes before any file is written; the rest goes after the last file,
// so the earlier site keeps what it links to. Nothing unlisted is removed but
// a stranded temp file or an empty directory; a missing or unparseable
// manifest lists nothing.
//
// Before writing anything it refuses, with an error matching ErrForeignDir,
// a plan whose write would overwrite, write through or be blocked by
// anything the previous valid manifest does not list and clean would not
// remove, since recording it in the first manifest would license a later
// --clean to delete it; and, without clean, a plan whose write would meet
// something only clean removes. What is left can fail only on I/O or a
// cancelled ctx.
func (p *plan) Write(ctx context.Context, clean bool) ([]string, Removal, error) {
	if err := ctx.Err(); err != nil {
		return nil, Removal{}, fmt.Errorf("create output directory: %w", err)
	}
	// #nosec G301 -- the output is a web root; a directory a server running as
	// another user cannot traverse is a broken deploy, not a hardened one.
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return nil, Removal{}, fmt.Errorf("create output directory: %w", err)
	}
	prev, planned, c, err := p.survey()
	if err != nil {
		return nil, Removal{}, err
	}
	if err := screen(p.dir, prev, planned, c, clean); err != nil {
		return nil, Removal{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, Removal{}, fmt.Errorf("write %q: %w", MarkerName, err)
	}
	if err := writeManifest(p.dir, union(prev, planned)); err != nil {
		return nil, Removal{}, err
	}
	left := c.left
	var removed Removal
	now, after := c.inTheWayOf(planned)
	if clean {
		if err := ctx.Err(); err != nil {
			return nil, Removal{}, fmt.Errorf("remove obsolete files: %w", err)
		}
		dirs, err := removeListed(p.dir, now.files(), now.dirs)
		if err != nil {
			return nil, Removal{}, hint.With(err, "pass a different --output, or remove the files yourself")
		}
		removed.Dirs = dirs
	}
	// An earlier export's listed pages go before anything is written, so a
	// write that stops partway leaves no page over a mix of old and new data.
	for _, f := range p.files {
		if path.Ext(f.Path) != ".html" || !slices.Contains(prev, f.Path) {
			continue
		}
		if err := os.Remove(filepath.Join(p.dir, filepath.FromSlash(f.Path))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, Removal{}, fmt.Errorf("remove the earlier export's %q: %w", f.Path, err)
		}
	}
	written := make([]string, 0, len(p.files)+1)
	written = append(written, MarkerName)
	for _, f := range p.files {
		if err := ctx.Err(); err != nil {
			return nil, Removal{}, fmt.Errorf("write %q: %w", f.Path, err)
		}
		dst := filepath.Join(p.dir, filepath.FromSlash(f.Path))
		// #nosec G301 -- as above: these are published directories.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, Removal{}, fmt.Errorf("create %q: %w", path.Dir(f.Path), err)
		}
		// #nosec G306 -- these are published web assets; 0600 would make the
		// output unreadable to a web server running as another user.
		if err := os.WriteFile(dst, f.Bytes, 0o644); err != nil {
			return nil, Removal{}, fmt.Errorf("write %q: %w", f.Path, err)
		}
		written = append(written, f.Path)
	}
	if err := ctx.Err(); err != nil {
		return nil, Removal{}, fmt.Errorf("write %q: %w", MarkerName, err)
	}
	if clean {
		dirs, err := removeListed(p.dir, after.files(), after.dirs)
		if err != nil {
			return nil, Removal{}, hint.With(err, "pass a different --output, or remove the files yourself")
		}
		removed.Files, removed.Temps = c.left, c.temps
		removed.Dirs = append(removed.Dirs, dirs...)
		left = nil
	}
	if err := writeManifest(p.dir, union(planned, left)); err != nil {
		return nil, Removal{}, err
	}
	sort.Strings(written)
	return written, removed, nil
}

// Obsolete is what Write with clean would remove: the files the previous
// valid manifest lists that this plan does not write and the stranded
// manifest temp files, sorted, and the directories prunedBy names, deepest
// first. It refuses what Write with clean would refuse, and it reads the
// directory and changes nothing.
func (p *plan) Obsolete() (files, dirs []string, err error) {
	prev, planned, c, err := p.survey()
	if err != nil {
		return nil, nil, err
	}
	if err = screen(p.dir, prev, planned, c, true); err != nil {
		return nil, nil, err
	}
	return c.files(), c.dirs, nil
}

// cleanup is what Write with clean would remove from the output directory,
// decided once so the screen, the dry run and the removal read one answer:
// left, what the earlier export wrote and this plan does not, sorted; temps,
// the stranded manifest temp files, sorted; and dirs, the directories prunedBy
// says removing both leaves empty, deepest first.
type cleanup struct{ left, temps, dirs []string }

// files is every file the cleanup removes, sorted.
func (c cleanup) files() []string { return union(c.left, c.temps) }

// inTheWayOf splits c into what stands in the way of writing planned -- a
// file at a directory above a planned path, and anything at or under one --
// and the rest. Write removes the first before it writes and the second
// after, so a served site keeps the files it links to while the write runs.
// A directory c prunes is never above a planned path, so nothing written
// lands in one the second part removes.
func (c cleanup) inTheWayOf(planned []string) (now, after cleanup) {
	above := make(map[string]bool)
	for _, rel := range planned {
		for d := path.Dir(rel); d != "."; d = path.Dir(d) {
			above[d] = true
		}
	}
	inTheWay := func(rel string) bool {
		if above[rel] {
			return true
		}
		for d := rel; d != "."; d = path.Dir(d) {
			if _, ok := slices.BinarySearch(planned, d); ok {
				return true
			}
		}
		return false
	}
	split := func(rels []string) (in, out []string) {
		for _, rel := range rels {
			if inTheWay(rel) {
				in = append(in, rel)
			} else {
				out = append(out, rel)
			}
		}
		return in, out
	}
	now.left, after.left = split(c.left)
	now.temps, after.temps = split(c.temps)
	now.dirs, after.dirs = split(c.dirs)
	return now, after
}

// survey reads the output directory once: the previous manifest's listing,
// this plan's paths, both sorted, and what a clean would remove.
func (p *plan) survey() (prev, planned []string, c cleanup, err error) {
	if prev, err = previousManifest(p.dir); err != nil {
		return nil, nil, cleanup{}, err
	}
	planned = p.paths()
	c.left = leftover(p.dir, prev, planned)
	if c.temps, err = strandedTemps(p.dir, prev, planned); err != nil {
		return nil, nil, cleanup{}, err
	}
	if c.dirs, err = prunedBy(p.dir, c.files(), planned); err != nil {
		return nil, nil, cleanup{}, err
	}
	return prev, planned, c, nil
}

// strandedTemps is the temp files an interrupted writeManifest left at the
// top of dir, sorted: regular files RefuseForeignDir disregards as stranded,
// that neither the previous manifest lists nor this plan writes.
func strandedTemps(dir string, prev, planned []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read output dir: %w", err)
	}
	var out []string
	for _, e := range entries {
		if !strandedTemp(e) {
			continue
		}
		_, listed := slices.BinarySearch(prev, e.Name())
		_, plannedHere := slices.BinarySearch(planned, e.Name())
		if !listed && !plannedHere {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// screen refuses a plan whose write would meet, on its way, anything the
// cleanup c does not remove, and one whose write would meet anything it does
// remove -- a leftover where this plan needs a directory, or a directory c
// prunes where it needs a file -- unless clean is set and so removes it first.
func screen(dir string, prev, planned []string, c cleanup, clean bool) error {
	foreign, clears, err := obstruction(dir, prev, planned, c)
	if err != nil {
		return err
	}
	if foreign != nil {
		return hint.With(refuse(
			"output directory %q holds %q, which no earlier export's manifest lists and --clean would not remove, in the way of %q, which this export writes; refusing to write over it",
			dir, foreign.at, foreign.rel),
			"move it out of the output directory, or pass a different --output")
	}
	if clears != nil && !clean {
		return hint.With(refuse(
			"output directory %q holds %q, which only --clean removes, in the way of %q, which this export writes; refusing to write over it",
			dir, clears.at, clears.rel),
			"pass --clean to remove what the earlier export wrote and this one does not, and the empty directories in the way")
	}
	return nil
}

// paths is the plan's output paths, sorted.
func (p *plan) paths() []string {
	planned := make([]string, len(p.files))
	for i, f := range p.files {
		planned[i] = f.Path
	}
	slices.Sort(planned)
	return planned
}

// blocked is something on disk in the way of writing the planned path rel,
// standing at at: rel itself or a directory above it.
type blocked struct{ at, rel string }

// obstruction walks each planned path down from dir and reports the first
// thing in the way of writing it that the cleanup c removes, and the first
// that nothing this export may do clears. In the way is anything above the
// path but a real directory, and at the path anything but a regular file the
// previous manifest prev lists. c clears a leftover above the path and a
// directory at the path it prunes. Both listings are sorted.
func obstruction(dir string, prev, planned []string, c cleanup) (foreign, clears *blocked, err error) {
	isLeft := func(rel string) bool { _, ok := slices.BinarySearch(c.left, rel); return ok }
	isPrev := func(rel string) bool { _, ok := slices.BinarySearch(prev, rel); return ok }
	note := func(b blocked, clear bool) {
		switch {
		case clear && clears == nil:
			clears = &b
		case !clear && foreign == nil:
			foreign = &b
		}
	}
	for _, rel := range planned {
		parts := strings.Split(rel, "/")
		for i := range parts {
			at := strings.Join(parts[:i+1], "/")
			info, lerr := os.Lstat(filepath.Join(dir, filepath.FromSlash(at)))
			if errors.Is(lerr, os.ErrNotExist) {
				break
			}
			if lerr != nil {
				return nil, nil, fmt.Errorf("stat %q: %w", at, lerr)
			}
			if i < len(parts)-1 {
				if !info.IsDir() {
					note(blocked{at: at, rel: rel}, isLeft(at))
					break
				}
				continue
			}
			switch {
			case info.IsDir():
				note(blocked{at: at, rel: rel}, slices.Contains(c.dirs, at))
			case !info.Mode().IsRegular() || !isPrev(rel):
				note(blocked{at: at, rel: rel}, false)
			}
		}
	}
	return foreign, clears, nil
}

// removeListed removes each of rels from dir, then each of dirs, which must
// come deepest first, and returns the directories it removed. rels are files
// reached through real directories, and dirs are what prunedBy said removing
// them leaves empty; dir itself is never among them. prunedBy decided before
// anything was removed, so a directory something has landed in since is not
// empty, and is skipped rather than failing the clean.
func removeListed(dir string, rels, dirs []string) ([]string, error) {
	for _, rel := range rels {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove %q: %w", rel, err)
		}
	}
	var removed []string
	for _, d := range dirs {
		// Rmdir and not os.Remove: a file another process put where the
		// directory was is listed by no manifest, and os.Remove would unlink
		// it. ENOTDIR is that file, and it is left alone.
		err := syscall.Rmdir(filepath.Join(dir, filepath.FromSlash(d)))
		switch {
		case err == nil:
			removed = append(removed, d)
		case errors.Is(err, os.ErrNotExist), notEmpty(err), errors.Is(err, syscall.ENOTDIR):
		default:
			return nil, fmt.Errorf("remove emptied directory %q: %w", d, err)
		}
	}
	return removed, nil
}

// notEmpty reports whether err is a directory removal refused because the
// directory holds something. POSIX lets rmdir say so with either errno.
func notEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
}

// prunedBy is the one rule for which directories a clean removes: those that
// removing rels leaves holding nothing, deepest first. It weighs every
// directory above one of rels and every directory at or under a planned file
// path, so an empty directory in the way is pruned too, and never one above a
// planned path. It only reads, so the screen, the dry run and the removal
// share it.
func prunedBy(dir string, rels, planned []string) ([]string, error) {
	gone := make(map[string]bool, len(rels))
	weigh := make(map[string]bool)
	keep := make(map[string]bool)
	for _, rel := range rels {
		gone[rel] = true
		for d := path.Dir(rel); d != "."; d = path.Dir(d) {
			weigh[d] = true
		}
	}
	for _, rel := range planned {
		for d := path.Dir(rel); d != "."; d = path.Dir(d) {
			keep[d] = true
		}
		info, ok := within(dir, rel)
		if !ok || !info.IsDir() {
			continue
		}
		err := filepath.WalkDir(filepath.Join(dir, filepath.FromSlash(rel)), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				sub, rerr := filepath.Rel(dir, p)
				if rerr != nil {
					return rerr
				}
				weigh[filepath.ToSlash(sub)] = true
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", rel, err)
		}
	}
	// Longer first puts every directory before its parent.
	dirs := slices.SortedFunc(maps.Keys(weigh), func(a, b string) int {
		if n := cmp.Compare(len(b), len(a)); n != 0 {
			return n
		}
		return strings.Compare(a, b)
	})
	var out []string
	for _, d := range dirs {
		if keep[d] {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(d)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", d, err)
		}
		if !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return !gone[d+"/"+e.Name()] }) {
			gone[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}

// previousManifest is the listing dir's manifest holds, or nothing where it
// holds none a reader can trust; only I/O fails it.
func previousManifest(dir string) ([]string, error) {
	paths, err := readManifest(dir)
	var invalid *invalidManifestError
	if errors.Is(err, errNoManifest) || errors.As(err, &invalid) {
		return nil, nil
	}
	return paths, err
}

// leftover is what prev lists and planned does not that is still a file in
// dir, reached through directories rather than symlinks: what an earlier
// export wrote and this one leaves behind. Both listings are sorted.
func leftover(dir string, prev, planned []string) []string {
	var out []string
	for _, rel := range prev {
		if _, ok := slices.BinarySearch(planned, rel); ok {
			continue
		}
		if fileWithin(dir, rel) {
			out = append(out, rel)
		}
	}
	return out
}

// fileWithin reports whether rel names something in dir other than a
// directory, reached through real directories.
func fileWithin(dir, rel string) bool {
	info, ok := within(dir, rel)
	return ok && !info.IsDir()
}

// within reports what rel names in dir, and whether it is there with every
// directory between them a real one: a symlinked directory on the way would
// put it outside dir.
func within(dir, rel string) (fs.FileInfo, bool) {
	cur := dir
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return nil, false
		}
		if i == len(parts)-1 {
			return info, true
		}
		// Lstat reports a symlink as no directory, so a symlinked directory
		// on the way fails here.
		if !info.IsDir() {
			return nil, false
		}
	}
	return nil, false
}

// union is the sorted, duplicate-free merge of two sorted listings.
func union(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	slices.Sort(out)
	return slices.Compact(out)
}
