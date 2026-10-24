package check

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// vacant is a check that reports vacuous under whatever id the test gives it,
// which is what lets these tests exercise a DECLARED id and an undeclared one
// with the same machinery.
func vacant(id string) *fake {
	return &fake{id: id, res: Result{Status: StatusVacuous, Summary: "nothing to look at"}}
}

// declarationFaults is every way a set of vacancy declarations is malformed:
// an id naming no check in real, an entry with no fisc- bead, a reason too
// thin to say what would have to exist, or two entries sharing a reason.
func declarationFaults(decls map[string]vacancy, real map[string]bool) []string {
	var out []string
	seen := map[string]string{}
	for _, id := range slices.Sorted(maps.Keys(decls)) {
		v := decls[id]
		if !real[id] {
			out = append(out, id+": names no check All() returns")
		}
		if !strings.HasPrefix(v.bead, "fisc-") {
			out = append(out, id+": no fisc- bead")
		}
		if len(v.reason) < 80 {
			out = append(out, id+": a reason of under 80 characters")
		}
		if prev, dup := seen[v.reason]; dup {
			out = append(out, id+": shares its reason with "+prev)
		}
		seen[v.reason] = id
	}
	return out
}

// TestEveryDeclarationIsWellFormed holds declaredVacuous to declarationFaults
// against the canonical registry: a run may hold a subset of the checks, so
// whether an id names a real check is asserted here and not by Run.
func TestEveryDeclarationIsWellFormed(t *testing.T) {
	t.Parallel()
	real := map[string]bool{}
	for _, c := range All() {
		real[c.ID()] = true
	}
	for _, f := range declarationFaults(declaredVacuous, real) {
		t.Error(f)
	}
}

// TestDeclarationFaultsRefusesEachMalformedEntry runs declarationFaults over
// one entry per fault, so the test above can fail while declaredVacuous is
// empty. Mutation: drop any arm and its entry goes unreported.
func TestDeclarationFaultsRefusesEachMalformedEntry(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a reason about the corpus that names what would have to exist ", 2)
	got := declarationFaults(map[string]vacancy{
		"ok":       {reason: long + "1", bead: "fisc-3s5d"},
		"typo":     {reason: long + "2", bead: "fisc-3s5d"},
		"no-bead":  {reason: long + "3", bead: ""},
		"thin":     {reason: "nothing", bead: "fisc-3s5d"},
		"copied-a": {reason: long + "4", bead: "fisc-3s5d"},
		"copied-b": {reason: long + "4", bead: "fisc-3s5d"},
	}, map[string]bool{"ok": true, "no-bead": true, "thin": true, "copied-a": true, "copied-b": true})
	want := []string{
		"copied-b: shares its reason with copied-a",
		"no-bead: no fisc- bead",
		"thin: a reason of under 80 characters",
		"typo: names no check All() returns",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("declarationFaults (-want +got):\n%s", diff)
	}
}

// TestADeclaredVacancyPassesStrictAndAnUndeclaredOneDoesNot is the mechanism's
// point in one test: --strict stops being a gate on how much is mapped and
// becomes a gate on whether anyone has looked.
func TestADeclaredVacancyPassesStrictAndAnUndeclaredOneDoesNot(t *testing.T) {
	t.Parallel()
	declared, s := testVacancy(t)

	rep := run(t, s, ReportOptions{Strict: true}, passed("a"), vacant(declared))
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

	rep = run(t, s, ReportOptions{Strict: true}, passed("a"), vacant("nothing-declares-this"))
	if !slices.Contains(rep.Undeclared, "nothing-declares-this") {
		t.Errorf("Undeclared = %v, want it to name the undeclared check", rep.Undeclared)
	}
	if !rep.Failed() {
		t.Error("Failed() = false under --strict for an UNDECLARED vacancy, which is " +
			"the only thing --strict is now for")
	}
	// And without --strict it is reported and tolerated, which is the behaviour
	// that keeps the default gate usable at 10% coverage.
	rep = run(t, s, ReportOptions{}, passed("a"), vacant("nothing-declares-this"))
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
	t.Parallel()
	declared, s := testVacancy(t)

	for _, strict := range []bool{false, true} {
		// The declared check now PASSES: the work landed and the entry did not
		// go with it.
		rep := run(t, s, ReportOptions{Strict: strict}, passed(declared))
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
		if !strings.Contains(stale[0].StaleReason(), s.Vacancies[declared].bead) {
			t.Errorf("strict=%v: stale reason does not name the bead that landed", strict)
		}
	}

	// A run that simply did not include the check is NOT stale.
	rep := run(t, s, ReportOptions{Strict: true}, passed("a"))
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
	t.Parallel()
	s := committed(t)
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
}

// TestADeclarationSurvivesACheckThatReachedNoVerdict: stale is a claim that the
// work landed, and only PASS and FAIL are verdicts. A check that ERRORED or was
// SKIPPED did not look, so the run establishes nothing about the declaration.
//
// ERROR IS THE REACHABLE ONE: run1 turns ctx.Err() into errored(...), so a
// Ctrl-C on `fisc verify` must not report a declaration stale. SKIPPED is
// asserted too, because ci.yml's --strict step passes no --full.
//
// A genuine error still fails the run. That is Counts.Error's job in Failed(),
// and it says the checker could not tell rather than claiming a bead landed.
func TestADeclarationSurvivesACheckThatReachedNoVerdict(t *testing.T) {
	t.Parallel()
	declared, s := testVacancy(t)

	t.Run("errored", func(t *testing.T) {
		broke := &fake{id: declared, err: errors.New("the manifest is unreadable")}
		rep := run(t, s, ReportOptions{Strict: true}, broke)

		if got := rep.StaleDeclarations(); len(got) != 0 {
			t.Errorf("StaleDeclarations() = %v for a check that errored; an error is "+
				"not a verdict and says nothing about whether %s landed",
				got, s.Vacancies[declared].bead)
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
		rep := run(t, s, ReportOptions{Strict: true}, gated)

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
		rep := run(t, s, ReportOptions{}, c)
		if len(rep.StaleDeclarations()) != 1 {
			t.Errorf("%s: StaleDeclarations() = %v, want the declaration to be stale",
				c.res.Status, rep.StaleDeclarations())
		}
	}
}

// testVacancy is the one declaration the cases below run over, on a subject
// carrying nothing else. A case about the mechanism must not borrow a live
// declaration: it would retire with the last thing the tree excuses. The id is
// a real check, because Declaration.Ran distinguishes "this run had no such
// check" from a stale entry and a made-up id would exercise the wrong arm.
func testVacancy(t *testing.T) (string, *Subject) {
	t.Helper()
	const id = "transfer-legs-pair"
	// Not a fisc- id: `make beadrefs` refuses one naming no bead, and the
	// cases need only that StaleReason repeats it.
	v := vacancy{
		bead: "the-bead-this-fixture-stands-in-for",
		reason: "a fixture declaration, installed by the test that drives this mechanism " +
			"rather than borrowed from the tree",
	}
	return id, &Subject{Vacancies: map[string]vacancy{id: v}}
}
