package project

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
)

// FundFlowsProjection is this document's name and file stem.
const FundFlowsProjection = "fund-flows"

// The three schedules this document is of.
//
// ScopeRevenueByFund is TrendsScope's string, named for the schedule rather
// than for its first consumer.
const (
	ScopeRevenueByFund           = TrendsScope
	scopeExpenditureByDepartment = "expenditure-by-department"
	scopeExpenditureByFund       = "expenditure-by-fund"
)

// FundFlowsScopes is the schedule set, in the order a reader meets the money:
// revenue first, then what it is spent on. expenditure-by-fund carries no
// General Fund -- p172's is general-fund-by-category, which pp.167-170
// decompose -- so the set constructs as a view (structure.ViewOf).
func FundFlowsScopes() []string {
	return []string{ScopeRevenueByFund, scopeExpenditureByDepartment, scopeExpenditureByFund}
}

// generalFund is the only fund pp.167-170 decompose, and the tier-3 node the
// department axis hangs from. p0167.txt:3 prints "General Fund Expenditures by
// Major Category".
const generalFund = 100

// constraintTierCaveat is the disclosure docs/sankey-contract.md requires of any
// document publishing a constraint tier, a constant so the check compares
// against the same bytes.
const constraintTierCaveat = "A fund's constraint tier is OUR reading of the " +
	"Description of Funds narrative (Budget Book pp. 258-261), not something the city " +
	"printed. Each fund node carries the note it was read from. \"unknown\" means the " +
	"document does not establish a restriction; it is a classification, not a gap."

// ConstraintTierCaveat is the disclosure the check compares a document against:
// found by ConstraintTierCaveatID, then Text and Summary each compared.
func ConstraintTierCaveat() Caveat {
	return Caveat{
		ID:      ConstraintTierCaveatID,
		Summary: "A fund's constraint tier is our reading of the Description of Funds narrative, not a figure the city printed.",
		Text:    constraintTierCaveat,
		// Document-wide: every fund carries a tier, so marking all marks none.
		AppliesTo: []string{},
	}
}

// ConstraintTierCaveatID is the anchor, separate from the sentence, so a missing
// disclosure and a drifted one are different findings.
const ConstraintTierCaveatID = "constraint-tier-is-our-reading"

// MultiScopeEnvelope is [Envelope] for a document of more than one schedule.
//
// A sibling rather than a widening of Envelope, so single-schedule documents
// keep `scope` as one string; the other three keys match Envelope's, in order.
type MultiScopeEnvelope struct {
	GeneratedBy string   `json:"generated_by"`
	Scopes      []string `json:"scopes"`
	Currency    string   `json:"currency"`
	Units       string   `json:"units"`
}

// FundFlowsCounts is how much of the corpus this document accounts for.
//
// It is not [Counts]: here a fact can be behind more than one link, so the
// overlap is published as a number.
type FundFlowsCounts struct {
	// Facts is every fact matching the columns and either scope.
	Facts int `json:"facts"`
	// FactsCited is how many DISTINCT facts some link carries.
	FactsCited int `json:"facts_cited"`
	// FactsUncited is the facts no link carries, every one in a cell that netted
	// to zero. Facts = FactsCited + FactsUncited. It is "uncited" rather than "in
	// a zero cell" because a zero object cell is still cited by the
	// fund-to-department link that sums its division.
	FactsUncited int `json:"facts_uncited"`
	// FactsCitedTwice is how many distinct facts are behind MORE THAN ONE link
	// (a set, not a tally). It warns that summing every link's value_cents
	// double-counts; folding within one tier pair does not.
	FactsCitedTwice int `json:"facts_cited_twice"`
	Nodes           int `json:"nodes"`
	Links           int `json:"links"`
}

// FundFlowsDocument is the whole published file.
type FundFlowsDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Projection    string            `json:"projection"`
	Metadata      FundFlowsMetadata `json:"metadata"`
	Nodes         []Node            `json:"nodes"`
	Links         []Link            `json:"links"`
}

// FundFlowsMetadata is the document's own block.
//
// It is of one column, and deliberately carries no headline (see fundFlows).
type FundFlowsMetadata struct {
	MultiScopeEnvelope
	FiscalYear      int             `json:"fiscal_year"`
	FiscalYearLabel string          `json:"fiscal_year_label"`
	Basis           string          `json:"basis"`
	Sources         []Source        `json:"sources"`
	Counts          FundFlowsCounts `json:"counts"`
	Caveats         []Caveat        `json:"caveats"`
}

