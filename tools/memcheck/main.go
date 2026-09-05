// Command memcheck refuses errata in the beads memories, which bd prime injects
// into every session automatically.
//
// It reads `bd memories --json` on stdin -- a flat object of key to body, with
// non-string values (schema_version) skipped -- and exits non-zero on any body
// containing a phrase that can only be a claim about the memory's own former
// text. AGENTS.md's "History's home is git" already forbids that shape in a
// source comment, and `make narration` already enforces it there; a memory is
// worse placed for it, because injection means a stale line arrives whether or
// not anyone opens the file it is about.
//
// THE PHRASE LIST IS DELIBERATELY NARROW, for the reason narration's Go arm
// gives. "an earlier version" and "used to say" have honest uses in a sentence
// about a DOCUMENT -- the city reprints schedules, and one memory correctly
// reports that an ACFR row "was quoted as producing '-512,946'". So every
// pattern here requires past-tense self-reference: the subject has to be the
// memory. A present-tense scope limit ("WHAT THIS MEMORY CANNOT TELL YOU IS THE
// CURRENT PUSH STATE") is a true and useful sentence and must keep passing.
//
// The correction pattern wants a full ISO date or an ALL-CAPS line-leading
// CORRECTED, and not a bare year, because "the city corrected 2024's printed
// total" is a sentence about the corpus and not about this text.
// TestScanFindsOnlyPastTenseSelfReference carries that case and the others that
// decide where each pattern's edge is.
//
// Unlike narration's Go arm this one has no file-touch signal to defer a
// pre-existing hit to the session that was editing that file anyway: memories
// are not files, so it fires on every commit against all of them at once. It
// can only be added to a tree whose memories are already clean.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// errata are matched against a memory body. Each requires the memory itself to
// be the subject of a past-tense claim; see the package comment for why nothing
// looser belongs here.
var errata = []*regexp.Regexp{
	regexp.MustCompile(`(?i)th(is|e) memory (used to|previously|once|said|no longer)`),
	regexp.MustCompile(`(?i)earlier versions? of this memory`),
	regexp.MustCompile(`(?i)\bthis used to (say|read|end|claim)`),
	regexp.MustCompile(`(?i)\(correction[,:]`),
	regexp.MustCompile(`(?i)\bcorrected 20\d\d-\d\d-\d\d`),
	regexp.MustCompile(`(?m)^CORRECTED\b`),
}

func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memcheck: reading stdin: %v\n", err)
		os.Exit(2)
	}
	hits, examined, err := scan(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memcheck: %v\n", err)
		os.Exit(2)
	}
	if examined == 0 {
		// A beads database that exists and holds no memories at all -- what a
		// fresh `bd init` looks like before the project's memories are pulled.
		// It is reported rather than refused, because refusing it stops a
		// contributor in that state from committing anything, and the arm this
		// runs under is advisory in the same way when bd is absent entirely.
		fmt.Fprintln(os.Stderr, "warning: the memory set is empty, so nothing was checked")
		return
	}
	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		fmt.Fprintln(os.Stderr, h)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "memcheck: the memories above say what they used to say.")
	fmt.Fprintln(os.Stderr, "  bd prime injects every memory into every session, so an erratum")
	fmt.Fprintln(os.Stderr, "  arrives in context whether or not anyone opens the file it is")
	fmt.Fprintln(os.Stderr, "  about. Keep the corrected statement and delete the sentence")
	fmt.Fprintln(os.Stderr, "  about what it replaced. Edit with 'bd remember --key <key>'.")
	os.Exit(1)
}

// scan reports one line per erratum found, sorted by key so the output is
// stable across runs of a map.
//
// IT REFUSES ANY SHAPE IT CANNOT READ AS BODIES rather than reporting it clean.
// A top-level null unmarshals into a nil map without error, and a NESTED value
// means the bodies have moved down a level
// ({"schema_version":1,"memories":{...}}) -- and note that counting the bodies
// does not catch the second on its own, because one stray top-level string
// beside the envelope makes the count non-zero while the real bodies go unread.
// Both are the shape AGENTS.md calls green because the gate fired: an exit code
// that says nothing was wrong when it means nothing was looked at.
//
// A WELL-FORMED OBJECT HOLDING NO BODIES IS NOT ONE OF THOSE. It is what a fresh
// `bd init` returns, and the count is handed back to the caller to report rather
// than refused here; see main.
func scan(in []byte) ([]string, int, error) {
	var raw map[string]any
	if err := json.Unmarshal(in, &raw); err != nil {
		return nil, 0, fmt.Errorf("parsing memories: %w", err)
	}
	if raw == nil {
		return nil, 0, fmt.Errorf("the input is JSON null rather than an object of memories")
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var hits []string
	examined := 0
	for _, k := range keys {
		switch v := raw[k].(type) {
		case string:
			examined++
			for _, re := range errata {
				for _, loc := range re.FindAllStringIndex(v, -1) {
					hits = append(hits, fmt.Sprintf("%s: %s", k, excerpt(v, loc[0], loc[1])))
				}
			}
		case map[string]any, []any:
			return nil, 0, fmt.Errorf("%q holds a nested value: the memories are not a flat object of key to body, so this check would read past them", k)
		default:
			// schema_version and any other scalar that is not a body.
		}
	}
	return hits, examined, nil
}

// excerpt quotes the match with enough either side to recognise it, on one line
// so a hit reads like a grep hit.
func excerpt(body string, start, end int) string {
	const pad = 50
	lo, hi := start-pad, end+pad
	prefix, suffix := "...", "..."
	if lo <= 0 {
		lo, prefix = 0, ""
	}
	if hi >= len(body) {
		hi, suffix = len(body), ""
	}
	// pad is a byte count and the bodies carry em-dashes, so both ends have to
	// be walked out to a rune boundary or the excerpt prints the tail of a
	// multi-byte character as replacement bytes.
	for lo > 0 && !utf8.RuneStart(body[lo]) {
		lo--
	}
	for hi < len(body) && !utf8.RuneStart(body[hi]) {
		hi++
	}
	return prefix + strings.Join(strings.Fields(body[lo:hi]), " ") + suffix
}
