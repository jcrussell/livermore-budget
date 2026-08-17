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
//
// Every asset path in the page is relative, so the same output serves from a
// user's file tree, from a project subpath on GitHub Pages, and from a domain
// root without a base-path setting anywhere.
package export

import (
	"errors"
	"fmt"
	"io/fs"
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

// MarkerName is the sentinel written into a generated site. It is what tells
// a later --clean run that the directory is fisc's to delete. The name is
// cmdutil's, not this package's: the writer and the deleter agreeing on it by
// coincidence is how a --clean starts refusing to clean.
const MarkerName = cmdutil.ExportMarkerName

// DefaultSourceBrowseURL is where the committed page text is browsable. It
// points at github.com's blob view, never at raw.githubusercontent.com. The
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

	// Docs are the source documents the page cites, keyed by doc id in the
	// projection's metadata.sources.
	Docs []Doc

	// SourceBrowseURL is the base URL for committed page text. Empty means
	// DefaultSourceBrowseURL.
	SourceBrowseURL string

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
	browse := o.SourceBrowseURL
	if browse == "" {
		browse = DefaultSourceBrowseURL
	}

	page, err := buildPage(o.Projections, o.Docs, browse, o.GeneratedBy)
	if err != nil {
		return nil, err
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
	write := func(rel string, b []byte) error {
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
