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

// DefaultOutputDir is where the site lands when --output is not given. It is
// gitignored: exporting into the working tree by default would make `git
// status` noise the price of looking at your own chart.
const DefaultOutputDir = "dist"

// Builder produces the projection documents to publish, keyed by the filename
// stem they are written under (data/<stem>.json).
//
// This is the seam onto internal/project (fisc-gxa.1). It is a function type
// rather than that package's Projection interface on purpose: internal/export
// must not know how a projection is built, and this command must not fall
// over because the projection package is mid-flight. When Registry() exists,
// buildProjections becomes a loop over it and nothing else here changes.
type Builder func(repoRoot string) (map[string][]byte, error)

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

	// Build produces the projections. Nil means buildProjections.
	Build Builder
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
// would not fail here — it would render 24 dead links and export cleanly.
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
		OutputDir: DefaultOutputDir,
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

	cmd.Flags().StringVarP(&opts.OutputDir, "output", "o", DefaultOutputDir, "Directory to write the site into")
	cmd.Flags().BoolVar(&opts.Clean, "clean", false, "Empty the output directory first (refuses anything that is not a generated site)")
	cmd.Flags().StringVar(&opts.SourceBrowseURL, "source-browse-url", "",
		"Cite the extracted page text at this base URL instead of shipping it in the site (e.g. "+export.DefaultSourceBrowseURL+")")

	return cmd
}

func exportRun(o *Options) error {
	root, err := o.RepoRoot()
	if err != nil {
		return err
	}

	build := o.Build
	if build == nil {
		build = buildProjections
	}
	projections, err := build(root)
	if err != nil {
		return err
	}

	docs, err := loadDocs(root)
	if err != nil {
		return err
	}

	if o.Clean {
		if cerr := cmdutil.SafeCleanDir(o.OutputDir); cerr != nil {
			return cmdutil.WithHint(cerr, "pass a different --output, or empty that directory yourself")
		}
	}

	written, err := export.Write(export.Options{
		Dir:         o.OutputDir,
		Projections: projections,
		// WHICH VIEWS THE SITE HAS IS STATED HERE, in the composition root, and
		// not inferred by the packager from its stems. internal/export does not
		// import internal/project and must not start guessing what a name
		// means -- which document is a year of which, and which are separate
		// views, are both facts about the projections and neither is legible
		// from a filename.
		Views:       views(projections),
		Docs:        docs,
		GeneratedBy: generatedBy(),
		// The extraction tree, so the site ships the text of the pages it
		// cites. Rooted at data/extracted/ because that is where the doc-id
		// directories start; internal/export reads only the cited pages out
		// of it. Passed even with --source-browse-url set: Write decides
		// between the two, and this command asserting the precedence too
		// would be a second place for it to change.
		PageText:        os.DirFS(filepath.Join(root, filepath.FromSlash(cmdutil.ExtractedDir))),
		SourceBrowseURL: o.SourceBrowseURL,
	})
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
