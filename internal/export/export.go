// Package export packages a set of projection documents into a static site.
//
// The seam this package sits on is deliberate: it consumes projections as
// filename stem -> JSON bytes and knows nothing about how they were built. It
// names what it reads with internal/project's constants and types, and it does
// not recompute anything the projection already published. Every figure the page shows — the fiscal year
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
//	<dir>/data/<name>.json    the projections that fold into no column
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
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/site"
)

// PrimaryProjection is the spine document, and the default view's projection.
// It must be among the projections built (ErrNoPrimary). It is NOT required to
// be the view at IndexPath: which document a site opens on is the caller's.
const PrimaryProjection = project.PublishedProjection

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
var verbatimAssets = []string{"app.js", "core.js", "sankey.js", "style.css", ".nojekyll"}

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
	// heading. Lede is the sentence under it. Both are the caller's words.
	//
	// Only the trends template renders a Lede: the spine's lede wraps a live
	// <span id="lede-year"> that app.js rewrites on every year switch. A Lede
	// on a template that renders none is refused rather than dropped.
	Title string
	Lede  string
	// YearStems are the documents that are the same projection for different
	// fiscal years, OLDEST FIRST, which is the order a reader meets them in.
	// Empty means this view has one document and renders no year control.
	//
	// THE PAGE OPENS ON THE LAST OF THEM, so reordering this list changes
	// which year a reader is greeted with.
	YearStems []string

	// Sections are the printed blocks a history table groups its rows under,
	// in printed order, in the caller's words. buildHistoryPage refuses a
	// series no section claims and a section that claims no series.
	Sections []Section

	// Overview is the view's own chart, its form and that form's hints,
	// shipped as FISC_CONFIG.overview. Required on a template that draws a
	// chart and refused on one that does not. A Sankey overview with no tiers
	// draws the document whole.
	//
	// THE SANKEY'S TIERS ARE A COLUMN ORDER AND NOT ONLY A SET: site/app.js
	// aligns a node on the list's indexOf, so adjacency is declared.
	// validateSteps refuses a kept flank against a parent that declares none.
	// The fold is the client's.
	Overview Chart

	// Steps is how this view's chart opens a node, one hop per step, or empty
	// for a chart that does not open; a node click then isolates instead.
	// Shipped as FISC_CONFIG.steps.
	//
	// THE LIST IS FLAT AND THE SHAPE IS A DAG: a step names the charts it opens
	// from ([DrillStep.After]). What a declared tree must satisfy is
	// validateSteps'.
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
	// handed and does not decide it.
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
	// chart resolve to the facts behind it: a link publishes locators, and
	// base + [RecordsFile] is each cited page's records link in the config.
	//
	// THIS PACKAGE DOES NOT KNOW HOW THAT PATH IS BUILT AND MUST NOT LEARN.
	// The rule is spelled once, in whoever produced the records -- the same
	// rule PageIndexEntry.Data is written by, which is why validate can assert
	// the two agree instead of taking this on trust.
	//
	// UNLIKE PageTextBase THERE IS NO LOCAL/REMOTE FORK. A --source-browse-url
	// export ships no page text and cites a remote URL, so a page's text link
	// goes absolute; the shards are always written into the output tree, so
	// its records link stays site-relative. The two therefore differ in kind
	// while sitting beside each other in the same clientPage.
	//
	// A doc with no entry publishes records: "" and the client renders no
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

