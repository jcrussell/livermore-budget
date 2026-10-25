package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestFixtureVerdicts states every check's status and subject count over the
// fixture: 12 facts in 12 cells, 4 stocks and 1 zero, so 7 links over 9 nodes.
// A check not listed must be vacuous over 0.
func TestFixtureVerdicts(t *testing.T) {
	t.Parallel()
	rep := runChecks(t, testSubject(t))

	want := map[string]string{
		"source-pdfs-match-both-records": "skipped over 0",

		"facts-sorted":                "pass over 12",
		"fact-ids-unique":             "pass over 12",
		"fact-ids-recompute":          "pass over 12",
		"fact-token-reparses":         "pass over 12",
		"fact-offset-points-at-token": "pass over 12",
		"fact-citations-are-declared": "pass over 12",
		// Two of the fixture's twelve facts are transfers, and neither is
		// printed against its kind's direction.
		"fact-transfer-orientation-is-declared": "pass over 2",
		"fact-vocabulary":                       "pass over 24", // 12 categories + 12 fund groups
		"fact-kind-matches-category":            "pass over 12",
		// Two fund balances, general and enterprise, each with all three of its
		// lines. See fixtureCells.
		"fund-balance-identity":         "pass over 2",
		"fund-group-sources-equal-uses": "pass over 2",
		"projections-build":             "pass over 1",
		"published-projection-built":    "pass over 1",
		// Two projections are registered, but the trends projection is of
		// nothing here, so one document is built and one examined.
		"documents-are-checked":           "pass over 1",
		"graph-acyclic":                   "pass over 7",
		"node-tiers-are-declared":         "pass over 9",
		"derived-nodes-justified":         "pass over 2",
		"link-locators-match-their-facts": "pass over 7",
		"link-ends-match-their-facts":     "pass over 7",
		"link-values-tie-to-facts":        "pass over 7",
		"link-kinds-match-their-facts":    "pass over 7",
		"counts-reconcile":                "pass over 1",
		"headline-ties-to-facts":          "pass over 3", // links leaving a revenue node or entering an expenditure one
		"headline-transfer-residual":      "pass over 2",
		"headline-naive-expenditure":      "pass over 1",
	}
	got := statuses(rep)
	for id := range got {
		if _, ok := want[id]; !ok {
			want[id] = "vacuous over 0"
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("verdicts mismatch (-want +got):\n%s", diff)
	}
	if rep.Failed() {
		t.Error("Failed() = true for a report with no failure, error or --strict")
	}
}

// TestVacuousFailsOnlyUnderStrict: a vacuous check is neither pass nor failure
// by default; --strict fails it.
func TestVacuousFailsOnlyUnderStrict(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	lenient := Run(t.Context(), s, All(), ReportOptions{})
	strict := Run(t.Context(), s, All(), ReportOptions{Strict: true})

	if lenient.Counts.Vacuous == 0 {
		t.Fatal("no check is vacuous over the fixture, so this test covers nothing")
	}
	if lenient.Failed() {
		t.Error("a run with vacuous checks failed without --strict")
	}
	if !strict.Failed() {
		t.Error("a run with vacuous checks passed under --strict")
	}
	// The statuses must be identical: --strict changes what a vacuous result
	// means for the exit code, not what any check concluded.
	if diff := cmp.Diff(statuses(lenient), statuses(strict)); diff != "" {
		t.Errorf("--strict changed a verdict (-lenient +strict):\n%s", diff)
	}
}

// TestVacuousChecksSayWhatIsAbsent covers the other half of legibility: the
// number says "nothing", and the sentence has to say what.
func TestVacuousChecksSayWhatIsAbsent(t *testing.T) {
	t.Parallel()
	rep := runChecks(t, testSubject(t))
	for id, want := range map[string]string{
		"transfer-legs-pair":                    "no link carries a transfer_id",
		"node-hierarchy-well-formed":            "no node carries a parent",
		"constraint-tier-vocabulary":            "no node carries a constraint_tier",
		"contra-links-name-their-schedule":      "no projection publishes a negative link",
		"fact-departments-resolve":              "no fact carries a department",
		"fact-funds-resolve":                    "no fact names a fund",
		"fact-revenue-lines-resolve":            "no fact is a revenue row of scope revenue-by-fund",
		"revenue-lines-tie-to-their-categories": "no projection built a drill-down",
		"expansion-opens-as-the-spine":          "no expansion schedule is built",
		"expansion-splits-tie":                  "no expansion schedule declares a split",
	} {
		res := resultFor(t, rep, id)
		if res.Status != StatusVacuous {
			t.Errorf("%s = %s, want vacuous", id, res.Status)
		}
		if !strings.Contains(res.Summary, want) {
			t.Errorf("%s summary %q does not say %q", id, res.Summary, want)
		}
		if res.Findings == nil {
			t.Errorf("%s findings are nil, want an empty slice", id)
		}
	}
}

// TestUnsortedFactStoreFails: a hand-edited or reordered facts.jsonl is caught
// here and nowhere else.
func TestUnsortedFactStoreFails(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	slices.Reverse(facts)
	res := resultFor(t, runChecks(t, testSubject(t, facts...)), "facts-sorted")

	if res.Status != StatusFail {
		t.Fatalf("status = %s over %d subjects, want fail", res.Status, res.Subjects)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(res.Findings))
	}
	if !strings.Contains(res.Findings[0].Detail, "out of order") {
		t.Errorf("finding %q does not say the facts are out of order", res.Findings[0].Detail)
	}
	// A failing check must not borrow the sentence written for the passing case:
	// "FAIL facts-sorted: 240 facts, in canonical order" is a contradiction.
	if want := "1 finding over 12 facts"; res.Summary != want {
		t.Errorf("summary = %q, want %q", res.Summary, want)
	}
}

// TestKindMatchesCategorySummariesArePinned pins both sentences
// fact-kind-matches-category can publish; the pass summary carries the count,
// so a silently shrunk denominator shows.
func TestKindMatchesCategorySummariesArePinned(t *testing.T) {
	t.Parallel()
	t.Run("pass", func(t *testing.T) {
		res := resultFor(t, runChecks(t, testSubject(t)), "fact-kind-matches-category")
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if want := "12 facts over 8 kind/category pairs, each kind one its category " +
			"declares in data/taxonomy.yaml"; res.Summary != want {
			t.Errorf("summary = %q, want %q", res.Summary, want)
		}
	})

	// Every fact stripped of its category: the check has nothing to assert
	// about and must SAY so rather than report a pass over zero.
	t.Run("vacuous", func(t *testing.T) {
		facts := testFacts()
		for i := range facts {
			facts[i].Category = ""
		}
		res := resultFor(t, runChecks(t, testSubject(t, facts...)), "fact-kind-matches-category")
		if res.Status != StatusVacuous {
			t.Fatalf("status = %s (%s), want vacuous", res.Status, res.Summary)
		}
		if want := "no fact carries both a kind and an assignable category " +
			"data/taxonomy.yaml defines"; res.Summary != want {
			t.Errorf("summary = %q, want %q", res.Summary, want)
		}
		if res.Findings == nil {
			t.Error("findings is nil; a vacuous result carries an empty slice")
		}
	})
}

// TestAnUnassignableCategoryReddensOneVocabularyCheck: `taxes` is a rollup, so
// factVocabulary reports it and fact-kind-matches-category passes over the
// rest rather than reddening or going vacuous.
func TestAnUnassignableCategoryReddensOneVocabularyCheck(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	// Both fields: taxes declares kinds: [revenue], so the kind must be one
	// `taxes` does not declare or there is no double-redden to refuse.
	facts[0].Category = "taxes"
	facts[0].Kind = vocab.KindExpenditure
	results := runChecks(t, testSubject(t, facts...))

	if res := resultFor(t, results, "fact-vocabulary"); res.Status != StatusFail {
		t.Errorf("fact-vocabulary = %s, want fail; it owns this finding", res.Status)
	}
	res := resultFor(t, results, "fact-kind-matches-category")
	if res.Status != StatusPass {
		t.Fatalf("fact-kind-matches-category = %s (%s), want pass over the rest",
			res.Status, res.Summary)
	}
	// One fact left the denominator, and the summary says so rather than
	// printing the old count over a smaller set.
	if want := "11 facts"; !strings.Contains(res.Summary, want) {
		t.Errorf("summary = %q, want it to contain %q; the excluded fact must leave "+
			"the count as well as the findings", res.Summary, want)
	}
}

