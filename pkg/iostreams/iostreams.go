// Package iostreams centralizes process input and output so that commands
// never touch os.Stdin, os.Stdout, or os.Stderr directly.
//
// The split between Out and ErrOut is a contract, not a style preference:
// Out carries the command's data — the bytes a user might pipe into jq or
// wc — and ErrOut carries everything else. If removing a write would change
// the meaning of piped output, it belongs on Out; otherwise it belongs on
// ErrOut.
package iostreams

import (
	"bytes"
	"io"
	"os"
)

// IOStreams holds the three process streams.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// System returns the IOStreams wired to the real process streams. It is the
// only function in the codebase that reads os.Stdin, os.Stdout, or os.Stderr,
// and it is called once, from main.
func System() *IOStreams {
	return &IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

// Test returns an IOStreams backed by buffers, along with those buffers, for
// use in tests.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	return &IOStreams{In: in, Out: out, ErrOut: errOut}, in, out, errOut
}
