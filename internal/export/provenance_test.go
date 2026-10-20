package export

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestInferredColumnsKeepsTwoFlowsWhoseEndsShareLabels: two derived flows
// between printed nodes with different ids and the same labels are two
// flows, and one carried by two schedules of the column is still one.
// Mutation: key seenFlow by the inferredFlow of labels again -- the second
// flow is dropped and the test goes red.
func TestInferredColumnsKeepsTwoFlowsWhoseEndsShareLabels(t *testing.T) {
	doc := []byte(`{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2025-26","basis":"adopted"},` +
		`"nodes":[{"id":"fund/100","label":"General Fund"},{"id":"fund/101","label":"General Fund"},` +
		`{"id":"dept/a","label":"Police"},{"id":"dept/b","label":"Police"}],` +
		`"links":[{"source":"fund/100","target":"dept/a","derived":true},` +
		`{"source":"fund/101","target":"dept/b","derived":true}]}`)
	o := &Options{Projections: map[string][]byte{"a": doc, "b": doc}}
	ix := ColumnIndex{columns: map[string]string{"a": "fy2026-adopted.json", "b": "fy2026-adopted.json"}}
	got, err := inferredColumns(o, ix)
	if err != nil {
		t.Fatalf("inferredColumns: %v", err)
	}
	flow := inferredFlow{From: "General Fund", To: "Police"}
	want := []inferredColumn{{Label: "FY 2025-26 adopted", Path: "fy2026-adopted.json",
		Flows: []inferredFlow{flow, flow}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("inferredColumns (-want +got):\n%s", diff)
	}
}
