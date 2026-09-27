package check

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestRowFundsCatchesTheTwinSwapNothingElseSees is this check's whole reason for
// existing, stated as the mutation the rest of the suite is blind to.
//
// Budget Book p76's payer side is hand-typed 44 times and the DOCUMENT cannot
// check it: a counterpart is resolved downstream of every comparison against the
// city's own arithmetic, so redeclaring a payer changes no printed total.
// Measured elsewhere in this repository and relied on here.
//
// Five of p76's payer labels match an operating fund AND its CIP twin, and every
// twin is type: capital. So:
//
//   - cuts-tie-along-the-lattice cannot see it: the swap moves the leg inside
//     the collapsed {capital, special-revenue, permanent} cell and the joint
//     sum is unchanged.
//   - factFundsResolve cannot see three of the five (510, 550, 560), because
//     those are already capital and the fund group does not change.
//
// This test runs the two hardest cases: the one that changes fund group and the
// one that does not.
func TestRowFundsCatchesTheTwinSwapNothingElseSees(t *testing.T) {
	for _, tt := range []struct {
		name          string
		anchor        string
		from, to      int
		alsoSetGroup  string
		printedFundNo int
	}{{
		name: "an operating fund swapped for its CIP twin, same type",
		// Traffic Imp Fee 510 -> CIP Traffic Impact Fee 823. Both capital, so
		// the fund GROUP does not change and factFundsResolve stays green.
		anchor: "Transfer From Traffic Imp Fee", from: 510, to: 823, printedFundNo: 510,
	}, {
		name: "an operating fund swapped for its CIP twin, across types",
		// Low Income Hsng 200 (special-revenue) -> 812 (capital). The group has
		// to move with it or factFundsResolve reddens first, which would make
		// this test prove the wrong thing.
		anchor: "Transfer From Low Income Hsng", from: 200, to: 812,
		alsoSetGroup: "capital", printedFundNo: 200,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			swapped := 0
			for _, f := range s.Files {
				for i := range f.Rules {
					for j := range f.Rules[i].Rows {
						row := &f.Rules[i].Rows[j]
						if row.Counterpart == nil || row.Counterpart.Fund != tt.from ||
							row.Label != tt.anchor {
							continue
						}
						row.Counterpart.Fund = tt.to
						if tt.alsoSetGroup != "" {
							row.Counterpart.FundGroup = tt.alsoSetGroup
						}
						swapped++
					}
				}
			}
			if swapped != 1 {
				t.Fatalf("swapped %d rows, want 1; the mutation missed its target", swapped)
			}

			res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Status != StatusFail {
				t.Fatalf("swapping fund %d for its twin %d reported %s: %s",
					tt.from, tt.to, res.Status, res.Summary)
			}
			var b strings.Builder
			for _, f := range res.Findings {
				b.WriteString(f.Subject + ": " + f.Detail + "\n")
			}
			for _, want := range []string{tt.anchor, "fund", "no other check sees it"} {
				if !strings.Contains(b.String(), want) {
					t.Errorf("no finding mentions %q:\n%s", want, b.String())
				}
			}
		})
	}
}

