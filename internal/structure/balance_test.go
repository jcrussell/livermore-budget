package structure_test

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// fund101Break is the shape of General Fund CIP Reserves' FY2024 -> FY2025
// carry-forward, which the rules commit for pp.186-209 declares.
func fund101Break() structure.BalanceException {
	return structure.BalanceException{
		Identity: structure.BalanceCarryForward,
		At: structure.BalanceAt{DocID: "livermore-budget-fy2026-2027", Scope: structure.ScopeFundBalancesByFund,
			FundGroup: "capital", Fund: "101", Year: 2024, Basis: mapping.BasisActual},
		Left: 0, Right: 3495436300,
		Printed: "p0189 '-' and p0194 34,954,363", Reason: "CIP funds were created in FY2024-25",
		Bead: "fisc-3eh2",
	}
}

func TestValidateBalanceExceptionsRefusesWhatCouldHoldNothingApart(t *testing.T) {
	if err := structure.ValidateBalanceExceptions(structure.FundBalances(),
		[]structure.BalanceException{fund101Break()}); err != nil {
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
			e := fund101Break()
			tt.edit(&e)
			err := structure.ValidateBalanceExceptions(structure.FundBalances(), []structure.BalanceException{e})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one saying %q", err, tt.want)
			}
		})
	}
	err := structure.ValidateBalanceExceptions(structure.FundBalances(),
		[]structure.BalanceException{fund101Break(), fund101Break()})
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