// DrillStep is one hop of a view's chart opening a node into its parts, by
// filtering and rescaling rather than expanding in place (fisc-ppkq records
// why d3-sankey cannot lay out the latter).
//
// FROM AND THE FORM'S TIERS ARE NOT NECESSARILY TIERS OF THE SAME DOCUMENT.
// From is a tier of the chart on screen; a Sankey's tiers are tiers of the
// document THIS step draws. On a step that switches document the two
// hierarchies are unrelated, so validateSteps never relates From to them.
//
// The fields here are the step's generic half, read by the client core and by
// validateStep whatever the form. What the form may fold is the embedded
// [Chart]'s, under the form's own key.
type DrillStep struct {
	// Key names this step for [DrillStep.After]. Required and unique within a
	// view.
	Key string `json:"key"`
	// After is the Keys of the steps whose charts this one opens from, and ""
	// is the view's own chart.
	//
	// EVERY ENTRY NAMES AN EARLIER STEP, so a cycle is undeclarable rather than
	// detected. A root says so with "", not with an empty list, which
	// validateSteps refuses.
	After []string `json:"after"`
	// From is the tier whose nodes open, in the chart on screen before they do.
	// One tier rather than a set: a step is one hop, and a second tier of the
	// same chart is a second step sharing this one's After.
	From int `json:"from"`
	// Role is which of the nodes at From open, in the caller's vocabulary, or
	// "" for all of them. This package gives it no meaning beyond requiring
	// (After, From, Role) to name at most one step.
	Role string `json:"role,omitempty"`
	// Projection is the SCHEDULE this step draws, or "" to draw the same
	// document as the step before it. Every column the view lists must carry
	// it. A schedule key and not a filename stem: the year is carried by the
	// column file the reader fetched ([ColumnIndex]).
	Projection string `json:"projection,omitempty"`
	// Chart is the form this step draws once a node has opened, and that
	// form's hints. Inlined on the wire: `form` and the form's own key.
	Chart
	// Back is what the breadcrumb's return control says, e.g. "All fund
	// groups". Declared rather than derived from From, because a tier number
	// does not know what the reader calls the things in it.
	Back string `json:"back"`
	// Tail is the plural noun the capped aggregate is counted in -- "funds",
	// "categories" -- so its label reads "24 smaller funds". A cap naming its
	// own [TierCap.Tail] takes that instead. Declared for Back's reason.
	Tail string `json:"tail"`
	// Noun is the singular noun for a node opened on this step -- "fund group",
	// "division" -- which the client uses only to tell two rungs of one trail
	// apart when the documents print them in the same words (Budget Book p66
	// and p255 both print "General Fund"). Required, though most trails never
	// draw it.
	Noun string `json:"noun"`
	// Description is the chart's long description once a node has opened on
	// this step, in the caller's words; the client writes it into the SVG's
	// <desc>. Required, and terminated because the client appends sentences.
	Description string `json:"description"`
	// Residual is the set of endpoints of the chart this step opens FROM whose
	// flow into or out of the opened node the document this step DRAWS does
	// not decompose: node id to reason. The client copies those links onto one
	// derived node beside the opened node's parts.
	//
	// Only on a step that switches document. The caller derives it from the
	// cuts and exceptions internal/structure declares.
	Residual map[string]string `json:"residual,omitempty"`
	// ResidualGrain is the grain the document this step draws does NOT split
	// that money by, in the city's singular word -- "fund". The client's
	// carryResidual names the mark for it. Required wherever Residual is
	// non-empty.
	ResidualGrain string `json:"residual_grain,omitempty"`
	// Gaps is the set of nodes this step OPENS whose total the document it
	// draws does not reach: node id to the licences for each column it differs
	// in, by how much, and why. The client's markGap holds the difference the
	// drawn chart comes to against the licence for its column and draws the
	// shortfall as one derived node, or refuses the chart.
	//
	// A gap key is the opened node itself; a [DrillStep.Residual] key is an
	// endpoint of the chart above. A step declaring a gap claims every other
	// node it opens balances. Only on a step that switches document.
	Gaps map[string][]project.Gap `json:"gaps,omitempty"`
}

// Chart is one drawn chart's form and that form's hints under the form's own
// key: the overview's on [View.Overview], a step's embedded in [DrillStep]. A
// hint foreign to the declared form is refused by schema/page.schema.json at
// the write, and the client core reads none of them. A second form adds one
// value to [ChartForms], one hint type here, one closed object in the schema
// and one renderer module under site/.
type Chart struct {
	// Form is which renderer draws the chart, one of [ChartForms].
	Form string `json:"form"`
	// Sankey is the Sankey form's hints, present exactly when Form is
	// [SankeyForm].
	Sankey *SankeyHints `json:"sankey,omitempty"`
}

// SankeyForm is the form site/sankey.js draws: columns of tiers joined by
// ribbons.
const SankeyForm = "sankey"

// ChartForms is the declared set of forms, which schema/enums.schema.json
// states as chart_form and schema/enums_test.go holds to this.
func ChartForms() []string { return []string{SankeyForm} }

// SankeyHints is what the Sankey form may fold and where. Go declares; the
// client's capColumn and foldDocument spend the declaration.
type SankeyHints struct {
	// Tiers is the tier set drawn, left to right: on a step, once a node has
	// opened, and a different declaration from the chart's before it; on the
	// overview, the page's own column order, or empty to draw the document
	// whole.
	Tiers []int `json:"tiers,omitempty"`
	// Keep is the flank of the chart on screen that stays drawn beside the
	// opened node, NEAREST THE CENTRE FIRST, or empty for a step that draws the
	// opened node's parts alone. A slice and not an int because tier 0 is a
	// real tier. The entries are contiguous and on one side of the opened tier.
	//
	// With an entry, Tiers is the flank, the opened node and what it opens
	// into, plus one column per [SankeyHints.Widen] entry, with the flank at
	// the end the parent's column order names. validateSteps checks that side
	// against every chart this step opens from.
	Keep []int `json:"keep,omitempty"`
	// Widen is the tier this step adds for each column beyond the window's own
	// three, in the order they are added; a client with room for fewer drops
	// them from the end. A widened column sits at the end of Tiers away from
	// the kept flank, and validateSteps refuses a Tiers that disagrees.
	// Refused on a step that keeps nothing.
	Widen []int `json:"widen,omitempty"`
	// Caps bounds the columns this step draws, one per tier that needs one; a
	// tier with no cap is drawn whole.
	Caps []TierCap `json:"caps,omitempty"`
	// Side is which end of a link the opened node sits on: "" for the node the
	// links point AT, [SideSource] for the node they come FROM. Declared, never
	// inferred from tier numbers.
	Side string `json:"side,omitempty"`
}

