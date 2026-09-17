package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestSpendingGapsIsTheSameDeclarationTheCheckReads is the seam the drill step
// reads, held against the declaration it is derived from.
//
// TWO SPELLINGS OF A SET THAT MUST AGREE DRIFT. This is the test that there is
// one: every cell of every exception the departmentwide cut declares against
// the spine reaches the exported map under the node id its category is drawn
// at, carrying both pinned figures, their difference and the bead -- and
// nothing else does.
func TestSpendingGapsIsTheSameDeclarationTheCheckReads(t *testing.T) {
	got := SpendingGaps()
	if len(got) == 0 {
		t.Fatal("SpendingGaps is empty, so nothing below asserts anything")
	}
	declared := 0
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != departmentwideCut || e.Against != spineCut {
			continue
		}
		for _, p := range e.Cells {
			declared++
			id := "expenditure/" + p.Coords[structure.AxisCategory]
			reason, ok := got[id]
			if !ok {
				t.Fatalf("SpendingGaps has no entry for %q, which is the node %s is drawn at",
					id, e.Key(p))
			}
			for _, want := range []string{
				e.Bead,
				amount.Cents(p.Against.Cents - p.Cut.Cents).String(),
				amount.Cents(p.Against.Cents).String(),
				amount.Cents(p.Cut.Cents).String(),
				e.Reason,
			} {
				if !strings.Contains(reason, want) {
					t.Errorf("SpendingGaps[%q] does not carry %q: %s", id, want, reason)
				}
			}
		}
	}
	if declared == 0 {
		t.Fatal("no exception is declared on the departmentwide cut, so the map above is of nothing")
	}
	if len(got) > declared {
		t.Errorf("SpendingGaps carries %d node(s) and the structure declares %d cell(s); the "+
			"extra ones come from no declaration", len(got), declared)
	}
	// A COPY, so no caller can widen the published set by writing to it.
	got["expenditure/grants"] = "invented"
	if _, ok := SpendingGaps()["expenditure/grants"]; ok {
		t.Error("SpendingGaps hands back a map a caller can extend")
	}
}
