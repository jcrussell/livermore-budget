package mapping

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// The spine's headline figures, as the Budget Book prints them on p66 and p67.
// They are stated here in dollars and converted, so the expectation reads the
// way the document does.
const (
	generalFundFY2026Expenditures = 144_650_802
	allFundsFY2026Revenues        = 299_969_007
)

// budgetDoc builds an extraction of the Budget Book containing only the pages
// a test needs, from the committed fixtures. The fixtures are byte-identical
// copies of data/extracted/, so a test that passes here passes against the
// real corpus (testdata/README.md).
func budgetDoc(t *testing.T, pages ...int) *corpus.Doc {
	t.Helper()
	return testDoc(t, "livermore-budget-fy2026-2027", pages, nil)
}

// inlineDoc builds a document from page text written in the test itself, for
// shapes the corpus does not currently contain. Fixtures copied from
// data/extracted/ are preferred wherever a real page exercises the behaviour;
// this is for the cases where none does.
func inlineDoc(t *testing.T, docID string, pages map[int]string) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	artifacts := map[string]corpus.Artifact{}
	for n, body := range pages {
		name := corpus.PagePath(n)
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
		artifacts[name] = corpus.Artifact{Bytes: int64(len(body))}
	}
	return openDoc(t, docID, fsys, artifacts)
}

func testDoc(t *testing.T, docID string, pages []int, tables map[string]string) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	artifacts := map[string]corpus.Artifact{}

	for _, p := range pages {
		body, err := os.ReadFile(fmt.Sprintf("../../testdata/pages/budget-p%04d.md", p))
		if err != nil {
			t.Fatalf("read page fixture: %v", err)
		}
		name := corpus.PagePath(p)
		fsys[name] = &fstest.MapFile{Data: body}
		artifacts[name] = corpus.Artifact{Bytes: int64(len(body))}
	}
	for name, fixture := range tables {
		body, err := os.ReadFile("../../testdata/tables/" + fixture)
		if err != nil {
			t.Fatalf("read table fixture: %v", err)
		}
		fsys[name] = &fstest.MapFile{Data: body}
		artifacts[name] = corpus.Artifact{Bytes: int64(len(body))}
	}

	return openDoc(t, docID, fsys, artifacts)
}

func openDoc(t *testing.T, docID string, fsys fstest.MapFS, artifacts map[string]corpus.Artifact) *corpus.Doc {
	t.Helper()
	man, err := json.Marshal(map[string]any{
		"schema_version": corpus.SchemaVersion,
		"doc_id":         docID,
		"artifacts":      artifacts,
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	fsys["manifest.json"] = &fstest.MapFile{Data: man}

	d, err := corpus.Open(fsys)
	if err != nil {
		t.Fatalf("corpus.Open: %v", err)
	}
	return d
}

func spineResolver(t *testing.T, pages ...int) (*Resolver, *File) {
	t.Helper()
	f, err := Load("testdata/spine.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, pages...), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, f
}

// diagnosis is everything a user sees for an error: the message, plus the
// remediation hint the runner prints under it. Tests assert on both, because a
// hint is where this project puts the command to run next and dropping one is
// a silent regression in the part of the error that is actually actionable.
func diagnosis(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("no error to diagnose")
	}
	var h *cmdutil.ErrHint
	if errors.As(err, &h) {
		return err.Error() + "\nhint: " + h.Hint
	}
	return err.Error()
}

func rule(t *testing.T, f *File, id string) *Rule {
	t.Helper()
	for i := range f.Rules {
		if f.Rules[i].ID == id {
			return &f.Rules[i]
		}
	}
	t.Fatalf("no rule %q in %s", id, f.Path)
	return nil
}

func partOn(t *testing.T, r *Rule, page int) *Part {
	t.Helper()
	for i := range r.Parts {
		if r.Parts[i].Page == page {
			return &r.Parts[i]
		}
	}
	t.Fatalf("rule %q has no part for page %d", r.ID, page)
	return nil
}

// TestSpineTiesToTheDocumentsOwnTotals is the check this package exists to
// make. It reads Budget Book pp. 66-67 through the committed rule and asserts
// that every column the city prints a total for reconciles to it exactly.
//
// The arithmetic is carried in the test rather than described, because a claim
// about these documents is only as good as the figures behind it: the M0 spike
// got 136 facts out of these two pages with all 16 column totals tying, and
// anything less than exact here is a bug, not a rounding question.
func TestSpineTiesToTheDocumentsOwnTotals(t *testing.T) {
	r, f := spineResolver(t, 66, 67)

	var readValues, checkedColumns int
	byColumn := map[string]amount.Cents{}
	for i := range f.Rules {
		ru := &f.Rules[i]
		for j := range ru.Parts {
			p := &ru.Parts[j]
			values, _, err := r.Values(ru, p)
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, p.Page, err)
			}
			readValues += len(values)
			for _, v := range values {
				byColumn[fmt.Sprintf("%s/%s/%d", ru.Kind, v.Column.FundGroup, v.Column.FiscalYear)] += v.Cents
			}

			if err := r.CheckTotals(ru, p); err != nil {
				t.Errorf("CheckTotals(%s, p%d): %v", ru.ID, p.Page, err)
				continue
			}
			checkedColumns += len(p.Columns)
		}
	}

	// 40 on p66 revenues + 16 on p66 expenditures + 80 on p67. The spike also
	// reported 136, but 8 of its were invented zeros standing in for a row it
	// believed p67 omits. The page prints that row; our extractor was dropping
	// it (fisc-c00). Every one of these 136 is a cell the city printed.
	if want := 136; readValues != want {
		t.Errorf("read %d values, want %d", readValues, want)
	}
	if want := 16; checkedColumns != want {
		t.Errorf("tied %d columns to a stated total, want %d", checkedColumns, want)
	}

	gf := byColumn["expenditure/general/2026"]
	if want := amount.Cents(generalFundFY2026Expenditures * 100); gf != want {
		t.Errorf("General Fund FY2025-26 expenditures = %s, want %s", gf, want)
	}

	var allFunds amount.Cents
	for key, sum := range byColumn {
		if strings.HasPrefix(key, "revenue/") && strings.HasSuffix(key, "/2026") {
			allFunds += sum
		}
	}
	if want := amount.Cents(allFundsFY2026Revenues * 100); allFunds != want {
		t.Errorf("all-funds FY2025-26 revenues = %s, want %s", allFunds, want)
	}
}

