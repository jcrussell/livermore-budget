package project

import (
	"reflect"
	"slices"
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
