package check

import (
	"context"
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
)

// TestFixtureVerdicts is the one test that states what every check concludes over
// a corpus small enough to count by hand: 10 facts in 10 cells, of which 2 are
// stocks and 1 is a zero, leaving 7 links over 9 nodes.
//
// The subject counts are asserted alongside the statuses because they are the
// substance of the vacuous distinction. A check reporting "pass" over 0 subjects
// is the failure this package exists to prevent, and only the number says which
// one happened.
func TestFixtureVerdicts(t *testing.T) {
	rep := runChecks(t, testSubject(t))

	want := map[string]string{
		// The structural checks have nothing to look at here, and that is a
		// property of the fixture rather than of the checks. This fixture is a
		// miniature of the SPINE — ten cells and the graph they make — and it
		// carries no extraction directory and no source registry, because the
		// claims those checks make are about a repository on disk. They get their
		// own fixture, in structural_test.go, which is a repository: it is
		// mutated file by file, and every one of these four is failed there. The
		// committed corpus is where they pass (TestTheCommittedCorpusVacuitySplit).
		"artifacts-match-manifest":         "vacuous over 0",
		"extraction-emitted-every-page":    "vacuous over 0",
		"extractor-reported-no-errors":     "vacuous over 0",
		"manifest-matches-source-registry": "vacuous over 0",
		"extraction-toolchain-pinned":      "vacuous over 0",
		"source-pdfs-match-both-records":   "skipped over 0",

		"facts-sorted":                "pass over 10",
		"fact-ids-unique":             "pass over 10",
		"fact-ids-recompute":          "pass over 10",
		"fact-token-reparses":         "pass over 10",
		"fact-offset-points-at-token": "pass over 10",
		"fact-vocabulary":             "pass over 20", // 10 categories + 10 fund groups
		"fact-kind-matches-category":  "pass over 10",
		// Vacuous over the FIXTURE and passing over the committed corpus, and
		// the difference is the fixture's own shape rather than a gap: the
		// miniature spine carries one scope, so there is no pair of scopes for
		// a projection to restate. TestTheCommittedCorpusVacuitySplit is where
		// this check's real verdict is pinned.
		"projection-scopes-are-disjoint": "vacuous over 0",
		"projections-build":              "pass over 1",
		"published-projection-built":     "pass over 1",
		// Two projections are registered, but the fixture is a miniature of the
		// SPINE and the trends projection is of nothing here, so one document is
		// built and one document is examined.
		"documents-are-checked":   "pass over 1",
		"facts-are-projected":     "pass over 10",
		"graph-acyclic":           "pass over 7",
		"node-tiers-are-declared": "pass over 9",
		// The drill-down's two checks are vacuous over the miniature spine,
		// which carries neither of the schedules it draws.
		"fund-flows-counts-reconcile":     "vacuous over 0",
		"derived-nodes-justified":         "pass over 2",
		"link-locators-match-their-facts": "pass over 7",
		"link-values-tie-to-facts":        "pass over 7",
		"link-kinds-match-their-facts":    "pass over 7",
		"counts-reconcile":                "pass over 1",
		"headline-ties-to-facts":          "pass over 5", // 3 revenue + 1 expenditure + 1 transfer out
		"headline-transfer-residual":      "pass over 2",
		"headline-naive-expenditure":      "pass over 1",
		// Nothing to check: no link carries a transfer_id, no node a parent or a
		// constraint tier, no fact a department or a fund number.
		"transfer-legs-pair": "vacuous over 0",
		// pp.167-170 and pp.127-140 are mapped, but this fixture is a miniature
		// of the SPINE and carries none of their facts, so there is no detail to
		// reconcile in either lane.
		"expenditure-detail-ties-to-spine": "vacuous over 0",
		"revenue-detail-ties-to-spine":     "vacuous over 0",
		"transfers-detail-ties-to-spine":   "vacuous over 0",
		// Same reason one step further on: no revenue-by-fund fact means the
		// trends projection declares no slice, builds no document, and there is
		// neither a point nor a series to examine.
		"trend-points-tie-to-facts":  "vacuous over 0",
		"trend-series-are-complete":  "vacuous over 0",
		"node-hierarchy-well-formed": "vacuous over 0",
		"constraint-tier-vocabulary": "vacuous over 0",
		"fact-departments-resolve":   "vacuous over 0",
		"fact-funds-resolve":         "vacuous over 0",
		// No rule file in the fixture subject declares a fund, so there is no
		// hand-typed number to check against a printed name.
		"rule-funds-match-their-headings": "vacuous over 0",
		// The fixture's funds are per column, which is the other check's case.
		"row-funds-match-their-anchors": "vacuous over 0",
	}
	if diff := cmp.Diff(want, statuses(rep)); diff != "" {
		t.Errorf("verdicts mismatch (-want +got):\n%s", diff)
	}
	if got := (Counts{Pass: 21, Vacuous: 19, Skipped: 1}); got != rep.Counts {
		t.Errorf("counts = %+v, want %+v", rep.Counts, got)
	}
	// The counts are pinned as numbers above rather than spelled in words here,
	// because a sentence naming them goes stale the first time a check is added
	// and nothing makes it go red. What this line is for is the property the
	// numbers cannot state: a run with nothing WRONG in it exits 0, however many
	// of its checks had nothing to look at.
	if rep.Failed() {
		t.Error("Failed() = true for a report with no failure, error or --strict")
	}
}

