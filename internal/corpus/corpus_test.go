package corpus

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

// realDoc is the committed extraction of the Budget Book. Tests read it
// directly: it is checked in, so this needs neither Python nor the PDF, and it
// is the only way to check that this reader and tools/extract.py still agree
// about the artifact contract.
const realDoc = "../../data/extracted/livermore-budget-fy2026-2027"

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
