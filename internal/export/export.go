// Package export packages a set of projection documents into a static site.
//
// The seam this package sits on is deliberate: it consumes projections as
// filename stem -> JSON bytes and knows nothing about how they were built.
// It does not import internal/project, and it does not recompute anything the
// projection already published. Every figure the page shows — the fiscal year
// label, the headline totals, the caveats, the source pages — is read back out
// of the projection JSON, because a packager that reassembles metadata is a
// second implementation of the contract and will drift from the first.
//
// The output layout is fixed:
//
//	<dir>/index.html          rendered from site/index.html.tmpl
//	<dir>/app.js              copied verbatim from site/
//	<dir>/style.css           copied verbatim from site/
//	<dir>/vendor/*            vendored d3 and d3-sankey, with licences
//	<dir>/.nojekyll           zero bytes; stops GitHub Pages running Jekyll
//	<dir>/data/<name>.json    one file per projection
//	<dir>/extracted/<doc>/pages/pNNNN.txt   the cited pages' committed text
//	<dir>/<Files...>          whatever else the caller ships
//
// Every asset path in the page is relative, so the same output serves from a
// user's file tree, from a project subpath on GitHub Pages, and from a domain
// root without a base-path setting anywhere. That is also why the cited pages'
// extracted text is copied into the output rather than linked at the forge:
// provenance a reader can only check with a network round-trip to a third
// party is provenance the site does not actually ship.
package export

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/site"
)

// PrimaryProjection is the spine document, and the default view's projection.
//
// IT IS NO LONGER "the projection whose metadata drives the page", which is what
// it meant while there was one page. Each view now decodes its own document and
// composes its own chrome. What survives is narrower than it looks and is worth
// stating exactly, because an earlier draft of this comment claimed more than
// the code does: this stem must be among the projections built (ErrNoPrimary),
// and it is the projection of the view a caller gets when it names none. It is
// NOT required to be the view at IndexPath -- validate asks only that exactly
// one view is there -- because which document a site opens on is the composition
// root's decision and there is no reason this package should own it.
const PrimaryProjection = "sankey"

// IndexPath is the view the site opens on. It is the fixed entry point of the
// output layout, so it is a constant rather than something a caller may move.
const IndexPath = "index.html"

// dataDir is the output subdirectory holding projection JSON. It is part of
// the published contract — docs/sankey-contract.md promises
// <output>/data/<projection>.json — so it is a constant, not a flag.
const dataDir = "data"

// PageTextDir is the output subdirectory holding the committed extraction of
// every page the projection cites. The tree under it mirrors the repository's
// data/extracted/ — <doc-id>/pages/pNNNN.txt — so the same base-plus-filename
// rule composes a citation whether the text is served from here or from a
// forge's blob view, and a reader who knows one layout knows the other.
//
// ONLY CITED PAGES ARE COPIED, where cited means cited by the SITE and not by a
// chart. The corpus is 786 pages and the site cites 21, so a packager that
// shipped the whole extraction would put the corpus in every deploy to publish
// a handful of citations.
//
// The set is the union of every projection's metadata.sources AND every entry
// of [Options.PageIndex]. That second half is what makes the published record
// store complete rather than a function of what the charts happen to draw:
// without it a page whose facts ship but whose figures no projection selects --
// p76 today -- would be published as records sitting beside a 404, and the set
// would change silently as views were added.
const PageTextDir = "extracted"

// markerName is the sentinel written into a generated site. It is what tells
// a later --clean run that the directory is fisc's to delete. The name is
// cmdutil's, not this package's: the writer and the deleter agreeing on it by
// coincidence is how a --clean starts refusing to clean.
const markerName = cmdutil.ExportMarkerName

// DefaultSourceBrowseURL is where the committed page text is browsable when
// the site does not ship it itself — that is, when the caller supplies neither
// [Options.PageText] nor [Options.SourceBrowseURL]. It is the last resort and
// not the normal path: a citation served from this constant needs a network
// round-trip to a third party, and it goes stale the moment the repository
// moves. `fisc export` always passes the extraction tree, so its output cites
// the bytes beside it.
//
// It points at github.com's blob view, never at raw.githubusercontent.com. The
// reason is not Git LFS: data/extracted/ is ordinary git — only data/pdf/** is
// LFS — so the raw host would serve the text correctly. It is that the blob
// view is the one worth citing, with line numbers, history and the rest of the
// document beside it, and that the artifacts are .txt precisely so that view
// renders them verbatim rather than collapsing the runs of spaces that are the
// column grid (see corpus.PagePath).
const DefaultSourceBrowseURL = "https://github.com/jcrussell/livermore-budget/blob/main"

// verbatimAssets are copied from the embedded site tree byte for byte. The
// page template is not in this list: it is rendered, and vendor/ is walked.
var verbatimAssets = []string{"app.js", "style.css", ".nojekyll"}

// View is one HTML page of the site: one document, rendered by one template.
//
// N FLAT PAGES RATHER THAN HASH ROUTING, and the reason is a property this
// package already defends: the page renders its figures server-side so the
// headline survives without JavaScript (see buildSankeyPage). Under hash routing
// views 2..N have no server-rendered content at all. Two further reasons.
// Citations are PER VIEW, so one page would need a footer listing the union of
// every view's sources -- a page claiming provenance for figures it never showed.
// And the client refuses the whole page on a schema mismatch, which under one
// page blanks the site rather than one view.
//
// FLAT rather than nested because every asset path in the output is relative
// (style.css, data/x.json, vendor/), and a page one directory deep would need
// ../ on every one of them or a <base> element.
type View struct {
	// Path is the output path, always .html and always at the root. Exactly one
	// view must be at [IndexPath].
	Path string
	// Nav is the words this view is listed under in every view's nav. Every
	// page carries the whole nav, which is what makes the set of views visible
	// from any one of them.
	Nav string
	// Template is the name of the template in the asset tree.
	Template string
	// Projection is the filename stem of the document this view renders. It
	// must be a key of [Options.Projections].
	Projection string
	// Title is the <title>, and on a template that renders one, the page
	// heading. Lede is the sentence under it.
	//
	// BOTH ARE THE CALLER'S WORDS, not composed here. A packager that wrote
	// prose about a document would be making a claim about figures it is
	// forbidden to recompute; what it may do is render what it was handed.
	//
	// ONLY THE TRENDS TEMPLATE RENDERS A LEDE, and that is a property of the
	// documents rather than an oversight. The spine's lede wraps a live
	// <span id="lede-year"> that app.js rewrites on every year switch, so it
	// cannot be a string handed over once at package time; its <h1> is a
	// standing question rather than a description of one year, which is why it
	// differs from the <title> that carries the year. The trends page has one
	// document, no year control and no such span, so both are simply rendered.
	//
	// A Lede on a template that renders none is REFUSED rather than dropped --
	// see validate. It was silently ignored, which is a trap for the next
	// caller: the field is set, the export succeeds, and the sentence is
	// nowhere on the page.
	Title string
	Lede  string
	// YearStems are the documents that are the same projection for different
	// fiscal years, in the order a reader should meet them, opening year first.
	// Empty means this view has one document and renders no year control.
	//
	// IT IS PER VIEW, not per site. Years are a property of the SPINE, which
	// publishes one document per fiscal year; the revenue trends publish one
	// document spanning four columns and have no year to switch between. A
	// single site-wide list could not say that.
	YearStems []string

	// Sections are the printed blocks a history table groups its rows under,
	// in printed order. The caller's words, like Title and Lede: a heading is
	// prose about the document, which this package may render and never
	// compose. buildHistoryPage refuses a series no section claims and a
	// section that claims no series, so the declaration cannot silently
	// mislabel a block the schedule gained or lost.
	Sections []Section

	// RenderTiers is the node tiers this view's chart draws, coarsest first,
	// shipped to the client as FISC_CONFIG.render_tiers. Empty draws the
	// document whole.
	//
	// IT IS PER VIEW BECAUSE THE DOCUMENTS HAVE DIFFERENT HIERARCHIES. The
	// spine publishes tiers 0, 2 and 5 and is drawn whole; the drill-down
	// publishes 0, 2, 3, 4 and 5 and cannot be drawn whole at all -- its
	// 61-node fund column lays every node and every ribbon out at zero height.
	// A tier set belonging to this package rather than to a view would be wrong
	// for one of them: the drill-down's set over the spine REFUSES to draw,
	// because a spine node is parentless and has no ancestor to fold to. A
	// refusal is the better of the two failures and still a broken page.
	//
	// The fold itself is the client's: see site/app.js's foldDocument and the
	// "Drawing it" section of docs/general-fund-drilldown-contract.md. This
	// package ships the declaration and never applies it, which is the same
	// division of labour as every other figure on the page.
	RenderTiers []int

	// ChartSubject is what this view's chart is OF, in a phrase that completes
	// "Sankey diagram of the FY 2025-26 adopted budget ...". It is the chart's
	// accessible name, which is what a screen reader announces and a different
	// string from Title, which is the document's.
	//
	// THE CALLER'S WORDS, LIKE Title AND Lede, and it became one when this
	// template stopped rendering a single page. It was the literal "by fund and
	// division", composed here, which was true of the one page that used it --
	// and the moment Revenue and Spending shared the template both announced
	// themselves as a chart of neither. Revenue draws revenue categories into
	// fund groups; Spending draws one fund into its divisions. A packager
	// cannot know that, and guessing it silently is worse than asking.
	ChartSubject string

	// ChartDescription is the chart's long description -- <desc>, which a screen
	// reader reads after the name. The caller's words for the same reason
	// ChartSubject is: the <desc> shipped in this template described the
	// three-column page it was written for and nothing ever rewrote it, so both
	// pages that replaced it described a chart neither draws.
	ChartDescription string

	// Root restricts this view's chart to one node's own money, or "" for a
	// view that draws its whole document. Shipped as FISC_CONFIG.root.
	//
	// IT IS WHAT MAKES A ONE-SIDED PAGE POSSIBLE AT ALL, and it is a refusal
	// rather than a preference. Spending draws tiers {3,4} of fund-flows -- the
	// General Fund into its divisions -- and that document also carries eleven
	// tier-0 revenue nodes with no ancestor at tier 3 or 4. foldDocument
	// refuses a node it cannot place, so without a root the page does not draw
	// a partial chart, it draws none: "node revenue/charges-for-services is
	// tier 0 and no ancestor of it is a tier this page draws (3, 4)".
	//
	// It also makes the page's central claim a declaration the code keeps
	// rather than a sentence in its lede. Spending says only the General Fund
	// has a spending side; Root is where it says so to the client.
	Root string

	// Steps is how this view's chart opens a node, one hop per step, or empty
	// for a view whose chart does not open at all. Shipped to the client as
	// FISC_CONFIG.steps.
	//
	// PER VIEW FOR RenderTiers' REASON, AND THEN SOME: it declares what
	// activating a node MEANS on this page. Without it a node click isolates,
	// which is what the spine has always done; with it a node at a step's From
	// opens into that step's Tiers, and a node at a child step's From in THAT
	// chart opens into the child's. Those are two interaction contracts and no
	// page has both, because a page that drills has two drawn columns and
	// isolating on two columns dims a column the reader was not looking at
	// (fisc-ppkq).
	//
	// THE LIST IS FLAT AND THE SHAPE IS A TREE. A step names the step whose
	// chart it opens from ([DrillStep.After]), so two openable tiers at one
	// depth are two steps sharing a parent rather than two entries of a path,
	// and a view may declare several steps that open from its own chart. Flat
	// rather than nested because tools/jscheck/harness.mjs reads this literal
	// out of the Go source and slices it on indentation, and because the
	// packager resolves one step document per entry. What a declared tree must
	// satisfy is validateSteps'.
	//
	// The behaviour is entirely the client's, like the fold. This package ships
	// the declaration.
	Steps []DrillStep
}

