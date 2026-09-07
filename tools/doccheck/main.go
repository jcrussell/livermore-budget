// Command doccheck refuses a pointer at nothing: a citation naming no section
// of AGENTS.md, a citation of it written outside the canonical form, and a
// docs/ path that resolves to no file.
//
// AGENTS.md is cited by name from Go, from the Makefile and from site/app.js: a
// see-also naming a section of it in quotes. Nothing checked those strings, so
// renaming a section silently turned every citation of it into a pointer at
// nothing, which reads as though the rule is written down somewhere and sends
// the reader looking for a heading that is gone.
//
// THIS COMMENT DELIBERATELY SPELLS NO CITATION OUT. The examples it wants are
// exactly the shape citePattern matches, so writing one here would make the
// command report itself. beadrefs solved the same problem by exempting its own
// fixtures; that is the wrong trade here, because the only other citation in
// this file is the one the failure message prints, and it is worth checking.
//
// FOUR OF THE CITATIONS ARE PRINTED TO A TERMINAL by a failing gate rather than
// only sitting in a comment -- tools/narration, tools/memcheck,
// tools/beadrefs, and this command's own failure message below -- so a stale one
// is a false claim the program makes to a user at the moment they are already
// dealing with a failure.
//
// It resolves against the committed AGENTS.md and nothing else: no bd, no Dolt,
// no node, no network. So it is a full gate in pre-commit and in CI both, unlike
// the memory arm of `make narration`, which warns and continues whenever bd
// cannot answer.
//
// A CITATION IS NOT A LINE. Three of the ones in the tree wrap across two lines
// -- two in Go comments and one in a docs/ blockquote -- so the scan joins the
// file and normalises markers out of the captured title rather than matching
// line by line. See titleOf.
//
// ONLY THE CANONICAL FORM IS RESOLVED. A looser net catches the anchor followed
// within a bounded run by a quoted title, and every hit the canonical pattern
// did not claim is REFUSED AS MALFORMED rather than interpreted -- resolving
// free prose means guessing, and a detector that guesses refuses true sentences
// and teaches people to reword around them. A citation carrying no quoted title
// at all is OUT OF REACH OF BOTH PATTERNS: prose that names a section without
// quoting it passes this gate in silence, and that gap is declared here rather
// than papered over.
//
// A DOCS/ PATH IS A CLAIM ABOUT THE TREE exactly as a section name is, so every
// one cited in the scanned files must resolve. That is the narrow half of the
// evidence-doc contract and deliberately no more of it: the declared map, the
// opening markers and the backlinks belong to fisc-ak39.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// citePattern finds a citation of AGENTS.md that names a section in quotes.
//
// The separators are the three the tree actually uses: a comma, a possessive,
// and a comma followed by the word "under". Anything between the anchor and the
// opening quote may be comment furniture, because a citation is allowed to wrap:
// the run in the middle absorbs newlines, `//`, `#` and leading `*`.
//
// The opening and closing quotes are optionally backslashed, because some
// citations are written with escaped quotes -- in Go string literals, and in a
// shell echo in the Makefile.
//
// ITS GAP CLASS MUST MATCH furniture's. They are two halves of one rule: this
// one decides whether a wrapped citation is FOUND, and furniture decides how its
// title is normalised once it is. A marker missing here is a dead citation that
// passes the gate in silence, which is the worse direction of the two -- and `>`
// was missing here after being added there.
var citePattern = regexp.MustCompile(`AGENTS\.md(?:'s|,)((?:[\s*>]|//|#)*(?:under)?(?:[\s*>]|//|#)*)\\?"([^"]*)"`)

// furniture is what a wrapped citation picks up between its words: a newline,
// the marker that opens the next line, and the indentation around it. The
// markers are per-language and `>` is one of them, because a citation inside a
// markdown blockquote -- which is how every docs/ evidence file names the
// section it belongs to -- wraps with a `>` at the head of the next line.
var furniture = regexp.MustCompile(`(?:[\s]|//|#|\*|>)+`)

