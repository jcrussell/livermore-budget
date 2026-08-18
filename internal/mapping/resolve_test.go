package mapping

import (
	"bytes"
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

// fixtureDoc is one document's extraction directory together with the prefix
// its fixtures are named with under testdata/, because the two differ:
// data/extracted/livermore-budget-fy2026-2027/pages/p0066.txt is copied to
// testdata/pages/budget-p0066.txt, where it has to share a directory with the
// other documents' pages.
type fixtureDoc struct{ id, prefix string }

var (
	budgetFixtures = fixtureDoc{id: "livermore-budget-fy2026-2027", prefix: "budget"}
	acfrFixtures   = fixtureDoc{id: "livermore-acfr-fy2025", prefix: "acfr"}
)

// budgetDoc builds an extraction of the Budget Book containing only the pages
// a test needs, from the committed fixtures. The fixtures are byte-identical
// copies of data/extracted/, so a test that passes here passes against the
// real corpus (testdata/README.md).
func budgetDoc(t *testing.T, pages ...int) *corpus.Doc {
	t.Helper()
	return testDoc(t, budgetFixtures, pages)
}

// inlineDoc builds a document from page text written in the test itself, for
// shapes the corpus does not currently contain. Fixtures copied from
// data/extracted/ are preferred wherever a real page exercises the behaviour;
// this is for the cases where none does.
func inlineDoc(t *testing.T, docID string, pages map[int]string) *corpus.Doc {
	t.Helper()
	return inlineDocWithGeometry(t, docID, pages, nil)
}

// inlineDocWithGeometry is inlineDoc for a test that also needs the second
// substrate, with each page's geometry written as the extractor writes it. A
// page with no geometry entry gets none, so a test can also state what a
// document missing that artifact does.
func inlineDocWithGeometry(t *testing.T, docID string, pages, geometry map[int]string) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	artifacts := map[string]corpus.Artifact{}
	add := func(name, body string) {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
		artifacts[name] = corpus.Artifact{Bytes: int64(len(body))}
	}
	for n, body := range pages {
		add(corpus.PagePath(n), body)
	}
	for n, body := range geometry {
		add(corpus.GeometryPath(n), body)
	}
	return openDoc(t, docID, fsys, artifacts)
}

