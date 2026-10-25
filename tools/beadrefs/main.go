// Command beadrefs refuses a bead id that names no bead.
//
// It resolves against .beads/issues.jsonl, the committed export, so CI can run
// it with no bd, no Dolt and no network. The export lags the database: where bd
// is on PATH a miss is re-asked of it; where bd is absent (CI) a miss fails.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jcrussell/livermore-budget/internal/beadid"
)

// exempt names files whose fisc- literals are fixtures rather than claims about
// the tracker. A declaration whose file is gone from the filesystem fails.
var exempt = map[string]string{
	"tools/beadrefs/main_test.go":    "its fixtures are ids that must NOT resolve, so a test for this command cannot be written out of real ones",
	"internal/beadid/beadid_test.go": "its fixtures are id shapes, most of which name no bead, so a test of the matcher cannot be written out of real ones",
	"tools/errata/beads_test.go":     "its fixtures are synthetic bead records, and building them out of real ids would tie a prose check to whichever beads happen to exist",
}

// exemptIDs names tokens shaped like a bead id that are not one. Each names
// ONE anchor file: it must exist, and if a run scanned it the token must have
// been seen there, not in this declaration.
var exemptIDs = map[string]exemptID{
	"fisc-theme": {file: "site/theme.js", what: "the localStorage key holding the reader's light/dark choice, and the event a switch dispatches; also read by the head script in site/chrome.html.tmpl"},
}

type exemptID struct {
	file string
	what string
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: beadrefs issues.jsonl path...")
		os.Exit(2)
	}
	if err := checkExemptions("."); err != nil {
		fmt.Fprintf(os.Stderr, "beadrefs: %v\n", err)
		os.Exit(2)
	}
	known, err := knownIDs(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "beadrefs: %v\n", err)
		os.Exit(2)
	}
	if bad := unrecognisable(known); len(bad) > 0 {
		for _, id := range bad {
			fmt.Fprintf(os.Stderr, "beadrefs: %s is a real bead the citation scanner cannot see\n", id)
		}
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "beadrefs: the ids above are in the export and outside what beadid.Cited")
		fmt.Fprintln(os.Stderr, "  recognises, so a citation to one would be dropped without a report")
		fmt.Fprintln(os.Stderr, "  -- the dead-pointer failure this gate exists to prevent, inside the")
		fmt.Fprintln(os.Stderr, "  gate. The ids are not wrong; bd mints them and this repo does not.")
		fmt.Fprintln(os.Stderr, "  Teach internal/beadid the new shape without unteaching the")
		fmt.Fprintln(os.Stderr, "  boundaries: today the second hyphen is what tells a fact id")
		fmt.Fprintln(os.Stderr, "  (fisc-f-<hash>) from a bead id.")
		os.Exit(2)
	}
	refs, scanned, err := refsUnder(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "beadrefs: %v\n", err)
		os.Exit(2)
	}
	kept, err := dropExemptIDs(refs, scanned)
	if err != nil {
		fmt.Fprintf(os.Stderr, "beadrefs: %v\n", err)
		os.Exit(2)
	}
	dead := resolve(kept, known, bdKnows(context.Background()))
	if len(dead) == 0 {
		return
	}
	for _, r := range dead {
		fmt.Fprintf(os.Stderr, "%s:%d: %s names no bead\n", r.file, r.line, r.id)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "beadrefs: the ids above resolve to nothing.")
	fmt.Fprintln(os.Stderr, "  An id that names no bead reads as though the work is tracked,")
	fmt.Fprintln(os.Stderr, "  so nobody goes looking. File the bead and use the id bd printed,")
	fmt.Fprintln(os.Stderr, "  or drop the citation. If the bead is real and newly filed, its")
	fmt.Fprintln(os.Stderr, "  export has not landed: commit .beads/issues.jsonl.")
	fmt.Fprintln(os.Stderr, "  See AGENTS.md, \"Before you quote a number\".")
	os.Exit(1)
}

// checkExemptions refuses a declaration that has outlived the file it exempts.
func checkExemptions(root string) error {
	for file, reason := range exempt {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil {
			return fmt.Errorf("the exemption for %s (%q) names a file that is not there: %w", file, reason, err)
		}
	}
	// A renamed declarationFile would silently disarm the self-anchor guard.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(declarationFile))); err != nil {
		return fmt.Errorf("declarationFile names %s, which is not there: %w", declarationFile, err)
	}
	for id, e := range exemptIDs {
		// Anchored to its own declaration file, the staleness check could
		// never fire.
		if e.file == declarationFile {
			return fmt.Errorf("the exemption for %s (%q) names %s, which is where the declaration itself lives; anchor it to the file that actually carries the token",
				id, e.what, e.file)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(e.file))); err != nil {
			return fmt.Errorf("the exemption for %s (%q) names a file that is not there: %w", id, e.what, err)
		}
	}
	return nil
}

// dropExemptIDs removes the declared non-bead tokens, and refuses a declaration
// whose anchor file was scanned without the token.
func dropExemptIDs(refs []ref, scanned map[string]bool) ([]ref, error) {
	seen := map[string]bool{}
	kept := refs[:0:0]
	for _, r := range refs {
		if e, ok := exemptIDs[r.id]; ok {
			// Only a sighting in the named file counts, or any other
			// occurrence would keep the exemption alive.
			if norm(r.file) == e.file {
				seen[r.id] = true
			}
			continue
		}
		kept = append(kept, r)
	}
	for id, e := range exemptIDs {
		if scanned[e.file] && !seen[id] {
			return nil, fmt.Errorf("the exemption for %s (%q) says it lives in %s, and %s was read without it: re-point it at the file that carries the token now, or delete it if the token is gone",
				id, e.what, e.file, e.file)
		}
	}
	return kept, nil
}

