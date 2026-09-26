package check

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// vacant is a check that reports vacuous under whatever id the test gives it,
// which is what lets these tests exercise a DECLARED id and an undeclared one
// with the same machinery.
func vacant(id string) *fake {
	return &fake{id: id, res: Result{Status: StatusVacuous, Summary: "nothing to look at"}}
}

// TestEveryDeclarationNamesACheckThatExists is the static half of the
// self-retiring property, and it is static for a reason.
//
// A run may legitimately hold a subset of the checks -- Run takes the slice it
// is given -- so "this run had no such check" cannot mean the declaration is
// stale, and an earlier version of Declaration.Stale said it did: every test
// running two checks reported three stale declarations and a clean run exited 3.
// Whether an id names a real check is a claim about All(), so it is asserted
// here, once, against the canonical registry.
func TestEveryDeclarationNamesACheckThatExists(t *testing.T) {
	real := map[string]bool{}
	for _, c := range All() {
		real[c.ID()] = true
	}
	for id := range declaredVacuous {
		if !real[id] {
			t.Errorf("declaredVacuous names %q, which is in no check All() returns; a "+
				"declaration for a check that does not exist excuses nothing and "+
				"hides that the real one is undeclared", id)
		}
	}
}

// TestEveryDeclarationCarriesItsReasonAndItsBead holds every declaration to one
// standard: a declaration whose reason is thin is a declaration nobody re-reads,
// and re-reading is the whole mechanism.
//
// The count is deliberately NOT pinned. Pinning it would make every wave that
// retires a check edit a number in a test, which is bookkeeping rather than
// evidence, and would equally make every wave that ADDS a vacant check able to
// pass by editing the same number.
func TestEveryDeclarationCarriesItsReasonAndItsBead(t *testing.T) {
	seen := map[string]string{}
	for id, v := range declaredVacuous {
		switch {
		case v.bead == "":
			t.Errorf("%s: no bead; an entry with no work behind it is a permanent "+
				"exemption, which is not what this mechanism is for", id)
		case !strings.HasPrefix(v.bead, "fisc-"):
			t.Errorf("%s: bead %q is not a fisc id", id, v.bead)
		}
		if len(v.reason) < 80 {
			t.Errorf("%s: reason is %d characters; it is printed verbatim on every "+
				"run and has to say what would need to exist for the check to have "+
				"a subject", id, len(v.reason))
		}
		if prev, dup := seen[v.reason]; dup {
			t.Errorf("%s and %s share a reason verbatim; two checks are vacuous for "+
				"the same cause or one of the reasons was copied without being "+
				"re-read", id, prev)
		}
		seen[v.reason] = id
	}
}

// TestADeclaredVacancyPassesStrictAndAnUndeclaredOneDoesNot is the mechanism's
// point in one test: --strict stops being a gate on how much is mapped and
// becomes a gate on whether anyone has looked.
func TestADeclaredVacancyPassesStrictAndAnUndeclaredOneDoesNot(t *testing.T) {
	declared, _ := testVacancy(t)

	rep := run(t, nil, ReportOptions{Strict: true}, passed("a"), vacant(declared))
	if rep.Counts.Vacuous != 1 {
		t.Fatalf("vacuous count = %d, want 1; a declaration must not change the "+
			"verdict, only whether it fails the run", rep.Counts.Vacuous)
	}
	if len(rep.Undeclared) != 0 {
		t.Errorf("Undeclared = %v, want empty for a declared check", rep.Undeclared)
	}
	if rep.Failed() {
		t.Error("Failed() = true under --strict for a vacancy that is declared")
	}

	rep = run(t, nil, ReportOptions{Strict: true}, passed("a"), vacant("nothing-declares-this"))
	if !slices.Contains(rep.Undeclared, "nothing-declares-this") {
		t.Errorf("Undeclared = %v, want it to name the undeclared check", rep.Undeclared)
	}
	if !rep.Failed() {
		t.Error("Failed() = false under --strict for an UNDECLARED vacancy, which is " +
			"the only thing --strict is now for")
	}
	// And without --strict it is reported and tolerated, which is the behaviour
	// that keeps the default gate usable at 10% coverage.
	rep = run(t, nil, ReportOptions{}, passed("a"), vacant("nothing-declares-this"))
	if rep.Failed() {
		t.Error("Failed() = true without --strict for an undeclared vacancy")
	}
}

