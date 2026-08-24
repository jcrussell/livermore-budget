package mapping

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// p127PropertyTaxes is the Property Taxes block of Budget Book p127, mapped
// against the committed page fixture. It is the smallest real rule that cannot
// build without a declared delta, which is what makes it the test for one.
//
// Four columns, and only the first needs the declaration: the FY2023-24 Actual
// column prints a total $1 higher than its thirteen rows add to, and the other
// three tie exactly. The block is bounded by "Property Taxes" and its own total
// line, so nothing below it is read.
const p127PropertyTaxes = `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: gf-property-taxes
    kind: revenue
    basis: adopted
    scope: all-funds-gross
    units: dollars
    total_row: "Total Property Taxes"
    rows:
      - {label: "Current Year - Secured", category: taxes/property}
      - {label: "Prior Year - Secured", category: taxes/property}
      - {label: "ERAF", category: taxes/property, sign: contra}
      - {label: "RPTTF Reduction", category: taxes/property, sign: contra}
      - {label: "Current Year - Unsecured", category: taxes/property}
      - {label: "Prior Year - Unsecured", category: taxes/property}
      - {label: "Supple - Sec Roll Current", category: taxes/property}
      - {label: "VLF Comp Fund", category: taxes/property}
      - {label: "Unitary Utility Tax", category: taxes/property}
      - {label: "Aircraft Taxes", category: taxes/property}
      - {label: "RPTTF Receipts & Other PropTax", category: taxes/property}
      - {label: "St Homeowner Prop Tax Re", category: taxes/property}
      - {label: "Pen & Int On Delinq Tax", category: taxes/property}
    parts:
      - page: 127
        section: "Property Taxes"
        section_ordinal: 1
        stop_at: "Total Property Taxes"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2024, basis: actual}
          - {fund_group: general, fund: 100, fiscal_year: 2025, basis: revised}
          - {fund_group: general, fund: 100, fiscal_year: 2026}
          - {fund_group: general, fund: 100, fiscal_year: 2027}
        #DELTAS
`

// declaration is the stated_total_deltas block the rule needs, indented to sit
// under the part. Tests substitute it for the #DELTAS placeholder line, or
// leave the placeholder in place to get the undeclared case. The placeholder is
// a comment rather than a format verb because a bare "%s" at the start of a
// line is a YAML directive marker.
const declaration = `        stated_total_deltas:
          - column: 1
            delta_cents: 100
            note: >-
              The city's own rounding. The thirteen rows print 58,179,467 and
              the document states 58,179,468; the other three columns tie
              exactly, so the difference is in the published figure and not in
              the read.`

