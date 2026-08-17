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

func TestOpenRejectsAManifestItCannotTrust(t *testing.T) {
	tests := []struct {
		name string
		man  string
		want string
	}{
		{"future schema", `{"schema_version": 3, "doc_id": "x"}`, "schema_version 3, want 2"},
		{"stale schema", `{"schema_version": 1, "doc_id": "x"}`, "schema_version 1, want 2"},
		{"no schema", `{"doc_id": "x"}`, "schema_version 0, want 2"},
		{"no doc id", `{"schema_version": 2}`, "no doc_id"},
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
		"pages/p0001.txt": "first page",
		"pages/p0002.txt": "", // extracted but empty
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
	if !strings.Contains(err.Error(), "pages/p0009.txt") {
		t.Errorf("Page(9) error = %q, want it to name the artifact path", err)
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
// its manifest and its pages agree with each other — so nothing downstream can
// notice. The facts it produces would cite real pages of the wrong document.
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
	// The schedule's title is printed on TWO lines — the geometry puts
	// "CITYWIDE REVENUES, EXPENDITURES, AND" at y=74.36 and
	// "FUND BALANCE/WORKING CAPITAL" at y=87.92 — and `pdftotext -layout`
	// reproduces the break. Asserting the joined single line would be asserting
	// that the reader reflows the page, which is precisely what this substrate
	// must not do: the runs of spaces between these lines ARE the column grid
	// every positional read on p67 depends on.
	for _, want := range []string{
		"CITYWIDE REVENUES, EXPENDITURES, AND\n",
		"FUND BALANCE/WORKING CAPITAL\n",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Page(66) does not contain %q", want)
		}
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
