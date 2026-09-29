package project

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// This file is where a cross-schedule licence the client carries -- a
// residual, a gap -- is spelled in node ids. Each is derived from
// internal/structure's declarations, which hold no node id, so the mapping
// from a kind or a cell to the node that draws it is this package's, and the
// prose a reader meets is declared beside it.

// spineEndpoint is one flow endpoint of the citywide spine: the node id, the
// kind of money it ends, and the category whose cell draws it.
type spineEndpoint struct {
	id       string
	kind     mapping.Kind
	category string
	tier     int
}

// spineEndpoints are the five nodes the spine draws outside its hierarchy,
// as sankey.Document draws them: a transfer's end at the category's own id,
// the reserve row at its own, and the two halves of the change in working
// capital. A test holds the list to the spine's golden.
var spineEndpoints = []spineEndpoint{
	{NodeTransfersIn, mapping.KindTransferIn, NodeTransfersIn, tierRevenueSource},
	{NodeTransfersOut, mapping.KindTransferOut, NodeTransfersOut, tierObjectCategory},
	{CategoryFundBalanceReserveIncrease, mapping.KindFundBalance, CategoryFundBalanceReserveIncrease, tierObjectCategory},
	{NodeFundBalanceDraw, mapping.KindFundBalance, CategoryFundBalanceChange, tierRevenueSource},
	{NodeFundBalanceContribution, mapping.KindFundBalance, CategoryFundBalanceChange, tierObjectCategory},
}

// EndpointCategory is the category every fact behind a spine flow endpoint
// carries, and whether id is one.
func EndpointCategory(id string) (string, bool) {
	for _, e := range spineEndpoints {
		if e.id == id {
			return e.category, true
		}
	}
	return "", false
}

// residualReasons is why a fund-level schedule cannot decompose each spine
// endpoint the drill-down's cuts leave undecomposed, in the words the client
// prints under the residual mark. The reasons quote no figure: one reason is
// shown under every column. Which endpoints ship is derived, not declared
// here: see FundFlowsResidual.
var residualReasons = map[string]string{
	NodeFundBalanceDraw: "a negative change in working capital, inferred from pp.66-67's " +
		"Change in Working Capital row and drawn into the group. pp.127-140 print no " +
		"fund-balance row at all, so no fund receives it",

	NodeFundBalanceContribution: "a positive change in working capital, inferred from " +
		"pp.66-67's Change in Working Capital row and drawn out of the group. pp.127-140 " +
		"print no fund-balance row at all, so no fund pays it",

	CategoryFundBalanceReserveIncrease: "a printed row of pp.66-67 that the city books against " +
		"the group as a whole: it has no division and no object category on pp.167-170, " +
		"which decompose expenditure and nothing else",

	NodeTransfersIn: "pp.66-67 print Transfers In per fund group, and pp.127-140, which this " +
		"chart draws the funds from, print it for none of this group's funds, so the group's " +
		"transfer in is carried here whole",

	NodeTransfersOut: "pp.66-67 print Transfers Out per fund group, and the pages this chart " +
		"is drawn from print no transfer out, so the group's transfer out leaves beside its " +
		"funds rather than through one",
}

// FundFlowsResidual is the spine endpoints the drill-down does not decompose,
// each with its reason, for the step that opens a fund group into it. The set
// is derived from structure.BudgetBookCuts and BudgetBookExceptions: an
// endpoint whose kind no cut of the drill-down's view prints, and one whose
// cell an exception pins the view's cut absent for -- pp.127-130 print no
// General Fund transfer in. An endpoint is residual per group, never split,
// which is carryResidual's rule in site/app.js; this is the set it may draw
// from.
//
// It errs on a derived endpoint with no reason declared.
func FundFlowsResidual() (map[string]string, error) {
	reference, err := referenceCut()
	if err != nil {
		return nil, err
	}
	view := structure.CutsOf(FundFlowsScopes())
	undecomposed := structure.Undecomposed(view, reference)
	absent := structure.AbsentCells(structure.BudgetBookExceptions(), view, reference)
	out := map[string]string{}
	for _, e := range spineEndpoints {
		carried := slices.Contains(undecomposed, e.kind)
		for _, a := range absent {
			if a.Pin.Coords[structure.AxisCategory] == e.category {
				carried = true
			}
		}
		if !carried {
			continue
		}
		reason, ok := residualReasons[e.id]
		if !ok {
			return nil, fmt.Errorf("project: %s is left undecomposed by the drill-down's cuts and no residual reason is declared for it", e.id)
		}
		out[e.id] = reason
	}
	return out, nil
}

// referenceCut is the cut every derived licence is against.
func referenceCut() (structure.Cut, error) {
	reference, ok := structure.Reference(structure.AllCuts())
	if !ok {
		return structure.Cut{}, errors.New("project: no declared cut is the reference, so nothing says what a derived licence is against")
	}
	return reference, nil
}

// Gap is one column's licence for a gap between the spine's figure for a
// node and what the schedule a step draws accounts for: the signed cents the
// spine carries over, and the reason a reader meets, terminated.
type Gap struct {
	FiscalYear int
	Basis      string
	Cents      int64
	Reason     string
}

// SpendingGaps is the gap between the spine's object-category cells and
// pp.85-125's division rows, keyed by the tier-5 node id an object category
// is drawn at: the departmentwide exceptions against the spine at the
// category level, which cuts-tie-along-the-lattice holds to the facts, spelled
// in node ids rather than declared again.
func SpendingGaps() (map[string][]Gap, error) {
	reference, err := referenceCut()
	if err != nil {
		return nil, err
	}
	cuts := structure.CutsOf(DepartmentSpendingScopes())
	if len(cuts) != 1 {
		return nil, fmt.Errorf("project: scope %s is read by %d cuts, and a gap is declared against one", DepartmentSpendingScope, len(cuts))
	}
	out := map[string][]Gap{}
	for _, e := range structure.ExceptionsOn(structure.BudgetBookExceptions(), cuts[0].Name, reference.Name, structure.LevelCategory) {
		for _, p := range e.Cells {
			id := PrefixExpenditure + p.Coords[structure.AxisCategory]
			out[id] = append(out[id], Gap{
				FiscalYear: p.Year, Basis: p.Basis,
				Cents:  p.Against.Cents - p.Cut.Cents,
				Reason: e.Reason + ".",
			})
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool {
			if out[id][i].FiscalYear != out[id][j].FiscalYear {
				return out[id][i].FiscalYear < out[id][j].FiscalYear
			}
			return out[id][i].Basis < out[id][j].Basis
		})
	}
	return out, nil
}
