package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// The page templates inside the site asset tree, one per view. They are this
// package's assets, so the caller picks one by name rather than by path.
const (
	SankeyTemplate = "flows.html.tmpl"
	// HistoryTemplate is a server-rendered series table grouped under a
	// [View.Sections]'s printed headings; it loads no client module.
	HistoryTemplate = "history.html.tmpl"
	// ProvenanceTemplate and CaveatsTemplate render no projection document
	// (templateRendersADocument).
	ProvenanceTemplate = "provenance.html.tmpl"
	CaveatsTemplate    = "caveats.html.tmpl"
	// DepartmentTemplate, CapitalTemplate and DebtTemplate each load their
	// own client module.
	DepartmentTemplate = "department.html.tmpl"
	CapitalTemplate    = "capital.html.tmpl"
	DebtTemplate       = "debt.html.tmpl"
	// GlanceTemplate renders tables of printed cells that site/glance.js
	// groups and sums.
	GlanceTemplate = "glance.html.tmpl"
	// LedgerTemplate loads site/ledger.js over site/sides.js.
	LedgerTemplate = "ledger.html.tmpl"
	// ChangeTemplate renders tables of printed points that site/change.js
	// stacks by category.
	ChangeTemplate = "change.html.tmpl"
)

// SchemaVersion is the projection schema this packager understands, the
// producer's.
const SchemaVersion = project.SchemaVersion

// ErrSchemaVersion reports a projection whose schema this packager does not
// understand. It is always wrapped with the versions involved, so callers
// match it with errors.Is rather than by string.
var ErrSchemaVersion = errors.New("unsupported schema_version")

// checkSchemaVersion refuses a projection this packager cannot read, with a
// hint saying which way the mismatch runs. A schema bump changes what the
// graph means, so rendering it anyway would publish a wrong page.
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

// projectionDoc is as much of a projection document as the packager reads;
// nodes and links are the browser's. Metadata is decoded by each reader.
type projectionDoc struct {
	SchemaVersion int             `json:"schema_version"`
	Projection    string          `json:"projection"`
	Metadata      json.RawMessage `json:"metadata"`
}

// decodeDocument reads any projection document's envelope and refuses an
// unknown schema version or a missing metadata block before any field is read.
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

// decodeWhole reads a projection document's envelope (decodeDocument) and
// then the whole of it into T.
func decodeWhole[T any](stem string, raw []byte) (T, error) {
	var d T
	if _, err := decodeDocument(stem, raw); err != nil {
		return d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("decode %s: %w", stem, err)
	}
	return d, nil
}

// decodeProjection is decodeWhole for one schedule a page draws, refusing one
// another projection built.
func decodeProjection[T any](page, stem, want string, raw []byte) (T, error) {
	d, err := decodeWhole[T](stem, raw)
	if err != nil {
		return d, err
	}
	if env, _ := decodeDocument(stem, raw); env.Projection != want {
		return d, fmt.Errorf("the %s page reads %s as %s and it is %q", page, stem, want, env.Projection)
	}
	return d, nil
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
	// years is the view's year select's keys, shared by every page's copy of
	// the item and filled once every view is built, so a link knows the years
	// its view opens before any page renders.
	years *[]string
}

// YearKeys is the years the view's top bar offers, space-separated, or ""
// for a view that draws every year at once. A page's year select carries its
// year onto the links whose view offers it, and onto no other.
func (n navItem) YearKeys() string {
	if n.years == nil {
		return ""
	}
	return strings.Join(*n.years, " ")
}

// yearSelect is the year control under the top bar, on a page that draws one year at
// a time. Each option's Key is a [YearKey]; "" is every year at once.
type yearSelect struct {
	Options []yearOption
}

// yearOption is one year the select offers.
type yearOption struct {
	Key      string
	Label    string
	Selected bool
}

// yearOptionOf is one column as every page's year select offers it, so a
// [YearKey] reads alike on every page that offers it.
func yearOptionOf(year int, fiscalYearLabel, basis string) yearOption {
	return yearOption{Key: YearKey(year, basis), Label: ledeOf(fiscalYearLabel, basis)}
}

// newest appends one column and opens on it: a page lists its columns
// oldest first and opens on the newest.
func (y *yearSelect) newest(year int, fiscalYearLabel, basis string) {
	for i := range y.Options {
		y.Options[i].Selected = false
	}
	o := yearOptionOf(year, fiscalYearLabel, basis)
	o.Selected = true
	y.Options = append(y.Options, o)
}

// keys is the options' column keys, every-year left out.
func (y yearSelect) keys() []string {
	var out []string
	for _, o := range y.Options {
		if o.Key != "" {
			out = append(out, o.Key)
		}
	}
	return out
}

