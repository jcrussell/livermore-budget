package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The anchors a citation may legitimately name. The post-colon case is the one
// worth stating: the tree cites "green because the gate fired", which is the
// tail of a heading that begins "The failure mode to look for:".
func TestAnchorsInTakesHeadingsBoldPhrasesAndPostColonTails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	write(t, path, `# Title

## Build & Test

### The failure mode to look for: green because the gate fired

**History's home is git.** No credit lines.

Ordinary **bold** mid-sentence is not an anchor.
`)
	got, err := anchorsIn(path)
	if err != nil {
		t.Fatalf("anchorsIn: %v", err)
	}
	want := map[string]bool{
		"title":        true,
		"build & test": true,
		"the failure mode to look for: green because the gate fired": true,
		"green because the gate fired":                               true,
		"history's home is git":                                      true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("anchorsIn (-want +got):\n%s", diff)
	}
}

// Bold anchors pair over the whole file and the body crosses a lone asterisk.
// Mutation: a `[^*]+` body loses the anchor after the fixture's `byob-*`.
func TestBoldAnchorsPairAcrossLinesAndAcrossALoneAsterisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	write(t, path, `# T

- **Never claim or close a `+"`byob-*`"+` bead**, ever.
- **A count against the
  documents** stays, and so does this.
- **A count that is the
  evidence** stays too.
`)
	got, err := anchorsIn(path)
	if err != nil {
		t.Fatalf("anchorsIn: %v", err)
	}
	want := map[string]bool{
		"t": true,
		// fold treats `*` as furniture, so the anchor normalises to a space;
		// what matters is that the run paired and the anchors after it survive.
		"never claim or close a `byob- ` bead": true,
		"a count against the documents":        true,
		"a count that is the evidence":         true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("anchorsIn (-want +got):\n%s", diff)
	}
}

// Only a bold lead phrase is an anchor: opening a line, a bullet or a numbered
// item. Inline emphasis is not, and numbered items are.
func TestInlineEmphasisIsNotAnAnchor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	write(t, path, `# T

- **A lead phrase.** With **inline** emphasis after it.
4. **A numbered one** counts too.

Prose with **stress** in the middle of a sentence.
`)
	got, err := anchorsIn(path)
	if err != nil {
		t.Fatalf("anchorsIn: %v", err)
	}
	want := map[string]bool{"t": true, "a lead phrase": true, "a numbered one": true}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("anchorsIn (-want +got):\n%s", diff)
	}
}

// A citation is not a line. Two in the tree today wrap across two comment lines,
// and some live inside string literals where the quotes are backslashed -- in
// Go, and in a shell echo in the Makefile.
func TestCitesInReadsWrappedAndEscapedCitations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, `package x

// One line: see AGENTS.md, "The extraction boundary".
//
// Wrapped over two: see AGENTS.md, "green because the
// gate fired".
//
// Possessive: AGENTS.md's "Before you quote a number".
func f() {
	println("AGENTS.md, \"History's home is git\"")
}

// Under: AGENTS.md, under "Review does not cover this project's main risks".
`)
	got, err := citesIn(path)
	if err != nil {
		t.Fatalf("citesIn: %v", err)
	}
	var titles []string
	for _, c := range got {
		titles = append(titles, c.title)
	}
	want := []string{
		"The extraction boundary",
		"green because the gate fired",
		"Before you quote a number",
		"History's home is git",
		"Review does not cover this project's main risks",
	}
	if diff := cmp.Diff(want, titles); diff != "" {
		t.Errorf("titles (-want +got):\n%s", diff)
	}
	// The wrapped one is reported where it STARTS, which is the line a reader
	// has to open to fix it. Fatal rather than Errorf: indexing got[1] after a
	// short read panics instead of failing readably.
	if len(got) < 2 {
		t.Fatalf("read %d citations, want %d", len(got), len(want))
	}
	if got[1].line != 5 {
		t.Errorf("wrapped citation line = %d, want 5", got[1].line)
	}
}

