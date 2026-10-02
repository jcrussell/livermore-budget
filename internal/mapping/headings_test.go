package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// p186Top maps the summary block at the top of Budget Book p186 and the first
// detail row under it. Between "Total City Budget"'s figures and "Low Income
// Housing Fund" the page prints, on a line of its own:
//
//	Total City Budget            $ 358,645,812  $ 255,315,420 ...
//
//	Special Revenue Funds
//
//	Low Income Housing Fund      $  39,881,994  $   5,852,912 ...
//
// "Special Revenue Funds" is also the label of the second row of this same
// block, which is the collision the parser has to allow. The block also holds
// the footnote marker "1" alone between two rows and a wrapped total, declared
// with the two declarations that already say those things.
const p186Top = `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: probe-p186
    kind: revenue
    basis: adopted
    scope: probe
    grain: fund-group-by-category
    units: dollars
    rows:
      - {label: "General Fund", category: taxes/property}
      - {label: "Special Revenue Funds", skip: true}
      - {label: "Debt Service Funds", skip: true}
      - {label: "Permanent Funds", skip: true}
      - {label: "Capital Funds", skip: true}
      - {label: "Enterprise Funds", skip: true}
      - {label: "Total Operating Budget", skip: true}
      - {label: "Internal Service Funds", skip: true}
      - {label: "Capital Improvement Program Funds", skip: true}
      - {label: "Total Capital Improvement Program", skip: true}
      - {label: "Total City Budget", skip: true}
      - {label: "Low Income Housing Fund", category: taxes/sales}
    parts:
      - page: 186
        section: "Total Sources"
        stop_at: "Housing Successor Agency"
        wrapped_labels: ["& Internal Service"]
        unmapped_text:
          - {text: "1", note: "footnote marker printed alone between two rows"}
        #HEADINGS
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
          - {fiscal_year: 2025, skip: true}
          - {fiscal_year: 2026, skip: true}
          - {fiscal_year: 2027, skip: true}
`

func p186Values(t *testing.T, decl string) ([]Value, error) {
	t.Helper()
	return probeValues(t, strings.Replace(p186Top, "        #HEADINGS", decl, 1), 186)
}

func probeValues(t *testing.T, src string, page int) ([]Value, error) {
	t.Helper()
	f, err := parse(strings.NewReader(src), "probe.yaml")
	if err != nil {
		return nil, err
	}
	r, err := NewResolver(budgetDoc(t, page), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	vals, _, err := r.Values(&f.Rules[0], &f.Rules[0].Parts[0])
	return vals, err
}

// TestHeadingBetweenRowsIsAdmitted reads p186 with the heading declared, and
// checks the figures either side of it are the ones the page prints: a
// heading swallowed as a row would shift the second row's figures.
func TestHeadingBetweenRowsIsAdmitted(t *testing.T) {
	vals, err := p186Values(t, `        headings: ["Special Revenue Funds"]`)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("got %d values, want 2 (two published rows x one column)", len(vals))
	}
	if got := vals[0].Cents; got != amount.Cents(1444069000) {
		t.Errorf("General Fund FY2024 = %d, want 1444069000", got)
	}
	if got := vals[1].Cents; got != amount.Cents(3988199400) {
		t.Errorf("Low Income Housing Fund FY2024 = %d, want 3988199400", got)
	}
}

