package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// fundGroupsAreTheirPrintedRows holds every fund-sources-uses document to the
// summary block Budget Book pp.186-209 print at the head of each year: one row
// per fund group, every line. The document draws funds parented to the type
// data/funds.yaml gives them, so a client folding it by group shows these sums.
//
// The facts it sums are the ones the document selected, so a selection that
// let the Capital Improvement Program Funds block in -- residue, whose CIP
// funds are typed enterprise and internal-service -- reads FY2026 Enterprise
// Funds revenues as 71,573,173 where p198 prints 67,514,261.
//
// It reaches FY2023-24 and FY2024-25, which the lattice does not: the spine
// prints neither column.
type fundGroupsAreTheirPrintedRows struct{}

var _ Check = (*fundGroupsAreTheirPrintedRows)(nil)

func (*fundGroupsAreTheirPrintedRows) ID() string { return "fund-groups-are-their-printed-rows" }
func (*fundGroupsAreTheirPrintedRows) Tier() int  { return 1 }
func (*fundGroupsAreTheirPrintedRows) Full() bool { return false }
func (*fundGroupsAreTheirPrintedRows) Description() string {
	return "every fund-sources-uses document's funds, summed by the fund type data/funds.yaml " +
		"gives them, equal the fund group rows pp.186-209 print in every line, to the cent"
}

// groupCell is one printed group row's figure in one line of one column.
type groupCell struct {
	year     int
	basis    string
	group    string
	category string
}

func (c groupCell) String() string {
	return fmt.Sprintf("FY%d %s %s %s", c.year, c.basis, c.group, c.category)
}

func (*fundGroupsAreTheirPrintedRows) Run(_ context.Context, s *Subject) (Result, error) {
	docs := s.documentsNamed(project.FundSourcesUsesProjection)
	if len(docs) == 0 {
		return conclusion{nothing: "no fund-sources-uses document is built"}.result(), nil
	}
	printed, rounded, findings := printedGroupRows(s)
	cells := 0
	for _, p := range docs {
		sums := map[groupCell]int64{}
		for _, f := range project.SelectFacts(s.Facts, p.Options) {
			if f.Fund == nil {
				continue
			}
			fund, ok := s.Vocabulary.Fund(*f.Fund)
			if !ok {
				findings = append(findings, finding(p.Name, "fact %s names fund %d, which data/funds.yaml "+
					"does not list", f.ID, *f.Fund))
				continue
			}
			sums[groupCell{f.FiscalYear, string(f.Basis), fund.Type, f.Category}] += f.AmountCents
		}
		col := p.Options.Columns[0]
		var keys []groupCell
		for k := range printed {
			if k.year == col.FiscalYear && k.basis == string(col.Basis) {
				keys = append(keys, k)
			}
		}
		seen := map[string]bool{}
		for _, k := range keys {
			seen[k.group] = true
		}
		for _, g := range slices.Sorted(maps.Values(structure.FundBalanceGroupRows())) {
			if g != "" && !seen[g] {
				findings = append(findings, finding(p.Name, "no %s row is read for %s, so its funds "+
					"are held to nothing", g, col))
				seen[g] = true
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, k := range keys {
			cells++
			// The page rounds a block total off its funds where the rule
			// declares it; the group row repeats that total.
			if got, want := sums[k], printed[k]-rounded[k]; got != want {
				findings = append(findings, finding(k.String(), "the document's %s funds sum to %s "+
					"and pp.186-209 print %s for the group, %s off its funds by declaration, "+
					"a difference of %s", k.group, amount.Cents(got), amount.Cents(printed[k]),
					amount.Cents(rounded[k]), amount.Cents(got-want)))
			}
		}
	}
	return conclusion{
		subjects: cells,
		unit:     "group cells",
		held: fmt.Sprintf("%d fund group cells over %d document(s), each the sum of the group's funds",
			cells, len(docs)),
		findings: findings,
	}.result(), nil
}

// printedGroupRows is each year's summary block, read from the rules of its
// subtotal chain: the group row's figure per line, and the subtotal_deltas
// declared on the group's block total, printed minus summed, per line. Every
// other row of the chain is a fund, and is not read here.
func printedGroupRows(s *Subject) (printed, rounded map[groupCell]int64, findings []Finding) {
	groups, totals := structure.FundBalanceGroupRows(), structure.FundBalanceGroupTotals()
	printed, rounded = map[groupCell]int64{}, map[groupCell]int64{}
	for _, f := range s.Files {
		r := s.Resolvers[f.Path]
		for i := range f.Rules {
			rule := &f.Rules[i]
			if rule.Scope != structure.ScopeFundBalancesByFund ||
				!strings.HasPrefix(rule.SubtotalChain, "fund-balances-fy") {
				continue
			}
			if r == nil {
				findings = append(findings, finding(rule.ID, "no resolver is loaded for %s", f.Path))
				continue
			}
			for j := range rule.Parts {
				cells, err := r.Cells(rule, &rule.Parts[j])
				if err != nil {
					findings = append(findings, finding(rule.ID, "p%d: %v", rule.Parts[j].Page, err))
					continue
				}
				part := &rule.Parts[j]
				for _, c := range cells {
					if group, ok := totals[c.Row.Label]; ok && c.Category() != "" {
						if d, ok := c.Row.SubtotalDeltaAt(part, c.ColumnIndex); ok {
							rounded[groupCell{c.Column.FiscalYear, string(rule.Basis), group, c.Category()}] += int64(d)
						}
						continue
					}
					group, ok := groups[c.Row.Label]
					if !ok || c.Category() == "" {
						continue
					}
					if group == "" {
						// The CIP funds' row: residue, held to p222 instead.
						continue
					}
					k := groupCell{c.Column.FiscalYear, string(rule.Basis), group, c.Category()}
					if _, dup := printed[k]; dup {
						findings = append(findings, finding(rule.ID, "%s is printed twice", k))
						continue
					}
					printed[k] = int64(c.Cents)
				}
			}
		}
	}
	return printed, rounded, findings
}
