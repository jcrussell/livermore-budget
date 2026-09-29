package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
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
		return cmdutil.WithHint(err,
			"this projection was written by a newer fisc; upgrade the binary")
	case got == 0:
		return cmdutil.WithHint(err,
			"schema_version is absent or zero; this may not be a fisc projection")
	default:
		return err
	}
}

// projectionDoc is as much of a projection document as the packager reads.
// Everything else — nodes, links — is bulk the browser fetches, and decoding
// it here would be a second parser for a contract that already has one.
//
// Metadata is kept as raw JSON as well as decoded, so window.FISC_CONFIG can
// carry the projection's own bytes rather than this package's re-rendering of
// them. A field this struct does not know about still reaches the client.
type projectionDoc struct {
	SchemaVersion int             `json:"schema_version"`
	Projection    string          `json:"projection"`
	Metadata      json.RawMessage `json:"metadata"`
}

// documentCaveats is as much of ANY document as the caveats page needs.
// Shape-blind, for documentSources' reason.
type documentCaveats struct {
	Metadata struct {
		FiscalYearLabel string       `json:"fiscal_year_label"`
		Basis           string       `json:"basis"`
		Caveats         []caveatMeta `json:"caveats"`
	} `json:"metadata"`
}

// sourceMeta is one cited document in any projection's metadata.
type sourceMeta struct {
	DocID string `json:"doc_id"`
	Pages []int  `json:"pages"`
}

// documentSources is as much of ANY document as the citation union needs.
//
// It is deliberately shape-blind: metadata.sources is the one block every
// projection carries whatever its body is.
type documentSources struct {
	Metadata struct {
		Sources []sourceMeta `json:"sources"`
	} `json:"metadata"`
}

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

// caveatMeta is a decoded caveat.
//
// A SEPARATE TYPE FROM project.Caveat, like every other decode struct in this
// file, because this package consumes projections as bytes and does not import
// internal/project. The field set is the contract, and it is pinned by
// TestEveryNameThisPackageDecodesIsOneAProjectionStates rather than by the two
// declarations happening to agree.
type caveatMeta struct {
	ID        string   `json:"id"`
	Summary   string   `json:"summary"`
	Text      string   `json:"text"`
	AppliesTo []string `json:"applies_to"`
}

// caveatRef is one caveat as a PAGE shows it: a line, and somewhere to go for
// the rest; the text is deliberately absent. Href is empty when the site has
// no caveats page, and the template then renders plain text.
type caveatRef struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Href    string `json:"href"`
}

// caveatRefs composes the page-facing form. base is the caveats view's path, or
// "" when the site has no such page.
//
// The anchor is per (caveat, document), not per caveat: one id can carry
// different text in different documents, so stem is required.
func caveatRefs(metas []caveatMeta, stem, base string) []caveatRef {
	out := make([]caveatRef, 0, len(metas))
	for _, m := range metas {
		ref := caveatRef{ID: m.ID, Summary: m.Summary}
		if base != "" {
			ref.Href = base + "#" + caveatAnchor(stem, m.ID)
		}
		out = append(out, ref)
	}
	return out
}

// caveatAnchor is the one spelling of the fragment, so the page that emits the
// id and the pages that link to it cannot disagree about its form.
func caveatAnchor(stem, id string) string { return "caveat-" + stem + "--" + id }

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

// pageRef is one cited page of one source document.
type pageRef struct {
	Number  int
	PDFURL  string
	TextURL string
}

// sourceRef is one cited source document.
type sourceRef struct {
	DocID     string
	Title     string
	Publisher string
	PDFURL    string
	Pages     []pageRef
}

