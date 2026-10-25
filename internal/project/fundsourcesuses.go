package project

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// FundSourcesUsesProjection is this document's name and file stem. The stem
// fund-balances is the ACFR history's.
const FundSourcesUsesProjection = "fund-sources-uses"

// FundSourcesUsesScope is Budget Book pp.186-209, each fund's sources and uses.
const FundSourcesUsesScope = structure.ScopeFundBalancesByFund

// FundSourcesUsesScopes is the schedule set, as [Options.Scopes] holds it.
func FundSourcesUsesScopes() []string { return []string{FundSourcesUsesScope} }

// fundSourcesUses draws Budget Book pp.186-209: where each fund's money comes
// from and where it goes, one fund per node.
//
// # The shape
//
//	tier 0  revenue/revenues, transfers/in, fund-balance/draw
//	          |  one link per printed cell, into the fund
//	tier 2  fund-group/<type>     parent of its funds, touching no link
//	tier 3  fund/<n>              parent fund-group/<data/funds.yaml type>
//	          |  one link per printed cell, out of the fund
//	tier 5  expenditure/expenses, transfers/out, transfers/out-to-cip,
//	        fund-balance/reserve-increase, fund-balance/contribution
//
// The page identity is beginning + revenues + transfers in = expenses +
// transfers out + out to CIP + reserve increase + ending, so a fund's change,
// ending less beginning, is the one figure that makes its sources equal its
// uses. It is drawn as ONE derived link citing both balances, from the draw
// when negative and to the contribution when positive; [ChangeCents] is its
// value. The balances themselves ride on the fund node as [NodeBalances].
//
// The Capital Improvement Program Funds block of pp.190, 196, 202 and 208 is
// p222's money and no cut admits it, so this document selects through the
// cuts ([Options.ThroughCuts]) and refuses options that do not.
type fundSourcesUses struct {
	Labels labels
}

var _ Projection = (*fundSourcesUses)(nil)

// Name is [Projection]'s, and it is this document's file stem.
func (*fundSourcesUses) Name() string { return FundSourcesUsesProjection }

// Slices is one Options per column pp.186-209 print, each selecting through
// the cuts.
func (*fundSourcesUses) Slices(facts []fact.Fact, version string) []Options {
	out := columnsCarrying(facts, FundSourcesUsesScopes(), FundSourcesUsesScopes(), nil, version)
	for i := range out {
		out[i].ThroughCuts = true
	}
	return out
}

// Build is [Projection]'s entry point.
func (u *fundSourcesUses) Build(facts []fact.Fact, o Options) ([]byte, error) {
	return encodeGraph(u, facts, o)
}

// fundCell is one line pp.186-209 print per fund that this document draws as
// a flow: which end of the fund it is on, the endpoint, and the link's kind.
type fundCell struct {
	kind     vocab.Kind
	category string
	// into is whether the money flows into the fund.
	into bool
	end  endpoint
	// boundary takes the link kind from the fund group, as the spine does for
	// revenue and expenditure.
	boundary bool
	linkKind LinkKind
}

// fundCells are the flows, in the order the page prints them.
var fundCells = []fundCell{
	{vocab.KindRevenue, "revenues", true,
		endpoint{id: PrefixRevenue + "revenues", slug: "revenues", role: RoleRevenueSource}, true, ""},
	{vocab.KindTransferIn, NodeTransfersIn, true,
		endpoint{id: NodeTransfersIn, slug: NodeTransfersIn, role: RoleTransferIn}, false, KindInternalTransfer},
	{vocab.KindExpenditure, "expenses", false,
		endpoint{id: PrefixExpenditure + "expenses", slug: "expenses", role: RoleObjectCategory}, true, ""},
	{vocab.KindTransferOut, NodeTransfersOut, false,
		endpoint{id: NodeTransfersOut, slug: NodeTransfersOut, role: RoleTransferOut}, false, KindInternalTransfer},
	{vocab.KindTransferOut, NodeTransfersOutToCIP, false,
		endpoint{id: NodeTransfersOutToCIP, slug: NodeTransfersOutToCIP, role: RoleTransferOut}, false, KindInternalTransfer},
	{vocab.KindFundBalance, CategoryFundBalanceReserveIncrease, false,
		endpoint{id: CategoryFundBalanceReserveIncrease, slug: CategoryFundBalanceReserveIncrease,
			role: RoleReserveIncrease}, false, KindFundBalance},
}

