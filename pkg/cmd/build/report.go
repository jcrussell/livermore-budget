package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// report is the build's account of itself. It names every unchecked part with
// its rule and page, because a column with no printed total rests only on our
// arithmetic and a report of failures alone would read as all good.
type report struct {
	// Output is the repository-relative path that was written.
	Output string `json:"output"`

	Facts     int `json:"facts"`
	RuleFiles int `json:"rule_files"`
	Rules     int `json:"rules"`
	Parts     int `json:"parts"`

	// PartsChecked is how many parts tied to a printed total, and ColumnsTied
	// how many columns those parts covered; a part is checked column by column.
	PartsChecked int `json:"parts_checked"`
	ColumnsTied  int `json:"columns_tied_to_a_stated_total"`

	// ColumnsTiedByDeclaration is how many of ColumnsTied tied only through a
	// declared discrepancy in the document's own arithmetic, reported apart so
	// an exact tie and a declared $1 of rounding are not one claim.
	ColumnsTiedByDeclaration int `json:"columns_tied_by_declaration"`

	// ColumnsTiedByTolerance is how many of ColumnsTied tied only inside the
	// tolerance a rule's printed_decimals derives, and ToleranceSlack what they
	// were out by. Kept apart from ColumnsTiedByDeclaration: a declaration names
	// the exact discrepancy, a tolerance bounds an unnamed one.
	ColumnsTiedByTolerance int            `json:"columns_tied_by_tolerance"`
	ToleranceSlack         []amount.Cents `json:"tolerance_slack"`

	// SpanningRulesChecked is how many checks were rule-level, one total over
	// a block straddling a page break (mapping.Rule.TotalSpansParts). One such
	// check credits several parts, so PartsChecked can exceed ColumnsTied.
	SpanningRulesChecked int `json:"spanning_rules_checked"`

	// RollupsAsserted is how many printed totals covering several rules tied,
	// and RollupColumnsTied the columns compared: printed subtotals to a
	// printed total, a different link from CheckTotals.
	RollupsAsserted   int `json:"rollups_asserted"`
	RollupColumnsTied int `json:"rollup_columns_tied"`

	// SubtotalLinesTied is how many printed subtotal rows tied to the rows
	// above them (mapping.Row.Subtotal), and SubtotalCellsTied their figures. A
	// part whose subtotal chain ties counts as checked with no total_row.
	SubtotalLinesTied int `json:"subtotal_lines_tied"`
	SubtotalCellsTied int `json:"subtotal_cells_tied"`

	// SubtotalCellsTiedByDeclaration is how many of those tie only through a
	// declared subtotal_deltas entry.
	SubtotalCellsTiedByDeclaration int `json:"subtotal_cells_tied_by_declaration"`

	PartsUnchecked    []uncheckedPart    `json:"parts_unchecked"`
	RollupsUnasserted []unassertedRollup `json:"rollups_unasserted"`
	Omissions         []declaredOmission `json:"declared_omissions"`
}

// unassertedRollup is a total the document prints over several rules that no
// rule structure can assert, with the declared reason. Reported, because
// silence would publish a gap (p140's ~$57M) as though it were not there.
type unassertedRollup struct {
	ID     string `json:"id"`
	Page   int    `json:"page"`
	Reason string `json:"reason"`
}

// uncheckedPart is a part whose figures no printed total corroborates.
type uncheckedPart struct {
	RuleID string `json:"rule_id"`
	Page   int    `json:"page"`
	Reason string `json:"reason"`
}

// declaredOmission is one cell the rule says the page leaves blank, under
// Column's header, on a row it prints. It produces no fact, so only this
// surfaces it. A row another page prints is not one.
type declaredOmission struct {
	RuleID   string `json:"rule_id"`
	Page     int    `json:"page"`
	RowLabel string `json:"row_label"`
	Column   string `json:"column"`
}

// Reasons a part could not be checked: no total to check against, or a total
// with no anchor on a label-less page. Neither claims the document prints no
// total; ErrNoStatedTotals reaches here only from a label-less part with no
// stop_at, a gap in the rule.
const (
	reasonNoTotalRow     = "the rule declares no total_row"
	reasonNoStatedTotals = "the rule gives this part no anchor for a stated total"
)

// newReport starts with non-nil slices, so --json has one shape whether or not
// anything went unchecked.
func newReport() *report {
	return &report{
		ToleranceSlack:    []amount.Cents{},
		PartsUnchecked:    []uncheckedPart{},
		RollupsUnasserted: []unassertedRollup{},
		Omissions:         []declaredOmission{},
	}
}

// checkTotals runs the document's own totals check for one part. It keys on
// rule.TotalRow rather than the error: with no total row, StatedTotals returns
// ErrNotFound, the same error a moved total row gives, which must fail the
// build. ErrNoStatedTotals is reported unchecked, neither pass nor failure.
func (rep *report) checkTotals(r *mapping.Resolver, rule *mapping.Rule, p *mapping.Part) error {
	if rule.TotalRow == "" {
		rep.unchecked(rule, p, reasonNoTotalRow)
		return nil
	}
	res, err := r.CheckTotals(rule, p)
	switch {
	case err == nil:
		rep.PartsChecked++
		// Columns counts only compared, non-skip columns.
		rep.ColumnsTied += res.Columns
		rep.ColumnsTiedByDeclaration += res.Declared
		rep.ColumnsTiedByTolerance += res.Tolerated
		rep.ToleranceSlack = append(rep.ToleranceSlack, res.Slack...)
		return nil
	case errors.Is(err, mapping.ErrNoStatedTotals):
		rep.unchecked(rule, p, reasonNoStatedTotals)
		return nil
	default:
		return err
	}
}

