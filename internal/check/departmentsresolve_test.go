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

// TestDepartmentsResolveAcceptsEitherTier is the widening itself. pp.85-125's
// Department Funding Sources block is printed per DEPARTMENT and six of the
// eleven departments are not division slugs, so a check resolving only
// divisions could not accept that schedule's own axis. `police-department` in
// the fixture is such a department: a slug in departments.yaml that names no
// division.
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

// TestDepartmentsResolveArmsAreEachReachable is the guard on the widening being
// a widening rather than a hole. The arms are ordered and one of them continues,
// so a relaxation can make a later arm unreachable without any test going red.
func TestDepartmentsResolveArmsAreEachReachable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		department string
		want       string
	}{{
		// Not a slug at all. Reported as shape, which is the more specific
		// diagnosis, and the resolution arm is skipped so one fact gets one fix.
		name: "shape", department: "police_dept",
		want: `department "police_dept" is not a slug`,
	}, {
		// Well formed and in neither tier. This is the arm the widening
		// rewrote, and it must still be able to fire.
		name: "resolution", department: "patrolx",
		want: `department "patrolx" is neither a division nor a department`,
	}, {
		// The category axis, which the two tiers must stay disjoint from
		// because row_path joins them on a "/". This one fires TOGETHER with
		// the resolution arm rather than instead of it -- the collision arm
		// does not continue -- and that is the honest report: the slug is both
		// on the wrong axis and in neither tier.
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
