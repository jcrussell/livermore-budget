package mapping

import (
	"math"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// The guards on a document-derived tolerance and on a total the page prints
// above its own rows. Each test mutates one working rule and asserts what
// still fails; acfr_p0041_test.go holds the good case against the real page.

// probeGG returns acfrGeneralGovernmentProbe with one substring replaced, and
// fails the test if the substring is not there. Silently replacing nothing is
// how a mutation test comes to assert the unmutated case.
func probeGG(t *testing.T, old, new string) *File {
	t.Helper()
	if !strings.Contains(acfrGeneralGovernmentProbe, old) {
		t.Fatalf("the probe rule no longer contains %q, so this mutation changes nothing", old)
	}
	f, err := parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe, old, new, 1)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// checkGG resolves the probe and returns whatever CheckTotals concluded.
func checkGG(t *testing.T, f *File) (*totalsResult, error) {
	t.Helper()
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r.CheckTotals(rule, &rule.Parts[0])
}

// TestPrintedDecimalsIsRequiredForTheBlockToTie is the baseline: with the
// declaration gone, the page's own $10,000 discrepancy fails the build.
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

// TestPrintedDecimalsOneDecimalFinerGoesRed is fisc-9hf's acceptance: three
// declared places make the bound $2,500 against the $10,000 error, and it goes
// red. One unit tighter cannot (the bound is 2.5 units). Driven through
// compareTotals because the witness refuses printed_decimals: 3 first.
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
	if got := err.Error(); !strings.Contains(got, "no token summed into a compared column prints that many") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsIsRefusedWhenNoColumnNeedsIt holds that a declaration
// loosening nothing is refused, on p41's exactly-tying Other Financing Sources.
func TestPrintedDecimalsIsRefusedWhenNoColumnNeedsIt(t *testing.T) {
	const yaml = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe-inert
    kind: transfer_in
    basis: audited
    scope: probe
    grain: fund-group-by-category
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
	f, err := parse(strings.NewReader(yaml), "probe.yaml")
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

// TestPrintedDecimalsIsRefusedOnDollarTables holds that dollar tables cannot
// declare a tolerance; their discrepancies (p127, pp.168-170) are declared
// exactly with stated_total_deltas.
func TestPrintedDecimalsIsRefusedOnDollarTables(t *testing.T) {
	_, err := parse(strings.NewReader(strings.NewReplacer(
		"units: millions", "units: dollars",
	).Replace(acfrGeneralGovernmentProbe)), "probe.yaml")
	if err == nil {
		t.Fatal("printed_decimals was accepted on a dollar-precision rule")
	}
	if got := err.Error(); !strings.Contains(got, "printed in dollars") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsAndStatedTotalDeltasCannotBothBeDeclared holds the exact
// and the bounded declaration disjoint.
func TestPrintedDecimalsAndStatedTotalDeltasCannotBothBeDeclared(t *testing.T) {
	_, err := parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe,
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
	_, err := parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe,
		`        section: "General Government:"`, `        section: "Current:"`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal("a total_row_above rule anchored somewhere other than its total was accepted")
	}
	if got := err.Error(); !strings.Contains(got, "requires it to be the total_row") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestTotalRowAboveStillRefusesABlockThatStartsTooEarly holds the leading-gap
// guard: the anchor moves one line up to "Current:", putting General
// Government's figures in the gap. Mutation: let checkGap skip every line
// rather than one and this goes green.
func TestTotalRowAboveStillRefusesABlockThatStartsTooEarly(t *testing.T) {
	f, err := parse(strings.NewReader(strings.NewReplacer(
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

// TestWrappedLabelsRefusesAnAmount holds that wrapped_labels: ["0.0"] on ACFR
// p41's revenue block is refused; unrefused it ties to the printed total.
func TestWrappedLabelsRefusesAnAmount(t *testing.T) {
	_, err := parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
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
	_, err := parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
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
	f, err := parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
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

// TestUnmappedTextIsRefusedOnALabellessPart is the same refusal wrapped_labels
// already carries: a positional part reads no gaps, so the
// declaration would be accepted and never looked at.
func TestUnmappedTextIsRefusedOnALabellessPart(t *testing.T) {
	const yaml = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe-labelless
    kind: revenue
    basis: audited
    scope: probe
    grain: fund-group-by-category
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
	_, err := parse(strings.NewReader(yaml), "probe.yaml")
	if err == nil {
		t.Fatal("unmapped_text was accepted on a labels_from part, where it is inert")
	}
	if got := err.Error(); !strings.Contains(got, "labels_from takes its row labels from page 41") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestToleranceIsSymmetric holds a negative diff to the same size bound.
// Mutation: make abs return c and a total ten million short ties; every corpus
// discrepancy is positive, so nothing else catches it.
func TestToleranceIsSymmetric(t *testing.T) {
	f := probeGG(t, "printed_decimals: 2", "printed_decimals: 2")
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	part := &rule.Parts[0]
	terms := []int{5, 5}
	// The real page: stated 18.45 against a mapped 18.44, diff +$10,000.
	stated := []amount.Cents{1_845_000_000, 1_600_000_000}
	within := []amount.Cents{1_844_000_000, 0}
	if _, err := r.compareTotals(rule, part, part, stated, within, terms); err != nil {
		t.Fatalf("a +$10,000 discrepancy should tie within a $25,000 bound: %v", err)
	}
	// The same magnitude the other way: mapped 18.46, diff -$10,000. Also within.
	over := []amount.Cents{1_846_000_000, 0}
	if _, err := r.compareTotals(rule, part, part, stated, over, terms); err != nil {
		t.Errorf("a -$10,000 discrepancy should tie too; the bound is symmetric: %v", err)
	}
	// And a NEGATIVE discrepancy far outside the bound must fail. This is the
	// assertion abs() exists for: without it, -$10,000,000 compares <= the
	// bound and passes.
	wild := []amount.Cents{2_845_000_000, 0}
	if _, err := r.compareTotals(rule, part, part, stated, wild, terms); err == nil {
		t.Error("a -$10,000,000 discrepancy tied within a $25,000 bound; the " +
			"comparison is not taking a magnitude")
	}
	// Symmetric in size as well as sign: +$10,000,000 must fail identically.
	wildPositive := []amount.Cents{845_000_000, 0}
	if _, err := r.compareTotals(rule, part, part, stated, wildPositive, terms); err == nil {
		t.Error("a +$10,000,000 discrepancy tied within a $25,000 bound")
	}
}

// TestToleranceScalesWithTheTermCount pins half a printed unit per row.
// Mutation: ignore the count and the bound no longer varies with block size.
func TestToleranceScalesWithTheTermCount(t *testing.T) {
	f := probeGG(t, "printed_decimals: 2", "printed_decimals: 2")
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	part := &rule.Parts[0]
	// A $30,000 discrepancy: three printed units. Six terms give a $30,000
	// bound and it ties; five give $25,000 and it does not.
	stated := []amount.Cents{1_847_000_000, 0}
	sums := []amount.Cents{1_844_000_000, 0}
	if _, err := r.compareTotals(rule, part, part, stated, sums, []int{6, 6}); err != nil {
		t.Errorf("six terms give a $30,000 bound, which admits $30,000: %v", err)
	}
	if _, err := r.compareTotals(rule, part, part, stated, sums, []int{5, 5}); err == nil {
		t.Error("five terms give a $25,000 bound and admitted $30,000; the bound " +
			"does not scale with the term count")
	}
}

// TestTotalRowAboveRefusesARowOnTheTotalsOwnLine holds a first row "18.45"
// on the anchor's own line refused. Mutation: skip the newline with
// strings.Cut and the empty remainder passes the digit refusal vacuously.
func TestTotalRowAboveRefusesARowOnTheTotalsOwnLine(t *testing.T) {
	f := probeGG(t, `      - {label: "City Council", category: general-government}`,
		`      - {label: "16.00", category: general-government}`)
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, _, err = r.Values(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("a row on the total's own line resolved; the no-newline gap is " +
			"passing the digit refusal vacuously")
	}
	if got := err.Error(); !strings.Contains(got, "is on the same printed line as the total") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestUnmappedTextIsHonouredBetweenRows drops the Miscellaneous row so the
// orphan "0.0" sits between mapped rows; p41's sits after the last. Mutation:
// delete the between-rows branch and only this fails.
func TestUnmappedTextIsHonouredBetweenRows(t *testing.T) {
	const yaml = `schema_version: 1
doc_id: livermore-acfr-fy2025
rules:
  - id: probe-midblock
    kind: revenue
    basis: audited
    scope: probe
    grain: fund-group-by-category
    units: millions
    parts:
      - page: 41
        # Anchored on the WHOLE of the preceding printed line, figures and all,
        # so the leading gap is whitespace and the orphan is reached by the
        # between-rows arm rather than the leading one. The parser refuses an
        # anchor amount.Parse accepts; it accepts this one, because a line of
        # several tokens is not an amount.
        section: "Use of money and property                                     12.81             10.40"
        stop_at: "Expenditures"
        unmapped_text:
          - text: "0.0"
            note: "the orphan, now sitting between two mapped rows"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}
    rows:
      - {label: "Miscellaneous", category: miscellaneous-revenue}
      - {label: "Total Revenues", skip: true}
`
	f, err := parse(strings.NewReader(yaml), "probe.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := &f.Rules[0]
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if _, _, verr := r.Values(rule, &rule.Parts[0]); verr != nil {
		t.Fatalf("the declared orphan was not honoured between two rows: %v", verr)
	}

	// And without the declaration the same read is refused, so the assertion
	// above is about the declaration rather than about the block.
	bare, err := parse(strings.NewReader(strings.Replace(yaml,
		`        unmapped_text:
          - text: "0.0"
            note: "the orphan, now sitting between two mapped rows"
`, "", 1)), "probe.yaml")
	if err != nil {
		t.Fatalf("parse without the declaration: %v", err)
	}
	r2, err := NewResolver(testDoc(t, acfrFixtures, []int{acfrStatementPage}), bare)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if _, _, verr := r2.Values(&bare.Rules[0], &bare.Rules[0].Parts[0]); verr == nil {
		t.Error("the block resolved with no declaration; the orphan is being " +
			"admitted by something other than unmapped_text")
	}
}

// TestUnmappedTextParseRefusals is the parse-time refusals on both new
// declarations. Mutation: delete any of the unmapped_text blank, whitespace,
// missing-note or duplicate arms, or printed_decimals' negative or multi-part
// arm, and its case fails.
func TestUnmappedTextParseRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		want  string
	}{
		{
			name:  "blank text",
			block: "        unmapped_text:\n          - {text: \"  \", note: \"x\"}",
			want:  "has an entry with no text",
		},
		{
			name:  "untrimmed text",
			block: "        unmapped_text:\n          - {text: \" 0.0\", note: \"x\"}",
			want:  "has leading or trailing whitespace",
		},
		{
			name:  "no note",
			block: "        unmapped_text:\n          - {text: \"0.0\"}",
			want:  "has no note",
		},
		{
			name:  "whitespace note",
			block: "        unmapped_text:\n          - {text: \"0.0\", note: \"   \"}",
			want:  "has no note",
		},
		{
			name:  "duplicate",
			block: "        unmapped_text:\n          - {text: \"0.0\", note: \"x\"}\n          - {text: \"0.0\", note: \"y\"}",
			want:  "is listed twice",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
				"        columns:", tc.block+"\n        columns:", 1)), "probe.yaml")
			if err == nil {
				t.Fatalf("accepted: %s", tc.block)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused for some other reason: %v", err)
			}
		})
	}
}

// TestWrappedLabelsParseRefusals covers the arms unmapped_text's were written as
// mirrors of, which review pointed out had never been exercised themselves.
func TestWrappedLabelsParseRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		want  string
	}{
		{"blank", `        wrapped_labels: ["  "]`, "has a blank entry"},
		{"untrimmed", `        wrapped_labels: [" Devel"]`, "has leading or trailing whitespace"},
		{"duplicate", `        wrapped_labels: ["Devel", "Devel"]`, "is listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(strings.Replace(acfrRevenueProbe,
				"        columns:", tc.block+"\n        columns:", 1)), "probe.yaml")
			if err == nil {
				t.Fatalf("accepted: %s", tc.block)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused for some other reason: %v", err)
			}
		})
	}
}