// TestCollidingIDsFail covers the other fact-store check. Two facts with one id
// mean two rules claim the same cell of the same document.
func TestCollidingIDsFail(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	facts[1].ID = facts[0].ID
	res := resultFor(t, runChecks(t, testSubject(t, facts...)), "fact-ids-unique")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if !strings.Contains(res.Findings[0].Detail, facts[0].ID) {
		t.Errorf("finding %q does not name the colliding id %s", res.Findings[0].Detail, facts[0].ID)
	}
}

// TestARelabelledFactNoLongerAddressesItself: editing row_label passes
// facts-sorted, fact-ids-unique and fact-token-reparses, since none relates
// the id to the fields it was hashed over.
func TestARelabelledFactNoLongerAddressesItself(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	stale := facts[0].ID
	facts[0].RowLabel = "Property Taxes (restated)"
	rep := runChecks(t, testSubject(t, facts...))

	res := resultFor(t, rep, "fact-ids-recompute")
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	// The finding names the stale id as subject and the recomputed id as the
	// remedy.
	want := fact.MakeID(facts[0].DocID, facts[0].RuleID, facts[0].RowPath,
		facts[0].RowLabel, facts[0].ColumnPath, facts[0].FiscalYear, facts[0].Basis)
	if got := res.Findings[0]; got.Subject != stale || !strings.Contains(got.Detail, want) {
		t.Errorf("finding = %+v, want subject %s naming %s", got, stale, want)
	}
	// The neighbours stay green.
	for _, id := range []string{"facts-sorted", "fact-ids-unique", "fact-token-reparses"} {
		if res := resultFor(t, rep, id); res.Status != StatusPass {
			t.Errorf("%s = %s over a relabelled fact, want pass: this check is not "+
				"redundant with it", id, res.Status)
		}
	}
}

// TestVocabularyCatchesAnUnknownCategory: `tax/property` for
// `taxes/property` joins to nothing.
func TestVocabularyCatchesAnUnknownCategory(t *testing.T) {
	t.Parallel()
	cells := slices.Clone(fixtureCells)
	cells[0].category = "tax/property"
	res := resultFor(t, runChecks(t, cellsSubject(t, cells)), "fact-vocabulary")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want 1: %v", got, res.Findings)
	}
	f := res.Findings[0]
	if !strings.Contains(f.Detail, `"tax/property"`) || !strings.Contains(f.Detail, "not defined") {
		t.Errorf("finding %q does not report an undefined category", f.Detail)
	}
	if !strings.HasPrefix(f.Subject, fact.IDPrefix) {
		t.Errorf("finding subject %q does not name the fact at fault", f.Subject)
	}
}

// TestVocabularyCatchesARollupCategory: `taxes` exists but is a rollup no rule
// may assign, and the finding says which of the two problems it is.
func TestVocabularyCatchesARollupCategory(t *testing.T) {
	t.Parallel()
	cells := slices.Clone(fixtureCells)
	cells[0].category = "taxes"
	res := resultFor(t, runChecks(t, cellsSubject(t, cells)), "fact-vocabulary")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	detail := res.Findings[0].Detail
	for _, want := range []string{`"taxes"`, "rollup", "assignable: false"} {
		if !strings.Contains(detail, want) {
			t.Errorf("finding %q does not contain %q", detail, want)
		}
	}
	if strings.Contains(detail, "not defined") {
		t.Errorf("finding %q reports a rollup as an undefined slug", detail)
	}
}

// TestVocabularyCatchesAnUnknownFundGroup: groups come from funds.yaml, so a
// group no fund is recorded under is not a group.
func TestVocabularyCatchesAnUnknownFundGroup(t *testing.T) {
	t.Parallel()
	cells := slices.Clone(fixtureCells)
	cells[0].group = "permanent"
	res := resultFor(t, runChecks(t, cellsSubject(t, cells)), "fact-vocabulary")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if !strings.Contains(res.Findings[0].Detail, `fund group "permanent"`) {
		t.Errorf("finding %q does not name the fund group", res.Findings[0].Detail)
	}
}

// TestATransferCountedAsRevenueIsCaughtAtTheFact: pp.131-140's funds tie if a
// Transfers In row is called revenue; `transfers/in` is assignable, so
// fact-vocabulary passes and only this check sees it.
func TestATransferCountedAsRevenueIsCaughtAtTheFact(t *testing.T) {
	t.Parallel()
	cells := slices.Clone(fixtureCells)
	if cells[4].category != "transfers/in" || cells[4].kind != vocab.KindTransferIn {
		t.Fatalf("fixture cell 4 is %+v, want the transfers/in row", cells[4])
	}
	cells[4].kind = vocab.KindRevenue
	rep := runChecks(t, cellsSubject(t, cells))

	res := resultFor(t, rep, "fact-kind-matches-category")
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if res.Subjects != 12 {
		t.Errorf("subjects = %d, want all 12 facts", res.Subjects)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want 1: %v", got, res.Findings)
	}
	f := res.Findings[0]
	for _, want := range []string{`kind "revenue"`, `category "transfers/in"`, "transfer_in"} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("finding %q does not contain %q", f.Detail, want)
		}
	}
	if want := cells[4].fact().ID; f.Subject != want {
		t.Errorf("finding subject = %q, want the fact at fault %q", f.Subject, want)
	}
	// Nothing else says a word about it.
	if got := resultFor(t, rep, "fact-vocabulary").Status; got != StatusPass {
		t.Errorf("fact-vocabulary = %s, want pass: transfers/in is a real assignable slug, "+
			"which is exactly why the pair needed its own check", got)
	}
}

// TestEveryKindIsWrongSomewhere walks the whole cross product the fixture taxonomy
// permits, so the check is not shown failable by one lucky pair.
func TestEveryKindIsWrongSomewhere(t *testing.T) {
	t.Parallel()
	tests := []struct {
		category string
		wrong    vocab.Kind
		declares string
	}{
		{"taxes/property", vocab.KindExpenditure, "revenue"},
		{"wages-and-benefits", vocab.KindRevenue, "expenditure"},
		{"transfers/out", vocab.KindFundBalance, "transfer_out"},
		{"fund-balance/beginning", vocab.KindTransferIn, "fund_balance"},
	}
	for _, tt := range tests {
		t.Run(tt.category+"/"+string(tt.wrong), func(t *testing.T) {
			facts := testFacts()
			facts[0].Category, facts[0].Kind = tt.category, tt.wrong
			res := resultFor(t, runChecks(t, factsSubject(t, facts)),
				"fact-kind-matches-category")

			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if got := len(res.Findings); got != 1 {
				t.Fatalf("findings = %d, want 1: %v", got, res.Findings)
			}
			// HasSuffix, not Contains: "declares transfer" is a prefix of
			// "declares transfer_in".
			if want := "which declares " + tt.declares; !strings.HasSuffix(res.Findings[0].Detail, want) {
				t.Errorf("finding %q does not end by saying the category declares exactly %q",
					res.Findings[0].Detail, tt.declares)
			}
		})
	}
}

// TestTheCommittedTaxonomyTellsTheTransferDirectionsApart reads
// data/taxonomy.yaml itself: a transfer category declaring `kinds: [transfer]`
// would miss both a transfer counted as revenue and one pointing the wrong way.
func TestTheCommittedTaxonomyTellsTheTransferDirectionsApart(t *testing.T) {
	t.Parallel()
	reg, err := registry.Load(os.DirFS(filepath.Join(repoRoot(t), "data")))
	if err != nil {
		t.Fatalf("load the committed registries: %v", err)
	}
	cells := []testCell{
		// Correct: each direction under its own category, and out-to-cip, which
		// is a sibling of transfers/out and carries the same kind.
		{vocab.KindTransferIn, "transfers/in", "general", 10_000},
		{vocab.KindTransferOut, "transfers/out", "enterprise", 30_000},
		{vocab.KindTransferOut, "transfers/out-to-cip", "general", 5_000},
		// Wrong family: the p131 Stormwater trap, a Transfers In row called
		// revenue so the printed Total <fund> ties.
		{vocab.KindRevenue, "transfers/in", "enterprise", 3_247_000},
		// Wrong direction: resolves clean if transfers/in declares `transfer`.
		{vocab.KindTransferOut, "transfers/in", "general", 7_000},
	}
	res := resultFor(t, runChecks(t, &Subject{Facts: testFacts(cells...), Vocabulary: reg}),
		"fact-kind-matches-category")

	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if res.Subjects != len(cells) {
		t.Errorf("subjects = %d, want %d", res.Subjects, len(cells))
	}
	// Exact equality, not Contains: "which declares transfer" is a prefix of
	// "which declares transfer_in".
	want := []string{
		testDoc + ` p66 "transfers/in": kind "revenue" is not one data/taxonomy.yaml ` +
			`declares for category "transfers/in", which declares transfer_in`,
		testDoc + ` p66 "transfers/in": kind "transfer_out" is not one data/taxonomy.yaml ` +
			`declares for category "transfers/in", which declares transfer_in`,
	}
	got := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		got = append(got, f.Detail)
	}
	slices.Sort(got)
	slices.Sort(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("findings mismatch (-want +got):\n%s", diff)
	}
	// The three correct rows are silent, so the check is not simply reddening
	// every transfer it sees.
	if got := resultFor(t, runChecks(t, &Subject{Facts: testFacts(cells[:3]...), Vocabulary: reg}),
		"fact-kind-matches-category").Status; got != StatusPass {
		t.Errorf("the three correct transfer rows = %s, want pass", got)
	}
}