// TestTheCommittedCorpusRowAnchorsHold pins what the check covers today,
// including what it does NOT, because the summary is printed on every run and a
// reader counting rows would otherwise have to guess.
func TestTheCommittedCorpusRowAnchorsHold(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("%s: %s", res.Status, res.Summary)
	}

	// THE THREE ROWS THE PAGE LEAVES UNANCHORED, named rather than counted.
	// p76 prints three continuation rows whose "Transfer From" carries over
	// from the row above, so their declared payer has no printed label on its
	// own line (fisc-30j). Three of 22 rows are unguarded on the side where the
	// operating/CIP ambiguity lives, and the summary has to say so.
	for _, want := range []string{
		`"to Wastewater Replacement" (the fund that pays, 620)`,
		`"to 2022 COPS" (the fund that pays, 100)`,
		`"to Downtown LMD" (the fund that pays, 100)`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("the summary does not name the unanchored row %s:\n%s", want, res.Summary)
		}
	}
	if !strings.Contains(res.Summary, "3 declared fund(s) have no printed anchor") {
		t.Errorf("the summary does not count the unanchored declarations:\n%s", res.Summary)
	}

	// THE SEVENTY-EIGHT ROWS ARE NOW READ RATHER THAN COUNTED, which is the
	// whole of fisc-90fp. pp.85-125's Department Funding Sources rows print a
	// bare fund name -- which names a fund and no direction, so
	// rowAnchorPrefixes matches nothing and this check used to say so and stop.
	// Their rules declare row_labels_name_funds, so the label is resolved whole
	// and the hand-typed number is checked against it.
	//
	// 118 IS THE NUMBER THAT SAYS SO: 40 printed anchors on p76 plus these 78.
	// Asserting the count rather than the absence of the old clause is
	// deliberate -- a regression that dropped the arm entirely would delete the
	// clause too, and an absence assertion would pass on it.
	if res.Subjects != 118 {
		t.Errorf("the check resolves %d row anchors, want 118: 40 printed on p76 plus "+
			"the 78 bare fund labels on pp.85-125\n%s", res.Subjects, res.Summary)
	}
	// THE WORDING SEARCHED FOR HERE IS THE ONE THE CODE EMITS, checked against
	// rowfunds.go rather than remembered. A negative assertion over a string no
	// code path can emit cannot fail -- it stays green with the whole bare-label
	// arm deleted -- and is the cheapest way to write a test that proves nothing.
	if strings.Contains(res.Summary, "declared fund(s) sit at an end this check does not read") {
		t.Errorf("the summary still reports the bare-label rows as ones it makes no "+
			"claim about:\n%s", res.Summary)
	}
	// heldLine's WIRING, not just its output. TestTheHeldLineNeverReportsAnEmptyArm
	// calls heldLine directly, so passing it the wrong arguments -- heldLine(subjects, 0)
	// -- left the whole suite green while restoring the exact false PASS line
	// cf96ed2 removed. This is the assertion that reads what fisc verify prints.
	for _, want := range []string{
		"40 printed with a verb phrase",
		"78 a bare fund name under row_labels_name_funds",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("the PASS line does not keep the two arms apart: no %q in\n%s",
				want, res.Summary)
		}
	}
	// AND NOT ONE BY ONE. Naming each row put this check's PASS line at 11 KB,
	// against roughly 450 bytes before, which is detailtie.go's "a count is the
	// honest middle" argument arriving one check late. A row that HOLDS is
	// counted; only a row that fails is named.
	//
	// THE LENGTH BOUND IS WHAT ENFORCES THAT, not a search for one row's name.
	// This used to assert the summary did not contain `funding-public-works
	// "Water"`, which no code path can emit: a bare label never sets `phrased`,
	// so it never reaches unanchoredRows, the only per-row list in the summary.
	// The assertion was unfalsifiable and read as coverage.
	if n := len(res.Summary); n > 2000 {
		t.Errorf("the summary is %d bytes; it is printed on every fisc verify run", n)
	}
}

// TestTheBareLabelArmIsWhatTheDeclarationTurnsOn is the proof that
// row_labels_name_funds is load-bearing rather than decorative.
//
// WITHOUT IT THE ARM WOULD STILL LOOK RIGHT. A commit that added the code and
// forgot the eleven YAML declarations passes every assertion above except the
// count, and a reader checking "does the new arm work" against a hand-built
// fixture would see it work. This runs the committed corpus with the flag
// cleared and asserts the check falls back exactly to where it was: 78 fewer
// subjects, and the clause naming those rows as unread returns.
func TestTheBareLabelArmIsWhatTheDeclarationTurnsOn(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := withoutRowLabelFunds(base)
	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Subjects != 40 {
		t.Errorf("with the declaration cleared the check resolves %d row anchors, "+
			"want 40 -- the p76 anchors alone", res.Subjects)
	}
	if !strings.Contains(res.Summary, "a further 78 declared fund(s) sit at an end this "+
		"check does not read") {
		t.Errorf("with the declaration cleared the summary does not report the 78 as "+
			"rows it makes no claim about:\n%s", res.Summary)
	}
	// ELEVEN RULES, NOT SEVENTY-EIGHT ROWS, and both halves of that are asserted
	// because a rewrite once dropped them: replacing the
	// seenRule dedup with a per-row append left the suite green, so the 11 KB
	// PASS line this counter exists to prevent could come back unnoticed.
	// detailtie.go's "a count is the honest middle" is the argument; this is
	// what holds it.
	for _, want := range []string{"funding-city-council", "funding-public-works"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("the summary does not attribute the unread declarations to their "+
				"rules: no %q in\n%s", want, res.Summary)
		}
	}
	if n := strings.Count(res.Summary, "funding-"); n != 11 {
		t.Errorf("the summary names funding-* %d times, want 11 -- one per rule. A "+
			"per-row list is 78 and is what put this check's PASS line at 11 KB", n)
	}
}