func testDoc(t *testing.T, doc fixtureDoc, pages []int) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	artifacts := map[string]corpus.Artifact{}

	// Both substrates, always. A document that carried page text without its
	// geometry is not a shape the extractor can produce, and building one here
	// would let a test pass against a corpus that cannot exist.
	for _, p := range pages {
		for _, c := range []fixtureCopy{pageCopy(doc, p), geometryCopy(doc, p)} {
			body, err := os.ReadFile(c.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			fsys[c.artifact] = &fstest.MapFile{Data: body}
			artifacts[c.artifact] = corpus.Artifact{Bytes: int64(len(body))}
		}
	}

	return openDoc(t, doc.id, fsys, artifacts)
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

// fixtureCopy is one committed fixture and the extraction artifact it must
// equal byte for byte.
type fixtureCopy struct {
	doc fixtureDoc
	// fixture is the committed copy, relative to this package.
	fixture string
	// artifact is the path within the extraction directory it was copied from.
	artifact string
}

// pageCopy names the page-text fixture for one page of one document.
func pageCopy(doc fixtureDoc, page int) fixtureCopy {
	return fixtureCopy{doc: doc,
		fixture:  fmt.Sprintf("../../testdata/pages/%s-p%04d.txt", doc.prefix, page),
		artifact: corpus.PagePath(page)}
}

// geometryCopy names the word-geometry fixture for one page of one document.
// Both substrates are committed for every fixture page, because a document
// carrying one and not the other is not a shape the real corpus can take.
func geometryCopy(doc fixtureDoc, page int) fixtureCopy {
	return fixtureCopy{doc: doc,
		fixture:  fmt.Sprintf("../../testdata/geometry/%s-p%04d.json", doc.prefix, page),
		artifact: corpus.GeometryPath(page)}
}

// extraction is the artifact this fixture was copied from.
func (f fixtureCopy) extraction() string {
	return fmt.Sprintf("../../data/extracted/%s/%s", f.doc.id, f.artifact)
}

// fixtureCopies is every fixture this package reads, as {document, artifact}
// rows. It replaces a hard-coded list of page numbers against one document and
// one substrate, which could express neither a second document nor the geometry
// artifacts the column guard needs (fisc-28u). Adding a fixture means adding a
// row here and nothing else.
func fixtureCopies() []fixtureCopy {
	var out []fixtureCopy
	for _, p := range fixturePages {
		out = append(out, pageCopy(budgetFixtures, p), geometryCopy(budgetFixtures, p))
	}
	return out
}

// fixturePages is every page of the Budget Book committed under testdata/.
// Each is there for a named failure mode; see testdata/README.md.
var fixturePages = []int{66, 67, 127, 167}

// TestFixturesAreVerbatimCopies is what makes every other test in this package
// mean anything. The fixtures are copies, and `make extract` does not touch
// them, so they can drift from the extraction silently — and when they do, the
// tests reading them stay green against a substrate that no longer exists. That
// is not hypothetical: it is exactly what the xberg → poppler migration
// produced (fisc-yqv.5), and the tests went on passing throughout.
func TestFixturesAreVerbatimCopies(t *testing.T) {
	copies := fixtureCopies()
	if len(copies) == 0 {
		t.Fatal("no fixtures listed, so this test asserts nothing")
	}
	for _, f := range copies {
		t.Run(f.fixture, func(t *testing.T) {
			fixture, err := os.ReadFile(f.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			extracted, err := os.ReadFile(f.extraction())
			if err != nil {
				t.Fatalf("read extraction: %v", err)
			}
			if !bytes.Equal(fixture, extracted) {
				t.Errorf("%s is not a verbatim copy of %s; "+
					"re-copy it rather than adjusting whatever now fails",
					f.fixture, f.extraction())
			}
		})
	}
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

			res, err := r.CheckTotals(ru, p)
			if err != nil {
				t.Errorf("CheckTotals(%s, p%d): %v", ru.ID, p.Page, err)
				continue
			}
			checkedColumns += res.Columns
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
	_, err = NewResolver(testDoc(t, acfrFixtures, nil), f)
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

// TestCurrencyMarkedTotalsRunSurvivesAGluedDataRow is the regression test for
// fisc-gxt. Extraction puts Budget Book p67's TOTAL EXPENDITURES row and its
// TRANSFER OUT row on one physical line, so sixteen amounts follow the "$"
// anchor where the part has eight columns. The page marks its totals with "$"
// and prints data rows bare, and that is the only thing separating them.
func TestCurrencyMarkedTotalsRunSurvivesAGluedDataRow(t *testing.T) {
	const glued = "$1,061,355 $969,934 $6,984,597 $6,969,898 " +
		"$19,267,561 $11,808,000 $25,077,367 $26,544,515 " +
		"28,584,740 36,047,736 - - 1,137,050 900,550 40,000 612,000"

	got, ok := amountRun(glued, 8, amount.Dollars)
	if !ok {
		t.Fatalf("amountRun(glued, 8) = not found; the $-marked prefix is exactly 8 wide")
	}
	want := []amount.Cents{
		1_061_355_00, 969_934_00, 6_984_597_00, 6_969_898_00,
		19_267_561_00, 11_808_000_00, 25_077_367_00, 26_544_515_00,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("amountRun mismatch (-want +got):\n%s", diff)
	}

	// The fallback must not rescue a run of the wrong width, or it would be a
	// licence to guess rather than a way to read what the page marked.
	if _, ok := amountRun(glued, 7, amount.Dollars); ok {
		t.Error("amountRun(glued, 7) found a run; a 7-wide read of an 8-wide totals row must fail")
	}
	// A bare run is still read exactly as before: no "$" means no prefix rule.
	if _, ok := amountRun("10 11 12 13 14", 3, amount.Dollars); ok {
		t.Error("amountRun on a bare 5-run found a 3-run; the maximal-run rule must still hold")
	}
}

// memoFixtureRule is a one-part rule whose page prints two labelled rows and a
// total. It is resolved against a scripted document, so a test can decide what
// a second read of the page would say.
const memoFixtureRule = `schema_version: 1
doc_id: memo-fixture
rules:
  - id: memo-demo
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
    rows:
      - {label: "Alpha", category: alpha}
      - {label: "Beta", category: beta}
`

// The scripted page, and the same page with different figures in its first row.
// The totals are the ones the FIRST reading ties to, so a resolver that went
// back to the document for a second look would not merely read different
// figures — it would fail to tie.
const (
	memoPageFirstRead  = "REVENUES: Alpha 10 11 Beta 20 21 TOTAL: 30 32\n"
	memoPageSecondRead = "REVENUES: Alpha 90 91 Beta 20 21 TOTAL: 30 32\n"
)

// TestCheckTotalsSeesTheFiguresValuesReturned pins the invariant fisc-uv6's
// memo is in service of: a build emits a part's facts and then asks the
// document's own totals to corroborate them, and those two steps must be
// looking at one reading of the page.
//
// This pins the invariant, not the mechanism, and it passed before the part
// memo existed: the page memo (Resolver.page) already made a second read of a
// part deterministic, which is why the part memo is a performance change and
// not a correctness one. The discriminating case is the failed read, which
// TestAFailedPartFailsTheSameWayEveryTime covers.
func TestCheckTotalsSeesTheFiguresValuesReturned(t *testing.T) {
	r, ru := scriptedResolver(t, memoFixtureRule, map[int][]pageRead{
		1: {{text: memoPageFirstRead}, {text: memoPageSecondRead}},
	})
	p := &ru.Parts[0]

	values, _, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	want := []amount.Cents{10_00, 11_00, 20_00, 21_00}
	var got []amount.Cents
	for _, v := range values {
		got = append(got, v.Cents)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}

	res, err := r.CheckTotals(ru, p)
	if err != nil {
		t.Fatalf("CheckTotals after Values: %v; it checked a different reading of the page", err)
	}
	if res.Columns != 2 {
		t.Errorf("TotalsResult.Columns = %d, want 2", res.Columns)
	}

	again, _, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values a second time: %v", err)
	}
	if diff := cmp.Diff(values, again); diff != "" {
		t.Errorf("the second Values differs from the first (-first +second):\n%s", diff)
	}
}

// TestAFailedPartFailsTheSameWayEveryTime is the one behavioural change
// fisc-uv6's memo makes: the memo caches the error too, so a part that could
// not be read does not become readable because a later caller happened to ask
// again. That would make what a build publishes depend on how many times it
// asked, and both answers would look equally authoritative.
//
// The scripted document fails the first read of the page and succeeds the
// second, which is exactly the shape that lets CheckTotals contradict the
// Values call before it: CheckTotals reads the page for the stated totals
// before it looks at the figures, so without the memo the second read is the
// one it gets.
func TestAFailedPartFailsTheSameWayEveryTime(t *testing.T) {
	errNoPage := errors.New("page is unavailable")
	r, ru := scriptedResolver(t, memoFixtureRule, map[int][]pageRead{
		1: {{err: errNoPage}, {text: memoPageFirstRead}},
	})
	p := &ru.Parts[0]

	if _, _, err := r.Values(ru, p); !errors.Is(err, errNoPage) {
		t.Fatalf("Values on an unreadable page = %v, want %v", err, errNoPage)
	}
	if _, err := r.CheckTotals(ru, p); !errors.Is(err, errNoPage) {
		t.Errorf("CheckTotals after a failed Values = %v, want %v; a part that "+
			"failed must not succeed on a retry the caller did not ask for", err, errNoPage)
	}
	if _, _, err := r.Values(ru, p); !errors.Is(err, errNoPage) {
		t.Errorf("Values a second time = %v, want %v", err, errNoPage)
	}
}

// TestValuesReturnsCopies: the memo hands out copies, or the first caller to
// sort or rewrite what it got would rewrite what every later caller sees.
// Value and Omission hold no slices, so a shallow copy is a whole one — this
// asserts that claim rather than restating it.
func TestValuesReturnsCopies(t *testing.T) {
	r, ru := omittedRowFixture(t, "Beta")
	p := partOn(t, ru, 2)

	values, omitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(values) == 0 || len(omitted) == 0 {
		t.Fatalf("got %d values and %d omissions, want some of each", len(values), len(omitted))
	}
	values[0].Cents = 99_99
	values[0].Row.Label = "MUTATED"
	values[0].Token = "MUTATED"
	omitted[0].Row.Label = "MUTATED"

	again, againOmitted, err := r.Values(ru, p)
	if err != nil {
		t.Fatalf("Values a second time: %v", err)
	}
	if again[0].Cents == 99_99 || again[0].Row.Label == "MUTATED" || again[0].Token == "MUTATED" {
		t.Errorf("a caller's writes reached the cache: second read = %+v", again[0])
	}
	if againOmitted[0].Row.Label == "MUTATED" {
		t.Errorf("a caller's writes reached the cached omissions: %+v", againOmitted[0])
	}
}