// TestDepartmentsResolveAgainstTheRegistry: a well-formed department that names
// neither a division nor a department joins to nothing. Built without a
// projection, which refuses department-carrying facts.
func TestDepartmentsResolveAgainstTheRegistry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		dept   string
		status Status
		want   string
	}{
		{"a listed division", "patrol", StatusPass, ""},
		// Either tier resolves: pp.85-125's funding-source rows name a
		// DEPARTMENT, and `police-department` is one the fixture lists that
		// names no division.
		{"a listed department", "police-department", StatusPass, ""},
		{"a slug the registry lists on neither tier", "traffic", StatusFail,
			`department "traffic" is neither a division nor a department data/departments.yaml lists`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := testFacts()
			facts[0].Department = tt.dept
			facts[1].Department = tt.dept
			rep := runChecks(t, factsSubject(t, facts))

			res := resultFor(t, rep, "fact-departments-resolve")
			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != 2 {
				t.Errorf("subjects = %d, want the 2 facts carrying a department", res.Subjects)
			}
			if tt.want != "" && !hasFinding(res, tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
			// A category typo and an unresolved department must not share a
			// verdict, because they will not share a fix.
			if got := resultFor(t, rep, "fact-vocabulary").Status; got != StatusPass {
				t.Errorf("fact-vocabulary = %s, want pass: no category or fund group is wrong here", got)
			}
		})
	}
}

// A malformed department is reported once, as a shape problem, not also as
// unlisted.
func TestAMalformedDepartmentIsNotAlsoReportedAsUnlisted(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	facts[0].Department = "Patrol"
	res := resultFor(t, runChecks(t, factsSubject(t, facts)), "fact-departments-resolve")

	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want 1:\n%s", got, said(res))
	}
	if !hasFinding(res, "is not a slug") {
		t.Errorf("finding %v is not the shape diagnosis", res.Findings)
	}
	if hasFinding(res, "is not a division") {
		t.Error("the shape failure is also reported as an unlisted division; " +
			"one fact with one fix must not produce two findings")
	}
}

// TestDepartmentsAreCheckedForWhatIsCheckable: a label used as a slug, one
// department spelled two ways, and a category slug used as a department.
func TestDepartmentsAreCheckedForWhatIsCheckable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		first string
		rest  string
		want  string
	}{
		{"not a slug", "Police Department", "patrol", "is not a slug"},
		{"two spellings", "patrol", "pa-trol", "spelled 2 ways"},
		{"collides with a category", "charges-for-services", "patrol", "also a data/taxonomy.yaml category slug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := testFacts()
			facts[0].Department = tt.first
			facts[1].Department = tt.rest
			res := resultFor(t, runChecks(t, factsSubject(t, facts)), "fact-departments-resolve")

			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if res.Subjects != 2 {
				t.Errorf("subjects = %d, want the 2 facts carrying a department", res.Subjects)
			}
			if !hasFinding(res, tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestFundNumbersResolveAgainstTheRegistry: a fact's fund resolves in
// data/funds.yaml and agrees with its fund group; a 0 is refused first.
func TestFundNumbersResolveAgainstTheRegistry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		fund   int
		group  string
		status Status
		want   string
	}{
		{"a listed fund", 100, "general", StatusPass, ""},
		{"a fund the registry does not list", 999, "general", StatusFail, "not in data/funds.yaml"},
		{"a listed fund under the wrong group", 100, "enterprise", StatusFail, `is type "general"`},
		{"fund 0", 0, "general", StatusFail, "fund 0 names no fund"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := testFacts()
			facts[0].Fund = fact.FundNumber(tt.fund)
			facts[0].FundGroup = tt.group
			res := resultFor(t, runChecks(t, factsSubject(t, facts)), "fact-funds-resolve")

			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != 1 {
				t.Errorf("subjects = %d, want the 1 fact naming a fund", res.Subjects)
			}
			if tt.want != "" && !hasFinding(res, tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestLinkValueMustEqualItsFacts: a link's value is the sum of the facts it
// cites.
func TestLinkValueMustEqualItsFacts(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	link := &s.Projections[0].Graph.Links[0]
	link.ValueCents += 1_00
	res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want 1: %v", got, res.Findings)
	}
	f := res.Findings[0]
	if !strings.Contains(f.Subject, link.Source) || !strings.Contains(f.Subject, link.Target) {
		t.Errorf("finding subject %q does not name the link's endpoints", f.Subject)
	}
	if !strings.Contains(f.Detail, "off by $1.00") {
		t.Errorf("finding %q does not state the difference", f.Detail)
	}
}

// TestLinkFactIDsMustAscend holds the contract's fact_ids order. Mutation:
// drop the strictlyAscending arm from link-values-tie-to-facts and a link
// citing its facts reversed passes, since the sum is the same.
func TestLinkFactIDsMustAscend(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	var link *project.Link
	for _, p := range s.linkedDocuments() {
		for i := range p.Links {
			if l := &p.Links[i]; len(l.FactIDs) > 1 && link == nil {
				link = l
			}
		}
	}
	if link == nil {
		t.Fatal("no link in the committed corpus cites two facts")
	}
	slices.Reverse(link.FactIDs)
	res := runCheck(t, &linkValuesTieToFacts{}, s)
	if res.Status != StatusFail || !hasFinding(res, "not strictly ascending") {
		t.Fatalf("status = %s, findings %v; want the reversed fact_ids refused", res.Status, res.Findings)
	}
	if strictlyAscending([]string{"a", "a"}) {
		t.Error("an id cited twice reads as ascending")
	}
}

// TestLinkLocatorsMustBeThePagesOfItsFacts: nothing else compares a link's
// locators to its facts' pages, so a mark could cite a page its figure is not
// printed on with every other gate green.
func TestLinkLocatorsMustBeThePagesOfItsFacts(t *testing.T) {
	t.Parallel()
	t.Run("a page the facts did not come from", func(t *testing.T) {
		s := testSubject(t)
		link := &s.Projections[0].Graph.Links[0]
		if len(link.Locators) == 0 || len(link.Locators[0].Pages) == 0 {
			t.Fatalf("the fixture's first link has no locator to corrupt: %+v", link.Locators)
		}
		was := link.Locators[0].Pages[0]
		link.Locators[0].Pages = []int{9999}
		res := resultFor(t, runChecks(t, s), "link-locators-match-their-facts")

		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if got := len(res.Findings); got != 1 {
			t.Fatalf("findings = %d, want 1: %v", got, res.Findings)
		}
		f := res.Findings[0]
		if !strings.Contains(f.Subject, link.Source) || !strings.Contains(f.Subject, link.Target) {
			t.Errorf("finding subject %q does not name the link's endpoints", f.Subject)
		}
		// The finding names both pages.
		for _, want := range []string{"p9999", fmt.Sprintf("p%d", was)} {
			if !strings.Contains(f.Detail, want) {
				t.Errorf("finding %q does not name %s", f.Detail, want)
			}
		}
	})

	// What a rule split produces: the facts still cited, one page dropped.
	t.Run("a locator dropped entirely", func(t *testing.T) {
		s := testSubject(t)
		link := &s.Projections[0].Graph.Links[0]
		link.Locators = []project.Source{}
		res := resultFor(t, runChecks(t, s), "link-locators-match-their-facts")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if got := len(res.Findings); got != 1 {
			t.Fatalf("findings = %d, want 1: %v", got, res.Findings)
		}
	})

	// A nil list gets its own sentence: it is what a projection that never
	// populated the field publishes.
	t.Run("no locators at all", func(t *testing.T) {
		s := testSubject(t)
		link := &s.Projections[0].Graph.Links[0]
		link.Locators = nil
		res := resultFor(t, runChecks(t, s), "link-locators-match-their-facts")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if got := res.Findings[0].Detail; !strings.Contains(got, "no locators at all") {
			t.Errorf("finding %q does not say the field is absent rather than wrong", got)
		}
	})
}

// TestALinkCitingAnUnresolvableFactIsNamedOnce: an unresolvable fact_id is
// link-values-tie-to-facts's finding and not repeated here.
func TestALinkCitingAnUnresolvableFactIsNamedOnce(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	link := &s.Projections[0].Graph.Links[0]
	link.FactIDs = []string{"fisc-f-notafact"}
	results := runChecks(t, s)

	if res := resultFor(t, results, "link-values-tie-to-facts"); res.Status != StatusFail {
		t.Errorf("link-values-tie-to-facts = %s, want fail; it owns this finding", res.Status)
	}
	if res := resultFor(t, results, "link-locators-match-their-facts"); res.Status == StatusFail {
		t.Errorf("link-locators-match-their-facts also failed (%v); an unresolvable "+
			"citation is one defect with one fix and must be named once", res.Findings)
	}
}

// TestFundBalanceDrawKeepsItsSign: the draw leg carries a negated value, and an
// "absolute sum" check would accept it pointing the wrong way.
func TestFundBalanceDrawKeepsItsSign(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	draw := linkFrom(t, s.Projections[0].Graph, project.NodeFundBalanceDraw)
	if draw.ValueCents != 20_000 {
		t.Fatalf("the draw link is %d cents, want the negation of the -20000 fact", draw.ValueCents)
	}
	if res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts"); res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
	}

	// The same value at the contribution node: the magnitude matches, the
	// direction is wrong.
	draw.Source, draw.Target = "fund-group/general", project.NodeFundBalanceContribution
	if res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts"); res.Status != StatusFail {
		t.Errorf("status = %s, want fail for a draw whose sign was not decomposed", res.Status)
	}
}

// TestCycleIsFoundAndNamed: the shipped graph has no cycle, and d3-sankey
// renders one without complaint.
func TestCycleIsFoundAndNamed(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	g := s.Projections[0].Graph
	// Send an expenditure category's money back to the fund group it came from.
	g.Links = append(g.Links, project.Link{
		Source: "expenditure/wages-and-benefits", Target: "fund-group/general",
		ValueCents: 1, Kind: project.KindExternal, FactIDs: []string{s.Facts[0].ID},
	})
	res := resultFor(t, runChecks(t, s), "graph-acyclic")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	detail := res.Findings[0].Detail
	for _, want := range []string{"fund-group/general", "expenditure/wages-and-benefits"} {
		if !strings.Contains(detail, want) {
			t.Errorf("finding %q does not name %q", detail, want)
		}
	}
}

// TestDerivedNodeNeedsItsProvenance: a derived node carries its rationale and
// source note.
func TestDerivedNodeNeedsItsProvenance(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		drop func(*project.Node)
		want string
	}{
		{"no rationale", func(n *project.Node) { n.Rationale = "" }, "no rationale"},
		{"no source note", func(n *project.Node) { n.SourceNote = "" }, "no source note"},
		{"not marked derived", func(n *project.Node) { n.Derived = false }, "not marked derived"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := testSubject(t)
			tt.drop(nodePointer(t, s.Projections[0].Graph, project.NodeFundBalanceDraw))
			res := resultFor(t, runChecks(t, s), "derived-nodes-justified")

			if res.Status != StatusFail {
				t.Fatalf("status = %s, want fail", res.Status)
			}
			if !strings.Contains(res.Findings[0].Detail, tt.want) {
				t.Errorf("finding %q does not say %q", res.Findings[0].Detail, tt.want)
			}
			if !strings.Contains(res.Findings[0].Subject, project.NodeFundBalanceDraw) {
				t.Errorf("finding subject %q does not name the node", res.Findings[0].Subject)
			}
		})
	}
}

