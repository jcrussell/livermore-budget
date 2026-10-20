package export

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// inferredRefs is the sources page's citations for pages 66 and 67 of doc d.
func inferredRefs() map[Citation]pageRef {
	return map[Citation]pageRef{
		{DocID: "d", Page: 66}: {Number: 66, PDFURL: "d.pdf#page=66", TextURL: "extracted/d/pages/p0066.txt"},
		{DocID: "d", Page: 67}: {Number: 67, PDFURL: "d.pdf#page=67", TextURL: "extracted/d/pages/p0067.txt"},
	}
}

// TestInferredColumnsListsEachCellOnce: two derived flows between printed
// nodes with the same labels and different cells are two entries, and a
// ribbon citing a cell another ribbon already draws -- in another schedule or
// the same one, between other ends -- adds none.
// Mutation: key the listing by a flow's ends and its fact ids -- the repeated
// cell is listed twice and the test goes red; key it by the flow's labels --
// the second cell is dropped.
func TestInferredColumnsListsEachCellOnce(t *testing.T) {
	doc := []byte(`{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2025-26","basis":"adopted"},` +
		`"nodes":[{"id":"fund/100","label":"General Fund"},{"id":"fund/101","label":"General Fund"},` +
		`{"id":"dept/a","label":"Police"},{"id":"dept/b","label":"Police"}],` +
		`"links":[{"source":"fund/100","target":"dept/a","value_cents":100,"fact_ids":["f1"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true},` +
		`{"source":"fund/101","target":"dept/b","value_cents":100,"fact_ids":["f2"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true},` +
		`{"source":"fund/100","target":"dept/b","value_cents":100,"fact_ids":["f1"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true}]}`)
	o := &Options{Projections: map[string][]byte{"a": doc, "b": doc}}
	ix := ColumnIndex{columns: map[string]string{"a": "fy2026-adopted.json", "b": "fy2026-adopted.json"}}
	got, err := inferredColumns(o, ix, inferredRefs())
	if err != nil {
		t.Fatalf("inferredColumns: %v", err)
	}
	flow := inferredFlow{From: "General Fund", To: "Police", Dollars: "$1", Pages: []pageRef{inferredRefs()[Citation{DocID: "d", Page: 66}]}}
	want := []inferredColumn{{Label: "FY 2025-26 adopted", Path: "fy2026-adopted.json",
		Flows: []inferredFlow{flow, flow}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("inferredColumns (-want +got):\n%s", diff)
	}
}

// TestInferredColumnsFilesAHubLegUnderItsDerivedNode: a hub leg between
// printed nodes repeating the cells of a flow out of a derived node is listed
// once, under that node and not among the flows between printed nodes, and
// the listing is the same whichever schedule's stem sorts first. A contra
// flow prints at its printed sign.
// Mutation: list the first ribbon met for a set of cells rather than the
// best-ranked one -- with the hub's stem first the cell lands among the
// printed nodes' flows and the first arm goes red.
func TestInferredColumnsFilesAHubLegUnderItsDerivedNode(t *testing.T) {
	const meta = `{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2026-27","basis":"adopted"},`
	hub := meta + `"nodes":[{"id":"draw","label":"Fund Balance Draw","derived":true,"rationale":"why","source_note":"note"},` +
		`{"id":"group/general","label":"General Fund"},{"id":"fund/100","label":"General Fund"}],"links":[` +
		`{"source":"group/general","target":"fund/100","value_cents":150,"fact_ids":["f2","f1"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true},` +
		`{"source":"draw","target":"group/general","value_cents":150,"fact_ids":["f1","f2"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true}]}`
	leaf := meta + `"nodes":[{"id":"draw","label":"Fund Balance Draw","derived":true,"rationale":"why","source_note":"note"},` +
		`{"id":"fund/100","label":"General Fund"},{"id":"fund/900","label":"Utility"}],"links":[` +
		`{"source":"draw","target":"fund/100","value_cents":150,"fact_ids":["f1","f2"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true},` +
		`{"source":"draw","target":"fund/900","value_cents":-250,"fact_ids":["f3"],"locators":[{"doc_id":"d","pages":[67]}],"derived":true}]}`
	ix := ColumnIndex{columns: map[string]string{"a": "fy2027-adopted.json", "b": "fy2027-adopted.json"}}
	refs := inferredRefs()
	p66, p67 := refs[Citation{DocID: "d", Page: 66}], refs[Citation{DocID: "d", Page: 67}]
	for _, tc := range []struct {
		name string
		a, b string
		want []inferredFlow
	}{
		{"hub first", hub, leaf, []inferredFlow{
			{From: "Fund Balance Draw", To: "General Fund", Dollars: "$1.50", Pages: []pageRef{p66}},
			{From: "Fund Balance Draw", To: "Utility", Dollars: "-$2.50", Pages: []pageRef{p67}}}},
		{"leaf first", leaf, hub, []inferredFlow{
			{From: "Fund Balance Draw", To: "General Fund", Dollars: "$1.50", Pages: []pageRef{p66}},
			{From: "Fund Balance Draw", To: "Utility", Dollars: "-$2.50", Pages: []pageRef{p67}}}},
	} {
		o := &Options{Projections: map[string][]byte{"a": []byte(tc.a), "b": []byte(tc.b)}}
		got, err := inferredColumns(o, ix, refs)
		if err != nil {
			t.Fatalf("%s: inferredColumns: %v", tc.name, err)
		}
		want := []inferredColumn{{Label: "FY 2026-27 adopted", Path: "fy2027-adopted.json",
			Nodes: []inferredNode{{Label: "Fund Balance Draw", Rationale: "why", SourceNote: "note", Flows: tc.want}}}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s: inferredColumns (-want +got):\n%s", tc.name, diff)
		}
	}
}

// TestInferredColumnsListsAFlowIntoADerivedNodeUnderIt: a derived flow into
// or out of a derived node is listed under that node, with the cents its link
// carries and the pages its locators cite; two flows sharing ends and citing
// other facts are two; a flow citing a page the sources page does not list,
// or one schedule restating a flow at another value, is refused.
// Mutation: skip a derived flow with a derived end -- the node arm goes red;
// drop the value check -- the restated arm does.
func TestInferredColumnsListsAFlowIntoADerivedNodeUnderIt(t *testing.T) {
	const head = `{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2026-27","basis":"adopted"},` +
		`"nodes":[{"id":"draw","label":"Fund balance drawn","derived":true,"rationale":"why","source_note":"note"},` +
		`{"id":"fund/100","label":"General Fund"},{"id":"fund/200","label":"Capital"}],"links":[`
	link := func(src, dst string, cents int, fact string, page int) string {
		return `{"source":"` + src + `","target":"` + dst + `","value_cents":` + strconv.Itoa(cents) +
			`,"fact_ids":["` + fact + `"],"locators":[{"doc_id":"d","pages":[` + strconv.Itoa(page) + `]}],"derived":true}`
	}
	ix := ColumnIndex{columns: map[string]string{"a": "fy2027-adopted.json", "b": "fy2027-adopted.json"}}
	refs := inferredRefs()

	doc := head + strings.Join([]string{
		link("draw", "fund/100", 150, "f1", 66),
		link("draw", "fund/100", 250, "f2", 67),
		link("fund/200", "draw", 300, "f3", 67),
	}, ",") + `]}`
	o := &Options{Projections: map[string][]byte{"a": []byte(doc), "b": []byte(doc)}}
	got, err := inferredColumns(o, ix, refs)
	if err != nil {
		t.Fatalf("inferredColumns: %v", err)
	}
	p66, p67 := refs[Citation{DocID: "d", Page: 66}], refs[Citation{DocID: "d", Page: 67}]
	want := []inferredColumn{{Label: "FY 2026-27 adopted", Path: "fy2027-adopted.json",
		Nodes: []inferredNode{{Label: "Fund balance drawn", Rationale: "why", SourceNote: "note", Flows: []inferredFlow{
			{From: "Fund balance drawn", To: "General Fund", Dollars: "$1.50", Pages: []pageRef{p66}},
			{From: "Fund balance drawn", To: "General Fund", Dollars: "$2.50", Pages: []pageRef{p67}},
			{From: "Capital", To: "Fund balance drawn", Dollars: "$3", Pages: []pageRef{p67}},
		}}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("inferredColumns (-want +got):\n%s", diff)
	}

	unlisted := head + link("draw", "fund/100", 150, "f1", 68) + `]}`
	o = &Options{Projections: map[string][]byte{"a": []byte(unlisted)}}
	if _, err := inferredColumns(o, ix, refs); err == nil || !strings.Contains(err.Error(), "does not list") {
		t.Errorf("a flow citing an unlisted page: got %v, want a refusal", err)
	}

	restated := head + link("draw", "fund/100", 999, "f1", 66) + `]}`
	o = &Options{Projections: map[string][]byte{"a": []byte(doc), "b": []byte(restated)}}
	if _, err := inferredColumns(o, ix, refs); err == nil || !strings.Contains(err.Error(), "another ribbon") {
		t.Errorf("a flow restated at another value: got %v, want a refusal", err)
	}
}

// TestBuildCellsCitesThroughSourcesFor: a trend cell's links are the ones
// sourcesFor composes for its page, and a point citing a document the
// registry does not describe refuses rather than publishing a cell with no
// PDF link.
// Mutation: drop citedPage's registry check -- the missing document's cell
// exports with an empty PDF link and the refusal arm goes red.
func TestBuildCellsCitesThroughSourcesFor(t *testing.T) {
	base := func(id string) string { return "extracted/" + id + "/pages/" }
	byID := map[string]Doc{"d": {ID: "d", PDFURL: "d.pdf"}}
	meta := []trendColumnMeta{{FiscalYear: 2025, Basis: "actual"}}
	columns := []columnRef{{Label: "FY 2024-25"}}
	cells, placed, err := buildCells([]trendPoint{{FiscalYear: 2025, Basis: "actual", AmountCents: 100, DocID: "d", Page: 67}},
		columns, meta, newCellCites(byID, base))
	if err != nil {
		t.Fatalf("buildCells: %v", err)
	}
	refs, _ := sourcesFor([]sourceMeta{{DocID: "d", Pages: []int{67}}}, byID, base, nil)
	want := refs[0].Pages[0]
	if placed != 1 || cells[0].Href != want.TextURL || cells[0].PDF != want.PDFURL {
		t.Errorf("cell cites %q and %q (placed %d), want sourcesFor's %q and %q", cells[0].Href, cells[0].PDF, placed, want.TextURL, want.PDFURL)
	}
	_, _, err = buildCells([]trendPoint{{FiscalYear: 2025, Basis: "actual", AmountCents: 100, DocID: "gone", Page: 67}},
		columns, meta, newCellCites(byID, base))
	if err == nil || !strings.Contains(err.Error(), "gone p67") {
		t.Errorf("buildCells over an undescribed document: err = %v, want a refusal naming gone p67", err)
	}
}
