package export

import (
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

// figure is one stat tile.
// The JSON tags are load-bearing, not decoration: a figure is rendered by the
// template AND shipped in window.FISC_CONFIG for the year toggle to swap in, and
// without them it serialises as Label/Value/Note/Kind while the client reads
// label/value/note/kind. That is a page of empty tiles, and it is silent — the
// template keeps working, because the template never sees the JSON.
type figure struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note"`
	// Kind selects the tile's treatment: "hero" for the headline, "error" for
	// the figure the page exists to argue against, "" for the rest. It is a
	// class name, not a colour: the stylesheet owns the palette.
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
	Figures []figure    `json:"figures"`
	Caveats []caveatRef `json:"caveats"`
	Counts  countsRef   `json:"counts"`
	// Steps is what this year's rungs disclose, one entry per [View.Steps]
	// entry, resolved for THIS year. Omitted on a view that opens nothing.
	Steps []stepView `json:"steps,omitempty"`
	// ChartTitle is the <title> inside the SVG -- the chart's accessible name,
	// and a different string from Title, which is the document's.
	ChartTitle string `json:"chart_title"`
	// Lede is the year and basis as the lede names them, "FY 2025-26 adopted";
	// the template renders it and app.js repaints it and names a column by it.
	Lede string `json:"lede"`
}