// TestCountsMustAccountForEveryFact: facts, facts cited, and the identity that
// the uncited are exactly the stocks plus the zero cells.
func TestCountsMustAccountForEveryFact(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tamper func(*project.Document)
		want   string
	}{
		{"facts", func(g *project.Document) { g.Metadata.Counts.Facts++ }, "counts.facts is 13"},
		{"cited", func(g *project.Document) { g.Metadata.Counts.FactsCited++ }, "counts.facts_cited is 8"},
		// A citation added with facts_cited raised: the identity goes red.
		{"a citation added and facts_uncited left", func(g *project.Document) {
			g.Links[0].FactIDs = append(g.Links[0].FactIDs, stockFactID(g))
			g.Metadata.Counts.FactsCited++
		}, "8 + 5 = 13"},
		{"nodes", func(g *project.Document) { g.Metadata.Counts.Nodes = 99 }, "counts.nodes is 99"},
		{"links", func(g *project.Document) { g.Metadata.Counts.Links = 99 }, "counts.links is 99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSubject(t)
			tt.tamper(s.Projections[0].Graph)
			res := resultFor(t, runChecks(t, s), "counts-reconcile")

			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if !hasFinding(res, tt.want) {
				t.Errorf("findings %v do not report %q", res.Findings, tt.want)
			}
		})
	}
}

// TestCountsReconcileNamesTheArithmetic: the passing summary shows where the
// gap went.
func TestCountsReconcileNamesTheArithmetic(t *testing.T) {
	t.Parallel()
	res := resultFor(t, runChecks(t, testSubject(t)), "counts-reconcile")
	if want := "(12 = 7 cited + 5 uncited, 0 cited twice)"; !strings.Contains(res.Summary, want) {
		t.Errorf("summary %q does not contain %q", res.Summary, want)
	}
}

// TestTransferHeadlineIsTheLinks checks the headline against the transfer
// links the document draws.
func TestTransferHeadlineIsTheLinks(t *testing.T) {
	t.Parallel()
	res := resultFor(t, runChecks(t, testSubject(t)), "headline-transfer-residual")
	if res.Status != StatusPass || res.Subjects != 2 {
		t.Fatalf("status = %s over %d, want pass over 2", res.Status, res.Subjects)
	}

	s := testSubject(t)
	s.Projections[0].Graph.Metadata.Headline.InternalTransferOutCents = 0
	res = resultFor(t, runChecks(t, s), "headline-transfer-residual")
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail for a transfer-out figure the links do not draw", res.Status)
	}
	if !hasFinding(res, "internal_transfer_out_cents is $0.00") {
		t.Errorf("findings %v do not state the figure that was published", res.Findings)
	}
}

// TestNaiveExpenditureMustStayWrong: the naive figure must differ from the
// real one.
func TestNaiveExpenditureMustStayWrong(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	h := s.Projections[0].Graph.Metadata.Headline
	if h.NaiveExpenditureCents != h.AllFundsGrossExpenditureCents+h.InternalTransferOutCents {
		t.Fatalf("the fixture's naive figure is not gross plus transfers out: %+v", h)
	}
	h.ExternalExpenditureCents = h.NaiveExpenditureCents
	res := resultFor(t, runChecks(t, s), "headline-naive-expenditure")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if !hasFinding(res, "no longer netting out the double count") {
		t.Errorf("findings %v do not say what agreement would mean", res.Findings)
	}
}

// TestNaiveExpenditureIsVacuousWithoutADoubleCount: with no transfers out and
// no internal service charges the two figures agree, and the check is vacuous.
func TestNaiveExpenditureIsVacuousWithoutADoubleCount(t *testing.T) {
	t.Parallel()
	cells := []testCell{
		{vocab.KindRevenue, "taxes/property", "general", 100_000},
		{vocab.KindExpenditure, "wages-and-benefits", "general", 70_000},
	}
	s := testSubject(t, testFacts(cells...)...)
	res := resultFor(t, runChecks(t, s), "headline-naive-expenditure")

	if res.Status != StatusVacuous {
		t.Fatalf("status = %s over %d subjects, want vacuous", res.Status, res.Subjects)
	}
	h := s.Projections[0].Graph.Metadata.Headline
	if h.ExternalExpenditureCents != h.NaiveExpenditureCents {
		t.Fatalf("the two figures differ (%d, %d), so this test is not covering the case it names",
			h.ExternalExpenditureCents, h.NaiveExpenditureCents)
	}
}