// TestPrintedDecimalsParseRefusals covers the two arms review found
// mutation-green, plus the units bound.
func TestPrintedDecimalsParseRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, want string
	}{
		{
			name: "negative", from: "printed_decimals: 2", to: "printed_decimals: -1",
			want: "a count of printed decimal places cannot be negative",
		},
		{
			name: "finer than the units hold", from: "printed_decimals: 2", to: "printed_decimals: 9",
			want: "can represent only 8 decimal places exactly",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(acfrGeneralGovernmentProbe, tc.from) {
				t.Fatalf("the probe no longer contains %q", tc.from)
			}
			_, err := parse(strings.NewReader(
				strings.Replace(acfrGeneralGovernmentProbe, tc.from, tc.to, 1)), "probe.yaml")
			if err == nil {
				t.Fatalf("accepted %s -> %q", tc.from, tc.to)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused for some other reason: %v", err)
			}
			// The FIELD, not just the sentence: two validators in this file
			// refuse a missing total_row in the same words.
			if !strings.Contains(err.Error(), "printed_decimals:") {
				t.Errorf("refused by some other validator: %v", err)
			}
		})
	}
}

// TestPrintedDecimalsWithoutATotalRowIsRefusedByItsOwnArm drops
// total_row_above too and re-anchors, since validateTotalRowAbove refuses a
// missing total_row first in the same words; the field name tells them apart.
func TestPrintedDecimalsWithoutATotalRowIsRefusedByItsOwnArm(t *testing.T) {
	src := strings.Replace(acfrGeneralGovernmentProbe, `    total_row: "General Government:"`+"\n", "", 1)
	src = strings.Replace(src, "    total_row_above: true\n", "", 1)
	src = strings.Replace(src, `        section: "General Government:"`, `        section: "Current:"`, 1)
	_, err := parse(strings.NewReader(src), "probe.yaml")
	if err == nil {
		t.Fatal("printed_decimals was accepted on a rule with no total_row")
	}
	if got := err.Error(); !strings.Contains(got, "printed_decimals: declared without a total_row") {
		t.Errorf("refused by some other arm: %v", got)
	}
}