// TestAStaleDeclarationFailsWithOrWithoutStrict is the self-retiring half, and
// the asymmetry with the test above is the argument.
//
// An undeclared vacancy is a coverage shortfall, which a default run
// deliberately tolerates. A declaration whose check has started reporting
// something else is a FALSE STATEMENT in this package's own source, and no run
// should pass on one -- otherwise the entry outlives the work it was waiting
// for, which is exactly the failure mode a declaration is supposed to prevent.
func TestAStaleDeclarationFailsWithOrWithoutStrict(t *testing.T) {
	declared, _ := testVacancy(t)

	for _, strict := range []bool{false, true} {
		// The declared check now PASSES: the work landed and the entry did not
		// go with it.
		rep := run(t, nil, ReportOptions{Strict: strict}, passed(declared))
		stale := rep.StaleDeclarations()
		if len(stale) != 1 || stale[0].CheckID != declared {
			t.Fatalf("strict=%v: StaleDeclarations() = %v, want just %s",
				strict, stale, declared)
		}
		if !rep.Failed() {
			t.Errorf("strict=%v: Failed() = false for a declaration that has stopped "+
				"being true", strict)
		}
		if got := stale[0].StaleReason(); !strings.Contains(got, "remove the declaration") {
			t.Errorf("strict=%v: stale reason %q does not say to remove it", strict, got)
		}
		if !strings.Contains(stale[0].StaleReason(), declaredVacuous[declared].bead) {
			t.Errorf("strict=%v: stale reason does not name the bead that landed", strict)
		}
	}

	// A run that simply did not include the check is NOT stale.
	rep := run(t, nil, ReportOptions{Strict: true}, passed("a"))
	if len(rep.StaleDeclarations()) != 0 {
		t.Errorf("StaleDeclarations() = %v for a run that held none of the declared "+
			"checks; a subset run says nothing about a declaration",
			rep.StaleDeclarations())
	}
	for _, d := range rep.Declared {
		if d.Ran() {
			t.Errorf("%s reports Ran() for a run that did not include it", d.CheckID)
		}
	}
}

// TestTheCommittedCorpusIsStrictClean is the claim that makes ci.yml able to
// pass --strict, asserted against the real corpus rather than a fixture.
//
// It is the pair to TestTheCommittedCorpusVacuitySplit, which says WHICH checks
// are vacuous. This one says that every one of them is accounted for, and it is
// the assertion that goes red the day a new check lands vacuous without an entry
// -- which is the whole reason the gate can be turned on before the backlog is
// finished.
//
// Like its pair, it deliberately pins no count.
func TestTheCommittedCorpusIsStrictClean(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rep := Run(t.Context(), s, All(), ReportOptions{GeneratedBy: testVersion, Strict: true})

	if len(rep.Undeclared) > 0 {
		t.Errorf("these checks are vacuous over the committed corpus and no entry in "+
			"declaredVacuous names them: %v. Add one with its reason and the bead "+
			"that retires it, or give the check a subject; ci.yml runs --strict and "+
			"this is what it fails on", rep.Undeclared)
	}
	for _, d := range rep.StaleDeclarations() {
		t.Errorf("%s: %s", d.CheckID, d.StaleReason())
	}
	if rep.Failed() {
		t.Errorf("the committed corpus does not pass --strict: %d failed, %d errored, "+
			"%d vacuous of which %d undeclared", rep.Counts.Fail, rep.Counts.Error,
			rep.Counts.Vacuous, len(rep.Undeclared))
	}

	// Every declared entry ran, over the real registry. An entry that never
	// runs here is one TestEveryDeclarationNamesACheckThatExists would have
	// caught, and this is the second, cheaper witness to it.
	for _, d := range rep.Declared {
		if !d.Ran() {
			t.Errorf("%s is declared vacuous but did not run over the real corpus", d.CheckID)
		}
	}
}

