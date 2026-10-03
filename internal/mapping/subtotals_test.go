package mapping

import (
	"fmt"
	"strings"
	"testing"
)

// The shape these tests are about, as Budget Book pp.224-235 print it: a
// labelled page carrying one year, a label-less page continuing the same rows,
// subtotals of two levels between the blocks, and a block that starts on one
// page pair and is totalled on the next.
//
//	p1  P1 Alpha    10     p2   5   15
//	    P2 Beta     20          7   27
//	    SUBTOTAL X  30         12   42    level 1
//	    P3 Gamma     1          2    3
//	p3  SUBTOTAL Y   1     p4   2    3    level 1, over p1's Gamma
//	    TOTAL FUND  31         14   45    level 2, over Alpha, Beta, Gamma
//	    P4 Delta     4          4    8
//	    TOTAL D      4          4    8    level 2
//	    TOTAL       35         18   53    level 3
//
// The second page's TOTAL column is skipped, and is still compared.
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

// textGeometry is the geometry a monospaced page implies: each word at its
// character column, six points a character, one line every twelve points.
func textGeometry(docID string, pages map[int]string) map[int]string {
	out := map[int]string{}
	for n, text := range pages {
		var words []word
		for i, line := range strings.Split(text, "\n") {
			for c := 0; c < len(line); {
				if line[c] == ' ' {
					c++
					continue
				}
				e := c
				for e < len(line) && line[e] != ' ' {
					e++
				}
				words = append(words, word{x0: float64(c * 6), y0: float64(i * 12),
					x1: float64(e * 6), y1: float64(i*12 + 10), text: line[c:e]})
				c = e
			}
		}
		out[n] = geomJSON(docID, n, words)
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

// TestASubtotalChainTiesAcrossRulesAndLevels is the whole capability: five
// printed subtotals, three levels, one block crossing from the first rule into
// the second, and the skipped TOTAL column compared like the others.
//
// Mutations: reset every level at every subtotal, and TOTAL FUND sees only
// Gamma; drop the chain, and SUBTOTAL Y sees no row above it.
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

// TestASubtotalTheRowsDoNotSumToIsRefused poses one wrong figure per kind of
// column: a published one, and the skipped TOTAL the rule never publishes.
func TestASubtotalTheRowsDoNotSumToIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, want string
		page                 int
	}{
		{"a published column", "$   30", "$   31", `"SUBTOTAL X"`, 1},
		{"a skipped column", "$   42", "$   43", `"TOTAL"`, 2},
		{"a level-2 total", "$   31", "$   30", `"TOTAL FUND": prints 30 under "FY A" on p3`, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := subtotalPages()
			if !strings.Contains(pages[tc.page], tc.from) {
				t.Fatalf("the mutation %q matches nothing on page %d", tc.from, tc.page)
			}
			pages[tc.page] = strings.Replace(pages[tc.page], tc.from, tc.to, 1)
			_, err := subtotalCheck(t, subtotalRules, pages)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want a refusal naming %s", err, tc.want)
			}
		})
	}
}

// TestASubtotalChainIsWhatCarriesABlockAcrossRules: without the chain each
// rule is its own, so Gamma sits under no subtotal of pair-1 and SUBTOTAL Y
// sums nothing.
func TestASubtotalChainIsWhatCarriesABlockAcrossRules(t *testing.T) {
	src := strings.ReplaceAll(subtotalRules, "    subtotal_chain: listing\n", "")
	_, err := subtotalCheck(t, src, subtotalPages())
	if err == nil || !strings.Contains(err.Error(), `rule "pair-1"`) ||
		!strings.Contains(err.Error(), "inside no printed total") {
		t.Errorf("two unchained rules: got %v, want pair-1's Gamma refused as under no total", err)
	}
}

