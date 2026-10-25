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

// nodeBalancesTieToFacts asserts every balance a node publishes is the figure
// its fact_id names: of this document's slice, the line its slot says, of the
// fund the node is, at the value printed and on the page its locators name.
//
// A balance is a figure published outside every link, so no link check reads
// it: re-pointing a fund's ending balance to its beginning, or to another
// fund's, moved no link and left counts-reconcile green, which counts the
// citation and not what it cites.
//
// A node's change is held the same way, as ending less beginning of the two
// facts it cites, which must be the node's own balances: it is the one
// figure on a node that no page prints, and nothing else reads it.
type nodeBalancesTieToFacts struct{}

func (*nodeBalancesTieToFacts) meta() meta {
	return meta{id: "node-balances-tie-to-facts", tier: 1,
		description: "every balance a node publishes equals the fact it cites, which is that fund's " +
			"beginning or ending balance in the document's own slice, read from the pages its " +
			"locators name; every node printing both balances states their change, derived, as " +
			"ending less beginning of those two facts, and none printing one states a change; and " +
			"on fund-sources-uses, every balance the slice prints is on its fund's node, as on the " +
			"expansion every balance of a fund it draws is"}
}

func (*nodeBalancesTieToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	balances, changes := 0, 0
	for _, p := range s.linkedDocuments() {
		slice, err := p.slice(s.Facts)
		if err != nil {
			return Result{}, err
		}
		selected := fact.ByID(slice)
		switch p.Name {
		case project.FundSourcesUsesProjection:
			findings = append(findings, unpublishedBalances(p, selected, nil)...)
		case project.ExpansionProjection:
			drawn := map[string]bool{}
			for _, n := range p.Nodes {
				drawn[n.ID] = true
			}
			findings = append(findings, unpublishedBalances(p, selected, drawn)...)
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
			if n.Balances.Change != nil {
				changes++
			}
			if msg := changeMismatch(n.Balances, selected); msg != "" {
				findings = append(findings, finding(fmt.Sprintf("%s %s change", p, n.ID), "%s", msg))
			}
		}
	}
	return conclusion{
		subjects: balances,
		unit:     "node balances",
		held: fmt.Sprintf("%d node balances, each the fund's own printed balance it cites, and %d "+
			"changes, each ending less beginning of those two", balances, changes),
		nothing:  "no node publishes a balance",
		findings: findings,
	}.result(), nil
}

// unpublishedBalances is every beginning or ending balance a fund-sources-uses
// slice prints that its fund's node does not publish: a balance the change
// ribbon cites is counted cited whether or not the node shows it, so nothing
// else sees one dropped. Where drawn is not nil, only a fund it holds is
// asked: the expansion draws a fund only where money moves through it.
func unpublishedBalances(p linked, selected map[string]fact.Fact, drawn map[string]bool) []Finding {
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
		if f.Kind != vocab.KindFundBalance || published[id] ||
			(f.Category != project.CategoryFundBalanceBeginning && f.Category != project.CategoryFundBalanceEnding) {
			continue
		}
		fund := project.PrefixFund + fact.FundString(f.Fund)
		if drawn != nil && !drawn[fund] {
			continue
		}
		out = append(out, finding(fmt.Sprintf("%s %s %s", p, fund, f.Category),
			"fact %s prints this balance and the fund's node does not publish it", f.ID))
	}
	return out
}

// changeMismatch is why a node's change is not ending less beginning of its
// own two printed balances, or is stated beside a blank one, or "".
func changeMismatch(b *project.NodeBalances, selected map[string]fact.Fact) string {
	c := b.Change
	switch {
	case b.Beginning == nil || b.Ending == nil:
		if c != nil {
			return fmt.Sprintf("states a change of %s and a balance is blank, which is absent and not zero",
				amount.Cents(c.ValueCents))
		}
		return ""
	case c == nil:
		return "prints both balances and states no change"
	case !c.Derived || c.Rationale == "" || c.SourceNote == "":
		return "states a change not marked derived with a rationale and a source note, and no page prints it"
	}
	want := []string{b.Beginning.FactID, b.Ending.FactID}
	slices.Sort(want)
	if !slices.Equal(want, c.FactIDs) {
		return fmt.Sprintf("change cites %v and the node's balances are facts %v", c.FactIDs, want)
	}
	beginning, okB := selected[b.Beginning.FactID]
	ending, okE := selected[b.Ending.FactID]
	if !okB || !okE {
		return "change cites a fact not in this projection's slice"
	}
	if got := ending.AmountCents - beginning.AmountCents; c.ValueCents != got {
		return fmt.Sprintf("change is %s and facts %s less %s come to %s", amount.Cents(c.ValueCents),
			ending.ID, beginning.ID, amount.Cents(got))
	}
	wantLocators := project.SourcesOf([]fact.Fact{beginning, ending})
	same := slices.EqualFunc(wantLocators, c.Locators, func(a, d project.Source) bool {
		return a.DocID == d.DocID && slices.Equal(a.Pages, d.Pages)
	})
	if !same {
		return fmt.Sprintf("change locators are %s and its facts were read from %s",
			describeSources(c.Locators), describeSources(wantLocators))
	}
	return ""
}

// balanceMismatch is why one published balance is not the figure it cites,
// or "".
func balanceMismatch(node string, b *project.NodeBalance, category string, selected map[string]fact.Fact) string {
	f, ok := selected[b.FactID]
	switch {
	case !ok:
		return fmt.Sprintf("cites fact %s, which is not in this projection's slice", b.FactID)
	case f.Kind != vocab.KindFundBalance || f.Category != category:
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