// withoutRowLabelFunds copies s with RowLabelsNameFunds cleared on every rule,
// deep enough that the original is untouched: Subject holds []*mapping.File and
// two tests share one Load.
func withoutRowLabelFunds(base *Subject) *Subject {
	s := *base
	s.Files = nil
	for _, f := range base.Files {
		cut := *f
		cut.Rules = append([]mapping.Rule(nil), f.Rules...)
		for i := range cut.Rules {
			cut.Rules[i].RowLabelsNameFunds = false
		}
		s.Files = append(s.Files, &cut)
	}
	return &s
}

// TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw is the arm that was
// missing.
//
// The vacuous reason is printed by --strict as the justification for a check
// having nothing to look at, so a false one is worse than no reason at all --
// which is what the comment beside `nothing` already said. It tested only the
// unanchored counter, so a corpus of nothing but pp.85-125 reported "no row
// declares a fund" while 78 did. Reverting the guard to `unanchored > 0` makes
// this red.
func TestAVacuousRowFundsSummaryCannotDenyTheRowsItSaw(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Only the funding-source rules, whose row labels carry no verb phrase, so
	// every declared fund is unphrased and none is unanchored.
	//
	// AND WITH row_labels_name_funds CLEARED, which fisc-90fp made necessary and
	// which does not weaken this test. Those rules declare it on the committed
	// corpus, so the check now READS all 78 and this corpus is no longer vacuous
	// at all -- the property under test would have nowhere to live. Clearing it
	// restores the exact shape the defect lived in: 78 declared funds, none of
	// them resolvable by this check, and a reason that must not deny them.
	// A corpus of rules that decline the declaration is also a real corpus; it
	// is what every schedule mapped before this one looked like.
	cleared := withoutRowLabelFunds(base)
	s := *cleared
	s.Files = nil
	for _, f := range cleared.Files {
		cut := *f
		cut.Rules = nil
		for i := range f.Rules {
			if strings.HasPrefix(f.Rules[i].ID, "funding-") {
				cut.Rules = append(cut.Rules, f.Rules[i])
			}
		}
		s.Files = append(s.Files, &cut)
	}

	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), &s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Fatalf("over a corpus of bare-label rows the check reported %s: %s",
			res.Status, res.Summary)
	}
	if strings.Contains(res.Summary, "no row declares a fund") {
		t.Errorf("the vacuous reason denies the 78 rows that do declare one:\n%s",
			res.Summary)
	}
	if !strings.Contains(res.Summary, "78 declared fund(s)") {
		t.Errorf("the vacuous reason does not say what it saw:\n%s", res.Summary)
	}
	// AND IT HAS TO READ. The assertion above passed on "; a further 78
	// declared fund(s)..." -- a clause with no antecedent, because the note was
	// seeded in the unanchored arm and appended to in the unphrased one.
	// Contains("78 declared fund(s)") is satisfied by that malformed string, so a
	// test written for this defect could not fail on it, which is why the two
	// assertions below pin the antecedent and the adjacency rather than a
	// substring.
	if strings.Contains(res.Summary, "a further") {
		t.Errorf("the summary says \"a further\" with nothing before it:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "resolve; 78 declared fund(s)") {
		t.Errorf("the one clause does not follow the reason directly:\n%s", res.Summary)
	}
}