// projectionRef names one data file the site publishes.
type projectionRef struct {
	Path string
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

type trendsPageData struct {
	chrome
	Columns []columnRef
	Series  []seriesRef
	Facts   int
	Count   int
}

// columnRef is one printed column as the table heads it.
type columnRef struct {
	Label string
	// Basis is the word the chip and every cell tooltip print, which is
	// [basisLabelFor]'s answer and not the document's basis enum.
	Basis string
	// Group is the column's comparable_group, rendered as a data attribute so
	// the boundary between measurements is in the markup rather than only in a
	// caveat. New says this column starts a new group, which is where a reader
	// should not carry a comparison across.
	Group string
	New   bool
}

// seriesRef is one printed row as the table renders it.
type seriesRef struct {
	Label    string
	Fund     string
	Group    string
	Category string
	Cells    []cellRef
	// Mark is the row's four columns drawn beside the figures they are.
	Mark markRef
}

// The mark's coordinate system, in SVG user units.
//
// INTEGERS, AND THE VIEWBOX IS FIXED. Every bar's geometry is computed from
// int64 cents with integer arithmetic and rendered as a whole number, so two
// runs of `fisc export` produce byte-identical markup — a float here would put
// 12.333333333333334 in the output and make the page's bytes depend on the
// platform's formatting. It is also the amounts-are-integer-cents discipline
// holding one step further than the store: nothing on the path from a fact to a
// pixel is a float either.
//
// The units are not pixels. The stylesheet gives the <svg> its rendered size
// and the viewBox scales to it, so the mark follows the table's type size
// without any of these numbers changing.
const (
	markBarWidth = 9
	markBarGap   = 3
	markHeight   = 30
	// markZeroRadius is the dot a PUBLISHED zero gets — a dash the city printed,
	// which is a figure and is therefore drawn.
	//
	// A DOT AND NOT A SHORT BAR, because a size difference is not a difference
	// at this scale. Measured on the shipped page: ten real figures
	// ($500,000, $74,748, $55,380…) rendered at exactly the height of the 165
	// published-zero ticks, and at 1.6em over 13px type one viewBox unit was
	// 0.69 CSS px — so the two were sub-pixel and told apart only by fill. A
	// circle against a rectangle is a difference in KIND, legible at any size
	// and in greyscale, and it cannot be eroded by rounding.
	markZeroRadius = 2
	// markMinBar is what a non-zero figure gets when its share of the row's
	// largest rounds to nothing. A row whose FY2023-24 actual is 0.3% of its
	// FY2026-27 adopted has a real figure that integer division puts at zero
	// height, and a bar that is not there says "no money" where the page says
	// $47,000.
	//
	// IT IS TWO AND NOT ONE, so that a tiny figure and a published zero are
	// never the same rectangle. At one unit they were, for the reason
	// markZeroRadius records. Absent is not zero, and a rounding-error figure is
	// neither.
	markMinBar = 2
)

// markRef is one row's four columns drawn as bars.
//
// IT DRAWS ONLY FIGURES THE ROW ALREADY CITES, and that is what keeps it on the
// right side of this project's central invariant. Every bar is one printed cell
// of this row, rendered beside the linked figure it is; the mark computes no
// total, no growth and no share, so it publishes nothing that could need a
// provenance pointer of its own. A chart of fund-group totals would have needed
// one for each of 28 sums, and there is no page to point at.
type markRef struct {
	// Width is the viewBox width: as many bar slots as the document has
	// columns, so a document of three columns is not a four-column mark with a
	// gap on the end.
	Width int
	// Height is the viewBox height. It is published rather than left to the
	// template because the template had it hardcoded while Width and Baseline
	// came from here, so raising markHeight clipped every bar and no test
	// noticed — a geometry split across two files where only one of them is
	// checked.
	Height int
	// Rules are the boundaries between comparable groups, at the same columns
	// the table heads with a rule. Two of these four columns are money that
	// moved and two are an intention, and a reader running an eye along a mark
	// has to meet the same warning they meet along the row.
	Rules []int
	// Baseline is where zero sits. A row with no negative figure gets the full
	// height for its positives; a row with a contra cell splits the box, so the
	// two directions are legible against each other rather than one being
	// clipped.
	Baseline int
	Bars     []barRef
	// Label is the mark's accessible name. The bars are aria-hidden beneath it
	// because every figure they draw is already in this row as linked text: a
	// screen reader that walked them would hear each number twice and reach 924
	// extra tab stops on the way, which is a worse page for the reader it was
	// added for.
	Label string
}

// barRef is one column's figure as a rectangle.
type barRef struct {
	X, Y, Width, Height int
	// Zero marks a PUBLISHED zero — a printed dash, which is a figure — drawn
	// as a DOT on the baseline rather than as a rectangle. Absent is not zero
	// (AGENTS.md, Provenance invariants) and the three states must not look alike: a
	// column this row does not carry produces no barRef at all and the slot is
	// empty, a published zero is a dot, and the smallest real figure is a
	// rectangle markMinBar high.
	Zero bool
	// CX and R place the zero dot. They are ignored when Zero is false.
	CX, R int
	// Negative marks a contra row, so the stylesheet can tell the two
	// directions apart without the reader inferring it from position.
	Negative bool
	// Title is the native SVG tooltip: the column and the figure, so a bar is
	// identifiable on hover without the mark carrying visible labels it has no
	// room for.
	Title string
}

// cellRef is one column of one row: the point published there, or the absence
// of one.
type cellRef struct {
	Value string
	// Missing says the series publishes no point in this column. The cell is
	// still rendered, because the alternative is what this type exists to
	// prevent — see buildCells.
	Missing bool
	// Negative marks a contra row — the General Fund's ERAF and RPTTF Reduction
	// are printed in parentheses and published signed — so the stylesheet can
	// colour the cell and fill the mark's bar downward.
	//
	// THE CELL STILL RENDERS A MINUS SIGN, DELIBERATELY. Rendering
	// (14,086,438) as the city prints it was considered and refused, because
	// screen readers do not announce parentheses at default punctuation
	// settings — the cell would be read aloud as a POSITIVE figure, and the
	// minus sign is the only part of it that carries direction to a reader who
	// is not looking at the colour. Nine cells ship. Accounting convention is a
	// visual convention, and this page has one reader it cannot see.
	//
	// So what the flag drives is .contra: the cell's colour, and .mark-bar.contra
	// in the mark beside it, where direction really is geometric and the bar
	// hanging below the baseline says it without punctuation.
	Negative bool
	// New repeats the column's group boundary onto the body cell, so the rule
	// between two measurements runs down the table rather than stopping at the
	// header.
	New bool
	// Cents is the figure the cell shows, carried alongside its rendered Value
	// so the mark beside it can be scaled without re-parsing the string the
	// packager just formatted. It is the DOCUMENT's number, read and not
	// recomputed, and it is integer cents like every other amount here.
	Cents int64
	// Page and Href cite the figure itself. Every point carries its own page in
	// the document, which is what makes a single cell citable.
	Page int
	Href string
}

// clientDoc is a source document as the client sees it: its title, and for
// every page the site cites, the three links a citation opens, built here so
// the client composes none.
type clientDoc struct {
	Title     string                `json:"title"`
	Publisher string                `json:"publisher"`
	Pages     map[string]clientPage `json:"pages"`
}

// clientPage is one cited page's links. An empty one is a link this export
// has nothing to point at: no PDF URL for the document, or no records.
type clientPage struct {
	PDF     string `json:"pdf"`
	Text    string `json:"text"`
	Records string `json:"records"`
}

// wording is every sentence site/app.js composes about the chart on screen,
// as templates the client fills in: the counts line, the chart hint, the way
// back and the breadcrumb's control. `{name}` is a variable's value and
// `{name:one|many}` is the value followed by the singular or the plural word,
// by whether the value is 1.
//
// Declared here and formatted there, so the words are the packager's and the
// numbers the chart's.
type wording struct {
	Counts            string `json:"counts"`
	CountsPartial     string `json:"counts_partial"`
	CountsCarried     string `json:"counts_carried"`
	CountsCarriedFrom string `json:"counts_carried_from"`
	OpenedHint        string `json:"opened_hint"`
	OpenFurther       string `json:"open_further"`
	OpenInto          string `json:"open_into"`
	NothingFurther    string `json:"nothing_further"`
	NothingOpens      string `json:"nothing_opens"`
	Follow            string `json:"follow"`
	Expand            string `json:"expand"`
	Swatch            string `json:"swatch"`
	InColumn          string `json:"in_column"`
	ColumnLeft        string `json:"column_left"`
	ColumnMiddle      string `json:"column_middle"`
	ColumnSecond      string `json:"column_second"`
	ColumnThird       string `json:"column_third"`
	ColumnRight       string `json:"column_right"`
	GoBack            string `json:"go_back"`
	BackControl       string `json:"back_control"`
	PrintedByCity     string `json:"printed_by_city"`
	InferredByUs      string `json:"inferred_by_us"`
	OurInference      string `json:"our_inference"`
	InferredChip      string `json:"inferred_chip"`
	PrintedChip       string `json:"printed_chip"`
	CarriedNote       string `json:"carried_note"`
	CarriedChip       string `json:"carried_chip"`
	DescOpens         string `json:"desc_opens"`
	DescExpands       string `json:"desc_expands"`
	DescFollows       string `json:"desc_follows"`
	FlowInferred      string `json:"flow_inferred"`
	NoneInferred      string `json:"none_inferred"`
	TablePointer      string `json:"table_pointer"`
}

// kindLabels is project's words for every link kind, keyed as a link names it.
func kindLabels() map[string]string {
	out := make(map[string]string, len(project.LinkKinds()))
	for _, k := range project.LinkKinds() {
		out[string(k)] = project.LinkKindLabel(k)
	}
	return out
}

// ledeOf is a column named the way the lede names it.
func ledeOf(label, basis string) string { return label + " " + basis }

// defaultWording is the site's English. The counts sentence's head is also
// rendered server-side by site/index.html.tmpl for the page before app.js
// runs, and a test holds that line to Counts.
func defaultWording() wording {
	return wording{
		Counts:            "{links:flow|flows} between {nodes:node|nodes}, from {facts:fact|facts}",
		CountsPartial:     "{links:flow|flows} between {nodes:node|nodes}, from {cited} of the document's {facts:fact|facts}",
		CountsCarried:     "{links:flow|flows} between {nodes:node|nodes}: {own} citing {cited} of the document's {facts:fact|facts}, and {carried} carried unchanged from the chart above",
		CountsCarriedFrom: ", citing {above} of its {theirs:fact|facts}",
		OpenedHint:        "This is {label}, broken into its parts.",
		OpenFurther:       "Double click a node{where} to open it further, or tab to one and press Enter.",
		OpenInto:          "Double click a node{where} to open it into its parts, or tab to one and press Enter.",
		NothingFurther:    "Nothing here opens further; go back to open another.",
		NothingOpens:      "Nothing on this chart opens.",
		Follow:            "A single click, or Space, follows one node's money.",
		Expand:            "The folded mark is several of them drawn as one; double click it, or tab to it and press Enter, to draw them separately.",
		Swatch:            "A fund swatch follows one group's money without opening anything.",
		InColumn:          " in the {columns} column",
		ColumnLeft:        "left-hand",
		ColumnMiddle:      "middle",
		ColumnSecond:      "second",
		ColumnThird:       "third",
		ColumnRight:       "right-hand",
		GoBack:            "Use the breadcrumb above the chart, or press Escape, to go back.",
		BackControl:       "\u2190 {back}",
		PrintedByCity:     "printed by the city",
		InferredByUs:      "inferred by us",
		OurInference:      "◇ our inference",
		InferredChip:      "◇ inferred",
		PrintedChip:       "printed",
		CarriedNote:       "figure printed by the city, re-pointed onto a mark of ours",
		CarriedChip:       "◇ re-pointed by us",
		DescOpens:         ", opens into its parts on a double click or Enter; a single click or Space follows this money",
		DescExpands:       ", draws all of them separately on a double click or Enter; a single click or Space follows this money",
		DescFollows:       ", follow this money",
		FlowInferred:      "This flow is inferred; both endpoints are printed by the city.",
		NoneInferred:      "Nothing on this chart is inferred: every node and flow is printed by the city.",
		TablePointer:      "The same figures are in the flow table below, which opens from the \"Every flow, as a table\" heading.",
	}
}

// clientConfig is window.FISC_CONFIG: the metadata the page needs before it
// has fetched anything, plus where to fetch the bulk from.
type clientConfig struct {
	SchemaVersion int    `json:"schema_version"`
	ExportedBy    string `json:"exported_by"`
	Primary       string `json:"primary"`
	// Metadata is the primary projection's metadata block, verbatim.
	Metadata json.RawMessage `json:"metadata"`
	// Years is every published year with the words that belong to it, built by
	// the packager so the client never composes a figure or a caveat itself.
	Years []yearView           `json:"years"`
	Docs  map[string]clientDoc `json:"docs"`
	// KindLabels is each link kind in the page's words.
	KindLabels map[string]string `json:"kind_labels"`
	// Wording is every sentence the client composes, as templates it fills.
	Wording wording `json:"wording"`
	// RenderTiers is the node tiers the page draws, left to right; omitted
	// when the page draws its document whole.
	//
	// OMITTED AND NOT [] WHEN ABSENT, which app.js relies on.
	RenderTiers []int `json:"render_tiers,omitempty"`
	// Steps is how the page opens a node, one hop per step, omitted (not [])
	// on a page that opens none.
	Steps []DrillStep `json:"steps,omitempty"`
	// Root is the node whose subtree the page draws, omitted when it draws the
	// whole document.
	Root string `json:"root,omitempty"`
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
		Note:  "All funds, gross, " + meta.FiscalYearLabel + " " + meta.Basis + " budget",
		Kind:  "hero",
	}
	return hero, []figure{{
		Label: "Naive column total",
		Value: amount.Cents(h.NaiveExpenditureCents).Dollars(),
		Note: "The wrong answer: summing the expenditure column counts transfers between funds twice, inflating the total by " +
			amount.Cents(h.NaiveExpenditureCents-h.AllFundsGrossExpenditureCents).Dollars() + ".",
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

// citationsOf is every (document, page) one projection's metadata cites.
//
// It decodes through documentSources rather than through the projection
// metadata, so a view of a document this packager has never heard of still
// contributes its pages to the set the site ships.
func citationsOf(stem string, raw []byte) ([]Citation, error) {
	if _, err := decodeDocument(stem, raw); err != nil {
		return nil, err
	}
	var src documentSources
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("decode %s sources: %w", stem, err)
	}
	out := make([]Citation, 0, len(src.Metadata.Sources))
	for _, s := range src.Metadata.Sources {
		for _, p := range s.Pages {
			out = append(out, Citation{DocID: s.DocID, Page: p})
		}
	}
	return out, nil
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
			return nil, nil, cmdutil.WithHint(
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

// caveatsPathOf is the caveats view's path, or "" when the site has none.
//
// Derived from the view set, so it cannot drift out of step with it.
func caveatsPathOf(o *Options) string {
	for _, v := range o.views() {
		if v.Template == CaveatsTemplate {
			return v.Path
		}
	}
	return ""
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

// sourcesFor builds a view's own footer citations, and the client's copy of the
// same documents.
func sourcesFor(srcs []sourceMeta, byID map[string]Doc, pageTextBase func(string) string,
	recordsBase map[string]string,
) ([]sourceRef, map[string]clientDoc) {
	sources := make([]sourceRef, 0, len(srcs))
	clientDocs := make(map[string]clientDoc, len(srcs))
	for _, s := range srcs {
		d := byID[s.DocID]
		ref := sourceRef{DocID: s.DocID, Title: d.Title, Publisher: d.Publisher, PDFURL: d.PDFURL}
		if ref.Title == "" {
			// A document the registry does not describe still gets cited, by
			// id. Dropping the citation because a title is missing would hide
			// the provenance the page exists to show.
			ref.Title = s.DocID
		}
		base := pageTextBase(s.DocID)
		pages := make(map[string]clientPage, len(s.Pages))
		for _, p := range s.Pages {
			ref.Pages = append(ref.Pages, pageRef{
				Number:  p,
				PDFURL:  pdfPageURL(d.PDFURL, p),
				TextURL: base + pageTextFile(p),
			})
			// An empty records base is a document published with no records:
			// no link, not one pointing nowhere.
			records := ""
			if rb := recordsBase[s.DocID]; rb != "" {
				records = rb + RecordsFile(p)
			}
			pages[strconv.Itoa(p)] = clientPage{PDF: pdfPageURL(d.PDFURL, p), Text: base + pageTextFile(p), Records: records}
		}
		sources = append(sources, ref)
		clientDocs[s.DocID] = clientDoc{Title: ref.Title, Publisher: ref.Publisher, Pages: pages}
	}
	return sources, clientDocs
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

// unionSources merges the sources of every year a view publishes into one list,
// deduplicated and ordered.
//
// A union and not the opening year's, because site/app.js's citations() skips
// a fact whose doc_id is not in CONFIG.docs, in silence. A union within a view
// and never across views.
func unionSources(srcs []sourceMeta) []sourceMeta {
	pages := map[string]map[int]bool{}
	for _, s := range srcs {
		if pages[s.DocID] == nil {
			pages[s.DocID] = map[int]bool{}
		}
		for _, p := range s.Pages {
			pages[s.DocID][p] = true
		}
	}
	out := make([]sourceMeta, 0, len(pages))
	for _, id := range sortedKeys(pages) {
		ps := make([]int, 0, len(pages[id]))
		for p := range pages[id] {
			ps = append(ps, p)
		}
		slices.Sort(ps)
		out = append(out, sourceMeta{DocID: id, Pages: ps})
	}
	return out
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

// projectionRefs is every data file the site publishes, which is the whole set
// on every page: they are downloadable provenance, not this view's figures.
// One entry per file and not per document.
func projectionRefs(o *Options, ix ColumnIndex) []projectionRef {
	seen := map[string]bool{}
	var paths []string
	for _, name := range sortedKeys(o.Projections) {
		p := ix.PublishedPath(name)
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	slices.Sort(paths)
	refs := make([]projectionRef, 0, len(paths))
	for _, p := range paths {
		refs = append(refs, projectionRef{Path: p})
	}
	return refs
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
		Metadata:      doc.Metadata,
		Years:         years,
		Docs:          clientDocs,
		KindLabels:    kindLabels(),
		Wording:       defaultWording(),
		RenderTiers:   v.RenderTiers,
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
		Lede:       open.Lede,
		Basis:      open.Basis,
		Wording:    defaultWording(),
		ChartTitle: open.ChartTitle,
		Hero:       open.Hero,
		Figures:    open.Figures,
		Years:      years,
		Opens:      open.Stem,
		Facts:      open.Counts.Facts,
		Nodes:      open.Counts.Nodes,
		Links:      open.Counts.Links,
		Drill:      len(v.Steps) > 0,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and & to their \u form, so it cannot terminate the script element
		// or inject markup. The alternative, letting html/template escape a
		// string, would corrupt the JSON.
		ConfigJSON: template.JS(blob),
	}, nil
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

// pdfPageURL points at a page of the published PDF. The city's CMS serves the
// document from a numeric id and viewers honour the #page fragment, so the
// citation lands on the page rather than the cover.
func pdfPageURL(pdfURL string, page int) string {
	if pdfURL == "" {
		return ""
	}
	return fmt.Sprintf("%s#page=%d", pdfURL, page)
}

// remotePageTextBase is the directory URL of a document's committed page text
// in a browsable copy of the repository, so the path after the base is the
// repository's own layout rather than the exported site's.
func remotePageTextBase(browseURL, docID string) string {
	return strings.TrimSuffix(browseURL, "/") + "/data/extracted/" + docID + "/pages/"
}

// pageTextFile is page n's text file name, corpus.PagePath's last element.
func pageTextFile(page int) string { return path.Base(corpus.PagePath(page)) }

// basisLabelFor is the word a column chip and a cell tooltip print for a basis.
//
// It is deliberately NOT the basis itself. Basis is component 7 of fact.MakeID,
// so every figure in the ACFR's ten-year schedules carries mapping.BasisAudited
// in its identity and cannot be re-based without rewriting every one of those
// fact ids and moving facts/facts.jsonl. The page is a different question from
// the identity: those schedules sit in the section the document's own caveat
// quotes the ACFR calling "(Unaudited)", and a page may not print a word one of
// its own caveats withdraws two paragraphs below.
//
// The document's caveats decide, so the label and the refusal read one source
// rather than a list of page names that would have to be kept in step by hand.
// Only the audited basis is rewritten: a document may ship this caveat over a
// column on some other basis, and relabelling that one would be the same defect
// pointing the other way.
func basisLabelFor(caveats []caveatMeta, basis string) string {
	if basis != string(mapping.BasisAudited) {
		return basis
	}
	for _, c := range caveats {
		if c.ID == project.UnauditedCaveatID {
			return "unaudited"
		}
	}
	return basis
}

// trendsMetadata is as much of a revenue-trends document as the page renders.
//
// It is a separate struct from projectionMetadata rather than a superset of it,
// because the two documents genuinely differ: this one has columns and no
// fiscal year, and no headline at all. Sharing one struct would mean a page
// reading a field its document never publishes and getting a zero.
type trendsMetadata struct {
	GeneratedBy string            `json:"generated_by"`
	Scope       string            `json:"scope"`
	Sources     []sourceMeta      `json:"sources"`
	Columns     []trendColumnMeta `json:"columns"`
	Counts      trendCountsMeta   `json:"counts"`
	Caveats     []caveatMeta      `json:"caveats"`
}

// trendCountsMeta is the document's own accounting of itself, which
// reconcileSeriesCounts holds the rendered page to.
type trendCountsMeta struct {
	Facts  int `json:"facts"`
	Series int `json:"series"`
	Points int `json:"points"`
}

// trendColumnMeta is one column as the document declares it.
type trendColumnMeta struct {
	FiscalYear      int    `json:"fiscal_year"`
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
	ComparableGroup string `json:"comparable_group"`
}

// trendPoint is one published point as this page reads it.
type trendPoint struct {
	FiscalYear  int    `json:"fiscal_year"`
	Basis       string `json:"basis"`
	AmountCents int64  `json:"amount_cents"`
	DocID       string `json:"doc_id"`
	Page        int    `json:"page"`
}

// trendsBody is the series this page tabulates.
type trendsBody struct {
	Series []struct {
		Label         string       `json:"label"`
		Fund          *int         `json:"fund"`
		FundName      string       `json:"fund_name"`
		FundGroup     string       `json:"fund_group"`
		Kind          string       `json:"kind"`
		CategoryLabel string       `json:"category_label"`
		Points        []trendPoint `json:"points"`
	} `json:"series"`
}

// buildTrendsPage renders the revenue-trends view.
//
// EVERY FIGURE IS READ OUT OF THE DOCUMENT AND NONE IS COMPUTED HERE. The
// packager's standing rule (see the package doc) is that it does not recompute
// what a projection published, and a table is the case where that is most
// tempting to break: a total column, a growth percentage, a per-fund subtotal
// would each be one line. None of them is published, and the contract says why —
// growth from an actual to an adopted figure is not a quantity this project can
// compute, and a total this document does not carry is a total the city did not
// print on the page these series came from.
func buildTrendsPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (trendsPageData, error) {
	caveatsPath := caveatsPathOf(o)
	raw := o.Projections[v.Projection]
	doc, err := decodeDocument(v.Projection, raw)
	if err != nil {
		return trendsPageData{}, err
	}
	var meta trendsMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return trendsPageData{}, fmt.Errorf("decode %s metadata: %w", v.Projection, err)
	}
	if len(meta.Columns) == 0 {
		return trendsPageData{}, fmt.Errorf("%s metadata publishes no columns", v.Projection)
	}
	var body trendsBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return trendsPageData{}, fmt.Errorf("decode %s series: %w", v.Projection, err)
	}

	columns := make([]columnRef, 0, len(meta.Columns))
	for i, c := range meta.Columns {
		columns = append(columns, columnRef{
			Label: c.FiscalYearLabel,
			Basis: basisLabelFor(meta.Caveats, c.Basis),
			Group: c.ComparableGroup,
			// The FIRST column of a group does not start a boundary a reader
			// could carry a comparison across; every later one does.
			New: i > 0 && c.ComparableGroup != meta.Columns[i-1].ComparableGroup,
		})
	}

	series := make([]seriesRef, 0, len(body.Series))
	rendered := 0
	for _, s := range body.Series {
		fund := s.FundName
		if fund == "" {
			// A fund the registry does not name renders as its number, which is
			// what the document's own fallback intends: the number is on the
			// page and is never nothing. A series under no fund at all has no
			// row header in this view, and is refused rather than headed "".
			if s.Fund == nil {
				return trendsPageData{}, fmt.Errorf("%s series %q names no fund and no "+
					"fund name; this view's row header is the fund", v.Projection, s.Label)
			}
			fund = fmt.Sprintf("Fund %d", *s.Fund)
		}
		cells, placed, err := buildCells(s.Points, columns, meta.Columns, pageTextBase)
		if err != nil {
			return trendsPageData{}, fmt.Errorf("%s series %q: %w", v.Projection, s.Label, err)
		}
		rendered += placed
		series = append(series, seriesRef{
			Label:    s.Label,
			Fund:     fund,
			Group:    s.FundGroup,
			Category: s.CategoryLabel,
			Cells:    cells,
			Mark:     buildMark(cells, columns),
		})
	}

	if err := reconcileSeriesCounts(v.Projection, rendered, len(body.Series), meta.Counts); err != nil {
		return trendsPageData{}, err
	}

	sources, _ := sourcesFor(meta.Sources, byID, pageTextBase, o.RecordsBase)
	refs := projectionRefs(o, ix)
	title := v.Title
	if title == "" {
		title = "City of Livermore revenue by fund"
	}
	return trendsPageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  refs,
			DataPath:     ix.PublishedPath(v.Projection),
			Scope:        meta.Scope,
			Caveats:      caveatRefs(meta.Caveats, v.Projection, caveatsPath),
			CaveatsPath:  caveatsPath,
		},
		Columns: columns,
		Series:  series,
		Facts:   meta.Counts.Facts,
		Count:   meta.Counts.Series,
	}, nil
}

// reconcileSeriesCounts refuses a series document whose own counts disagree
// with the cells the page just laid out (fisc-4j5).
//
// `fisc export` runs no checks by design and this is the one thing it insists
// on. A series legitimately SHORT A COLUMN still reconciles: internal/check
// declares that state through incompleteSeries, and a short series carries
// fewer points and says so in its own counts.
//
// rendered IS HELD TO counts.facts AS WELL AS counts.points, because facts is
// the number the lede prints and the two are computed independently -- facts
// off the projection's selection, points off the series actually built. A
// document that dropped a series publishes points below facts, and reconciling
// only points once shipped a lede claiming 924 figures over a table carrying
// 920 (fisc-5tu).
func reconcileSeriesCounts(projection string, rendered, series int, counts trendCountsMeta) error {
	if rendered != counts.Points {
		return fmt.Errorf(
			"%s declares counts.points %d and its series carry %d; the document has "+
				"lost a figure between the projection that built it and this page",
			projection, counts.Points, rendered)
	}
	if rendered != counts.Facts {
		return fmt.Errorf(
			"%s prints counts.facts %d in its lede and its series carry %d cells; "+
				"the page would claim more figures than it shows",
			projection, counts.Facts, rendered)
	}
	if series != counts.Series {
		return fmt.Errorf(
			"%s declares counts.series %d and carries %d",
			projection, counts.Series, series)
	}
	return nil
}

// historyPageData is the history template's input. Like trendsPageData it
// carries no ConfigJSON and its page loads no app.js: every figure is rendered
// server-side, so the table works with JavaScript off.
type historyPageData struct {
	chrome
	Columns []columnRef
	// Sections are the printed blocks, in the order the view declares them,
	// each holding its rows in document order.
	Sections []historySection
	// HeadSpan is every column a section heading spans: the label, category
	// and mark columns plus one per printed column.
	HeadSpan int
	Facts    int
	Count    int
}

// historySection is one printed block of the table.
type historySection struct {
	Heading string
	Series  []seriesRef
}

// buildHistoryPage renders one ACFR ten-year view.
//
// buildTrendsPage's rules apply whole: every figure is read out of the
// document, none is computed here, and the counts must reconcile. What is new
// is the grouping — each series lands in the [View.Sections] entry matching
// its (kind, fund_group) exactly, and both a series no section claims and a
// section claiming no series are refused, because either one is a heading
// telling a reader something the document does not say. A section that closes
// itself with [Section.Rows] adds the refusal the key match cannot make: a
// series its key claims but its rows do not name.
func buildHistoryPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (historyPageData, error) {
	caveatsPath := caveatsPathOf(o)
	raw := o.Projections[v.Projection]
	doc, err := decodeDocument(v.Projection, raw)
	if err != nil {
		return historyPageData{}, err
	}
	var meta trendsMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return historyPageData{}, fmt.Errorf("decode %s metadata: %w", v.Projection, err)
	}
	if len(meta.Columns) == 0 {
		return historyPageData{}, fmt.Errorf("%s metadata publishes no columns", v.Projection)
	}
	var body trendsBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return historyPageData{}, fmt.Errorf("decode %s series: %w", v.Projection, err)
	}

	columns := make([]columnRef, 0, len(meta.Columns))
	for i, c := range meta.Columns {
		columns = append(columns, columnRef{
			Label: c.FiscalYearLabel,
			Basis: basisLabelFor(meta.Caveats, c.Basis),
			Group: c.ComparableGroup,
			New:   i > 0 && c.ComparableGroup != meta.Columns[i-1].ComparableGroup,
		})
	}

	// sectionKey is the (kind, fund_group) a series is claimed by. It is not
	// [Section] itself because Rows makes that struct uncomparable, and the
	// identity of a section deliberately excludes both its heading and its
	// rows: two sections may not share a key however differently they would
	// render.
	type sectionKey struct {
		kind, fundGroup string
	}
	sections := make([]historySection, len(v.Sections))
	at := make(map[sectionKey]int, len(v.Sections))
	for i, s := range v.Sections {
		sections[i] = historySection{Heading: s.Heading}
		key := sectionKey{kind: s.Kind, fundGroup: s.FundGroup}
		if _, dup := at[key]; dup {
			return historyPageData{}, fmt.Errorf(
				"view %q declares two sections for kind %q fund group %q; a series "+
					"cannot land under both headings", v.Path, s.Kind, s.FundGroup)
		}
		at[key] = i
	}

	named := make([]map[string]bool, len(v.Sections))
	rendered := 0
	for _, s := range body.Series {
		i, ok := at[sectionKey{kind: s.Kind, fundGroup: s.FundGroup}]
		if !ok {
			return historyPageData{}, fmt.Errorf(
				"%s series %q is kind %q in fund group %q, which no section of view %q "+
					"declares; a row under the wrong printed heading is a claim the "+
					"schedule does not make", v.Projection, s.Label, s.Kind, s.FundGroup, v.Path)
		}
		label := s.Label
		if rows := v.Sections[i].Rows; rows != nil {
			display, claims := rows[s.Label]
			if !claims {
				return historyPageData{}, fmt.Errorf(
					"%s series %q is kind %q in fund group %q, which lands in section %q "+
						"of view %q, and that section's rows do not name it; a heading "+
						"that enumerates its rows must refuse a new one rather than "+
						"absorb it", v.Projection, s.Label, s.Kind, s.FundGroup,
					v.Sections[i].Heading, v.Path)
			}
			if display != "" {
				label = display
			}
			if named[i] == nil {
				named[i] = make(map[string]bool, len(rows))
			}
			named[i][s.Label] = true
		}
		cells, placed, err := buildCells(s.Points, columns, meta.Columns, pageTextBase)
		if err != nil {
			return historyPageData{}, fmt.Errorf("%s series %q: %w", v.Projection, s.Label, err)
		}
		rendered += placed
		sections[i].Series = append(sections[i].Series, seriesRef{
			Label:    label,
			Group:    s.FundGroup,
			Category: s.CategoryLabel,
			Cells:    cells,
			Mark:     buildMark(cells, columns),
		})
	}
	// The counts first: a document that disagrees with itself is refused before
	// any question about how this view groups it.
	if err := reconcileSeriesCounts(v.Projection, rendered, len(body.Series), meta.Counts); err != nil {
		return historyPageData{}, err
	}
	for i, s := range sections {
		if len(s.Series) == 0 {
			return historyPageData{}, fmt.Errorf(
				"view %q declares section %q (kind %q, fund group %q) and %s carries no "+
					"such series; a heading over nothing says the schedule prints a block "+
					"it does not", v.Path, s.Heading, v.Sections[i].Kind,
				v.Sections[i].FundGroup, v.Projection)
		}
	}
	for i, s := range v.Sections {
		for _, key := range slices.Sorted(maps.Keys(s.Rows)) {
			if !named[i][key] {
				return historyPageData{}, fmt.Errorf(
					"view %q section %q names row %q and %s carries no such series; "+
						"a name that matches nothing checks nothing",
					v.Path, s.Heading, key, v.Projection)
			}
		}
	}

	sources, _ := sourcesFor(meta.Sources, byID, pageTextBase, o.RecordsBase)
	refs := projectionRefs(o, ix)
	title := v.Title
	if title == "" {
		title = "City of Livermore ten-year history"
	}
	return historyPageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  refs,
			DataPath:     ix.PublishedPath(v.Projection),
			Scope:        meta.Scope,
			Caveats:      caveatRefs(meta.Caveats, v.Projection, caveatsPath),
			CaveatsPath:  caveatsPath,
		},
		Columns:  columns,
		Sections: sections,
		HeadSpan: 3 + len(columns),
		Facts:    meta.Counts.Facts,
		Count:    meta.Counts.Series,
	}, nil
}

