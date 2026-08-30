package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// fundBalanceResult runs the whole suite over a fixture whose cells have been
// altered, and returns this check's verdict. The whole suite rather than the one
// check, because a mutation that reddens four other checks as well is a mutation
// that proves less than it looks — the tests below say which.
func fundBalanceResult(t *testing.T, cells []testCell) (Result, *Report) {
	t.Helper()
	rep := runChecks(t, cellsSubject(t, cells))
	return resultFor(t, rep, "fund-balance-identity"), rep
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

	res, _ := fundBalanceResult(t, cells)
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

	res, _ := fundBalanceResult(t, cells)
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
// rather than a fourth line beside beginning and ending. Sum it in and eleven of
// the twelve committed spine cells go red at once, which reads as a broken
// corpus rather than a broken check.
func TestFundBalanceIdentityIgnoresReserveIncrease(t *testing.T) {
	cells := append(slices.Clone(fixtureCells), testCell{
		mapping.KindFundBalance, "fund-balance/reserve-increase", "general", 33_000,
	})

	res, _ := fundBalanceResult(t, cells)
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
