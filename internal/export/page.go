package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// The page templates inside the site asset tree, one per view.
//
// They are named here rather than in the caller because a template is an ASSET
// of this package's embedded tree, and a caller naming one would be reaching
// into a directory it does not own. What the caller chooses is which view uses
// which, by name.
const (
	SankeyTemplate    = "index.html.tmpl"
	TrendsTemplate    = "revenue.html.tmpl"
	DrilldownTemplate = "drilldown.html.tmpl"
	// ProvenanceTemplate renders the fact store's index. It is the one
	// template that renders no projection document -- see
	// templateRendersADocument.
	ProvenanceTemplate = "provenance.html.tmpl"
)

// SchemaVersion is the projection schema this packager understands.
//
// It is deliberately a second copy of project.SchemaVersion and not an import
// of it. This package consumes projections as filename stem -> JSON bytes and
// does not import internal/project (see the package doc); reaching for the
// producer's constant here to save a line would put back the seam the package
// exists to hold open.
//
// What makes the duplication safe is TestSchemaVersionIsPinnedToTheProducer,
// which asserts the two are equal. The test is load-bearing, not decorative:
// without it the constants drift the first time the producer's version moves,
// and this gate then waves through exactly the document it exists to refuse.
// The same test pins the client's copy in site/app.js, which nothing compiles
// against at all.
const SchemaVersion = 1

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
	// The refusal names the document it read, not the primary one. It used to
	// say PrimaryProjection unconditionally, which was harmless while there was
	// one document and is a wrong signpost the moment there are two: an
	// operator sent to sankey.json to fix sankey-2027.json finds nothing wrong
	// with it.
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

// sourceMeta is one cited document in any projection's metadata.
type sourceMeta struct {
	DocID string `json:"doc_id"`
	Pages []int  `json:"pages"`
}

// documentSources is as much of ANY document as the citation union needs.
//
// It is deliberately shape-blind: metadata.sources is the one block every
// projection carries whatever its body is, so the set of pages a site must ship
// can be collected across views without this package knowing whether a given
// document holds nodes and links or series and points. That is what makes
// fisc-fjy's fix general rather than a second special case beside the year loop.
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
// decodeSankey is about: fiscal_year_label, basis and headline are singular or
// spine-specific, and a trends document carries none of them and is not
// defective for that (internal/project/document.go says so in writing).
type projectionMetadata struct {
	GeneratedBy     string       `json:"generated_by"`
	FiscalYear      int          `json:"fiscal_year"`
	FiscalYearLabel string       `json:"fiscal_year_label"`
	Basis           string       `json:"basis"`
	Scope           string       `json:"scope"`
	Currency        string       `json:"currency"`
	Units           string       `json:"units"`
	Sources         []sourceMeta `json:"sources"`
	Headline        headline     `json:"headline"`
	Counts          struct {
		Facts int `json:"facts"`
		Nodes int `json:"nodes"`
		Links int `json:"links"`
	} `json:"counts"`
	Caveats []caveatMeta `json:"caveats"`
}

// drilldownMetadata is the decoded metadata block of a DRILL-DOWN document.
//
// IT IS NOT projectionMetadata WITH A FIELD RENAMED, and the difference is the
// reason this type exists rather than a widened one.
//
//   - scopes is a LIST. A document of two schedules writing scopes[0] into a
//     singular scope would publish one of them as the whole of it.
//   - There is NO headline, deliberately. The document holds the same money at
//     more than one grain, so "the total" is ambiguous and no key disambiguates
//     it; docs/general-fund-drilldown-contract.md's "No headline" section is the
//     argument. decodeSankey's refusal of a headline-less document is correct
//     and stays; this is the shape that has no business being asked.
//   - counts carries six keys, not three, and facts_cited is not the spine's.
//     Only the three the page renders are read; the rest are ignored by
//     encoding/json, which is what we want here rather than a refusal.
type drilldownMetadata struct {
	GeneratedBy     string       `json:"generated_by"`
	Scopes          []string     `json:"scopes"`
	FiscalYear      int          `json:"fiscal_year"`
	FiscalYearLabel string       `json:"fiscal_year_label"`
	Basis           string       `json:"basis"`
	Sources         []sourceMeta `json:"sources"`
	Counts          struct {
		Facts int `json:"facts"`
		Nodes int `json:"nodes"`
		Links int `json:"links"`
	} `json:"counts"`
	Caveats []caveatMeta `json:"caveats"`
}

// caveatMeta is a decoded caveat.
//
// A SEPARATE TYPE FROM project.Caveat, like every other decode struct in this
// file, because this package consumes projections as bytes and does not import
// internal/project. The field set is the contract, and it is pinned by
// TestCaveatMetaKeysAreTheOnesTheDocumentCarries rather than by the two
// declarations happening to agree.
type caveatMeta struct {
	ID        string   `json:"id"`
	Summary   string   `json:"summary"`
	Text      string   `json:"text"`
	AppliesTo []string `json:"applies_to"`
}