// TestVacuousFailsOnlyUnderStrict pins the policy the whole status model turns
// on. A vacuous check is not a pass and not a failure by default, because a gate
// that is red until the corpus is finished gets commented out; --strict is how a
// run demands that nothing be left unchecked.
func TestVacuousFailsOnlyUnderStrict(t *testing.T) {
	s := testSubject(t)
	lenient := Run(t.Context(), s, All(), ReportOptions{})
	strict := Run(t.Context(), s, All(), ReportOptions{Strict: true})

	if lenient.Counts.Vacuous != 19 {
		t.Fatalf("vacuous count = %d, want 19", lenient.Counts.Vacuous)
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
	rep := runChecks(t, testSubject(t))
	for id, want := range map[string]string{
		"transfer-legs-pair":         "no link carries a transfer_id",
		"node-hierarchy-well-formed": "no node carries a parent",
		"constraint-tier-vocabulary": "no node carries a constraint_tier",
		"fact-departments-resolve":   "no fact carries a department",
		"fact-funds-resolve":         "no fact names a fund",
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

// TestUnsortedFactStoreFails is the file-level version of the check `fisc build`
// makes on the way out. A hand-edited or differently-ordered facts.jsonl is
// caught here and nowhere else.
func TestUnsortedFactStoreFails(t *testing.T) {
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
	// A failing check must not borrow the sentence written for the passing case.
	// "FAIL facts-sorted: 240 facts, in canonical order" is a contradiction, and
	// it is what the report said before conclusion.unit existed.
	if want := "1 finding over 10 facts"; res.Summary != want {
		t.Errorf("summary = %q, want %q", res.Summary, want)
	}
}

// TestKindMatchesCategorySummariesArePinned covers the two sentences
// fact-kind-matches-category can publish, neither of which was asserted by
// anything (fisc-9nw part 2).
//
// A check that quietly shrinks its own denominator while still printing a pass
// line is the coverage.go incident fisc-u2v records. The pass summary carries
// the count, so pinning it is what makes a silent drop visible; the vacuous
// sentence was unreachable in any test at all, which means the wording a
// reader sees when the check has nothing to say had never been read.
func TestKindMatchesCategorySummariesArePinned(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		res := resultFor(t, runChecks(t, testSubject(t)), "fact-kind-matches-category")
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if want := "10 facts over 8 kind/category pairs, each kind one its category " +
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

// TestAnUnassignableCategoryReddensOneVocabularyCheck is fisc-9nw part 3, and
// it is the condition that was wrong rather than the comment.
//
// `taxes` is a rollup: assignable: false, so no rule may write it as a
// category. factVocabulary reports that precisely, with the fix. Until
// 2026-08-29 fact-kind-matches-category excluded only categories the taxonomy
// does not DEFINE, so it reddened as well -- and its finding said the kind was
// not among the category's declared kinds, which sends a reader to fix a
// `kinds:` list that is not the problem.
//
// The surviving check must PASS over the remaining facts rather than go
// vacuous: this package distinguishes "checked and held" from "had nothing to
// check", and collapsing the two is how a hole gets reported as a pass.
func TestAnUnassignableCategoryReddensOneVocabularyCheck(t *testing.T) {
	facts := testFacts()
	// BOTH FIELDS, and the kind is the load-bearing one. The fixture taxonomy
	// declares taxes with kinds: [revenue] and facts[0] is a revenue cell, so
	// setting only the category leaves declaresKind TRUE -- the pre-fix build
	// emitted no finding either, and this test passed in both. It has to be a
	// kind `taxes` does NOT declare for the double-redden to exist at all.
	facts[0].Category = "taxes"
	facts[0].Kind = mapping.KindExpenditure
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
	if want := "9 facts"; !strings.Contains(res.Summary, want) {
		t.Errorf("summary = %q, want it to contain %q; the excluded fact must leave "+
			"the count as well as the findings", res.Summary, want)
	}
}

// TestCollidingIDsFail covers the other fact-store check. Two facts with one id
// mean two rules claim the same cell of the same document.
func TestCollidingIDsFail(t *testing.T) {
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

// TestARelabelledFactNoLongerAddressesItself is fisc-c2j's failability, and the
// mutation is chosen to be the one the OTHER fact-store checks miss.
//
// Editing row_label leaves the store sorted, leaves every id unique, and leaves
// every amount re-derivable from its own token — facts-sorted, fact-ids-unique
// and fact-token-reparses all pass, because none of them relates the id to the
// fields it was hashed over. The line now carries an address that names a row
// the record does not contain, which on a file whose whole purpose is to be
// addressable is the silent failure fisc-28h's decision was made to prevent.
func TestARelabelledFactNoLongerAddressesItself(t *testing.T) {
	facts := testFacts()
	stale := facts[0].ID
	facts[0].RowLabel = "Property Taxes (restated)"
	rep := runChecks(t, testSubject(t, facts...))

	res := resultFor(t, rep, "fact-ids-recompute")
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	// The finding names the STALE id as its subject and the id the published
	// fields now hash to as the remedy, so a reader can tell which of the two
	// is wrong without recomputing anything by hand.
	want := fact.MakeID(facts[0].DocID, facts[0].RuleID, facts[0].RowPath,
		facts[0].RowLabel, facts[0].ColumnPath, facts[0].FiscalYear, facts[0].Basis)
	if got := res.Findings[0]; got.Subject != stale || !strings.Contains(got.Detail, want) {
		t.Errorf("finding = %+v, want subject %s naming %s", got, stale, want)
	}
	// And the neighbours stay green, which is what makes this check worth
	// having rather than a restatement of one of them.
	for _, id := range []string{"facts-sorted", "fact-ids-unique", "fact-token-reparses"} {
		if res := resultFor(t, rep, id); res.Status != StatusPass {
			t.Errorf("%s = %s over a relabelled fact, want pass: this check is not "+
				"redundant with it", id, res.Status)
		}
	}
}

// TestVocabularyCatchesAnUnknownCategory is the reason data/taxonomy.yaml exists:
// a rule that writes `tax/property` where the taxonomy says `taxes/property`
// produces a fact that is confidently wrong and joins to nothing.
func TestVocabularyCatchesAnUnknownCategory(t *testing.T) {
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

// TestVocabularyCatchesARollupCategory is the case Registry.Assignable's doc
// comment draws its distinction for. `taxes` EXISTS — it is the rollup a view
// groups under — and a rule may still never assign it, so reporting only
// "unassignable" would leave the reader to work out which of the two problems
// they have.
func TestVocabularyCatchesARollupCategory(t *testing.T) {
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

// TestVocabularyCatchesAnUnknownFundGroup covers the funds.yaml half. The answer
// comes from the file rather than from a list in Go, so a group no fund is
// recorded under is not a group.
func TestVocabularyCatchesAnUnknownFundGroup(t *testing.T) {
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

// TestATransferCountedAsRevenueIsCaughtAtTheFact is the failure fisc-u2v measured
// as route 3, at the place it is cheapest to name.
//
// pp.131-140 print eleven funds whose one `Total <fund>` covers revenue rows AND a
// Transfers In row, so a rule author without a per-row kind can make the fund tie
// by calling the transfer revenue. `transfers/in` is a real slug and it is
// assignable, so fact-vocabulary passes over it — which is asserted here, because
// the whole reason this check exists is that the two halves are each valid alone.
func TestATransferCountedAsRevenueIsCaughtAtTheFact(t *testing.T) {
	cells := slices.Clone(fixtureCells)
	if cells[4].category != "transfers/in" || cells[4].kind != mapping.KindTransferIn {
		t.Fatalf("fixture cell 4 is %+v, want the transfers/in row", cells[4])
	}
	cells[4].kind = mapping.KindRevenue
	rep := runChecks(t, cellsSubject(t, cells))

	res := resultFor(t, rep, "fact-kind-matches-category")
	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if res.Subjects != 10 {
		t.Errorf("subjects = %d, want all 10 facts", res.Subjects)
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
	// The point of the bead: nothing else says a word about it.
	if got := resultFor(t, rep, "fact-vocabulary").Status; got != StatusPass {
		t.Errorf("fact-vocabulary = %s, want pass: transfers/in is a real assignable slug, "+
			"which is exactly why the pair needed its own check", got)
	}
}

// TestEveryKindIsWrongSomewhere walks the whole cross product the fixture taxonomy
// permits, so the check is not shown failable by one lucky pair.
func TestEveryKindIsWrongSomewhere(t *testing.T) {
	tests := []struct {
		category string
		wrong    mapping.Kind
		declares string
	}{
		{"taxes/property", mapping.KindExpenditure, "revenue"},
		{"wages-and-benefits", mapping.KindRevenue, "expenditure"},
		{"transfers/out", mapping.KindFundBalance, "transfer_out"},
		{"fund-balance/beginning", mapping.KindTransferIn, "fund_balance"},
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
			// HasSuffix, not Contains: the declared kinds are the last thing the
			// message says, and "declares transfer" is a PREFIX of "declares
			// transfer_in". An assertion that holds against a list it did not
			// mean is one that will not fail when it goes stale.
			if want := "which declares " + tt.declares; !strings.HasSuffix(res.Findings[0].Detail, want) {
				t.Errorf("finding %q does not end by saying the category declares exactly %q",
					res.Findings[0].Detail, tt.declares)
			}
		})
	}
}

// TestTheCommittedTaxonomyTellsTheTransferDirectionsApart reads data/taxonomy.yaml
// itself rather than the fixture, because the two used to disagree and the
// disagreement was the defect. Every transfer category declared `kinds: [transfer]`
// — a string that is not one of the five mapping.Kind values — and the `transfers`
// rationale asserted that 'a transfer\'s `kind` is only "transfer"'. That was false
// when it was written: transfer_in and transfer_out predate the file (2e514fb
// against 0c43404). The check had been widened to accept the family; the file is
// corrected instead, and the fixture taxonomy was already the corrected model.
//
// So both transfer errors are one literal comparison over the committed file:
// a transfer counted as revenue (fisc-f0k) and a transfer pointing the wrong way
// (fisc-ttq), the latter being what a declared `transfer` family could never see.
func TestTheCommittedTaxonomyTellsTheTransferDirectionsApart(t *testing.T) {
	reg, err := registry.Load(os.DirFS(filepath.Join(repoRoot(t), "data")))
	if err != nil {
		t.Fatalf("load the committed registries: %v", err)
	}
	cells := []testCell{
		// Correct: each direction under its own category, and out-to-cip, which
		// is a sibling of transfers/out and carries the same kind.
		{mapping.KindTransferIn, "transfers/in", "general", 10_000},
		{mapping.KindTransferOut, "transfers/out", "enterprise", 30_000},
		{mapping.KindTransferOut, "transfers/out-to-cip", "general", 5_000},
		// Wrong family: the p131 Stormwater trap, a Transfers In row called
		// revenue so the printed Total <fund> ties.
		{mapping.KindRevenue, "transfers/in", "enterprise", 3_247_000},
		// Wrong direction: fisc-ttq. This pair resolved clean while transfers/in
		// declared `transfer`, because both directions satisfied the family.
		{mapping.KindTransferOut, "transfers/in", "general", 7_000},
	}
	res := resultFor(t, runChecks(t, &Subject{Facts: testFacts(cells...), Vocabulary: reg}),
		"fact-kind-matches-category")

	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if res.Subjects != len(cells) {
		t.Errorf("subjects = %d, want %d", res.Subjects, len(cells))
	}
	// Exact whole-message equality, not Contains: "which declares transfer" is a
	// prefix of "which declares transfer_in", so a containment assertion here
	// would have held against the broken file and the corrected one alike.
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

// TestDepartmentsResolveAgainstTheRegistry is what replaced this check's
// unconditional error once data/departments.yaml existed (fisc-o15).
//
// It is the clause the other three used to stand in for, and it is the one that
// makes the axis a controlled vocabulary rather than a set of free strings the
// check happens to like the shape of: a well-formed, consistently spelled,
// non-colliding department that names no division still joins to nothing.
//
// The subject is built without a projection: internal/project refuses a
// department-carrying fact outright, which is a different and also correct answer
// to the same problem, and it would otherwise stop this test at Load.
func TestDepartmentsResolveAgainstTheRegistry(t *testing.T) {
	tests := []struct {
		name   string
		dept   string
		status Status
		want   string
	}{
		{"a listed division", "patrol", StatusPass, ""},
		{"a division the registry does not list", "traffic", StatusFail,
			`department "traffic" is not a division data/departments.yaml lists`},
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
			if tt.want != "" && !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
			// A category typo and an unresolved department must not share a
			// verdict, because they will not share a fix — or a
			// reconciliations.yaml entry.
			if got := resultFor(t, rep, "fact-vocabulary").Status; got != StatusPass {
				t.Errorf("fact-vocabulary = %s, want pass: no category or fund group is wrong here", got)
			}
		})
	}
}

// A malformed department is reported ONCE, as the shape problem it is.
//
// It is also absent from the registry — departments.yaml applies the same slug
// rule when it loads, so it could not be there — and reporting both would put two
// findings and one fix against one fact. The specific diagnosis wins.
func TestAMalformedDepartmentIsNotAlsoReportedAsUnlisted(t *testing.T) {
	facts := testFacts()
	facts[0].Department = "Patrol"
	res := resultFor(t, runChecks(t, factsSubject(t, facts)), "fact-departments-resolve")

	if res.Status != StatusFail {
		t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want 1:\n%s", got, findingDetails(res))
	}
	if !strings.Contains(findingDetails(res), "is not a slug") {
		t.Errorf("finding %v is not the shape diagnosis", res.Findings)
	}
	if strings.Contains(findingDetails(res), "is not a division") {
		t.Error("the shape failure is also reported as an unlisted division; " +
			"one fact with one fix must not produce two findings")
	}
}

// TestDepartmentsAreCheckedForWhatIsCheckable covers the three claims that stand
// beside resolution. Each is a real way the axis goes wrong: a label used as a
// slug, one department spelled two ways across the store, and a string that is a
// category on the other axis — the near miss data/taxonomy.yaml exists to prevent.
//
// The last two are belt and braces now that departments.yaml refuses both at load
// time. They stay because this check is written against the Vocabulary interface
// and not against that one implementation, and an assertion implied by a loader is
// not the same thing as one nobody makes.
func TestDepartmentsAreCheckedForWhatIsCheckable(t *testing.T) {
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
			if !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestFundNumbersResolveAgainstTheRegistry covers the axis bd show fisc-1wr.1 names
// and nothing had a caller for: registry.Fund. Every fact carries fund 0 today, so
// the check is vacuous — and fisc-5gk.1 starts emitting real fund numbers, at which
// point the join key has to resolve and has to agree with the fund group beside it.
func TestFundNumbersResolveAgainstTheRegistry(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := testFacts()
			facts[0].Fund = tt.fund
			facts[0].FundGroup = tt.group
			res := resultFor(t, runChecks(t, factsSubject(t, facts)), "fact-funds-resolve")

			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != 1 {
				t.Errorf("subjects = %d, want the 1 fact naming a fund", res.Subjects)
			}
			if tt.want != "" && !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestLinkValueMustEqualItsFacts is the join between the chart and its
// provenance. A wrong value with a working citation is worse than a broken link:
// the reader follows it and lands on rows that do not add up to what they saw.
func TestLinkValueMustEqualItsFacts(t *testing.T) {
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

// TestLinkLocatorsMustBeThePagesOfItsFacts proves the locator check can fail,
// in the two ways a real defect would produce.
//
// This matters more than it looks. Nothing ELSE compares the two citations a
// link publishes: link-values-tie-to-facts reads fact_ids only, and the shard
// a wrong locator points at is a well-formed file either way. So if this check
// could not fail, a mark on the chart could send a reader to a page its figure
// was never printed on and every gate would stay green.
func TestLinkLocatorsMustBeThePagesOfItsFacts(t *testing.T) {
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
		// The finding has to name BOTH pages, or a reader cannot tell whether
		// the locator is wrong or the facts moved.
		for _, want := range []string{"p9999", fmt.Sprintf("p%d", was)} {
			if !strings.Contains(f.Detail, want) {
				t.Errorf("finding %q does not name %s", f.Detail, want)
			}
		}
	})

	// The failure a rule split actually produces: the projection keeps citing
	// the facts and stops citing one of their pages.
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

	// A nil list is not the same as an empty one and gets its own sentence,
	// because it is what a projection that never populated the field at all
	// would publish -- the state every link was in before fisc-5hxr.
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

// TestALinkCitingAnUnresolvableFactIsNamedOnce stops the locator check
// double-reporting. A fact_id that resolves in no slice is already
// link-values-tie-to-facts's finding, with the fix; repeating it here would
// send a reader to chase one edit twice.
func TestALinkCitingAnUnresolvableFactIsNamedOnce(t *testing.T) {
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

// TestFundBalanceDrawKeepsItsSign pins the one link allowed a value that is not
// the plain sum of its facts, and pins it in the direction that matters. The
// draw leg carries the negation because a Sankey cannot draw a negative link, so
// a check written as "value equals the absolute sum" would accept a leg pointing
// the wrong way — which is the error the decomposition can actually make.
func TestFundBalanceDrawKeepsItsSign(t *testing.T) {
	s := testSubject(t)
	draw := linkFrom(t, s.Projections[0].Graph, project.NodeFundBalanceDraw)
	if draw.ValueCents != 20_000 {
		t.Fatalf("the draw link is %d cents, want the negation of the -20000 fact", draw.ValueCents)
	}
	if res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts"); res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
	}

	// Point the same value at the contribution node instead: the magnitude still
	// matches its facts, and it is still wrong.
	draw.Source, draw.Target = "fund-group/general", project.NodeFundBalanceContribution
	if res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts"); res.Status != StatusFail {
		t.Errorf("status = %s, want fail for a draw whose sign was not decomposed", res.Status)
	}
}

// TestCycleIsFoundAndNamed covers the check with no failing case in the shipped
// graph. A cycle is money funding itself, d3-sankey renders one without
// complaint, so nothing downstream would notice.
func TestCycleIsFoundAndNamed(t *testing.T) {
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

// TestDerivedNodeNeedsItsProvenance covers the fourth provenance invariant: a
// figure the city printed and a classification we inferred must not be presented
// alike.
func TestDerivedNodeNeedsItsProvenance(t *testing.T) {
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

// TestCountsMustAccountForEveryFact asserts the identity three ways, because the
// published counts are three separate claims: how many facts the projection was
// of, how many its links cite, and that the difference is exactly the stocks plus
// the zero-valued cells.
func TestCountsMustAccountForEveryFact(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*project.Graph)
		want   string
	}{
		{"facts", func(g *project.Graph) { g.Metadata.Counts.Facts++ }, "counts.facts is 11"},
		{"cited", func(g *project.Graph) { g.Metadata.Counts.FactsCited++ }, "counts.facts_cited is 8"},
		{"a stock row grew a link", func(g *project.Graph) {
			g.Links[0].FactIDs = append(g.Links[0].FactIDs, stockFactID(g))
			g.Metadata.Counts.FactsCited++
		}, "accounts for 11"},
		{"nodes", func(g *project.Graph) { g.Metadata.Counts.Nodes = 99 }, "counts.nodes is 99"},
		{"links", func(g *project.Graph) { g.Metadata.Counts.Links = 99 }, "counts.links is 99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSubject(t)
			tt.tamper(s.Projections[0].Graph)
			res := resultFor(t, runChecks(t, s), "counts-reconcile")

			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("findings %v do not report %q", res.Findings, tt.want)
			}
		})
	}
}

// TestCountsReconcileNamesTheArithmetic keeps the passing case legible: the whole
// value of publishing both counts is that a reader can see where the gap went.
func TestCountsReconcileNamesTheArithmetic(t *testing.T) {
	res := resultFor(t, runChecks(t, testSubject(t)), "counts-reconcile")
	if want := "(10 = 7 cited + 2 stock + 1 zero-valued)"; !strings.Contains(res.Summary, want) {
		t.Errorf("summary %q does not contain %q", res.Summary, want)
	}
}

// TestTransferHeadlineIsTheFacts checks the headline against the fact store
// rather than against itself, which is the only way a headline check means
// anything.
func TestTransferHeadlineIsTheFacts(t *testing.T) {
	res := resultFor(t, runChecks(t, testSubject(t)), "headline-transfer-residual")
	if res.Status != StatusPass || res.Subjects != 2 {
		t.Fatalf("status = %s over %d, want pass over 2", res.Status, res.Subjects)
	}

	s := testSubject(t)
	s.Projections[0].Graph.Metadata.Headline.TransferResidualCents = 0
	res = resultFor(t, runChecks(t, s), "headline-transfer-residual")
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail for a residual that is not out minus in", res.Status)
	}
	if !strings.Contains(findingDetails(res), "transfer_residual_cents is $0.00") {
		t.Errorf("findings %v do not state the residual that was published", res.Findings)
	}
}

// TestNaiveExpenditureMustStayWrong covers the figure internal/project ships
// deliberately wrong. It is worth publishing only while it differs from the real
// one; the day they agree, the page is illustrating nothing.
func TestNaiveExpenditureMustStayWrong(t *testing.T) {
	s := testSubject(t)
	h := &s.Projections[0].Graph.Metadata.Headline
	if h.NaiveExpenditureCents != h.AllFundsGrossExpenditureCents+h.InternalTransferOutCents {
		t.Fatalf("the fixture's naive figure is not gross plus transfers out: %+v", h)
	}
	h.ExternalExpenditureCents = h.NaiveExpenditureCents
	res := resultFor(t, runChecks(t, s), "headline-naive-expenditure")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if !strings.Contains(findingDetails(res), "no longer netting out the double count") {
		t.Errorf("findings %v do not say what agreement would mean", res.Findings)
	}
}

// TestNaiveExpenditureIsVacuousWithoutADoubleCount is the guard that keeps the
// check above from being a trap. The two figures differ by transfers out plus
// internal service charges, so a graph with neither makes the claim one about
// nothing — and a check that failed there would fail a correct projection.
func TestNaiveExpenditureIsVacuousWithoutADoubleCount(t *testing.T) {
	cells := []testCell{
		{mapping.KindRevenue, "taxes/property", "general", 100_000},
		{mapping.KindExpenditure, "wages-and-benefits", "general", 70_000},
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

// TestTransferLegsPairWhenLegsExist covers the check that is vacuous in every
// shipped graph. It has to work the day a link can carry a transfer_id, and the
// only way to know that today is to hand it legs.
func TestTransferLegsPairWhenLegsExist(t *testing.T) {
	tests := []struct {
		name   string
		values []int64
		status Status
		want   string
	}{
		{"two equal legs", []int64{5_000, 5_000}, StatusPass, ""},
		{"two unequal legs", []int64{5_000, 4_000}, StatusFail, "off by $10.00"},
		{"one leg", []int64{5_000}, StatusFail, "names 1 legs, want 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSubject(t)
			g := s.Projections[0].Graph
			for i, v := range tt.values {
				g.Links[i].TransferID = "t-1"
				g.Links[i].ValueCents = v
			}
			res := resultFor(t, runChecks(t, s), "transfer-legs-pair")

			if res.Status != tt.status {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Summary, tt.status)
			}
			if res.Subjects != 1 {
				t.Errorf("subjects = %d, want 1 transfer id", res.Subjects)
			}
			if tt.want != "" && !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestNodeHierarchyWellFormedIsFailable damages a hierarchy four ways, one per
// claim.
//
// THIS REPLACES TestAggregationInvarianceErrorsWhenAHierarchyArrives, which
// asserted the old check's hard error on the first parented node. That error was
// a ratchet on an unwritten fold and it did its job: the fold is now written, and
// it is STRUCTURAL rather than arithmetic. The arithmetic version could not fail
// -- a fold maps each link to exactly one folded link, so the sum is invariant
// under relabelling whatever node.parent says -- and the inter-document version
// is already discharged, per cell and at zero tolerance, by the
// <kind>-detail-ties-to-spine family.
//
// The fixture's spine carries no hierarchy, so each case builds one, which is
// also what proves the check is not merely counting nothing.
func TestNodeHierarchyWellFormedIsFailable(t *testing.T) {
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
			// The fold must run coarse to fine. Parenting a COARSE node to a
			// fine one inverts it, and a client walking the chain renders the
			// hierarchy inside out.
			name: "a parent at a finer tier than its child",
			damage: func(t *testing.T, s *Subject) {
				g := s.Projections[0].Graph
				nodePointer(t, g, tierNode(t, g, 2)).Parent = tierNode(t, g, 5)
			},
			want: "strictly coarser",
		},
		{
			// Node.Parent is one string, so a cycle needs two nodes -- which is
			// exactly why the old doc comment's "double-parented node" could
			// never have been the thing this caught.
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
			if !strings.Contains(findingDetails(res), c.want) {
				t.Errorf("findings %v do not mention %q", res.Findings, c.want)
			}
		})
	}
}

// tierNode is the id of some node at the given tier, so the cases above damage
// the fixture's shape rather than a node id spelled into the test.
func tierNode(t *testing.T, g *project.Graph, tier int) string {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Tier == tier {
			return n.ID
		}
	}
	t.Fatalf("the fixture carries no node at tier %d", tier)
	return ""
}

// TestConstraintTierComesFromTheFile pins where the vocabulary comes from. The
// closed set registry.go declares is the ceiling; the tiers funds.yaml actually
// uses are the floor, and a node may only carry the latter — the same reading
// Registry.FundGroup takes of a fund type.
func TestConstraintTierComesFromTheFile(t *testing.T) {
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
			// The disclosure travels with the tier, which is the other arm of
			// this check: a node carrying one and no source note is a finding
			// whatever the tier says. Set here so these cases are about the
			// VOCABULARY alone.
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

// TestAConstraintTierWithoutItsDisclosureIsAFinding is fisc-yor's requirement
// asserted rather than described.
//
// A FUND NODE IS THE INVERSE OF A DERIVED ONE: the node is published -- the city
// prints the fund and its revenue -- while the constraint tier is our reading of
// the Description of Funds narrative, pp.258-261. So node.derived stays FALSE,
// which means derived-nodes-justified skips the node entirely (graph.go:144) and
// the disclosure has nowhere else to be asserted. Without these three arms the
// site would publish an editorial classification unmarked, which is the single
// thing this project's premise refuses.
func TestAConstraintTierWithoutItsDisclosureIsAFinding(t *testing.T) {
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
			if !strings.Contains(findingDetails(res), c.want) {
				t.Errorf("findings %v do not mention %q", res.Findings, c.want)
			}
		})
	}
}

// factsSubject builds a subject with no projection in it, for the checks that
// are about the fact store alone. Some of the facts these tests construct are
// ones internal/project refuses outright — it will not project a fact carrying a
// department — and building a graph from them would make the test about the
// projection rather than about the vocabulary.
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
func nodePointer(t *testing.T, g *project.Graph, id string) *project.Node {
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
func linkFrom(t *testing.T, g *project.Graph, source string) *project.Link {
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
func stockFactID(g *project.Graph) string {
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

func findingDetails(res Result) string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.Detail)
	}
	return strings.Join(out, "\n")
}

// internalServiceCells is a schedule with an internal-service column, which the
// main fixture deliberately does not have: an Internal Service Fund charge is
// billed by one city department to another, so it is inside the city and the
// external headline figures exclude it.
var internalServiceCells = []testCell{
	{mapping.KindRevenue, "taxes/property", "general", 100_000},
	{mapping.KindRevenue, "charges-for-services", "internal-service", 5_000},
	{mapping.KindExpenditure, "wages-and-benefits", "general", 70_000},
	{mapping.KindExpenditure, "wages-and-benefits", "internal-service", 3_000},
}

// TestHeadlineExternalFiguresExcludeInternalService is the arithmetic behind the
// four headline figures, over a schedule where the gross and external answers
// differ. Getting this wrong is the error the projection exists to prevent, one
// order of magnitude smaller than the transfer double count.
func TestHeadlineExternalFiguresExcludeInternalService(t *testing.T) {
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

// TestEveryHeadlineFigureIsTiedToTheFacts covers the figures a reader quotes
// without reading the chart. Each is accumulated inside internal/project, and
// before this check nothing at all would have noticed one of them being wrong.
func TestEveryHeadlineFigureIsTiedToTheFacts(t *testing.T) {
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
			tt.shift(&s.Projections[0].Graph.Metadata.Headline)
			res := resultFor(t, runChecks(t, s), "headline-ties-to-facts")

			if res.Status != StatusFail {
				t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
			}
			if !strings.Contains(findingDetails(res), tt.key) {
				t.Errorf("findings %v do not name %s", res.Findings, tt.key)
			}
			// The sign of the difference is part of the message: the last case
			// shifts the figure down rather than up.
			if !strings.Contains(findingDetails(res), "999.00)") {
				t.Errorf("findings %v do not state the difference", res.Findings)
			}
		})
	}
}