// fundFlows draws the General Fund drill-down: where a fund's revenue comes from
// and, for the General Fund, what it is spent on.
//
// IT PUBLISHES NO HEADLINE: it holds the same money at more than one grain, so
// "the total" is ambiguous (fisc-xau). sankey.json stays the only document
// with a headline; cuts-tie-along-the-lattice reconciles the two per cell.
//
// # The shape
//
//	tier 0  revenue/<category>, transfers/in
//	tier 1  revenue-line/<line>   parent = revenue/<category>
//	          |  one link per (line, kind) back INTO the category, the sum of
//	          |  that row's own cells
//	          |  one link per netted (kind, line, fund) cell, and one per
//	          |  (transfers/in, fund) cell straight from tier 0
//	tier 2  fund-group/<type>
//	          |  one link per (fund, kind) INTO the fund, that fund's own inflow
//	tier 3  fund/<n>            parent = fund-group/<type> from data/funds.yaml
//	          |  one link per (fund, division) -- the SUM over its object rows
//	tier 4  dept/<division>     parent = fund/100
//	          |  one link per netted (department, category) cell
//	tier 5  expenditure/<division>/<object>   parent = dept/<division>
//	tier 5  expenditure/fund/<n>/<object>     parent = fund/<n>, one link per
//	          netted (fund, category) cell of pp.173-183, from the fund itself
//
// MIXED GRAIN IS UNAVOIDABLE: pp.167-170 are General Fund only, so every other
// fund reaches its object categories with no division between (fisc-gkv).
//
// The (3,4), (1,0) and (2,3) links are emitted rather than left to the client
// to fold: without them a view drawing those tiers finds nothing flowing into
// the middle node and draws it at zero. The cost is that a fact is cited more
// than once (FactsCitedTwice).
type fundFlows struct {
	// Labels supplies the city's words and the fund hierarchy; it is required.
	Labels labels
}

var (
	_ Projection = (*fundFlows)(nil)
	_ Sliced     = (*fundFlows)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*fundFlows) Name() string { return FundFlowsProjection }

// Slices is one Options per column that pp.127-140 and pp.167-170 both carry,
// so no document draws a revenue side against an empty expenditure side.
// pp.173-183 are not required of a column: one they left out would draw every
// fund but the General Fund ending at itself, which some-funds-show-no-spending
// discloses.
func (*fundFlows) Slices(facts []fact.Fact, version string) []Options {
	type col = Column
	seen := map[string]map[col]bool{
		ScopeRevenueByFund:           {},
		scopeExpenditureByDepartment: {},
	}
	for i := range facts {
		c := facts[i].Scope
		if m, ok := seen[c]; ok {
			m[col{FiscalYear: facts[i].FiscalYear, Basis: facts[i].Basis}] = true
		}
	}
	cols := make([]col, 0, len(seen[ScopeRevenueByFund]))
	for c := range seen[ScopeRevenueByFund] {
		if seen[scopeExpenditureByDepartment][c] {
			cols = append(cols, c)
		}
	}
	sort.Slice(cols, func(i, j int) bool {
		if cols[i].FiscalYear != cols[j].FiscalYear {
			return cols[i].FiscalYear < cols[j].FiscalYear
		}
		return cols[i].Basis < cols[j].Basis
	})
	out := make([]Options, 0, len(cols))
	for _, c := range cols {
		out = append(out, Options{Columns: []Column{c}, Scopes: FundFlowsScopes(), Version: version})
	}
	return out
}

// Build is [Projection]'s entry point.
func (f *fundFlows) Build(facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := f.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, f.Name(), schema.Projection)
}

