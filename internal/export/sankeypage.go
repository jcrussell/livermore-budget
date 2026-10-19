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

// projectionMetadata is the decoded metadata block of a SANKEY document. The
// page renders these figures server-side so the headline survives without
// JavaScript.
//
// IT IS THE SPINE'S AND NOT EVERY DOCUMENT'S, which is what the split in
// decodeSankey is about: headline is the spine's alone, and a trends document
// carries neither it nor a fiscal year and is not defective for that.
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

// Scope is the schedule set as the footer prints it: the scopes joined, in
// the document's own order.
func (m projectionMetadata) Scope() string { return strings.Join(m.Scopes, ", ") }

// figure is one headline figure: the index's headline, and a glance bar's
// printed total. The JSON tags are load-bearing: the hero ships in
// window.FISC_CONFIG for the year toggle to paint, which reads label and value,
// and the template never sees the JSON to notice a missing tag.
type figure struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note"`
	// Kind selects the figure's treatment: "hero" for the headline, "" for
	// the rest. It is a class name, not a colour: the stylesheet owns the
	// palette.
	Kind string `json:"kind"`
}

// yearView is one published fiscal year's worth of everything the page states
// in words rather than draws.
//
// It is built in Go for every year, not just the one the page opens on, and
// the client only chooses between them.
type yearView struct {
	Year  int    `json:"year"`
	Label string `json:"label"`
	Stem  string `json:"stem"`
	Path  string `json:"path"`
	Basis string `json:"basis"`
	// Title is this year's <title>. See sankeyTitle for why the caller's words
	// survive the switch.
	Title   string      `json:"title"`
	Hero    figure      `json:"hero"`
	Caveats []caveatRef `json:"caveats"`
	Counts  countsRef   `json:"counts"`
	// ChartTitle is the <title> inside the SVG -- the chart's accessible name,
	// and a different string from Title, which is the document's.
	ChartTitle string `json:"chart_title"`
	// Lede is the year and basis as the lede names them, "FY 2025-26 adopted";
	// the template renders it and app.js repaints it and names a column by it.
	Lede string `json:"lede"`
}

// countsRef is the "N flows between M nodes, from K facts" line, per year.
type countsRef struct {
	Facts int `json:"facts"`
	Nodes int `json:"nodes"`
	Links int `json:"links"`
}

// pageData is the Sankey template's input: the shared chrome plus everything
// only a spine page has.
type pageData struct {
	chrome
	// Lede is the opening year's, as yearView carries it.
	Lede  string
	Basis string
	// Wording is the client's words, which the template's legend shares.
	Wording wording
	// TableHeading is flowTableHeading.
	TableHeading string
	// ChartTitle is the SVG's accessible name: the opening yearView's string,
	// the same one app.js repaints on a year switch.
	ChartTitle string
	Hero       figure
	// SourcesPath is the provenance index, or "" when the site has none: with
	// one, the footer links there instead of listing every cited page.
	SourcesPath string
	// Years is every published year, oldest first. The template renders the
	// Opens year's headline and lists them all as a selector.
	Years []yearView
	// Opens is the stem of the year the page opens on: the last of Years.
	Opens string
	Facts int
	Nodes int
	Links int
	// Description is [View.Description].
	Description string
	// ConfigJSON is window.FISC_CONFIG. json.Marshal escapes <, > and & to
	// their \u form, so the blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// ledeOf is a column named the way the lede names it.
func ledeOf(label, basis string) string { return label + " " + basis }

// clientConfig is window.FISC_CONFIG: the metadata the page needs before it
// has fetched anything, plus where to fetch the bulk from.
type clientConfig struct {
	SchemaVersion int    `json:"schema_version"`
	ExportedBy    string `json:"exported_by"`
	Primary       string `json:"primary"`
	// Years is every published year with the words that belong to it, built by
	// the packager so the client never composes a figure or a caveat itself.
	Years []yearView           `json:"years"`
	Docs  map[string]clientDoc `json:"docs"`
	// KindLabels is each link kind in the page's words.
	KindLabels map[string]string `json:"kind_labels"`
	// SplitLabels is what each split key's members are called, in the page's
	// words, keyed as the expansion schedule names a split.
	SplitLabels map[string]string `json:"split_labels"`
	// MemberNouns is what one member of each split key is called, in the
	// page's words, keyed as SplitLabels.
	MemberNouns map[string]string `json:"member_nouns"`
	// Wording is every sentence the client composes, as templates it fills.
	Wording wording `json:"wording"`
	// Overview is the page's own chart: its form and that form's hints.
	Overview Chart `json:"overview"`
}

// encodeConfig renders window.FISC_CONFIG and refuses bytes that do not match
// the published schema, as encodeColumn does for a column.
//
// The template renders these structs by Go field name, so a missing JSON tag
// would blank only what a reader gets after a year switch; this catches it.
func encodeConfig(cfg clientConfig) ([]byte, error) {
	blob, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode page config: %w", err)
	}
	resolved, err := schema.Load(schema.Page)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(blob, &v); err != nil {
		return nil, fmt.Errorf("re-read page config: %w", err)
	}
	if err := resolved.Validate(v); err != nil {
		return nil, fmt.Errorf("the page config this build produced for %q does not match %s: %w",
			cfg.Primary, schema.Page, err)
	}
	return blob, nil
}