// fundLines is one fund's printed figures in one column, by (kind, category).
type fundLines map[cellLine]*fact.Fact

// cellLine is one printed line of a fund's block.
type cellLine struct {
	kind     vocab.Kind
	category string
}

var (
	lineBeginning = cellLine{vocab.KindFundBalance, CategoryFundBalanceBeginning}
	lineEnding    = cellLine{vocab.KindFundBalance, CategoryFundBalanceEnding}
)

// Document builds the per-fund sources and uses and returns it, so `fisc
// verify` reads the same structure `fisc export` writes.
func (u *fundSourcesUses) Document(facts []fact.Fact, o Options) (*Document, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("%s options: %w", u.Name(), err)
	}
	if !sameScopes(o.Scopes, FundSourcesUsesScopes()) || len(o.Kinds) != 0 || !o.ThroughCuts {
		return nil, hint.With(
			fmt.Errorf("%s: scopes are %q, kinds %v and through-cuts %v, want %q, every kind and true",
				u.Name(), o.ScopeList(), o.Kinds, o.ThroughCuts, FundSourcesUsesScope),
			"pp.190, 196, 202 and 208 print the Capital Improvement Program funds, p222's money, "+
				"some typed enterprise and internal service in data/funds.yaml; only the cuts leave "+
				"that block out")
	}
	if len(o.Columns) != 1 {
		return nil, hint.With(
			fmt.Errorf("%s: a graph is of one column, got %d", u.Name(), len(o.Columns)),
			"two budget years in one flow diagram add every figure to its own successor")
	}
	if u.Labels == nil {
		return nil, hint.With(
			fmt.Errorf("%s: no registry is attached", u.Name()),
			"a fund node's parent comes from data/funds.yaml's type:, so this document "+
				"cannot be built without one")
	}
	selected, err := SelectFacts(facts, o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", u.Name(), err)
	}

	funds, err := u.fundBlocks(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	var links []Link
	for _, number := range sortedFundNumbers(funds) {
		lines := funds[number]
		fund, group, nodeErr := u.fundNode(number, lines)
		if nodeErr != nil {
			return nil, nodeErr
		}
		for _, c := range fundCells {
			f, ok := lines[cellLine{c.kind, c.category}]
			if !ok || f.AmountCents == 0 {
				// A blank draws nothing, and a printed dash is a fact and not a flow.
				continue
			}
			if f.AmountCents < 0 {
				return nil, hint.With(
					fmt.Errorf("%s: fact %s (%s p%d, fund %d) prints %s as %s",
						u.Name(), f.ID, f.DocID, f.Page, number, c.category, amount.Cents(f.AmountCents)),
					"a negative flow has no direction this document declares, so it is refused "+
						"rather than drawn the wrong way round")
			}
			kind := c.linkKind
			if c.boundary {
				kind = boundaryKind(group)
			}
			src, dst := c.end.id, fund.ID
			if !c.into {
				src, dst = fund.ID, c.end.id
			}
			u.addEndpoint(nodes, c.end)
			links = append(links, Link{
				Source: src, Target: dst, ValueCents: f.AmountCents, Kind: kind,
				FactIDs: []string{f.ID}, Locators: SourcesOf([]fact.Fact{*f}),
			})
		}
		if l, ok := changeLink(fund.ID, lines); ok {
			end := endpoint{id: l.Target, role: RoleFundBalanceContribution}
			if l.Source == NodeFundBalanceDraw {
				end = endpoint{id: l.Source, role: RoleFundBalanceDraw}
			}
			u.addEndpoint(nodes, end)
			links = append(links, l)
		}
		nodes[fund.ID] = fund
		gid := PrefixFundGroup + group
		if _, ok := nodes[gid]; !ok {
			nodes[gid] = Node{ID: gid, Label: u.label(endpoint{id: gid}),
				Tier: endpoint{id: gid}.tier(), Role: RoleFundGroup}
		}
	}

	out, err := drawGraph(nodes, links)
	if err != nil {
		return nil, err
	}
	return graphDocument(u.Name(), o, FundSourcesUsesScopes(), selected, out, links, nil,
		func(Counts) []Caveat { return fundSourcesUsesCaveats(links, out) })
}

