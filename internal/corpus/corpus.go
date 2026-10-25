// Package corpus reads the committed extraction artifacts for one document,
// the read side of the extraction boundary: fisc needs neither Python nor the
// source PDFs.
//
// Reads do not hash. The manifest's sha256 records are exposed for the sweep
// in `fisc verify`, which can name every drifted file rather than the first.
package corpus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/geom"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/repo"
)

// SchemaVersion is the only manifest version this package understands. A
// reader pinned to an older version would have Open succeed and then report
// every page as ErrNotFound -- the normal "no such page" case -- so a mismatch
// fails in Open.
const SchemaVersion = 2

// ManifestFile is the manifest at the root of every extraction directory. It
// is NOT one of its own artifacts (it cannot record its own sha256), so a
// directory sweep must exclude it.
const ManifestFile = "manifest.json"

// The extraction toolchain, pinned: the versions that produced the committed
// artifacts under data/extracted/.
//
// These are NOT gated in [Open] as SchemaVersion is: SchemaVersion is a read
// contract, while an extraction from another toolchain is still readable and
// its drift is a `fisc verify` finding.
const (
	// PinnedExtractorVersion is tools/extract.py's EXTRACTOR_VERSION.
	PinnedExtractorVersion = 3
	// PinnedPopplerVersion is the `pdftotext -v` version that produced the
	// committed artifacts. Expect a different poppler to change extracted bytes;
	// see README.md for the re-extraction workflow.
	PinnedPopplerVersion = "24.02.0"
)

// ErrNotFound reports an artifact the manifest does not list -- the normal "no
// such page" -- distinct from fs.ErrNotExist, a file missing from disk.
var ErrNotFound = errors.New("not listed in the extraction manifest")

// Artifact is the manifest's record of one extracted file.
type Artifact struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// ExtractionError is one artifact the extractor could not produce: a poppler
// invocation that exited non-zero, or output it could not parse. The page it
// names has NO artifact, so a reader that does not look here sees only a page
// the document "does not have".
type ExtractionError struct {
	// Stage is which invocation failed: "layout" for the page text, "bbox" for
	// the geometry, "pdfinfo" for the page count.
	Stage string `json:"stage"`
	// Page is the page it failed on, or 0 for a document-level failure.
	Page    int    `json:"page"`
	Message string `json:"message"`
}

// Warning is one distinct line poppler wrote to stderr, folded across the
// invocations that produced it. Warnings are NOT failures: poppler exits 0
// with free-form English, so an empty [Doc.Errors] is not a promise that every
// page came out whole.
type Warning struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
	// Count is how many times poppler said it, which can exceed len(Pages) when
	// one invocation said it twice.
	Count int `json:"count"`
	// Pages is the pages whose invocations produced it, absent for a
	// document-level complaint.
	Pages []int `json:"pages"`
}

// manifest is the on-disk manifest.json. Unknown fields are tolerated;
// SchemaVersion guards the fields this package reads.
type manifest struct {
	SchemaVersion    int                 `json:"schema_version"`
	DocID            string              `json:"doc_id"`
	PageCount        int                 `json:"page_count"`
	BlankPageCount   int                 `json:"blank_page_count"`
	ExtractorVersion int                 `json:"extractor_version"`
	PopplerVersion   string              `json:"poppler_version"`
	SourceFile       string              `json:"source_file"`
	SourceSHA256     string              `json:"source_sha256"`
	SourceBytes      int64               `json:"source_bytes"`
	Warnings         []Warning           `json:"warnings"`
	Errors           []ExtractionError   `json:"errors"`
	Artifacts        map[string]Artifact `json:"artifacts"`
}

// Doc is one extracted document: its manifest, and lazy access to the pages
// it lists.
type Doc struct {
	fsys fs.FS
	man  manifest
}

// Open reads the manifest at the root of fsys. It does not read any page.
func Open(fsys fs.FS) (*Doc, error) {
	b, err := fs.ReadFile(fsys, ManifestFile)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("manifest schema_version %d, want %d",
			m.SchemaVersion, SchemaVersion)
	}
	if m.DocID == "" {
		return nil, errors.New("manifest has no doc_id")
	}

	return &Doc{fsys: fsys, man: m}, nil
}

