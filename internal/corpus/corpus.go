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
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

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

// manifest is the on-disk manifest.json. Unknown fields are tolerated rather
// than rejected: extract.py may add reporting keys, and SchemaVersion is the
// guard that matters for the fields this package actually reads.
type manifest struct {
	SchemaVersion    int                 `json:"schema_version"`
	DocID            string              `json:"doc_id"`
	PageCount        int                 `json:"page_count"`
	ExtractorVersion int                 `json:"extractor_version"`
	SourceFile       string              `json:"source_file"`
	SourceSHA256     string              `json:"source_sha256"`
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
	b, err := fs.ReadFile(fsys, "manifest.json")
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

// extractedDir is where tools/extract.py writes, relative to the repository
// root. It is spelled once, here, because a command that joined its own copy
// of the path would drift from the reader that has to find the result.
const extractedDir = "data/extracted"

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
			"doc_id names one directory under "+extractedDir+
				"; it may not contain a path separator or \"..\"")
	}
	dir := filepath.Join(root, extractedDir, docID)
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

// PageCount is how many pages the source PDF had.
func (d *Doc) PageCount() int { return d.man.PageCount }

// SourceSHA256 is the hash of the PDF this extraction was made from, as
// recorded by extract.py. `fisc verify` cross-checks it against the registry;
// the two are recorded independently on purpose, so agreement is evidence.
func (d *Doc) SourceSHA256() string { return d.man.SourceSHA256 }

// PagePath is the artifact path for page n.
//
// .txt, not .md: GitHub renders markdown in its blob view and collapses the
// runs of spaces that ARE the column grid, which would silently break the
// provenance deep links these paths are published as.
func PagePath(n int) string { return fmt.Sprintf("pages/p%04d.txt", n) }

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

// Artifact returns the manifest's record for an artifact path.
func (d *Doc) Artifact(p string) (Artifact, bool) {
	a, ok := d.man.Artifacts[p]
	return a, ok
}

// ArtifactPaths returns every artifact path the manifest lists, sorted. This
// is the input to the drift sweep in fisc-1wr.5; the hashes to compare against
// come from Artifact.
func (d *Doc) ArtifactPaths() []string {
	out := make([]string, 0, len(d.man.Artifacts))
	for p := range d.man.Artifacts {
		out = append(out, path.Clean(p))
	}
	sort.Strings(out)
	return out
}
