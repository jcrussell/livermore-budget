package cmdutil

import (
	"errors"
	"fmt"
)

// FlagError marks an error caused by bad flag input; the runner maps it to
// exit code 2.
type FlagError struct{ Err error }

func (e *FlagError) Error() string { return e.Err.Error() }
func (e *FlagError) Unwrap() error { return e.Err }

// FlagErrorf returns a FlagError wrapping a formatted message; a plain
// fmt.Errorf from a validation path would exit 1 instead of 2.
func FlagErrorf(format string, args ...any) error {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}

// ErrSilent reports failure that has already been explained to the user.
// The runner exits 1 without printing anything further.
var ErrSilent = errors.New("silent")

// ErrCancel reports that the user cancelled the operation.
var ErrCancel = errors.New("cancel")

// ExitCodeError carries an explicit exit code and prints nothing, for when
// the code itself is the result.
type ExitCodeError struct{ Code int }

func (e *ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.Code) }
