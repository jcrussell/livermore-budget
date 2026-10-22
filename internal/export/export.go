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
//	<dir>/<view>.html         each view, rendered from its own site/ template;
//	                          the view the site opens on is at index.html
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
	"context"
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

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/project"

	"github.com/jcrussell/livermore-budget/site"
)

// PrimaryProjection is the spine document. It must be among the projections
// built (ErrNoPrimary). It is NOT required to be the view at IndexPath: which
// document a site opens on is the caller's Views.
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
const DefaultSourceBrowseURL = RepositoryURL + "/blob/main"

// RepositoryURL is this project's repository, which every page's footer links.
const RepositoryURL = "https://github.com/jcrussell/livermore-budget"

// verbatimAssets are copied from the embedded site tree byte for byte. The
// page template is not in this list: it is rendered, and vendor/ is walked.
var verbatimAssets = []string{"app.js", "core.js", "sankey.js",
	"department.js", "capital.js", "debt.js", "glance.js", "sides.js", "ledger.js",
	"change.js", "format.js", "theme.js", "style.css", ".nojekyll"}

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
	// The spine's template renders no Lede: its lede wraps a live
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
	// THE SANKEY'S TIERS ARE A COLUMN ORDER AND NOT ONLY A SET: site/sankey.js
	// places a node by its slot, and a root of the expansion schedule by its
	// tier's place in this list. The fold is the client's.
	Overview Chart

	// Draws is the schedule the view's own chart draws out of each year's
	// column, or "" for Projection's own. The page's figures, year control
	// and sources stay Projection's; the chart, its counts line and its
	// caveats are this schedule's, shipped as FISC_CONFIG.primary.
	Draws string
	// Description is the view's own chart said in words: the SVG's <desc>
	// and the paragraph under the chart, or "" for the template's own, which
	// describes the spine. Only a template that draws a chart renders it.
	Description string

	// Reads are schedules other than Projection's that the page reads out of
	// each year's column, by projection name. Each year's column must carry
	// every one, and a reader reaches each as it reaches the years. Only a
	// template that reads a second schedule (templateReadsBeside) takes any.
	Reads []string
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
	// (ResolveOutputDir) before getting here.
	Dir string

	// Projections maps a filename stem to a projection's JSON document, which
	// is written verbatim to data/<stem>.json. Must contain PrimaryProjection.
	Projections map[string][]byte

	// Views are the site's pages, in nav order, the view the site opens on
	// first. Empty is refused.
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

	// GeneratedBy names the tool and version that wrote the output. The
	// sources page states it, and every client config carries it.
	GeneratedBy string

	// Assets is the site source tree. Empty means the embedded one; a test
	// can substitute an fstest.MapFS.
	Assets fs.FS

	// Departments is the department page's membership, stated by the caller
	// from the department registry: each department and every division the
	// Budget Book prints under it. Only [DepartmentTemplate] reads it.
	Departments []Department

	// DepartmentsHeldApart are the cells, one department in one column, where
	// the caller's checks declare pp.85-125's two blocks do not tie, each with
	// the declared reason. Every other cell of a department the page shows is
	// one `fisc verify` holds to tie to the cent.
	DepartmentsHeldApart []HeldApart

	// DepartmentsApartFromSpine are the columns where the caller's checks
	// declare pp.85-125's department rows apart from pp.66-67's citywide
	// figure, each with the declared residual and reason. Only
	// [GlanceTemplate] reads it.
	DepartmentsApartFromSpine []ApartFromSpine

	// Gate is the caller's own refusal of a site this package would accept,
	// handed the ColumnIndex validation folded. Prepare runs it once the fold
	// and Options.validate have accepted, and before buildSite: a drawn
	// schedule buildSite refuses, a page text it cannot resolve and every page
	// render come after it. Nil is none.
	Gate func(ix ColumnIndex) error
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

// Chart is one drawn chart's form and that form's hints under the form's own
// key, as [View.Overview] declares it. A hint foreign to the declared form is
// refused by schema/page.schema.json at the write, and the client core reads
// none of them. A second form adds one
// value to [ChartForms], one hint type here, one closed object in the schema
// and one renderer module under site/.
type Chart struct {
	// Form is which renderer draws the chart, one of [ChartForms].
	Form string `json:"form"`
	// Sankey is the Sankey form's hints, present exactly when Form is
	// [SankeyForm].
	Sankey *SankeyHints `json:"sankey,omitempty"`
	// Ledger is the sides form's hints, present exactly when Form is
	// [LedgerForm].
	Ledger *SidesHints `json:"ledger,omitempty"`
}

