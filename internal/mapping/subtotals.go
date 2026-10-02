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
// figures to the rows above it, column by column (see Row.Subtotal).
//
// A row's figures are gathered from every part that prints it, so a labelled
// page and its label-less continuation are one row. A column is a part's
// position and that column's place in it, which is why every rule in a chain
// must lay its parts and headers out alike: Budget Book p81 prints Principal,
// Interest and Total twice, once a year, so a header alone names no column. A
// row that is not a subtotal adds to every level's running sum; a subtotal of
// level L compares every column either it or the rows above it print against
// level L's sum, and then clears every level up to L. A subtotal over no row
// is refused, and so is a row after the chain's last subtotal: a printed
// table's rows are all inside some total, and one that is not has been read
// from the wrong place.
func (r *Resolver) CheckSubtotals(chain []*Rule) (*SubtotalsResult, error) {
	type column struct{ part, col int }
	first := chain[0]
	name := func(k column) string {
		return fmt.Sprintf("%q on p%d", first.Parts[k.part].ColumnHeaders[k.col].Text,
			first.Parts[k.part].Page)
	}
	levels := 0
	for _, rule := range chain {
		if err := sameColumns(first, rule); err != nil {
			return nil, err
		}
		for _, row := range rule.Rows {
			levels = max(levels, row.Subtotal)
		}
	}
	if levels == 0 {
		return nil, fmt.Errorf("subtotal chain of rule %q declares no subtotal row", first.ID)
	}
	sums := make([]map[column]amount.Cents, levels+1)
	rows := make([]int, levels+1)
	reset := func(upTo int) {
		for l := 1; l <= upTo; l++ {
			sums[l] = map[column]amount.Cents{}
			rows[l] = 0
		}
	}
	reset(levels)

	res := &SubtotalsResult{}
	for _, rule := range chain {
		figures := make([]map[column]Value, len(rule.Rows))
		for i := range rule.Parts {
			cells, err := r.Cells(rule, &rule.Parts[i])
			if err != nil {
				return nil, err
			}
			for _, c := range cells {
				if figures[c.RowIndex] == nil {
					figures[c.RowIndex] = map[column]Value{}
				}
				figures[c.RowIndex][column{i, c.ColumnIndex}] = c
			}
		}
		for i, row := range rule.Rows {
			if row.Subtotal == 0 {
				for k, c := range figures[i] {
					for l := 1; l <= levels; l++ {
						sums[l][k] += c.Cents
					}
				}
				for l := 1; l <= levels; l++ {
					rows[l]++
				}
				continue
			}
			above := rows[row.Subtotal]
			if above == 0 {
				return nil, r.subtotalError(rule, row, 0, fmt.Sprintf(
					"is a level-%d subtotal over no row: nothing stands between it and the "+
						"last subtotal of its level or higher", row.Subtotal))
			}
			want := sums[row.Subtotal]
			deltas := map[column]amount.Cents{}
			for _, d := range row.SubtotalDeltas {
				for pi, p := range rule.Parts {
					for ci, h := range p.ColumnHeaders {
						if h.Text == d.Column {
							deltas[column{pi, ci}] = d.Cents
						}
					}
				}
			}
			keys := make([]column, 0, len(want))
			for k := range want {
				keys = append(keys, k)
			}
			for k := range figures[i] {
				if _, ok := want[k]; !ok {
					keys = append(keys, k)
				}
			}
			slices.SortFunc(keys, func(a, b column) int {
				if a.part != b.part {
					return a.part - b.part
				}
				return a.col - b.col
			})
			for _, k := range keys {
				got, ok := figures[i][k]
				if !ok {
					return nil, r.subtotalError(rule, row, 0, fmt.Sprintf(
						"prints no figure under %s, which the %d row(s) above it do", name(k), above))
				}
				delta, declared := deltas[k]
				delete(deltas, k)
				switch {
				case declared && got.Cents == want[k]:
					return nil, r.subtotalError(rule, row, got.Page, fmt.Sprintf(
						"declares a delta of %s under %s, and the column ties exactly at %s",
						delta.String(), name(k), got.Token))
				case got.Cents != want[k]+delta:
					return nil, r.subtotalError(rule, row, got.Page, fmt.Sprintf(
						"prints %s under %s, and the %d row(s) above it since the last level-%d "+
							"subtotal sum to %s (a declared delta of %s)",
						got.Token, name(k), above, row.Subtotal, want[k].String(), delta.String()))
				}
				res.Cells++
			}
			for k := range deltas {
				return nil, r.subtotalError(rule, row, 0, fmt.Sprintf(
					"declares a delta under %s, a column neither it nor a row above it prints", name(k)))
			}
			res.Lines++
			reset(row.Subtotal)
		}
	}
	if rows[1] > 0 {
		last := chain[len(chain)-1]
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: last.ID,
			Page: last.Parts[len(last.Parts)-1].Page, Field: "rows", Err: ErrNotFound,
			Msg: fmt.Sprintf("%d row(s) after the chain's last subtotal are inside no printed total",
				rows[1])},
			"end the chain with the subtotal the page prints under them")
	}
	return res, nil
}

// sameColumns refuses a chain member whose parts or headers are laid out
// unlike the chain's first rule, since CheckSubtotals matches columns by
// position.
func sameColumns(first, rule *Rule) error {
	if len(rule.Parts) != len(first.Parts) {
		return fmt.Errorf("rule %q has %d parts and rule %q, first in its subtotal chain, has %d; "+
			"a chain compares columns by position", rule.ID, len(rule.Parts), first.ID, len(first.Parts))
	}
	for i := range rule.Parts {
		a, b := first.Parts[i].ColumnHeaders, rule.Parts[i].ColumnHeaders
		if !slices.Equal(a, b) {
			return fmt.Errorf("rule %q part %d declares column_headers %v and rule %q, first in "+
				"its subtotal chain, declares %v; a chain compares columns by position",
				rule.ID, i+1, b, first.ID, a)
		}
	}
	return nil
}

func (r *Resolver) subtotalError(rule *Rule, row Row, page int, msg string) error {
	if page == 0 {
		page = rule.Parts[0].Page
	}
	return &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: page,
		Field: fmt.Sprintf("row %q", row.PrintedLabel()), Err: ErrNotFound, Msg: msg}
}
