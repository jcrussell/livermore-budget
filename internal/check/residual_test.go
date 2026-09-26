package check

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestTheResidualSetIsTheEndpointSet pins the two declarations to each other:
// an endpoint is a flow outside the hierarchy, and that is exactly what a
// schedule of the hierarchy cannot decompose. A node in one table and not the
// other is a flow either drawn at a tier nothing declared or reconciled by
// nothing.
func TestTheResidualSetIsTheEndpointSet(t *testing.T) {
	want := slices.Sorted(maps.Keys(endpointTiers))
	got := slices.Sorted(maps.Keys(residualNodes))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("residualNodes and endpointTiers name different nodes (-endpoints +residual):\n%s", diff)
	}
	for id, reason := range residualNodes {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s is declared residual with no reason", id)
		}
	}
	// The published copy is a copy.
	published := ResidualNodes()
	published["coined/node"] = "widened by a caller"
	if _, leaked := residualNodes["coined/node"]; leaked {
		t.Error("ResidualNodes() handed out the package's own map")
	}
}