// Doc describes one source document the page cites. The caller supplies these
// from data/sources.yaml, which is the only place a URL is asserted; this
// package must not invent one.
type Doc struct {
	ID        string
	Title     string
	Publisher string
	// PDFURL is the canonical, unversioned document URL. The page appends a
	// #page=N fragment to it per citation.
	PDFURL string
}

// Options is a call to Write.
type Options struct {
	// Dir is the output directory. The caller resolves and screens it
	// (cmdutil.ResolveOutputDir) before getting here.
	Dir string

	// Projections maps a filename stem to a projection's JSON document, which
	// is written verbatim to data/<stem>.json. Must contain PrimaryProjection.
	Projections map[string][]byte

	// Views are the site's pages, in nav order, the view the site opens on
	// first. Empty means the single Sankey page, which is what every caller
	// wanted before there were two documents.
	//
	// The list is STATED BY THE CALLER, as YearStems was and for the same
	// reason: which documents are views of what, and which are years of which,
	// is the composition root's knowledge. This package lays out what it is
	// handed and does not import internal/project to find out. `func views()`
	// lives in pkg/cmd/export.
	Views []View

	// Docs are the source documents the page cites, keyed by doc id in the
	// projection's metadata.sources.
	Docs []Doc

	// SourceBrowseURL is the base URL for committed page text: a remote whose
	// tree holds data/extracted/<doc-id>/pages/pNNNN.txt, such as a forge's
	// blob view of this repository. Setting it means the page cites there
	// INSTEAD of shipping the text, so PageText is not read and nothing is
	// copied; a caller that wants both would be publishing two answers to the
	// same question. Empty means ship the text (PageText) or, failing that,
	// DefaultSourceBrowseURL.
	SourceBrowseURL string

	// PageText is the committed extraction tree, rooted where the repository's
	// data/extracted/ is: the text of page N of document D is read from
	// <D>/pages/pNNNN.txt, which is corpus.PagePath under a doc-id directory.
	//
	// Only the pages the projection cites are read, and a cited page missing
	// from the tree is an error rather than a citation quietly left dangling.
	// Nil means the page text is not shipped and citations go to a URL.
	PageText fs.FS

	// Files is the non-projection asset channel: slash-separated path relative
	// to Dir -> bytes, written verbatim into the output tree.
	//
	// It is general on purpose, and the generality is the point rather than
	// speculation. Projections are a closed namespace — a filename STEM that
	// validate rejects for containing '/', '\' or '.', written to
	// data/<stem>.json because DataDir is part of the published contract
	// (docs/sankey-contract.md) — so anything else the site has to carry needs
	// a channel of its own. There are two such things already: the cited pages'
	// extracted text, which Write puts through this same path (see writeFiles),
	// and the fact store the provenance browser downloads (fisc-4ua.8). Built
	// once, used twice; a page-text-shaped option would have to be built again
	// for the second caller.
	//
	// Paths are screened by assetPath: relative, clean, slash-separated, and
	// outside the fixed site layout, so an asset cannot shadow index.html, an
	// embedded asset or a projection.
	Files map[string][]byte

	// PageIndex is what the provenance view publishes: one entry per locator
	// the site can resolve, in the order the page lists them.
	//
	// THE PACKAGE STILL DOES NOT KNOW WHAT A FACT IS. An entry says "there are
	// N records for (doc, page), they are at this path, and here is a sentence
	// about them" -- which is a statement the composition root makes, in the
	// same way Views is. Inferring it here from the shape of the Files keys
	// would be this package guessing what a name means, which is exactly what
	// Views' doc comment above refuses.
	//
	// IT ALSO DECIDES WHAT PAGE TEXT SHIPS. Every entry's page is cited, so
	// the extraction copied into the output covers every locator published --
	// not merely every page a chart happens to draw from. See PageTextDir.
	PageIndex []PageIndexEntry

	// Downloads are the whole-store artifacts a page offers, described so the
	// reader knows the size before starting the fetch.
	Downloads []Download

	// RecordsBase is where each document's records live, keyed by doc id, in
	// the form a client appends pNNNN.jsonl to. It is what lets a mark on a
	// chart resolve to the facts behind it: a link publishes locators, and the
	// client composes base + file.
	//
	// THIS PACKAGE DOES NOT KNOW HOW THAT PATH IS BUILT AND MUST NOT LEARN.
	// The rule is spelled once, in whoever produced the records -- the same
	// rule PageIndexEntry.Data is written by, which is why validate can assert
	// the two agree instead of taking this on trust.
	//
	// UNLIKE PageTextBase THERE IS NO LOCAL/REMOTE FORK. A --source-browse-url
	// export ships no page text and cites a remote URL, so page_text_base goes
	// absolute; the shards are always written into the output tree, so this
	// stays site-relative. The two therefore differ in kind while sitting
	// beside each other in the same clientDoc.
	//
	// A doc with no entry publishes records_base: "" and the client renders no
	// records link. Absent is not zero: a base that was never supplied is not
	// a base pointing at nothing.
	RecordsBase map[string]string

	// GeneratedBy names the tool and version that wrote the output. It is
	// shown in the page footer beside the projection's own generated_by.
	GeneratedBy string

	// Assets is the site source tree. Empty means the embedded one; a test
	// can substitute an fstest.MapFS.
	Assets fs.FS
}

// Citation is one (document, page) pair the site cites.
//
// Exported because [PageIndexEntry] embeds it: a page index entry IS a
// citation, plus what is published about it, and saying so in the type is what
// makes "every published locator ships its page text" a property of the code
// rather than a claim in a comment.
type Citation struct {
	DocID string
	Page  int
}

// PageIndexEntry is one locator the provenance view publishes.
type PageIndexEntry struct {
	Citation
	// Records is how many records the shard holds. Named for what it counts
	// rather than "Facts", because this package does not know what a fact is.
	Records int
	// Data is the site-relative path the records are at. It must be a key of
	// [Options.Files]; validate refuses an entry whose file was not supplied,
	// so the page cannot publish a link to bytes the site does not ship.
	Data string
	// Bytes is the size of that file, for a reader sizing a fetch. Validate
	// checks it against the file rather than trusting it.
	Bytes int
	// Note is the caller's sentence about this page, rendered verbatim.
	Note string
}

