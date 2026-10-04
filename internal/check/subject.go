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
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
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
	// Categories is every taxonomy entry, ordered by slug. An entry's children
	// are not a field on it; they are found by reading them all.
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
	// A fact's `department` holds EITHER tier because the documents do:
	// pp.85-125's Department Funding Sources block prints one schedule per
	// department with no division on it.
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
// the document rather than its bytes, so verify checks the structure without
// parsing back the JSON it is trying to validate.
//
// NOT EVERY PROJECTION IS ONE. A trends document is a set of series with no
// nodes and no links, and it is not defective for that; [trendsBuilder] is
// its shape. A projection satisfying neither still builds and still reports
// -- refusing it made Load return an error and `fisc verify` exit non-zero
// having printed no report -- and documentsAreChecked fails it, because a
// document no structural check reads can be wrong on the published site while
// verify prints all-green.
type graphBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.Document, error)
}

// trendsBuilder is a projection whose document is a set of SERIES, and which
// hands back the document rather than its bytes, for graphBuilder's reason.
type trendsBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.TrendsDocument, error)
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
	// one. Read it through [Subject.linkedDocuments] rather than dereferencing
	// it: a check that means "every graph" and writes "every projection" is
	// one non-graph projection away from a nil panic inside a report.
	//
	// EXACTLY ONE document field IS NON-NIL on a healthy projection, and
	// [documentsAreChecked] is what asserts it: a projection carrying none is
	// a document no structural check reads, which is the state that lets a wrong
	// document ship under an all-green verify.
	Graph *project.Document
	// Trends is the built trends document, or nil for a projection that is not
	// one. Read it through [Subject.trendDocuments], for Graph's reason.
	Trends *project.TrendsDocument
}

// linked is one graph document's nodes and links.
//
// THE STRUCTURAL CHECKS ARE ABOUT A GRAPH AND NOT ABOUT A HEADLINE. Acyclicity,
// tier ordering, a link's value against its citation, a parent that resolves --
// every one of those claims is true of any document made of nodes and links.
// The three headline checks read [Subject.spine], the documents the site
// publishes as the spine, and that narrowing is a DECLARATION rather than a
// value test: a document is in that set because Published names it, never
// because it carries a headline block or because the figures in one happen to
// be non-zero.
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

// slice is the facts p was built over: project.SelectFacts under p's own
// Options, refused by the document's name when the selection cannot be made.
func (p projection) slice(facts []fact.Fact) ([]fact.Fact, error) {
	out, err := project.SelectFacts(facts, p.Options)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return out, nil
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
	//
	// EVERY ENTRY OF Files HAS ONE, OR Load FAILS: openDocs builds a resolver
	// for each file in turn and returns the first error. So a lookup keyed by a
	// File's Path cannot miss, and a check indexes the map directly rather than
	// guarding a case no Subject can present.
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
	// BalanceExceptions is every balance the documents print apart from an
	// identity. Both balance checks hold the store to it and report an entry
	// stale where it matches no balance. [Load] fills it from
	// structure.BalanceExceptions; a subject built by hand carries only what
	// its builder sets, and nil means none.
	BalanceExceptions []structure.BalanceException
	// Residue is every set of facts the documents put outside every cut.
	// cuts-tie-along-the-lattice holds the store to it and reports an entry
	// stale where it matches no fact. [Load] fills it from
	// structure.BudgetBookResidue; a subject built by hand carries only what
	// its builder sets, and nil means none.
	Residue []structure.Residue
	// Exceptions is every cell the documents print apart from the cut it should
	// decompose, pinned on both sides to a printed residual. The lattice check,
	// the peer check and revenue-lines-tie-to-their-categories read it. [Load]
	// fills it from structure.BudgetBookExceptions; a subject built by hand
	// carries only what its builder sets, and nil means none.
	Exceptions []structure.Exception
	// Splits is every cut the documents print as the sum of parts rather than
	// against a coarser cut. [Load] fills it from structure.BudgetBookSplits;
	// nil means none.
	Splits []structure.Split
	// Restatements is every residue the documents print as a restatement of a
	// cut outside the reference, held to it line by line against Residue.
	// [Load] fills it from structure.BudgetBookRestatements; nil means none.
	Restatements []structure.Restatement
	// Identities is every cell two cuts at one level both print, with the
	// reading each takes. [Load] fills it from structure.BudgetBookIdentities;
	// nil means none.
	Identities []structure.Identity
	// UncheckedDocuments is every projection shape no structural check reads
	// yet, by projection name. [Load] fills it from uncheckedDocuments; nil
	// means none.
	UncheckedDocuments map[string]string
	// IncompleteSeries is every trends series the documents legitimately leave
	// short a column, by series id. [Load] fills it from incompleteSeries; nil
	// means none.
	IncompleteSeries map[string]string
	// Vacancies is every check declared to have nothing to look at over this
	// corpus, by check id. [Run] settles each against the verdict its check
	// reached. [Load] fills it from declaredVacuous; nil means none.
	Vacancies map[string]vacancy
}