// provenancePageData is the provenance index's own shape.
type provenancePageData struct {
	chrome
	Rows      []provenanceRow
	Downloads []downloadRef
	Documents int
	Pages     int
	Records   int
}

// caveatsPageData is every published document's caveats, in full.
//
// GROUPED BY DOCUMENT AND NOT BY CAVEAT, which is forced rather than chosen.
// One id can carry different text in different documents -- transfer-legs-
// unpaired has three sentences and the FY2027 spine carries a contested-total
// entry FY2026 does not -- so a page keyed on id alone would have to pick one
// text and would be wrong about the others.
type caveatsPageData struct {
	chrome
	Documents []caveatDocument
	// Count is every caveat on the page, across documents, so the lede can say
	// how many without the template summing a nested range.
	Count int
}

// caveatDocument is one published document's section of the caveats page.
type caveatDocument struct {
	Stem     string
	Label    string
	DataPath string
	Entries  []caveatEntry
	// Drawn is whether a view that DRAWS A CHART renders this document, so the
	// page can promise a chart flag only where there is a chart.
	//
	// NOT "any view renders it", which is what this said and what
	// templateDrawsAChart exists to correct: trends.html renders revenue-trends
	// and ships no app.js, so a caveat on that document can be listed and never
	// chipped on a mark.
	//
	// False for every document no view renders (the fund-flows columns
	// unviewedDocuments declares) and for every one rendered as a TABLE —
	// revenue-trends and the two ACFR history documents. This page lists all
	// of their caveats anyway, because a caveat is owed to whoever fetches the
	// file. What it must not do is tell that reader the charts flag these
	// marks.
	Drawn bool
}

