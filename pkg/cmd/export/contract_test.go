package export

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/schema"
)

// TestTheSchemaStatesWhatTheRungAnswerCarries holds schema/rungs.schema.json
// to the emitted structs: every JSON name the artifact can carry is a property
// there, nothing is a property there that the artifact cannot carry, and the
// declared schema version is the one the packager stamps.
//
// THE SCHEMA AND NOT A FENCED BLOCK IN docs/, because a schema is compared
// against the emitted BYTES by encodeRungs and a block can only ever be
// compared against the struct tags. The prose keeps the argument, which is what
// a schema cannot say.
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

	stated, err := schema.Names(schema.Rungs)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Rungs, err)
	}
	emitted := schema.StructNames(reflect.TypeOf(rungsDoc{}), "")
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
