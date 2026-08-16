package build

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// Report is the build's account of itself.
//
// The unchecked list is why this is a struct rather than a couple of counters.
// A column that ties to the total the document prints is verified by the
// document; a column with no printed total rests on nothing but our own
// arithmetic, and most schedules in this corpus print no total at all. A build
// that reported only its failures would read as "all good" while most of what
// it published was never checked by anything. Naming every unchecked part, with
// its rule and page, is what keeps that distinction in front of the reader —
// and, via --json, in front of CI.
type Report struct {
	// Output is the repository-relative path that was written.
	Output string `json:"output"`

	Facts     int `json:"facts"`
	RuleFiles int `json:"rule_files"`
	Rules     int `json:"rules"`
	Parts     int `json:"parts"`

	// PartsChecked is how many parts tied to a total the document prints, and
	// ColumnsTied how many columns those parts covered. The column count is the
	// honest measure of coverage: a part is checked one column at a time.
	PartsChecked int `json:"parts_checked"`
	ColumnsTied  int `json:"columns_tied_to_a_stated_total"`

	PartsUnchecked []UncheckedPart    `json:"parts_unchecked"`
	Omissions      []DeclaredOmission `json:"declared_omissions"`
}

// UncheckedPart is a part whose figures no printed total corroborates.
type UncheckedPart struct {
	RuleID string `json:"rule_id"`
	Page   int    `json:"page"`
	Reason string `json:"reason"`
}

// DeclaredOmission is a row the rule says the page does not print. It is
// reported because it is a claim about the document that nothing else surfaces:
// the row produces no fact, so its absence is invisible in facts.jsonl.
type DeclaredOmission struct {
	RuleID   string `json:"rule_id"`
	Page     int    `json:"page"`
	RowLabel string `json:"row_label"`
}

// Reasons a part could not be checked. They are distinct because they call for
// different responses: the first is a rule that has not been given a total to
// check against, the second a rule that has been given one but has not said
// where on a label-less page the totals begin.
//
// Neither says the DOCUMENT prints no total, and the second used to. That is a
// claim about the source, and only a human reading the page can make it —
// ErrNoStatedTotals reaches here solely from a label-less part with no stop_at,
// which is a gap in the rule.
const (
	reasonNoTotalRow     = "the rule declares no total_row"
	reasonNoStatedTotals = "the rule gives this part no anchor for a stated total"
)

// newReport starts an empty report with non-nil slices, so the --json shape is
// the same whether or not anything went unchecked. A key that becomes null when
// a list is empty makes a consumer handle two shapes for one meaning.
func newReport() *Report {
	return &Report{
		PartsUnchecked: []UncheckedPart{},
		Omissions:      []DeclaredOmission{},
	}
}

// checkTotals runs the document's own totals check for one part, and records
// the parts it cannot run for.
//
// The condition is `rule.TotalRow != ""` and deliberately not "run it and see
// what comes back". A rule that declares no total row still has a stop_at
// anchor on its label-less parts, and StatedTotals will look for an amount run
// after that anchor: there is none, so the result is ErrNotFound — the same
// error a rule that DOES declare a total row produces when that row has moved,
// which is a genuine failure that must fail the build. Asking the rule keeps
// the two apart; asking the error cannot.
//
// ErrNoStatedTotals is neither a pass nor a failure. A part carrying a
// total_row that the document prints no total for is unchecked, exactly like a
// rule that declares none, and is reported the same way.
func (rep *Report) checkTotals(r *mapping.Resolver, rule *mapping.Rule, p *mapping.Part) error {
	if rule.TotalRow == "" {
		rep.unchecked(rule, p, reasonNoTotalRow)
		return nil
	}
	err := r.CheckTotals(rule, p)
	switch {
	case err == nil:
		rep.PartsChecked++
		for _, c := range p.Columns {
			// A skipped column produces no facts, so tying it would count
			// coverage this build did not earn.
			if !c.Skip {
				rep.ColumnsTied++
			}
		}
		return nil
	case errors.Is(err, mapping.ErrNoStatedTotals):
		rep.unchecked(rule, p, reasonNoStatedTotals)
		return nil
	default:
		return err
	}
}

func (rep *Report) unchecked(rule *mapping.Rule, p *mapping.Part, reason string) {
	rep.PartsUnchecked = append(rep.PartsUnchecked,
		UncheckedPart{RuleID: rule.ID, Page: p.Page, Reason: reason})
}

func (rep *Report) addOmissions(rule *mapping.Rule, omissions []mapping.Omission) {
	for _, o := range omissions {
		rep.Omissions = append(rep.Omissions,
			DeclaredOmission{RuleID: rule.ID, Page: o.Page, RowLabel: o.Row.Label})
	}
}

// print reports the build. The facts are the data and they went to a file, so
// the summary is chatter and belongs on ErrOut; --json is the machine-readable
// form and is the only thing this command ever puts on Out.
func (rep *Report) print(ios *iostreams.IOStreams, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(ios.Out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		return nil
	}

	w := ios.ErrOut
	fmt.Fprintf(w, "wrote %d facts to %s\n", rep.Facts, rep.Output)
	fmt.Fprintf(w, "%d rules over %d parts in %d rule %s\n",
		rep.Rules, rep.Parts, rep.RuleFiles, plural(rep.RuleFiles, "file", "files"))
	fmt.Fprintf(w, "%d of %d parts tie to a total the document prints, covering %d %s\n",
		rep.PartsChecked, rep.Parts, rep.ColumnsTied,
		plural(rep.ColumnsTied, "column", "columns"))
	for _, u := range rep.PartsUnchecked {
		fmt.Fprintf(w, "UNCHECKED %s p%d: %s\n", u.RuleID, u.Page, u.Reason)
	}
	for _, o := range rep.Omissions {
		fmt.Fprintf(w, "DECLARED OMISSION %s p%d: the page does not print row %q\n",
			o.RuleID, o.Page, o.RowLabel)
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