// TestNaiveExpenditureIgnoresInternalServiceRevenue is a regression test. The
// subject condition once tested for ANY internal_service link, which a projection
// whose only internal-service flow is revenue satisfies while having no
// expenditure double count at all — so a correct graph was failed for publishing a
// naive figure that legitimately equals the external one.
func TestNaiveExpenditureIgnoresInternalServiceRevenue(t *testing.T) {
	cells := []testCell{
		{mapping.KindRevenue, "charges-for-services", "internal-service", 5_000},
		{mapping.KindRevenue, "taxes/property", "general", 100_000},
		{mapping.KindExpenditure, "wages-and-benefits", "general", 70_000},
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

// TestALinkMayOnlyCiteItsOwnSlice covers the failure project.Options exists to
// prevent, from the provenance side. A citation that resolves in facts.jsonl but
// belongs to another fiscal year is not provenance for this graph, and it would be
// accepted by anything that looked the id up in the whole fact store.
func TestALinkMayOnlyCiteItsOwnSlice(t *testing.T) {
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
	detail := findingDetails(res)
	if !strings.Contains(detail, "not this projection's slice") {
		t.Errorf("findings %v do not say the citation is out of slice", res.Findings)
	}
	if !strings.Contains(detail, "FY2027") {
		t.Errorf("findings %v do not say which slice the fact belongs to", res.Findings)
	}
}

// TestALinkCitingNothingInTheStoreIsADifferentProblem keeps the two citation
// failures apart: an id from another slice and an id from nowhere at all need
// different fixes.
func TestALinkCitingNothingInTheStoreIsADifferentProblem(t *testing.T) {
	s := testSubject(t)
	s.Projections[0].Graph.Links[0].FactIDs = []string{"fisc-f-doesnotexist"}
	res := resultFor(t, runChecks(t, s), "link-values-tie-to-facts")

	if res.Status != StatusFail {
		t.Fatalf("status = %s, want fail", res.Status)
	}
	if !strings.Contains(findingDetails(res), "not in facts/facts.jsonl") {
		t.Errorf("findings %v do not say the fact is absent from the store", res.Findings)
	}
}

// hasLinkKind reports whether any link is of kind k.
func hasLinkKind(g *project.Graph, k project.LinkKind) bool {
	for _, l := range g.Links {
		if l.Kind == k {
			return true
		}
	}
	return false
}

// A projection that refuses to build is REPORTED, not fatal — and the report
// still has every other check in it.
//
// Before fisc-o15 this path aborted check.Load, so `fisc verify` exited 1 having
// printed nothing: the reason was wrapped inside a load error and every other
// finding in the run was lost. The refusal itself stays — internal/project is
// right to reject a department-bearing fact, because the citywide spine has no
// department tier — and what changed is that a check now owns saying so.
func TestARefusedProjectionIsReportedAndNotFatal(t *testing.T) {
	facts := testFacts()
	facts[0].Department = "patrol"
	rep := runChecks(t, testSubject(t, facts...))

	res := resultFor(t, rep, "projections-build")
	if res.Status != StatusFail {
		t.Fatalf("projections-build = %s (%s), want fail", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), `carries department "patrol"`) {
		t.Errorf("finding %v does not carry internal/project's own refusal", res.Findings)
	}
	// The checks that read the fact store rather than a graph are unaffected,
	// which is the whole point of reporting instead of aborting.
	for _, id := range []string{"facts-sorted", "fact-ids-unique", "fact-token-reparses"} {
		if got := resultFor(t, rep, id).Status; got != StatusPass {
			t.Errorf("%s = %s, want pass: a refused projection must not take the report down", id, got)
		}
	}
	if !rep.Failed() {
		t.Error("Failed() = false; a refused projection silences every graph check and must be a failure")
	}
}

// One refusal produces ONE finding in facts-are-projected, not one per fact.
//
// Every fact of a refused slice is unprojected, so the naive answer restates a
// single cause ten times here and 120 times on the committed corpus — which is a
// report nobody reads to the end. The verdict is still a failure; only the
// restatement is suppressed.
func TestARefusedProjectionDoesNotDrownTheReport(t *testing.T) {
	facts := testFacts()
	facts[0].Department = "patrol"
	res := resultFor(t, runChecks(t, testSubject(t, facts...)), "facts-are-projected")

	if res.Status != StatusFail {
		t.Fatalf("facts-are-projected = %s (%s), want fail", res.Status, res.Summary)
	}
	if got := len(res.Findings); got != 1 {
		t.Errorf("findings = %d over %d facts, want 1: the cause is one refused projection\n%s",
			got, len(facts), findingDetails(res))
	}
	if !strings.Contains(findingDetails(res), "projections-build") {
		t.Errorf("finding %v does not point at the check that owns the cause", res.Findings)
	}
}

// projections-build must never be VACUOUS while a refusal is recorded.
//
// Vacuous is the verdict that exits 0 without --strict, and a refused projection
// silences ten graph checks at once. This is the arm that would make the whole
// mechanism pointless if it regressed, so it is asserted by name rather than
// left to the status assertions above.
func TestProjectionsBuildIsNeverVacuousWithARefusalRecorded(t *testing.T) {
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

// TWO PROJECTIONS REFUSING ONE SLICE STRAND ITS FACTS ONCE, NOT TWICE.
//
// facts-are-projected used to sum refused[k] over each failure's own columns,
// independently, while `refused` is keyed on (fiscal year, basis, scope). So
// two projections refusing the SAME slice each claimed the full count and a
// reader adding the findings up got twice the facts that exist -- and the count
// is what a reader triages on (fisc-rwo). The slice is what the facts are
// stranded in, so it is what the finding is now about, and both refusals are
// named in it.
func TestTwoProjectionsRefusingOneSliceStrandItsFactsOnce(t *testing.T) {
	facts := testFacts()
	facts[0].Department = "patrol" // refuses the spine slice
	s := testSubject(t, facts...)
	if len(s.ProjectionFailures) != 1 {
		t.Fatalf("recorded %d failures, want the 1 this test doubles", len(s.ProjectionFailures))
	}
	// A second projection refusing the very same slice, which is what a corpus
	// with two graph documents over one year looks like the day both refuse.
	second := s.ProjectionFailures[0]
	second.Name = "sankey-also"
	s.ProjectionFailures = append(s.ProjectionFailures, second)

	res, err := (&factsAreProjected{}).Run(context.Background(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(res.Findings); got != 1 {
		t.Fatalf("findings = %d, want 1: one slice was refused, however many "+
			"projections refused it\n%s", got, findingDetails(res))
	}
	details := findingDetails(res)
	if !strings.Contains(details, fmt.Sprintf("%d facts", len(facts))) {
		t.Errorf("the finding does not count the slice's facts once:\n%s", details)
	}
	// Both refusals are named, or the second one is reported by nothing here.
	if !strings.Contains(details, "sankey-also") {
		t.Errorf("the finding does not name the second projection that refused:\n%s", details)
	}
}

// projections-build COUNTS SLICES, which is the unit it has always named.
//
// It counted PROJECTIONS under the unit "projection slices", and one projection
// stopped being one slice when the first multi-column document landed. Measured
// on the committed corpus before the fix: "3 projection slices built" over six.
// A denominator that is not the thing the unit says is a denominator a reader
// cannot use.
func TestProjectionsBuildCountsSlicesNotProjections(t *testing.T) {
	s := &Subject{Projections: []Projection{{
		Name:  "trends",
		Graph: &project.Graph{},
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
	res, err := (&projectionsBuild{}).Run(context.Background(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Summary, "4 projection slices built") {
		t.Errorf("summary %q does not count the four slices the one projection covers",
			res.Summary)
	}
}

// A refused projection must not hide an undeclared scope somewhere else in the
// store.
//
// Collecting the refused slice's own facts against the refusal is what keeps the
// report readable; suppressing the whole check while any projection failed would
// cost a second fix-and-rerun round for a finding that was already there. One
// aggregate finding for the refusal, one ordinary finding for the fact the
// refusal has nothing to do with.
func TestARefusedProjectionDoesNotHideAnUndeclaredScope(t *testing.T) {
	facts := testFacts()
	facts[0].Department = "patrol" // refuses the spine slice
	facts[1].Scope = "a-scope-nobody-declared"
	res := resultFor(t, runChecks(t, testSubject(t, facts...)), "facts-are-projected")

	if res.Status != StatusFail {
		t.Fatalf("facts-are-projected = %s (%s), want fail", res.Status, res.Summary)
	}
	details := findingDetails(res)
	if !strings.Contains(details, "a-scope-nobody-declared") {
		t.Errorf("findings do not name the undeclared scope:\n%s", details)
	}
	if !strings.Contains(details, "projections-build") {
		t.Errorf("findings do not point at the refusal:\n%s", details)
	}
	// One for the refusal, one for the stray fact — not one per fact of the
	// refused slice.
	if got := len(res.Findings); got != 2 {
		t.Errorf("findings = %d, want 2 (the refusal, and the undeclared scope)\n%s", got, details)
	}
}

// detailFact makes one pp.167-170-shaped fact: General Fund expenditure at the
// department scope, carrying a division.
func detailFact(t *testing.T, category string, cents int64) fact.Fact {
	t.Helper()
	f := testCell{mapping.KindExpenditure, category, "general", cents}.fact()
	f.Scope = expenditureDetailScope
	f.Department = "patrol"
	f.RowPath = "patrol/" + category
	return f
}

// TestExpenditureDetailTiesToTheSpine covers the check's three verdicts, which
// are three different states of the corpus and not three ways of failing.
func TestExpenditureDetailTiesToTheSpine(t *testing.T) {
	// The fixture spine carries wages-and-benefits/general at 70,000 and no
	// other General Fund expenditure.
	const spineWages = 70_000

	t.Run("no detail at all is vacuous", func(t *testing.T) {
		res := resultFor(t, runChecks(t, factsSubject(t, testFacts())),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusVacuous {
			t.Fatalf("status = %s (%s), want vacuous", res.Status, res.Summary)
		}
		// This arm is why the check can be landed at all: the committed fixture
		// holds a spine expenditure cell with no detail counterpart, and under a
		// literal "a spine key with no detail is a failure" it would go red over
		// a corpus with nothing wrong in it.
		if !strings.Contains(res.Summary, expenditureDetailScope) {
			t.Errorf("summary %q does not name the scope that is absent", res.Summary)
		}
	})

	t.Run("a complete detail ties", func(t *testing.T) {
		facts := append(testFacts(), detailFact(t, "wages-and-benefits", spineWages))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if res.Subjects != 1 {
			t.Errorf("subjects = %d, want the 1 cell both scopes carry", res.Subjects)
		}
	})

	t.Run("a detail that does not tie fails", func(t *testing.T) {
		facts := append(testFacts(), detailFact(t, "wages-and-benefits", spineWages+1))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "same money decomposed two ways") {
			t.Errorf("findings %v do not say what the two figures are", res.Findings)
		}
	})

	// THE UNION, and the reason for it. A detail that carries SOME categories
	// must still be measured against every spine key inside the restriction: a
	// category the schedule stopped printing is a dropped rule, not an empty
	// cell, and a loop over the detail's own keys would compare nothing.
	t.Run("a spine key the detail dropped fails", func(t *testing.T) {
		facts := append(testFacts(), detailFact(t, "services-and-supplies", 5_000))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: wages-and-benefits is on the spine and "+
				"not in the detail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "no such row at all") {
			t.Errorf("findings %v do not name the dropped rule\n%s",
				res.Findings, findingDetails(res))
		}
	})

	// THE PAIR THE DETAIL DROPPED. This is the hole a "pairs present in both
	// scopes" reading leaves open, and it is worth its own case because the
	// obvious implementation passes it: seed the reconciled pairs from the
	// DETAIL and a schedule that stops publishing a whole fiscal-year column
	// takes that column out of the check with it. Measured before the fix:
	// status=pass, one cell checked, an entire spine year unreconciled.
	t.Run("a whole spine year the detail dropped fails", func(t *testing.T) {
		facts := testFacts()
		var nextYear []fact.Fact
		for _, f := range facts {
			g := f
			g.FiscalYear = f.FiscalYear + 1
			g.ID = f.ID + "-second-year"
			nextYear = append(nextYear, g)
		}
		facts = append(facts, nextYear...)
		// The detail covers the first year only.
		facts = append(facts, detailFact(t, "wages-and-benefits", spineWages))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: the spine publishes two years of General "+
				"Fund wages and the detail decomposes one", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "no such row at all") {
			t.Errorf("findings %v do not name the missing year\n%s",
				res.Findings, findingDetails(res))
		}
	})

	// The detail's own extra columns are NOT a failure, and this is the
	// asymmetry stated as a test: pp.66-67 print no actual or revised column,
	// so pp.167-170's FY2024 and FY2025 figures are published with nothing to
	// tie to. A gap in the document is not a gap in the mapping.
	t.Run("a detail year the spine never printed is not reconciled", func(t *testing.T) {
		extra := detailFact(t, "wages-and-benefits", 999_999)
		extra.FiscalYear--
		extra.Basis = mapping.BasisActual
		extra.ID += "-actual"
		facts := append(testFacts(),
			detailFact(t, "wages-and-benefits", spineWages), extra)
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if !strings.Contains(res.Summary, "no spine column") {
			t.Errorf("summary %q does not say the extra column is unreconciled; a reader "+
				"counting facts would think the check covers all of them", res.Summary)
		}
	})

	// The converse: detail that the spine has no cell for. A detail scope
	// decomposes the spine; it never extends it.
	t.Run("a detail key the spine does not have fails", func(t *testing.T) {
		facts := append(testFacts(),
			detailFact(t, "wages-and-benefits", spineWages),
			detailFact(t, "capital-outlay", 1_000))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)),
			"expenditure-detail-ties-to-spine")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "never extend it") {
			t.Errorf("findings %v do not say the detail may not extend the spine", res.Findings)
		}
	})
}

// TestUnprojectedScopesAreDeclarations is fisc-u2v's acceptance test D, as
// reworded on 2026-08-22: the map is a list of DECIDED exemptions, not a map of
// a particular size.
//
// The count is deliberately NOT pinned. Three scopes are decided — one per
// mapped non-spine schedule — and a test asserting "exactly one entry" would go
// red the moment the second landed, with deleting the assertion as the cheapest
// way to make it green again. That would retire the only test that reads the map
// at all.
func TestUnprojectedScopesAreDeclarations(t *testing.T) {
	if len(unprojectedScopes) == 0 {
		t.Fatal("unprojectedScopes is empty; pp.167-170 are mapped and must be declared")
	}
	seen := map[string]string{}
	for _, scope := range sortedStrings(unprojectedScopes) {
		reason := unprojectedScopes[scope]
		// A bare "" is a hole with a name on it.
		if strings.TrimSpace(reason) == "" {
			t.Errorf("scope %q is declared with no reason", scope)
		}
		// Copy-paste is how a declaration stops describing its own schedule.
		if prev, dup := seen[reason]; dup {
			t.Errorf("scopes %q and %q share a reason verbatim", prev, scope)
		}
		seen[reason] = scope
		// The reason has to say what the facts ARE, not merely that they are
		// excluded, so a reader can tell a decision from an oversight.
		if !strings.Contains(reason, "fisc-") && !strings.Contains(reason, "p") {
			t.Errorf("scope %q's reason cites no page or bead: %q", scope, reason)
		}
	}
}

// It uses transfersDetailScope because that is the only entry left in the map:
// expenditure-by-department's declaration retired for real when the drill-down
// began drawing it, which is this very mechanism firing over the committed
// corpus rather than over a fixture.
// A declaration that has stopped being true goes red rather than going quiet.
//
// factsAreProjected consults the map only for UNPROJECTED facts, so the moment
// something projects a declared scope the entry falls silent while remaining a
// false claim about the corpus. Both arms are exercised, because they need
// different fixes: delete the entry, or find out why the rules stopped writing
// the scope.
func TestAStaleUnprojectedScopeDeclarationFails(t *testing.T) {
	t.Run("the scope is now projected", func(t *testing.T) {
		// The spine facts, relabelled into the declared scope, and a projection
		// built OF that scope — which is what fisc-gxa.2 will do for real.
		facts := testFacts()
		for i := range facts {
			facts[i].Scope = transfersDetailScope
		}
		fact.Sort(facts)
		s := factsSubject(t, facts)
		s.Projections = []Projection{{
			Name: "sankey",
			Options: project.Options{
				Columns: []project.Column{{
					FiscalYear: facts[0].FiscalYear, Basis: facts[0].Basis,
				}},
				Scopes: []string{transfersDetailScope},
			},
		}}
		// The check is run directly rather than through the whole set: this
		// Subject carries a projection with no graph behind it, which is a
		// shape only a test produces and which the graph checks would read.
		res, err := (&factsAreProjected{}).Run(context.Background(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: the declaration exempts nothing now",
				res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "exempting nothing") {
			t.Errorf("findings %v do not say the declaration is doing nothing", res.Findings)
		}
	})

	t.Run("no rule writes the scope", func(t *testing.T) {
		// A Subject WITH rule files, none of which writes the declared scope:
		// the string here and the string in mappings/ have drifted apart.
		// testSubject rather than factsSubject, so every fact IS projected and
		// the only thing left for the check to report is the declaration.
		s := testSubject(t)
		s.Files = []*mapping.File{{
			DocID: testDoc,
			Rules: []mapping.Rule{{ID: "spine-revenues", Scope: spineScope}},
		}}
		res := resultFor(t, runChecks(t, s), "facts-are-projected")
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "drifted apart") {
			t.Errorf("findings %v do not name the drift\n%s", res.Findings, findingDetails(res))
		}
	})

	// THE MISTYPED SCOPE, MASKED BY THE ARM ABOVE IT (fisc-rwo).
	//
	// The "exempting nothing" arm tested only that some projection DRAWS the
	// scope and that no fact of it is still unprojected. It never consulted the
	// fact count, so a scope no rule writes -- which carries no facts at all --
	// satisfied both clauses the instant any projection declared it, and the
	// check reported "all 0 of its facts are in some projection's slice" about a
	// scope with no facts and no rules. The mistyped-scope diagnosis the map
	// exists for was unreachable, and the two need different fixes: delete the
	// entry, or find out why the rules stopped writing the string.
	t.Run("a scope no rule writes is not a scope that is fully drawn", func(t *testing.T) {
		s := testSubject(t)
		s.Files = []*mapping.File{{
			DocID: testDoc,
			Rules: []mapping.Rule{{ID: "spine-revenues", Scope: spineScope}},
		}}
		// A projection OF the declared scope, over a store that carries no fact
		// in it. Drawn, and empty.
		s.Projections = append(s.Projections, Projection{
			Name: "detail",
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2026, Basis: project.PublishedBasis}},
				Scopes:  []string{transfersDetailScope},
			},
		})
		res, err := (&factsAreProjected{}).Run(context.Background(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		details := findingDetails(res)
		if !strings.Contains(details, "drifted apart") {
			t.Errorf("findings do not diagnose the scope as one no rule writes:\n%s", details)
		}
		if strings.Contains(details, "all 0 of its facts") {
			t.Errorf("findings report a scope with no facts as fully drawn:\n%s", details)
		}
	})

	// THE PARTIAL ARM (fisc-rmw), and it is the one that was silent. The two
	// arms above are the total cases: everything drawn, or nothing drawn and
	// nothing written. Between them sits the state that actually arrives when a
	// projection lands -- HALF a declared scope drawn -- and before this arm the
	// entry stayed green through it, because `declared` counts only facts that
	// fell outside EVERY projection, so the total arm's guard skipped and verify
	// went on printing the entry's reason verbatim beside a smaller number.
	//
	// A reader would see the count fall and the declaration hold, which reads as
	// coverage improving under a stable exemption. It is the opposite.
	t.Run("half the scope is drawn and half is not", func(t *testing.T) {
		// Two columns of one declared scope, and a projection of ONE of them --
		// the shape a revenue-trends over the two adopted years alone would have
		// had, leaving 462 of 924 facts declared and saying nothing.
		facts := testFacts()
		for i := range facts {
			facts[i].Scope = transfersDetailScope
		}
		other := make([]fact.Fact, 0, len(facts))
		for _, f := range facts {
			f.FiscalYear = 2027
			f.ID = fact.MakeID(f.DocID, f.RuleID, f.RowPath, f.RowLabel, f.ColumnPath,
				f.FiscalYear, f.Basis)
			other = append(other, f)
		}
		facts = append(facts, other...)
		fact.Sort(facts)

		s := factsSubject(t, facts)
		s.Projections = []Projection{{
			Name: "half",
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2027, Basis: facts[0].Basis}},
				Scopes:  []string{transfersDetailScope},
			},
		}}
		res, err := (&factsAreProjected{}).Run(context.Background(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: the declaration is true of one column "+
				"and false of the other", res.Status, res.Summary)
		}
		details := findingDetails(res)
		// The finding must name BOTH sides. A finding saying only that something
		// is wrong would leave the reader to work out which half moved, and the
		// fix differs: draw the rest, or rewrite the reason.
		if !strings.Contains(details, "FY2027") || !strings.Contains(details, "FY2026") {
			t.Errorf("findings do not name which slices are drawn and which are still "+
				"declared:\n%s", details)
		}
		if !strings.Contains(details, "no longer true of the whole scope") {
			t.Errorf("findings do not say the entry is partly false:\n%s", details)
		}
	})

	// And the arm that must NOT fire: a Subject with no rule files at all is not
	// a repository in which another schedule's declaration has gone stale.
	t.Run("a fixture with no rule files says nothing", func(t *testing.T) {
		res := resultFor(t, runChecks(t, testSubject(t)), "facts-are-projected")
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass: a miniature of one schedule must not "+
				"have to carry every other schedule to stay green", res.Status, res.Summary)
		}
	})
}

