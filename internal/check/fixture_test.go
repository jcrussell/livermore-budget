package check

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// The fixture is a miniature of the citywide spine: two fund groups, one row of
// each kind, and one cell of each shape the projection treats specially — a zero
// that earns no link, the two stock rows, and a change in working capital of each
// sign so both inferred nodes exist.
//
// It is hand-made rather than read from the committed corpus so that these tests
// state what each check should conclude over a shape a reader can count. The
// committed corpus gets its own test (TestTheCommittedCorpusVacuitySplit), which
// is where the claim "these checks are non-vacuous today" is pinned.
const (
	testDoc     = "livermore-budget-fy2026-2027"
	testScope   = "all-funds-gross"
	testYear    = 2026
	testBasis   = mapping.BasisAdopted
	testVersion = "fisc test (fixture)"
)

const testFundsYAML = `schema_version: 1
funds:
  - number: 100
    name: General Fund
    type: general
    constraint_tier: discretionary
    restriction_note: "Fixture: the general operating fund."
  - number: 500
    name: Water Utility Fund
    type: enterprise
    constraint_tier: restricted-by-law
    restriction_note: "Fixture: utility revenues are restricted to utility purposes."
  - number: 700
    name: Information Technology Fund
    type: internal-service
    constraint_tier: restricted-by-law
    restriction_note: "Fixture: departmental charges, so that a fixture can carry an internal-service column."
`

// The taxonomy carries the two rollups the vocabulary check has to tell apart
// from a typo: `taxes` and `fund-balance` exist so a view can group and may never
// be written as a fact's category.
const testTaxonomyYAML = `schema_version: 1
categories:
  - slug: taxes
    label: Taxes
    kinds: [revenue]
    assignable: false
  - slug: taxes/property
    label: Property Taxes
    parent: taxes
    kinds: [revenue]
  - slug: charges-for-services
    label: Charges for Services
    kinds: [revenue]
  - slug: wages-and-benefits
    label: Wages & Benefits
    kinds: [expenditure]
  - slug: transfers
    label: Transfers
    kinds: [transfer_in, transfer_out]
    assignable: false
  - slug: transfers/in
    label: Transfers In
    parent: transfers
    kinds: [transfer_in]
  - slug: transfers/out
    label: Transfers Out
    parent: transfers
    kinds: [transfer_out]
  - slug: fund-balance
    label: Fund Balance
    kinds: [fund_balance]
    assignable: false
  - slug: fund-balance/beginning
    label: Beginning Working Capital
    parent: fund-balance
    kinds: [fund_balance]
  - slug: fund-balance/ending
    label: Ending Working Capital
    parent: fund-balance
    kinds: [fund_balance]
  - slug: fund-balance/change
    label: Change in Working Capital
    parent: fund-balance
    kinds: [fund_balance]
`

// testCell is one printed cell of the fixture schedule.
type testCell struct {
	kind     mapping.Kind
	category string
	group    string
	cents    int64
}

// fixtureCells is the whole fixture: 12 cells, of which 7 earn a link. Four are
// the stock rows and one is a zero the document printed, which is what makes the
// counts identity — cited + stock + zero = facts — worth asserting.
//
// THE FUND BALANCES SATISFY beginning + change == ending, AND THEY DID NOT USED
// TO. This fixture published general as 500,000 beginning, (20,000) change and
// 510,000 ending — a balance $30,000 short of its own arithmetic — and gave
// enterprise a change with no beginning or ending at all. No document could
// print either shape. Nothing noticed until fund-balance-identity landed and
// went red on both, which is the check earning its place before it ever ran
// against the real corpus: a fixture describing a corpus that cannot exist is
// the substrate problem testdata/README.md is about, one layer in.
//
// So general is now 500,000 + (20,000) = 480,000, and enterprise carries the two
// stock rows its change always implied: 200,000 + 40,000 = 240,000. The two
// added cells are why the count above reads 12 and not 10.
var fixtureCells = []testCell{
	{mapping.KindRevenue, "taxes/property", "general", 100_000},
	{mapping.KindRevenue, "taxes/property", "enterprise", 0},
	{mapping.KindRevenue, "charges-for-services", "enterprise", 50_000},
	{mapping.KindExpenditure, "wages-and-benefits", "general", 70_000},
	{mapping.KindTransferIn, "transfers/in", "general", 10_000},
	{mapping.KindTransferOut, "transfers/out", "enterprise", 30_000},
	// Negative and positive, so both nodes the projection infers exist.
	{mapping.KindFundBalance, "fund-balance/change", "general", -20_000},
	{mapping.KindFundBalance, "fund-balance/change", "enterprise", 40_000},
	// Stocks. Facts the city printed that carry no link.
	{mapping.KindFundBalance, "fund-balance/beginning", "general", 500_000},
	{mapping.KindFundBalance, "fund-balance/ending", "general", 480_000},
	{mapping.KindFundBalance, "fund-balance/beginning", "enterprise", 200_000},
	{mapping.KindFundBalance, "fund-balance/ending", "enterprise", 240_000},
}