// buildSite renders every view and returns the pages plus the union of the
// citations they made. The shipped page text is that union; a footer lists
// only its own view's sources, unioned across its years because app.js drops
// a citation whose doc_id is missing from CONFIG.docs in silence.
//
// pageTextBase resolves a doc id to its page-text directory, with a trailing
// slash; Write decides between a remote view and the shipped copy.
func buildSite(o *Options, ix ColumnIndex, pageTextBase func(docID string) string) ([]sitePage, []Citation, error) {
	views := o.Views
	nav := make([]navItem, 0, len(views))
	keys := make([][]string, len(views))
	for i, v := range views {
		label := v.Nav
		if label == "" {
			label = v.Title
		}
		nav = append(nav, navItem{Label: label, Path: v.Path, years: &keys[i]})
	}

	cx, err := buildCaveatIndex(o, ix, nav)
	if err != nil {
		return nil, nil, err
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

	built := make([]any, 0, len(views))
	for i, v := range views {
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

		// Every template has a builder and the unknown one is refused: a
		// template handed data it does not read renders blanks in silence.
		spec, ok := templates[v.Template]
		if !ok {
			return nil, nil, hint.With(
				fmt.Errorf("view %q renders template %q, which this package has no builder for",
					v.Path, v.Template),
				"every template needs an entry in templates naming the page data it is built from")
		}
		data, err := spec.build(pageCtx{o: o, v: v, nav: here, byID: byID, ix: ix, pageTextBase: pageTextBase, cx: cx})
		if err != nil {
			return nil, nil, err
		}
		c, ok := data.(chromed)
		if !ok {
			return nil, nil, fmt.Errorf("view %q builds %T, which embeds no chrome", v.Path, data)
		}
		keys[i] = c.yearSelect().keys()
		built = append(built, data)
	}
	// Rendered only once every view is built, so each nav link carries the
	// years its own view offers.
	pages := make([]sitePage, 0, len(views))
	for i, v := range views {
		html, err := renderPage(o.assetTree(), v.Template, built[i])
		if err != nil {
			return nil, nil, err
		}
		pages = append(pages, sitePage{Path: v.Path, HTML: html})
	}

	// Every published document ships its pages, not only every viewed one.
	// Collected after the view loop, so this only ever appends.
	for _, stem := range slices.Sorted(maps.Keys(o.Projections)) {
		if err := collect(stem); err != nil {
			return nil, nil, err
		}
	}

	// Every page the index publishes ships its page text whatever the charts
	// draw.
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
	Title       string
	Lede        string
	Nav         []navItem
	Sources     []sourceRef
	ExportedBy  string
	Projections []projectionRef
	DataPath    string
	Scope       string
	CaveatsHere caveatsHere
	// SourcesPath is the provenance index, or "" when the site has none. With
	// one, the footer links there and lists no cited page.
	SourcesPath string
	// SourcesPage is set on the provenance index itself.
	SourcesPage bool
	// YearSelect is empty on a page that draws every year at once.
	YearSelect yearSelect
}

// pageCtx is what every page builder reads.
type pageCtx struct {
	o            *Options
	v            View
	nav          []navItem
	byID         map[string]Doc
	ix           ColumnIndex
	pageTextBase func(string) string
	cx           *caveatIndex
}

// chrome fills c with what every page citing figures renders alike, and
// returns the documents the page's client reads.
func (p pageCtx) chrome(c chrome, cited []sourceMeta, cells *cellCites) (chrome, map[string]clientDoc, error) {
	sources, sourcesPath, docs, err := citedSources(p.o, p.v.Path, cited, cells, p.byID, p.pageTextBase)
	if err != nil {
		return chrome{}, nil, err
	}
	c.Lede = p.v.Lede
	c.Nav = p.nav
	c.Sources = sources
	c.SourcesPath = sourcesPath
	c.ExportedBy = p.o.GeneratedBy
	c.Projections = projectionRefs(p.o, p.ix)
	c.CaveatsHere = p.cx.here(p.v.Path)
	return c, docs, nil
}

// chromed is every page's data, whose top bar is the embedded chrome's.
type chromed interface{ yearSelect() yearSelect }

func (c chrome) yearSelect() yearSelect { return c.YearSelect }

// Repository is the source repository the "footer" template links.
func (chrome) Repository() string { return RepositoryURL }

// SourcesPartial defines the footer's "sources" list on a site with no
// sources page. renderPage parses it BEFORE the page, so a page's own
// "sources-heading" replaces the partial's default.
const SourcesPartial = "sources.html.tmpl"

// ChromePartial defines the "theme-stamp" head script, the "topbar" nav and
// the "footer" every page template executes.
const ChromePartial = "chrome.html.tmpl"

// renderPage executes one view's template against its assembled data.
func renderPage(assets fs.FS, name string, data any) ([]byte, error) {
	tmpl, err := template.New(name).ParseFS(assets, ChromePartial, SourcesPartial, name)
	if err != nil {
		return nil, fmt.Errorf("parse page template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %q: %w", name, err)
	}
	return buf.Bytes(), nil
}
