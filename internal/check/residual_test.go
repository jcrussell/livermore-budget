package check

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestTheResidualSetIsTheEndpointSet pins the two declarations to each other:
// an endpoint sits outside the hierarchy, which is what a schedule of the
// hierarchy cannot decompose.
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