// headline is the projection's published totals, in cents.
type headline struct {
	AllFundsGrossRevenueCents     int64 `json:"all_funds_gross_revenue_cents"`
	AllFundsGrossExpenditureCents int64 `json:"all_funds_gross_expenditure_cents"`
	ExternalRevenueCents          int64 `json:"external_revenue_cents"`
	ExternalExpenditureCents      int64 `json:"external_expenditure_cents"`
	InternalTransferInCents       int64 `json:"internal_transfer_in_cents"`
	InternalTransferOutCents      int64 `json:"internal_transfer_out_cents"`
	NaiveExpenditureCents         int64 `json:"naive_expenditure_cents"`
	TransferResidualCents         int64 `json:"transfer_residual_cents"`
}

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

// projectionRef names a projection file the page ships.
type projectionRef struct {
	Name string
	Path string
}

// yearView is one published fiscal year's worth of everything the page states
// in words rather than draws.
//
// IT IS BUILT IN GO FOR EVERY YEAR, not just the one the page opens on, and the
// client swaps between them. The alternative was for app.js to rebuild the
// tiles itself on a year switch, which would put the prose — "The wrong answer:
// summing the expenditure column counts transfers between funds twice" — in two
// languages and let them drift. Here there is one implementation and the client
// only chooses.
//
// It also keeps the no-JavaScript headline: the template renders the opening
// year's tiles into the HTML exactly as before, and these are what the toggle
// reaches for afterwards.
type yearView struct {
	Year  int    `json:"year"`
	Label string `json:"label"`
	Stem  string `json:"stem"`
	Path  string `json:"path"`
	Basis string `json:"basis"`
	// Title is this year's <title>, built here rather than composed in the
	// client. app.js used to assemble it from a literal copied out of
	// sankeyTitle below, which is the one string paintYearWords wrote that the
	// packager had not built -- and it overwrote a caller's own Title without a
	// word. See sankeyTitle for why the caller's words survive the switch.
	Title   string       `json:"title"`
	Hero    figure       `json:"hero"`
	Figures []figure     `json:"figures"`
	Caveats []caveatMeta `json:"caveats"`
	Counts  countsRef    `json:"counts"`
	// ChartTitle is the <title> inside the SVG -- the chart's accessible name,
	// and a different string from Title, which is the document's.
	//
	// BUILT HERE FOR THE REASON Title IS. paintYearWords composed this from a
	// literal naming a Sankey "of the <year> <basis> budget", which is right on
	// the spine and wrong on any other chart: the drill-down's template names a
	// diagram by fund and division, and the first year repaint replaced it, so
	// two different charts announced themselves identically to a screen reader.
	ChartTitle string `json:"chart_title"`
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
	FiscalYearLabel string
	Basis           string
	Hero            figure
	Figures         []figure
	// Years is every published year, opening year first. The template renders
	// Years[0]'s tiles and caveats into the HTML and lists the rest as a
	// selector; app.js swaps between them without refetching the page.
	Years []yearView
	Facts int
	Nodes int
	Links int
	// ConfigJSON is window.FISC_CONFIG. json.Marshal escapes <, > and & to
	// their \u form, so the blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// drilldownPageData is the drill-down template's input.
//
// IT CARRIES Scopes RATHER THAN WIDENING chrome.Scope. chrome is embedded in
// both other page types and its Scope is rendered in both their footers, so
// making it a list to suit a third page edits two shipped pages and the prose
// pinned about them. chrome.Scope is left empty here and this template never
// asks for it.
//
// IT CARRIES NO Hero AND NO Figures, and that is the page's design rather than
// an omission. See the template.
type drilldownPageData struct {
	chrome
	FiscalYearLabel string
	Basis           string
	// Scopes is every schedule this document publishes, in the order it
	// publishes them.
	Scopes []string
	Years  []yearView
	Facts  int
	Nodes  int
	Links  int
	// ConfigJSON is window.FISC_CONFIG, as on the spine page: this view draws a
	// chart, so it ships app.js and the config app.js reads.
	ConfigJSON template.JS
}

// trendsPageData is the revenue-trends template's input.
//
// IT CARRIES NO ConfigJSON AND THE PAGE LOADS NO SCRIPT, which is a decision
// rather than an omission. site/app.js reads CONFIG.projections[CONFIG.primary]
// and draws a Sankey; handing it a document of series would blank the page
// through understands(). The table below is rendered entirely server-side, so
// this view works with JavaScript off — which is the property the spine page
// already defends for its headline, applied to a whole page. A chart for these
// series is its own change (fisc-4ua.3).
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
	// at this scale. /code-review measured the shipped page: ten real figures
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
	// never the same rectangle. At one unit they were: /code-review measured
	// ten shipped bars that are real money ($500,000, $74,748, $55,380,
	// $15,708…) rendering at exactly the height of the 165 published-zero
	// ticks, told apart only by fill — and at 1.6em over 13px type one unit is
	// 0.69 CSS px, so both were sub-pixel and the fill was carrying a
	// distinction the geometry had thrown away. Absent is not zero, and a
	// rounding-error figure is neither.
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
	// THE CELL STILL RENDERS A MINUS SIGN, DELIBERATELY, and this comment used
	// to say the opposite: that the flag existed "so the stylesheet can show it
	// as the document does rather than as a minus sign in a table". Rendering
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

// clientDoc is a source document as the client sees it.
type clientDoc struct {
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	PDFURL    string `json:"pdf_url"`
	// PageTextBase is the directory holding the committed page text; the
	// client appends pNNNN.txt. Splitting it this way keeps the zero-padding
	// rule (corpus.PagePath) in one place per side rather than in a template
	// string the client has to parse.
	//
	// It is a relative path into this site — see export.PageTextDir — whenever
	// the export shipped the text, and an absolute URL only when it was told
	// to cite a remote instead. The client appends the same filename either
	// way and must not assume a scheme.
	PageTextBase string `json:"page_text_base"`
	// RecordsBase is the directory holding this document's fact-store shards;
	// the client appends pNNNN.jsonl. It is what turns a link's locators into
	// a fetchable URL, which is the whole point of publishing them.
	//
	// IT IS ALWAYS SITE-RELATIVE, unlike PageTextBase beside it. Shards are
	// written into the output tree on every export; page text is not, and goes
	// absolute under --source-browse-url. The two look alike and are not.
	//
	// "" means the caller published no records for this document and the
	// client renders no records link -- absent, not a base pointing nowhere.
	RecordsBase string `json:"records_base"`
}

// clientConfig is window.FISC_CONFIG: the metadata the page needs before it
// has fetched anything, plus where to fetch the bulk from.
type clientConfig struct {
	SchemaVersion int               `json:"schema_version"`
	ExportedBy    string            `json:"exported_by"`
	Primary       string            `json:"primary"`
	Projections   map[string]string `json:"projections"`
	// Metadata is the primary projection's metadata block, verbatim.
	Metadata json.RawMessage `json:"metadata"`
	// Years is every published year with the words that belong to it, built by
	// the packager so the client never composes a figure or a caveat itself.
	Years []yearView           `json:"years"`
	Docs  map[string]clientDoc `json:"docs"`
	// RenderTiers is the node tiers the page draws, coarsest first; omitted
	// when the page draws its document whole.
	//
	// OMITTED AND NOT [] WHEN ABSENT, which app.js relies on: a page that
	// declares nothing is laid out by exactly the code that laid it out before
	// the fold existed, and the spine's config blob is unchanged byte for byte.
	RenderTiers []int `json:"render_tiers,omitempty"`
}

// buildPage decodes the primary projection and assembles everything the
// template and the client need, plus the citations it composed.
//
// pageTextBase resolves a doc id to the directory the page text is cited from,
// with its trailing slash. It is a function and not a URL because the caller,
// not this file, decides between a remote browse view and the copy the site
// ships (see Write).
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
		Value: dollars(h.AllFundsGrossExpenditureCents),
		Note:  "All funds, gross, " + meta.FiscalYearLabel + " " + meta.Basis + " budget",
		Kind:  "hero",
	}
	return hero, []figure{{
		Label: "Naive column total",
		Value: dollars(h.NaiveExpenditureCents),
		Note: "The wrong answer: summing the expenditure column counts transfers between funds twice, inflating the total by " +
			dollars(h.NaiveExpenditureCents-h.AllFundsGrossExpenditureCents) + ".",
		Kind: "error",
	}, {
		Label: "All-funds gross revenue",
		Value: dollars(h.AllFundsGrossRevenueCents),
		Note:  "Ties to the printed schedule; includes internal service charges.",
	}, {
		Label: "External revenue",
		Value: dollars(h.ExternalRevenueCents),
		Note:  "Net of internal service charges billed between city departments.",
	}, {
		Label: "External spending",
		Value: dollars(h.ExternalExpenditureCents),
		Note:  "Net of internal service charges.",
	}, {
		Label: "Transfers in / out",
		Value: dollars(h.InternalTransferInCents) + " / " + dollars(h.InternalTransferOutCents),
		Note:  "Money moving between the city's own funds.",
	}, {
		Label: "Unmatched transfers",
		Value: dollars(h.TransferResidualCents),
		// THIS NOTE SAYS WHAT THE NUMBER IS AND DEFERS WHY, deliberately, and
		// it is the one tile that has to. It used to restate the mechanism --
		// "the schedule that would pair them is not mapped yet" -- which is a
		// claim internal/project's transferCaveat also makes, about the same
		// difference, from the facts. Two copies of one claim in two packages
		// drifted exactly as you would expect: both went stale when p76 was
		// published in ced45b4, and they were not even greppable together,
		// because this one said "not mapped yet" and the caveat said "not yet
		// mapped". The caveat is the copy with the arithmetic behind it, so it
		// keeps the explanation and this tile points at it.
		//
		// IT PROMISES NO CAVEAT, deliberately. An earlier wording said "the
		// caveats below say what the difference is" -- but this tile is
		// unconditional and internal/project emits the transfer caveat only when
		// the document has transfers at all, so a document with none would point
		// at a caveat that is not there.
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
// THE TWO REFUSALS BELOW ARE THE SANKEY'S, NOT EVERY DOCUMENT'S. They used to
// live in the one decode path every document went through, which was correct
// while the Sankey was the only document and would have refused the revenue
// trends outright: internal/project/document.go states in writing that a trends
// document carries neither a fiscal_year_label nor a headline, and is not
// defective for that. They stay, because a spine document missing either IS
// defective -- a page with blanks where the headline goes is the thing this
// packager exists not to publish -- and they moved here so that being a Sankey is
// what invokes them.
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

// decodeDrilldown reads a document as a DRILL-DOWN document, and refuses one
// that is not.
//
// Its refusals are the mirror of decodeSankey's. A fiscal year label is
// required, because the page states one in its lede and a blank there is a page
// that will not say which budget it is about. At least one scope is required,
// because the footer names the schedules the figures come from. A headline is
// NOT required and not looked for.
func decodeDrilldown(stem string, raw []byte) (projectionDoc, drilldownMetadata, error) {
	doc, err := decodeDocument(stem, raw)
	if err != nil {
		return doc, drilldownMetadata{}, err
	}
	var meta drilldownMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return doc, drilldownMetadata{}, fmt.Errorf("decode %s metadata: %w", stem, err)
	}
	if meta.FiscalYearLabel == "" {
		return doc, drilldownMetadata{}, fmt.Errorf("%s metadata has no fiscal_year_label", stem)
	}
	if len(meta.Scopes) == 0 {
		return doc, drilldownMetadata{}, fmt.Errorf("%s metadata declares no scopes", stem)
	}
	return doc, meta, nil
}

// citationsOf is every (document, page) one projection's metadata cites.
//
// It decodes through documentSources rather than through either shape's
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
// THE CITATION SET IS THE UNION AND THE FOOTERS ARE NOT (fisc-fjy). Write copies
// the extracted text of the pages named here into the output, and it used to be
// handed the PRIMARY document's citations alone -- fine while the primary was the
// only document, and fourteen dead links the moment a view cites pp.127-140. So
// the shipped file set is unioned across every view and every year. The footer's
// Sources list stays each view's own, because a page claiming provenance for
// figures it never showed is its own defect, and unioning that too would trade
// one wrong page for another.
//
// THAT PRINCIPLE NOW HAS ONE STATED EXCEPTION, added by fisc-yi4 in 19a580f: a
// view's footer IS unioned across its own YEARS. Both directions are wrong and
// they are not equally wrong. Under-citing was SILENT -- app.js drops a citation
// whose doc_id is missing from CONFIG.docs, so a fact's provenance row simply
// vanished, with no error and no banner. Over-citing is VISIBLE: under an FY2027
// chart the footer lists a page only FY2026 cites, and a reader can see it and
// follow it. Between a defect a reader cannot detect and one they can, this
// takes the one they can -- and then says so on the page rather than leaving
// them to infer the set, which is why the heading names the years.
func buildSite(o *Options, pageTextBase func(docID string) string) ([]sitePage, []Citation, error) {
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
		// A VIEW WITH NO PROJECTION CITES NOTHING THROUGH THIS PATH, and the
		// guard is here rather than at the call site because there are three
		// of them. Without it the provenance view reaches
		// json.Unmarshal(nil, ...) and Write fails with "unexpected end of
		// JSON input" naming an empty stem -- a refusal that describes neither
		// the view nor the cause.
		if stem == "" {
			return nil
		}
		// Deduplicated: a document that cites a page twice is one file to ship,
		// and shipping it twice is a write collision. Two VIEWS citing one page
		// is the same statement one scale up, and is the ordinary case -- both
		// spine years cite pp.66-67.
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
		// EVERY TEMPLATE IS AN EXPLICIT ARM AND THE UNKNOWN ONE IS REFUSED.
		// This was `default: buildSankeyPage`, which meant any template name
		// that was not the trends one -- including a typo, and including a
		// third template added without a matching arm here -- was handed
		// pageData and rendered as a spine. The failure is silent by
		// construction: a template that reads none of the fields it is given
		// renders a page with blanks where the figures should be, and nothing
		// in the pipeline compares a template against the shape of the data it
		// received. Fail closed instead.
		switch v.Template {
		case TrendsTemplate:
			data, err = buildTrendsPage(o, v, here, byID, pageTextBase)
		case SankeyTemplate:
			data, err = buildSankeyPage(o, v, here, byID, pageTextBase)
		case DrilldownTemplate:
			data, err = buildDrilldownPage(o, v, here, byID, pageTextBase)
		case ProvenanceTemplate:
			data, err = buildProvenancePage(o, v, here, byID, pageTextBase)
		default:
			return nil, nil, cmdutil.WithHint(
				fmt.Errorf("view %q renders template %q, which this package has no builder for",
					v.Path, v.Template),
				"every template needs an arm in buildSite naming the page data it is "+
					"built from; a template with no arm used to be rendered as a spine "+
					"and would publish a page of blanks")
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

	// EVERY PUBLISHED DOCUMENT SHIPS ITS PAGES, NOT EVERY VIEWED ONE. The loop
	// above walks views, so a projection the site writes to data/<stem>.json
	// but renders no page for contributes nothing -- and its citations name
	// pages dist/extracted/ would not hold. Every other document is guaranteed
	// the opposite: a cited page the tree does not carry is an error, and a
	// reader following a provenance link would get a 404 from the one part of
	// this site that exists to be checkable.
	//
	// Collected AFTER the view loop and in sorted stem order so the pages a
	// viewed document cites keep the order they had; this only ever appends.
	for _, stem := range sortedKeys(o.Projections) {
		if err := collect(stem); err != nil {
			return nil, nil, err
		}
	}

	// AND EVERY PAGE THE INDEX PUBLISHES IS CITED, which is what makes the
	// published store complete rather than a function of what the charts
	// happen to draw.
	//
	// Before this, the shipped extraction was whatever the projections' own
	// metadata.sources named. The fact store covers a page the charts do not:
	// p76's 88 transfer facts are in scope transfers-by-fund, which no
	// projection selects, so a provenance link to that page resolved to a
	// shard beside a 404. Worse, the set was unstable -- a fact would enter
	// and leave the published extraction as views were added, with no event
	// anyone could see.
	//
	// Routing it through cited rather than through a second mechanism means
	// withPageText's existing refusal covers it: a cited page absent from the
	// extraction tree is an error, so a locator the site publishes cannot
	// point at text the site does not carry.
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
	Caveats      []caveatMeta
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
		for _, p := range s.Pages {
			ref.Pages = append(ref.Pages, pageRef{
				Number:  p,
				PDFURL:  pdfPageURL(d.PDFURL, p),
				TextURL: base + pageTextFile(p),
			})
		}
		sources = append(sources, ref)
		clientDocs[s.DocID] = clientDoc{
			Title:        ref.Title,
			Publisher:    ref.Publisher,
			PDFURL:       d.PDFURL,
			PageTextBase: base,
			RecordsBase:  recordsBase[s.DocID],
		}
	}
	return sources, clientDocs
}

// sankeyTitle is one year's <title> on the spine page.
//
// IT EXISTS SO THE LITERAL DOES NOT. site/app.js used to compose
// "City of Livermore budget flows — " + year.label itself, which was a copy of
// the fallback below in a second language -- and paintYearWords' own doc comment
// says every string it writes was built by the packager. That was the one line
// that did not.
//
// THE CALLER'S WORDS SURVIVE THE SWITCH. A View that sets a Title gets it on
// every year, unsuffixed: the packager composing prose over the top of a
// caller's would be the trap [View.Title] warns about, and a caller who names a
// page has said what they want it called. Only the fallback carries a year,
// because a title composed here has nothing else to tell one year from another.
//
// Refusing a Title on this template instead was considered and rejected. The
// [View.Lede] refusal reads as the precedent and is not: it fires because
// index.html.tmpl renders no {{.Lede}}, so the sentence would vanish in
// silence. This template renders {{.Title}} at line 6. Nothing is dropped, so
// there is nothing to refuse -- and refusing would make this package the
// mandatory author of the site's front page, strand buildSite's v.Nav fallback
// for this view, and leave the branch below dead by construction.
func sankeyTitle(callerTitle, yearLabel string) string {
	if callerTitle != "" {
		return callerTitle
	}
	return "City of Livermore budget flows — " + yearLabel
}

// unionSources merges the sources of every year a view publishes into one list,
// deduplicated and ordered.
//
// WHY A UNION AND NOT THE OPENING YEAR'S. The footer and clientConfig.Docs were
// built from ONE document's metadata -- the year the view opens on -- while
// CONFIG.years lists every published year and site/app.js will switch to any of
// them. app.js composes a citation per cited fact as
// CONFIG.docs[doc_id].page_text_base, and citations() SKIPS a fact whose doc_id
// is not in that map:
//
//	if (!doc) continue;
//
// So a year whose document cites a doc_id the opening year does not -- an
// ACFR-backed column, a schedule mapped out of a different book -- would have
// its citations silently DROPPED. Not a 404 and not a banner: the provenance
// panel simply shows fewer rows than the chart has facts, and nothing says so.
// That is the one failure mode this project exists to prevent, arriving through
// the only channel that produces no error. The footer's source list had the same
// shape one level less severely, advertising the opening year's pages under a
// chart drawn from another year's.
//
// THIS IS THE FIX fisc-fjy ALREADY MADE ONE LEVEL UP, which is the argument for
// it being right: that made the SHIPPED page-text set the union over every VIEW,
// shape-blind through metadata.sources. This is the same union over every YEAR
// of one view. The asymmetry that must survive is that it is a union WITHIN a
// view and never across views -- TestEachViewsFooterCitesItsOwnSources asserts a
// view's footer does not advertise another view's pages, and it still holds.
//
// Latent until the day two published years of one view cite different documents,
// which is why it is worth a test rather than a comment.
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
		// Sorted rather than first-seen: the footer's page list is published
		// output, so its order has to be a property of the pages and not of
		// which year happened to open the view.
		slices.Sort(ps)
		out = append(out, sourceMeta{DocID: id, Pages: ps})
	}
	return out
}