// SideSource is [SankeyHints.Side] for a step opening the node its chart's
// links come FROM.
const SideSource = "source"

// drawnTiers is the column order a chart of this form draws, or nil for a
// Sankey drawing its document whole.
func (c Chart) drawnTiers() []int {
	if c.Form == SankeyForm && c.Sankey != nil {
		return c.Sankey.Tiers
	}
	return nil
}

// keptFlank is a Sankey chart's kept flank, or nil.
func (c Chart) keptFlank() []int {
	if c.Form == SankeyForm && c.Sankey != nil {
		return c.Sankey.Keep
	}
	return nil
}

// TierCap is how many nodes one drawn tier may hold before its tail, by value,
// is folded into one aggregate node. Rescaling alone does not make a group's
// funds legible: the concentration is within the group.
type TierCap struct {
	Tier int `json:"tier"`
	Cap  int `json:"cap"`
	// Tail is the plural noun this tier's folded tail is counted in, or "" to
	// take the step's [DrillStep.Tail]. Per cap because one step may cap two
	// tiers of different things.
	Tail string `json:"tail,omitempty"`
}

// Download is one whole-store artifact a page offers.
type Download struct {
	Path  string
	Label string
	Note  string
	Bytes int
}

// ColumnPath is the file one published column ships at, at the site root.
func ColumnPath(year int, basis string) string {
	return fmt.Sprintf("fy%d-%s.json", year, basis)
}

// ErrNoPrimary reports a projection set with no PrimaryProjection in it.
var ErrNoPrimary = errors.New("no " + PrimaryProjection + " projection to build the page from")

