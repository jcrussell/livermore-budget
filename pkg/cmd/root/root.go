// Package root assembles the fisc command tree. It aggregates feature
// packages and owns nothing else — adding a command is one import and one
// AddCommand line.
package root

import (
	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/build"
	// internal/build is the link-time version metadata and already holds the
	// name `build`, so the command package that produces facts is aliased
	// rather than renamed: `fisc build` is the user-facing name and the
	// package should keep it.
	buildcmd "github.com/jcrussell/livermore-budget/pkg/cmd/build"
	exportcmd "github.com/jcrussell/livermore-budget/pkg/cmd/export"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// Command groups. Commands set GroupID so `fisc --help` reads as a workflow
// rather than an alphabetical list.
const (
	GroupData = "data"
	GroupSite = "site"
)

// NewCmdRoot builds the root command.
func NewCmdRoot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fisc",
		Short: "Trace City of Livermore budget figures from PDF to published chart",
		Long: `fisc turns the City of Livermore's published budget documents into a
verified fact store, and that fact store into a static site.

Every figure it publishes carries a provenance pointer back to a page and
cell of a source PDF, and 'fisc verify' fails if any link in that chain
breaks.

Extraction is deliberately not part of this tool: run 'make extract' to
regenerate data/extracted/ with tools/extract.py. fisc reads only the
committed artifacts, so it needs neither Python nor the source PDFs.`,
		Version: build.Get().String(),

		// Reject stray positional args. Without this, an unknown command is
		// silently treated as an argument to root, which prints help and
		// exits 0 -- a typo'd command would look like success. cobra.NoArgs
		// reports it as `unknown command "x" for "fisc"`, which the runner
		// maps to exit 2.
		//
		// The RunE below is what makes that validation reachable: cobra
		// returns flag.ErrHelp for a non-runnable command *before* it
		// validates args, so a root with no RunE ignores Args entirely.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},

		// Errors are reported by the runner, which knows about exit codes.
		// Usage is silenced so a runtime failure doesn't dump the help text.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetVersionTemplate("{{.Version}}\n")

	// Route pflag's parse errors into the typed vocabulary so they exit 2.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cmdutil.FlagError{Err: err}
	})

	cmd.AddGroup(
		&cobra.Group{ID: GroupData, Title: "Data commands"},
		&cobra.Group{ID: GroupSite, Title: "Site commands"},
	)

	// Feature commands are added here as they land: verify, reanchor.
	// See beads fisc-mq4.* and fisc-gxa.*.
	cmd.AddCommand(buildcmd.NewCmdBuild(f, nil))
	cmd.AddCommand(exportcmd.NewCmdExport(f, nil))

	return cmd
}
