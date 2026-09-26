// Command beadcheck refuses errata in the beads issue text.
//
// It reads .beads/issues.jsonl, the committed export, and exits non-zero on any
// prose field carrying a phrase that can only be a claim about the bead's own
// former text. AGENTS.md's "History's home is git" forbids that shape in all
// prose; `make narration` enforced it in Go comments and in the injected
// memories, and a bead was the largest medium left ungoverned.
//
// IT IS A FULL GATE, UNLIKE THE MEMORY ARM IT RUNS BESIDE. Memories live in the
// Dolt DB and in no git artifact, so memcheck warns and continues whenever bd
// cannot answer. This resolves against a committed file with no bd, no Dolt and
// no network, which is what makes beadrefs a required CI check; this is built
// the same way and for the same reason.
//
// BARE "SAID" IS DELIBERATELY ABSENT FROM THE LIST, and it is the whole of what
// makes this landable. Measured over the 680 issues in the export: six beads use
// "this bead said" to name their own REQUIREMENTS rather than their own former
// text -- "TWO THINGS THIS BEAD SAID WERE NEEDED HAD NOT BEEN FILED",
// "THE GUARD THIS BEAD SAID WAS THE POINT IS NOW PAID FOR", "lifted as this bead
// said to". Every one is a true sentence, and a pattern that took them would
// teach people to reword around the gate rather than to stop writing errata. The
// discriminator is tense about the TEXT: "used to", "previously", "once", "no
// longer", "an earlier version of". See fisc-3vq1.
//
// THE REVIEW-CREDIT LINE THE GO ARM REFUSES -- spelled once, in
// tools/narration -- IS NOT REFUSED HERE AND MUST NOT BE ADDED. AGENTS.md
// forbids that credit in SOURCE and asks for it in a commit message; in a bead
// it is provenance of a finding rather than a claim about the tree's past, and
// 109 of the 680 issues carry it. Adding it would demand the sweep AGENTS.md
// refuses. Writing it out here would take the Go arm red on this very file,
// which is the collision the last paragraph is about.
//
// EVERY GAP BETWEEN WORDS IS \s+ AND NOT A SPACE, for memcheck's reason: bead
// bodies are hard-wrapped paragraphs and most of these phrases are long enough
// to wrap, so a literal space misses the majority of the places one could sit.
//
// A BEAD THAT DOCUMENTS THIS LIST BY QUOTING IT IS REFUSED BY IT, and that is
// declared rather than papered over. fisc-3vq1 quoted the phrases in a
// measurement table and had to be reworded to describe them instead. doccheck's
// package comment records the same collision against its own citation pattern.
// The alternative -- exempting a bead id -- is worse: it is an exemption nothing
// re-derives, on the one bead most likely to be read as the rule.
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

// errata are matched against one bead's prose. Each requires the bead itself to
// be the subject of a past-tense claim about its own text; see the package
// comment for why nothing looser belongs here.
var errata = []*regexp.Regexp{
	regexp.MustCompile(`(?i)th(is|e)\s+bead\s+(used\s+to|previously|once|no\s+longer)`),
	regexp.MustCompile(`(?i)earlier\s+versions?\s+of\s+this\s+bead`),
	regexp.MustCompile(`(?i)th(is|e)\s+bead's\s+(own\s+)?(text|description|notes?)\s+(above\s+)?(is|was|are|were)\s+wrong`),
}

// prose are the fields a reader of a bead is shown. A field absent from this
// list is one an erratum could sit in unseen, so adding a field to the tracker
// means adding it here.
var prose = []string{"title", "description", "notes", "design", "close_reason", "acceptance_criteria"}

// commentField is how a hit in a comment's text is reported; a comment is
// shown by `bd show` like any field in prose.
const commentField = "comments[].text"

// hit is one refused phrase: which bead, which field, and what matched.
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

// scan reports one hit per erratum found, sorted by bead id and then by field
// so the output is stable across runs.
//
// IT REFUSES AN EXPORT IT CANNOT READ rather than reporting it clean, and
// refuses one carrying no beads at all. Both are the shape AGENTS.md calls green
// because the gate fired: an exit code that says nothing was wrong when it means
// nothing was looked at. Unlike memcheck's empty-memory case there is no honest
// way to reach zero here -- the export is committed, so a checkout always has
// one.
func scan(path string) ([]hit, int, error) {
	f, err := os.Open(path) // #nosec G304,G703 -- the path is this command's argument.
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	var hits []hit
	examined := 0
	sc := bufio.NewScanner(f)
	// The same ceiling beadrefs reads this file with: a bead's notes have run
	// past 10KB and the default 64KB token limit is not far above that.
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
	// pad is a byte count and bead bodies carry em-dashes, so both ends have to
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
