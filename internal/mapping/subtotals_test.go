package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/geom/geomtest"
)

// subtotalRules is the pp.224-235 shape: a labelled page and its label-less
// continuation, subtotals at levels 1-3, and a block opened on one page pair
// and totalled on the next. The second page's skipped TOTAL column is still
// compared.
const subtotalRules = `schema_version: 1
doc_id: subtotal-doc

rules:
  - id: pair-1
    kind: expenditure
    basis: adopted
    scope: cip-project-listing
    grain: category
    units: dollars
    subtotal_chain: listing
    parts:
      - page: 1
        section: "FY A\n"
        stop_at: "END"
        column_headers: ["FY A"]
        columns:
          - {fiscal_year: 2025}
      - page: 2
        labels_from: 1
        section: "TOTAL\n"
        stop_at: "END"
        column_headers: ["FY B", "TOTAL"]
        columns:
          - {fiscal_year: 2026}
          - {skip: true}
    rows:
      - {label: "P1 Alpha", category: capital-projects}
      - {label: "P2 Beta", category: capital-projects}
      - {label: "SUBTOTAL X", skip: true, subtotal: 1}
      - {label: "P3 Gamma", category: capital-projects}
  - id: pair-2
    kind: expenditure
    basis: adopted
    scope: cip-project-listing
    grain: category
    units: dollars
    subtotal_chain: listing
    parts:
      - page: 3
        section: "FY A\n"
        stop_at: "END"
        column_headers: ["FY A"]
        columns:
          - {fiscal_year: 2025}
      - page: 4
        labels_from: 3
        section: "TOTAL\n"
        stop_at: "END"
        column_headers: ["FY B", "TOTAL"]
        columns:
          - {fiscal_year: 2026}
          - {skip: true}
    rows:
      - {label: "SUBTOTAL Y", skip: true, subtotal: 1}
      - {label: "TOTAL FUND", skip: true, subtotal: 2}
      - {label: "P4 Delta", category: capital-projects}
      - {label: "TOTAL D", skip: true, subtotal: 2}
      - label: "TOTAL"
        skip: true
        subtotal: 3
        #DELTAS
`

func subtotalPages() map[int]string {
	labelled := func(rows ...[2]string) string {
		out := fmt.Sprintf("%22s\n", "FY A")
		for _, r := range rows {
			out += fmt.Sprintf("%-12s%10s\n", r[0], r[1])
		}
		return out + "END\n"
	}
	figures := func(rows ...[2]string) string {
		out := fmt.Sprintf("%10s%10s\n", "FY B", "TOTAL")
		for _, r := range rows {
			out += fmt.Sprintf("%10s%10s\n", r[0], r[1])
		}
		return out + "END\n"
	}
	return map[int]string{
		1: labelled([2]string{"P1 Alpha", "$   10"}, [2]string{"P2 Beta", "20"},
			[2]string{"  SUBTOTAL X", "$   30"}, [2]string{"P3 Gamma", "1"}),
		2: figures([2]string{"$    5", "$   15"}, [2]string{"7", "27"},
			[2]string{"$   12", "$   42"}, [2]string{"2", "3"}),
		3: labelled([2]string{"  SUBTOTAL Y", "$    1"}, [2]string{"  TOTAL FUND", "$   31"},
			[2]string{"P4 Delta", "4"}, [2]string{"  TOTAL D", "$    4"},
			[2]string{"  TOTAL", "$   35"}),
		4: figures([2]string{"$    2", "$    3"}, [2]string{"$   14", "$   45"},
			[2]string{"4", "8"}, [2]string{"$    4", "$    8"}, [2]string{"$   18", "$   53"}),
	}
}

// textGeometry is geomtest.Monospaced over each of a document's pages.
func textGeometry(docID string, pages map[int]string) map[int]string {
	out := map[int]string{}
	for n, text := range pages {
		out[n] = geomtest.Monospaced(docID, n, text)
	}
	return out
}

