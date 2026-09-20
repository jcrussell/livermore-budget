package export

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// contractPath is the document that states what the rung answer carries and
// what it deliberately does not, relative to this package.
const contractPath = "../../../docs/general-fund-drilldown-contract.md"

// contractHeading opens the section this test reads. The section's first
// fenced json block is the one it parses, which is why the shape goes first
// there.
const contractHeading = "## Go's half: the rung answer"

// TestTheContractStatesWhatTheRungAnswerCarries holds the contract's shape
// block to the emitted structs: every JSON name the artifact can carry is
// named there, nothing is named there that the artifact cannot carry, and the
// declared schema version is the one the packager stamps.
//
// WHY THE DOCUMENT IS A TEST SUBJECT AT ALL. This artifact's Go half and its
// client half are written in different languages and checked by different
// gates, so the only place that states the seam whole is prose -- and prose
// that nothing reads drifts from the code beside it in silence, which is the
// defect this section was written for. A key set and a version number are the
// two claims in it a machine can hold, so those two are held and the rest is
// left to a reader.
//
// IT PARSES A FENCED BLOCK AND NOT A SENTENCE, deliberately. A checker that
// pattern-matched the prose would be a regex fighting English, red on a
// rewording that changed no claim and green on a claim that quietly went
// false. A fenced json block is a machine-readable region a writer opts into,
// and everything outside it here is out of this test's reach by design.
func TestTheContractStatesWhatTheRungAnswerCarries(t *testing.T) {
	block, err := contractShapeBlock()
	if err != nil {
		t.Fatal(err)
	}
	var shape any
	if err = json.Unmarshal(block, &shape); err != nil {
		t.Fatalf("%s: the shape block under %q is not valid JSON: %v", contractPath, contractHeading, err)
	}

	stated, err := statedNames(shape, "")
	if err != nil {
		t.Fatalf("%s: %v", contractPath, err)
	}
	emitted := emittedNames(reflect.TypeOf(rungsDoc{}), "")
	slices.Sort(stated)
	slices.Sort(emitted)
	if diff := cmp.Diff(emitted, stated); diff != "" {
		t.Errorf("%s's shape block and the emitted artifact name different fields (-emitted +stated):\n%s\n"+
			"Every name the rung answer carries is stated in the %q section, and only those.",
			contractPath, diff, contractHeading)
	}

	top, ok := shape.(map[string]any)
	if !ok {
		t.Fatalf("%s: the shape block is not a JSON object", contractPath)
	}
	version, ok := top["schema_version"].(float64)
	if !ok {
		t.Fatalf("%s: the shape block states no numeric schema_version", contractPath)
	}
	if int(version) != rungsSchemaVersion {
		t.Errorf("%s states schema_version %d and the packager stamps %d; a bump moves both.",
			contractPath, int(version), rungsSchemaVersion)
	}
}

// contractShapeBlock is the contents of the first fenced json block after
// contractHeading. A missing heading or a missing block is an error and not an
// empty answer: either would otherwise pass this test by having nothing to
// compare.
func contractShapeBlock() ([]byte, error) {
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	start := slices.Index(lines, contractHeading)
	if start < 0 {
		return nil, fmt.Errorf("%s has no %q section, which is where the rung answer's shape is stated", contractPath, contractHeading)
	}
	open := -1
	for i := start + 1; i < len(lines); i++ {
		switch {
		case open < 0 && lines[i] == "```json":
			open = i
		case open >= 0 && lines[i] == "```":
			return []byte(strings.Join(lines[open+1:i], "\n")), nil
		case open < 0 && strings.HasPrefix(lines[i], "## "):
			return nil, fmt.Errorf("%s's %q section ends before it states a ```json shape block", contractPath, contractHeading)
		}
	}
	return nil, fmt.Errorf("%s's %q section opens a ```json block and does not close it", contractPath, contractHeading)
}

// statedNames is every dotted JSON name the shape block spells, with array
// nesting collapsed: "columns", "columns.stem", "columns.rungs.draws.ids".
//
// A SAMPLE OF ONE IS REQUIRED. Reading the first element of a longer array
// would let a second entry state a name this test never compared, which is
// exactly the silent gap the section exists to close.
func statedNames(v any, prefix string) ([]string, error) {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			name := k
			if prefix != "" {
				name = prefix + "." + k
			}
			out = append(out, name)
			nested, err := statedNames(child, name)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
		}
	case []any:
		if len(t) != 1 {
			return nil, fmt.Errorf("the shape block gives %d entries at %q; one stands for the shape and more than one hides a name", len(t), prefix)
		}
		return statedNames(t[0], prefix)
	}
	return out, nil
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
