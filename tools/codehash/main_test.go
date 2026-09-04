package main

import (
	"bytes"
	"testing"
)

// TestCommentsDoNotMoveTheHash is the property the sweep depends on: a rewrite
// that changes only comments must fingerprint identically, or the check would
// fire on every unit and mean nothing.
func TestCommentsDoNotMoveTheHash(t *testing.T) {
	before := []byte(`package p

// Old is the old wording, which said several things at length.
// It goes on for a while.
func Old() string { return "kept" }
`)
	after := []byte(`package p

// Old is the new wording.
func Old() string { return "kept" }
`)

	a, err := hashSource("before.go", before)
	if err != nil {
		t.Fatalf("hash before: %v", err)
	}
	b, err := hashSource("after.go", after)
	if err != nil {
		t.Fatalf("hash after: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("comment-only change moved the hash:\n before %x\n after  %x", a, b)
	}
}

// TestAStringLiteralMovesTheHash is the mutation proof. It is written with the
// literal inside a raw string, spelled so that it reads as a comment line,
// because that is the case this command exists for: a textual "every changed
// line starts with //" filter over git diff cannot see this edit, and thirteen
// such lines are live in the tree.
//
// Deleting the `lit` argument from the Fprintf in hashSource makes this test
// fail while TestCommentsDoNotMoveTheHash still passes.
func TestAStringLiteralMovesTheHash(t *testing.T) {
	before := []byte("package p\n\nconst usage = `\n// verify\n`\n")
	after := []byte("package p\n\nconst usage = `\n// verifyy\n`\n")

	a, err := hashSource("before.go", before)
	if err != nil {
		t.Fatalf("hash before: %v", err)
	}
	b, err := hashSource("after.go", after)
	if err != nil {
		t.Fatalf("hash after: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Errorf("a changed string literal left the hash at %x; the edit would pass as comments-only", a)
	}
}

// TestAnUnscannableFileIsAnError keeps the check from failing open. A file the
// scanner cannot read must not hash to something that happens to match.
func TestAnUnscannableFileIsAnError(t *testing.T) {
	if _, err := hashSource("bad.go", []byte("package p\n\nconst s = \"unterminated\n")); err == nil {
		t.Error("hashSource returned no error for a file with an unterminated string")
	}
}
