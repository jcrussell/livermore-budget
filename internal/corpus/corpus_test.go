package corpus

import (
	"encoding/json"
	"errors"
	"fmt"
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

// TestReadsTheCommittedProvenance is the other half of "this reader and
// tools/extract.py still agree": the manifest keys that are not artifacts.
//
// It pins the pinned versions against a real manifest, which is what makes the
// pin a fact about this corpus rather than a constant nobody compares. A
// re-extraction changes these numbers, and the change has to be a deliberate
// edit here in the same commit — that is the whole mechanism.
func TestReadsTheCommittedProvenance(t *testing.T) {
	d, err := Open(os.DirFS(realDoc))
	if err != nil {
		t.Fatalf("Open(%s): %v", realDoc, err)
	}
	if got, want := d.ExtractorVersion(), PinnedExtractorVersion; got != want {
		t.Errorf("ExtractorVersion() = %d, want the pinned %d", got, want)
	}
	if got, want := d.PopplerVersion(), PinnedPopplerVersion; got != want {
		t.Errorf("PopplerVersion() = %q, want the pinned %q", got, want)
	}
	// The source records, which `fisc verify` compares against data/sources.yaml.
	// Asserted as non-empty and self-consistent here; the comparison against the
	// registry is verify's, because agreement between two files is not something
	// either file's reader can claim.
	if got, want := d.SourceFile(), "data/pdf/"+realDocID+".pdf"; got != want {
		t.Errorf("SourceFile() = %q, want %q", got, want)
	}
	if len(d.SourceSHA256()) != 64 {
		t.Errorf("SourceSHA256() = %q, want a hex sha256", d.SourceSHA256())
	}
	if d.SourceBytes() <= 0 {
		t.Errorf("SourceBytes() = %d, want the size of the PDF it was made from", d.SourceBytes())
	}
	// The two channels the extractor reports through, and the difference between
	// them. Errors mean artifacts were not written; warnings mean poppler complained
	// and carried on, and this document's sibling carries three of them.
	if got := d.Errors(); len(got) != 0 {
		t.Errorf("Errors() = %v, want none: the committed extraction is complete", got)
	}
	if d.BlankPageCount() != 0 {
		t.Errorf("BlankPageCount() = %d, want 0 for the Budget Book", d.BlankPageCount())
	}
}

// TestReadsTheExtractorsWarnings is the ACFR, which is the document that actually
// carries some: poppler wrote three distinct complaints and exited 0 for every one.
// Decoding them is what lets `fisc verify` count them into its report instead of
// dropping the extractor's only account of what it could not read.
func TestReadsTheExtractorsWarnings(t *testing.T) {
	d, err := OpenDoc(realRoot, "livermore-acfr-fy2025")
	if err != nil {
		t.Fatalf("OpenDoc: %v", err)
	}
	warnings := d.Warnings()
	if len(warnings) == 0 {
		t.Fatal("the ACFR extraction records no warnings; it recorded three when written")
	}
	for _, w := range warnings {
		if w.Stage == "" || w.Message == "" || w.Count <= 0 {
			t.Errorf("warning %+v is missing a stage, a message or a count", w)
		}
	}
	// A blank page still has both artifacts, so it is invisible to any count of
	// them: this is the one genuinely blank page in the corpus, and the reason the
	// completeness check prints the number rather than asserting it is zero.
	if got, want := d.BlankPageCount(), 1; got != want {
		t.Errorf("BlankPageCount() = %d, want %d", got, want)
	}
	// Errors are the other channel and are empty, which is what extract.py warns is
	// not a promise that every page came out whole.
	if got := d.Errors(); len(got) != 0 {
		t.Errorf("Errors() = %v, want none", got)
	}
}

// TestGeometryPath pins the second substrate's artifact path. It is asserted rather
// than derived because tools/extract.py writes this name and Go only ever composes
// it: the two spellings are held together by this test and by the manifest it
// checks the path against.
func TestGeometryPath(t *testing.T) {
	if got, want := GeometryPath(1), "geometry/p0001.json"; got != want {
		t.Errorf("GeometryPath(1) = %q, want %q", got, want)
	}
	if got, want := GeometryPath(323), "geometry/p0323.json"; got != want {
		t.Errorf("GeometryPath(323) = %q, want %q", got, want)
	}
	d, err := Open(os.DirFS(realDoc))
	if err != nil {
		t.Fatalf("Open(%s): %v", realDoc, err)
	}
	for _, rel := range []string{PagePath(66), GeometryPath(66)} {
		if _, ok := d.Artifacts()[rel]; !ok {
			t.Errorf("the committed manifest does not list %q, so this reader and "+
				"tools/extract.py disagree about the artifact contract", rel)
		}
	}
}

// TestArtifactsIsACopy: the drift sweep in `fisc verify` walks these records and
// must not be able to edit the manifest the Doc answers questions from.
func TestArtifactsIsACopy(t *testing.T) {
	d, err := Open(docFS(t, map[string]string{"pages/p0001.txt": "first page"}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	artifacts := d.Artifacts()
	if len(artifacts) != 1 {
		t.Fatalf("Artifacts() = %v, want the one page", artifacts)
	}
	delete(artifacts, PagePath(1))
	artifacts["pages/p0002.txt"] = Artifact{Bytes: 1}

	if got := d.Artifacts(); len(got) != 1 {
		t.Errorf("Artifacts() = %v after a caller edited an earlier result, want the one page", got)
	}
	// And the page is still readable, which is the consequence that would bite:
	// Page consults the same records.
	if _, err := d.Page(1); err != nil {
		t.Errorf("Page(1) after a caller edited Artifacts(): %v", err)
	}
}

// TestTreeSeesWhatTheManifestDoesNot is why Tree exists. An unlisted file is
// unreachable through every manifest-keyed accessor by definition, and it is the
// thing the drift sweep has to be able to find.
func TestTreeSeesWhatTheManifestDoesNot(t *testing.T) {
	fsys := docFS(t, map[string]string{"pages/p0001.txt": "first page"})
	fsys["pages/p0002.txt"] = &fstest.MapFile{Data: []byte("a page nothing extracted")}

	d, err := Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := d.Artifacts()[PagePath(2)]; ok {
		t.Fatal("the fixture manifest lists the stray page, so this test covers nothing")
	}
	b, err := fs.ReadFile(d.Tree(), PagePath(2))
	if err != nil {
		t.Fatalf("read the unlisted page through Tree(): %v", err)
	}
	if got, want := string(b), "a page nothing extracted"; got != want {
		t.Errorf("Tree() read %q, want %q", got, want)
	}
	// Tree is confined to the extraction: an fs.FS refuses a path that climbs
	// out, so a hostile manifest key cannot address the filesystem at large.
	if _, err := fs.ReadFile(d.Tree(), "../../../etc/passwd"); err == nil {
		t.Error("Tree() read a path outside the extraction directory")
	}
}

// geometryJSON renders a geometry artifact the way tools/extract.py writes one.
func geometryJSON(docID string, page int, words string) string {
	return fmt.Sprintf(`{
 "doc_id": %q,
 "height": 792.0,
 "page": %d,
 "schema_version": 1,
 "width": 612.0,
 "words": [%s]
}
`, docID, page, words)
}

const oneWord = `[216.84,2.88,293.18,30.27,"BUDGET"]`

func TestGeometryReadsTheSecondSubstrate(t *testing.T) {
	d, err := Open(docFS(t, map[string]string{
		GeometryPath(1): geometryJSON("test-doc", 1, oneWord),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	g, err := d.Geometry(1)
	if err != nil {
		t.Fatalf("Geometry(1): %v", err)
	}
	if got, want := len(g.Words), 1; got != want {
		t.Fatalf("Geometry(1) has %d words, want %d", got, want)
	}
	if got, want := g.Words[0].Text, "BUDGET"; got != want {
		t.Errorf("word text = %q, want %q", got, want)
	}
	if got, want := g.Words[0].Right(), 293.18; got != want {
		t.Errorf("word right edge = %v, want %v", got, want)
	}
}

// TestGeometryRefusesWhatItCannotVouchFor covers the three ways this reader
// declines to hand back a page. The doc_id/page disagreement is the one that
// needs the request as a witness: an artifact copied between extraction
// directories agrees with itself perfectly.
func TestGeometryRefusesWhatItCannotVouchFor(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		page  int
		want  string
	}{
		{
			name:  "a page the manifest does not list",
			files: map[string]string{GeometryPath(1): geometryJSON("test-doc", 1, oneWord)},
			page:  2,
			want:  "geometry for page 2 (geometry/p0002.json)",
		},
		{
			name:  "an artifact this reader cannot decode",
			files: map[string]string{GeometryPath(1): `{"schema_version": 99}`},
			page:  1,
			want:  "geometry schema_version 99, want 1",
		},
		{
			name:  "geometry from another document",
			files: map[string]string{GeometryPath(1): geometryJSON("some-other-doc", 1, oneWord)},
			page:  1,
			want:  `declares doc_id "some-other-doc" page 1, but was read as test-doc page 1`,
		},
		{
			name:  "geometry from another page",
			files: map[string]string{GeometryPath(1): geometryJSON("test-doc", 7, oneWord)},
			page:  1,
			want:  "page 7, but was read as test-doc page 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Open(docFS(t, tt.files))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_, err = d.Geometry(tt.page)
			if err == nil {
				t.Fatalf("Geometry(%d) = nil error, want one mentioning %q", tt.page, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Geometry(%d) error = %q, want it to mention %q", tt.page, err, tt.want)
			}
		})
	}
}

// TestGeometryUnlistedPageIsErrNotFound keeps the two substrates' failure
// vocabularies identical, so a caller can treat "this document has no page 900"
// the same way whichever one it asked for.
func TestGeometryUnlistedPageIsErrNotFound(t *testing.T) {
	d, err := Open(docFS(t, map[string]string{PagePath(1): "text"}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := d.Geometry(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Geometry(1) on a page with text but no geometry = %v, want ErrNotFound", err)
	}
	if _, err := d.Page(900); !errors.Is(err, ErrNotFound) {
		t.Errorf("Page(900) = %v, want ErrNotFound", err)
	}
}

// TestGeometryReadsTheCommittedCorpus is the claim the inline fixtures above
// cannot make: that this reader and tools/extract.py still agree about the real
// artifacts. It reads the spine's own geometry, which is what the column guard
// will read.
func TestGeometryReadsTheCommittedCorpus(t *testing.T) {
	d, err := Open(os.DirFS(realDoc))
	if err != nil {
		t.Fatalf("Open(%s): %v", realDoc, err)
	}
	for _, page := range []int{66, 67} {
		g, err := d.Geometry(page)
		if err != nil {
			t.Fatalf("Geometry(%d): %v", page, err)
		}
		if got, want := g.DocID, realDocID; got != want {
			t.Errorf("p%d doc_id = %q, want %q", page, got, want)
		}
		// Every page of this schedule is a wall of figures; a page that decoded
		// to a handful of words would mean the artifact contract moved.
		if len(g.Words) < 100 {
			t.Errorf("p%d decoded %d words, want the full page", page, len(g.Words))
		}
		if _, ok := g.MedianWordHeight(); !ok {
			t.Errorf("p%d reports no median word height", page)
		}
	}
}
