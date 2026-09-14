package check

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	factsFile    = cmdutil.FactsPath
	mappingsDir  = cmdutil.MappingsDir
	dataDir      = cmdutil.DataDir
	extractedDir = cmdutil.ExtractedDir
	// sourcesFile is the source registry, composed from the two packages that
	// already name its parts rather than spelled a third time: the directory is
	// cmdutil's, the file name is the registry's.
	sourcesFile = dataDir + "/" + registry.SourcesFile
	// departmentsFile is named in findings rather than only read, because
	// "department %q is not a division departments.yaml lists" tells the reader
	// which file to open and the slug alone does not.
	departmentsFile = dataDir + "/" + registry.DepartmentsFile
	taxonomyFile    = dataDir + "/" + registry.TaxonomyFile
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
	// Categories is every taxonomy entry, ordered by slug. A check that could
	// only look a slug up could never say what the registry declares that no
	// fact prints, and an entry's children are not a field on it: the lines
	// nested under a category are found by reading the whole file.
	Categories() []registry.Category
	// FundGroup reports whether name is a fund type data/funds.yaml uses.
	FundGroup(name string) bool
	// Fund is the registry entry for a fund number, which is the join key a
	// fact carries once a single-fund schedule is mapped (fisc-5gk.1).
	Fund(number int) (registry.Fund, bool)
	// Division is the data/departments.yaml entry for the slug a fact's
	// `department` field holds.
	//
	// The two words disagree, and the registry's own type is where that is
	// explained: pp.167-170 print 23 mixed-case DIVISIONS under 11 ALL-CAPS
	// DEPARTMENTS, a fact names the division, and `department` is the field
	// name facts.jsonl has already published. The method is named for what it
	// returns rather than for the field it answers about, so a reader here is
	// told which tier resolves.
	Division(slug string) (registry.Division, bool)
	// Department reports whether slug is a department data/departments.yaml
	// lists — the ALL-CAPS tier above [Vocabulary.Division].
	//
	// The `department` field holds EITHER tier, which is the document's doing
	// rather than a relaxation: pp.167-170 and pp.85-125's upper block print a
	// division per row, and pp.85-125's Department Funding Sources block prints
	// one schedule PER DEPARTMENT with no division on it. Six of the eleven
	// departments are not division slugs, so a check that resolved only
	// divisions could not accept that schedule's natural axis at all.
	//
	// It is a predicate and not an accessor because the registry's department
	// type is unexported; what this package needs is whether the fact joins.
	Department(slug string) bool
	// Funds is every fund the registry lists. The checks read it to learn the
	// constraint tiers the file actually uses, rather than carrying a second
	// copy of that closed vocabulary.
	Funds() []registry.Fund
	// FundByLabel resolves a fund by a name the city PRINTS, exactly — a name
	// or a declared alias, never a prefix and never case-folded, with ambiguity
	// refused when data/funds.yaml loads.
	//
	// It is the only way to get from a printed heading to a number, which is
	// what rule-funds-match-their-headings needs: `fund:` is written by hand on
	// every column and nothing else derives it from the section the rule reads.
	FundByLabel(label string) (registry.Fund, error)
}

var _ Vocabulary = (*registry.Registry)(nil)

// graphBuilder is a projection whose document is a GRAPH, and which hands back
// the graph rather than its bytes.
//
// internal/project's Sankey.Graph is exported for exactly this, so verify can
// check the structure without parsing back the JSON it is trying to validate.
//
// NOT EVERY PROJECTION IS ONE. This interface used to be a requirement: a
// projection that did not satisfy it made [Load] return an error, so registering
// the first non-graph projection would have made `fisc verify` exit non-zero
// HAVING PRINTED NO REPORT — every finding in the run lost, which is the failure
// [ProjectionFailure] exists to have stopped happening. A trends document is a
// set of series and has no nodes and no links; it is not defective for that.
//
// So the graph checks now ask for [Subject.Graphs] and the rest read
// Projection.Options. What is still refused is a projection that exposes NO
// checkable structure at all — see buildProjections.
type graphBuilder interface {
	Name() string
	Graph(facts []fact.Fact, o project.Options) (*project.Graph, error)
}

