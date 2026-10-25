package export

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestEveryNameThisPackageDecodesIsOneAProjectionStates: every name this
// package's decoders read is in schema/projection.schema.json or
// schema/series.schema.json (containment, not equality).
func TestEveryNameThisPackageDecodesIsOneAProjectionStates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		schema   string
		decoders map[string]any
	}{
		{
			name:   "graph projections",
			schema: schema.Projection,
			decoders: map[string]any{
				"decoded":            decoded{},
				"projectionDoc":      projectionDoc{},
				"projectionMetadata": projectionMetadata{},
				"caveatMeta":         caveatMeta{},
				"sourceMeta":         sourceMeta{},
			},
		},
		{
			name:   "the debt service projection",
			schema: schema.DebtService,
			decoders: map[string]any{
				"debtDoc":    debtDoc{},
				"debtColumn": debtColumn{},
				"debtPair":   debtPair{},
				"debtFigure": debtFigure{},
			},
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
			// Every suffix of every path: a nested decoder names `id` where the schema
			// states `metadata.caveats.id`. Leaf names alone would accept a name from
			// anywhere; whole paths would refuse every non-root decoder.
			tails := map[string]bool{}
			for _, n := range stated {
				parts := strings.Split(n, ".")
				for i := range parts {
					tails[strings.Join(parts[i:], ".")] = true
				}
			}
			names := make([]string, 0, len(tc.decoders))
			for name := range tc.decoders {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				read := schema.StructNames(reflect.TypeOf(tc.decoders[name]), "")
				for _, n := range read {
					if !tails[n] {
						t.Errorf("%s decodes %q, which %s states nowhere: it will read as a "+
							"zero value on every document, for ever, with no error anywhere",
							name, n, tc.schema)
					}
				}
			}
		})
	}
}