// TestPrintedDecimalsIsRefusedOnAMultiPartRuleThatDoesNotSpan holds that a
// multi-part rule needs total_spans_parts, or would be compared per part and
// refused as inert on a part that ties exactly.
func TestPrintedDecimalsIsRefusedOnAMultiPartRuleThatDoesNotSpan(t *testing.T) {
	_, err := parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe,
		`          - {fiscal_year: 2024, skip: true}`,
		`          - {fiscal_year: 2024, skip: true}
      - page: 42
        section: "General Government:"
        stop_at: "Fire"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal("printed_decimals was accepted on a two-part rule whose total does not span")
	}
	if got := err.Error(); !strings.Contains(got, "whose total does not span its parts") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestPrintedDecimalsAndUnmappedTextCannotBothBeDeclared holds that a
// tolerance cannot sit beside unmapped_text, where it would absorb the
// shortfall of a wrongly removed figure.
func TestPrintedDecimalsAndUnmappedTextCannotBothBeDeclared(t *testing.T) {
	_, err := parse(strings.NewReader(strings.Replace(acfrGeneralGovernmentProbe,
		"        columns:", `        unmapped_text:
          - {text: "0.0", note: "a figure declared out of a block that also rounds"}
        columns:`, 1)), "probe.yaml")
	if err == nil {
		t.Fatal("a rule declaring both an unmapped figure and a tolerance was accepted")
	}
	if got := err.Error(); !strings.Contains(got, "unmapped_text: declared on a rule that also declares printed_decimals") {
		t.Errorf("refused for some other reason: %v", got)
	}
}

// TestTotalRowAboveRefusals covers two arms of validateTotalRowAbove.
// Mutation: delete either and its case fails.
func TestTotalRowAboveRefusals(t *testing.T) {
	t.Run("with total_spans_parts", func(t *testing.T) {
		// A second part as well, because validateTotalSpansParts requires two
		// and would otherwise refuse first -- which would leave this passing on
		// the wrong arm, the mistake this range already made once.
		src := strings.Replace(acfrGeneralGovernmentProbe,
			"    total_row_above: true", "    total_row_above: true\n    total_spans_parts: true", 1)
		src = strings.Replace(src, `          - {fiscal_year: 2024, skip: true}`,
			`          - {fiscal_year: 2024, skip: true}
      - page: 42
        section: "General Government:"
        stop_at: "Fire"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}`, 1)
		_, err := parse(strings.NewReader(src), "probe.yaml")
		if err == nil {
			t.Fatal("total_row_above was accepted with total_spans_parts")
		}
		if got := err.Error(); !strings.Contains(got, "total_row_above: declared with total_spans_parts") {
			t.Errorf("refused for some other reason: %v", got)
		}
	})

	t.Run("with labels_from", func(t *testing.T) {
		src := strings.Replace(acfrGeneralGovernmentProbe, `          - {fiscal_year: 2024, skip: true}`,
			`          - {fiscal_year: 2024, skip: true}
      - page: 42
        labels_from: 41
        section: "General Government:"
        stop_at: "Fire"
        columns:
          - {fund_group: general, fiscal_year: 2025}
          - {fiscal_year: 2024, skip: true}`, 1)
		// printed_decimals refuses a multi-part rule of its own accord, so it
		// comes off: without this the test would pass on that arm instead.
		src = strings.Replace(src, "    printed_decimals: 2\n", "", 1)
		_, err := parse(strings.NewReader(src), "probe.yaml")
		if err == nil {
			t.Fatal("total_row_above was accepted on a rule with a labels_from part")
		}
		if got := err.Error(); !strings.Contains(got, "declared on a rule with total_row_above") {
			t.Errorf("refused for some other reason: %v", got)
		}
	})
}

// TestTotalRowAboveAllowsAWrappedLabelAfterTheTotalsLine holds checkGap to
// skipping the total's line before the wrapped_labels test. Calls checkGap
// directly, as the claim is the order of two tests inside it. Mutation: test
// wrapped labels against the whole gap and the declared case fails as stale.
func TestTotalRowAboveAllowsAWrappedLabelAfterTheTotalsLine(t *testing.T) {
	rule := &Rule{TotalRow: "Total Revenues", TotalRowAbove: true}
	rows := []Row{{Label: "General Government:"}}
	res := &Resolver{file: &File{DocID: "probe"}}
	const gap = "   157.20   145.50\n      Current:\n   "

	used := map[string]bool{}
	if err := res.checkGap(rule, &Part{Page: 41, WrappedLabels: []string{"Current:"}},
		gap, rows, 0, used, nil); err != nil {
		t.Fatalf("a wrapped label after the total's line was refused: %v", err)
	}
	if !used["Current:"] {
		t.Error("the wrapped label was accepted but not marked used, so it would " +
			"fail later as a stale declaration")
	}

	// Undeclared, the same fragment is accepted and nothing is marked used;
	// the digit refusal is the only guard over it.
	used = map[string]bool{}
	if err := res.checkGap(rule, &Part{Page: 41}, gap, rows, 0, used, nil); err != nil {
		t.Errorf("undeclared words after the total's line were refused: %v", err)
	}
	if len(used) != 0 {
		t.Errorf("undeclared words were marked used: %v", used)
	}
	if err := res.checkGap(&Rule{TotalRow: "Total Revenues"}, &Part{Page: 41},
		"      Current:\n   ", rows, 0, map[string]bool{}, nil); err != nil {
		t.Errorf("undeclared leading words were refused on a rule without "+
			"total_row_above, so the arm above compares against nothing: %v", err)
	}
	// FIGURES on the line after the total's are refused, which is the guard the
	// skip must not clear.
	if err := res.checkGap(rule, &Part{Page: 41},
		"   157.20   145.50\n      18.45\n", rows, 0, map[string]bool{}, nil); err == nil {
		t.Error("figures on the line AFTER the total's were accepted; the skip is " +
			"clearing more than one line")
	}
}

// TestWithinToleranceRefusesRatherThanWrapping holds the bound fail-closed:
// an inline `abs(diff)*2 <= unit*terms` wraps negative near 9.2e18 cents, and
// math.MinInt64 doubles to 0, and either would tie.
func TestWithinToleranceRefusesRatherThanWrapping(t *testing.T) {
	const unit = amount.Cents(1_000_000) // $10,000, ACFR p41's printed digit
	for _, tc := range []struct {
		name  string
		diff  amount.Cents
		unit  amount.Cents
		terms int
		want  bool
	}{
		{"the real page", 1_000_000, unit, 5, true},
		{"the real page, other sign", -1_000_000, unit, 5, true},
		{"exactly on the bound", 2_500_000, unit, 5, true},
		{"one cent past it", 2_500_001, unit, 5, false},
		{"one cent past it, other sign", -2_500_001, unit, 5, false},
		// The wrapping cases.
		{"a diff that doubles past MaxInt64", math.MaxInt64 - 1, unit, 5, false},
		{"MinInt64, whose abs is itself", amount.Cents(math.MinInt64), unit, 5, false},
		{"MaxInt64", amount.Cents(math.MaxInt64), unit, 5, false},
		// A bound that would itself wrap.
		{"a term count that overflows the bound", 1_000_000, math.MaxInt64, 3, false},
		// Degenerate inputs: no declaration, or nothing summed.
		{"no unit", 1_000_000, 0, 5, false},
		{"no terms", 0, unit, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinTolerance(tc.diff, tc.unit, tc.terms); got != tc.want {
				t.Errorf("withinTolerance(%d, %d, %d) = %v, want %v",
					tc.diff, tc.unit, tc.terms, got, tc.want)
			}
		})
	}
}
