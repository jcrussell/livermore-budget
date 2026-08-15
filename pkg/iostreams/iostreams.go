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

// IOStreams holds the three process streams and what is known about them.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	stdinIsTTY  bool
	stdoutIsTTY bool
	stderrIsTTY bool
}

// IsStdinTTY reports whether standard input is a terminal.
func (s *IOStreams) IsStdinTTY() bool { return s.stdinIsTTY }

// IsStdoutTTY reports whether standard output is a terminal.
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutIsTTY }

// IsStderrTTY reports whether standard error is a terminal.
func (s *IOStreams) IsStderrTTY() bool { return s.stderrIsTTY }

// System returns the IOStreams wired to the real process streams. It is the
// only function in the codebase that reads os.Stdin, os.Stdout, or os.Stderr,
// and it is called once, from main.
func System() *IOStreams {
	return &IOStreams{
		In:          os.Stdin,
		Out:         os.Stdout,
		ErrOut:      os.Stderr,
		stdinIsTTY:  isTerminal(os.Stdin),
		stdoutIsTTY: isTerminal(os.Stdout),
		stderrIsTTY: isTerminal(os.Stderr),
	}
}

// Test returns an IOStreams backed by buffers, along with those buffers, for
// use in tests. All three streams report false for their TTY flags, which is
// the correct default: a test is not a terminal.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	return &IOStreams{In: in, Out: out, ErrOut: errOut}, in, out, errOut
}

// isTerminal reports whether f refers to a character device. This is a
// stdlib-only substitute for a term/isatty dependency; it is accurate enough
// for deciding whether to colorize or prompt, which is all it is used for.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
