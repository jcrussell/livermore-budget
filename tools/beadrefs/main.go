// Command beadrefs refuses a bead id that names no bead.
//
// An invented id does not read as a mistake: the work really is tracked and only
// the pointer is dead, so nobody goes looking. It has happened twice here.
// Writing one is easy, because `bd create` prints an id that is not guessable
// and a sentence citing it is often drafted before the bead exists.
//
// It resolves against .beads/issues.jsonl, the committed export, and NOT against
// bd. That is what makes it a gate rather than an advisory check: the export is
// in every checkout, so CI can run this with no bd, no Dolt server and no
// network -- unlike the memory arm of `make narration`, which reads a database
// no checkout carries.
//
// THE EXPORT LAGS THE DATABASE, which is the one way this can be wrong about a
// real bead. It is a passive export, so an id filed minutes ago may not be in it
// yet. Where bd IS on PATH the misses are re-asked of it before anything is
// reported, and where bd is absent -- CI -- a miss is a failure, because the
// export is the only artifact CI has and the remedy is to commit it.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// idPattern matches the project's own ids. byobPattern matches byob's, which
// are in this graph too.
//
// BOTH ARE CHECKED, and the reason only one used to be does not survive being
// written down: a byob id names reference material and AGENTS.md says never to
// claim or close one, so -- the argument went -- nothing here should assert
// they exist. That is a rule about what an agent may DO to a bead, not about
// whether a citation of one should resolve. The 105 byob beads are rows of
// .beads/issues.jsonl like any other, so this resolves them the same way, and a
// typo'd byob id now fails here instead of sending a reader to `bd show` and an
// empty answer. AGENTS.md's Go section cites eleven of them.
//
// NO EXAMPLE ID IS SPELLED IN THIS FILE, which is the same trade tools/doccheck
// makes about citations: an example is exactly the shape the pattern matches,
// so writing one would make the command report itself.
//
// NOT A LIVE DEFECT WHEN THIS LANDED, measured: all 16 distinct byob ids cited
// in tracked Go, markdown, .mjs and the Makefile resolve. What was missing is
// the guard.
//
// THE PATTERN ALONE IS NOT ENOUGH, because `fisc-` is an overloaded prefix in
// this tree and not a namespace: fact ids are fisc-f-<hash>, series ids are
// fisc-s-<hash>, the export marker file is .fisc-export and a scratch directory
// is .fisc-write-probe-*. Every one of those begins with something a bead id
// could be. What tells them apart is the SECOND hyphen and the leading dot, so
// the boundaries are applied in citedIn rather than here; RE2 has no lookaround.
var idPattern = regexp.MustCompile(`fisc-[a-z0-9]+(?:\.[0-9]+)*`)

// byobPattern takes the whole hyphenated slug, because a byob id's hyphens are
// PART OF IT: a two-word slug with a dotted child is one id and not three. It
// cannot end on a hyphen, which is what the grouping is for -- an id written
// before an em dash must match up to the slug and stop.
//
// SO THE TRAILING-HYPHEN RULE IN citedIn DOES NOT APPLY TO IT. That rule exists
// to keep fisc-f-<hash> from reading as a bead; here a following hyphen is
// already known not to start another segment, so refusing on it would drop
// every id written before an em dash.
var byobPattern = regexp.MustCompile(`byob-[a-z0-9]+(?:-[a-z0-9]+)*(?:\.[0-9]+)*`)

// exempt names files whose fisc- literals are fixtures rather than claims about
// the tracker, with the reason each is here. It is a declaration and not a
// pattern, so adding one is a visible decision.
//
// A declaration that has gone stale fails, the way internal/check/vacuity.go's
// does: the file must still exist. Staleness is measured against the FILESYSTEM
// rather than against the walk, because a run over a narrower path set is a
// narrower run and not a stale declaration.
var exempt = map[string]string{
	"tools/beadrefs/main_test.go":  "its fixtures are ids that must NOT resolve, so a test for this command cannot be written out of real ones",
	"tools/beadcheck/main_test.go": "its fixtures are synthetic bead records, and building them out of real ids would tie a prose check to whichever beads happen to exist",
}

