package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"

	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// The page templates inside the site asset tree, one per view.
//
// They are named here rather than in the caller because a template is an ASSET
// of this package's embedded tree, and a caller naming one would be reaching
// into a directory it does not own. What the caller chooses is which view uses
// which, by name.
const (
	SankeyTemplate = "index.html.tmpl"
	TrendsTemplate = "trends.html.tmpl"
	// HistoryTemplate is a server-rendered series table like TrendsTemplate,
	// with the rows grouped under the printed block headings a [View.Sections]
	// declares. It ships no script beyond the theme stamp.
	HistoryTemplate = "history.html.tmpl"
	// ProvenanceTemplate renders the fact store's index, and CaveatsTemplate
	// every document's caveats in one place. They render no projection
	// document (templateRendersADocument).
	ProvenanceTemplate = "provenance.html.tmpl"
	CaveatsTemplate    = "caveats.html.tmpl"
)

// SchemaVersion is the projection schema this packager understands, the
// producer's.
const SchemaVersion = project.SchemaVersion

// ErrSchemaVersion reports a projection whose schema this packager does not
// understand. It is always wrapped with the versions involved, so callers
// match it with errors.Is rather than by string.
var ErrSchemaVersion = errors.New("unsupported schema_version")

// checkSchemaVersion refuses a projection this packager cannot read.
//
// A version this binary does not know is not a malformed document — nothing is
// wrong with the file — so the two directions get the hint that says which way
// the mismatch runs, as registry.schemaVersionErr and mapping.File.validate
// already do for their own files. Rendering it anyway is the failure worth
// preventing: a schema bump changes what the graph MEANS, so an old packager
// handed a new projection would publish a page that is wrong rather than one
// that fails.
func checkSchemaVersion(stem string, got int) error {
	if got == SchemaVersion {
		return nil
	}
	// The refusal names the document it read, not the primary one.
	err := fmt.Errorf("%s projection: %w: got %d, want %d",
		stem, ErrSchemaVersion, got, SchemaVersion)
	switch {
	case got > SchemaVersion:
		return hint.With(err,
			"this projection was written by a newer fisc; upgrade the binary")
	case got == 0:
		return hint.With(err,
			"schema_version is absent or zero; this may not be a fisc projection")
	default:
		return err
	}
}

// projectionDoc is as much of a projection document as the packager reads.
// Everything else — nodes, links — is bulk the browser fetches, and decoding
// it here would be a second parser for a contract that already has one.
//
// Metadata is kept as raw JSON and decoded by each reader for what it needs.
type projectionDoc struct {
	SchemaVersion int             `json:"schema_version"`
	Projection    string          `json:"projection"`
	Metadata      json.RawMessage `json:"metadata"`
}

// decodeDocument reads the envelope of ANY projection document and refuses it
// before anything is read out of it, because every field a view names belongs to
// a contract a version this packager does not know may have renamed.
//
// What it checks is what every document has: a schema version this binary
// understands and a metadata block. Anything shape-specific is the caller's,
// below.
func decodeDocument(stem string, raw []byte) (projectionDoc, error) {
	var doc projectionDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, fmt.Errorf("decode %s projection: %w", stem, err)
	}
	if err := checkSchemaVersion(stem, doc.SchemaVersion); err != nil {
		return doc, err
	}
	if len(doc.Metadata) == 0 {
		return doc, fmt.Errorf("%s projection has no metadata block", stem)
	}
	return doc, nil
}

// sitePage is one rendered view: where it goes and what it says.
type sitePage struct {
	Path string
	HTML []byte
}

// navItem is one view as every other view lists it.
type navItem struct {
	Label   string
	Path    string
	Current bool
}

