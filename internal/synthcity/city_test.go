package synthcity

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

func built(t *testing.T) Built {
	t.Helper()
	reg, err := registry.Load(os.DirFS(filepath.Join("..", "..", "data")))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Default().Facts(reg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEveryPlantedShapeIsPrinted: each shape Default plants for a mechanism
// the client is tested on is in the facts it prints, so an edit to the spec
// that drops one goes red here and not as a client test asserting nothing.
// Mutation: drop ERAF from the General Fund's lines and the contra arm goes
// red; drop the Gap and its exceptions arm does.
func TestEveryPlantedShapeIsPrinted(t *testing.T) {
	b := built(t)
	of := func(match func(f fact.Fact) bool) []fact.Fact {
		var out []fact.Fact
		for _, f := range b.Facts {
			if match(f) {
				out = append(out, f)
			}
		}
		return out
	}
	fund := func(f fact.Fact, n int) bool { return f.Fund != nil && *f.Fund == n }
	labels := func(fs []fact.Fact) []string {
		var out []string
		for _, f := range fs {
			if !slices.Contains(out, f.RowLabel) {
				out = append(out, f.RowLabel)
			}
		}
		return out
	}
	years := func(fs []fact.Fact) []int {
		var out []int
		for _, f := range fs {
			if !slices.Contains(out, f.FiscalYear) {
				out = append(out, f.FiscalYear)
			}
		}
		slices.Sort(out)
		return out
	}
	for _, c := range []struct {
		shape string
		ok    bool
	}{
		{"two revenue lines of one category printed as reductions", len(labels(of(func(f fact.Fact) bool {
			return f.Sign == vocab.SignContra && f.AmountCents < 0 && f.Scope == structure.ScopeRevenueByFund &&
				f.Category == "taxes/property"
		}))) == 2},
		{"a special revenue group longer than the funds cap names", len(slices.DeleteFunc(slices.Clone(Default().Funds),
			func(f Fund) bool { return f.Group != "special-revenue" })) > 8},
		{"a fund printing figures in FY2026 only", slices.Equal(years(of(func(f fact.Fact) bool {
			return fund(f, 285) && f.AmountCents != 0 && f.Scope == structure.ScopeRevenueByFund
		})), []int{2026})},
		{"a group every row of which is a dash", len(of(func(f fact.Fact) bool { return fund(f, 470) })) > 0 &&
			len(of(func(f fact.Fact) bool { return fund(f, 470) && f.AmountCents != 0 })) == 0},
		{"a transfer within one fund group", len(of(func(f fact.Fact) bool {
			return f.Scope == structure.ScopeTransfersByFund && f.Kind == vocab.KindTransferIn && fund(f, 610) &&
				f.RowLabel == "Transfer From Wastewater to Stormwater"
		})) > 0},
		{"a division's Transfers Out in FY2023-24 alone", slices.Equal(years(of(func(f fact.Fact) bool {
			return f.Scope == structure.ScopeDepartmentwideExpenditures && f.Kind == vocab.KindTransferOut
		})), []int{2024})},
		{"a declared gap, held apart five ways", len(b.Exceptions) == 7 && len(slices.DeleteFunc(slices.Clone(b.Exceptions),
			func(e structure.Exception) bool { return e.Residual != 25000000 && e.Residual != -25000000 })) == 5},
	} {
		if !c.ok {
			t.Errorf("synthcity.Default prints no %s", c.shape)
		}
	}
}

// TestTheCitysTotalsAreTheSpecs: the spine's revenue in FY2025-26, the column
// printing the spec at 100%, is every fund's revenue lines summed.
func TestTheCitysTotalsAreTheSpecs(t *testing.T) {
	var want int64
	for _, f := range Default().Funds {
		if f.pct(Columns[2]) == 0 {
			continue
		}
		for _, l := range f.Revenue {
			want += l.Cents
		}
	}
	var got int64
	for _, f := range built(t).Facts {
		if f.Scope == structure.ScopeAllFundsGross && f.Kind == vocab.KindRevenue && f.FiscalYear == 2026 {
			got += f.AmountCents
		}
	}
	if got != want || got == 0 {
		t.Errorf("the spine's FY2025-26 revenue is %d cents and the spec's lines sum to %d", got, want)
	}
}