func p127Resolver(t *testing.T, deltas string) (*Resolver, *Rule) {
	t.Helper()
	src := strings.Replace(p127PropertyTaxes, "        #DELTAS", deltas, 1)
	f, err := Parse(strings.NewReader(src), "p127.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 127), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// TestP127NeedsItsDeltaDeclared carries the arithmetic, because a claim about
// these documents is only as good as the figures behind it. Summed from the
// committed fixture, the thirteen Property Taxes rows of Budget Book p127:
//
//	FY2023-24 Actual   58,179,467   printed 58,179,468   off by +1
//	FY2024-25 Revised  60,621,418   printed 60,621,418   exact
//	FY2025-26 Budget   64,143,762   printed 64,143,762   exact
//	FY2026-27 Budget   67,416,730   printed 67,416,730   exact
//
// Three of four columns tying exactly is what makes the fourth the document's
// rounding rather than our read: a misread row would not land on three columns
// and miss one by a dollar. This is the page fisc-2sd was opened for, and the
// reason "all column totals tie exactly" must not be hardened into a property
// of this corpus -- it is a property of pp.66-67 alone.
func TestP127NeedsItsDeltaDeclared(t *testing.T) {
	t.Run("undeclared, it fails closed", func(t *testing.T) {
		r, ru := p127Resolver(t, "")
		_, err := r.CheckTotals(ru, &ru.Parts[0])
		if err == nil {
			t.Fatal("CheckTotals succeeded; an undeclared discrepancy must still fail")
		}
		got := diagnosis(t, err)
		for _, want := range []string{"column 1", "58,179,467", "58,179,468", "stated_total_deltas"} {
			if !strings.Contains(got, want) {
				t.Errorf("error does not mention %q:\n%s", want, got)
			}
		}
	})

	t.Run("declared, it ties", func(t *testing.T) {
		r, ru := p127Resolver(t, declaration)
		res, err := r.CheckTotals(ru, &ru.Parts[0])
		if err != nil {
			t.Fatalf("CheckTotals with the delta declared: %v", err)
		}
		if res.Columns != 4 {
			t.Errorf("Columns = %d, want 4", res.Columns)
		}
		// The declared column is reported separately so a part that ties on
		// the document's rounding cannot read as one that ties exactly.
		if res.Declared != 1 {
			t.Errorf("Declared = %d, want 1", res.Declared)
		}
	})
}

// TestTheUndeclaredFailureStatesTheDeltaToPasteIn is fisc-7a6, and p127 is the
// fixture that shows why the sign is not cosmetic.
//
// compareTotals computes `diff := stated - mapped` under a comment saying the
// direction is chosen so a failure can be pasted into a stated_total_deltas
// declaration -- and then printed `mapped - stated`, the negation of what it
// had just computed. Its own next branch printed `diff`, so ONE FUNCTION
// REPORTED THE SAME QUANTITY IN TWO DIRECTIONS three lines apart.
//
// The consequence is not a cosmetic slip, it is a loop. p127 column 1 maps
// 58,179,467 against a printed 58,179,468. An author reading the old message
// declared delta_cents: -100, and got back "declared delta -$1.00 but the
// difference is $1.00" -- the second branch, correctly signed, contradicting
// the first. The number in the failure was the one number that could not be
// pasted in.
//
// So this test pins the two branches AGAINST EACH OTHER rather than against a
// literal. A future edit that re-inverts either one has to make them disagree
// to pass, and that is the defect itself.
func TestTheUndeclaredFailureStatesTheDeltaToPasteIn(t *testing.T) {
	r, ru := p127Resolver(t, "")
	_, err := r.CheckTotals(ru, &ru.Parts[0])
	if err == nil {
		t.Fatal("CheckTotals succeeded; an undeclared discrepancy must fail")
	}
	got := diagnosis(t, err)
	if !strings.Contains(got, "off by $1.00") {
		t.Errorf("the undeclared failure does not offer the delta to declare:\n%s", got)
	}
	// The negation must be absent, not merely outranked by a substring match:
	// "off by $1.00" is not a substring of "off by -$1.00", but asserting its
	// absence is what states the claim.
	if strings.Contains(got, "off by -$1.00") {
		t.Errorf("the undeclared failure reports the delta with the sign inverted:\n%s", got)
	}

	// Now paste it in. The declaration the message just offered must be the one
	// that ties -- which is the whole content of "a failure message can be
	// pasted into a declaration", and what the inverted sign made false.
	r, ru = p127Resolver(t, declaration)
	if _, err := r.CheckTotals(ru, &ru.Parts[0]); err != nil {
		t.Fatalf("the delta the failure named does not tie: %v", err)
	}
	if !strings.Contains(declaration, "delta_cents: 100") {
		t.Fatal("this test assumes the fixture declares +100; it no longer does")
	}
}

// TestDeclaredDeltaAcceptsThatFigureAndNoOther is the other half of fisc-2sd's
// acceptance criteria. A declaration is a claim about one exact figure, so
// every neighbouring figure must still fail -- otherwise it is a tolerance
// with extra steps.
func TestDeclaredDeltaAcceptsThatFigureAndNoOther(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cents amount.Cents
		want  string
	}{
		{"a dollar too generous", 200, "declared delta $2.00 but the difference is $1.00"},
		{"the right size, wrong sign", -100, "declared delta -$1.00 but the difference is $1.00"},
		{"wildly wrong", 500_00, "declared delta $500.00 but the difference is $1.00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := strings.Replace(declaration, "delta_cents: 100",
				"delta_cents: "+amountLiteral(tc.cents), 1)
			r, ru := p127Resolver(t, d)
			_, err := r.CheckTotals(ru, &ru.Parts[0])
			if err == nil {
				t.Fatalf("CheckTotals accepted a delta of %s; only the declared figure may pass", tc.cents)
			}
			if got := diagnosis(t, err); !strings.Contains(got, tc.want) {
				t.Errorf("error does not say %q:\n%s", tc.want, got)
			}
		})
	}
}

