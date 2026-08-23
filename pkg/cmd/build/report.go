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

	// ColumnsTiedByDeclaration is how many of ColumnsTied tied only because
	// the rule declared a discrepancy in the document's own arithmetic
	// (fisc-2sd). Reported separately and printed whenever it is non-zero,
	// because "24 columns tie" and "23 tie exactly and one ties to a declared
	// $1 of the city's rounding" are different claims, and a build that stated
	// the first while meaning the second would be overstating its evidence.
	ColumnsTiedByDeclaration int `json:"columns_tied_by_declaration"`

	// SpanningRulesChecked is how many of the checks above were rule-level
	// rather than per-part -- a block whose rows straddle a page break, tied
	// once against the one total the document prints for it
	// (mapping.Rule.TotalSpansParts).
	//
	// It is reported because it changes what the two counters above MEAN
	// without changing what they COUNT. ColumnsTied is the number of column
	// comparisons made, and PartsChecked the number of parts those comparisons
	// corroborate; for a per-part check those coincide part by part, and for a
	// spanning rule one set of comparisons corroborates several parts. So a
	// build with spanning rules can report more parts checked than columns
	// tied, which is correct and would otherwise look like a bug.
	SpanningRulesChecked int `json:"spanning_rules_checked"`

	// RollupsAsserted is how many printed totals covering SEVERAL rules were
	// tied, and RollupColumnsTied the columns they compared. They are separate
	// from the counters above because they measure a different link of the
	// chain: a rollup ties printed subtotals to a printed total, where
	// CheckTotals ties mapped rows to a printed total.
	RollupsAsserted   int `json:"rollups_asserted"`
	RollupColumnsTied int `json:"rollup_columns_tied"`

	PartsUnchecked    []UncheckedPart    `json:"parts_unchecked"`
	RollupsUnasserted []UnassertedRollup `json:"rollups_unasserted"`
	Omissions         []DeclaredOmission `json:"declared_omissions"`
}

// UnassertedRollup is a total the DOCUMENT prints over several rules that no
// rule structure here can assert, with the declared reason.
//
// It is reported rather than omitted because silence is the failure mode. A
// build that mapped p140's ten pages and never mentioned that the page's own
// closing total exceeds them by ~$57M would be publishing the gap as though it
// were not there.
type UnassertedRollup struct {
	ID     string `json:"id"`
	Page   int    `json:"page"`
	Reason string `json:"reason"`
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
		PartsUnchecked:    []UncheckedPart{},
		RollupsUnasserted: []UnassertedRollup{},
		Omissions:         []DeclaredOmission{},
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
	res, err := r.CheckTotals(rule, p)
	switch {
	case err == nil:
		rep.PartsChecked++
		// TotalsResult.Columns counts the non-skip columns the check actually
		// compared; a skipped column produces no facts, so tying it would
		// count coverage this build did not earn.
		rep.ColumnsTied += res.Columns
		rep.ColumnsTiedByDeclaration += res.Declared
		return nil
	case errors.Is(err, mapping.ErrNoStatedTotals):
		rep.unchecked(rule, p, reasonNoStatedTotals)
		return nil
	default:
		return err
	}
}

// checkSpanningTotals runs the rule-level totals check for a block whose rows
// straddle a page break, and credits EVERY part of the rule.
//
// Crediting all of them is the honest accounting and not a generosity: the one
// comparison covers every row the rule maps, so every one of its parts rests on
// the document's arithmetic rather than on ours. Crediting only the part that
// prints the total would leave the others in PartsUnchecked under a reason
// saying the rule declares no total_row -- which is false, and is exactly the
// misreport this whole flag exists to remove.
func (rep *Report) checkSpanningTotals(r *mapping.Resolver, rule *mapping.Rule) error {
	res, err := r.CheckSpanningTotals(rule)
	if err != nil {
		// Unlike the per-part path there is no ErrNoStatedTotals branch here.
		// A rule that DECLARES its total spans its parts has asserted that one
		// of its pages prints that total; if none does, that is a false
		// declaration and a hard failure, not an unchecked part.
		return err
	}
	rep.SpanningRulesChecked++
	rep.PartsChecked += len(rule.Parts)
	rep.ColumnsTied += res.Columns
	rep.ColumnsTiedByDeclaration += res.Declared
	return nil
}

// checkRollups asserts each printed total that covers several rules, and
// records the ones the document prints that cannot be asserted.
func (rep *Report) checkRollups(r *mapping.Resolver, f *mapping.File) error {
	for i := range f.Rollups {
		ro := &f.Rollups[i]
		// Empty is spelled one way: the parser refuses a whitespace-only
		// reason, so this and validateRollups cannot disagree about which
		// entries are declarations.
		if ro.Unassertable != "" {
			rep.RollupsUnasserted = append(rep.RollupsUnasserted,
				UnassertedRollup{ID: ro.ID, Page: ro.Page, Reason: ro.Unassertable})
			continue
		}
		res, err := r.CheckRollup(ro)
		if err != nil {
			return err
		}
		rep.RollupsAsserted++
		rep.RollupColumnsTied += res.Columns
	}
	return nil
}

func (rep *Report) unchecked(rule *mapping.Rule, p *mapping.Part, reason string) {
	rep.PartsUnchecked = append(rep.PartsUnchecked,
		UncheckedPart{RuleID: rule.ID, Page: p.Page, Reason: reason})
}

func (rep *Report) addOmissions(rule *mapping.Rule, omissions []mapping.Omission) {
	for _, o := range omissions {
		rep.Omissions = append(rep.Omissions,
			DeclaredOmission{RuleID: rule.ID, Page: o.Page, RowLabel: o.Row.PrintedLabel()})
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
	if rep.SpanningRulesChecked > 0 {
		fmt.Fprintf(w, "%d of those %s checked against a total spanning its parts\n",
			rep.SpanningRulesChecked,
			plural(rep.SpanningRulesChecked, "rule was", "rules were"))
	}
	if rep.ColumnsTiedByDeclaration > 0 {
		// "of those columns" stays plural and the VERB agrees: one column ties,
		// several tie. Pluralising the noun here gives "1 of those column tie".
		fmt.Fprintf(w, "%d of those columns %s only to a declared delta in the document's own arithmetic\n",
			rep.ColumnsTiedByDeclaration,
			plural(rep.ColumnsTiedByDeclaration, "ties", "tie"))
	}
	if rep.RollupsAsserted > 0 {
		fmt.Fprintf(w, "%d printed %s covering several rules %s, over %d %s\n",
			rep.RollupsAsserted, plural(rep.RollupsAsserted, "total", "totals"),
			plural(rep.RollupsAsserted, "ties", "tie"), rep.RollupColumnsTied,
			plural(rep.RollupColumnsTied, "column", "columns"))
	}
	for _, u := range rep.RollupsUnasserted {
		fmt.Fprintf(w, "UNASSERTED ROLLUP %s p%d: the document prints this total and %s\n",
			u.ID, u.Page, u.Reason)
	}
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
