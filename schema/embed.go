// Package schema holds the machine-checkable contracts for the artifacts this
// project publishes or reads across a language boundary.
//
// Go validates fully, through this package. tools/extract.py reads the same
// files and compares its own refusals against the `required` arrays, because
// it may not take a dependency; the client reads none and draws what Go
// validated.
//
// The embed is here because //go:embed patterns cannot escape the directory of
// the file that declares them, which is site/embed.go's reason too.
//
// Why, measured: docs/schema-contracts.md.
package schema

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed *.schema.json
var files embed.FS

// FS returns the embedded schemas, as an fs.FS so a test can substitute one.
func FS() fs.FS { return files }

// Load compiles the named schema, resolved and ready to validate.
func Load(name string) (*jsonschema.Resolved, error) {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("reading schema %s: %w", name, err)
	}
	var doc jsonschema.Schema
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", name, err)
	}
	resolved, err := doc.Resolve(&jsonschema.ResolveOptions{Loader: load})
	if err != nil {
		return nil, fmt.Errorf("resolving schema %s: %w", name, err)
	}
	return resolved, nil
}

// load answers a $ref from the embedded copy.
//
// Each schema's $id is the URL it will be published at, so a ref reads as a
// public identifier and still resolves with no network.
func load(uri *url.URL) (*jsonschema.Schema, error) {
	name := path.Base(uri.Path)
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("no schema in this package answers %s: %w", uri, err)
	}
	var doc jsonschema.Schema
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing referenced schema %s: %w", name, err)
	}
	return &doc, nil
}

// Validate holds one decoded JSON value to the named schema.
//
// It compiles on every call. A caller in a loop should Load once instead.
func Validate(name string, v any) error {
	resolved, err := Load(name)
	if err != nil {
		return err
	}
	return resolved.Validate(v)
}

// ValidateJSONL holds every line of a JSONL stream to the named schema,
// reporting the first line that fails and refusing a stream with no records.
func ValidateJSONL(r io.Reader, name string) error {
	resolved, err := Load(name)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	seen := 0
	for line := 1; sc.Scan(); line++ {
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(text, &v); err != nil {
			return fmt.Errorf("line %d is not JSON: %w", line, err)
		}
		seen++
		if err := resolved.Validate(v); err != nil {
			return fmt.Errorf("line %d does not match %s: %w", line, name, err)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if seen == 0 {
		return fmt.Errorf("nothing to check against %s: the stream carries no records", name)
	}
	return nil
}

// The schemas a caller names. Three more are embedded and have no constant --
// fact-id, locator and caveat -- because nothing loads them by name: they are
// reached only as $ref targets, resolved from the referring schema's URL.
const (
	// Fact is one line of facts/facts.jsonl and of every published page shard.
	Fact = "fact.schema.json"
	// Manifest is data/extracted/<doc_id>/manifest.json, which
	// tools/extract.py writes and internal/corpus reads.
	Manifest = "manifest.schema.json"
	// Column is one published column: a node table, the tier order, and one
	// entry per printed schedule.
	Column = "column.schema.json"
	// Rungs is the answer a page opens nodes against: which nodes each column
	// of each reachable chart holds, and which marks the client adds.
	Rungs = "rungs.schema.json"
	// Page is window.FISC_CONFIG: what the page has before it has fetched
	// anything, and which artifacts it may fetch.
	Page = "page.schema.json"
	// Projection is one document internal/project builds as a graph: the
	// citywide spine and the four schedules drawn beside it.
	Projection = "projection.schema.json"
	// Series is one built as a series per printed row instead: the revenue
	// trends and the two ACFR fund-balance schedules.
	Series = "series.schema.json"
)
