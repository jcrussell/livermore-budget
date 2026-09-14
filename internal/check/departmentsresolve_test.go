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
	// THE TIER SPLIT IS THE POINT OF THE SUMMARY, not decoration. One total
	// reads the same whether a schedule resolved at the grain its rules meant
	// or at the other one, so a whole block silently re-graining itself would
	// pass unseen.
	for _, want := range []string{
		"1 facts name one of 1 divisions",
		"1 facts name one of 1 departments",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q does not say %q", res.Summary, want)
		}
	}
}

// TestDepartmentsResolveCountsABothTierSlugAsADivision pins the tie-break. Five
// slugs in the real file name a department AND its single division of the same
// name, because that is what the city prints. The division is the finer grain
// and the one every rule predating the funding schedule meant, so reading such
// a slug as the coarser tier would re-grain those facts in the counts above
// while the verdict stayed green.
func TestDepartmentsResolveCountsABothTierSlugAsADivision(t *testing.T) {
	const both = `schema_version: 1
departments:
  - {slug: city-council, label: City Council, document_term: CITY COUNCIL, pages: [167]}
divisions:
  - {slug: city-council, label: City Council, department: city-council, pages: [167]}
`
	res, err := (&factDepartmentsResolve{}).Run(context.Background(), &Subject{
		Facts:      []fact.Fact{departmentFact("f1", "city-council")},
		Vocabulary: vocabularyWithDepartments(t, both),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Summary, "1 facts name one of 1 divisions and 0 facts") {
		t.Errorf("summary %q does not count the both-tier slug as a division", res.Summary)
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