// projectionRefs is every data file the site publishes, which is the whole set
// on every page: they are downloadable provenance, not this view's figures.
func projectionRefs(projections map[string][]byte) []projectionRef {
	refs := make([]projectionRef, 0, len(projections))
	for _, name := range sortedKeys(projections) {
		refs = append(refs, projectionRef{Name: name, Path: path.Join(DataDir, name+".json")})
	}
	return refs
}

func buildSankeyPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	pageTextBase func(string) string,
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
		// THE FOOTER SENTENCE HAS THREE VALUES AND ONLY ONE OF THEM IS
		// REPAINTED. "Scope X, basis Y. Projection: Z." is rendered from the
		// OPENING year's metadata; basis is genuinely per-year and travels in
		// yearView, and the other two fail closed here instead.
		//
		// Scope, because repainting it would fix one of TWO copies of one claim:
		// the lede three screens up says "all funds, gross" in template prose
		// that no switch touches. And it cannot vary anyway -- Sankey.Slices
		// fixes it, its doc comment saying "The scope is fixed rather than
		// derived. It selects the SCHEDULE." Building a repaint for a state
		// nothing can emit means pinning it with a test that can never go red,
		// which is the defect this whole branch has been removing.
		//
		// GeneratedBy, because it is a claim about the TOOL that built the
		// document, not about the year. A page attributing its FY2026 figures to
		// one builder and drawing FY2027's from another is not a wording problem
		// a repaint fixes; the two documents disagree about their own
		// provenance, and this project's answer to ambiguity is to refuse it.
		// Found by /code-review of this change: the basis fix left its two
		// sentence-mates unguarded, which is how the original defect got in.
		if m.Scope != meta.Scope {
			return pageData{}, fmt.Errorf(
				"view %q opens on %q with scope %q but its year stem %q has scope %q; "+
					"one page cannot state two scopes, and its lede's wording is not per-year",
				v.Path, v.Projection, meta.Scope, stem, m.Scope)
		}
		if m.GeneratedBy != meta.GeneratedBy {
			return pageData{}, fmt.Errorf(
				"view %q opens on %q built by %q but its year stem %q was built by %q; "+
					"the footer credits one projection for figures drawn from both",
				v.Path, v.Projection, meta.GeneratedBy, stem, m.GeneratedBy)
		}
		cited = append(cited, m.Sources...)
		hero, figures := tilesFor(m)
		years = append(years, yearView{
			Year:       m.FiscalYear,
			Label:      m.FiscalYearLabel,
			Stem:       stem,
			Path:       path.Join(DataDir, stem+".json"),
			Basis:      m.Basis,
			Title:      sankeyTitle(v.Title, m.FiscalYearLabel),
			ChartTitle: "Sankey diagram of the " + m.FiscalYearLabel + " " + m.Basis + " budget",
			Hero:       hero,
			Figures:    figures,
			Caveats:    m.Caveats,
			Counts: countsRef{
				Facts: m.Counts.Facts, Nodes: m.Counts.Nodes, Links: m.Counts.Links,
			},
		})
	}
	hero, figures := tilesFor(meta)
	sources, clientDocs := sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)

	refs := projectionRefs(o.Projections)
	files := make(map[string]string, len(refs))
	for _, r := range refs {
		files[r.Name] = r.Path
	}
	cfg := clientConfig{
		SchemaVersion: doc.SchemaVersion,
		ExportedBy:    o.GeneratedBy,
		Primary:       v.Projection,
		Projections:   files,
		Metadata:      doc.Metadata,
		Years:         years,
		Docs:          clientDocs,
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return pageData{}, fmt.Errorf("encode page config: %w", err)
	}

	title := sankeyTitle(v.Title, meta.FiscalYearLabel)
	return pageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  refs,
			DataPath:     files[v.Projection],
			Scope:        meta.Scope,
			Caveats:      meta.Caveats,
		},
		FiscalYearLabel: meta.FiscalYearLabel,
		Basis:           meta.Basis,
		Hero:            hero,
		Figures:         figures,
		Years:           years,
		Facts:           meta.Counts.Facts,
		Nodes:           meta.Counts.Nodes,
		Links:           meta.Counts.Links,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and & to their \u form, so it cannot terminate the script element
		// or inject markup. The alternative, letting html/template escape a
		// string, would corrupt the JSON.
		ConfigJSON: template.JS(blob),
	}, nil
}

