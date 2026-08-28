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

// DataDir is the output subdirectory holding projection JSON. It is part of
// the published contract — docs/sankey-contract.md promises
// <output>/data/<projection>.json — so it is a constant, not a flag.
const DataDir = "data"

// PageTextDir is the output subdirectory holding the committed extraction of
// every page the projection cites. The tree under it mirrors the repository's
// data/extracted/ — <doc-id>/pages/pNNNN.txt — so the same base-plus-filename
// rule composes a citation whether the text is served from here or from a
// forge's blob view, and a reader who knows one layout knows the other.
//
// Only CITED pages are copied. The corpus is 786 pages; the projection cites
// two of them today and around two dozen once the coverage lanes land, and a
// packager that shipped the whole extraction would put the corpus in every
// deploy to publish a handful of citations.
const PageTextDir = "extracted"

// MarkerName is the sentinel written into a generated site. It is what tells
// a later --clean run that the directory is fisc's to delete. The name is
// cmdutil's, not this package's: the writer and the deleter agreeing on it by
// coincidence is how a --clean starts refusing to clean.
const MarkerName = cmdutil.ExportMarkerName

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

	// GeneratedBy names the tool and version that wrote the output. It is
	// shown in the page footer beside the projection's own generated_by.
	GeneratedBy string

	// Assets is the site source tree. Empty means the embedded one; a test
	// can substitute an fstest.MapFS.
	Assets fs.FS
}

// ErrNoPrimary reports a projection set with no PrimaryProjection in it.
var ErrNoPrimary = errors.New("no " + PrimaryProjection + " projection to build the page from")

// Validate reports whether this Options could be written, without writing it.
//
// IT EXISTS SO A CALLER CAN REFUSE BEFORE IT DESTROYS SOMETHING. Write validates
// too, and that is not enough for `fisc export --clean`, which empties the
// output directory first: a caller that learns its input is bad from Write has
// already deleted the reader's site to find out. Nothing validate inspects is
// produced by cleaning, so the check can always be hoisted, and the same
// argument is written out at length beside assertPublishedReachable in
// pkg/cmd/export.
//
// Write still validates. This is a second opportunity to refuse, not a
// precondition callers are trusted to have met.
func (o Options) Validate() error { return (&o).validate() }

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
	// after that, so the nav ships <a href="revenue.html"></a> -- a link a
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
	return nil
}

// fixedPaths are the output paths Write owns whatever the caller asked for. An
// asset landing on one of them would not be an extra file but a replaced one:
// the site would still export, and the page would be broken in the browser only.
//
// IT NO LONGER CONTAINS index.html, and that is the point of the split. The
// pages are now a property of Options -- one per View -- so which paths are
// reserved is too, and a package-level set could only ever know about the one
// page that used to exist. An asset at revenue.html would have shadowed a view
// silently.
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
	case fixedPaths[v.Path], strings.HasPrefix(v.Path, DataDir+"/"):
		return fmt.Errorf("view path %q is part of the fixed site layout", v.Path)
	case !strings.HasSuffix(v.Path, ".html"):
		return fmt.Errorf("view path %q is not an .html file", v.Path)
	case path.Base(v.Path) != v.Path:
		// Flat, per the View doc comment: every asset path in the output is
		// relative, so a page in a subdirectory would need ../ on all of them.
		return fmt.Errorf("view path %q is not at the site root", v.Path)
	case v.Projection == "":
		return fmt.Errorf("view %q names no projection", v.Path)
	case v.Template == "":
		return fmt.Errorf("view %q names no template", v.Path)
	case v.Lede != "" && !templateRendersLede(v.Template):
		return fmt.Errorf(
			"view %q sets a lede and renders template %q, which has no {{.Lede}}; "+
				"the sentence would be dropped in silence", v.Path, v.Template)
	case len(v.YearStems) > 0 && !templateRendersAYearControl(v.Template):
		return fmt.Errorf(
			"view %q lists %d year stems and renders template %q, which has no year "+
				"control; the years would be dropped in silence",
			v.Path, len(v.YearStems), v.Template)
	case len(v.RenderTiers) > 0 && !templateRendersTiers(v.Template):
		return fmt.Errorf(
			"view %q asks for render tiers %v and renders template %q, which publishes "+
				"none; the chart would draw every tier", v.Path, v.RenderTiers, v.Template)
	}
	if _, ok := built[v.Projection]; !ok {
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
	case TrendsTemplate, DrilldownTemplate:
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
// and year stems dropped in silence lose whole documents. revenue.html took a
// four-stem list and rendered one year, with every check green, because the
// only thing that reads YearStems is a template arm that page does not have.
//
// TWO TEMPLATES, NOT ONE. The drill-down grew a year control after the bead
// that named this defect was filed, so a guard spelled
// `v.Template == SankeyTemplate` would have been born stale -- which is the
// exact failure templateRendersLede exists to document.
func templateRendersAYearControl(name string) bool {
	switch name {
	case SankeyTemplate, DrilldownTemplate:
		return true
	default:
		return false
	}
}

// templateRendersTiers answers whether a template publishes [View.RenderTiers]
// to the client.
//
// The third field of this family, found by review of the commit that closed the
// first two -- which is the argument for writing them as a family rather than as
// three guards. Only buildDrilldownPage puts RenderTiers in the config blob;
// buildSankeyPage omits the key entirely, and app.js reads
// `CONFIG.render_tiers ?? []`, so a fold asked for on the spine is not refused,
// not reported, and not applied: the chart draws every tier and looks like a
// chart rather than like a defect.
func templateRendersTiers(name string) bool {
	return name == DrilldownTemplate
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
	case reserved[rel], strings.HasPrefix(rel, DataDir+"/"), strings.HasPrefix(rel, "vendor/"):
		return fmt.Errorf("asset path %q is part of the fixed site layout", rel)
	}
	return nil
}

