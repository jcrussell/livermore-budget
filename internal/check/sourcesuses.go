package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

const categoryReserveIncrease = "fund-balance/reserve-increase"

// fundGroupSourcesEqualUses asserts each fund group column of the spine
// balances: revenue + transfers in - expenditure - transfers out - reserve
// increase == fund-balance/change, exactly.
//
// A column that prints flows and no change line is a finding, not a skip: the
// change is what the draw and contribution links are drawn from.
type fundGroupSourcesEqualUses struct{}

var _ Check = (*fundGroupSourcesEqualUses)(nil)

func (*fundGroupSourcesEqualUses) ID() string { return "fund-group-sources-equal-uses" }
func (*fundGroupSourcesEqualUses) Tier() int  { return 1 }
func (*fundGroupSourcesEqualUses) Full() bool { return false }
func (*fundGroupSourcesEqualUses) Description() string {
	return "every fund group column of the spine satisfies revenue + transfers in - expenditure - " +
		"transfers out - reserve increase == fund-balance/change, exactly"
}

type sourcesUsesKey struct {
	docID      string
	fundGroup  string
	fiscalYear int
	basis      mapping.Basis
}

func (k sourcesUsesKey) String() string {
	return fmt.Sprintf("%s %s FY%d %s", k.docID, k.fundGroup, k.fiscalYear, k.basis)
}

type sourcesUses struct {
	revenue, transfersIn, expenditure, transfersOut, reserve, change int64
	flows, changes                                                   int
}

func (*fundGroupSourcesEqualUses) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	columns := map[sourcesUsesKey]*sourcesUses{}
	for _, f := range s.Facts {
		if f.Scope != project.PublishedScope {
			continue
		}
		k := sourcesUsesKey{f.DocID, f.FundGroup, f.FiscalYear, f.Basis}
		c := columns[k]
		if c == nil {
			c = &sourcesUses{}
			columns[k] = c
		}
		switch {
		case f.Kind == mapping.KindRevenue:
			c.revenue += f.AmountCents
			c.flows++
		case f.Kind == mapping.KindExpenditure:
			c.expenditure += f.AmountCents
			c.flows++
		case f.Kind == mapping.KindTransferIn:
			c.transfersIn += f.AmountCents
		case f.Kind == mapping.KindTransferOut:
			c.transfersOut += f.AmountCents
		case f.Category == categoryReserveIncrease:
			c.reserve += f.AmountCents
		case f.Category == categoryFundBalanceChange:
			c.change += f.AmountCents
			c.changes++
		case f.Category == categoryFundBalanceBeginning, f.Category == categoryFundBalanceEnding:
		default:
			findings = append(findings, finding(f.ID,
				"%s carries kind %q and category %q, which is no term of sources = uses", k, f.Kind, f.Category))
		}
	}
	keys := make([]sourcesUsesKey, 0, len(columns))
	for k := range columns {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	checked := 0
	for _, k := range keys {
		c := columns[k]
		if c.changes == 0 {
			if c.flows > 0 {
				findings = append(findings, finding(k.String(),
					"prints revenue or expenditure and no %s, so its sources and uses balance against nothing",
					categoryFundBalanceChange))
			}
			continue
		}
		checked++
		net := c.revenue + c.transfersIn - c.expenditure - c.transfersOut - c.reserve
		if net != c.change {
			findings = append(findings, finding(k.String(),
				"revenue %s + transfers in %s - expenditure %s - transfers out %s - reserve increase %s "+
					"= %s, and its %s is %s (off by %s)",
				amount.Cents(c.revenue), amount.Cents(c.transfersIn), amount.Cents(c.expenditure),
				amount.Cents(c.transfersOut), amount.Cents(c.reserve), amount.Cents(net),
				categoryFundBalanceChange, amount.Cents(c.change), amount.Cents(net-c.change)))
		}
	}
	return conclusion{
		subjects: checked,
		unit:     "fund group columns",
		held:     fmt.Sprintf("%d fund group columns, each with sources minus uses equal to its change", checked),
		nothing:  "no spine column prints a fund-balance/change",
		findings: findings,
	}.result(), nil
}
