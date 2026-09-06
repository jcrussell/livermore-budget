package check

import (
	"context"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// excessFact is one fact of the fields this check reads. The subjects here are
// hand-built rather than run through the whole suite because the miniature
// spine fixture carries no ACFR statistical fact, and grafting one onto it
// reddens the provenance checks on the unmutated baseline.
func excessFact(id string, kind mapping.Kind, category string, year int, cents int64) fact.Fact {
	return fact.Fact{
		ID: id, DocID: "livermore-acfr-fy2025", Scope: acfrChangesScope,
		Kind: kind, Category: category, Basis: mapping.BasisAudited,
		FiscalYear: year, AmountCents: cents,
	}
}

// excessBaseline is two complete columns that satisfy the identity: two revenue
// rows minus two expenditure rows equals the printed excess, per year.
func excessBaseline() []fact.Fact {
	return []fact.Fact{
		excessFact("r1-2016", mapping.KindRevenue, "taxes/property", 2016, 700),
		excessFact("r2-2016", mapping.KindRevenue, "taxes/sales", 2016, 300),
		excessFact("e1-2016", mapping.KindExpenditure, "fire", 2016, 250),
		excessFact("e2-2016", mapping.KindExpenditure, "police", 2016, 150),
		excessFact("x-2016", mapping.KindFundBalance, categoryExcessOfRevenues, 2016, 600),
		excessFact("r1-2017", mapping.KindRevenue, "taxes/property", 2017, 900),
		excessFact("e1-2017", mapping.KindExpenditure, "fire", 2017, 1000),
		excessFact("x-2017", mapping.KindFundBalance, categoryExcessOfRevenues, 2017, -100),
	}
}

func runExcessIdentity(t *testing.T, facts []fact.Fact) Result {
	t.Helper()
	res, err := (&excessOfRevenuesIdentity{}).Run(context.Background(), &Subject{Facts: facts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func TestExcessIdentityHoldsOverTheBaseline(t *testing.T) {
	res := runExcessIdentity(t, excessBaseline())
	if res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
	}
	if res.Subjects != 2 {
		t.Errorf("subjects = %d, want 2 columns", res.Subjects)
	}
}

// TestExcessIdentityCatchesAWrongExcess is the mutation the check exists for,
// and $0.01 is deliberate: the identity is exact, and the corpus's ten columns
// tie at zero, so a discrepancy of any size is a wrong figure rather than
// rounding.
func TestExcessIdentityCatchesAWrongExcess(t *testing.T) {
	facts := excessBaseline()
	for i := range facts {
		if facts[i].ID == "x-2016" {
			facts[i].AmountCents++
		}
	}
	res := runExcessIdentity(t, facts)
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly the 2016 column", res.Findings)
	}
	// The finding carries the arithmetic — both sums, the difference and the
	// printed figure — so a reader knows which block to go and re-read.
	for _, want := range []string{
		"2 revenue rows sum to $10.00", "2 expenditure rows to $4.00",
		"prints an excess of $6.01", "off by $0.01",
	} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
		}
	}
	if res.Subjects != 2 {
		t.Errorf("subjects = %d, want 2: the 2017 column is still examined", res.Subjects)
	}
}

// TestExcessIdentityCatchesADroppedSide is the fail-closed arm: a rule that
// stopped publishing any of the three sides is a finding, never a skip,
// because with the excess line gone there is nothing left to violate.
func TestExcessIdentityCatchesADroppedSide(t *testing.T) {
	for _, tc := range []struct {
		name string
		drop func(fact.Fact) bool
		want string
	}{{
		name: "the printed excess line",
		drop: func(f fact.Fact) bool { return f.ID == "x-2016" },
		want: "missing the printed excess line",
	}, {
		name: "every expenditure row",
		drop: func(f fact.Fact) bool { return f.Kind == mapping.KindExpenditure && f.FiscalYear == 2016 },
		want: "missing expenditure rows",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			var facts []fact.Fact
			for _, f := range excessBaseline() {
				if !tc.drop(f) {
					facts = append(facts, f)
				}
			}
			res := runExcessIdentity(t, facts)
			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if !strings.Contains(findingDetails(res), tc.want) {
				t.Errorf("findings %v do not say %q", res.Findings, tc.want)
			}
		})
	}
}

// TestExcessIdentityRefusesADuplicatedExcessLine mirrors fundBalanceIdentity's
// duplicate arm: two excess facts on one column leave no single value to
// check, and picking either would be a guess.
func TestExcessIdentityRefusesADuplicatedExcessLine(t *testing.T) {
	facts := append(excessBaseline(),
		excessFact("x-2016-dup", mapping.KindFundBalance, categoryExcessOfRevenues, 2016, 601))
	res := runExcessIdentity(t, facts)
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "twice") {
		t.Errorf("findings %v do not name the duplicate", res.Findings)
	}
}

// TestExcessIdentityIgnoresOtherScopes pins the restriction: a revenue fact in
// another scope must not enter these sums, or the spine would redden a check
// about one ACFR schedule.
func TestExcessIdentityIgnoresOtherScopes(t *testing.T) {
	stray := excessFact("stray", mapping.KindRevenue, "taxes/property", 2016, 5_000)
	stray.Scope = "all-funds-gross"
	res := runExcessIdentity(t, append(excessBaseline(), stray))
	if res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass: the stray fact is out of scope", res.Status, res.Summary)
	}
}

func TestExcessIdentityIsVacuousOverAnEmptyScope(t *testing.T) {
	res := runExcessIdentity(t, nil)
	if res.Status != StatusVacuous {
		t.Fatalf("status = %s (%s), want vacuous", res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, acfrChangesScope) {
		t.Errorf("summary %q does not say which scope is empty", res.Summary)
	}
}
