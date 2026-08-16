package mapping

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// TestLoadSpine parses the real citywide spine rule. If the schema cannot
// express pp. 66-67 it cannot express anything in this corpus, since that
// schedule is both the control total for everything else and the most awkward
// shape in it.
func TestLoadSpine(t *testing.T) {
	f, err := Load("testdata/spine.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if diff := cmp.Diff("livermore-budget-fy2026-2027", f.DocID); diff != "" {
		t.Errorf("doc_id mismatch (-want +got):\n%s", diff)
	}
	if len(f.Rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(f.Rules))
	}

	rev := f.Rules[0]
	if rev.ID != "spine-revenues" || rev.Units != amount.Dollars {
		t.Errorf("got rule %q units %q, want spine-revenues in dollars", rev.ID, rev.Units)
	}
	if len(rev.Rows) != 10 {
		t.Errorf("got %d rows, want the 10 revenue categories", len(rev.Rows))
	}

	p66, p67 := rev.Parts[0], rev.Parts[1]

	// p66 carries labels and four columns.
	if p66.LabelsFrom != 0 {
		t.Errorf("p66 should carry its own labels, got labels_from=%d", p66.LabelsFrom)
	}
	if got, want := rev.ExpectedValues(&p66), 10*4; got != want {
		t.Errorf("p66 expected values = %d, want %d", got, want)
	}

	// p67 borrows them, omits one row, and has eight columns. 9*8=72 is the
	// count the M0 spike measured; 10*8=80 is what a naive read expects, and
	// the gap is the whole reason omitted_rows exists.
	if got := rev.LabelledPart(&p67); got == nil || got.Page != 66 {
		t.Fatalf("p67 should borrow labels from p66")
	}
	if got, want := rev.ExpectedValues(&p67), 9*8; got != want {
		t.Errorf("p67 expected values = %d, want %d", got, want)
	}
	active := rev.ActiveRows(&p67)
	if len(active) != 9 {
		t.Fatalf("got %d active rows on p67, want 9", len(active))
	}
	for _, r := range active {
		if r.Label == "Licenses & Permits" {
			t.Error("omitted row still present in p67's active rows")
		}
	}
	// Order must be preserved: positional identity depends on it.
	if active[0].Label != "Property Taxes" || active[8].Label != "Fines & Forfeitures" {
		t.Errorf("active row order changed: first=%q last=%q", active[0].Label, active[8].Label)
	}
}

func TestLoadDirRejectsDuplicateRuleIDs(t *testing.T) {
	const rule = `
schema_version: 1
doc_id: doc-a
rules:
  - id: dupe
    substrate: text
    kind: revenue
    basis: adopted
    units: dollars
    parts: [{page: 1, columns: [{fund_group: general, fiscal_year: 2026}]}]
    rows: [{label: "A", category: a}]
`
	fsys := fstest.MapFS{
		"m/a.yaml": {Data: []byte(rule)},
		"m/b.yaml": {Data: []byte(strings.Replace(rule, "doc-a", "doc-b", 1))},
	}
	_, err := LoadDir(fsys, "m")
	if err == nil {
		t.Fatal("got nil error, want a duplicate rule id rejection")
	}
	if !strings.Contains(err.Error(), "duplicate rule id") {
		t.Errorf("got %q, want it to report a duplicate rule id", err)
	}
}

func TestLoadDirIsDeterministic(t *testing.T) {
	mk := func(doc, id string) string {
		return "schema_version: 1\ndoc_id: " + doc + "\nrules:\n  - id: " + id +
			"\n    substrate: text\n    kind: revenue\n    basis: adopted\n" +
			"    units: dollars\n    parts: [{page: 1, columns: [{fiscal_year: 2026}]}]\n" +
			"    rows: [{label: \"A\", category: a}]\n"
	}
	fsys := fstest.MapFS{
		"m/z.yaml": {Data: []byte(mk("z", "z1"))},
		"m/a.yaml": {Data: []byte(mk("a", "a1"))},
		"m/m.yaml": {Data: []byte(mk("m", "m1"))},
	}
	var first []string
	for i := 0; i < 5; i++ {
		files, err := LoadDir(fsys, "m")
		if err != nil {
			t.Fatalf("LoadDir: %v", err)
		}
		var got []string
		for _, f := range files {
			got = append(got, f.DocID)
		}
		if first == nil {
			first = got
			continue
		}
		if diff := cmp.Diff(first, got); diff != "" {
			t.Fatalf("LoadDir order varies between calls (-first +got):\n%s", diff)
		}
	}
	if diff := cmp.Diff([]string{"a", "m", "z"}, first); diff != "" {
		t.Errorf("not sorted by filename (-want +got):\n%s", diff)
	}
}