// Section is one printed block of a history document: the heading the schedule
// prints, and the (kind, fund_group) its series carry. A series belongs to the
// section whose Kind and FundGroup equal its own — exact match, so two sections
// cannot contest one series.
type Section struct {
	Heading   string
	Kind      string
	FundGroup string
	// Rows, when non-nil, closes the section: it is every printed row label the
	// section claims, each mapped to the label its Line cell displays, where ""
	// displays the printed label unchanged. The display label exists for the
	// wrapped row — a fact's row_label is text the page prints, so a label the
	// document wraps across lines arrives here as its printed tail, and the
	// view is the layer allowed to say what a reader should call that row
	// without touching what the page said. Both directions refuse: a series
	// landing here that Rows does not name is an error, because a heading that
	// enumerates its rows must not absorb a new one silently, and a name
	// matching no series is an error, because a declaration that marks nothing
	// never goes red. That is the opposite of the category and fund-name
	// lookups, whose miss falls back so a newly mapped slug still renders;
	// Rows is a small closed declaration about one printed block, so a miss
	// is a defect rather than a gap. Nil leaves the section open: any series
	// matching its key belongs, under its printed label.
	Rows map[string]string
}

// DrillStep is one hop of a view's chart opening a node into its parts.
//
// WHY FILTER-AND-RESCALE AND NOT EXPAND-IN-PLACE. Measured, and recorded on
// fisc-ppkq: the vendored d3-sankey derives its column count from topology and
// clamps the align function into it, so expanding one node in place draws that
// node's children in the same column as the next tier while the unexpanded
// ribbons span two -- which tools/jscheck/layout.mjs's bands() refuses outright.
// Filtering to one node keeps every tier set uniform, which is the only shape
// this build lays out.
//
// FROM AND TIERS ARE NOT NECESSARILY TIERS OF THE SAME DOCUMENT. From is a tier
// of the chart on screen when the reader opens a node -- the step After names,
// or the view's own document for a root step -- and Tiers are tiers of the
// document THIS step draws. On a step that switches document the two
// hierarchies are unrelated: From: 2 is the spine's fund-group tier while
// Tiers: {0, 3, 4} are fund-flows'. So validateSteps places From against this
// step's parent and Tiers against this step's caps, and cannot relate From to
// Tiers at all. A reader expecting it to has read the struct as one document.
//
// THE TREE IS ON THE WIRE BECAUSE THE CLIENT WALKS IT. Key, After, Side and
// Role say what this step opens and what it opens from, and site/app.js
// resolves the step a node opens into by matching all three against the rung
// on screen rather than by depth -- two steps open from the spine's chart, one
// per tier, and a depth cannot tell them apart. They were `json:"-"` while no
// client read them, so that a key on the wire could not be a second
// declaration of a tree nothing walked.
type DrillStep struct {
	// Key names this step, so another step can declare that it opens from this
	// step's chart. Required and unique within a view: a step no other step can
	// name is a chart no second edge can ever be attached to, and the
	// uniqueness is what makes [DrillStep.After] resolve to one parent.
	Key string `json:"key"`
	// After is the Key of the step whose chart this one opens from, or "" for a
	// step that opens from the view's own chart. Several steps may share one
	// After: that is the second edge out of one chart a path cannot express.
	//
	// IT NAMES AN EARLIER STEP, ALWAYS, and validateSteps refuses one that does
	// not. A cycle is then undeclarable rather than detected, which is a
	// property of the type rather than an arm that has to be kept correct.
	After string `json:"after"`
	// From is the tier whose nodes open, in the chart on screen before they do.
	// One tier rather than a set: a step is one hop, and a second tier of the
	// same chart is a second step sharing this one's After.
	From int `json:"from"`
	// Side is which end of a link the opened node sits on: "" for the node the
	// links point AT, which is every step the site ships today, and
	// [SideSource] for the node they come FROM.
	//
	// DECLARED, NEVER INFERRED. "The opened tier is below every tier this step
	// draws, so it must be a source" is a mapping from tier numbers to meaning
	// -- the construct paintBreadcrumb's comment in site/app.js refuses. It is
	// true of the columns that exist and says nothing a third document would
	// have to obey.
	Side string `json:"side,omitempty"`
	// Role is which of the nodes at From open, in the caller's vocabulary, or
	// "" for all of them.
	//
	// A DISCRIMINATOR THIS PACKAGE GIVES NO MEANING. validateSteps requires
	// only that (After, From, Role) name at most one step, so two steps opening
	// one tier of one chart are told apart by something the caller declared
	// rather than by declaration order.
	Role string `json:"role,omitempty"`
	// Projection is the filename stem of the document this step draws, or ""
	// to draw the same document as the step before it -- the view's own, for
	// the first step. A named one must be a key of [Options.Projections].
	//
	// Omitted from the JSON when empty, so the client reads an absent key as
	// "the same document" rather than as a stem named "".
	Projection string `json:"projection,omitempty"`
	// YearProjections is the document this step draws for each year the view
	// lists, keyed by the view's year stem: the step's own per-year join. One
	// entry per [View.YearStems] entry, the opening year's equal to Projection,
	// and none at all on a view that lists no years or a step that names no
	// projection.
	//
	// NOT SHIPPED AS THIS MAP. The client never joins: the packager resolves
	// each year's step documents into that year's config entry, beside the
	// year's own path and caveat refs, so a year on screen already knows the
	// files its rungs will draw. A map on the wire would be a second join for
	// the client to spell.
	//
	// DECLARED BY THE CALLER, NOT DERIVED HERE, for [View.YearStems]'s reason:
	// which document of one projection is the same fiscal year as which
	// document of another is the composition root's knowledge, and this package
	// does not import internal/project to find out.
	YearProjections map[string]string `json:"-"`
	// Tiers is the tier set drawn once a node has opened -- this step's
	// RenderTiers, and a different declaration from the chart's before it.
	Tiers []int `json:"tiers"`
	// Caps bounds the columns this step draws, one per tier that needs one; a
	// tier with no cap is drawn whole.
	//
	// A SLICE AND NOT A MAP. The shipped JSON then has one order whatever the
	// declaration's, and validateSteps can refuse a cap on a tier the step does
	// not draw, which a map keyed by tier would carry in silence.
	Caps []TierCap `json:"caps,omitempty"`
	// Back is what the breadcrumb's return control says, e.g. "All fund
	// groups". Declared rather than derived from From, because a tier number
	// does not know what the reader calls the things in it.
	Back string `json:"back"`
	// Tail is the plural noun the capped aggregate is counted in -- "funds",
	// "categories" -- so its label reads "24 smaller funds". A cap naming its
	// own [TierCap.Tail] takes that instead; this is the default for the rest.
	//
	// DECLARED FOR Back's REASON, and it was derived for one commit: app.js
	// read `tier === 3 ? "funds" : "categories"`, which is the exact construct
	// the comment on paintBreadcrumb refuses two functions away. A third page
	// drilling into a third tier would have been given "categories" and nothing
	// would have said so.
	Tail string `json:"tail"`
	// Description is the chart's long description once a node has opened on
	// this step: what the columns are and what the marks mean, in the caller's
	// words, like [View.ChartDescription] is for a chart's opening state. The
	// client writes it into the SVG's <desc> at that depth and appends how to
	// get back and where the flow table is.
	//
	// REQUIRED AND TERMINATED. An opened chart with no description of its own
	// announces the opening state's -- "revenue categories flow into six fund
	// groups" over a chart of one group's funds -- and only to the readers who
	// cannot see the marks disagree. Terminated for ChartDescription's reason:
	// the client appends its own sentences after it.
	Description string `json:"description"`
	// Residual is the set of endpoints of the chart this step opens FROM whose
	// flow into or out of the opened node the document this step DRAWS does
	// not decompose, each with the reason it cannot: node id to reason. The
	// client copies those links verbatim onto one derived node beside the
	// opened node's parts, so a reader sees the money the finer document does
	// not carry rather than a total that silently fell short.
	//
	// ONLY ON A STEP THAT SWITCHES DOCUMENT. A residual is what one document
	// prints at a grain the other does not, and a step drawing the document
	// before it has no second grain for anything to be residual between;
	// validateSteps refuses one there rather than let the client carry a set
	// that nothing on that step could match.
	//
	// DECLARED BY THE CALLER FROM ONE PLACE, not composed here or in the
	// client. The composition root reads it off the check that guards the
	// identity it states (check.ResidualNodes), and the reasons ride along
	// because they are what the node's rationale says to a reader. Omitted
	// from the JSON when empty, so the client reads an absent key as "this
	// step carries nothing across", which is every same-document step.
	Residual map[string]string `json:"residual,omitempty"`
}