// OpenDoc opens the extraction of docID beneath a repository root, and refuses
// one whose manifest names a different document: the directory name is the one
// witness a self-consistent wrong extraction cannot satisfy.
func OpenDoc(root, docID string) (*Doc, error) {
	// docID comes from a rule file unvalidated; unguarded, "../../../etc"
	// would address an out-of-tree extraction that agrees with itself.
	if !fs.ValidPath(docID) || docID == "." || strings.ContainsAny(docID, `/\`) {
		return nil, hint.With(
			fmt.Errorf("document id %q is not a single directory name", docID),
			"doc_id names one directory under "+repo.ExtractedDir+
				"; it may not contain a path separator or \"..\"")
	}
	dir := filepath.Join(root, repo.ExtractedDir, docID)
	d, err := Open(os.DirFS(dir))
	if err != nil {
		return nil, fmt.Errorf("open extraction %q: %w", dir, err)
	}
	if d.DocID() != docID {
		return nil, hint.With(
			fmt.Errorf("extraction %q declares doc_id %q, want %q", dir, d.DocID(), docID),
			"an extraction directory is named for the document it holds; "+
				"re-run make extract rather than renaming or copying the directory")
	}
	return d, nil
}

// DocID is the document this extraction belongs to; callers resolving rules
// against it are expected to compare.
func (d *Doc) DocID() string { return d.man.DocID }

// PageCount is how many pages the source PDF had, as pdfinfo reported it: the
// only manifest claim about how many artifacts there should be. `fisc verify`
// compares it against the artifacts and data/sources.yaml.
func (d *Doc) PageCount() int { return d.man.PageCount }

// BlankPageCount is how many pages poppler returned no text for at all. A
// blank page still has both artifacts, so it is invisible to any count.
func (d *Doc) BlankPageCount() int { return d.man.BlankPageCount }

// Errors is every failure the extractor recorded, and a non-empty result means
// this extraction is INCOMPLETE: the pages named have no artifact. A copy.
func (d *Doc) Errors() []ExtractionError { return slices.Clone(d.man.Errors) }

// Warnings is every distinct line poppler wrote to stderr; not failures (see
// [Warning]).
func (d *Doc) Warnings() []Warning { return slices.Clone(d.man.Warnings) }

// SourceFile is the path extract.py read the document from, as it recorded it.
// `fisc verify` cross-checks it and the two below against data/sources.yaml,
// recorded independently (AGENTS.md, "The extraction boundary").
func (d *Doc) SourceFile() string { return d.man.SourceFile }

// SourceSHA256 is the hash of the PDF this extraction was made from.
func (d *Doc) SourceSHA256() string { return d.man.SourceSHA256 }

// SourceBytes is the size of that PDF.
func (d *Doc) SourceBytes() int64 { return d.man.SourceBytes }

// ExtractorVersion is tools/extract.py's version, as this extraction recorded
// it. Compare against [PinnedExtractorVersion].
func (d *Doc) ExtractorVersion() int { return d.man.ExtractorVersion }

// PopplerVersion is the poppler build that produced this extraction. Compare
// against [PinnedPopplerVersion].
func (d *Doc) PopplerVersion() string { return d.man.PopplerVersion }

// PagePath is the artifact path for page n. .txt, not .md: GitHub's blob view
// collapses the runs of spaces that ARE the column grid.
func PagePath(n int) string { return "pages/" + PageStem(n) + ".txt" }

// PageStem is page n's file name without its extension, the one spelling of
// the zero-padded page number every per-page file is named by.
func PageStem(n int) string { return fmt.Sprintf("p%04d", n) }

// GeometryPath is the artifact path for page n's word geometry, the `-bbox`
// substrate that settles which column a token belongs to
// (AGENTS.md, "The extraction boundary").
func GeometryPath(n int) string { return "geometry/" + PageStem(n) + ".json" }

// Page returns the extracted layout text for page n.
//
// A page the document has but the extractor found empty reads as "", not an
// error; only a page the manifest does not list is ErrNotFound.
func (d *Doc) Page(n int) (string, error) {
	p := PagePath(n)
	if _, ok := d.man.Artifacts[p]; !ok {
		return "", fmt.Errorf("%s page %d (%s): %w", d.man.DocID, n, p, ErrNotFound)
	}
	b, err := fs.ReadFile(d.fsys, p)
	if err != nil {
		return "", fmt.Errorf("read %s page %d: %w", d.man.DocID, n, err)
	}
	return string(b), nil
}

// Geometry returns the word geometry for page n, the `-bbox` substrate. It
// mirrors [Doc.Page], including [ErrNotFound] for an unlisted page, and also
// decodes. The artifact's doc_id and page are checked against the request,
// which is the only witness to an artifact copied between extractions.
// Nothing is cached.
func (d *Doc) Geometry(n int) (*geom.Page, error) {
	p := GeometryPath(n)
	if _, ok := d.man.Artifacts[p]; !ok {
		return nil, fmt.Errorf("%s geometry for page %d (%s): %w", d.man.DocID, n, p, ErrNotFound)
	}
	b, err := fs.ReadFile(d.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("read %s geometry for page %d: %w", d.man.DocID, n, err)
	}
	g, err := geom.ParsePage(b)
	if err != nil {
		return nil, fmt.Errorf("%s (%s): %w", d.man.DocID, p, err)
	}
	if g.DocID != d.man.DocID || g.Number != n {
		return nil, hint.With(
			fmt.Errorf("%s declares doc_id %q page %d, but was read as %s page %d",
				p, g.DocID, g.Number, d.man.DocID, n),
			"a geometry artifact was copied or renamed between extractions; "+
				"re-run make extract rather than moving files between "+
				repo.ExtractedDir+" directories")
	}
	return g, nil
}

// Artifacts is the manifest's record of every file the extractor emitted: the
// path it wrote, and the size and sha256 of the bytes it wrote there. The map
// is a copy, and its keys are the manifest's own, uncleaned, so a finding cites
// the path the file contains.
func (d *Doc) Artifacts() map[string]Artifact { return maps.Clone(d.man.Artifacts) }

// Tree is the extraction directory, as the filesystem it was opened over, for
// the drift sweep that must see files the manifest does NOT list. An fs.FS
// rejects absolute paths and "..", so reads stay in the directory.
func (d *Doc) Tree() fs.FS { return d.fsys }