// Each of these is a mistake a human will actually make in a hand-written
// rule file, and each must be reported rather than silently accepted.
func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string // substring of the error
	}{
		{
			name: "wrong schema version",
			yaml: "schema_version: 2\ndoc_id: d\nrules: []\n",
			want: "schema_version",
		},
		{
			name: "missing doc_id",
			yaml: "schema_version: 1\nrules: []\n",
			want: "doc_id",
		},
		{
			name: "unknown field is not silently ignored",
			yaml: base("") + "    omited_rows: [\"A\"]\n",
			want: "omited_rows",
		},
		{
			name: "missing units",
			yaml: "schema_version: 1\ndoc_id: d\nrules:\n  - id: r\n    substrate: text\n" +
				"    kind: revenue\n    basis: adopted\n" +
				"    parts: [{page: 1, columns: [{fiscal_year: 2026}]}]\n" +
				"    rows: [{label: \"A\"}]\n",
			want: "units",
		},
		{
			name: "bad kind",
			yaml: strings.Replace(base(""), "kind: revenue", "kind: income", 1),
			want: "kind",
		},
		{
			name: "duplicate row label",
			yaml: base("") + "      - {label: \"A\", category: a2}\n",
			want: "duplicate row label",
		},
		{
			name: "omitted row is not a row of the rule",
			yaml: strings.Replace(base(""),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts:\n      - page: 1\n        omitted_rows: [\"Nope\"]\n"+
					"        columns: [{fiscal_year: 2026}]", 1),
			want: "not one of this rule's rows",
		},
		{
			name: "labels_from points at a missing page",
			yaml: strings.Replace(base(""),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts:\n      - page: 1\n        columns: [{fiscal_year: 2026}]\n"+
					"      - page: 2\n        labels_from: 99\n"+
					"        columns: [{fiscal_year: 2026}]", 1),
			want: "not a part of this rule",
		},
		{
			name: "labels_from points at another continuation",
			yaml: strings.Replace(base(""),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts:\n      - page: 1\n        columns: [{fiscal_year: 2026}]\n"+
					"      - page: 2\n        labels_from: 1\n"+
					"        columns: [{fiscal_year: 2026}]\n"+
					"      - page: 3\n        labels_from: 2\n"+
					"        columns: [{fiscal_year: 2026}]", 1),
			want: "itself a continuation",
		},
		{
			name: "total row also listed as a row",
			yaml: base("") + "    total_row: \"A\"\n",
			want: "also listed in rows",
		},
		{
			name: "column without a fiscal year",
			yaml: strings.Replace(base(""), "{fiscal_year: 2026}", "{fund_group: general}", 1),
			want: "fiscal_year",
		},
		{
			name: "manual rule without a checksum row",
			yaml: strings.Replace(base(""), "substrate: text", "substrate: manual", 1),
			want: "total_row",
		},
		{
			name: "section_ordinal without a section",
			yaml: strings.Replace(base(""),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts: [{page: 1, section_ordinal: 2, columns: [{fiscal_year: 2026}]}]", 1),
			want: "section is empty",
		},
		{
			name: "section_ordinal counts from zero",
			yaml: strings.Replace(base(""),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts: [{page: 1, section: \"S\", section_ordinal: -1, "+
					"columns: [{fiscal_year: 2026}]}]", 1),
			want: "ordinals count from 1",
		},
		{
			// A text rule that carried a table locator would read the page text
			// while its author believed it read the grid they pointed at.
			name: "table locator on a text rule",
			yaml: strings.Replace(base(""),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts: [{page: 1, table: {ordinal: 1, label_fingerprint: \"sha256:x\"}, "+
					"columns: [{fiscal_year: 2026}]}]", 1),
			want: "is set on a text rule",
		},
		{
			name: "table rule without a locator",
			yaml: strings.Replace(base(""), "substrate: text", "substrate: table", 1),
			want: "required on a table rule",
		},
		{
			// Ordinal alone shifts the moment the extractor finds one more
			// table on the page, so it is not an identity by itself.
			name: "table locator without a fingerprint",
			yaml: strings.Replace(
				strings.Replace(base(""), "substrate: text", "substrate: table", 1),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts: [{page: 1, table: {ordinal: 1}, columns: [{fiscal_year: 2026}]}]", 1),
			want: "label_fingerprint",
		},
		{
			name: "table centroid that is not a point",
			yaml: strings.Replace(
				strings.Replace(base(""), "substrate: text", "substrate: table", 1),
				"parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts: [{page: 1, table: {ordinal: 1, label_fingerprint: \"sha256:x\", "+
					"bbox_centroid: [1.0]}, columns: [{fiscal_year: 2026}]}]", 1),
			want: "want [x, y]",
		},
		{
			name: "empty file",
			yaml: "",
			want: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.yaml), "test.yaml")
			if err == nil {
				t.Fatalf("got nil error, want one mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// The errors a human reads must say where the problem is, not just that there
// is one.
func TestParseErrorNamesTheRule(t *testing.T) {
	_, err := Parse(strings.NewReader(
		strings.Replace(base(""), "kind: revenue", "kind: income", 1)), "mappings/budget.yaml")
	if err == nil {
		t.Fatal("got nil error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("got %T, want a *ParseError", err)
	}
	for _, want := range []string{"mappings/budget.yaml", "r", "kind"} {
		if !strings.Contains(pe.Error(), want) {
			t.Errorf("error %q is missing %q", pe, want)
		}
	}
}

// A schema_version from the future gets a hint, because the fix is not in the
// file the user is looking at.
func TestFutureSchemaVersionHasAHint(t *testing.T) {
	_, err := Parse(strings.NewReader("schema_version: 99\ndoc_id: d\nrules: []\n"), "x.yaml")
	var h *cmdutil.ErrHint
	if !errors.As(err, &h) {
		t.Fatalf("got %T, want an *ErrHint telling the user to upgrade", err)
	}
}

// base returns a minimal valid rule file, with extra text appended inside the
// rule so tests can introduce one specific defect.
func base(extra string) string {
	return "schema_version: 1\ndoc_id: d\nrules:\n  - id: r\n    substrate: text\n" +
		"    kind: revenue\n    basis: adopted\n    units: dollars\n" +
		"    parts: [{page: 1, columns: [{fiscal_year: 2026}]}]\n" +
		"    rows:\n      - {label: \"A\", category: a}\n" + extra
}

// The defects a code review found in the first cut of this parser. Each was
// verified to slip through before the fix.
func TestParseRejectsSilentLosses(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			// A second `---` document parsed to nothing, with err == nil --
			// the same quiet loss KnownFields exists to prevent, one level up.
			name: "second yaml document",
			yaml: base("") + "---\nschema_version: 1\ndoc_id: d2\nrules: []\n",
			want: "more than one YAML document",
		},
		{
			// Column count still matches the page, so the value-count
			// assertion passes and FY2027 figures publish as FY2026 facts.
			name: "duplicate column",
			yaml: strings.Replace(base(""), "columns: [{fiscal_year: 2026}]",
				"columns: [{fund_group: general, fiscal_year: 2026}, "+
					"{fund_group: general, fiscal_year: 2026}]", 1),
			want: "duplicates an earlier column",
		},
		{
			name: "row with no classification",
			yaml: strings.Replace(base(""), `{label: "A", category: a}`, `{label: "A"}`, 1),
			want: "neither category nor department",
		},
		{
			// Anchoring a block on the total it is checked against is
			// circular, and the anchor dies on any revision.
			name: "block anchored on a currency amount",
			yaml: strings.Replace(base(""), "parts: [{page: 1, columns: [{fiscal_year: 2026}]}]",
				"parts:\n      - page: 1\n        stop_at: \"$27,145,882\"\n"+
					"        columns: [{fiscal_year: 2026}]", 1),
			want: "is a currency amount",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.yaml), "x.yaml")
			if err == nil {
				t.Fatalf("got nil error, want one mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// A missing schema_version must not tell the author to upgrade the binary,
// which cannot help.
func TestMissingSchemaVersionDoesNotSayUpgrade(t *testing.T) {
	_, err := Parse(strings.NewReader(strings.Replace(base(""), "schema_version: 1\n", "", 1)), "x.yaml")
	if err == nil {
		t.Fatal("got nil error")
	}
	if strings.Contains(err.Error(), "upgrade") {
		t.Errorf("got %q, want it not to suggest upgrading for a missing key", err)
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("got %q, want it to say schema_version is required", err)
	}
}

// A mapping file that is never read produces no facts and no explanation.
func TestLoadDirRejectsUnreadableFiles(t *testing.T) {
	fsys := fstest.MapFS{"m/spine.yaml.bak": {Data: []byte(base(""))}}
	if _, err := LoadDir(fsys, "m"); err == nil {
		t.Fatal("got nil error, want a rejection of the unreadable file")
	}
	// .yml is a legitimate spelling and must be read, not skipped.
	ok := fstest.MapFS{"m/spine.yml": {Data: []byte(base(""))}}
	files, err := LoadDir(ok, "m")
	if err != nil || len(files) != 1 {
		t.Errorf("got %d files, err=%v; want .yml to be read", len(files), err)
	}
}

// ActiveRows must never alias the rule, or a caller that writes through the
// result mutates the rule on pages without omissions and not on pages with.
func TestActiveRowsDoesNotAliasTheRule(t *testing.T) {
	f, err := Load("testdata/spine.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rule := f.Rules[0]
	got := rule.ActiveRows(&rule.Parts[0]) // the no-omissions path
	got[0].Label = "MUTATED"
	if rule.Rows[0].Label == "MUTATED" {
		t.Error("ActiveRows returned a slice aliasing rule.Rows")
	}
}
