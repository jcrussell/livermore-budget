package mapping

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
)

// ACFR p41, the General Fund statement "(in Millions)" at two decimals: one
// printed unit is $10,000.
const acfrStatementPage = 41

// The two substrates of that page, both committed under testdata/ and both
// verbatim copies of data/extracted -- TestFixturesAreVerbatimCopies is what
// keeps them so.
var (
	acfrStatementText     = fmt.Sprintf("../../testdata/pages/acfr-p%04d.txt", acfrStatementPage)
	acfrStatementGeometry = fmt.Sprintf("../../testdata/geometry/acfr-p%04d.json", acfrStatementPage)
)

// acfrRevenueProbe is the revenue block's rule without its unmapped_text
// declaration, so the two tests over it differ by exactly that declaration.
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

// TestACFRp0041RevenueBlockNeedsItsOrphanDeclared: without the unmapped_text
// declaration the resolver refuses the block, naming the orphan.
func TestACFRp0041RevenueBlockNeedsItsOrphanDeclared(t *testing.T) {
	const yaml = acfrRevenueProbe
	f, err := parse(strings.NewReader(yaml), "probe.yaml")
	if err != nil {
		t.Fatalf("the rule does not even parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	_, _, err = r.Values(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("the block resolved with no unmapped_text declaration; the " +
			"declaration in mappings/livermore-acfr-fy2025.yaml is doing nothing, " +
			"and the orphan is being admitted by something else")
	}
	// The refusal must be ABOUT THE ORPHAN: a test satisfied by any error would
	// pass on a mistyped anchor.
	if got := err.Error(); !strings.Contains(got, `"0.0" follows the last mapped row`) {
		t.Errorf("the read was refused by something other than the orphan: %v", got)
	}

	// The standalone "$" is not a blocker: the first row prints two bare marks
	// and is read before the refusal above.
	bare := 0
	for _, field := range strings.Fields(acfrLine(t, "Property taxes and special assessments")) {
		if field == "$" {
			bare++
		}
	}
	if bare != 2 {
		t.Errorf("got %d standalone \"$\" tokens on the first revenue row, want 2; "+
			"this row is the evidence that dropCurrencyMarks handles them", bare)
	}
}

// TestACFRp0041RevenueBlockResolvesWithTheOrphanDeclared: with the declaration
// all ten rows read and tie EXACTLY, with no printed_decimals.
func TestACFRp0041RevenueBlockResolvesWithTheOrphanDeclared(t *testing.T) {
	f, err := parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
		"        columns:", `        unmapped_text:
          - text: "0.0"
            note: "the spreadsheet artefact this test is about"
        columns:`, 1)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("the declared block does not resolve: %v", err)
	}
	// Ten rows, one unskipped column. A count assertion is what stops this
	// passing on a read that found some other block.
	if got, want := len(values), 10; got != want {
		t.Errorf("got %d values, want %d", got, want)
	}
	res, err := r.CheckTotals(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("CheckTotals: %v", err)
	}
	// Nothing tied by declaration and nothing by tolerance: the page's own
	// arithmetic holds here at zero slack.
	if res.Declared != 0 || res.Tolerated != 0 {
		t.Errorf("declared %d and tolerated %d columns, want 0 and 0; ACFR p41's "+
			"revenue block ties to 157.20 exactly", res.Declared, res.Tolerated)
	}
}

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

// acfrLine returns the one page line whose label begins with want; a prefix
// matching two lines is fatal.
func acfrLine(t *testing.T, want string) string {
	t.Helper()
	b, err := os.ReadFile(acfrStatementText)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var found string
	hits := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), want) {
			hits++
			found = line
		}
	}
	if hits != 1 {
		t.Fatalf("label %q matches %d lines of the fixture, want exactly 1", want, hits)
	}
	return found
}

// TestACFRp0041HasNoGeometryColumnGuard: the twenty figures on p41 are read
// with no column-position guard, and both reasons are measured here. If the
// pairing starts to SUCCEED, rewrite the rule file's paragraph on the guard.
func TestACFRp0041HasNoGeometryColumnGuard(t *testing.T) {
	txt, err := os.ReadFile(acfrStatementText)
	if err != nil {
		t.Fatalf("read page fixture: %v", err)
	}
	raw, err := os.ReadFile(acfrStatementGeometry)
	if err != nil {
		t.Fatalf("read geometry fixture: %v", err)
	}
	g, err := geom.ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}

	nonblank := 0
	for _, line := range strings.Split(string(txt), "\n") {
		if strings.TrimSpace(line) != "" {
			nonblank++
		}
	}
	if got, want := nonblank, 52; got != want {
		t.Errorf("got %d non-blank text lines, want %d", got, want)
	}
	if got, want := len(g.Lines()), 51; got != want {
		t.Errorf("got %d geometry lines, want %d", got, want)
	}
	if _, err := buildPairing(string(txt), g); err == nil {
		t.Error("buildPairing succeeded; the page now has a column guard available " +
			"and every comment saying it does not is stale")
	}

	// The orphan "0.0" sits close enough to the row above that -bbox groups
	// them into one line while -layout keeps it on its own.
	var orphan, miscellaneous *geom.Word
	for i := range g.Words {
		switch g.Words[i].Text {
		case "0.0":
			orphan = &g.Words[i]
		case "Miscellaneous":
			miscellaneous = &g.Words[i]
		}
	}
	if orphan == nil || miscellaneous == nil {
		t.Fatal("the orphan \"0.0\" or the Miscellaneous label is gone from the geometry")
	}
	if got := orphan.Y0 - miscellaneous.Y0; got < 0 || got > 1 {
		t.Errorf("the orphan sits %.2fpt below the Miscellaneous baseline; it was 0.32pt, "+
			"and the line grouping folds it into that row only while the two are close",
			got)
	}
}
