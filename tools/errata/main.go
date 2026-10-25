// Command errata refuses errata -- prose making a past-tense claim about its
// own former text (AGENTS.md, "History's home is git") -- in two places a
// session reads unasked:
//
//	errata beads issues.jsonl   the committed export; a full gate
//	errata memories < json      `bd memories --json`; advisory, see Makefile
//
// Each arm has its own phrase list, because "said" and "used to say" are true
// prose in one place and an erratum in the other. Gaps between words are \s+
// because bodies are hard-wrapped.
package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

func main() {
	switch {
	case len(os.Args) == 3 && os.Args[1] == "beads":
		os.Exit(beadsMain(os.Args[2], os.Stderr))
	case len(os.Args) == 2 && os.Args[1] == "memories":
		os.Exit(memoriesMain(os.Stdin, os.Stderr))
	default:
		fmt.Fprintln(os.Stderr, "usage: errata beads issues.jsonl | errata memories < memories.json")
		os.Exit(2)
	}
}

// refuse prints each hit, a blank line and the arm's explanation, and returns
// the exit code for a refusal.
func refuse(w io.Writer, hits []string, explanation ...string) int {
	for _, h := range hits {
		fmt.Fprintln(w, h)
	}
	fmt.Fprintln(w, "")
	for _, line := range explanation {
		fmt.Fprintln(w, line)
	}
	return 1
}

// matches returns an excerpt for every match of every pattern in body.
func matches(patterns []*regexp.Regexp, body string) []string {
	var out []string
	for _, re := range patterns {
		for _, loc := range re.FindAllStringIndex(body, -1) {
			out = append(out, excerpt(body, loc[0], loc[1]))
		}
	}
	return out
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