// SankeyForm is the form site/sankey.js draws: columns of tiers joined by
// ribbons.
const SankeyForm = "sankey"

// LedgerForm is the form site/ledger.js draws: each declared side of the
// expansion schedule as ranked bars on one scale, opening in place.
const LedgerForm = "ledger"

// ChartForms is the declared set of forms, which schema/enums.schema.json
// states as chart_form and schema/enums_test.go holds to this.
func ChartForms() []string { return []string{SankeyForm, LedgerForm} }

// SidesHints is a ledger's hints: the sides it draws.
type SidesHints struct {
	Sides []Side `json:"sides" yaml:"sides"`
}

// Side is one side of a sides chart: a spine tier whose roots open in place,
// and the heading it stands under.
type Side struct {
	Tier    int    `json:"tier" yaml:"tier"`
	Heading string `json:"heading" yaml:"heading"`
}

// sides is the sides hints of a chart of a sides form, or nil.
func (c Chart) sides() *SidesHints {
	if c.Form == LedgerForm {
		return c.Ledger
	}
	return nil
}

// foreignHints names a hint object the chart carries that is not its form's,
// or "" when it carries none.
func (c Chart) foreignHints() string {
	switch {
	case c.Sankey != nil && c.Form != SankeyForm:
		return SankeyForm
	case c.Ledger != nil && c.Form != LedgerForm:
		return LedgerForm
	}
	return ""
}

// MarkRoles is the declared set of roles a node the CLIENT makes may wear:
// the aggregate a split cap folds a split's tail into, and the smaller-flows
// mark the reader's smallest-flow filter folds small ribbons onto. No
// document carries one: [ColumnsOf] refuses a producer node wearing a mark's
// role or id, schema/enums.schema.json states the set as mark_role, and
// schema/mark.schema.json is the shape, held on the client by
// site/marks.test.mjs.
func MarkRoles() []string { return []string{"aggregate", "smaller"} }

// MarkPrefixes is the id prefix each mark role's ids carry, in [MarkRoles]
// order; schema/mark.schema.json's id pattern states the same set.
func MarkPrefixes() []string { return []string{"aggregate/tail/", "smaller/"} }

// markPrefixOf is the mark prefix an id carries, or "".
func markPrefixOf(id string) string {
	for _, p := range MarkPrefixes() {
		if strings.HasPrefix(id, p) {
			return p
		}
	}
	return ""
}

// SankeyHints is what the Sankey form may fold and where. Go declares; the
// client's capColumn and foldDocument spend the declaration.
type SankeyHints struct {
	// Tiers is the page's own column order, or empty to draw the document
	// whole.
	Tiers []int `json:"tiers,omitempty" yaml:"tiers"`
	// Grow lays the chart out on a canvas sized by the busiest column the
	// client draws, or nil for the form's own canvas.
	Grow *CanvasGrowth `json:"grow,omitempty" yaml:"grow"`
	// SplitCaps bounds how many members of one split of the expansion
	// schedule the chart draws before the rest fold into a tail inside the
	// node they split; a split with no cap is drawn whole. Keyed by the
	// split's key, so one cap holds every node's split of that key.
	SplitCaps []SplitCap `json:"split_caps,omitempty" yaml:"split_caps"`
}

// CanvasGrowth is [SankeyHints.Grow]: NodePx px for each node of the busiest
// column, held between Floor and Ceiling.
type CanvasGrowth struct {
	NodePx  int `json:"node_px" yaml:"node_px"`
	Floor   int `json:"floor" yaml:"floor"`
	Ceiling int `json:"ceiling" yaml:"ceiling"`
}

// SplitCap is one [SankeyHints.SplitCaps] entry: the split key, how many of
// its members are drawn, and the plural noun its tail is counted in.
type SplitCap struct {
	Split string `json:"split" yaml:"split"`
	Cap   int    `json:"cap" yaml:"cap"`
	Tail  string `json:"tail" yaml:"tail"`
}

// drawnTiers is the column order a chart of this form draws, or nil for a
// Sankey drawing its document whole.
func (c Chart) drawnTiers() []int {
	if c.Form == SankeyForm && c.Sankey != nil {
		return c.Sankey.Tiers
	}
	if h := c.sides(); h != nil {
		out := make([]int, 0, len(h.Sides))
		for _, s := range h.Sides {
			out = append(out, s.Tier)
		}
		return out
	}
	return nil
}

// Download is one whole-store artifact a page offers.
type Download struct {
	Path  string
	Label string
	Note  string
	Bytes int
}

