package english_test

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/english"
)

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "checks", 1: "check", 2: "checks"} {
		if got := english.Plural(n, "check", "checks"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}