// fundBlocks groups the selected facts by fund, refusing a fact naming no
// fund, a line printed twice for one fund, and a line the page does not print.
func (u *fundSourcesUses) fundBlocks(selected []fact.Fact) (map[int]fundLines, error) {
	known := map[cellLine]bool{lineBeginning: true, lineEnding: true}
	for _, c := range fundCells {
		known[cellLine{c.kind, c.category}] = true
	}
	out := map[int]fundLines{}
	for i := range selected {
		f := &selected[i]
		if f.Fund == nil {
			return nil, fmt.Errorf("%s: fact %s (%s p%d %q) names no fund",
				u.Name(), f.ID, f.DocID, f.Page, f.RowLabel)
		}
		line := cellLine{f.Kind, f.Category}
		if !known[line] {
			return nil, fmt.Errorf("%s: fact %s is %s under %q, a line pp.186-209 do not print",
				u.Name(), f.ID, f.Kind, f.Category)
		}
		lines := out[*f.Fund]
		if lines == nil {
			lines = fundLines{}
			out[*f.Fund] = lines
		}
		if was, dup := lines[line]; dup {
			return nil, hint.With(
				fmt.Errorf("%s: fund %d prints %s twice, facts %s and %s",
					u.Name(), *f.Fund, f.Category, was.ID, f.ID),
				"one fund's block prints each line once; two facts for one line is a rule "+
					"reading another block, which the cuts exist to leave out")
		}
		lines[line] = f
	}
	return out, nil
}

// fundNode is a fund's node, parented to its data/funds.yaml type and
// carrying its printed balances, and that type. A fact read under another
// group's header is refused: the parent and the link kind would disagree.
func (u *fundSourcesUses) fundNode(number int, lines fundLines) (Node, string, error) {
	group, ok := u.Labels.FundType(number)
	if !ok {
		return Node{}, "", hint.With(
			fmt.Errorf("%s: fund %d is in no data/funds.yaml entry", u.Name(), number),
			"a fund node's parent is its type, so a fund the registry does not list "+
				"cannot be placed in the hierarchy")
	}
	for _, f := range lines {
		if f.FundGroup != group {
			return Node{}, "", fmt.Errorf("%s: fact %s is printed under %q and data/funds.yaml "+
				"puts fund %d in %q", u.Name(), f.ID, f.FundGroup, number, group)
		}
	}
	e := endpoint{id: PrefixFund + strconv.Itoa(number), role: transferFundRole(number)}
	n := Node{ID: e.id, Label: u.label(e), Tier: e.tier(), Role: e.role,
		Parent: PrefixFundGroup + group}
	annotateFund(&n, u.Labels, number)
	b := &NodeBalances{Beginning: nodeBalance(lines[lineBeginning]), Ending: nodeBalance(lines[lineEnding]),
		Change: nodeChange(lines)}
	if b.Beginning != nil || b.Ending != nil {
		n.Balances = b
	}
	return n, group, nil
}

// nodeChange is a fund's change in balance citing both balances, nil where
// either is blank. A zero change is stated: two printed balances that agree
// are a change of nothing, where a blank is no change at all.
func nodeChange(lines fundLines) *NodeChange {
	cited, ids, ok := bothBalances(lines)
	if !ok {
		return nil
	}
	return &NodeChange{ValueCents: ChangeCents(cited), Derived: true,
		Rationale:  NodeChangeRationale,
		SourceNote: NodeChangeSourceNote,
		FactIDs:    ids, Locators: SourcesOf(cited)}
}