// loosePattern is the malformed-citation net: the anchor, a bounded run of
// anything but a quote, then a quoted run that opens the way a section title
// does. citePattern decides which of its hits are canonical, and every hit left
// over is refused. Nothing here resolves a title.
//
// THE GAP MAY CROSS A NEWLINE ONLY INTO CITATION FURNITURE, OR STRAIGHT INTO
// THE OPENING QUOTE. A citation is allowed to wrap, so a gap that refused every
// newline would let a wrapped near-miss escape the net -- but one that crosses
// into an arbitrary next line walks off the anchor's own sentence and into the
// following statement, where a string literal turns true prose into a finding.
// This command is a required gate, and a detector that refuses true sentences
// teaches people to reword around them. The markers are the set furniture
// declares -- the gap classes here and there are halves of one rule, as with
// citePattern's -- and the newline straight into the quote is written out as
// its own optional branch, because Go's regexp has no lookahead to assert it
// with.
//
// Both bounds are load-bearing, and neither is a measurement of the tree. The
// gap is short enough that this file's own comments, which mention the anchor
// and quote things only at a distance, stay out of the net; the quoted run must
// OPEN like a title so that a list of quoted string literals with the anchor's
// file name among them is not a hit -- the run such a list puts after the
// anchor opens with a comma.
var loosePattern = regexp.MustCompile("AGENTS\\.md(?:[^\"\\n]|\\n[ \t]*(?://|#|\\*|>)+[ \t]*){0,40}(?:\\n[ \t]*)?\\\\?\"[A-Za-z0-9`][^\"]{0,79}\"")

// docPathPattern finds a docs/ markdown path cited as a root-relative claim
// about this tree. The character before the anchor is part of the rule rather
// than trivia: a slash or a word character in front means the path is the tail
// of something longer -- a URL naming some other repository's docs directory --
// and a claim about a different tree is not this gate's to check. Group 2 is
// the path.
var docPathPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])(docs/[A-Za-z0-9._/-]+\.md)`)

// headingPattern and boldPattern are the two shapes an anchor takes in AGENTS.md.
// Bold is an anchor and not only a decoration because the tree cites one:
// "History's home is git" is a bolded lead phrase inside a section, not a
// heading, and four separate places cite it as though it were a section name.
var (
	headingPattern = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.*\S)[ \t]*$`)
	boldPattern    = regexp.MustCompile(`(?sm)^[ \t]*(?:(?:[-*+]|[0-9]+\.)[ \t]+)?\*\*(.+?)\*\*`)
)

// exempt is the files whose citations are not claims about the tree. It is one
// entry and should stay near one: main_test.go's fixtures are synthetic
// citations, some of which must NOT resolve, so scanning them means a rename
// reports the fixtures beside the real orphan and a negative-path test cannot be
// written at all.
//
// Its own main.go is deliberately NOT here. The only citation in that file is the
// one the failure message prints, and that one is worth checking.
var exempt = map[string]string{
	"tools/doccheck/main_test.go": "its fixtures are citations and paths that must not resolve",
}

// checkExemptions refuses a declaration that has outlived the file it exempts,
// which is otherwise a hole nobody can see.
func checkExemptions(root string) error {
	for file, reason := range exempt {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil { // #nosec G703 -- root is the directory of this command's AGENTS.md argument: it checks the tree it is asked to.
			return fmt.Errorf("the exemption for %s (%q) names a file that is not there: %w", file, reason, err)
		}
	}
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }

// treeRoot is the directory every root-relative claim resolves against: the one
// holding the AGENTS.md argument, which the usage line makes the tree root by
// construction. The process's working directory is no part of the interface --
// the same arguments check the same tree from anywhere -- and a bare "AGENTS.md"
// argument yields ".", so the Makefile's invocation keeps meaning what it meant.
func treeRoot(agents string) string { return filepath.Dir(agents) }

