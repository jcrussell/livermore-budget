package check

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// What verify reads, all of it committed, and none of it a user's choice: verify
// checks the repository, and a verify pointed at a different fact store than the
// one `fisc build` writes would be checking something nobody reads. The paths are
// cmdutil's single declaration of the layout, not this package's.
const (
	factsFile   = cmdutil.FactsPath
	mappingsDir = cmdutil.MappingsDir
	dataDir     = cmdutil.DataDir
)

// spineScope is the scope of the projection this package checks: the one the site
// publishes, which internal/project declares and `fisc export` builds. It is a
// reference rather than a copy because a copy meant verify could check a graph the
// site does not publish and pass — and publishedProjectionBuilt now asserts the
// published triple was built at all, which no amount of shared spelling can.
const spineScope = project.PublishedScope

// Vocabulary is the view of the curated registries (internal/registry) the
// checks need, declared here in the consumer and kept to the methods actually
// used (byob-interfaces.2), as internal/project does with its Labels.
type Vocabulary interface {
	// Assignable reports whether a mapping rule may classify a fact as slug. It
	// is false both for a slug the taxonomy does not define and for a rollup
	// category that exists so a view can name a group.
	Assignable(slug string) bool
	// Category reports whether the taxonomy defines slug at all, which is what
	// tells those two cases apart: "you named a rollup" and "you typo'd a slug"
	// need different fixes.
	Category(slug string) (registry.Category, bool)
	// FundGroup reports whether name is a fund type data/funds.yaml uses.
	FundGroup(name string) bool
	// Fund is the registry entry for a fund number, which is the join key a
	// fact carries once a single-fund schedule is mapped (fisc-5gk.1).
	Fund(number int) (registry.Fund, bool)
	// Funds is every fund the registry lists. The checks read it to learn the
	// constraint tiers the file actually uses, rather than carrying a second
	// copy of that closed vocabulary.
	Funds() []registry.Fund
}

var _ Vocabulary = (*registry.Registry)(nil)

// graphBuilder is the projection a structural check can read: one that hands
// back the graph rather than its bytes.
//
// internal/project's Sankey.Graph is exported for exactly this, so verify can
// check the structure without parsing back the JSON it is trying to validate.
// A projection that does not offer it cannot be checked here, and [Load] says
// so rather than skipping it.
type graphBuilder interface {
	Name() string
	Graph(facts []fact.Fact, o project.Options) (*project.Graph, error)
}

// Projection is one built graph, with the options it was built under.
//
// The options are carried because they are the difference between a graph that
// means something and one that does not: a projection built over two fiscal
// years doubles every figure and still balances (see project.Options), so a
// check that reports on a graph has to be able to say which slice it was of.
type Projection struct {
	Name    string
	Options project.Options
	Graph   *project.Graph
}

// String names the projection the way a report should: the file stem plus the
// slice of the corpus it covers.
func (p Projection) String() string {
	return fmt.Sprintf("%s FY%d %s %s", p.Name, p.Options.FiscalYear, p.Options.Basis, p.Options.Scope)
}

// Subject is everything the checks read, loaded once.
//
// It is loaded once because several checks read the same fact store and the same
// projection, and because resolving a mapping part twice is how a build ends up
// corroborating a different read from the one it published (fisc-uv6). The
// resolvers below are the memoized ones for that reason.
type Subject struct {
	// Root is the repository root every path is relative to.
	Root string
	// Full says whether the inputs only `--full` supplies are available. It is
	// false in CI and in a plain clone. No check requires it yet; see Check.Full.
	Full bool

	// Facts is the committed fact store, in the order the file lists them —
	// deliberately not re-sorted, because whether it is already sorted is one of
	// the things being checked.
	Facts []fact.Fact
	// Files is every parsed rule file under mappings/.
	Files []*mapping.File
	// Vocabulary is data/funds.yaml and data/taxonomy.yaml, loaded and validated.
	// It is required: the checks that read it do so without a nil guard, because a
	// subject assembled without a vocabulary is a programming error and not a state
	// of the corpus.
	Vocabulary Vocabulary
	// Docs is the extraction manifest of each document the rule files name,
	// keyed by document id. Opening one reads its manifest and no page.
	Docs map[string]*corpus.Doc
	// Resolvers is one memoized resolver per rule file, keyed by the file's
	// path. Nothing in tier 1 needs them: they are here because the totals
	// reconciliation (fisc-1wr.2) and the structural sweep (fisc-1wr.5) both
	// resolve rules, and building them anywhere else would mean two runs of the
	// same read in one command.
	Resolvers map[string]*mapping.Resolver
	// Projections is every graph the facts support, one per (fiscal year, basis)
	// the fact store carries within spineScope, in that order.
	Projections []Projection
}

