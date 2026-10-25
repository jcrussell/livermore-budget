package export

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// projectionMetadata is the decoded metadata block of a SANKEY document,
// rendered server-side so the headline survives without JavaScript. Only the
// spine carries a headline (decodeSankey).
type projectionMetadata struct {
	GeneratedBy     string           `json:"generated_by"`
	FiscalYear      int              `json:"fiscal_year"`
	FiscalYearLabel string           `json:"fiscal_year_label"`
	Basis           string           `json:"basis"`
	Scopes          []string         `json:"scopes"`
	Currency        string           `json:"currency"`
	Units           string           `json:"units"`
	Sources         []sourceMeta     `json:"sources"`
	Headline        project.Headline `json:"headline"`
	Counts          struct {
		Facts int `json:"facts"`
		Nodes int `json:"nodes"`
		Links int `json:"links"`
	} `json:"counts"`
	Caveats []caveatMeta `json:"caveats"`
}

// Scope is the scopes joined, in the document's own order.
func (m projectionMetadata) Scope() string { return strings.Join(m.Scopes, ", ") }

// figure is one headline figure. The JSON tags are load-bearing: a year
// switch paints the hero from window.FISC_CONFIG, and the template would not
// notice a missing tag.
type figure struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note"`
	// Kind is a class name: "hero" for the headline, "" for the rest.
	Kind string `json:"kind"`
}

// yearView is one published year's worth of everything the page states in
// words, built in Go for every year; the client only chooses between them.
type yearView struct {
	Year  int    `json:"year"`
	Label string `json:"label"`
	Stem  string `json:"stem"`
	Path  string `json:"path"`
	Basis string `json:"basis"`
	// Title is this year's <title> (sankeyTitle).
	Title   string      `json:"title"`
	Hero    figure      `json:"hero"`
	Caveats []caveatRef `json:"caveats"`
	Counts  countsRef   `json:"counts"`
	// ChartTitle is the SVG's <title>, the chart's accessible name.
	ChartTitle string `json:"chart_title"`
	// Lede is "FY 2025-26 adopted"; app.js repaints it and names a column by
	// it.
	Lede string `json:"lede"`
}

// countsRef is the "N flows between M nodes, from K facts" line, per year.
type countsRef struct {
	Facts int `json:"facts"`
	Nodes int `json:"nodes"`
	Links int `json:"links"`
}

// pageData is the Sankey template's input. Lede, ChartTitle and Hero are the
// opening year's.
type pageData struct {
	chrome
	Lede  string
	Basis string
	// Wording is the client's words, which the template's legend shares.
	Wording    wording
	ChartTitle string
	Hero       figure
	// Years is every published year, oldest first; the page opens on the
	// newest.
	Years       []yearView
	Facts       int
	Nodes       int
	Links       int
	Description string
	// Narrow is where a narrow screen is pointed in place of the chart; its
	// Path is "" on a site with none.
	Narrow navItem
	// ConfigJSON is window.FISC_CONFIG. json.Marshal escapes <, > and & to
	// their \u form, so the blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// ledeOf is a column named the way the lede names it.
func ledeOf(label, basis string) string { return label + " " + basis }

// PageConfigVersion is the version window.FISC_CONFIG declares, and
// site/core.js's SCHEMA_VERSION must equal it. It versions the page as
// site/app.js reads it, so a script cached across a page change asks for a
// reload rather than throwing.
const PageConfigVersion = 2

// clientConfig is window.FISC_CONFIG: the metadata the page needs before it
// has fetched anything, plus where to fetch the bulk from.
type clientConfig struct {
	SchemaVersion int `json:"schema_version"`
	// ProjectionVersion is project.SchemaVersion; site/core.js refuses one
	// not its PROJECTION_VERSION.
	ProjectionVersion int    `json:"projection_version"`
	ExportedBy        string `json:"exported_by"`
	Primary           string `json:"primary"`
	// Years carries each year's words, so the client never composes a figure
	// or a caveat itself.
	Years      []yearView           `json:"years"`
	Docs       map[string]clientDoc `json:"docs"`
	KindLabels map[string]string    `json:"kind_labels"`
	// SplitLabels and MemberNouns name a split key's members and one member,
	// keyed as the expansion schedule names a split.
	SplitLabels map[string]string `json:"split_labels"`
	MemberNouns map[string]string `json:"member_nouns"`
	Wording     wording           `json:"wording"`
	// Overview is the page's own chart: its form and that form's hints.
	Overview Chart `json:"overview"`
}

// encodeConfig renders window.FISC_CONFIG and refuses bytes that do not match
// the published schema, which catches a missing JSON tag the template cannot.
func encodeConfig(cfg clientConfig) ([]byte, error) {
	blob, err := json.Marshal(cfg)
	return schemaChecked(blob, err, schema.Page, fmt.Sprintf("page config for %q", cfg.Primary))
}

// schemaChecked holds blob, the encoding of what, to the named schema, so
// nothing this build writes leaves it unchecked; err is the encoder's.
func schemaChecked(blob []byte, err error, name, what string) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", what, err)
	}
	var v any
	if err := json.Unmarshal(blob, &v); err != nil {
		return nil, fmt.Errorf("re-read %s: %w", what, err)
	}
	if err := schema.Validate(name, v); err != nil {
		return nil, fmt.Errorf("the %s this build produced does not match %s: %w", what, name, err)
	}
	return blob, nil
}

