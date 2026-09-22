package export

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestEveryNameThisPackageDecodesIsOneAProjectionStates holds the decoders in
// this package to schema/projection.schema.json and schema/series.schema.json.
//
// THIS IS THE SEAM THE SCHEMAS WERE WRITTEN FOR. This package does not import
// internal/project -- deliberately, so a packager cannot recompute what a
// projection published -- and the price is that [decoded] and the decoders in
// page.go are a second spelling of those documents' shapes, joined to them by
// json tags alone. A tag renamed on one side reads as a zero value here with no
// error anywhere: the page renders, the figure is absent, and nothing is red.
//
// CONTAINMENT AND NOT EQUALITY, which is the difference between this side and
// internal/project's. A decoder legitimately reads a SUBSET: [decoded] wants
// enough of a document to fold it into a column and has no reason to know what
// a caveat's fields are. What it may not do is read a name no document carries,
// because that name decodes to a zero value forever.
//
// THE DEEP NAME SET IS THE ONE COMPARED, because these decoders are not opaque
// where the column's are: sourceMeta and caveatMeta are real structs that
// [schema.StructNames] walks into, so the locator and caveat shapes the schema
// reaches through a $ref are shapes this package genuinely reads. Where it IS
// opaque -- [decoded] holds a link's locators as json.RawMessage -- the emitted
// side simply stops, and containment has nothing to say about what it did not
// look at.
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			stated, err := schema.NamesDeep(tc.schema)
			if err != nil {
				t.Fatalf("read %s: %v", tc.schema, err)
			}
			if len(stated) == 0 {
				t.Fatalf("%s states no property, so this test compares nothing", tc.schema)
			}
			// THE STATED SET IS EVERY SUFFIX OF EVERY PATH, because a decoder
			// reaches a nested shape through a type of its own: caveatMeta is
			// compared as `id`, `summary`, ... where the schema states them as
			// `metadata.caveats.id`. Comparing the leaf name alone would accept
			// a name from anywhere in the document; comparing whole paths would
			// refuse every decoder that is not the root one.
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