// YearKey names one column the same way on every page: the value of the
// top bar's year select and of a page's #y=, so a year chosen on one page is
// the year another opens on.
func YearKey(year int, basis string) string {
	return fmt.Sprintf("fy%d-%s", year, basis)
}

// ColumnPath is the file one published column ships at, at the site root.
func ColumnPath(year int, basis string) string {
	return YearKey(year, basis) + ".json"
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
	if len(o.Views) == 0 {
		return errors.New("no views to export; the caller states every page the site has")
	}
	for i := range o.Views {
		if err := o.Views[i].validate(o.Projections, ix); err != nil {
			return err
		}
	}
	seenPath := map[string]bool{}
	index := 0
	for _, v := range o.Views {
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
	if len(o.Views) > 1 {
		for _, v := range o.Views {
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
	m := map[string]bool{MarkerName: true}
	for _, name := range verbatimAssets {
		m[name] = true
	}
	return m
}()

// reservedPaths is fixedPaths plus this Options' own views.
func (o *Options) reservedPaths() map[string]bool {
	m := make(map[string]bool, len(fixedPaths)+len(o.Views)+1)
	maps.Copy(m, fixedPaths)
	for _, v := range o.Views {
		m[v.Path] = true
	}
	return m
}

// validate refuses a view that could not be rendered, or that would land on a
// path the site owns.
func (v View) validate(built map[string][]byte, ix ColumnIndex) error {
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
	case len(v.Reads) > 0 && !templateReadsBeside(v.Template):
		return fmt.Errorf(
			"view %q reads %v beside its own document and renders template %q, which "+
				"reads no second schedule; the schedules would be dropped in silence",
			v.Path, v.Reads, v.Template)
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
	case v.Overview.Form != "" && formOfTemplate(v.Template) != v.Overview.Form:
		return fmt.Errorf(
			"view %q declares a %q chart and renders template %q, whose module draws a %q; "+
				"the page would refuse itself at boot", v.Path, v.Overview.Form, v.Template, formOfTemplate(v.Template))
	case v.Overview.foreignHints() != "":
		return fmt.Errorf(
			"view %q carries %s hints and declares a %q chart; the hints "+
				"would be dropped in silence", v.Path, v.Overview.foreignHints(), v.Overview.Form)
	case v.Overview.Form == LedgerForm && v.Overview.sides() == nil:
		return fmt.Errorf(
			"view %q declares a %s chart with no %s hints; which tiers are its sides is a declaration",
			v.Path, v.Overview.Form, v.Overview.Form)
	case v.Overview.Form == SankeyForm && v.Overview.Sankey == nil:
		return fmt.Errorf(
			"view %q declares a sankey chart with no sankey hints; the column order "+
				"is a declaration, and an absent one is not the same as an empty one",
			v.Path)
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
	if err := v.projectionAmongYears(); err != nil {
		return err
	}
	if err := v.validateOverview(built, ix); err != nil {
		return err
	}
	for _, year := range v.yearStems() {
		if _, err := v.readStems(year, ix); err != nil {
			return err
		}
	}
	drawn, err := v.drawnStems(ix)
	if err != nil {
		return err
	}
	return v.validateOverviewTiersCarried(drawn, ix)
}

// templateRendersLede answers whether a template has a {{.Lede}} to render.
// An allow-list: a template absent from it is refused a lede.
func templateRendersLede(name string) bool {
	switch name {
	case HistoryTemplate, ProvenanceTemplate, CaveatsTemplate,
		DepartmentTemplate, CapitalTemplate, DebtTemplate, GlanceTemplate,
		LedgerTemplate, ChangeTemplate:
		return true
	default:
		return false
	}
}

// templateRendersAYearControl answers whether a template renders
// [View.YearStems]: as the top bar's year select, or on GlanceTemplate as one
// section per year. Its own switch rather than templateDrawsAChart: the two
// guard different failures.
func templateRendersAYearControl(name string) bool {
	switch name {
	case SankeyTemplate, LedgerTemplate,
		DepartmentTemplate, GlanceTemplate:
		return true
	default:
		return false
	}
}

// templateDrawsAChart answers whether a template ships a client module and a
// figure for it to draw into, and with it [View.Overview]. Not the same
// as rendering a document: balances.html renders one as a server-side table
// with no module.
func templateDrawsAChart(name string) bool {
	return formOfTemplate(name) != ""
}

// formOfTemplate is the one chart form a template's module draws, or "" for
// a template that draws none. Each module refuses a page declaring another
// form at boot, after the write; this is the write's own refusal of it.
func formOfTemplate(name string) string {
	switch name {
	case SankeyTemplate:
		return SankeyForm
	case LedgerTemplate:
		return LedgerForm
	default:
		return ""
	}
}

// templateRendersADocument answers whether a template renders a projection
// document, as opposed to being built from Options directly. An allow-list,
// because the negated form fails open for a new document-less template.
func templateRendersADocument(name string) bool {
	switch name {
	case SankeyTemplate, HistoryTemplate,
		CapitalTemplate,
		DebtTemplate,
		DepartmentTemplate, GlanceTemplate, LedgerTemplate, ChangeTemplate:
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
		name == ProvenanceTemplate || name == CaveatsTemplate ||
		name == DepartmentTemplate || name == CapitalTemplate || name == DebtTemplate
}

// templateReadsBeside answers whether a template reads [View.Reads], a
// schedule beside its own out of each year's column.
func templateReadsBeside(name string) bool {
	return name == DepartmentTemplate || name == GlanceTemplate
}

// templateRendersSections answers whether a template groups its rows under
// [View.Sections] headings.
func templateRendersSections(name string) bool {
	return name == HistoryTemplate
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
	if o.Gate != nil {
		if err := o.Gate(ix); err != nil {
			return nil, err
		}
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
	// The pages are written last, and index.html last of all, so in a fresh
	// or cleaned directory the entry point does not exist until the site
	// around it does -- including the other views it links to, or it would
	// appear while its own nav pointed at 404s. Over an earlier export
	// written without --clean the old index.html is already there, and a
	// write that stops partway leaves it beside a mix of old and new files
	// (fisc-3s5d.69).
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
// slash-separated paths it wrote, relative to that directory and sorted, the
// manifest included. It checks ctx before creating o.Dir and before each file,
// and stops with an error wrapping ctx.Err(), leaving the files already
// written in place.
//
// The manifest goes down first and last. First it lists every path the
// previous manifest did and every path this plan writes, so a run stopped at
// any point leaves a manifest covering everything an export may have put
// there; last it lists this plan's paths and whatever the previous export
// wrote that is still there, which is what a later --clean may remove. A
// previous manifest that is missing or does not parse lists nothing.
//
// Everything that can be decided has been. What is left can fail only on I/O
// or stop on a cancelled ctx.
func (p *plan) Write(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	// #nosec G301 -- the output is a web root; a directory a server running as
	// another user cannot traverse is a broken deploy, not a hardened one.
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	prev, err := previousManifest(p.dir)
	if err != nil {
		return nil, err
	}
	planned := make([]string, len(p.files))
	for i, f := range p.files {
		planned[i] = f.Path
	}
	slices.Sort(planned)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("write %q: %w", MarkerName, err)
	}
	if err := writeManifest(p.dir, union(prev, planned)); err != nil {
		return nil, err
	}
	written := make([]string, 0, len(p.files)+1)
	written = append(written, MarkerName)
	for _, f := range p.files {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("write %q: %w", f.Path, err)
		}
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
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("write %q: %w", MarkerName, err)
	}
	if err := writeManifest(p.dir, union(planned, leftover(p.dir, prev, planned))); err != nil {
		return nil, err
	}
	sort.Strings(written)
	return written, nil
}

// previousManifest is the listing dir's manifest holds, or nothing where it
// holds none a reader can trust; only I/O fails it.
func previousManifest(dir string) ([]string, error) {
	paths, err := readManifest(dir)
	var invalid *invalidManifestError
	if errors.Is(err, errNoManifest) || errors.As(err, &invalid) {
		return nil, nil
	}
	return paths, err
}

// leftover is what prev lists and planned does not that is still a file in
// dir, reached through directories rather than symlinks: what an earlier
// export wrote and this one leaves behind. Both listings are sorted.
func leftover(dir string, prev, planned []string) []string {
	var out []string
	for _, rel := range prev {
		if _, ok := slices.BinarySearch(planned, rel); ok {
			continue
		}
		if fileWithin(dir, rel) {
			out = append(out, rel)
		}
	}
	return out
}

// fileWithin reports whether rel names something in dir other than a
// directory, with every directory between them a real one: a symlinked
// directory on the way would put the file outside dir.
func fileWithin(dir, rel string) bool {
	cur := dir
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return false
		}
		// Lstat reports a symlink as no directory, so a symlinked directory
		// on the way fails here.
		if i == len(parts)-1 {
			return !info.IsDir()
		}
		if !info.IsDir() {
			return false
		}
	}
	return false
}

// union is the sorted, duplicate-free merge of two sorted listings.
func union(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	slices.Sort(out)
	return slices.Compact(out)
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