// trendsBuilder is a projection whose document is a set of SERIES, and which
// hands back the document rather than its bytes.
//
// It is graphBuilder for the second document shape, declared for the same
// reason and satisfied by internal/project's Trends.Document: verify checks the
// structure without parsing back the JSON it is trying to validate.
//
// A projection satisfying NEITHER interface still builds and still reports --
// that is the whole of what fisc-744 changed -- but documentsAreChecked fails
// it, because a document no structural check reads is a document that can be
// wrong on the published site while verify prints all-green.
type trendsBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.TrendsDocument, error)
}

// fundFlowsBuilder is a projection whose document is a graph WITHOUT a headline.
//
// IT IS A THIRD INTERFACE RATHER THAN A THIRD IMPLEMENTATION OF graphBuilder,
// and the reason is not the missing key. project.Graph.Metadata is the spine's
// concrete Metadata type, whose headline block is required, so a drill-down
// returning *project.Graph would have to publish eight zeros -- absent-is-not-
// zero at document level, and the false green fisc-xau measured: a revenue-side
// drill-down has transfers IN and none out, so transfer_residual_cents would
// publish -21,045,597 and headline-transfer-residual would report it correct,
// the figure being the sum of the facts and the facts being one leg.
//
// The STRUCTURAL checks do not care which of the two shapes they are given --
// both carry nodes and links -- and read [Subject.Linked] instead. The three
// headline checks stay on [Subject.Graphs], which is the set that publishes one.
type fundFlowsBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.FundFlowsDocument, error)
}

// departmentSpendingBuilder is a projection whose document is the departmentwide
// cross-tab.
//
// A FOURTH INTERFACE AND NOT A SECOND USE OF fundFlowsBuilder, and the reason is
// what the shape means rather than what it holds. Both are nodes and links with
// no headline, so the STRUCTURAL checks read them identically through
// [Subject.LinkedDocuments] -- and the checks that are of the drill-down's shape
// ALONE would then be handed this one: drill-reconciles-across-documents indexes
// drill-downs by column and would see two documents of FY2026, and
// fund-flows-counts-reconcile would re-derive facts_cited_twice on a document
// that has no second grain. Sharing the type would make those two checks report
// on a document neither was written about.
type departmentSpendingBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.DepartmentSpendingDocument, error)
}

// departmentFundingBuilder is a projection whose document is pp.85-125's
// funding-source graph.
//
// A SIXTH INTERFACE FOR departmentSpendingBuilder's REASON, and the near miss
// here is the sharpest of the five. This document and the cross-tab are of the
// SAME ELEVEN PAGES and both are nodes and links with no headline, so sharing a
// type would be easy to argue for and wrong: they are two readings of one
// figure at two grains, and the checks that are of the drill-down's shape would
// then be handed a document built from the other block of the same pages.
type departmentFundingBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.DepartmentFundingDocument, error)
}

// transfersByFundBuilder is a projection whose document is p76's transfer
// network.
//
// A FIFTH INTERFACE FOR departmentSpendingBuilder's REASON, and the case here
// is sharper than that one's. This document is the only one in the project
// whose links come in PAIRS -- two per printed figure, one for each end of a
// movement -- so every count taken off it is twice what a document of cells
// would mean by the same number. fund-flows-counts-reconcile re-derives
// facts_cited_twice and drill-reconciles-across-documents indexes drill-downs by
// column; handed this shape, both would report on a document neither was
// written about.
type transfersByFundBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.TransfersByFundDocument, error)
}

