// Command doccheck refuses a pointer at nothing: a citation naming no section
// of AGENTS.md, a citation of it written outside the canonical form, a docs/
// or source path that resolves to no file, a Go test name no _test.go
// declares, and a schema reference naming no node.
//
// THIS COMMENT SPELLS NO CITATION OUT: an example would be exactly the shape
// citePattern matches, and the command would report itself.
//
// It resolves against the committed tree only (no bd, no Dolt, no network), so
// it is a full gate in pre-commit and CI. A citation may wrap across lines, so
// files are scanned whole; see titleOf. A non-canonical hit is refused as
// malformed only when its quoted run resolves against the section anchors.
// Out of reach: prose naming a section without quoting it, a malformed
// citation of a renamed-away section, and a near-misspelling of a title.
// The rest of the evidence-doc contract belongs to fisc-ak39.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// citePattern finds a citation of AGENTS.md that names a section in quotes:
// a comma, a possessive, or a comma and "under", with comment furniture
// allowed in the gap and the quotes optionally backslashed (Go literals, the
// Makefile's echo). ITS GAP CLASS MUST MATCH furniture's: a marker missing
// here is a wrapped citation never found, which passes in silence.
var citePattern = regexp.MustCompile(`AGENTS\.md(?:'s|,)((?:[\s*>]|//|#)*(?:under)?(?:[\s*>]|//|#)*)\\?"([^"]*)"`)

// furniture is what a wrapped citation picks up between its words: newline,
// next-line marker and indentation. `>` is one because docs/ evidence files
// cite from a markdown blockquote.
var furniture = regexp.MustCompile(`(?:[\s]|//|#|\*|>)+`)

// loosePattern is the malformed-citation net: the anchor, a gap, then the
// first quoted run after it, which must open like a title (group 1).
// malformedIn refuses only hits citePattern did not claim whose run RESOLVES
// against the section anchors, so prose quoting a non-title is out by
// construction. The gap crosses a newline only into furniture's markers, never
// a sentence end (a full stop then whitespace), and has no length bound.
// Declared escapes: a separator containing a full sentence, one ending in ?
// or !, and a quote opening a bare next line with no marker.
var loosePattern = regexp.MustCompile("AGENTS\\.md(?:[^\".\\n]|\\.[^\" \t\n]|\\n[ \t]*(?://|#|\\*|>)+[ \t]*)*\\\\?\"([A-Za-z0-9`][^\"]{0,79})\"")

