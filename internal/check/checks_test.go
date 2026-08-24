package check

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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
		"projections-build":           "pass over 1",
		"published-projection-built":  "pass over 1",
		"facts-are-projected":         "pass over 10",
		"graph-acyclic":               "pass over 7",
		"derived-nodes-justified":     "pass over 2",
		"link-values-tie-to-facts":    "pass over 7",
		"counts-reconcile":            "pass over 1",
		"headline-ties-to-facts":      "pass over 5", // 3 revenue + 1 expenditure + 1 transfer out
		"headline-transfer-residual":  "pass over 2",
		"headline-naive-expenditure":  "pass over 1",
		// Nothing to check: no link carries a transfer_id, no node a parent or a
		// constraint tier, no fact a department or a fund number.
		"transfer-legs-pair": "vacuous over 0",
		// pp.167-170 are mapped, but this fixture is a miniature of the SPINE
		// and carries none of their facts, so there is no detail to reconcile.
		"expenditure-detail-ties-to-spine": "vacuous over 0",
		"aggregation-invariance":           "vacuous over 0",
		"constraint-tier-vocabulary":       "vacuous over 0",
		"fact-departments-resolve":         "vacuous over 0",
		"fact-funds-resolve":               "vacuous over 0",
	}
	if diff := cmp.Diff(want, statuses(rep)); diff != "" {
		t.Errorf("verdicts mismatch (-want +got):\n%s", diff)
	}
	if got := (Counts{Pass: 17, Vacuous: 11, Skipped: 1}); got != rep.Counts {
		t.Errorf("counts = %+v, want %+v", rep.Counts, got)
	}
	// Seventeen passes, eleven vacuous and one skipped is not twenty-nine of
	// anything, and a run with nothing wrong in it still exits 0.
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

	if lenient.Counts.Vacuous != 11 {
		t.Fatalf("vacuous count = %d, want 10", lenient.Counts.Vacuous)
	}
	if lenient.Failed() {
		t.Error("a run with ten vacuous checks failed without --strict")
	}
	if !strict.Failed() {
		t.Error("a run with ten vacuous checks passed under --strict")
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
		"aggregation-invariance":     "no node carries a parent",
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

// TestAggregationInvarianceErrorsWhenAHierarchyArrives is the ratchet on
// fisc-1wr.1.1. The fold is not implemented, so the moment a node carries a
// parent this check must refuse to report a verdict — an error, which is neither
// a pass nor a claim about the corpus.
func TestAggregationInvarianceErrorsWhenAHierarchyArrives(t *testing.T) {
	s := testSubject(t)
	nodePointer(t, s.Projections[0].Graph, project.NodeFundBalanceDraw).Parent = "fund-balance"
	res := resultFor(t, runChecks(t, s), "aggregation-invariance")

	if res.Status != StatusError {
		t.Fatalf("status = %s, want error", res.Status)
	}
	if !strings.Contains(res.Summary, "fisc-1wr.1.1") {
		t.Errorf("summary %q does not name the bead that owns the fold", res.Summary)
	}
	// An error fails the run without --strict, which is the difference between
	// this and a vacuous result.
	if !runChecks(t, s).Failed() {
		t.Error("Failed() = false for a report containing an error")
	}
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
			nodePointer(t, s.Projections[0].Graph, project.NodeFundBalanceDraw).ConstraintTier = tt.tier
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
			facts[i].Scope = expenditureDetailScope
		}
		fact.Sort(facts)
		s := factsSubject(t, facts)
		s.Projections = []Projection{{
			Name: "sankey",
			Options: project.Options{
				FiscalYear: facts[0].FiscalYear,
				Basis:      facts[0].Basis,
				Scope:      expenditureDetailScope,
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
