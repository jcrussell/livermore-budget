package project

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestTheSchemasStateWhatTheProjectionsCarry holds schema/projection.schema.json
// and schema/series.schema.json to the structs [encode] marshals: every JSON
// name a document can carry is a property there, and nothing is a property there
// a document cannot carry.
//
// EQUALITY AND NOT CONTAINMENT, which is the difference between this side and
// internal/export's. These structs ARE the document, so a name on one side and
// not the other is a defect whichever side it is on: a key the schema does not
// state is one additionalProperties would refuse at the next build, and a
// property no struct emits is a claim the contract makes about nothing.
//
// IT IS THE UNION ACROSS THE FIVE GRAPH DOCUMENTS, because they share one
// schema and no single one of them carries every key: only the spine has a
// headline, only fund-flows states `scopes` rather than `scope`, and the counts
// block differs three ways. Which of those a given projection must carry is the
// schema's own if/then, held against real bytes by [encode] on every build.
func TestTheSchemasStateWhatTheProjectionsCarry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schema  string
		emitted []any
	}{
		{
			name:   "graph projections",
			schema: schema.Projection,
			emitted: []any{
				Graph{}, FundFlowsDocument{}, DepartmentSpendingDocument{},
				DepartmentFundingDocument{}, TransfersByFundDocument{},
			},
		},
		{
			name:    "series projections",
			schema:  schema.Series,
			emitted: []any{TrendsDocument{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stated, err := schema.NamesDeep(tc.schema)
			if err != nil {
				t.Fatalf("read %s: %v", tc.schema, err)
			}
			if len(stated) == 0 {
				t.Fatalf("%s states no property, so this test compares nothing", tc.schema)
			}
			var emitted []string
			for _, v := range tc.emitted {
				emitted = append(emitted, schema.StructNames(reflect.TypeOf(v), "")...)
			}
			slices.Sort(stated)
			slices.Sort(emitted)
			stated = slices.Compact(stated)
			emitted = slices.Compact(emitted)
			if diff := cmp.Diff(emitted, stated); diff != "" {
				t.Errorf("%s and the documents it holds name different fields "+
					"(-emitted +stated):\n%s\ninternal/export decodes these without "+
					"importing this package, so a tag renamed on one side alone reads "+
					"as a zero value with no error anywhere.", tc.schema, diff)
			}
		})
	}
}

// TestTheSchemaHoldsASignToItsSentence poses all four quadrants of the
// biconditional the projection schema states about a reduction.
//
// THE PROSE FORM OF THIS CLAIM WAS THE ALTERNATIVE, and AGENTS.md prefers the
// machine-checkable one for a measured reason: a shape stated in a comment,
// again in a struct tag and a third time in a hand-written key check is three
// spellings none of which can be held against the bytes. What is held here is
// the schema itself, against documents this test builds to be valid in every
// other respect.
//
// BOTH ACCEPTING CASES ARE POSED, and that is the half a one-sided test would
// miss. A schema that refused every link would pass an "it refuses the bad
// shape" test while making the whole field unpublishable.
//
// THE REFUSALS ARE READ FOR THEIR REASON and not only for their exit: the first
// draft of this test refused all four, for an empty metadata.counts, and a
// version checking only that an error came back would have reported the
// biconditional working when nothing had exercised it.
func TestTheSchemaHoldsASignToItsSentence(t *testing.T) {
	loc := []any{map[string]any{"doc_id": "livermore-budget-fy2026-2027", "pages": []int{127}}}
	link := func(cents int64, contra string) map[string]any {
		return map[string]any{
			"source": "a", "target": "a", "value_cents": cents, "kind": "external",
			"transfer_id": "", "fact_ids": []string{"fisc-f-0000000000aa"},
			"locators": loc, "derived": false, "partition": false, "contra": contra,
		}
	}
	const sentence = "printed as a reduction of Property Taxes"

	for _, c := range []struct {
		name    string
		link    map[string]any
		refused bool
	}{
		{"a reduction that names no schedule", link(-250, ""), true},
		{"an addition that claims to be a reduction", link(250, sentence), true},
		{"a reduction that names one", link(-250, sentence), false},
		{"an addition that claims nothing", link(250, ""), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := map[string]any{
				"schema_version": 1, "projection": "fund-flows",
				"metadata": map[string]any{
					"fiscal_year": 2026, "fiscal_year_label": "FY2025-26", "basis": "adopted",
					"generated_by": "t", "currency": "USD", "units": "cents",
					"scopes": []string{"revenue-by-fund"}, "sources": loc,
					"counts": map[string]any{
						"facts": 1, "facts_cited": 1, "nodes": 0, "links": 1,
					},
					"caveats": []any{},
				},
				"nodes": []any{},
				"links": []any{c.link},
			}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var v any
			if err = json.Unmarshal(raw, &v); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			err = schema.Validate("projection.schema.json", v)
			switch {
			case c.refused && err == nil:
				t.Errorf("the schema accepted it; value_cents and contra are stated as a "+
					"biconditional and this shape holds one half without the other: %s", raw)
			case c.refused && !strings.Contains(err.Error(), "contra"):
				t.Errorf("the schema refused it, but not for contra -- so this case proves "+
					"nothing about the biconditional: %v", err)
			case !c.refused && err != nil:
				t.Errorf("the schema refused a shape the documents publish: %v", err)
			}
		})
	}
}
