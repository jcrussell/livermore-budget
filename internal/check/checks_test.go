package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
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
		"fact-token-reparses":         "pass over 10",
		"fact-offset-points-at-token": "pass over 10",
		"fact-vocabulary":             "pass over 20", // 10 categories + 10 fund groups
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
		"transfer-legs-pair":         "vacuous over 0",
		"aggregation-invariance":     "vacuous over 0",
		"constraint-tier-vocabulary": "vacuous over 0",
		"fact-departments-resolve":   "vacuous over 0",
		"fact-funds-resolve":         "vacuous over 0",
	}
	if diff := cmp.Diff(want, statuses(rep)); diff != "" {
		t.Errorf("verdicts mismatch (-want +got):\n%s", diff)
	}
	if got := (Counts{Pass: 14, Vacuous: 10, Skipped: 1}); got != rep.Counts {
		t.Errorf("counts = %+v, want %+v", rep.Counts, got)
	}
	// Fourteen passes, ten vacuous and one skipped is not twenty-five of
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

	if lenient.Counts.Vacuous != 10 {
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

// TestDepartmentsCannotResolveYetAndSaySo records what today's registries can and
// cannot answer. data/taxonomy.yaml says in as many words that departments are a
// second axis needing a registry of their own; until that file exists, a
// well-formed department is not a resolved one, and the honest verdict is that this
// check could not reach one.
//
// The subject is built without a projection: internal/project refuses a
// department-carrying fact outright, which is a different and also correct answer to
// the same problem, and it would otherwise stop this test at Load.
func TestDepartmentsCannotResolveYetAndSaySo(t *testing.T) {
	facts := testFacts()
	facts[0].Department = "police"
	facts[1].Department = "police"
	rep := runChecks(t, factsSubject(t, facts))

	res := resultFor(t, rep, "fact-departments-resolve")
	if res.Status != StatusError {
		t.Fatalf("status = %s (%s), want error: the slugs are fine and the registry is missing",
			res.Status, res.Summary)
	}
	for _, want := range []string{"police", "no department registry", "fisc-5gk.2"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q does not contain %q", res.Summary, want)
		}
	}
	// A category typo and an unbuilt registry must not share a verdict, because
	// they will not share a fix — or a reconciliations.yaml entry.
	if got := resultFor(t, rep, "fact-vocabulary").Status; got != StatusPass {
		t.Errorf("fact-vocabulary = %s, want pass: no category or fund group is wrong here", got)
	}
}

// TestDepartmentsAreCheckedForWhatIsCheckable covers the three claims that need no
// registry. Each is a real way the axis goes wrong: a label used as a slug, one
// department spelled two ways across the store, and a string that is a category on
// the other axis — the near miss data/taxonomy.yaml exists to prevent.
func TestDepartmentsAreCheckedForWhatIsCheckable(t *testing.T) {
	tests := []struct {
		name  string
		first string
		rest  string
		want  string
	}{
		{"not a slug", "Police Department", "police", "is not a slug"},
		{"two spellings", "police", "po-lice", "spelled 2 ways"},
		{"collides with a category", "charges-for-services", "police", "also a data/taxonomy.yaml category slug"},
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
