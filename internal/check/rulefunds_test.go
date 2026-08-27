package check

import "testing"

// TestFundNameInReadsBothPrintedShapes pins the two forms this corpus prints and
// the exactness that separates them from a prefix match.
//
// WHY THE SECOND SHAPE EXISTS. A leading `Total ` cut answered every anchor that
// existed while the fund-bearing rules were the revenue schedule's: pp.131-140
// print `Total <fund>` and pp.127-130's rollup prints `Total General Fund`.
// pp.167-170 put the fund first -- p0170:23 is `General Fund Total Expenses`,
// mapped as the rollup gf-total-expenses over all 23 division rules -- so
// declaring `fund: 100` on those rules produced 23 findings reading "no printed
// total governing it names that fund: the totals governing it are ...,
// "General Fund Total Expenses", ...". The anchor was in the list it was
// reported as missing from.
//
// WHAT THIS IS NOT is a prefix match. data/funds.yaml refuses one in writing and
// the exactness is what keeps the five operating/CIP twins apart. The cases
// below assert that what is cut is the word Total and the schedule's own
// trailing noun, never a fragment of a fund name: the caller then resolves the
// whole candidate through FundByLabel, which answers or does not.
func TestFundNameInReadsBothPrintedShapes(t *testing.T) {
	cases := []struct {
		name  string
		total string
		want  string
		ok    bool
	}{
		{"leading, pp.131-140", "Total Airport", "Airport", true},
		{"leading, p130's rollup", "Total General Fund", "General Fund", true},
		{"trailing, p170's rollup", "General Fund Total Expenses", "General Fund", true},
		{"trailing, no noun after Total", "General Fund Total", "General Fund", true},

		// The whole candidate, never a fragment. "Water" and "Water
		// Replacement" are different funds (640, 642) and a prefix match is
		// what would confuse them.
		{"a twin keeps its whole name", "Total Water Replacement", "Water Replacement", true},
		{"a twin keeps its whole name, trailing", "Water Replacement Total Expenses",
			"Water Replacement", true},

		// pp.167-170's own division totals: the printed label is the bare word,
		// so there is no name to read and the anchor is skipped rather than
		// guessed at. This is why unclaimedFundTotals does not report them.
		{"a bare Total names nothing", "Total", "", false},

		// A category total is not a defect either -- pp.127-130 print ten.
		// It resolves to a candidate that FundByLabel will reject.
		{"a category total still yields a candidate", "Total Property Taxes",
			"Property Taxes", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := fundNameIn(c.total)
			if got != c.want || ok != c.ok {
				t.Errorf("fundNameIn(%q) = %q, %v, want %q, %v",
					c.total, got, ok, c.want, c.ok)
			}
		})
	}
}