// Document builds the drill-down and returns it, so `fisc verify` reads the
// same structure `fisc export` writes rather than re-parsing the JSON.
func (f *fundFlows) Document(facts []fact.Fact, o Options) (*FundFlowsDocument, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("fund-flows options: %w", err)
	}
	if !sameScopes(o.Scopes, FundFlowsScopes()) {
		return nil, cmdutil.WithHint(
			fmt.Errorf("fund-flows: scopes are %q, want %q", o.ScopeList(),
				Options{Scopes: FundFlowsScopes()}.ScopeList()),
			"this document is of three schedules and all are required; without one it "+
				"draws a revenue side with no spending, a department axis with no income, "+
				"or every fund but the General Fund ending at itself")
	}
	if len(o.Columns) != 1 {
		return nil, cmdutil.WithHint(
			fmt.Errorf("fund-flows: a graph is of one column, got %d", len(o.Columns)),
			"two budget years in one flow diagram add every figure to its own successor")
	}
	if f.Labels == nil {
		return nil, cmdutil.WithHint(
			fmt.Errorf("fund-flows: no registry is attached"),
			"a fund node's parent comes from data/funds.yaml's type:, so this document "+
				"cannot be built without one")
	}
	col := o.Columns[0]
	selected := selectFacts(facts, o)

	rev, exp, byFund, err := f.netFundFlows(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	var links []Link
	cited := map[string]bool{}
	zero := map[string]bool{}
	twice := map[string]bool{}

	// Tier 0 -> 3, one link per revenue cell; tier 1 -> 0, one link per
	// (line, kind) carrying that line's own sum; and tier 2 -> 3, one link per
	// (fund, kind) carrying the fund's own inflow.
	perLine := map[rollupKey]*lineRollup{}
	perFund := map[fundRollupKey]*cellSum{}
	for _, k := range sortedRevKeys(rev) {
		c := rev[k]
		src, srcErr := f.revenueEndpoint(k)
		if srcErr != nil {
			return nil, srcErr
		}
		dst, dstErr := f.fundEndpoint(k.fund)
		if dstErr != nil {
			return nil, dstErr
		}
		if c.cents == 0 {
			// The facts survive; a printed dash is a fact and not a flow.
			for _, id := range c.factIDs {
				zero[id] = true
			}
			continue
		}
		f.addFundFlowNode(nodes, src)
		f.addFundFlowNode(nodes, dst)
		for _, id := range c.factIDs {
			cited[id] = true
		}
		kind := revenueLinkKind(k)
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			Kind: kind, FactIDs: c.factIDs,
			Locators: c.locs.sources(),
		})
		// The (2,3) rollup includes transfers: a fund group's printed inflow on
		// p0067 counts them.
		fk := fundRollupKey{fund: k.fund, kind: kind}
		fr := perFund[fk]
		if fr == nil {
			fr = &cellSum{}
			perFund[fk] = fr
		}
		fr.cents += c.cents
		fr.factIDs = append(fr.factIDs, c.factIDs...)
		fr.locs.merge(&c.locs)
		// A transfer is not a line, so it has no (1,0) rollup.
		if src.tier != tierRevenueLine {
			continue
		}
		// Unlike the tier-3-to-4 division total, this rollup skips zero cells,
		// so a printed dash stays uncited.
		rk := rollupKey{line: k.line, kind: kind}
		r := perLine[rk]
		if r == nil {
			r = &lineRollup{src: src}
			perLine[rk] = r
		}
		r.cents += c.cents
		r.factIDs = append(r.factIDs, c.factIDs...)
		r.locs.merge(&c.locs)
	}

	// Tier 1 -> 0: the printed rows added back up into the category they are
	// printed under.
	for _, k := range sortedRollupKeys(perLine) {
		r := perLine[k]
		// A line whose cells cancel draws nothing; its cells are already cited.
		if r.cents == 0 {
			continue
		}
		sort.Strings(r.factIDs)
		f.addFundFlowNode(nodes, r.src)
		for _, id := range r.factIDs {
			if cited[id] {
				twice[id] = true
			}
			cited[id] = true
		}
		links = append(links, Link{
			Source: r.src.id, Target: r.src.parent, ValueCents: r.cents,
			Kind: k.kind, FactIDs: r.factIDs,
			Locators: r.locs.sources(),
		})
	}

	// Tier 2 -> 3: each fund's own inflow, under the group data/funds.yaml puts
	// it in.
	for _, k := range sortedFundRollupKeys(perFund) {
		r := perFund[k]
		// A fund whose cells cancel draws nothing; its cells are already cited.
		if r.cents == 0 {
			continue
		}
		dst, dstErr := f.fundEndpoint(k.fund)
		if dstErr != nil {
			return nil, dstErr
		}
		sort.Strings(r.factIDs)
		f.addFundFlowNode(nodes, dst)
		// The group is read off the node addFundFlowNode built, and an empty
		// one is refused rather than published as a link from "".
		group := nodes[dst.id].Parent
		if group == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("fund-flows: fund %d is in no fund group", k.fund),
				"the tier-2 link that lets a group be drawn between the categories and "+
					"its funds runs from that group, so a fund with none has no source end")
		}
		for _, id := range r.factIDs {
			if cited[id] {
				twice[id] = true
			}
			cited[id] = true
		}
		links = append(links, Link{
			Source: group, Target: dst.id, ValueCents: r.cents,
			Kind: k.kind, FactIDs: r.factIDs,
			Locators: r.locs.sources(),
		})
	}

	// Tier 4 -> 5, one link per expenditure cell, and tier 3 -> 4, one link per
	// division carrying the sum.
	perDivision := map[string]*cellSum{}
	for _, k := range sortedExpKeys(exp) {
		c := exp[k]
		d := perDivision[k.division]
		if d == nil {
			d = &cellSum{}
			perDivision[k.division] = d
		}
		// The division's total and citation include the zero cells, so both
		// sides of link-values-tie-to-facts are of one set.
		d.cents += c.cents
		d.factIDs = append(d.factIDs, c.factIDs...)
		d.locs.merge(&c.locs)

		if c.cents == 0 {
			for _, id := range c.factIDs {
				zero[id] = true
			}
			continue
		}
		src, divErr := f.divisionEndpoint(k.division)
		if divErr != nil {
			return nil, divErr
		}
		dst := f.objectEndpoint(k.division, k.category)
		f.addFundFlowNode(nodes, src)
		f.addFundFlowNode(nodes, dst)
		for _, id := range c.factIDs {
			cited[id] = true
		}
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			Kind: KindExternal, FactIDs: c.factIDs,
			Locators: c.locs.sources(),
		})
	}

	fundNode, err := f.fundEndpoint(generalFund)
	if err != nil {
		return nil, err
	}
	for _, division := range sortedKeys(perDivision) {
		d := perDivision[division]
		if d.cents == 0 {
			continue
		}
		sort.Strings(d.factIDs)
		dst, divErr := f.divisionEndpoint(division)
		if divErr != nil {
			return nil, divErr
		}
		f.addFundFlowNode(nodes, fundNode)
		f.addFundFlowNode(nodes, dst)
		for _, id := range d.factIDs {
			if cited[id] {
				twice[id] = true
			}
			cited[id] = true
		}
		links = append(links, Link{
			Source: fundNode.id, Target: dst.id, ValueCents: d.cents,
			Kind: KindExternal, FactIDs: d.factIDs,
			Locators: d.locs.sources(),
		})
	}

	// Tier 3 -> 5, one link per pp.173-183 cell, from the fund itself.
	for _, k := range sortedFundExpKeys(byFund) {
		c := byFund[k]
		if c.cents == 0 {
			for _, id := range c.factIDs {
				zero[id] = true
			}
			continue
		}
		src, srcErr := f.fundEndpoint(k.fund)
		if srcErr != nil {
			return nil, srcErr
		}
		dst := fundObjectEndpoint(k.fund, k.category)
		f.addFundFlowNode(nodes, src)
		f.addFundFlowNode(nodes, dst)
		for _, id := range c.factIDs {
			cited[id] = true
		}
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			Kind: boundaryKind(k.fundGroup), FactIDs: c.factIDs,
			Locators: c.locs.sources(),
		})
	}

	// Every parent must resolve to a node in this document, even one no link
	// touches.
	if err := f.addParents(nodes); err != nil {
		return nil, err
	}
	nameContraLinks(links, nodes)

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}
	out := sortedNodes(nodes)

	// Every uncited fact must be a printed zero; anything else is money
	// dropped in silence.
	uncited := 0
	for i := range selected {
		id := selected[i].ID
		if cited[id] {
			continue
		}
		uncited++
		if !zero[id] {
			return nil, cmdutil.WithHint(
				fmt.Errorf("fund-flows: fact %s is carried by no link and is not a printed "+
					"zero", id),
				"facts = facts_cited + facts_uncited is this document's published identity "+
					"and every uncited fact is a cell that netted to zero; a fact reaching "+
					"no link for another reason is money dropped in silence")
		}
	}

	// Validated against this document's own nodes: an AppliesTo naming a
	// node it does not carry marks nothing.
	cavs := fundFlowsCaveats(len(twice), out)
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &FundFlowsDocument{
		SchemaVersion: SchemaVersion,
		Projection:    f.Name(),
		Metadata: FundFlowsMetadata{
			MultiScopeEnvelope: MultiScopeEnvelope{
				GeneratedBy: o.Version,
				Scopes:      FundFlowsScopes(),
				Currency:    "USD",
				Units:       "cents",
			},
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Sources:         sourcesOf(selected),
			Counts: FundFlowsCounts{
				Facts:           len(selected),
				FactsCited:      len(cited),
				FactsUncited:    uncited,
				FactsCitedTwice: len(twice),
				Nodes:           len(out),
				Links:           len(links),
			},
			Caveats: cavs,
		},
		Nodes: out,
		Links: links,
	}, nil
}