// caveatEntry is one caveat, rendered whole. Anchor is what every other page's
// summary links to.
type caveatEntry struct {
	ID        string
	Anchor    string
	Summary   string
	Text      string
	AppliesTo []string
}

// provenanceRow is one published locator, with both halves of its citation
// already composed.
type provenanceRow struct {
	DocID    string
	DocTitle string
	Page     int
	Records  int
	Bytes    int
	Note     string
	DataURL  string
	PDFURL   string
	TextURL  string
}

// downloadRef is one whole-store artifact offered for download.
type downloadRef struct {
	Path  string
	Label string
	Note  string
	Bytes int
}

// buildProvenancePage renders the index of the published fact store.
//
// IT COMPOSES NO URL ITSELF, and that is the one thing worth guarding here.
// Every PDF and page-text link comes back out of sourcesFor, the same function
// the other three pages' footers go through, so pdfPageURL and pageTextFile are
// reached by exactly one path in this package. It matters beyond tidiness: the
// PDF link must target the city's canonical URL with #page=N and never a forge's
// raw host, which serves LFS pointer text rather than the document. A second
// composition here would be a second place for that to go wrong, on the one
// page whose entire purpose is that its links resolve.
//
// THE DATA LINK IS THE EXCEPTION AND IS NOT COMPOSED EITHER: it is
// PageIndexEntry.Data verbatim, the path the caller says it wrote the records
// to, which Options.validate has already checked against the files being
// written. This package does not know how that path is built and must not
// learn -- the locator-to-URL rule belongs to whoever produced the records.
func buildProvenancePage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(docID string) string) (provenancePageData, error) {
	if len(o.PageIndex) == 0 {
		// A page listing nothing is not an empty state, it is a page that
		// should not have been asked for: views() adds this one only when
		// there is an index, so reaching here means two callers disagree.
		return provenancePageData{}, fmt.Errorf(
			"view %q renders the provenance index and Options carries no page index", v.Path)
	}

	// The sources this page cites ARE its rows, so they are built from the
	// index rather than read out of a document -- and then put through the
	// same union and the same composition every other page uses.
	byDoc := map[string][]int{}
	order := []string{}
	for _, e := range o.PageIndex {
		if _, ok := byDoc[e.DocID]; !ok {
			order = append(order, e.DocID)
		}
		byDoc[e.DocID] = append(byDoc[e.DocID], e.Page)
	}
	metas := make([]sourceMeta, 0, len(order))
	for _, id := range order {
		metas = append(metas, sourceMeta{DocID: id, Pages: byDoc[id]})
	}
	sources, _ := sourcesFor(unionSources(metas), byID, pageTextBase, o.RecordsBase)

	// Index the composed refs so each row can take its own, rather than
	// recomposing them.
	type key struct {
		doc  string
		page int
	}
	refs := map[key]pageRef{}
	titles := map[string]string{}
	for _, s := range sources {
		titles[s.DocID] = s.Title
		for _, p := range s.Pages {
			refs[key{s.DocID, p.Number}] = p
		}
	}

	rows := make([]provenanceRow, 0, len(o.PageIndex))
	records := 0
	for _, e := range o.PageIndex {
		ref, ok := refs[key{e.DocID, e.Page}]
		if !ok {
			return provenancePageData{}, fmt.Errorf(
				"page index entry %s p%d composed no citation", e.DocID, e.Page)
		}
		records += e.Records
		rows = append(rows, provenanceRow{
			DocID: e.DocID, DocTitle: titles[e.DocID], Page: e.Page,
			Records: e.Records, Bytes: e.Bytes, Note: e.Note,
			DataURL: e.Data, PDFURL: ref.PDFURL, TextURL: ref.TextURL,
		})
	}

	downloads := make([]downloadRef, 0, len(o.Downloads))
	for _, d := range o.Downloads {
		// A conversion rather than a field-by-field copy: the two types have
		// the same shape ON PURPOSE -- Download is the caller's vocabulary and
		// downloadRef is the template's -- and a conversion cannot silently
		// drop a field the day one of them gains one.
		downloads = append(downloads, downloadRef(d))
	}

	title := v.Title
	if title == "" {
		title = "City of Livermore budget: every published figure"
	}
	return provenancePageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			Sources:     sources,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
			// NO DataPath. Every other page names the projection it draws;
			// this one draws none, and pointing it at an unrelated document
			// would be a false statement about where its figures came from.
		},
		Rows:      rows,
		Downloads: downloads,
		Documents: len(order),
		Pages:     len(rows),
		Records:   records,
	}, nil
}

