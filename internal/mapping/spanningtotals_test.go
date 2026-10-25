package mapping

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// spanningPages is pp.169-170 at one column: Environmental Services' rows split
// over the page break, $2,056 + $132,186 = $134,242 printed on p170.
const spanningPages = `schema_version: 1
doc_id: spanning-doc

rules:
  - id: environmental-services
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total Environmental Services"
    #SPANS
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits, page: 169}
      - {label: "Services & Supplies", category: services-and-supplies, page: 170}
    parts:
      - page: 169
        section: "ENVIRONMENTAL SERVICES"
        stop_at: "END OF PAGE"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
      - page: 170
        section: "ENVIRONMENTAL SERVICES"
        stop_at: "Total Environmental Services"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
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
	f, err := parse(strings.NewReader(src), "spanning.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, spanningDoc(t)), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// TestAStraddlingBlockTiesOnlyWithTheFlag: the spanning check ties, and the
// per-part check fails on both parts.
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

// TestSpanningTotalsHonourTheDeclarationOnTheTotalsPage: a $1 rounding delta
// applies from the part whose page prints the total.
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

	f, err := parse(strings.NewReader(src), "spanning.yaml")
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

// TestADeltaOnTheWrongPageIsRefused: the resolve-time half; the parser only
// refuses two parts declaring a delta.
func TestADeltaOnTheWrongPageIsRefused(t *testing.T) {
	src := strings.Replace(spanningPages, "    #SPANS", "    total_spans_parts: true", 1)
	src = strings.Replace(src,
		`        stop_at: "END OF PAGE"`,
		`        stop_at: "END OF PAGE"
        stated_total_deltas:
          - column: 1
            delta_cents: 100
            note: "declared against a page that prints no total"`, 1)
	f, err := parse(strings.NewReader(src), "spanning.yaml")
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

// TestTheTotalRowMustIdentifyOnePage: absent or on two pages. The absent case
// mutates the rule's total_row, since p170's stop_at names the same string.
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
			f, err := parse(strings.NewReader(src), "spanning.yaml")
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

// TestTheSpineCannotDeclareItsTotalSpansItsParts: pp.66-67 split the spine by
// column, so total_spans_parts on spine-revenues would sum unrelated funds.
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

	_, err = parse(strings.NewReader(mutated), publishedSpine)
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
			// Cut, not Index: a missing anchor leaves src unmutated and the
			// subtest fails legibly instead of panicking.
			head, _, _ := strings.Cut(src, "      - page: 170")
			// The row p170 printed goes with it, or the placement is refused
			// first for naming no part, and so does the one part's page on
			// the other, or it is refused for placing nothing.
			return strings.NewReplacer(", page: 170", "", ", page: 169", "").Replace(head)
		},
	}, {
		name: "on parts whose columns differ",
		want: "must be identical",
		mutate: func(src string) string {
			return strings.Replace(src,
				"        stop_at: \"Total Environmental Services\"\n        columns:\n"+
					"          - {fund_group: general, fiscal_year: 2024, basis: actual}",
				"        stop_at: \"Total Environmental Services\"\n        columns:\n"+
					"          - {fund_group: general, fiscal_year: 2025, basis: actual}", 1)
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
			if _, err := parse(strings.NewReader(tc.mutate(src)), "spanning.yaml"); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
