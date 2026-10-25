// Package root assembles the fisc command tree and owns nothing else.
package root

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/build"
	// internal/build already holds the name `build`.
	buildcmd "github.com/jcrussell/livermore-budget/pkg/cmd/build"
	exportcmd "github.com/jcrussell/livermore-budget/pkg/cmd/export"
	verifycmd "github.com/jcrussell/livermore-budget/pkg/cmd/verify"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// Command groups, so `fisc --help` reads as a workflow.
const (
	groupData = "data"
	groupSite = "site"
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

		// Without NoArgs an unknown command prints help and exits 0. The RunE
		// makes Args reachable: cobra returns flag.ErrHelp for a non-runnable
		// command before it validates args.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},

		// The runner reports errors; usage is silenced on runtime failures.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetVersionTemplate("{{.Version}}\n")

	// Route pflag's parse errors into the typed vocabulary so they exit 2.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cmdutil.FlagError{Err: err}
	})

	cmd.AddGroup(
		&cobra.Group{ID: groupData, Title: "Data commands"},
		&cobra.Group{ID: groupSite, Title: "Site commands"},
	)

	// reanchor is still to come (fisc-mq4.*, fisc-gxa.*).
	cmd.AddCommand(buildcmd.NewCmdBuild(f, nil))
	cmd.AddCommand(verifycmd.NewCmdVerify(f, nil))
	cmd.AddCommand(exportcmd.NewCmdExport(f, nil))

	refuseUnknownHelpTopics(cmd)

	return cmd
}

// refuseUnknownHelpTopics makes `fisc help <typo>` a usage error. With Args
// set, root's Find never fails, so cobra's help prints root's help and exits 0.
// It wraps cobra's help command so the text and completion stay cobra's.
func refuseUnknownHelpTopics(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	help, _, err := root.Find([]string{"help"})
	if err != nil || help == root || help.Run == nil {
		return // TestUnknownHelpTopicExitsTwo is what notices
	}
	show := help.Run
	help.Run = nil
	help.RunE = func(c *cobra.Command, args []string) error {
		if _, rest, ferr := c.Root().Find(args); ferr != nil || len(rest) > 0 {
			return cmdutil.FlagErrorf("unknown help topic %q", strings.Join(args, " "))
		}
		show(c, args)
		return nil
	}
}
