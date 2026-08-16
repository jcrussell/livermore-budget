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

	// 40 on p66 revenues + 16 on p66 expenditures + 72 on p67. The spike's 136
	// is these 128 plus the 8 column positions of the one row p67 omits, which
	// resolution reports as a declared omission rather than inventing as zeros.
	if want := 128; readValues != want {
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

// TestOmittedRowIsReportedNotInvented guards the "absent is not zero"
// invariant: p67 does not print Licenses & Permits, and resolution says so
// rather than manufacturing eight zero facts pointing at a row the page has
// no line for.
func TestOmittedRowIsReportedNotInvented(t *testing.T) {
	r, f := spineResolver(t, 66, 67)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 67)

	values, omitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if want := 72; len(values) != want {
		t.Errorf("read %d values, want %d (9 rows × 8 columns)", len(values), want)
	}
	if len(omitted) != 1 {
		t.Fatalf("got %d declared omissions, want 1", len(omitted))
	}
	if got, want := omitted[0].Row.Label, "Licenses & Permits"; got != want {
		t.Errorf("omitted row = %q, want %q", got, want)
	}
	for _, v := range values {
		if v.Row.Label == "Licenses & Permits" {
			t.Errorf("read a value for %q, which p67 does not print", v.Row.Label)
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
	r, f := spineResolver(t, 66, 67)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 67)
	p.OmittedRows = nil

	_, _, err := r.Values(ru, p)
	if err == nil {
		t.Fatal("Values with the omission undeclared = nil error, want a failure")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want it to wrap ErrNotFound", err)
	}
	// 9 rows were read positionally where the rule now claims 10.
	for _, want := range []string{"read 72 values", "want 80"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
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
	r, f := spineResolver(t, 66, 67)
	ru := rule(t, f, "spine-revenues")
	p := partOn(t, ru, 67)

	// "Intergovernmental" is row 2 of 10. Standing it in for the real omission
	// keeps the count at 9 rows, so the positional read still resolves.
	p.OmittedRows = []string{"Intergovernmental"}

	values, omitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(omitted) != 1 || omitted[0].RowIndex != 2 {
		t.Fatalf("omission = %+v, want RowIndex 2 (Intergovernmental)", omitted)
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