// TestTransferLegsPairWhenLegsExist hands the check legs, which no shipped
// graph's links carry.
func TestTransferLegsPairWhenLegsExist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values []int64
		status Status
		want   string
	}{
		{"two equal legs", []int64{5_000, 5_000}, StatusPass, ""},
		{"two unequal legs", []int64{5_000, 4_000}, StatusFail, "off by $10.00"},
		{"one leg", []int64{5_000}, StatusFail, "names 1 legs, want 2"},
		{"two receiving legs", []int64{5_000, 5_000}, StatusFail, "not one receiving and one paying"},
		{"two legs from a payer's end to a receiver's", []int64{5_000, 5_000}, StatusFail,
			"not one receiving and one paying"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSubject(t)
			g := s.Projections[0].Graph
			for i, v := range tt.values {
				g.Links[i].TransferID = "t-1"
				g.Links[i].ValueCents = v
			}
			// Leg 0 receives from a payer's end; leg 1 pays into a receiver's,
			// unless the case makes both receiving.
			setRole := func(id, role string) {
				for j := range g.Nodes {
					if g.Nodes[j].ID == id {
						g.Nodes[j].Role = role
					}
				}
			}
			setRole(g.Links[0].Source, project.RoleTransferSource)
			if len(tt.values) > 1 {
				switch tt.name {
				case "two receiving legs":
					setRole(g.Links[1].Source, project.RoleTransferSource)
				case "two legs from a payer's end to a receiver's":
					for _, l := range g.Links[:2] {
						setRole(l.Source, project.RoleTransferSource)
						setRole(l.Target, project.RoleTransferSink)
					}
				default:
					setRole(g.Links[1].Target, project.RoleTransferSink)
				}
			}
			res := resultFor(t, runChecks(t, s), "transfer-legs-pair")

			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != 1 {
				t.Errorf("subjects = %d, want 1 transfer id", res.Subjects)
			}
			if tt.want != "" && !hasFinding(res, tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestTransferLegsPairSeesALegWithNoID: every link of p76's document is a
// leg, so one carrying no transfer_id is half a movement the pairing would
// otherwise skip, leaving the check vacuous over a document that drew legs.
func TestTransferLegsPairSeesALegWithNoID(t *testing.T) {
	t.Parallel()
	res := runCheck(t, &transferLegsPair{}, scheduleSubject())
	if res.Status != StatusFail || !hasFinding(res, "carries no transfer_id") {
		t.Fatalf("status = %s (%s), findings %v; want a fail naming the leg with no transfer_id",
			res.Status, res.Summary, res.Findings)
	}
	if n := len(res.Findings); n != 1 {
		t.Errorf("%d findings, want the transfers document's one link; the other documents carry no legs", n)
	}
}

// TestNodeHierarchyWellFormedIsFailable damages a hierarchy one way per claim.
// The fixture carries no hierarchy, so each case builds one.
func TestNodeHierarchyWellFormedIsFailable(t *testing.T) {
	t.Parallel()
	const id = "node-hierarchy-well-formed"

	// A well-formed two-level hierarchy over the fixture: an object-category
	// node (tier 5) parented to a fund-group node (tier 2) that exists.
	wellFormed := func(t *testing.T) *Subject {
		t.Helper()
		s := testSubject(t)
		g := s.Projections[0].Graph
		child, parent := "", ""
		for _, n := range g.Nodes {
			if n.Tier == 5 && child == "" {
				child = n.ID
			}
			if n.Tier == 2 && parent == "" {
				parent = n.ID
			}
		}
		if child == "" || parent == "" {
			t.Fatal("the fixture has no tier-5 and tier-2 pair to hang a hierarchy on")
		}
		nodePointer(t, g, child).Parent = parent
		return s
	}

	if res := resultFor(t, runChecks(t, wellFormed(t)), id); res.Status != StatusPass {
		t.Fatalf("a well-formed hierarchy is %s, want pass: %v", res.Status, res.Findings)
	}

	cases := []struct {
		name   string
		damage func(t *testing.T, s *Subject)
		want   string
	}{
		{
			name: "a parent that is not a node of this document",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				nodePointer(t, g, tierNode(t, g, 5)).Parent = "fund-group/nowhere"
			},
			want: "not a node of this document",
		},
		{
			// The fold runs coarse to fine; a coarse node under a fine one
			// inverts it.
			name: "a parent at a finer tier than its child",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				nodePointer(t, g, tierNode(t, g, 2)).Parent = tierNode(t, g, 5)
			},
			want: "strictly coarser",
		},
		{
			// Only a flow endpoint drawn as a container may hold a node at its
			// own tier; two fund groups may not fold into each other.
			name: "a parent at its child's own tier that is not a container",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				var at2 []string
				for _, n := range g.Nodes {
					if end := slices.Contains(project.Endpoints(), n.ID); n.Tier == 2 && !end {
						at2 = append(at2, n.ID)
					}
				}
				if len(at2) < 2 {
					t.Fatal("the fixture has fewer than two fund groups")
				}
				nodePointer(t, g, at2[0]).Parent = at2[1]
			},
			want: "strictly coarser",
		},
		{
			// A container endpoint other than transfers/out may not hold a node
			// at its own tier.
			name: "a node at its container's own tier under an endpoint that holds none",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				g.Nodes = append(g.Nodes, project.Node{ID: "fund-balance/reserve-increase", Tier: 5})
				for _, n := range g.Nodes {
					if end := slices.Contains(project.Endpoints(), n.ID); n.Tier == 5 && !end {
						nodePointer(t, g, n.ID).Parent = "fund-balance/reserve-increase"
						return
					}
				}
				t.Fatal("the fixture has no ordinary tier-5 node")
			},
			want: "strictly coarser",
		},
		{
			// Node.Parent is one string, so a cycle needs two nodes.
			name: "a node that is its own ancestor",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				a, b := tierNode(t, g, 5), tierNode(t, g, 2)
				nodePointer(t, g, a).Parent = b
				nodePointer(t, g, b).Parent = a
			},
			want: "its own ancestor",
		},
		{
			// transfers/in and the fund-balance nodes are ends of a flow, not
			// levels of a fold: nothing aggregates into them.
			name: "a node parented to a flow endpoint",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				nodePointer(t, g, tierNode(t, g, 2)).Parent = project.NodeFundBalanceDraw
			},
			want: "flow endpoint",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testSubject(t)
			c.damage(t, s)
			res := resultFor(t, runChecks(t, s), id)
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want fail", res.Status)
			}
			if !hasFinding(res, c.want) {
				t.Errorf("findings %v do not mention %q", res.Findings, c.want)
			}
		})
	}
}

// tierNode is the id of some node at the given tier, so the cases above damage
// the fixture's shape rather than a node id spelled into the test.
func tierNode(t *testing.T, g *project.Document, tier int) string {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Tier == tier {
			return n.ID
		}
	}
	t.Fatalf("the fixture carries no node at tier %d", tier)
	return ""
}

// TestConstraintTierComesFromTheFile: a node may carry only a tier funds.yaml
// uses, not every tier registry.go declares.
func TestConstraintTierComesFromTheFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, tier string
		status     Status
	}{
		{"a tier the file uses", "discretionary", StatusPass},
		{"a declared tier no fund uses", "committed", StatusFail},
		{"not a tier at all", "whatever", StatusFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSubject(t)
			// Disclosure set so these cases are about the vocabulary alone.
			tierNodeWithDisclosure(t, s, tt.tier)
			res := resultFor(t, runChecks(t, s), "constraint-tier-vocabulary")

			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != 1 {
				t.Errorf("subjects = %d, want 1", res.Subjects)
			}
		})
	}
}

// tierNodeWithDisclosure hangs a constraint tier on the fixture's fund-balance
// node, with the source note and rationale the contract requires beside it.
func tierNodeWithDisclosure(t *testing.T, s *Subject, tier string) {
	t.Helper()
	n := nodePointer(t, s.Projections[0].Graph, project.NodeFundBalanceDraw)
	n.ConstraintTier = tier
	n.SourceNote = "data/funds.yaml, our reading of Budget Book pp.258-261"
	n.Rationale = "A restriction note read off the narrative."
	s.Projections[0].Graph.Metadata.Caveats = append(
		s.Projections[0].Graph.Metadata.Caveats, project.ConstraintTierCaveat())
}

