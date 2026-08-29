// Package corpus reads the committed extraction artifacts for one document.
//
// Extraction itself is not a Go responsibility: `make extract` runs
// tools/extract.py, whose output is committed under data/extracted/<doc-id>/.
// This package is the read side of that boundary, so fisc needs neither Python
// nor the source PDFs — which is what lets CI verify without a venv or an LFS
// checkout.
//
// Reads deliberately do not hash. The manifest records a sha256 for every
// artifact, and this package exposes those records, but checking them is a
// sweep that belongs to `fisc verify` (fisc-1wr.5): a reader that hard-failed
// on the first mismatch could report "something drifted" but never "these
// eleven files drifted", which is the useful answer.
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
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// SchemaVersion is the only manifest version this package understands.
// tools/extract.py stamps it; a bump there means the artifact contract changed
// and the reader must be revisited rather than guess.
//
// 2: poppler replaced xberg. Page artifacts are pages/pNNNN.txt, the tables/
// namespace is gone in favour of geometry/pNNNN.json, six manifest keys were
// dropped and poppler_version added, and the warnings/errors elements changed
// shape. A reader pinned to 1 would have Open() succeed and then report every
// page as ErrNotFound -- which this package documents as the NORMAL "no such
// page" case -- so the mismatch has to fail here, loudly, or not at all.
const SchemaVersion = 2

// ManifestFile is the manifest at the root of every extraction directory.
//
// It is the one file under an extraction that is NOT one of its own artifacts:
// it records a sha256 for everything the extractor emitted, and it cannot
// record its own. A sweep that reads the directory has to know that, or it
// reports the manifest as an unvouched-for extra file on every run.
const ManifestFile = "manifest.json"

// The extraction toolchain, pinned. Every manifest records the versions that
// produced it, and these are the values the committed artifacts under
// data/extracted/ were produced by.
//
// The pin lives here, next to [SchemaVersion], because this package is the only
// Go reader of a manifest and the versions are its provenance chain. It is a Go
// constant and not a parse of requirements.txt on purpose: that file documents
// the poppler expectation for a human installing the toolchain and says in
// prose that poppler "is not pinned the way a pip requirement would be, because
// it is whatever the platform ships". Prose in a file no Go code reads cannot be
// a machine-checked pin, and scraping a version out of a comment would make an
// edit to that comment change what fisc accepts.
//
// These are NOT gated in [Open] the way SchemaVersion is, and the difference is
// the point. SchemaVersion is a READ CONTRACT: at version 1 the artifact
// namespace was different, so this reader cannot make sense of the directory at
// all and has to refuse it. An extraction from a different extractor or poppler
// build is still perfectly readable — it just is not the extraction that was
// reviewed, which is a claim about the corpus and belongs in a `fisc verify`
// finding that names the drift.
const (
	// PinnedExtractorVersion is tools/extract.py's EXTRACTOR_VERSION.
	PinnedExtractorVersion = 3
	// PinnedPopplerVersion is the `pdftotext -v` version that produced the
	// committed artifacts. Expect a different poppler to change extracted bytes;
	// see requirements.txt for the re-extraction workflow.
	PinnedPopplerVersion = "24.02.0"
)

// ErrNotFound reports an artifact the manifest does not list. It is distinct
// from fs.ErrNotExist: the manifest is the authority on what was extracted, so
// "the file is missing from disk" and "the document has no such page" are
// different failures and only the second is normal.
var ErrNotFound = errors.New("not listed in the extraction manifest")

// Artifact is the manifest's record of one extracted file.
type Artifact struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// ExtractionError is one artifact the extractor could not produce: a poppler
// invocation that exited non-zero, or output it could not parse.
//
// It is the extractor's own account of a failure, recorded in the manifest
// rather than only logged, because a run that failed on some pages must be
// visible to `fisc verify` instead of looking clean (tools/extract.py). The page
// it names has NO artifact — extract.py writes nothing rather than something
// plausible and wrong — so a reader that does not look here sees only a page the
// document "does not have", which this package documents as normal.
type ExtractionError struct {
	// Stage is which invocation failed: "layout" for the page text, "bbox" for
	// the geometry, "pdfinfo" for the page count.
	Stage string `json:"stage"`
	// Page is the page it failed on, or 0 for a document-level failure.
	Page    int    `json:"page"`
	Message string `json:"message"`
}

