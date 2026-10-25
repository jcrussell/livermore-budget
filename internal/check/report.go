package check

import (
	"context"
	"fmt"
)

// Report is a verify run's account of itself. Every key is present on every
// run, with no omitempty and no nil slice, so a diff of two reports never
// reads a data change as a schema change.
type Report struct {
	// GeneratedBy is the binary that ran the checks, as `fisc --version` prints
	// it.
	GeneratedBy string `json:"generated_by"`
	// Full and Strict say how the run was configured. Full is read off the
	// subject, since what the loader supplied decides whether a check ran.
	Full   bool   `json:"full"`
	Strict bool   `json:"strict"`
	Counts counts `json:"counts"`
	// Results is one entry per check in [All]'s order, skipped ones included.
	Results []Result `json:"results"`
	// Declared is every entry in Subject.Vacancies with what its check reported
	// here, in check-id order, so the exemption surface is in the report.
	Declared []declaration `json:"declared_vacuous"`
	// Undeclared are the vacuous checks no declaration covers. They are what
	// --strict fails on; see [Report.Failed].
	Undeclared []string `json:"undeclared_vacuous"`
}

// counts is how many checks reached each status. They are five separate
// counters and never summed into a "checks passed" figure: vacuous and skipped
// are not passes.
type counts struct {
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
	// Strict makes an undeclared vacuous check fail the run; off by default
	// (fisc-1wr.4).
	Strict bool
}

// Run evaluates checks over s, in the order given, and reports what each
// concluded. It never returns an error: every way a check can go wrong is a
// status. Cancellation is tested between checks, and a cancelled run reports
// every remaining check as an error rather than truncating Results.
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
	rep.resolveDeclarations(s.Vacancies)
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
// counts it. An unrecognized status, including a zero Result's empty one, is
// recorded as an error.
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

// Failed reports whether this run should fail the command: on a failure or an
// error, never on a skip. An undeclared vacuous check fails only under
// --strict; a stale declaration fails either way, being a false statement in
// source rather than a coverage shortfall.
func (r *Report) Failed() bool {
	if r.Counts.Fail > 0 || r.Counts.Error > 0 {
		return true
	}
	if len(r.StaleDeclarations()) > 0 {
		return true
	}
	return r.Strict && len(r.Undeclared) > 0
}
