package schema

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
)

// Names is every dotted JSON name a schema declares, with array nesting
// collapsed: "years", "years.stem", "years.steps.caveats.href". A contract
// test compares it with [StructNames]; names, not types. A map is stepped
// through by its additionalProperties, and a $ref is a leaf (see [NamesDeep]),
// matching structs that pass another package's bytes as [json.RawMessage].
func Names(name string) ([]string, error) {
	return namesIn(name, nil)
}

// NamesDeep is [Names] with every $ref followed, by filename, into the
// embedded schema it names: for structs that decode locators and caveats.
func NamesDeep(name string) ([]string, error) {
	return namesIn(name, map[string]bool{})
}

func namesIn(name string, seen map[string]bool) ([]string, error) {
	return namesAt(name, "", seen)
}

// namesAt is [namesIn] at a JSON pointer into the file, as a $ref's fragment
// names one of its $defs.
func namesAt(name, pointer string, seen map[string]bool) ([]string, error) {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("reading schema %s: %w", name, err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", name, err)
	}
	for _, key := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if key == "" {
			continue
		}
		next, ok := doc[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema %s has nothing at %s", name, pointer)
		}
		doc = next
	}
	return names(doc, "", seen)
}

func names(node map[string]any, prefix string, seen map[string]bool) ([]string, error) {
	if ref, ok := node["$ref"].(string); ok {
		if seen == nil {
			return nil, nil
		}
		target, fragment, _ := strings.Cut(ref, "#")
		file := target[strings.LastIndex(target, "/")+1:]
		// A cycle is a stop, not an error.
		if seen[ref] {
			return nil, nil
		}
		seen[ref] = true
		defer delete(seen, ref)
		under, err := namesAt(file, fragment, seen)
		if err != nil {
			return nil, err
		}
		if prefix == "" {
			return under, nil
		}
		out := make([]string, 0, len(under))
		for _, n := range under {
			out = append(out, prefix+"."+n)
		}
		return out, nil
	}
	if items, ok := node["items"].(map[string]any); ok {
		return names(items, prefix, seen)
	}
	if extra, ok := node["additionalProperties"].(map[string]any); ok {
		return names(extra, prefix, seen)
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		return nil, nil
	}
	var out []string
	for k, v := range props {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out = append(out, name)
		child, ok := v.(map[string]any)
		if !ok {
			continue
		}
		under, err := names(child, name, seen)
		if err != nil {
			return nil, err
		}
		out = append(out, under...)
	}
	return out, nil
}

// StructNames is [Names]' other half: the same set read off the structs an
// encoder marshals. An untagged field is named by its Go name, as encoding/json
// writes it; a map's value is walked; [json.RawMessage] is a leaf.
func StructNames(t reflect.Type, prefix string) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		// An untagged embedded struct is inlined, as encoding/json writes it.
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			out = append(out, StructNames(f.Type, prefix)...)
			continue
		}
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
		// A type that marshals itself is a leaf.
		marshals := func(t reflect.Type) bool { return t.Implements(reflect.TypeFor[json.Marshaler]()) }
		ft := f.Type
		for !marshals(ft) && (ft.Kind() == reflect.Slice || ft.Kind() == reflect.Pointer ||
			ft.Kind() == reflect.Map) {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && !marshals(ft) {
			out = append(out, StructNames(ft, name)...)
		}
	}
	return out
}
