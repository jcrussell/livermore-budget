package mapping

import "testing"

// TestAColumnPublishesOnItsOwnBasisOrTheRules is the one fallback every reader
// of a column's basis goes through: facts, the subtotal grid and the checks.
func TestAColumnPublishesOnItsOwnBasisOrTheRules(t *testing.T) {
	rule := &Rule{Basis: BasisAdopted}
	if got := (Column{}).EffectiveBasis(rule); got != BasisAdopted {
		t.Errorf("a column declaring no basis publishes on %q, want the rule's %q", got, BasisAdopted)
	}
	if got := (Column{Basis: BasisActual}).EffectiveBasis(rule); got != BasisActual {
		t.Errorf("a column declaring actual publishes on %q", got)
	}
}
