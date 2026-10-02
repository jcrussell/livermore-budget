package mapping

import (
	"fmt"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// SubtotalsResult counts what [Resolver.CheckSubtotals] compared.
type SubtotalsResult struct {
	// Lines is the subtotal rows tied, and Cells the figures on them.
	Lines, Cells int
}

// Chains groups a file's rules into the runs CheckSubtotals reads, in file
// order: each named Rule.SubtotalChain is one run, and a rule with subtotal
// rows and no chain is a run of its own. Rules in neither are left out.
func Chains(f *File) [][]*Rule {
	var out [][]*Rule
	at := map[string]int{}
	for i := range f.Rules {
		rule := &f.Rules[i]
		name := rule.SubtotalChain
		if name == "" {
			if !slices.ContainsFunc(rule.Rows, func(r Row) bool { return r.Subtotal > 0 }) {
				continue
			}
			name = "rule:" + rule.ID
		}
		j, ok := at[name]
		if !ok {
			j = len(out)
			at[name] = j
			out = append(out, nil)
		}
		out[j] = append(out[j], rule)
	}
	return out
}

// CheckSubtotals walks a chain's rows in order and holds every subtotal row's
// figures to the rows above it, by column header (see Row.Subtotal).
//
// A row's figures are gathered from every part that prints it, so a labelled
// page and its label-less continuation are one row. A row that is not a
// subtotal adds to every level's running sum; a subtotal of level L compares
// against level L's sum and then clears every level up to L. A header in the
// sum that the subtotal row does not print is refused, and so is a row after
// the chain's last subtotal: a printed table's rows are all inside some total,
// and one that is not has been read from the wrong place.
func (r *Resolver) CheckSubtotals(chain []*Rule) (*SubtotalsResult, error) {
	levels := 0
	for _, rule := range chain {
		for _, row := range rule.Rows {
			levels = max(levels, row.Subtotal)
		}
	}
	if levels == 0 {
		return nil, fmt.Errorf("subtotal chain of rule %q declares no subtotal row", chain[0].ID)
	}
	sums := make([]map[string]amount.Cents, levels+1)
	rows := make([]int, levels+1)
	reset := func(upTo int) {
		for l := 1; l <= upTo; l++ {
			sums[l] = map[string]amount.Cents{}
			rows[l] = 0
		}
	}
	reset(levels)

	res := &SubtotalsResult{}
	var lastRule *Rule
	for _, rule := range chain {
		lastRule = rule
		figures := make([]map[string]Value, len(rule.Rows))
		for i := range rule.Parts {
			p := &rule.Parts[i]
			cells, err := r.Cells(rule, p)
			if err != nil {
				return nil, err
			}
			for _, c := range cells {
				if figures[c.RowIndex] == nil {
					figures[c.RowIndex] = map[string]Value{}
				}
				figures[c.RowIndex][p.ColumnHeaders[c.ColumnIndex].Text] = c
			}
		}
		for i, row := range rule.Rows {
			if row.Subtotal == 0 {
				for h, c := range figures[i] {
					for l := 1; l <= levels; l++ {
						sums[l][h] += c.Cents
					}
				}
				for l := 1; l <= levels; l++ {
					rows[l]++
				}
				continue
			}
			want := sums[row.Subtotal]
			deltas := map[string]amount.Cents{}
			for _, d := range row.SubtotalDeltas {
				deltas[d.Column] = d.Cents
			}
			headers := make([]string, 0, len(want))
			for h := range want {
				headers = append(headers, h)
			}
			slices.Sort(headers)
			for _, h := range headers {
				got, ok := figures[i][h]
				if !ok {
					return nil, r.subtotalError(rule, row, 0, fmt.Sprintf(
						"prints no figure under %q, which the %d row(s) above it do", h, rows[row.Subtotal]))
				}
				delta, declared := deltas[h]
				delete(deltas, h)
				switch {
				case declared && got.Cents == want[h]:
					return nil, r.subtotalError(rule, row, got.Page, fmt.Sprintf(
						"declares a delta of %s under %q, and the column ties exactly at %s",
						delta.String(), h, got.Token))
				case got.Cents != want[h]+delta:
					return nil, r.subtotalError(rule, row, got.Page, fmt.Sprintf(
						"prints %s under %q, and the %d row(s) above it since the last level-%d "+
							"subtotal sum to %s (a declared delta of %s)",
						got.Token, h, rows[row.Subtotal], row.Subtotal, want[h].String(),
						delta.String()))
				}
				res.Cells++
			}
			for h := range deltas {
				return nil, r.subtotalError(rule, row, 0, fmt.Sprintf(
					"declares a delta under %q, a column no row above it prints", h))
			}
			res.Lines++
			reset(row.Subtotal)
		}
	}
	if rows[1] > 0 {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: lastRule.ID,
			Page: lastRule.Parts[len(lastRule.Parts)-1].Page, Field: "rows", Err: ErrNotFound,
			Msg: fmt.Sprintf("%d row(s) after the chain's last subtotal are inside no printed total",
				rows[1])},
			"end the chain with the subtotal the page prints under them")
	}
	return res, nil
}

func (r *Resolver) subtotalError(rule *Rule, row Row, page int, msg string) error {
	if page == 0 {
		page = rule.Parts[0].Page
	}
	return &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: page,
		Field: fmt.Sprintf("row %q", row.PrintedLabel()), Err: ErrNotFound, Msg: msg}
}
