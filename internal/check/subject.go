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
	"slices"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/repo"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/schema"
)

// What verify reads, all of it committed and none of it a user's choice.
const (
	factsFile       = repo.FactsPath
	mappingsDir     = repo.MappingsDir
	dataDir         = repo.DataDir
	extractedDir    = repo.ExtractedDir
	sourcesFile     = dataDir + "/" + registry.SourcesFile
	departmentsFile = dataDir + "/" + registry.DepartmentsFile
	taxonomyFile    = dataDir + "/" + registry.TaxonomyFile
)

// spineScope is the scope of the projection the site publishes.
const spineScope = project.PublishedScope

// Vocabulary is the view of the curated registries (internal/registry) the
// checks need, declared here in the consumer and kept to the methods actually
// used (byob-interfaces.2), as internal/project does with its Labels.
type Vocabulary interface {
	// Assignable reports whether a mapping rule may classify a fact as slug. It
	// is false both for a slug the taxonomy does not define and for a rollup
	// category that exists so a view can name a group.
	Assignable(slug string) bool
	// Category reports whether the taxonomy defines slug at all, telling a
	// rollup from a typo.
	Category(slug string) (registry.Category, bool)
	// Categories is every taxonomy entry, ordered by slug.
	Categories() []registry.Category
	// FundGroup reports whether name is a fund type data/funds.yaml uses.
	FundGroup(name string) bool
	// Fund is the registry entry for a fund number.
	Fund(number int) (registry.Fund, bool)
	// Division is the data/departments.yaml entry for the slug a fact's
	// `department` field holds: the field is named department, and names the
	// division tier.
	Division(slug string) (registry.Division, bool)
	// Department reports whether slug is a department data/departments.yaml
	// lists — the tier above [Vocabulary.Division]. A fact's `department` holds
	// either tier, because pp.85-125 print departments with no division.
	Department(slug string) bool
	// Funds is every fund the registry lists.
	Funds() []registry.Fund
	// FundByLabel resolves a fund by a name the city prints, exactly: a name
	// or a declared alias, never a prefix, never case-folded.
	FundByLabel(label string) (registry.Fund, error)
}

var _ Vocabulary = (*registry.Registry)(nil)

// graphBuilder is a projection whose document is a GRAPH, and which hands back
// the document rather than its bytes, so verify checks the structure without
// parsing back the JSON it is trying to validate. A projection satisfying no
// builder interface still builds; documentsAreChecked fails it.
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

type debtBuilder interface {
	Name() string
	Document(facts []fact.Fact, o project.Options) (*project.DebtServiceDocument, error)
}

// projection is one built document, with the options it was built under, so a
// report can say which slice it was of.
type projection struct {
	Name    string
	Options project.Options
	// Graph is the built graph, or nil for a projection whose document is not
	// one; read it through [Subject.linkedDocuments]. Exactly one document
	// field is non-nil on a healthy projection, which [documentsAreChecked]
	// asserts.
	Graph *project.Document
	// Trends is the built trends document, or nil for a projection that is not
	// one. Read it through [Subject.trendDocuments], for Graph's reason.
	Trends *project.TrendsDocument
	// Debt is the built debt-service document, or nil for a projection that
	// is not one. debt-service-ties-to-facts reads it.
	Debt *project.DebtServiceDocument
}

// linked is one graph document's nodes and links, which the structural checks
// read; the headline checks read [Subject.spine] instead.
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
// refusal, recorded rather than returned so the run still reports. The one
// hard error is a fact store with nothing in the published scope.
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

// Subject is everything the checks read, loaded once, so a mapping part is
// resolved once and corroborated against the read that was published.
type Subject struct {
	// Root is the repository root every path is relative to.
	Root string
	// Full says whether the inputs only `--full` supplies are available; see
	// Check.Full.
	Full bool

	// Facts is the committed fact store in file order, not re-sorted, since
	// its order is checked.
	Facts []fact.Fact
	// Files is every parsed rule file under mappings/.
	Files []*mapping.File
	// Vocabulary is data/funds.yaml and data/taxonomy.yaml, loaded and
	// validated. Required; the checks read it without a nil guard.
	Vocabulary Vocabulary
	// Docs is the extraction manifest of each document the rule files name,
	// keyed by document id. Opening one reads its manifest and no page.
	Docs map[string]*corpus.Doc
	// Extractions is every extraction committed under data/extracted/, keyed by
	// directory name. Docs is a subset sharing the same values; the structural
	// checks read this, since an unmapped extraction still drifts.
	Extractions map[string]*corpus.Doc
	// Sources is data/sources.yaml: the registry's independently recorded claim
	// about the bytes each extraction was made from.
	Sources []registry.Source
	// SourcePDFs is what Full found where each source document should be, keyed
	// by document id; empty when Full is false.
	SourcePDFs map[string]sourcePDF
	// Resolvers is one memoized resolver per rule file, keyed by the file's
	// path. Every entry of Files has one or Load fails, so a check indexes it
	// directly.
	Resolvers map[string]*mapping.Resolver
	// Projections is every graph the facts support, one per (fiscal year, basis)
	// the fact store carries within spineScope, in that order.
	Projections []projection
	// ProjectionFailures is every slice a projection refused to build, which
	// projectionsBuild reports.
	ProjectionFailures []projectionFailure
	// Published is every document the site publishes, and the slice each must
	// be built over. [Load] fills it from project.PublishedDocuments; a field
	// so a fixture can be a smaller repository.
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
// It is selected by the published declaration, not by what the document
// carries: a spine document with no headline is a finding, not dropped from
// the set.
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

// trendDocuments is every projection whose document is a set of series.
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
	// metadata.generated_by. Required. Pass build.Get().String(), as `fisc
	// export` stamps.
	Version string
	// Full loads the source documents under data/pdf/, hashed into
	// Subject.SourcePDFs. An absent file or an unsmudged LFS pointer is
	// recorded, not a load failure.
	Full bool
}

