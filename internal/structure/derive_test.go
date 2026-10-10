package structure

import (
	"testing"
)

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