// TestAConstraintTierWithoutItsDisclosureIsAFinding: a constraint tier is our
// reading of pp.258-261 on a published fund node, and must carry its
// disclosure (fisc-yor).
func TestAConstraintTierWithoutItsDisclosureIsAFinding(t *testing.T) {
	t.Parallel()
	const id = "constraint-tier-vocabulary"
	cases := []struct {
		name  string
		strip func(*testing.T, *Subject)
		want  string
	}{
		{"no source note", func(t *testing.T, s *Subject) {
			nodePointer(t, s.Projections[0].Graph, project.NodeFundBalanceDraw).SourceNote = ""
		}, "no source_note"},
		{"no rationale", func(t *testing.T, s *Subject) {
			nodePointer(t, s.Projections[0].Graph, project.NodeFundBalanceDraw).Rationale = ""
		}, "no rationale"},
		{"the document does not disclose", func(_ *testing.T, s *Subject) {
			s.Projections[0].Graph.Metadata.Caveats = nil
		}, "does not carry the disclosure sentence"},
		// Text drift with the id intact: matching on the id alone goes green.
		// Mutation: rewrite this arm as `case false:` and this goes green.
		{"the disclosure is present under the right id and says something else",
			func(_ *testing.T, s *Subject) {
				c := project.ConstraintTierCaveat()
				c.Text = "Constraint tiers come from the budget book."
				s.Projections[0].Graph.Metadata.Caveats = []project.Caveat{c}
			}, "not the disclosure internal/project declares"},
		// Summary drift: index.html prints the summary and links to the text.
		// Mutation: rewrite this arm as `case false:` and this goes green.
		{"the summary is not the one internal/project declares",
			func(_ *testing.T, s *Subject) {
				c := project.ConstraintTierCaveat()
				c.Summary = "Fund restrictions, as printed by the city."
				s.Projections[0].Graph.Metadata.Caveats = []project.Caveat{c}
			}, "whose SUMMARY is not the one"},
		// Mutation: drop the theme arm of constraint-tier-vocabulary and
		// this goes green.
		{"the disclosure is filed under a theme no page states",
			func(_ *testing.T, s *Subject) {
				c := project.ConstraintTierCaveat()
				c.Theme = project.ThemeDataFiles
				s.Projections[0].Graph.Metadata.Caveats = []project.Caveat{c}
			}, "under theme \"data-files\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testSubject(t)
			tierNodeWithDisclosure(t, s, "discretionary")
			c.strip(t, s)
			res := resultFor(t, runChecks(t, s), id)
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want fail: %s", res.Status, res.Summary)
			}
			if !hasFinding(res, c.want) {
				t.Errorf("findings %v do not mention %q", res.Findings, c.want)
			}
		})
	}
}

// factsSubject builds a subject with no projection, for fact-store checks over
// facts internal/project would refuse.
func factsSubject(t *testing.T, facts []fact.Fact) *Subject {
	t.Helper()
	return &Subject{Facts: facts, Vocabulary: testVocabulary(t)}
}

// cellsSubject is factsSubject over cells.
func cellsSubject(t *testing.T, cells []testCell) *Subject {
	t.Helper()
	return factsSubject(t, testFacts(cells...))
}

// nodePointer returns a pointer to the node with an id, so a test can tamper
// with it.
func nodePointer(t *testing.T, g *project.Document, id string) *project.Node {
	t.Helper()
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	t.Fatalf("no node %q in %d nodes", id, len(g.Nodes))
	return nil
}

// linkFrom returns a pointer to the one link leaving a node.
func linkFrom(t *testing.T, g *project.Document, source string) *project.Link {
	t.Helper()
	for i := range g.Links {
		if g.Links[i].Source == source {
			return &g.Links[i]
		}
	}
	t.Fatalf("no link out of %q in %d links", source, len(g.Links))
	return nil
}

// stockFactID is the id of a fact no link cites: one of the two stock rows.
func stockFactID(g *project.Document) string {
	cited := map[string]bool{}
	for _, l := range g.Links {
		for _, id := range l.FactIDs {
			cited[id] = true
		}
	}
	for _, c := range fixtureCells {
		if c.category == "fund-balance/beginning" {
			if id := c.fact().ID; !cited[id] {
				return id
			}
		}
	}
	return ""
}

// internalServiceCells is a schedule with an internal-service column, which the
// main fixture lacks; the external headline figures exclude it.
var internalServiceCells = []testCell{
	{vocab.KindRevenue, "taxes/property", "general", 100_000},
	{vocab.KindRevenue, "charges-for-services", "internal-service", 5_000},
	{vocab.KindExpenditure, "wages-and-benefits", "general", 70_000},
	{vocab.KindExpenditure, "wages-and-benefits", "internal-service", 3_000},
}

// TestHeadlineExternalFiguresExcludeInternalService: the four headline figures
// over a schedule where gross and external differ.
func TestHeadlineExternalFiguresExcludeInternalService(t *testing.T) {
	t.Parallel()
	s := testSubject(t, testFacts(internalServiceCells...)...)
	h := s.Projections[0].Graph.Metadata.Headline

	for _, tt := range []struct {
		key       string
		got, want int64
	}{
		{"all_funds_gross_revenue_cents", h.AllFundsGrossRevenueCents, 105_000},
		{"external_revenue_cents", h.ExternalRevenueCents, 100_000},
		{"all_funds_gross_expenditure_cents", h.AllFundsGrossExpenditureCents, 73_000},
		{"external_expenditure_cents", h.ExternalExpenditureCents, 70_000},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.key, tt.got, tt.want)
		}
	}

	res := resultFor(t, runChecks(t, s), "headline-ties-to-facts")
	if res.Status != StatusPass || res.Subjects != 4 {
		t.Fatalf("status = %s over %d (%s), want pass over 4", res.Status, res.Subjects, res.Summary)
	}
}

// TestEveryHeadlineFigureIsTiedToTheFacts: each headline figure is tied to the
// facts.
func TestEveryHeadlineFigureIsTiedToTheFacts(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		key   string
		shift func(*project.Headline)
	}{
		{"all_funds_gross_revenue_cents", func(h *project.Headline) { h.AllFundsGrossRevenueCents += 99_900 }},
		{"all_funds_gross_expenditure_cents", func(h *project.Headline) { h.AllFundsGrossExpenditureCents += 99_900 }},
		{"external_revenue_cents", func(h *project.Headline) { h.ExternalRevenueCents += 99_900 }},
		{"external_expenditure_cents", func(h *project.Headline) { h.ExternalExpenditureCents -= 99_900 }},
	} {
		t.Run(tt.key, func(t *testing.T) {
			s := testSubject(t, testFacts(internalServiceCells...)...)
			tt.shift(s.Projections[0].Graph.Metadata.Headline)
			res := resultFor(t, runChecks(t, s), "headline-ties-to-facts")

			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if !hasFinding(res, tt.key) {
				t.Errorf("findings %v do not name %s", res.Findings, tt.key)
			}
			// The sign of the difference is in the message.
			if !hasFinding(res, "999.00)") {
				t.Errorf("findings %v do not state the difference", res.Findings)
			}
		})
	}
}

// TestNaiveExpenditureIgnoresInternalServiceRevenue: an internal-service
// revenue flow alone is no expenditure double count, so the check does not
// fail a naive figure equal to the external one.
func TestNaiveExpenditureIgnoresInternalServiceRevenue(t *testing.T) {
	t.Parallel()
	cells := []testCell{
		{vocab.KindRevenue, "charges-for-services", "internal-service", 5_000},
		{vocab.KindRevenue, "taxes/property", "general", 100_000},
		{vocab.KindExpenditure, "wages-and-benefits", "general", 70_000},
	}
	s := testSubject(t, testFacts(cells...)...)
	if !hasLinkKind(s.Projections[0].Graph, project.KindInternalService) {
		t.Fatal("the fixture carries no internal_service link, so this test covers nothing")
	}
	h := s.Projections[0].Graph.Metadata.Headline
	if h.InternalTransferOutCents != 0 || h.ExternalExpenditureCents != h.NaiveExpenditureCents {
		t.Fatalf("the fixture does not have the shape this test needs: %+v", h)
	}

	res := resultFor(t, runChecks(t, s), "headline-naive-expenditure")
	if res.Status != StatusVacuous {
		t.Errorf("status = %s (%s), want vacuous: there is no expenditure double count to net out",
			res.Status, res.Summary)
	}
}

