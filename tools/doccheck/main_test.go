package main

import (
	"os"
	"path/filepath"
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

Ordinary **bold** in a sentence.
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
		"bold":                                                       true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("anchorsIn (-want +got):\n%s", diff)
	}
}

// A citation is not a line. Two in the tree today wrap across two comment lines,
// and four live inside Go string literals where the quotes are backslashed.
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
	// has to open to fix it.
	if got[1].line != 5 {
		t.Errorf("wrapped citation line = %d, want 5", got[1].line)
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

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
