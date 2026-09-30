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
	if got, want := rev.expectedValues(&p66), 10*4; got != want {
		t.Errorf("p66 expected values = %d, want %d", got, want)
	}

	// p67 borrows them and has eight columns, and it declares NO omissions:
	// the page prints all ten rows. The spike measured 9*8=72 here and the
	// rule declared "Licenses & Permits" omitted to match, but the page was
	// never short a row — our extractor was deleting one (fisc-c00). 10*8=80
	// is both what a naive read expects and what the document actually prints.
	if got := rev.labelledPart(&p67); got == nil || got.Page != 66 {
		t.Fatalf("p67 should borrow labels from p66")
	}
	if len(p67.OmittedRows) != 0 {
		t.Errorf("p67 declares omissions %q; the page prints all ten rows", p67.OmittedRows)
	}
	if got, want := rev.expectedValues(&p67), 10*8; got != want {
		t.Errorf("p67 expected values = %d, want %d", got, want)
	}
	active := rev.ActiveRows(&p67)
	if len(active) != 10 {
		t.Fatalf("got %d active rows on p67, want 10", len(active))
	}
	// Order must be preserved: positional identity depends on it.
	if active[0].Label != "Property Taxes" || active[9].Label != "Licenses & Permits" {
		t.Errorf("active row order changed: first=%q last=%q", active[0].Label, active[9].Label)
	}
}