// buildCaveatsPage lists every published document's caveats, in full, with an
// anchor per (document, caveat) that every other page's summary links to.
//
// It names no projection: it is an index across documents. It refuses an
// empty page, a caveat with no id, summary or text, and two entries claiming
// one anchor -- a repeated id in one document, or the "--" separator making
// two compositions ambiguous. project.ValidateCaveats never sees a document
// decoded from bytes, which is every document here.
func buildCaveatsPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (caveatsPageData, error) {
	caveatsPath := caveatsPathOf(o)
	// Which documents a chart actually draws: through a view's projection,
	// its year stems, or its steps' documents.
	drawn := map[string]bool{}
	for _, v := range o.views() {
		// A view that draws no chart flags nothing, whatever it renders.
		if !templateDrawsAChart(v.Template) {
			continue
		}
		if v.Projection != "" {
			drawn[v.Projection] = true
		}
		for _, stem := range v.YearStems {
			drawn[stem] = true
		}
		// A document a chart opens into is drawn too, resolved per year.
		for _, stem := range v.DrawnStems(ix) {
			drawn[stem] = true
		}
	}
	anchors := map[string]string{}
	docs := make([]caveatDocument, 0, len(o.Projections))
	count := 0
	for _, stem := range sortedKeys(o.Projections) {
		var dc documentCaveats
		if err := json.Unmarshal(o.Projections[stem], &dc); err != nil {
			return caveatsPageData{}, fmt.Errorf("decode %s caveats: %w", stem, err)
		}
		if len(dc.Metadata.Caveats) == 0 {
			continue
		}
		entries := make([]caveatEntry, 0, len(dc.Metadata.Caveats))
		for _, c := range dc.Metadata.Caveats {
			switch {
			case c.ID == "":
				return caveatsPageData{}, fmt.Errorf(
					"%s carries a caveat with no id, and the id is the anchor every other page links to", stem)
			case c.Summary == "":
				return caveatsPageData{}, fmt.Errorf(
					"%s caveat %q has no summary, and the summary is what the other pages show in its place", stem, c.ID)
			case c.Text == "":
				return caveatsPageData{}, fmt.Errorf(
					"%s caveat %q has no text, so this page would publish a heading over nothing", stem, c.ID)
			}
			a := caveatAnchor(stem, c.ID)
			if prev, dup := anchors[a]; dup {
				return caveatsPageData{}, fmt.Errorf(
					"caveat anchor %q is claimed by both %s and %s; an anchor is a published "+
						"URL fragment and a link to a repeated one lands on whichever the browser finds first",
					a, prev, stem)
			}
			anchors[a] = stem
			entries = append(entries, caveatEntry{
				ID: c.ID, Anchor: a, Summary: c.Summary, Text: c.Text, AppliesTo: c.AppliesTo,
			})
		}
		count += len(entries)
		label := dc.Metadata.FiscalYearLabel
		if label != "" && dc.Metadata.Basis != "" {
			label += " " + dc.Metadata.Basis
		}
		docs = append(docs, caveatDocument{
			Stem:     stem,
			Label:    label,
			DataPath: ix.PublishedPath(stem),
			Entries:  entries,
			Drawn:    drawn[stem],
		})
	}
	if count == 0 {
		return caveatsPageData{}, fmt.Errorf(
			"view %q renders the caveats index and no published document carries a caveat, "+
				"so the site would ship a nav entry to a page with nothing on it", v.Path)
	}

	title := v.Title
	if title == "" {
		title = "What this site's figures do not say"
	}
	return caveatsPageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
			CaveatsPath: caveatsPath,
			// NO Sources, AND THAT IS THE POINT rather than an omission.
			// TestEachViewsFooterCitesItsOwnSources enforces that a page must
			// not advertise pages it never showed a figure from, and this page
			// shows no figures at all -- it publishes sentences about
			// documents, each of which links to the document itself. Naming
			// the union of every document's pages here would be this site's
			// broadest false provenance claim.
			//
			// NO DataPath either, for buildProvenancePage's reason: this page
			// draws no projection.
		},
		Documents: docs,
		Count:     count,
	}, nil
}

