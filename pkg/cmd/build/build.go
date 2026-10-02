// Package build implements `fisc build`, which resolves the curated mapping
// rules against the committed extraction and writes the fact store.
//
// It is the only writer of facts.jsonl, and the file it writes is a build
// product: every line is derivable from data/extracted/ plus mappings/, so a
// rebuild that differs is either a rule change or a bug, and CI can tell which
// by diffing. That is why the write is atomic and the order canonical — a
// half-written or differently-ordered file would make the diff meaningless,
// which would cost the project its cheapest real check.
package build

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// Defaults. Both paths are repository-relative so a build is reproducible from
// any working directory, which is what lets CI and a developer's shell produce
// byte-identical output.
const (
	defaultOutput   = cmdutil.FactsPath
	defaultMappings = cmdutil.MappingsDir
)

// factsPerm is the mode of the written fact store. It is world-readable
// because facts.jsonl is committed and published; nothing about it is secret.
const factsPerm fs.FileMode = 0o644

// Options carries the command's dependencies and its parsed flags.
type Options struct {
	IO       *iostreams.IOStreams
	RepoRoot func() (string, error)

	// Output and Mappings are both relative to the repository root.
	Output   string
	Mappings string

	JSON bool
}

// NewCmdBuild builds the `fisc build` command.
func NewCmdBuild(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{IO: f.IOStreams, RepoRoot: f.RepoRoot}

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Resolve the mapping rules into facts.jsonl",
		Long: `Resolve every mapping rule against the committed extraction and write the
fact store.

A fact is one figure the city printed, plus everything needed to find it
again: document, page, byte offset, and the token that was parsed. The rules
under mappings/ decide which rows of which pages become which facts; this
command applies them and writes the result in a canonical order, so a rebuild
diffs cleanly against the committed file.

The build fails closed. A locator that no longer resolves, a part whose value
count has changed, two rules claiming the same cell, and a column that does
not sum to the total the document itself prints are all errors, and nothing is
written when one occurs.

Where a rule names a total row, that total is checked — the document checking
our work rather than us checking our own. Most schedules print no total, so
every part that could not be checked is listed as UNCHECKED with its rule and
page. That list is not noise to be silenced: it is the inventory of published
figures resting on our arithmetic alone.`,
		Example: `  # Rebuild facts.jsonl from the committed rules
  fisc build

  # Try a work-in-progress rule directory without touching the fact store
  fisc build --mappings mappings/draft --output facts/draft.jsonl

  # Machine-readable report, for CI
  fisc build --json`,
		GroupID: "data",
		// A stray argument is a mistyped flag, not an input: this command
		// takes its paths from flags so it can state their defaults.
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}
			return buildRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Output, "output", "o", defaultOutput,
		"Write the facts here, relative to the repository root")
	cmd.Flags().StringVar(&opts.Mappings, "mappings", defaultMappings,
		"Read rule files from this directory, relative to the repository root")
	cmd.Flags().BoolVar(&opts.JSON, "json", false,
		"Emit the build report as JSON on stdout instead of a summary on stderr")

	return cmd
}

// Validate checks the flags before anything is opened or written, so a
// mistyped path fails as a usage error rather than part way through a build
// that has already replaced the fact store.
func (o *Options) Validate() error {
	for _, f := range []struct{ flag, value string }{
		{"--output", o.Output},
		{"--mappings", o.Mappings},
	} {
		if err := validateRepoPath(f.flag, f.value); err != nil {
			return err
		}
	}
	return nil
}

// validateRepoPath refuses a path that is absolute or that climbs out of the
// repository. Both flags are joined onto the repository root, so an absolute
// value would silently mean something other than what it says, and a `../..`
// would let a mistyped flag read or overwrite files outside the tree this
// command is supposed to be describing.
func validateRepoPath(flag, value string) error {
	switch {
	case value == "":
		return cmdutil.FlagErrorf("%s is empty", flag)
	case filepath.IsAbs(value):
		return cmdutil.FlagErrorf("%s %q is an absolute path, but it is resolved "+
			"against the repository root", flag, value)
	}
	clean := filepath.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return cmdutil.FlagErrorf("%s %q climbs above the repository root", flag, value)
	}
	return nil
}