// projection is one built graph, with the options it was built under.
//
// The options are carried because they are the difference between a graph that
// means something and one that does not: a projection built over two fiscal
// years doubles every figure and still balances (see project.Options), so a
// check that reports on a graph has to be able to say which slice it was of.
type projection struct {
	Name    string
	Options project.Options
	// Graph is the built graph, or nil for a projection whose document is not
	// one. Read it through [Subject.Graphs] rather than dereferencing it: a
	// check that means "every graph" and writes "every projection" is one
	// non-graph projection away from a nil panic inside a report.
	Graph *project.Graph
	// Trends is the built trends document, or nil for a projection that is not
	// one. Read it through [Subject.TrendDocuments], for Graph's reason.
	//
	// EXACTLY ONE OF Graph AND Trends IS NON-NIL on a healthy projection, and
	// [documentsAreChecked] is what asserts it: a projection carrying neither is
	// a document no structural check reads, which is the state that lets a wrong
	// document ship under an all-green verify.
	Trends *project.TrendsDocument
	// FundFlows is the built drill-down, or nil. Read it through
	// [Subject.LinkedDocuments] for the structural checks, and through
	// [Subject.FundFlowsDocuments] for the ones that are of this shape alone.
	//
	// EXACTLY ONE OF THE FOUR IS NON-NIL on a healthy projection.
	FundFlows *project.FundFlowsDocument
	// DepartmentSpending is the built cross-tab, or nil. Read it through
	// [Subject.LinkedDocuments] for the structural checks, and through
	// [Subject.DepartmentSpendingDocuments] for the one that is of this shape
	// alone.
	DepartmentSpending *project.DepartmentSpendingDocument
	// DepartmentFunding is the built funding-source graph, or nil. Read it
	// through [Subject.LinkedDocuments] for the structural checks; no check is
	// of this shape alone, because funding-sources-tie-to-spine reads the FACTS
	// and is what this scope's arithmetic rests on.
	DepartmentFunding *project.DepartmentFundingDocument
	// TransfersByFund is the built transfer network, or nil. Read it through
	// [Subject.LinkedDocuments] for the structural checks; no check is of this
	// shape alone, because transfers-detail-ties-to-spine reads the FACTS and
	// is what this scope's arithmetic rests on.
	TransfersByFund *project.TransfersByFundDocument
}

// linked is one document's nodes and links, whatever shape carried them.
//
// THE STRUCTURAL CHECKS ARE ABOUT A GRAPH AND NOT ABOUT A HEADLINE. Acyclicity,
// tier ordering, a link's value against its citation, a parent that resolves --
// every one of those claims is true of any document made of nodes and links, and
// none of them reads Metadata at all. Written against [Subject.Graphs], which is
// the set of documents that publish a HEADLINE, all six would have skipped the
// first headline-less document entirely while documents-are-checked reported it
// as a shape no check reads.
//
// WHAT STAYS ON Graphs IS THE THREE HEADLINE CHECKS, and that narrowing has a
// STRUCTURAL predicate rather than a value test: a document is in that set
// because its TYPE publishes a headline, never because the figures in one happen
// to be non-zero. A value test would go green over a spine whose headline had
// been zeroed, which publishedProjectionBuilt's doc comment calls worse than no
// coverage at all.
type linked struct {
	projection
	Nodes []project.Node
	Links []project.Link
}

// String names the projection the way a report should: the file stem plus the
// slice of the corpus it covers.
func (p projection) String() string {
	return fmt.Sprintf("%s %s %s", p.Name, project.Describe(p.Options.Columns), p.Options.ScopeList())
}

// projectionFailure is one slice a projection refused to build, with the
// refusal.
//
// It is RECORDED rather than returned because a projection that will not build
// is a claim about the corpus, and verify's whole job is to report those. Before
// this, internal/project's refusal of a department-bearing fact reached the
// operator as `exit 1` with no report at all — every other finding in the run
// lost, and the reason buried in a wrapped error — which is strictly worse than
// a red check saying the same thing.
//
// EXACTLY ONE THING STAYS A HARD ERROR: a fact store with no slice in the
// published scope. It is not a verdict about the corpus and there is no
// projection there that failed — there is nothing to project, so recording it
// would leave every graph check vacuous at once and the run exiting 0, which is
// the state this whole mechanism exists to avoid.
//
// A projection that exposes no graph is NOT that error, though it used to be,
// and that was the defect: see buildProjections, which records it with a nil
// document, and documentsAreChecked, which reports it with a report around it
// rather than by killing the run.
type projectionFailure struct {
	Name    string
	Options project.Options
	Err     error
}