// SideSource is [DrillStep.Side] for a step opening the node its chart's links
// come FROM, as against "", which opens the node they point at.
//
// A CONSTANT BECAUSE THE PACKAGER SPELLS IT. The two values are a vocabulary
// this package refuses outside of, and a caller's "Source" would otherwise be a
// declaration that validates and means nothing.
const SideSource = "source"

// TierCap is how many nodes one drawn tier may hold before its tail, by value,
// is folded into one aggregate node.
//
// IT IS NOT A TIDINESS SETTING. fisc-ppkq claims rescaling to a group's own
// total is what makes its funds legible, and that is measured false: the
// special-revenue group rescaled to itself still puts 22 of its 49 ribbons
// under one pixel, because the concentration is WITHIN the group -- one fund
// is 34.9% of it and the smallest two are 0.034%. Rescaling cannot fix a
// distribution. At cap 8 the same graph draws 2 sub-pixel ribbons.
type TierCap struct {
	Tier int `json:"tier"`
	Cap  int `json:"cap"`
	// Tail is the plural noun this tier's folded tail is counted in, or "" to
	// take the step's [DrillStep.Tail].
	//
	// PER CAP BECAUSE ONE STEP CAPS TWO TIERS OF DIFFERENT THINGS. A revenue
	// category opens into its printed lines and the funds they land in, and
	// both columns fold on the committed corpus -- so one noun on the step
	// labels the fund tail "26 smaller lines". The fund-group step had the same
	// defect latent: its division cap read "funds" and never engaged.
	Tail string `json:"tail,omitempty"`
}

// Download is one whole-store artifact a page offers.
type Download struct {
	Path  string
	Label string
	Note  string
	Bytes int
}

// ErrNoPrimary reports a projection set with no PrimaryProjection in it.
var ErrNoPrimary = errors.New("no " + PrimaryProjection + " projection to build the page from")

func (o *Options) validate() error {
	if o.Dir == "" {
		return errors.New("output directory is required")
	}
	if len(o.Projections) == 0 {
		return errors.New("no projections to export")
	}
	if _, ok := o.Projections[PrimaryProjection]; !ok {
		return ErrNoPrimary
	}
	for name := range o.Projections {
		if name == "" || strings.ContainsAny(name, `/\.`) {
			return fmt.Errorf("projection name %q is not a usable filename stem", name)
		}
	}
	for i := range o.Views {
		if err := o.views()[i].validate(o.Projections); err != nil {
			return err
		}
	}
	seenPath := map[string]bool{}
	index := 0
	for _, v := range o.views() {
		if seenPath[v.Path] {
			return fmt.Errorf("two views claim the output path %q", v.Path)
		}
		seenPath[v.Path] = true
		if v.Path == IndexPath {
			index++
		}
	}
	// Exactly one, not at least one: a site with no index.html has no entry
	// point and a caller listing it twice has already been refused above, so
	// what is left to say is that the front door is a single view.
	if index != 1 {
		return fmt.Errorf("%d views are at %s; the site opens on exactly one", index, IndexPath)
	}
	// A NAV LABEL IS ONLY OWED WHEN A NAV IS RENDERED, which is why this is
	// here and not in View.validate: only Options knows how many views there
	// are. Every template guards its nav with {{if gt (len .Nav) 1}}, so a
	// single-view site draws none and a view with neither field loses nothing.
	// From two views up, buildSite falls back Nav -> Title and has nothing
	// after that, so the nav ships <a href="trends.html"></a> -- a link a
	// reader can see, cannot read, and can still click.
	//
	// Refused rather than defaulted to path.Base(v.Path). A filename is not a
	// label, and dropping the caller's intent into one is what the Lede rule
	// above declines to do.
	if len(o.views()) > 1 {
		for _, v := range o.views() {
			if v.Nav == "" && v.Title == "" {
				return fmt.Errorf(
					"view %q has neither a nav label nor a title; every page's nav "+
						"would list it as an empty link", v.Path)
			}
		}
	}
	for rel := range o.Files {
		if err := assetPath(rel, o.reservedPaths()); err != nil {
			return err
		}
	}
	// A PUBLISHED LOCATOR MUST POINT AT BYTES THIS SITE SHIPS. The provenance
	// page's whole claim is that a citation resolves; an entry naming a file
	// the caller did not supply publishes a link that 404s, which is worse
	// than publishing nothing.
	for _, e := range o.PageIndex {
		switch {
		case e.DocID == "":
			return errors.New("a page index entry names no document")
		case e.Page < 1:
			return fmt.Errorf("page index entry for %q is at page %d", e.DocID, e.Page)
		case e.Data == "":
			return fmt.Errorf("page index entry %s p%d names no file", e.DocID, e.Page)
		}
		b, ok := o.Files[e.Data]
		if !ok {
			return fmt.Errorf("page index entry %s p%d publishes %q, which is not among "+
				"the files to be written", e.DocID, e.Page, e.Data)
		}
		// Checked rather than trusted: the size is printed to a reader as a
		// promise about a download, and a caller computing it from something
		// other than these bytes is how it comes to be wrong.
		if e.Bytes != len(b) {
			return fmt.Errorf("page index entry %s p%d says %q is %d bytes and it is %d",
				e.DocID, e.Page, e.Data, e.Bytes, len(b))
		}
		// AND THE BASE THE CLIENT COMPOSES WITH MUST BE THE DIRECTORY THE
		// RECORDS WERE ACTUALLY WRITTEN INTO. Both sides come from the same
		// producer, so this cannot witness a wrong path rule -- it catches a
		// MIS-WIRED CALLER, one that fills RecordsBase from a different source
		// than the entries, which is how the client would come to compose a
		// URL for a file no one wrote. Claimed as that and no more.
		if base, ok := o.RecordsBase[e.DocID]; ok {
			// A DIRECTORY PREFIX, NOT A STRING PREFIX. Without the trailing
			// separator "facts/d/pages" is a clean prefix of
			// "facts/d/pages/p0066.jsonl" and the client composes
			// "facts/d/pagesp0066.jsonl" -- every records anchor 404s, from a
			// base that validated. Options.Build is a public seam, so a caller
			// reaching for path.Join instead of shardBase is the likely way in,
			// and it is exactly the mis-wired caller this guard claims to catch.
			// EXACT DIRECTORY, not a prefix and not an ancestor. A prefix test
			// accepts "facts/" while the shards live at "facts/<doc>/pages/",
			// and the client -- which appends only a filename -- then composes
			// "facts/p0066.jsonl" and 404s every anchor. The trailing-separator
			// arm alone does not catch that: "facts/" has one. Both failures
			// are the same mis-wired caller and this states the relationship
			// the client actually relies on, which is that the base IS the
			// entry's directory.
			if want := path.Dir(e.Data) + "/"; base != want {
				return fmt.Errorf("records base for %s is %q, but page index entry p%d "+
					"has its records in %q; the client appends only a filename to the base",
					e.DocID, base, e.Page, want)
			}
		}
	}
	// A base for a document the site publishes no records of is a link the
	// client would build and nothing would answer.
	for doc := range o.RecordsBase {
		if !slices.ContainsFunc(o.PageIndex, func(e PageIndexEntry) bool { return e.DocID == doc }) {
			return fmt.Errorf("records base names document %q, which no page index entry does", doc)
		}
	}
	for _, d := range o.Downloads {
		b, ok := o.Files[d.Path]
		if !ok {
			return fmt.Errorf("download %q is offered and is not among the files to be written",
				d.Path)
		}
		if d.Bytes != len(b) {
			return fmt.Errorf("download %q is offered as %d bytes and is %d", d.Path, d.Bytes, len(b))
		}
		if d.Label == "" {
			return fmt.Errorf("download %q has no label", d.Path)
		}
	}
	return nil
}

// fixedPaths are the output paths Write owns whatever the caller asked for. An
// asset landing on one of them would not be an extra file but a replaced one:
// the site would still export, and the page would be broken in the browser only.
//
// IT NO LONGER CONTAINS index.html, and that is the point of the split. The
// pages are now a property of Options -- one per View -- so which paths are
// reserved is too, and a package-level set could only ever know about the one
// page that used to exist. An asset at trends.html would have shadowed a view
// silently.
var fixedPaths = func() map[string]bool {
	m := map[string]bool{markerName: true}
	for _, name := range verbatimAssets {
		m[name] = true
	}
	return m
}()

// reservedPaths is fixedPaths plus this Options' own views.
func (o *Options) reservedPaths() map[string]bool {
	m := make(map[string]bool, len(fixedPaths)+len(o.Views)+1)
	maps.Copy(m, fixedPaths)
	for _, v := range o.views() {
		m[v.Path] = true
	}
	return m
}

// views is Views, or the single Sankey page a caller that named none meant.
//
// The default is here rather than in every reader so that "no views" and "the
// one view this site had before views existed" are the same thing to everything
// downstream. It also keeps the existing callers -- and the existing output --
// working unchanged.
func (o *Options) views() []View {
	if len(o.Views) > 0 {
		return o.Views
	}
	return []View{{
		Path:       IndexPath,
		Nav:        "Budget flows",
		Template:   SankeyTemplate,
		Projection: PrimaryProjection,
	}}
}