// TestTheRestrictionFiltersAndTheKeyCarriesTheFundGroup pins the seam the
// detail-tie machinery was factored around, because getting it backwards is
// silent in one direction and catastrophic in the other.
//
// detailRestriction.FundGroup FILTERS which facts a check looks at.
// detailKey ALWAYS carries the fund group, whether the check pins one or not.
// Collapsing the two is the tempting simplification and it breaks both checks
// that exist:
//
//   - Make the pin a key component INSTEAD of a filter, and this check meets the
//     other five fund groups' spine expenditure — about $109M in FY2026 — with a
//     detail that is General Fund only, and fails over a correct corpus.
//   - Drop the fund group from the KEY because "the pin already handles it", and
//     fisc-5gk.1's revenue check, which pins nothing because pp.127-140 span
//     every group, compares six groups' facts under one key. That one is worse:
//     it ties on a sum that happens to match and reports green.
//
// The first direction is what this test measures, because it is the one the
// committed corpus can express today.
func TestTheRestrictionFiltersAndTheKeyCarriesTheFundGroup(t *testing.T) {
	// A spine expenditure cell OUTSIDE the restriction's fund group, with no
	// detail counterpart anywhere — which is the truth about the corpus, since
	// pp.167-170 are a General Fund schedule.
	cells := append([]testCell{}, fixtureCells...)
	cells = append(cells, testCell{mapping.KindExpenditure, "services-and-supplies", "enterprise", 33_000})

	facts := append(testFacts(cells...), detailFact(t, "wages-and-benefits", 70_000))
	fact.Sort(facts)

	res := resultFor(t, runChecks(t, factsSubject(t, facts)), "expenditure-detail-ties-to-spine")
	if res.Status != StatusPass {
		t.Fatalf("status = %s (%s), want pass: enterprise expenditure is outside this "+
			"schedule's restriction and must not be reconciled against a General Fund "+
			"detail", res.Status, res.Summary)
	}
	// One cell, not two: the enterprise key was never built, so it cannot be
	// counted as examined either. A subject count that grew here would mean the
	// filter had become a key.
	if res.Subjects != 1 {
		t.Errorf("subjects = %d, want the 1 General Fund expenditure cell; the enterprise "+
			"cell must be filtered out rather than compared and skipped", res.Subjects)
	}
	if strings.Contains(findingDetails(res), "enterprise") {
		t.Errorf("findings name the enterprise cell, so the fund-group pin is keying rather "+
			"than filtering:\n%s", findingDetails(res))
	}
}

