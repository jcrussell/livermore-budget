package mapping

import (
	"fmt"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// SubtotalsResult is what [Resolver.CheckSubtotals] compared.
type SubtotalsResult struct {
	// Tied is each tied subtotal row's figures, so a caller counting across
	// chains can count a printed figure once: debt-service-principal and
	// debt-service-interest read one block, and each chain compares all of it.
	Tied [][]Value

	// Declared is the figures among Tied that tie only through a declared
	// subtotal_deltas entry, which a report must not count as the document
	// agreeing with itself.
	Declared []Value
}

// Chains groups a file's rules into the runs CheckSubtotals reads, in file
// order: each named Rule.SubtotalChain is one run, and a rule with subtotal
// rows and no chain is a run of its own. Rules in neither are left out.
func Chains(f *File) [][]*Rule {
	// Keyed apart, so a declared chain cannot be spelled into a rule's own.
	type key struct {
		declared bool
		name     string
	}
	var out [][]*Rule
	at := map[key]int{}
	for i := range f.Rules {
		rule := &f.Rules[i]
		name := key{true, rule.SubtotalChain}
		if rule.SubtotalChain == "" {
			if !slices.ContainsFunc(rule.Rows, func(r Row) bool { return r.Subtotal > 0 }) {
				continue
			}
			name = key{false, rule.ID}
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
	byPosition := func(a, b column) int {
		if a.part != b.part {
			return a.part - b.part
		}
		return a.col - b.col
	}
	first := chain[0]
	levels := 0
	for _, rule := range chain {
		if msg := sameColumns(first, rule); msg != "" {
			return nil, &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: rule.Parts[0].Page,
				Field: "subtotal_chain", Err: ErrNotFound, Msg: msg}
		}
		for _, row := range rule.Rows {
			levels = max(levels, row.Subtotal)
		}
	}
	if levels == 0 {
		return nil, cmdutil.WithHint(&resolveError{DocID: r.file.DocID, RuleID: first.ID,
			Page: first.Parts[0].Page, Field: "subtotal_chain", Err: ErrNotFound,
			Msg: fmt.Sprintf("names chain %q, and no rule in it declares a subtotal row",
				first.SubtotalChain)},
			"a chain exists to carry rows to the subtotals they sit under")
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
		name := func(k column) string {
			return fmt.Sprintf("%q on p%d", rule.Parts[k.part].ColumnHeaders[k.col].Text,
				rule.Parts[k.part].Page)
		}
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
				// A row no part prints -- omitted on every page -- adds
				// nothing, and counting it would let a subtotal over it pass
				// with nothing summed.
				if len(figures[i]) > 0 {
					for l := 1; l <= levels; l++ {
						rows[l]++
					}
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
			slices.SortFunc(keys, byPosition)
			var tied []Value
			for _, k := range keys {
				got, ok := figures[i][k]
				if !ok {
					return nil, r.subtotalError(rule, row, rule.Parts[k.part].Page, fmt.Sprintf(
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
				tied = append(tied, got)
				if declared {
					res.Declared = append(res.Declared, got)
				}
			}
			// A delta naming no compared column -- a header only a column
			// neither the subtotal nor its rows print -- is a stale claim.
			// Reported in column order, so two of them name the same one first
			// on every run.
			unused := make([]column, 0, len(deltas))
			for k := range deltas {
				unused = append(unused, k)
			}
			if len(unused) > 0 {
				slices.SortFunc(unused, byPosition)
				return nil, r.subtotalError(rule, row, rule.Parts[unused[0].part].Page, fmt.Sprintf(
					"declares a delta under %s, a column neither it nor a row above it prints",
					name(unused[0])))
			}
			res.Tied = append(res.Tied, tied)
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

// sameColumns says how a chain member's parts or headers are laid out unlike
// the chain's first rule, or "" when they are not, since CheckSubtotals
// matches columns by position.
func sameColumns(first, rule *Rule) string {
	if len(rule.Parts) != len(first.Parts) {
		return fmt.Sprintf("has %d parts and rule %q, first in its subtotal chain, has %d; "+
			"a chain compares columns by position", len(rule.Parts), first.ID, len(first.Parts))
	}
	for i := range rule.Parts {
		a, b := first.Parts[i].ColumnHeaders, rule.Parts[i].ColumnHeaders
		if !slices.Equal(a, b) {
			return fmt.Sprintf("part %d declares column_headers %v and rule %q, first in its "+
				"subtotal chain, declares %v; a chain compares columns by position", i+1, b, first.ID, a)
		}
	}
	return ""
}

func (r *Resolver) subtotalError(rule *Rule, row Row, page int, msg string) error {
	if page == 0 {
		page = rule.Parts[0].Page
	}
	return &resolveError{DocID: r.file.DocID, RuleID: rule.ID, Page: page,
		Field: fmt.Sprintf("row %q", row.PrintedLabel()), Err: ErrNotFound, Msg: msg}
}
