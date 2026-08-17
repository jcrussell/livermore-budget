package check

import (
	"context"
	"fmt"
)

// Report is a verify run's account of itself.
//
// Every key is present on every run, in declaration order, with no omitempty
// and no nil slice — the discipline pkg/cmd/build's Report follows, for the same
// reason: this document is read by diffing two releases of it, and a key that
// disappears when its value is empty makes a data change read as a schema
// change.
//
// Counts is a struct rather than four loose fields because the vacuous count has
// to be as visible as the pass count. A summary that reads "11 of 12 passed"
// while three checks looked at nothing is the failure mode this whole package
// exists to prevent.
type Report struct {
	// GeneratedBy is the binary that ran the checks, as `fisc --version` prints
	// it, so a report that is pasted into an issue says what produced it.
	GeneratedBy string `json:"generated_by"`
	// Full and Strict say how the run was configured, because the same report
	// shape means different things under each: without Full some checks never
	// ran, and without Strict a vacuous check did not fail the run.
	//
	// Full is read off the subject rather than off the flag, because what decides
	// whether a check ran is what the loader supplied. Today Load carries the
	// flag through unchanged, so the two agree; see LoadOptions.Full for the gap
	// that leaves.
	Full   bool   `json:"full"`
	Strict bool   `json:"strict"`
	Counts Counts `json:"counts"`
	// Results is one entry per check in [All]'s order, including the checks that
	// were skipped. A check that produced no entry would be indistinguishable
	// from a check that does not exist.
	Results []Result `json:"results"`
}

// Counts is how many checks reached each status. They are five separate
// counters and never summed into a "checks passed" figure: vacuous and skipped
// are not passes.
type Counts struct {
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Vacuous int `json:"vacuous"`
	Skipped int `json:"skipped"`
	Error   int `json:"error"`
}

// ReportOptions are the run-level facts a report states about itself.
type ReportOptions struct {
	// GeneratedBy is the version line of the binary running the checks.
	GeneratedBy string
	// Strict makes a vacuous check fail the run. It is off by default because a
	// check with nothing to look at is the expected state of a corpus that is
	// 10% mapped, and a gate that is red until mapping finishes is a gate that
	// gets commented out (fisc-1wr.4). It is on in the run that must not let a
	// vacuous check quietly become permanent.
	Strict bool
}

// Run evaluates checks over s, in the order given, and reports what each
// concluded.
//
// It never returns an error. Every way a check can go wrong is a status in the
// report: that is what keeps "one check could not run" from hiding the eleven
// results that did, and what lets the caller decide the exit code from the
// counts rather than from whichever failure happened to be returned first.
//
// Cancellation is tested between checks, not inside them: a cancelled run
// reports every remaining check as an error rather than truncating Results,
// because a report with checks missing from it would read as a shorter list of
// checks rather than as an incomplete run.
func Run(ctx context.Context, s *Subject, checks []Check, o ReportOptions) *Report {
	rep := &Report{
		GeneratedBy: o.GeneratedBy,
		Full:        s.Full,
		Strict:      o.Strict,
		Results:     make([]Result, 0, len(checks)),
	}
	for _, c := range checks {
		rep.add(c, run1(ctx, c, s))
	}
	return rep
}

// run1 produces one check's result, including the results the check itself
// cannot produce: skipped, and the error a returned error becomes.
func run1(ctx context.Context, c Check, s *Subject) Result {
	if err := ctx.Err(); err != nil {
		return errored(err)
	}
	if c.Full() && !s.Full {
		return Result{
			Status: StatusSkipped,
			Summary: "needs the source documents under data/pdf/, which --full was not " +
				"given to look for",
			Findings: []Finding{},
		}
	}
	res, err := c.Run(ctx, s)
	if err != nil {
		return errored(err)
	}
	return res
}

// errored is the result of a check that could not reach a verdict.
func errored(err error) Result {
	return Result{Status: StatusError, Summary: err.Error(), Findings: []Finding{}}
}

// add stamps a result with the identity of the check that produced it and
// counts it.
//
// A status this package does not recognize — including the empty string a
// zero-valued Result carries — is recorded as an error rather than ignored. A
// check that returns nothing is broken, and of the ways to report a broken
// check, the one that does not go into a green count is the safe one.
func (r *Report) add(c Check, res Result) {
	res.CheckID = c.ID()
	res.Tier = c.Tier()
	res.Description = c.Description()
	if res.Findings == nil {
		res.Findings = []Finding{}
	}
	switch res.Status {
	case StatusPass:
		r.Counts.Pass++
	case StatusFail:
		r.Counts.Fail++
	case StatusVacuous:
		r.Counts.Vacuous++
	case StatusSkipped:
		r.Counts.Skipped++
	case StatusError:
		r.Counts.Error++
	default:
		broken := errored(fmt.Errorf("check %s returned status %q, which is not one of "+
			"pass, fail, vacuous, skipped, error", c.ID(), res.Status))
		broken.CheckID, broken.Tier, broken.Description = res.CheckID, res.Tier, res.Description
		res = broken
		r.Counts.Error++
	}
	r.Results = append(r.Results, res)
}

// Failed reports whether this run should fail the command.
//
// A failure and an error both fail: one says the corpus is wrong, the other that
// the checker could not tell, and neither is a green run. They are not told apart
// here because both mean the same thing to a caller — read the report, which says
// per check which happened. A skipped check never fails; it is the documented
// consequence of not passing --full. A vacuous check fails only under --strict;
// see ReportOptions.Strict for why that is not the default.
func (r *Report) Failed() bool {
	if r.Counts.Fail > 0 || r.Counts.Error > 0 {
		return true
	}
	return r.Strict && r.Counts.Vacuous > 0
}