// subtotalCheck parses src over pages and runs every chain it declares,
// returning the first error and the summed result.
func subtotalCheck(t *testing.T, src string, pages map[int]string) (*SubtotalsResult, error) {
	t.Helper()
	f, err := parse(strings.NewReader(src), "subtotal.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := NewResolver(inlineDocWithGeometry(t, f.DocID, pages, textGeometry(f.DocID, pages)), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	sum := &SubtotalsResult{}
	for _, chain := range Chains(f) {
		res, err := r.CheckSubtotals(chain)
		if err != nil {
			return nil, err
		}
		sum.Tied = append(sum.Tied, res.Tied...)
		sum.Declared = append(sum.Declared, res.Declared...)
	}
	return sum, nil
}

// TestASubtotalChainTiesAcrossRulesAndLevels: five subtotals, three levels, one
// block crossing rules. Mutations: reset every level at every subtotal and
// TOTAL FUND sees only Gamma; drop the chain and SUBTOTAL Y sees no row.
func TestASubtotalChainTiesAcrossRulesAndLevels(t *testing.T) {
	res, err := subtotalCheck(t, subtotalRules, subtotalPages())
	if err != nil {
		t.Fatalf("CheckSubtotals: %v", err)
	}
	cells := 0
	for _, line := range res.Tied {
		cells += len(line)
	}
	if len(res.Tied) != 5 || cells != 15 || len(res.Declared) != 0 {
		t.Errorf("tied %d lines over %d cells, %d by declaration, want 5 lines over 15 "+
			"(three headers each) and none declared", len(res.Tied), cells, len(res.Declared))
	}
}

// mustReplace replaces the first from in s and fails t when there is none.
func mustReplace(t *testing.T, s, from, to string) string {
	t.Helper()
	if !strings.Contains(s, from) {
		t.Fatalf("the edit %q matches nothing", from)
	}
	return strings.Replace(s, from, to, 1)
}

// TestSubtotalChecksRefuse holds each refusal CheckSubtotals makes, one edit
// of subtotalRules or subtotalPages per row. An edit that matches nothing
// fails the row rather than leaving the fixture whole.
func TestSubtotalChecksRefuse(t *testing.T) {
	rep := func(s, from, to string) string { t.Helper(); return mustReplace(t, s, from, to) }
	repAll := func(s, from, to string) string {
		t.Helper()
		return strings.ReplaceAll(rep(s, from, to), from, to)
	}
	repLast := func(s, from, to string) string {
		t.Helper()
		i := strings.LastIndex(rep(s, from, from), from)
		return s[:i] + to + s[i+len(from):]
	}
	repPage := func(n int, from, to string) func(*testing.T, map[int]string) {
		return func(t *testing.T, p map[int]string) { t.Helper(); p[n] = mustReplace(t, p[n], from, to) }
	}
	for _, tc := range []struct {
		name  string
		src   string
		pages func(*testing.T, map[int]string)
		want  []string
	}{
		// One wrong figure per kind of column, a published one and the skipped
		// TOTAL the rule never publishes.
		{name: "TheRowsDoNotSumTo/a published column", pages: repPage(1, "$   30", "$   31"),
			want: []string{`"SUBTOTAL X"`}},
		{name: "TheRowsDoNotSumTo/a skipped column", pages: repPage(2, "$   42", "$   43"),
			want: []string{`"TOTAL"`}},
		{name: "TheRowsDoNotSumTo/a level-2 total", pages: repPage(3, "$   31", "$   30"),
			want: []string{`"TOTAL FUND": prints 30 under "FY A" on p3`}},
		// Without the chain each rule is its own, so Gamma sits under no
		// subtotal of pair-1 and SUBTOTAL Y sums nothing.
		{name: "ChainIsWhatCarriesABlockAcrossRules",
			src:  repAll(subtotalRules, "    subtotal_chain: listing\n", ""),
			want: []string{`rule "pair-1"`, "inside no printed total"}},
		// Demote the grand TOTAL to a plain skipped row and the last row is
		// under no printed total.
		{name: "RowUnderNoSubtotal",
			src: rep(subtotalRules, "      - label: \"TOTAL\"\n        skip: true\n        subtotal: 3\n        #DELTAS\n",
				"      - {label: \"TOTAL\", skip: true}\n"),
			want: []string{"inside no printed total"}},
		// A grand total declared one level too low follows a level-2 total that
		// cleared its sum, so it would compare nothing and be counted as tied.
		{name: "SubtotalOverNoRow", src: rep(subtotalRules, "        subtotal: 3\n", "        subtotal: 2\n"),
			want: []string{"over no row"}},
		// A chain matches columns by position, so its rules must agree on what
		// each position is.
		{name: "ChainLaidOutUnlikeItsFirstRule",
			src:   repLast(subtotalRules, `column_headers: ["FY B", "TOTAL"]`, `column_headers: ["FY C", "TOTAL"]`),
			pages: repPage(4, fmt.Sprintf("%10s", "FY B"), fmt.Sprintf("%10s", "FY C")),
			want:  []string{"compares columns by position"}},
		// A non-amount row yields no cell, so a subtotal over it alone sums
		// nothing and a printed dash there would tie against an empty sum.
		{name: "SubtotalOverARowWithNoAmount",
			src: rep(subtotalRules, "      - {label: \"SUBTOTAL X\", skip: true, subtotal: 1}\n",
				"      - {label: \"SUBTOTAL X\", skip: true, subtotal: 1}\n"+
					"      - {label: \"P9 Count\", quantity: number}\n"+
					"      - {label: \"ZERO TOTAL\", skip: true, subtotal: 1}\n"),
			pages: func(t *testing.T, p map[int]string) {
				repPage(1, "P3 Gamma", fmt.Sprintf("%-12s%10s\n%-12s%10s\nP3 Gamma",
					"P9 Count", "7", "  ZERO TOTAL", "-"))(t, p)
				repPage(2, fmt.Sprintf("%10s%10s\n", "2", "3"),
					fmt.Sprintf("%10s%10s\n%10s%10s\n%10s%10s\n", "7", "7", "-", "-", "2", "3"))(t, p)
			},
			want: []string{`"ZERO TOTAL"`, "over no row"}},
		// A chain named on a rule with no subtotal row is refused with the
		// document, rule and page.
		{name: "ChainWithNoSubtotalSaysWhere",
			src: rep(rep(subtotalRules, "      - {label: \"SUBTOTAL X\", skip: true, subtotal: 1}\n",
				"      - {label: \"SUBTOTAL X\", skip: true}\n"),
				"    subtotal_chain: listing\n", "    subtotal_chain: lonely\n"),
			want: []string{"subtotal-doc p1", `"pair-1"`, `"lonely"`}},
		// A delta on a column no figure is summed under is refused on p4, which
		// prints it. Mutation: report the rule's first page and this names p3.
		{name: "RefusalNamesThePageOfItsColumn",
			src: rep(repAll(subtotalRules, "          - {fiscal_year: 2026}\n          - {skip: true}\n",
				"          - {fiscal_year: 2026}\n          - {quantity: number}\n"),
				"#DELTAS", `subtotal_deltas: [{column: "TOTAL", delta_cents: 100, note: "x"}]`),
			want: []string{"subtotal-doc p4", `under "TOTAL" on p4`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			if src == "" {
				src = subtotalRules
			}
			pages := subtotalPages()
			if tc.pages != nil {
				tc.pages(t, pages)
			}
			_, err := subtotalCheck(t, src, pages)
			for _, want := range tc.want {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("got %v, want a refusal naming %s", err, want)
				}
			}
		})
	}
}