// heroFor renders one year's headline figure. Its prose lives here only; a
// client copy for a year switch would let two figures on one page disagree.
func heroFor(meta projectionMetadata) figure {
	h := meta.Headline
	return figure{
		Label: "What the city actually spends",
		Value: amount.Cents(h.AllFundsGrossExpenditureCents).Dollars(),
		Note:  "All funds, gross, " + ledeOf(meta.FiscalYearLabel, meta.Basis) + " budget",
		Kind:  "hero",
	}
}

// decodeSankey reads a document as a SPINE document, and refuses one with no
// fiscal_year_label or headline.
func decodeSankey(stem string, raw []byte) (projectionDoc, projectionMetadata, error) {
	doc, err := decodeDocument(stem, raw)
	if err != nil {
		return doc, projectionMetadata{}, err
	}
	var meta projectionMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return doc, projectionMetadata{}, fmt.Errorf("decode %s metadata: %w", stem, err)
	}
	if meta.FiscalYearLabel == "" {
		return doc, projectionMetadata{}, fmt.Errorf("%s metadata has no fiscal_year_label", stem)
	}
	if meta.Headline.AllFundsGrossExpenditureCents == 0 {
		return doc, projectionMetadata{}, fmt.Errorf("%s metadata has no headline expenditure", stem)
	}
	return doc, meta, nil
}

// sankeyTitle is one year's <title>: a View's own Title on every year,
// unsuffixed; only the fallback carries a year.
func sankeyTitle(callerTitle, yearLabel string) string {
	if callerTitle != "" {
		return callerTitle
	}
	return "City of Livermore budget flows — " + yearLabel
}

// scheduleDocument is as much of any document as a view's chart needs when it
// draws a schedule other than the view's own.
type scheduleDocument struct {
	Metadata struct {
		GeneratedBy string       `json:"generated_by"`
		FiscalYear  int          `json:"fiscal_year"`
		Basis       string       `json:"basis"`
		Sources     []sourceMeta `json:"sources"`
		Caveats     []caveatMeta `json:"caveats"`
		Counts      countsRef    `json:"counts"`
	} `json:"metadata"`
}

// drawn is the document a view's own chart draws in one year, by stem.
type drawn struct {
	stem string
	doc  scheduleDocument
}

// drawnDocument is the year's column's document of [View.Draws], refused
// where the column carries none or another build made it.
func drawnDocument(v View, year, builtBy string, projections map[string][]byte, ix ColumnIndex) (drawn, error) {
	var out drawn
	col, folded := ix.Column(year)
	stem, ok := ix.Stem(col, v.Draws)
	raw, built := projections[stem]
	if !folded || !ok || !built {
		return out, fmt.Errorf("view %q draws schedule %q and year stem %q's column carries none",
			v.Path, v.Draws, year)
	}
	if _, err := decodeDocument(stem, raw); err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out.doc); err != nil {
		return out, fmt.Errorf("decode %s metadata: %w", stem, err)
	}
	if out.doc.Metadata.GeneratedBy != builtBy {
		return out, fmt.Errorf(
			"view %q opens on %q built by %q and draws %q, built by %q; one view is one build", v.Path, v.Projection, builtBy, stem,
			out.doc.Metadata.GeneratedBy)
	}
	out.stem = stem
	return out, nil
}

