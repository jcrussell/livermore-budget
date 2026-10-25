// Package verify implements `fisc verify`, which tests the fact store's order
// and identity, the vocabulary, and the graph's arithmetic from committed files
// alone: no source PDF, no Python, no network.
//
// Exit codes: 0 clean; 3 a check failed or reached no verdict (read the
// report); 1 no report, the corpus could not load; 2 a bad flag. A vacuous
// check is not a pass: `--strict` fails an undeclared vacancy, and a
// declaredVacuous entry whose check is no longer vacuous fails every run.
package verify

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/build"
	"github.com/jcrussell/livermore-budget/internal/check"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// exitReportNotClean is the exit code for a report that is not clean, distinct
// from 1 (no report). Fixed rather than the failure count: an exit code is one
// byte, so 256 failures would exit 0.
const exitReportNotClean = 3

// loader builds the subject the checks read. Nil means [check.Load], defaulted
// in the run function so a command built for `--help` touches no filesystem.
type loader func(check.LoadOptions) (*check.Subject, error)

// Options carries the command's dependencies and its parsed flags.
type Options struct {
	IO       *iostreams.IOStreams
	RepoRoot func() (string, error)

	// JSON emits the report as JSON on stdout instead of a summary on stderr.
	JSON bool
	// Full runs the checks that need the source documents under data/pdf/.
	Full bool
	// Strict makes a check that had nothing to check fail the run.
	Strict bool

	// Load builds the subject. Nil means check.Load.
	Load loader
	// Checks is a seam because the committed corpus produces only some of
	// the five verdicts, and each must render and score correctly.
	Checks func() []check.Check
}

// NewCmdVerify builds the `fisc verify` command.
func NewCmdVerify(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{IO: f.IOStreams, RepoRoot: f.RepoRoot}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check every claim the published data rests on",
		Long: `Check the committed corpus against itself and against the documents it came
from.

Verify reads facts/facts.jsonl, the rules under mappings/, the three registries
under data/, and every artifact under data/extracted/ -- which it hashes against
the manifest that lists them. It needs neither the source PDFs nor Python, which
is what lets it run in CI on a plain clone; --full adds the one check that reads
the source documents.

Each check reports one of five verdicts, and the distinction between two of
them is the point of this command. PASS means the check ran over at least one
subject and every one held. VACUOUS means it ran and had nothing to look at —
"every transfer_id has two equal legs" is true of a graph in which no link
carries one, and reporting that as a pass would tell you the legs had been
checked. A vacuous check is never counted as a pass and never silently becomes
one.

A vacancy is either DECLARED or it fails --strict. Most of the budget is still
unmapped, so a plain run exits 0 on any number of them; --strict exits 3 on a
vacuous check that no entry in internal/check's declaredVacuous names. Each
declaration carries the reason and the bead whose landing deletes it, and every
one is printed on every run whether or not anything is wrong with it.

The converse is not gated on the flag. A declaration whose check has started
reporting PASS or FAIL has stopped being true, and that exits 3 with or without
--strict -- it is a false statement in this repository's source rather than a
shortfall in coverage.

FAIL means a claim did not hold and exits 3. ERROR means the checker could not
reach a verdict, which is a different problem from a corpus that is wrong, and
also exits 3. SKIPPED means the check needed --full and did not run.`,
		Example: `  # Check everything that needs no source document
  fisc verify

  # Machine-readable, for CI
  fisc verify --json

  # Fail if any check had nothing to check and no declaration says why
  fisc verify --strict`,
		GroupID: "data", // root.GroupData; a literal, because root imports this package.
		// A stray argument is a mistyped flag; refused, not ignored.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}
			return verifyRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false,
		"Emit the report as JSON on stdout instead of a summary on stderr")
	cmd.Flags().BoolVar(&opts.Full, "full", false,
		"Also hash the source PDFs under data/pdf/ against what the registry and the "+
			"manifests record (needs `git lfs pull`)")
	cmd.Flags().BoolVar(&opts.Strict, "strict", false,
		"Fail if any check had nothing to check and no declaration names the work "+
			"that would give it a subject")

	return cmd
}

// Validate reports flags that cannot produce a meaningful run, before anything
// is opened (byob-input-validation.5). There is nothing to reject today.
func (*Options) Validate() error { return nil }

func verifyRun(ctx context.Context, o *Options) error {
	if err := o.Validate(); err != nil {
		return err
	}
	root, err := o.RepoRoot()
	if err != nil {
		return err
	}

	load := o.Load
	if load == nil {
		load = check.Load
	}
	// The version is this binary's, the string `fisc export` stamps, so the
	// graph verified is byte for byte the graph export would publish.
	subject, err := load(check.LoadOptions{
		Root:    root,
		Version: build.Get().String(),
		Full:    o.Full,
	})
	if err != nil {
		return err
	}

	checks := o.Checks
	if checks == nil {
		checks = check.All
	}
	rep := check.Run(ctx, subject, checks(), check.ReportOptions{
		GeneratedBy: "fisc " + build.Get().String(),
		Strict:      o.Strict,
	})
	if err := printReport(o.IO, rep, o.JSON); err != nil {
		return err
	}
	// The report prints first; ExitCodeError prints nothing, so no "error:"
	// line follows a report that already said what is wrong.
	if rep.Failed() {
		return &cmdutil.ExitCodeError{Code: exitReportNotClean}
	}
	return nil
}