// buildMark draws one row's cells as bars, scaled to that row's own largest
// figure.
//
// THE SCALE IS PER ROW, AND THE CAPTION HAS TO SAY SO. These 231 rows span
// $47,000 to $58,000,000, so one scale across the table renders all but a
// handful of rows as an invisible line — a mark that shows nothing is not a
// conservative choice, it is a blank column pretending to be information. Per
// row, every mark is legible and none is comparable with its neighbour, which
// is a real limit and belongs in the words beside it rather than in a comment
// only a maintainer reads.
//
// It is also the honest scale for what this document IS. The four columns are
// not one measurement — an actual, a re-forecast, and two years of one adopted
// budget — so the comparison the mark supports is the one WITHIN a row, which
// is the only one the contract permits at all.
//
// NO FIGURE IS COMPUTED HERE. The bars' heights are geometry, not money: the
// only arithmetic is a proportion of a box, and the numbers a reader takes away
// are the linked figures in the same row. internal/export may not recompute a
// figure a document publishes, and this does not.
func buildMark(cells []cellRef, cols []columnRef) markRef {
	// The row's largest magnitude, which is the only quantity the scale needs.
	// Magnitude and not value, so a contra row scales against its own biggest
	// number rather than against zero.
	var largest int64
	for _, c := range cells {
		if c.Missing {
			continue
		}
		if m := abs64(c.Cents); m > largest {
			largest = m
		}
	}

	negative := false
	for _, c := range cells {
		if !c.Missing && c.Cents < 0 {
			negative = true
			break
		}
	}
	// A row with a contra cell splits the box; one without gives its whole
	// height to the positives.
	up, baseline := markHeight, markHeight
	if negative {
		up, baseline = markHeight/2, markHeight/2
	}

	m := markRef{
		Width:    len(cols)*markBarWidth + max(len(cols)-1, 0)*markBarGap,
		Height:   markHeight,
		Baseline: baseline,
	}
	// The rule sits in the middle of the gap BEFORE a column that starts a new
	// comparable group, which is where the table's own border-left falls. The
	// first column can never carry one -- there is nothing to its left to be
	// separated from -- and columnRef.New is false there, so no guard is needed
	// beyond reading the flag the header already reads.
	for i, c := range cols {
		if c.New && i > 0 {
			m.Rules = append(m.Rules, i*(markBarWidth+markBarGap)-markBarGap/2-1)
		}
	}
	drawn := 0
	for i, c := range cells {
		if c.Missing {
			// No bar at all. The slot stays empty, which is how an absence is
			// told from a published zero further down.
			continue
		}
		drawn++
		x := i * (markBarWidth + markBarGap)
		bar := barRef{
			X: x, Width: markBarWidth, Negative: c.Cents < 0,
			Title: cols[i].Label + " " + cols[i].Basis + ": " + c.Value,
		}
		switch {
		case c.Cents == 0:
			// A printed dash is a figure the city published, so it is drawn --
			// as a dot ON the baseline, which is what zero looks like.
			bar.Zero = true
			bar.CX, bar.Y, bar.R = x+markBarWidth/2, baseline, markZeroRadius
		case largest == 0:
			// Unreachable while any cell is non-zero, and it is here so that a
			// row of published zeros cannot divide by zero on the way to
			// drawing nothing.
			bar.Zero = true
			bar.CX, bar.Y, bar.R = x+markBarWidth/2, baseline, markZeroRadius
		default:
			h := int(abs64(c.Cents) * int64(up) / largest)
			if h < markMinBar {
				h = markMinBar
			}
			if c.Cents < 0 {
				bar.Y, bar.Height = baseline, h
			} else {
				bar.Y, bar.Height = baseline-h, h
			}
		}
		m.Bars = append(m.Bars, bar)
	}
	m.Label = markLabel(drawn, len(cols))
	return m
}

