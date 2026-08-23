package mapping

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// The shape these tests are about, as Budget Book pp.169-170 print it: one
// division's rows split across a page break, with the printed Total on the
// second page covering both.
//
//	p169  Environmental Services  Wages & Benefits      2,056
//	p170  Environmental Services  Services & Supplies 132,186
//	p170                          Total              $134,242
//
// The pages are written inline rather than copied from data/extracted because
// the committed fixtures do not yet carry pp.169-170 -- adding them is the
// coverage lane's own work, and it owes both substrates and a README entry per
// testdata/README.md. What is asserted here is the CAPABILITY, at one column so
// the arithmetic is checkable by eye: $2,056 + $132,186 = $134,242.
const spanningPages = `schema_version: 1
doc_id: spanning-doc

rules:
  - id: environmental-services
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    units: dollars
    total_row: "Total Environmental Services"
    #SPANS
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits}
      - {label: "Services & Supplies", category: services-and-supplies}
    parts:
      - page: 169
        section: "ENVIRONMENTAL SERVICES"
        stop_at: "END OF PAGE"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
        omitted_rows: ["Services & Supplies"]
      - page: 170
        section: "ENVIRONMENTAL SERVICES"
        stop_at: "Total Environmental Services"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
        omitted_rows: ["Wages & Benefits"]
`

func spanningDoc(t *testing.T) map[int]string {
	t.Helper()
	return map[int]string{
		169: "ENVIRONMENTAL SERVICES\n" +
			"    Wages & Benefits        2,056\n" +
			"END OF PAGE\n",
		170: "ENVIRONMENTAL SERVICES\n" +
			"    Services & Supplies   132,186\n" +
			"    Total Environmental Services   $134,242\n",
	}
}

