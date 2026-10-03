package structure_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheCIPFundsBlockIsHeldToP222LineByLine holds pp.186-209's Capital
// Improvement Program Funds block to p222 fund by fund, and shows the hold can
// fail: one transfer in moved by a dollar is a finding naming its fund.
func TestTheCIPFundsBlockIsHeldToP222LineByLine(t *testing.T) {
	facts := committedFacts(t)
	cuts := structure.AllCuts()
	rs := structure.BudgetBookRestatements()
	if err := structure.ValidateRestatements(cuts, structure.BudgetBookResidue(), rs); err != nil {
		t.Fatalf("ValidateRestatements: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("%d restatements, want the CIP block's one", len(rs))
	}
	hold := func(facts []fact.Fact) []string {
		t.Helper()
		cs, err := structure.HoldRestatement(facts, cuts, rs[0])
		if err != nil {
			t.Fatalf("HoldRestatement: %v", err)
		}
		if len(cs) != len(rs[0].Lines) {
			t.Fatalf("%d comparisons for %d lines", len(cs), len(rs[0].Lines))
		}
		var out []string
		for _, c := range cs {
			if !slices.Equal(c.Columns, []string{"FY2025 revised", "FY2026 adopted", "FY2027 adopted"}) {
				t.Errorf("%s covers %v, want p222's three columns", c.Name(), c.Columns)
			}
			out = append(out, c.Findings...)
		}
		return out
	}
	if got := hold(facts); len(got) != 0 {
		t.Fatalf("the committed block disagrees with p222:\n  %s", strings.Join(got, "\n  "))
	}

	moved := slices.Clone(facts)
	n := -1
	for i := range moved {
		f := &moved[i]
		if f.Scope == structure.ScopeFundBalancesByFund && f.RuleID == "fund-balances-fy2026-p0202" &&
			f.Kind == mapping.KindTransferIn && f.AmountCents != 0 {
			f.AmountCents += 100
			n = *f.Fund
			break
		}
	}
	if n < 0 {
		t.Fatal("no non-zero FY2026 transfer in in the CIP block to move")
	}
	t.Run("a column where p222 prints nothing on a line", func(t *testing.T) {
		var without []fact.Fact
		for _, f := range facts {
			if f.Scope == structure.ScopeCIPFundingSources && f.Kind == mapping.KindFundBalance && f.FiscalYear == 2025 {
				continue
			}
			without = append(without, f)
		}
		got := strings.Join(hold(without), "\n")
		if !strings.Contains(got, "FY2025 revised") || !strings.Contains(got, "fund=831") {
			t.Errorf("with p222's FY2025 balance lines gone the hold says %q; want fund 831's FY2025 draw named", got)
		}
	})

	t.Run("a residue line neither restated nor declared unrestated", func(t *testing.T) {
		r := rs[0]
		r.Unrestated = r.Unrestated[1:]
		if _, err := structure.HoldRestatement(facts, cuts, r); err == nil ||
			!strings.Contains(err.Error(), "is on no line it restates or declares unrestated") {
			t.Errorf("HoldRestatement with the expenses line undeclared = %v, want refused", err)
		}
	})

	got := hold(moved)
	if len(got) != 1 || !strings.Contains(got[0], "fund="+strconv.Itoa(n)) {
		t.Errorf("moving fund %d's transfer in by a dollar gave %q, want one finding naming it", n, got)
	}
}

// TestARestatementIsOfAnOutsideCutAndReadsOnlyResidue walks the refusals.
func TestARestatementIsOfAnOutsideCutAndReadsOnlyResidue(t *testing.T) {
	cuts, residue := structure.AllCuts(), structure.BudgetBookResidue()
	for _, tc := range []struct {
		name   string
		breaks func(*structure.Restatement)
		want   string
	}{
		{"against a cut the lattice compares", func(r *structure.Restatement) { r.Against = structure.CutSpine },
			"is not declared Outside"},
		{"against no cut", func(r *structure.Restatement) { r.Against = "nowhere" }, "is not a declared cut"},
		{"reading a rule that is not residue", func(r *structure.Restatement) {
			r.Rules = append(slices.Clone(r.Rules), "fund-balances-fy2026-p0198")
		}, "is not declared residue"},
		{"at a level the cut cannot be summed to", func(r *structure.Restatement) {
			r.At = structure.LevelDepartment
		}, "is not one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := structure.BudgetBookRestatements()[0]
			tc.breaks(&r)
			err := structure.ValidateRestatements(cuts, residue, []structure.Restatement{r})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ValidateRestatements = %v, want %q", err, tc.want)
			}
		})
	}
}
