package export_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// historySeries is one series of a history fixture document, small enough for a
// test to mutate.
type historySeries struct {
	label, kind, group string
}

// historyDoc is a hand-written ACFR-shaped history document: two audited
// columns, and, as on p167, one row label in two blocks told apart by fund group.
func historyDoc(series ...historySeries) []byte {
	years := []int{2016, 2017}
	columns := make([]map[string]any, 0, len(years))
	for _, y := range years {
		columns = append(columns, map[string]any{
			"fiscal_year":       y,
			"fiscal_year_label": fmt.Sprintf("FY %d-%02d", y-1, y%100),
			"basis":             "audited",
			"comparable_group":  "audited",
		})
	}
	out := make([]map[string]any, 0, len(series))
	points := 0
	for i, s := range series {
		pts := make([]map[string]any, 0, len(years))
		for j, y := range years {
			points++
			pts = append(pts, map[string]any{
				"fiscal_year": y, "basis": "audited", "amount_cents": int64(1000*i + j),
				"fact_id": fmt.Sprintf("fisc-f-%06d%06d", i, j), "doc_id": budgetDocID,
				"page": 167, "offset": 10*i + j, "token": "1,000", "derived": false,
			})
		}
		out = append(out, map[string]any{
			"series_id": fmt.Sprintf("fisc-s-%012d", i), "label": s.label,
			"fund": nil, "fund_name": "", "fund_group": s.group,
			"kind": s.kind, "category": "fund-balance/committed",
			"category_label": "Committed", "points": pts,
		})
	}
	doc := map[string]any{
		"schema_version": 1,
		"projection":     "fund-balances",
		"metadata": map[string]any{
			"generated_by": "fisc test",
			"scope":        "acfr-fund-balances",
			"currency":     "USD",
			"units":        "cents",
			"columns":      columns,
			"sources":      []map[string]any{{"doc_id": budgetDocID, "pages": []int{167}}},
			"counts":       map[string]any{"facts": points, "series": len(series), "points": points},
			"caveats":      []map[string]any{},
		},
		"series": out,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return b
}

// balancesFixture is p167 in miniature: the two blocks print the same label.
func balancesFixture() []historySeries {
	return []historySeries{
		{label: "Committed", kind: "fund_balance", group: "general"},
		{label: "Committed", kind: "fund_balance", group: ""},
	}
}

// balancesSections is the declaration the real view makes for that shape.
func balancesSections() []export.Section {
	return []export.Section{
		{Heading: "General Fund", Kind: "fund_balance", FundGroup: "general"},
		{Heading: "All Other Governmental Funds", Kind: "fund_balance"},
	}
}

// writeHistorySite exports one history view over the given document bytes.
func writeHistorySite(t *testing.T, doc []byte, sections []export.Section) (string, error) {
	t.Helper()
	dir := t.TempDir()
	_, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey":        goldenSankey(t),
			"fund-balances": doc,
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Overview: export.Chart{Form: export.SankeyForm, Sankey: &export.SankeyHints{}}, Projection: "sankey"},
			{Path: "balances.html", Nav: "Fund balances",
				Template: export.HistoryTemplate, Projection: "fund-balances",
				Sections: sections, Title: "Fund balances", Lede: "A lede."},
		},
		Docs:        budgetDocs(),
		GeneratedBy: "fisc test",
		PageText:    twoViewPageText(167),
	})
	return dir, err
}

