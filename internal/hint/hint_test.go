package hint_test

import (
	"errors"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/hint"
)

func TestWithPassesNilThrough(t *testing.T) {
	if got := hint.With(nil, "never shown"); got != nil {
		t.Errorf("With(nil) = %v, want nil so `return hint.With(f(), ...)` needs no nil check", got)
	}
}

func TestWithKeepsTheCauseAndCarriesTheHint(t *testing.T) {
	cause := errors.New("no manifest")
	err := hint.Withf(cause, "run %s", "make extract")

	if err.Error() != cause.Error() {
		t.Errorf("Error() = %q, want the cause's %q with no hint folded in", err, cause)
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false, want the hint to unwrap to its cause")
	}
	var h *hint.ErrHint
	if !errors.As(err, &h) {
		t.Fatalf("got %T, want an *ErrHint the runner can find", err)
	}
	if h.Hint != "run make extract" {
		t.Errorf("Hint = %q, want the formatted remediation", h.Hint)
	}
}
