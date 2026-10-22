// Package beadid finds the tracker ids a line of prose cites: the project's
// own, and byob's, which are rows of the same graph. tools/beadrefs resolves
// what it finds against the tracker, and internal/project refuses any in a
// caveat a reader is shown. No example id is spelled in this file: beadrefs
// scans it and would report it.
package beadid

import "regexp"

// fiscPattern alone is not enough, because `fisc-` is an overloaded prefix in
// this tree and not a namespace: fact ids are fisc-f-<hash>, series ids are
// fisc-s-<hash>, the export marker file is .fisc-export and a scratch
// directory is .fisc-write-probe-*. Every one of those begins with something
// a bead id could be. What tells them apart is the SECOND hyphen and the
// leading dot, so the boundaries are applied in Cited rather than here; RE2
// has no lookaround.
var fiscPattern = regexp.MustCompile(`fisc-[a-z0-9]+(?:\.[0-9]+)*`)

// byobPattern takes the whole hyphenated slug, which is one id, and cannot
// end on a hyphen, so an id written before an em dash stops at the slug.
// Cited's trailing-hyphen rule therefore does not apply to it.
var byobPattern = regexp.MustCompile(`byob-[a-z0-9]+(?:-[a-z0-9]+)*(?:\.[0-9]+)*`)

// Cited returns the ids one line CITES, which is not every fisc- token it
// contains: the project's ids first, then byob's, each in line order. A match
// is a citation only when nothing runs into it on either side: a preceding
// dot or hyphen or word byte makes it part of a longer name (.fisc-export),
// and a following hyphen or word byte makes it a prefix of one
// (fisc-f-0ba7b00dfaf7). A following dot is left alone -- the pattern has
// already taken any dotted child, so what remains is the period ending a
// sentence.
func Cited(line string) []string {
	var ids []string
	for _, loc := range fiscPattern.FindAllStringIndex(line, -1) {
		if loc[0] > 0 && runsInto(line[loc[0]-1]) {
			continue
		}
		if loc[1] < len(line) && (line[loc[1]] == '-' || isWordByte(line[loc[1]])) {
			continue
		}
		ids = append(ids, line[loc[0]:loc[1]])
	}
	// No trailing-hyphen rule here; see byobPattern.
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