// TestADeclarationSurvivesACheckThatReachedNoVerdict is the arm that a cancelled
// run walked into.
//
// Stale is a claim that the work landed, and only PASS and FAIL are verdicts. A
// check that ERRORED or was SKIPPED did not find its subject -- it did not look
// -- so the run establishes nothing about the declaration in either direction.
//
// ERROR IS THE REACHABLE ONE AND IT WAS LIVE. run1 turns ctx.Err() into
// errored(...), so under the old rule -- anything but vacuous is stale -- a
// Ctrl-C on `fisc verify` reported all three declarations stale and exited 3
// saying "<bead> landed, so remove the declaration", about work that had not
// landed. SKIPPED is not reachable today (sourcePDFsMatchBothRecords is the only
// Full() check in the tree and nobody would declare it vacuous) and is asserted
// anyway, because ci.yml's --strict step passes no --full and the next --full
// check inherits the hole.
//
// A genuine error still fails the run. That is Counts.Error's job in Failed(),
// and it says the checker could not tell rather than claiming a bead landed.
func TestADeclarationSurvivesACheckThatReachedNoVerdict(t *testing.T) {
	declared, _ := testVacancy(t)

	t.Run("errored", func(t *testing.T) {
		broke := &fake{id: declared, err: errors.New("the manifest is unreadable")}
		rep := run(t, nil, ReportOptions{Strict: true}, broke)

		if got := rep.StaleDeclarations(); len(got) != 0 {
			t.Errorf("StaleDeclarations() = %v for a check that errored; an error is "+
				"not a verdict and says nothing about whether %s landed",
				got, declaredVacuous[declared].bead)
		}
		// It still fails -- through the arm that describes what happened.
		if rep.Counts.Error != 1 {
			t.Errorf("Counts.Error = %d, want 1", rep.Counts.Error)
		}
		if !rep.Failed() {
			t.Error("Failed() = false for a run with an errored check")
		}
	})

	t.Run("skipped", func(t *testing.T) {
		gated := &fake{id: declared, full: true,
			res: Result{Status: StatusVacuous, Summary: "nothing to look at"}}
		rep := run(t, nil, ReportOptions{Strict: true}, gated)

		if got := rep.StaleDeclarations(); len(got) != 0 {
			t.Errorf("StaleDeclarations() = %v for a check that needed --full and did "+
				"not run", got)
		}
		if rep.Failed() {
			t.Error("Failed() = true for a run whose only check was skipped; a skipped " +
				"check never fails, and a declaration over one cannot go stale")
		}
	})

	// And the verdicts that ARE verdicts still go stale, so the guard above is
	// a whitelist rather than a hole.
	for _, c := range []*fake{passed(declared), failed(declared)} {
		rep := run(t, nil, ReportOptions{}, c)
		if len(rep.StaleDeclarations()) != 1 {
			t.Errorf("%s: StaleDeclarations() = %v, want the declaration to be stale",
				c.res.Status, rep.StaleDeclarations())
		}
	}
}

// withDeclaredVacancy declares one vacancy for the duration of a test.
//
// A CASE ABOUT THE DECLARATION MECHANISM MUST NOT BORROW A LIVE DECLARATION,
// withUnprojectedScope's argument one map over, and here it is sharper: the
// tree now declares NO vacancy at all, so the two cases below picked the
// lowest id of an empty map and drove the whole mechanism through "". They
// were green only while some check in this repository had nothing to look at,
// which is a state the backlog exists to end -- the tests that watch the
// mechanism would have retired with the last thing it excused.
func withDeclaredVacancy(t *testing.T, id string, v vacancy) {
	t.Helper()
	prev := declaredVacuous
	next := make(map[string]vacancy, len(prev)+1)
	for k, val := range prev {
		next[k] = val
	}
	next[id] = v
	declaredVacuous = next
	t.Cleanup(func() { declaredVacuous = prev })
}

// testVacancy is the declaration the cases below install. The id is a real
// check, because Declaration.Ran distinguishes "this run had no such check"
// from a stale entry and a made-up id would exercise the wrong arm.
func testVacancy(t *testing.T) (string, vacancy) {
	t.Helper()
	const id = "transfer-legs-pair"
	// THE BEAD IS DELIBERATELY NOT A fisc- ID. `make beadrefs` reads every
	// tracked file and refuses an id naming no bead, on the ground that a
	// pointer to nothing reads as though the work is tracked -- and a fixture
	// bead points at nothing by construction. What the two cases below need of
	// this field is only that StaleReason repeats it, so a string that could
	// not be mistaken for a real id serves better than one that could.
	v := vacancy{
		bead: "the-bead-this-fixture-stands-in-for",
		reason: "a fixture declaration, installed by the test that drives this mechanism " +
			"rather than borrowed from the tree, which declares no vacancy of its own",
	}
	withDeclaredVacancy(t, id, v)
	return id, v
}