// TestABareFundLabelIsNotReportedAsAnUnprintedAnchor is the property the split
// above exists for, stated so it can fail on its own.
//
// Before pp.85-125 landed, every declared fund with no resolved anchor was p76's
// continuation-row case and the one sentence was true. Adding 78 rows whose
// label IS printed made it false 78 times over, in a string fisc verify prints
// verbatim. Reverting anchorHasVerbPhrase to a constant true collapses the two
// lists back into one and makes this test red on the count.
func TestABareFundLabelIsNotReportedAsAnUnprintedAnchor(t *testing.T) {
	if anchorHasVerbPhrase("General Fund") {
		t.Error(`"General Fund" carries no verb phrase, so it cannot be evidence of direction`)
	}
	// "to " is a prefix of nothing in the funding-source label set, and the
	// nearest thing to a trap -- a fund whose name begins with a lowercase
	// preposition -- does not exist in data/funds.yaml.
	if !anchorHasVerbPhrase("to Downtown LMD") || !anchorHasVerbPhrase("Transfer From CASP Fee") {
		t.Error("a p76 anchor stopped being recognised as carrying a verb phrase")
	}
}

// TestRowFundsIsVacuousWithoutAPerRowSchedule is the arm that lets it be
// registered: the shared fixture is a miniature of the spine, whose fund
// dimension is per column.
func TestRowFundsIsVacuousWithoutAPerRowSchedule(t *testing.T) {
	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), testSubject(t))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Errorf("over a corpus whose funds are all per column the check reported %s: %s",
			res.Status, res.Summary)
	}
	if !strings.Contains(res.Summary, "rule-funds-match-their-headings") {
		t.Errorf("the vacuous summary does not name the check that covers the "+
			"per-column case instead: %s", res.Summary)
	}
}

// TestRowFundsCatchesASameGroupEndSwap is the direction half: swapping the ends
// of "Transfer From Water  to Water Replacement" (640/642, both enterprise)
// moves no group sum, so cuts-tie-along-the-lattice cannot see it.
func TestRowFundsCatchesASameGroupEndSwap(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	swapped := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if row.Label != "Transfer From Water" || row.Counterpart == nil {
					continue
				}
				// Both ends are enterprise, so no fund-group sum moves.
				row.Fund, row.Counterpart.Fund = row.Counterpart.Fund, row.Fund
				swapped++
			}
		}
	}
	if swapped != 1 {
		t.Fatalf("swapped %d rows, want 1", swapped)
	}

	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("inverting a transfer's two ends within one fund group reported %s: %s",
			res.Status, res.Summary)
	}
	// Both anchors are now wrong, and each finding says which end it is about.
	if len(res.Findings) != 2 {
		t.Errorf("got %d findings, want 2 (the payer anchor and the payee anchor)",
			len(res.Findings))
	}
	var b strings.Builder
	for _, f := range res.Findings {
		b.WriteString(f.Detail + "\n")
	}
	for _, want := range []string{"the fund which pays", "the fund which receives"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no finding says %q; a direction failure that does not name the "+
				"direction is not actionable:\n%s", want, b.String())
		}
	}
}