// TestTheHistoryViewGroupsRowsUnderPrintedHeadings: two rows of one label
// land under their two headings, in declared order, every figure a link.
func TestTheHistoryViewGroupsRowsUnderPrintedHeadings(t *testing.T) {
	dir, err := writeHistorySite(t, historyDoc(balancesFixture()...), balancesSections())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "balances.html"))
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	html := string(b)
	gf := strings.Index(html, ">General Fund</th>")
	other := strings.Index(html, ">All Other Governmental Funds</th>")
	if gf < 0 || other < 0 {
		t.Fatalf("the page is missing a block heading (general at %d, other at %d)", gf, other)
	}
	if other < gf {
		t.Error("the blocks render out of declared order")
	}
	if got := strings.Count(html, `<th scope="row">Committed</th>`); got != 2 {
		t.Errorf("got %d Committed rows, want the two same-labelled blocks", got)
	}
	if got := strings.Count(html, `title="Read on page 167"`); got != 4 {
		t.Errorf("got %d cited cells, want every one of the document's 4 figures", got)
	}
	if strings.Contains(html, "app.js") {
		t.Error("the history page ships app.js; it is server-rendered and must stay so")
	}
}

// TestTheHistoryViewRefuses: each row is a document and a view's sections
// that do not claim each other's rows one to one, and wants the refusal to
// name the mismatch rather than the page to guess.
func TestTheHistoryViewRefuses(t *testing.T) {
	for _, row := range []struct {
		name     string
		doc      func(t *testing.T) []byte
		sections func() []export.Section
		want     string
	}{
		// An unclaimed series is an error rather than a guess.
		{"TheHistoryViewRefusesASeriesNoSectionClaims", balancesDoc, func() []export.Section {
			return []export.Section{{Heading: "General Fund", Kind: "fund_balance", FundGroup: "general"}}
		}, "no section of view"},
		// A heading over nothing says the schedule prints a block it does not.
		{"TheHistoryViewRefusesASectionClaimingNoSeries", balancesDoc, func() []export.Section {
			return append(balancesSections(), export.Section{Heading: "Revenues", Kind: "revenue"})
		}, "carries no such series"},
		// A contested key is refused rather than settled by declaration order.
		{"TheHistoryViewRefusesTwoSectionsForOneKey", balancesDoc, func() []export.Section {
			return append(balancesSections(),
				export.Section{Heading: "General Fund again", Kind: "fund_balance", FundGroup: "general"})
		}, "cannot land under both headings"},
		// A document whose counts promise more series than it carries is
		// refused: the page would render fewer rows than the document claims.
		// Three series, so the dropped one empties no heading and only the
		// count reconciliation can catch it.
		{"TheHistoryViewRefusesADocumentShortASeries", func(t *testing.T) []byte {
			raw := historyDoc(append(balancesFixture(),
				historySeries{label: "Assigned", kind: "fund_balance", group: "general"})...)
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			doc["series"] = doc["series"].([]any)[:2]
			short, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			return short
		}, balancesSections, "lost a figure"},
		// A second series at the excess section's key is caught only by the
		// section's Rows; it would render under the excess heading silently.
		{"TheHistoryViewRefusesASeriesItsSectionDoesNotName", func(*testing.T) []byte {
			return historyDoc(append(changesFixture(),
				historySeries{label: "Net change in fund balances", kind: "fund_balance", group: ""})...)
		}, changesSections, "do not name it"},
		// A Rows key matching no series is an error.
		{"TheHistoryViewRefusesARowNameMatchingNoSeries", func(*testing.T) []byte {
			return historyDoc(changesFixture()...)
		}, func() []export.Section {
			sections := changesSections()
			sections[1].Rows["Net change in fund balances"] = ""
			return sections
		}, "names row"},
	} {
		t.Run(row.name, func(t *testing.T) {
			_, err := writeHistorySite(t, row.doc(t), row.sections())
			if err == nil || !strings.Contains(err.Error(), row.want) {
				t.Errorf("Write = %v, want a refusal naming %q", err, row.want)
			}
		})
	}
}

// balancesDoc is the balances fixture as a history document.
func balancesDoc(*testing.T) []byte { return historyDoc(balancesFixture()...) }

// changesFixture is p168 in miniature: an open revenue block, and the single
// excess row whose printed label is the tail of a three-line wrap.
func changesFixture() []historySeries {
	return []historySeries{
		{label: "Sales taxes", kind: "revenue", group: ""},
		{label: "over (under) expenditures", kind: "fund_balance", group: ""},
	}
}