// run carries main's whole behaviour behind a testable seam: args is everything
// after the command name, and the exit code is returned rather than taken.
func run(args []string, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: doccheck AGENTS.md path...")
		return 2
	}
	agents := args[0]
	root := treeRoot(agents)
	if err := checkExemptions(root); err != nil {
		fmt.Fprintf(stderr, "doccheck: %v\n", err)
		return 2
	}
	anchors, err := anchorsIn(agents)
	if err != nil {
		fmt.Fprintf(stderr, "doccheck: %v\n", err)
		return 2
	}
	// An AGENTS.md with no anchors would pass every citation in the tree, so the
	// one shape that must never be silent is the one where the substrate failed
	// to parse.
	if len(anchors) == 0 {
		fmt.Fprintf(stderr, "doccheck: %s yielded no headings at all, so every citation would resolve\n", agents)
		return 2
	}
	cites, malformed, docRefs, err := scanUnder(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "doccheck: %v\n", err)
		return 2
	}
	// A SCAN THAT FINDS NO CITATION AT ALL IS NOT A GREEN RUN, it is a broken
	// pattern. citePattern has already lost a marker class once -- `>` was added
	// to furniture and not to it -- and that failure is invisible from the exit
	// code, because a command that matches nothing reports nothing dead. The
	// committed tree has never carried zero, so zero means the matcher stopped
	// working rather than that the tree got clean.
	//
	// IT IS A FLOOR AND NOT A COVERAGE CHECK. It cannot see a PARTIAL loss --
	// dropping one marker class from citePattern would stop it reading docs/ and
	// site/app.js while the Go citations still counted, and the run would exit 0.
	// Exempting this command's own fixtures at least stops those standing in for
	// a tree that is no longer being read.
	if len(cites) == 0 {
		fmt.Fprintf(stderr, "doccheck: no citation of %s found anywhere in the scanned paths\n", agents)
		fmt.Fprintln(stderr, "  The tree has always carried some, so this is citePattern failing to")
		fmt.Fprintln(stderr, "  match rather than a tree with nothing to check.")
		return 2
	}
	// The same floor for the same reason: the root files have always cited
	// docs/ by path, so an empty sweep is docPathPattern failing to match
	// rather than a tree with no claims to check.
	if len(docRefs) == 0 {
		fmt.Fprintln(stderr, "doccheck: no docs/ path found anywhere in the scanned paths")
		fmt.Fprintln(stderr, "  The tree has always carried some, so this is docPathPattern failing")
		fmt.Fprintln(stderr, "  to match rather than a tree with nothing to check.")
		return 2
	}
	var dead []cite
	for _, c := range cites {
		if !anchors[fold(c.title)] {
			dead = append(dead, c)
		}
	}
	missing := missingDocs(root, docRefs)
	fail := false
	if len(dead) > 0 {
		fail = true
		sortCites(dead)
		for _, c := range dead {
			fmt.Fprintf(stderr, "%s:%d: %q names no section of %s\n", c.file, c.line, c.title, agents)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "doccheck: the citations above point at nothing.")
		fmt.Fprintln(stderr, "  A citation that names no section reads as though the rule is")
		fmt.Fprintln(stderr, "  written down, so the reader goes looking for a heading that is")
		fmt.Fprintln(stderr, "  gone. Re-point it at the section that carries the rule now, or")
		fmt.Fprintln(stderr, "  drop it. If you renamed a section, grep for its other citations")
		fmt.Fprintln(stderr, "  in the same commit.")
		fmt.Fprintln(stderr, "  See AGENTS.md, \"Before you quote a number\".")
	}
	if len(malformed) > 0 {
		fail = true
		sortCites(malformed)
		for _, c := range malformed {
			fmt.Fprintf(stderr, "%s:%d: %q is not the canonical citation form\n", c.file, c.line, c.title)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintf(stderr, "doccheck: the citations above cannot be resolved against %s.\n", agents)
		fmt.Fprintln(stderr, "  Only the canonical form is resolved against the section anchors,")
		fmt.Fprintln(stderr, "  so any other shape would survive a section rename in silence.")
		fmt.Fprintln(stderr, "  Rewrite each as the file name, then a comma or a possessive, then")
		fmt.Fprintf(stderr, "  the section title in quotes: %s\n", fmt.Sprintf("%s, %q", agents, "<section title>"))
	}
	if len(missing) > 0 {
		fail = true
		sortCites(missing)
		// A markdown link writes its path twice on one line -- once as the
		// visible text, once as the target -- and one report line is enough.
		var prev cite
		for _, c := range missing {
			if c == prev {
				continue
			}
			prev = c
			fmt.Fprintf(stderr, "%s:%d: cites %s, which is not in the tree\n", c.file, c.line, c.title)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "doccheck: the docs/ paths above resolve to no file.")
		fmt.Fprintln(stderr, "  A path cited from a comment or a doc is a claim about the tree,")
		fmt.Fprintln(stderr, "  exactly as a section name is. Re-point it at the file that carries")
		fmt.Fprintln(stderr, "  the text now, or drop it. If you renamed a docs/ file, grep for")
		fmt.Fprintln(stderr, "  its other citations in the same commit.")
	}
	if fail {
		return 1
	}
	return 0
}

