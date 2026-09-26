package check

import (
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

// SpendingGap is one column's licence for a gap: the column, the signed cents
// the spine carries over pp.85-125's division rows in it, and the reason, as a
// sentence a reader of the mark meets.
type SpendingGap struct {
	FiscalYear int
	Basis      string
	Cents      int64
	Reason     string
}

// SpendingGaps is the declared gap between the spine's object-category cells and
// what pp.85-125's division rows come to, as the site's drill seam holds it: the
// tier-5 node id, and one licence per column the two documents differ in.
//
// DERIVED FROM structure.BudgetBookExceptions AND NOT WRITTEN AGAIN. The
// exceptions the departmentwide cut declares against the spine are the
// declaration, and cuts-tie-along-the-lattice holds both sides of each to the
// facts on every run; this is the same set spelled for a consumer whose
// vocabulary is node ids rather than cells.
func SpendingGaps() map[string][]SpendingGap {
	out := map[string][]SpendingGap{}
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != departmentwideCut || e.Against != spineCut || e.At != structure.LevelCategory {
			continue
		}
		for _, p := range e.Cells {
			id := spendingCategoryNode(p.Coords[structure.AxisCategory])
			out[id] = append(out[id], SpendingGap{
				FiscalYear: p.Year, Basis: p.Basis,
				Cents:  p.Against.Cents - p.Cut.Cents,
				Reason: e.Reason + ".",
			})
		}
	}
	return out
}

// spendingCategoryNode is the tier-5 node id an object category is drawn at.
func spendingCategoryNode(category string) string { return spendingObjectPrefix + category }
