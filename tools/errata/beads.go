package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

// beadErrata are matched against one bead's prose; each needs the bead itself
// as the subject of a past-tense claim about its own text. Bare "said" is
// absent: beads use "this bead said" to name their own requirements. The
// review-credit line tools/narration refuses in source is not refused here
// either; in a bead it is provenance.
var beadErrata = []*regexp.Regexp{
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

func beadsMain(path string, stderr io.Writer) int {
	hits, _, err := scanBeads(path)
	if err != nil {
		fmt.Fprintf(stderr, "beadcheck: %v\n", err)
		return 2
	}
	if len(hits) == 0 {
		return 0
	}
	lines := make([]string, len(hits))
	for i, h := range hits {
		lines[i] = fmt.Sprintf("%s (%s): %s", h.id, h.field, h.text)
	}
	return refuse(stderr, lines,
		"beadcheck: the beads above say what they used to say.",
		"  A bead says what needs DOING. A passage about the text it",
		"  replaced leaves the wrong claim as the thing a session reads",
		"  first, which is what correcting it was meant to fix. Keep the",
		"  corrected statement, and any measurement a reader would",
		"  otherwise re-derive, and delete the account of what changed.",
		"  Edit with 'bd update <id> --description/--notes', which REPLACE.",
		"  See AGENTS.md, \"History's home is git\".")
}

// scanBeads reports one hit per erratum, sorted by bead id then field, and how
// many beads it read. An unreadable export or one with no beads is an error,
// never a clean result.
func scanBeads(path string) ([]hit, int, error) {
	f, err := os.Open(path) // #nosec G304,G703 -- the path is this command's argument.
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	var hits []hit
	add := func(id, field, body string) {
		for _, text := range matches(beadErrata, body) {
			hits = append(hits, hit{id: id, field: field, text: text})
		}
	}
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
				add(id, field, body)
			}
		}
		comments, _ := rec["comments"].([]any)
		for _, c := range comments {
			if m, ok := c.(map[string]any); ok {
				if body, ok := m["text"].(string); ok {
					add(id, commentField, body)
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