// sortCites orders findings for a stable report: by file, then line, then
// title, so that equal findings sit together for the dedupe above.
func sortCites(cs []cite) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].file != cs[j].file {
			return cs[i].file < cs[j].file
		}
		if cs[i].line != cs[j].line {
			return cs[i].line < cs[j].line
		}
		return cs[i].title < cs[j].title
	})
}

// cite is one citation: which section it names, and where it was written.
type cite struct {
	title string
	file  string
	line  int
}

// anchorsIn collects every name a citation may legitimately use, folded for
// comparison.
//
// A heading that carries a colon offers TWO names, and the tree uses the second
// one: `### The failure mode to look for: green because the gate fired` is cited
// as "green because the gate fired". Refusing that would be refusing a citation
// that names its section perfectly well.
func anchorsIn(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- the path is this command's argument, which is the whole of its interface: it reads the file it is asked to.
	if err != nil {
		return nil, err
	}
	text := string(b)
	anchors := map[string]bool{}
	for _, m := range headingPattern.FindAllStringSubmatch(text, -1) {
		anchors[fold(m[1])] = true
		if _, after, found := strings.Cut(m[1], ": "); found {
			anchors[fold(after)] = true
		}
	}
	// BOLD IS PAIRED OVER THE WHOLE FILE, LAZILY, AND MUST BE ABLE TO CROSS A
	// LONE ASTERISK. Two failures got here in two commits, and the second was
	// caused by the fix for the first.
	//
	// Line by line, the `**` CLOSING a run opened on the previous line pairs with
	// the `**` OPENING the next, which loses every wrapped anchor and invents one
	// from the prose between two runs.
	//
	// Pairing over the whole file fixed that and introduced worse: with `[^*]+`
	// as the body, the lone `*` in **... `byob-*` bead** desynchronised every
	// pair after it. Measured on the committed file at the time, 14 spans of
	// ordinary prose became anchors and four real lead phrases vanished, among
	// them BOTH rules that have to sit above the generated block. THAT FIGURE
	// WAS TAKEN BEFORE THE `^` ANCHOR BELOW; with it, the same mutation loses
	// one anchor and invents none, because a desynchronised run rarely starts a
	// line. The body still has to cross a lone `*`, which is what the test pins.
	//
	// ONLY A LEAD PHRASE COUNTS -- bold opening a line, a bullet or a NUMBERED
	// item. Accepting bold anywhere made one-word inline emphasis into anchors,
	// so "not", "range" and "message" resolved and a citation naming no section
	// of this file passed the gate; that is the fail-open direction, and a lead
	// phrase is the shape a cited rule actually takes.
	//
	// The ordered-list arm is not decoration: leaving it out took the generated
	// block's seven numbered rules out of the anchor set, so a citation of
	// "Push to remote" would have been reported dead.
	for _, m := range boldPattern.FindAllStringSubmatch(text, -1) {
		anchors[fold(m[1])] = true
	}
	return anchors, nil
}

// fold puts a title in the form both sides are compared in: collapsed
// whitespace, no surrounding quotes or trailing sentence punctuation, lowercase.
//
// Trailing punctuation has to go because a bolded lead phrase carries the
// sentence's full stop INSIDE the bold -- `**History's home is git.**` -- while
// the citations of it do not.
func fold(s string) string {
	s = furniture.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `.,:;"`)
	return strings.ToLower(strings.TrimSpace(s))
}