// validate refuses a view that could not be rendered, or that would land on a
// path the site owns.
func (v View) validate(built map[string][]byte) error {
	badRoot := v.rootOutsideRenderTiers()
	switch {
	case v.Path == "":
		return errors.New("a view has no output path")
	// THIS ARM RUNS BEFORE THE SUFFIX CHECK, and the order is the whole point.
	// Behind it, every fixedPaths key -- .fisc-export, app.js, style.css,
	// .nojekyll -- was caught first by "is not an .html file", and any
	// data/x.html by the flat-root check, so the branch could never fire on any
	// input. views_test.go's case named "a path that shadows an asset" asserted
	// "not an .html file", which is to say it pinned the arm's unreachability
	// rather than the shadowing it is named for.
	//
	// The distinction matters to whoever hits it: "app.js is not an .html file"
	// invites you to rename it to app.html, which shadows nothing and is still
	// wrong. "app.js is part of the fixed site layout" says why.
	case fixedPaths[v.Path], strings.HasPrefix(v.Path, dataDir+"/"):
		return fmt.Errorf("view path %q is part of the fixed site layout", v.Path)
	case !strings.HasSuffix(v.Path, ".html"):
		return fmt.Errorf("view path %q is not an .html file", v.Path)
	case path.Base(v.Path) != v.Path:
		// Flat, per the View doc comment: every asset path in the output is
		// relative, so a page in a subdirectory would need ../ on all of them.
		return fmt.Errorf("view path %q is not at the site root", v.Path)
	// BEFORE ANYTHING ABOUT WHAT A TEMPLATE RENDERS, because nothing can be
	// said about the renderings of a template that is not named. With this arm
	// below the pair that follows, an empty Template made
	// templateRendersADocument return false and a view with a projection was
	// refused as "renders template \"\", which renders no document" -- true,
	// and useless next to "names no template".
	case v.Template == "":
		return fmt.Errorf("view %q names no template", v.Path)
	// A WEAKENING, NOT A THIRD RULE OF THE FAMILY BELOW, and it is worth being
	// plain about that. Lede, YearStems and RenderTiers each REFUSE a field a
	// template cannot render. This arm stops refusing something: a template
	// that renders no projection document -- the provenance index, which is
	// built from Options.PageIndex and decodes nothing -- has no projection to
	// name, and requiring one would mean naming an unrelated document to
	// satisfy a guard. Both directions are refused so the weakening stays
	// narrow: a template that DOES render a document still must name one.
	case v.Projection == "" && templateRendersADocument(v.Template):
		return fmt.Errorf("view %q names no projection", v.Path)
	case v.Projection != "" && templateIsKnown(v.Template) && !templateRendersADocument(v.Template):
		return fmt.Errorf(
			"view %q names projection %q and renders template %q, which renders no "+
				"document; the projection would be ignored", v.Path, v.Projection, v.Template)
	case v.Lede != "" && !templateRendersLede(v.Template):
		return fmt.Errorf(
			"view %q sets a lede and renders template %q, which has no {{.Lede}}; "+
				"the sentence would be dropped in silence", v.Path, v.Template)
	case len(v.YearStems) > 0 && !templateRendersAYearControl(v.Template):
		return fmt.Errorf(
			"view %q lists %d year stems and renders template %q, which has no year "+
				"control; the years would be dropped in silence",
			v.Path, len(v.YearStems), v.Template)
	case len(v.Sections) > 0 && !templateRendersSections(v.Template):
		return fmt.Errorf(
			"view %q declares %d sections and renders template %q, which groups nothing; "+
				"the headings would be dropped in silence",
			v.Path, len(v.Sections), v.Template)
	case len(v.Sections) == 0 && templateRendersSections(v.Template):
		return fmt.Errorf(
			"view %q renders template %q and declares no sections, so every row would "+
				"land under no printed heading", v.Path, v.Template)
	case len(v.RenderTiers) > 0 && !templateRendersTiers(v.Template):
		return fmt.Errorf(
			"view %q asks for render tiers %v and renders template %q, which publishes "+
				"none; the chart would draw every tier", v.Path, v.RenderTiers, v.Template)
	case len(v.Steps) > 0 && !templateRendersSteps(v.Template):
		return fmt.Errorf(
			"view %q declares a drill chain and renders template %q, which publishes none; "+
				"the chart would isolate on a click while this view believes it opens",
			v.Path, v.Template)
	case v.ChartSubject != "" && !templateRendersChartDeclarations(v.Template):
		return fmt.Errorf(
			"view %q names a chart subject and renders template %q, which composes its "+
				"own; the phrase would be dropped in silence", v.Path, v.Template)
	case v.Template == ChartTemplate && v.ChartSubject == "":
		return fmt.Errorf(
			"view %q renders a chart and names no subject, so its diagram would announce "+
				"itself to a screen reader as a chart of nothing in particular", v.Path)
	case v.Template == ChartTemplate && v.ChartDescription == "":
		return fmt.Errorf(
			"view %q renders a chart and gives it no description, so a screen reader "+
				"reaches its <desc> and is told nothing about what the marks mean", v.Path)
	case v.ChartDescription != "" && !templateRendersChartDeclarations(v.Template):
		return fmt.Errorf(
			"view %q describes a chart and renders template %q, which has no <desc> of "+
				"its own to fill; the sentence would be dropped in silence", v.Path, v.Template)
	case v.ChartDescription != "" && !endsASentence(v.ChartDescription):
		return fmt.Errorf(
			"view %q gives its chart a description ending %q rather than in a sentence "+
				"terminator; the template appends the pointer to the flow table after it, "+
				"and app.js re-appends that pointer on a drill by taking the description's "+
				"LAST SENTENCE -- so an unterminated one runs into it and a drilled reader "+
				"loses the only route they have to a table that ships closed",
			v.Path, lastRune(v.ChartDescription))
	case v.Root != "" && !templateRendersChartDeclarations(v.Template):
		return fmt.Errorf(
			"view %q declares root %q and renders template %q, which publishes none; the "+
				"chart would draw the whole document", v.Path, v.Root, v.Template)
	// TWO ARMS, NOT ONE ARM BEHIND A `len(v.RenderTiers) > 0 &&` GUARD, and
	// not one plain arm either. RenderTiers empty means the document is drawn
	// WHOLE, every tier on screen, so a root step's From is drawn by
	// definition: the spine draws whole and its tier 2 IS drawn. Without the
	// guard, `!slices.Contains(v.RenderTiers, From)` refuses exactly that view.
	// With the guard wrapping the only arm, the configuration that most needs
	// refusing passes outright: a view on a template whose documents need
	// folding, with a chain and NO tier set, ships the breadcrumb and the
	// "click to open" hint over an unfolded 61-node column that lays every
	// node out at zero height. So the first arm says only what it can know,
	// and the hazard the guard hid is the second arm's, stated on its own.
	//
	// EVERY ROOT, NOT THE FIRST STEP. A view may declare several steps that
	// open from its own chart; each is placed against RenderTiers here, and
	// every other step is placed against its parent by validateSteps.
	case len(v.RenderTiers) > 0 && badRoot >= 0:
		return fmt.Errorf(
			"view %q's step %d drills from tier %d and draws tiers %v, which do not include "+
				"it; the page would ship the breadcrumb and the words about opening a node "+
				"while no node on it is ever openable",
			v.Path, badRoot, v.Steps[badRoot].From, v.RenderTiers)
	case len(v.Steps) > 0 && len(v.RenderTiers) == 0 && templateRendersTiers(v.Template):
		return fmt.Errorf(
			"view %q drills and declares no render tiers on template %q, which publishes "+
				"render tiers because its documents cannot be drawn whole; the page would "+
				"ship the breadcrumb over an unfolded column that lays every node out at "+
				"zero height", v.Path, v.Template)
	}
	if _, ok := built[v.Projection]; !ok && v.Projection != "" {
		// Named rather than "a projection is missing": the fix differs by which
		// side is wrong, and an operator holding both names can tell.
		return fmt.Errorf("view %q renders projection %q, which was not built", v.Path, v.Projection)
	}
	for i, stem := range v.YearStems {
		if _, ok := built[stem]; !ok {
			return fmt.Errorf("view %q lists year stem %q, which names no projection that was built",
				v.Path, stem)
		}
		if i == 0 && stem != v.Projection {
			return fmt.Errorf("view %q opens on %q but its first year stem is %q",
				v.Path, v.Projection, stem)
		}
		for _, other := range v.YearStems[:i] {
			if other == stem {
				return fmt.Errorf("view %q lists year stem %q twice", v.Path, stem)
			}
		}
	}
	return v.validateSteps(built)
}

// rootOutsideRenderTiers is the first step opening from the view's own chart at
// a tier the view does not draw, or -1 when every root is placed. Its caller
// guards it on a non-empty RenderTiers, which is where the reason lives.
func (v View) rootOutsideRenderTiers() int {
	for i, s := range v.Steps {
		if s.After == "" && !slices.Contains(v.RenderTiers, s.From) {
			return i
		}
	}
	return -1
}