// TestDeclaredDeltaOnAColumnThatTiesFails guards the direction nothing else
// would catch. If the city reissues p127 with the arithmetic corrected, the
// declaration becomes a false statement about the document -- and a check that
// let it pass would be carrying a stale claim forever. Column 2 already ties.
func TestDeclaredDeltaOnAColumnThatTiesFails(t *testing.T) {
	d := strings.Replace(declaration, "column: 1", "column: 2", 1)
	r, ru := p127Resolver(t, d)
	_, err := r.CheckTotals(ru, &ru.Parts[0])
	if err == nil {
		t.Fatal("CheckTotals accepted a declaration on a column that ties exactly")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"column 2", "now ties exactly", "remove the declaration"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}

// amountLiteral renders cents as the plain integer the YAML field takes.
func amountLiteral(c amount.Cents) string {
	return strconv.FormatInt(int64(c), 10)
}

// TestStatedTotalDeltaIsRefusedAtParseTime covers the ways of writing a
// declaration that do not say what a declaration has to say. Each is refused by
// the parser rather than reaching CheckTotals, so a rule file that is wrong is
// wrong when it is read and not when it is applied.
func TestStatedTotalDeltaIsRefusedAtParseTime(t *testing.T) {
	for _, tc := range []struct {
		name, block, want string
	}{{
		name:  "column out of range",
		block: "        stated_total_deltas:\n          - {column: 9, delta_cents: 100, note: x}",
		want:  "column 9 is out of range; this part has 4 columns",
	}, {
		name:  "column zero, mistaking it for an index",
		block: "        stated_total_deltas:\n          - {column: 0, delta_cents: 100, note: x}",
		want:  "column 0 is out of range",
	}, {
		name: "the same column twice",
		block: "        stated_total_deltas:\n" +
			"          - {column: 1, delta_cents: 100, note: x}\n" +
			"          - {column: 1, delta_cents: 200, note: y}",
		want: "column 1 already has a delta",
	}, {
		name:  "a zero delta, which declares nothing",
		block: "        stated_total_deltas:\n          - {column: 1, delta_cents: 0, note: x}",
		want:  "delta_cents is zero",
	}, {
		name:  "no note, which makes it a tolerance",
		block: "        stated_total_deltas:\n          - {column: 1, delta_cents: 100}",
		want:  "note is required",
	}, {
		name:  "a note that is only whitespace",
		block: "        stated_total_deltas:\n          - {column: 1, delta_cents: 100, note: \"   \"}",
		want:  "note is required",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(p127PropertyTaxes, "        #DELTAS", tc.block, 1)
			_, err := Parse(strings.NewReader(src), "p127.yaml")
			if err == nil {
				t.Fatal("Parse accepted it")
			}
			if got := diagnosis(t, err); !strings.Contains(got, tc.want) {
				t.Errorf("error does not say %q:\n%s", tc.want, got)
			}
		})
	}
}

// TestStatedTotalDeltaOnASkippedColumnIsRefused is separate because it needs a
// part with a skipped column, which p127 has none of. A skipped column produces
// no facts and is never totalled, so a delta describing one describes nothing.
func TestStatedTotalDeltaOnASkippedColumnIsRefused(t *testing.T) {
	src := strings.Replace(p127PropertyTaxes,
		"          - {fund_group: general, fund: 100, fiscal_year: 2027}",
		"          - {skip: true}\n"+
			"        stated_total_deltas:\n"+
			"          - {column: 4, delta_cents: 100, note: x}", 1)
	_, err := Parse(strings.NewReader(src), "p127.yaml")
	if err == nil {
		t.Fatal("Parse accepted a delta on a skipped column")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"column 4 is skipped", "never totalled"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}
