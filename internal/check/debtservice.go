package check

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

const (
	debtPrincipal = "debt-services/principal"
	debtInterest  = "debt-services/interest"
)

// debtServiceTiesToFacts holds the debt-service document to the store: every
// figure it publishes is the fact it cites, to the cent, page, offset and
// token, and sits in that fact's slot -- an issue cell its row, its column
// and its side, a requirements cell its period, its fund type and its side;
// every fact of its slice is published by exactly one figure; and each fund
// type has exactly one total, which cites exactly the figures its column lays
// out and is their sum.
//
// It is the check the debt page's figures rest on. A figure dropped, carried
// twice or published under the wrong period goes red here before
// `fisc export` lays the page out.
type debtServiceTiesToFacts struct{}

func (*debtServiceTiesToFacts) meta() meta {
	return meta{id: "debt-service-ties-to-facts", tier: 1,
		description: "every figure the debt-service document publishes is the fact it cites, every fact " +
			"of its slice is published once, each period is its facts' years and each total sums " +
			"exactly the facts it cites"}
}

func (*debtServiceTiesToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	byID := make(map[string]*fact.Fact, len(s.Facts))
	for i := range s.Facts {
		byID[s.Facts[i].ID] = &s.Facts[i]
	}
	var findings []Finding
	figures := 0
	for _, p := range s.Projections {
		if p.Debt == nil {
			continue
		}
		seen := map[string]int{}
		// slot is where a figure is laid out: what its fact must be.
		type slot struct {
			column            project.Column
			rowLabel          string
			fundGroup         string
			fund              *int
			category          string
			checkRow, checkFT bool
		}
		check := func(where string, fig *project.DebtFigure, want slot) {
			if fig == nil {
				return
			}
			figures++
			seen[fig.FactID]++
			f, ok := byID[fig.FactID]
			switch {
			case !ok:
				findings = append(findings, finding(where, "cites fact %s, which is not in %s", fig.FactID, factsFile))
				return
			case fig.AmountCents != f.AmountCents || fig.DocID != f.DocID || fig.Page != f.Page ||
				fig.Offset != f.Offset || fig.Token != f.Token:
				findings = append(findings, finding(where, "publishes %s at %s p%d+%d %q, and fact %s is %s at %s p%d+%d %q",
					amount.Cents(fig.AmountCents), fig.DocID, fig.Page, fig.Offset, fig.Token, f.ID,
					amount.Cents(f.AmountCents), f.DocID, f.Page, f.Offset, f.Token))
			case project.ColumnOf(f) != want.column:
				findings = append(findings, finding(where, "sits in %s, and fact %s is %s",
					want.column, f.ID, project.ColumnOf(f)))
			case f.Category != want.category:
				findings = append(findings, finding(where, "sits in the %s slot, and fact %s is %s",
					want.category, f.ID, f.Category))
			case want.checkRow && f.RowLabel != want.rowLabel:
				findings = append(findings, finding(where, "sits in row %q, and fact %s prints %q",
					want.rowLabel, f.ID, f.RowLabel))
			case want.checkFT && (f.FundGroup != want.fundGroup || !fact.SameFund(f.Fund, want.fund)):
				findings = append(findings, finding(where, "sits under fund group %q fund %s, and fact %s is %q fund %s",
					want.fundGroup, fact.FundString(want.fund), f.ID, f.FundGroup, fact.FundString(f.Fund)))
			}
		}
		d := p.Debt
		for _, is := range d.Issues {
			if len(is.Cells) != len(d.IssueColumns) {
				findings = append(findings, finding(fmt.Sprintf("%s issue %q", p.Name, is.RowLabel),
					"carries %d cells for %d columns", len(is.Cells), len(d.IssueColumns)))
				continue
			}
			for i, c := range is.Cells {
				where := fmt.Sprintf("%s issue %q column %d", p.Name, is.RowLabel, i)
				col := project.Column{FiscalYear: d.IssueColumns[i].FiscalYear,
					Basis: vocab.Basis(d.IssueColumns[i].Basis)}
				check(where+" principal", c.Principal, slot{column: col, rowLabel: is.RowLabel,
					category: debtPrincipal, checkRow: true})
				check(where+" interest", c.Interest, slot{column: col, rowLabel: is.RowLabel,
					category: debtInterest, checkRow: true})
			}
		}
		// laid is each fund type's figures as its column lays them out.
		laid := make([]map[string]bool, len(d.Requirements.FundTypes))
		for i := range laid {
			laid[i] = map[string]bool{}
		}
		for _, per := range d.Requirements.Periods {
			col := project.Column{FiscalYear: per.FiscalYear, Basis: vocab.BasisProjected}
			if per.FiscalYearThrough != nil {
				col.Through = *per.FiscalYearThrough
			}
			if len(per.Cells) != len(d.Requirements.FundTypes) {
				findings = append(findings, finding(fmt.Sprintf("%s period %q", p.Name, per.Label),
					"carries %d cells for %d fund types", len(per.Cells), len(d.Requirements.FundTypes)))
				continue
			}
			for i, c := range per.Cells {
				ft := d.Requirements.FundTypes[i]
				where := fmt.Sprintf("%s period %q %s", p.Name, per.Label, ft.Label)
				check(where+" principal", c.Principal, slot{column: col, fundGroup: ft.FundGroup,
					fund: ft.Fund, category: debtPrincipal, checkFT: true})
				check(where+" interest", c.Interest, slot{column: col, fundGroup: ft.FundGroup,
					fund: ft.Fund, category: debtInterest, checkFT: true})
				for _, fig := range []*project.DebtFigure{c.Principal, c.Interest} {
					if fig != nil {
						laid[i][fig.FactID] = true
					}
				}
			}
		}
		slice, err := p.slice(s.Facts)
		if err != nil {
			return Result{}, err
		}
		for _, f := range slice {
			if seen[f.ID] != 1 {
				findings = append(findings, finding(p.String(), "fact %s (%q %s) is published by %d figures, want 1",
					f.ID, f.RowLabel, project.ColumnOf(&f), seen[f.ID]))
			}
		}
		// Export lays one total under each fund type's column: a type with
		// none prints blank total cells, and of two it keeps the last.
		totals := map[string]int{}
		for _, t := range d.Requirements.Totals {
			totals[t.FundType]++
		}
		for _, ft := range d.Requirements.FundTypes {
			if n := totals[ft.Key]; n != 1 {
				findings = append(findings, finding(p.Name+" total "+ft.Key,
					"the document heads fund type %q and carries %d totals for it, want 1", ft.Key, n))
			}
		}
		for _, t := range d.Requirements.Totals {
			col := slices.IndexFunc(d.Requirements.FundTypes, func(ft project.DebtFundType) bool {
				return ft.Key == t.FundType
			})
			if col < 0 {
				findings = append(findings, finding(p.Name+" total "+t.FundType, "totals a fund type the document does not head"))
				continue
			}
			// The total cites exactly its column's figures: one it leaves out
			// is a row the printed Total covers and the page's total does not.
			cited := map[string]bool{}
			for _, id := range t.FactIDs {
				cited[id] = true
				if !laid[col][id] {
					findings = append(findings, finding(p.Name+" total "+t.FundType,
						"cites %s, which its column does not lay out", id))
				}
			}
			for _, id := range slices.Sorted(maps.Keys(laid[col])) {
				if !cited[id] {
					findings = append(findings, finding(p.Name+" total "+t.FundType,
						"leaves out %s, which its column lays out", id))
				}
			}
			var pc, ic int64
			for _, id := range t.FactIDs {
				f, ok := byID[id]
				if !ok || !slices.Contains(project.DebtServiceScopes(), f.Scope) {
					findings = append(findings, finding(p.Name+" total "+t.FundType, "cites %s, which is no fact of pp.80-81", id))
					continue
				}
				if f.Category == debtPrincipal {
					pc += f.AmountCents
				} else {
					ic += f.AmountCents
				}
			}
			if pc != t.PrincipalCents || ic != t.InterestCents {
				findings = append(findings, finding(p.Name+" total "+t.FundType,
					"publishes %s principal and %s interest, and the facts it cites sum to %s and %s",
					amount.Cents(t.PrincipalCents), amount.Cents(t.InterestCents), amount.Cents(pc), amount.Cents(ic)))
			}
		}
	}
	return conclusion{
		subjects: figures,
		unit:     "figures",
		held:     fmt.Sprintf("%d debt-service figures, each its cited fact, every fact of the slice published once", figures),
		nothing:  "no debt-service document was built",
		findings: findings,
	}.result(), nil
}
