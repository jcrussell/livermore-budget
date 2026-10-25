package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestFundBalanceIdentityCatchesAnEndingThatDoesNotFollow is the mutation the
// check exists for: a balance whose three published lines do not agree.
//
// $1 is deliberate. The identity is exact, and a tolerance is the thing this
// check must never grow — the corpus's fund balances tie at zero in all thirteen
// cells, so a discrepancy of any size is a wrong figure rather than the
// document's rounding.
func TestFundBalanceIdentityCatchesAnEndingThatDoesNotFollow(t *testing.T) {
	t.Parallel()
	cells := slices.Clone(fixtureCells)
	for i := range cells {
		if cells[i].category == project.CategoryFundBalanceEnding && cells[i].group == "general" {
			cells[i].cents += 100
		}
	}

	res := runCheck(t, &fundBalanceIdentity{}, cellsSubject(t, cells))
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
	t.Parallel()
	var cells []testCell
	for _, c := range fixtureCells {
		if c.category == project.CategoryFundBalanceChange && c.group == "general" {
			continue
		}
		cells = append(cells, c)
	}

	res := runCheck(t, &fundBalanceIdentity{}, cellsSubject(t, cells))
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail; a balance missing a line must not "+
			"pass by having nothing to violate", res.Status, res.Summary)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %v, want exactly the general fund balance", res.Findings)
	}
	got := res.Findings[0].Detail
	if !strings.Contains(got, "publishes 2 of the 3 fund-balance lines") ||
		!strings.Contains(got, project.CategoryFundBalanceChange) {
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
// Summing it in reddens every committed balance whose page prints it non-zero,
// which would fail the check against a corpus that is not wrong, and is what
// makes the exclusion worth a test.
func TestFundBalanceIdentityIgnoresReserveIncrease(t *testing.T) {
	t.Parallel()
	cells := append(slices.Clone(fixtureCells), testCell{
		vocab.KindFundBalance, "fund-balance/reserve-increase", "general", 33_000,
	})

	res := runCheck(t, &fundBalanceIdentity{}, cellsSubject(t, cells))
	if res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass: a reserve increase is not a term of "+
			"beginning + change == ending", res.Status, res.Summary)
	}
	if res.Subjects != 2 {
		t.Errorf("subjects = %d, want 2: the extra category must not invent a balance",
			res.Subjects)
	}
}

// TestFundBalanceIdentityReportsADuplicateWithoutAbandoningTheRest pins the
// difference between a defect in the STORE and a failure of the harness.
//
// Two facts of one category on one balance leave the identity with no single
// value to check. That is a finding, not an error: an error gives the whole
// check StatusError, so one duplicated line would leave every balance
// unexamined and report a corpus defect as a broken checker.
//
// The shape is not hypothetical: fisc-2x7y is exactly this, measured on ACFR
// p41, where two rules over one page section can publish one printed figure
// twice.
func TestFundBalanceIdentityReportsADuplicateWithoutAbandoningTheRest(t *testing.T) {
	t.Parallel()
	cells := slices.Clone(fixtureCells)
	for _, c := range fixtureCells {
		if c.category == project.CategoryFundBalanceEnding && c.group == "general" {
			// The same balance's ending line a second time, at a different
			// figure, which is what makes it unanswerable rather than merely
			// repeated.
			cells = append(cells, testCell{c.kind, c.category, c.group, c.cents + 999})
		}
	}

	res := runCheck(t, &fundBalanceIdentity{}, cellsSubject(t, cells))
	// want fail and specifically NOT error: the store is wrong, the checker is
	// not, and the report distinguishes them.
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
	for _, want := range []string{project.CategoryFundBalanceEnding, "twice", "excluded from it"} {
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

// TestFundBalanceFindingsAlwaysNameASubject covers BOTH arms that emit a
// finding, one case each: the duplicate arm runs first and `continue`s, so a
// balance with both defects would reach only it.
func TestFundBalanceFindingsAlwaysNameASubject(t *testing.T) {
	t.Parallel()
	// Both cases drop general's BEGINNING line, the id a subject-less finding
	// would reach for; a balance that still has one cannot show the defect.
	withoutBeginning := func() []testCell {
		var out []testCell
		for _, c := range fixtureCells {
			if c.category == project.CategoryFundBalanceBeginning && c.group == "general" {
				continue
			}
			out = append(out, c)
		}
		return out
	}

	for _, tt := range []struct {
		name  string
		cells func() []testCell
	}{
		{
			// Reaches the MISSING-LINES arm: two of three lines, no duplicate.
			name:  "a balance missing a line",
			cells: withoutBeginning,
		},
		{
			// Reaches the DUPLICATE arm, which returns before the one above.
			name: "a balance with a duplicated line",
			cells: func() []testCell {
				var out []testCell
				for _, c := range withoutBeginning() {
					out = append(out, c)
					if c.category == project.CategoryFundBalanceEnding && c.group == "general" {
						out = append(out, testCell{c.kind, c.category, c.group, c.cents + 999})
					}
				}
				return out
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := runCheck(t, &fundBalanceIdentity{}, cellsSubject(t, tt.cells()))
			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if len(res.Findings) == 0 {
				t.Fatal("no findings, so this case asserts nothing about their subjects")
			}
			for _, f := range res.Findings {
				if f.Subject == "" {
					t.Errorf("finding %q has an empty subject; it addresses nothing", f.Detail)
				}
			}
		})
	}
}