// LoadOptions says what to load and from where.
type LoadOptions struct {
	// Root is the repository root. Required.
	Root string
	// Version is the version line stamped into each projection's
	// metadata.generated_by. Required, because project.Options requires it: a
	// document that does not say which binary wrote it is a chart with no
	// caption. Pass build.Get().String(), which is also what `fisc export`
	// stamps, so a future drift check can compare the two byte for byte.
	Version string
	// Full loads the inputs only `--full` supplies. Nothing does yet, so today
	// this is carried through to Subject.Full and read by nobody.
	//
	// When the first Full check lands, this is the one place in the program
	// allowed to look at data/pdf/, and it is also where a missing data/pdf has
	// to be dealt with. It is NOT dealt with here now: --full with no PDFs
	// present currently means the checks run and find no file, rather than being
	// skipped or refused. Deciding which of those it should be is part of
	// building the first check that needs them (fisc-1wr.5), not something to
	// guess at while nothing does.
	Full bool
}

// Load reads the subject from a repository.
//
// Everything it opens is a committed artifact: the fact store, the rule files,
// the two curated registries, and each document's extraction manifest. It does
// not open a page, does not run the extractor, and does not read data/pdf/ —
// which is what makes "verify needs no PDFs and no Python" a property of the
// code rather than a claim in a README.
//
// A failure here is a failure of the harness and not of a check: verify cannot
// conclude anything about a fact store it could not parse, and reporting that as
// a failed check would put a corpus problem and a broken input in the same
// column of the report.
func Load(o LoadOptions) (*Subject, error) {
	if o.Root == "" {
		return nil, errors.New("check: the repository root is required")
	}
	if o.Version == "" {
		return nil, errors.New("check: a version is required for each projection's metadata.generated_by")
	}

	s := &Subject{
		Root:      o.Root,
		Full:      o.Full,
		Docs:      map[string]*corpus.Doc{},
		Resolvers: map[string]*mapping.Resolver{},
	}
	fsys := os.DirFS(o.Root)

	var err error
	if s.Facts, err = loadFacts(fsys); err != nil {
		return nil, err
	}
	reg, err := registry.Load(os.DirFS(filepath.Join(o.Root, dataDir)))
	if err != nil {
		return nil, fmt.Errorf("load the data registries: %w", err)
	}
	s.Vocabulary = reg

	if s.Files, err = mapping.LoadDir(fsys, mappingsDir); err != nil {
		return nil, err
	}
	if err = s.openDocs(o.Root); err != nil {
		return nil, err
	}
	if s.Projections, err = buildProjections(project.Registry(reg), s.Facts, o.Version); err != nil {
		return nil, err
	}
	return s, nil
}

// loadFacts reads the committed fact store through an fs.FS rooted at the
// repository, so the path cannot name anything outside it.
func loadFacts(fsys fs.FS) ([]fact.Fact, error) {
	f, err := fsys.Open(factsFile)
	if err != nil {
		return nil, cmdutil.Hintf(fmt.Errorf("read the fact store: %w", err),
			"run `fisc build` to generate %s", factsFile)
	}
	defer f.Close() //nolint:errcheck // read-only file; nothing to flush
	facts, err := fact.Read(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", factsFile, err)
	}
	if len(facts) == 0 {
		return nil, cmdutil.WithHint(fmt.Errorf("%s has no facts in it", factsFile),
			"run `fisc build` to regenerate the fact store")
	}
	return facts, nil
}