// validateSteps refuses a drill tree a reader could not walk, and a step that
// would fold nothing, say nothing, draw a document that was not built, or draw
// one year's document under another year's chart.
//
// EVERY STEP IS PLACED AGAINST ITS PARENT. A step opens from the chart its
// After names -- or from the view's own, for a root -- so its From must be a
// tier that chart draws, which is what makes every breadcrumb rung reachable
// from the one above it. Steps sharing an After are two edges out of one chart,
// which is what a tier-0 revenue category opening beside a tier-2 fund group
// needs and what a path could not declare at all (fisc-ko1j.12).
//
// A CYCLE CANNOT BE DECLARED, SO NOTHING HERE DETECTS ONE. After may only name
// a step EARLIER in the list, so every parent chain ends at a root in at most
// as many hops as there are steps. The arms below say what a declared tree must
// satisfy; that it is a tree is the type's property, not theirs.
func (v View) validateSteps(built map[string][]byte) error {
	// KEYS FIRST, AS A PASS OF THEIR OWN, so that After below resolves against
	// a set already known to name one step each. Interleaved with the arms that
	// follow, a duplicate key later in the list would be met by whichever arm
	// its first parent broke, and the reader would be told about a tier.
	index := make(map[string]int, len(v.Steps))
	for i, s := range v.Steps {
		if s.Key == "" {
			return fmt.Errorf(
				"view %q declares step %d with no key; a step nothing can name is a chart no "+
					"other step can ever be declared to open from", v.Path, i)
		}
		if j, ok := index[s.Key]; ok {
			return fmt.Errorf(
				"view %q declares steps %d and %d both keyed %q; After would name two charts "+
					"and a rung would open from whichever was declared first",
				v.Path, j, i, s.Key)
		}
		index[s.Key] = i
	}
	// The document each step draws, filled in declaration order: a step naming
	// no projection draws its parent's, and a parent is always earlier.
	docs := make([]string, len(v.Steps))
	for i, s := range v.Steps {
		// The tiers on screen before this step opens, and the document they
		// belong to. For a root that is the view's own chart, and RenderTiers
		// empty means the whole document -- which is why a root's From is
		// placed by validate's two arms above rather than here.
		parentTiers, parentDoc := v.RenderTiers, v.Projection
		if s.After != "" {
			j, ok := index[s.After]
			switch {
			case !ok:
				return fmt.Errorf(
					"view %q's step %d opens from %q, which no step declares as its key; the "+
						"breadcrumb would carry a rung hanging off a chart this view never draws",
					v.Path, i, s.After)
			case j >= i:
				return fmt.Errorf(
					"view %q's step %d opens from %q, which is step %d; After names an EARLIER "+
						"step, and that is what makes a cycle undeclarable rather than something "+
						"this has to detect", v.Path, i, s.After, j)
			}
			parentTiers, parentDoc = v.Steps[j].Tiers, docs[j]
		}
		doc := s.Projection
		if doc == "" {
			doc = parentDoc
		}
		docs[i] = doc
		switch {
		case len(s.Tiers) == 0:
			return fmt.Errorf(
				"view %q declares step %d with no tiers, so a node opened on it would be "+
					"drawn by the same tier set it was closed under", v.Path, i)
		case s.Tail == "":
			return fmt.Errorf(
				"view %q declares step %d with no tail noun, so a capped column would be "+
					"labelled \"24 smaller\" and stop there", v.Path, i)
		case s.Back == "":
			return fmt.Errorf(
				"view %q declares step %d with no back label, so the breadcrumb out of an "+
					"opened node would be a button with no words in it", v.Path, i)
		case s.Description == "":
			return fmt.Errorf(
				"view %q declares step %d with no description, so a reader who cannot see "+
					"the chart would be told the opening state's over a chart it no longer "+
					"draws", v.Path, i)
		case !endsASentence(s.Description):
			return fmt.Errorf(
				"view %q gives step %d a description ending %q rather than in a sentence "+
					"terminator; the client appends the way back and the table pointer after "+
					"it, and an unterminated one runs into them", v.Path, i, lastRune(s.Description))
		// A RESIDUAL NEEDS TWO DOCUMENTS AND A REASON. On a same-document
		// step there is no second grain, so a set declared there would be
		// carried by the client against a document that decomposes every
		// one of its own links -- a node of nothing, or worse, a copy of a
		// link the chart already draws. And an endpoint with no reason
		// draws a mark whose rationale explains nothing, which is the
		// derived-node rule broken in output.
		case s.Projection == "" && len(s.Residual) > 0:
			return fmt.Errorf(
				"view %q's step %d carries a residual of %d endpoint(s) and draws the "+
					"document before it; a residual is what one document prints at a grain "+
					"the other does not, and a step that switches no document has no second "+
					"grain", v.Path, i, len(s.Residual))
		// THE PER-YEAR JOIN IS EXACT OR REFUSED. A year with no entry would
		// have the client open the year's chart into a file it was never
		// told about; an entry for no year is a claim about a document the
		// page cannot show; and an opening-year entry disagreeing with
		// Projection is two answers to which file the first drill fetches.
		case s.Projection == "" && len(s.YearProjections) > 0:
			return fmt.Errorf(
				"view %q's step %d names no projection and maps %d year(s) to one; a step "+
					"drawing the document before it draws that document's every year, so the "+
					"map would be ignored", v.Path, i, len(s.YearProjections))
		case s.Projection != "" && len(v.YearStems) == 0 && len(s.YearProjections) > 0:
			return fmt.Errorf(
				"view %q's step %d maps %d year(s) and the view lists no year stems; there "+
					"is no year on screen for the map to be read against",
				v.Path, i, len(s.YearProjections))
		// A SIDE IS A VOCABULARY OF TWO. Anything else is a declaration that
		// would be carried to a client reading it against the two it knows,
		// which is a step that silently opens the wrong end of its links.
		case s.Side != "" && s.Side != SideSource:
			return fmt.Errorf(
				"view %q's step %d opens side %q; the sides are \"\", the node a link points "+
					"at, and %q, the node it comes from", v.Path, i, s.Side, SideSource)
		case s.After != "" && !slices.Contains(parentTiers, s.From):
			return fmt.Errorf(
				"view %q's step %d opens from tier %d, and step %q draws tiers "+
					"%v, which do not include it; the breadcrumb would carry a rung nothing "+
					"on the chart can reach", v.Path, i, s.From, s.After, parentTiers)
		// THE SAME DOCUMENT ONLY. Across a document switch the two tier sets
		// are numbered by different hierarchies, and equal numbers are not the
		// same chart -- the DrillStep doc comment says why validate cannot do
		// better than skip.
		case s.After != "" && doc == parentDoc && slices.Equal(parentTiers, s.Tiers):
			return fmt.Errorf(
				"view %q's step %d draws tiers %v of %q, the set step %q "+
					"already draws; opening a node would redraw the chart it was opened "+
					"from", v.Path, i, s.Tiers, doc, s.After)
		}
		// ONE STEP PER (After, From, Role), AND A ROLE-LESS STEP TAKES THE
		// WHOLE TIER. Two steps a node matches are two rungs it opens into, and
		// the client would take whichever it found first -- the silent choice
		// the key and the role exist to make impossible. Compared against every
		// earlier step rather than through a set, because the two refusals want
		// to name the other step and say which of the three parts collided.
		for j, o := range v.Steps[:i] {
			if o.After != s.After || o.From != s.From {
				continue
			}
			if o.Role == s.Role {
				return fmt.Errorf(
					"view %q declares steps %d and %d both opening tier %d of the same chart "+
						"in role %q; a node there would open into two different charts",
					v.Path, j, i, s.From, s.Role)
			}
			if o.Role == "" || s.Role == "" {
				return fmt.Errorf(
					"view %q declares steps %d and %d both opening tier %d of the same chart, "+
						"in roles %q and %q; a step with no role opens EVERY node at its tier, "+
						"so it cannot share one with a step that names which nodes open",
					v.Path, j, i, s.From, o.Role, s.Role)
			}
		}
		for _, id := range slices.Sorted(maps.Keys(s.Residual)) {
			if id == "" || s.Residual[id] == "" {
				return fmt.Errorf(
					"view %q's step %d declares residual endpoint %q with reason %q; the node "+
						"that carries it tells the reader why no part of the opened node "+
						"receives that flow in the reason's words, and an empty one draws a "+
						"mark that explains nothing", v.Path, i, id, s.Residual[id])
			}
		}
		if s.Projection != "" && len(v.YearStems) > 0 {
			for _, stem := range v.YearStems {
				got, ok := s.YearProjections[stem]
				switch {
				case !ok:
					return fmt.Errorf(
						"view %q's step %d names no document for year stem %q; a reader on "+
							"that year would open a node into nothing", v.Path, i, stem)
				case stem == v.Projection && got != s.Projection:
					return fmt.Errorf(
						"view %q's step %d renders %q and maps the opening year %q to %q; one "+
							"step cannot draw two documents for one year",
						v.Path, i, s.Projection, stem, got)
				}
				if _, built := built[got]; !built {
					return fmt.Errorf(
						"view %q's step %d renders projection %q for year stem %q, which was "+
							"not built", v.Path, i, got, stem)
				}
			}
			for stem := range s.YearProjections {
				if !slices.Contains(v.YearStems, stem) {
					return fmt.Errorf(
						"view %q's step %d maps year stem %q, which the view does not list; "+
							"the entry describes a year no reader can switch to", v.Path, i, stem)
				}
			}
		}
		for j, c := range s.Caps {
			switch {
			case c.Cap < 1:
				return fmt.Errorf(
					"view %q's step %d caps tier %d at %d; the cap is what keeps a fine "+
						"column drawable and a column of one node is not a chart",
					v.Path, i, c.Tier, c.Cap)
			case !slices.Contains(s.Tiers, c.Tier):
				return fmt.Errorf(
					"view %q's step %d caps tier %d and draws tiers %v, which do not include "+
						"it; the cap would fold nothing, in silence", v.Path, i, c.Tier, s.Tiers)
			case slices.ContainsFunc(s.Caps[:j], func(o TierCap) bool { return o.Tier == c.Tier }):
				return fmt.Errorf("view %q's step %d caps tier %d twice", v.Path, i, c.Tier)
			}
		}
		if _, ok := built[doc]; s.Projection != "" && !ok {
			// Named for the same reason v.Projection's refusal is.
			return fmt.Errorf("view %q's step %d renders projection %q, which was not built",
				v.Path, i, s.Projection)
		}
	}
	return nil
}

