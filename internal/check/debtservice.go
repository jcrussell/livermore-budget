package check

import (
	"context"
	"fmt"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// debtServiceTiesToFacts holds the debt-service document to the store: every
// figure it publishes is the fact it cites, to the cent, page, offset and
// token; every fact of its slice is published by exactly one figure; a
// requirements period's years are its facts' years, band included; and each
// fund type's total is the sum of exactly the facts it cites.
//
// It is the check the debt page's figures rest on. A figure dropped, carried
// twice or published under the wrong period goes red here before
// `fisc export` lays the page out.
type debtServiceTiesToFacts struct{}

var _ Check = (*debtServiceTiesToFacts)(nil)

func (*debtServiceTiesToFacts) ID() string { return "debt-service-ties-to-facts" }
func (*debtServiceTiesToFacts) Tier() int  { return 1 }
func (*debtServiceTiesToFacts) Full() bool { return false }
func (*debtServiceTiesToFacts) Description() string {
	return "every figure the debt-service document publishes is the fact it cites, every fact " +
		"of its slice is published once, each period is its facts' years and each total sums " +
		"exactly the facts it cites"
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
		check := func(where string, fig *project.DebtFigure, want *project.Column) {
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
			case want != nil && project.ColumnOf(f) != *want:
				findings = append(findings, finding(where, "sits in %s, and fact %s is %s",
					want, f.ID, project.ColumnOf(f)))
			}
		}
		d := p.Debt
		for _, is := range d.Issues {
			for i, c := range is.Cells {
				where := fmt.Sprintf("%s issue %q column %d", p.Name, is.RowLabel, i)
				check(where+" principal", c.Principal, nil)
				check(where+" interest", c.Interest, nil)
			}
		}
		for _, per := range d.Requirements.Periods {
			col := project.Column{FiscalYear: per.FiscalYear, Basis: "projected"}
			if per.FiscalYearThrough != nil {
				col.Through = *per.FiscalYearThrough
			}
			for i, c := range per.Cells {
				where := fmt.Sprintf("%s period %q fund type %d", p.Name, per.Label, i)
				check(where+" principal", c.Principal, &col)
				check(where+" interest", c.Interest, &col)
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
		for _, t := range d.Requirements.Totals {
			var pc, ic int64
			for _, id := range t.FactIDs {
				f, ok := byID[id]
				if !ok || !slices.Contains(project.DebtServiceScopes(), f.Scope) {
					findings = append(findings, finding(p.Name+" total "+t.FundType, "cites %s, which is no fact of pp.80-81", id))
					continue
				}
				if f.Category == "debt-services/principal" {
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