// fundFlowsCaveats are the things a reader of this file has to be told. The
// first three are unconditional; the last two are published only where their
// sentences are true of this column.
func fundFlowsCaveats(twice int, nodes []Node) []Caveat {
	out := []Caveat{
		ConstraintTierCaveat(),
		revenueSchedulePublishedTwiceCaveat(),
		{
			ID:      "mixed-grain-double-counts",
			Summary: "This document holds the same money at two grains, so summing every link double-counts.",
			Text: fmt.Sprintf("This document holds the same money at more than one grain, so summing "+
				"every link double-counts: %d fact(s) are behind more than one link. A revenue "+
				"row is behind its own flow into a fund, the rollup of its line into the "+
				"category it is printed under, and the rollup of its fund under the group that "+
				"fund belongs to; an expenditure row is behind both its "+
				"department's object rows and the fund-to-department link that totals them. "+
				"Fold within one tier pair, never across the whole graph. It publishes no "+
				"headline for this reason.",
				twice),
			AppliesTo: []string{},
		},
	}
	divided, direct := spendingSides(nodes)
	if len(divided) == 1 && divided[prefixFundGroup+"general"] && len(direct) > 0 {
		out = append(out, Caveat{
			ID: "only-the-general-fund-has-divisions",
			Summary: "Only the General Fund opens into divisions; every other fund's spending " +
				"goes straight to its object categories.",
			Text: "Budget Book pp.167-170 break the General Fund down by division and then by " +
				"object category. pp.173-183 print every other fund by object category alone, " +
				"with no division, so their spending is drawn from the fund straight to its " +
				"categories and the division column holds the General Fund's alone. The " +
				"revenue and spending schedules are read side by side and do not balance " +
				"fund by fund: a fund pp.127-140 print revenue for and pp.173-183 print no " +
				"spending for is drawn with money arriving and none leaving, and one that " +
				"spends with no printed revenue with money leaving and none arriving.",
			// The groups drawn without divisions, and fund/100, the exception.
			AppliesTo: append(sortedKeys(direct), prefixFund+strconv.Itoa(generalFund)),
		})
	}
	if stopped := truncatedGroups(nodes); len(stopped) > 0 {
		out = append(out, Caveat{
			ID: "some-funds-show-no-spending",
			// The count is the document's: columns differ in which groups stop.
			Summary: fmt.Sprintf("The %s money ends at their funds.", plural(len(stopped), "fund group")),
			Text: fmt.Sprintf("The schedules this document reads for spending, Budget Book "+
				"pp.167-170 and pp.173-183, give none in this column for the %s funds, so "+
				"their money is drawn into the fund and no further.",
				plural(len(stopped), "fund group")),
			AppliesTo: stopped,
		})
	}
	return out
}