// String names the failed slice the way Projection.String names a built one, so
// a report can list the two together.
func (f projectionFailure) String() string {
	return fmt.Sprintf("%s %s %s", f.Name, project.Describe(f.Options.Columns), f.Options.ScopeList())
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
	// false in CI and in a plain clone, and the checks that need them are skipped
	// rather than failed when it is; see Check.Full.
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
	// Extractions is every extraction committed under data/extracted/, keyed by
	// the directory name — which corpus.OpenDoc has already checked the manifest
	// agrees with.
	//
	// Docs is a subset of this, sharing the same values so that one object
	// answers for one extraction. They are separate fields because they are
	// different sets and the difference is what the structural checks are about:
	// Docs is what the rules read pages from, and Extractions is what the
	// repository commits. An extraction no rule maps still drifts, and an
	// extraction with no source registry entry is drift in itself — neither is
	// visible in Docs at all.
	Extractions map[string]*corpus.Doc
	// Sources is data/sources.yaml: the registry's independently recorded claim
	// about the bytes each extraction was made from.
	Sources []registry.Source
	// SourcePDFs is what Full found where each source document should be, keyed
	// by document id, and it is empty when Full is false. It carries the state of
	// each file rather than its bytes: a hash Load computed, or the reason there
	// was nothing to hash.
	SourcePDFs map[string]sourcePDF
	// Resolvers is one memoized resolver per rule file, keyed by the file's
	// path. They are shared because the totals reconciliation (fisc-1wr.2), the
	// structural sweep (fisc-1wr.5) and fact-offset-is-not-a-stated-total all
	// resolve rules, and building them anywhere else would mean several runs of
	// the same read in one command.
	Resolvers map[string]*mapping.Resolver
	// Projections is every graph the facts support, one per (fiscal year, basis)
	// the fact store carries within spineScope, in that order.
	Projections []projection
	// ProjectionFailures is every slice a projection refused to build. It is
	// empty on a healthy corpus, and projectionsBuild is the check that reports
	// it — a failure here silences every graph check at once, so it must not be
	// reachable only through a missing entry in Projections.
	ProjectionFailures []projectionFailure
	// Published is every document the site publishes, and the slice of the
	// fact store each must be built over.
	//
	// [Load] fills it from project.PublishedDocuments, which is the single
	// declaration `fisc export` also reads, so the two commands cannot disagree
	// about what the site serves. It is a FIELD rather than a direct call so
	// that a fixture can be a smaller repository than this one — a miniature of
	// a single year is not a repository that has lost a published document, and
	// reading the package declaration directly would make it look like one.
	//
	// IT WAS A YEAR LIST UNTIL 2026-08-26, and the widening is not cosmetic. A
	// list of years can only describe a site whose every document is a year of
	// one schedule; revenue-trends is one document of a different scope over
	// four columns and no year, so under the old field it was published and
	// unguarded. See project.PublishedDocument.
	Published []project.PublishedDocument
}

// graphs is every projection that produced a graph, which is what the
// structural checks are about.
//
// It exists so that a check meaning "every graph" cannot be written as "every
// projection" and be one non-graph projection away from dereferencing nil
// inside a report. Ranging over this instead is the whole discipline.
//
// A projection with no graph is NOT skipped coverage: it is a document of a
// different shape, checked by whatever check is about that shape. What would be
// a gap is a projection no check reads at all, and that is what
// projectionsBuild and facts-are-projected are for.
func (s *Subject) graphs() []projection {
	out := make([]projection, 0, len(s.Projections))
	for _, p := range s.Projections {
		if p.Graph != nil {
			out = append(out, p)
		}
	}
	return out
}