// TestTheDetailKeyNamesItsFundGroup asserts the finding subject carries the
// group, which is what makes a revenue finding readable at all: pp.127-140
// publish `taxes/property` under five different fund groups, and a subject
// naming only the category would identify five cells at once.
func TestTheDetailKeyNamesItsFundGroup(t *testing.T) {
	facts := append(testFacts(), detailFact(t, "wages-and-benefits", 70_001))
	fact.Sort(facts)

	res := resultFor(t, runChecks(t, factsSubject(t, facts)), "expenditure-detail-ties-to-spine")
	if res.Status != StatusFail || len(res.Findings) != 1 {
		t.Fatalf("status = %s over %d findings, want one failure", res.Status, len(res.Findings))
	}
	if want := "FY2026 adopted general wages-and-benefits"; res.Findings[0].Subject != want {
		t.Errorf("finding subject = %q, want %q", res.Findings[0].Subject, want)
	}
}

// revenueFact makes one pp.127-140-shaped fact: revenue at the revenue-by-fund
// scope, carrying a FUND number, which is what that schedule's columns are and
// what makes this lane different from the department one.
func revenueFact(t *testing.T, category, group string, fund int, cents int64) fact.Fact {
	t.Helper()
	f := testCell{mapping.KindRevenue, category, group, cents}.fact()
	f.Scope = revenueDetailScope
	f.Fund = fund
	f.RowLabel = "a line item under " + category
	return f
}