// norm puts a path in the form the declarations are written in, so a comparison
// does not turn on whether the caller wrote ./Makefile or Makefile.
func norm(path string) string { return filepath.ToSlash(filepath.Clean(path)) }

// declarationFile is where exemptIDs is written; a sighting there does not
// count.
const declarationFile = "tools/beadrefs/main.go"

// ref is one citation: which id, and where it was written.
type ref struct {
	id   string
	file string
	line int
}

// knownIDs reads every id out of the committed export. The file is one JSON
// object per line and records without an id are skipped rather than refused,
// because the export carries more than issues.
func knownIDs(path string) (map[string]bool, error) {
	f, err := os.Open(path) // #nosec G304,G703 -- the path is this command's argument.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	known := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if rec.ID != "" {
			known[rec.ID] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("%s carries no ids at all, so every citation would be reported dead", path)
	}
	return known, nil
}

// unrecognisable returns the export ids beadid.Cited could never return,
// sorted. bd mints ids and beadid.Cited refuses a match running into a second
// hyphen (a fact id), so an export id past that boundary would be INVISIBLE and
// every citation of it dropped unreported. The probe is beadid.Cited itself,
// and recognised means recognised WHOLE.
func unrecognisable(known map[string]bool) []string {
	var bad []string
	for id := range known {
		if got := beadid.Cited(id); len(got) != 1 || got[0] != id {
			bad = append(bad, id)
		}
	}
	sort.Strings(bad)
	return bad
}

// refsUnder collects every citation in the named files and directories.
// Anything unreadable is an error rather than a skip.
func refsUnder(paths []string) ([]ref, map[string]bool, error) {
	var refs []ref
	scanned := map[string]bool{}
	for _, p := range paths {
		// #nosec G703 -- the path list is this command's argument, which is the
		// whole of its interface: it checks the paths it is asked to.
		err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// A file named on the command line is scanned whatever its
			// extension (Makefile); the filter applies only to a walk.
			if norm(path) != norm(p) && !scannable(path) {
				return nil
			}
			if _, ok := exempt[filepath.ToSlash(filepath.Clean(path))]; ok {
				return nil
			}
			found, err := refsIn(path)
			if err != nil {
				return err
			}
			scanned[norm(path)] = true
			refs = append(refs, found...)
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return refs, scanned, nil
}

// scannable says whether a file found by walking a directory can carry a claim
// about the tracker. .json is in (a golden carries prose notes; fact ids are
// refused by beadid.Cited); .txt is out (testdata/pages/ is the city's text).
func scannable(path string) bool {
	switch filepath.Ext(path) {
	case ".md", ".go", ".mjs", ".js", ".tmpl", ".css", ".py", ".yaml", ".yml", ".json":
		return true
	}
	return false
}

func refsIn(path string) ([]ref, error) {
	f, err := os.Open(path) // #nosec G304,G703 -- see knownIDs.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var refs []ref
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for n := 1; sc.Scan(); n++ {
		for _, id := range beadid.Cited(sc.Text()) {
			refs = append(refs, ref{id: id, file: path, line: n})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return refs, nil
}

// resolve reports the citations that name nothing, sorted by file and line so a
// run reads like a grep. secondOpinion is consulted once per distinct missing
// id, and only for ids the export lacks, so a green tree never shells out.
func resolve(refs []ref, known map[string]bool, secondOpinion func(string) bool) []ref {
	verdict := map[string]bool{}
	var dead []ref
	for _, r := range refs {
		if known[r.id] {
			continue
		}
		ok, asked := verdict[r.id]
		if !asked {
			ok = secondOpinion(r.id)
			verdict[r.id] = ok
		}
		if !ok {
			dead = append(dead, r)
		}
	}
	sort.Slice(dead, func(i, j int) bool {
		if dead[i].file != dead[j].file {
			return dead[i].file < dead[j].file
		}
		return dead[i].line < dead[j].line
	})
	return dead
}

// bdKnows asks bd about an id the export lacks. With bd absent it answers no,
// so CI decides on the export alone. The deadline keeps a locked Dolt server
// from hanging pre-commit.
func bdKnows(ctx context.Context) func(string) bool {
	path, err := exec.LookPath("bd")
	if err != nil {
		return func(string) bool { return false }
	}
	return func(id string) bool {
		ask, cancel := context.WithTimeout(ctx, bdTimeout)
		defer cancel()
		out, err := exec.CommandContext(ask, path, "show", id, "--json").Output() // #nosec G204 -- id is one beadid.Cited returned.
		if err != nil {
			return false
		}
		return answersFor(out, id)
	}
}

// answersFor says whether bd's reply is about the id that was asked for. bd
// RESOLVES A UNIQUE PREFIX and exits 0, so a truncated id would pass
// pre-commit and fail CI. It is a function of bytes so it can be tested.
func answersFor(out []byte, id string) bool {
	var recs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &recs); err != nil {
		return false
	}
	for _, r := range recs {
		if r.ID == id {
			return true
		}
	}
	return false
}

// bdTimeout bounds one `bd show`.
const bdTimeout = 10 * time.Second