// TestASubtotalDeltaIsADeclarationNotATolerance: a declared delta ties the
// column it names by exactly that much, and is refused once the column ties
// without it.
func TestASubtotalDeltaIsADeclarationNotATolerance(t *testing.T) {
	delta := "subtotal_deltas:\n          - {column: \"FY B\", delta_cents: -100, note: \"rounding\"}"
	src := strings.Replace(subtotalRules, "#DELTAS", delta, 1)

	pages := subtotalPages()
	pages[4] = strings.Replace(pages[4], "$   18", "$   17", 1)
	res, err := subtotalCheck(t, src, pages)
	if err != nil {
		t.Errorf("a grand total a dollar under its rows, declared: %v", err)
	} else if len(res.Declared) != 1 || res.Declared[0].Token != "17" {
		t.Errorf("declared figures = %+v, want the one 17 the delta ties", res.Declared)
	}
	if _, err = subtotalCheck(t, subtotalRules, pages); err == nil {
		t.Error("the same dollar undeclared was accepted")
	}
	_, err = subtotalCheck(t, src, subtotalPages())
	if err == nil || !strings.Contains(err.Error(), "ties exactly") {
		t.Errorf("a delta over a column that ties: got %v, want it refused as stale", err)
	}
}

func TestSubtotalDeclarationsAreRefusedWhereTheyCannotMean(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"a subtotal that publishes", `{label: "SUBTOTAL X", skip: true, subtotal: 1}`,
			`{label: "SUBTOTAL X", category: capital-projects, subtotal: 1}`, "is not skip: true"},
		{"a negative level", `{label: "SUBTOTAL X", skip: true, subtotal: 1}`,
			`{label: "SUBTOTAL X", skip: true, subtotal: -1}`, "levels count from 1"},
		{"a part with no headers", `        column_headers: ["FY A"]` + "\n", "", "compare figures by column header"},
		{"a delta on a row that is no subtotal", `{label: "P4 Delta", category: capital-projects}`,
			`{label: "P4 Delta", category: capital-projects, subtotal_deltas: [{column: "FY B", delta_cents: 1, note: "x"}]}`,
			"is not a subtotal"},
		{"a delta with no note", "#DELTAS", `subtotal_deltas: [{column: "FY B", delta_cents: 1}]`, "has no note"},
		{"a delta naming no column", "#DELTAS", `subtotal_deltas: [{column: "FY Z", delta_cents: 1, note: "x"}]`,
			"names exactly one"},
		{"a published row repeated", `{label: "P2 Beta", category: capital-projects}`,
			`{label: "P1 Alpha", category: capital-projects}`, "duplicate row label"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(subtotalRules, tc.from) {
				t.Fatalf("the mutation %q matches nothing", tc.from)
			}
			src := strings.Replace(subtotalRules, tc.from, tc.to, 1)
			_, err := parse(strings.NewReader(src), "subtotal.yaml")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parse: got %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// TestTwoSkippedRowsMayShareALabel is p230's fund 611, printed twice on lines
// identical but for figures; a blank cell declared on it cannot say which.
func TestTwoSkippedRowsMayShareALabel(t *testing.T) {
	src := strings.Replace(subtotalRules,
		`      - {label: "P2 Beta", category: capital-projects}`,
		`      - {label: "P1 Alpha", skip: true}`, 1)
	src = strings.Replace(src,
		`      - {label: "P1 Alpha", category: capital-projects}`,
		`      - {label: "P1 Alpha", skip: true}`, 1)
	pages := subtotalPages()
	pages[1] = strings.Replace(pages[1], "P2 Beta ", "P1 Alpha", 1)
	if _, err := subtotalCheck(t, src, pages); err != nil {
		t.Errorf("two skipped rows sharing a label: %v", err)
	}
	blank := strings.Replace(src, "        column_headers: [\"FY A\"]\n",
		"        omitted_cells: [{label: \"P1 Alpha\", column: \"FY A\", note: n}]\n        column_headers: [\"FY A\"]\n", 1)
	if _, err := parse(strings.NewReader(blank), "subtotal.yaml"); err == nil ||
		!strings.Contains(err.Error(), "more than once") {
		t.Errorf("a blank cell naming a repeated row: got %v, want it refused", err)
	}
}

// TestAColumnIsItsPositionNotItsHeader: p81 repeats Principal, Interest and
// Total each year; keyed by header, a wrong first-column subtotal is never
// compared.
func TestAColumnIsItsPositionNotItsHeader(t *testing.T) {
	src := strings.ReplaceAll(subtotalRules, `column_headers: ["FY B", "TOTAL"]`, `column_headers: ["TOTAL", "TOTAL"]`)
	pages := subtotalPages()
	for _, n := range []int{2, 4} {
		pages[n] = strings.Replace(pages[n], fmt.Sprintf("%10s%10s\n", "FY B", "TOTAL"),
			fmt.Sprintf("%10s%10s\n", "TOTAL", "TOTAL"), 1)
	}
	if _, err := subtotalCheck(t, src, pages); err != nil {
		t.Fatalf("two columns sharing a header: %v", err)
	}
	pages[2] = strings.Replace(pages[2], "$   12", "$   13", 1)
	_, err := subtotalCheck(t, src, pages)
	if err == nil || !strings.Contains(err.Error(), `"SUBTOTAL X"`) {
		t.Errorf("a wrong figure in the first of two like-headed columns: got %v, want it refused", err)
	}
}

// TestAChainWhoseColumnsMeanDifferentThingsIsRefused: a position whose second
// rule files under another year, category or (inherited) kind is refused.
func TestAChainWhoseColumnsMeanDifferentThingsIsRefused(t *testing.T) {
	last := func(src, old, repl string) string {
		i := strings.LastIndex(src, old)
		if i < 0 {
			t.Fatalf("no %q in the rules", old)
		}
		return src[:i] + repl + src[i+len(old):]
	}
	byColumn := strings.NewReplacer(
		"category: capital-projects}", "fund: 100, fund_group: general}",
		"{fiscal_year: 2025}", "{fiscal_year: 2025, category: transfers/out}",
		"{fiscal_year: 2026}", "{fiscal_year: 2026, category: transfers/out}",
	).Replace(subtotalRules)
	if _, err := subtotalCheck(t, byColumn, subtotalPages()); err != nil {
		t.Fatalf("the chain with its columns carrying the category: %v", err)
	}
	for _, tc := range []struct{ name, src string }{
		{"another year", last(subtotalRules, "{fiscal_year: 2026}", "{fiscal_year: 2027}")},
		{"another category", last(byColumn, "{fiscal_year: 2026, category: transfers/out}",
			"{fiscal_year: 2026, category: transfers/in}")},
		{"another kind", last(byColumn, "    kind: expenditure\n", "    kind: transfer_out\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := subtotalCheck(t, tc.src, subtotalPages())
			if err == nil || !strings.Contains(err.Error(), "compares columns by position") {
				t.Errorf("a chain whose second rule files a column under %s: got %v, want it refused", tc.name, err)
			}
		})
	}
}

// TestADeclaredChainCannotJoinARulesOwn: a rule with subtotal rows and no
// subtotal_chain is a chain of its own, whatever another rule names its chain.
func TestADeclaredChainCannotJoinARulesOwn(t *testing.T) {
	src := strings.Replace(subtotalRules, "    subtotal_chain: listing\n", "", 1)
	src = strings.Replace(src, "    subtotal_chain: listing\n", "    subtotal_chain: \"rule:pair-1\"\n", 1)
	f, err := parse(strings.NewReader(src), "subtotal.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(Chains(f)); got != 2 {
		t.Errorf("Chains = %d runs, want 2: pair-1's own and the one pair-2 names", got)
	}
}
