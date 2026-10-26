// Package export packages a set of projection documents into a static site.
//
// It consumes projections as filename stem -> JSON bytes and recomputes
// nothing they publish: every figure the page shows is read back out of the
// projection JSON.
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
// Every asset path in the page is relative, so the output serves from any
// root, and the cited pages' extracted text ships beside it.
package export

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/project"

	"github.com/jcrussell/livermore-budget/site"
)

// PrimaryProjection is the spine document. It must be among the projections
// built (ErrNoPrimary). It is NOT required to be the view at IndexPath: which
// document a site opens on is the caller's Views.
const PrimaryProjection = project.PublishedProjection

// IndexPath is the view the site opens on, the fixed entry point of the layout.
const IndexPath = "index.html"

// dataDir is the output subdirectory holding projection JSON, part of the
// published contract (docs/outputs/sankey.md).
const dataDir = "data"

// PageTextDir is the output subdirectory holding the committed extraction of
// every cited page, mirroring data/extracted/ as <doc-id>/pages/pNNNN.txt.
// Only pages the SITE cites are copied: the union of every projection's
// metadata.sources and every [Options.PageIndex] entry, so every published
// record has its page text beside it.
const PageTextDir = "extracted"

// DefaultSourceBrowseURL is where committed page text is cited when the caller
// supplies neither [Options.PageText] nor [Options.SourceBrowseURL]: the
// github.com blob view, which renders the .txt grid verbatim (see
// corpus.PagePath).
const DefaultSourceBrowseURL = RepositoryURL + "/blob/main"

// RepositoryURL is this project's repository, which every page's footer links.
const RepositoryURL = "https://github.com/jcrussell/livermore-budget"

// verbatimAssets are copied from the embedded site tree byte for byte. The
// page template is not in this list: it is rendered, and vendor/ is walked.
var verbatimAssets = []string{"app.js", "core.js", "sankey.js",
	"department.js", "capital.js", "glance.js", "sides.js", "ledger.js",
	"change.js", "format.js", "theme.js", "style.css", ".nojekyll"}

