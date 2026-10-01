package schema_test

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// strs is a Go set as sorted strings.
func strs[T ~string](in []T) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	slices.Sort(out)
	return out
}

// enumAt reads the enum at a path of object keys in one embedded schema.
func enumAt(t *testing.T, file string, path ...string) []string {
	t.Helper()
	raw, err := fs.ReadFile(schema.FS(), file)
	if err != nil {
		t.Fatal(err)
	}
	var at any
	if err := json.Unmarshal(raw, &at); err != nil {
		t.Fatal(err)
	}
	for _, k := range path {
		m, ok := at.(map[string]any)
		if !ok {
			t.Fatalf("%s has nothing at %v", file, path)
		}
		at = m[k]
	}
	m, _ := at.(map[string]any)
	list, _ := m["enum"].([]any)
	if len(list) == 0 {
		t.Fatalf("%s states no enum at %v, so nothing is compared", file, path)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v.(string))
	}
	slices.Sort(out)
	return out
}

// TestEverySharedEnumIsItsGoSet holds each closed set schema/enums.schema.json
// states to the Go set it mirrors, both ways: a value Go writes and the schema
// omits is a document the encoder refuses, and one the schema admits and Go
// never writes is a promise about nothing.
func TestEverySharedEnumIsItsGoSet(t *testing.T) {
	for def, want := range map[string][]string{
		"basis":      strs(mapping.Bases()),
		"kind":       strs(mapping.Kinds()),
		"sign":       strs(mapping.Signs()),
		"units":      strs(amount.AllUnits()),
		"link_kind":  strs(project.LinkKinds()),
		"role":       strs(project.Roles()),
		"chart_form": strs(export.ChartForms()),
		"mark_role":  strs(export.MarkRoles()),
	} {
		if diff := cmp.Diff(want, enumAt(t, "enums.schema.json", "$defs", def)); diff != "" {
			t.Errorf("enums.schema.json's %s and its Go set differ (-go +schema):\n%s", def, diff)
		}
	}
}

// TestEveryProjectionNameEnumIsTheRegistry holds the two documents' projection
// enums to the registry: a graph's Document returns *project.Document, a
// series' *project.TrendsDocument.
func TestEveryProjectionNameEnumIsTheRegistry(t *testing.T) {
	var graphs, series []string
	for _, p := range project.Registry(nil) {
		switch p.(type) {
		case interface {
			Document([]fact.Fact, project.Options) (*project.Document, error)
		}:
			graphs = append(graphs, p.Name())
		case interface {
			Document([]fact.Fact, project.Options) (*project.TrendsDocument, error)
		}:
			series = append(series, p.Name())
		default:
			t.Errorf("projection %q builds neither a graph nor a series", p.Name())
		}
	}
	slices.Sort(graphs)
	slices.Sort(series)
	if diff := cmp.Diff(graphs, enumAt(t, schema.Projection, "properties", "projection")); diff != "" {
		t.Errorf("%s's projection enum and the registry's graphs differ (-registry +schema):\n%s", schema.Projection, diff)
	}
	if diff := cmp.Diff(series, enumAt(t, schema.Series, "properties", "projection")); diff != "" {
		t.Errorf("%s's projection enum and the registry's series differ (-registry +schema):\n%s", schema.Series, diff)
	}
}
