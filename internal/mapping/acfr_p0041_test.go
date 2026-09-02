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
// In Fund Balances (in Millions)". This is the page mappings/livermore-acfr-fy2025.yaml
// reads, and this file is the arithmetic behind every decision that file makes
// about it -- including what it still declines to map, which is nine rows of the
// expenditure block rather than a block.
//
// The page is read here with strings.Fields and amount.Parse rather than through
// the resolver, and the reason has CHANGED since this comment was written. It
// used to be that two of the page's four blocks could not be resolved at all, so
// reading by hand was the only way to carry evidence about them. Both can be
// resolved now (fisc-hcus and fisc-h96o), and the reason is the one
// acfr_p177_test.go gives instead: these claims are about what the DOCUMENT
// says, and a hand read states them without depending on the rules that were
// written FROM them.
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

// acfrRow reads the one line whose label begins with want and returns its
// figures, at millions.
//
// Bare "$" tokens are dropped rather than parsed, because this helper reads the
// page with strings.Fields and the resolver's dropCurrencyMarks is not in play.
//
// The marks are not a blocker: dropCurrencyMarks reads a lone mark in the
// labelled path, and the revenue block is mapped, the orphan "0.0" carrying an
// honest declaration (fisc-hcus). The assertion that settles it is
// TestACFRp0041RevenueBlockNeedsItsOrphanDeclared, where running the rule does.
func acfrRow(t *testing.T, want string) []amount.Cents {
	t.Helper()

	b, err := os.ReadFile(acfrStatementText)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var found []amount.Cents
	hits := 0
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, want) {
			continue
		}
		hits++
		var cents []amount.Cents
		for _, f := range strings.Fields(strings.TrimPrefix(trimmed, want)) {
			if f == "$" {
				continue
			}
			c, err := amount.Parse(f, amount.Millions)
			if err != nil {
				t.Fatalf("row %q: Parse(%q): %v", want, f, err)
			}
			cents = append(cents, c)
		}
		found = cents
	}
	// A label matching two lines would silently read whichever came last, and
	// every claim in this file would be about a row nobody chose.
	if hits != 1 {
		t.Fatalf("label %q matches %d lines of the fixture, want exactly 1", want, hits)
	}
	if len(found) != 2 {
		t.Fatalf("row %q carries %d figures, want 2 (FY2025 and FY2024)", want, len(found))
	}
	return found
}

const (
	fy2025 = 0
	fy2024 = 1
)

func sum(cs ...amount.Cents) amount.Cents {
	var t amount.Cents
	for _, c := range cs {
		t += c
	}
	return t
}

// TestACFRp0041OtherFinancingSourcesTiesToItsPrintedTotal is the arithmetic
// behind acfr-p0041-gf-other-financing-sources, and it ties in BOTH columns --
// which is worth stating, because the prior-year column fails in every other
// block on the page and it would be easy to describe FY2024 as simply broken.
func TestACFRp0041OtherFinancingSourcesTiesToItsPrintedTotal(t *testing.T) {
	in := acfrRow(t, "Transfers in")
	out := acfrRow(t, "Transfers (out)")
	stated := acfrRow(t, "Total Other Financing Sources (Uses)")

	for _, col := range []struct {
		name string
		i    int
	}{{"FY2025", fy2025}, {"FY2024", fy2024}} {
		if got, want := sum(in[col.i], out[col.i]), stated[col.i]; got != want {
			t.Errorf("%s: transfers in + out = %s, page states %s", col.name, got, want)
		}
	}

	// The published figures, pinned so a re-extraction that moved them fails
	// here rather than in a golden nobody reads.
	if got, want := in[fy2025], amount.Cents(53_000_000); got != want {
		t.Errorf("FY2025 transfers in = %s, want %s", got, want)
	}
	// Negative as printed. The Budget Book prints its TRANSFER OUT row as a
	// positive magnitude in a uses column, so transfer_out facts carry opposite
	// signs in the two documents; see the alias note in data/taxonomy.yaml.
	if got, want := out[fy2025], amount.Cents(-2_572_000_000); got != want {
		t.Errorf("FY2025 transfers out = %s, want %s", got, want)
	}
}

