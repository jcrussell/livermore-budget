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
// a corpus small enough to count by hand: 12 facts in 12 cells, of which 4 are
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

		"facts-sorted":                "pass over 12",
		"fact-ids-unique":             "pass over 12",
		"fact-ids-recompute":          "pass over 12",
		"fact-token-reparses":         "pass over 12",
		"fact-offset-points-at-token": "pass over 12",
		"fact-citations-are-declared": "pass over 12",
		// Vacuous over the fixture, and correctly so: the fixture carries no rule
		// files, so no stated-total line resolves and there is nothing a fact
		// could collide with. Over the committed corpus it PASSES -- see
		// TestNoCommittedFactCitesAStatedTotal, which is where it has subjects.
		"fact-offset-is-not-a-stated-total": "vacuous over 0",
		// Two of the fixture's twelve facts are transfers, and neither is
		// printed against its kind's direction. The committed corpus has one
		// that is: ACFR p41's Transfers (out).
		"fact-transfer-orientation-is-declared": "pass over 2",
		"fact-vocabulary":                       "pass over 24", // 12 categories + 12 fund groups
		"fact-kind-matches-category":            "pass over 12",
		// Two fund balances, general and enterprise, each with all three of its
		// lines. Both satisfy beginning + change == ending; neither did before
		// this check was written. See fixtureCells.
		"fund-balance-identity":         "pass over 2",
		"fund-group-sources-equal-uses": "pass over 2",
		// The fixture is a miniature of the SPINE and carries no ACFR
		// Changes in Fund Balances fact, so there is no column to recompute.
		// TestTheCommittedCorpusVacuitySplit is where its real verdict is
		// pinned.
		"excess-of-revenues-identity": "vacuous over 0",
		"projections-build":           "pass over 1",
		"published-projection-built":  "pass over 1",
		// Two projections are registered, but the fixture is a miniature of the
		// SPINE and the trends projection is of nothing here, so one document is
		// built and one document is examined.
		"documents-are-checked":   "pass over 1",
		"graph-acyclic":           "pass over 7",
		"node-tiers-are-declared": "pass over 9",
		// The drill-down's three checks are vacuous over the miniature spine,
		// which carries neither of the schedules it draws.
		// And the two over the schedule documents likewise: the fixture builds
		// none of the four, so there is no count to re-derive and no uncited
		// fact to value.
		// No drill-down, no line node. Not declared vacuous: the committed
		// corpus gives it a subject (TestTheCommittedCorpusVacuitySplit).
		"revenue-lines-tie-to-their-categories": "vacuous over 0",
		"derived-nodes-justified":               "pass over 2",
		"link-locators-match-their-facts":       "pass over 7",
		// No fixture document draws pp.186-209, so no node carries a balance.
		"node-balances-tie-to-facts": "vacuous over 0",
		// The fixture builds no fund-sources-uses document.
		"fund-groups-are-their-printed-rows": "vacuous over 0",
		"link-ends-match-their-facts":        "pass over 7",
		"link-values-tie-to-facts":           "pass over 7",
		"link-kinds-match-their-facts":       "pass over 7",
		"counts-reconcile":                   "pass over 1",
		"headline-ties-to-facts":             "pass over 3", // links leaving a revenue node or entering an expenditure one
		"headline-transfer-residual":         "pass over 2",
		"headline-naive-expenditure":         "pass over 1",
		// Nothing to check: no link carries a transfer_id, no node a parent or a
		// constraint tier, no fact a department or a fund number.
		"transfer-legs-pair": "vacuous over 0",
		// The one comparison every detail schedule ties to the spine through:
		// the fixture is a miniature of the SPINE and carries no cut that
		// decomposes it, so there is no pair to compare.
		"cuts-tie-along-the-lattice": "vacuous over 0",
		// And the peer half of the same comparison: the fixture carries one
		// cut, so there is no pair at one level to overlap.
		"peers-overlap-only-by-declared-identity": "vacuous over 0",
		// Same reason one step further on: no revenue-by-fund fact means the
		// trends projection declares no slice, builds no document, and there is
		// neither a point nor a series to examine.
		"trend-points-tie-to-facts":        "vacuous over 0",
		"trend-series-are-complete":        "vacuous over 0",
		"node-hierarchy-well-formed":       "vacuous over 0",
		"constraint-tier-vocabulary":       "vacuous over 0",
		"contra-links-name-their-schedule": "vacuous over 0",
		"fact-departments-resolve":         "vacuous over 0",
		"fact-funds-resolve":               "vacuous over 0",
		// The fixture taxonomy nests taxes/property under `taxes`, and that is
		// not a line: a line's parent is assignable, and no fixture fact is a
		// revenue-by-fund row. Vacuous is the verdict the definition demands.
		"fact-revenue-lines-resolve": "vacuous over 0",
		// No rule file in the fixture subject declares a fund, so there is no
		// hand-typed number to check against a printed name.
		"rule-funds-match-their-headings": "vacuous over 0",
		// The fixture's funds are per column, which is the other check's case.
		"row-funds-match-their-anchors": "vacuous over 0",
	}
	if diff := cmp.Diff(want, statuses(rep)); diff != "" {
		t.Errorf("verdicts mismatch (-want +got):\n%s", diff)
	}
	if got := (counts{Pass: 25, Vacuous: 23, Skipped: 1}); got != rep.Counts {
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

	if lenient.Counts.Vacuous != 23 {
		t.Fatalf("vacuous count = %d, want 23", lenient.Counts.Vacuous)
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
		"transfer-legs-pair":                    "no link carries a transfer_id",
		"node-hierarchy-well-formed":            "no node carries a parent",
		"constraint-tier-vocabulary":            "no node carries a constraint_tier",
		"contra-links-name-their-schedule":      "no projection publishes a negative link",
		"fact-departments-resolve":              "no fact carries a department",
		"fact-funds-resolve":                    "no fact names a fund",
		"fact-revenue-lines-resolve":            "no fact is a revenue row of scope revenue-by-fund",
		"revenue-lines-tie-to-their-categories": "no projection built a drill-down",
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
	if want := "1 finding over 12 facts"; res.Summary != want {
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
	if want := "11 facts"; !strings.Contains(res.Summary, want) {
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
// non-colliding department that names neither a division nor a department still
// joins to nothing.
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
			if tt.want != "" && !strings.Contains(findingDetails(res), tt.want) {
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

// TestFundNumbersResolveAgainstTheRegistry covers the join key a fact's fund is:
// it has to resolve in data/funds.yaml and has to agree with the fund group
// beside it. An absent fund is null, so a 0 is refused before the registry is
// asked.
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
		tamper func(*project.Document)
		want   string
	}{
		{"facts", func(g *project.Document) { g.Metadata.Counts.Facts++ }, "counts.facts is 13"},
		{"cited", func(g *project.Document) { g.Metadata.Counts.FactsCited++ }, "counts.facts_cited is 8"},
		// A fact newly cited with facts_cited raised and facts_uncited left:
		// the identity, not the kind of fact, is what goes red.
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
	if want := "(12 = 7 cited + 5 uncited, 0 cited twice)"; !strings.Contains(res.Summary, want) {
		t.Errorf("summary %q does not contain %q", res.Summary, want)
	}
}

// TestTransferHeadlineIsTheLinks checks the headline against the transfer
// links the document draws rather than against itself, which is the only way a
// headline check means anything.
func TestTransferHeadlineIsTheLinks(t *testing.T) {
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
	if !strings.Contains(findingDetails(res), "internal_transfer_out_cents is $0.00") {
		t.Errorf("findings %v do not state the figure that was published", res.Findings)
	}
}

// TestNaiveExpenditureMustStayWrong covers the figure internal/project ships
// deliberately wrong. It is worth publishing only while it differs from the real
// one; the day they agree, the page is illustrating nothing.
func TestNaiveExpenditureMustStayWrong(t *testing.T) {
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
			if tt.want != "" && !strings.Contains(findingDetails(res), tt.want) {
				t.Errorf("findings %v do not contain %q", res.Findings, tt.want)
			}
		})
	}
}

// TestTransferLegsPairSeesALegWithNoID: every link of p76's document is a
// leg, so one carrying no transfer_id is half a movement the pairing would
// otherwise skip, leaving the check vacuous over a document that drew legs.
func TestTransferLegsPairSeesALegWithNoID(t *testing.T) {
	res, err := (&transferLegsPair{}).Run(t.Context(), scheduleSubject())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail || !strings.Contains(findingDetails(res), "carries no transfer_id") {
		t.Fatalf("status = %s (%s), findings %v; want a fail naming the leg with no transfer_id",
			res.Status, res.Summary, res.Findings)
	}
	if n := len(res.Findings); n != 1 {
		t.Errorf("%d findings, want the transfers document's one link; the other documents carry no legs", n)
	}
}

// TestNodeHierarchyWellFormedIsFailable damages a hierarchy four ways, one per
// claim.
//
// THE CHECK IS STRUCTURAL: a fold's sum is invariant under relabelling whatever
// node.parent says, so an arithmetic version could not fail.
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
// the Description of Funds narrative, pp.258-261. The tier is the editorial half
// and this check is what asserts its disclosure. Without these arms the site
// would publish an editorial classification unmarked, which is the single
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
		// THE TWO ARMS BELOW WERE ADDED WITH NO CASE, and were unfalsifiable:
		// rewriting both as `case false:` left
		// ./internal/check green, because only the missing-caveat arm above was
		// ever exercised. They are the reason the check looks the caveat up by
		// ID rather than comparing whole values, so leaving them unproved would
		// have made that whole design unverified.
		//
		// TEXT DRIFT WITH THE ID INTACT is the quieter failure of the two: the
		// anchor still resolves, the page still renders a paragraph under the
		// right heading, and the paragraph says something the contract does not
		// require. Matching on the id alone -- the obvious simplification --
		// goes green over exactly this.
		{"the disclosure is present under the right id and says something else",
			func(_ *testing.T, s *Subject) {
				c := project.ConstraintTierCaveat()
				c.Text = "Constraint tiers come from the budget book."
				s.Projections[0].Graph.Metadata.Caveats = []project.Caveat{c}
			}, "not the disclosure internal/project declares"},
		// AND SUMMARY DRIFT, which is the one a reader is most likely to meet:
		// index.html prints the summary and links to the text,
		// so a document whose text is word-perfect and whose summary says
		// something else misleads everyone who does not follow the link.
		{"the summary is not the one internal/project declares",
			func(_ *testing.T, s *Subject) {
				c := project.ConstraintTierCaveat()
				c.Summary = "Fund restrictions, as printed by the city."
				s.Projections[0].Graph.Metadata.Caveats = []project.Caveat{c}
			}, "whose SUMMARY is not the one"},
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
			tt.shift(s.Projections[0].Graph.Metadata.Headline)
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
func hasLinkKind(g *project.Document, k project.LinkKind) bool {
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

// projections-build COUNTS SLICES, which is the unit it has always named.
//
// It counted PROJECTIONS under the unit "projection slices", and one projection
// stopped being one slice when the first multi-column document landed. Measured
// on the committed corpus before the fix: "3 projection slices built" over six.
// A denominator that is not the thing the unit says is a denominator a reader
// cannot use.
func TestProjectionsBuildCountsSlicesNotProjections(t *testing.T) {
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
	res, err := (&projectionsBuild{}).Run(context.Background(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Summary, "4 projection slices built") {
		t.Errorf("summary %q does not count the four slices the one projection covers",
			res.Summary)
	}
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
	// A rule file carries a resolver, as Subject.Resolvers promises: the
	// stated-total check indexes the map by the file's path and runs over this
	// fixture with every other check.
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
		ID: id, Kind: mapping.KindRevenue, Basis: mapping.BasisAdopted,
		Scope: project.ScopeRevenueByFund, Units: "dollars", TotalRow: totalRow,
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
	// way — so fact-funds-resolve, cuts-tie-along-the-lattice and CheckTotals
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
	// unchanged and cuts-tie-along-the-lattice green. Clause 1 cannot see it:
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

// TestAnEndpointCarryingNoFlowMayBeAParent covers node-hierarchy-well-formed's
// one exception, and the mutation that closes it. The subject is the committed
// corpus because testSubject builds no transfers-by-fund document, and a case
// there would be green because the shape never arrived.
func TestAnEndpointCarryingNoFlowMayBeAParent(t *testing.T) {
	const id = "node-hierarchy-well-formed"
	// Spelled rather than imported, so the two cannot agree by construction.
	const endpoint = "transfers/in"

	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("load the committed corpus: %v", err)
	}
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
			res.Status, findingDetails(res))
	}

	// THE MUTATION: one link at the endpoint, and each folded node is refused.
	doc.Links = append(doc.Links, project.Link{
		Source: endpoint, Target: doc.Nodes[0].ID, Kind: project.KindInternalTransfer,
	})
	res := resultFor(t, runChecks(t, s), id)
	if res.Status != StatusFail {
		t.Fatalf("status = %s with a link drawn at %q, want fail", res.Status, endpoint)
	}
	if got := strings.Count(findingDetails(res), "a flow endpoint this document draws a flow at"); got != folded {
		t.Errorf("%d findings name the endpoint, want one per folded node (%d): %s",
			got, folded, findingDetails(res))
	}
}

// scheduleSubject is one document of each of the four schedule shapes, each
// over one cited fact and one uncited printed zero. Hand-assembled, because
// every producer would build more than the check under test needs.
func scheduleSubject() *Subject {
	col := project.Column{FiscalYear: 2026, Basis: mapping.BasisAdopted}
	mk := func(id, scope string, cents int64) fact.Fact {
		return fact.Fact{ID: id, Scope: scope, Kind: mapping.KindRevenue,
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