func TestLoadDirRejectsDuplicateRuleIDs(t *testing.T) {
	const rule = `
schema_version: 1
doc_id: doc-a
rules:
  - id: dupe
    kind: revenue
    basis: adopted
    grain: fund-group-by-category
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
			"\n    kind: revenue\n    basis: adopted\n" +
			"    grain: category\n    units: dollars\n    parts: [{page: 1, columns: [{fiscal_year: 2026}]}]\n" +
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
			yaml: "schema_version: 1\ndoc_id: d\nrules:\n  - id: r\n" +
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
			name: "bad basis names every basis",
			yaml: strings.Replace(base(""), "basis: adopted", "basis: budgeted", 1),
			want: "want one of adopted, revised, actual, audited, projected",
		},
		{
			// SignNetted says a row is printed against its KIND's direction, so
			// it means nothing on a kind that has no direction -- and
			// fact-transfer-orientation-is-declared witnesses only transfers,
			// so an unrefused one would publish facts nothing checks.
			name: "sign netted on a kind with no direction",
			yaml: strings.Replace(base(""), `- {label: "A"`,
				`- {label: "A", sign: netted`, 1),
			want: "sign netted on kind",
		},
		{
			// fact.FromValues builds the far leg from a COPY of the row and
			// overrides only Category and Kind, so Sign is inherited. A netted
			// row whose counterpart is not a transfer therefore publishes a
			// netted revenue fact, which the orientation check filters out and
			// nothing else reads.
			name: "sign netted with a counterpart that is not a transfer",
			yaml: strings.Replace(
				strings.Replace(base(""), "kind: revenue", "kind: transfer_out", 1),
				`- {label: "A"`,
				`- {label: "A", sign: netted, counterpart: {category: c, kind: revenue, fund: 1, fund_group: general}`, 1),
			want: "sign netted on kind \"revenue\"",
		},
		{
			// The kind arm must be reached first, or the author is told about
			// the sign when the actual mistake is the kind.
			name: "an invalid kind is reported as a kind even when the sign is netted",
			yaml: strings.Replace(base(""), `- {label: "A"`,
				`- {label: "A", sign: netted, kind: income`, 1),
			want: "is not one of the five",
		},
		{
			// The netted guard reads the counterpart's kind, so it must sit
			// below checkCounterpart or a mistyped one is reported as a sign
			// error. Same class as the row.Kind ordering above.
			name: "a mistyped counterpart kind is reported as a kind, not as a sign",
			yaml: strings.Replace(
				strings.Replace(base(""), "kind: revenue", "kind: transfer_out", 1),
				`- {label: "A"`,
				`- {label: "A", sign: netted, counterpart: {category: c, kind: incom, fund: 1, fund_group: general}`, 1),
			want: "counterpart kind \"incom\" is not one of the five",
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
			name: "empty file",
			yaml: "",
			want: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(tt.yaml), "test.yaml")
			if err == nil {
				t.Fatalf("got nil error, want one mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestSignNettedIsAcceptedOnATransferRow is the other half of the rejection
// above. Without it the rule could be "netted is never accepted", which would
// also pass TestParseRejects and would refuse the one row in the corpus that
// needs it -- ACFR p41's Transfers (out).
func TestSignNettedIsAcceptedOnATransferRow(t *testing.T) {
	y := strings.Replace(base(""), "kind: revenue", "kind: transfer_out", 1)
	y = strings.Replace(y, `- {label: "A"`, `- {label: "A", sign: netted`, 1)
	f, err := parse(strings.NewReader(y), "test.yaml")
	if err != nil {
		t.Fatalf("netted on a transfer_out row was refused: %v", err)
	}
	if got := f.Rules[0].Rows[0].Sign; got != SignNetted {
		t.Errorf("row sign = %q, want %q", got, SignNetted)
	}
}

// The errors a human reads must say where the problem is, not just that there
// is one.
func TestParseErrorNamesTheRule(t *testing.T) {
	_, err := parse(strings.NewReader(
		strings.Replace(base(""), "kind: revenue", "kind: income", 1)), "mappings/budget.yaml")
	if err == nil {
		t.Fatal("got nil error")
	}
	var pe *parseError
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
	_, err := parse(strings.NewReader("schema_version: 99\ndoc_id: d\nrules: []\n"), "x.yaml")
	var h *cmdutil.ErrHint
	if !errors.As(err, &h) {
		t.Fatalf("got %T, want an *ErrHint telling the user to upgrade", err)
	}
}

// base returns a minimal valid rule file, with extra text appended inside the
// rule so tests can introduce one specific defect.
func base(extra string) string {
	return "schema_version: 1\ndoc_id: d\nrules:\n  - id: r\n" +
		"    kind: revenue\n    basis: adopted\n    grain: category\n    units: dollars\n" +
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
			want: "has no category",
		},
		{
			// A DEPARTMENT IS NOT A SUBSTITUTE FOR A CATEGORY, and until
			// 2026-08-29 it was accepted as one. A fact with no category is in
			// no graph unless its scope is projected, so in an unprojected
			// scope it was named by nothing at all -- fisc verify stayed fully
			// green with two such facts in the store.
			name: "row that declares a department and no category",
			yaml: strings.Replace(base(""), `{label: "A", category: a}`,
				`{label: "A", department: city-manager}`, 1),
			want: "has no category",
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
			_, err := parse(strings.NewReader(tt.yaml), "x.yaml")
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
	_, err := parse(strings.NewReader(strings.Replace(base(""), "schema_version: 1\n", "", 1)), "x.yaml")
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

// headerRule writes a one-rule file whose single part declares the given
// column_headers over two columns, so the validation can be exercised without
// restating the whole schema each time.
func headerRule(headers string) string {
	return headerRuleCols("[{fiscal_year: 2026}, {fiscal_year: 2027}]", headers)
}

// headerRuleCols is the same, with the column list spelled out, for the rules
// about a null entry -- which are claims about the COLUMN opposite it.
func headerRuleCols(columns, headers string) string {
	return "schema_version: 1\ndoc_id: d\nrules:\n  - id: r\n" +
		"    kind: revenue\n    basis: adopted\n    grain: category\n    units: dollars\n" +
		"    parts:\n      - page: 1\n" +
		"        columns: " + columns + "\n" +
		"        column_headers: " + headers + "\n" +
		"    rows:\n      - {label: \"A\", category: a}\n"
}

func TestParseAcceptsColumnHeaders(t *testing.T) {
	f, err := parse(strings.NewReader(
		headerRule(`["FY 2025-26", "FY 2026-27"]`)), "headers.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := f.Rules[0].Parts[0].ColumnHeaders
	if diff := cmp.Diff(columnHeaders{{Text: "FY 2025-26"}, {Text: "FY 2026-27"}}, got); diff != "" {
		t.Errorf("column_headers (-want +got):\n%s", diff)
	}
}

// TestParseAcceptsRepeatedColumnHeaders is the opposite of the rule for columns,
// and the comment matters more than the assertion. Budget Book p66 prints
// "FY 2025-26" over the General Fund and again over Enterprise Funds; the header
// says where a column sits on the page, not which column it is. A reviewer
// reaching for the duplicate-column check would break the spine.
func TestParseAcceptsRepeatedColumnHeaders(t *testing.T) {
	if _, err := parse(strings.NewReader(
		headerRule(`["FY 2025-26", "FY 2025-26"]`)), "headers.yaml"); err != nil {
		t.Errorf("Parse rejected repeated headers: %v", err)
	}
}

// TestParseAcceptsANullOverASkippedLastColumn is the channel fisc-wfi opened,
// and the assertion is that the entry COUNT survives it.
//
// Budget Book p76 prints four year headers and a footnote marker right of them
// on every row. The marker column has to be declared -- labelledValues
// truncates a row to len(Columns), so at four the read stops short and checkGap
// refuses "(10)" as unexplained text between rows -- and the page prints no
// header to name it with. Without this, the one-entry-per-column rule and the
// no-empty-entry rule between them make the page unpublishable.
func TestParseAcceptsANullOverASkippedLastColumn(t *testing.T) {
	f, err := parse(strings.NewReader(headerRuleCols(
		"[{fiscal_year: 2026}, {fiscal_year: 2027}, {skip: true}]",
		`["FY 2025-26", "FY 2026-27", ~]`)), "headers.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := f.Rules[0].Parts[0].ColumnHeaders
	want := columnHeaders{{Text: "FY 2025-26"}, {Text: "FY 2026-27"}, {Unheaded: true}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("column_headers (-want +got):\n%s", diff)
	}
	// The count is still the column count, which is the whole point: a short
	// list would build a grid one band too narrow and file every figure right
	// of the gap one place left, silently.
	if len(got) != len(f.Rules[0].Parts[0].Columns) {
		t.Errorf("%d headers for %d columns; the entry count has stopped being the "+
			"column-count check", len(got), len(f.Rules[0].Parts[0].Columns))
	}
}

func TestParseRejectsBadColumnHeaders(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{
			// One band short: every figure right of the missing column files one
			// place left, and the value count still matches.
			name: "fewer headers than columns",
			yaml: headerRule(`["FY 2025-26"]`),
			want: "has 1 entries but the part has 2 columns",
		},
		{
			name: "more headers than columns",
			yaml: headerRule(`["FY 2025-26", "FY 2026-27", "FY 2027-28"]`),
			want: "has 3 entries but the part has 2 columns",
		},
		{
			name: "an empty entry",
			yaml: headerRule(`["FY 2025-26", ""]`),
			want: "entry 2 is empty",
		},
		{
			name: "a whitespace-only entry",
			yaml: headerRule(`["FY 2025-26", "   "]`),
			want: "entry 2 is empty",
		},
		{
			// A grid built from a data row files that row perfectly and every
			// other row by luck.
			name: "a header that is a currency amount",
			yaml: headerRule(`["FY 2025-26", "1,234"]`),
			want: `entry 2 is a currency amount: "1,234"`,
		},
		{
			name: "a header that is a bare year",
			yaml: headerRule(`["FY 2025-26", "2026"]`),
			want: "entry 2 is a currency amount",
		},
		{
			// A dash is how this corpus spells zero, so it parses.
			name: "a header that is a zero dash",
			yaml: headerRule(`["FY 2025-26", "-"]`),
			want: "entry 2 is a currency amount",
		},
		{
			// null says the page prints no header, which leaves the column no
			// band. A column the rule READS then has nowhere to be placed.
			name: "a null over a column the rule reads",
			yaml: headerRule(`["FY 2025-26", ~]`),
			want: "entry 2 is null but column 2 is not skipped",
		},
		{
			// Between two headers the gap has known bounds and the grid would
			// have checked it, so a null there declines a check that was
			// available.
			name: "a null that is not the last entry",
			yaml: headerRuleCols(
				"[{fiscal_year: 2026}, {skip: true}, {fiscal_year: 2027}]",
				`["FY 2025-26", ~, "FY 2026-27"]`),
			want: "entry 2 is null but is not the last of 3",
		},
		{
			// The grid IS the headers, so a list naming none asks for a guard
			// that cannot be built. Left unrefused it panicked: geom.NewGrid
			// was handed an empty span list.
			name: "every entry is null",
			yaml: headerRuleCols("[{skip: true}, {skip: true}]", `[~, ~]`),
			want: "every entry is null",
		},
		{
			// "" is a typo, not a claim, and stays refused -- which is why
			// Unheaded is a field rather than an empty Text.
			name: "an empty string is not a null",
			yaml: headerRuleCols(
				"[{fiscal_year: 2026}, {skip: true}]", `["FY 2025-26", ""]`),
			want: "entry 2 is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(tt.yaml), "headers.yaml")
			if err == nil {
				t.Fatalf("Parse = nil error, want one mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestParseRejectsDisagreeingColumnHeaders covers the defect no per-part check
// can see. The published spine writes one page's header list out five times, once
// per rule; a copy that drifted would build a different grid for the same page in
// one rule than in another, and both reads would go on tying against their own
// printed totals.
func TestParseRejectsDisagreeingColumnHeaders(t *testing.T) {
	two := func(a, b string) string {
		part := func(headers string) string {
			return "    parts:\n      - page: 1\n" +
				"        columns: [{fiscal_year: 2026}, {fiscal_year: 2027}]\n" +
				"        column_headers: " + headers + "\n" +
				"    rows:\n      - {label: \"A\", category: a}\n"
		}
		return "schema_version: 1\ndoc_id: d\nrules:\n" +
			"  - id: first\n    kind: revenue\n    basis: adopted\n    grain: category\n    units: dollars\n" + part(a) +
			"  - id: second\n    kind: expenditure\n    basis: adopted\n    grain: category\n    units: dollars\n" + part(b)
	}

	agree := `["FY 2025-26", "FY 2026-27"]`
	if _, err := parse(strings.NewReader(two(agree, agree)), "headers.yaml"); err != nil {
		t.Fatalf("Parse rejected two parts that agree: %v", err)
	}

	_, err := parse(strings.NewReader(
		two(agree, `["FY 2025-26", "FY 2027-28"]`)), "headers.yaml")
	if err == nil {
		t.Fatal("Parse accepted two rules describing one page's grid differently")
	}
	want := `rule "first" in headers.yaml declares`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Parse error = %q, want it to name the rule that declared it first (%q)",
			err, want)
	}
}

// TestParseRejectsAPartLeftOutOfTheColumnGuard covers the hole a per-page
// agreement check leaves open on its own: a page where four parts declare a grid
// and the fifth does not validates fine, and that fifth part's figures are then
// placed by position alone with nothing saying so.
func TestParseRejectsAPartLeftOutOfTheColumnGuard(t *testing.T) {
	part := func(headers string) string {
		s := "    parts:\n      - page: 1\n" +
			"        columns: [{fiscal_year: 2026}, {fiscal_year: 2027}]\n"
		if headers != "" {
			s += "        column_headers: " + headers + "\n"
		}
		return s + "    rows:\n      - {label: \"A\", category: a}\n"
	}
	src := "schema_version: 1\ndoc_id: d\nrules:\n" +
		"  - id: guarded\n    kind: revenue\n    basis: adopted\n    grain: category\n    units: dollars\n" +
		part(`["FY 2025-26", "FY 2026-27"]`) +
		"  - id: unguarded\n    kind: expenditure\n    basis: adopted\n    grain: category\n    units: dollars\n" +
		part("")

	_, err := parse(strings.NewReader(src), "headers.yaml")
	if err == nil {
		t.Fatal("Parse accepted a page one of whose parts opts out of the column guard")
	}
	for _, want := range []string{"unguarded", "declares no column_headers", `rule "guarded"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Parse error = %q, want it to mention %q", err, want)
		}
	}
}

// TestLoadDirRejectsGridsDisagreeingAcrossFiles is the same check one level up.
// LoadDir dedupes rule ids and never looks at doc_id, so two files mapping one
// document are otherwise never compared.
func TestLoadDirRejectsGridsDisagreeingAcrossFiles(t *testing.T) {
	file := func(rule, headers string) string {
		return "schema_version: 1\ndoc_id: shared-doc\nrules:\n  - id: " + rule +
			"\n    kind: revenue\n    basis: adopted\n    grain: category\n    units: dollars\n" +
			"    parts:\n      - page: 1\n" +
			"        columns: [{fiscal_year: 2026}, {fiscal_year: 2027}]\n" +
			"        column_headers: " + headers + "\n" +
			"    rows:\n      - {label: \"A\", category: a}\n"
	}
	fsys := fstest.MapFS{
		"m/a.yaml": &fstest.MapFile{Data: []byte(file("first", `["FY 2025-26", "FY 2026-27"]`))},
		"m/b.yaml": &fstest.MapFile{Data: []byte(file("second", `["FY 2025-26", "FY 2027-28"]`))},
	}
	_, err := LoadDir(fsys, "m")
	if err == nil {
		t.Fatal("LoadDir accepted two files describing one page's grid differently")
	}
	if want := "m/a.yaml"; !strings.Contains(err.Error(), want) {
		t.Errorf("LoadDir error = %q, want it to name the file that declared it first (%q)",
			err, want)
	}
}
