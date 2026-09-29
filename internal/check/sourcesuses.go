package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// fundGroupSourcesEqualUses asserts each fund group column of the spine
// balances: revenue + transfers in - expenditure - transfers out - reserve
// increase == fund-balance/change, exactly.
//
// A column that prints any term and no change line is a finding, not a skip: the
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
	return fmt.Sprintf("%s %s %s", k.docID, k.fundGroup, fact.ColumnLabel(k.fiscalYear, k.basis))
}

func (*fundGroupSourcesEqualUses) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	columns := map[sourcesUsesKey]*structure.SourcesUses{}
	for _, f := range s.Facts {
		if f.Scope != project.PublishedScope {
			continue
		}
		k := sourcesUsesKey{f.DocID, f.FundGroup, f.FiscalYear, f.Basis}
		c := columns[k]
		if c == nil {
			c = &structure.SourcesUses{}
			columns[k] = c
		}
		if !c.Add(&f) {
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
		if c.Changes == 0 {
			if c.Terms > 0 {
				findings = append(findings, finding(k.String(),
					"prints a term of sources = uses and no %s, so its sources and uses balance against nothing",
					project.CategoryFundBalanceChange))
			}
			continue
		}
		checked++
		net := c.Net()
		if net != c.Change {
			findings = append(findings, finding(k.String(),
				"revenue %s + transfers in %s - expenditure %s - transfers out %s - reserve increase %s "+
					"= %s, and its %s is %s (off by %s)",
				amount.Cents(c.Revenue), amount.Cents(c.TransfersIn), amount.Cents(c.Expenditure),
				amount.Cents(c.TransfersOut), amount.Cents(c.Reserve), amount.Cents(net),
				project.CategoryFundBalanceChange, amount.Cents(c.Change), amount.Cents(net-c.Change)))
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