// A citation that wraps inside a markdown blockquote must be FOUND, not skipped.
// A docs/ page may open with one, so a gap here is a dead citation
// that passes in silence -- the fail-open direction. citePattern's gap class and
// furniture's must agree; `>` was in the second and missing from the first.
func TestABlockquoteCitationIsFoundAndNormalised(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evidence.md")
	write(t, path, `# Evidence

> Evidence for AGENTS.md, "Where
> writing goes". This file states no rule.
`)
	got, err := citesIn(path)
	if err != nil {
		t.Fatalf("citesIn: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d citations, want 1; a zero here is the fail-open this test exists for", len(got))
	}
	if diff := cmp.Diff("Where writing goes", got[0].title); diff != "" {
		t.Errorf("title (-want +got):\n%s", diff)
	}
}

// The failure this command exists to catch, end to end: a heading is renamed and
// the citations of it are left behind.
func TestARenamedHeadingOrphansItsCitations(t *testing.T) {
	dir := t.TempDir()
	agents := filepath.Join(dir, "AGENTS.md")
	src := filepath.Join(dir, "x.go")
	write(t, src, "package x\n\n// see AGENTS.md, \"The extraction boundary\".\n")

	write(t, agents, "# T\n\n## The extraction boundary\n")
	anchors, err := anchorsIn(agents)
	if err != nil {
		t.Fatalf("anchorsIn: %v", err)
	}
	cites, err := citesIn(src)
	if err != nil {
		t.Fatalf("citesIn: %v", err)
	}
	// The green case must be green for the right reason: the citation was read
	// and matched, not skipped. A run that examined nothing also reports no dead
	// citations -- see AGENTS.md, "green because the gate fired".
	if len(cites) != 1 {
		t.Fatalf("read %d citations, want 1; a zero here would make the assertion below vacuous", len(cites))
	}
	if !anchors[fold(cites[0].title)] {
		t.Errorf("%q does not resolve against the heading it names", cites[0].title)
	}

	write(t, agents, "# T\n\n## The extraction boundaries\n")
	renamed, err := anchorsIn(agents)
	if err != nil {
		t.Fatalf("anchorsIn: %v", err)
	}
	if renamed[fold(cites[0].title)] {
		t.Errorf("%q still resolves after the heading was renamed", cites[0].title)
	}
}

// An AGENTS.md that yields no anchors would pass every citation in the tree.
// main refuses that case; this pins the condition it turns on.
func TestAnchorlessAgentsFileYieldsNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	write(t, path, "no headings here, just prose\n")
	got, err := anchorsIn(path)
	if err != nil {
		t.Fatalf("anchorsIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("anchorsIn = %v, want empty", got)
	}
}

