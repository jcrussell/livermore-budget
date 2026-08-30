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
// about it -- including the three blocks it declines to map.
//
// The page is read here with strings.Fields and amount.Parse rather than through
// the resolver, for the reason acfr_p177_test.go gives: the claims below are
// about what the DOCUMENT says, and two of its four blocks cannot be resolved at
// all. Reading them by hand is the only way to carry the evidence for that.
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
// Bare "$" tokens are dropped rather than parsed. That is not tidying: the
// revenue block prints a STANDALONE "$" before each figure on its first row
// ("$       60.42    $        57.70"), which is fisc-yun -- the same shape that
// makes ACFR p177 unreadable to the resolver. It is one of the two reasons the
// revenue block is unmapped, and TestACFRp0041RevenueBlockCannotBeResolved
// asserts it rather than leaving it to this helper's silence.
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
// decided this lane, and it is the reason no document-derived tolerance landed
// with it.
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
// blocks do not reconcile in that column, by $200,000, $270,000 and $100,000 on
// a page printed to the nearest $10,000. Those are twenty, twenty-seven and ten
// printed units -- nowhere near the document's own rounding, and nothing a
// declared delta should absorb.
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

// TestACFRp0041RevenueBlockCannotBeResolved runs the rule that would map the
// revenue block and asserts the resolver refuses it, naming the orphan.
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
// So the orphan "0.0" is the ONLY thing standing between this block and
// publication (fisc-hcus). If it acquires an honest declaration, this test goes
// red by succeeding, and the rule file's paragraph has to be rewritten.
func TestACFRp0041RevenueBlockCannotBeResolved(t *testing.T) {
	const yaml = `schema_version: 1
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
		t.Fatal("the revenue block now resolves; fisc-hcus is fixed and " +
			"mappings/livermore-acfr-fy2025.yaml should publish it")
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

// acfrLine returns the one page line whose label begins with want.
func acfrLine(t *testing.T, want string) string {
	t.Helper()
	b, err := os.ReadFile(acfrStatementText)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), want) {
			return line
		}
	}
	t.Fatalf("the fixture has no line beginning %q", want)
	return ""
}

// TestACFRp0041HasNoGeometryColumnGuard pins the claim that
// mappings/livermore-acfr-fy2025.yaml, data/taxonomy.yaml's neighbourhood and
// internal/check's unprojectedScopes entry all make in prose: the five figures
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