// checkSpanningTotals runs the rule-level totals check for a block whose rows
// straddle a page break, and credits every part of the rule: the one
// comparison covers every row it maps.
func (rep *report) checkSpanningTotals(r *mapping.Resolver, rule *mapping.Rule) error {
	res, err := r.CheckSpanningTotals(rule)
	if err != nil {
		// No ErrNoStatedTotals branch: a rule declaring a spanning total
		// asserts one of its pages prints it, so none is a hard failure.
		return err
	}
	rep.SpanningRulesChecked++
	rep.PartsChecked += len(rule.Parts)
	rep.ColumnsTied += res.Columns
	rep.ColumnsTiedByDeclaration += res.Declared
	rep.ColumnsTiedByTolerance += res.Tolerated
	rep.ToleranceSlack = append(rep.ToleranceSlack, res.Slack...)
	return nil
}

// checkRollups asserts each printed total that covers several rules, and
// records the ones the document prints that cannot be asserted.
func (rep *report) checkRollups(r *mapping.Resolver, f *mapping.File) error {
	for i := range f.Rollups {
		ro := &f.Rollups[i]
		// The parser refuses a whitespace-only reason, so this and
		// validateRollups agree on which entries are declarations.
		if ro.Unassertable != "" {
			rep.RollupsUnasserted = append(rep.RollupsUnasserted,
				unassertedRollup{ID: ro.ID, Page: ro.Page, Reason: ro.Unassertable})
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

func (rep *report) unchecked(rule *mapping.Rule, p *mapping.Part, reason string) {
	rep.PartsUnchecked = append(rep.PartsUnchecked,
		uncheckedPart{RuleID: rule.ID, Page: p.Page, Reason: reason})
}

func (rep *report) addOmissions(rule *mapping.Rule, omissions []mapping.Omission) {
	for _, o := range omissions {
		rep.Omissions = append(rep.Omissions, declaredOmission{
			RuleID: rule.ID, Page: o.Page, RowLabel: o.Row.PrintedLabel(), Column: o.Header})
	}
}

// print reports the build on ErrOut; --json is the only thing this command
// puts on Out.
func (rep *report) print(ios *iostreams.IOStreams, asJSON bool) error {
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
		rep.Rules, rep.Parts, rep.RuleFiles, english.Plural(rep.RuleFiles, "file", "files"))
	fmt.Fprintf(w, "%d of %d parts tie to a total the document prints, covering %d %s\n",
		rep.PartsChecked, rep.Parts, rep.ColumnsTied,
		english.Plural(rep.ColumnsTied, "column", "columns"))
	if rep.SpanningRulesChecked > 0 {
		fmt.Fprintf(w, "%d of those %s checked against a total spanning its parts\n",
			rep.SpanningRulesChecked,
			english.Plural(rep.SpanningRulesChecked, "rule was", "rules were"))
	}
	if rep.ColumnsTiedByDeclaration > 0 {
		// The verb agrees, not the noun: "1 of those column tie" is wrong.
		fmt.Fprintf(w, "%d of those columns %s only to a declared delta in the document's own arithmetic\n",
			rep.ColumnsTiedByDeclaration,
			english.Plural(rep.ColumnsTiedByDeclaration, "ties", "tie"))
	}
	if rep.ColumnsTiedByTolerance > 0 {
		// The slack is printed so a reader can check it against the page.
		amounts := make([]string, 0, len(rep.ToleranceSlack))
		for _, c := range rep.ToleranceSlack {
			amounts = append(amounts, c.String())
		}
		fmt.Fprintf(w, "%d of those columns %s only within the tolerance the page's own printed precision allows, by %s\n",
			rep.ColumnsTiedByTolerance,
			english.Plural(rep.ColumnsTiedByTolerance, "ties", "tie"),
			strings.Join(amounts, ", "))
	}
	if rep.SubtotalLinesTied > 0 {
		fmt.Fprintf(w, "%d printed %s inside a block %s the rows above, over %d %s\n",
			rep.SubtotalLinesTied, english.Plural(rep.SubtotalLinesTied, "subtotal", "subtotals"),
			english.Plural(rep.SubtotalLinesTied, "ties to", "tie to"), rep.SubtotalCellsTied,
			english.Plural(rep.SubtotalCellsTied, "figure", "figures"))
		if rep.SubtotalCellsTiedByDeclaration > 0 {
			fmt.Fprintf(w, "%d of those figures %s only to a declared delta in the document's own arithmetic\n",
				rep.SubtotalCellsTiedByDeclaration,
				english.Plural(rep.SubtotalCellsTiedByDeclaration, "ties", "tie"))
		}
	}
	if rep.RollupsAsserted > 0 {
		fmt.Fprintf(w, "%d printed %s covering several rules %s, over %d %s\n",
			rep.RollupsAsserted, english.Plural(rep.RollupsAsserted, "total", "totals"),
			english.Plural(rep.RollupsAsserted, "ties", "tie"), rep.RollupColumnsTied,
			english.Plural(rep.RollupColumnsTied, "column", "columns"))
	}
	for _, u := range rep.RollupsUnasserted {
		fmt.Fprintf(w, "UNASSERTED ROLLUP %s p%d: the document prints this total and %s\n",
			u.ID, u.Page, u.Reason)
	}
	for _, u := range rep.PartsUnchecked {
		fmt.Fprintf(w, "UNCHECKED %s p%d: %s\n", u.RuleID, u.Page, u.Reason)
	}
	for _, o := range rep.Omissions {
		fmt.Fprintf(w, "DECLARED OMISSION %s p%d: the page leaves row %q blank under %q\n",
			o.RuleID, o.Page, o.RowLabel, o.Column)
	}
	return nil
}
