package structure_test

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// fund101Break is General Fund CIP Reserves' FY2024 -> FY2025 carry-forward
// as BalanceExceptions declares it.
func fund101Break(t *testing.T) structure.BalanceException {
	t.Helper()
	for _, e := range structure.BalanceExceptions() {
		if e.Identity == structure.BalanceCarryForward && e.At.FundGroup == "capital" && e.At.Fund == "101" {
			return e
		}
	}
	t.Fatal("BalanceExceptions declares no carry-forward break for fund 101")
	return structure.BalanceException{}
}

func TestValidateBalanceExceptionsRefusesWhatCouldHoldNothingApart(t *testing.T) {
	if err := structure.ValidateBalanceExceptions(structure.FundBalances(),
		[]structure.BalanceException{fund101Break(t)}); err != nil {
		t.Fatalf("the well-formed declaration is refused: %v", err)
	}
	for _, tt := range []struct {
		name string
		edit func(*structure.BalanceException)
		want string
	}{
		{"an unknown identity", func(e *structure.BalanceException) { e.Identity = "roll-forward" }, "not declared"},
		{"an undeclared scope", func(e *structure.BalanceException) { e.At.Scope = "revenue-by-fund" }, "does not declare"},
		{"sources = uses on a scope that does not hold it", func(e *structure.BalanceException) {
			e.Identity, e.At.Scope = structure.BalanceSourcesUses, structure.ScopeACFRGeneralFundSummary
		}, "does not hold it"},
		{"two sides that agree", func(e *structure.BalanceException) { e.Right = e.Left }, "needs no exception"},
		{"no printed figures", func(e *structure.BalanceException) { e.Printed = "" }, "where it is printed"},
		{"no reason", func(e *structure.BalanceException) { e.Reason = "" }, "where it is printed"},
		{"no bead", func(e *structure.BalanceException) { e.Bead = "" }, "where it is printed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := fund101Break(t)
			tt.edit(&e)
			err := structure.ValidateBalanceExceptions(structure.FundBalances(), []structure.BalanceException{e})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one saying %q", err, tt.want)
			}
		})
	}
	err := structure.ValidateBalanceExceptions(structure.FundBalances(),
		[]structure.BalanceException{fund101Break(t), fund101Break(t)})
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("err = %v, want a duplicate refused", err)
	}
}

// TestEveryScopePrintingABalanceDeclaresItsStocks holds the table to the one
// thing both checks read off every entry: a balance is its two stocks.
func TestEveryScopePrintingABalanceDeclaresItsStocks(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range structure.FundBalances() {
		if seen[b.Scope] {
			t.Errorf("scope %q is declared twice", b.Scope)
		}
		seen[b.Scope] = true
		var begins, ends bool
		for _, l := range b.Lines {
			begins = begins || l == structure.LineBeginning
			ends = ends || l == structure.LineEnding
		}
		if !begins || !ends {
			t.Errorf("scope %q declares lines %v, without both stocks", b.Scope, b.Lines)
		}
	}
	if b, _ := structure.BalanceOf(structure.FundBalances(), structure.ScopeFundBalancesByFund); b.PrintsChange() {
		t.Errorf("%s is declared to print a change line, and pp.186-209 print none", structure.ScopeFundBalancesByFund)
	}
}