// View is one HTML page of the site: one document, rendered by one template.
// Each view is its own server-rendered page at the root, citing only its own
// sources.
type View struct {
	// Path is the output path, always .html and always at the root. Exactly one
	// view must be at [IndexPath].
	Path string
	// Nav is the words this view is listed under in every page's nav.
	Nav string
	// Template is the name of the template in the asset tree.
	Template string
	// Projection is the filename stem of the document this view renders. It
	// must be a key of [Options.Projections].
	Projection string
	// Title is the <title>, and on a template that renders one, the page
	// heading. Lede is the sentence under it. A Lede on a template that renders
	// none (the spine's, whose lede app.js rewrites) is refused.
	Title string
	Lede  string
	// YearStems are the same projection for different fiscal years, OLDEST
	// FIRST; the page opens on the last. Empty renders no year control.
	YearStems []string

	// Sections are the printed blocks a history table groups its rows under,
	// in printed order, in the caller's words. buildHistoryPage refuses a
	// series no section claims and a section that claims no series.
	Sections []Section

	// Overview is the view's own chart, its form and that form's hints,
	// shipped as FISC_CONFIG.overview. Required on a template that draws a
	// chart and refused on one that does not. A Sankey's tiers are a column
	// order, not only a set; with none it draws the document whole.
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
	// each year's column, which must carry every one. Only a template with
	// templateSpec.reads takes any.
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
	// first, stated by the caller. Empty is refused.
	Views []View

	// Docs are the source documents the page cites, keyed by doc id in the
	// projection's metadata.sources.
	Docs []Doc

	// SourceBrowseURL is the base URL of a remote holding
	// data/extracted/<doc-id>/pages/pNNNN.txt. Setting it cites there INSTEAD
	// of shipping the text, so PageText is not read. Empty means ship PageText
	// or, failing that, cite DefaultSourceBrowseURL.
	SourceBrowseURL string

	// PageText is the committed extraction tree, rooted at data/extracted/.
	// Only cited pages are read, and a cited page missing from it is an error.
	// Nil means the page text is not shipped and citations go to a URL.
	PageText fs.FS

	// Files is the non-projection asset channel: slash-separated path relative
	// to Dir -> bytes, written verbatim. The cited page text goes through it
	// too. assetPath screens each path, so an asset cannot shadow a view, an
	// embedded asset or a projection.
	Files map[string][]byte

	// PageIndex is what the provenance view publishes: one entry per locator
	// the site can resolve, in the order the page lists them, stated by the
	// caller. Every entry's page is cited, so its text ships (PageTextDir).
	PageIndex []PageIndexEntry

	// Downloads are the whole-store artifacts a page offers, described so the
	// reader knows the size before starting the fetch.
	Downloads []Download

	// RecordsBase is where each document's records live, keyed by doc id, in
	// the form a client appends [RecordsFile] to. The caller builds it by the
	// same rule as PageIndexEntry.Data, and validate holds the two to agree.
	// It is always site-relative, even when page text is cited remotely.
	// A doc with no entry publishes records: "" and the client renders no
	// records link.
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

// Citation is one (document, page) pair the site cites. [PageIndexEntry]
// embeds it, so every published locator is a citation and ships its page text.
type Citation struct {
	DocID string
	Page  int
}

// PageIndexEntry is one locator the provenance view publishes.
type PageIndexEntry struct {
	Citation
	// Records is how many records the shard holds.
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
	// Rows, when non-nil, closes the section: every printed row label it
	// claims, mapped to the label its Line cell displays ("" for the printed
	// label unchanged; a wrapped row's display label names it whole). Both
	// directions refuse: a series Rows does not name, and a name matching no
	// series. Nil leaves the section open under printed labels.
	Rows map[string]string
}

// Chart is one drawn chart's form and that form's hints under the form's own
// key, as [View.Overview] declares it. A hint foreign to the declared form is
// refused by schema/page.schema.json at the write.
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
// a split cap's tail aggregate and the smallest-flow filter's mark. [ColumnsOf]
// refuses a producer node wearing one; schema/enums.schema.json states the set
// as mark_role and schema/mark.schema.json the shape.
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
// year select under the top bar and of a page's #y=, so a year chosen on one page is
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
	spec, known := templates[v.Template]
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
	case v.Projection == "" && spec.document:
		return fmt.Errorf("view %q names no projection", v.Path)
	case v.Projection != "" && known && !spec.document:
		return fmt.Errorf(
			"view %q names projection %q and renders template %q, which renders no "+
				"document; the projection would be ignored", v.Path, v.Projection, v.Template)
	case v.Lede != "" && !spec.lede:
		return fmt.Errorf(
			"view %q sets a lede and renders template %q, which has no {{.Lede}}; "+
				"the sentence would be dropped in silence", v.Path, v.Template)
	case len(v.YearStems) > 0 && !spec.years:
		return fmt.Errorf(
			"view %q lists %d year stems and renders template %q, which has no year "+
				"control; the years would be dropped in silence",
			v.Path, len(v.YearStems), v.Template)
	case len(v.Reads) > 0 && !spec.reads:
		return fmt.Errorf(
			"view %q reads %v beside its own document and renders template %q, which "+
				"reads no second schedule; the schedules would be dropped in silence",
			v.Path, v.Reads, v.Template)
	case len(v.Sections) > 0 && !spec.sections:
		return fmt.Errorf(
			"view %q declares %d sections and renders template %q, which groups nothing; "+
				"the headings would be dropped in silence",
			v.Path, len(v.Sections), v.Template)
	case len(v.Sections) == 0 && spec.sections:
		return fmt.Errorf(
			"view %q renders template %q and declares no sections, so every row would "+
				"land under no printed heading", v.Path, v.Template)
	case v.Overview.Form != "" && spec.form == "":
		return fmt.Errorf(
			"view %q declares a %q chart and renders template %q, which draws none; "+
				"the form would be dropped in silence", v.Path, v.Overview.Form, v.Template)
	case v.Overview.Form == "" && spec.form != "":
		return fmt.Errorf(
			"view %q renders template %q, which draws a chart, and declares no form for "+
				"it; the client would have no renderer to hand the document to",
			v.Path, v.Template)
	case v.Overview.Form != "" && !slices.Contains(ChartForms(), v.Overview.Form):
		return fmt.Errorf(
			"view %q declares chart form %q, which is not one of %v",
			v.Path, v.Overview.Form, ChartForms())
	case v.Overview.Form != "" && spec.form != v.Overview.Form:
		return fmt.Errorf(
			"view %q declares a %q chart and renders template %q, whose module draws a %q; "+
				"the page would refuse itself at boot", v.Path, v.Overview.Form, v.Template, spec.form)
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

// templateSpec is what this package knows of one page template: the builder
// of its page data and what of a [View] it renders. A field left false is a
// refusal: View.validate refuses a view setting what its template drops.
type templateSpec struct {
	build func(pageCtx) (any, error)
	// document is whether the template renders a projection document, as
	// opposed to being built from Options directly.
	document bool
	// lede is whether the template has a {{.Lede}} to render.
	lede bool
	// years is whether the template renders [View.YearStems], as the year
	// select under the top bar, which on GlanceTemplate shows one of its
	// sections per year.
	years bool
	// reads is whether the template reads [View.Reads], a schedule beside
	// its own out of each year's column.
	reads bool
	// sections is whether the template groups its rows under
	// [View.Sections] headings.
	sections bool
	// form is the one chart form the template's module draws, or "" for a
	// template that ships no module and with it no [View.Overview].
	form string
}

// templates is every page template this package has a builder for. Filled
// in init, because builders reach templateRendersADocument, which reads it.
var templates map[string]templateSpec

func init() {
	templates = map[string]templateSpec{
		SankeyTemplate:     {build: page(buildSankeyPage), document: true, years: true, form: SankeyForm},
		LedgerTemplate:     {build: page(buildSidesPage), document: true, lede: true, years: true, form: LedgerForm},
		HistoryTemplate:    {build: page(buildHistoryPage), document: true, lede: true, sections: true},
		ProvenanceTemplate: {build: page(buildProvenancePage), lede: true},
		CaveatsTemplate:    {build: page(buildCaveatsPage), lede: true},
		DepartmentTemplate: {build: page(buildDepartmentPage), document: true, lede: true, years: true, reads: true},
		CapitalTemplate:    {build: page(buildCapitalPage), document: true, lede: true},
		DebtTemplate:       {build: page(buildDebtPage), document: true, lede: true},
		GlanceTemplate:     {build: page(buildGlancePage), document: true, lede: true, years: true, reads: true},
		ChangeTemplate:     {build: page(buildChangePage), document: true, lede: true},
	}
}

// page is a builder as templates holds it.
func page[T any](build func(pageCtx) (T, error)) func(pageCtx) (any, error) {
	return func(p pageCtx) (any, error) { return build(p) }
}

// templateRendersADocument answers whether a template renders a projection
// document; an unknown template renders none.
func templateRendersADocument(name string) bool { return templates[name].document }

// assetPath screens one [Options.Files] key: a clean relative path outside the
// fixed site layout.
func assetPath(rel string, reserved map[string]bool) error {
	switch {
	case reserved[rel], strings.HasPrefix(rel, dataDir+"/"), strings.HasPrefix(rel, "vendor/"):
		return fmt.Errorf("asset path %q is part of the fixed site layout", rel)
	case !manifestEntry(rel):
		return fmt.Errorf("asset path %q is not a clean relative path on one line", rel)
	}
	return nil
}

// plan is a site resolved in full and not yet written: every byte of every
// file, in the order it goes down. It exists so everything that can refuse an
// export does so before `fisc export` writes or removes anything.
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
		if !manifestEntry(rel) {
			return fmt.Errorf("output path %q is not a clean relative path on one line", rel)
		}
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
	for _, name := range slices.Sorted(maps.Keys(o.Projections)) {
		if _, folded := ix.Column(name); folded {
			continue
		}
		if err := add(ix.PublishedPath(name), o.Projections[name]); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(columns)) {
		encoded, eerr := encodeColumn(columns[name])
		if eerr != nil {
			return nil, eerr
		}
		if err := add(name, encoded); err != nil {
			return nil, err
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		if err := add(rel, files[rel]); err != nil {
			return nil, err
		}
	}
	// Pages go last, and index.html last of all, so the entry point does not
	// exist until the site it links to does.
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
	// A path planned as a file and as a directory above another, or at or
	// under the manifest, can only fail in Write, so it is refused here.
	for _, f := range plan.files {
		for d := f.Path; d != "."; d = path.Dir(d) {
			if d == MarkerName {
				return nil, fmt.Errorf("the output path %q is at or under %s, the export's manifest", f.Path, MarkerName)
			}
			if d != f.Path && seen[d] {
				return nil, fmt.Errorf("the output path %q is planned as a file and as a directory holding %q", d, f.Path)
			}
		}
	}
	return plan, nil
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

// withPageText returns a copy of files plus the committed text of every cited
// page, screened as any [Options.Files] entry. A cited page the tree does not
// hold is an error.
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
		// Before Prepare's duplicate guard can see it, so refuse here.
		if _, taken := out[rel]; taken {
			return nil, fmt.Errorf("two assets claim the output path %q", rel)
		}
		out[rel] = b
	}
	return out, nil
}

// LocalPageTextBase is the output directory holding one document's cited page
// text, site-relative, with the trailing slash the client appends a filename to.
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