func spanningResolver(t *testing.T, spans bool) (*Resolver, *Rule) {
	t.Helper()
	src := spanningPages
	if spans {
		src = strings.Replace(src, "    #SPANS", "    total_spans_parts: true", 1)
	}
	f, err := Parse(strings.NewReader(src), "spanning.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, spanningDoc(t)), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// TestAStraddlingBlockTiesOnlyWithTheFlag is the whole of fisc-w0o in one test:
// the block cannot be checked part by part, and can be checked as a rule.
//
// The two halves matter equally. That the spanning check TIES proves the
// capability; that the per-part check FAILS on both parts proves the capability
// was needed, and that this flag is not quietly relaxing a check that already
// worked.
func TestAStraddlingBlockTiesOnlyWithTheFlag(t *testing.T) {
	r, rule := spanningResolver(t, true)
	res, err := r.CheckSpanningTotals(rule)
	if err != nil {
		t.Fatalf("CheckSpanningTotals: %v; 2,056 + 132,186 should tie to the printed 134,242", err)
	}
	if res.Columns != 1 {
		t.Errorf("Columns = %d, want 1", res.Columns)
	}
	if res.Declared != 0 {
		t.Errorf("Declared = %d, want 0; the block ties exactly and declares nothing", res.Declared)
	}

	// The same rule, checked the old way, one part at a time.
	rp, rulep := spanningResolver(t, false)
	// The head part does not print the total row at all.
	if _, headErr := rp.CheckTotals(rulep, &rulep.Parts[0]); headErr == nil {
		t.Error("CheckTotals(p169) succeeded; the page prints no total for this block, " +
			"so a per-part check must not report one")
	}
	// The tail part prints it, and sums only its own row against it.
	_, err = rp.CheckTotals(rulep, &rulep.Parts[1])
	if err == nil {
		t.Fatal("CheckTotals(p170) tied; it sums only 132,186 and must not tie to 134,242")
	}
	if !strings.Contains(err.Error(), "132,186") {
		t.Errorf("CheckTotals(p170) = %v; want the message to name the 132,186 it summed", err)
	}
}

// TestSpanningTotalsHonourTheDeclarationOnTheTotalsPage is the ADMINISTRATIVE
// SERVICES shape from the plan's measurement, reduced to one column: a block
// whose printed total exceeds its rows by $1 of the city's own rounding.
//
// It is here because a spanning rule has one printed total and several parts,
// so "which part's stated_total_deltas apply" is a real question, and reading
// it off the wrong page would be a silent wrong answer.
func TestSpanningTotalsHonourTheDeclarationOnTheTotalsPage(t *testing.T) {
	src := strings.Replace(spanningPages, "    #SPANS", "    total_spans_parts: true", 1)
	// The document now states one dollar more than the rows add to.
	pages := spanningDoc(t)
	pages[170] = strings.Replace(pages[170], "$134,242", "$134,243", 1)
	src = strings.Replace(src,
		`        stop_at: "Total Environmental Services"`,
		`        stop_at: "Total Environmental Services"
        stated_total_deltas:
          - column: 1
            delta_cents: 100
            note: >-
              The city's own rounding: the two rows print 134,242 and the
              document states 134,243.`, 1)

	f, err := Parse(strings.NewReader(src), "spanning.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, pages), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	res, err := r.CheckSpanningTotals(&f.Rules[0])
	if err != nil {
		t.Fatalf("CheckSpanningTotals with the delta declared on the totals page: %v", err)
	}
	if res.Declared != 1 {
		t.Errorf("Declared = %d, want 1; the column tied only to a declared delta and "+
			"the report must be able to say so", res.Declared)
	}
}

// TestADeltaOnTheWrongPageIsRefused states where a declaration may live.
//
// The parser cannot catch this one: it does not know which page prints the
// total, so it can only refuse TWO parts declaring a delta. This is the
// resolve-time half, and without it a rule could declare the city's rounding
// against a page that prints no total at all.
func TestADeltaOnTheWrongPageIsRefused(t *testing.T) {
	src := strings.Replace(spanningPages, "    #SPANS", "    total_spans_parts: true", 1)
	src = strings.Replace(src,
		`        omitted_rows: ["Services & Supplies"]`,
		`        omitted_rows: ["Services & Supplies"]
        stated_total_deltas:
          - column: 1
            delta_cents: 100
            note: "declared against a page that prints no total"`, 1)
	f, err := Parse(strings.NewReader(src), "spanning.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, spanningDoc(t)), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, err = r.CheckSpanningTotals(&f.Rules[0])
	if err == nil {
		t.Fatal("a delta declared on p169 was accepted, but p170 prints the total")
	}
	if !strings.Contains(err.Error(), "page 170 is the page that prints") {
		t.Errorf("error = %v; want it to name the page that prints the total", err)
	}
}

// TestTheTotalRowMustIdentifyOnePage covers both ways the anchor can fail to
// name one page, because a spanning rule locates its total by searching every
// part's page rather than the one it was handed.
//
// The absent case mutates the RULE's total_row rather than the PAGE, and the
// distinction is worth keeping: p170's stop_at names the same string, so
// editing the page makes the block boundary fail first and would prove nothing
// about the total lookup.
func TestTheTotalRowMustIdentifyOnePage(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		mutate     func(src string, pages map[int]string) string
	}{{
		name: "on no page",
		want: "occurs after the block on none of pages",
		mutate: func(src string, _ map[int]string) string {
			return strings.Replace(src,
				`total_row: "Total Environmental Services"`,
				`total_row: "Total Envirnmental Servcies"`, 1)
		},
	}, {
		name: "on two pages",
		want: "it must identify one",
		mutate: func(src string, pages map[int]string) string {
			pages[169] += "    Total Environmental Services   $2,056\n"
			return src
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(spanningPages, "    #SPANS", "    total_spans_parts: true", 1)
			pages := spanningDoc(t)
			src = tc.mutate(src, pages)
			f, err := Parse(strings.NewReader(src), "spanning.yaml")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			r, err := NewResolver(inlineDoc(t, f.DocID, pages), f)
			if err != nil {
				t.Fatalf("NewResolver: %v", err)
			}
			_, err = r.CheckSpanningTotals(&f.Rules[0])
			if err == nil {
				t.Fatal("the ambiguous or absent anchor was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("error is not ErrNotFound: %v", err)
			}
		})
	}
}

// TestTheSpineCannotDeclareItsTotalSpansItsParts is the guard that keeps
// total_spans_parts off the one rule in the corpus it would silently corrupt.
//
// It reads the PUBLISHED rule file and adds the flag to spine-revenues, because
// the claim worth pinning is about that rule and not about a shape invented
// here. pp.66-67 partition the schedule by COLUMN -- p66 carries the General
// Fund and Enterprise columns and p67 the other four fund groups -- so summing
// a row's figures across both parts would add General Fund dollars to Capital
// Funds dollars and compare the result against a total that covers neither.
//
// The failure would be silent, which is what makes a parser guard the right
// place for it: the sum would be a number, and a wrong number tied against the
// wrong total is this project's defining failure mode.
func TestTheSpineCannotDeclareItsTotalSpansItsParts(t *testing.T) {
	src, err := os.ReadFile(publishedSpine)
	if err != nil {
		t.Fatalf("read %s: %v", publishedSpine, err)
	}
	const anchor = "    total_row: \"TOTAL REVENUES:\"\n"
	if !strings.Contains(string(src), anchor) {
		t.Fatalf("%s no longer declares %q on spine-revenues; this test has gone stale",
			publishedSpine, strings.TrimSpace(anchor))
	}
	mutated := strings.Replace(string(src), anchor,
		anchor+"    total_spans_parts: true\n", 1)

	_, err = Parse(strings.NewReader(mutated), publishedSpine)
	if err == nil {
		t.Fatal("the parser accepted total_spans_parts on spine-revenues; p66 declares " +
			"4 columns and p67 declares 8, and summing across them is meaningless")
	}
	for _, want := range []string{"spine-revenues", "total_spans_parts", "identical"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

// TestTotalSpansPartsRefusesWhatItCannotMean states the three refusals the
// parser can make without the pages.
func TestTotalSpansPartsRefusesWhatItCannotMean(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		mutate     func(string) string
	}{{
		name: "with no total row to span",
		want: "declares no total_row",
		mutate: func(src string) string {
			return strings.Replace(src, `    total_row: "Total Environmental Services"`+"\n", "", 1)
		},
	}, {
		name: "on a rule with one part",
		want: "is set on a rule with 1 part",
		mutate: func(src string) string {
			// Cut rather than Index: a missing anchor would make Index return
			// -1 and the slice panic. Returning src unmutated instead leaves a
			// file that parses, so the subtest fails on its own "accepted"
			// assertion — a red test either way, and a legible one.
			head, _, _ := strings.Cut(src, "      - page: 170")
			return head
		},
	}, {
		name: "on parts whose columns differ",
		want: "must be identical",
		mutate: func(src string) string {
			return strings.Replace(src,
				"          - {fund_group: general, fiscal_year: 2024, basis: actual}\n"+
					`        omitted_rows: ["Wages & Benefits"]`,
				"          - {fund_group: general, fiscal_year: 2025, basis: actual}\n"+
					`        omitted_rows: ["Wages & Benefits"]`, 1)
		},
	}, {
		name: "with a delta declared on two parts",
		want: "each declare a delta, but this rule has one printed total",
		mutate: func(src string) string {
			d := "\n        stated_total_deltas:\n          - {column: 1, delta_cents: 100, note: \"x\"}"
			return strings.ReplaceAll(src,
				"          - {fund_group: general, fiscal_year: 2024, basis: actual}",
				"          - {fund_group: general, fiscal_year: 2024, basis: actual}"+d)
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(spanningPages, "    #SPANS", "    total_spans_parts: true", 1)
			if _, err := Parse(strings.NewReader(tc.mutate(src)), "spanning.yaml"); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