// Write renders the site into o.Dir, creating it if needed, and returns the
// slash-separated paths it wrote, relative to o.Dir and sorted.
//
// Write does not clean the directory: destroying files is a separate decision
// with its own guard rail (cmdutil.SafeCleanDir), and burying it in a writer
// would make every caller of Write a caller of RemoveAll.
func Write(o Options) ([]string, error) {
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

	// #nosec G301 -- the output is a web root; a directory a server running as
	// another user cannot traverse is a broken deploy, not a hardened one.
	if err := os.MkdirAll(o.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	var written []string
	seen := make(map[string]bool)
	write := func(rel string, b []byte) error {
		if seen[rel] {
			// Two writers landing on one path is a silent overwrite: the file
			// is there, the export succeeds, and which of the two bytes won
			// depends on map order.
			return fmt.Errorf("two assets claim the output path %q", rel)
		}
		seen[rel] = true
		dst := filepath.Join(o.Dir, filepath.FromSlash(rel))
		// #nosec G301 -- as above: these are published directories.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create %q: %w", path.Dir(rel), err)
		}
		// #nosec G306 -- these are published web assets; 0600 would make the
		// output unreadable to a web server running as another user.
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", rel, err)
		}
		written = append(written, rel)
		return nil
	}

	// The marker goes down first, before anything that can fail. A run that
	// dies midway then leaves a directory that --clean recognises as ours;
	// writing it last would dead-end the retry, because index.html is not
	// there either and SafeCleanDir would refuse to touch the debris.
	if err := write(MarkerName, []byte("fisc export\n")); err != nil {
		return nil, err
	}
	for _, name := range verbatimAssets {
		b, rerr := fs.ReadFile(o.assetTree(), name)
		if rerr != nil {
			return nil, fmt.Errorf("read embedded asset %q: %w", name, rerr)
		}
		if werr := write(name, b); werr != nil {
			return nil, werr
		}
	}
	if err := copyTree(o.assetTree(), "vendor", write); err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(o.Projections) {
		if err := write(path.Join(DataDir, name+".json"), o.Projections[name]); err != nil {
			return nil, err
		}
	}
	for _, rel := range sortedKeys(files) {
		if err := write(rel, files[rel]); err != nil {
			return nil, err
		}
	}
	// The pages are written last, and index.html last of all: it is the
	// completed-site sentinel, so it must not exist until the site around it
	// does -- including the other views it links to, or the entry point would
	// appear while its own nav pointed at 404s.
	for _, p := range pages {
		if p.Path == IndexPath {
			continue
		}
		if err := write(p.Path, p.HTML); err != nil {
			return nil, err
		}
	}
	for _, p := range pages {
		if p.Path != IndexPath {
			continue
		}
		if err := write(p.Path, p.HTML); err != nil {
			return nil, err
		}
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
// A cited page the tree does not hold is an error. The alternative is a page
// that renders a citation nobody can follow, which is the failure this whole
// change exists to remove.
func withPageText(files map[string][]byte, tree fs.FS, cited []citation, reserved map[string]bool) (map[string][]byte, error) {
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