// bothBalances is a fund's beginning and ending balance and their sorted ids,
// and false where either is blank.
func bothBalances(lines fundLines) ([]fact.Fact, []string, bool) {
	beginning, ending := lines[lineBeginning], lines[lineEnding]
	if beginning == nil || ending == nil {
		return nil, nil, false
	}
	ids := []string{beginning.ID, ending.ID}
	slices.Sort(ids)
	return []fact.Fact{*beginning, *ending}, ids, true
}

// NodeChangeRationale and NodeChangeSourceNote are every [NodeChange]'s words.
const (
	NodeChangeRationale = "Our subtraction of two printed figures: this fund's ending balance less " +
		"its beginning balance. The city prints both balances and not their difference."
	NodeChangeSourceNote = "Budget Book PDF pp.186-209, this fund's beginning and ending balance"
)

// nodeBalance cites one printed balance, nil when the page prints none.
func nodeBalance(f *fact.Fact) *NodeBalance {
	if f == nil {
		return nil
	}
	return &NodeBalance{ValueCents: f.AmountCents, FactID: f.ID, Locators: SourcesOf([]fact.Fact{*f})}
}

// changeLink is a fund's change in balance as one derived link, and false
// when the change is zero or a balance is not printed: a blank is absent,
// and reading it as zero would invent the change.
func changeLink(fund string, lines fundLines) (Link, bool) {
	cited, ids, ok := bothBalances(lines)
	if !ok {
		return Link{}, false
	}
	change := ChangeCents(cited)
	if change == 0 {
		return Link{}, false
	}
	l := Link{Source: fund, Target: NodeFundBalanceContribution, ValueCents: change,
		Kind: KindFundBalance, Derived: true, FactIDs: ids, Locators: SourcesOf(cited)}
	if change < 0 {
		l.Source, l.Target, l.ValueCents = NodeFundBalanceDraw, fund, -change
	}
	return l, true
}

// addEndpoint records a flow endpoint the first time a link touches it.
func (u *fundSourcesUses) addEndpoint(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	n := Node{ID: e.id, Label: u.label(e), Tier: e.tier(), Role: e.role}
	if d, ok := derivedNodes[e.id]; ok {
		n.Derived, n.Rationale, n.SourceNote = true, d.rationale, d.sourceNote
	}
	nodes[e.id] = n
}

// label is nodeLabel over this document's registry.
func (u *fundSourcesUses) label(e endpoint) string { return nodeLabel(u.Labels, e.id, e.slug) }

// sortedFundNumbers is the funds in ascending order.
func sortedFundNumbers(m map[int]fundLines) []int { return slices.Sorted(maps.Keys(m)) }

// FundImbalance is each fund node's drawn sources less its drawn uses, for
// every fund where they differ.
func FundImbalance(links []Link) map[string]int64 {
	net := map[string]int64{}
	for _, l := range links {
		if strings.HasPrefix(l.Target, PrefixFund) {
			net[l.Target] += l.ValueCents
		}
		if strings.HasPrefix(l.Source, PrefixFund) {
			net[l.Source] -= l.ValueCents
		}
	}
	for id, v := range net {
		if v == 0 {
			delete(net, id)
		}
	}
	return net
}

