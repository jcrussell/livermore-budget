package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jcrussell/livermore-budget/schema"
)

// Every committed geometry file. tools/extract.py writes these and holds each
// to the schema's subset it can check; internal/geom reads them. This is the
// whole schema, over the whole committed tree.
func TestEveryCommittedGeometryFileMatchesTheSchema(t *testing.T) {
	resolved, err := schema.Load(schema.Geometry)
	if err != nil {
		t.Fatalf("load %s: %v", schema.Geometry, err)
	}
	pages, err := filepath.Glob(filepath.Join("..", "data", "extracted", "*", "geometry", "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no committed geometry file found, so this test asserts nothing")
	}
	for _, path := range pages {
		raw, readErr := os.ReadFile(path) // #nosec G304 -- the path comes from a glob of the committed tree.
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s does not parse as JSON: %v", path, err)
		}
		if err := resolved.Validate(v); err != nil {
			t.Errorf("%s does not match %s: %v", path, schema.Geometry, err)
		}
	}
	t.Logf("%d geometry file(s) checked against %s", len(pages), schema.Geometry)
}