// TestACFRp0041FundBalancesSatisfyTheIdentity is the page's half of the
// fund-balance-identity check: beginning + change == ending. It holds exactly in
// FY2025, which is the column mappings/livermore-acfr-fy2025.yaml publishes, and
// MISSES BY 0.10 IN FY2024, which is one of the three reasons that column is
// declared skip: true.
func TestACFRp0041FundBalancesSatisfyTheIdentity(t *testing.T) {
	beginning := acfrRow(t, "Fund Balances- Beginning, as restated")
	change := acfrRow(t, "Net Change in Fund Balances")
	ending := acfrRow(t, "Fund Balances- Ending")

	if got, want := sum(beginning[fy2025], change[fy2025]), ending[fy2025]; got != want {
		t.Errorf("FY2025: beginning + change = %s, page states an ending balance of %s",
			got, want)
	}

	// 83.90 + 8.10 = 92.00 against a printed 92.10. $100,000 on a page printed
	// to the nearest $10,000 -- ten printed units, and not the document's own
	// rounding.
	gap := ending[fy2024] - sum(beginning[fy2024], change[fy2024])
	if want := amount.Cents(10_000_000); gap != want {
		t.Errorf("FY2024: the identity misses by %s, want %s -- if the page now ties, "+
			"the reason mappings/livermore-acfr-fy2025.yaml skips this column has changed",
			gap, want)
	}
}

// TestACFRp0041ExpendituresTieUnderTheTopLevelReading carries the finding that
// decided this lane. It used to end "and it is the reason no document-derived
// tolerance landed with it"; one landed on this page, on the strength of the
// arithmetic below, so what it is now is the reason the tolerance sits on the
// SUBTOTAL and not on Total Expenditures.
//
// The page prints a "General Government:" subtotal AND its five constituent
// rows. Read at the top level -- the subtotal and the nine rows beside it -- the
// block ties to the printed Total Expenditures EXACTLY. Descend past the
// subtotal into its five children and the sum is 0.01 short, because those five
// are themselves 0.01 short of the subtotal they roll into.
//
// So the 0.01 belongs to the General Government subtotal, one level down, and
// NOT to Total Expenditures. A rule that compared fourteen leaves against a
// total the page prints over ten rows would be asserting an equality the city
// never states, and would then need a tolerance to absorb an error it had itself
// introduced. fisc-9hf and fisc-1wr.2 are corrected against this.
func TestACFRp0041ExpendituresTieUnderTheTopLevelReading(t *testing.T) {
	generalGovernment := acfrRow(t, "General Government:")
	children := []string{
		"City Council", "City Manager", "City Attorney",
		"Administrative Services", "General Services",
	}
	siblings := []string{
		"Fire", "Police", "Public Works", "Community Development",
		"Economic Development", "Library", "Capital Outlay",
		"Principal", "Interest and fiscal charges",
	}
	stated := acfrRow(t, "Total Expenditures")

	topLevel := generalGovernment[fy2025]
	var leaves amount.Cents
	for _, c := range children {
		leaves += acfrRow(t, c)[fy2025]
	}
	for _, s := range siblings {
		v := acfrRow(t, s)[fy2025]
		topLevel += v
		leaves += v
	}

	if got, want := topLevel, stated[fy2025]; got != want {
		t.Errorf("FY2025 top-level reading = %s, page states %s; the ten rows the "+
			"page prints at the top level are what its Total Expenditures covers", got, want)
	}
	if got, want := stated[fy2025]-leaves, amount.Cents(1_000_000); got != want {
		t.Errorf("FY2025 leaf reading is short by %s, want %s", got, want)
	}
	// The 0.01 is the General Government subtotal's own, and this is where it
	// lives: five children against the subtotal printed above them.
	var childSum amount.Cents
	for _, c := range children {
		childSum += acfrRow(t, c)[fy2025]
	}
	if got, want := generalGovernment[fy2025]-childSum, amount.Cents(1_000_000); got != want {
		t.Errorf("General Government subtotal exceeds its five children by %s, want %s",
			got, want)
	}
}