// printed renders cents the way these schedules print the figure, so that the
// fixture's tokens are ones amount.Parse actually has to handle: a zero is a dash,
// a negative is parenthesized (a leading minus sign is rejected as an extraction
// artifact), and everything else is grouped in thousands.
func printed(cents int64) string {
	if cents == 0 {
		return "-"
	}
	digits := strings.TrimPrefix(strings.TrimSuffix(amount.Cents(cents).String(), ".00"), "$")
	if cents < 0 {
		return "(" + strings.TrimPrefix(digits, "-") + ")"
	}
	return digits
}

// fact builds the fixture's fact for one cell, with an id from fact.MakeID over
// the same tuple `fisc build` hashes, so a finding that names an id names one
// that could exist.
//
// The offset is filled in by testFacts, which lays the cells out as a page.
func (c testCell) fact() fact.Fact {
	rule := "fixture-" + string(c.kind)
	rowPath := fact.RowPath(mapping.Row{Category: c.category})
	columnPath := fact.ColumnPath(mapping.Column{FundGroup: c.group}, testScope)
	return fact.Fact{
		ID:          fact.MakeID(testDoc, rule, rowPath, c.category, columnPath, testYear, testBasis),
		DocID:       testDoc,
		Page:        66,
		RuleID:      rule,
		Kind:        c.kind,
		Basis:       testBasis,
		Scope:       testScope,
		FiscalYear:  testYear,
		RowPath:     rowPath,
		RowLabel:    c.category,
		Category:    c.category,
		ColumnPath:  columnPath,
		FundGroup:   c.group,
		Sign:        mapping.SignPositive,
		Units:       "dollars",
		AmountCents: c.cents,
		Token:       printed(c.cents),
	}
}

// testFacts renders the fixture as a fact store: sorted, because that is what the
// file `fisc build` writes looks like and what facts-sorted asserts, and with each
// fact's token placed at its own offset in a page this fixture also generates, so
// that the provenance checks have something real to read.
func testFacts(cells ...testCell) []fact.Fact {
	if len(cells) == 0 {
		cells = fixtureCells
	}
	out := make([]fact.Fact, 0, len(cells))
	// One row per cell, label then figure, which is the shape of the schedules
	// this corpus is made of. Only the offsets matter to the checks; the layout is
	// here so that a failure message shows something page-like.
	var page strings.Builder
	for _, c := range cells {
		f := c.fact()
		page.WriteString(f.RowLabel + "   ")
		f.Offset = page.Len()
		page.WriteString(f.Token)
		page.WriteString("\n")
		out = append(out, f)
	}
	fact.Sort(out)
	return out
}

