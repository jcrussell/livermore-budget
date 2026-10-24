// Package export implements `fisc export`, which writes the static site.
//
// The command owns three things and delegates everything else: where the
// output goes (and whether deleting what is there is safe), which projections
// to publish, and telling the user how to look at the result. Rendering is
// internal/export's; building the projection JSON is internal/project's.
package export

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/repo"
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
}

// builder produces everything to publish: the projection documents, keyed by
// the projection name (a column of its own, or data/<stem>.json where it
// states no column), and any other assets the site ships.
//
// This is the seam onto internal/project. It is a function type
// rather than that package's Projection interface on purpose: internal/export
// must not know how a projection is built, and this command must not fall
// over because the projection package is mid-flight.
//
// IT RETURNS ASSETS AS WELL AS PROJECTIONS because the asset channel
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

	// Clean removes the files the earlier export's manifest lists and this
	// export does not write, the manifest temp files (.fisc-tmp-N) an
	// interrupted export stranded at the top of the directory, and the
	// directories that leaves empty or that stand empty where this export
	// writes a file. No other file a manifest does not list is removed; see
	// export.RefuseForeignDir for which directories carry one.
	Clean bool

	// DryRun prints the files and directories Clean would remove and writes
	// nothing. It requires Clean.
	DryRun bool

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
	if o.DryRun && !o.Clean {
		return cmdutil.FlagErrorf("--dry-run requires --clean")
	}
	resolved, err := export.ResolveOutputDir(o.OutputDir)
	if err != nil {
		return cmdutil.FlagErrorf("%s", err)
	}
	if err := export.RefuseForeignDir(resolved); err != nil {
		if errors.Is(err, export.ErrForeignDir) {
			return cmdutil.FlagErrorf("%s", err)
		}
		return err
	}
	// A dry run writes nothing, so it neither needs a writable directory nor
	// leaves the probe's trace in one.
	if !o.DryRun {
		if err := export.WritableDir(resolved); err != nil {
			return cmdutil.FlagErrorf("%s", err)
		}
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
	// The base is copied into every citation the site publishes, so a
	// credential in it would be published too, and a token passed as the
	// username is as secret as a password. Userinfo needs an "@", and a
	// browse base has no other use for one, so any "@" is refused here
	// without repeating the value, and every later refusal may name raw:
	// none of them can see an "@".
	if strings.Contains(raw, "@") {
		return errors.New(`carries an "@", which may set off a credential, so the value is not repeated; the base is copied into every published citation`)
	}
	u, err := url.Parse(raw)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q must be an http or https URL", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("%q has no host", raw)
	}
	// Each citation appends a repository path to the base, so a query or a
	// fragment would sit in front of the path, and an empty one is still a
	// "?" or a "#" there: url.URL keeps no flag for an empty fragment.
	if u.RawQuery != "" || u.ForceQuery {
		return fmt.Errorf("%q carries a query; every citation appends a path to the base, which would land after it", raw)
	}
	if strings.Contains(raw, "#") {
		return fmt.Errorf("%q carries a fragment; every citation appends a path to the base, which would land after it", raw)
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
the repository instead.

It writes only into a directory that is missing, empty, or carries the
manifest an earlier export left (.fisc-export, listing every file it wrote);
any other directory is refused before anything is written. A manifest an
older fisc wrote is not one this fisc reads: remove that directory and export
again.

Writing never deletes, and never writes over a file the manifest does not
list: one at a path the export writes is refused before anything is written.
--clean removes what the earlier export's manifest lists and this export does
not write, any manifest temp file (.fisc-tmp-N) an interrupted export left at
the top of the directory, and the directories that leaves empty or that stand
empty where this export writes a file; any other file no export wrote is left
where it is. --clean --dry-run prints the files --clean would remove, then
the directories, each with a trailing separator, one per line, and writes
nothing.`,
		Example: `  # Write the site to dist/ and read it in a browser
  fisc export
  python3 -m http.server -d dist 8000

  # Refresh an earlier export, removing the files it wrote that this one does not
  fisc export --output build/site --clean

  # See what --clean would remove, without writing or removing anything
  fisc export --output build/site --clean --dry-run

  # Cite the extracted page text on a forge instead of shipping it
  fisc export --source-browse-url https://github.com/you/livermore-budget/blob/main`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := opts.Validate(); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return exportRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputDir, "output", "o", defaultOutputDir, "Directory to write the site into (missing, empty, or an earlier export)")
	cmd.Flags().BoolVar(&opts.Clean, "clean", false, "Remove the files the earlier export wrote and this one does not")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "With --clean, print the files and directories it would remove and write nothing")
	cmd.Flags().StringVar(&opts.SourceBrowseURL, "source-browse-url", "",
		"Cite the extracted page text at this base URL instead of shipping it in the site (e.g. "+export.DefaultSourceBrowseURL+")")

	return cmd
}