// TestACFRp0041PriorYearColumnDoesNotReconcile is the arithmetic carrying the
// decision to declare the FY2024 column skip: true rather than publish it.
//
// data/sources.yaml declares this document carries fiscal_years: [2025], which
// would be reason enough. This is the stronger reason: three of the page's four
// PRINTED blocks do not reconcile in that column, by $200,000, $100,000 and
// $100,000 on a page printed to the nearest $10,000. Those are twenty, ten and
// ten printed units -- nowhere near the document's own rounding, and nothing a
// declared delta or a page-derived tolerance should absorb.
//
// THE GENERAL GOVERNMENT SUB-BLOCK IS NOT ONE OF THOSE FOUR and misses by
// $170,000 of its own, seventeen units. Keeping the two counts apart matters
// here more than anywhere: FOUR MEANS THE FOUR THE PAGE PRINTS, never the four
// this file maps, and the two readings differ by exactly this sub-block.
//
// The expenditure figure is the TOP-LEVEL reading, the one that ties exactly in
// FY2025, so FY2024 fails under either reading.
func TestACFRp0041PriorYearColumnDoesNotReconcile(t *testing.T) {
	revenue := []string{
		"Property taxes and special assessments", "Sales Taxes", "Other taxes",
		"Licenses and permits", "Intergovernmental",
		"Contributions from outside sources", "Fines and forfeitures",
		"Charges for current services", "Use of money and property", "Miscellaneous",
	}
	var revenueSum amount.Cents
	for _, r := range revenue {
		revenueSum += acfrRow(t, r)[fy2024]
	}
	if got, want := revenueSum-acfrRow(t, "Total Revenues")[fy2024],
		amount.Cents(20_000_000); got != want {
		t.Errorf("FY2024 revenues exceed the printed total by %s, want %s", got, want)
	}

	// The expenditure block, at the top level -- the reading that ties exactly
	// in FY2025. It does not tie here, so FY2024 fails under either reading.
	top := []string{
		"General Government:", "Fire", "Police", "Public Works",
		"Community Development", "Economic Development", "Library",
		"Capital Outlay", "Principal", "Interest and fiscal charges",
	}
	var expenditureSum amount.Cents
	for _, e := range top {
		expenditureSum += acfrRow(t, e)[fy2024]
	}
	if got, want := acfrRow(t, "Total Expenditures")[fy2024]-expenditureSum,
		amount.Cents(10_000_000); got != want {
		t.Errorf("FY2024 expenditures fall short of the printed total by %s, want %s",
			got, want)
	}

	// The fund-balance identity is asserted in its own test above; named here
	// so the count of failing blocks in the rule file's comment is checkable.
	beginning := acfrRow(t, "Fund Balances- Beginning, as restated")[fy2024]
	change := acfrRow(t, "Net Change in Fund Balances")[fy2024]
	ending := acfrRow(t, "Fund Balances- Ending")[fy2024]
	if beginning+change == ending {
		t.Error("the FY2024 fund-balance identity now ties; the rule file says it does not")
	}
}

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
// It is written as a real read rather than as assertions about the page's text
// because the first draft of this test was assertions about the text, and one of
// them was WRONG. It claimed a second blocker -- the first revenue row prints a
// standalone "$" before each figure, which is the shape fisc-yun is about -- and
// fisc-yun is CLOSED: dropCurrencyMarks handles a lone currency mark in the
// labelled read, and all ten rows here read through it without complaint. The
// claim came from a bead title instead of from the tree. Running the rule cannot
// make that mistake: whatever refuses, refuses.
//
// WHAT IT ASSERTS CHANGED WHEN fisc-hcus LANDED AND THE NAME CHANGED WITH IT.
// It used to be called ...CannotBeResolved and its comment ended "if it acquires
// an honest declaration, this test goes red by succeeding". It did not, and
// would not have: this probe rule declares no unmapped_text, so it stays refused
// whatever the production rule does, and a test still named for a page that
// cannot be read would have gone on passing beside a mapping file that reads it.
// The assertion is now the useful half of the same read -- that the declaration
// is LOAD-BEARING, and removing it puts the block back out of reach.
//
// Its sibling TestACFRp0041RevenueBlockResolvesWithTheOrphanDeclared is the
// other half, and neither is worth much alone.
func TestACFRp0041RevenueBlockNeedsItsOrphanDeclared(t *testing.T) {
	const yaml = acfrRevenueProbe
	f, err := Parse(strings.NewReader(yaml), "probe.yaml")
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
	// The refusal must be ABOUT THE ORPHAN. A test satisfied by any error would
	// pass on a mistyped anchor, which is how the first draft of this file's
	// sibling test passed on a leading-gap error that had nothing to do with the
	// row it was named for (fisc-i0d9 is the same shape).
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
	f, err := Parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
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
// consumer of a document-derived tolerance in this corpus (fisc-1wr.2).
//
// The five divisions print 18.44 against a subtotal of 18.45 printed ABOVE them
// (fisc-h96o). The page is captioned "(in Millions)" and prints two decimals, so
// the unit is $10,000 and the bound is half a unit per row: $25,000 over five
// terms, against a $10,000 error. Read the CAPTION instead of the printed digits
// and the same sum would carry $2,500,000, which is a hundredfold wider and is
// the mistake the declaration exists to prevent.
func TestACFRp0041GeneralGovernmentTiesOnlyWithinThePageDerivedTolerance(t *testing.T) {
	f, err := Parse(strings.NewReader(acfrGeneralGovernmentProbe), "probe.yaml")
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

// TestACFRp0041GeneralGovernmentStopsAtFire pins the one assumption the
// production rule makes that the resolver does not check for itself.
//
// Block resolves stop_at with a plain strings.Index and no uniqueness guard,
// unlike section, which goes through anchor() and refuses an ambiguous match.
// "Fire" is a four-character substring; it is unique after this rule's anchor
// today, and that is a property of the page rather than of the code, so it is
// asserted here. fisc-km5n owns giving stop_at a guard of its own.
func TestACFRp0041GeneralGovernmentStopsAtFire(t *testing.T) {
	b, err := os.ReadFile(acfrStatementText)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text := string(b)
	at := strings.Index(text, "General Government:")
	if at < 0 {
		t.Fatal(`the page no longer prints "General Government:"`)
	}
	if got := strings.Count(text[at:], "Fire"); got != 1 {
		t.Errorf(`"Fire" occurs %d times after the General Government anchor, want 1; `+
			"the rule's stop_at would bind to whichever came first", got)
	}
}

// acfrLine returns the one page line whose label begins with want.
//
// "The one" is enforced rather than assumed, for the reason acfrRow enforces it:
// a prefix matching two lines would make every assertion about it a claim about
// whichever line came first, which nobody chose. This helper returned the first
// match while its own doc comment said "the one line" and its sibling fatalled
// on exactly this.
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
// mappings/livermore-acfr-fy2025.yaml, data/taxonomy.yaml's neighbourhood and
// internal/check's unprojectedScopes entry all make in prose: the twenty figures
// this lane publishes are read with NO column-position guard over them.
//
// AGENTS.md calls that guard "the designed answer" for a token landing in the
// wrong column, so a page that cannot have it is a real gap, and a gap asserted
// only in a comment is one nobody re-measures. Both halves of the reason are
// measured here.
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
	// TestACFRp0041RevenueBlockCannotBeResolved is printed close enough to the
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
