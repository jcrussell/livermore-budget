package check

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// nodeBalancesTieToFacts asserts every balance a node publishes is the figure
// its fact_id names: of this document's slice, the line its slot says, of the
// fund the node is, at the value printed and on the page its locators name.
//
// A balance is a figure published outside every link, so no link check reads
// it: re-pointing a fund's ending balance to its beginning, or to another
// fund's, moved no link and left counts-reconcile green, which counts the
// citation and not what it cites.
type nodeBalancesTieToFacts struct{}

var _ Check = (*nodeBalancesTieToFacts)(nil)

func (*nodeBalancesTieToFacts) ID() string { return "node-balances-tie-to-facts" }
func (*nodeBalancesTieToFacts) Tier() int  { return 1 }
func (*nodeBalancesTieToFacts) Full() bool { return false }
func (*nodeBalancesTieToFacts) Description() string {
	return "every balance a node publishes equals the fact it cites, which is that fund's " +
		"beginning or ending balance in the document's own slice, read from the pages its " +
		"locators name; and on fund-sources-uses, every balance the slice prints is on its fund's node"
}

func (*nodeBalancesTieToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	balances := 0
	for _, p := range s.linkedDocuments() {
		selected := factIndex(project.SelectFacts(s.Facts, p.Options))
		if p.Name == project.FundSourcesUsesProjection {
			findings = append(findings, unpublishedBalances(p, selected)...)
		}
		for _, n := range p.Nodes {
			if n.Balances == nil {
				continue
			}
			for _, slot := range []struct {
				b        *project.NodeBalance
				category string
			}{
				{n.Balances.Beginning, project.CategoryFundBalanceBeginning},
				{n.Balances.Ending, project.CategoryFundBalanceEnding},
			} {
				if slot.b == nil {
					continue
				}
				balances++
				subject := fmt.Sprintf("%s %s %s", p, n.ID, slot.category)
				if msg := balanceMismatch(n.ID, slot.b, slot.category, selected); msg != "" {
					findings = append(findings, finding(subject, "%s", msg))
				}
			}
		}
	}
	return conclusion{
		subjects: balances,
		unit:     "node balances",
		held:     fmt.Sprintf("%d node balances, each the fund's own printed balance it cites", balances),
		nothing:  "no node publishes a balance",
		findings: findings,
	}.result(), nil
}

// unpublishedBalances is every beginning or ending balance a fund-sources-uses
// slice prints that its fund's node does not publish: a balance the change
// ribbon cites is counted cited whether or not the node shows it, so nothing
// else sees one dropped.
func unpublishedBalances(p linked, selected map[string]fact.Fact) []Finding {
	published := map[string]bool{}
	for _, n := range p.Nodes {
		if n.Balances == nil {
			continue
		}
		for _, b := range []*project.NodeBalance{n.Balances.Beginning, n.Balances.Ending} {
			if b != nil {
				published[b.FactID] = true
			}
		}
	}
	var out []Finding
	for _, id := range slices.Sorted(maps.Keys(selected)) {
		f := selected[id]
		if f.Kind != mapping.KindFundBalance || published[id] ||
			(f.Category != project.CategoryFundBalanceBeginning && f.Category != project.CategoryFundBalanceEnding) {
			continue
		}
		out = append(out, finding(fmt.Sprintf("%s %s%s %s", p, project.PrefixFund, fact.FundString(f.Fund), f.Category),
			"fact %s prints this balance and the fund's node does not publish it", f.ID))
	}
	return out
}

// balanceMismatch is why one published balance is not the figure it cites,
// or "".
func balanceMismatch(node string, b *project.NodeBalance, category string, selected map[string]fact.Fact) string {
	f, ok := selected[b.FactID]
	switch {
	case !ok:
		return fmt.Sprintf("cites fact %s, which is not in this projection's slice", b.FactID)
	case f.Kind != mapping.KindFundBalance || f.Category != category:
		return fmt.Sprintf("cites fact %s, which is %s under %q", f.ID, f.Kind, f.Category)
	case project.PrefixFund+fact.FundString(f.Fund) != node:
		return fmt.Sprintf("cites fact %s, which is fund %s's", f.ID, fact.FundString(f.Fund))
	case b.ValueCents != f.AmountCents:
		return fmt.Sprintf("is %s and fact %s prints %s", amount.Cents(b.ValueCents), f.ID,
			amount.Cents(f.AmountCents))
	}
	want := project.SourcesOf([]fact.Fact{f})
	same := slices.EqualFunc(want, b.Locators, func(a, c project.Source) bool {
		return a.DocID == c.DocID && slices.Equal(a.Pages, c.Pages)
	})
	if !same {
		return fmt.Sprintf("locators are %s and fact %s was read from %s",
			describeSources(b.Locators), f.ID, describeSources(want))
	}
	return ""
}
