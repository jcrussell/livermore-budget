package check

import (
	"maps"
	"regexp"
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

// TestAResidualReasonHoldsUnderEveryColumn is that the reasons are true under
// whichever year a reader is on. One reason is shown under every column, so a
// figure in it is one column's and wrong under the other -- the general
// group's transfer in is 480,400 in FY2026 and 486,735 in FY2027 -- and the
// ribbons beside the mark already carry the money.
func TestAResidualReasonHoldsUnderEveryColumn(t *testing.T) {
	figure := regexp.MustCompile(`\d{1,3}(,\d{3})+|FY ?\d{4}`)
	for id, reason := range residualNodes {
		if m := figure.FindString(reason); m != "" {
			t.Errorf("%s's reason quotes %q, which is one column's: %s", id, m, reason)
		}
	}
	// THE TRANSFERS ARE PRINTED PER FUND ON p.76, which the site draws; a
	// reason saying no fund-level schedule prints them is false beside that
	// chart.
	for _, id := range []string{"transfers/in", "transfers/out"} {
		if !strings.Contains(residualNodes[id], "p.76") {
			t.Errorf("%s's reason does not name p.76, which prints it per fund: %s", id, residualNodes[id])
		}
	}
}
