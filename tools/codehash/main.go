// Command codehash fingerprints Go files by their code alone, so that a change
// asserted to be comments-only can be proved to be one.
//
// It prints "sha256  path" per named file, over the token stream with comments
// not scanned. Two versions of a file whose comments differ hash identically;
// a version whose code differs in any token does not.
//
// The obvious cheaper check -- read `git diff` and require every changed line
// to begin with // -- answers a different question. A diff reports which LINES
// differ, not whether any code moved, and it cannot see a token that changed
// while its line did not.
//
// It is also unsound line by line, because a comment line is not a
// self-contained unit in this tree: measured at 256f733, 20 comment lines
// across 9 files end with a backtick span still open, so the command they name
// is completed by the line below and the two cannot be judged apart. The count
// is pinned to a commit because nothing re-measures it.
//
// The token stream is used rather than a printed AST because go/printer decides
// blank lines from node positions, and those shift when a comment's length
// changes. Comments are absent from both inputs either way; only the token
// stream is also position-independent.
package main

import (
	"crypto/sha256"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: codehash file.go...")
		os.Exit(2)
	}
	status := 0
	for _, path := range os.Args[1:] {
		sum, err := hashFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codehash: %v\n", err)
			status = 1
			continue
		}
		fmt.Printf("%x  %s\n", sum, path)
	}
	os.Exit(status)
}

func hashFile(path string) ([]byte, error) {
	// #nosec G304,G703 -- the path is tainted because it comes from argv, which
	// is the whole of this command's interface: it fingerprints the files it is
	// asked to. A developer tool run by hand, off the build path and never
	// linked into fisc.
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return hashSource(path, src)
}

func hashSource(path string, src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	file := fset.AddFile(path, fset.Base(), len(src))

	var errs scanner.ErrorList
	var s scanner.Scanner
	// Mode 0: comments are not emitted at all, which is the whole point.
	s.Init(file, src, func(pos token.Position, msg string) { errs.Add(pos, msg) }, 0)

	h := sha256.New()
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		// The literal is hashed alongside the token so that a changed string,
		// number or identifier moves the sum. Without it every string literal
		// in the file would be interchangeable.
		fmt.Fprintf(h, "%s\x00%s\n", tok, lit)
	}
	if errs.Len() > 0 {
		return nil, errs.Err()
	}
	return h.Sum(nil), nil
}
