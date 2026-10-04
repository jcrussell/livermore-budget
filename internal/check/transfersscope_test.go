package check

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestTheGeneralFundTransferInLegIsNotDoubled is fisc-5gk.3.1's acceptance
// criterion, against the committed corpus rather than a fixture.
//
// THE DEFECT IT GUARDS IS SILENT, which is why the number is written out. p76
// and pp.66-67 are the SAME MONEY: every destination fund group's section total
// on p76 equals the spine's TRANSFER IN cell exactly. So a rule written at
// scope all-funds-gross would join the spine projection and netCells would sum
// the two -- the General Fund in-leg going 480,400 -> 960,800, the published
// transfer residual roughly doubling -- with headline-transfer-residual and
// link-values-tie-to-facts both still GREEN, because they tie the graph to the
// facts it was built from rather than to the document. That is fisc-u2v's
// mechanism one schedule over (fisc-aes).
//
// The one check that would see it is exempt exactly there:
// cuts-tie-along-the-lattice reads structure.BudgetBookExceptions, which pins
// (transfer_in, transfers/in, general) on both sides because pp.127-130 print no
// General Fund Transfers In row -- and that is the same key a doubled leg would
// land on.
func TestTheGeneralFundTransferInLegIsNotDoubled(t *testing.T) {
	t.Parallel()
	s := committed(t)

	type cell struct {
		scope string
		year  int
	}
	got := map[cell]int64{}
	scopes := map[string]int{}
	for _, f := range s.Facts {
		scopes[f.Scope]++
		if f.Kind == vocab.KindTransferIn && f.Category == "transfers/in" &&
			f.FundGroup == "general" && f.Basis == vocab.BasisAdopted {
			got[cell{f.Scope, f.FiscalYear}] += f.AmountCents
		}
	}

	// Each scope states it ONCE. Doubling would show as the spine's own cell
	// carrying both, not as two scopes -- so both halves are asserted.
	for _, tt := range []struct {
		scope string
		year  int
		want  int64
	}{
		{"all-funds-gross", 2026, 48040000},
		{"all-funds-gross", 2027, 48673500},
		{"transfers-by-fund", 2026, 48040000},
		{"transfers-by-fund", 2027, 48673500},
	} {
		if g := got[cell{tt.scope, tt.year}]; g != tt.want {
			t.Errorf("%s FY%d General Fund transfers in = %d cents, want %d; "+
				"%d is the doubled value p76 at the spine's scope would produce",
				tt.scope, tt.year, g, tt.want, tt.want*2)
		}
	}

	// BOTH BUDGET YEARS, TOGETHER: the hand-off is one exception per year, so
	// FY2026 alone leaves FY2027's pin naming a cell the detail does not carry.
	if p76 := scopes["transfers-by-fund"]; p76 != 88 {
		t.Errorf("the transfers-by-fund scope holds %d facts, want 88 "+
			"(22 printed rows x 2 budget years x 2 legs)", p76)
	}
}

// TestEveryP76LegNamesAFundOrSaysWhyNot is the payer side's own assertion, and
// it exists because the DOCUMENT cannot make it.
//
// fact.FromValues' counterpart fan-out is downstream of everything that
// compares our read against the city's arithmetic, so a payer declared as the
// wrong fund entirely leaves the page's totals byte for byte unchanged --
// proven in mapping's TestTheDocumentCannotCheckACounterpart, which redeclares
// every p76 payer as fund 999 and changes nothing. What is asserted here is the
// weaker structural claim the fact store CAN make: every leg is addressable.
func TestEveryP76LegNamesAFundOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	s := committed(t)

	legs := map[vocab.Kind]int{}
	noFund := map[string]int{}
	for _, f := range s.Facts {
		if f.Scope != "transfers-by-fund" {
			continue
		}
		legs[f.Kind]++
		if f.ColumnPath == "" || f.RowPath == "" {
			t.Errorf("%s: row_path %q column_path %q; a leg with either empty is "+
				"unaddressable and its id is hashed over the gap",
				f.RowLabel, f.RowPath, f.ColumnPath)
		}
		if f.Fund == nil {
			noFund[f.RowLabel]++
		}
	}

	// One in-leg and one out-leg from every printed figure.
	if legs[vocab.KindTransferIn] != 44 || legs[vocab.KindTransferOut] != 44 {
		t.Errorf("legs = %d in / %d out, want 44 each; every printed figure is "+
			"evidence for both directions of one movement", legs[vocab.KindTransferIn],
			legs[vocab.KindTransferOut])
	}

	// EXACTLY ONE ROW HAS NO FUND AT ITS RECEIVING END, and it is named.
	// "LAVWMA / Wastewater Connection" is a joint powers authority, not a City
	// fund; data/funds.yaml has no entry and no alias for it, and
	// registry.FundByLabel fails on it by name -- deliberately, because the
	// alternative is silently picking Wastewater Connection Fees (623). Its
	// column path is the fund group alone, which is addressable.
	const lavwma = "Transfer From Wastewater to LAVWMA / Wastewater"
	if len(noFund) != 1 || noFund[lavwma] != 2 {
		t.Errorf("rows publishing a leg with no fund = %v, want just %q twice "+
			"(one per budget year); a fundless leg anywhere else is a payer or "+
			"payee that was not resolved", noFund, lavwma)
	}
}