// TestARowUnderNoSubtotalIsRefused: the table's last subtotal must cover its
// last row. Demote the grand TOTAL to a plain skipped row and it is a row
// under no printed total.
func TestARowUnderNoSubtotalIsRefused(t *testing.T) {
	from := "      - label: \"TOTAL\"\n        skip: true\n        subtotal: 3\n        #DELTAS\n"
	if !strings.Contains(subtotalRules, from) {
		t.Fatal("the mutation matches nothing")
	}
	src := strings.Replace(subtotalRules, from, "      - {label: \"TOTAL\", skip: true}\n", 1)
	_, err := subtotalCheck(t, src, subtotalPages())
	if err == nil || !strings.Contains(err.Error(), "inside no printed total") {
		t.Errorf("a row after the last subtotal: got %v, want it refused", err)
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

// TestTwoSkippedRowsMayShareALabel is p230's fund 611, printed once under its
// federal grant and once under its state grant on lines identical but for
// the figures. Skipped, the pair reads; omitted, it cannot say which.
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
	omitted := strings.Replace(src, "        column_headers: [\"FY A\"]\n", "        omitted_rows: [\"P1 Alpha\"]\n        column_headers: [\"FY A\"]\n", 1)
	if _, err := parse(strings.NewReader(omitted), "subtotal.yaml"); err == nil ||
		!strings.Contains(err.Error(), "more than once") {
		t.Errorf("an omission naming a repeated row: got %v, want it refused", err)
	}
}

// TestASubtotalOverNoRowIsRefused: a grand total declared one level too low
// follows a level-2 total that cleared its sum, so it would compare nothing
// and be counted as tied.
func TestASubtotalOverNoRowIsRefused(t *testing.T) {
	from := "        subtotal: 3\n"
	if !strings.Contains(subtotalRules, from) {
		t.Fatal("the mutation matches nothing")
	}
	src := strings.Replace(subtotalRules, from, "        subtotal: 2\n", 1)
	_, err := subtotalCheck(t, src, subtotalPages())
	if err == nil || !strings.Contains(err.Error(), "over no row") {
		t.Errorf("a subtotal with no row since its level last cleared: got %v, want it refused", err)
	}
}

// TestAColumnIsItsPositionNotItsHeader: Budget Book p81 prints Principal,
// Interest and Total once a year, so two columns may share a header. Keyed by
// header, the second column's figures overwrite the first's and a wrong
// subtotal in the first is never compared.
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

// TestAChainLaidOutUnlikeItsFirstRuleIsRefused: a chain matches columns by
// position, so its rules must agree on what each position is.
func TestAChainLaidOutUnlikeItsFirstRuleIsRefused(t *testing.T) {
	i := strings.LastIndex(subtotalRules, `column_headers: ["FY B", "TOTAL"]`)
	src := subtotalRules[:i] + `column_headers: ["FY C", "TOTAL"]` +
		subtotalRules[i+len(`column_headers: ["FY B", "TOTAL"]`):]
	pages := subtotalPages()
	pages[4] = strings.Replace(pages[4], fmt.Sprintf("%10s", "FY B"), fmt.Sprintf("%10s", "FY C"), 1)
	_, err := subtotalCheck(t, src, pages)
	if err == nil || !strings.Contains(err.Error(), "compares columns by position") {
		t.Errorf("a chain whose second rule names its columns differently: got %v, want it refused", err)
	}
}

// TestAChainWhoseColumnsMeanDifferentThingsIsRefused: headers alike are not
// enough. A position whose second rule files its figures under another year,
// or, on a chain whose columns carry the category, under another category or
// another kind, would be summed with the first rule's figures as though they
// were one column. The kind case declares none on the column, so it is the
// rule's kind the column inherits that differs.
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

// TestASubtotalOverARowNoPagePrintsIsRefused: a row omitted from every part
// adds nothing, so a subtotal over it alone sums nothing, and a printed dash
// there would tie against an empty sum.
func TestASubtotalOverARowNoPagePrintsIsRefused(t *testing.T) {
	src := strings.Replace(subtotalRules,
		"      - {label: \"SUBTOTAL X\", skip: true, subtotal: 1}\n",
		"      - {label: \"SUBTOTAL X\", skip: true, subtotal: 1}\n"+
			"      - {label: \"P9 Ghost\", category: capital-projects}\n"+
			"      - {label: \"ZERO TOTAL\", skip: true, subtotal: 1}\n", 1)
	src = strings.Replace(src, "        column_headers: [\"FY A\"]\n",
		"        omitted_rows: [\"P9 Ghost\"]\n        column_headers: [\"FY A\"]\n", 1)
	src = strings.Replace(src, "        column_headers: [\"FY B\", \"TOTAL\"]\n",
		"        omitted_rows: [\"P9 Ghost\"]\n        column_headers: [\"FY B\", \"TOTAL\"]\n", 1)
	pages := subtotalPages()
	pages[1] = strings.Replace(pages[1], "P3 Gamma", fmt.Sprintf("%-12s%10s\nP3 Gamma", "  ZERO TOTAL", "-"), 1)
	pages[2] = strings.Replace(pages[2], fmt.Sprintf("%10s%10s\n", "2", "3"),
		fmt.Sprintf("%10s%10s\n%10s%10s\n", "-", "-", "2", "3"), 1)
	_, err := subtotalCheck(t, src, pages)
	if err == nil || !strings.Contains(err.Error(), `"ZERO TOTAL"`) || !strings.Contains(err.Error(), "over no row") {
		t.Errorf("a subtotal over a row no page prints: got %v, want it refused", err)
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

// TestAChainWithNoSubtotalSaysWhere: a chain named on a rule with no subtotal
// row is refused with the document, rule and page, like every resolve failure.
func TestAChainWithNoSubtotalSaysWhere(t *testing.T) {
	src := strings.Replace(subtotalRules, "      - {label: \"SUBTOTAL X\", skip: true, subtotal: 1}\n",
		"      - {label: \"SUBTOTAL X\", skip: true}\n", 1)
	src = strings.Replace(src, "    subtotal_chain: listing\n", "    subtotal_chain: lonely\n", 1)
	_, err := subtotalCheck(t, src, subtotalPages())
	for _, want := range []string{"subtotal-doc p1", `"pair-1"`, `"lonely"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("a chain with no subtotal: got %v, want it to name %s", err, want)
		}
	}
}

// TestASubtotalRefusalNamesThePageOfItsColumn: a delta naming a column no
// figure is summed under -- a TOTAL column read as a quantity -- is refused on
// the page that prints that column, p4, not the rule's first, p3.
//
// Mutation: report the rule's first page, and this names p3.
func TestASubtotalRefusalNamesThePageOfItsColumn(t *testing.T) {
	src := strings.ReplaceAll(subtotalRules, "          - {fiscal_year: 2026}\n          - {skip: true}\n",
		"          - {fiscal_year: 2026}\n          - {quantity: number}\n")
	src = strings.Replace(src, "#DELTAS",
		`subtotal_deltas: [{column: "TOTAL", delta_cents: 100, note: "x"}]`, 1)
	_, err := subtotalCheck(t, src, subtotalPages())
	if err == nil || !strings.Contains(err.Error(), "subtotal-doc p4") ||
		!strings.Contains(err.Error(), `under "TOTAL" on p4`) {
		t.Errorf("a delta under a quantity column: got %v, want it refused on p4", err)
	}
}
