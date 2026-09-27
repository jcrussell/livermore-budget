// Command beadcheck refuses errata in .beads/issues.jsonl: prose in which a
// bead makes a past-tense claim about its own text (AGENTS.md, "History's home
// is git"). It reads the committed export, so unlike memcheck it is a full gate.
//
// Bare "said" is absent from the list: beads use "this bead said" to name their
// own requirements, which is true prose. The review-credit line tools/narration
// refuses in source is not refused here either; in a bead it is provenance.
// Gaps between words are \s+ because bead bodies are hard-wrapped.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// errata are matched against one bead's prose; each needs the bead itself as
// the subject of a past-tense claim about its own text.
var errata = []*regexp.Regexp{
	regexp.MustCompile(`(?i)th(is|e)\s+bead\s+(used\s+to|previously|once|no\s+longer)`),
	regexp.MustCompile(`(?i)earlier\s+versions?\s+of\s+this\s+bead`),
	regexp.MustCompile(`(?i)th(is|e)\s+bead's\s+(own\s+)?(text|description|notes?)\s+(above\s+)?(is|was|are|were)\s+wrong`),
}

// prose are the fields a reader of a bead is shown; a field missing here is one
// an erratum could sit in unseen.
var prose = []string{"title", "description", "notes", "design", "close_reason", "acceptance_criteria"}

// commentField is how a hit in a comment's text is reported.
const commentField = "comments[].text"

// hit is one refused phrase.
type hit struct {
	id    string
	field string
	text  string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: beadcheck issues.jsonl")
		os.Exit(2)
	}
	hits, examined, err := scan(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "beadcheck: %v\n", err)
		os.Exit(2)
	}
	if len(hits) == 0 {
		_ = examined
		return
	}
	for _, h := range hits {
		fmt.Fprintf(os.Stderr, "%s (%s): %s\n", h.id, h.field, h.text)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "beadcheck: the beads above say what they used to say.")
	fmt.Fprintln(os.Stderr, "  A bead says what needs DOING. A passage about the text it")
	fmt.Fprintln(os.Stderr, "  replaced leaves the wrong claim as the thing a session reads")
	fmt.Fprintln(os.Stderr, "  first, which is what correcting it was meant to fix. Keep the")
	fmt.Fprintln(os.Stderr, "  corrected statement, and any measurement a reader would")
	fmt.Fprintln(os.Stderr, "  otherwise re-derive, and delete the account of what changed.")
	fmt.Fprintln(os.Stderr, "  Edit with 'bd update <id> --description/--notes', which REPLACE.")
	fmt.Fprintln(os.Stderr, "  See AGENTS.md, \"History's home is git\".")
	os.Exit(1)
}

// scan reports one hit per erratum, sorted by bead id then field. An unreadable
// export or one with no beads is an error, never a clean result.
func scan(path string) ([]hit, int, error) {
	f, err := os.Open(path) // #nosec G304,G703 -- the path is this command's argument.
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	var hits []hit
	examined := 0
	sc := bufio.NewScanner(f)
	// A bead's notes have run past 10KB; the default 64KB limit is too close.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", path, err)
		}
		id, _ := rec["id"].(string)
		if id == "" {
			return nil, 0, fmt.Errorf("%s: a record carries no id, so a hit in it could not be reported", path)
		}
		examined++
		for _, field := range prose {
			if body, ok := rec[field].(string); ok {
				hits = appendErrata(hits, id, field, body)
			}
		}
		comments, _ := rec["comments"].([]any)
		for _, c := range comments {
			if m, ok := c.(map[string]any); ok {
				if body, ok := m["text"].(string); ok {
					hits = appendErrata(hits, id, commentField, body)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}
	if examined == 0 {
		return nil, 0, fmt.Errorf("%s carries no beads at all, so nothing was checked", path)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].id != hits[j].id {
			return hits[i].id < hits[j].id
		}
		return hits[i].field < hits[j].field
	})
	return hits, examined, nil
}

func appendErrata(hits []hit, id, field, body string) []hit {
	for _, re := range errata {
		for _, loc := range re.FindAllStringIndex(body, -1) {
			hits = append(hits, hit{id: id, field: field, text: excerpt(body, loc[0], loc[1])})
		}
	}
	return hits
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
