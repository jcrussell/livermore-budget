package mapping

import (
	"fmt"
	"os"
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

// p190Mixed maps the two rows either side of p190's mixed gap, under the
// column guard whose geometry raises the footnote marker "1": the marker and
// the heading on consecutive lines between them.
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
        column_headers: ["7/1/23", "Revenues", "Transfers In", "Total Sources"]
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
// lines only when the geometry raises it as a footnote marker, with headings
// alone, on the line above a heading or above the row the gap ends at: "12" in
// millions among a wrapped label's fragments is a figure and not a marker, and
// so is a "12" the page prints full height. Alone, a declared figure is still
// the whole gap and is admitted.
func TestAFigureSharesAGapOnlyAsAFootnoteMarker(t *testing.T) {
	p := &Part{
		Headings:      []string{"Foo Fund", "Capital Improvement Program Funds"},
		WrappedLabels: []string{"Special Revenue", "Funds"},
		UnmappedText:  []unmappedText{{Text: "1,234"}, {Text: "1"}, {Text: "12"}},
	}
	// raised stands in for the geometry raising every line, so the shape of
	// what it raises is still the marker's; flat raises none.
	var gap string
	raised := func(off int) bool { return isFootnoteMarker(strings.Fields(gap[off:])[0]) }
	flat := func(int) bool { return false }
	for _, tc := range []struct {
		name, gap string
		raised    func(int) bool
		want      bool
	}{
		{"broken row", "Foo Fund\n1,234", raised, false},
		{"broken row, figure first", "1,234\nFoo Fund", raised, false},
		{"broken row, a two-digit figure printed full height", "Foo Fund\n12", flat, false},
		{"marker above heading", "1\nCapital Improvement Program Funds", raised, true},
		{"marker above heading, printed full height", "1\nCapital Improvement Program Funds", flat, false},
		{"marker among wrapped-label fragments", "Special Revenue\n12\nFunds", raised, false},
		{"marker above a wrapped label", "12\nFunds", raised, false},
		{"marker below heading, above the next row", "Capital Improvement Program Funds\n1", raised, true},
		{"marker below heading, after the last row", "Capital Improvement Program Funds\n1", nil, false},
		{"marker below heading, above a wrapped label", "Capital Improvement Program Funds\n1\nFunds", raised, false},
		{"marker with a wrapped label before its heading", "1\nFunds\nCapital Improvement Program Funds", raised, false},
		{"marker above a marker", "12\n1\nCapital Improvement Program Funds", raised, false},
		{"lone figure", "1,234", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gap = tc.gap
			if got := declaredGap(p, tc.gap, map[string]bool{}, true, tc.raised); got != tc.want {
				t.Errorf("declaredGap(%q) = %v, want %v", tc.gap, got, tc.want)
			}
		})
	}
}

// markerRule reads two rows, Alpha and Beta, with one heading and one marker
// declared, under the column guard; markerPage prints mid between them and
// tail after Beta.
const markerRule = `schema_version: 1
doc_id: marker-doc

rules:
  - id: marker
    kind: revenue
    basis: adopted
    scope: probe
    grain: category
    units: dollars
    rows:
      - {label: "Alpha", category: taxes/property}
      - {label: "Beta", category: taxes/sales}
    parts:
      - page: 1
        section: "FY A\n"
        stop_at: "END"
        headings: ["Foo Fund"]
        unmapped_text:
          - {text: "MARK", note: "the line under the heading"}
        column_headers: ["FY A"]
        columns:
          - {fiscal_year: 2025}
`

