// Package export implements `fisc export`, which writes the static site. It
// owns where the output goes and whether cleaning it is safe, which
// projections to publish, and how to view the result; rendering is
// internal/export's and the projection JSON is internal/project's.
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

// defaultOutputDir is gitignored, so a default export is no `git status` noise.
const defaultOutputDir = "dist"

// result is everything a builder produces for one export. Projections is keyed
// by filename stem; internal/export writes each into the column file it folds
// into, or to data/<stem>.json where it states no column. Files is keyed by a
// path relative to the output root. Two maps, because telling a stem from a
// path would be a guess.
type result struct {
	Projections map[string][]byte
	// Files is every non-projection asset; see export.Options.Files.
	Files map[string][]byte

	// PageIndex and Downloads say what those files are, so internal/export
	// never parses a path to learn what a name means.
	PageIndex []export.PageIndexEntry
	Downloads []export.Download
	// RecordsBase maps doc id to the base a client appends pNNNN.jsonl to; the
	// locator-to-URL rule is this package's.
	RecordsBase map[string]string
}

// builder produces the projection documents, keyed by projection name, and any
// other assets the site ships. A function type, not internal/project's
// Projection interface, so internal/export never learns how one is built.
type builder func(repoRoot string) (result, error)

// Options is one invocation of the command.
type Options struct {
	IO       *iostreams.IOStreams
	RepoRoot func() (string, error)

	// OutputDir is the site's destination. Validate canonicalises it, so
	// after validation it is absolute and symlink-resolved.
	OutputDir string

	// Clean removes what the earlier manifest lists and this export does not
	// write, stranded manifest temp files (.fisc-tmp-N), and the directories
	// left or standing empty where this export writes. Nothing else is removed;
	// see export.RefuseForeignDir.
	Clean bool

	// DryRun prints the files and directories Clean would remove and writes
	// nothing. It requires Clean.
	DryRun bool

	// SourceBrowseURL cites the committed page text at a remote instead of
	// shipping it. Empty, the default, copies the cited pages from
	// data/extracted/ so provenance resolves with no third party.
	SourceBrowseURL string

	// Build produces the projections and any other assets. Nil means buildAll.
	Build builder

	// extractedDir is the committed extraction tree; empty means the
	// repository's. A test seam, so a missing page can be planted without
	// moving a committed file aside.
	extractedDir string
}

// Validate checks the flags and canonicalises the output path before anything
// is created or deleted.
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

// validateBrowseURL refuses a base the page could not cite: a bad one would
// make every citation a dead link while the export finished cleanly.
func validateBrowseURL(raw string) error {
	// The base is copied into every citation, so a credential in it would be
	// published. Any "@" is refused without repeating the value, so every
	// later refusal may name raw.
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
	// A citation appends a path to the base, so a query or fragment, even an
	// empty "?" or "#", would sit in front of it.
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
		// Which views the site has is stated here, never inferred by the
		// packager from stems.
		Views:       siteViews,
		Docs:        docs,
		GeneratedBy: generatedBy(),
		// Passed even with --source-browse-url set: Write alone decides the
		// precedence.
		PageText:        os.DirFS(o.extractionTree(root)),
		SourceBrowseURL: o.SourceBrowseURL,
		// Every key is screened through assetPath.
		Files:       built.Files,
		PageIndex:   built.PageIndex,
		Downloads:   built.Downloads,
		RecordsBase: built.RecordsBase,
		// The department page's membership and the cells pp.85-125's two
		// blocks are declared not to tie in.
		Departments:          departments,
		DepartmentsHeldApart: departmentsHeldApart(),
		// The columns where the department rows are declared apart from the
		// spine's citywide figure, which the glance page prints.
		DepartmentsApartFromSpine: apartFromSpine,
		// Every published document is rendered or declared unrendered
		// (project.PublishedDocuments). A gate: it runs after validation and
		// before any page renders, over export.View.ReachedStems.
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

	// The whole site is resolved before anything is written: after Prepare
	// returns, only I/O can fail.
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
		// Files, then directories deepest first with a trailing separator: the
		// order --clean removes them in.
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

	// The paths are the command's data; everything else is chatter.
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
	// Said every time: fetch() from a file:// origin is blocked in Chrome.
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
