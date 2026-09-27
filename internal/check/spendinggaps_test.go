package check

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestSpendingGapsIsTheSameDeclarationTheCheckReads: every cell of every
// departmentwide-against-spine exception reaches the map under its node id and
// column at the difference of its pins, with its reason, and nothing else does.
func TestSpendingGapsIsTheSameDeclarationTheCheckReads(t *testing.T) {
	got := SpendingGaps()
	if len(got) == 0 {
		t.Fatal("SpendingGaps is empty, so nothing below asserts anything")
	}
	want := map[string][]SpendingGap{}
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != departmentwideCut || e.Against != spineCut {
			continue
		}
		for _, p := range e.Cells {
			id := "expenditure/" + p.Coords[structure.AxisCategory]
			want[id] = append(want[id], SpendingGap{FiscalYear: p.Year, Basis: p.Basis,
				Cents: p.Against.Cents - p.Cut.Cents, Reason: e.Reason + "."})
		}
	}
	if len(want) == 0 {
		t.Fatal("no exception is declared on the departmentwide cut, so the map above is of nothing")
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SpendingGaps (-declared +exported):\n%s", diff)
	}
	// READER-FACING, so it carries no tracker id.
	for id, gaps := range got {
		for _, g := range gaps {
			if strings.Contains(g.Reason, "fisc-") {
				t.Errorf("SpendingGaps[%q]'s reason names a tracker id: %s", id, g.Reason)
			}
		}
	}
}
