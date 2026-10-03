package structure

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/registry"
)

// TestAHelpersMistypedPinIsCaught: each exception helper declares its
// residual beside the pins, so a pin a cent off is refused rather than
// carried into a residual computed from it.
func TestAHelpersMistypedPinIsCaught(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(off int64) Exception
	}{
		{"roundsADollar", func(off int64) Exception {
			return roundsADollar("rounds", CutFundBalanceFlows, CutSpine, LevelFundGroupByCategory, 2026, "adopted",
				map[Axis]string{AxisFundGroup: registry.FundTypeCapital, AxisCategory: "fund-balance/ending"},
				10821333100+off, 10821333200, 100, "p", "r")
		}},
		{"carriesADollar", func(off int64) Exception {
			return carriesADollar("carry", 2026, registry.FundTypeCapital, "fund-balance/ending",
				10821333100+off, 10821333200, 100, "p")
		}},
		{"roundsAnActual", func(off int64) Exception {
			return roundsAnActual("actual", CutRevenueDetail, CutFundBalanceRevenues, registry.FundTypeEnterprise, "600",
				488652400+off, 488652500, 100, "p")
		}},
		{"printsNoGFTransferIn", func(off int64) Exception {
			return printsNoGFTransferIn("gf", 2024, "actual", 73745500+off, 73745500, "p")
		}},
		{"printsNoGFTransferInP76", func(off int64) Exception {
			return printsNoGFTransferInP76(2026, 48040000+off, 48040000, "p")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateExceptions([]Exception{tc.build(0)}); err != nil {
				t.Fatalf("as declared: %v", err)
			}
			err := ValidateExceptions([]Exception{tc.build(1)})
			if err == nil || !strings.Contains(err.Error(), "mistyped") {
				t.Fatalf("a pin a cent off: err = %v, want it refused as mistyped", err)
			}
		})
	}
}
