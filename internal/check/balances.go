package check

import (
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// blankLines is, per balance, the lines its rules declare the page leaves
// blank: absent, not zero, and not a line a rule stopped publishing.
type blankLines map[structure.BalanceAt]map[structure.Line]bool

// balanceBlanks is what declaredBlanks reads: the blank lines, the balances
// the store prints a figure on one of them, and a finding per such figure.
type balanceBlanks struct {
	lines blankLines
	// contradicted is every balance a fact prints on a line its rule declares
	// blank. Neither identity has one value for that line, so neither check
	// holds the balance.
	contradicted map[structure.BalanceAt]bool
	findings     []Finding
}

// declaredBlanks reads every blank cell a rule in a balance scope declares,
// from mapping.Omissions and with no matching of its own, on a cell
// mapping.Row.Publishes says would become a fact were the page to print it,
// and finds every fact the store prints on a line its rule declares blank.
// Both balance checks read their blanks here; fund-balance-identity, which
// covers every declared scope, reports the findings, and
// fund-group-sources-equal-uses discards them so one defect is one finding,
// and sets the contradicted balance aside rather than hold it.
// A row another page prints is no blank: its cells are missing when absent,
// and mapping.Omissions never names one.
//
// THE ADDRESS IS fact.FromValues', fed the cell the page leaves blank, so the
// blank lands on exactly the balance and line its figure would have. The
// facts it returns carry no token and are never published.
func declaredBlanks(s *Subject, balances []structure.Balance) balanceBlanks {
	out := blankLines{}
	contradicted := map[structure.BalanceAt]bool{}
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
					col := p.Columns[o.ColumnIndex]
					if !o.Row.Publishes(col) {
						continue
					}
					blanks, err := fact.FromValues(file, rule, []mapping.Value{{
						Row: o.Row, Column: col, RowIndex: o.RowIndex, ColumnIndex: o.ColumnIndex, Page: o.Page,
					}})
					if err != nil {
						findings = append(findings, finding(file.Path, "%v", err))
						continue
					}
					for k := range blanks {
						l, ok := b.LineOf(&blanks[k])
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
	for i := range s.Facts {
		f := &s.Facts[i]
		b, ok := structure.BalanceOf(balances, f.Scope)
		if !ok {
			continue
		}
		at := structure.BalanceAtOf(f)
		if l, ok := b.LineOf(f); ok && out[at][l] {
			contradicted[at] = true
			findings = append(findings, finding(f.ID,
				"%s prints %s, and its rule declares that cell blank", at, l))
		}
	}
	return balanceBlanks{lines: out, contradicted: contradicted, findings: findings}
}
