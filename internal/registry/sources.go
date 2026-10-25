package registry

import (
	"fmt"
	"io/fs"
	"regexp"
)

// SourcesFile is the source registry, the only place a URL, a retrieval date
// or a source hash is asserted. It is read by [LoadSources], not by [Load]:
// nothing that classifies a fact reads it.
const SourcesFile = "sources.yaml"

// sourcesSchemaVersion is the only sources.yaml version this package reads,
// versioned independently of the vocabulary files.
const sourcesSchemaVersion = 1

// Source is one entry in sources.yaml: a document this project reads, and the
// bytes it was read from. Every key the file carries is declared, because the
// decoder rejects an unknown key.
type Source struct {
	// ID is the join key: the data/extracted/ directory name and every fact's
	// doc_id.
	ID string `yaml:"id"`
	// Title and Publisher are the city's words, published as the citation.
	Title     string `yaml:"title"`
	Publisher string `yaml:"publisher"`
	// AppropriatedFiscalYears is the subset of FiscalYears actually
	// appropriated; the CIP's tables run to FY2030 as planning intent.
	FiscalYears             []int `yaml:"fiscal_years"`
	AppropriatedFiscalYears []int `yaml:"appropriated_fiscal_years"`
	// Basis is not validated against vocab.Bases: a fact's basis comes from
	// its rule, never from here.
	Basis string `yaml:"basis"`
	// DocumentID is the city CMS's numeric id both URLs are built from.
	DocumentID int `yaml:"document_id"`
	// URL is the stable address the site links to; URLVersioned carries the
	// CMS cache-buster and is where the committed bytes were retrieved from.
	URL          string `yaml:"url"`
	URLVersioned string `yaml:"url_versioned"`
	Retrieved    string `yaml:"retrieved"`
	// File is the repository-relative path, not composed from ID; it is read
	// through an fs.FS rooted at the repository, so "../" escapes nothing.
	File string `yaml:"file"`
	// Bytes and SHA256 are recorded independently of the extraction manifest's
	// copies (tools/extract.py has no YAML parser), so `fisc verify --full`
	// compares two parties.
	Bytes  int64  `yaml:"bytes"`
	SHA256 string `yaml:"sha256"`
	Pages  int    `yaml:"pages"`
}

// sourcesDoc is the whole of sources.yaml.
type sourcesDoc struct {
	SchemaVersion int      `yaml:"schema_version"`
	Sources       []Source `yaml:"sources"`
}

// sha256Hex is lower-case hex. Upper case is refused, not folded: the registry
// and the manifests are compared for equality, and extract.py does not fold.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LoadSources reads and validates the source registry: each entry well formed
// and self-consistent. Agreement with the bytes on disk or the manifest is
// `fisc verify`'s question.
func LoadSources(fsys fs.FS) ([]Source, error) {
	var doc sourcesDoc
	if err := decodeFile(fsys, SourcesFile, &doc); err != nil {
		return nil, err
	}

	errf := func(entry, field, format string, args ...any) error {
		return &Error{File: SourcesFile, Entry: entry, Field: field,
			Msg: fmt.Sprintf(format, args...)}
	}
	if doc.SchemaVersion != sourcesSchemaVersion {
		return nil, schemaVersionErr(SourcesFile, doc.SchemaVersion, sourcesSchemaVersion)
	}
	if len(doc.Sources) == 0 {
		return nil, errf("", "sources", "is empty")
	}

	seen := make(map[string]bool, len(doc.Sources))
	for i, s := range doc.Sources {
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
		// Pages is the only independent witness to how much was extracted; a
		// zero would let an emptied extraction pass by zeroing both.
		if s.Pages <= 0 {
			return nil, errf(entry, "pages", "is %d; it is what says how much of the "+
				"document an extraction should contain", s.Pages)
		}
	}
	return doc.Sources, nil
}