func buildRun(o *Options) error {
	if err := o.Validate(); err != nil {
		return err
	}
	root, err := o.RepoRoot()
	if err != nil {
		return err
	}

	// LoadDir reads through an fs.FS rooted at the repository, so the rule
	// directory cannot name anything outside it however the flag was spelled.
	dir := filepath.ToSlash(filepath.Clean(o.Mappings))
	files, err := mapping.LoadDir(os.DirFS(root), dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cmdutil.Hintf(err,
				"--mappings is resolved against the repository root %q", root)
		}
		return err
	}

	facts, rep, err := resolve(root, files)
	if err != nil {
		return err
	}

	// An empty result is refused rather than written. A mistyped --mappings
	// resolves to a directory with no rules in it, and truncating the fact
	// store on the way to reporting success is the most expensive way this
	// command could fail.
	if len(facts) == 0 {
		return cmdutil.WithHint(
			fmt.Errorf("%d rule files in %q produced no facts", len(files), dir),
			"check that --mappings points at the rule files")
	}

	fact.Sort(facts)
	if err := fact.CheckUniqueIDs(facts); err != nil {
		return err
	}
	// Sortedness is checked rather than assumed: Sort and the published order
	// are two statements of the same contract, and this is where they meet.
	if err := fact.CheckSorted(facts); err != nil {
		return err
	}

	rep.Facts = len(facts)
	rep.Output = filepath.ToSlash(filepath.Clean(o.Output))
	if err := cmdutil.WriteFile(filepath.Join(root, o.Output), factsPerm, func(w io.Writer) error {
		return fact.Write(w, facts)
	}); err != nil {
		return err
	}
	return rep.print(o.IO, o.JSON)
}

// resolve turns every rule file into facts, reporting what it could and could
// not check along the way.
//
// The whole corpus is held in memory before anything is written, because the
// canonical order is global and the uniqueness check is too: a fact from the
// last rule file can sort before the first one's, and two rules in different
// files can claim the same cell. Streaming would trade a check the project
// depends on for memory this corpus does not need — the Budget Book's whole
// mapping is in the low tens of thousands of facts.
func resolve(root string, files []*mapping.File) ([]fact.Fact, *report, error) {
	rep := newReport()
	var facts []fact.Fact

	for _, f := range files {
		doc, err := corpus.OpenDoc(root, f.DocID)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		r, err := mapping.NewResolver(doc, f)
		if err != nil {
			return nil, nil, err
		}
		rep.RuleFiles++

		chains := mapping.Chains(f)
		inChain := map[string]bool{}
		for _, chain := range chains {
			for _, rule := range chain {
				inChain[rule.ID] = true
			}
		}

		for i := range f.Rules {
			rule := &f.Rules[i]
			rep.Rules++
			for j := range rule.Parts {
				p := &rule.Parts[j]
				rep.Parts++

				values, omissions, err := r.Values(rule, p)
				if err != nil {
					return nil, nil, err
				}
				got, err := fact.FromValues(f, rule, values)
				if err != nil {
					return nil, nil, err
				}
				facts = append(facts, got...)
				rep.addOmissions(rule, omissions)

				// A rule whose printed total spans its parts is checked once,
				// after every part has been read, rather than part by part; a
				// rule in a subtotal chain with no total_row is checked when
				// its chain is.
				if inChain[rule.ID] && rule.TotalRow == "" {
					continue
				}
				if !rule.TotalSpansParts {
					if err := rep.checkTotals(r, rule, p); err != nil {
						return nil, nil, err
					}
				}
			}
			if rule.TotalSpansParts {
				if err := rep.checkSpanningTotals(r, rule); err != nil {
					return nil, nil, err
				}
			}
		}

		// A printed figure is counted once however many chains compare it:
		// debt-service-principal and debt-service-interest read one block.
		type at struct{ page, offset int }
		seen := map[at]bool{}
		for _, chain := range chains {
			res, err := r.CheckSubtotals(chain)
			if err != nil {
				return nil, nil, err
			}
			for _, line := range res.Tied {
				if len(line) > 0 && !seen[at{line[0].Page, line[0].Offset}] {
					rep.SubtotalLinesTied++
				}
				for _, c := range line {
					if !seen[at{c.Page, c.Offset}] {
						seen[at{c.Page, c.Offset}] = true
						rep.SubtotalCellsTied++
					}
				}
			}
			for _, rule := range chain {
				if rule.TotalRow == "" {
					rep.PartsChecked += len(rule.Parts)
				}
			}
		}

		// Rollups run after every rule in the file has resolved: a total
		// covering several rules cannot be summed until all of them have
		// stated their own.
		if err := rep.checkRollups(r, f); err != nil {
			return nil, nil, err
		}
	}
	return facts, rep, nil
}