// linkedDocuments is every projection carrying nodes and links, whatever
// document shape carried them, in the order they were built.
//
// This is what the STRUCTURAL checks read. See [Linked] for why they cannot read
// [Subject.Graphs] and why the headline checks still do.
func (s *Subject) linkedDocuments() []linked {
	out := make([]linked, 0, len(s.Projections))
	for _, p := range s.Projections {
		switch {
		case p.Graph != nil:
			out = append(out, linked{projection: p, Nodes: p.Graph.Nodes, Links: p.Graph.Links})
		case p.FundFlows != nil:
			out = append(out, linked{projection: p,
				Nodes: p.FundFlows.Nodes, Links: p.FundFlows.Links})
		case p.DepartmentSpending != nil:
			out = append(out, linked{projection: p,
				Nodes: p.DepartmentSpending.Nodes, Links: p.DepartmentSpending.Links})
		case p.DepartmentFunding != nil:
			out = append(out, linked{projection: p,
				Nodes: p.DepartmentFunding.Nodes, Links: p.DepartmentFunding.Links})
		case p.TransfersByFund != nil:
			out = append(out, linked{projection: p,
				Nodes: p.TransfersByFund.Nodes, Links: p.TransfersByFund.Links})
		}
	}
	return out
}

// departmentSpendingDocuments is every projection that built the departmentwide
// cross-tab, for the check that is of that shape alone.
func (s *Subject) departmentSpendingDocuments() []projection {
	out := make([]projection, 0, len(s.Projections))
	for _, p := range s.Projections {
		if p.DepartmentSpending != nil {
			out = append(out, p)
		}
	}
	return out
}

// fundFlowsDocuments is every projection that built a drill-down, for the checks
// that are of that shape alone.
func (s *Subject) fundFlowsDocuments() []projection {
	out := make([]projection, 0, len(s.Projections))
	for _, p := range s.Projections {
		if p.FundFlows != nil {
			out = append(out, p)
		}
	}
	return out
}

