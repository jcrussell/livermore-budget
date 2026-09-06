package export_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// historySeries is one series of a history fixture document, small enough for a
// test to mutate.
type historySeries struct {
	label, kind, group string
}

// historyDoc is a minimal ACFR-shaped history document: every series spans the
// same two audited columns, and — as on the real p167 — two blocks may print
// the same row label, distinguished only by fund group.
//
// Hand-written rather than built by internal/project, for trendsDoc's reason:
// the packager consumes bytes and a test reaching for the producer would
// retire that seam.
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
			"fund": 0, "fund_name": "", "fund_group": s.group,
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
	_, err := export.Write(export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			"sankey":        goldenSankey(t),
			"fund-balances": doc,
		},
		Views: []export.View{
			{Path: export.IndexPath, Nav: "Budget flows",
				Template: export.SankeyTemplate, Projection: "sankey"},
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

// TestTheHistoryViewGroupsRowsUnderPrintedHeadings is the happy path: two rows
// printing the same label land under the two headings that tell them apart, in
// declared order, every figure a link.
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

// TestTheHistoryViewRefusesASeriesNoSectionClaims: a row under the wrong
// printed heading is a claim the schedule does not make, so an unclaimed one is
// an error rather than a guess.
func TestTheHistoryViewRefusesASeriesNoSectionClaims(t *testing.T) {
	_, err := writeHistorySite(t, historyDoc(balancesFixture()...),
		[]export.Section{{Heading: "General Fund", Kind: "fund_balance", FundGroup: "general"}})
	if err == nil {
		t.Fatal("a series no section claims was accepted")
	}
	if !strings.Contains(err.Error(), "no section of view") {
		t.Errorf("error %q does not name the unclaimed series", err)
	}
}

// TestTheHistoryViewRefusesASectionClaimingNoSeries: a heading over nothing
// says the schedule prints a block it does not.
func TestTheHistoryViewRefusesASectionClaimingNoSeries(t *testing.T) {
	_, err := writeHistorySite(t, historyDoc(balancesFixture()...),
		append(balancesSections(), export.Section{Heading: "Revenues", Kind: "revenue"}))
	if err == nil {
		t.Fatal("a section claiming no series was accepted")
	}
	if !strings.Contains(err.Error(), "carries no such series") {
		t.Errorf("error %q does not name the empty section", err)
	}
}

// TestTheHistoryViewRefusesTwoSectionsForOneKey: exact matching is what makes
// the grouping deterministic, and a contested key would decide by declaration
// order in silence.
func TestTheHistoryViewRefusesTwoSectionsForOneKey(t *testing.T) {
	_, err := writeHistorySite(t, historyDoc(balancesFixture()...),
		append(balancesSections(),
			export.Section{Heading: "General Fund again", Kind: "fund_balance", FundGroup: "general"}))
	if err == nil {
		t.Fatal("two sections for one (kind, fund group) were accepted")
	}
	if !strings.Contains(err.Error(), "cannot land under both headings") {
		t.Errorf("error %q does not name the contested key", err)
	}
}

// TestTheHistoryViewRefusesADocumentShortASeries is the missing-row proof: a
// document whose own counts promise more than its series carry is refused
// rather than rendered smaller.
func TestTheHistoryViewRefusesADocumentShortASeries(t *testing.T) {
	// Three series so the dropped one is NOT a whole section: its block keeps a
	// row, no heading goes empty, and the count reconciliation is the only
	// thing left standing between the reader and a quietly smaller table.
	raw := historyDoc(append(balancesFixture(),
		historySeries{label: "Assigned", kind: "fund_balance", group: "general"})...)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	series := doc["series"].([]any)
	doc["series"] = series[:2]
	short, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writeHistorySite(t, short, balancesSections())
	if err == nil {
		t.Fatal("a document short a series was accepted; the page would render fewer " +
			"rows than the document claims")
	}
	if !strings.Contains(err.Error(), "lost a figure") {
		t.Errorf("error %q does not reconcile the counts", err)
	}
}
