package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jcrussell/livermore-budget/schema"
)

// EVERY COMMITTED MANIFEST IS HELD TO THE SCHEMA. This is the one artifact in
// the tree written by one language and read by another: tools/extract.py writes
// it, internal/corpus reads it, and neither shares a type with the other. Until
// this existed the agreement was prose plus Go struct tags, and a Python change
// that broke it surfaced as a decode error at best and a field silently
// decoding to its zero value at worst.
func TestEveryCommittedManifestMatchesTheSchema(t *testing.T) {
	resolved, err := schema.Load(schema.Manifest)
	if err != nil {
		t.Fatalf("load %s: %v", schema.Manifest, err)
	}

	docs, err := filepath.Glob(filepath.Join("..", "data", "extracted", "*", "manifest.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("no committed manifest found, so this test asserts nothing")
	}
	for _, path := range docs {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			raw, readErr := os.ReadFile(path) // #nosec G304 -- the path comes from a glob of the committed tree.
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatalf("%s does not parse as JSON: %v", path, err)
			}
			if err := resolved.Validate(v); err != nil {
				t.Errorf("%s does not match %s: %v", path, schema.Manifest, err)
			}
		})
	}
	t.Logf("%d manifest(s) checked against %s", len(docs), schema.Manifest)
}

// THE TOP LEVEL STAYS OPEN, AND THAT IS A DECISION RATHER THAN AN OMISSION.
// internal/corpus's manifest type documents it: "Unknown fields are tolerated
// rather than rejected: extract.py may add reporting keys, and SchemaVersion is
// the guard that matters." A schema that closed the object would refuse a
// manifest the Go reader accepts, which is a contract disagreeing with itself.
func TestTheManifestSchemaToleratesAReportingKeyGoIgnores(t *testing.T) {
	resolved, err := schema.Load(schema.Manifest)
	if err != nil {
		t.Fatalf("load %s: %v", schema.Manifest, err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "data", "extracted",
		"livermore-acfr-fy2025", "manifest.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	m["elapsed_seconds"] = 12.5
	if err := resolved.Validate(m); err != nil {
		t.Errorf("the schema refuses a manifest carrying a key Go ignores: %v", err)
	}
}

// THE TWO LANGUAGES REFUSE THE SAME MANIFESTS, which is the whole claim of a
// shared schema and the one thing neither side can assert alone.
//
// The mutations below are the ones tools/extract.py's check_against_schema is
// driven with in the same commit; it reads this same file with the standard
// library and reports the same four. A schema only one side honoured would be a
// contract in name -- so if either list changes, both move.
func TestGoRefusesTheManifestsPythonRefuses(t *testing.T) {
	resolved, err := schema.Load(schema.Manifest)
	if err != nil {
		t.Fatalf("load %s: %v", schema.Manifest, err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "data", "extracted",
		"livermore-acfr-fy2025", "manifest.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"schema_version bumped without the schema", func(m map[string]any) { m["schema_version"] = 3 }},
		{"source_sha256 dropped", func(m map[string]any) { delete(m, "source_sha256") }},
		{"page_count as a string", func(m map[string]any) { m["page_count"] = "268" }},
		{"an error with an unknown stage", func(m map[string]any) {
			m["errors"] = []any{map[string]any{"stage": "ocr", "page": 1, "message": "x"}}
		}},
		// Go-side only, because Python's subset does not walk patterns: the
		// artifact path grammar is part of the contract, since a reader
		// resolves a page by COMPUTING this path.
		{"an artifact outside the path grammar", func(m map[string]any) {
			m["artifacts"] = map[string]any{"tables/p0001.csv": map[string]any{
				"bytes": 1, "sha256": "0000000000000000000000000000000000000000000000000000000000000000"}}
		}},
		{"a source hash that is not 64 hex", func(m map[string]any) { m["source_sha256"] = "abc" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("parse: %v", err)
			}
			tc.mutate(m)
			if err := resolved.Validate(m); err == nil {
				t.Errorf("accepted a manifest with %s, which the contract forbids", tc.name)
			}
		})
	}
}
