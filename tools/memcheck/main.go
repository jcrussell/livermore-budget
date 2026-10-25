// Command memcheck refuses errata in the beads memories, which bd prime injects
// into every session automatically.
//
// It reads `bd memories --json` on stdin -- a flat object of key to body, with
// non-string values (schema_version) skipped -- and exits non-zero on any body
// claiming something about the memory's own former text, per
// AGENTS.md's "History's home is git".
//
// THE PHRASE LIST IS NARROW: "used to say" has honest uses about a DOCUMENT
// the city reprinted. Three patterns require past-tense self-reference; the
// ISO-dated "corrected" and line-leading CORRECTED are house-style shapes that
// may refuse a true sentence about the corpus (fisc-kaq1). A bare-year or
// "(correction: ...)" form must not be added. Gaps are \s+ because bodies are
// hard-wrapped. TestScanFindsOnlyPastTenseSelfReference holds each edge.
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

// errata are matched against a memory body; see the package comment.
var errata = []*regexp.Regexp{
	regexp.MustCompile(`(?i)th(is|e)\s+memory\s+(used\s+to|previously|once|said|no\s+longer)`),
	regexp.MustCompile(`(?i)earlier\s+versions?\s+of\s+this\s+memory`),
	regexp.MustCompile(`(?i)\bthis\s+used\s+to\s+(say|read|end|claim)`),
	regexp.MustCompile(`(?i)\bcorrected\s+20\d\d-\d\d-\d\d`),
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
		// A fresh `bd init` holds no memories; warn rather than refuse, as the
		// arm is advisory when bd is absent too.
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
	fmt.Fprintln(os.Stderr, "  See AGENTS.md, \"History's home is git\".")
	os.Exit(1)
}

// scan reports one line per erratum found, sorted by key. It refuses any shape
// it cannot read as bodies -- a top-level null, or a nested value (the bodies
// moved down a level, which a body count alone misses) -- rather than reporting
// it clean. An empty object is returned with a zero count; see main.
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
	// pad is in bytes and bodies carry em-dashes: walk out to rune boundaries.
	for lo > 0 && !utf8.RuneStart(body[lo]) {
		lo--
	}
	for hi < len(body) && !utf8.RuneStart(body[hi]) {
		hi++
	}
	return prefix + strings.Join(strings.Fields(body[lo:hi]), " ") + suffix
}