// buildDrilldownPage assembles the drill-down view.
//
// IT IS A THIRD ARM AND NOT A RELAXED buildSankeyPage. The two pages differ in
// what they are allowed to say, not only in which keys they read: this one has
// no headline, no hero tile and no stat row, because the document holds the same
// money at more than one grain and any figure that invites a reader to add a
// column up would be wrong QUIETLY -- every number on it tying to a fact. See
// the template and docs/general-fund-drilldown-contract.md.
func buildDrilldownPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	pageTextBase func(string) string,
) (drilldownPageData, error) {
	doc, meta, err := decodeDrilldown(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return drilldownPageData{}, err
	}

	stems := v.YearStems
	if len(stems) == 0 {
		stems = []string{v.Projection}
	}
	years := make([]yearView, 0, len(stems))
	var cited []sourceMeta
	for _, stem := range stems {
		m := meta
		if stem != v.Projection {
			if _, m, err = decodeDrilldown(stem, o.Projections[stem]); err != nil {
				return drilldownPageData{}, err
			}
		}
		// The spine's two cross-stem refusals, restated for a plural scope.
		// Same argument as buildSankeyPage's: the footer states one scope list
		// and one builder for a page that can show several years, so a year
		// disagreeing about either would have the footer describe a document
		// other than the one on screen.
		if !slices.Equal(m.Scopes, meta.Scopes) {
			return drilldownPageData{}, fmt.Errorf(
				"view %q opens on %q with scopes %v but its year stem %q has scopes %v; "+
					"one page cannot state two scope lists",
				v.Path, v.Projection, meta.Scopes, stem, m.Scopes)
		}
		if m.GeneratedBy != meta.GeneratedBy {
			return drilldownPageData{}, fmt.Errorf(
				"view %q opens on %q built by %q but its year stem %q was built by %q; "+
					"the footer credits one projection for figures drawn from both",
				v.Path, v.Projection, meta.GeneratedBy, stem, m.GeneratedBy)
		}
		cited = append(cited, m.Sources...)
		years = append(years, yearView{
			Year:  m.FiscalYear,
			Label: m.FiscalYearLabel,
			Stem:  stem,
			Path:  path.Join(DataDir, stem+".json"),
			Basis: m.Basis,
			Title: v.Title,
			ChartTitle: "Sankey diagram of the " + m.FiscalYearLabel + " " + m.Basis +
				" budget by fund and division",
			// NO HERO AND NO FIGURES, and the empty slices are the point rather
			// than a gap: paintYearWords replaces the tile row from these on
			// every year switch, so a page that renders none server-side must
			// hand the client none either.
			Caveats: m.Caveats,
			Counts: countsRef{
				Facts: m.Counts.Facts, Nodes: m.Counts.Nodes, Links: m.Counts.Links,
			},
		})
	}
	sources, clientDocs := sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)

	refs := projectionRefs(o.Projections)
	files := make(map[string]string, len(refs))
	for _, r := range refs {
		files[r.Name] = r.Path
	}
	cfg := clientConfig{
		SchemaVersion: doc.SchemaVersion,
		ExportedBy:    o.GeneratedBy,
		Primary:       v.Projection,
		Projections:   files,
		Metadata:      doc.Metadata,
		Years:         years,
		Docs:          clientDocs,
		RenderTiers:   v.RenderTiers,
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return drilldownPageData{}, fmt.Errorf("encode page config: %w", err)
	}

	return drilldownPageData{
		chrome: chrome{
			Title:        v.Title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  refs,
			DataPath:     files[v.Projection],
			Caveats:      meta.Caveats,
		},
		FiscalYearLabel: meta.FiscalYearLabel,
		Basis:           meta.Basis,
		Scopes:          meta.Scopes,
		Years:           years,
		Facts:           meta.Counts.Facts,
		Nodes:           meta.Counts.Nodes,
		Links:           meta.Counts.Links,
		// #nosec G203 -- see buildSankeyPage; blob is encoding/json's output.
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

// pageTextFile mirrors corpus.PagePath's zero padding. It is spelled out
// rather than imported because it is a URL here, not a filesystem path, and
// the two only look alike.
func pageTextFile(page int) string { return fmt.Sprintf("p%04d.txt", page) }

// dollars renders integer cents the way the schedule prints them: whole
// dollars with thousands separators, keeping the cents only when a figure
// actually has some.
func dollars(cents int64) string {
	s := amount.Cents(cents).String()
	return strings.TrimSuffix(s, ".00")
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
	Counts      struct {
		Facts  int `json:"facts"`
		Series int `json:"series"`
		Points int `json:"points"`
	} `json:"counts"`
	Caveats []caveatMeta `json:"caveats"`
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
		Fund          int          `json:"fund"`
		FundName      string       `json:"fund_name"`
		FundGroup     string       `json:"fund_group"`
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
	pageTextBase func(string) string,
) (trendsPageData, error) {
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
			Basis: c.Basis,
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
			// page and is never nothing.
			fund = fmt.Sprintf("Fund %d", s.Fund)
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

	// THE RECONCILIATION buildCells' COMMENT HAS ALWAYS PROMISED, performed at
	// last. counts.points and counts.series were decoded and read nowhere, so a
	// document could lose a figure between the projection and the page and still
	// print a lede saying how many figures it had drawn -- a published page whose
	// own prose contradicts what it shows, which is the class this project exists
	// to refuse (fisc-4j5).
	//
	// THE RULE IS NARROW AND IT IS WORTH STATING, because `fisc export` runs no
	// checks by design and this is the one thing it now insists on: a document
	// whose counts disagree with its own series is refused. A series legitimately
	// SHORT A COLUMN is not -- that is a state internal/check declares through
	// incompleteSeries, and it reconciles here because a short series carries
	// fewer points and says so in its own counts.
	if rendered != meta.Counts.Points {
		return trendsPageData{}, fmt.Errorf(
			"%s declares counts.points %d and its series carry %d; the document has "+
				"lost a figure between the projection that built it and this page",
			v.Projection, meta.Counts.Points, rendered)
	}
	// AND AGAINST facts, WHICH IS THE NUMBER THE LEDE ACTUALLY PRINTS.
	// revenue.html.tmpl renders {{.Facts}} -- "N figures in all" -- fed from
	// counts.facts, while the arm above reconciles against counts.points, and
	// project.TrendCounts' doc comment says in so many words that the two are
	// computed independently: facts off the selection, points off the series
	// actually built, so "a document that dropped a series publishes points
	// below facts".
	//
	// So counts.facts 924 / counts.points 920 / 920 rendered cells passed
	// everything above and published a lede claiming 924 figures over a table
	// carrying 920 -- the page whose own prose contradicts what it shows, which
	// is the thing the comment above claims to have closed. Reconciling against
	// facts rather than printing points in the lede is the direction that keeps
	// the sentence meaning what it says (fisc-5tu).
	if rendered != meta.Counts.Facts {
		return trendsPageData{}, fmt.Errorf(
			"%s prints counts.facts %d in its lede and its series carry %d cells; "+
				"the page would claim more figures than it shows",
			v.Projection, meta.Counts.Facts, rendered)
	}
	if len(body.Series) != meta.Counts.Series {
		return trendsPageData{}, fmt.Errorf(
			"%s declares counts.series %d and carries %d",
			v.Projection, meta.Counts.Series, len(body.Series))
	}

	sources, _ := sourcesFor(meta.Sources, byID, pageTextBase, o.RecordsBase)
	refs := projectionRefs(o.Projections)
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
			DataPath:     path.Join(DataDir, v.Projection+".json"),
			Scope:        meta.Scope,
			Caveats:      meta.Caveats,
		},
		Columns: columns,
		Series:  series,
		Facts:   meta.Counts.Facts,
		Count:   meta.Counts.Series,
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
	pageTextBase func(docID string) string) (provenancePageData, error) {
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
			Projections: projectionRefs(o.Projections),
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
				"two points publish FY%d %s: %s and %s; one of them would be dropped",
				p.FiscalYear, p.Basis, dollars(prev.AmountCents), dollars(p.AmountCents))
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
		cell.Value = dollars(p.AmountCents)
		cell.Cents = p.AmountCents
		cell.Negative = p.AmountCents < 0
		cell.Page = p.Page
		cell.Href = pageTextBase(p.DocID) + pageTextFile(p.Page)
		out = append(out, cell)
		placed++
	}
	return out, placed, nil
}
