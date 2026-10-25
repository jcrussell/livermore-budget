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

// TestInferredColumnsListsEachCellOnce: two cells under one pair of labels
// are two entries; a cell another ribbon already draws adds none.
// Mutation: key the listing by ends and fact ids and the repeated cell is
// listed twice. Mutation: key it by labels and the second cell is dropped.
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
	flow := func(fact string) inferredFlow {
		return inferredFlow{From: "General Fund", To: "Police", Dollars: "$1", Facts: fact,
			Pages: []pageRef{inferredRefs()[Citation{DocID: "d", Page: 66}]}}
	}
	want := []inferredColumn{{Label: "FY 2025-26 adopted", Path: "fy2026-adopted.json",
		Flows: []inferredFlow{flow("f1"), flow("f2")}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("inferredColumns (-want +got):\n%s", diff)
	}
}

// TestInferredColumnsFilesAHubLegUnderItsDerivedNode: one cell drawn by a hub
// leg, a re-pointed leaf and a coarser schedule is listed once under the
// derived node, at the Repeats' ends, whichever stem sorts first.
// Mutation: drop the preference for a ribbon naming its Repeats and the
// coarse-first cell reads "General Fund group". Mutation: label every row
// from the first stem and the leaf reads "General Fund (spine)".
func TestInferredColumnsFilesAHubLegUnderItsDerivedNode(t *testing.T) {
	const meta = `{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2026-27","basis":"adopted"},`
	const draw = `{"id":"draw","label":"Fund Balance Draw","derived":true,"rationale":"why","source_note":"note"},`
	const repeats = `,"repeats":"draw -> fund/100 fund_balance"`
	hub := meta + `"nodes":[` + draw +
		`{"id":"group/general","label":"General Fund group"},{"id":"fund/100","label":"General Fund","parent":"group/general"},` +
		`{"id":"fund/900","label":"Utility"}],"links":[` +
		`{"source":"group/general","target":"fund/100","kind":"fund_balance","value_cents":150,"fact_ids":["f2","f1"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true` + repeats + `},` +
		`{"source":"draw","target":"group/general","kind":"fund_balance","value_cents":150,"fact_ids":["f1","f2"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true` + repeats + `},` +
		`{"source":"draw","target":"fund/900","kind":"fund_balance","value_cents":-250,"fact_ids":["f3"],"locators":[{"doc_id":"d","pages":[67]}],"derived":true}]}`
	coarse := meta + `"nodes":[` + draw +
		`{"id":"group/general","label":"General Fund group"},{"id":"fund/100","label":"General Fund (spine)"}],"links":[` +
		`{"source":"draw","target":"group/general","kind":"fund_balance","value_cents":150,"fact_ids":["f1","f2"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true}]}`
	ix := ColumnIndex{columns: map[string]string{"a": "fy2027-adopted.json", "b": "fy2027-adopted.json"}}
	refs := inferredRefs()
	p66, p67 := refs[Citation{DocID: "d", Page: 66}], refs[Citation{DocID: "d", Page: 67}]
	want := []inferredFlow{
		{From: "Fund Balance Draw", To: "General Fund", Dollars: "$1.50", Facts: "f1 f2", Pages: []pageRef{p66}},
		{From: "Fund Balance Draw", To: "Utility", Dollars: "-$2.50", Facts: "f3", Pages: []pageRef{p67}}}
	for _, tc := range []struct {
		name string
		a, b string
	}{
		{"coarse first", coarse, hub},
		{"hub first", hub, coarse},
	} {
		o := &Options{Projections: map[string][]byte{"a": []byte(tc.a), "b": []byte(tc.b)}}
		got, err := inferredColumns(o, ix, refs)
		if err != nil {
			t.Fatalf("%s: inferredColumns: %v", tc.name, err)
		}
		want := []inferredColumn{{Label: "FY 2026-27 adopted", Path: "fy2027-adopted.json",
			Nodes: []inferredNode{{Label: "Fund Balance Draw", Rationale: "why", SourceNote: "note", Flows: want}}}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s: inferredColumns (-want +got):\n%s", tc.name, diff)
		}
	}
}

