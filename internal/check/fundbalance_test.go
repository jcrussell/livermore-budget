package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// fundBalanceResult runs the whole suite over a fixture whose cells have been
// altered and returns this check's verdict.
//
// It used to also return the *Report, on the stated grounds that the tests below
// would say which OTHER checks a mutation reddened. None of them did — all three
// callers discarded it — and the premise was wrong anyway: cellsSubject leaves
// fact-offset-points-at-token and facts-are-projected red on the UNMUTATED
// baseline, so "what else went red" says nothing about the mutation.
func fundBalanceResult(t *testing.T, cells []testCell) Result {
	t.Helper()
	return resultFor(t, runChecks(t, cellsSubject(t, cells)), "fund-balance-identity")
}

// TestFundBalanceIdentityCatchesAnEndingThatDoesNotFollow is the mutation the
// check exists for: a balance whose three published lines do not agree.
//
// $1 is deliberate. The identity is exact, and a tolerance is the thing this
// check must never grow — the corpus's fund balances tie at zero in all thirteen
// cells, so a discrepancy of any size is a wrong figure rather than the
// document's rounding.
func TestFundBalanceIdentityCatchesAnEndingThatDoesNotFollow(t *testing.T) {
	cells := slices.Clone(fixtureCells)
	for i := range cells {
		if cells[i].category == fundBalanceEnding && cells[i].group == "general" {
			cells[i].cents += 100
		}
	}

	res := fundBalanceResult(t, cells)
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly the general fund balance", res.Findings)
	}
	// The finding has to carry the arithmetic, not just the verdict: whoever
	// reads it needs to know which of the three lines to go and look at.
	for _, want := range []string{
		"beginning $5,000.00", "change -$200.00", "$4,800.00",
		"ending balance of $4,801.00", "difference of $1.00",
	} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("finding %q does not contain %q", res.Findings[0].Detail, want)
		}
	}
	// Both remaining balances are still examined. A check that stopped at the
	// first bad balance would report one subject and look identical.
	if res.Subjects != 2 {
		t.Errorf("subjects = %d, want 2: the enterprise balance is still examined", res.Subjects)
	}
}

// TestFundBalanceIdentityCatchesADroppedLine is the arm that makes this check
// able to see a mapping that stopped publishing a row, rather than only one that
// publishes a wrong figure.
//
// Without it the check fails OPEN in the direction that matters most: drop the
// `change` line and there is nothing left to violate, so the identity would go
// quietly from thirteen subjects to twelve and still report a pass.
func TestFundBalanceIdentityCatchesADroppedLine(t *testing.T) {
	var cells []testCell
	for _, c := range fixtureCells {
		if c.category == fundBalanceChange && c.group == "general" {
			continue
		}
		cells = append(cells, c)
	}

	res := fundBalanceResult(t, cells)
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail; a balance missing a line must not "+
			"pass by having nothing to violate", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly the general fund balance", res.Findings)
	}
	got := res.Findings[0].Detail
	if !strings.Contains(got, "publishes 2 of the three fund-balance lines") ||
		!strings.Contains(got, fundBalanceChange) {
		t.Errorf("finding %q does not say which line is missing", got)
	}
	// The finding must name a FACT, so it points at a line of facts.jsonl. A
	// finding whose subject is a group description addresses nothing.
	if !strings.HasPrefix(res.Findings[0].Subject, "fisc-f-") {
		t.Errorf("finding subject = %q, want a fact id", res.Findings[0].Subject)
	}
}

// TestFundBalanceIdentityIgnoresReserveIncrease pins the exclusion that is
// easiest to get wrong, and the one whose absence would be invisible.
//
// fund-balance/reserve-increase is a FOURTH category carrying kind fund_balance
// — the spine's ADDITION TO RESERVES — and it is a movement inside the change
// rather than a fourth line beside beginning and ending.
//
// Measured: 12 facts carry it and only TWO are non-zero, general FY2026 and
// FY2027, because p66 prints a dash for every other fund group. So summing it in
// reddens two of the twelve committed spine balances — enough to fail the check
// against a corpus that is not wrong, which is what makes the exclusion worth a
// test. An earlier version of this comment said "eleven of the twelve" and was
// wrong; the mutation still reddens, but for a smaller reason than claimed.
func TestFundBalanceIdentityIgnoresReserveIncrease(t *testing.T) {
	cells := append(slices.Clone(fixtureCells), testCell{
		mapping.KindFundBalance, "fund-balance/reserve-increase", "general", 33_000,
	})

	res := fundBalanceResult(t, cells)
	if res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass: a reserve increase is not a term of "+
			"beginning + change == ending", res.Status, res.Summary)
	}
	if res.Subjects != 2 {
		t.Errorf("subjects = %d, want 2: the extra category must not invent a balance",
			res.Subjects)
	}
}