// fundSourcesUsesCaveats are what a reader of this document has to be told.
// Every figure in them is summed from the document's own links.
func fundSourcesUsesCaveats(links []Link, nodes []Node) []Caveat {
	out := []Caveat{ConstraintTierCaveat(), cipBlockCaveat}

	parent := map[string]string{}
	for _, n := range nodes {
		parent[n.ID] = n.Parent
	}
	var draws, contributions int64
	var drawn, contributed int
	byGroup := map[string]int64{}
	for _, l := range links {
		switch {
		case l.Source == NodeFundBalanceDraw:
			draws += l.ValueCents
			drawn++
			byGroup[parent[l.Target]] -= l.ValueCents
		case l.Target == NodeFundBalanceContribution:
			contributions += l.ValueCents
			contributed++
			byGroup[parent[l.Source]] += l.ValueCents
		}
	}
	if drawn+contributed > 0 {
		var netDraws, netContributions int64
		for _, v := range byGroup {
			if v < 0 {
				netDraws -= v
			} else {
				netContributions += v
			}
		}
		net := " Within a fund group some funds draw and others contribute, so these add up " +
			"to more than the group's net change."
		if netDraws == draws && netContributions == contributions {
			net = " No fund group here both draws and contributes, so these are also each " +
				"group's net change."
		}
		out = append(out, Caveat{
			ID:      "each-fund-change-is-gross",
			Theme:   ThemeSameMoneyTwice,
			Summary: "Each fund's own change in balance is drawn, so these can add to more than the citywide chart's.",
			Text: fmt.Sprintf("Budget Book pp.186-209 print each fund's beginning and ending "+
				"balance, and this chart draws the change fund by fund: %s draw %s and %s "+
				"contribute %s. Netted within each fund group, the same changes come to draws of "+
				"%s and contributions of %s, and that net is what the citywide chart draws, from "+
				"pp.66-67's own figures.",
				counted(drawn, "fund"), amount.Cents(draws).Dollars(),
				counted(contributed, "fund"), amount.Cents(contributions).Dollars(),
				amount.Cents(netDraws).Dollars(), amount.Cents(netContributions).Dollars()) + net,
			AppliesTo: changeEnds(links),
		})
	}

	imbalance := FundImbalance(links)
	if len(imbalance) > 0 {
		ids := make([]string, 0, len(imbalance))
		var blank, rounding int
		var largest int64
		for _, n := range nodes {
			v, ok := imbalance[n.ID]
			if !ok {
				continue
			}
			ids = append(ids, n.ID)
			if n.Balances == nil || n.Balances.Beginning == nil || n.Balances.Ending == nil {
				blank++
				continue
			}
			rounding++
			largest = max(largest, v, -v)
		}
		sort.Strings(ids)
		text := fmt.Sprintf("Each fund's page prints its beginning balance plus money in as equal "+
			"to money out plus its ending balance. For %s drawn here, money in and money out "+
			"differ.", counted(len(ids), "fund"))
		if blank > 0 {
			text += fmt.Sprintf(" For %s the page leaves a balance blank, so no change in "+
				"balance is drawn.", counted(blank, "fund"))
		}
		if rounding > 0 {
			text += fmt.Sprintf(" For %s the printed figures themselves miss by at most %s, "+
				"and are drawn as printed.", counted(rounding, "fund"),
				amount.Cents(largest).Dollars())
		}
		out = append(out, Caveat{
			ID:        "sources-and-uses-differ",
			Theme:     ThemeBooksDisagree,
			Summary:   "For some funds drawn here, money in does not equal money out.",
			Text:      text,
			AppliesTo: ids,
		})
	}
	return out
}

// changeEnds is the change endpoints the links draw, sorted.
func changeEnds(links []Link) []string {
	seen := map[string]bool{}
	for _, l := range links {
		for _, id := range []string{l.Source, l.Target} {
			if id == NodeFundBalanceDraw || id == NodeFundBalanceContribution {
				seen[id] = true
			}
		}
	}
	out := []string{}
	for _, id := range []string{NodeFundBalanceDraw, NodeFundBalanceContribution} {
		if seen[id] {
			out = append(out, id)
		}
	}
	return out
}

// cipBlockCaveat says which block of the pages is not drawn.
var cipBlockCaveat = Caveat{
	ID:      "the-cip-funds-block-is-not-drawn",
	Theme:   ThemeLeftOut,
	Summary: "Capital project funds are not drawn here; their money is on the capital projects pages.",
	Text: "Budget Book pp.190, 196, 202 and 208 print a block of capital project funds below " +
		"the operating funds. That money is outside the operating budget pp.66-67 total, and " +
		"p222 lists it, so this chart draws the operating funds alone; adding the block would " +
		"produce group totals no page prints.",
	AppliesTo: []string{},
}
