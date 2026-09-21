// Package export implements `fisc export`, which writes the static site.
//
// The command owns three things and delegates everything else: where the
// output goes (and whether deleting what is there is safe), which projections
// to publish, and telling the user how to look at the result. Rendering is
// internal/export's; building the projection JSON is internal/project's.
package export

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// defaultOutputDir is where the site lands when --output is not given. It is
// gitignored: exporting into the working tree by default would make `git
// status` noise the price of looking at your own chart.
const defaultOutputDir = "dist"

// result is everything a Builder produces for one export.
//
// TWO MAPS RATHER THAN ONE, because they land in different namespaces and the
// difference is contractual. Projections is keyed by filename STEM and written
// to data/<stem>.json, a name internal/export refuses if it contains '/', '\\'
// or '.', because docs/sankey-contract.md promises that directory means "one
// file per projection". Files is keyed by a slash-separated path RELATIVE TO
// THE OUTPUT ROOT and written verbatim wherever it says. Merging them would
// need a rule for telling a stem from a path, and every such rule is a guess
// about what a name means.
type result struct {
	Projections map[string][]byte
	// Files is the non-projection asset channel: anything the site ships that
	// is not a projection document. See export.Options.Files, which has had
	// exactly one user -- the cited pages' text, put through it by Write
	// itself -- because until now no Builder could reach it.
	Files map[string][]byte

	// PageIndex and Downloads are what the composition root SAYS ABOUT those
	// files: which locator each one holds the records for, and which are whole-
	// store downloads. They are here rather than recovered inside
	// internal/export by reading the Files keys, because that would be the
	// packager parsing a path to learn what a name means -- the guessing
	// export.Options.Views' doc comment refuses.
	PageIndex []export.PageIndexEntry
	Downloads []export.Download
	// RecordsBase is where each document's shards live, keyed by doc id, in the
	// form a client appends pNNNN.jsonl to. Same reason as the two above: the
	// locator-to-URL rule is this package's, and internal/export publishes the
	// base without learning it.
	RecordsBase map[string]string
	// Structure is the verification lattice, one document per fiscal year.
	//
	// IT IS BUILT AND IT IS NOT PUBLISHED, which is why it is its own field
	// and not an entry in Files. Nothing under site/, tools/ or docs/ names
	// these documents and `fisc verify` never reads the exported tree, so
	// shipping them put 877,403 bytes -- measured over the committed corpus --
	// in front of a reader who has no way to use them. They stay built so
	// structure_test.go can go on re-measuring each part against the whole.
	//
	// THE OPEN QUESTION IS NOT WHICH PATH IT TAKES, it is who reads it at all:
	// an artifact built and tested and consumed by nothing is a check with no
	// subject. fisc-d02r records it.
	Structure map[string][]byte
}

// builder produces everything to publish: the projection documents, keyed by
// the projection name (a column of its own, or data/<stem>.json where it
// states no column), and any other assets the site ships.
//
// This is the seam onto internal/project (fisc-gxa.1). It is a function type
// rather than that package's Projection interface on purpose: internal/export
// must not know how a projection is built, and this command must not fall
// over because the projection package is mid-flight.
//
// IT RETURNS ASSETS AS WELL AS PROJECTIONS (fisc-xgr) because the asset channel
// was reachable only from inside internal/export. export.Options.Files existed
// and this command never set it, so the one thing that travelled it was the
// page text Write ships on its own behalf; nothing a Builder produced could get
// out. That is what the fact store needs.
type builder func(repoRoot string) (result, error)

// Options is one invocation of the command.
type Options struct {
	IO       *iostreams.IOStreams
	RepoRoot func() (string, error)

	// OutputDir is the site's destination. Validate canonicalises it, so
	// after validation it is absolute and symlink-resolved.
	OutputDir string

	// Clean empties OutputDir first. It refuses to empty a directory that
	// does not look like a generated site; see cmdutil.SafeCleanDir.
	Clean bool

	// SourceBrowseURL cites the committed page text at a remote — a forge's
	// blob view of this repository — instead of shipping it inside the site.
	// Empty is the default and the better answer: the export copies the cited
	// pages out of data/extracted/ and cites those, so the provenance resolves
	// with no third party involved. This exists for a deploy that would rather
	// link to a browsable tree, and it is opt-in because the link is a promise
	// about somebody else's server.
	SourceBrowseURL string

	// Build produces the projections and any other assets. Nil means buildAll.
	Build builder

	// extractedDir is the committed extraction tree. Empty means the
	// repository's, which is what every real invocation uses.
	//
	// It is a seam of the same kind as Build above, and it exists because the
	// property worth testing -- that a pre-detectable fault never costs a
	// reader their site -- is reached through a page the extraction does not
	// carry. The alternative was for a test to move a committed file aside and
	// put it back, which takes the repository down with it if the test fails
	// midway.
	extractedDir string
}