// changesSections is the real view's declaration for that shape: revenue
// open, excess closed on its one row under the whole printed phrase.
func changesSections() []export.Section {
	return []export.Section{
		{Heading: "Revenues", Kind: "revenue"},
		{Heading: "Excess of revenues over (under) expenditures", Kind: "fund_balance",
			Rows: map[string]string{
				"over (under) expenditures": "Excess of Revenues over (under) expenditures",
			}},
	}
}

// TestTheHistoryViewDisplaysADeclaredRowLabel: a section's Rows may give a
// wrap-fragment row its whole printed phrase, an empty value keeps the printed
// label, and a section without Rows is untouched.
func TestTheHistoryViewDisplaysADeclaredRowLabel(t *testing.T) {
	sections := changesSections()
	sections[0].Rows = map[string]string{"Sales taxes": ""}
	dir, err := writeHistorySite(t, historyDoc(changesFixture()...), sections)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "balances.html"))
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	html := string(b)
	if !strings.Contains(html, `<th scope="row">Excess of Revenues over (under) expenditures</th>`) {
		t.Error("the declared display label is not the row's visible name")
	}
	if strings.Contains(html, `<th scope="row">over (under) expenditures</th>`) {
		t.Error("the wrap fragment is still a row's visible name")
	}
	if !strings.Contains(html, `<th scope="row">Sales taxes</th>`) {
		t.Error("a row named with an empty display label lost its printed label")
	}
}

// TestAnUnauditedDocumentsColumnsAreLabelledUnaudited: history and change
// label columns by basisLabelFor, in the chip and every mark's tooltip.
// Mutation: label buildHistoryPage's or buildChangePage's columns with the
// document's raw basis and that page's arm goes red on "audited".
func TestAnUnauditedDocumentsColumnsAreLabelledUnaudited(t *testing.T) {
	const caveat = `"caveats":[{"id":"` + project.UnauditedCaveatID + `","summary":"Unaudited.",` +
		`"theme":"left-out","text":"No assurance.","applies_to":[]}]`
	chip := regexp.MustCompile(`<span class="basis">([^<]*)</span>`)
	tooltip := regexp.MustCompile(`<(?:circle|rect)\b[^>]*>\s*<title>([^<]*)</title>`)

	doc := strings.Replace(string(historyDoc(balancesFixture()...)), `"caveats":[]`, caveat, 1)
	dir, err := writeHistorySite(t, []byte(doc), balancesSections())
	if err != nil {
		t.Fatalf("Write the history page: %v", err)
	}
	pages := map[string]string{"balances.html": readFile(t, dir, "balances.html")}

	dir = t.TempDir()
	o := changeOptions(t, dir, false)
	o.Projections[project.ChangesProjection] = []byte(strings.Replace(
		string(o.Projections[project.ChangesProjection]), `"caveats":[]`, caveat, 1))
	if _, err := writeSite(o); err != nil {
		t.Fatalf("Write the change page: %v", err)
	}
	pages["change.html"] = readFile(t, dir, "change.html")

	for name, page := range pages {
		chips := chip.FindAllStringSubmatch(page, -1)
		tips := tooltip.FindAllStringSubmatch(page, -1)
		// change.js draws change.html's marks, so only the history page
		// renders a mark's tooltip.
		if len(chips) == 0 || (name != "change.html" && len(tips) == 0) {
			t.Errorf("%s renders %d basis chips and %d mark tooltips, so this asserts nothing of one", name, len(chips), len(tips))
		}
		for _, m := range append(chips, tips...) {
			if strings.Contains(strings.ReplaceAll(strings.ToLower(m[1]), "unaudited", ""), "audit") {
				t.Errorf("%s labels a column %q under the %s caveat", name, m[1], project.UnauditedCaveatID)
			}
		}
	}
}
