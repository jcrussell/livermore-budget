package registry

import (
	"fmt"
	"io/fs"
	"regexp"
)

// SourcesFile is the source registry, read from the root of the same
// filesystem [Load] reads the vocabulary from.
//
// It is loaded separately, by [LoadSources], and deliberately not by Load. The
// two files answer to different consumers: funds.yaml and taxonomy.yaml are
// the vocabulary a fact's fields resolve in, and every command that touches a
// fact needs both, while this file is the provenance registry — the only place
// a URL, a retrieval date or a source hash is asserted. `fisc export` reads it
// for its citation targets and `fisc verify` reads it for the hashes; nothing
// that classifies a fact reads it at all.
const SourcesFile = "sources.yaml"

// SourcesSchemaVersion is the only sources.yaml version this package reads. It
// versions independently of the other two: a change to the source registry's
// shape says nothing about the fund schema.
const SourcesSchemaVersion = 1

// Source is one entry in sources.yaml: a document this project reads, and the
// bytes it was read from.
//
// Every field the file carries is declared, because the decoder rejects an
// unknown key (see decodeFile). A key this struct forgot would be a load
// failure rather than a silently dropped claim, which is the right way round
// for a file whose whole purpose is to be the authority on where the data came
// from.
type Source struct {
	// ID is the document id, and it is the join key: the extraction directory
	// under data/extracted/ is named for it, and every mapping rule and every
	// fact names it as doc_id.
	ID string `yaml:"id"`
	// Title and Publisher are the city's words for the document, published as
	// the citation on the site.
	Title     string `yaml:"title"`
	Publisher string `yaml:"publisher"`
	// FiscalYears is every year the document carries figures for, and
	// AppropriatedFiscalYears the subset that was actually appropriated — the
	// CIP's tables run to FY2030 as planning intent (see the file's own note).
	FiscalYears             []int `yaml:"fiscal_years"`
	AppropriatedFiscalYears []int `yaml:"appropriated_fiscal_years"`
	// Basis is adopted or audited. It is not validated here against
	// mapping.Basis: this package would then have to import the mapping engine
	// to read a registry, and the basis a fact carries comes from its rule.
	Basis string `yaml:"basis"`
	// DocumentID is the city CMS's numeric id, which both URLs are built from.
	DocumentID int `yaml:"document_id"`
	// URL is the stable canonical address the site links to; URLVersioned
	// carries the CMS cache-buster and is the exact address the committed bytes
	// were retrieved from. Both are recorded because the second changes
	// whenever the city re-uploads.
	URL          string `yaml:"url"`
	URLVersioned string `yaml:"url_versioned"`
	// Retrieved is the date the bytes were fetched, as written.
	Retrieved string `yaml:"retrieved"`
	// File is where the document lives in this repository, relative to its
	// root. It is the registry's own claim about the path rather than one
	// composed from ID, so a document stored under another name is still
	// addressable — and it is read through an fs.FS rooted at the repository,
	// so a "../" in it cannot address anything outside the tree.
	File string `yaml:"file"`
	// Bytes and SHA256 are the size and hash of that file. They are what makes
	// `fisc verify --full` a check rather than a formality, and they are
	// recorded here INDEPENDENTLY of the same two numbers in each extraction
	// manifest: tools/extract.py computes its own and has no YAML parser, so
	// neither copy is derived from the other.
	Bytes  int64  `yaml:"bytes"`
	SHA256 string `yaml:"sha256"`
	// Pages is the document's page count, as the extractor found it.
	Pages int `yaml:"pages"`
}

// sourcesDoc is the whole of sources.yaml.
type sourcesDoc struct {
	SchemaVersion int      `yaml:"schema_version"`
	Sources       []Source `yaml:"sources"`
}

// sha256Hex is a lower-case hex sha256, the form both this file and every
// extraction manifest record. Upper case is not accepted rather than folded:
// the two records are compared for equality by everything that reads them, and
// a registry that normalized while extract.py did not would make agreement
// depend on which side did the comparing.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LoadSources reads and validates the source registry.
//
// It validates that each entry is well formed and self-consistent — an id, a
// file, a plausible hash and size — and nothing more. Whether those numbers
// agree with the bytes on disk, or with what the extractor recorded, is
// `fisc verify`'s question: a disagreement there is a finding that names the
// three parties, not a file this package refuses to parse.
func LoadSources(fsys fs.FS) ([]Source, error) {
	var doc sourcesDoc
	if err := decodeFile(fsys, SourcesFile, &doc); err != nil {
		return nil, err
	}

	errf := func(entry, field, format string, args ...any) error {
		return &Error{File: SourcesFile, Entry: entry, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}
	if doc.SchemaVersion != SourcesSchemaVersion {
		return nil, schemaVersionErr(SourcesFile, doc.SchemaVersion, SourcesSchemaVersion)
	}
	if len(doc.Sources) == 0 {
		return nil, errf("", "sources", "is empty")
	}

	seen := make(map[string]bool, len(doc.Sources))
	for i, s := range doc.Sources {
		// The id names the extraction directory and is the doc_id every fact
		// carries, so an entry without one is not addressable at all.
		if s.ID == "" {
			return nil, errf(fmt.Sprintf("sources[%d]", i), "id",
				"is required (title %q)", s.Title)
		}
		entry := fmt.Sprintf("source %q", s.ID)
		if seen[s.ID] {
			return nil, errf(entry, "id", "duplicate document id")
		}
		seen[s.ID] = true
		if s.File == "" {
			return nil, errf(entry, "file", "is required")
		}
		if !fs.ValidPath(s.File) {
			return nil, errf(entry, "file",
				"%q is not a repository-relative slash-separated path", s.File)
		}
		if !sha256Hex.MatchString(s.SHA256) {
			return nil, errf(entry, "sha256",
				"%q is not a lower-case hex sha256", s.SHA256)
		}
		if s.Bytes <= 0 {
			return nil, errf(entry, "bytes", "is %d; the size of the retrieved file is "+
				"half of what identifies it", s.Bytes)
		}
		// The page count is the only independent witness to how much of a document
		// was extracted: `fisc verify` compares it against the artifacts the
		// manifest lists, which are otherwise checkable only against themselves. A
		// zero here would make that comparison vacuous, and an emptied extraction
		// could be laundered by zeroing both.
		if s.Pages <= 0 {
			return nil, errf(entry, "pages", "is %d; it is what says how much of the "+
				"document an extraction should contain", s.Pages)
		}
	}
	return doc.Sources, nil
}