// Validate checks the flags and canonicalises the output path. It runs before
// anything is created or deleted, so a bad --output is a usage error rather
// than a half-written directory.
func (o *Options) Validate() error {
	if o.OutputDir == "" {
		return cmdutil.FlagErrorf("--output requires a directory")
	}
	resolved, err := cmdutil.ResolveOutputDir(o.OutputDir)
	if err != nil {
		return cmdutil.FlagErrorf("%s", err)
	}
	if err := cmdutil.WritableDir(resolved); err != nil {
		return cmdutil.FlagErrorf("%s", err)
	}
	if o.SourceBrowseURL != "" {
		if err := validateBrowseURL(o.SourceBrowseURL); err != nil {
			return cmdutil.FlagErrorf("--source-browse-url %s", err)
		}
	}
	o.OutputDir = resolved
	return nil
}

// validateBrowseURL refuses a base the page could not cite. Every citation on
// the site is composed from it, so a value that is not an absolute http(s) base
// would not fail here — every citation on the site would be a dead link, and
// the export would still finish cleanly.
func validateBrowseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q must be an http or https URL", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("%q has no host", raw)
	}
	return nil
}

// NewCmdExport builds the export command. runF is the test seam: when it is
// non-nil the command validates and hands the options over instead of running.
func NewCmdExport(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{
		IO:        f.IOStreams,
		RepoRoot:  f.RepoRoot,
		OutputDir: defaultOutputDir,
	}

	cmd := &cobra.Command{
		Use:     "export",
		Short:   "Write the static site",
		GroupID: "site", // root.GroupSite; a literal, because root imports this package.
		Long: `Write the static site into a directory.

The output is self-contained and every path in it is relative, so it serves
from a file tree, from a GitHub Pages project subpath, or from a domain root
without configuration. The page fetches its data at runtime, which browsers
refuse to do from a file:// URL — serve the directory over HTTP to look at it.

Self-contained includes the provenance: the committed text of every page the
projection cites is copied into the site, so both citation classes resolve
without reaching a forge. Pass --source-browse-url to cite a browsable copy of
the repository instead.`,
		Example: `  # Write the site to dist/ and read it in a browser
  fisc export
  python3 -m http.server -d dist 8000

  # Refresh the committed site directory
  fisc export --output build/site --clean

  # Cite the extracted page text on a forge instead of shipping it
  fisc export --source-browse-url https://github.com/you/livermore-budget/blob/main`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := opts.Validate(); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return exportRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputDir, "output", "o", defaultOutputDir, "Directory to write the site into")
	cmd.Flags().BoolVar(&opts.Clean, "clean", false, "Empty the output directory first (refuses anything that is not a generated site)")
	cmd.Flags().StringVar(&opts.SourceBrowseURL, "source-browse-url", "",
		"Cite the extracted page text at this base URL instead of shipping it in the site (e.g. "+export.DefaultSourceBrowseURL+")")

	return cmd
}

// extractionTree is where the committed page text is read from.
func (o *Options) extractionTree(repoRoot string) string {
	if o.extractedDir != "" {
		return o.extractedDir
	}
	return filepath.Join(repoRoot, filepath.FromSlash(cmdutil.ExtractedDir))
}