// transfersFact makes a p76-shaped fact at the scope fisc-aes decided, so the
// exception's hand-off arms have something to hand off to.
func transfersFact(t *testing.T, category, group string, cents int64) fact.Fact {
	t.Helper()
	f := testCell{mapping.KindTransferIn, category, group, cents}.fact()
	f.Scope = transfersDetailScope
	f.Fund = 100
	f.RowLabel = "Transfer From Somewhere to " + group
	return f
}

// revenueDetailTying is the fixture spine's revenue side decomposed: the same
// three cells, at the detail scope. It ties by construction, which is what lets
// each subtest below perturb exactly one thing.
func revenueDetailTying(t *testing.T) []fact.Fact {
	t.Helper()
	return []fact.Fact{
		revenueFact(t, "taxes/property", "general", 100, 100_000),
		revenueFact(t, "taxes/property", "enterprise", 500, 0),
		revenueFact(t, "charges-for-services", "enterprise", 500, 50_000),
	}
}

// TestRevenueDetailTiesToTheSpine covers the check's verdicts over the shared
// fixture, whose spine carries three revenue cells across two fund groups and
// one General Fund transfer in — which is, conveniently and not by accident, the
// exact shape the one declared exception is about.
func TestRevenueDetailTiesToTheSpine(t *testing.T) {
	const id = "revenue-detail-ties-to-spine"

	t.Run("no detail at all is vacuous", func(t *testing.T) {
		res := resultFor(t, runChecks(t, factsSubject(t, testFacts())), id)
		if res.Status != StatusVacuous {
			t.Fatalf("status = %s (%s), want vacuous", res.Status, res.Summary)
		}
		if !strings.Contains(res.Summary, revenueDetailScope) {
			t.Errorf("summary %q does not name the empty scope", res.Summary)
		}
	})

	t.Run("a complete detail ties", func(t *testing.T) {
		facts := append(testFacts(), revenueDetailTying(t)...)
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		// Three cells, and the transfer-in cell is exempted rather than counted:
		// a subject count of four would mean the exemption is not being applied.
		if res.Subjects != 3 {
			t.Errorf("subjects = %d, want the 3 revenue cells; the General Fund transfer "+
				"in is the declared exception and is not one of them", res.Subjects)
		}
	})

	t.Run("a detail that does not tie fails", func(t *testing.T) {
		facts := append(testFacts(), revenueFact(t, "taxes/property", "general", 100, 100_001))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "same money decomposed two ways") {
			t.Errorf("findings %v do not say what the two figures are", res.Findings)
		}
	})

	// THE FUND GROUP IS IN THE KEY, and this is the case that proves it. The
	// fixture's spine carries taxes/property under BOTH general and enterprise.
	// A detail that puts the General Fund's figure under enterprise sums to the
	// same total and ties on every key a group-blind check would build.
	t.Run("the right money under the wrong fund group fails", func(t *testing.T) {
		facts := append(testFacts(),
			revenueFact(t, "taxes/property", "enterprise", 500, 100_000),
			revenueFact(t, "charges-for-services", "enterprise", 500, 50_000))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: the General Fund's property tax is "+
				"filed under enterprise and the citywide total is unchanged",
				res.Status, res.Summary)
		}
	})

	t.Run("a spine key the detail dropped fails", func(t *testing.T) {
		facts := append(testFacts(),
			revenueFact(t, "taxes/property", "general", 100, 100_000),
			revenueFact(t, "taxes/property", "enterprise", 500, 0))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: charges-for-services is on the spine "+
				"and not in the detail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "no such row at all") {
			t.Errorf("findings %v do not name the dropped rule", res.Findings)
		}
	})

	t.Run("a detail key the spine does not have fails", func(t *testing.T) {
		facts := append(testFacts(), revenueDetailTying(t)...)
		facts = append(facts, revenueFact(t, "licenses-and-permits", "general", 100, 1_000))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "never extend it") {
			t.Errorf("findings %v do not say the detail may not extend the spine", res.Findings)
		}
	})
}