// plural writes "1 group's" and "5 groups'".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s's", n, noun)
	}
	return fmt.Sprintf("%d %ss'", n, noun)
}

// spendingSides is the fund groups this document draws spending for:
// divided, a division beneath one of its funds (pp.167-170), and direct, an
// object category hung from one of its funds itself (pp.173-183).
//
// The caveats' counts and their marks all come from here, so they agree.
func spendingSides(nodes []Node) (divided, direct map[string]bool) {
	parent := map[string]string{}
	for _, n := range nodes {
		if n.Tier == tierFund {
			parent[n.ID] = n.Parent
		}
	}
	divided, direct = map[string]bool{}, map[string]bool{}
	for _, n := range nodes {
		g := parent[n.Parent]
		switch {
		case g == "":
		case n.Tier == tierDepartment:
			divided[g] = true
		case n.Tier == tierObjectCategory:
			direct[g] = true
		}
	}
	return divided, direct
}

// truncatedGroups is the fund groups whose money ends at their funds, sorted.
// Counted per document, because the published columns differ.
func truncatedGroups(nodes []Node) []string {
	divided, direct := spendingSides(nodes)
	out := []string{}
	for _, n := range nodes {
		if strings.HasPrefix(n.ID, prefixFundGroup) && !divided[n.ID] && !direct[n.ID] {
			out = append(out, n.ID)
		}
	}
	slices.Sort(out)
	return out
}

// cellSum is a netted figure and the facts behind it.
type cellSum struct {
	cents   int64
	factIDs []string
	locs    locatorSet
}

// revKey addresses a revenue cell: the money a fund takes in, by printed row.
// line is a data/taxonomy.yaml line slug (`taxes/property/eraf`), empty for a
// transfer, whose node is a flow endpoint rather than a line.
type revKey struct {
	kind      mapping.Kind
	category  string
	line      string
	fundGroup string
	fund      int
}

// expKey addresses an expenditure cell: what a division spends, by object.
//
// The fund is not in the key; netFundFlows refuses a second fund.
type expKey struct {
	division string
	category string
}

// fundExpKey addresses an expenditure cell of pp.173-183: what a fund other
// than the General Fund spends, by object.
type fundExpKey struct {
	fundGroup string
	fund      int
	category  string
}

// rollupKey addresses a revenue line's rollup into its category, per link kind:
// one printed row reaches the Internal Service Funds as an internal service
// charge and the rest of the city as external revenue.
type rollupKey struct {
	line string
	kind LinkKind
}

// fundRollupKey addresses a fund's inflow rolled up under its group, per link
// kind for rollupKey's reason.
type fundRollupKey struct {
	fund int
	kind LinkKind
}

// lineRollup is one line's own sum and the endpoint that carries its category.
type lineRollup struct {
	cellSum
	src endpoint
}