func exportRun(o *Options) error {
	root, err := o.RepoRoot()
	if err != nil {
		return err
	}

	build := o.Build
	if build == nil {
		build = buildAll
	}
	built, err := build(root)
	if err != nil {
		return err
	}
	projections := built.Projections

	docs, err := loadDocs(root)
	if err != nil {
		return err
	}

	// EVERY PUBLISHED DOCUMENT IS EITHER RENDERED OR DECLARED UNRENDERED. It
	// lives here rather than inside export.Write because internal/export does
	// not know what the site publishes -- PublishedDocuments lives in
	// internal/project, which that package deliberately does not import.
	// assertPublishedBuilt above says the document exists; this says a reader
	// can get to it.
	//
	// AND IT RUNS BEFORE --clean, with every other validation, rather than
	// between Clean and Write. It needs nothing Clean produces, and running it
	// after would mean `fisc export --clean` over a corpus that has just
	// published an undeclared document EMPTIES the output directory and then
	// refuses -- destroying a site to report a fault that was detectable before
	// anything was touched.
	siteViews := views(built)
	if err = assertPublishedReachable(siteViews, projections); err != nil {
		return err
	}

	site := export.Options{
		Dir:         o.OutputDir,
		Projections: projections,
		// WHICH VIEWS THE SITE HAS IS STATED HERE, in the composition root, and
		// not inferred by the packager from its stems. internal/export does not
		// import internal/project and must not start guessing what a name
		// means -- which document is a year of which, and which are separate
		// views, are both facts about the projections and neither is legible
		// from a filename.
		Views:       siteViews,
		Docs:        docs,
		GeneratedBy: generatedBy(),
		// The extraction tree, so the site ships the text of the pages it
		// cites. Rooted at data/extracted/ because that is where the doc-id
		// directories start; internal/export reads only the cited pages out
		// of it. Passed even with --source-browse-url set: Write decides
		// between the two, and this command asserting the precedence too
		// would be a second place for it to change.
		PageText:        os.DirFS(o.extractionTree(root)),
		SourceBrowseURL: o.SourceBrowseURL,
		// Whatever else the Builder produced. Every key is screened through
		// assetPath, so a path that escapes the output root or shadows a fixed
		// one is refused rather than written and noticed later.
		Files:       built.Files,
		PageIndex:   built.PageIndex,
		Downloads:   built.Downloads,
		RecordsBase: built.RecordsBase,
	}

	// AND THE WHOLE SITE IS RESOLVED BEFORE --clean, for the reason spelled out
	// above assertPublishedReachable and now applying to every remaining class
	// of input. Prepare validates, renders every page, and resolves the cited
	// page text out of the extraction tree; after it returns, only I/O can
	// fail. A caller that learns any of that from Write has already emptied the
	// reader's site to find out.
	//
	// THIS REPLACED A NARROWER HOIST, and the gap is worth recording. An
	// earlier version of this hunk called a validate-only method here and left
	// withPageText inside Write, so an export whose fact store covers a page
	// the extraction does not still destroyed the output and then refused --
	// and this command's own PageIndex seeding widened that class, because it
	// makes pages cited that no projection names. Reproduced by moving one
	// committed page text aside. Resolving everything is the only
	// version of this that stays true as more is added to Write.
	plan, err := export.Prepare(site)
	if err != nil {
		return err
	}

	if o.Clean {
		if cerr := cmdutil.SafeCleanDir(o.OutputDir); cerr != nil {
			return cmdutil.WithHint(cerr, "pass a different --output, or empty that directory yourself")
		}
	}

	written, err := plan.Write()
	if err != nil {
		return err
	}

	// The paths are the command's data: they are what a caller pipes into a
	// checksum, an uploader, or wc -l. Everything else is chatter.
	for _, rel := range written {
		fmt.Fprintln(o.IO.Out, filepath.Join(o.OutputDir, filepath.FromSlash(rel)))
	}

	display := o.OutputDir
	if wd, werr := os.Getwd(); werr == nil {
		if rel, rerr := filepath.Rel(wd, o.OutputDir); rerr == nil && len(rel) < len(display) {
			display = rel
		}
	}
	fmt.Fprintf(o.IO.ErrOut, "wrote %d files to %s\n", len(written), display)
	// Say this every time, not only on failure: the page fetches its data,
	// and fetch() from a file:// origin is blocked in Chrome, so the obvious
	// way to look at the output is the one way that does not work.
	fmt.Fprintf(o.IO.ErrOut, "the page fetches its data, which browsers block on file:// URLs — serve it over HTTP:\n  python3 -m http.server -d %s 8000\n", display)

	return nil
}
