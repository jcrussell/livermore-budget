// Command doccheck refuses a citation that names no section of AGENTS.md.
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
// only sitting in a comment -- the Makefile's narration arm, tools/memcheck,
// tools/beadrefs, and this command's own failure message below -- so a stale one
// is a false claim the program makes to a user at the moment they are already
// dealing with a failure.
//
// It resolves against the committed AGENTS.md and nothing else: no bd, no Dolt,
// no node, no network. So it is a full gate in pre-commit and in CI both, unlike
// the memory arm of `make narration`, which warns and continues whenever bd
// cannot answer.
//
// A CITATION IS NOT A LINE. Two of the ones in the tree today wrap across two
// comment lines, so the scan joins the file and normalises comment markers out
// of the captured title rather than matching line by line -- see titleOf.
package main

import (
	"fmt"
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
// The opening and closing quotes are optionally backslashed, because four of the
// citations are written with escaped quotes -- three in Go string literals and
// one in a shell echo in the Makefile.
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

// headingPattern and boldPattern are the two shapes an anchor takes in AGENTS.md.
// Bold is an anchor and not only a decoration because the tree cites one:
// "History's home is git" is a bolded lead phrase inside a section, not a
// heading, and four separate places cite it as though it were a section name.
var (
	headingPattern = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.*\S)[ \t]*$`)
	boldPattern    = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: doccheck AGENTS.md path...")
		os.Exit(2)
	}
	agents := os.Args[1]
	anchors, err := anchorsIn(agents)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doccheck: %v\n", err)
		os.Exit(2)
	}
	// An AGENTS.md with no anchors would pass every citation in the tree, so the
	// one shape that must never be silent is the one where the substrate failed
	// to parse.
	if len(anchors) == 0 {
		fmt.Fprintf(os.Stderr, "doccheck: %s yielded no headings at all, so every citation would resolve\n", agents)
		os.Exit(2)
	}
	cites, err := citesUnder(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "doccheck: %v\n", err)
		os.Exit(2)
	}
	// A SCAN THAT FINDS NO CITATION AT ALL IS NOT A GREEN RUN, it is a broken
	// pattern. citePattern has already lost a marker class once -- `>` was added
	// to furniture and not to it -- and that failure is invisible from the exit
	// code, because a command that matches nothing reports nothing dead. The
	// committed tree carries 27 over the Makefile's path list -- this command's
	// own files and docs/ included, because it does not exclude them -- so zero
	// means the matcher stopped working rather than that the tree got clean.
	if len(cites) == 0 {
		fmt.Fprintf(os.Stderr, "doccheck: no citation of %s found anywhere in the scanned paths\n", agents)
		fmt.Fprintln(os.Stderr, "  The tree has always carried some, so this is citePattern failing to")
		fmt.Fprintln(os.Stderr, "  match rather than a tree with nothing to check.")
		os.Exit(2)
	}
	var dead []cite
	for _, c := range cites {
		if !anchors[fold(c.title)] {
			dead = append(dead, c)
		}
	}
	if len(dead) == 0 {
		return
	}
	sort.Slice(dead, func(i, j int) bool {
		if dead[i].file != dead[j].file {
			return dead[i].file < dead[j].file
		}
		return dead[i].line < dead[j].line
	})
	for _, c := range dead {
		fmt.Fprintf(os.Stderr, "%s:%d: %q names no section of %s\n", c.file, c.line, c.title, agents)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "doccheck: the citations above point at nothing.")
	fmt.Fprintln(os.Stderr, "  A citation that names no section reads as though the rule is")
	fmt.Fprintln(os.Stderr, "  written down, so the reader goes looking for a heading that is")
	fmt.Fprintln(os.Stderr, "  gone. Re-point it at the section that carries the rule now, or")
	fmt.Fprintln(os.Stderr, "  drop it. If you renamed a section, grep for its other citations")
	fmt.Fprintln(os.Stderr, "  in the same commit.")
	fmt.Fprintln(os.Stderr, "  See AGENTS.md, \"Before you quote a number\".")
	os.Exit(1)
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
	// BOLD IS PAIRED OVER THE WHOLE FILE, NOT LINE BY LINE. A line-wise scan
	// pairs the `**` that CLOSES a bold run opened on the previous line with the
	// `**` that opens the next one, so it both loses every wrapped anchor and
	// invents one out of the prose lying between two runs -- measured on the
	// committed file, `("pp.85-125's 78 rows", "the corpus is 786 pages"), and`
	// was in the anchor set. Pairing from the start of the document is what
	// makes the delimiters line up.
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

// citesUnder collects every citation in the named files and directories. As in
// beadrefs, a path that cannot be READ is an error rather than a skip, so an
// entry that has gone from the tree takes this red.
//
// A path DELETED FROM THE MAKEFILE'S LIST is a different thing and this cannot
// see it: the path is simply never walked, and the scan gets quietly smaller.
// Neither that list nor scannable can know about an entry nobody added.
func citesUnder(paths []string) ([]cite, error) {
	var cites []cite
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
			found, err := citesIn(path)
			if err != nil {
				return err
			}
			cites = append(cites, found...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return cites, nil
}

// citesIn reads the whole file rather than scanning it line by line, because a
// citation may wrap across two comment lines and a line-wise scan sees only the
// half that carries the opening quote.
func citesIn(path string) ([]cite, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see citesUnder.
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