// TestFundBalanceIdentityIsNotVacuousOverTheCommittedCorpus is the guard on this
// check's own worth.
//
// TestTheCommittedCorpusVacuitySplit pins the STATUS, and a status of pass is
// exactly what a check with nothing to look at would report if it were written
// to skip instead of to conclude. This pins the population: thirteen balances,
// over two documents, which is the twelve the Budget Book spine publishes plus
// the one ACFR p41 does. If a coverage lane adds a fund balance, this number
// moves and the mover has to decide whether the new balance really does tie.
func TestFundBalanceIdentityIsNotVacuousOverTheCommittedCorpus(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res := resultFor(t, Run(t.Context(), s, All(), ReportOptions{}), "fund-balance-identity")

	if res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
	}
	if res.Subjects != 13 {
		t.Errorf("subjects = %d, want 13: twelve spine balances and ACFR p41's one",
			res.Subjects)
	}
	if !strings.Contains(res.Summary, "across 2 document(s)") {
		t.Errorf("summary %q does not say how many documents it spans", res.Summary)
	}
}

// TestFundBalanceIdentityReportsADuplicateWithoutAbandoningTheRest pins the
// difference between a defect in the STORE and a failure of the harness.
//
// Two facts of one category on one balance leave the identity with no single
// value to check. The first version of this check returned an error for that,
// which gives the whole check StatusError -- so ONE duplicated line anywhere in
// the corpus would have left all thirteen balances unexamined and reported a
// corpus defect as a broken checker. The report tells those two apart on purpose
// and a check must not conflate them.
//
// The shape is not hypothetical: fisc-2x7y is exactly this, measured on ACFR
// p41, where two rules over one page section can publish one printed figure
// twice.
func TestFundBalanceIdentityReportsADuplicateWithoutAbandoningTheRest(t *testing.T) {
	cells := slices.Clone(fixtureCells)
	for _, c := range fixtureCells {
		if c.category == fundBalanceEnding && c.group == "general" {
			// The same balance's ending line a second time, at a different
			// figure, which is what makes it unanswerable rather than merely
			// repeated.
			cells = append(cells, testCell{c.kind, c.category, c.group, c.cents + 999})
		}
	}

	res := fundBalanceResult(t, cells)
	// want fail and specifically NOT error: the store is wrong, the checker is
	// not, and the report distinguishes them. Asserted as one comparison because
	// a separate `if res.Status == StatusError` after this line is unreachable,
	// which is what the first version of this test shipped -- dead code reading
	// as the guard that carries the test's whole point.
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail; StatusError would mean a duplicated "+
			"line in the store had been reported as a broken checker", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly the general balance", res.Findings)
	}
	// The finding must address a FACT. A balance can be duplicated on a line
	// other than its beginning, and an arm that reached for the beginning id
	// would emit an empty subject there; see TestFundBalanceFindingsAlwaysName
	// ASubject below.
	if !strings.HasPrefix(res.Findings[0].Subject, "fisc-f-") {
		t.Errorf("finding subject = %q, want a fact id", res.Findings[0].Subject)
	}
	got := res.Findings[0].Detail
	for _, want := range []string{fundBalanceEnding, "twice", "excluded from it"} {
		if !strings.Contains(got, want) {
			t.Errorf("finding %q does not contain %q", got, want)
		}
	}

	// AND THE OTHER BALANCE IS STILL EXAMINED. This is the whole point: an
	// error return would have made this 0.
	if res.Subjects != 2 {
		t.Errorf("subjects = %d, want 2: the enterprise balance is unaffected by "+
			"general's duplicate and must still be checked", res.Subjects)
	}
	if !strings.Contains(res.Summary, "1 finding") {
		t.Errorf("summary = %q, want one finding over both balances", res.Summary)
	}
}

// TestFundBalanceFindingsAlwaysNameASubject is the test the second review pass
// was owed, and it is written over BOTH arms because the defect it guards was
// fixed in one arm and reintroduced in the other by the same commit.
//
// A finding whose Subject is "" addresses nothing. The missing-lines arm chose
// its id by ranging a map (non-reproducible across runs); the fix walked the
// three categories in order; the duplicate arm then hard-coded the BEGINNING
// line's id, which is "" on a balance that has no beginning line. Both now go
// through balance.subject, and this asserts the property rather than the
// implementation.
func TestFundBalanceFindingsAlwaysNameASubject(t *testing.T) {
	// A balance with NO beginning line, whose ending line is duplicated: the
	// exact shape that produced subject="".
	var cells []testCell
	for _, c := range fixtureCells {
		if c.category == fundBalanceBeginning && c.group == "general" {
			continue
		}
		cells = append(cells, c)
		if c.category == fundBalanceEnding && c.group == "general" {
			cells = append(cells, testCell{c.kind, c.category, c.group, c.cents + 999})
		}
	}

	res := fundBalanceResult(t, cells)
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if len(res.Findings) == 0 {
		t.Fatal("no findings, so this test asserts nothing about their subjects")
	}
	for _, f := range res.Findings {
		if f.Subject == "" {
			t.Errorf("finding %q has an empty subject; it addresses nothing", f.Detail)
		}
	}
}