func buildSankeyPage(p pageCtx) (pageData, error) {
	o, v, nav, ix, cx := p.o, p.v, p.nav, p.ix, p.cx
	_, meta, err := decodeSankey(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return pageData{}, err
	}

	// Each year's document is refused on its own terms, so one bad document
	// is named rather than skipped.
	stems := v.YearStems
	if len(stems) == 0 {
		stems = []string{v.Projection}
	}
	years := make([]yearView, 0, len(stems))
	var sel yearSelect
	// The footer and the client's doc map are the union over every year
	// (unionSources).
	var cited []sourceMeta
	for _, stem := range stems {
		m := meta
		if stem != v.Projection {
			if _, m, err = decodeSankey(stem, o.Projections[stem]); err != nil {
				return pageData{}, err
			}
		}
		// Basis travels per year in yearView; scope and builder fail closed
		// here instead of being repainted.
		if m.Scope() != meta.Scope() {
			return pageData{}, fmt.Errorf(
				"view %q opens on %q with scope %q but its year stem %q has scope %q; "+
					"one page cannot state two scopes, and its lede's wording is not per-year",
				v.Path, v.Projection, meta.Scope(), stem, m.Scope())
		}
		if m.GeneratedBy != meta.GeneratedBy {
			return pageData{}, fmt.Errorf(
				"view %q opens on %q built by %q but its year stem %q was built by %q; "+
					"one view is one build",
				v.Path, v.Projection, meta.GeneratedBy, stem, m.GeneratedBy)
		}
		cited = append(cited, m.Sources...)
		hero := heroFor(m)
		counts := countsRef{Facts: m.Counts.Facts, Nodes: m.Counts.Nodes, Links: m.Counts.Links}
		caveats, refErr := cx.refs(m.Caveats, stem, v.Path)
		if refErr != nil {
			return pageData{}, refErr
		}
		if v.Draws != "" {
			chart, chartErr := drawnDocument(v, stem, meta.GeneratedBy, o.Projections, ix)
			if chartErr != nil {
				return pageData{}, chartErr
			}
			// The chart, counts and caveats are the drawn schedule's; the
			// headline and its caveats stay the spine's, first.
			cited = append(cited, chart.doc.Metadata.Sources...)
			counts = chart.doc.Metadata.Counts
			drawnRefs, drawnErr := cx.refs(chart.doc.Metadata.Caveats, chart.stem, v.Path)
			if drawnErr != nil {
				return pageData{}, drawnErr
			}
			caveats = append(caveats, drawnRefs...)
		}
		sel.newest(m.FiscalYear, m.FiscalYearLabel, m.Basis)
		years = append(years, yearView{
			Year:       m.FiscalYear,
			Label:      m.FiscalYearLabel,
			Stem:       stem,
			Path:       ColumnPath(m.FiscalYear, m.Basis),
			Basis:      m.Basis,
			Title:      sankeyTitle(v.Title, m.FiscalYearLabel),
			ChartTitle: "Sankey diagram of the " + ledeOf(m.FiscalYearLabel, m.Basis) + " budget",
			Lede:       ledeOf(m.FiscalYearLabel, m.Basis),
			Hero:       hero,
			Caveats:    caveats,
			Counts:     counts,
		})
	}
	// The page opens on the newest year.
	open := years[len(years)-1]

	ch, clientDocs, err := p.chrome(chrome{
		Title:      open.Title,
		DataPath:   open.Path,
		Scope:      meta.Scope(),
		YearSelect: sel,
	}, cited, nil)
	if err != nil {
		return pageData{}, err
	}

	cfg := clientConfig{
		SchemaVersion:     PageConfigVersion,
		ProjectionVersion: SchemaVersion,
		ExportedBy:        o.GeneratedBy,
		Primary:           cmp.Or(v.Draws, v.Projection),
		Years:             years,
		Docs:              clientDocs,
		KindLabels:        kindLabels(),
		SplitLabels:       splitLabels(),
		MemberNouns:       memberNouns(),
		Wording:           defaultWording(),
		Overview:          v.Overview,
	}
	blob, err := encodeConfig(cfg)
	if err != nil {
		return pageData{}, err
	}

	return pageData{
		chrome:      ch,
		Lede:        open.Lede,
		Basis:       open.Basis,
		Wording:     defaultWording(),
		ChartTitle:  open.ChartTitle,
		Hero:        open.Hero,
		Years:       years,
		Facts:       open.Counts.Facts,
		Nodes:       open.Counts.Nodes,
		Links:       open.Counts.Links,
		Description: v.Description,
		Narrow:      narrowRoute(o, nav),
		// #nosec G203 -- encoding/json escapes <, > and & to their \u form,
		// so blob cannot terminate the script element.
		ConfigJSON: template.JS(blob),
	}, nil
}

// narrowRoute is the nav's item for the view rendering LedgerTemplate, or the
// zero item on a site with none.
func narrowRoute(o *Options, nav []navItem) navItem {
	path := templatePathOf(o, LedgerTemplate)
	for _, n := range nav {
		if path != "" && n.Path == path {
			n.Current = false
			return n
		}
	}
	return navItem{}
}

// templatePathOf is the path of the view rendering template name, or "" when
// the site has none.
func templatePathOf(o *Options, name string) string {
	for _, v := range o.Views {
		if v.Template == name {
			return v.Path
		}
	}
	return ""
}

// listedBy refuses a page view cites that the provenance index at
// sourcesPath does not list: view links there in place of its own source
// list, so an unlisted page would be a citation no link reaches.
func listedBy(index []PageIndexEntry, cited []sourceMeta, view, sourcesPath string) error {
	listed := make(map[Citation]bool, len(index))
	for _, e := range index {
		listed[e.Citation] = true
	}
	for _, s := range cited {
		for _, p := range s.Pages {
			if !listed[Citation{DocID: s.DocID, Page: p}] {
				return fmt.Errorf("view %q cites %s p%d and links its sources to %q, which does not list it",
					view, s.DocID, p, sourcesPath)
			}
		}
	}
	return nil
}