// TestRowFundsCatchesABareLabelTwinTheGateDoesNot is fisc-90fp's proof, and the
// mutation it runs is the one the bead was filed for.
//
// WHAT WAS ALREADY TRUE, said first so this test is not read as closing a hole
// it did not close. TestEveryFundingSourceFactMatchesThePrintedRow has resolved
// every funding-source fact's RowLabel through FundByLabel since cd1192c, so
// `go test` was already red on Water 640 -> 641. What was NOT red was the GATE:
// measured at 02156a7, that mutation left `fisc verify` fully green. A guarantee
// that lives only inside one lane's test is not one the fact store carries, and
// the acceptance criterion on fisc-90fp is "makes fisc verify fail".
//
// SO THIS ASSERTS ON THE Result AND NOT ON THE SUITE, deliberately. Running the
// whole suite over this mutation goes red either way, and a proof that cannot
// tell the new arm from the old test is green because the other gate fired --
// which is the shape AGENTS.md names and which this repo has shipped four times.
//
// Measured with the arm in place: `fisc verify` goes red on
// row-funds-match-their-anchors alone, with 1 finding over 118 row anchors,
// while fact-funds-resolve and cuts-tie-along-the-lattice both stay PASS --
// 640 and 641 are both `enterprise` in data/funds.yaml, so no money leaves its
// group and no sum moves. Deleting the BARE-LABEL arm from Run -- the second of
// the two gated on ru.RowLabelsNameFunds, not the phrased-label refusal above it
// -- returns it to green.
func TestRowFundsCatchesABareLabelTwinTheGateDoesNot(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Water 640 -> CIP Water 641, with the fund group left alone. All four of
	// Public Works' operating/CIP twins keep their operating fund's type -- the
	// OPPOSITE of p76, where every twin is capital -- which is exactly why the
	// group is not touched here and why the two sibling checks below stay green.
	swapped := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if f.Rules[i].ID != "funding-public-works" || row.Label != "Water" {
					continue
				}
				if row.Fund != 640 {
					t.Fatalf("row %q declares fund %d, want 640", row.Label, row.Fund)
				}
				row.Fund = 641
				swapped++
			}
		}
	}
	if swapped != 1 {
		t.Fatalf("swapped %d rows, want 1; the mutation missed its target", swapped)
	}

	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("Water 640 -> 641 reported %s: %s", res.Status, res.Summary)
	}
	var b strings.Builder
	for _, f := range res.Findings {
		b.WriteString(f.Subject + ": " + f.Detail + "\n")
	}
	for _, want := range []string{`funding-public-works "Water"`, "fund 640", "fund 641"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no finding mentions %q:\n%s", want, b.String())
		}
	}

	// THE GREEN HALF, and it is what makes the mutation worth guarding against
	// rather than merely detectable. Both of the checks that catch the OTHER two
	// twin-swap shapes report PASS over the same swap.
	//
	// THE FACTS HAVE TO BE MUTATED TOO: factFundsResolve and
	// cutsTieAlongTheLattice read s.Facts, everything above reads s.Files, and
	// with only the rules mutated both pass without ever seeing the swap.
	// Editing the facts in place is what `fisc build` would emit from the
	// mutated rule: 640 and 641 are both enterprise, so no group sum moves.
	swappedFacts := 0
	for i := range s.Facts {
		f := &s.Facts[i]
		if f.RuleID == "funding-public-works" && f.Fund != nil && *f.Fund == 640 {
			f.Fund = fact.FundNumber(641)
			swappedFacts++
		}
	}
	if swappedFacts == 0 {
		t.Fatal("no fact carries funding-public-works fund 640; the green half would " +
			"pass over an unmutated store, which is what it exists to refuse")
	}
	for _, c := range []Check{&factFundsResolve{}, &cutsTieAlongTheLattice{}} {
		got, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatalf("%s: %v", c.ID(), err)
		}
		if got.Status != StatusPass {
			t.Errorf("%s reported %s over the same-type swap; this test proves the "+
				"wrong thing if another check sees it: %s", c.ID(), got.Status, got.Summary)
		}
	}
}

// TestABareLabelThatResolvesToNoFundIsAFinding is the fail-closed arm, and it is
// the difference between a declaration and a decoration.
//
// If an unresolvable label were a `continue`, a rule could declare
// row_labels_name_funds over labels data/funds.yaml has never heard of and the
// check would report exactly nothing -- vacuously green, with a declaration
// standing over it that reads to the next author like a guarantee. That is this
// project's "green because the gate fired" in its purest form: the gate is the
// registry lookup, and skipping on its failure means the assertion below it
// never runs.
//
// It cannot be proved by mutating the published file, which is why it is here.
// Renaming a row label breaks the page anchor first -- and on funding-public-works
// it breaks the omitted_rows declaration before even that -- so the resolver
// refuses long before this check runs. Mutating the loaded rules reaches it.
func TestABareLabelThatResolvesToNoFundIsAFinding(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	renamed := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				row := &f.Rules[i].Rows[j]
				if f.Rules[i].ID != "funding-city-attorney" || row.Label != "General Fund" {
					continue
				}
				row.Label = "No Such Fund"
				renamed++
			}
		}
	}
	if renamed != 1 {
		t.Fatalf("renamed %d rows, want 1; the mutation missed its target", renamed)
	}

	res, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("a declared row label naming no fund reported %s: %s",
			res.Status, res.Summary)
	}
	var b strings.Builder
	for _, f := range res.Findings {
		b.WriteString(f.Subject + ": " + f.Detail + "\n")
	}
	// ONE SPELLING, NOT TWO. This used to OR in "checked against\nnothing",
	// which the single-line format string cannot emit -- a dead disjunct that
	// read as tolerance and was really unfalsifiable coverage, the third
	// instance of that class in this file. The finding is one line; assert the
	// line.
	for _, want := range []string{"No Such Fund", "checked against nothing"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the finding does not contain %q:\n%s", want, b.String())
		}
	}
}

