package export_test

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/schema"
)

// TestMarkPrefixesAreTheMarkSchemasIDPattern holds export.MarkPrefixes, which
// ColumnsOf and validateSteps refuse producer ids under, to the alternatives
// of schema/mark.schema.json's id pattern, in order: a prefix the client
// moved and the schema followed is otherwise a guard on ids nothing makes.
// site/pins.test.mjs holds the client's constants to the same pattern.
func TestMarkPrefixesAreTheMarkSchemasIDPattern(t *testing.T) {
	raw, err := fs.ReadFile(schema.FS(), schema.Mark)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			ID struct {
				Pattern string `json:"pattern"`
			} `json:"id"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`^\^\(([^()|]+(?:\|[^()|]+)*)\)$`).FindStringSubmatch(doc.Properties.ID.Pattern)
	if m == nil {
		t.Fatalf("%s's id pattern %q is not ^(a|b|c), so the prefixes cannot be read off it", schema.Mark, doc.Properties.ID.Pattern)
	}
	if diff := cmp.Diff(export.MarkPrefixes(), strings.Split(m[1], "|")); diff != "" {
		t.Errorf("export.MarkPrefixes and %s's id pattern differ (-go +schema):\n%s", schema.Mark, diff)
	}
}
