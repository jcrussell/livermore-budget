package check

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fake is a check whose verdict the test chooses, for the engine's own
// behaviour: what a returned error becomes, what --full does, and what happens to
// a check that answers with something this package does not recognize. The real
// checks cannot express those cases.
type fake struct {
	id   string
	full bool
	res  Result
	err  error
	// ran records that Run was called, which is the only way to tell a skipped
	// check from one that ran and reported nothing.
	ran bool
}

func (f *fake) ID() string          { return f.id }
func (*fake) Tier() int             { return 1 }
func (f *fake) Description() string { return "fixture check " + f.id }
func (f *fake) Full() bool          { return f.full }

func (f *fake) Run(context.Context, *Subject) (Result, error) {
	f.ran = true
	return f.res, f.err
}

var _ Check = (*fake)(nil)

func passed(id string) *fake {
	return &fake{id: id, res: Result{Status: StatusPass, Subjects: 1, Summary: "held"}}
}

func failed(id string) *fake {
	return &fake{id: id, res: Result{Status: StatusFail, Subjects: 2, Summary: "one of two did not hold",
		Findings: []Finding{{Subject: "node/x", Detail: "is 3, want 4"}}}}
}

func run(t *testing.T, s *Subject, o ReportOptions, checks ...Check) *Report {
	t.Helper()
	if s == nil {
		s = &Subject{}
	}
	return Run(t.Context(), s, checks, o)
}

// TestAnErrorIsNotAFailure is the distinction StatusError exists for. "The corpus
// is wrong" and "the checker could not tell" call for different responses, and a
// report that collapsed them would send someone looking for a bad figure when the
// bug is here.
func TestAnErrorIsNotAFailure(t *testing.T) {
	broke := &fake{id: "broke", err: errors.New("the manifest is unreadable")}
	rep := run(t, nil, ReportOptions{}, failed("claim"), broke)

	if diff := cmp.Diff(Counts{Fail: 1, Error: 1}, rep.Counts); diff != "" {
		t.Errorf("counts mismatch (-want +got):\n%s", diff)
	}
	got := statuses(rep)
	if diff := cmp.Diff(map[string]string{
		"claim": "fail over 2",
		"broke": "error over 0",
	}, got); diff != "" {
		t.Errorf("statuses mismatch (-want +got):\n%s", diff)
	}
	// The error's message is the summary, because a reader of the report has no
	// other way to see it: an error is not a Finding about a subject.
	if res := resultFor(t, rep, "broke"); res.Summary != "the manifest is unreadable" {
		t.Errorf("summary = %q, want the error's message", res.Summary)
	}
	// Both fail the run, and neither needs --strict to do it.
	if !rep.Failed() {
		t.Error("Failed() = false for a report with a failure and an error in it")
	}
}

// TestFullOnlyChecksAreSkippedNotFailed covers the flag no check reads yet. The
// path has to be wired end to end now, or the first check that needs --full will
// be the one discovering whether the wiring works.
func TestFullOnlyChecksAreSkippedNotFailed(t *testing.T) {
	needsPDFs := &fake{id: "needs-pdfs", full: true,
		res: Result{Status: StatusPass, Subjects: 1, Summary: "read the PDFs"}}

	rep := run(t, &Subject{Full: false}, ReportOptions{}, needsPDFs)
	if got := resultFor(t, rep, "needs-pdfs").Status; got != StatusSkipped {
		t.Errorf("status = %s without --full, want skipped", got)
	}
	if needsPDFs.ran {
		t.Error("the check ran without --full, so its input was never actually optional")
	}
	if rep.Failed() {
		t.Error("Failed() = true for a run whose only check was skipped")
	}
	if !strings.Contains(resultFor(t, rep, "needs-pdfs").Summary, "--full") {
		t.Error("the skipped result does not say what would have run it")
	}

	rep = run(t, &Subject{Full: true}, ReportOptions{}, needsPDFs)
	if got := resultFor(t, rep, "needs-pdfs").Status; got != StatusPass {
		t.Errorf("status = %s with --full, want pass", got)
	}
	if !needsPDFs.ran {
		t.Error("the check did not run under --full")
	}
}

// TestSkippedNeverFailsEvenUnderStrict pins the difference between "nothing to
// check" and "did not look". --strict is about the first: a vacuous check is a gap
// in the corpus, while a skipped one is the documented consequence of not passing
// a flag, and failing on it would make --strict impossible to use in CI.
func TestSkippedNeverFailsEvenUnderStrict(t *testing.T) {
	rep := run(t, &Subject{}, ReportOptions{Strict: true},
		&fake{id: "needs-pdfs", full: true, res: Result{Status: StatusPass, Subjects: 1}})
	if rep.Counts.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", rep.Counts.Skipped)
	}
	if rep.Failed() {
		t.Error("Failed() = true under --strict for a skipped check")
	}
}