// TestASkippedRowIsInvisibleToThisCheck makes the skip guard falsifiable, which
// its first version was not.
//
// A skipped row is never read from the page, so this check -- which is entirely
// about what the page prints on a row's line -- has nothing to say about it. It
// must not be a subject, and it must not be counted as a fund the page prints no
// anchor for either.
//
// THE FIRST ATTEMPT AT THIS GUARD FAILED BOTH TESTS OF A GUARD. It sat on the
// bare-label arm alone, so a skipped funded row was excluded from being CHECKED
// and left in the omission counters -- and the PASS line then said "no
// row_labels_name_funds declaration covers that end" about a rule that declares
// it, which is the same false sentence the counterpart refusal had closed at the
// root one commit earlier. And no test reached it: deleting `&& !row.Skip` left
// `go test ./...` fully green, so the commit message claiming each fix reddened
// its own test was wrong about that one.
//
// The corpus cannot exercise it: all 29 skip: true rows carry no fund, measured.
// So the row is built here.
func TestASkippedRowIsInvisibleToThisCheck(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	before, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), base)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// A skipped row carrying a fund, inside a rule that declares
	// row_labels_name_funds, with a label no fund in the registry answers to.
	// If the guard is missing this is either a finding (the label resolves to
	// nothing) or a subject; either way the numbers below move.
	added := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID != "funding-city-attorney" {
				continue
			}
			f.Rules[i].Rows = append(f.Rules[i].Rows, mapping.Row{
				Label: "Not A Fund At All", Skip: true, Fund: 100, FundGroup: "general",
			})
			added++
		}
	}
	if added != 1 {
		t.Fatalf("added %d rows, want 1", added)
	}

	got, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusPass {
		t.Fatalf("a skipped row made the check %s: %s", got.Status, got.Summary)
	}
	if got.Subjects != before.Subjects {
		t.Errorf("a skipped row moved the subject count from %d to %d; it is not read "+
			"from the page and cannot be an anchor", before.Subjects, got.Subjects)
	}
	if got.Summary != before.Summary {
		t.Errorf("a skipped row changed the summary:\nbefore: %s\nafter:  %s",
			before.Summary, got.Summary)
	}
}