// siteOptions is the site exportRun writes from built: the views
// data/views.yaml declares, and everything the packager reads beside them.
func (o *Options) siteOptions(root string, built result) (export.Options, error) {
	projections := built.Projections

	docs, err := loadDocs(root)
	if err != nil {
		return export.Options{}, err
	}

	siteViews, err := views(root, built)
	if err != nil {
		return export.Options{}, err
	}
	departments, err := departmentMembership(root)
	if err != nil {
		return export.Options{}, err
	}
	apartFromSpine, err := departmentsApartFromSpine()
	if err != nil {
		return export.Options{}, err
	}
	return export.Options{
		Dir:         o.OutputDir,
		Projections: projections,
		// WHICH VIEWS THE SITE HAS IS STATED HERE, in the composition root, and
		// not inferred by the packager from its stems. internal/export must not
		// start guessing what a name means -- which document is a year of
		// which, and which are separate views, are both facts about the
		// projections and neither is legible from a filename.
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
		// The department page's membership and the cells pp.85-125's two
		// blocks are declared not to tie in: both this command's to state.
		Departments:          departments,
		DepartmentsHeldApart: departmentsHeldApart(),
		// And the columns where the department rows are declared apart from
		// the spine's citywide figure, which the glance page prints.
		DepartmentsApartFromSpine: apartFromSpine,
		// EVERY PUBLISHED DOCUMENT IS EITHER RENDERED OR DECLARED UNRENDERED.
		// It is this command's gate rather than internal/export's because what
		// the site publishes, project.PublishedDocuments, is this command's to
		// hold the views to; internal/export lays out the views it is handed.
		// assertPublishedBuilt above says the document exists; this says a
		// reader can get to it.
		//
		// A GATE, SO IT RUNS AFTER VALIDATION AND BEFORE ANY PAGE RENDERS: it
		// reads the documents a view reaches through export.View.ReachedStems,
		// over the column index validation has accepted.
		Gate: func(ix export.ColumnIndex) error {
			return assertPublishedReachable(siteViews, projections, ix)
		},
	}, nil
}

// extractionTree is where the committed page text is read from.
func (o *Options) extractionTree(repoRoot string) string {
	if o.extractedDir != "" {
		return o.extractedDir
	}
	return filepath.Join(repoRoot, filepath.FromSlash(repo.ExtractedDir))
}

// exportRun checks ctx between stages and Write checks it between files: a
// cancellation seen before Write leaves the output directory as it was, and
// one during it leaves a partial site whose manifest lists every file an
// export may have put there, so the next run, with --clean, converges.
func exportRun(ctx context.Context, o *Options) error {
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
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("export: %w", cerr)
	}
	site, err := o.siteOptions(root, built)
	if err != nil {
		return err
	}

	// THE WHOLE SITE IS RESOLVED BEFORE ANYTHING IS WRITTEN. Prepare validates,
	// runs the siteOptions gate, renders every page, and resolves the cited
	// page text out of the extraction tree, this command's PageIndex seeding
	// included; after it returns, only I/O can fail. A caller that learned any
	// of that from Write would leave the reader's site half overwritten to find
	// out.
	plan, err := export.Prepare(site)
	if err != nil {
		return err
	}
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("export: %w", cerr)
	}

	if o.DryRun {
		files, dirs, oerr := plan.Obsolete()
		if oerr != nil {
			return usage(oerr)
		}
		// Files first, then directories deepest first with a trailing
		// separator: the order --clean removes them in.
		for _, rel := range files {
			fmt.Fprintln(o.IO.Out, filepath.Join(o.OutputDir, filepath.FromSlash(rel)))
		}
		for _, rel := range dirs {
			fmt.Fprintln(o.IO.Out, filepath.Join(o.OutputDir, filepath.FromSlash(rel))+string(filepath.Separator))
		}
		fmt.Fprintf(o.IO.ErrOut, "--clean would remove %d files and %d directories; dry run, nothing written\n", len(files), len(dirs))
		return nil
	}

	written, removed, err := plan.Write(ctx, o.Clean)
	if err != nil {
		return usage(err)
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
	if o.Clean {
		fmt.Fprintf(o.IO.ErrOut, "removed %d files the earlier export wrote and this one does not, %d stranded manifest temp files and %d empty directories\n",
			len(removed.Files), len(removed.Temps), len(removed.Dirs))
	}
	// Say this every time, not only on failure: the page fetches its data,
	// and fetch() from a file:// origin is blocked in Chrome, so the obvious
	// way to look at the output is the one way that does not work.
	fmt.Fprintf(o.IO.ErrOut, "the page fetches its data, which browsers block on file:// URLs — serve it over HTTP:\n  python3 -m http.server -d %s 8000\n", display)

	return nil
}

// usage makes a refusal of the output directory a usage error wherever it is
// found, as Validate does with the ones it finds, so it exits 2.
func usage(err error) error {
	if errors.Is(err, export.ErrForeignDir) {
		return &cmdutil.FlagError{Err: err}
	}
	return err
}
