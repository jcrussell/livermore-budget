// Package hint attaches a remediation to an error at the point of failure, so
// the runner can print what the user should do next beneath what went wrong.
package hint

import "fmt"

// ErrHint pairs an error with a remediation suggestion. The runner prints the
// error and then the hint on its own line, so the hint is a place to put the
// command the user should run next.
type ErrHint struct {
	Err  error
	Hint string
}

func (e *ErrHint) Error() string { return e.Err.Error() }
func (e *ErrHint) Unwrap() error { return e.Err }

// With attaches a hint to err, passing nil through unchanged so callers can
// write `return hint.With(doThing(), "try --force")` without a nil check.
func With(err error, hint string) error {
	if err == nil {
		return nil
	}
	return &ErrHint{Err: err, Hint: hint}
}

// Withf attaches a formatted hint to err.
func Withf(err error, format string, args ...any) error {
	return With(err, fmt.Sprintf(format, args...))
}
