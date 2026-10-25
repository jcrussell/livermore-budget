package mapping

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
)

// ACFR p41, the General Fund's "Statement of Revenues, Expenditures And Changes
// In Fund Balances (in Millions)", the page mappings/livermore-acfr-fy2025.yaml
// reads.
//
// Units are millions printed to two decimals, so the least significant printed
// digit is 0.01 million = $10,000, and one cent of amount.Cents is one cent.
const acfrStatementPage = 41

// The two substrates of that page, both committed under testdata/ and both
// verbatim copies of data/extracted -- TestFixturesAreVerbatimCopies is what
// keeps them so.
var (
	acfrStatementText     = fmt.Sprintf("../../testdata/pages/acfr-p%04d.txt", acfrStatementPage)
	acfrStatementGeometry = fmt.Sprintf("../../testdata/geometry/acfr-p%04d.json", acfrStatementPage)
)

// acfrRevenueProbe is the revenue block's rule WITHOUT the unmapped_text
// declaration the production rule carries. Two tests read it -- one adds the
// declaration and one does not -- so the difference between them is exactly the
// declaration and nothing else.
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
// mappings/livermore-acfr-fy2025.yaml declares it. It is spelled here rather
// than loaded from that file because the tests below mutate it, and a test that
// edited the committed mapping would be checking whatever the edit produced.
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

// TestACFRp0041RevenueBlockNeedsItsOrphanDeclared runs the rule that maps the
// revenue block WITHOUT the unmapped_text declaration the production rule
// carries, and asserts the resolver refuses it, naming the orphan.
//
// It is a real read rather than assertions about the page's text, so a claim
// about what blocks the block cannot come from anywhere but the resolver:
// whatever refuses, refuses. The declaration is LOAD-BEARING, and removing it
// puts the block back out of reach.
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

	// And the standalone "$" is NOT a blocker, said as an assertion because a
	// comment saying so is what went wrong last time. The first row prints two
	// bare marks and the resolver reads it anyway -- which is only observable
	// because the refusal above happens after every row has been read.
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

// TestACFRp0041RevenueBlockResolvesWithTheOrphanDeclared is the other half of
// the test above: the same rule, plus the one unmapped_text entry the production
// rule carries, reads all ten rows and ties to the printed total EXACTLY.
//
// The exactness is the point and is why this block declares no
// printed_decimals. A tolerance no column needs is refused, so a rule that
// reached for one here would not build -- see
// TestPrintedDecimalsIsRefusedWhenNoColumnNeedsIt.
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

// TestACFRp0041GeneralGovernmentTiesOnlyWithinThePageDerivedTolerance is the
// arithmetic behind acfr-p0041-gf-general-government, and it is the ONLY
// consumer of a document-derived tolerance in this corpus.
//
// The five divisions print 18.44 against a subtotal of 18.45 printed ABOVE them.
// The page is captioned "(in Millions)" and prints two decimals, so
// the unit is $10,000 and the bound is half a unit per row: $25,000 over five
// terms, against a $10,000 error. Read the CAPTION instead of the printed digits
// and the same sum would carry $2,500,000, which is a hundredfold wider and is
// the mistake the declaration exists to prevent.
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

// acfrLine returns the one page line whose label begins with want.
//
// A prefix matching two lines is fatal: every assertion about it would be a
// claim about whichever line came first, which nobody chose.
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

// TestACFRp0041HasNoGeometryColumnGuard pins the claim that
// mappings/livermore-acfr-fy2025.yaml and data/taxonomy.yaml's neighbourhood
// both make in prose: the twenty figures this page publishes are read with NO
// column-position guard over them.
//
// docs/extraction-substrate.md calls that guard "the designed answer" for a
// token landing in the wrong column, so a page that cannot have it is a real
// gap, and a gap asserted only in a comment is one nobody re-measures. Both
// halves of the reason are measured here.
//
// If this test starts failing because the pairing now SUCCEEDS, that is good
// news and the rule file's paragraph about the missing guard has to be
// rewritten -- along with declaring column_headers, which is the other half and
// is separately blocked (the page's printed headers are the bare years 2025 and
// 2024, and the parser refuses a header amount.Parse accepts).
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

	// WHY the two substrates disagree, so the count above is explained rather
	// than merely recorded: the orphan "0.0" of
	// TestACFRp0041RevenueBlockNeedsItsOrphanDeclared is printed close enough to the
	// baseline of the row above that -bbox groups the two into one line, while
	// -layout puts the orphan on its own. One page feature, both failures.
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