// trendDocuments is every projection whose document is a set of series, with the
// options it was built under. It is Graphs for the other shape, and it exists
// for the same reason: a check that means "every trends document" must not have
// to write "every projection" and remember the nil.
func (s *Subject) trendDocuments() []projection {
	out := make([]projection, 0, len(s.Projections))
	for _, p := range s.Projections {
		if p.Trends != nil {
			out = append(out, p)
		}
	}
	return out
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
	// Full loads the inputs only `--full` supplies: the source documents under
	// data/pdf/, hashed into Subject.SourcePDFs.
	//
	// This is the one place in the program allowed to look at data/pdf/, which is
	// why the hashing happens here rather than in the check that compares the
	// results: the checks then read three recorded claims and compare them, the
	// same shape every other check in this package has.
	//
	// A source document that is absent, or that is an unsmudged Git LFS pointer
	// rather than a PDF, does NOT fail here. Load records what it found and the
	// check reports it: "the bytes are wrong" is a claim about the corpus and "the
	// bytes are not here" is a claim about the checkout, and collapsing the second
	// into a load failure would mean a plain clone could not tell them apart.
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
		Root:        o.Root,
		Full:        o.Full,
		Docs:        map[string]*corpus.Doc{},
		Extractions: map[string]*corpus.Doc{},
		SourcePDFs:  map[string]sourcePDF{},
		Resolvers:   map[string]*mapping.Resolver{},
	}
	fsys := os.DirFS(o.Root)
	dataFS := os.DirFS(filepath.Join(o.Root, dataDir))

	var err error
	if s.Facts, err = loadFacts(fsys); err != nil {
		return nil, err
	}
	reg, err := registry.Load(dataFS)
	if err != nil {
		return nil, fmt.Errorf("load the data registries: %w", err)
	}
	s.Vocabulary = reg
	if s.Sources, err = registry.LoadSources(dataFS); err != nil {
		return nil, fmt.Errorf("load the source registry: %w", err)
	}

	if s.Files, err = mapping.LoadDir(fsys, mappingsDir); err != nil {
		return nil, err
	}
	if err = s.openExtractions(o.Root); err != nil {
		return nil, err
	}
	if err = s.openDocs(o.Root); err != nil {
		return nil, err
	}
	if o.Full {
		if s.SourcePDFs, err = loadSourcePDFs(fsys, s.Sources); err != nil {
			return nil, err
		}
	}
	s.Published = project.PublishedDocuments()
	registry := project.Registry(reg)
	if s.Projections, s.ProjectionFailures, err = buildProjections(
		registry, s.Facts, o.Version); err != nil {
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
			// openExtractions has already opened everything data/extracted/ holds,
			// so this is a lookup and not a second read. It falls back to OpenDoc
			// for the case that lookup misses, which is a rule file naming a
			// document that has never been extracted: OpenDoc is what says so, and
			// says which directory it looked in.
			if doc, ok = s.Extractions[f.DocID]; !ok {
				var err error
				if doc, err = corpus.OpenDoc(root, f.DocID); err != nil {
					return fmt.Errorf("%s: %w", f.Path, err)
				}
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

// openExtractions opens every extraction committed under data/extracted/.
//
// It reads the directory rather than the rule files, which is the whole point:
// the structural checks are about what the repository commits, and two of the
// three documents are not mapped yet. An extraction nothing reads can still drift
// away from its manifest, and it stays drifted for as long as nobody maps it.
//
// A manifest this reader cannot understand fails here rather than becoming a
// finding. That is the same line internal/corpus draws for schema_version and
// internal/registry draws for its two files: at a schema version this fisc does
// not know, the artifact namespace itself may be different, so there is no
// verdict to reach about that extraction — only a statement that the checker
// cannot read it, which is a load failure by this package's own definition.
func (s *Subject) openExtractions(root string) error {
	dir := filepath.Join(root, filepath.FromSlash(extractedDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return cmdutil.Hintf(fmt.Errorf("read the extraction directory: %w", err),
			"%s holds one directory per document; run `make extract` to write it", extractedDir)
	}
	for _, e := range entries {
		// One directory per document. A stray file beside them is not an
		// extraction and is not swept: what an unlisted file could get read as is
		// a question about the inside of an extraction, which is where
		// artifacts-match-manifest asks it.
		if !e.IsDir() {
			continue
		}
		doc, err := corpus.OpenDoc(root, e.Name())
		if err != nil {
			return err
		}
		s.Extractions[e.Name()] = doc
	}
	return nil
}

// sourceState is what Load found where a source document should be. The three
// values are three different things to do about it, which is why one bool would
// not have done: fetch the bytes, restore the file, or look at what changed them.
type sourceState string

// The states a source document can be in.
const (
	// sourcePresent is a readable file, whose bytes Load hashed.
	sourcePresent sourceState = "present"
	// sourceMissing is no file at all.
	sourceMissing sourceState = "missing"
	// sourcePointer is an unsmudged Git LFS pointer: the file the registry names
	// is there, and it holds a 130-byte text stanza naming the bytes instead of
	// the bytes. This is the NORMAL state of data/pdf/ in a clone made without
	// git-lfs, and in CI, which sets GIT_LFS_SKIP_SMUDGE deliberately — so it
	// must never be reported as a corrupted document.
	sourcePointer sourceState = "pointer"
)

// sourcePDF is what Load found at one source document's path.
//
// It carries what was found rather than a verdict about it, because the verdict
// needs the registry and the manifest beside it and belongs in a check.
type sourcePDF struct {
	// Path is the file Load looked at, repository-relative, as the registry
	// spelled it.
	Path  string
	State sourceState
	// Bytes and SHA256 are the size and hash of the bytes on disk. They are set
	// only for SourcePresent; for the other two states there were no bytes to
	// hash, and a zero hash must not read as one that failed to match.
	Bytes  int64
	SHA256 string
	// PointerOID and PointerBytes are what an LFS pointer says the real file's
	// hash and size are, for SourcePointer only.
	//
	// They are recorded because they are evidence rather than noise: the pointer
	// is Git's own record of the same sha256 the registry and the manifest claim,
	// so a report can say whether `git lfs pull` would fetch the expected bytes
	// or whether the three records already disagree without the bytes present.
	PointerOID   string
	PointerBytes int64
}

// lfsPointerPrefix is the first line of a Git LFS pointer file, per the v1
// pointer spec, and it is the sentinel that tells an unfetched pointer from a
// PDF. A PDF begins "%PDF-", so there is no overlap to be careful about.
const lfsPointerPrefix = "version https://git-lfs.github.com/spec/v1\n"

// lfsPointerLimit is how much of a file is read before deciding it is not a
// pointer. The spec caps a pointer at "less than 200 bytes"; 1 KiB is generous
// enough to survive a future key without being enough of a PDF to matter.
const lfsPointerLimit = 1024

// loadSourcePDFs hashes the source documents the registry names.
//
// This is the only function in fisc that opens anything under data/pdf/, and it
// runs only under --full. Everything is read through an fs.FS rooted at the
// repository, so a `file:` in sources.yaml cannot address anything outside the
// tree however it is spelled.
//
// A file that is not there, and a file that is an unsmudged LFS pointer, are
// recorded and returned. Any other read failure is returned as an error: those
// are failures of the machine rather than states of the repository, and a check
// cannot conclude anything about bytes the filesystem would not hand over.
func loadSourcePDFs(fsys fs.FS, sources []registry.Source) (map[string]sourcePDF, error) {
	out := make(map[string]sourcePDF, len(sources))
	for _, s := range sources {
		got, err := readSourcePDF(fsys, s.File)
		if err != nil {
			return nil, fmt.Errorf("read the source document for %s: %w", s.ID, err)
		}
		out[s.ID] = got
	}
	return out, nil
}

// readSourcePDF classifies and, where there are bytes to hash, hashes one file.
func readSourcePDF(fsys fs.FS, name string) (sourcePDF, error) {
	got := sourcePDF{Path: name, State: sourceMissing}
	f, err := fsys.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return got, nil
	}
	if err != nil {
		return got, err
	}
	defer f.Close() //nolint:errcheck // read-only file; nothing to flush

	// The head is read once and then either parsed as a pointer or fed back into
	// the hash, so a 35 MB PDF is still read exactly once and never held whole in
	// memory.
	head := make([]byte, lfsPointerLimit)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return got, err
	}
	head = head[:n]
	if oid, size, ok := parseLFSPointer(head); ok {
		got.State, got.PointerOID, got.PointerBytes = sourcePointer, oid, size
		return got, nil
	}

	h := sha256.New()
	written, err := io.Copy(h, io.MultiReader(bytes.NewReader(head), f))
	if err != nil {
		return got, err
	}
	got.State, got.Bytes, got.SHA256 = sourcePresent, written, hex.EncodeToString(h.Sum(nil))
	return got, nil
}

// parseLFSPointer reads the oid and size out of a Git LFS pointer stanza, and
// reports whether the bytes are one at all.
//
// The oid is returned bare, without its "sha256:" scheme, so it can be compared
// against the hashes the registry and the manifests record. A pointer whose keys
// are unreadable is still a pointer — the sentinel line is what decides that —
// and reports empty values rather than being mistaken for a document.
func parseLFSPointer(b []byte) (oid string, size int64, ok bool) {
	if !bytes.HasPrefix(b, []byte(lfsPointerPrefix)) {
		return "", 0, false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "oid sha256:"):
			oid = strings.TrimPrefix(line, "oid sha256:")
		case strings.HasPrefix(line, "size "):
			if n, err := strconv.ParseInt(strings.TrimPrefix(line, "size "), 10, 64); err == nil {
				size = n
			}
		}
	}
	return oid, size, true
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
func buildProjections(ps []project.Projection, facts []fact.Fact, version string) (
	[]projection, []projectionFailure, error,
) {
	slices := factSlices(facts, version)
	// Zero slices means zero projections, and a report over zero projections is
	// the failure mode this refusal exists to stop: ten of the checks below read
	// nothing but the graph, so they all go vacuous at once and the run exits 0.
	// Changing every fact's scope to a mistyped value did exactly that.
	//
	// This one stays a HARD ERROR and does not become a recorded failure. There
	// is no projection here that failed — there is nothing to project, which is
	// a statement about the fact store rather than a verdict a check could
	// reach, and recording it would put the run back in the state where every
	// graph check is vacuous and the exit code is 0.
	if len(slices) == 0 {
		return nil, nil, cmdutil.Hintf(
			fmt.Errorf("no fact is in scope %q, so there is nothing to project and nothing "+
				"to check", spineScope),
			"the scope on every fact comes from its mapping rule; `fisc verify` checks the "+
				"slice %q that `fisc export` publishes", spineScope)
	}

	out := make([]projection, 0, len(ps))
	var failed []projectionFailure
	for _, p := range ps {
		// Each projection is built over the slices IT says it is of, not over
		// the cartesian product of every projection and every slice. The
		// product was correct while the Sankey was the only projection and is
		// wrong for the first one of a different schedule, which would be
		// handed a slice containing none of its facts and would refuse to
		// build -- a red check reporting a scheduling mistake as a corpus
		// defect.
		want := slices
		if sl, ok := p.(project.Sliced); ok {
			want = sl.Slices(facts, version)
		}

		// A projection that is not a graph is RECORDED WITH A NIL GRAPH, not
		// refused. Refusing was the old behaviour and it was the wrong failure
		// mode: it returned an error out of Load, so `fisc verify` exited
		// non-zero having printed no report at all, losing every finding in the
		// run to a projection that was merely of a different shape. A trends
		// document is a set of series with no nodes and no links, and it is not
		// defective for that.
		//
		// What must not happen instead is a document nothing checks shipping
		// quietly. That is documentsAreChecked's job, and it is a red check
		// with a report around it rather than a dead run.
		g, isGraph := p.(graphBuilder)
		t, isTrends := p.(trendsBuilder)
		ff, isFundFlows := p.(fundFlowsBuilder)
		ds, isSpending := p.(departmentSpendingBuilder)
		df, isFunding := p.(departmentFundingBuilder)
		tr, isTransfers := p.(transfersByFundBuilder)

		for _, o := range want {
			built := projection{Name: p.Name(), Options: o}
			var err error
			switch {
			case isGraph:
				built.Graph, err = g.Graph(facts, o)
			case isTrends:
				built.Trends, err = t.Document(facts, o)
			case isFundFlows:
				built.FundFlows, err = ff.Document(facts, o)
			case isSpending:
				built.DepartmentSpending, err = ds.Document(facts, o)
			case isFunding:
				built.DepartmentFunding, err = df.Document(facts, o)
			case isTransfers:
				built.TransfersByFund, err = tr.Document(facts, o)
			}
			if err != nil {
				// Recorded, not returned: see ProjectionFailure. The loop goes
				// on so that one bad slice does not hide a second one, and so
				// that the checks reading the slices that DID build still run.
				failed = append(failed, projectionFailure{Name: p.Name(), Options: o, Err: err})
				continue
			}
			// A projection satisfying neither interface is appended with both
			// document fields nil, which is not an error here and IS a finding
			// in documentsAreChecked. Refusing it at this level was the old
			// behaviour and it killed the whole run.
			out = append(out, built)
		}
	}
	return out, failed, nil
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
			Columns: []project.Column{{FiscalYear: k.year, Basis: k.basis}},
			Scopes:  []string{spineScope},
			Version: version,
		})
	}
	return out
}
