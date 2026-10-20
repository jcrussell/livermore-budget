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

// TestInferredColumnsKeepsTwoFlowsWhoseEndsShareLabels: two derived flows
// between printed nodes with different ids and the same labels are two
// flows, and one carried by two schedules of the column is still one.
// Mutation: key seenFlow by the inferredFlow of labels again -- the second
// flow is dropped and the test goes red.
func TestInferredColumnsKeepsTwoFlowsWhoseEndsShareLabels(t *testing.T) {
	doc := []byte(`{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2025-26","basis":"adopted"},` +
		`"nodes":[{"id":"fund/100","label":"General Fund"},{"id":"fund/101","label":"General Fund"},` +
		`{"id":"dept/a","label":"Police"},{"id":"dept/b","label":"Police"}],` +
		`"links":[{"source":"fund/100","target":"dept/a","value_cents":100,"fact_ids":["f1"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true},` +
		`{"source":"fund/101","target":"dept/b","value_cents":100,"fact_ids":["f1"],"locators":[{"doc_id":"d","pages":[66]}],"derived":true}]}`)
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

// TestInferredColumnsListsAFlowIntoADerivedNodeUnderIt: a derived flow into
// or out of a derived node is listed under that node, with the cents its link
// carries and the pages its locators cite; two flows sharing ends and citing
// other facts are two; a flow citing a page the sources page does not list,
// or one schedule restating a flow at another value, is refused.
// Mutation: skip a derived flow with a derived end, as inferredColumns once
// did -- the node arm goes red; drop the value check -- the restated arm does.
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
	if _, err := inferredColumns(o, ix, refs); err == nil || !strings.Contains(err.Error(), "another schedule") {
		t.Errorf("a flow restated at another value: got %v, want a refusal", err)
	}
}