// TestABrokenCheckCannotBeGreen covers the check that answers with nothing at
// all — a zero Result, whose status is the empty string. Counting it anywhere but
// the error column would let a broken check read as a passing one.
func TestABrokenCheckCannotBeGreen(t *testing.T) {
	rep := run(t, nil, ReportOptions{}, &fake{id: "silent"})

	res := resultFor(t, rep, "silent")
	if res.Status != StatusError {
		t.Fatalf("status = %q, want error", res.Status)
	}
	if !strings.Contains(res.Summary, "silent") || !strings.Contains(res.Summary, "not one of") {
		t.Errorf("summary %q does not say which check returned what", res.Summary)
	}
	if rep.Counts.Pass != 0 || rep.Counts.Error != 1 {
		t.Errorf("counts = %+v, want the broken check in the error column alone", rep.Counts)
	}
}

// TestResultsAreStampedFromTheCheck keeps a result from claiming an identity its
// check does not answer to, which matters because the id is what a reader greps
// for and what a vacuity declaration names.
func TestResultsAreStampedFromTheCheck(t *testing.T) {
	liar := &fake{id: "honest", res: Result{
		CheckID: "some-other-check", Tier: 4, Description: "not mine",
		Status: StatusPass, Subjects: 1,
	}}
	res := resultFor(t, run(t, nil, ReportOptions{}, liar), "honest")

	if res.Tier != 1 || res.Description != "fixture check honest" {
		t.Errorf("tier %d description %q, want the check's own", res.Tier, res.Description)
	}
}

// TestCancellationStillReportsEveryCheck: a report with checks missing from it
// reads as a shorter list of checks rather than as an interrupted run.
func TestCancellationStillReportsEveryCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	first, second := passed("first"), passed("second")
	rep := Run(ctx, &Subject{}, []Check{first, second}, ReportOptions{})

	if got := len(rep.Results); got != 2 {
		t.Fatalf("results = %d, want one per check", got)
	}
	if rep.Counts.Error != 2 {
		t.Errorf("counts = %+v, want both checks reported as errors", rep.Counts)
	}
	if first.ran || second.ran {
		t.Error("a check ran after the context was cancelled")
	}
}

// TestFindingsAreNeverNull keeps the JSON shape stable. A key that becomes null
// when a list is empty makes a consumer handle two shapes for one meaning.
func TestFindingsAreNeverNull(t *testing.T) {
	rep := run(t, nil, ReportOptions{}, passed("a"), &fake{id: "b", err: errors.New("x")})
	if rep.Results == nil {
		t.Error("Results is nil")
	}
	for _, res := range rep.Results {
		if res.Findings == nil {
			t.Errorf("%s: findings are nil", res.CheckID)
		}
	}
	// And on a report with no checks at all, which is the shape a caller who
	// filtered every check away would get.
	if empty := run(t, nil, ReportOptions{}); empty.Results == nil {
		t.Error("Results is nil for a run with no checks")
	}
}

// TestAllIsNotAppendable covers why All is a function. A package variable could
// be appended to — or truncated — by any importer, which would silently change
// what `fisc verify` checks.
func TestAllIsNotAppendable(t *testing.T) {
	first := All()
	first[0] = passed("hijacked")
	first = append(first, passed("extra"))

	second := All()
	if len(second) == len(first) {
		t.Error("appending to All()'s result changed the published set")
	}
	if second[0].ID() == "hijacked" {
		t.Error("writing to All()'s result changed the published set")
	}
}

// TestEveryCheckIsWellFormed guards the contract in Check.ID: the ids are what a
// vacuity declaration names and what a reader greps for, so they have to be
// unique and stable, and every check has to be able to say what it claims.
func TestEveryCheckIsWellFormed(t *testing.T) {
	kebab := regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	seen := map[string]bool{}
	for _, c := range All() {
		id := c.ID()
		switch {
		case !kebab.MatchString(id):
			t.Errorf("check id %q is not kebab-case", id)
		case seen[id]:
			t.Errorf("two checks answer to the id %q", id)
		}
		seen[id] = true
		if c.Description() == "" {
			t.Errorf("%s has no description, so a report of it explains nothing", id)
		}
		if tier := c.Tier(); tier < 0 || tier > 4 {
			t.Errorf("%s is tier %d, want 0 (structural) or 1 to 4", id, tier)
		}
	}
	if len(seen) != len(All()) {
		t.Errorf("All() returned %d checks with %d distinct ids", len(All()), len(seen))
	}
}
