package mapping

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestAColumnPublishesOnItsOwnBasisOrTheRules is the one fallback every reader
// of a column's basis goes through: facts, the subtotal grid and the checks.
func TestAColumnPublishesOnItsOwnBasisOrTheRules(t *testing.T) {
	rule := &Rule{Basis: vocab.BasisAdopted}
	if got := (Column{}).EffectiveBasis(rule); got != vocab.BasisAdopted {
		t.Errorf("a column declaring no basis publishes on %q, want the rule's %q", got, vocab.BasisAdopted)
	}
	if got := (Column{Basis: vocab.BasisActual}).EffectiveBasis(rule); got != vocab.BasisActual {
		t.Errorf("a column declaring actual publishes on %q", got)
	}
}