// netFundFlows sums the selected facts into the two cell maps, refusing anything
// it cannot address.
func (f *fundFlows) netFundFlows(facts []fact.Fact) (map[revKey]*cellSum, map[expKey]*cellSum,
	map[fundExpKey]*cellSum, error) {
	rev := map[revKey]*cellSum{}
	exp := map[expKey]*cellSum{}
	byFund := map[fundExpKey]*cellSum{}
	var expFund *int
	for i := range facts {
		fa := &facts[i]
		switch fa.Scope {
		case ScopeRevenueByFund:
			if fa.Category == "" {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s carries no category", fa.ID),
					"a revenue node is a category, so a fact without one has no source end")
			}
			if fa.Fund == nil {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) names no fund", fa.ID, fa.Category),
					"this document's tier 3 IS the fund, and a fact without one has no box "+
						"to land in")
			}
			if fa.FundGroup == "" {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) names no fund group", fa.ID, fa.Category),
					"the fund group decides whether a flow crosses the city's boundary")
			}
			line, lineErr := f.revenueLine(fa)
			if lineErr != nil {
				return nil, nil, nil, lineErr
			}
			add(rev, revKey{fa.Kind, fa.Category, line, fa.FundGroup, *fa.Fund}, fa)
		case scopeExpenditureByDepartment:
			if fa.Department == "" {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s carries no department", fa.ID),
					"this document's tier 4 IS the department")
			}
			if fa.Category == "" {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) carries no category", fa.ID, fa.Department),
					"an object-category node is a category")
			}
			// A fundless fact would be attributed to the General Fund on no
			// evidence, and no downstream check would see it.
			if fa.Fund == nil {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) names no fund", fa.ID, fa.Department),
					"this document parents every department to the fund that pays it, and "+
						"a fact naming none has no parent to give it")
			}
			// One fund on the whole expenditure side: the tier-5 id carries no
			// fund, so a second one would collide on one link.
			if expFund == nil {
				expFund = fa.Fund
			}
			if *fa.Fund != *expFund {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: the expenditure side names funds %d and %d",
						*expFund, *fa.Fund),
					"pp.167-170 are a General Fund schedule and this document's department "+
						"axis assumes it; pp.72-75 give a per-fund expenses column and are "+
						"the schedule a wider key would be for")
			}
			add(exp, expKey{fa.Department, fa.Category}, fa)
		case scopeExpenditureByFund:
			if fa.Category == "" || fa.Fund == nil || fa.FundGroup == "" {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s names no category, fund or fund group", fa.ID),
					"pp.173-183 draw a fund into its object categories, so a fact missing "+
						"either end has no link to be")
			}
			// The General Fund is drawn through its divisions; a second
			// reading of it here would draw its spending twice.
			if *fa.Fund == generalFund {
				return nil, nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s draws fund %d from expenditure-by-fund", fa.ID, generalFund),
					"pp.167-170 decompose the General Fund, and p172's block of it is "+
						"general-fund-by-category, which this document does not read")
			}
			// The node's group is the registry's; a fact filed under another
			// would draw a link of the wrong kind under the right group.
			t, ok := f.Labels.FundType(*fa.Fund)
			if !ok {
				return nil, nil, nil, fmt.Errorf("fund-flows: fact %s names fund %d, which "+
					"data/funds.yaml does not list", fa.ID, *fa.Fund)
			}
			if t != fa.FundGroup {
				return nil, nil, nil, fmt.Errorf("fund-flows: fact %s files fund %d under %q and "+
					"data/funds.yaml puts it in %q", fa.ID, *fa.Fund, fa.FundGroup, t)
			}
			add(byFund, fundExpKey{fundGroup: fa.FundGroup, fund: *fa.Fund, category: fa.Category}, fa)
		default:
			return nil, nil, nil, fmt.Errorf("fund-flows: fact %s is in scope %q, which this "+
				"document does not select", fa.ID, fa.Scope)
		}
	}
	if expFund != nil && *expFund != generalFund {
		return nil, nil, nil, fmt.Errorf("fund-flows: the expenditure side is fund %d, want %d",
			*expFund, generalFund)
	}
	return rev, exp, byFund, nil
}

// revenueLine is the data/taxonomy.yaml line a revenue fact's printed row names,
// and "" for a transfer, whose node is a flow endpoint rather than a line.
//
// A row the registry cannot place is refused rather than folded into its
// category, which every amount check would pass. Absence and ambiguity are
// refused separately because they are different taxonomy defects.
func (f *fundFlows) revenueLine(fa *fact.Fact) (string, error) {
	if fa.Kind != mapping.KindRevenue {
		return "", nil
	}
	if fa.RowLabel == "" {
		return "", cmdutil.WithHint(
			fmt.Errorf("fund-flows: fact %s (%s) carries no row label", fa.ID, fa.Category),
			"a revenue line IS the printed row, and a fact with no row label names no row "+
				"for the registry to resolve")
	}
	lines := f.Labels.LinesPrintedAs(fa.Category, fa.RowLabel, string(fa.Kind))
	switch len(lines) {
	case 1:
		return lines[0], nil
	case 0:
		return "", cmdutil.WithHint(
			fmt.Errorf("fund-flows: fact %s: no data/taxonomy.yaml line under %q is printed as %q",
				fa.ID, fa.Category, fa.RowLabel),
			"every revenue row of pp.127-140 is a node of this document, so a row the "+
				"taxonomy does not declare has nowhere to go; fact-revenue-lines-resolve "+
				"reports the same gap over the whole store")
	default:
		return "", cmdutil.WithHint(
			fmt.Errorf("fund-flows: fact %s: %q under %q is printed by %d lines, %s",
				fa.ID, fa.RowLabel, fa.Category, len(lines), strings.Join(lines, ", ")),
			"one printed row is one node, so two lines claiming one spelling is an "+
				"identity nothing can draw once")
	}
}

