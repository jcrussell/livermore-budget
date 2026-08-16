package corpus

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

// realRoot is the repository root, and realDocID the Budget Book extraction
// committed under it. Tests read them directly: the artifacts are checked in,
// so this needs neither Python nor the PDF, and it is the only way to check
// that this reader and tools/extract.py still agree about the artifact
// contract.
const (
	realRoot  = "../.."
	realDocID = "livermore-budget-fy2026-2027"
	realDoc   = realRoot + "/data/extracted/" + realDocID
)

// docFS builds a minimal in-memory document. Fixtures are inline so the
// expectation and the input read on one screen (test-tempdir).
func docFS(t *testing.T, files map[string]string) fstest.MapFS {
	t.Helper()
	artifacts := map[string]Artifact{}
	fsys := fstest.MapFS{}
	for name, body := range files {
		artifacts[name] = Artifact{Bytes: int64(len(body))}
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	man, err := json.Marshal(map[string]any{
		"schema_version": SchemaVersion,
		"doc_id":         "test-doc",
		"page_count":     2,
		"artifacts":      artifacts,
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	fsys["manifest.json"] = &fstest.MapFile{Data: man}
	return fsys
}

func tableJSON(t *testing.T, page, ordinal int, fingerprint string, bbox []float64, cells [][]string) string {
	t.Helper()
	b, err := json.Marshal(Table{
		SchemaVersion: SchemaVersion, DocID: "test-doc", Page: page, Ordinal: ordinal,
		BBox: bbox, NRows: len(cells), Cells: cells, LabelFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatalf("marshal table: %v", err)
	}
	return string(b)
}

func TestOpenRejectsAManifestItCannotTrust(t *testing.T) {
	tests := []struct {
		name string
		man  string
		want string
	}{
		{"future schema", `{"schema_version": 2, "doc_id": "x"}`, "schema_version 2, want 1"},
		{"no schema", `{"doc_id": "x"}`, "schema_version 0, want 1"},
		{"no doc id", `{"schema_version": 1}`, "no doc_id"},
		{"not json", `{`, "parse manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte(tt.man)}})
			if err == nil {
				t.Fatalf("Open(%s) = nil error, want %q", tt.man, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open(%s) error = %q, want it to contain %q", tt.man, err, tt.want)
			}
		})
	}
}

func TestOpenWithNoManifest(t *testing.T) {
	if _, err := Open(fstest.MapFS{}); err == nil {
		t.Fatal("Open(empty fs) = nil error, want a failure")
	}
}

func TestPage(t *testing.T) {
	d, err := Open(docFS(t, map[string]string{
		"pages/p0001.md": "first page",
		"pages/p0002.md": "", // extracted but empty
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := d.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	if want := "first page"; got != want {
		t.Errorf("Page(1) = %q, want %q", got, want)
	}

	// A page the extractor found empty is empty, not an error: "the page is
	// blank" and "the document has no such page" are different facts.
	got, err = d.Page(2)
	if err != nil {
		t.Fatalf("Page(2): %v", err)
	}
	if got != "" {
		t.Errorf("Page(2) = %q, want %q", got, "")
	}

	_, err = d.Page(9)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Page(9) error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "pages/p0009.md") {
		t.Errorf("Page(9) error = %q, want it to name the artifact path", err)
	}
}

func TestTablesOnReturnsOrdinalOrder(t *testing.T) {
	d, err := Open(docFS(t, map[string]string{
		"pages/p0007.md":        "seventh",
		"tables/p0007-t02.json": tableJSON(t, 7, 2, "sha256:bbb", []float64{0, 0, 10, 20}, [][]string{{"second"}}),
		"tables/p0007-t01.json": tableJSON(t, 7, 1, "sha256:aaa", []float64{0, 0, 10, 20}, [][]string{{"first"}}),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	tables, err := d.TablesOn(7)
	if err != nil {
		t.Fatalf("TablesOn(7): %v", err)
	}
	var gotOrdinals []int
	var gotFirstRows []string
	for _, tb := range tables {
		gotOrdinals = append(gotOrdinals, tb.Ordinal)
		gotFirstRows = append(gotFirstRows, tb.FirstRow()[0])
	}
	if diff := cmp.Diff([]int{1, 2}, gotOrdinals); diff != "" {
		t.Errorf("TablesOn(7) ordinals mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"first", "second"}, gotFirstRows); diff != "" {
		t.Errorf("TablesOn(7) first rows mismatch (-want +got):\n%s", diff)
	}

	// A page with no tables is the common case in this corpus, not an error.
	none, err := d.TablesOn(1)
	if err != nil {
		t.Fatalf("TablesOn(1): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("TablesOn(1) = %d tables, want 0", len(none))
	}

	if diff := cmp.Diff([]int{7}, d.TablePages()); diff != "" {
		t.Errorf("TablePages mismatch (-want +got):\n%s", diff)
	}
}

// writeExtraction lays down an extraction directory named dirName whose
// manifest claims manifestDocID, so a test can make the two disagree.
func writeExtraction(t *testing.T, root, dirName, manifestDocID string) {
	t.Helper()
	dir := filepath.Join(root, "data", "extracted", dirName)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	man, err := json.Marshal(map[string]any{
		"schema_version": SchemaVersion,
		"doc_id":         manifestDocID,
		"artifacts":      map[string]Artifact{},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), man, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestOpenDoc(t *testing.T) {
	root := t.TempDir()
	writeExtraction(t, root, "budget", "budget")

	d, err := OpenDoc(root, "budget")
	if err != nil {
		t.Fatalf("OpenDoc: %v", err)
	}
	if got, want := d.DocID(), "budget"; got != want {
		t.Errorf("DocID() = %q, want %q", got, want)
	}
}

// TestOpenDocRefusesAMismatchedManifest is the check OpenDoc adds over Open.
// A directory holding another document's extraction is internally consistent —
// its manifest, pages and tables all agree with each other — so nothing
// downstream can notice. The facts it produces would cite real pages of the
// wrong document.
func TestOpenDocRefusesAMismatchedManifest(t *testing.T) {
	root := t.TempDir()
	writeExtraction(t, root, "budget", "acfr")

	_, err := OpenDoc(root, "budget")
	if err == nil {
		t.Fatal("OpenDoc = nil error, want a refusal")
	}
	for _, want := range []string{`"acfr"`, `"budget"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("OpenDoc error = %q, want it to contain %s", err, want)
		}
	}
}

func TestOpenDocWithNoSuchDocument(t *testing.T) {
	_, err := OpenDoc(t.TempDir(), "nope")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("OpenDoc error = %v, want it to wrap fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), filepath.Join("data", "extracted", "nope")) {
		t.Errorf("OpenDoc error = %q, want it to name the directory it looked in", err)
	}
}

func TestOpenDocReadsTheCommittedExtraction(t *testing.T) {
	d, err := OpenDoc(realRoot, realDocID)
	if err != nil {
		t.Fatalf("OpenDoc(%s, %s): %v", realRoot, realDocID, err)
	}
	if got, want := d.DocID(), realDocID; got != want {
		t.Errorf("DocID() = %q, want %q", got, want)
	}
}

func TestCentroid(t *testing.T) {
	tb := &Table{BBox: []float64{61, 275, 547, 398}}
	x, y, ok := tb.Centroid()
	if !ok {
		t.Fatal("Centroid ok = false, want true")
	}
	if wantX, wantY := 304.0, 336.5; x != wantX || y != wantY {
		t.Errorf("Centroid() = (%v, %v), want (%v, %v)", x, y, wantX, wantY)
	}

	// No bbox must report "unknown" rather than (0,0), which would collide
	// with a real table at the page origin.
	if _, _, ok := (&Table{}).Centroid(); ok {
		t.Error("Centroid() ok = true for a table with no bbox, want false")
	}
}

// TestReadsTheCommittedExtraction is the check that this reader and
// tools/extract.py still agree. It reads data/extracted/, which is committed,
// so it needs neither Python nor the source PDF.
func TestReadsTheCommittedExtraction(t *testing.T) {
	d, err := Open(os.DirFS(realDoc))
	if err != nil {
		t.Fatalf("Open(%s): %v", realDoc, err)
	}
	if got, want := d.DocID(), "livermore-budget-fy2026-2027"; got != want {
		t.Errorf("DocID() = %q, want %q", got, want)
	}
	if got, want := d.PageCount(), 268; got != want {
		t.Errorf("PageCount() = %d, want %d", got, want)
	}

	page, err := d.Page(66)
	if err != nil {
		t.Fatalf("Page(66): %v", err)
	}
	if want := "CITYWIDE REVENUES, EXPENDITURES, AND FUND BALANCE/WORKING CAPITAL"; !strings.Contains(page, want) {
		t.Errorf("Page(66) does not contain %q", want)
	}

	// p68 carries two tables; reading them exercises every field of the
	// serialization contract, including the null entries in row_page_lines.
	tables, err := d.TablesOn(68)
	if err != nil {
		t.Fatalf("TablesOn(68): %v", err)
	}
	if got, want := len(tables), 2; got != want {
		t.Fatalf("TablesOn(68) = %d tables, want %d", got, want)
	}
	first := tables[0]
	if got, want := first.Ordinal, 1; got != want {
		t.Errorf("ordinal = %d, want %d", got, want)
	}
	if got, want := first.NCols, 5; got != want {
		t.Errorf("n_cols = %d, want %d", got, want)
	}
	if !strings.HasPrefix(first.LabelFingerprint, "sha256:") {
		t.Errorf("label_fingerprint = %q, want a sha256: prefix", first.LabelFingerprint)
	}
	if got, want := len(first.RowPageLines), first.NRows; got != want {
		t.Errorf("row_page_lines has %d entries, want one per row (%d)", got, want)
	}
	if _, _, ok := first.Centroid(); !ok {
		t.Error("Centroid() ok = false for a real table, want true")
	}
}

// TestOpenDocRefusesAPathForADocumentID guards the one input to OpenDoc that
// does not come from this repository: doc_id is whatever a mapping rule file
// declares, and internal/mapping only checks that it is non-empty. A traversal
// escapes the repository entirely, and the doc_id-agreement check cannot catch
// it — an extraction sitting outside the tree declares its own doc_id and so
// agrees with itself.
func TestOpenDocRefusesAPathForADocumentID(t *testing.T) {
	for _, docID := range []string{
		"../../../../tmp/evil",
		"..",
		".",
		"a/b",
		`a\b`,
		"",
	} {
		t.Run(docID, func(t *testing.T) {
			if _, err := OpenDoc(t.TempDir(), docID); err == nil {
				t.Fatalf("OpenDoc(root, %q) = nil error, want a refusal", docID)
			}
		})
	}
}