// omittedRowFixture builds a two-page document whose continuation page really
// does omit a row, plus the rule that reads it.
//
// It is synthetic on purpose. The spine used to serve as this fixture, on the
// belief that Budget Book p67 omits its all-zero "Licenses & Permits" row; the
// page prints all ten rows and our own extractor was deleting one (fisc-c00).
// No page in the corpus is currently known to omit a row, so exercising the
// declaration against a real one would mean waiting for a case that may not
// exist — while the machinery still has to work the day one turns up.
//
// The omitted row is deliberately in the MIDDLE. A trailing omission makes a
// Value's RowIndex and an Omission's agree by accident, which is exactly the
// bug the indices exist to prevent.
func omittedRowFixture(t *testing.T, declared string) (*Resolver, *Rule) {
	t.Helper()

	const labelled = "REVENUES: Alpha 10 11 Beta 20 21 Gamma 30 31 TOTAL: 60 63\n"
	// The continuation carries no labels and prints nothing for Beta.
	const continuation = "HEADER 100 101 300 301 TOTAL: 400 402\n"

	omitted := ""
	if declared != "" {
		omitted = fmt.Sprintf("        omitted_rows: [%q]\n", declared)
	}
	src := fmt.Sprintf(`schema_version: 1
doc_id: omission-fixture
rules:
  - id: omit-demo
    substrate: text
    kind: revenue
    basis: adopted
    scope: fixture
    units: dollars
    total_row: "TOTAL:"
    parts:
      - page: 1
        section: "REVENUES:"
        stop_at: "TOTAL:"
        columns:
          - {fund_group: general, fiscal_year: 2026}
          - {fund_group: general, fiscal_year: 2027}
      - page: 2
        labels_from: 1
        section: "HEADER"
        stop_at: "TOTAL:"
%s        columns:
          - {fund_group: enterprise, fiscal_year: 2026}
          - {fund_group: enterprise, fiscal_year: 2027}
    rows:
      - {label: "Alpha", category: alpha}
      - {label: "Beta", category: beta}
      - {label: "Gamma", category: gamma}
`, omitted)

	f, err := Parse(strings.NewReader(src), "omission-fixture.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(
		inlineDoc(t, "omission-fixture", map[int]string{1: labelled, 2: continuation}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}

// TestOmittedRowIsReportedNotInvented guards the "absent is not zero"
// invariant: where a page does not print a row, resolution says so rather than
// manufacturing zero facts pointing at a line the page has no line for.
func TestOmittedRowIsReportedNotInvented(t *testing.T) {
	r, ru := omittedRowFixture(t, "Beta")
	p := partOn(t, ru, 2)

	values, omitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if want := 4; len(values) != want {
		t.Errorf("read %d values, want %d (2 rows × 2 columns)", len(values), want)
	}
	if len(omitted) != 1 {
		t.Fatalf("got %d declared omissions, want 1", len(omitted))
	}
	if got, want := omitted[0].Row.Label, "Beta"; got != want {
		t.Errorf("omitted row = %q, want %q", got, want)
	}
	for _, v := range values {
		if v.Row.Label == "Beta" {
			t.Errorf("read a value for %q, which the page does not print", v.Row.Label)
		}
	}
}

// TestUnmappedRowFailsLoudly is the guard on the labelled read. Most of the
// corpus prints no total, so on those pages this check is the only thing
// standing between a rule that forgets a row and a published breakdown that
// quietly omits it.
func TestUnmappedRowFailsLoudly(t *testing.T) {
	r, f := spineResolver(t, 66)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 66)

	// Drop "Sales Taxes", which p66 prints between Miscellaneous Revenue and
	// Fines & Forfeitures.
	kept := make([]Row, 0, len(ru.Rows))
	for _, row := range ru.Rows {
		if row.Label != "Sales Taxes" {
			kept = append(kept, row)
		}
	}
	ru.Rows = kept

	_, _, err := r.Values(ru, p)
	if err == nil {
		t.Fatal("Values with a row missing from the rule = nil error, want a failure")
	}
	for _, want := range []string{"Sales Taxes", "Miscellaneous Revenue", "Fines & Forfeitures"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestUndeclaredOmissionFailsLoudly covers the label-less page, where row
// identity is positional and a missing declaration shifts every row after the
// gap. The count is what catches it; nothing else can.
func TestUndeclaredOmissionFailsLoudly(t *testing.T) {
	r, ru := omittedRowFixture(t, "")
	p := partOn(t, ru, 2)

	_, _, err := r.Values(ru, p)
	if err == nil {
		t.Fatal("Values with the omission undeclared = nil error, want a failure")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want it to wrap ErrNotFound", err)
	}
	// 2 rows were read positionally where the rule claims 3.
	for _, want := range []string{"read 4 values", "want 6"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestExtractionDropOutIsNotAnOmission is the regression test for fisc-c00.
//
// xberg's strip_repeating_text deleted the third of three identical all-dash
// rows from Budget Book p67, and the project recorded the loss as a property
// of the document by declaring it in omitted_rows. That is the one thing
// OmittedRows must never be used for: it is a claim about what the city
// printed, and RowLabel is part of the fact id, so the mislabelled facts would
// have been citable. This asserts the page carries all ten rows, so a
// re-extraction that silently loses one fails here rather than downstream.
func TestExtractionDropOutIsNotAnOmission(t *testing.T) {
	r, f := spineResolver(t, 66, 67)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 67)

	values, omitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if want := 10 * 8; len(values) != want {
		t.Errorf("read %d values on p67, want %d (10 rows × 8 columns)", len(values), want)
	}
	if len(omitted) != 0 {
		t.Errorf("got %d omissions on p67, want 0: the page prints every row", len(omitted))
	}

	// The three trailing rows are the ones the extractor used to collapse.
	// They are zero in all four of this page's fund groups, which is why the
	// loss was invisible to every check except the row count.
	for _, label := range []string{"Sales Taxes", "Fines & Forfeitures", "Licenses & Permits"} {
		n := 0
		for _, v := range values {
			if v.Row.Label != label {
				continue
			}
			n++
			if v.Cents != 0 {
				t.Errorf("p67 %s column %d = %s, want 0", label, v.ColumnIndex+1, v.Cents)
			}
		}
		if n != 8 {
			t.Errorf("p67 read %d values for %q, want 8", n, label)
		}
	}
}

// TestAmbiguousSectionAnchorListsCandidates: p66 prints "REVENUES:" twice, so
// without an ordinal there is no unique answer and resolution must say which
// two it found rather than take the first.
func TestAmbiguousSectionAnchorListsCandidates(t *testing.T) {
	r, f := spineResolver(t, 66)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 66)
	p.SectionOrdinal = 0

	_, err := r.Block(ru, p)
	if err == nil {
		t.Fatal("Block with an ambiguous anchor = nil error, want a failure")
	}
	if !errors.Is(err, ErrAmbiguous) {
		t.Errorf("error = %v, want it to wrap ErrAmbiguous", err)
	}
	got := diagnosis(t, err)
	for _, want := range []string{"occurs 2 times", "#1", "#2", "section_ordinal"} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnosis %q does not contain %q", got, want)
		}
	}
}

// TestSectionOrdinalPastTheEnd: asking for an occurrence the page does not
// have must fail, not fall back to the last one.
func TestSectionOrdinalPastTheEnd(t *testing.T) {
	r, f := spineResolver(t, 66)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 66)
	p.SectionOrdinal = 3

	_, err := r.Block(ru, p)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Block with section_ordinal past the end = %v, want ErrNotFound", err)
	}
}