// inlinePageDoc builds a one-page document whose text is written in the test.
//
// testDocs derives its pages FROM the facts, which is right for the provenance
// checks and wrong for a check about what the page prints: those tests want the
// printed line and the rule that reads it on the same screen.
func inlinePageDoc(t *testing.T, docID string, page int, text string) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	name := corpus.PagePath(page)
	fsys[name] = &fstest.MapFile{Data: []byte(text)}
	man, err := json.Marshal(map[string]any{
		"schema_version": corpus.SchemaVersion,
		"doc_id":         docID,
		"artifacts":      map[string]corpus.Artifact{name: {Bytes: int64(len(text))}},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	fsys["manifest.json"] = &fstest.MapFile{Data: man}
	d, err := corpus.Open(fsys)
	if err != nil {
		t.Fatalf("corpus.Open: %v", err)
	}
	return d
}

// testDocs builds an extraction whose page text carries each fact's token at that
// fact's offset, by writing the tokens into a padded buffer.
//
// It is derived from the facts on purpose: the fixture then satisfies
// fact-offset-points-at-token by construction, and these tests are about the other
// checks. A test that wants the offset check to FAIL mutates a fact AFTER the
// subject is built, and the input-mutation tests in subject_test.go do it properly
// — through Load, over a copy of the real corpus, where the pages are fixed and it
// is the fact store that moves.
func testDocs(t *testing.T, facts []fact.Fact) map[string]*corpus.Doc {
	t.Helper()
	type key struct {
		doc  string
		page int
	}
	buf := map[key][]byte{}
	for _, f := range facts {
		if f.Offset < 0 {
			continue
		}
		k := key{f.DocID, f.Page}
		b := buf[k]
		for len(b) < f.Offset+len(f.Token) {
			b = append(b, ' ')
		}
		copy(b[f.Offset:], f.Token)
		buf[k] = b
	}

	pages := map[string]map[int][]byte{}
	for k, b := range buf {
		if pages[k.doc] == nil {
			pages[k.doc] = map[int][]byte{}
		}
		pages[k.doc][k.page] = b
	}

	docs := map[string]*corpus.Doc{}
	for docID, byPage := range pages {
		fsys := fstest.MapFS{}
		artifacts := map[string]corpus.Artifact{}
		for n, b := range byPage {
			name := corpus.PagePath(n)
			fsys[name] = &fstest.MapFile{Data: b}
			artifacts[name] = corpus.Artifact{Bytes: int64(len(b))}
		}
		man, err := json.Marshal(map[string]any{
			"schema_version": corpus.SchemaVersion,
			"doc_id":         docID,
			"artifacts":      artifacts,
		})
		if err != nil {
			t.Fatalf("marshal the fixture manifest: %v", err)
		}
		fsys["manifest.json"] = &fstest.MapFile{Data: man}
		doc, err := corpus.Open(fsys)
		if err != nil {
			t.Fatalf("open the fixture extraction: %v", err)
		}
		docs[docID] = doc
	}
	return docs
}

// The fixture department axis. `patrol` is what the department-bearing fixture
// facts carry; `records` is here so a test can name a division the registry
// does NOT list without inventing a department too, and `police-department`
// shares its name with no division on purpose -- the real file's five
// name-sharing pairs are pinned in internal/registry, not here.
const testDepartmentsYAML = `schema_version: 1
departments:
  - slug: police-department
    label: Police Department
    document_term: POLICE DEPARTMENT
    pages: [168]
divisions:
  - slug: patrol
    label: Patrol
    department: police-department
    pages: [168]
  - slug: records
    label: Records
    department: police-department
    pages: [168]
`

// testVocabulary loads the fixture registries the way Load does, through an
// fs.FS, so the tests exercise the real validation rather than a stub.
func testVocabulary(t *testing.T) *registry.Registry {
	t.Helper()
	reg, err := registry.Load(fstest.MapFS{
		registry.FundsFile:       &fstest.MapFile{Data: []byte(testFundsYAML)},
		registry.TaxonomyFile:    &fstest.MapFile{Data: []byte(testTaxonomyYAML)},
		registry.DepartmentsFile: &fstest.MapFile{Data: []byte(testDepartmentsYAML)},
	})
	if err != nil {
		t.Fatalf("load the fixture registries: %v", err)
	}
	return reg
}

// testSubject builds the subject the checks read, through the same projection
// path Load uses. The facts default to the whole fixture.
func testSubject(t *testing.T, facts ...fact.Fact) *Subject {
	t.Helper()
	if len(facts) == 0 {
		facts = testFacts()
	}
	reg := testVocabulary(t)
	registry := project.Registry(reg)
	projections, failures, err := buildProjections(registry, facts, testVersion)
	if err != nil {
		t.Fatalf("build the fixture projections: %v", err)
	}
	// Resolvers are left empty: nothing in tier 1 resolves a mapping rule. Docs
	// are not — fact-offset-points-at-token reads page text, which is committed
	// extraction and still not a PDF.
	return &Subject{
		Root:               t.TempDir(),
		Facts:              facts,
		Vocabulary:         reg,
		Docs:               testDocs(t, facts),
		Projections:        projections,
		ProjectionFailures: failures,
		// One spine document, because the fixture is a miniature of one year of
		// the spine and publishes nothing else. Load fills this from
		// project.PublishedDocuments over the real corpus, where it is three:
		// two years of the spine and the revenue trends.
		Published: spineDocuments(testYear),
	}
}

// spineDocuments is the published set of a repository that publishes only the
// spine, one document per year.
//
// It exists so a fixture can be a SMALLER repository than this one rather than a
// broken one. project.PublishedDocuments names revenue-trends, and a fixture
// carrying no revenue-by-fund fact is not a repository that has lost a published
// document — it is a miniature that never published it, and
// published-projection-built must not report on the difference.
func spineDocuments(years ...int) []project.PublishedDocument {
	slices := make([]project.Options, 0, len(years))
	for _, y := range years {
		slices = append(slices, project.Options{
			Columns: []project.Column{{FiscalYear: y, Basis: project.PublishedBasis}},
			Scopes:  []string{project.PublishedScope},
		})
	}
	out := make([]project.PublishedDocument, 0, len(years))
	for _, o := range slices {
		// project.Stem rather than a stem spelled here: a fixture that named its
		// documents by its own rule would stop being a miniature of this
		// repository the moment the rule moved, and pass while the real one did
		// not.
		stem, err := project.Stem(project.PublishedProjection, o, slices)
		if err != nil {
			panic(err)
		}
		out = append(out, project.PublishedDocument{
			Projection: project.PublishedProjection,
			Stem:       stem,
			Scopes:     []string{project.PublishedScope},
			Columns:    o.Columns,
		})
	}
	return out
}

// runChecks runs every check over s and returns the report.
func runChecks(t *testing.T, s *Subject) *Report {
	t.Helper()
	return Run(t.Context(), s, All(), ReportOptions{GeneratedBy: testVersion})
}

// resultFor returns the result of one check id, or fails.
func resultFor(t *testing.T, rep *Report, id string) Result {
	t.Helper()
	for _, res := range rep.Results {
		if res.CheckID == id {
			return res
		}
	}
	t.Fatalf("no result for check %q in %d results", id, len(rep.Results))
	return Result{}
}

// statuses is every check's status and subject count, which is what most of
// these tests are about: a check that concluded the right word over the wrong
// number of subjects has not checked what it says it did.
func statuses(rep *Report) map[string]string {
	out := make(map[string]string, len(rep.Results))
	for _, res := range rep.Results {
		out[res.CheckID] = fmt.Sprintf("%s over %d", res.Status, res.Subjects)
	}
	return out
}