// TestInferredColumnsListsAFlowIntoADerivedNodeUnderIt: a flow touching a
// derived node is listed under it; an unlisted page or a restated value is
// refused.
// Mutation: skip a derived flow with a derived end and the node arm goes red.
// Mutation: drop the value check and the restated arm goes red.
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
			{From: "Fund balance drawn", To: "General Fund", Dollars: "$1.50", Facts: "f1", Pages: []pageRef{p66}},
			{From: "Fund balance drawn", To: "General Fund", Dollars: "$2.50", Facts: "f2", Pages: []pageRef{p67}},
			{From: "Capital", To: "Fund balance drawn", Dollars: "$3", Facts: "f3", Pages: []pageRef{p67}},
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

// TestBuildCellsCitesThroughSourcesFor: a trend cell links through
// sourcesFor; a point citing an undescribed document refuses.
// Mutation: drop citedPage's registry check and the refusal arm goes red.
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

// TestInferredColumnsDropsARowOnlyWhereCompletesSaysSo: a completed cell is
// dropped only where completing rows cite all its facts and pages.
// Mutation: skip the page check and k's row and its p66 go. Mutation: drop a
// row its members merely sum to and h's row goes. Mutation: ignore Completes
// and g's row is listed beside its legs.
func TestInferredColumnsDropsARowOnlyWhereCompletesSaysSo(t *testing.T) {
	const meta = `{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2026-27","basis":"adopted"},`
	link := func(src, dst string, cents int, completes string, pages string, facts ...string) string {
		s := `{"source":"` + src + `","target":"` + dst + `","kind":"fund_balance","value_cents":` + strconv.Itoa(cents) +
			`,"fact_ids":["` + strings.Join(facts, `","`) + `"],"locators":[{"doc_id":"d","pages":[` + pages + `]}],"derived":true`
		if completes != "" {
			s += `,"completes":"` + completes + `"`
		}
		return s + `}`
	}
	nodes := `"nodes":[{"id":"draw","label":"draw","derived":true},{"id":"in","label":"in","derived":true},` +
		`{"id":"g","label":"g"},{"id":"h","label":"h"},{"id":"k","label":"k"},` +
		`{"id":"g1","label":"g1","parent":"g"},{"id":"g-only","label":"g-only","parent":"g"},` +
		`{"id":"h1","label":"h1","parent":"h"},{"id":"k1","label":"k1","parent":"k"}],"links":[`
	spine := meta + nodes + strings.Join([]string{
		link("draw", "g", 300, "", "66", "G"),
		link("h", "in", 50, "", "66", "H"),
		link("k", "in", 70, "", "66", "K"),
	}, ",") + `]}`
	expansion := meta + nodes + strings.Join([]string{
		link("draw", "g1", 100, "draw -> g fund_balance", "67", "a1"),
		link("draw", "g-only", 200, "draw -> g fund_balance", "66,67", "G", "a1"),
		link("h1", "in", 50, "", "67", "c1"),
		link("k1", "in", 70, "k -> in fund_balance", "67", "K", "x"),
	}, ",") + `]}`
	ix := ColumnIndex{columns: map[string]string{"a": "fy2027-adopted.json", "b": "fy2027-adopted.json"}}
	refs := inferredRefs()
	p66, p67 := refs[Citation{DocID: "d", Page: 66}], refs[Citation{DocID: "d", Page: 67}]
	o := &Options{Projections: map[string][]byte{"a": []byte(spine), "b": []byte(expansion)}}
	got, err := inferredColumns(o, ix, refs)
	if err != nil {
		t.Fatalf("inferredColumns: %v", err)
	}
	want := []inferredColumn{{Label: "FY 2026-27 adopted", Path: "fy2027-adopted.json", Nodes: []inferredNode{
		{Label: "draw", Flows: []inferredFlow{
			{From: "draw", To: "g1", Dollars: "$1", Facts: "a1", Pages: []pageRef{p67}},
			{From: "draw", To: "g-only", Dollars: "$2", Facts: "G a1", Pages: []pageRef{p66, p67}},
		}},
		{Label: "in", Flows: []inferredFlow{
			{From: "h", To: "in", Dollars: "$0.50", Facts: "H", Pages: []pageRef{p66}},
			{From: "k", To: "in", Dollars: "$0.70", Facts: "K", Pages: []pageRef{p66}},
			{From: "h1", To: "in", Dollars: "$0.50", Facts: "c1", Pages: []pageRef{p67}},
			{From: "k1", To: "in", Dollars: "$0.70", Facts: "K x", Pages: []pageRef{p67}},
		}},
	}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("inferredColumns (-want +got):\n%s", diff)
	}
}
