package check

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestTheGeneralFundTransferInLegIsNotDoubled holds the committed corpus to one
// General Fund transfer-in leg: p76 and pp.66-67 are the same money, and a rule
// at scope all-funds-gross would sum them (480,400 -> 960,800) with every
// graph-to-fact check still green.
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

// TestEveryP76LegNamesAFundOrSaysWhyNot holds every p76 leg to being
// addressable; the page's totals cannot tell a wrong payer fund, so this is the
// structural claim the fact store can make.
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

	// Exactly one row has no fund at its receiving end: LAVWMA is a joint
	// powers authority, not a City fund, so its column path is the fund group.
	const lavwma = "Transfer From Wastewater to LAVWMA / Wastewater"
	if len(noFund) != 1 || noFund[lavwma] != 2 {
		t.Errorf("rows publishing a leg with no fund = %v, want just %q twice "+
			"(one per budget year); a fundless leg anywhere else is a payer or "+
			"payee that was not resolved", noFund, lavwma)
	}
}