// exemptIDs names tokens that are shaped like a bead id and are not one, with
// what each actually is. All of them are in the client, and none can be told
// from a bead id by any rule: a localStorage key and a radio-group name both
// read as `fisc-` plus a short token, and so does a bead id.
//
// Renaming them would be the better fix and is not free: AGENTS.md requires a
// change to site/app.js to ship its jscheck guard in the same commit, and these
// are strings a returning reader's browser already holds.
//
// Each names ONE file it lives in, and that is what its staleness is measured
// against: the file must exist, and if a run scanned that file the token must
// have been seen there. Both of these tokens occur in several files, so the
// anchor is a choice; if one moves out of its anchor the run says so and names
// both remedies, since re-pointing and deleting are different answers and
// deleting the wrong one turns the surviving occurrences into dead beads. Anchoring it to a file rather than to the walk is the
// same principle checkExemptions follows -- a run over a narrower path set is a
// narrower run and not a stale declaration.
//
// The sighting must also come from OUTSIDE this file, because the declaration
// below is itself scanned; without that, every exemption satisfies its own
// staleness test. That was measured rather than reasoned: renaming a key here to
// a token in no other file left a full run green.
var exemptIDs = map[string]exemptID{
	"fisc-theme":   {file: "site/app.js", what: "the localStorage key holding the reader's light/dark choice; also inlined in every site/*.html.tmpl"},
	"fisc-year":    {file: "site/index.html.tmpl", what: "the radio-group name for the fiscal-year control; also in chart.html.tmpl"},
	"fisc-columns": {file: "site/app.js", what: "the localStorage key holding the reader's chosen column count; also in tools/jscheck/lifecycle.mjs"},
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
		fmt.Fprintln(os.Stderr, "beadrefs: the ids above are in the export and outside what citedIn")
		fmt.Fprintln(os.Stderr, "  recognises, so a citation to one would be dropped without a report")
		fmt.Fprintln(os.Stderr, "  -- the dead-pointer failure this gate exists to prevent, inside the")
		fmt.Fprintln(os.Stderr, "  gate. The ids are not wrong; bd mints them and this repo does not.")
		fmt.Fprintln(os.Stderr, "  Teach idPattern and citedIn the new shape without unteaching the")
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

// checkExemptions refuses a declaration that has outlived the file it exempts,
// which is otherwise a hole nobody can see. It takes the root explicitly because
// the declared paths are repo-relative and this must be answerable from anywhere
// -- refsUnder deliberately does not do it, since a run over a narrower path set
// is a narrower run rather than a stale declaration.
func checkExemptions(root string) error {
	for file, reason := range exempt {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil {
			return fmt.Errorf("the exemption for %s (%q) names a file that is not there: %w", file, reason, err)
		}
	}
	// declarationFile is a hardcoded path and so is a claim about the tree like
	// any other: nothing else ever resolves it, so a rename would leave the
	// self-anchor guard below silently disarmed.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(declarationFile))); err != nil {
		return fmt.Errorf("declarationFile names %s, which is not there: %w", declarationFile, err)
	}
	for id, e := range exemptIDs {
		// A declaration cannot anchor itself to the file it is written in: the
		// declaration below would then be the sighting that keeps it alive, and
		// the staleness check could never fire. Refused here, where the message
		// can say so, rather than left to produce a permanent unexplained error
		// in dropExemptIDs.
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
// that no longer has a subject. It belongs here rather than in refsUnder because
// only a run over the whole path list can say an id appears nowhere; a narrower
// run is a narrower run.
func dropExemptIDs(refs []ref, scanned map[string]bool) ([]ref, error) {
	seen := map[string]bool{}
	kept := refs[:0:0]
	for _, r := range refs {
		if e, ok := exemptIDs[r.id]; ok {
			// The sighting must be in the file the declaration names. Accepting
			// one anywhere else made the check unfalsifiable in the ordinary
			// way: the token could vanish from its own file entirely and any
			// other occurrence -- including in this file's own declaration --
			// would keep the exemption alive.
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

// declarationFile is where exemptIDs is written, and a sighting there does not
// count. It is a path rather than something derived because nothing in Go tells
// a file its own name at runtime that a test could also assert against.
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

// unrecognisable returns the export ids citedIn could never return, sorted so a
// failure reads the same way twice. bd chooses what a bead id looks like and
// this repo does not, and citedIn refuses a match that runs into a hyphen
// because the second hyphen is what tells a fact id from a bead id. An export
// id on the wrong side of that boundary is not invalid -- it is INVISIBLE, and
// every citation to it would be dropped with no report, which is this command's
// own failure mode happening inside it. Failing at startup takes the run red in
// the run where the export first carries such an id, before anyone writes a
// citation nothing can check; the hyphen refusal is safe to keep because this
// is watching it.
//
// The probe is citedIn itself, on the id alone on a line, rather than a second
// pattern: a parallel regex would be a copy of the boundaries that nothing
// keeps in step. Recognised means recognised WHOLE -- an id citedIn returns a
// prefix of is as unseeable as one it drops.
func unrecognisable(known map[string]bool) []string {
	var bad []string
	for id := range known {
		if got := citedIn(id); len(got) != 1 || got[0] != id {
			bad = append(bad, id)
		}
	}
	sort.Strings(bad)
	return bad
}

// refsUnder collects every citation in the named files and directories. A
// directory is walked; anything unreadable is an error rather than a skip,
// because a path that cannot be read is a path that cannot be checked.
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
			// A file named on the command line is scanned whatever it is called,
			// which is how Makefile gets read; the extension filter is only for
			// deciding what to pick up while walking a DIRECTORY.
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
// about the tracker.
//
// .json IS IN THE LIST, which is not obvious: it is mostly extracted geometry,
// where a fisc- token would be a value. But testdata/sankey.golden.json carries
// a prose note citing a bead, so "a golden holds values and not claims" was
// simply false, and the citedIn boundaries already reject the one value shape
// that occurs there (fact ids, fisc-f-<hash>).
//
// .txt IS NOT, and that is the same judgement going the other way: the page
// fixtures under testdata/pages/ are the city's own printed text and cannot
// contain a claim about this tracker. requirements.txt is named directly in the
// Makefile instead.
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
		for _, id := range citedIn(sc.Text()) {
			refs = append(refs, ref{id: id, file: path, line: n})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return refs, nil
}

// citedIn returns the bead ids one line CITES, which is not every fisc- token it
// contains. A match is a citation only when nothing runs into it on either side:
// a preceding dot or hyphen or letter makes it part of a longer name (.fisc-export),
// and a following hyphen or letter makes it a prefix of one (fisc-f-0ba7b00dfaf7).
// A following dot is left alone -- the pattern has already taken any dotted child,
// so what remains is the period ending a sentence.
func citedIn(line string) []string {
	var ids []string
	for _, loc := range idPattern.FindAllStringIndex(line, -1) {
		if loc[0] > 0 && runsInto(line[loc[0]-1]) {
			continue
		}
		if loc[1] < len(line) && (line[loc[1]] == '-' || isWordByte(line[loc[1]])) {
			continue
		}
		ids = append(ids, line[loc[0]:loc[1]])
	}
	// A byob id has already taken every hyphen-joined segment, so what follows
	// a match cannot begin another one and the trailing-hyphen rule above does
	// not apply -- see byobPattern.
	for _, loc := range byobPattern.FindAllStringIndex(line, -1) {
		if loc[0] > 0 && runsInto(line[loc[0]-1]) {
			continue
		}
		if loc[1] < len(line) && isWordByte(line[loc[1]]) {
			continue
		}
		ids = append(ids, line[loc[0]:loc[1]])
	}
	return ids
}

func runsInto(b byte) bool { return b == '.' || b == '-' || isWordByte(b) }

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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

// bdKnows asks bd about an id the export lacks, which covers the window between
// filing a bead and committing the export. With bd absent it answers no, so CI
// -- where bd never exists -- decides on the export alone.
//
// The deadline is not decoration. bd talks to a Dolt server that can be locked
// by another process, and this runs inside pre-commit; without one, an id the
// export happens to lack would hang the commit rather than fail it.
func bdKnows(ctx context.Context) func(string) bool {
	path, err := exec.LookPath("bd")
	if err != nil {
		return func(string) bool { return false }
	}
	return func(id string) bool {
		ask, cancel := context.WithTimeout(ctx, bdTimeout)
		defer cancel()
		out, err := exec.CommandContext(ask, path, "show", id, "--json").Output() // #nosec G204 -- id matched idPattern.
		if err != nil {
			return false
		}
		return answersFor(out, id)
	}
}

// answersFor says whether bd's reply is about the id that was asked for.
//
// bd RESOLVES A UNIQUE PREFIX: asked about the first few characters of an id it
// exits 0 and returns the whole one, and exec gives the caller no way to tell
// that from a hit. A truncated id is a dead pointer whatever bd does with it,
// and accepting one would pass pre-commit and fail CI, where bd is absent and
// only the export answers.
//
// It is a function of bytes rather than a step inside bdKnows because the
// version inside could not be tested: bdKnows closes over exec.LookPath, so
// reducing this comparison to "did bd return anything" left every test green.
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