func (o *Options) validate(ix ColumnIndex) error {
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
		if err := o.views()[i].validate(o.Projections, ix); err != nil {
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
	if index != 1 {
		return fmt.Errorf("%d views are at %s; the site opens on exactly one", index, IndexPath)
	}
	// A nav label is only owed when a nav is rendered, from two views up;
	// without one the nav ships an empty link.
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
	// A published locator must point at bytes this site ships.
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
		if e.Bytes != len(b) {
			return fmt.Errorf("page index entry %s p%d says %q is %d bytes and it is %d",
				e.DocID, e.Page, e.Data, e.Bytes, len(b))
		}
		// The base must be exactly the entry's directory, trailing slash
		// included: the client appends only a filename. Both sides come from
		// one producer, so this catches a mis-wired caller, not a wrong path
		// rule.
		if base, ok := o.RecordsBase[e.DocID]; ok {
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

// fixedPaths are the output paths Write owns whatever the caller asked for.
// The views' own paths are added per Options by reservedPaths.
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
func (o *Options) views() []View {
	if len(o.Views) > 0 {
		return o.Views
	}
	return []View{{
		Path:       IndexPath,
		Nav:        "Budget flows",
		Template:   SankeyTemplate,
		Projection: PrimaryProjection,
		Overview:   Chart{Form: SankeyForm, Sankey: &SankeyHints{}},
	}}
}

// validate refuses a view that could not be rendered, or that would land on a
// path the site owns.
func (v View) validate(built map[string][]byte, ix ColumnIndex) error {
	badRoot := v.rootOutsideOverview()
	switch {
	case v.Path == "":
		return errors.New("a view has no output path")
	// Before the suffix check, or every fixedPaths key is reported as "not an
	// .html file" and this arm is unreachable.
	case fixedPaths[v.Path], strings.HasPrefix(v.Path, dataDir+"/"):
		return fmt.Errorf("view path %q is part of the fixed site layout", v.Path)
	case !strings.HasSuffix(v.Path, ".html"):
		return fmt.Errorf("view path %q is not an .html file", v.Path)
	case path.Base(v.Path) != v.Path:
		return fmt.Errorf("view path %q is not at the site root", v.Path)
	// Before anything about what a template renders.
	case v.Template == "":
		return fmt.Errorf("view %q names no template", v.Path)
	// A template that renders no document (the provenance index) names no
	// projection; one that does must name one.
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
	case v.Overview.Form != "" && !templateDrawsAChart(v.Template):
		return fmt.Errorf(
			"view %q declares a %q chart and renders template %q, which draws none; "+
				"the form would be dropped in silence", v.Path, v.Overview.Form, v.Template)
	case v.Overview.Form == "" && templateDrawsAChart(v.Template):
		return fmt.Errorf(
			"view %q renders template %q, which draws a chart, and declares no form for "+
				"it; the client would have no renderer to hand the document to",
			v.Path, v.Template)
	case v.Overview.Form != "" && !slices.Contains(ChartForms(), v.Overview.Form):
		return fmt.Errorf(
			"view %q declares chart form %q, which is not one of %v",
			v.Path, v.Overview.Form, ChartForms())
	case v.Overview.Form == SankeyForm && v.Overview.Sankey == nil:
		return fmt.Errorf(
			"view %q declares a sankey chart with no sankey hints; the column order "+
				"is a declaration, and an absent one is not the same as an empty one",
			v.Path)
	case len(v.Steps) > 0 && !templateDrawsAChart(v.Template):
		return fmt.Errorf(
			"view %q declares a drill chain and renders template %q, which draws no "+
				"chart; the chain would be dropped in silence", v.Path, v.Template)
	// Two arms: a Sankey with no tiers draws the document whole, so every
	// root's From is drawn; the next arm refuses a drilling view with no tier
	// set.
	case len(v.Overview.drawnTiers()) > 0 && badRoot >= 0:
		return fmt.Errorf(
			"view %q's step %d drills from tier %d and draws tiers %v, which do not include "+
				"it; the page would ship the breadcrumb and the words about opening a node "+
				"while no node on it is ever openable",
			v.Path, badRoot, v.Steps[badRoot].From, v.Overview.drawnTiers())
	// A chart that opens declares its column order: without one nothing is
	// adjacent to anything, and a kept flank has no side.
	case len(v.Steps) > 0 && len(v.Overview.drawnTiers()) == 0 && templateDrawsAChart(v.Template):
		return fmt.Errorf(
			"view %q drills and declares no render tiers on template %q, which publishes "+
				"them; the page would ship the breadcrumb over a chart whose columns are "+
				"d3's own inference, so no tier is adjacent to any other and a document "+
				"that needs folding lays every node out at zero height",
			v.Path, v.Template)
	}
	if _, ok := built[v.Projection]; !ok && v.Projection != "" {
		return fmt.Errorf("view %q renders projection %q, which was not built", v.Path, v.Projection)
	}
	for i, stem := range v.YearStems {
		if _, ok := built[stem]; !ok {
			return fmt.Errorf("view %q lists year stem %q, which names no projection that was built",
				v.Path, stem)
		}
		for _, other := range v.YearStems[:i] {
			if other == stem {
				return fmt.Errorf("view %q lists year stem %q twice", v.Path, stem)
			}
		}
	}
	// The projection is one of the years, not necessarily the first; outside
	// the list, the page's non-per-year metadata would come from a document
	// the reader can never select.
	if len(v.YearStems) > 0 && !slices.Contains(v.YearStems, v.Projection) {
		return fmt.Errorf("view %q renders projection %q, which is not among its year stems %v",
			v.Path, v.Projection, v.YearStems)
	}
	return v.validateSteps(built, ix)
}

// rootOutsideOverview is the first step opening from the view's own chart at
// a tier the view does not draw, or -1 when every root is placed.
func (v View) rootOutsideOverview() int {
	for i, s := range v.Steps {
		if slices.Contains(s.After, "") && !slices.Contains(v.Overview.drawnTiers(), s.From) {
			return i
		}
	}
	return -1
}

// parentChart is one chart a step opens from: the key naming it, "" for the
// view's own, with the column order it draws (not a set: a kept flank's side
// is a position in it) and the document it draws them of.
type parentChart struct {
	key   string
	tiers []int
	doc   string
	// keep is that chart's own kept flank, or nil where it keeps none.
	keep []int
}

// validateSteps refuses a drill tree a reader could not walk, and a step that
// would fold nothing, say nothing, draw a document that was not built, or draw
// one year's document under another year's chart. Every step is placed
// against every parent it names. A cycle cannot be declared, since After names
// only earlier steps, so nothing here detects one.
func (v View) validateSteps(built map[string][]byte, ix ColumnIndex) error {
	// Keys first, as a pass of their own, so After resolves against a set
	// already known to name one step each.
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
	// The columns a reader can open, which a gap licence must name one of: the
	// view's years, or its own document where it lists none.
	years := map[string]bool{}
	for _, stem := range append([]string{v.Projection}, v.YearStems...) {
		if col, folded := ix.Column(stem); folded {
			years[col] = true
		}
	}
	// The document each step draws, filled in declaration order: a step naming
	// no projection draws its parent's, and a parent is always earlier.
	docs := make([]string, len(v.Steps))
	for i, s := range v.Steps {
		if len(s.After) == 0 {
			return fmt.Errorf(
				"view %q declares step %d opening from no chart at all; a step that opens "+
					"from the view's own chart says so with \"\", and an empty list is a rung "+
					"hanging off nothing", v.Path, i)
		}
		// "" is the view's own chart, whose From validate places.
		parents := make([]parentChart, 0, len(s.After))
		for k, a := range s.After {
			if slices.Contains(s.After[:k], a) {
				return fmt.Errorf(
					"view %q's step %d opens from %q twice; one chart reaching a step is one "+
						"edge, and every arm below would place the same chart twice and the "+
						"other parents once", v.Path, i, a)
			}
			if a == "" {
				parents = append(parents, parentChart{tiers: v.Overview.drawnTiers(), doc: v.Projection})
				continue
			}
			j, ok := index[a]
			switch {
			case !ok:
				return fmt.Errorf(
					"view %q's step %d opens from %q, which no step declares as its key; the "+
						"breadcrumb would carry a rung hanging off a chart this view never draws",
					v.Path, i, a)
			case j >= i:
				return fmt.Errorf(
					"view %q's step %d opens from %q, which is step %d; After names an EARLIER "+
						"step, and that is what makes a cycle undeclarable rather than something "+
						"this has to detect", v.Path, i, a, j)
			}
			parents = append(parents, parentChart{key: a, tiers: v.Steps[j].drawnTiers(), doc: docs[j],
				keep: v.Steps[j].keptFlank()})
		}
		doc := s.Projection
		if doc == "" {
			// One document before it, or name one: otherwise the rung's file
			// would depend on the route the reader took.
			doc = parents[0].doc
			for _, p := range parents[1:] {
				if p.doc != doc {
					return fmt.Errorf(
						"view %q's step %d names no projection and opens from charts drawing "+
							"%q and %q; a step that draws the document before it needs ONE "+
							"document before it", v.Path, i, doc, p.doc)
				}
			}
		}
		docs[i] = doc
		switch {
		case !slices.Contains(ChartForms(), s.Form):
			return fmt.Errorf(
				"view %q declares step %d in form %q, which is not one of %v; the client "+
					"would have no renderer to hand the opened node to", v.Path, i, s.Form, ChartForms())
		case s.Form == SankeyForm && s.Sankey == nil:
			return fmt.Errorf(
				"view %q declares step %d as a sankey with no sankey hints; what it draws "+
					"once a node opens is a declaration and not an inference", v.Path, i)
		case s.Tail == "":
			return fmt.Errorf(
				"view %q declares step %d with no tail noun, so a capped column would be "+
					"labelled \"24 smaller\" and stop there", v.Path, i)
		case s.Back == "":
			return fmt.Errorf(
				"view %q declares step %d with no back label, so the breadcrumb out of an "+
					"opened node would be a button with no words in it", v.Path, i)
		case s.Noun == "":
			return fmt.Errorf(
				"view %q declares step %d with no noun, so two rungs of one trail drawing "+
					"the same words could not be told apart", v.Path, i)
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
		case s.Projection == "" && len(s.Residual) > 0:
			return fmt.Errorf(
				"view %q's step %d carries a residual of %d endpoint(s) and draws the "+
					"document before it; a residual is what one document prints at a grain "+
					"the other does not, and a step that switches no document has no second "+
					"grain", v.Path, i, len(s.Residual))
		case s.Projection == "" && len(s.Gaps) > 0:
			return fmt.Errorf(
				"view %q's step %d declares a gap on %d node(s) and draws the document "+
					"before it; a gap is one cell two documents print at two figures, and a "+
					"step that switches no document has only one", v.Path, i, len(s.Gaps))
		}
		// Every parent draws the tier this step opens from, whatever the form.
		for _, p := range parents {
			if p.key != "" && !slices.Contains(p.tiers, s.From) {
				return fmt.Errorf(
					"view %q's step %d opens from tier %d, and step %q draws tiers "+
						"%v, which do not include it; the breadcrumb would carry a rung nothing "+
						"on the chart can reach", v.Path, i, s.From, p.key, p.tiers)
			}
		}
		// One arm per form; a second form adds its own here.
		if s.Form == SankeyForm {
			if err := v.validateSankeyStep(i, s, parents, doc); err != nil {
				return err
			}
		}
		// One step per (After, From, Role), per SHARED parent rather than per
		// equal After list, and a role-less step takes the whole tier.
		for j, o := range v.Steps[:i] {
			if o.From != s.From {
				continue
			}
			shared, found := "", false
			for _, a := range s.After {
				if slices.Contains(o.After, a) {
					shared, found = a, true
					break
				}
			}
			if !found {
				continue
			}
			where := "the view's own chart"
			if shared != "" {
				where = fmt.Sprintf("step %q's chart", shared)
			}
			if o.Role == s.Role {
				return fmt.Errorf(
					"view %q declares steps %d and %d both opening tier %d of %s "+
						"in role %q; a node there would open into two different charts",
					v.Path, j, i, s.From, where, s.Role)
			}
			if o.Role == "" || s.Role == "" {
				return fmt.Errorf(
					"view %q declares steps %d and %d both opening tier %d of %s, "+
						"in roles %q and %q; a step with no role opens EVERY node at its tier, "+
						"so it cannot share one with a step that names which nodes open",
					v.Path, j, i, s.From, where, o.Role, s.Role)
			}
		}
		// Residual and gap SHAPE -- a grain exactly where a residual is, no
		// empty id or reason, no licence of 0 cents -- is page.schema.json's,
		// held at the write; what follows is what a schema cannot say.
		for _, id := range slices.Sorted(maps.Keys(s.Gaps)) {
			seen := map[[2]string]bool{}
			for _, g := range s.Gaps[id] {
				if !years[ColumnPath(g.FiscalYear, g.Basis)] {
					return fmt.Errorf(
						"view %q's step %d licenses a gap on node %q for %s, a column the view "+
							"lists no year of; the licence would match no chart the reader can open",
						v.Path, i, id, fact.ColumnLabel(g.FiscalYear, g.Basis))
				}
				col := [2]string{strconv.Itoa(g.FiscalYear), g.Basis}
				if seen[col] {
					return fmt.Errorf(
						"view %q's step %d declares two gaps on node %q for %s; the client "+
							"holds a column's chart to one licence, and would keep one and drop "+
							"the other in silence", v.Path, i, id, fact.ColumnLabel(g.FiscalYear, g.Basis))
				}
				seen[col] = true
			}
		}
		// Every year the view lists carries the schedule the step draws, asked
		// of the column the reader will fetch. Whether any node of it opens is
		// the client's to answer, and site/*.test.mjs walks every rung.
		if s.Projection != "" {
			for _, stem := range v.YearStems {
				col, folded := ix.Column(stem)
				if !folded {
					return fmt.Errorf(
						"view %q lists year stem %q, whose document folded into no column, "+
							"so step %d has no column to open into", v.Path, stem, i)
				}
				if _, ok := ix.Stem(col, s.Projection); !ok {
					return fmt.Errorf(
						"view %q's step %d opens into schedule %q, and column %s (year stem "+
							"%q) carries no such schedule; a reader on that year would open "+
							"a node into nothing", v.Path, i, s.Projection, col, stem)
				}
			}
		}
		if _, ok := built[doc]; s.Projection != "" && !ok {
			return fmt.Errorf("view %q's step %d renders projection %q, which was not built",
				v.Path, i, s.Projection)
		}
	}
	return nil
}

// validateSankeyStep is the Sankey form's half of a step's validation: the
// window's shape, its placement against every chart it opens from, and its
// caps. The generic half is validateSteps'.
func (v View) validateSankeyStep(i int, s DrillStep, parents []parentChart, doc string) error {
	h := s.Sankey
	repeated := repeatedTier(h.Tiers)
	switch {
	case len(h.Tiers) == 0:
		return fmt.Errorf(
			"view %q declares step %d with no tiers, so a node opened on it would be "+
				"drawn by the same tier set it was closed under", v.Path, i)
	case len(s.Gaps) > 0 && len(h.Widen) > 0:
		return fmt.Errorf(
			"view %q's step %d declares a gap on %d node(s) and widens tiers %v; the "+
				"client stands the gap mark at the step's first or last declared tier, and "+
				"a viewport that does not buy that tier would draw the mark in a column "+
				"that is not there", v.Path, i, len(s.Gaps), h.Widen)
	case h.Side != "" && h.Side != SideSource:
		return fmt.Errorf(
			"view %q's step %d opens side %q; the sides are \"\", the node a link points "+
				"at, and %q, the node it comes from", v.Path, i, h.Side, SideSource)
	case len(h.Keep) > 0 && h.Side != "":
		return fmt.Errorf(
			"view %q's step %d keeps tier(s) %v and opens side %q; a window's opened node "+
				"is the TARGET of one half and the SOURCE of the other, so its side is "+
				"both and a step declaring one would be two declarations of one thing",
			v.Path, i, h.Keep, h.Side)
	case len(h.Widen) > 0 && len(h.Keep) == 0:
		return fmt.Errorf(
			"view %q's step %d widens by tier(s) %v and keeps no flank; a step that keeps "+
				"nothing draws the opened node's parts alone and has no centre to add a "+
				"column out from, so the widening names a construct this is not",
			v.Path, i, h.Widen)
	case repeated >= 0:
		return fmt.Errorf(
			"view %q's step %d draws tiers %v, which name tier %d twice; a tier is a "+
				"column, two columns of one tier is the same nodes drawn twice, and which "+
				"end a kept flank is at could not be read off the list either",
			v.Path, i, h.Tiers, repeated)
	case len(h.Keep) > 0 && len(h.Tiers) != len(h.Keep)+2+len(h.Widen):
		return fmt.Errorf(
			"view %q's step %d keeps tier(s) %v, widens by %d column(s) and draws tiers "+
				"%v; a window is its kept flank, the node that was opened and what it opens "+
				"into, one column each and one more for every widening -- %d columns here, "+
				"and not %d",
			v.Path, i, h.Keep, len(h.Widen), h.Tiers, len(h.Keep)+2+len(h.Widen), len(h.Tiers))
	}
	// Which end the flank is at is read off Tiers once; every arm after
	// this reads that answer.
	keptLeft := false
	if m := len(h.Keep); m > 0 {
		n := len(h.Tiers)
		switch {
		case slices.Equal(h.Tiers[:m], reversedTiers(h.Keep)):
			keptLeft = true
		case slices.Equal(h.Tiers[n-m:], h.Keep):
		default:
			return fmt.Errorf(
				"view %q's step %d keeps tier(s) %v and draws tiers %v, whose ends are not "+
					"that flank; the kept columns are the ones at ONE end of what the step "+
					"draws, outermost first, and a widening on the flank's side would push "+
					"them off it -- a flank two columns deep is declared by keeping two",
				v.Path, i, h.Keep, h.Tiers)
		}
		centre := m
		if !keptLeft {
			centre = n - 1 - m
		}
		if h.Tiers[centre] != s.From {
			return fmt.Errorf(
				"view %q's step %d keeps tier(s) %v and draws tiers %v, whose column %d is "+
					"tier %d and not the opened tier %d; the node the reader clicked is the "+
					"centre of a window", v.Path, i, h.Keep, h.Tiers, centre, h.Tiers[centre], s.From)
		}
		if w := len(h.Widen); w > 0 {
			drawn, want := h.Tiers[n-w:], h.Widen
			if !keptLeft {
				drawn, want = h.Tiers[:w], reversedTiers(h.Widen)
			}
			if !slices.Equal(drawn, want) {
				return fmt.Errorf(
					"view %q's step %d widens by tier(s) %v and draws tiers %v, whose %d "+
						"column(s) away from the kept flank are %v; a widened column is on the "+
						"opened node's side and the widening order is the order OUT from it, so "+
						"a client dropping the last of them draws a narrower window and not a "+
						"hole", v.Path, i, h.Widen, h.Tiers, w, drawn)
			}
		}
	}
	// Every parent places this step, not the first one that fits.
	for _, p := range parents {
		where := "the view's own chart"
		if p.key != "" {
			where = fmt.Sprintf("step %q", p.key)
		}
		// Same document only, and equality not containment (fisc-ke1f): a
		// strict subset of a widened parent's tiers is a narrower chart.
		if p.key != "" && doc == p.doc && slices.Equal(p.tiers, h.Tiers) {
			return fmt.Errorf(
				"view %q's step %d draws tiers %v of %q, the set step %q "+
					"already draws; opening a node would redraw the chart it was opened "+
					"from", v.Path, i, h.Tiers, doc, p.key)
		}
		// A kept flank is drawn at its share of the centre, not whole, so
		// nothing on one may open: the node would take one figure in and
		// send its whole decomposition out. Per parent, every flank column.
		if slices.Contains(p.keep, s.From) {
			return fmt.Errorf(
				"view %q's step %d opens tier %d of %s, which KEEPS that tier; a kept "+
					"flank is drawn at its share of that chart's centre rather than whole, "+
					"so this step would draw a node taking one figure in and sending its "+
					"whole decomposition out, with the difference left as node height "+
					"nothing accounts for", v.Path, i, s.From, where)
		}
		if len(h.Keep) == 0 {
			continue
		}
		// Every parent here has a column order and already contains From,
		// so fi >= 0. The flank walks out from the opened node.
		fi := slices.Index(p.tiers, s.From)
		step := -1
		if !keptLeft {
			step = 1
		}
		for n, k := range h.Keep {
			ki := slices.Index(p.tiers, k)
			drawnSide := "RIGHT"
			if ki < fi {
				drawnSide = "LEFT"
			}
			switch {
			case ki < 0:
				return fmt.Errorf(
					"view %q's step %d keeps tier %d and opens from %s, which draws tiers %v "+
						"and does not include it; the flank the reader came from has to be a "+
						"column they were looking at", v.Path, i, k, where, p.tiers)
			case n == 0 && ki != fi-1 && ki != fi+1:
				return fmt.Errorf(
					"view %q's step %d keeps tier %d and opens tier %d of %s, which draws "+
						"them as columns %d and %d of %v; a window slides by one column, and "+
						"which way it slides is the SIGN of that adjacency",
					v.Path, i, k, s.From, where, ki, fi, p.tiers)
			case n == 0 && ki != fi+step:
				return fmt.Errorf(
					"view %q's step %d keeps tier %d, which %s draws to the %s of the "+
						"opened tier %d, and draws tiers %v, which put it at the other end; "+
						"the kept flank stays on the side the reader saw it on",
					v.Path, i, k, where, drawnSide, s.From, h.Tiers)
			case ki != fi+step*(n+1):
				return fmt.Errorf(
					"view %q's step %d keeps tier(s) %v and opens tier %d of %s, which draws "+
						"tier %d as column %d of %v and the opened tier as column %d; a flank "+
						"is the columns BESIDE EACH OTHER walking out from the node that was "+
						"opened, so a gap in it is a column the reader was looking at dropped "+
						"out of the middle of the ones that stay",
					v.Path, i, h.Keep, s.From, where, k, ki, p.tiers, fi)
			}
		}
	}
	for j, c := range h.Caps {
		switch {
		case c.Cap < 1:
			return fmt.Errorf(
				"view %q's step %d caps tier %d at %d; the cap is what keeps a fine "+
					"column drawable and a column of one node is not a chart",
				v.Path, i, c.Tier, c.Cap)
		case !slices.Contains(h.Tiers, c.Tier):
			return fmt.Errorf(
				"view %q's step %d caps tier %d and draws tiers %v, which do not include "+
					"it; the cap would fold nothing, in silence", v.Path, i, c.Tier, h.Tiers)
		case slices.ContainsFunc(h.Caps[:j], func(o TierCap) bool { return o.Tier == c.Tier }):
			return fmt.Errorf("view %q's step %d caps tier %d twice", v.Path, i, c.Tier)
		}
	}
	return nil
}

// templateRendersLede answers whether a template has a {{.Lede}} to render.
// An allow-list: a template absent from it is refused a lede.
func templateRendersLede(name string) bool {
	switch name {
	case TrendsTemplate, HistoryTemplate, ProvenanceTemplate, CaveatsTemplate:
		return true
	default:
		return false
	}
}

// templateRendersAYearControl answers whether a template has a year control to
// render [View.YearStems] in. Its own switch rather than templateDrawsAChart:
// the two agree on every template today and guard different failures.
func templateRendersAYearControl(name string) bool {
	switch name {
	case SankeyTemplate:
		return true
	default:
		return false
	}
}

// templateDrawsAChart answers whether a template ships app.js and an SVG for it
// to draw into, and with them [View.Overview] and [View.Steps]. Not the same
// as rendering a document: trends.html renders one as a server-side table with
// no app.js.
func templateDrawsAChart(name string) bool {
	switch name {
	case SankeyTemplate:
		return true
	default:
		return false
	}
}

// templateRendersADocument answers whether a template renders a projection
// document, as opposed to being built from Options directly. An allow-list,
// because the negated form fails open for a new document-less template.
func templateRendersADocument(name string) bool {
	switch name {
	case SankeyTemplate, TrendsTemplate, HistoryTemplate:
		return true
	default:
		return false
	}
}

// templateIsKnown answers whether this package has a builder for a template.
// It keeps buildSite's dispatch refusal reachable: without it, validate's
// ignored-projection arm refuses an unknown template first, with advice that
// leads past validate. Drift against buildSite's switch is benign both ways.
func templateIsKnown(name string) bool {
	return templateRendersADocument(name) ||
		name == ProvenanceTemplate || name == CaveatsTemplate
}

// templateRendersSections answers whether a template groups its rows under
// [View.Sections] headings.
func templateRendersSections(name string) bool {
	return name == HistoryTemplate
}

// repeatedTier returns a tier the list names twice, or -1.
func repeatedTier(tiers []int) int {
	seen := make(map[int]bool, len(tiers))
	for _, t := range tiers {
		if seen[t] {
			return t
		}
		seen[t] = true
	}
	return -1
}

// reversedTiers is tiers back to front, in a copy: the callers hold the
// caller's own [DrillStep] fields.
func reversedTiers(tiers []int) []int {
	out := slices.Clone(tiers)
	slices.Reverse(out)
	return out
}

// endsASentence reports whether s closes with a sentence terminator.
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

// lastRune is the final character of s as a string.
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
// file, in the order it goes down. It exists so everything that can refuse an
// export does so before `fisc export --clean` empties the output directory.
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
	// One fold for the whole export: the write plan takes the columns and the
	// pages take the index.
	columns, ix, cerr := ColumnsOf(o.Projections, o.GeneratedBy)
	if cerr != nil {
		return nil, cerr
	}
	if err := o.validate(ix); err != nil {
		return nil, err
	}
	// An explicit remote wins, otherwise the site cites what it ships, and
	// only a caller that offers neither falls back to the constant.
	ship := o.SourceBrowseURL == "" && o.PageText != nil
	browse := o.SourceBrowseURL
	if browse == "" {
		browse = DefaultSourceBrowseURL
	}
	base := func(docID string) string { return remotePageTextBase(browse, docID) }
	if ship {
		base = LocalPageTextBase
	}

	pages, cited, err := buildSite(&o, ix, base)
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
	// A document that folded into a column is published as that column and
	// not also as itself. One stating no fiscal year or basis ships as itself.
	for _, name := range sortedKeys(o.Projections) {
		if _, folded := ix.Column(name); folded {
			continue
		}
		if err := add(ix.PublishedPath(name), o.Projections[name]); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(columns) {
		encoded, eerr := encodeColumn(columns[name])
		if eerr != nil {
			return nil, eerr
		}
		if err := add(name, encoded); err != nil {
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
// A cited page the tree does not hold is an error.
func withPageText(files map[string][]byte, tree fs.FS, cited []Citation, reserved map[string]bool) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files)+len(cited))
	maps.Copy(out, files)
	for _, c := range cited {
		src := path.Join(c.DocID, corpus.PagePath(c.Page))
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

// RecordsFile is the published records shard for one page, named by the page
// as its text is.
func RecordsFile(page int) string { return corpus.PageStem(page) + ".jsonl" }

// assetTree is the site source tree the templates and verbatim assets come
// from: the caller's, or the embedded one.
func (o *Options) assetTree() fs.FS {
	if o.Assets != nil {
		return o.Assets
	}
	return site.FS()
}