// Load reads the subject from a repository. It opens only committed
// artifacts, and data/pdf/ only under Full. A failure here is the harness's,
// not a check's.
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
		return nil, hint.Withf(fmt.Errorf("read the fact store: %w", err),
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
		return nil, hint.With(fmt.Errorf("%s: %w", factsFile, err),
			"the fact store does not match schema/fact.schema.json, so no check over it "+
				"would mean anything; run `fisc build` to regenerate it")
	}

	facts, err := fact.Read(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", factsFile, err)
	}
	if len(facts) == 0 {
		return nil, hint.With(fmt.Errorf("%s has no facts in it", factsFile),
			"run `fisc build` to regenerate the fact store")
	}
	return facts, nil
}

// openDocs opens the extraction of every document the rule files name, and pairs
// each rule file with a resolver over it. A document is opened once however
// many rule files map it, so one object answers for one extraction.
func (s *Subject) openDocs(root string) error {
	for _, f := range s.Files {
		doc, ok := s.Docs[f.DocID]
		if !ok {
			// A lookup; OpenDoc is the fallback that reports a document never
			// extracted.
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

// openExtractions opens every extraction committed under data/extracted/,
// read off the directory rather than the rule files. A manifest this reader
// cannot understand is a load failure, not a finding.
func (s *Subject) openExtractions(root string) error {
	dir := filepath.Join(root, filepath.FromSlash(extractedDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return hint.Withf(fmt.Errorf("read the extraction directory: %w", err),
			"%s holds one directory per document; run `make extract` to write it", extractedDir)
	}
	for _, e := range entries {
		// One directory per document; a stray file beside them is not swept.
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

// sourceState is what Load found where a source document should be.
type sourceState string

// The states a source document can be in.
const (
	// sourcePresent is a readable file, whose bytes Load hashed.
	sourcePresent sourceState = "present"
	// sourceMissing is no file at all.
	sourceMissing sourceState = "missing"
	// sourcePointer is an unsmudged Git LFS pointer, the normal state in CI;
	// never reported as a corrupted document.
	sourcePointer sourceState = "pointer"
)

// sourcePDF is what Load found at one source document's path, not a verdict.
type sourcePDF struct {
	// Path is the file Load looked at, repository-relative, as the registry
	// spelled it.
	Path  string
	State sourceState
	// Bytes and SHA256 are set only for sourcePresent.
	Bytes  int64
	SHA256 string
	// PointerOID and PointerBytes are what an LFS pointer says the real file's
	// hash and size are, for sourcePointer only: Git's own record, comparable
	// to the other two without the bytes.
	PointerOID   string
	PointerBytes int64
}

// lfsPointerPrefix is the first line of a Git LFS v1 pointer file.
const lfsPointerPrefix = "version https://git-lfs.github.com/spec/v1\n"

// lfsPointerLimit is how much of a file is read before deciding it is not a
// pointer, which the spec caps under 200 bytes.
const lfsPointerLimit = 1024

// loadSourcePDFs hashes the source documents the registry names: the only
// function in fisc that opens data/pdf/, through an fs.FS rooted at the
// repository. A missing file or LFS pointer is recorded; any other read
// failure is an error.
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

	// The head is parsed as a pointer or fed back into the hash, so the file is
	// read once and never held whole.
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
// reports whether the bytes are one at all. The oid is returned without its
// "sha256:" scheme. The sentinel line alone decides; unreadable keys report
// empty values.
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

// buildProjections builds every registered projection over the slices it
// declares. Each projection's Slices reads them off the facts rather than a
// list, so mapping a revised column or a second budget year puts that graph
// under verify's checks without anyone adding it here.
func buildProjections(ps []project.Projection, facts []fact.Fact, version string) (
	[]projection, []projectionFailure, error,
) {
	// No fact in the spine's scope means no spine: every graph check would go
	// vacuous at once and the run exit 0, as a mistyped scope on every fact
	// does. So this stays a hard error rather than a recorded failure.
	if !slices.ContainsFunc(facts, func(f fact.Fact) bool { return f.Scope == spineScope }) {
		return nil, nil, hint.Withf(
			fmt.Errorf("no fact is in scope %q, so there is nothing to project and nothing "+
				"to check", spineScope),
			"the scope on every fact comes from its mapping rule; `fisc verify` checks the "+
				"slice %q that `fisc export` publishes", spineScope)
	}

	out := make([]projection, 0, len(ps))
	var failed []projectionFailure
	for _, p := range ps {
		want := p.Slices(facts, version)

		g, isGraph := p.(graphBuilder)
		t, isTrends := p.(trendsBuilder)
		d, isDebt := p.(debtBuilder)

		for _, o := range want {
			built := projection{Name: p.Name(), Options: o}
			var err error
			switch {
			case isGraph:
				built.Graph, err = g.Document(facts, o)
			case isTrends:
				built.Trends, err = t.Document(facts, o)
			case isDebt:
				built.Debt, err = d.Document(facts, o)
			}
			if err != nil {
				// Recorded, not returned, so one bad slice hides no other.
				failed = append(failed, projectionFailure{Name: p.Name(), Options: o, Err: err})
				continue
			}
			// A projection satisfying no interface is appended with every
			// document nil; documentsAreChecked fails it.
			out = append(out, built)
		}
	}
	return out, failed, nil
}