// TestTheHeldLineNeverReportsAnEmptyArm covers all three shapes of the PASS
// line, because two of them are corpora this repo does not have today and both
// were wrong when they were first written.
//
// The line is printed by fisc verify on every run. A clause reading "0 printed
// with a verb phrase, each the fund its row declares at that end of the
// movement" reports the absence of a whole arm as though it were a result, and
// the direction claim in it is about no row at all.
func TestTheHeldLineNeverReportsAnEmptyArm(t *testing.T) {
	// wantNot ARE REGEXPS AND NOT SUBSTRINGS, and that is the third attempt at
	// this test rather than a flourish. "0 " matches inside "40 printed", and so
	// does "0 printed with a verb phrase" -- a substring test over a formatted
	// number catches digits, not clauses, and both earlier drafts failed the
	// committed-corpus case for that reason and not for the reason they were
	// written. \b refuses the match inside 40 because 4 and 0 are both word
	// characters, so there is no boundary between them.
	zeroPhrased := regexp.MustCompile(`\b0 printed with a verb phrase`)
	zeroBare := regexp.MustCompile(`\b0 a bare fund name`)

	for _, tc := range []struct {
		name           string
		subjects, bare int
		wantNot        []*regexp.Regexp
		wantHas        []string
	}{{
		name:     "verb phrases only, as every corpus before pp.85-125",
		subjects: 40, bare: 0,
		wantNot: []*regexp.Regexp{regexp.MustCompile(`bare fund name`),
			regexp.MustCompile(`row_labels_name_funds`)},
		wantHas: []string{"40 row anchors", "at that end of the movement"},
	}, {
		name:     "bare labels only, as a corpus of nothing but funding-* rules",
		subjects: 78, bare: 78,
		wantNot: []*regexp.Regexp{regexp.MustCompile(`printed with a verb phrase`), zeroBare},
		wantHas: []string{"78 row anchors", "names no direction"},
	}, {
		name:     "both, as the committed corpus",
		subjects: 118, bare: 78,
		wantNot: []*regexp.Regexp{zeroPhrased, zeroBare},
		wantHas: []string{"118 row anchors", "40 printed with a verb phrase", "78 a bare"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got := heldLine(tc.subjects, tc.bare)
			for _, re := range tc.wantNot {
				if re.MatchString(got) {
					t.Errorf("held line matches %s, which is a clause about no row:\n%s", re, got)
				}
			}
			for _, w := range tc.wantHas {
				if !strings.Contains(got, w) {
					t.Errorf("held line does not contain %q:\n%s", w, got)
				}
			}
		})
	}
}

// TestAVerbPhrasedLabelUnderTheDeclarationIsRefused is the third answer to a
// question two earlier commits got wrong, and both wrong answers are worth
// keeping because each read as correct.
//
// The declaration says a rule's row labels are the printed names of funds.
// "Transfer From General Fund  to Horizons" is not one: it names a movement.
//
// ATTEMPT ONE resolved it whole -- FundByLabel finds nothing, so the check went
// FAIL over 121 subjects with three spurious findings and every such row counted
// twice, because the prefix arm read it as well.
//
// ATTEMPT TWO deferred to the prefix arm, and turned a noisy defect into a
// silent one. The parser forbids a counterpart on a declaring rule, so far == 0;
// a row labelled "Transfer From X" resolves as the FAR end, want == far == 0,
// and the arm compares nothing. Measured then: subjects 118 -> 117, PASS, and
// the row reported under "no printed anchor on their own line" -- false, the
// label is printed.
//
// ATTEMPT THREE is this: refuse. The author has said something untrue about
// their own page, and saying so is what the declaration is for.
func TestAVerbPhrasedLabelUnderTheDeclarationIsRefused(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	before, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), base)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	flagged := 0
	for _, f := range s.Files {
		for i := range f.Rules {
			if f.Rules[i].ID == "p76-transfers-in-special-revenue" {
				f.Rules[i].RowLabelsNameFunds = true
				flagged++
			}
		}
	}
	if flagged != 1 {
		t.Fatalf("flagged %d rules, want 1", flagged)
	}

	got, err := (&rowFundsMatchTheirAnchors{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusFail {
		t.Fatalf("declaring row_labels_name_funds over verb-phrased labels reported "+
			"%s: %s", got.Status, got.Summary)
	}
	var b strings.Builder
	for _, f := range got.Findings {
		b.WriteString(f.Subject + ": " + f.Detail + "\n")
	}
	for _, want := range []string{"opens with a verb phrase", "names a movement"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no finding says the label names a movement rather than a fund:\n%s",
				b.String())
		}
	}
	// THE SUBJECT COUNT MUST NOT MOVE, which is the half that catches both
	// earlier attempts. Attempt one inflated it to 121 by reading each row
	// twice; attempt two deflated it to 117 by reading one of them not at all.
	if got.Subjects != before.Subjects {
		t.Errorf("the declaration moved the subject count from %d to %d; these rows "+
			"are read by the anchor arm and by it alone", before.Subjects, got.Subjects)
	}
}
