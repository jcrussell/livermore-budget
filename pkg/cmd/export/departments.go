package export

import (
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// departmentMembership is every department of data/departments.yaml with
// every division it files under it, as the department page reads them: the
// registry `fisc verify` reads too, through which cuts-tie-along-the-lattice
// sums a department's divisions.
func departmentMembership(repoRoot string) ([]export.Department, error) {
	reg, err := loadRegistry(repoRoot)
	if err != nil {
		return nil, err
	}
	divisions := map[string][]export.Division{}
	for _, d := range reg.Divisions() {
		divisions[d.Department] = append(divisions[d.Department],
			export.Division{ID: project.PrefixDept + d.Slug, Label: d.Label})
	}
	out := make([]export.Department, 0, len(reg.Departments()))
	for _, d := range reg.Departments() {
		out = append(out, export.Department{
			ID: project.PrefixDepartment + d.Slug, Label: d.Label, Divisions: divisions[d.Slug],
		})
	}
	return out, nil
}

// departmentsHeldApart is every cell where structure.BudgetBookExceptions
// declares pp.85-125's upper block apart from its lower one at a department,
// with the declaration's own reason, which the department page prints in
// place of saying the two tie.
func departmentsHeldApart() []export.HeldApart {
	var out []export.HeldApart
	for _, e := range structure.ExceptionsOn(structure.BudgetBookExceptions(),
		structure.CutDepartmentwide, structure.CutFundingSources, structure.LevelDepartment) {
		for _, c := range e.Cells {
			out = append(out, export.HeldApart{
				Department: project.PrefixDepartment + c.Coords[structure.AxisDepartment],
				FiscalYear: c.Year,
				Basis:      c.Basis,
				Reason:     e.Reason,
			})
		}
	}
	return out
}