// add sums one fact into a cell map.
func add[K comparable](m map[K]*cellSum, k K, fa *fact.Fact) {
	c := m[k]
	if c == nil {
		c = &cellSum{}
		m[k] = c
	}
	c.cents += fa.AmountCents
	c.factIDs = append(c.factIDs, fa.ID)
	c.locs.add(fa)
}

// revenueLinkKind classifies a tier-0-to-3 flow.
//
// The fact's kind decides first: a transfer crosses no boundary whatever group
// receives it, so boundaryKind applies to revenue only.
func revenueLinkKind(k revKey) LinkKind {
	if k.kind == mapping.KindTransferIn {
		return KindInternalTransfer
	}
	return boundaryKind(k.fundGroup)
}

// revenueEndpoint is the source end of a revenue flow: a tier-1 line for a
// revenue row, and the tier-0 flow endpoint for a transfer.
//
// The parent rides on the endpoint because it cannot be cut back out of the
// line slug: a category slug is itself one or two segments.
func (*fundFlows) revenueEndpoint(k revKey) (endpoint, error) {
	switch k.kind {
	case mapping.KindRevenue:
		if k.line == "" {
			return endpoint{}, cmdutil.WithHint(
				fmt.Errorf("fund-flows: a revenue cell of %q carries no line", k.category),
				"pp.127-140 print revenue as rows and this document draws the rows, so a "+
					"cell with no line has no source end")
		}
		return endpoint{id: prefixRevenueLine + k.line, slug: k.line,
			tier: tierRevenueLine, role: roleRevenueLine,
			parent: prefixRevenue + k.category}, nil
	case mapping.KindTransferIn:
		return endpoint{id: nodeTransfersIn, slug: k.category,
			tier: tierRevenueSource, role: roleTransferIn}, nil
	default:
		return endpoint{}, cmdutil.WithHint(
			fmt.Errorf("fund-flows: kind %q has no source end in this document", k.kind),
			"pp.127-140 print revenue and transfers in; a third kind means the schedule "+
				"or the taxonomy changed under this projection")
	}
}

// fundEndpoint is a tier-3 fund node.
//
// Fund 100 takes the General Fund role whatever any column holds.
func (f *fundFlows) fundEndpoint(number int) (endpoint, error) {
	if _, ok := f.Labels.FundType(number); !ok {
		return endpoint{}, cmdutil.WithHint(
			fmt.Errorf("fund-flows: fund %d is in no data/funds.yaml entry", number),
			"a fund node's parent is its type, so a fund the registry does not list "+
				"cannot be placed in the hierarchy")
	}
	role := roleFund
	if number == generalFund {
		role = roleGeneralFund
	}
	return endpoint{id: prefixFund + strconv.Itoa(number), tier: tierFund, role: role}, nil
}

func (f *fundFlows) divisionEndpoint(division string) (endpoint, error) {
	if l, ok := f.Labels.DivisionLabel(division); !ok || l == "" {
		return endpoint{}, fmt.Errorf("fund-flows: data/departments.yaml lists no division %q, "+
			"so the row has no division to draw", division)
	}
	return endpoint{id: prefixDept + division, tier: tierDepartment, role: roleDepartment}, nil
}

// objectEndpoint is a tier-5 node, and its id carries the DIVISION.
//
// A bare expenditure/<object> node would need one parent per division.
func (*fundFlows) objectEndpoint(division, category string) endpoint {
	return endpoint{id: prefixExpenditure + division + "/" + category, slug: category,
		tier: tierObjectCategory, role: roleObjectCategory}
}

// fundObjectEndpoint is a tier-5 node of pp.173-183, under the fund that
// spends it: ReachOf draws a node only beneath the one opened, so a bare
// expenditure/<object> shared by every fund would reach no group's window.
func fundObjectEndpoint(fund int, category string) endpoint {
	return endpoint{id: prefixExpenditure + "fund/" + strconv.Itoa(fund) + "/" + category,
		slug: category, tier: tierObjectCategory, role: roleObjectCategory,
		parent: prefixFund + strconv.Itoa(fund)}
}

// addFundFlowNode records a node the first time something touches it, and hangs
// the constraint tier and its disclosure on a fund.
func (f *fundFlows) addFundFlowNode(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	n := Node{ID: e.id, Label: f.label(e), Tier: e.tier, Role: e.role, Parent: e.parent}
	if e.tier == tierFund {
		number, err := strconv.Atoi(e.id[len(prefixFund):])
		if err == nil {
			if t, ok := f.Labels.FundType(number); ok {
				n.Parent = prefixFundGroup + t
			}
			// The node is printed and only the tier is ours, so Derived stays
			// false; the disclosure rides on SourceNote and Rationale.
			if tier := f.Labels.ConstraintTier(number); tier != "" {
				n.ConstraintTier = tier
				n.SourceNote = "data/funds.yaml, our reading of Budget Book pp.258-261"
				n.Rationale = f.Labels.RestrictionNote(number)
			}
		}
	}
	if e.tier == tierDepartment {
		n.Parent = prefixFund + strconv.Itoa(generalFund)
	}
	if e.tier == tierObjectCategory && e.parent == "" {
		// expenditure/<division>/<object> -> dept/<division>, cut at the FIRST
		// slash: a category may contain one, a division cannot.
		rest := e.id[len(prefixExpenditure):]
		if i := strings.Index(rest, "/"); i > 0 {
			n.Parent = prefixDept + rest[:i]
		}
	}
	nodes[e.id] = n
}

