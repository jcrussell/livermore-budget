package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// The guards on a document-derived tolerance (fisc-1wr.2) and on a total the
// page prints above its own rows (fisc-h96o).
//
// EVERY TEST HERE IS A MUTATION OF ONE WORKING RULE, and that is deliberate.
// Both mechanisms open a guard that stood for a reason, so the question worth
// answering is not "does the good case pass" -- acfr_p0041_test.go answers that
// against the real page -- but "what still fails". A test that only exercised
// the happy path would pass identically against a flag that opened everything.

// probeGG returns acfrGeneralGovernmentProbe with one substring replaced, and
// fails the test if the substring is not there. Silently replacing nothing is
// how a mutation test comes to assert the unmutated case.
func probeGG(t *testing.T, old, new string) *File {
	t.Helper()
	if !strings.Contains(acfrGeneralGovernmentProbe, old) {
		t.Fatalf("the probe rule no longer contains %q, so this mutation changes nothing", old)
	}
	f, err := Parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe, old, new, 1)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// checkGG resolves the probe and returns whatever CheckTotals concluded.
func checkGG(t *testing.T, f *File) (*TotalsResult, error) {
	t.Helper()
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r.CheckTotals(rule, &rule.Parts[0])
}

// TestPrintedDecimalsIsRequiredForTheBlockToTie is the baseline the rest of
// this file mutates away from: with the declaration gone, the page's own $10,000
// discrepancy fails the build.
//
// It is the plainest statement that the tolerance is load-bearing rather than
// decorative, and it is the reverse of the usual worry -- here the risk is a
// tolerance that does nothing, not one that hides something.
func TestPrintedDecimalsIsRequiredForTheBlockToTie(t *testing.T) {
	_, err := checkGG(t, probeGG(t, "    printed_decimals: 2\n", ""))
	if err == nil {
		t.Fatal("the block ties with no tolerance declared; the page's 18.44 against a " +
			"printed 18.45 has gone away and every comment quoting it is stale")
	}
	// The message must be the totals one, naming both figures. A test satisfied
	// by any error would pass on a mistyped anchor.
	for _, want := range []string{"mapped $18,440,000.00", "document states $18,450,000.00", "off by $10,000.00"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
}

// TestPrintedDecimalsOneDecimalFinerGoesRed is fisc-9hf's acceptance criterion,
// under the reading that turned out to be the satisfiable one.
//
// THE LITERAL READING -- "a tolerance ONE UNIT too tight goes red" -- IS NOT
// SATISFIABLE ON THIS CORPUS, which is worth recording because both fisc-9hf and
// fisc-1wr.2 are written in those words. The derived bound here is 2.5 units
// against a 1-unit error, so subtracting a unit still passes; every other
// candidate the corpus offers is looser still. What does go red is one DECIMAL
// finer: declaring three places makes the unit $1,000 and the bound $2,500,
// against the same $10,000 error.
//
// IT IS DRIVEN THROUGH compareTotals RATHER THAN THROUGH YAML, and the reason is
// itself a guard working: printed_decimals: 3 never reaches the arithmetic,
// because the witness refuses a declaration no token bears out first. That
// refusal is asserted separately below. Reaching past it here is what lets the
// bound itself be tested.
func TestPrintedDecimalsOneDecimalFinerGoesRed(t *testing.T) {
	f := probeGG(t, "printed_decimals: 2", "printed_decimals: 2")
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	part := &rule.Parts[0]
	stated, err := r.StatedTotals(rule, part)
	if err != nil {
		t.Fatalf("StatedTotals: %v", err)
	}
	values, _, err := r.Values(rule, part)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	sums := make([]amount.Cents, len(part.Columns))
	terms := make([]int, len(part.Columns))
	for _, v := range values {
		sums[v.ColumnIndex] += v.Cents
		terms[v.ColumnIndex]++
	}
	if got, want := terms[0], 5; got != want {
		t.Fatalf("summed %d terms, want %d; the bound scales with this count", got, want)
	}

	// Two decimals: $10,000 unit, $25,000 bound, tolerated.
	if _, err := r.compareTotals(rule, part, part, stated, sums, terms); err != nil {
		t.Fatalf("at 2 decimals the column should tie within the bound: %v", err)
	}
	// Three: $1,000 unit, $2,500 bound, red.
	finer := 3
	rule.PrintedDecimals = &finer
	if _, err := r.compareTotals(rule, part, part, stated, sums, terms); err == nil {
		t.Error("a bound of $2,500 accepted a $10,000 discrepancy; the tolerance " +
			"is not derived from printed_decimals at all")
	}
}

// TestPrintedDecimalsRefusesATokenFinerThanDeclared is the unsafe direction.
// Under-declaring makes the unit coarser and the bound WIDER, so it is the way
// round that would quietly buy slack.
func TestPrintedDecimalsRefusesATokenFinerThanDeclared(t *testing.T) {
	_, err := checkGG(t, probeGG(t, "printed_decimals: 2", "printed_decimals: 1"))
	if err == nil {
		t.Fatal("a rule declaring one decimal over two-decimal tokens was accepted; " +
			"its bound would be ten times what the page justifies")
	}
	if got := err.Error(); !strings.Contains(got, "carries 2 decimal place(s), but the rule declares 1") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsRefusesADeclarationNoTokenBearsOut is the stale direction,
// and it is why the test above it has to reach past the witness.
func TestPrintedDecimalsRefusesADeclarationNoTokenBearsOut(t *testing.T) {
	_, err := checkGG(t, probeGG(t, "printed_decimals: 2", "printed_decimals: 3"))
	if err == nil {
		t.Fatal("a declaration of three decimals was accepted over a page printing two")
	}
	if got := err.Error(); !strings.Contains(got, "no token this rule reads prints that many") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsIsRefusedWhenNoColumnNeedsIt is the mirror of the stale
// stated_total_delta arm: a declaration that has stopped loosening anything is
// the one shape a declaration in this repository must not have.
//
// The subject is p41's Other Financing Sources block, which ties EXACTLY.
func TestPrintedDecimalsIsRefusedWhenNoColumnNeedsIt(t *testing.T) {
	const yaml = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe-inert
    kind: transfer_in
    basis: audited
    scope: probe
    units: millions
    printed_decimals: 2
    total_row: "Total Other Financing Sources (Uses)"
    parts:
      - page: 41
        section: "Other Financing Sources (Uses)"
        section_ordinal: 1
        stop_at: "Total Other Financing Sources (Uses)"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}
    rows:
      - {label: "Transfers in", category: transfers/in}
      - {label: "Transfers (out)", category: transfers/out, kind: transfer_out}
`
	f, err := Parse(strings.NewReader(yaml), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, err = r.CheckTotals(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("a tolerance was accepted on a block that ties exactly; nothing " +
			"would ever retire it")
	}
	if got := err.Error(); !strings.Contains(got, "tie exactly") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsIsRefusedOnDollarTables keeps the dollar branch from
// existing at all. It would have no consumer here, and the dollar-level
// discrepancies it looks like it would cover -- p127's $1 over thirteen rows,
// six more on pp.168-170 -- are the city's own arithmetic, declared exactly
// with stated_total_deltas (fisc-2sd) rather than bounded.
func TestPrintedDecimalsIsRefusedOnDollarTables(t *testing.T) {
	_, err := Parse(strings.NewReader(strings.NewReplacer(
		"units: millions", "units: dollars",
	).Replace(acfrGeneralGovernmentProbe)), "probe.yaml")
	if err == nil {
		t.Fatal("printed_decimals was accepted on a dollar-precision rule")
	}
	if got := err.Error(); !strings.Contains(got, "printed in dollars") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsAndStatedTotalDeltasCannotBothBeDeclared keeps the two
// disjoint by construction rather than by prose. One names the exact figure a
// document is out by; the other bounds an unnamed one. A rule holding both
// offers somewhere to hide the difference.
func TestPrintedDecimalsAndStatedTotalDeltasCannotBothBeDeclared(t *testing.T) {
	_, err := Parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe,
		"        columns:", `        stated_total_deltas:
          - {column: 1, delta_cents: 1000000, note: "the same discrepancy, named"}
        columns:`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal("a rule declaring both a delta and a tolerance was accepted")
	}
	if got := err.Error(); !strings.Contains(got, "also declares printed_decimals") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestTotalRowAboveRequiresTheSectionAnchorToBeTheTotalRow is the whole of what
// keeps that flag from being a loosening. Without it the flag would mean "the
// total is somewhere above", which no guard in this package could bound.
func TestTotalRowAboveRequiresTheSectionAnchorToBeTheTotalRow(t *testing.T) {
	_, err := Parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe,
		`        section: "General Government:"`, `        section: "Current:"`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal("a total_row_above rule anchored somewhere other than its total was accepted")
	}
	if got := err.Error(); !strings.Contains(got, "requires it to be the total_row") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestTotalRowAboveStillRefusesABlockThatStartsTooEarly is the test that says
// the leading-gap guard survived.
//
// IT IS THE ONE THAT MATTERS, and deleting the checkGap arm would not fail it --
// that mutation makes the good case red, which the p41 tests catch. What this
// catches is the OPPOSITE mistake: an arm written permissively enough to skip
// every line rather than one, which would leave "the block starts too early"
// with nothing to stand on and would look identical from the good case's exit
// code.
//
// The mutation moves the anchor one printed line up, to the "Current:" heading,
// so General Government's own figures are now inside the gap AFTER the skipped
// line. Both anchors are renamed together, so the rule is internally consistent
// and only its position on the page is wrong.
func TestTotalRowAboveStillRefusesABlockThatStartsTooEarly(t *testing.T) {
	f, err := Parse(strings.NewReader(strings.NewReplacer(
		`total_row: "General Government:"`, `total_row: "Current:"`,
		`section: "General Government:"`, `section: "Current:"`,
	).Replace(acfrGeneralGovernmentProbe)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, _, err = r.Values(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("a block anchored a line too early resolved; total_row_above's " +
			"newline skip is clearing more than the total's own line")
	}
	if got := err.Error(); !strings.Contains(got, `figures appear before the first row "City Council"`) {
		t.Errorf("refused for some other reason: %v", got)
	}
	// And the refusal quotes the subtotal line, which is the evidence that the
	// skip cleared exactly one line and stopped.
	if got := err.Error(); !strings.Contains(got, "General Government:") {
		t.Errorf("the refusal does not quote the line that was not skipped: %v", got)
	}
}

// TestWrappedLabelsRefusesAnAmount closes the dishonest route to publishing an
// orphan figure, and it is the reason fisc-hcus was worth landing rather than
// declining.
//
// MEASURED BEFORE THE REFUSAL EXISTED: wrapped_labels: ["0.0"] on ACFR p41's
// revenue block parsed, resolved all ten rows and tied to the printed total,
// with nothing objecting. The mapping file told a human not to do it and the
// parser did not care. An honest declaration is worth nothing while the
// dishonest one still works.
func TestWrappedLabelsRefusesAnAmount(t *testing.T) {
	_, err := Parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
		"        columns:", `        wrapped_labels: ["0.0"]
        columns:`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal(`wrapped_labels: ["0.0"] was accepted; a figure can still be published ` +
			"under a declaration asserting the page wrapped a label")
	}
	if got := err.Error(); !strings.Contains(got, `"0.0" is a currency amount`) {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestUnmappedTextRefusesTextThatIsNotAFigure is the other half of the pair
// above: neither declaration can stand in for the other.
func TestUnmappedTextRefusesTextThatIsNotAFigure(t *testing.T) {
	_, err := Parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
		"        columns:", `        unmapped_text:
          - text: "Miscellaneous"
            note: "a row label, which is not what this declares"
        columns:`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal("unmapped_text accepted a row label; it is meant for figures only")
	}
	if got := err.Error(); !strings.Contains(got, "is not a figure") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestUnmappedTextRefusesAStaleDeclaration is the only thing standing over an
// unmapped_text entry, which is why it is asserted rather than assumed. See
// Part.UnmappedText on why this is the weakest declaration class here.
func TestUnmappedTextRefusesAStaleDeclaration(t *testing.T) {
	f, err := Parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
		"        columns:", `        unmapped_text:
          - text: "0.0"
            note: "the real one"
          - text: "9.9"
            note: "a figure this page does not print"
        columns:`, 1)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if _, _, err = r.Values(rule, &rule.Parts[0]); err == nil {
		t.Fatal("a declared figure the page does not print was accepted")
	}
	if got := err.Error(); !strings.Contains(got, `"9.9" is declared but does not appear`) {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestUnmappedTextRefusesADeclarationOnALabellessPart is the same refusal
// wrapped_labels already carries (fisc-ekj): a positional part reads no gaps, so
// the declaration would be accepted and never looked at.
func TestUnmappedTextIsRefusedOnALabellessPart(t *testing.T) {
	const yaml = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe-labelless
    kind: revenue
    basis: audited
    scope: probe
    units: millions
    parts:
      - page: 41
        section: "Revenues"
        section_ordinal: 2
        stop_at: "Total Revenues"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}
      - page: 42
        labels_from: 41
        stop_at: "Total revenues"
        unmapped_text:
          - text: "0.0"
            note: "declared where nothing reads it"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}
    rows:
      - {label: "Miscellaneous", category: miscellaneous-revenue}
`
	_, err := Parse(strings.NewReader(yaml), "probe.yaml")
	if err == nil {
		t.Fatal("unmapped_text was accepted on a labels_from part, where it is inert")
	}
	if got := err.Error(); !strings.Contains(got, "labels_from takes its row labels from page 41") {
		t.Errorf("refused for some other reason: %v", got)
	}
}
