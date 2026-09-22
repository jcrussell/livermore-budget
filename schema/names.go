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
// A $ref IS A LEAF HERE, and [NamesDeep] is the other half of that choice.
// Which one a comparison wants is decided by the STRUCT it is compared against:
// internal/export holds a column's locators and caveats as [json.RawMessage],
// passing another package's bytes through without decoding them, so the schema
// naming what is inside would be knowledge the struct does not have.
func Names(name string) ([]string, error) {
	return namesIn(name, nil)
}

// NamesDeep is [Names] with every $ref followed into the schema it names.
//
// FOR THE SIDE THAT WALKS IN. internal/project's own documents hold locators as
// a []Source and caveats as a []Caveat, so [StructNames] walks into both; a
// schema that stopped at the reference would state a shorter name set than the
// encoder can write, and the pair would disagree on every document citing a
// page. The reference is resolved by FILENAME against this package's embedded
// files: every $id here is a published URL whose last segment is the file, and
// nothing in this tree refs a schema it does not ship.
func NamesDeep(name string) ([]string, error) {
	return namesIn(name, map[string]bool{})
}

func namesIn(name string, seen map[string]bool) ([]string, error) {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("reading schema %s: %w", name, err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", name, err)
	}
	return names(doc, "", seen)
}

func names(node map[string]any, prefix string, seen map[string]bool) ([]string, error) {
	if ref, ok := node["$ref"].(string); ok {
		if seen == nil {
			return nil, nil
		}
		file := ref[strings.LastIndex(ref, "/")+1:]
		// A CYCLE IS A STOP AND NOT AN ERROR. Nothing here refs itself today;
		// one that did would otherwise recurse until the stack ran out, and a
		// name set is finite whatever the reference graph looks like.
		if seen[file] {
			return nil, nil
		}
		seen[file] = true
		defer delete(seen, file)
		under, err := namesIn(file, seen)
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
		// AN EMBEDDED STRUCT WITH NO TAG IS INLINED, because that is what
		// encoding/json writes: project.Envelope's four keys appear at the level
		// of the struct embedding it and under no name of their own. Walking it
		// as a named field states `metadata.Envelope.currency` for a document
		// whose bytes say `metadata.currency`, which makes the comparison this
		// feeds disagree on every document that embeds anything.
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