// addParents adds the two kinds of node this document parents to and does not
// otherwise build: the fund group above every fund, and the revenue category
// above every line.
//
// A category comes from the hierarchy rather than from its rollup link,
// because a line whose cells cancel draws no rollup.
func (f *fundFlows) addParents(nodes map[string]Node) error {
	type edge struct{ child, parent string }
	var want []edge
	for _, id := range sortedKeys(nodes) {
		if p := nodes[id].Parent; p != "" {
			want = append(want, edge{child: id, parent: p})
		}
	}
	for _, p := range want {
		if _, ok := nodes[p.parent]; ok {
			continue
		}
		switch {
		case strings.HasPrefix(p.parent, prefixFundGroup):
			nodes[p.parent] = Node{ID: p.parent, Label: f.label(endpoint{id: p.parent}),
				Tier: tierFundGroup, Role: roleFundGroup}
		case strings.HasPrefix(p.parent, prefixRevenue):
			slug := p.parent[len(prefixRevenue):]
			nodes[p.parent] = Node{ID: p.parent,
				Label: f.label(endpoint{id: p.parent, slug: slug, tier: tierRevenueSource}),
				Tier:  tierRevenueSource, Role: roleRevenueSource}
		default:
			return fmt.Errorf("fund-flows: node %q is parented to %q, which this document "+
				"does not build and cannot infer -- only a fund group and a revenue "+
				"category are added on demand", p.child, p.parent)
		}
	}
	return nil
}

// label resolves a node's words: a built-in, then the registry, then the id.
func (f *fundFlows) label(e endpoint) string {
	if l, ok := builtinLabels[e.id]; ok {
		return l
	}
	if e.tier == tierFund {
		if n, err := strconv.Atoi(e.id[len(prefixFund):]); err == nil {
			if name, ok := f.Labels.FundName(n); ok && name != "" {
				return name
			}
		}
	}
	if e.tier == tierDepartment {
		if l, ok := f.Labels.DivisionLabel(e.id[len(prefixDept):]); ok && l != "" {
			return l
		}
	}
	if e.slug != "" {
		if l, ok := f.Labels.Label(e.slug); ok && l != "" {
			return l
		}
	}
	return slugLabel(e.id)
}

// nameContraLinks gives every negative link the sentence naming what it is
// printed as a reduction of. It runs after addParents, because that parent may
// be a node no link touches, and as one post-pass so every site that can emit
// a negative is covered.
func nameContraLinks(links []Link, nodes map[string]Node) {
	for i := range links {
		if links[i].ValueCents >= 0 {
			continue
		}
		if up, ok := nodes[nodes[links[i].Source].Parent]; ok {
			links[i].Contra = "printed as a reduction of " + up.Label
			continue
		}
		links[i].Contra = "printed rows netting to a reduction"
	}
}

// sortedRevKeys, sortedRollupKeys, sortedFundRollupKeys and sortedExpKeys are
// total orders over the cell maps, so a node's first touch does not depend on
// map iteration order.
func sortedRevKeys(m map[revKey]*cellSum) []revKey {
	out := make([]revKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.kind != b.kind:
			return a.kind < b.kind
		case a.category != b.category:
			return a.category < b.category
		case a.line != b.line:
			return a.line < b.line
		case a.fundGroup != b.fundGroup:
			return a.fundGroup < b.fundGroup
		default:
			return a.fund < b.fund
		}
	})
	return out
}

func sortedRollupKeys(m map[rollupKey]*lineRollup) []rollupKey {
	out := make([]rollupKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].kind < out[j].kind
	})
	return out
}

func sortedFundRollupKeys(m map[fundRollupKey]*cellSum) []fundRollupKey {
	out := make([]fundRollupKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].fund != out[j].fund {
			return out[i].fund < out[j].fund
		}
		return out[i].kind < out[j].kind
	})
	return out
}

func sortedFundExpKeys(m map[fundExpKey]*cellSum) []fundExpKey {
	out := make([]fundExpKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].fund != out[j].fund {
			return out[i].fund < out[j].fund
		}
		if out[i].category != out[j].category {
			return out[i].category < out[j].category
		}
		return out[i].fundGroup < out[j].fundGroup
	})
	return out
}

func sortedExpKeys(m map[expKey]*cellSum) []expKey {
	out := make([]expKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].division != out[j].division {
			return out[i].division < out[j].division
		}
		return out[i].category < out[j].category
	})
	return out
}

// sortedKeys orders a string-keyed map.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