// TestTheGeneralFundTransfersInException covers all four arms of the one
// declared exception in the corpus.
//
// It gets its own test because an exception is the part of a reconciliation that
// rots: it is written once, about a state of the world, and the world moves.
// fisc-u2v's 2026-08-23 correction is explicit that an exemption may not simply
// be silent — it names the scope that covers the key instead, and fails if that
// scope carries facts and not this one. Arm 4 is the honest reading of the other
// half of that sentence, "if no scope covers it, the exception must say in words
// that the cell is unreconciled", and arms 1 and 3 are what stop arm 4 becoming
// permanent.
func TestTheGeneralFundTransfersInException(t *testing.T) {
	const id = "revenue-detail-ties-to-spine"
	tying := revenueDetailTying(t)

	// ARM 4: nothing carries transfers-by-fund yet. The cell is unreconciled
	// and the summary says so by name, on every run, with the bead.
	t.Run("pending: the covering scope carries nothing yet", func(t *testing.T) {
		facts := append(testFacts(), tying...)
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		for _, want := range []string{"NOT RECONCILED", transfersDetailScope, "fisc-5gk.3.1"} {
			if !strings.Contains(res.Summary, want) {
				t.Errorf("summary does not contain %q, so an unreconciled cell is "+
					"published quietly:\n%s", want, res.Summary)
			}
		}
	})

	// ARM 2: p76 lands and covers the key. The exception steps aside and says so.
	t.Run("covered: the named scope carries the key", func(t *testing.T) {
		facts := append(testFacts(), tying...)
		facts = append(facts, transfersFact(t, "transfers/in", "general", 10_000))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if strings.Contains(res.Summary, "NOT RECONCILED") {
			t.Errorf("summary still calls the cell unreconciled after %q covers it:\n%s",
				transfersDetailScope, res.Summary)
		}
		if !strings.Contains(res.Summary, "handed off to scope") {
			t.Errorf("summary does not say which scope took the cell over:\n%s", res.Summary)
		}
		// AND IT MUST NOT CLAIM THE RECONCILIATION (fisc-8ka). The arm tests
		// that the covering scope publishes the KEY -- covers[k] counts facts,
		// never cents -- so a scope carrying one cent under it would satisfy
		// this arm too. transfers-detail-ties-to-spine is what compares the
		// amount; a check that asserted its conclusion would be putting the
		// claim and the evidence for it in different places.
		if strings.Contains(res.Summary, "reconciled by scope") {
			t.Errorf("summary claims a reconciliation this check did not perform:\n%s",
				res.Summary)
		}
	})

	// The same arm, with the covering scope publishing a DIFFERENT amount. The
	// report line must read the same, because the arm never looked at the
	// amount in either case -- that is precisely what its wording now admits.
	t.Run("covered by a scope carrying the wrong value", func(t *testing.T) {
		facts := append(testFacts(), tying...)
		facts = append(facts, transfersFact(t, "transfers/in", "general", 1))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if !strings.Contains(res.Summary, "handed off to scope") {
			t.Errorf("summary does not name the scope the cell went to:\n%s", res.Summary)
		}
		if strings.Contains(res.Summary, "reconciled by scope") {
			t.Errorf("a scope publishing one cent under the key is reported as having "+
				"reconciled it:\n%s", res.Summary)
		}
	})

	// ARM 3: p76 lands and does NOT cover the key. This is the arm an
	// unconditional exemption would never have, and the one that keeps the
	// declaration from outliving the work it names.
	t.Run("failed hand-off: the named scope exists and drops the key", func(t *testing.T) {
		facts := append(testFacts(), tying...)
		facts = append(facts, transfersFact(t, "transfers/in", "enterprise", 10_000))
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: %q was mapped and this cell was not "+
				"in it, so nothing reconciles it now", res.Status, res.Summary,
				transfersDetailScope)
		}
		if !strings.Contains(findingDetails(res), "hand-off has failed") {
			t.Errorf("findings %v do not say the hand-off failed", res.Findings)
		}
	})

	// ARM 2 AND ARM 3 AT ONCE, which is the case a whole-store count gets
	// wrong. transfers-by-fund lands covering FY2026 and not FY2027. Counting
	// "does the successor cover this cell" over the whole fact store and then
	// exempting every key answers yes, reports both years reconciled, and never
	// reaches arm 3 -- so a whole year of the spine's General Fund transfers in
	// becomes a cell no check looks at, which is the exact shape of the hole
	// this exception's condition exists to refuse.
	t.Run("partial hand-off: the named scope covers one year and not the other", func(t *testing.T) {
		later := transfersFact(t, "transfers/in", "general", 10_000)
		later.FiscalYear++
		later.ID += "-second-year"
		facts := append(testFacts(), tying...)
		var nextYear []fact.Fact
		for _, f := range facts {
			g := f
			g.FiscalYear = f.FiscalYear + 1
			g.ID = f.ID + "-second-year"
			nextYear = append(nextYear, g)
		}
		facts = append(facts, nextYear...)
		// The successor covers the SECOND year only; the first is still owed.
		facts = append(facts, later)
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: %q was mapped and covers only one of "+
				"the two years the spine publishes", res.Status, res.Summary,
				transfersDetailScope)
		}
		if !strings.Contains(findingDetails(res), "hand-off has failed") {
			t.Errorf("findings %v do not say the hand-off failed for the uncovered year",
				res.Findings)
		}
	})

	// ARM 1: the schedule turns out to print the row after all, so the
	// exemption is a false claim about the DOCUMENT and would silently excuse a
	// real figure.
	t.Run("false claim: the schedule does publish the row", func(t *testing.T) {
		f := testCell{mapping.KindTransferIn, "transfers/in", "general", 10_000}.fact()
		f.Scope = revenueDetailScope
		f.Fund = 100
		facts := append(testFacts(), tying...)
		facts = append(facts, f)
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: pp.127-140 publish the cell the "+
				"exception says they do not print", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "false claim") {
			t.Errorf("findings %v do not say the exception is a false claim", res.Findings)
		}
	})

	// And the stale direction: an exception for a cell the spine does not
	// publish exempts nothing and must be removed, the same shape
	// staleDeclarations enforces on unprojectedScopes.
	t.Run("stale: the spine publishes no such cell", func(t *testing.T) {
		cells := make([]testCell, 0, len(fixtureCells))
		for _, c := range fixtureCells {
			if c.kind == mapping.KindTransferIn {
				continue
			}
			cells = append(cells, c)
		}
		facts := append(testFacts(cells...), tying...)
		fact.Sort(facts)
		res := resultFor(t, runChecks(t, factsSubject(t, facts)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: the exception exempts nothing",
				res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "exempts nothing") {
			t.Errorf("findings %v do not say the declaration has stopped describing the "+
				"corpus", res.Findings)
		}
	})
}