// scanUnder collects every citation, malformed citation and docs/ path in the
// named files and directories. As in beadrefs, a path that cannot be READ is an
// error rather than a skip, so an entry that has gone from the tree takes this
// red.
//
// A path DELETED FROM THE MAKEFILE'S LIST is a different thing and this cannot
// see it: the path is simply never walked, and the scan gets quietly smaller.
// Neither that list nor scannable can know about an entry nobody added.
func scanUnder(paths []string) (cites, malformed, docRefs []cite, err error) {
	for _, p := range paths {
		// #nosec G703 -- the path list is this command's argument; it checks the
		// paths it is asked to.
		err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if norm(path) != norm(p) && !scannable(path) {
				return nil
			}
			if _, ok := exempt[norm(path)]; ok {
				return nil
			}
			found, err := citesIn(path)
			if err != nil {
				return err
			}
			cites = append(cites, found...)
			bad, err := malformedIn(path)
			if err != nil {
				return err
			}
			malformed = append(malformed, bad...)
			refs, err := docRefsIn(path)
			if err != nil {
				return err
			}
			docRefs = append(docRefs, refs...)
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return cites, malformed, docRefs, nil
}

// citesIn reads the whole file rather than scanning it line by line, because a
// citation may wrap across two comment lines and a line-wise scan sees only the
// half that carries the opening quote.
func citesIn(path string) ([]cite, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see scanUnder.
	if err != nil {
		return nil, err
	}
	text := string(b)
	var cites []cite
	for _, loc := range citePattern.FindAllStringSubmatchIndex(text, -1) {
		title := titleOf(text[loc[4]:loc[5]])
		if title == "" {
			continue
		}
		cites = append(cites, cite{
			title: title,
			file:  norm(path),
			line:  1 + strings.Count(text[:loc[0]], "\n"),
		})
	}
	return cites, nil
}

// malformedIn reports the loosePattern hits that citePattern did not claim. A
// loose hit is claimed when a canonical match STARTS ANYWHERE INSIDE ITS SPAN,
// not only at its own anchor: a passing mention of the anchor file shortly
// before a canonical citation would otherwise capture that citation's quotes
// and report prose that is fine. The mention itself carries no quoted title,
// which is the class this gate declares out of reach.
func malformedIn(path string) ([]cite, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see scanUnder.
	if err != nil {
		return nil, err
	}
	text := string(b)
	var claimed []int
	for _, loc := range citePattern.FindAllStringIndex(text, -1) {
		claimed = append(claimed, loc[0])
	}
	inSpan := func(start, end int) bool {
		for _, p := range claimed {
			if start <= p && p < end {
				return true
			}
		}
		return false
	}
	var out []cite
	for _, loc := range loosePattern.FindAllStringIndex(text, -1) {
		if inSpan(loc[0], loc[1]) {
			continue
		}
		out = append(out, cite{
			title: strings.TrimSpace(furniture.ReplaceAllString(text[loc[0]:loc[1]], " ")),
			file:  norm(path),
			line:  1 + strings.Count(text[:loc[0]], "\n"),
		})
	}
	return out, nil
}

// docRefsIn collects every docs/ path the file cites. Group 2 of the pattern is
// the path; group 1 is the guard character and is no part of the claim.
func docRefsIn(path string) ([]cite, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see scanUnder.
	if err != nil {
		return nil, err
	}
	text := string(b)
	var out []cite
	for _, loc := range docPathPattern.FindAllStringSubmatchIndex(text, -1) {
		out = append(out, cite{
			title: text[loc[4]:loc[5]],
			file:  norm(path),
			line:  1 + strings.Count(text[:loc[4]], "\n"),
		})
	}
	return out, nil
}

// missingDocs returns the cited paths that resolve to nothing under root. A
// path that exists is not examined further: its opening marker and its backlink
// belong to the wider contract this arm deliberately leaves alone.
func missingDocs(root string, refs []cite) []cite {
	var missing []cite
	for _, r := range refs {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(r.title))); err != nil { // #nosec G703 -- see checkExemptions: root comes from this command's own argument.
			missing = append(missing, r)
		}
	}
	return missing
}

// titleOf normalises a captured title. A wrapped citation carries the next
// line's comment marker and indentation inside the quotes, and one written in a
// Go string literal carries the backslash of its own closing escape.
func titleOf(raw string) string {
	raw = strings.TrimSuffix(raw, `\`)
	return strings.TrimSpace(furniture.ReplaceAllString(raw, " "))
}

func norm(path string) string { return filepath.ToSlash(filepath.Clean(path)) }

// scannable says whether a file found by walking a directory can carry a
// citation. It is deliberately the same list beadrefs uses, minus the data
// formats that cannot carry prose about this file: a citation is a sentence,
// and .json here is extracted geometry.
func scannable(path string) bool {
	switch filepath.Ext(path) {
	case ".md", ".go", ".mjs", ".js", ".tmpl", ".css", ".py", ".yaml", ".yml":
		return true
	}
	return false
}