// buildSite renders every view and returns the pages plus the union of the
// citations they made.
//
// THE CITATION SET IS THE UNION AND THE FOOTERS ARE NOT: the shipped page text
// is unioned across every view and year, while a footer lists its own view's
// sources. The one exception is a view's own years, unioned because app.js
// drops a citation whose doc_id is missing from CONFIG.docs in silence, and
// the heading names the years.
//
// pageTextBase resolves a doc id to the directory the page text is cited from,
// with its trailing slash. A FUNCTION AND NOT A URL because the caller, not
// this file, decides between a remote browse view and the copy the site ships
// -- see Write.
func buildSite(o *Options, ix ColumnIndex, pageTextBase func(docID string) string) ([]sitePage, []Citation, error) {
	views := o.views()
	nav := make([]navItem, 0, len(views))
	for _, v := range views {
		label := v.Nav
		if label == "" {
			label = v.Title
		}
		nav = append(nav, navItem{Label: label, Path: v.Path})
	}

	byID := make(map[string]Doc, len(o.Docs))
	for _, d := range o.Docs {
		byID[d.ID] = d
	}

	var cited []Citation
	seen := map[Citation]bool{}
	collect := func(stem string) error {
		// A view with no projection cites nothing through this path.
		if stem == "" {
			return nil
		}
		// Deduplicated: shipping one page twice is a write collision.
		cs, err := citationsOf(stem, o.Projections[stem])
		if err != nil {
			return err
		}
		for _, c := range cs {
			if !seen[c] {
				seen[c] = true
				cited = append(cited, c)
			}
		}
		return nil
	}

	pages := make([]sitePage, 0, len(views))
	for i, v := range views {
		// Every document this view renders contributes citations: the one it
		// opens on and every year of it.
		if err := collect(v.Projection); err != nil {
			return nil, nil, err
		}
		for _, stem := range v.YearStems {
			if stem == v.Projection {
				continue
			}
			if err := collect(stem); err != nil {
				return nil, nil, err
			}
		}

		here := make([]navItem, len(nav))
		copy(here, nav)
		here[i].Current = true

		var (
			data any
			err  error
		)
		// Every template is an explicit arm and the unknown one is refused: a
		// template handed data it does not read renders blanks in silence.
		switch v.Template {
		case TrendsTemplate:
			data, err = buildTrendsPage(o, v, here, byID, ix, pageTextBase)
		case HistoryTemplate:
			data, err = buildHistoryPage(o, v, here, byID, ix, pageTextBase)
		case SankeyTemplate:
			data, err = buildSankeyPage(o, v, here, byID, ix, pageTextBase)
		case ProvenanceTemplate:
			data, err = buildProvenancePage(o, v, here, byID, ix, pageTextBase)
		case CaveatsTemplate:
			data, err = buildCaveatsPage(o, v, here, byID, ix, pageTextBase)
		default:
			return nil, nil, hint.With(
				fmt.Errorf("view %q renders template %q, which this package has no builder for",
					v.Path, v.Template),
				"every template needs an arm in buildSite naming the page data it is built from")
		}
		if err != nil {
			return nil, nil, err
		}
		html, err := renderPage(o.assetTree(), v.Template, data)
		if err != nil {
			return nil, nil, err
		}
		pages = append(pages, sitePage{Path: v.Path, HTML: html})
	}

	// Every published document ships its pages, not only every viewed one.
	// Collected after the view loop, so this only ever appends.
	for _, stem := range sortedKeys(o.Projections) {
		if err := collect(stem); err != nil {
			return nil, nil, err
		}
	}

	// And every page the index publishes is cited, so the published store
	// ships its page text whatever the charts draw, under withPageText's
	// refusal of a page the extraction tree lacks.
	for _, e := range o.PageIndex {
		if !seen[e.Citation] {
			seen[e.Citation] = true
			cited = append(cited, e.Citation)
		}
	}
	return pages, cited, nil
}

// chrome is what every view renders whatever its document is.
type chrome struct {
	Title        string
	Lede         string
	Nav          []navItem
	Sources      []sourceRef
	ProjectionBy string
	ExportedBy   string
	Projections  []projectionRef
	DataPath     string
	Scope        string
	Caveats      []caveatRef
	// CaveatsPath is the caveats page, or "" when the site has none. Separate
	// from each ref's Href because the templates use it for a "read them all"
	// link that belongs to no single caveat.
	CaveatsPath string
}

// renderPage executes one view's template against its assembled data.
func renderPage(assets fs.FS, name string, data any) ([]byte, error) {
	tmpl, err := template.New(name).ParseFS(assets, name)
	if err != nil {
		return nil, fmt.Errorf("parse page template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %q: %w", name, err)
	}
	return buf.Bytes(), nil
}
