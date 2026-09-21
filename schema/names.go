package schema

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
)

// Names is every dotted JSON name a schema declares, with array nesting
// collapsed: "columns", "columns.stem", "columns.rungs.draws.ids".
//
// IT IS HALF OF A PAIR AND USELESS ALONE. [StructNames] produces the same
// spelling off the structs an encoder marshals, and a contract test compares
// the two: every name the artifact can carry is a property here, and nothing is
// a property here the artifact cannot carry. The comparison is of NAMES AND NOT
// TYPES, deliberately -- whether `ids` holds strings is this schema's to enforce
// against real bytes, while whether the struct and the schema even agree on
// WHICH keys exist is what a reader of either would otherwise check by eye.
//
// A MAP IS STEPPED THROUGH BY ITS additionalProperties, because a path names
// keys and not instances: the page config's `docs` is one shape under arbitrary
// doc ids, and naming the ids would make this test a copy of the corpus.
func Names(name string) ([]string, error) {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("reading schema %s: %w", name, err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", name, err)
	}
	return names(doc, ""), nil
}

func names(node map[string]any, prefix string) []string {
	if items, ok := node["items"].(map[string]any); ok {
		return names(items, prefix)
	}
	if extra, ok := node["additionalProperties"].(map[string]any); ok {
		return names(extra, prefix)
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	for k, v := range props {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out = append(out, name)
		if child, ok := v.(map[string]any); ok {
			out = append(out, names(child, name)...)
		}
	}
	return out
}

// StructNames is [Names]' other half: the same set read off the structs an
// encoder marshals.
//
// A field with no json tag is named by its Go name, which is what encoding/json
// would write, so an untagged field goes red in the comparison rather than
// slipping past a tag lookup that returned "".
//
// TWO KINDS STOP THE WALK RATHER THAN DESCENDING. A map's VALUE is walked,
// matching the schema's additionalProperties; [json.RawMessage] is not, because
// its contents are another package's document passed through verbatim and this
// side knows nothing about their shape -- which is why the schema holds it as a
// bare object too.
func StructNames(t reflect.Type, prefix string) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" {
			name = f.Name
		}
		if name == "-" {
			continue
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		out = append(out, name)
		if f.Type == reflect.TypeOf(json.RawMessage(nil)) {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Slice || ft.Kind() == reflect.Pointer ||
			ft.Kind() == reflect.Map {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			out = append(out, StructNames(ft, name)...)
		}
	}
	return out
}
