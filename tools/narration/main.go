// Command narration refuses history in Go source comments: a review credit,
// and an erratum about a comment's own former text. AGENTS.md's "History's
// home is git" is the rule; this is that rule as something that can go red.
//
// THE COMMENT BLOCK IS THE UNIT, NOT THE LINE. A refused phrase is several
// words at roughly 77 columns, so it wraps, and the wrap point is chosen by
// whoever gofmt'd the comment rather than by whoever wrote the history. A
// line-based pattern cannot see a phrase split across two lines, and cannot
// see a bare interior line of a /* */ block at all, which carries no marker
// to hold on to. The sibling arm over the memories, tools/memcheck, matches
// each phrase with \s+ between its words for the same reason -- and only a
// scan over the joined block can honour that in a file of marked-up lines.
//
// A block is a run of comments with no code token and no blank line between
// them, and joining any wider would be the defect in the other direction: the
// last words of one comment and the first words of the next, adjacent across
// a blank line or a line of code, must never add up to a finding neither
// comment contains.
//
// NEITHER REFUSED PHRASE APPEARS CONTIGUOUSLY IN THIS FILE'S COMMENTS, for
// the reason tools/doccheck's package comment spells no citation out: the
// examples this comment wants are exactly what the patterns match. The
// pattern strings below and the fixtures in main_test.go carry the phrases
// only inside string literals, and a string literal is not a comment --
// go/scanner is what guarantees that, which is also why a // inside a string
// can never read as a comment marker here.
package main

import (
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// refused matches over a comment block's joined text. Every gap between words
// is \s+ rather than a space, so a phrase keeps matching wherever the block
// happens to wrap. Case-insensitive because the erratum form opens sentences,
// where its first word arrives capitalised.
var refused = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Found\s+by\s+/code-review`),
	regexp.MustCompile(`(?i)this\s+comment\s+used\s+to\s+say`),
}

// finding is one refused phrase: where it starts, and what matched.
type finding struct {
	file   string
	line   int
	phrase string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: narration path...")
		os.Exit(2)
	}
	findings, scanned, err := scanPaths(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "narration: %v\n", err)
		os.Exit(2)
	}
	if scanned == 0 {
		fmt.Fprintln(os.Stderr, "narration: no .go files under the named paths, so a green run would mean nothing")
		os.Exit(2)
	}
	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "%s:%d: %s\n", f.file, f.line, f.phrase)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "narration: the comments above put HISTORY in a source comment.")
	fmt.Fprintln(os.Stderr, "  A review credit belongs in the commit body; an erratum belongs")
	fmt.Fprintln(os.Stderr, "  in git. Keep the rule the comment carries and delete the history")
	fmt.Fprintln(os.Stderr, "  that argued for it. See AGENTS.md, \"History's home is git\".")
	os.Exit(1)
}

// scanPaths walks the named files and directories for .go files. Anything
// unreadable is an error rather than a skip, because the path list is
// hand-maintained in the Makefile and an entry gone from the tree must take
// the gate red rather than silently narrow the scan.
func scanPaths(paths []string) ([]finding, int, error) {
	var findings []finding
	scanned := 0
	for _, p := range paths {
		// #nosec G703 -- the path list is this command's argument, which is
		// the whole of its interface: it scans the paths it is asked to.
		err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			found, err := scanFile(path)
			if err != nil {
				return err
			}
			scanned++
			findings = append(findings, found...)
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].file != findings[j].file {
			return findings[i].file < findings[j].file
		}
		return findings[i].line < findings[j].line
	})
	return findings, scanned, nil
}

// scanFile reads one file for scanSource.
func scanFile(path string) ([]finding, error) {
	// #nosec G304,G703 -- the path came off the walk over this command's
	// arguments, which are the whole of its interface.
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return scanSource(path, src)
}

// segment is one comment's contribution to a block: the line its text starts
// on, and that text with the markers stripped.
type segment struct {
	line int
	text string
}

// scanSource tokenises one file and matches the refused phrases over each
// comment block. go/scanner is the lexer so that the tokens BETWEEN comments
// are what tell a block's edge from a line that merely looks like one.
func scanSource(path string, src []byte) ([]finding, error) {
	fset := token.NewFileSet()
	file := fset.AddFile(path, fset.Base(), len(src))
	var errs scanner.ErrorList
	var s scanner.Scanner
	s.Init(file, src, func(pos token.Position, msg string) { errs.Add(pos, msg) }, scanner.ScanComments)

	var findings []finding
	var block []segment
	endLine := 0
	flush := func() {
		findings = append(findings, matchBlock(path, block)...)
		block = block[:0]
	}
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			// Any code token ends the block -- except the semicolon the
			// scanner inserts at a line break, whose literal is the newline
			// itself. One of those lands AFTER a comment trailing code on the
			// same line, and honouring it would cut that comment off from the
			// lines directly under it, which no non-comment LINE separates
			// from it. Every line of actual code still flushes, because its
			// tokens arrive before any inserted semicolon does.
			if tok != token.SEMICOLON || lit != "\n" {
				flush()
			}
			continue
		}
		line := file.Position(pos).Line
		if len(block) > 0 && line > endLine+1 {
			// A blank line between comments ends the block too; see the
			// package comment for what joining across one would cost.
			flush()
		}
		text := commentText(lit)
		block = append(block, segment{line: line, text: text})
		endLine = line + strings.Count(text, "\n")
	}
	flush()
	if errs.Len() > 0 {
		return nil, errs.Err()
	}
	return findings, nil
}

// commentText drops the comment markers and nothing else, so that \s+ in a
// pattern is what bridges the remaining line breaks and indentation.
func commentText(lit string) string {
	if strings.HasPrefix(lit, "//") {
		return lit[2:]
	}
	lit = strings.TrimPrefix(lit, "/*")
	return strings.TrimSuffix(lit, "*/")
}

// matchBlock joins one block's segments and reports each refused phrase at the
// line its match starts on -- for a wrapped phrase, where its first word sits
// rather than where the block does.
func matchBlock(path string, block []segment) []finding {
	if len(block) == 0 {
		return nil
	}
	var b strings.Builder
	starts := make([]int, len(block))
	for i, seg := range block {
		if i > 0 {
			b.WriteByte('\n')
		}
		starts[i] = b.Len()
		b.WriteString(seg.text)
	}
	joined := b.String()
	var findings []finding
	for _, pat := range refused {
		for _, loc := range pat.FindAllStringIndex(joined, -1) {
			findings = append(findings, finding{
				file:   path,
				line:   lineAt(block, starts, joined, loc[0]),
				phrase: collapse(joined[loc[0]:loc[1]]),
			})
		}
	}
	return findings
}

// lineAt maps an offset in the joined text back to a file line: the segment it
// falls in, plus any of that segment's own interior newlines before it.
func lineAt(block []segment, starts []int, joined string, off int) int {
	i := sort.Search(len(starts), func(i int) bool { return starts[i] > off }) - 1
	return block[i].line + strings.Count(joined[starts[i]:off], "\n")
}

// collapse renders a matched phrase on one report line; the match itself may
// span several.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
