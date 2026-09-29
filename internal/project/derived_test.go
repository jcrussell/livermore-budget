package project

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheSpineEndpointsAreTheGoldensOwn holds the endpoint table to the
// document it describes: every node of testdata/sankey.golden.json outside
// the revenue and expenditure hierarchy is in the table, and nothing else is.
func TestTheSpineEndpointsAreTheGoldensOwn(t *testing.T) {
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	var drawn []string
	for _, n := range doc.Nodes {
		if strings.HasPrefix(n.ID, PrefixRevenue) || strings.HasPrefix(n.ID, PrefixExpenditure) ||
			strings.HasPrefix(n.ID, PrefixFundGroup) {
			continue
		}
		drawn = append(drawn, n.ID)
	}
	var declared []string
	for _, e := range spineEndpoints {
		declared = append(declared, e.id)
	}
	slices.Sort(drawn)
	slices.Sort(declared)
	if diff := cmp.Diff(drawn, declared); diff != "" {
		t.Errorf("the golden's endpoints and spineEndpoints differ (-golden +declared):\n%s", diff)
	}
}

// TestTheResidualSetIsDerivedFromTheCuts pins FundFlowsResidual to what
// structure declares: every spine endpoint of a kind the drill-down's cuts do
// not print, plus the one an exception pins absent, each with a non-empty
// reason that names no tracker id; and the reason table carries nothing the
// derivation does not ship. A reason for an endpoint the cuts decompose would
// be prose the client never shows.
func TestTheResidualSetIsDerivedFromTheCuts(t *testing.T) {
	got, err := FundFlowsResidual()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		NodeFundBalanceContribution, NodeFundBalanceDraw, "fund-balance/reserve-increase",
		NodeTransfersIn, NodeTransfersOut,
	}
	slices.Sort(want)
	if diff := cmp.Diff(want, slices.Sorted(maps.Keys(got))); diff != "" {
		t.Errorf("FundFlowsResidual's endpoints (-want +got):\n%s", diff)
	}
	for id, reason := range got {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s is residual with no reason", id)
		}
		if strings.Contains(reason, "fisc-") {
			t.Errorf("%s's reason names a tracker id, and it is reader-facing: %s", id, reason)
		}
	}
	if diff := cmp.Diff(slices.Sorted(maps.Keys(residualReasons)), slices.Sorted(maps.Keys(got))); diff != "" {
		t.Errorf("residualReasons declares a reason the derivation does not ship, or lacks one it does (-reasons +derived):\n%s", diff)
	}
	// transfers/in is carried by the exception alone: the drill-down's
	// revenue cut prints transfer_in, and pp.127-130 print no General Fund
	// row for it.
	reference, _ := structure.Reference(structure.AllCuts())
	undecomposed := structure.Undecomposed(structure.CutsOf(FundFlowsScopes()), reference)
	if slices.Contains(undecomposed, "transfer_in") {
		t.Fatal("transfer_in is undecomposed by kind, so the exception arm below is not what carries transfers/in")
	}
	if _, ok := got[NodeTransfersIn]; !ok {
		t.Error("transfers/in is not residual, and pp.127-130 print no General Fund transfer in")
	}
	// The published map is a fresh copy.
	got["coined/node"] = "widened by a caller"
	if got, _ := FundFlowsResidual(); got["coined/node"] != "" {
		t.Error("FundFlowsResidual handed out a shared map")
	}
}

// TestSpendingGapsAreTheDeclaredExceptions: every cell of every
// departmentwide-against-spine exception at the category level reaches the
// map under its node id and column at the difference of its pins, with its
// reason, and nothing else does.
func TestSpendingGapsAreTheDeclaredExceptions(t *testing.T) {
	got, err := SpendingGaps()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("SpendingGaps is empty, so nothing below asserts anything")
	}
	want := map[string][]Gap{}
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != "departmentwide" || e.Against != "spine" || e.At != structure.LevelCategory {
			continue
		}
		for _, p := range e.Cells {
			id := "expenditure/" + p.Coords[structure.AxisCategory]
			want[id] = append(want[id], Gap{FiscalYear: p.Year, Basis: p.Basis,
				Cents: p.Against.Cents - p.Cut.Cents, Reason: e.Reason + "."})
		}
	}
	if len(want) == 0 {
		t.Fatal("no exception is declared on the departmentwide cut, so the map above is of nothing")
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SpendingGaps (-declared +derived):\n%s", diff)
	}
	for id, gaps := range got {
		for _, g := range gaps {
			if strings.Contains(g.Reason, "fisc-") {
				t.Errorf("SpendingGaps[%q]'s reason names a tracker id: %s", id, g.Reason)
			}
			if g.Cents == 0 {
				t.Errorf("SpendingGaps[%q] licenses a gap of nothing in FY%d %s", id, g.FiscalYear, g.Basis)
			}
		}
	}
}

// TestContestedTotalsReadTheirFiguresOffTheException: the one declared entry
// carries the exception's column, group, both figures and bead, so the caveat
// and cuts-tie-along-the-lattice cannot name two different pairs.
func TestContestedTotalsReadTheirFiguresOffTheException(t *testing.T) {
	all, err := ContestedTotals()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d contested totals, want 1", len(all))
	}
	c := all[0]
	var e structure.Exception
	for _, x := range structure.BudgetBookExceptions() {
		if x.Name == c.Exception {
			e = x
		}
	}
	if e.Name == "" {
		t.Fatalf("contested total names exception %q, which is not declared", c.Exception)
	}
	p := e.Cells[0]
	want := contestedTotal{
		Exception: c.Exception, Column: Column{FiscalYear: p.Year, Basis: "adopted"},
		FundGroup: p.Coords[structure.AxisFundGroup],
		Published: p.Against.Cents, Elsewhere: p.Cut.Cents, Bead: e.Bead,
		SpinePages: c.SpinePages, Row: c.Row, PrintedBy: c.PrintedBy, ImpliedBy: c.ImpliedBy,
	}
	if diff := cmp.Diff(want, c, cmp.AllowUnexported(contestedTotal{})); diff != "" {
		t.Errorf("ContestedTotals (-exception +declared):\n%s", diff)
	}
	if c.Published-c.Elsewhere != e.Residual {
		t.Errorf("the caveat's difference is %d and the exception's residual %d", c.Published-c.Elsewhere, e.Residual)
	}
}

// TestAContestedTotalNamingNoExceptionIsAnError pins that a declaration naming
// an exception structure no longer carries refuses with a message, which
// verify records as a projection that did not build, rather than a panic.
func TestAContestedTotalNamingNoExceptionIsAnError(t *testing.T) {
	was := contestedTotals
	t.Cleanup(func() { contestedTotals = was })
	contestedTotals = slices.Clone(was)
	contestedTotals[0].Exception = "renamed-away"
	if _, err := resolveContestedTotals(); err == nil || !strings.Contains(err.Error(), `"renamed-away", which is not declared`) {
		t.Errorf("resolveContestedTotals = %v, want the undeclared exception named", err)
	}
}