// fold is what makes a citation and an anchor comparable. The bolded lead phrase
// carries its sentence's full stop inside the bold; the citations of it do not.
func TestFoldDropsFurnitureAndTrailingPunctuation(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"History's home is git.", "history's home is git"},
		{"green because the\n// gate fired", "green because the gate fired"},
		{"  Testing  ", "testing"},
		{`Before you quote a number`, "before you quote a number"},
	} {
		if got := fold(tc.in); got != tc.want {
			t.Errorf("fold(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The near-misses citePattern cannot see are refused when their quoted title
// resolves: a verb between anchor and quote, and a backticked anchor. The
// canonical citation beside them is not reported.
func TestANearMissCitationIsRefusedAndACanonicalOneIsNot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, `package x

// Canonical: see AGENTS.md, "The extraction boundary".
//
// A verb: AGENTS.md calls that guard "the designed answer".
//
// Backticks: `+"`AGENTS.md`"+` names it under "The node boundary".
`)
	anchors := map[string]bool{
		"the extraction boundary": true,
		"the designed answer":     true,
		"the node boundary":       true,
	}
	got, err := malformedIn(path, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	var lines []int
	for _, c := range got {
		lines = append(lines, c.line)
	}
	if diff := cmp.Diff([]int{5, 7}, lines); diff != "" {
		t.Fatalf("malformed lines (-want +got):\n%s", diff)
	}
	for _, c := range got {
		if c.title == "" {
			t.Errorf("a malformed finding at line %d carries no snippet to fix", c.line)
		}
	}
}

// A mention with no quoted title at all is out of reach, and this pins the
// boundary the package comment declares rather than a wish: a possessive with
// a bare phrase after it yields nothing from either arm.
func TestACitationWithNoQuotedTitleIsOutOfReach(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package x\n\n// The table AGENTS.md's review-loop section describes has moved.\n")
	// The phrase IS an anchor here, so the exclusion below can only come from
	// the absence of quotes -- not from a failed resolution.
	bad, err := malformedIn(path, map[string]bool{"review-loop section": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	cites, err := citesIn(path)
	if err != nil {
		t.Fatalf("citesIn: %v", err)
	}
	if len(bad) != 0 || len(cites) != 0 {
		t.Errorf("malformed = %v, cites = %v; want both empty", bad, cites)
	}
}

// A list of quoted string literals with the anchor's file name among them is
// not a citation, and the quoted-run-opens-like-a-title rule is what keeps it
// out: the run after the anchor's own closing quote opens with a comma. The
// tree carries exactly this shape in another tool's test fixture.
func TestAQuotedStringListIsNotAMalformedCitation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package x\n\nvar files = []string{\"AGENTS.md\", \"internal/x.go\"}\n")
	// The neighbouring literal IS an anchor here, so the exclusion can only
	// come from the shape rule -- the run after the anchor's own closing quote
	// opens with a comma, and a quoted run cannot be crossed to reach further.
	got, err := malformedIn(path, map[string]bool{"internal/x.go": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("malformedIn = %v, want empty", got)
	}
}

// A passing mention of the anchor file shortly before a canonical citation
// must not turn that citation's own quotes into a malformed finding: a loose
// hit is claimed by any canonical match starting inside its span.
func TestAMentionBeforeACanonicalCitationIsNotMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package x\n\n// It is in AGENTS.md; see AGENTS.md, \"Testing\".\n")
	bad, err := malformedIn(path, map[string]bool{"testing": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(bad) != 0 {
		t.Errorf("malformedIn = %v, want empty", bad)
	}
	cites, err := citesIn(path)
	if err != nil {
		t.Fatalf("citesIn: %v", err)
	}
	if len(cites) != 1 {
		t.Fatalf("read %d citations, want 1; the canonical one must still be resolved", len(cites))
	}
}

// A true sentence ending with the anchor is not a finding when the next
// statement opens with a string literal: that line has neither citation
// furniture nor the quote, so it is outside loosePattern's gap.
func TestATrueSentenceAboveAStringLiteralIsNotMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package x\n\n// The rule lives in AGENTS.md.\nvar basis = \"audited\"\n")
	// The literal IS an anchor here, so the exclusion can only come from the
	// gap rule -- the sentence ends at the anchor and the next line opens with
	// no citation furniture -- not from a failed resolution.
	got, err := malformedIn(path, map[string]bool{"audited": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("malformedIn = %v, want empty; the gap crossed the newline into an unrelated statement", got)
	}
}

// A quoted run resolving against the anchors is a near-miss citation; one
// resolving against nothing is a sentence. Resolution, not distance, decides.
func TestProseQuotingANonSectionWordIsNotMalformed(t *testing.T) {
	dir := t.TempDir()
	anchors := map[string]bool{"the node boundary": true}

	prose := filepath.Join(dir, "a.go")
	write(t, prose, "package x\n\n// The rule in AGENTS.md forbids the word \"audited\" on these pages.\nvar X int\n")
	got, err := malformedIn(prose, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("malformedIn = %v, want empty; the quoted word names no section", got)
	}

	nearMiss := filepath.Join(dir, "b.go")
	write(t, nearMiss, "package x\n\n// AGENTS.md at \"The node boundary\".\nvar X int\n")
	got, err = malformedIn(nearMiss, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 1 || got[0].line != 3 {
		t.Errorf("malformedIn = %v, want one finding at line 3; the quoted run names a section through a non-canonical separator", got)
	}
}

// The net has no length bound: a near-miss whose quoted title sits a long
// clause after the anchor is still a citation of the section it names, and a
// bounded gap let exactly this shape escape in silence.
func TestALongSeparatorNamingARealSectionIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package x\n\n// AGENTS.md is entirely clear about this particular matter, under \"The node boundary\".\n")
	got, err := malformedIn(path, map[string]bool{"the node boundary": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 1 || got[0].line != 3 {
		t.Errorf("malformedIn = %v, want one finding at line 3; distance is not what makes a citation", got)
	}
}

// What replaced the length bound: the gap may not cross a sentence end. Both
// fixtures are the shape of real comment blocks in this tree -- a mention of
// the anchor file whose PARAGRAPH later quotes a genuine section title -- and
// their titles resolve, so only the sentence rule keeps them out of the net.
func TestASentenceEndBetweenAnchorAndQuoteIsNotACitation(t *testing.T) {
	dir := t.TempDir()
	anchors := map[string]bool{"history's home is git": true}

	sameLine := filepath.Join(dir, "x.go")
	write(t, sameLine, "package x\n\n// the shapes an anchor takes in AGENTS.md. Bold is an anchor because the tree cites one: \"History's home is git\".\n")
	got, err := malformedIn(sameLine, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("same line: malformedIn = %v, want empty; a sentence ended between the anchor and the quote", got)
	}

	wrapped := filepath.Join(dir, "y.go")
	write(t, wrapped, "package y\n\n// It is mentioned in AGENTS.md.\n// \"History's home is git\" is quoted afterwards.\n")
	got, err = malformedIn(wrapped, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("across furniture: malformedIn = %v, want empty; the sentence ended before the wrap", got)
	}
}

// The declared misspelling gap: the net catches a near-SEPARATOR of a real
// title, never a near-MISSPELLING of one -- a title one letter off resolves
// against nothing, so neither arm sees it, and the package comment says so.
func TestANearMisspelledTitleEscapesBothArms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package x\n\n// AGENTS.md is entirely clear about this particular matter, under \"The node boundry\".\n")
	bad, err := malformedIn(path, map[string]bool{"the node boundary": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	cites, err := citesIn(path)
	if err != nil {
		t.Fatalf("citesIn: %v", err)
	}
	if len(bad) != 0 || len(cites) != 0 {
		t.Errorf("malformed = %v, cites = %v; want both empty -- this gap is declared, not covered", bad, cites)
	}
}

// A near-miss wrapping into comment furniture is netted in any file; one
// wrapping into a bare quote-led line is out of reach everywhere, as
// loosePattern's doc comment declares.
func TestAWrappedNearMissIsStillRefused(t *testing.T) {
	dir := t.TempDir()

	anchors := map[string]bool{"the node boundary": true}

	goFile := filepath.Join(dir, "x.go")
	write(t, goFile, "package x\n\n// A verb, wrapped: AGENTS.md names it\n// under \"The node boundary\".\n")
	got, err := malformedIn(goFile, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 1 || got[0].line != 3 {
		t.Errorf("comment-wrapped near-miss: malformedIn = %v, want one finding at line 3", got)
	}

	mdFile := filepath.Join(dir, "x.md")
	write(t, mdFile, "# T\n\nSee AGENTS.md at\n\"The node boundary\".\n")
	got, err = malformedIn(mdFile, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("markdown quote-led wrap: malformedIn = %v, want empty; this shape is the gap loosePattern declares", got)
	}

	goQuoteLed := filepath.Join(dir, "y.go")
	write(t, goQuoteLed, "package y\n\n// See AGENTS.md at\n\"The node boundary\".\n")
	got, err = malformedIn(goQuoteLed, anchors)
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Go quote-led line: malformedIn = %v, want empty; a marker-less quote-led line in Go is a string literal, not a wrapped sentence", got)
	}
}

// A markdown sentence ending with the anchor, above a sentence opening with a
// quoted phrase, cites nothing; the gap must not cross the bare newline.
func TestProseEndingWithTheAnchorAboveAQuoteLedSentenceIsNotMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.md")
	write(t, path, "# Doc\n\nThe rule lives in AGENTS.md.\n\"Absent is not zero\" is a phrase this file uses loosely.\n")
	// The phrase IS an anchor here: the exclusion must come from the gap rule
	// -- the sentence ends at the anchor, and the bare newline carries no
	// furniture -- which is the shape this test pins.
	got, err := malformedIn(path, map[string]bool{"absent is not zero": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("malformedIn = %v, want empty; the file contains no citation", got)
	}
}

// A comment ending with the anchor above an indented string element in a
// composite literal is not stitched to it.
func TestACommentAboveAnIndentedLiteralElementIsNotMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	write(t, path, "package geom\n\nvar names = []string{\n\t// The band names, in the order AGENTS.md.\n\t\"left\",\n\t\"right\",\n}\n")
	// The element IS an anchor here: the exclusion must come from the gap rule
	// -- the sentence ends at the anchor, and the indented element line carries
	// no furniture -- which is the shape this test pins.
	got, err := malformedIn(path, map[string]bool{"left": true})
	if err != nil {
		t.Fatalf("malformedIn: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("malformedIn = %v, want empty; the file contains no citation", got)
	}
}

// The scan resolves against the tree holding the AGENTS.md argument, not the
// working directory. The exempt file is a real fixture scanned by absolute
// path, so the run stays green only if the exempt lookup skipped it.
func TestTheScanRunsFromAnyWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	// Every exempted file must exist, or the run fails on checkExemptions.
	for _, d := range []string{"docs", filepath.Join("tools", "doccheck"), filepath.Join("tools", "beadrefs")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	write(t, filepath.Join(root, "AGENTS.md"), "# T\n\n## The extraction boundary\n")
	write(t, filepath.Join(root, "docs", "real.md"), "# real\n")
	write(t, filepath.Join(root, "tools", "doccheck", "main_test.go"),
		"package main\n\n// AGENTS.md calls that guard \"the designed answer\". See docs/gone.md.\n")
	write(t, filepath.Join(root, "tools", "beadrefs", "main_test.go"), "package main\n")
	src := filepath.Join(root, "x.go")
	write(t, src, "package x\n\n// see AGENTS.md, \"The extraction boundary\". See docs/real.md.\n"+
		"// TestTheOnlyOne is what pins it.\n")
	// And a declaration for it: run refuses a scan that finds none.
	decls := filepath.Join(root, "x_test.go")
	write(t, decls, "package x\n\nfunc TestTheOnlyOne(t *testing.T) {}\n")

	t.Chdir(t.TempDir())
	var stderr strings.Builder
	code := run([]string{filepath.Join(root, "AGENTS.md"), src, decls, filepath.Join(root, "tools")}, &stderr)
	if code != 0 {
		t.Errorf("run = %d from an unrelated working directory, want 0; stderr:\n%s", code, stderr.String())
	}
}

// A docs/ path is found in a markdown link, in backticks, opening a line, and
// behind ./ and ../; a URL naming another repository's docs is not.
func TestDocRefsInFindsCitedPathsAndSkipsURLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.md")
	write(t, path, `# T

See [the charter](docs/schema-contracts.md) and `+"`docs/m0-spike.md`"+`.

docs/review-loop-evidence.md opens a line. Not ours:
https://github.com/x/y/blob/main/docs/SYNC.md

Relative, from a subdirectory: [contract](../docs/sankey-contract.md) and
./docs/local.md too.
`)
	got, err := docRefsIn(path)
	if err != nil {
		t.Fatalf("docRefsIn: %v", err)
	}
	var paths []string
	for _, c := range got {
		paths = append(paths, c.title)
	}
	want := []string{
		"docs/schema-contracts.md",
		"docs/m0-spike.md",
		"docs/review-loop-evidence.md",
		"../docs/sankey-contract.md",
		"./docs/local.md",
	}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("paths (-want +got):\n%s", diff)
	}
}

// Renaming a docs/ page reddens every pointer at it: the resolution arm
// reports the path that no longer exists and stays silent about the one that
// does.
func TestAMissingDocPathReddens(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write(t, filepath.Join(root, "docs", "real.md"), "# real\n")
	refs := []cite{
		{title: "docs/real.md", file: "x.go", line: 3},
		{title: "docs/gone.md", file: "x.go", line: 5},
	}
	got := missingDocs(root, refs)
	want := []cite{{title: "docs/gone.md", file: "x.go", line: 5}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(cite{})); diff != "" {
		t.Errorf("missingDocs (-want +got):\n%s", diff)
	}
}

// A ./ or ../ path resolves against the citing file's directory and a bare
// docs/ path against root: the third ref is the first written from root, and
// it must redden while the first stays green.
func TestARelativeDocPathResolvesAgainstTheCitingFile(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"docs", "testdata"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	write(t, filepath.Join(root, "docs", "real.md"), "# real\n")
	fromTestdata := filepath.Join(root, "testdata", "README.md")
	refs := []cite{
		{title: "../docs/real.md", file: fromTestdata, line: 3},
		{title: "./docs/gone.md", file: fromTestdata, line: 5},
		{title: "../docs/real.md", file: filepath.Join(root, "x.md"), line: 7},
		{title: "docs/real.md", file: fromTestdata, line: 9},
	}
	got := missingDocs(root, refs)
	want := []cite{
		{title: "./docs/gone.md", file: fromTestdata, line: 5},
		{title: "../docs/real.md", file: filepath.Join(root, "x.md"), line: 7},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(cite{})); diff != "" {
		t.Errorf("missingDocs (-want +got):\n%s", diff)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A cited test name resolves against a declaration or it is dead; each half of
// the wrap rule has a case here.
func TestADeadTestNameIsRefusedAndTheThreeLiveShapesAreNot(t *testing.T) {
	dir := t.TempDir()
	decls := filepath.Join(dir, "x_test.go")
	write(t, decls, "package x\n\nfunc TestTheLiveOne(t *testing.T) {}\n\n"+
		"func TestTheWrappedOneEndsHere(t *testing.T) {}\n\n"+
		"func TestTheHyphenatedOne(t *testing.T) {}\n\n"+
		"func TestTheSentenceRunsOn(t *testing.T) {}\n")
	src := filepath.Join(dir, "y.go")
	write(t, src, "package y\n\n"+
		"// TestTheLiveOne pins it.\n"+
		"// TestTheWrappedOne\n// EndsHere pins it too.\n"+
		"// TestTheHyphenated-\n// One pins it as well.\n"+
		"// TestTheSentenceRunsOn. Is the point of it.\n"+
		"// TestTheDeadOne pins nothing at all.\n")

	refs, err := testRefsIn(src)
	if err != nil {
		t.Fatalf("testRefsIn: %v", err)
	}
	// Five citations, or the assertion below is about a file nothing read.
	if len(refs) != 5 {
		t.Fatalf("read %d cited names, want 5: %v", len(refs), refs)
	}
	declared, err := testDeclsIn(decls)
	if err != nil {
		t.Fatalf("testDeclsIn: %v", err)
	}
	set := map[string]bool{}
	for _, d := range declared {
		set[d] = true
	}

	var got []string
	for _, c := range deadTests(refs, set) {
		got = append(got, c.title)
	}
	want := []string{"TestTheDeadOne"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("deadTests (-want +got):\n%s", diff)
	}
}

// The over-join case: the joined form names no test, the unwrapped one answers.
func TestUnwrapOverJoinsAndTheUnwrappedFormIsWhatAnswers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "y.go")
	write(t, src, "package y\n\n// TestTheSentenceRunsOn\n// Is the point of it.\n")
	refs, err := testRefsIn(src)
	if err != nil {
		t.Fatalf("testRefsIn: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("read %d cited names, want 1", len(refs))
	}
	if !slicesContains(refs[0].alts, "TestTheSentenceRunsOnIs") {
		t.Fatalf("unwrap did not produce the over-joined form; alts = %v", refs[0].alts)
	}
	set := map[string]bool{"TestTheSentenceRunsOn": true}
	if dead := deadTests(refs, set); len(dead) != 0 {
		t.Errorf("the unwrapped form is declared and the citation still reported dead: %v", dead)
	}
}

// testDeclPattern reads a declaration, not a citation; without the line anchor
// every citation would resolve against itself.
func TestTestDeclPatternReadsDeclarationsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x_test.go")
	write(t, path, "package x\n\n"+
		"// TestNotDeclaredHere is only named.\n"+
		"func TestDeclared(t *testing.T) {\n\tname := \"TestInAStringLiteral\"\n\t_ = name\n}\n"+
		"\tfunc TestIndentedIsNotATopLevelFunc(t *testing.T) {}\n")
	got, err := testDeclsIn(path)
	if err != nil {
		t.Fatalf("testDeclsIn: %v", err)
	}
	if diff := cmp.Diff([]string{"TestDeclared"}, got); diff != "" {
		t.Errorf("testDeclsIn (-want +got):\n%s", diff)
	}
}

func slicesContains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// TestDeadSchemaRefsResolvesEveryPointerAndEveryTypedef holds the schema arm
// to both halves: a cited pointer must name a node of a schema this tree has,
// and a Fisc* typedef must cite a schema path at all.
func TestDeadSchemaRefsResolvesEveryPointerAndEveryTypedef(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("schema/x.schema.json", `{"properties": {"nodes": {"items": {}}, "words": {"prefixItems": [{}, {"default": null}]}}}`)
	write("site/app.js", "/** schema/x.schema.json#/properties/nodes/items. @typedef {Object} FiscNode */\n"+
		"/** schema/x.schema.json#/properties/edges/items. @typedef {Object} FiscLink */\n"+
		"/** schema/y.schema.json. @typedef {Object} FiscDoc */\n"+
		"/** a shape with no path. @typedef {Object} FiscYear */\n"+
		"/** a shape with no path. @typedef {Record<string, any> & {\n *   a?: {b: number}\n * }} FiscSpan */\n"+
		"/** schema/x.schema.json#/properties/words/prefixItems/1/default. @typedef {Object} FiscWord */\n"+
		"/** schema/x.schema.json#/properties/words/prefixItems/2. @typedef {Object} FiscPast */\n")
	got, err := deadSchemaRefs(root, []string{filepath.Join(root, "site")})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, c := range got {
		titles = append(titles, c.title)
	}
	want := []string{
		"schema/x.schema.json#/properties/edges/items names no node of it",
		"schema/y.schema.json is not a schema this tree has",
		"schema/x.schema.json#/properties/words/prefixItems/2 names no node of it",
		"FiscYear names no schema path, so nothing holds it to a shape",
		"FiscSpan names no schema path, so nothing holds it to a shape",
	}
	if diff := cmp.Diff(want, titles); diff != "" {
		t.Errorf("deadSchemaRefs (-want +got):\n%s", diff)
	}
}

// A markdown link between two docs/ pages names no docs/ path, so only
// mdLinksIn sees it: a sibling that exists stays green, one that does not
// reddens in every link shape and to any file type, a bare docs/ target
// resolves from the linking file as GitHub resolves it, a URL, a mailto, a
// fragment-only link and a path docPathPattern already owns are skipped, and a
// .go file is not read for links at all.
func TestARelativeMarkdownLinkResolvesAgainstTheLinkingFile(t *testing.T) {
	root := t.TempDir()
	outputs := filepath.Join(root, "docs", "outputs")
	if err := os.MkdirAll(filepath.Join(root, "docs", "design"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(outputs, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write(t, filepath.Join(outputs, "real.md"), "# real\n")
	write(t, filepath.Join(root, "docs", "design", "model.md"), "# model\n")
	page := filepath.Join(outputs, "page.md")
	write(t, page, `# page

[a sibling](real.md#a-section), [up and over](../design/model.md),
[gone](gone.md), [a site](https://example.com/x.md), [here](#top),
[titled](gone-titled.md "t"), [bracketed](<gone-bracketed.md>),
[already docPathPattern's](../../docs/gone-docs.md), [mail](mailto:x@y.z),
[a schema](../../schema/gone.json), [root-relative](docs/outputs/real.md),
[a picture under docs/](../../docs/gone.svg).

[ref]: gone-ref.md
[^1]: A footnote, which names no file.
`)
	goFile := filepath.Join(root, "x.go")
	write(t, goFile, "package x\n\n// [gone](gone.md)\n")

	var refs []cite
	for _, path := range []string{page, goFile} {
		got, err := mdLinksIn(path)
		if err != nil {
			t.Fatalf("mdLinksIn(%s): %v", path, err)
		}
		refs = append(refs, got...)
	}
	var titles []string
	for _, r := range refs {
		titles = append(titles, r.title)
	}
	want := []string{"./real.md", "../design/model.md", "./gone.md",
		"./gone-titled.md", "./gone-bracketed.md", "../../schema/gone.json",
		"./docs/outputs/real.md", "../../docs/gone.svg", "./gone-ref.md"}
	if diff := cmp.Diff(want, titles); diff != "" {
		t.Errorf("links found (-want +got):\n%s", diff)
	}
	var dead []string
	for _, m := range missingDocs(root, refs) {
		dead = append(dead, m.title)
	}
	if diff := cmp.Diff(want[2:], dead); diff != "" {
		t.Errorf("missingDocs (-want +got):\n%s", diff)
	}
}

// The docs/ path floor counts docPathPattern's own finds: a tree whose only
// docs/ references are relative markdown links still trips it, so a pattern
// that stopped matching cannot hide behind the link arm.
func TestTheDocPathFloorIgnoresMarkdownLinks(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{filepath.Join("docs", "sub"), filepath.Join("tools", "doccheck"), filepath.Join("tools", "beadrefs")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	write(t, filepath.Join(root, "AGENTS.md"), "# T\n\n## The extraction boundary\n")
	write(t, filepath.Join(root, "tools", "doccheck", "main_test.go"), "package main\n")
	write(t, filepath.Join(root, "tools", "beadrefs", "main_test.go"), "package main\n")
	write(t, filepath.Join(root, "docs", "sub", "a.md"), "# a\n\n[b](b.md)\n")
	write(t, filepath.Join(root, "docs", "sub", "b.md"), "# b\n")
	src := filepath.Join(root, "x.go")
	write(t, src, "package x\n\n// see AGENTS.md, \"The extraction boundary\".\n// TestTheOnlyOne pins it.\n")
	decls := filepath.Join(root, "x_test.go")
	write(t, decls, "package x\n\nfunc TestTheOnlyOne(t *testing.T) {}\n")

	var stderr strings.Builder
	code := run([]string{filepath.Join(root, "AGENTS.md"), src, decls, filepath.Join(root, "docs")}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "no docs/ path found") {
		t.Errorf("run = %d, want 2 on the docs/ path floor; stderr:\n%s", code, stderr.String())
	}
}