// docPathPattern finds a docs/ markdown path, bare or behind ./ and ../ (a
// relative markdown link). The guard before the path excludes a URL tail
// naming another repository's docs. Group 2 is the path, prefix included;
// missingDocs decides what a prefixed one resolves against.
var docPathPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])((?:\.\.?/)*docs/[A-Za-z0-9._/-]+\.md)`)

// srcPathPattern finds a source path, with docPathPattern's guard. Only four
// directories and two extensions: other repo-shaped paths (data/, dist/,
// facts/, testdata/, site/*.html) are false positives.
var srcPathPattern = regexp.MustCompile(
	`(^|[^A-Za-z0-9_./-])((?:internal|pkg|cmd|tools)/[A-Za-z0-9._/-]+\.(?:go|mjs))`)

// testNamePattern finds a Go test cited by name, and testDeclPattern is what it
// resolves against. A wrapped name is tried both unwrapped and joined; see
// unwrap.
var (
	testNamePattern = regexp.MustCompile(`(^|[^A-Za-z0-9_])(Test[A-Z][A-Za-z0-9_]*)`)
	testDeclPattern = regexp.MustCompile(`(?m)^func (Test[A-Z][A-Za-z0-9_]*)\(`)
)

// unwrap rejoins an identifier a comment broke across two lines, but only
// between two identifier characters; without that flank condition it would
// glue ordinary prose into names. It still over-joins a name followed by a
// capitalised word, which is why the unwrapped form is tried as well.
var unwrap = regexp.MustCompile(`([A-Za-z0-9_])-?\n[ \t]*(?://+|\*|#)?[ \t]*([A-Za-z0-9_])`)

// headingPattern and boldPattern are the two shapes an anchor takes in AGENTS.md.
// Bold counts because the tree cites bolded lead phrases as section names.
var (
	headingPattern = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.*\S)[ \t]*$`)
	boldPattern    = regexp.MustCompile(`(?sm)^[ \t]*(?:(?:[-*+]|[0-9]+\.)[ \t]+)?\*\*(.+?)\*\*`)
)

// exempt is the files whose citations are not claims about the tree: test
// fixtures that must NOT resolve. This command's own main.go is NOT exempt,
// because its failure messages' citations are worth checking.
var exempt = map[string]string{
	"tools/doccheck/main_test.go": "its fixtures are citations and paths that must not resolve",
	"tools/beadrefs/main_test.go": "its fixtures name source paths that must NOT resolve, so a test for that command cannot be written out of real ones",
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
// holding the AGENTS.md argument. The working directory is no part of the
// interface.
func treeRoot(agents string) string { return filepath.Dir(agents) }

// run is main behind a testable seam, returning the exit code.
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
	// No anchors would pass every citation.
	if len(anchors) == 0 {
		fmt.Fprintf(stderr, "doccheck: %s yielded no headings at all, so every citation would resolve\n", agents)
		return 2
	}
	cites, malformed, docRefs, srcRefs, testRefs, declaredTests, err := scanUnder(root, args[1:], anchors)
	if err != nil {
		fmt.Fprintf(stderr, "doccheck: %v\n", err)
		return 2
	}
	// Zero citations means a broken pattern, not a clean tree. It is a floor,
	// not a coverage check: a partial loss (one marker class) still exits 0.
	if len(cites) == 0 {
		fmt.Fprintf(stderr, "doccheck: no citation of %s found anywhere in the scanned paths\n", agents)
		fmt.Fprintln(stderr, "  The tree has always carried some, so this is citePattern failing to")
		fmt.Fprintln(stderr, "  match rather than a tree with nothing to check.")
		return 2
	}
	// The same floor for docPathPattern.
	if len(docRefs) == 0 {
		fmt.Fprintln(stderr, "doccheck: no docs/ path found anywhere in the scanned paths")
		fmt.Fprintln(stderr, "  The tree has always carried some, so this is docPathPattern failing")
		fmt.Fprintln(stderr, "  to match rather than a tree with nothing to check.")
		return 2
	}
	// A scan with no cited test or no declaration would pass every citation.
	if len(testRefs) == 0 || len(declaredTests) == 0 {
		fmt.Fprintf(stderr, "doccheck: the scanned paths yielded %d cited test name(s) and %d declaration(s)\n",
			len(testRefs), len(declaredTests))
		fmt.Fprintln(stderr, "  The tree has always carried both, so this is testNamePattern or")
		fmt.Fprintln(stderr, "  testDeclPattern failing to match rather than a tree with nothing to check.")
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
		fmt.Fprintf(stderr, "doccheck: the citations above each name a section of %s\n", agents)
		fmt.Fprintln(stderr, "  outside the canonical form. Only the canonical form is resolved")
		fmt.Fprintln(stderr, "  against the section anchors, so a rename would orphan these in silence.")
		fmt.Fprintln(stderr, "  Rewrite each as the file name, then a comma or a possessive, then")
		fmt.Fprintf(stderr, "  the section title in quotes: %s\n", fmt.Sprintf("%s, %q", agents, "<section title>"))
	}
	if len(missing) > 0 {
		fail = true
		sortCites(missing)
		// A markdown link writes its path twice on one line; report it once.
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
	if deadSrc := missingDocs(root, srcRefs); len(deadSrc) > 0 {
		fail = true
		sortCites(deadSrc)
		var prev cite
		for _, c := range deadSrc {
			if c == prev {
				continue
			}
			prev = c
			fmt.Fprintf(stderr, "%s:%d: cites %s, which is not in the tree\n", c.file, c.line, c.title)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "doccheck: the source paths above resolve to no file.")
		fmt.Fprintln(stderr, "  THE FIX IS USUALLY TO DROP THE PATH, NOT TO CORRECT IT. A comment")
		fmt.Fprintln(stderr, "  names a symbol; it does not say where the symbol lives, because a")
		fmt.Fprintln(stderr, "  path is a second source nothing keeps in step -- which is how these")
		fmt.Fprintln(stderr, "  die. Name the function or the type and let the reader grep.")
		fmt.Fprintln(stderr, "  See AGENTS.md, \"Where writing goes\".")
	}
	if deadRefs := deadTests(testRefs, declaredTests); len(deadRefs) > 0 {
		fail = true
		sortCites(deadRefs)
		var prev cite
		for _, c := range deadRefs {
			if c == prev {
				continue
			}
			prev = c
			fmt.Fprintf(stderr, "%s:%d: cites %s, which no _test.go declares\n", c.file, c.line, c.title)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "doccheck: the test names above resolve to nothing.")
		fmt.Fprintln(stderr, "  A comment naming a test reads as coverage, so a dead one says the")
		fmt.Fprintln(stderr, "  claim beside it is guarded when nothing guards it -- which is worse")
		fmt.Fprintln(stderr, "  than saying nothing. Re-point it at the test that makes the claim")
		fmt.Fprintln(stderr, "  now, drop the sentence, or write the test.")
		fmt.Fprintln(stderr, "  See AGENTS.md, \"Prove it can fail\".")
	}
	deadSchema, err := deadSchemaRefs(root, args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "doccheck: %v\n", err)
		return 2
	}
	if len(deadSchema) > 0 {
		fail = true
		sortCites(deadSchema)
		for _, c := range deadSchema {
			fmt.Fprintf(stderr, "%s:%d: %s\n", c.file, c.line, c.title)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "doccheck: the schema references above resolve to nothing.")
		fmt.Fprintln(stderr, "  site/app.js's typedefs name the shape they stand for by its schema")
		fmt.Fprintln(stderr, "  path and state none of its fields, so a path the schema no longer has")
		fmt.Fprintln(stderr, "  is the one way one can drift. Re-point it at the shape's path now.")
		fmt.Fprintln(stderr, "  See AGENTS.md, \"Where writing goes\".")
	}
	if fail {
		return 1
	}
	return 0
}

// schemaRefPattern is a schema file cited by path, and optionally a JSON
// pointer into it after '#'.
var schemaRefPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])schema/([a-z-]+\.schema\.json)(#[A-Za-z0-9_$/.~-]*)?`)

// fiscTypedefName is the name after a typedef's type, which fiscTypedefs
// finds by brace depth because the type can span lines.
var fiscTypedefName = regexp.MustCompile(`^\s*(Fisc[A-Za-z]+)`)

// fiscTypedefs is every site/app.js typedef standing for a wire shape: its
// name and the offset of its "@typedef".
func fiscTypedefs(text string) (names []string, at []int) {
	const tag = "@typedef {"
	for i := 0; ; {
		j := strings.Index(text[i:], tag)
		if j < 0 {
			return names, at
		}
		start := i + j
		end, depth := start+len(tag), 1
		for ; end < len(text) && depth > 0; end++ {
			switch text[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if m := fiscTypedefName.FindStringSubmatch(text[end:]); m != nil {
			names, at = append(names, m[1]), append(at, start)
		}
		i = end
	}
}

// deadSchemaRefs is every schema/<file>.schema.json#<pointer> cited in the
// scanned paths that names no file or no node of it, and every Fisc* typedef
// in site/app.js whose doc comment names no schema path at all.
func deadSchemaRefs(root string, paths []string) ([]cite, error) {
	var out []cite
	for _, p := range paths {
		// #nosec G703 -- the path list is this command's argument; it checks the
		// paths it is asked to.
		err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if norm(path) != norm(p) && !scannable(path) {
				return nil
			}
			if _, ok := exempt[relTo(root, path)]; ok {
				return nil
			}
			raw, err := os.ReadFile(path) // #nosec G304 G122 -- a file of the tree this command is asked to check.
			if err != nil {
				return err
			}
			text := string(raw)
			rel := relTo(root, path)
			for _, m := range schemaRefPattern.FindAllStringSubmatchIndex(text, -1) {
				file, pointer := text[m[4]:m[5]], ""
				if m[6] >= 0 {
					// A pointer may hold '.', so a sentence's full stop is not
					// part of it.
					pointer = strings.TrimRight(text[m[6]+1:m[7]], ".")
				}
				line := strings.Count(text[:m[4]], "\n") + 1
				if msg := resolveSchemaRef(root, file, pointer); msg != "" {
					out = append(out, cite{title: msg, file: rel, line: line})
				}
			}
			// The shipped client modules; a test file carries no wire typedef.
			if strings.HasPrefix(norm(rel), "site/") && strings.HasSuffix(rel, ".js") {
				names, at := fiscTypedefs(text)
				for i, name := range names {
					start := strings.LastIndex(text[:at[i]], "/**")
					if start < 0 || !strings.Contains(text[start:at[i]], "schema/") {
						out = append(out, cite{title: name + " names no schema path, so nothing holds it to a shape",
							file: rel, line: strings.Count(text[:at[i]], "\n") + 1})
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resolveSchemaRef is "" when schema/file exists and the JSON pointer names a
// node of it, and otherwise what is missing.
func resolveSchemaRef(root, file, pointer string) string {
	raw, err := os.ReadFile(filepath.Join(root, "schema", file)) // #nosec G304 G703 -- a schema file the checked tree cites.
	if err != nil {
		return "schema/" + file + " is not a schema this tree has"
	}
	var at any
	if err := json.Unmarshal(raw, &at); err != nil {
		return "schema/" + file + " does not parse: " + err.Error()
	}
	for _, key := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if key == "" {
			continue
		}
		key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
		var ok bool
		switch node := at.(type) {
		case map[string]any:
			at, ok = node[key]
		case []any:
			i, bad := strconv.Atoi(key)
			if ok = bad == nil && i >= 0 && i < len(node); ok {
				at = node[i]
			}
		}
		if !ok {
			return "schema/" + file + "#" + pointer + " names no node of it"
		}
	}
	return ""
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
// comparison. A heading with a colon offers two names: the whole, and the part
// after the colon.
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
	// Bold is paired over the whole file, lazily, and must cross a lone `*`
	// (as in `byob-*`): line by line, a wrapped run is lost, and a `[^*]+` body
	// desynchronises every pair after the lone star. Only a lead phrase counts
	// (bold opening a line, a bullet or a numbered item): inline emphasis as an
	// anchor would let a citation of a single emphasised word pass, fail-open.
	for _, m := range boldPattern.FindAllStringSubmatch(text, -1) {
		anchors[fold(m[1])] = true
	}
	return anchors, nil
}

// fold puts a title in the form both sides are compared in: collapsed
// whitespace, no surrounding quotes or trailing sentence punctuation (a bold
// lead phrase carries its full stop inside the bold), lowercase.
func fold(s string) string {
	s = furniture.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `.,:;"`)
	return strings.ToLower(strings.TrimSpace(s))
}

// scanUnder collects every citation, malformed citation, path and test
// reference in the named files and directories, plus the declared tests. A path
// that cannot be read is an error, not a skip; a path dropped from the
// Makefile's list is invisible. root makes walked paths repo-relative for the
// exempt lookup.
func scanUnder(root string, paths []string, anchors map[string]bool) (cites, malformed, docRefs, srcRefs []cite, testRefs []testRef, declared map[string]bool, err error) {
	declared = map[string]bool{}
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
			if _, ok := exempt[relTo(root, path)]; ok {
				return nil
			}
			found, err := citesIn(path)
			if err != nil {
				return err
			}
			cites = append(cites, found...)
			bad, err := malformedIn(path, anchors)
			if err != nil {
				return err
			}
			malformed = append(malformed, bad...)
			refs, err := docRefsIn(path)
			if err != nil {
				return err
			}
			docRefs = append(docRefs, refs...)
			src, err := srcRefsIn(path)
			if err != nil {
				return err
			}
			srcRefs = append(srcRefs, src...)
			tests, err := testRefsIn(path)
			if err != nil {
				return err
			}
			testRefs = append(testRefs, tests...)
			// Declarations come from the same walk, so an unscanned
			// package's tests do not resolve citations.
			if strings.HasSuffix(norm(path), "_test.go") {
				decls, err := testDeclsIn(path)
				if err != nil {
					return err
				}
				for _, d := range decls {
					declared[d] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	return cites, malformed, docRefs, srcRefs, testRefs, declared, nil
}

// citesIn reads the whole file, because a citation may wrap across lines.
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

// malformedIn reports the loosePattern hits that citePattern did not claim and
// whose quoted title resolves against anchors. A loose hit is claimed when a
// canonical match starts anywhere inside its span, so a mention of the anchor
// file just before a canonical citation is not reported.
func malformedIn(path string, anchors map[string]bool) ([]cite, error) {
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
	for _, loc := range loosePattern.FindAllStringSubmatchIndex(text, -1) {
		if inSpan(loc[0], loc[1]) {
			continue
		}
		if !anchors[fold(titleOf(text[loc[2]:loc[3]]))] {
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

// docRefsIn collects every docs/ path the file cites.
func docRefsIn(path string) ([]cite, error) { return pathRefsIn(path, docPathPattern) }

// srcRefsIn collects every source path the file cites.
func srcRefsIn(path string) ([]cite, error) { return pathRefsIn(path, srcPathPattern) }

// pathRefsIn collects every path one pattern finds. Group 2 of each pattern is
// the path; group 1 is the guard character and is no part of the claim.
func pathRefsIn(path string, pattern *regexp.Regexp) ([]cite, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see scanUnder.
	if err != nil {
		return nil, err
	}
	text := string(b)
	var out []cite
	for _, loc := range pattern.FindAllStringSubmatchIndex(text, -1) {
		out = append(out, cite{
			title: text[loc[4]:loc[5]],
			file:  norm(path),
			line:  1 + strings.Count(text[:loc[4]], "\n"),
		})
	}
	return out, nil
}

// testRef is one cited Go test name, with the joined forms a wrap could have
// produced; it is alive when any of them is declared.
type testRef struct {
	cite
	alts []string
}

// testDeclsIn returns the test functions a _test.go file declares.
func testDeclsIn(path string) ([]string, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see scanUnder.
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range testDeclPattern.FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	return out, nil
}

// testRefsIn returns every Go test name cited in a file, each carrying the
// longer names unwrapping the file yields for it.
func testRefsIn(path string) ([]testRef, error) {
	b, err := os.ReadFile(path) // #nosec G304,G703 -- see scanUnder.
	if err != nil {
		return nil, err
	}
	text := string(b)
	var joined []string
	for _, m := range testNamePattern.FindAllStringSubmatch(unwrap.ReplaceAllString(text, "$1$2"), -1) {
		joined = append(joined, m[2])
	}
	var refs []testRef
	for _, loc := range testNamePattern.FindAllStringSubmatchIndex(text, -1) {
		name := text[loc[4]:loc[5]]
		var alts []string
		for _, j := range joined {
			if j != name && strings.HasPrefix(j, name) {
				alts = append(alts, j)
			}
		}
		refs = append(refs, testRef{
			cite: cite{title: name, file: norm(path), line: 1 + strings.Count(text[:loc[4]], "\n")},
			alts: alts,
		})
	}
	return refs, nil
}

// deadTests returns the cited names that no declaration answers, in either form.
func deadTests(refs []testRef, declared map[string]bool) []cite {
	var dead []cite
	for _, r := range refs {
		if declared[r.title] {
			continue
		}
		var alive bool
		for _, a := range r.alts {
			if declared[a] {
				alive = true
				break
			}
		}
		if !alive {
			dead = append(dead, r.cite)
		}
	}
	return dead
}

// missingDocs returns the cited paths that resolve to nothing. A bare path
// resolves under root; one opening with ./ or ../ resolves against the citing
// file's directory, as a reader following the link would.
func missingDocs(root string, refs []cite) []cite {
	var missing []cite
	for _, r := range refs {
		base := root
		if strings.HasPrefix(r.title, "./") || strings.HasPrefix(r.title, "../") {
			base = filepath.Dir(filepath.FromSlash(r.file))
		}
		if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(r.title))); err != nil { // #nosec G703 -- see checkExemptions: root and the walked files come from this command's own arguments.
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

// relTo puts a walked path in the repo-relative slash form exempt's keys are
// written in, whatever mix of relative and absolute the invocation used. On
// error it returns norm's form, which fails toward scanning an exempt file --
// reported, not hidden.
func relTo(root, path string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return norm(path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return norm(path)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return norm(path)
	}
	return filepath.ToSlash(rel)
}

// scannable says whether a file found by walking a directory can carry a
// citation: beadrefs' list minus .json, which here is extracted geometry.
func scannable(path string) bool {
	switch filepath.Ext(path) {
	case ".md", ".go", ".mjs", ".js", ".tmpl", ".css", ".py", ".yaml", ".yml":
		return true
	}
	return false
}
