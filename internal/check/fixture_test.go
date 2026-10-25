package check

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// The fixture is a hand-made miniature of the citywide spine: two fund groups,
// one row of each kind, a zero that earns no link, the two stock rows, and a
// change in working capital of each sign so both inferred nodes exist.
const (
	testDoc     = "livermore-budget-fy2026-2027"
	testScope   = "all-funds-gross"
	testYear    = 2026
	testBasis   = vocab.BasisAdopted
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
	kind     vocab.Kind
	category string
	group    string
	cents    int64
}

// fixtureCells is the whole fixture: 12 cells, 7 earning a link, 4 stock rows
// and one printed zero. Each group ties beginning + change == ending and
// revenue + transfers in - expenditure - transfers out == change.
var fixtureCells = []testCell{
	{vocab.KindRevenue, "taxes/property", "general", 100_000},
	{vocab.KindRevenue, "taxes/property", "enterprise", 0},
	{vocab.KindRevenue, "charges-for-services", "enterprise", 50_000},
	{vocab.KindExpenditure, "wages-and-benefits", "general", 130_000},
	{vocab.KindTransferIn, "transfers/in", "general", 10_000},
	{vocab.KindTransferOut, "transfers/out", "enterprise", 30_000},
	// Negative and positive, so both nodes the projection infers exist.
	{vocab.KindFundBalance, "fund-balance/change", "general", -20_000},
	{vocab.KindFundBalance, "fund-balance/change", "enterprise", 20_000},
	// Stocks. Facts the city printed that carry no link.
	{vocab.KindFundBalance, "fund-balance/beginning", "general", 500_000},
	{vocab.KindFundBalance, "fund-balance/ending", "general", 480_000},
	{vocab.KindFundBalance, "fund-balance/beginning", "enterprise", 200_000},
	{vocab.KindFundBalance, "fund-balance/ending", "enterprise", 220_000},
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

// fact builds a cell's fact with an id from fact.MakeID over the tuple `fisc
// build` hashes; testFacts fills in the offset.
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
		Sign:        vocab.SignPositive,
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

// inlinePageDoc builds a one-page document whose text is written in the test,
// for checks about what a page prints.
func inlinePageDoc(t *testing.T, docID string, page int, text string) *corpus.Doc {
	t.Helper()
	return inlinePagesDoc(t, docID, map[int]string{page: text})
}

// inlinePagesDoc is inlinePageDoc for a test that needs more than one page --
// which is what it takes to show that a claim about one page does not reach
// another.
func inlinePagesDoc(t *testing.T, docID string, pages map[int]string) *corpus.Doc {
	t.Helper()
	fsys := fstest.MapFS{}
	artifacts := map[string]corpus.Artifact{}
	for page, text := range pages {
		name := corpus.PagePath(page)
		fsys[name] = &fstest.MapFile{Data: []byte(text)}
		artifacts[name] = corpus.Artifact{Bytes: int64(len(text))}
	}
	man, err := json.Marshal(map[string]any{
		"schema_version": corpus.SchemaVersion,
		"doc_id":         docID,
		"artifacts":      artifacts,
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

// testDocs builds an extraction carrying each fact's token at its offset, so
// the fixture satisfies fact-offset-points-at-token by construction.
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

// The fixture department axis: `records` is a division the registry does not
// list, and `police-department` shares its name with no division.
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
	return vocabularyWithDepartments(t, testDepartmentsYAML)
}

// vocabularyWithDepartments is testVocabulary with departments.yaml supplied by
// the caller. It still goes through Load, so a fixture the real file's rules
// would refuse fails here.
func vocabularyWithDepartments(t *testing.T, departments string) *registry.Registry {
	t.Helper()
	reg, err := registry.Load(fstest.MapFS{
		registry.FundsFile:       &fstest.MapFile{Data: []byte(testFundsYAML)},
		registry.TaxonomyFile:    &fstest.MapFile{Data: []byte(testTaxonomyYAML)},
		registry.DepartmentsFile: &fstest.MapFile{Data: []byte(departments)},
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
	// Resolvers are empty because Files is, so fact-offset-is-not-a-stated-total
	// is vacuous here (its subjects are in statedtotals_test.go); Docs are not,
	// because fact-offset-points-at-token reads page text.
	return &Subject{
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

// spineDocuments is the published set of a repository publishing only the
// spine, one document per year, so published-projection-built does not report
// on documents a fixture never published.
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

// runCheck runs one check over s; an error from the harness fails the test.
func runCheck(t *testing.T, c local, s *Subject) Result {
	t.Helper()
	res, err := c.Run(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// findingLines renders findings one per line, subject then detail.
func findingLines(res Result) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.Subject+": "+f.Detail)
	}
	return out
}

// said is a result's findings as one string, for a test asserting on prose.
func said(res Result) string { return strings.Join(findingLines(res), "\n") }

// hasFinding reports whether one finding's line contains every part.
func hasFinding(res Result, parts ...string) bool {
	for _, line := range findingLines(res) {
		if !slices.ContainsFunc(parts, func(p string) bool { return !strings.Contains(line, p) }) {
			return true
		}
	}
	return false
}

// wantPass fails the test unless res passed over exactly subjects subjects.
func wantPass(t *testing.T, res Result, subjects int) {
	t.Helper()
	if res.Status != StatusPass || res.Subjects != subjects {
		t.Fatalf("%s over %d (%s), findings %s; want pass over %d",
			res.Status, res.Subjects, res.Summary, said(res), subjects)
	}
}

// wantFail fails the test unless res failed and its findings, taken together,
// say every one of want.
func wantFail(t *testing.T, res Result, want ...string) {
	t.Helper()
	got := said(res)
	if res.Status != StatusFail {
		t.Fatalf("%s (%s), findings %s; want fail saying %q", res.Status, res.Summary, got, want)
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("findings %s do not say %q", got, w)
		}
	}
}
