package export

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestTheSchemaStatesWhatTheRungAnswerCarries holds schema/rungs.schema.json
// to the emitted structs: every JSON name the artifact can carry is a property
// there, nothing is a property there that the artifact cannot carry, and the
// declared schema version is the one the packager stamps.
//
// IT USED TO READ A FENCED BLOCK IN docs/general-fund-drilldown-contract.md,
// and that block was a third spelling of a shape the structs already state and
// the schema now states machine-readably. A schema is better than the block in
// the way that matters here: it is compared against the emitted BYTES by
// encodeRungs, where the block could only ever be compared against the struct
// tags. The prose kept the argument, which is what a schema cannot say.
//
// THE COMPARISON IS OF NAMES AND NOT OF TYPES, deliberately. Whether `ids` is
// an array of strings is the schema's to enforce against real bytes; whether
// the struct and the schema even agree on WHICH keys exist is the thing a
// reader of either one would otherwise have to check by eye.
func TestTheSchemaStatesWhatTheRungAnswerCarries(t *testing.T) {
	raw, err := fs.ReadFile(schema.FS(), schema.Rungs)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Rungs, err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not valid JSON: %v", schema.Rungs, err)
	}

	stated := schemaNames(doc, "")
	emitted := emittedNames(reflect.TypeOf(rungsDoc{}), "")
	slices.Sort(stated)
	slices.Sort(emitted)
	if len(stated) == 0 {
		t.Fatalf("%s states no property, so this test compares nothing", schema.Rungs)
	}
	if diff := cmp.Diff(emitted, stated); diff != "" {
		t.Errorf("%s and the emitted artifact name different fields (-emitted +stated):\n%s\n"+
			"Every name the rung answer carries is a property of the schema, and only those.",
			schema.Rungs, diff)
	}

	props, _ := doc["properties"].(map[string]any)
	version, ok := props["schema_version"].(map[string]any)
	if !ok {
		t.Fatalf("%s states no schema_version property", schema.Rungs)
	}
	got, ok := version["const"].(float64)
	if !ok {
		t.Fatalf("%s pins schema_version to no const; any version would validate", schema.Rungs)
	}
	if int(got) != rungsSchemaVersion {
		t.Errorf("%s pins schema_version %d and the packager stamps %d; a bump moves both.",
			schema.Rungs, int(got), rungsSchemaVersion)
	}
}

// schemaNames is every dotted JSON name the schema declares, with array
// nesting collapsed: "columns", "columns.stem", "columns.rungs.draws.ids" --
// the same spelling emittedNames produces off the structs.
func schemaNames(node map[string]any, prefix string) []string {
	var out []string
	if items, ok := node["items"].(map[string]any); ok {
		return schemaNames(items, prefix)
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		return nil
	}
	for k, v := range props {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out = append(out, name)
		if child, ok := v.(map[string]any); ok {
			out = append(out, schemaNames(child, name)...)
		}
	}
	return out
}

// emittedNames is the same set read off the structs encodeRungs marshals. A
// field with no json tag is named by its Go name, which is what encoding/json
// would write, so an untagged field goes red here rather than slipping past a
// tag lookup that returned "".
func emittedNames(t reflect.Type, prefix string) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" {
			name = f.Name
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		out = append(out, name)
		ft := f.Type
		for ft.Kind() == reflect.Slice || ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			out = append(out, emittedNames(ft, name)...)
		}
	}
	return out
}
