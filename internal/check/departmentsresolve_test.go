package check

import (
	"context"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
)

// departmentFact is one fact of the fields fact-departments-resolve reads.
func departmentFact(id, department string) fact.Fact {
	return fact.Fact{
		ID: id, DocID: "livermore-budget-fy2026-2027", Page: 85,
		RowLabel: "General Fund", Department: department,
	}
}

func runDepartments(t *testing.T, facts ...fact.Fact) Result {
	t.Helper()
	res, err := (&factDepartmentsResolve{}).Run(context.Background(),
		&Subject{Facts: facts, Vocabulary: testVocabulary(t)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// TestDepartmentsResolveAcceptsEitherTier: pp.85-125's Department Funding
// Sources block is printed per department, and `police-department` in the
// fixture is a department that names no division.
func TestDepartmentsResolveAcceptsEitherTier(t *testing.T) {
	res := runDepartments(t,
		departmentFact("f1", "patrol"),
		departmentFact("f2", "police-department"))
	if res.Status != StatusPass {
		t.Fatalf("Status = %s (%s), want pass", res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, "2 facts name one of 2 slugs") {
		t.Errorf("summary %q does not count both facts", res.Summary)
	}
}

// TestDepartmentsResolveArmsAreEachReachable: the arms are ordered and one
// continues, so a relaxation can make a later arm unreachable.
func TestDepartmentsResolveArmsAreEachReachable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		department string
		want       string
	}{{
		// Not a slug: reported as shape only.
		name: "shape", department: "police_dept",
		want: `department "police_dept" is not a slug`,
	}, {
		// Well formed and in neither tier.
		name: "resolution", department: "patrolx",
		want: `department "patrolx" is neither a division nor a department`,
	}, {
		// A category slug (row_path joins the axes on "/"): fires together
		// with the resolution arm, since the collision arm does not continue.
		name: "category collision", department: "wages-and-benefits",
		want: `department "wages-and-benefits" is also a`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			res := runDepartments(t, departmentFact("f1", tc.department))
			if res.Status != StatusFail {
				t.Fatalf("Status = %s (%s), want fail", res.Status, res.Summary)
			}
			var got []string
			for _, f := range res.Findings {
				got = append(got, f.Detail)
			}
			if !strings.Contains(strings.Join(got, "\n"), tc.want) {
				t.Errorf("findings %v do not say %q", got, tc.want)
			}
		})
	}
}
