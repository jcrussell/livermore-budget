package check

import (
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// The spine's vocabulary for its right-hand column, spelled here rather than
// imported, which is this package's habit wherever the point is an independent
// second reading: internal/project composes it from its own constant, and a
// seam taking it from the producer would agree with it by construction.
const spendingObjectPrefix = "expenditure/"

// departmentwideCut is the cut name structure.BudgetBookCuts gives pp.85-125's
// upper block, which is how an exception names its pair.
// TestScopeConstantsNameDeclaredCuts holds it to the declaration.
const departmentwideCut = "departmentwide"

// SpendingGaps is the declared gap between the spine's object-category cells and
// what pp.85-125's division rows come to, as the site's drill seam holds it: the
// tier-5 node id, and the reason the two documents do not close.
//
// DERIVED FROM structure.BudgetBookExceptions AND NOT WRITTEN AGAIN. The
// exceptions the departmentwide cut declares against the spine are the
// declaration, and cuts-tie-along-the-lattice holds both sides of each to the
// facts on every run; this is the same set spelled for a consumer whose
// vocabulary is node ids rather than cells. Two spellings of a set that must
// agree drift; one declaration with two readers cannot.
//
// THE COLUMN IS IN THE SENTENCE AND NOT IN THE KEY, because the seam it feeds
// has no year axis: a drill step's residual is a map from node id to reason, and
// the same step is declared for every year the chart can open. An entry
// therefore names its own column in words, which is what a reader of the mark
// needs anyway -- the figure is right in one budget year and zero in the other,
// and a sentence that did not say which would be wrong half the time.
func SpendingGaps() map[string]string {
	out := map[string]string{}
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != departmentwideCut || e.Against != spineCut || e.At != structure.LevelCategory {
			continue
		}
		for _, p := range e.Cells {
			id := spendingCategoryNode(p.Coords[structure.AxisCategory])
			spine, detail := amount.Cents(p.Against.Cents), amount.Cents(p.Cut.Cents)
			reason := fmt.Sprintf("FY%d %s: the citywide spine publishes %s in this object "+
				"category and Budget Book pp.85-125's division rows come to %s, a difference of "+
				"%s. %s (%s)", p.Year, p.Basis, spine, detail, spine-detail, e.Reason, e.Bead)
			if prev, dup := out[id]; dup {
				// TWO COLUMNS' GAPS ON ONE NODE ARE BOTH NAMED, in declaration
				// order, because the key cannot tell them apart and a map that
				// kept the last one would publish whichever the slice happened
				// to end with. There is one entry today; the day there are two,
				// a reader sees both rather than a figure that is right for a
				// year they are not looking at.
				out[id] = prev + " " + reason
				continue
			}
			out[id] = reason
		}
	}
	return out
}

// spendingCategoryNode is the tier-5 node id an object category is drawn at.
func spendingCategoryNode(category string) string { return spendingObjectPrefix + category }