// heroFor renders one year's headline figure.
//
// The prose lives here and nowhere else. The note is a claim about what the
// number means, and a second implementation of it — in the client, for a year
// switch — is the way two figures on one page come to disagree about the same
// schedule.
func heroFor(meta projectionMetadata) figure {
	h := meta.Headline
	return figure{
		Label: "What the city actually spends",
		Value: amount.Cents(h.AllFundsGrossExpenditureCents).Dollars(),
		Note:  "All funds, gross, " + ledeOf(meta.FiscalYearLabel, meta.Basis) + " budget",
		Kind:  "hero",
	}
}

// decodeSankey reads a document as a SPINE document, and refuses one that is not.
//
// The two refusals below are the Sankey's, not every document's: a trends
// document carries neither a fiscal_year_label nor a headline.
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

// sankeyTitle is one year's <title> on the spine page.
//
// THE CALLER'S WORDS SURVIVE THE SWITCH: a View that sets a Title gets it on
// every year, unsuffixed. Only the fallback carries a year.
func sankeyTitle(callerTitle, yearLabel string) string {
	if callerTitle != "" {
		return callerTitle
	}
	return "City of Livermore budget flows — " + yearLabel
}

// scheduleDocument is as much of ANY document as a view's chart needs when it
// draws a schedule other than the view's own: who built it, what it cites and
// what it discloses.
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

// drawnDocument is the document a view's own chart draws in one year, where
// [View.Draws] names a schedule other than the view's: the year's column's
// document of it, refused where the column carries none or another
// projection built it.
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
			"view %q opens on %q built by %q and draws %q, built by %q; the footer credits "+
				"one projection for figures drawn from both", v.Path, v.Projection, builtBy, stem,
			out.doc.Metadata.GeneratedBy)
	}
	out.stem = stem
	return out, nil
}

func buildSankeyPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string, cx *caveatIndex,
) (pageData, error) {
	doc, meta, err := decodeSankey(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return pageData{}, err
	}

	// Every published year, in the order the caller handed them, opening year
	// first. A year's document is decoded and refused on its own terms: one bad
	// document is named, rather than the page silently opening on whichever year
	// happened to parse.
	stems := v.YearStems
	if len(stems) == 0 {
		stems = []string{v.Projection}
	}
	years := make([]yearView, 0, len(stems))
	// THE FOOTER AND THE CLIENT'S DOC MAP ARE THE UNION OVER EVERY YEAR, not the
	// opening year's. See unionSources for what breaks otherwise; the union is
	// accumulated here because this loop already decodes every year.
	var cited []sourceMeta
	for _, stem := range stems {
		m := meta
		if stem != v.Projection {
			if _, m, err = decodeSankey(stem, o.Projections[stem]); err != nil {
				return pageData{}, err
			}
		}
		// The footer's "Scope X, basis Y. Projection: Z." is rendered from the
		// opening year; basis travels per year in yearView, and scope and
		// builder fail closed here instead of being repainted.
		if m.Scope() != meta.Scope() {
			return pageData{}, fmt.Errorf(
				"view %q opens on %q with scope %q but its year stem %q has scope %q; "+
					"one page cannot state two scopes, and its lede's wording is not per-year",
				v.Path, v.Projection, meta.Scope(), stem, m.Scope())
		}
		if m.GeneratedBy != meta.GeneratedBy {
			return pageData{}, fmt.Errorf(
				"view %q opens on %q built by %q but its year stem %q was built by %q; "+
					"the footer credits one projection for figures drawn from both",
				v.Path, v.Projection, meta.GeneratedBy, stem, m.GeneratedBy)
		}
		cited = append(cited, m.Sources...)
		hero := heroFor(m)
		counts := countsRef{Facts: m.Counts.Facts, Nodes: m.Counts.Nodes, Links: m.Counts.Links}
		caveats, refErr := cx.refs(m.Caveats, stem)
		if refErr != nil {
			return pageData{}, refErr
		}
		if v.Draws != "" {
			chart, chartErr := drawnDocument(v, stem, meta.GeneratedBy, o.Projections, ix)
			if chartErr != nil {
				return pageData{}, chartErr
			}
			// The chart, its counts line and its caveats are the drawn
			// schedule's; the headline stays the spine's, whose caveats
			// come first.
			cited = append(cited, chart.doc.Metadata.Sources...)
			counts = chart.doc.Metadata.Counts
			drawnRefs, drawnErr := cx.refs(chart.doc.Metadata.Caveats, chart.stem)
			if drawnErr != nil {
				return pageData{}, drawnErr
			}
			caveats = append(caveats, drawnRefs...)
		}
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
	// The page opens on the newest year, the last of them; everything
	// per-year the page renders statically comes from this entry.
	open := years[len(years)-1]

	union := unionSources(cited)
	sourcesPath := templatePathOf(o, ProvenanceTemplate)
	if sourcesPath != "" {
		if listErr := listedBy(o.PageIndex, union, v.Path, sourcesPath); listErr != nil {
			return pageData{}, listErr
		}
	}
	sources, clientDocs := sourcesFor(union, byID, pageTextBase, o.RecordsBase)

	cfg := clientConfig{
		SchemaVersion: doc.SchemaVersion,
		ExportedBy:    o.GeneratedBy,
		Primary:       cmp.Or(v.Draws, v.Projection),
		Years:         years,
		Docs:          clientDocs,
		KindLabels:    kindLabels(),
		SplitLabels:   splitLabels(),
		MemberNouns:   memberNouns(),
		Wording:       defaultWording(),
		Overview:      v.Overview,
	}
	blob, err := encodeConfig(cfg)
	if err != nil {
		return pageData{}, err
	}

	return pageData{
		chrome: chrome{
			Title:        open.Title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  projectionRefs(o, ix),
			DataPath:     open.Path,
			Scope:        meta.Scope(),
			CaveatsHere:  cx.here(v.Path),
		},
		Lede:         open.Lede,
		Basis:        open.Basis,
		Wording:      defaultWording(),
		TableHeading: flowTableHeading,
		ChartTitle:   open.ChartTitle,
		Hero:         open.Hero,
		SourcesPath:  sourcesPath,
		Years:        years,
		Opens:        open.Stem,
		Facts:        open.Counts.Facts,
		Nodes:        open.Counts.Nodes,
		Links:        open.Counts.Links,
		Description:  v.Description,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and & to their \u form, so it cannot terminate the script element
		// or inject markup. The alternative, letting html/template escape a
		// string, would corrupt the JSON.
		ConfigJSON: template.JS(blob),
	}, nil
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
