package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// ACFR p41, the General Fund statement "(in Millions)" at two decimals: one
// printed unit is $10,000.
const acfrStatementPage = 41

// acfrRevenueProbe is the revenue block's rule without its unmapped_text
// declaration, which a test adds when it needs the block to read.
const acfrRevenueProbe = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe
    kind: revenue
    basis: audited
    scope: probe
    grain: fund-group-by-category
    units: millions
    total_row: "Total Revenues"
    parts:
      - page: 41
        section: "Revenues"
        section_ordinal: 2
        stop_at: "Total Revenues"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}
    rows:
      - {label: "Property taxes and special assessments", category: taxes/property}
      - {label: "Sales Taxes", category: taxes/sales}
      - {label: "Other taxes", category: taxes/other}
      - {label: "Licenses and permits", category: licenses-and-permits}
      - {label: "Intergovernmental", category: intergovernmental}
      - {label: "Contributions from outside sources", category: contributions-outsourced}
      - {label: "Fines and forfeitures", category: fines-and-forfeitures}
      - {label: "Charges for current services", category: charges-for-services}
      - {label: "Use of money and property", category: use-of-money-and-property}
      - {label: "Miscellaneous", category: miscellaneous-revenue}
`

// acfrGeneralGovernmentProbe is acfr-p0041-gf-general-government as
// committed, spelled here because the tests below mutate it.
const acfrGeneralGovernmentProbe = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe-general-government
    kind: expenditure
    basis: audited
    scope: probe
    grain: fund-group-by-category
    units: millions
    printed_decimals: 2
    total_row: "General Government:"
    total_row_above: true
    parts:
      - page: 41
        section: "General Government:"
        stop_at: "Fire"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}
    rows:
      - {label: "City Council", category: general-government}
      - {label: "City Manager", category: general-government}
      - {label: "City Attorney", category: general-government}
      - {label: "Administrative Services", category: general-government}
      - {label: "General Services", category: general-government}
`

// TestACFRp0041GeneralGovernmentTiesOnlyWithinThePageDerivedTolerance: the
// five divisions print 18.44 against a subtotal of 18.45; the bound is half a
// printed unit per row ($25,000 over five terms), not the caption's millions.
func TestACFRp0041GeneralGovernmentTiesOnlyWithinThePageDerivedTolerance(t *testing.T) {
	f, err := parse(strings.NewReader(acfrGeneralGovernmentProbe), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	res, err := r.CheckTotals(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("CheckTotals: %v", err)
	}
	if res.Tolerated != 1 || len(res.Slack) != 1 {
		t.Fatalf("tolerated %d column(s) with %d slack figure(s), want 1 and 1; "+
			"this block is the tolerance's only consumer and a clean tie here "+
			"would mean the page changed", res.Tolerated, len(res.Slack))
	}
	// The FIGURE, not just the count. A test asserting only that something was
	// tolerated would pass on a tolerance ten times too wide.
	if got, want := res.Slack[0], amount.Cents(1_000_000); got != want {
		t.Errorf("slack = %s, want %s (0.01 million, one printed unit)", got, want)
	}
	// And it is not a clean tie: Columns counts it, Tolerated says how it tied.
	if got, want := res.Columns, 1; got != want {
		t.Errorf("compared %d columns, want %d", got, want)
	}
}
