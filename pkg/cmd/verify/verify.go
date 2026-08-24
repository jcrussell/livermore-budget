// Package verify implements `fisc verify`, the command that makes this
// project's central claim checkable.
//
// Everything the site publishes is derived from committed artifacts, and this
// command is where that derivation is tested: the fact store's order and
// identity, the vocabulary every classification resolves in, and the arithmetic
// of the graph the site draws. It reads only committed files — no source PDF, no
// Python, no network — so CI can run it on a plain clone.
//
// Its exit code is the contract, and the line it draws is whether a report was
// produced at all: 0 when the report is clean, 3 when it is not — a check failed,
// or a check could not reach a verdict — and 1 when there is no report, because
// the corpus could not be loaded. 2 is a flag this command did not understand.
//
// 3 covering both a failure and an error is deliberate: both mean "read the
// report", and the report says which happened, with a status per check. 1 is
// reserved for the case where reading the report is not an option.
//
// A run that has nothing to check is NOT a pass, and this command says so on its
// own output while still exiting 0. `--strict` turns an UNDECLARED vacancy into a
// failure; a vacancy named in internal/check's declaredVacuous, with the bead that
// retires it, is reported and tolerated. The converse is not gated on the flag: a
// declaration whose check has stopped being vacuous fails EVERY run, because it is
// a false statement in this repository's own source. See internal/check for why
// --strict is not the default.
package verify

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/build"
	"github.com/jcrussell/livermore-budget/internal/check"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// exitReportNotClean is the exit code for a run whose report is not clean: a
// check failed, a check could not reach a verdict, or --strict was given and a
// check had nothing to check. It is distinct from 1, which means there is no
// report to read at all, so a script can tell "the corpus is wrong" from "fisc
// could not look at it".
//
// It is a fixed 3 and deliberately not the number of failed checks, which
// cmdutil.ExitCodeError's doc comment offers as a possibility: an exit code is
// one byte, so a run with exactly 256 failures would exit 0 and a CI job would
// go green on the worst result this command can produce. The count is in the
// report, where it cannot wrap.
const exitReportNotClean = 3

// Loader builds the subject the checks read. Nil means [check.Load].
//
// This is the seam a test uses to hand the checks a corpus it constructed,
// rather than standing up a repository on disk — the same shape `fisc export`
// uses for its Builder, and defaulted in the run function for the same reason:
// the default reaches the filesystem, and a command constructed for `--help`
// must not.
type Loader func(check.LoadOptions) (*check.Subject, error)

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
	Load Loader
	// Checks are the checks to run. Nil means check.All.
	//
	// This is a seam for the same reason Load is one. What this command owns is the
	// mapping from a mix of verdicts to an exit code and two output shapes, and the
	// committed corpus deliberately produces only three of the five verdicts — a
	// failure, an error and a skip are all states the report has to render and
	// score correctly before any of them can be produced by the data. Injecting the
	// set is how that mapping is tested without a corpus contrived to break.
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
		// A stray argument is a mistyped flag. Selecting individual checks is
		// not a feature yet, and silently ignoring the name of one would be
		// worse than refusing it.
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
// is opened (byob-input-validation.5).
//
// There is nothing to reject today: this command takes no paths, and its three
// flags are independent booleans, each meaningful in every combination. The
// method exists so that the first flag which does need rejecting has one
// obvious place to be rejected in, and so this command has the shape every
// other one has rather than validation inlined into its run function.
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
	// The version is stamped into each projection's metadata.generated_by. It is
	// this binary's, not a placeholder: `fisc export` stamps the same string, so
	// the graph verify checks is byte-for-byte the graph export would publish —
	// which is what the artifact drift check in fisc-1wr.5 will compare.
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
	// The report is printed first and the exit code carries the verdict, so a
	// failing run still tells the reader everything it found. ExitCodeError
	// prints nothing itself, which is what keeps the runner from appending an
	// "error:" line to a report that has already said what is wrong.
	if rep.Failed() {
		return &cmdutil.ExitCodeError{Code: exitReportNotClean}
	}
	return nil
}
