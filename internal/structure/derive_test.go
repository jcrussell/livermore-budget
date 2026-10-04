package structure

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestUndecomposedIsTheKindsNoCutOfTheViewPrints holds the derivation to the
// committed declarations: over the drill-down's three cuts the spine's
// undecomposed kinds are the two no cut prints, and a cut that gained a kind
// takes it out of the set. Widening a cut's Kinds in cuts.go is therefore
// something this test sees.
func TestUndecomposedIsTheKindsNoCutOfTheViewPrints(t *testing.T) {
	reference, ok := Reference(AllCuts())
	if !ok {
		t.Fatal("no declared cut is the reference")
	}
	view := CutsOf([]string{"revenue-by-fund", "expenditure-by-department", "expenditure-by-fund"})
	if len(view) != 3 {
		t.Fatalf("the drill-down's scopes select %d cuts, want 3", len(view))
	}
	want := []vocab.Kind{vocab.KindFundBalance, vocab.KindTransferOut}
	if diff := cmp.Diff(want, Undecomposed(view, reference)); diff != "" {
		t.Errorf("Undecomposed over the drill-down's cuts (-want +got):\n%s", diff)
	}

	// A cut printing transfers out leaves only fund balances undecomposed.
	widened := make([]Cut, len(view))
	copy(widened, view)
	widened[0].Kinds = append([]vocab.Kind{vocab.KindTransferOut}, widened[0].Kinds...)
	if diff := cmp.Diff([]vocab.Kind{vocab.KindFundBalance}, Undecomposed(widened, reference)); diff != "" {
		t.Errorf("Undecomposed after widening one cut (-want +got):\n%s", diff)
	}
	// A view printing every kind the reference prints leaves nothing.
	if got := Undecomposed([]Cut{reference}, reference); len(got) != 0 {
		t.Errorf("Undecomposed of the reference over itself = %v, want none", got)
	}
}

// TestAbsentCellsAreThePinsTheViewHasNoRowFor: the two General Fund transfer-in
// pins are the only cells an exception declares a drill-down cut absent for,
// and a pin present on both sides is not one.
func TestAbsentCellsAreThePinsTheViewHasNoRowFor(t *testing.T) {
	reference, _ := Reference(AllCuts())
	view := CutsOf([]string{"revenue-by-fund", "expenditure-by-department", "expenditure-by-fund"})
	got := AbsentCells(BudgetBookExceptions(), view, reference)
	if len(got) != 2 {
		t.Fatalf("AbsentCells = %d cells, want the two General Fund transfer-in pins: %+v", len(got), got)
	}
	for _, a := range got {
		if a.Pin.Cut.Present {
			t.Errorf("%s: a pin present on the view's side is not absent", a.Exception)
		}
		if a.Pin.Coords[AxisCategory] != "transfers/in" || a.Pin.Coords[AxisFundGroup] != "general" {
			t.Errorf("%s pins %v, want the General Fund's transfers/in", a.Exception, a.Pin.Coords)
		}
	}
	if got[0].Pin.Year == got[1].Pin.Year {
		t.Errorf("both pins are FY%d; the exceptions declare one per adopted year", got[0].Pin.Year)
	}
	// An exception on a cut outside the view is not the view's absence.
	if got := AbsentCells(BudgetBookExceptions(), CutsOf([]string{"departmentwide-expenditures"}), reference); len(got) != 0 {
		t.Errorf("the departmentwide cut has %d absent cells against the spine, want none", len(got))
	}
}

// TestExceptionsOnSelectsOneComparisonAtOneLevel: the departmentwide cut's
// category-level exceptions against the spine are the two p0067 entries and
// nothing declared at another level or on another pair.
func TestExceptionsOnSelectsOneComparisonAtOneLevel(t *testing.T) {
	reference, _ := Reference(AllCuts())
	got := ExceptionsOn(BudgetBookExceptions(), "departmentwide", reference.Name, LevelCategory)
	if len(got) != 1 || got[0].Name != "p0067-internal-service-is-250000-high-by-object" {
		names := make([]string, 0, len(got))
		for _, e := range got {
			names = append(names, e.Name)
		}
		t.Errorf("ExceptionsOn(departmentwide -> spine, category) = %v, want the one by-object entry", names)
	}
	if got := ExceptionsOn(BudgetBookExceptions(), "departmentwide", reference.Name, LevelDepartment); len(got) != 0 {
		t.Errorf("the departmentwide cut has %d department-level exceptions against the spine, want none: "+
			"its department-level entries are against funding-sources", len(got))
	}
}
