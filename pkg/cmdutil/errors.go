package cmdutil

import (
	"errors"
	"fmt"
)

// FlagError marks an error caused by bad flag input rather than a runtime
// failure. The runner maps it to exit code 2 so scripts can distinguish
// "you called me wrong" from "something went wrong".
type FlagError struct{ Err error }

func (e *FlagError) Error() string { return e.Err.Error() }
func (e *FlagError) Unwrap() error { return e.Err }

// FlagErrorf returns a FlagError wrapping a formatted message. Its whole
// value is the type it returns: a plain fmt.Errorf from a validation path is
// indistinguishable from a runtime error and would exit 1 instead of 2.
func FlagErrorf(format string, args ...any) error {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}

// ErrSilent reports failure that has already been explained to the user.
// The runner exits 1 without printing anything further.
var ErrSilent = errors.New("silent")

// ErrCancel reports that the user cancelled the operation.
var ErrCancel = errors.New("cancel")

// ExitCodeError carries an explicit exit code and prints nothing. Use it when
// the code itself is the result — a verify command reporting how many checks
// failed, say — rather than to signal an error condition.
type ExitCodeError struct{ Code int }

func (e *ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.Code) }