// TestUndeclaredHeadingIsRefused: without the declaration the gap is still an
// unmapped row, and the hint names the declaration that would say otherwise.
func TestUndeclaredHeadingIsRefused(t *testing.T) {
	_, err := p186Values(t, "")
	if err == nil {
		t.Fatal("resolved with no headings declaration; checkGap must still refuse it")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"Special Revenue Funds", "not mapped", "headings"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}

// TestStaleHeadingIsRefused: a declared heading the page does not print
// between this part's rows is a claim that has stopped being true.
func TestStaleHeadingIsRefused(t *testing.T) {
	_, err := p186Values(t, `        headings: ["Special Revenue Funds", "Debt Service Funds Detail"]`)
	if err == nil {
		t.Fatal("resolved; a declared heading the page does not use must fail")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"Debt Service Funds Detail", "declared but does not appear", "headings"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}

// p190Mixed maps the two rows either side of p190's mixed gap: the footnote
// marker "1" and the heading on consecutive lines between them.
//
//	Total Internal Service Funds   $  27,749,075 ...
//
//	                                       1
//	Capital Improvement Program Funds
//
//	CIP Airport                    $           - ...
const p190Mixed = `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: probe-p190
    kind: revenue
    basis: adopted
    scope: probe
    grain: fund-group-by-category
    units: dollars
    rows:
      - {label: "Facilities Rehab Pgm", skip: true}
      - {label: "Total Internal Service Funds", category: taxes/property}
      - {label: "CIP Airport", category: taxes/sales}
    parts:
      - page: 190
        section: "Total Sources"
        stop_at: "CIP Stormwater"
        headings: ["Capital Improvement Program Funds"]
        #UNMAPPED
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
          - {fiscal_year: 2025, skip: true}
          - {fiscal_year: 2026, skip: true}
          - {fiscal_year: 2027, skip: true}
`

// TestMixedGapOfFigureAndHeading: every line of the gap is declared, one as
// an unmapped figure and one as a heading, so the gap is admitted; drop the
// figure's declaration and the same gap is refused on it.
func TestMixedGapOfFigureAndHeading(t *testing.T) {
	const unmapped = "        unmapped_text:\n" +
		`          - {text: "1", note: "footnote marker printed above the heading"}`
	vals, err := probeValues(t, strings.Replace(p190Mixed, "        #UNMAPPED", unmapped, 1), 190)
	if err != nil {
		t.Fatalf("a gap of a declared figure and a declared heading was refused: %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("got %d values, want 2", len(vals))
	}
	if got := vals[0].Cents; got != amount.Cents(2774907500) {
		t.Errorf("Total Internal Service Funds FY2024 = %d, want 2774907500", got)
	}

	_, err = probeValues(t, strings.Replace(p190Mixed, "        #UNMAPPED", "", 1), 190)
	if err == nil {
		t.Fatal("the gap was admitted with its figure undeclared")
	}
	if got := diagnosis(t, err); !strings.Contains(got, "not mapped") {
		t.Errorf("refused for some other reason:\n%s", got)
	}
}

// TestHeadingsParseRefusals mirrors TestWrappedLabelsParseRefusals, plus the
// two arms a heading has of its own.
func TestHeadingsParseRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, block, want string
	}{
		{"blank", `        headings: ["  "]`, "has a blank entry"},
		{"untrimmed", `        headings: [" Special Revenue Funds"]`, "has leading or trailing whitespace"},
		{"duplicate", `        headings: ["Special Revenue Funds", "Special Revenue Funds"]`, "is listed twice"},
		{"amount", `        headings: ["1"]`, "is a currency amount"},
		{"also a wrapped label", `        headings: ["& Internal Service"]`, "is also declared in wrapped_labels"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(strings.Replace(p186Top,
				"        #HEADINGS", tc.block, 1)), "probe.yaml")
			if err == nil {
				t.Fatalf("accepted: %s", tc.block)
			}
			if !strings.Contains(err.Error(), "headings") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused for some other reason: %v", err)
			}
		})
	}
}

// TestHeadingsOnALabelsFromPartAreRefused: a labels_from part reads no gaps,
// so a heading declared there would be accepted and never looked at.
func TestHeadingsOnALabelsFromPartAreRefused(t *testing.T) {
	const src = `schema_version: 1
doc_id: labelsfrom-doc

rules:
  - id: two-part
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 1
        section: "Division One"
        stop_at: "END"
        columns:
          - {fund_group: general, fiscal_year: 2026}
      - page: 2
        labels_from: 1
        section: "CONTINUED"
        stop_at: "END"
        headings: ["Anything At All"]
        columns:
          - {fund_group: general, fiscal_year: 2026}
`
	_, err := parse(strings.NewReader(src), "labelsfrom.yaml")
	if err == nil {
		t.Fatal("headings was accepted on a labels_from part, where nothing reads it")
	}
	for _, want := range []string{"headings", "labels_from", "page 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// TestAFigureSharesAGapOnlyAsAFootnoteMarker: a row broken over two lines
// ("Foo Fund" then "1,234") declared as heading plus unmapped figure would drop
// the row's figure, so a declared figure shares a gap with other declared
// lines only when it is a footnote marker. Alone, a declared figure is still
// the whole gap and is admitted.
func TestAFigureSharesAGapOnlyAsAFootnoteMarker(t *testing.T) {
	p := &Part{
		Headings:     []string{"Foo Fund", "Capital Improvement Program Funds"},
		UnmappedText: []unmappedText{{Text: "1,234"}, {Text: "1"}},
	}
	for _, tc := range []struct {
		name, gap string
		want      bool
	}{
		{"broken row", "Foo Fund\n1,234", false},
		{"broken row, figure first", "1,234\nFoo Fund", false},
		{"marker above heading", "1\nCapital Improvement Program Funds", true},
		{"lone figure", "1,234", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaredGap(p, tc.gap, map[string]bool{}, true); got != tc.want {
				t.Errorf("declaredGap(%q) = %v, want %v", tc.gap, got, tc.want)
			}
		})
	}
}
