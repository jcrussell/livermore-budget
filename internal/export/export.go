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

// PrimaryProjection is the projection whose metadata drives the page. The
// page is a Sankey page: its title, headline figures and caveats come from
// that document, and exporting without it is an error rather than a page with
// blanks where the numbers go.
const PrimaryProjection = "sankey"

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

	// YearStems are the documents that are the same projection for different
	// fiscal years, in the order a reader should meet them, opening year first.
	// Empty means one year, and the page renders no year control.
	//
	// It is STATED BY THE CALLER rather than inferred from the stems. The
	// obvious shortcut — treat every "sankey-*" stem as a year of "sankey" — is
	// a guess about what a name means, and it is wrong for the first projection
	// named after the primary that is not a year of it. Which documents are
	// years of which is the composition root's knowledge; this package lays out
	// what it is handed and does not import internal/project to find out.
	//
	// Every entry must be a key of Projections, and the first must be
	// PrimaryProjection. Write refuses otherwise.
	YearStems []string

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
	// A year stem naming a document that was not built would render a control
	// the reader can move to a 404, so it is refused here rather than discovered
	// in the browser. The first entry must be the opening year, because that is
	// the document the page's server-rendered figures came from.
	if len(o.YearStems) > 0 {
		if o.YearStems[0] != PrimaryProjection {
			return fmt.Errorf("the first year stem is %q, but the page opens on %q",
				o.YearStems[0], PrimaryProjection)
		}
		seen := make(map[string]bool, len(o.YearStems))
		for _, stem := range o.YearStems {
			if _, ok := o.Projections[stem]; !ok {
				return fmt.Errorf("year stem %q names no projection that was built", stem)
			}
			if seen[stem] {
				return fmt.Errorf("year stem %q is listed twice", stem)
			}
			seen[stem] = true
		}
	}
	for rel := range o.Files {
		if err := assetPath(rel); err != nil {
			return err
		}
	}
	return nil
}

// reservedPaths are the output paths Write owns. An asset landing on one of
// them would not be an extra file but a replaced one: the site would still
// export, and the page would be broken in the browser only.
var reservedPaths = func() map[string]bool {
	m := map[string]bool{"index.html": true, MarkerName: true}
	for _, name := range verbatimAssets {
		m[name] = true
	}
	return m
}()

// assetPath screens one [Options.Files] key. The output tree is a web root
// somebody will unpack, serve, or rsync, so a path that escapes it is a defect
// worth refusing at the door rather than a file written outside Dir.
func assetPath(rel string) error {
	switch {
	case rel == "":
		return errors.New("asset path is empty")
	case strings.ContainsRune(rel, '\\'):
		return fmt.Errorf("asset path %q is not slash-separated", rel)
	case path.IsAbs(rel), rel == ".", rel == "..", strings.HasPrefix(rel, "../"), path.Clean(rel) != rel:
		return fmt.Errorf("asset path %q is not a clean relative path", rel)
	case reservedPaths[rel], strings.HasPrefix(rel, DataDir+"/"), strings.HasPrefix(rel, "vendor/"):
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
	assets := o.Assets
	if assets == nil {
		assets = site.FS()
	}
	// Where a page-text citation points is decided once, here, and handed to
	// buildPage as the base it composes: an explicit remote wins, otherwise the
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

	page, cited, err := buildPage(o.Projections, o.YearStems, o.Docs, base, o.GeneratedBy)
	if err != nil {
		return nil, err
	}

	files := o.Files
	if ship {
		files, err = withPageText(files, o.PageText, cited)
		if err != nil {
			return nil, err
		}
	}
	html, err := renderPage(assets, page)
	if err != nil {
		return nil, err
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
		b, rerr := fs.ReadFile(assets, name)
		if rerr != nil {
			return nil, fmt.Errorf("read embedded asset %q: %w", name, rerr)
		}
		if werr := write(name, b); werr != nil {
			return nil, werr
		}
	}
	if err := copyTree(assets, "vendor", write); err != nil {
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
	// index.html is written last: it is the completed-site sentinel, so it
	// must not exist until the site around it does.
	if err := write("index.html", html); err != nil {
		return nil, err
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
func withPageText(files map[string][]byte, tree fs.FS, cited []citation) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files)+len(cited))
	maps.Copy(out, files)
	for _, c := range cited {
		src := path.Join(c.DocID, pageTextPath(c.Page))
		b, err := fs.ReadFile(tree, src)
		if err != nil {
			return nil, fmt.Errorf("read the extracted text of %s page %d: %w", c.DocID, c.Page, err)
		}
		rel := LocalPageTextBase(c.DocID) + pageTextFile(c.Page)
		if err := assetPath(rel); err != nil {
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
