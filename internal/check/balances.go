package check

import (
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// lineOf is the line of its scope's declaration a fact is printed on.
func lineOf(b structure.Balance, f *fact.Fact) (structure.Line, bool) {
	for _, l := range b.Lines {
		if l.Matches(f) {
			return l, true
		}
	}
	return structure.Line{}, false
}

// blankLines is, per balance, the lines its rules declare the page leaves
// blank: absent, not zero, and not a line a rule stopped publishing.
type blankLines map[structure.BalanceAt]map[structure.Line]bool

// declaredBlanks reads every omission a rule in a balance scope declares, on
// a cell mapping.Rule.OmissionPublishes says would become a fact, and finds
// every fact the store prints on a line its rule declares blank: both balance
// checks read their blanks here, so neither can let a printed figure override
// one.
//
// THE ADDRESS IS fact.FromValues', fed the cell the page leaves blank, so the
// blank lands on exactly the balance and line its figure would have. The
// facts it returns carry no token and are never published.
func declaredBlanks(s *Subject, balances []structure.Balance) (blankLines, []Finding) {
	out := blankLines{}
	var findings []Finding
	for _, file := range s.Files {
		for i := range file.Rules {
			rule := &file.Rules[i]
			b, ok := structure.BalanceOf(balances, rule.Scope)
			if !ok {
				continue
			}
			for j := range rule.Parts {
				p := &rule.Parts[j]
				for _, o := range mapping.Omissions(rule, p) {
					for c, col := range p.Columns {
						if !rule.OmissionPublishes(p, c, o) {
							continue
						}
						blanks, err := fact.FromValues(file, rule, []mapping.Value{{
							Row: o.Row, Column: col, RowIndex: o.RowIndex, ColumnIndex: c, Page: o.Page,
						}})
						if err != nil {
							findings = append(findings, finding(file.Path, "%v", err))
							continue
						}
						for k := range blanks {
							l, ok := lineOf(b, &blanks[k])
							if !ok {
								continue
							}
							at := structure.BalanceAtOf(&blanks[k])
							if out[at] == nil {
								out[at] = map[structure.Line]bool{}
							}
							out[at][l] = true
						}
					}
				}
			}
		}
	}
	for i := range s.Facts {
		f := &s.Facts[i]
		b, ok := structure.BalanceOf(balances, f.Scope)
		if !ok {
			continue
		}
		if l, ok := lineOf(b, f); ok && out[structure.BalanceAtOf(f)][l] {
			findings = append(findings, finding(f.ID,
				"%s prints %s, and its rule declares that cell blank", structure.BalanceAtOf(f), l))
		}
	}
	return out, findings
}