// markerValues reads markerRule with mark declared, over a page printing mid
// between the rows and tail after them. The geometry is the monospaced page's,
// with each word reshape names moved to the box it gives.
func markerValues(t *testing.T, mark, mid, tail string, reshape func(line int, mark string) [][2]string) error {
	t.Helper()
	row := func(label, v string) string { return fmt.Sprintf("%-12s%10s\n", label, v) }
	pages := map[int]string{1: fmt.Sprintf("%22s\n", "FY A") + row("Alpha", "100") + mid +
		row("Beta", "200") + tail + "END\n"}
	geometry := textGeometry("marker-doc", pages)
	if reshape != nil {
		line := strings.Count(pages[1][:strings.Index(pages[1], "\n"+mark+"\n")+1], "\n")
		for _, r := range reshape(line, mark) {
			g := strings.Replace(geometry[1], r[0], r[1], 1)
			if g == geometry[1] {
				t.Fatalf("no word %s in the geometry", r[0])
			}
			geometry[1] = g
		}
	}
	f, err := parse(strings.NewReader(strings.Replace(markerRule, "MARK", mark, 1)), "marker.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := NewResolver(inlineDocWithGeometry(t, f.DocID, pages, geometry), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, _, err = r.Values(&f.Rules[0], &f.Rules[0].Parts[0])
	return err
}

// box is one monospaced word's geometry as geomtest.Monospaced writes it.
func box(x0, y0, x1, y1 int, text string) string {
	return fmt.Sprintf("[%d,%d,%d,%d,%q]", x0, y0, x1, y1, text)
}

// superscript prints the marker on line half height with its foot inside the
// line below, which is where the geometry clusters it into that line.
func superscript(line int, mark string) [][2]string {
	w := len(mark) * 6
	return [][2]string{{box(0, line*12, w, line*12+10, mark), box(0, line*12+10, w, line*12+15, mark)}}
}

// overTallHeading leaves the marker on line body size and on a geometry line
// of its own, and sets the "Foo Fund" heading below it taller than the body.
func overTallHeading(line int, _ string) [][2]string {
	y := (line + 1) * 12
	return [][2]string{
		{box(0, y, 18, y+10, "Foo"), box(0, y, 18, y+14, "Foo")},
		{box(24, y, 48, y+10, "Fund"), box(24, y, 48, y+14, "Fund")},
	}
}

// TestOnlyARaisedMarkerSharesAGapWithAHeading reads the gap rule through the
// resolver: a superscript "1" under a heading keys the row below it and is
// read; the same "1" after the last row keys no row, a "12" the page prints
// full height is the figure of a row broken over two lines, and so is a
// body-size "12" on a line of its own above a heading set larger than it.
// All three are refused.
func TestOnlyARaisedMarkerSharesAGapWithAHeading(t *testing.T) {
	if err := markerValues(t, "1", "Foo Fund\n1\n", "", superscript); err != nil {
		t.Errorf("a raised marker between a heading and the next row: %v", err)
	}
	for _, tc := range []struct {
		name, mark, mid, tail string
		reshape               func(int, string) [][2]string
	}{
		{"a full-height figure under a heading", "12", "Foo Fund\n12\n", "", nil},
		{"a raised marker after the last row", "1", "", "Foo Fund\n1\n", superscript},
		{"a body-size figure above a taller heading", "12", "12\nFoo Fund\n", "", overTallHeading},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := markerValues(t, tc.mark, tc.mid, tc.tail, tc.reshape)
			if !strings.Contains(fmt.Sprint(err), "but is not mapped") {
				t.Fatalf("got %v, want the gap's refusal", err)
			}
		})
	}
}

// TestTheBudgetBooksFootnoteMarkersAreRaised: every footnote marker a
// committed Budget Book rule declares as unmapped_text is one the guard reads
// as raised, found on its page by content.
func TestTheBudgetBooksFootnoteMarkersAreRaised(t *testing.T) {
	files, err := LoadDir(os.DirFS("../.."), "mappings")
	if err != nil {
		t.Fatalf("load the committed rule files: %v", err)
	}
	seen := 0
	for _, f := range files {
		if f.DocID != "livermore-budget-fy2026-2027" {
			continue
		}
		for _, r := range f.Rules {
			for _, p := range r.Parts {
				for _, u := range p.UnmappedText {
					if !isFootnoteMarker(u.Text) {
						continue
					}
					seen++
					t.Run(fmt.Sprintf("%s/p%d/%s", r.ID, p.Page, u.Text), func(t *testing.T) {
						text, g := budgetFixturePair(t, p.Page)
						pr, err := buildPairing(text, g)
						if err != nil {
							t.Fatalf("buildPairing: %v", err)
						}
						off := loneMarker(t, text, u.Text)
						if !(&columnGuard{pair: pr}).raisedMarker(off) {
							t.Errorf("p%d's %q at offset %d is not read as raised", p.Page, u.Text, off)
						}
					})
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no committed Budget Book rule declares a footnote marker, so nothing was checked")
	}
}