// fundRuleSubject builds a subject carrying rule files and a one-page document,
// which is what rule-funds-match-their-headings reads. The page is written here
// rather than copied, because these tests are about the RULES and want to state
// the printed line they are checked against on the same screen.
func fundRuleSubject(t *testing.T, page string, rules []mapping.Rule,
	rollups []mapping.Rollup) *Subject {
	t.Helper()
	s := factsSubject(t, testFacts())
	s.Files = []*mapping.File{{
		SchemaVersion: 1, DocID: testDoc, Rules: rules, Rollups: rollups,
		Path: "testdata/fixture.yaml",
	}}
	s.Docs = map[string]*corpus.Doc{testDoc: inlinePageDoc(t, testDoc, 1, page)}
	return s
}

// fundRule is one pp.131-140-shaped rule: a fund per rule, four columns all
// naming it, and a printed `Total <fund>` of its own.
func fundRule(id string, fund int, group, totalRow string) mapping.Rule {
	return mapping.Rule{
		ID: id, Kind: mapping.KindRevenue, Basis: mapping.BasisAdopted,
		Scope: revenueDetailScope, Units: "dollars", TotalRow: totalRow,
		Parts: []mapping.Part{{Page: 1, Columns: []mapping.Column{
			{FundGroup: group, Fund: fund, FiscalYear: 2026},
		}}},
	}
}

// TestRuleFundsMatchTheirHeadings covers the check fisc-u54 asks for, over the
// hazard it was filed about: a fund mis-assigned WITHIN its own fund type, which
// changes no column sum and which every other check passes.
func TestRuleFundsMatchTheirHeadings(t *testing.T) {
	const id = "rule-funds-match-their-headings"
	// data/funds.yaml's fixture: 100 General Fund (general), 500 Water Utility
	// Fund (enterprise), 700 Information Technology Fund (internal-service).
	const page = "" +
		"      Total Water Utility Fund              1,000\n" +
		"      Total Information Technology Fund     2,000\n"

	t.Run("a rule whose printed total names its fund passes", func(t *testing.T) {
		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{
			fundRule("water", 500, "enterprise", "Total Water Utility Fund"),
			fundRule("it", 700, "internal-service", "Total Information Technology Fund"),
		}, nil)), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass", res.Status, res.Summary)
		}
		if res.Subjects != 2 {
			t.Errorf("subjects = %d, want 2 fund-bearing rules", res.Subjects)
		}
	})

	// THE BEAD'S OWN CASE. Both funds exist, both are the right TYPE for the
	// fund group the columns declare, and the column sums are identical either
	// way — so fact-funds-resolve, revenue-detail-ties-to-spine and CheckTotals
	// all pass. Only the printed name disagrees.
	t.Run("a fund swapped for another of the same type fails", func(t *testing.T) {
		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{
			// Reads the Water block and files it under Information Technology.
			fundRule("water", 700, "internal-service", "Total Water Utility Fund"),
			fundRule("it", 700, "internal-service", "Total Information Technology Fund"),
		}, nil)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "mis-assigned inside its own type") {
			t.Errorf("findings %v do not name the hazard", res.Findings)
		}
	})

	t.Run("a printed name no fund claims fails", func(t *testing.T) {
		res := resultFor(t, runChecks(t, fundRuleSubject(t,
			"      Total Wastewater Utility            1,000\n", []mapping.Rule{
				fundRule("ww", 500, "enterprise", "Total Wastewater Utility"),
			}, nil)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "anchored to nothing on the page") {
			t.Errorf("findings %v do not say the number is unanchored", res.Findings)
		}
	})

	// CLAUSE 2, AND THE REASON IT EXISTS. Nine of the schedule's 69 funds print
	// zero in BOTH budget years, so dropping one leaves every reconciled sum
	// unchanged and revenue-detail-ties-to-spine green. Clause 1 cannot see it:
	// it only inspects rules that exist.
	t.Run("a printed fund total no rule claims fails", func(t *testing.T) {
		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{
			fundRule("water", 500, "enterprise", "Total Water Utility Fund"),
		}, nil)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: the page prints an Information "+
				"Technology total and no rule reads it", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "no rule or rollup declares that total") {
			t.Errorf("findings %v do not name the unmapped section", res.Findings)
		}
	})

	// A MAPPED SECTION MUST NOT BE REPORTED AS UNCLAIMED (fisc-948).
	//
	// Claiming a printed total says SOME rule reads that heading. It used to
	// happen after two guards, so a rule that maps a fund section without
	// declaring one fund -- fund_group-only columns, which is what p76 does --
	// never claimed its own total, and clause 2 reported the section it maps as
	// one no rule declares.
	t.Run("a rule with no fund of its own still claims its printed total", func(t *testing.T) {
		noFund := fundRule("it", 700, "internal-service", "Total Information Technology Fund")
		noFund.Parts[0].Columns[0].Fund = 0 // the group is declared, the fund is not

		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{
			fundRule("water", 500, "enterprise", "Total Water Utility Fund"),
			noFund,
		}, nil)), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass: both printed totals are mapped",
				res.Status, res.Summary)
		}
		if strings.Contains(findingDetails(res), "no rule or rollup declares that total") {
			t.Errorf("a section a rule maps is reported as unclaimed:\n%s",
				findingDetails(res))
		}
	})

	// A MISMATCH IS ONE FINDING, NOT TWO THAT CONTRADICT EACH OTHER (fisc-948).
	//
	// The mismatch arm did not mark the rule as reported, so the "anchored to
	// nothing on the page" arm fired as well -- while listing the very total
	// that anchors it. The second sentence was false and the two point a reader
	// at different fixes.
	t.Run("a fund mismatch produces exactly one finding", func(t *testing.T) {
		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{
			fundRule("water", 700, "internal-service", "Total Water Utility Fund"),
			fundRule("it", 700, "internal-service", "Total Information Technology Fund"),
		}, nil)), id)

		mismatches := 0
		for _, f := range res.Findings {
			if f.Subject == "water" {
				mismatches++
			}
		}
		if mismatches != 1 {
			t.Errorf("rule \"water\" has %d findings, want 1: the mismatch and the "+
				"unanchored arm are two answers to one question\n%s",
				mismatches, findingDetails(res))
		}
		if strings.Contains(findingDetails(res), "anchored to nothing on the page") {
			t.Errorf("a rule whose anchor resolved is reported as anchored to nothing:\n%s",
				findingDetails(res))
		}
	})

	// pp.127-130's shape: ONE fund decomposed by CATEGORY across several rules
	// whose printed totals are category names. The fund is named exactly once,
	// by the rollup that covers them, and that is what anchors the number.
	t.Run("a fund named only by a rollup that covers the rules passes", func(t *testing.T) {
		gf := func(id, totalRow string) mapping.Rule {
			return fundRule(id, 100, "general", totalRow)
		}
		res := resultFor(t, runChecks(t, fundRuleSubject(t,
			"      Total Property Taxes                 1,000\n"+
				"      Total Sales Taxes                    2,000\n"+
				"      Total General Fund                   3,000\n",
			[]mapping.Rule{gf("prop", "Total Property Taxes"), gf("sales", "Total Sales Taxes")},
			[]mapping.Rollup{{ID: "gf", Page: 1, TotalRow: "Total General Fund",
				Covers: []string{"prop", "sales"}}})), id)
		if res.Status != StatusPass {
			t.Fatalf("status = %s (%s), want pass: `Total General Fund` names fund 100 "+
				"and covers both rules", res.Status, res.Summary)
		}
	})

	// The same shape with the rollup covering only ONE of them. The uncovered
	// rule's number is anchored to nothing printed, which is the state
	// pp.127-130 would be in if a category rule fell out of the rollup's covers.
	t.Run("a rule the naming rollup does not cover fails", func(t *testing.T) {
		gf := func(id, totalRow string) mapping.Rule {
			return fundRule(id, 100, "general", totalRow)
		}
		res := resultFor(t, runChecks(t, fundRuleSubject(t,
			"      Total Property Taxes                 1,000\n"+
				"      Total Sales Taxes                    2,000\n"+
				"      Total General Fund                   3,000\n",
			[]mapping.Rule{gf("prop", "Total Property Taxes"), gf("sales", "Total Sales Taxes")},
			[]mapping.Rollup{{ID: "gf", Page: 1, TotalRow: "Total General Fund",
				Covers: []string{"prop"}}})), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail: nothing printed names fund 100 for "+
				"the sales rule", res.Status, res.Summary)
		}
	})

	t.Run("a rule whose columns declare two funds fails", func(t *testing.T) {
		ru := fundRule("mixed", 500, "enterprise", "Total Water Utility Fund")
		ru.Parts[0].Columns = append(ru.Parts[0].Columns,
			mapping.Column{FundGroup: "enterprise", Fund: 600, FiscalYear: 2027})
		res := resultFor(t, runChecks(t, fundRuleSubject(t, page, []mapping.Rule{ru}, nil)), id)
		if res.Status != StatusFail {
			t.Fatalf("status = %s (%s), want fail", res.Status, res.Summary)
		}
		if !strings.Contains(findingDetails(res), "more than one fund") {
			t.Errorf("findings %v do not name the mixed columns", res.Findings)
		}
	})

	// The spine: no rule names a fund, so there is nothing to check and the
	// honest report is vacuous rather than a pass over zero.
	t.Run("no rule declares a fund is vacuous", func(t *testing.T) {
		res := resultFor(t, runChecks(t, testSubject(t)), id)
		if res.Status != StatusVacuous {
			t.Fatalf("status = %s (%s), want vacuous", res.Status, res.Summary)
		}
	})
}