// stepView is what one rung's document discloses for one year: the caveats its
// marks link to. Nothing here names a file: the client selects the schedule
// out of its year's column, and which of its nodes open is read off that
// document by the client's own reach (decomposable in site/app.js).
type stepView struct {
	Caveats []caveatRef `json:"caveats"`
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
	Figures    []figure
	// Years is every published year, oldest first. The template renders the
	// Opens year's tiles and caveats and lists them all as a selector.
	Years []yearView
	// Opens is the stem of the year the page opens on: the last of Years.
	Opens string
	Facts int
	Nodes int
	Links int
	// Drill is whether this page's chart opens a node, which the lede's
	// sentence about a click depends on.
	Drill bool
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
	// Wording is every sentence the client composes, as templates it fills.
	Wording wording `json:"wording"`
	// Overview is the page's own chart: its form and that form's hints.
	Overview Chart `json:"overview"`
	// Steps is how the page opens a node, one hop per step, omitted (not [])
	// on a page that opens none.
	Steps []DrillStep `json:"steps,omitempty"`
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

// tilesFor renders one year's headline figures.
//
// The prose lives here and nowhere else. Each note is a claim about what the
// number means, and a second implementation of it — in the client, for a year
// switch — is the way two figures on one page come to disagree about the same
// schedule.
func tilesFor(meta projectionMetadata) (figure, []figure) {
	h := meta.Headline
	hero := figure{
		Label: "What the city actually spends",
		Value: amount.Cents(h.AllFundsGrossExpenditureCents).Dollars(),
		Note:  "All funds, gross, " + ledeOf(meta.FiscalYearLabel, meta.Basis) + " budget",
		Kind:  "hero",
	}
	return hero, []figure{{
		Label: "Naive column total",
		Value: amount.Cents(h.NaiveExpenditureCents).Dollars(),
		// The inflation is the transfers out the naive total counts a second time,
		// cited from the field rather than recomputed as naive minus gross.
		Note: "The wrong answer: summing the expenditure column counts transfers between funds twice, inflating the total by " +
			amount.Cents(h.InternalTransferOutCents).Dollars() + ".",
		Kind: "error",
	}, {
		Label: "All-funds gross revenue",
		Value: amount.Cents(h.AllFundsGrossRevenueCents).Dollars(),
		Note:  "Ties to the printed schedule; includes internal service charges.",
	}, {
		Label: "External revenue",
		Value: amount.Cents(h.ExternalRevenueCents).Dollars(),
		Note:  "Net of internal service charges billed between city departments.",
	}, {
		Label: "External spending",
		Value: amount.Cents(h.ExternalExpenditureCents).Dollars(),
		Note:  "Net of internal service charges.",
	}, {
		Label: "Transfers in / out",
		Value: amount.Cents(h.InternalTransferInCents).Dollars() + " / " + amount.Cents(h.InternalTransferOutCents).Dollars(),
		Note:  "Money moving between the city's own funds.",
	}, {
		Label: "Unmatched transfers",
		Value: amount.Cents(h.TransferResidualCents).Dollars(),
		// Says what the number is and defers why to internal/project's
		// transfer caveat, and promises no caveat: that one is conditional.
		Note: "Transfers out minus transfers in. No link in this chart pairs a transfer's two legs.",
	}}
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

// stepDocument is as much of ANY document as a rung needs: who built it, what
// it cites and what it discloses.
type stepDocument struct {
	Metadata struct {
		GeneratedBy string       `json:"generated_by"`
		FiscalYear  int          `json:"fiscal_year"`
		Basis       string       `json:"basis"`
		Sources     []sourceMeta `json:"sources"`
		Caveats     []caveatMeta `json:"caveats"`
	} `json:"metadata"`
}

// stepDocuments is what one year's rungs will draw: the document each step
// resolves to for that year, with its own caveat refs, and the pages those
// documents cite.
//
// The join is [StepStems]'. The pages feed the same union the years do, for
// unionSources' reason. builtBy is the view's own projection's generated_by;
// a step document built by another is refused, as a year's is.
func stepDocuments(v View, year, builtBy string, fiscalYear int, basis string,
	projections map[string][]byte, ix ColumnIndex, caveatsPath string,
) ([]stepView, []sourceMeta, error) {
	stems, serr := StepStems(v.Steps, year, ix)
	if serr != nil {
		return nil, nil, fmt.Errorf("view %q (year stem %q declares fiscal_year %d, basis %q): %w",
			v.Path, year, fiscalYear, basis, serr)
	}
	var (
		out   []stepView
		cited []sourceMeta
	)
	for i := range v.Steps {
		stem := stems[i]
		raw, ok := projections[stem]
		if !ok {
			return nil, nil, fmt.Errorf(
				"view %q's step %d renders projection %q for year stem %q, which was not built",
				v.Path, i, stem, year)
		}
		if _, err := decodeDocument(stem, raw); err != nil {
			return nil, nil, err
		}
		var doc stepDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, fmt.Errorf("decode %s metadata: %w", stem, err)
		}
		if doc.Metadata.GeneratedBy != builtBy {
			return nil, nil, fmt.Errorf(
				"view %q opens on %q built by %q but step %d's document %q for year stem %q "+
					"was built by %q; the footer credits one projection for figures drawn "+
					"from both", v.Path, v.Projection, builtBy, i, stem, year, doc.Metadata.GeneratedBy)
		}
		// No column guard here: [ColumnIndex] selects a document only under
		// the column it itself declares, so it cannot be another year's.
		cited = append(cited, doc.Metadata.Sources...)
		out = append(out, stepView{
			// Stem-keyed: two schedules of one column can carry one caveat id.
			Caveats: caveatRefs(doc.Metadata.Caveats, stem, caveatsPath),
		})
	}
	return out, cited, nil
}

func buildSankeyPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (pageData, error) {
	caveatsPath := caveatsPathOf(o)
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
		steps, stepped, stepErr := stepDocuments(v, stem, meta.GeneratedBy, m.FiscalYear, m.Basis, o.Projections, ix, caveatsPath)
		if stepErr != nil {
			return pageData{}, stepErr
		}
		cited = append(cited, stepped...)
		hero, figures := tilesFor(m)
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
			Figures:    figures,
			Caveats:    caveatRefs(m.Caveats, stem, caveatsPath),
			Counts: countsRef{
				Facts: m.Counts.Facts, Nodes: m.Counts.Nodes, Links: m.Counts.Links,
			},
			Steps: steps,
		})
	}
	// The page opens on the newest year, the last of them; everything
	// per-year the page renders statically comes from this entry.
	open := years[len(years)-1]

	sources, clientDocs := sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)

	cfg := clientConfig{
		SchemaVersion: doc.SchemaVersion,
		ExportedBy:    o.GeneratedBy,
		Primary:       v.Projection,
		Years:         years,
		Docs:          clientDocs,
		KindLabels:    kindLabels(),
		Wording:       defaultWording(),
		Overview:      v.Overview,
		Steps:         v.Steps,
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
			Caveats:      open.Caveats,
			CaveatsPath:  caveatsPath,
		},
		Lede:         open.Lede,
		Basis:        open.Basis,
		Wording:      defaultWording(),
		TableHeading: flowTableHeading,
		ChartTitle:   open.ChartTitle,
		Hero:         open.Hero,
		Figures:      open.Figures,
		Years:        years,
		Opens:        open.Stem,
		Facts:        open.Counts.Facts,
		Nodes:        open.Counts.Nodes,
		Links:        open.Counts.Links,
		Drill:        len(v.Steps) > 0,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and & to their \u form, so it cannot terminate the script element
		// or inject markup. The alternative, letting html/template escape a
		// string, would corrupt the JSON.
		ConfigJSON: template.JS(blob),
	}, nil
}