// templateRendersLede answers whether a template has a {{.Lede}} to render.
//
// IT IS A PROPERTY OF THE TEMPLATE AND NOT A LIST OF EXCEPTIONS, which is the
// whole reason it exists as a function. The guard above read
// `v.Template != TrendsTemplate` while exactly one template rendered a lede,
// and that spelling has one failure mode: it goes stale silently the next time
// a template lands, refusing a lede on a page that would have rendered one
// perfectly well. That is what it did to the drill-down (fisc-5miz.5).
//
// A template ABSENT from this set is refused a lede rather than dropping it,
// which is the conservative direction: a page missing a sentence somebody wrote
// is louder than a page quietly not showing it.
func templateRendersLede(name string) bool {
	switch name {
	case TrendsTemplate, HistoryTemplate, ChartTemplate, ProvenanceTemplate, CaveatsTemplate:
		return true
	default:
		return false
	}
}

// templateRendersAYearControl answers whether a template has a year control to
// render [View.YearStems] in.
//
// The same shape as templateRendersLede above and for the same reason, but the
// trap it closes is a step worse: a lede dropped in silence loses a sentence,
// and year stems dropped in silence lose whole documents. The trends page took
// a four-stem list and rendered one year, with every check green, because the
// only thing that reads YearStems is a template arm that page does not have.
//
// TWO TEMPLATES, NOT ONE. The drill-down grew a year control after the bead
// that named this defect was filed, so a guard spelled
// `v.Template == SankeyTemplate` would have been born stale -- which is the
// exact failure templateRendersLede exists to document.
func templateRendersAYearControl(name string) bool {
	// ITS OWN SWITCH, NOT templateDrawsAChart'S. The two agree on every template
	// that exists, and delegating made them one predicate wearing two names --
	// which is what the fourteen lines above argue against: a template could
	// gain a chart without a year control, or a control without a chart, and
	// the failure each of them guards is different. A lede dropped in silence
	// loses a sentence; year stems dropped in silence lose whole documents.
	switch name {
	case SankeyTemplate, ChartTemplate:
		return true
	default:
		return false
	}
}

// templateDrawsAChart answers whether a template ships app.js and an SVG for it
// to draw into.
//
// IT IS NOT "renders a document". trends.html renders revenue-trends and ships
// no app.js at all -- its figures are a server-rendered table -- so a caveat on
// that document can be listed and can never be flagged on a mark. The caveats
// page promises a chart flag per document, and "some view names this stem" was
// the wrong test for it.
func templateDrawsAChart(name string) bool {
	switch name {
	case SankeyTemplate, ChartTemplate:
		return true
	default:
		return false
	}
}

// templateRendersADocument answers whether a template renders a projection
// document, as opposed to being built from Options directly.
//
// Every template did until the provenance index, which is an index OF the
// site's own artifacts and reads no projection at all.
//
// AN ALLOW-LIST, like its two siblings, and the first draft of this was the
// negated form -- `name != ProvenanceTemplate` -- which its own doc comment
// then claimed not to be. That form fails OPEN: a second document-less
// template would default to true, so View.validate would demand a projection
// the page cannot render and the mirror arm that catches an ignored projection
// would never fire. The allow-list fails the other way, loudly, and a new
// template needs an arm in buildSite's exhaustive switch regardless.
func templateRendersADocument(name string) bool {
	switch name {
	case SankeyTemplate, TrendsTemplate, HistoryTemplate, ChartTemplate:
		return true
	default:
		return false
	}
}

// templateIsKnown answers whether this package has a builder for a template.
//
// IT KEEPS buildSite's DISPATCH REFUSAL REACHABLE, which is its only job. The
// arm above refuses a projection named on a template that renders none, and
// without this clause it also swallowed a template with no builder AT ALL: an
// unrecognised name renders no document by this package's reckoning, so the
// view was refused before the dispatch ever saw it, with a message that told a
// reader to drop the projection. Following that advice slips the view past
// validate and into a decode of an empty stem.
//
// Measured: restoring the historical `default: buildSankeyPage` bug -- the one
// that rendered any unknown template as a page of blanks -- left
// TestATemplateWithNoArmIsRefusedRatherThanRenderedAsASpine PASSING, because
// the new message happened to contain both strings it asserted. A guard that
// passes over the defect it is named for is worse than no guard.
//
// Drift against buildSite's switch is benign in both directions, which is why
// there is no test pairing them: a template missing here loses the
// ignored-projection guard and is still dispatched correctly, and one missing
// there is refused by the dispatch.
func templateIsKnown(name string) bool {
	return templateRendersADocument(name) ||
		name == ProvenanceTemplate || name == CaveatsTemplate
}

// templateRendersSections answers whether a template groups its rows under
// [View.Sections] headings. The same family as the five around it, and both
// directions are refused in validate: headings dropped in silence lose the
// only thing telling p167's two same-labelled blocks apart.
func templateRendersSections(name string) bool {
	return name == HistoryTemplate
}

// templateRendersTiers answers whether a template publishes [View.RenderTiers]
// to the client.
//
// The third field of this family, and it was missing when the first two were
// closed -- which is the argument for writing them as a family rather than as
// three guards. Only buildChartPage puts RenderTiers in the config blob;
// buildSankeyPage omits the key entirely, and app.js reads
// `CONFIG.render_tiers ?? []`, so a fold asked for on the spine is not refused,
// not reported, and not applied: the chart draws every tier and looks like a
// chart rather than like a defect.
func templateRendersTiers(name string) bool {
	return name == ChartTemplate
}

// templateRendersSteps answers whether a template publishes [View.Steps] to the
// client.
//
// THE FOURTH FIELD OF THE FAMILY, and it exists for the same reason as the
// third. app.js reads `CONFIG.steps`, so a chain asked for on a template whose
// builder omits the key is not refused, not reported, and not applied. The page
// would then isolate on a click while its view believed it opened -- which
// looks like a chart rather than like a defect.
//
// BOTH CHART TEMPLATES, because both builders put Steps in the config blob and
// both templates ship the #breadcrumb and #chart-hint a chain comes back out of
// a node by. It is the arm that keeps the two interaction contracts apart: a
// view that sets Steps is declaring that activating a node OPENS it, and a
// template with no breadcrumb and no way back would make that a trapdoor.
func templateRendersSteps(name string) bool {
	return name == ChartTemplate || name == SankeyTemplate
}

// templateRendersChartDeclarations answers whether a template publishes
// [View.ChartSubject], [View.ChartDescription] and [View.Root].
//
// THREE FIELDS, ONE PREDICATE, AND NOT THE ONE Steps USES. Only buildChartPage
// renders a subject into the chart's accessible name, fills its <desc> from the
// caller and puts a root in the config blob; buildSankeyPage composes its own
// name, ships its own <desc> and draws its document whole. Sharing Steps'
// predicate would widen these three with it, and a Root on the spine would
// then pass validate and be dropped -- the silence this family exists to
// refuse.
func templateRendersChartDeclarations(name string) bool {
	return name == ChartTemplate
}

// endsASentence reports whether s closes with a terminator, which is what keeps
// a caller's chart description separable from the template's own sentence after
// it. The validate arm that calls this says what depends on the separation.
func endsASentence(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '!', '?':
		return true
	}
	return false
}