// markLabel is the mark's accessible name.
//
// It says what the mark IS rather than what it shows, because what it shows is
// the row's own figures and a screen reader is about to read them as text. A
// label repeating four dollar amounts would be the same information twice, and
// the second time without the column headers that make it meaningful.
func markLabel(drawn, columns int) string {
	if drawn == columns {
		return fmt.Sprintf("This row's %d figures, drawn to the row's own scale", columns)
	}
	return fmt.Sprintf("%d of this row's %d columns carry a figure, drawn to the row's own scale",
		drawn, columns)
}

// abs64 is the magnitude of a signed cents value.
//
// Spelled here rather than reached for, and it does not guard math.MinInt64 --
// which cannot arise: these are cents read off a printed page, and
// internal/amount refuses anything that is not one of a closed set of shapes.
func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// buildCells lays one series' points out against the document's COLUMNS.
//
// POSITIONAL WAS WRONG AND THE FAILURE IS THE ONE THIS PROJECT EXISTS TO
// PREVENT. An earlier version emitted one cell per point, in the order the
// document listed them, against headers built from metadata.columns. Those two
// lists agree only while every series is complete -- and a series short a column
// is a state the project explicitly supports: internal/check declares it through
// incompleteSeries, and `fisc export` runs no checks, so a document with a gap
// can be packaged. Under the positional version every figure after the gap slid
// one column left, printing FY2026's money under FY2025 with a citation link to
// FY2026's page. A plausible wrong value, published, with provenance that
// disagrees with it.
//
// So the cells are keyed on (fiscal_year, basis) -- the column identity the
// document itself publishes -- and a column with no point gets a rendered gap
// rather than a shifted neighbour. A gap a reader can see is the honest form of
// something the document does not say.
//
// A point in NO published column cannot occur through internal/project, which
// filters facts on the same column set. If one ever arrives it is dropped here,
// so this reports how many points it actually PLACED and the caller reconciles
// that against the document's own counts.points. That reconciliation used to be
// promised by this comment and performed by nobody: counts.points was decoded
// and read nowhere, so a 1-column document carrying a 2-point series exported
// successfully, rendered one cell, lost the other figure silently, and printed
// a lede saying "2 figures in all" (fisc-4j5).
func buildCells(points []trendPoint, columns []columnRef, meta []trendColumnMeta,
	pageTextBase func(string) string,
) ([]cellRef, int, error) {
	type key struct {
		year  int
		basis string
	}
	byColumn := make(map[key]trendPoint, len(points))
	for _, p := range points {
		k := key{p.FiscalYear, p.Basis}
		// TWO POINTS IN ONE COLUMN IS THE SAME DEFECT FROM THE OTHER SIDE. A
		// plain assignment keeps the last and loses the first as quietly as a
		// dropped column does, and the two need not even agree -- so the count
		// below would still reconcile while a figure had vanished.
		if prev, dup := byColumn[k]; dup {
			return nil, 0, fmt.Errorf(
				"two points publish %s: %s and %s; one of them would be dropped",
				fact.ColumnLabel(p.FiscalYear, p.Basis), amount.Cents(prev.AmountCents).Dollars(), amount.Cents(p.AmountCents).Dollars())
		}
		byColumn[k] = p
	}

	out := make([]cellRef, 0, len(columns))
	placed := 0
	for i, c := range meta {
		cell := cellRef{New: columns[i].New}
		p, ok := byColumn[key{c.FiscalYear, c.Basis}]
		if !ok {
			// The em dash is the city's own mark for a cell it did not print,
			// and this is not that: this is a row the schedule does not carry in
			// this column at all. The title says which, because the two are
			// indistinguishable on the page otherwise and a published zero is a
			// fact while an absence is not (AGENTS.md, Provenance invariants).
			cell.Missing = true
			cell.Value = "—"
			out = append(out, cell)
			continue
		}
		cell.Value = amount.Cents(p.AmountCents).Dollars()
		cell.Cents = p.AmountCents
		cell.Negative = p.AmountCents < 0
		cell.Page = p.Page
		cell.Href = pageTextBase(p.DocID) + pageTextFile(p.Page)
		out = append(out, cell)
		placed++
	}
	return out, placed, nil
}