// openDocs opens the extraction of every document the rule files name, and pairs
// each rule file with a resolver over it.
//
// A document is opened once however many rule files map it. That is about the
// manifest and not about page caching — corpus.Doc reads a page on every call, and
// the memo that stops a part being resolved twice lives in the Resolver, one per
// rule file — but a manifest of this corpus is tens of thousands of artifact
// records, and re-reading and re-parsing it per rule file also means two objects
// answering for one extraction.
func (s *Subject) openDocs(root string) error {
	for _, f := range s.Files {
		doc, ok := s.Docs[f.DocID]
		if !ok {
			var err error
			if doc, err = corpus.OpenDoc(root, f.DocID); err != nil {
				return fmt.Errorf("%s: %w", f.Path, err)
			}
			s.Docs[f.DocID] = doc
		}
		r, err := mapping.NewResolver(doc, f)
		if err != nil {
			return err
		}
		s.Resolvers[f.Path] = r
	}
	return nil
}

// buildProjections builds every registered projection over every slice of the
// fact store that carries one.
//
// The slices are read off the facts rather than hard-coded: each distinct
// (fiscal year, basis) pair within spineScope gets its own graph, so mapping a
// revised column or a second budget year puts that graph under verify's checks
// without anyone remembering to add it here. What is NOT read off the facts is
// the scope, which selects the schedule and therefore the projection — see
// spineScope.
func buildProjections(ps []project.Projection, facts []fact.Fact, version string) ([]Projection, error) {
	slices := factSlices(facts, version)
	// Zero slices means zero projections, and a report over zero projections is
	// the failure mode this refusal exists to stop: ten of the checks below read
	// nothing but the graph, so they all go vacuous at once and the run exits 0.
	// Changing every fact's scope to a mistyped value did exactly that.
	if len(slices) == 0 {
		return nil, cmdutil.Hintf(
			fmt.Errorf("no fact is in scope %q, so there is nothing to project and nothing "+
				"to check", spineScope),
			"the scope on every fact comes from its mapping rule; `fisc verify` checks the "+
				"slice %q that `fisc export` publishes", spineScope)
	}

	out := make([]Projection, 0, len(ps)*len(slices))
	for _, p := range ps {
		g, ok := p.(graphBuilder)
		if !ok {
			return nil, fmt.Errorf("projection %q does not expose its graph, so verify "+
				"cannot check its structure without parsing back the JSON it is validating",
				p.Name())
		}
		for _, o := range slices {
			graph, err := g.Graph(facts, o)
			if err != nil {
				return nil, fmt.Errorf("build the %s projection for FY%d %s: %w",
					p.Name(), o.FiscalYear, o.Basis, err)
			}
			out = append(out, Projection{Name: p.Name(), Options: o, Graph: graph})
		}
	}
	return out, nil
}

// factSlices returns the projection options the fact store supports, ordered by
// fiscal year and then basis so two runs report in the same order.
func factSlices(facts []fact.Fact, version string) []project.Options {
	type key struct {
		year  int
		basis mapping.Basis
	}
	seen := map[key]bool{}
	for _, f := range facts {
		if f.Scope == spineScope {
			seen[key{f.FiscalYear, f.Basis}] = true
		}
	}
	keys := make([]key, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].year != keys[j].year {
			return keys[i].year < keys[j].year
		}
		return keys[i].basis < keys[j].basis
	})

	out := make([]project.Options, 0, len(keys))
	for _, k := range keys {
		out = append(out, project.Options{
			FiscalYear: k.year,
			Basis:      k.basis,
			Scope:      spineScope,
			Version:    version,
		})
	}
	return out
}
