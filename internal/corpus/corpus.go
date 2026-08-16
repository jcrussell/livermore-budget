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
// eleven files drifted", which is the useful answer. Note also that the
// manifest's per-artifact sha256 is a different hash from a mapping rule's
// expected_content_hash — one says the extracted file changed, the other says
// the rows a rule was written against changed.
package corpus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
)

// SchemaVersion is the only manifest version this package understands.
// tools/extract.py stamps it; a bump there means the artifact contract changed
// and the reader must be revisited rather than guess.
const SchemaVersion = 1

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
	SchemaVersion     int                 `json:"schema_version"`
	DocID             string              `json:"doc_id"`
	PageCount         int                 `json:"page_count"`
	TableCount        int                 `json:"table_count"`
	ExtractorVersion  int                 `json:"extractor_version"`
	NormalizerVersion int                 `json:"normalizer_version"`
	SourceFile        string              `json:"source_file"`
	SourceSHA256      string              `json:"source_sha256"`
	Artifacts         map[string]Artifact `json:"artifacts"`
}

// Table is one detected table, mirroring what tools/extract.py serializes.
// The field set is deliberately the whole record: a reader that dropped, say,
// `ragged` would make a caller unable to tell a clean grid from a salvaged one.
type Table struct {
	SchemaVersion    int        `json:"schema_version"`
	DocID            string     `json:"doc_id"`
	Page             int        `json:"page"`
	Ordinal          int        `json:"ordinal"`
	BBox             []float64  `json:"bbox"`
	NRows            int        `json:"n_rows"`
	NCols            int        `json:"n_cols"`
	Ragged           bool       `json:"ragged"`
	Cells            [][]string `json:"cells"`
	RowPageLines     []*int     `json:"row_page_lines"`
	LabelFingerprint string     `json:"label_fingerprint"`
	ContentHash      string     `json:"content_hash"`

	// Path is the artifact this was read from, for error messages.
	Path string `json:"-"`
}

// Centroid returns the midpoint of the table's bounding box. It is the
// positional half of a table locator: a table that keeps its fingerprint but
// moves across the page is not the same table.
//
// The second result is false when the extractor recorded no bbox, which it
// does for a table whose geometry xberg could not report.
func (t *Table) Centroid() (x, y float64, ok bool) {
	if len(t.BBox) != 4 {
		return 0, 0, false
	}
	return (t.BBox[0] + t.BBox[2]) / 2, (t.BBox[1] + t.BBox[3]) / 2, true
}

// FirstRow returns the table's first row, for candidate lists in error
// messages. It returns nil for an empty table rather than panicking, because
// the callers are error paths and a panic there would replace a useful
// diagnosis with a stack trace.
func (t *Table) FirstRow() []string {
	if len(t.Cells) == 0 {
		return nil
	}
	return t.Cells[0]
}

// Doc is one extracted document: its manifest, and lazy access to the pages
// and tables it lists.
type Doc struct {
	fsys fs.FS
	man  manifest

	// tables indexes the manifest's table artifacts by page, in ordinal order,
	// so TablesOn does not have to probe for filenames that may not exist.
	tables map[int][]string
}

var tableArtifact = regexp.MustCompile(`^tables/p(\d{4})-t(\d{2})\.json$`)

// Open reads the manifest at the root of fsys and indexes its artifacts.
// It does not read any page or table.
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

	d := &Doc{fsys: fsys, man: m, tables: map[int][]string{}}
	for p := range m.Artifacts {
		match := tableArtifact.FindStringSubmatch(p)
		if match == nil {
			continue
		}
		// The regexp guarantees four digits, so this cannot fail.
		page, _ := strconv.Atoi(match[1])
		d.tables[page] = append(d.tables[page], p)
	}
	// Filenames are zero-padded, so lexical order is ordinal order.
	for page := range d.tables {
		sort.Strings(d.tables[page])
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
func PagePath(n int) string { return fmt.Sprintf("pages/p%04d.md", n) }

// Page returns the extracted markdown for page n.
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

// TablesOn returns every table detected on page n, in ordinal order. A page
// with no tables returns an empty slice and no error: most pages in this
// corpus have none, and 46% of the Budget Book's money-bearing pages yield no
// table at all, so absence is normal rather than exceptional.
func (d *Doc) TablesOn(n int) ([]*Table, error) {
	paths := d.tables[n]
	out := make([]*Table, 0, len(paths))
	for _, p := range paths {
		t, err := d.table(p)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (d *Doc) table(p string) (*Table, error) {
	b, err := fs.ReadFile(d.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var t Table
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if t.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%s: table schema_version %d, want %d",
			p, t.SchemaVersion, SchemaVersion)
	}
	t.Path = p
	return &t, nil
}

// TablePages returns every page carrying at least one table, ascending. It
// exists so a caller that must scan the whole document — building a candidate
// list for a locator that did not resolve — does not have to walk page numbers
// the manifest never listed.
func (d *Doc) TablePages() []int {
	pages := make([]int, 0, len(d.tables))
	for p := range d.tables {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	return pages
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