// TestALinkMayOnlyCiteItsOwnSlice: a citation resolving in facts.jsonl but to
// another fiscal year is not provenance for this graph.
func TestALinkMayOnlyCiteItsOwnSlice(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	// A fact of the same shape from the other budget year: in the store, not in
	// this projection's slice.
	other := fixtureCells[0].fact()
	other.ID = "fisc-f-nextyearxxxx"
	other.FiscalYear = testYear + 1
	s.Facts = append(s.Facts, other)
	s.Projections[0].Graph.Links[0].FactIDs = []string{other.ID}

	res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts")
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	detail := said(res)
	if !strings.Contains(detail, "not this projection's slice") {
		t.Errorf("findings %v do not say the citation is out of slice", res.Findings)
	}
	if !strings.Contains(detail, "FY2027") {
		t.Errorf("findings %v do not say which slice the fact belongs to", res.Findings)
	}
}

// TestALinkCitingNothingInTheStoreIsADifferentProblem: an id from another slice
// and one from nowhere get different findings.
func TestALinkCitingNothingInTheStoreIsADifferentProblem(t *testing.T) {
	t.Parallel()
	s := testSubject(t)
	s.Projections[0].Graph.Links[0].FactIDs = []string{"fisc-f-doesnotexist"}
	res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if !hasFinding(res, "not in facts/facts.jsonl") {
		t.Errorf("findings %v do not say the fact is absent from the store", res.Findings)
	}
}

// hasLinkKind reports whether any link is of kind k.
func hasLinkKind(g *project.Document, k project.LinkKind) bool {
	for _, l := range g.Links {
		if l.Kind == k {
			return true
		}
	}
	return false
}

// A projection that refuses to build is reported, not fatal, and the report
// still has every other check in it.
func TestARefusedProjectionIsReportedAndNotFatal(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	facts[0].Department = "patrol"
	rep := runChecks(t, testSubject(t, facts...))

	res := resultFor(t, rep, "projections-build")
	if res.Status != StatusFail {
		t.Fatalf("projections-build = %s (%s), want fail", res.Status, res.Summary)
	}
	if !hasFinding(res, `carries department "patrol"`) {
		t.Errorf("finding %v does not carry internal/project's own refusal", res.Findings)
	}
	// The fact-store checks are unaffected.
	for _, id := range []string{"facts-sorted", "fact-ids-unique", "fact-token-reparses"} {
		if got := resultFor(t, rep, id).Status; got != StatusPass {
			t.Errorf("%s = %s, want pass: a refused projection must not take the report down", id, got)
		}
	}
	if !rep.Failed() {
		t.Error("Failed() = false; a refused projection silences every graph check and must be a failure")
	}
}

// projections-build must never be vacuous while a refusal is recorded: vacuous
// exits 0 without --strict, and a refusal silences the graph checks.
func TestProjectionsBuildIsNeverVacuousWithARefusalRecorded(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	facts[0].Department = "patrol"
	s := testSubject(t, facts...)

	if len(s.ProjectionFailures) == 0 {
		t.Fatal("no projection failure was recorded, so this test proves nothing")
	}
	if got := resultFor(t, runChecks(t, s), "projections-build").Status; got == StatusVacuous {
		t.Error("projections-build = vacuous with a refusal recorded; vacuous exits 0")
	}
}

// projections-build counts slices, the unit it names: one projection may build
// several.
func TestProjectionsBuildCountsSlicesNotProjections(t *testing.T) {
	t.Parallel()
	s := &Subject{Projections: []projection{{
		Name:  "trends",
		Graph: &project.Document{},
		Options: project.Options{
			Columns: []project.Column{
				{FiscalYear: 2024, Basis: "actual"},
				{FiscalYear: 2025, Basis: "revised"},
				{FiscalYear: 2026, Basis: project.PublishedBasis},
				{FiscalYear: 2027, Basis: project.PublishedBasis},
			},
			Scopes: []string{spineScope},
		},
	}}}
	res := runCheck(t, &projectionsBuild{}, s)
	if !strings.Contains(res.Summary, "4 projection slices built") {
		t.Errorf("summary %q does not count the four slices the one projection covers",
			res.Summary)
	}
}

// fundRuleSubject builds a subject carrying rule files and a one-page document
// written here, for rule-funds-match-their-headings.
func fundRuleSubject(t *testing.T, page string, rules []mapping.Rule,
	rollups []mapping.Rollup) *Subject {
	t.Helper()
	s := factsSubject(t, testFacts())
	s.Files = []*mapping.File{{
		SchemaVersion: 1, DocID: testDoc, Rules: rules, Rollups: rollups,
		Path: "testdata/fixture.yaml",
	}}
	s.Docs = map[string]*corpus.Doc{testDoc: inlinePageDoc(t, testDoc, 1, page)}
	// A rule file carries a resolver, as Subject.Resolvers promises.
	r, err := mapping.NewResolver(s.Docs[testDoc], s.Files[0])
	if err != nil {
		t.Fatalf("build the fixture's resolver: %v", err)
	}
	s.Resolvers = map[string]*mapping.Resolver{s.Files[0].Path: r}
	return s
}

// fundRule is one pp.131-140-shaped rule: a fund per rule, four columns all
// naming it, and a printed `Total <fund>` of its own.
func fundRule(id string, fund int, group, totalRow string) mapping.Rule {
	return mapping.Rule{
		ID: id, Kind: vocab.KindRevenue, Basis: vocab.BasisAdopted,
		Scope: project.ScopeRevenueByFund, Units: "dollars", TotalRow: totalRow,
		Parts: []mapping.Part{{Page: 1, Columns: []mapping.Column{
			{FundGroup: group, Fund: fund, FiscalYear: 2026},
		}}},
	}
}