// spine is the citywide spine as `fisc export` publishes it: for every column
// Published declares of project.PublishedProjection, the graph built over it.
// The three headline checks read it; everything structural reads
// [Subject.linkedDocuments].
//
// IT IS SELECTED BY THE PUBLISHED DECLARATION AND NOT BY WHAT THE DOCUMENT
// CARRIES. A spine document whose metadata carries no headline is returned as
// a finding, one per such document, because each headline check reads a figure
// off it and none can be examined; selecting on the pointer instead would drop
// that document out of the set, and the checks would go on passing over the
// spine documents that remain. A published column nothing built is
// published-projection-built's finding and is not repeated here. Each built
// document is returned once, whichever of the published columns it covers.
func (s *Subject) spine() (docs []projection, findings []Finding) {
	for _, p := range s.Projections {
		if p.Graph == nil || !s.publishesSpine(p) {
			continue
		}
		if p.Graph.Metadata.Headline == nil {
			findings = append(findings, finding(p.String(),
				"`fisc export` publishes this document as the spine and its metadata "+
					"carries no headline, so the page shows no figure and nothing here "+
					"ties one to the facts"))
			continue
		}
		docs = append(docs, p)
	}
	return docs, findings
}

// publishesSpine reports whether p is the spine projection built over a
// column Published declares for it. The match is project.MissingColumns, the
// same comparison published-projection-built and `fisc export` make, so one
// spelling decides what the site serves and what these checks read.
func (s *Subject) publishesSpine(p projection) bool {
	for _, d := range s.Published {
		if d.Projection == project.PublishedProjection && p.Name == d.Projection &&
			len(project.MissingColumns(d, p.Options)) == 0 {
			return true
		}
	}
	return false
}

// linkedDocuments is every projection carrying nodes and links, in the order
// they were built.
func (s *Subject) linkedDocuments() []linked {
	out := make([]linked, 0, len(s.Projections))
	for _, p := range s.Projections {
		if p.Graph != nil {
			out = append(out, linked{projection: p, Nodes: p.Graph.Nodes, Links: p.Graph.Links})
		}
	}
	return out
}

// documentsNamed is every built graph of one projection, for a check that is
// of that schedule's shape alone.
func (s *Subject) documentsNamed(name string) []projection {
	out := make([]projection, 0, len(s.Projections))
	for _, p := range s.Projections {
		if p.Graph != nil && p.Name == name {
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
	s.BalanceExceptions = structure.BalanceExceptions()
	s.Residue = structure.BudgetBookResidue()
	s.Exceptions = structure.BudgetBookExceptions()
	s.Splits = structure.BudgetBookSplits()
	s.Restatements = structure.BudgetBookRestatements()
	s.Identities = structure.BudgetBookIdentities()
	s.UncheckedDocuments = uncheckedDocuments
	s.IncompleteSeries = incompleteSeries
	s.Vacancies = declaredVacuous
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

	// Shape before semantics, and over the RAW file: a struct has already lost
	// the difference between a key that was absent and one present and empty,
	// which is what "absent is not zero" rests on.
	//
	// Why, measured: docs/schema-contracts.md.
	raw, err := fs.ReadFile(fsys, factsFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", factsFile, err)
	}
	if err = schema.ValidateJSONL(bytes.NewReader(raw), schema.Fact); err != nil {
		return nil, cmdutil.WithHint(fmt.Errorf("%s: %w", factsFile, err),
			"the fact store does not match schema/fact.schema.json, so no check over it "+
				"would mean anything; run `fisc build` to regenerate it")
	}

	facts, err := fact.Read(bytes.NewReader(raw))
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

		for _, o := range want {
			built := projection{Name: p.Name(), Options: o}
			var err error
			switch {
			case isGraph:
				built.Graph, err = g.Document(facts, o)
			case isTrends:
				built.Trends, err = t.Document(facts, o)
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