// Warning is one distinct line poppler wrote to stderr, folded across the
// invocations that produced it.
//
// Warnings are NOT failures and must not be read as any: poppler has no
// structured error channel, writes free-form English, and exits 0 for a damaged
// xref and an unread metadata key alike. The manifest records every line so that
// a human can read them; the consequence extract.py states plainly is that an
// empty [Doc.Errors] is not a promise that every page came out whole.
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

// manifest is the on-disk manifest.json. Unknown fields are tolerated rather
// than rejected: extract.py may add reporting keys, and SchemaVersion is the
// guard that matters for the fields this package actually reads.
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
// one whose manifest names a different document.
//
// The disagreement check is the whole reason this exists rather than callers
// writing the Join themselves. NewResolver already compares a rule file's
// doc_id against the extraction's — but it compares against the manifest, so a
// directory holding the wrong extraction agrees with itself and yields
// confident wrong facts carrying working-looking provenance. The directory
// name is the one witness a self-consistent corpus cannot satisfy.
func OpenDoc(root, docID string) (*Doc, error) {
	// docID arrives from a rule file's doc_id, which the parser only checks is
	// non-empty. Joining it unguarded lets "../../../etc" address anything on
	// the filesystem, and the identity check below cannot catch that: an
	// extraction placed out of tree is free to declare whatever doc_id it
	// likes and would agree with itself. A document id names one directory, so
	// it may not contain a separator or a parent reference at all.
	if !fs.ValidPath(docID) || docID == "." || strings.ContainsAny(docID, `/\`) {
		return nil, cmdutil.WithHint(
			fmt.Errorf("document id %q is not a single directory name", docID),
			"doc_id names one directory under "+cmdutil.ExtractedDir+
				"; it may not contain a path separator or \"..\"")
	}
	dir := filepath.Join(root, cmdutil.ExtractedDir, docID)
	d, err := Open(os.DirFS(dir))
	if err != nil {
		return nil, fmt.Errorf("open extraction %q: %w", dir, err)
	}
	if d.DocID() != docID {
		return nil, cmdutil.WithHint(
			fmt.Errorf("extraction %q declares doc_id %q, want %q", dir, d.DocID(), docID),
			"an extraction directory is named for the document it holds; "+
				"re-run make extract rather than renaming or copying the directory")
	}
	return d, nil
}

// DocID is the document this extraction belongs to. Mapping rules declare the
// same id, and resolving one document's rules against another's pages would
// produce confident wrong facts with working-looking provenance, so callers
// are expected to compare.
func (d *Doc) DocID() string { return d.man.DocID }

// PageCount is how many pages the source PDF had, as pdfinfo reported it.
//
// It is the extraction's own claim about the document's SIZE, and it is the only
// thing in the manifest that says how many artifacts there should be: the
// artifact map is otherwise checkable only against itself, so a manifest with
// every page deleted from it agrees with a directory with every page deleted from
// it. `fisc verify` compares this against both the artifacts and the page count
// data/sources.yaml records.
func (d *Doc) PageCount() int { return d.man.PageCount }

// BlankPageCount is how many pages poppler returned no text for at all.
//
// A blank page still HAS both artifacts — an empty pages/pNNNN.txt and a geometry
// file with no words — so it is invisible to any count. It is the honest caveat to
// "every page was extracted": one of the ACFR's 195 pages is genuinely blank, and
// a run where fifty came out blank would look complete.
func (d *Doc) BlankPageCount() int { return d.man.BlankPageCount }

// Errors is every failure the extractor recorded, and a non-empty result means
// this extraction is INCOMPLETE: the pages named have no artifact.
//
// The slice is a copy, for the reason [Doc.Artifacts] is.
func (d *Doc) Errors() []ExtractionError { return slices.Clone(d.man.Errors) }

// Warnings is every distinct line poppler wrote to stderr. See [Warning]: these
// are not failures, and an empty [Doc.Errors] beside a long Warnings is the normal
// state of this corpus.
func (d *Doc) Warnings() []Warning { return slices.Clone(d.man.Warnings) }

// SourceFile is the path extract.py read the document from, as it recorded it.
//
// This and the two below are the extraction's record of the document it came
// from. `fisc verify` cross-checks all three against data/sources.yaml, which
// records the same three claims; two parties recording them independently is
// what makes agreement evidence, and extract.py deliberately has no YAML parser
// so that neither copy is derived from the other
// (AGENTS.md, Provenance invariants, "the extraction boundary").
func (d *Doc) SourceFile() string { return d.man.SourceFile }

// SourceSHA256 is the hash of the PDF this extraction was made from.
func (d *Doc) SourceSHA256() string { return d.man.SourceSHA256 }

// SourceBytes is the size of that PDF.
func (d *Doc) SourceBytes() int64 { return d.man.SourceBytes }

// ExtractorVersion is tools/extract.py's version, as this extraction recorded
// it. Compare against [PinnedExtractorVersion]: a mismatch means the committed
// artifacts are not the ones that were reviewed.
func (d *Doc) ExtractorVersion() int { return d.man.ExtractorVersion }

// PopplerVersion is the poppler build that produced this extraction. Compare
// against [PinnedPopplerVersion].
func (d *Doc) PopplerVersion() string { return d.man.PopplerVersion }

// PagePath is the artifact path for page n.
//
// .txt, not .md: GitHub renders markdown in its blob view and collapses the
// runs of spaces that ARE the column grid, which would silently break the
// provenance deep links these paths are published as.
func PagePath(n int) string { return fmt.Sprintf("pages/p%04d.txt", n) }

// GeometryPath is the artifact path for page n's word geometry, the `-bbox`
// substrate that carries the x-position of every token.
//
// Both substrates are emitted for every page and both are needed: `-layout`
// reproduces the printed grid in runs of spaces but says nothing about which
// column a token belongs to on a sparse row, and geometry is what settles it
// (AGENTS.md, Provenance invariants, "the extraction boundary"). [Doc.Geometry] is the
// reader; `fisc verify` separately checks that both artifacts exist for every
// page, because an extraction missing half its geometry looks complete right up
// until a rule asks for the half that is gone.
func GeometryPath(n int) string { return fmt.Sprintf("geometry/p%04d.json", n) }

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

// Geometry returns the word geometry for page n, the `-bbox` substrate.
//
// It mirrors [Doc.Page] exactly, including the failure vocabulary: a page the
// manifest does not list is [ErrNotFound], which is the normal "no such page"
// case and not a defect. There is deliberately no separate "this page has text
// but no geometry" sentinel -- both artifacts are emitted for every page, so a
// missing one is a broken extraction rather than a state a caller branches on,
// and the message names the path either way.
//
// Unlike Page it also decodes, so a corrupt or unrecognised artifact fails here
// rather than downstream. The doc_id and page it carries are checked against the
// ones asked for: an artifact copied between extraction directories agrees with
// itself, and the request is the only witness that can catch it -- the same
// argument [OpenDoc] makes about a directory name.
//
// Nothing is cached. A caller reading the same page repeatedly is expected to
// hold on to the result, as internal/mapping's resolver does.
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
		return nil, cmdutil.WithHint(
			fmt.Errorf("%s declares doc_id %q page %d, but was read as %s page %d",
				p, g.DocID, g.Number, d.man.DocID, n),
			"a geometry artifact was copied or renamed between extractions; "+
				"re-run make extract rather than moving files between "+
				cmdutil.ExtractedDir+" directories")
	}
	return g, nil
}

// Artifacts is the manifest's record of every file the extractor emitted: the
// path it wrote, and the size and sha256 of the bytes it wrote there.
//
// This is the input to the drift sweep in `fisc verify`, and it hands back the
// records rather than a verdict for the reason this package does not hash: a
// reader that failed on the first mismatch could report "something drifted" but
// never "these eleven files drifted", which is the useful answer.
//
// The map is a copy, so a caller sweeping it cannot edit the manifest this Doc
// answers questions from. The keys are the manifest's own, uncleaned: they are
// the strings a finding has to name, and silently canonicalising one would make
// a report cite a path the file does not contain.
func (d *Doc) Artifacts() map[string]Artifact { return maps.Clone(d.man.Artifacts) }

// Tree is the extraction directory, as the filesystem it was opened over.
//
// It exists for the drift sweep, which is the one consumer that must see what
// the manifest does NOT list: an unlisted file is how an artifact gets read that
// nothing vouches for, and no manifest-keyed accessor can reach one by
// definition. Reads through it are confined to the directory — an fs.FS rejects
// an absolute path and a "..", so a hostile manifest key cannot address the
// filesystem at large.
func (d *Doc) Tree() fs.FS { return d.fsys }