// lastRune is the final character of s as a string, so a refusal can show what
// a description actually ended with rather than quoting the whole sentence.
func lastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	return string(r[len(r)-1])
}

// assetPath screens one [Options.Files] key. The output tree is a web root
// somebody will unpack, serve, or rsync, so a path that escapes it is a defect
// worth refusing at the door rather than a file written outside Dir.
func assetPath(rel string, reserved map[string]bool) error {
	switch {
	case rel == "":
		return errors.New("asset path is empty")
	case strings.ContainsRune(rel, '\\'):
		return fmt.Errorf("asset path %q is not slash-separated", rel)
	case path.IsAbs(rel), rel == ".", rel == "..", strings.HasPrefix(rel, "../"), path.Clean(rel) != rel:
		return fmt.Errorf("asset path %q is not a clean relative path", rel)
	case reserved[rel], strings.HasPrefix(rel, dataDir+"/"), strings.HasPrefix(rel, "vendor/"):
		return fmt.Errorf("asset path %q is part of the fixed site layout", rel)
	}
	return nil
}

// plan is a site resolved in full and not yet written: every byte of every
// file, in the order it goes down.
//
// IT EXISTS SO A CALLER CAN LEARN ITS INPUT IS BAD BEFORE IT DESTROYS ANYTHING.
// `fisc export --clean` empties the output directory, and everything that can
// refuse an export -- validation, rendering, resolving the cited page text out
// of the extraction tree, reading the embedded assets -- needs nothing that
// cleaning produces. Leaving any of it inside the writer means a fault that was
// detectable up front is discovered after the reader's site is gone.
//
// That is not hypothetical and it is why this type replaced a plain
// Options.Validate(): validation alone was hoisted first, and withPageText was
// left behind inside Write, so an export whose store covers a page the
// extraction does not still emptied the directory and then refused. Reproduced
// by moving one committed page text aside.
type plan struct {
	dir   string
	files []plannedFile
}

// plannedFile is one output file, resolved.
type plannedFile struct {
	Path  string
	Bytes []byte
}

// Prepare resolves an Options into a Plan, or refuses it. It touches no
// filesystem outside the inputs it reads.
func Prepare(o Options) (*plan, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	// Where a page-text citation points is decided once, here, and handed to
	// buildSite as the base it composes: an explicit remote wins, otherwise the
	// site cites what it ships, and only a caller that offers neither falls
	// back to the constant.
	ship := o.SourceBrowseURL == "" && o.PageText != nil
	browse := o.SourceBrowseURL
	if browse == "" {
		browse = DefaultSourceBrowseURL
	}
	base := func(docID string) string { return remotePageTextBase(browse, docID) }
	if ship {
		base = LocalPageTextBase
	}

	pages, cited, err := buildSite(&o, base)
	if err != nil {
		return nil, err
	}

	files := o.Files
	if ship {
		files, err = withPageText(files, o.PageText, cited, o.reservedPaths())
		if err != nil {
			return nil, err
		}
	}

	plan := &plan{dir: o.Dir}
	seen := make(map[string]bool)
	add := func(rel string, b []byte) error {
		if seen[rel] {
			// Two writers landing on one path is a silent overwrite: the file
			// is there, the export succeeds, and which of the two bytes won
			// depends on map order.
			return fmt.Errorf("two assets claim the output path %q", rel)
		}
		seen[rel] = true
		plan.files = append(plan.files, plannedFile{Path: rel, Bytes: b})
		return nil
	}

	// The marker goes down first, before anything that can fail. A run that
	// dies midway then leaves a directory that --clean recognises as ours;
	// writing it last would dead-end the retry, because index.html is not
	// there either and SafeCleanDir would refuse to touch the debris.
	if err := add(markerName, []byte("fisc export\n")); err != nil {
		return nil, err
	}
	for _, name := range verbatimAssets {
		b, rerr := fs.ReadFile(o.assetTree(), name)
		if rerr != nil {
			return nil, fmt.Errorf("read embedded asset %q: %w", name, rerr)
		}
		if aerr := add(name, b); aerr != nil {
			return nil, aerr
		}
	}
	if err := copyTree(o.assetTree(), "vendor", add); err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(o.Projections) {
		if err := add(path.Join(dataDir, name+".json"), o.Projections[name]); err != nil {
			return nil, err
		}
	}
	for _, rel := range sortedKeys(files) {
		if err := add(rel, files[rel]); err != nil {
			return nil, err
		}
	}
	// The pages are written last, and index.html last of all: it is the
	// completed-site sentinel, so it must not exist until the site around it
	// does -- including the other views it links to, or the entry point would
	// appear while its own nav pointed at 404s.
	for _, pg := range pages {
		if pg.Path == IndexPath {
			continue
		}
		if err := add(pg.Path, pg.HTML); err != nil {
			return nil, err
		}
	}
	for _, pg := range pages {
		if pg.Path != IndexPath {
			continue
		}
		if err := add(pg.Path, pg.HTML); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

// Write puts a prepared Plan on disk, creating o.Dir if needed, and returns the
// slash-separated paths it wrote, relative to that directory and sorted.
//
// Everything that can be decided has been. What is left can fail only on I/O,
// so a caller that has cleaned a directory is past the point where anything it
// could have known in advance will stop it.
//
// It does not clean the directory: destroying files is a separate decision with
// its own guard rail (cmdutil.SafeCleanDir), and burying it in a writer would
// make every caller of Write a caller of RemoveAll.
func (p *plan) Write() ([]string, error) {
	// #nosec G301 -- the output is a web root; a directory a server running as
	// another user cannot traverse is a broken deploy, not a hardened one.
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	written := make([]string, 0, len(p.files))
	for _, f := range p.files {
		dst := filepath.Join(p.dir, filepath.FromSlash(f.Path))
		// #nosec G301 -- as above: these are published directories.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("create %q: %w", path.Dir(f.Path), err)
		}
		// #nosec G306 -- these are published web assets; 0600 would make the
		// output unreadable to a web server running as another user.
		if err := os.WriteFile(dst, f.Bytes, 0o644); err != nil {
			return nil, fmt.Errorf("write %q: %w", f.Path, err)
		}
		written = append(written, f.Path)
	}
	sort.Strings(written)
	return written, nil
}

// Write renders the site into o.Dir in one step, for a caller with nothing to
// destroy. A caller that cleans first wants Prepare, then Plan.Write.
func Write(o Options) ([]string, error) {
	plan, err := Prepare(o)
	if err != nil {
		return nil, err
	}
	return plan.Write()
}

// copyTree copies every file under root in fsys, preserving relative paths.
func copyTree(fsys fs.FS, root string, write func(string, []byte) error) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", root, err)
		}
		if d.IsDir() {
			return nil
		}
		b, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return fmt.Errorf("read embedded asset %q: %w", p, rerr)
		}
		return write(p, b)
	})
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// withPageText returns files plus the committed text of every cited page, read
// out of the extraction tree.
//
// It copies through [Options.Files] rather than around it: the page text is not
// a special kind of output, it is the first user of the general channel, and a
// second write path would be a second set of rules about what may land where.
// The input map is not modified — a caller's map is the caller's.
//
// A cited page the tree does not hold is an error. The alternative is a page
// that renders a citation nobody can follow, which is the failure this whole
// change exists to remove.
func withPageText(files map[string][]byte, tree fs.FS, cited []Citation, reserved map[string]bool) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files)+len(cited))
	maps.Copy(out, files)
	for _, c := range cited {
		src := path.Join(c.DocID, pageTextPath(c.Page))
		b, err := fs.ReadFile(tree, src)
		if err != nil {
			return nil, fmt.Errorf("read the extracted text of %s page %d: %w", c.DocID, c.Page, err)
		}
		rel := LocalPageTextBase(c.DocID) + pageTextFile(c.Page)
		if err := assetPath(rel, reserved); err != nil {
			return nil, err
		}
		// Assigning over a caller's asset would resolve the collision in
		// silence, and this map is written before Write's own duplicate guard
		// can see two claims on the path.
		if _, taken := out[rel]; taken {
			return nil, fmt.Errorf("two assets claim the output path %q", rel)
		}
		out[rel] = b
	}
	return out, nil
}

// LocalPageTextBase is the output directory holding one document's cited page
// text, relative to the site root and with the trailing slash the client
// appends a filename to. It is exported because it is where a caller has to
// look for what Write copied.
func LocalPageTextBase(docID string) string {
	return path.Join(PageTextDir, docID, "pages") + "/"
}

// pageTextPath is corpus.PagePath, spelled out for the same reason
// pageTextFile is: this package reads an fs.FS the caller rooted, and importing
// internal/corpus to compose two path elements would give the packager a
// dependency on the extraction reader it otherwise has no use for.
func pageTextPath(page int) string { return "pages/" + pageTextFile(page) }

// assetTree is the site source tree the templates and verbatim assets come
// from: the caller's, or the embedded one.
func (o *Options) assetTree() fs.FS {
	if o.Assets != nil {
		return o.Assets
	}
	return site.FS()
}
