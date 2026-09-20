package schema_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/schema"
)

// The committed audit trail, record by record. It reads the file rather than a
// fixture: a schema held against a fixture says only that the fixture conforms.
func TestEveryCommittedFactMatchesTheSchema(t *testing.T) {
	resolved, err := schema.Load(schema.Fact)
	if err != nil {
		t.Fatalf("load %s: %v", schema.Fact, err)
	}

	path := filepath.Join("..", "facts", "facts.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	checked, failed := 0, 0
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Fatalf("%s:%d does not parse as JSON: %v", path, line, err)
		}
		checked++
		if err := resolved.Validate(v); err != nil {
			failed++
			// The first few in full; the rest would bury them.
			if failed <= 3 {
				t.Errorf("%s:%d does not match %s: %v", path, line, schema.Fact, err)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if failed > 3 {
		t.Errorf("%s: %d records in total do not match %s", path, failed, schema.Fact)
	}
	// Green because it looked, not because the file was empty.
	if checked == 0 {
		t.Fatalf("%s carries no records, so this test asserts nothing", path)
	}
	t.Logf("%d fact records checked against %s", checked, schema.Fact)
}

// The enums are the declared set, not today's data: basis "projected" and units
// "thousands" are declared and appear in no committed fact.
func TestTheSchemaAllowsDeclaredValuesTheCorpusHasNotReachedYet(t *testing.T) {
	resolved, err := schema.Load(schema.Fact)
	if err != nil {
		t.Fatalf("load %s: %v", schema.Fact, err)
	}
	base := map[string]any{
		"id": "fisc-f-000000000000", "doc_id": "d", "page": 1, "offset": 0,
		"token": "1", "rule_id": "r", "kind": "revenue", "basis": "adopted",
		"scope": "s", "fiscal_year": 2026, "row_path": "", "row_label": "",
		"category": "", "department": "", "column_path": "", "fund_group": "",
		"fund": nil, "sign": "positive", "units": "dollars",
		"amount_cents": 1, "derived": false,
	}
	for _, tc := range []struct{ key, value string }{
		{"basis", "projected"},
		{"units", "thousands"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			rec := map[string]any{}
			for k, v := range base {
				rec[k] = v
			}
			rec[tc.key] = tc.value
			if err := resolved.Validate(rec); err != nil {
				t.Errorf("the schema refuses %s %q, which internal/mapping declares: %v",
					tc.key, tc.value, err)
			}
		})
	}
}