// TestWrongDocumentIsRefused: the rule file and the extraction must agree
// before a single page is read. A rule resolved against the wrong document
// would not fail loudly — it would produce figures with working provenance.
func TestWrongDocumentIsRefused(t *testing.T) {
	f, err := Load("testdata/spine.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err = NewResolver(testDoc(t, "livermore-acfr-fy2025", nil, nil), f)
	if err == nil {
		t.Fatal("NewResolver against the wrong document = nil error, want a failure")
	}
	for _, want := range []string{"livermore-budget-fy2026-2027", "livermore-acfr-fy2025"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// The defects a code review found in the first cut of this resolver. Each was
// verified to slip through before the fix.
func TestResolveRejectsWhatItCannotRead(t *testing.T) {
	r, f := spineResolver(t, 66)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 66)

	t.Run("a table rule is not read as page text", func(t *testing.T) {
		// Substrate was never consulted, so a table rule resolved as text and
		// quietly produced figures from whatever the page happened to say.
		ru := *ru
		ru.Substrate = SubstrateTable
		if _, _, err := r.Values(&ru, p); err == nil {
			t.Fatal("Values on a table rule = nil error, want a refusal")
		} else if !strings.Contains(err.Error(), "substrate") {
			t.Errorf("error = %q, want it to name the substrate", err)
		}
	})

	t.Run("a text part has no table to resolve", func(t *testing.T) {
		// ResolveTable dereferenced p.Table unconditionally: nil for every
		// text part, so this panicked instead of reporting anything.
		if _, err := r.ResolveTable(ru, p); err == nil {
			t.Fatal("ResolveTable on a text part = nil error, want a refusal")
		} else if !errors.Is(err, ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("a negative section ordinal is refused, not indexed", func(t *testing.T) {
		p := *p
		p.SectionOrdinal = -1
		if _, err := r.Block(ru, &p); err == nil {
			t.Fatal("Block with a negative ordinal = nil error, want a refusal")
		} else if !strings.Contains(err.Error(), "count from 1") {
			t.Errorf("error = %q, want it to say ordinals count from 1", err)
		}
	})
}

// TestRowIndexIsStableAcrossAnOmission: a Value's RowIndex and an Omission's
// must index the same list, or a caller rebuilding the grid collides at one
// slot and leaves another empty. They agree by accident when the omitted row
// is last, as it is in the spine, so the check moves it to the middle.
func TestRowIndexIsStableAcrossAnOmission(t *testing.T) {
	r, ru := omittedRowFixture(t, "Beta")
	p := partOn(t, ru, 2)

	values, omitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(omitted) != 1 || omitted[0].RowIndex != 1 {
		t.Fatalf("omission = %+v, want RowIndex 1 (Beta)", omitted)
	}

	seen := map[int]string{}
	for _, v := range values {
		if prev, dup := seen[v.RowIndex]; dup && prev != v.Row.Label {
			t.Fatalf("RowIndex %d used by both %q and %q", v.RowIndex, prev, v.Row.Label)
		}
		seen[v.RowIndex] = v.Row.Label
		if got := ru.Rows[v.RowIndex].Label; got != v.Row.Label {
			t.Errorf("RowIndex %d is %q in the rule but the value carries %q",
				v.RowIndex, got, v.Row.Label)
		}
	}
	if _, collided := seen[omitted[0].RowIndex]; collided {
		t.Errorf("a value claims RowIndex %d, which the omission also claims",
			omitted[0].RowIndex)
	}
}

// tableRule builds a one-part table rule pointing at a locator, for the
// locator tests. It is inline rather than a fixture because each test varies
// one field of it.
func tableRule(t *testing.T, page int, loc *TableLocator) (*File, *Rule) {
	t.Helper()
	f := &File{SchemaVersion: SchemaVersion, DocID: "livermore-acfr-fy2025",
		Path: "inline.yaml", Rules: []Rule{{
			ID: "header", Substrate: SubstrateTable, Kind: KindRevenue,
			Basis: BasisAudited, Units: amount.Dollars,
			Rows: []Row{{Label: "City of Livermore", Category: "x"}},
			Parts: []Part{{Page: page, Table: loc,
				Columns: []Column{{FundGroup: "general", FiscalYear: 2025}}}},
		}}}
	if err := f.validate(); err != nil {
		t.Fatalf("the inline rule is itself invalid: %v", err)
	}
	return f, &f.Rules[0]
}

// TestTableLocatorDoesNotFollowARelocatedTable is the point of separating
// identity from integrity. ACFR pp. 12 and 13 carry byte-identical tables —
// same label fingerprint, same content hash, same bounding box — so a resolver
// that searched by fingerprint when the page missed would happily return the
// wrong page's table and report success.
func TestTableLocatorDoesNotFollowARelocatedTable(t *testing.T) {
	// The real fingerprint, read from the fixture rather than hard-coded, so
	// the test cannot drift from the artifact.
	twelve := loadFixtureTable(t, "acfr-p0012-t01.json")
	thirteen := loadFixtureTable(t, "acfr-p0013-t01.json")
	if twelve.LabelFingerprint != thirteen.LabelFingerprint {
		t.Fatalf("fixtures no longer share a fingerprint: %s vs %s",
			twelve.LabelFingerprint, thirteen.LabelFingerprint)
	}

	// The document, with the table only on p13 — as if p12's copy had moved.
	d := testDoc(t, "livermore-acfr-fy2025", nil, map[string]string{
		"tables/p0013-t01.json": "acfr-p0013-t01.json",
	})
	f, ru := tableRule(t, 12, &TableLocator{Ordinal: 1,
		LabelFingerprint: twelve.LabelFingerprint})
	r, err := NewResolver(d, f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	_, err = r.ResolveTable(ru, &ru.Parts[0])
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveTable for a table that moved = %v, want ErrNotFound", err)
	}
	if got := diagnosis(t, err); !strings.Contains(got, "re-anchor") {
		t.Errorf("diagnosis %q does not point at re-anchoring", got)
	}
}

func TestTableLocatorChecksFingerprintAndPosition(t *testing.T) {
	twelve := loadFixtureTable(t, "acfr-p0012-t01.json")
	d := testDoc(t, "livermore-acfr-fy2025", nil, map[string]string{
		"tables/p0012-t01.json": "acfr-p0012-t01.json",
	})
	x, y, ok := twelve.Centroid()
	if !ok {
		t.Fatal("the fixture has no bbox")
	}

	tests := []struct {
		name    string
		loc     *TableLocator
		wantErr string
	}{
		{"resolves", &TableLocator{Ordinal: 1, LabelFingerprint: twelve.LabelFingerprint,
			BBoxCentroid: []float64{x, y}}, ""},
		{"wrong fingerprint", &TableLocator{Ordinal: 1,
			LabelFingerprint: "sha256:0000"}, "label_fingerprint"},
		{"wrong ordinal", &TableLocator{Ordinal: 2,
			LabelFingerprint: twelve.LabelFingerprint}, "none with ordinal 2"},
		{"moved on the page", &TableLocator{Ordinal: 1,
			LabelFingerprint: twelve.LabelFingerprint,
			BBoxCentroid:     []float64{x, y + 40}}, "further than 10pt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ru := tableRule(t, 12, tt.loc)
			r, err := NewResolver(d, f)
			if err != nil {
				t.Fatalf("NewResolver: %v", err)
			}
			got, err := r.ResolveTable(ru, &ru.Parts[0])
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ResolveTable: %v", err)
				}
				if diff := cmp.Diff(twelve.Cells, got.Cells); diff != "" {
					t.Errorf("resolved the wrong table (-want +got):\n%s", diff)
				}
				return
			}
			if err == nil {
				t.Fatalf("ResolveTable = nil error, want one mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func loadFixtureTable(t *testing.T, fixture string) *corpus.Table {
	t.Helper()
	b, err := os.ReadFile("../../testdata/tables/" + fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var tb corpus.Table
	if err := json.Unmarshal(b, &tb); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &tb
}
