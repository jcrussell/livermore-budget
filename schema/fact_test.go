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

// THE COMMITTED AUDIT TRAIL IS HELD TO THE SCHEMA, RECORD BY RECORD.
// facts/facts.jsonl is what every other check in this tree is measured against
// and what CI compares byte for byte, so it is the artifact whose shape is worth
// stating in something a machine reads.
//
// IT READS THE FILE RATHER THAN A FIXTURE, deliberately. A schema held against a
// fixture says the fixture conforms; held against the committed store it says
// the project's own data does, and it goes red the day a mapping emits a shape
// the contract does not allow.
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
	// GREEN BECAUSE IT LOOKED, NOT BECAUSE THE FILE WAS EMPTY. A store this
	// could not read would otherwise pass with nothing compared, which is the
	// shape AGENTS.md's "Prove it can fail" names.
	if checked == 0 {
		t.Fatalf("%s carries no records, so this test asserts nothing", path)
	}
	t.Logf("%d fact records checked against %s", checked, schema.Fact)
}

// THE ENUMS ARE THE DECLARED SET, NOT TODAY'S DATA, and this is what says so.
// Writing the schema from the corpus would have omitted basis "projected" and
// units "thousands": both are declared in internal/mapping and internal/amount
// and neither appears in any committed fact. A schema that excluded them would
// refuse a correct record the first time the corpus grew one.
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