// TestRuleFundsMatchTheirHeadings covers a fund mis-assigned WITHIN its own
// fund type, which changes no column sum and which every other check passes.
func TestRuleFundsMatchTheirHeadings(t *testing.T) {
	t.Parallel()
	const id = "rule-funds-match-their-headings"
	// data/funds.yaml's fixture: 100 General Fund (general), 500 Water Utility
	// Fund (enterprise), 700 Information Technology Fund (internal-service).
	const page = "" +
		"      Total Water Utility Fund              1,000\n" +
		"      Total Information Technology Fund     2,000\n"

	// The swapped fund exists and matches the columns' fund group, and the
	// column sums are identical, so every other check passes; only the printed
	// name differs. The mismatch is one finding, not also "anchored to nothing
	// on the page".
	t.Run("a fund swapped for another of the same type fails once", func(t *testing.T) {
		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{
			// Reads the Water block and files it under Information Technology.
			fundRule("water", 700, "internal-service", "Total Water Utility Fund"),
			fundRule("it", 700, "internal-service", "Total Information Technology Fund"),
		}, nil)), id)
		wantFail(t, res, "mis-assigned inside its own type")
		mismatches := 0
		for _, f := range res.Findings {
			if f.Subject == "water" {
				mismatches++
			}
		}
		if mismatches != 1 {
			t.Errorf("rule \"water\" has %d findings, want 1: the mismatch and the "+
				"unanchored arm are two answers to one question\n%s",
				mismatches, said(res))
		}
		if hasFinding(res, "anchored to nothing on the page") {
			t.Errorf("a rule whose anchor resolved is reported as anchored to nothing:\n%s",
				said(res))
		}
	})

	// A rule mapping a fund section with fund_group-only columns (p76's shape)
	// still claims its own printed total.
	noFund := fundRule("it", 700, "internal-service", "Total Information Technology Fund")
	noFund.Parts[0].Columns[0].Fund = 0 // the group is declared, the fund is not
	// pp.127-130's shape: one fund split by category across rules, named once
	// by the rollup that covers them.
	const gfPage = "" +
		"      Total Property Taxes                 1,000\n" +
		"      Total Sales Taxes                    2,000\n" +
		"      Total General Fund                   3,000\n"
	gfRules := []mapping.Rule{
		fundRule("prop", 100, "general", "Total Property Taxes"),
		fundRule("sales", 100, "general", "Total Sales Taxes"),
	}
	gfRollup := func(covers ...string) []mapping.Rollup {
		return []mapping.Rollup{{ID: "gf", Page: 1, TotalRow: "Total General Fund", Covers: covers}}
	}
	for _, c := range []struct {
		name     string
		page     string
		rules    []mapping.Rule
		rollups  []mapping.Rollup
		status   Status
		subjects int    // for a pass
		finding  string // for a fail; empty asserts the status alone
	}{
		{name: "a rule whose printed total names its fund passes", page: page, rules: []mapping.Rule{
			fundRule("water", 500, "enterprise", "Total Water Utility Fund"),
			fundRule("it", 700, "internal-service", "Total Information Technology Fund"),
		}, status: StatusPass, subjects: 2},
		{name: "a printed name no fund claims fails",
			page:   "      Total Wastewater Utility            1,000\n",
			rules:  []mapping.Rule{fundRule("ww", 500, "enterprise", "Total Wastewater Utility")},
			status: StatusFail, finding: "anchored to nothing on the page"},
		// Clause 2: nine of the schedule's 69 funds print zero in both years, so
		// dropping one moves no sum, and clause 1 inspects only rules that exist.
		{name: "a printed fund total no rule claims fails", page: page,
			rules:  []mapping.Rule{fundRule("water", 500, "enterprise", "Total Water Utility Fund")},
			status: StatusFail, finding: "no rule or rollup declares that total"},
		{name: "a rule with no fund of its own still claims its printed total", page: page,
			rules:  []mapping.Rule{fundRule("water", 500, "enterprise", "Total Water Utility Fund"), noFund},
			status: StatusPass, subjects: 1},
		{name: "a fund named only by a rollup that covers the rules passes", page: gfPage,
			rules: gfRules, rollups: gfRollup("prop", "sales"), status: StatusPass, subjects: 2},
		// The rollup covering only one of them: the uncovered rule is anchored
		// to nothing printed.
		{name: "a rule the naming rollup does not cover fails", page: gfPage,
			rules: gfRules, rollups: gfRollup("prop"), status: StatusFail},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := resultFor(t, runChecks(t, fundRuleSubject(t, c.page, c.rules, c.rollups)), id)
			if c.status == StatusPass {
				wantPass(t, res, c.subjects)
			} else {
				wantFail(t, res, c.finding)
			}
		})
	}

	// A two-fund rule is held to a heading naming them in column order, as
	// p81 heads "Airport Fund" and "Water Fund".
	mixed := func(heading string) Result {
		ru := fundRule("mixed", 500, "enterprise", "Total")
		ru.Parts[0].Columns = append(ru.Parts[0].Columns,
			mapping.Column{FundGroup: "internal-service", Fund: 700, FiscalYear: 2026})
		return resultFor(t, runChecks(t, fundRuleSubject(t, heading+page, []mapping.Rule{ru}, nil)), id)
	}
	t.Run("a rule whose columns declare two funds its page heads in order passes", func(t *testing.T) {
		if res := mixed("      Water Utility Fund     Information Technology Fund\n"); res.Status != StatusPass &&
			hasFinding(res, "more than one fund") {
			t.Fatalf("status = %s, findings %v", res.Status, res.Findings)
		}
	})
	t.Run("a rule whose columns declare two funds in the other order fails", func(t *testing.T) {
		res := mixed("      Information Technology Fund     Water Utility Fund\n")
		if res.Status != StatusFail || !hasFinding(res, "more than one fund") {
			t.Errorf("status = %s, findings %v; want the mixed columns named", res.Status, res.Findings)
		}
	})

	// The spine names no fund, so the report is vacuous.
	t.Run("no rule declares a fund is vacuous", func(t *testing.T) {
		res := resultFor(t, runChecks(t, testSubject(t)), id)
		if res.Status != StatusVacuous {
			t.Fatalf("status = %s (%s), want vacuous", res.Status, res.Summary)
		}
	})
}

// TestAnEndpointCarryingNoFlowMayBeAParent covers node-hierarchy-well-formed's
// one exception over the committed corpus, which has the transfers-by-fund
// document testSubject lacks.
func TestAnEndpointCarryingNoFlowMayBeAParent(t *testing.T) {
	t.Parallel()
	const id = "node-hierarchy-well-formed"
	// Spelled rather than imported, so the two cannot agree by construction.
	const endpoint = "transfers/in"

	s := isolated(t)
	var doc *project.Document
	for i := range s.Projections {
		if d := s.Projections[i].Graph; d != nil && s.Projections[i].Name == project.TransfersByFundProjection {
			doc = d
			break
		}
	}
	if doc == nil {
		t.Fatal("the committed corpus builds no transfers-by-fund document, so this test " +
			"would assert the exception over a shape that is not there")
	}

	// Both halves of the exception, or the arm below is not what is under test.
	folded := 0
	for _, n := range doc.Nodes {
		if n.Parent == endpoint {
			folded++
		}
	}
	if folded == 0 {
		t.Fatalf("no node of %s folds into %q, so the exception has no subject",
			doc.Projection, endpoint)
	}
	for _, l := range doc.Links {
		if l.Source == endpoint || l.Target == endpoint {
			t.Fatalf("%s draws a link at %q (%s -> %s), so this document is not the "+
				"container case the exception is for", doc.Projection, endpoint, l.Source, l.Target)
		}
	}
	if res := resultFor(t, runChecks(t, s), id); res.Status != StatusPass {
		t.Fatalf("status = %s over the committed corpus, want pass: %s",
			res.Status, said(res))
	}

	// Mutation: one link at the endpoint, and each folded node is refused.
	doc.Links = append(doc.Links, project.Link{
		Source: endpoint, Target: doc.Nodes[0].ID, Kind: project.KindInternalTransfer,
	})
	res := resultFor(t, runChecks(t, s), id)
	if res.Status != StatusFail {
		t.Fatalf("status = %s with a link drawn at %q, want fail", res.Status, endpoint)
	}
	if got := strings.Count(said(res), "a flow endpoint this document draws a flow at"); got != folded {
		t.Errorf("%d findings name the endpoint, want one per folded node (%d): %s",
			got, folded, said(res))
	}
}

// scheduleSubject is one document of each of the four schedule shapes, each
// over one cited fact and one uncited printed zero.
func scheduleSubject() *Subject {
	col := project.Column{FiscalYear: 2026, Basis: vocab.BasisAdopted}
	mk := func(id, scope string, cents int64) fact.Fact {
		return fact.Fact{ID: id, Scope: scope, Kind: vocab.KindRevenue,
			RowLabel: "Police", Category: "wages-and-benefits", FiscalYear: col.FiscalYear,
			Basis: col.Basis, AmountCents: cents}
	}
	link := func(cited string) []project.Link {
		return []project.Link{{Source: "expenditure/wages-and-benefits", Target: "dept/police",
			ValueCents: 100, FactIDs: []string{cited}}}
	}
	nodes := []project.Node{{ID: "expenditure/wages-and-benefits"}, {ID: "dept/police"}}
	facts := []fact.Fact{
		mk("ff-a", project.TrendsScope, 100), mk("ff-z", project.TrendsScope, 0),
		mk("ds-a", project.DepartmentSpendingScope, 100), mk("ds-z", project.DepartmentSpendingScope, 0),
		mk("df-a", project.DepartmentFundingScope, 100), mk("df-z", project.DepartmentFundingScope, 0),
		mk("tb-a", project.TransfersByFundScope, 100), mk("tb-z", project.TransfersByFundScope, 0),
	}
	options := func(scopes []string) project.Options {
		return project.Options{Columns: []project.Column{col}, Scopes: scopes, Version: testVersion}
	}
	doc := func(cited string) *project.Document {
		return &project.Document{Nodes: nodes, Links: link(cited),
			Metadata: project.Metadata{Counts: project.Counts{
				Facts: 2, FactsCited: 1, FactsUncited: 1, Nodes: 2, Links: 1}}}
	}
	return &Subject{
		Facts: facts,
		Projections: []projection{
			{Name: project.FundFlowsProjection, Options: options(project.FundFlowsScopes()), Graph: doc("ff-a")},
			{Name: project.DepartmentSpendingProjection, Options: options(project.DepartmentSpendingScopes()), Graph: doc("ds-a")},
			{Name: project.DepartmentFundingProjection, Options: options(project.DepartmentFundingScopes()), Graph: doc("df-a")},
			{Name: project.TransfersByFundProjection, Options: options(project.TransfersByFundScopes()), Graph: doc("tb-a")},
		},
	}
}
