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
)

// FundFlowsProjection is this document's name and file stem.
const FundFlowsProjection = "fund-flows"

// The two schedules this document is of.
//
// ScopeRevenueByFund is the same string TrendsScope holds, and is named again
// rather than reused under that name because the string identifies a SCHEDULE
// and TrendsScope identifies it by its first consumer. Two documents drawing one
// schedule is the ordinary case; a constant that reads as "the trends one" makes
// the second look like a borrowing.
const (
	ScopeRevenueByFund           = TrendsScope
	scopeExpenditureByDepartment = "expenditure-by-department"
)

// FundFlowsScopes is the schedule set, in the order a reader meets the money:
// revenue first, then what it is spent on.
func FundFlowsScopes() []string {
	return []string{ScopeRevenueByFund, scopeExpenditureByDepartment}
}

// generalFund is the only fund pp.167-170 decompose, and the tier-3 node the
// department axis hangs from. p0167.txt:3 prints "General Fund Expenditures by
// Major Category".
const generalFund = 100

// constraintTierCaveat is the disclosure docs/sankey-contract.md requires of any
// document publishing a constraint tier.
//
// A CONSTANT SO THE CAVEAT AND THE CHECK CANNOT DRIFT. The contract says a
// document whose nodes carry constraint tiers must carry this sentence, and a
// check that looked for prose written twice would be asserting that two authors
// agreed rather than that the document says the thing.
const constraintTierCaveat = "A fund's constraint tier is OUR reading of the " +
	"Description of Funds narrative (Budget Book pp. 258-261), not something the city " +
	"printed. Each fund node carries the note it was read from. \"unknown\" means the " +
	"document does not establish a restriction; it is a classification, not a gap."

// ConstraintTierCaveat is the disclosure, exported so internal/check compares a
// document against THIS DECLARATION rather than against prose written twice.
// Two authors agreeing that a caveat says roughly the right thing is not the
// same claim as the document carrying the sentence the contract requires.
//
// THE CHECK MAKES THREE COMPARISONS, NOT ONE, and it is worth knowing which
// before editing any field here: it finds the caveat by ConstraintTierCaveatID,
// then compares Text and Summary separately, reporting each as its own finding.
// Changing the id retires a document's disclosure; changing either sentence
// without changing the document's is a finding naming which of the two drifted.
func ConstraintTierCaveat() Caveat {
	return Caveat{
		ID:      ConstraintTierCaveatID,
		Summary: "A fund's constraint tier is our reading of the Description of Funds narrative, not a figure the city printed.",
		Text:    constraintTierCaveat,
		// DOCUMENT-WIDE, and not the 61 fund nodes it is about. Listing them
		// would be true and useless: every fund in the column carries a tier,
		// so marking all of them marks none of them.
		AppliesTo: []string{},
	}
}

// ConstraintTierCaveatID is the anchor, separate from the sentence, so
// internal/check can say "this document does not carry the disclosure" and
// "this document's disclosure is not the one the contract requires" as two
// different findings.
const ConstraintTierCaveatID = "constraint-tier-is-our-reading"

// MultiScopeEnvelope is [Envelope] for a document of more than one schedule.
//
// A SIBLING RATHER THAN A WIDENING OF Envelope, and the reason is bytes. Envelope
// publishes `scope` as one string, and TestSharedMetadataTagsHaveNotDrifted
// requires every one of its tags to appear in the spine's Metadata too; turning
// that key plural would move it in sankey.json and revenue-trends.json, neither
// of which is of two schedules and neither of which should pay for this document.
// Embedding Envelope unchanged is worse still -- it would publish `"scope": ""`,
// which is the absent-is-not-zero mistake at document level.
//
// The other three keys are identical, in the same order, deliberately: a reader
// comparing two documents' metadata should find the envelope where they expect
// it, and a test pins the two blocks against each other.
type MultiScopeEnvelope struct {
	GeneratedBy string   `json:"generated_by"`
	Scopes      []string `json:"scopes"`
	Currency    string   `json:"currency"`
	Units       string   `json:"units"`
}

// FundFlowsCounts is how much of the corpus this document accounts for.
//
// IT IS NOT [Counts], and the difference is the whole shape of the document.
// Counts documents an identity -- facts = facts_cited + zero cells + stocks --
// that assumes each fact is behind at most one link. Here a fund->department
// link is the SUM of that department's object rows, so every expenditure fact is
// behind two links, and the reader needs both the identity and the overlap
// stated as numbers rather than left to be discovered from a diff.
type FundFlowsCounts struct {
	// Facts is every fact matching the columns and either scope.
	Facts int `json:"facts"`
	// FactsCited is how many DISTINCT facts a link carries. Distinct, not the
	// sum of the links' citation lengths -- see FactsCitedTwice.
	FactsCited int `json:"facts_cited"`
	// FactsUncited is the facts no link carries, and every one of them is in a
	// cell that netted to zero. A printed dash is a fact and is not a flow.
	//
	// Facts = FactsCited + FactsUncited is this document's identity. It holds
	// without a stock term because neither schedule prints a stock row, and it
	// is stated as "uncited" rather than "in a zero cell" because those are NOT
	// the same set here: an expenditure fact whose own object cell is zero is
	// still carried by the fund-to-department link that sums the division,
	// since that link's value and its citation are taken over every cell
	// including the zero ones. Counting zero CELLS would overstate the gap by
	// exactly those facts -- measured, 2 in FY2024 and 5 in FY2026 -- and the
	// identity would not close.
	FactsUncited int `json:"facts_uncited"`
	// FactsCitedTwice is how many facts are behind two links: a revenue fact
	// cited by its own line->fund flow AND by the rollup of that line into its
	// category, and an expenditure fact cited by its own department->object link
	// AND by the fund->department link that sums it.
	//
	// PUBLISHED AS A NUMBER BECAUSE `links` IS NOT A PARTITION OF `facts_cited`
	// HERE, which the spine's shape would lead a reader to assume. Summing every
	// link's value_cents counts both sides' money twice over; folding within one
	// tier pair does not.
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
// fiscal_year and basis ARE here, unlike TrendsMetadata's, because this document
// is of one column: a flow diagram of two budgets is not a chart of anything.
// headline is NOT here, and its absence is the point -- see [FundFlows].
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
// IT PUBLISHES NO HEADLINE, and that is a decision rather than an omission. A
// headline is a property of a SINGLE-GRAIN document: the moment one file holds
// the same money at two grains, "the total" is ambiguous, and no key can
// disambiguate it because the ambiguity is in the accumulation. Giving this
// document one is not the cheap way out either -- a revenue-side drill-down has
// transfers IN and no transfers out, so transfer_residual_cents would publish
// -21,045,597 and headline-transfer-residual would report it GREEN, the figure
// being the sum of the facts and the facts being one leg (fisc-xau).
//
// THE SPINE IS NOT RETIRED BY THIS. sankey.json remains the citywide overview
// and the only document that publishes a headline; this is the drill-down. The
// two are reconciled per cell at zero tolerance by the <kind>-detail-ties-to-spine
// family, which reads Subject.Facts and needs no graph at all (fisc-l25).
//
// # The shape
//
//	tier 0  revenue/<category>, transfers/in
//	tier 1  revenue-line/<line>   parent = revenue/<category>
//	          |  one link per (line, kind) back INTO the category, the sum of
//	          |  that row's own cells
//	          |  one link per netted (kind, line, fund) cell, and one per
//	          |  (transfers/in, fund) cell straight from tier 0
//	tier 3  fund/<n>            parent = fund-group/<type> from data/funds.yaml
//	          |  one link per (fund, division) -- the SUM over its object rows
//	tier 4  dept/<division>     parent = fund/100
//	          |  one link per netted (department, category) cell
//	tier 5  expenditure/<division>/<object>   parent = dept/<division>
//
// MIXED GRAIN IS UNAVOIDABLE. pp.167-170 are General Fund only, so six of the
// seven fund groups' revenue bottoms out at tier 3 with no expenditure side at
// all. fisc-gkv established this and it is why "one graph, tiers 0-5" was not
// achievable as fisc-gxa.2 first wrote it.
//
// WHY THE MIDDLE LINK IS EMITTED rather than left to the client to fold. The
// obvious design is finest-grain links only -- fisc-gxa.2's own words -- and it
// does not survive a mixed-grain document: fold a tier-4-to-5 link up to tier 3
// and it becomes fund/100 -> fund/100, a self-loop, while a client rendering
// tiers 0/3/4 would find every department node with no inbound link and a value
// of zero. A sankey needs a link at each adjacent tier pair it can be drawn at.
//
// THE (1,0) ROLLUP IS THAT SAME ARGUMENT ON THE REVENUE SIDE, one rung earlier.
// A category is the SOURCE of every other link it touches, so a view that puts
// it between its own lines and the funds has nothing flowing into it and draws
// it at zero. The rollup is what a chart drawing tiers 1/0/2 folds; every tier
// set this repository draws today drops it, which is the lane's own proof rather
// than its hope.
//
// The cost of both is that a fact is cited twice, which
// FundFlowsCounts.FactsCitedTwice publishes as a number.
type fundFlows struct {
	// Labels supplies the city's words and the fund hierarchy. Unlike Sankey's,
	// it is NOT optional: a nil Labels cannot answer FundType, and a fund node
	// with no parent is the fold with nothing in it.
	Labels labels
}

var (
	_ Projection = (*fundFlows)(nil)
	_ Sliced     = (*fundFlows)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*fundFlows) Name() string { return FundFlowsProjection }

// Slices is one Options per column, [Sankey.Slices]'s rule and not
// [Trends.Slices]'s: a flow diagram of two budgets is not a chart of anything.
//
// A COLUMN IS DECLARED ONLY IF BOTH SCHEDULES CARRY IT. pp.127-140 and
// pp.167-170 both print four columns on the committed corpus, so this drops
// nothing today -- and the day one schedule gains a column the other does not,
// a document over it would draw a whole revenue side against an empty
// expenditure side and look like a city that stopped spending.
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
	return encode(doc, f.Name())
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
			"this document is of two schedules and both are required; one alone draws "+
				"a revenue side with no spending or a department axis with no income")
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

	rev, exp, err := f.netFundFlows(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	var links []Link
	cited := map[string]bool{}
	zero := map[string]bool{}
	twice := map[string]bool{}

	// Tier 0 -> 3, one link per revenue cell, and tier 1 -> 0, one link per
	// (line, kind) carrying that line's own sum.
	perLine := map[rollupKey]*lineRollup{}
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
		// A TRANSFER IS NOT A LINE OF ANYTHING. Its source is the tier-0 flow
		// endpoint, which is nobody's child, so there is nothing to roll it
		// into.
		if src.tier != tierRevenueLine {
			continue
		}
		// ACCUMULATED BELOW THE ZERO CELLS AND NOT ABOVE THEM, which is the one
		// place the tier-3-to-4 rule below is deliberately not copied: that
		// link's division total is taken over every object cell including the
		// dashes, and on THIS side a printed dash is a fact and not a flow,
		// uncited by construction and counted in facts_uncited. Rolling them up
		// would cite them and move that number, while the value would not
		// change by a cent -- so the citation is the whole of what is at stake,
		// which is exactly what link-values-tie-to-facts compares.
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
		// A LINE WHOSE CELLS CANCEL DRAWS NOTHING, the same rule as a cell that
		// nets to zero: p127 prints ERAF and the RPTTF reduction as negatives,
		// so a line summing to zero across its funds is arithmetic and not a
		// flow. Its cells are cited by their own links already, so nothing goes
		// uncited here.
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
		// The division's total is over EVERY cell including the zero ones, and
		// so is its citation, so the two sides of link-values-tie-to-facts stay
		// of one set: a zero cell adds 0 to the value and its ids to the list.
		d.cents += c.cents
		d.factIDs = append(d.factIDs, c.factIDs...)
		d.locs.merge(&c.locs)

		if c.cents == 0 {
			for _, id := range c.factIDs {
				zero[id] = true
			}
			continue
		}
		src := f.divisionEndpoint(k.division)
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
		dst := f.divisionEndpoint(division)
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

	// Every parent must resolve to a node in THIS document, so a fund's fund
	// group is added even though no link touches it. sankey.go's "nodes exist
	// because links do" is deliberately broken here: these exist because the
	// hierarchy does, and aggregation is what they are for.
	if err := f.addParents(nodes); err != nil {
		return nil, err
	}

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}
	out := sortedNodes(nodes)

	// EVERY UNCITED FACT MUST BE A PRINTED ZERO, and it is refused here rather
	// than asserted downstream. A fact that reached no link for any other
	// reason is money this document silently dropped, which is the failure the
	// counts identity exists to make visible -- and a projection that published
	// the identity while quietly failing it would be worse than one that
	// published no counts at all.
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

	// Validated against this document's OWN nodes, for the reason spelled out
	// in ValidateCaveats: an AppliesTo naming a node the document does not carry
	// marks nothing, and marking nothing is indistinguishable from having
	// nothing to mark.
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

// fundFlowsCaveats are the things a reader of this file has to be told, each of
// which is a property of the document rather than a hedge about it. All but the
// last are unconditional; the comment on that one says what its condition is
// and why the condition is narrower than it looks.
func fundFlowsCaveats(twice int, nodes []Node) []Caveat {
	truncated := len(truncatedGroups(nodes))
	// THE THIRD CAVEAT IS CONDITIONAL ON THE SENTENCE BEING TRUE, which is a
	// narrower test than the one it first got. It opens "ONLY the General Fund
	// has a spending side", and `len(spendingSides) > 0` asserts something else
	// -- that SOMETHING is decomposed. A column decomposing capital and not
	// general satisfies that and makes the sentence false, and the applies_to
	// built beside it would then list fund-group/general among the groups the
	// sentence counts as stopping: the exact contradiction this lane removed,
	// re-entered through the guard.
	//
	// Latent either way -- all four published columns decompose the General
	// Fund and nothing else -- and a caveat describing a distinction the
	// document does not draw is worse than a missing one, because it reads as
	// though the distinction was checked.
	sides := spendingSides(nodes)
	decomposed := len(sides) == 1 && sides[prefixFundGroup+"general"]
	out := []Caveat{
		ConstraintTierCaveat(),
		revenueSchedulePublishedTwiceCaveat(),
		{
			ID:      "mixed-grain-double-counts",
			Summary: "This document holds the same money at two grains, so summing every link double-counts.",
			// TWO OVERLAPS AND NOT ONE. This named the expenditure side alone
			// while the count beneath it had already grown the revenue side's:
			// a revenue row is behind its own flow into a fund AND the rollup
			// that adds its row back into the category it is printed under.
			// Naming one overlap under a number that counts two invites the
			// reader to look for the difference on the spending side, where it
			// is not.
			Text: fmt.Sprintf("This document holds the same money at more than one grain, so summing "+
				"every link double-counts: %d fact(s) are behind two links. A revenue row is "+
				"behind both its own flow into a fund and the rollup of its line into the "+
				"category it is printed under; an expenditure row is behind both its "+
				"department's object rows and the fund-to-department link that totals them. "+
				"Fold within one tier pair, never across the whole graph. It publishes no "+
				"headline for this reason.",
				twice),
			AppliesTo: []string{},
		},
		{
			ID: "only-the-general-fund-is-decomposed",
			// THE COUNT IS THE DOCUMENT'S, NOT A LITERAL. This said "the other
			// six fund groups" in both sentences, and six is true of exactly
			// one of the four published columns: fund-flows-2024-actual carries
			// a seventh group, permanent, so six of its seven stop short. The
			// other three -- including FY2025-26, the one the site draws --
			// carry six groups, so FIVE stop. The page shipped a figure that
			// was wrong on the column a reader was looking at.
			// PLURALISED, because the count is the document's and a document
			// can have one. "The other 1 groups' revenue ends at their funds"
			// would have gone into the caveat, the caveats page and every
			// tooltip badge on a column with a single truncated group.
			Summary: fmt.Sprintf("Only the General Fund has a spending side; the other %s "+
				"revenue ends at their funds.", plural(truncated, "group")),
			Text: fmt.Sprintf("Only the General Fund has a spending side. Budget Book "+
				"pp.167-170 decompose that fund alone, so the other %s revenue ends at "+
				"their funds -- the money is not missing, the schedule that would break it "+
				"down is not published.", plural(truncated, "fund group")),
			// THE GROUPS THAT STOP, AND THE ONE THAT DOES NOT. Marking fund/100
			// alone was half the sentence: this caveat is about the six fund
			// groups whose money ends at their funds, and the badge landed on
			// the one group that continues. A reader wondering why five columns
			// stop short is pointing at one of the five, and found nothing.
			//
			// Both halves are named, so each rung of the chart marks what it
			// draws: the opened General Fund draws fund/100, and the spine
			// the six groups.
			//
			// The other two caveats here stay document-wide and correctly carry
			// an empty list: a constraint tier is on every fund in the column,
			// so marking all 61 marks none, and the mixed-grain warning is
			// about summing the graph rather than about any node in it.
			AppliesTo: appliesToTruncatedGroups(nodes),
		},
	}
	if !decomposed {
		// The conditional caveat is the last entry, and this bound says so
		// without counting the ones before it.
		return out[:len(out)-1]
	}
	return out
}

// plural writes "1 group's" and "5 groups'".
//
// A HELPER BECAUSE THE COUNT IS THE DOCUMENT'S. Every number in these caveats
// used to be a literal and every literal was written for one column; the moment
// they became computed, a column with exactly one truncated group would have
// published "the other 1 groups' revenue ends at their funds" wherever the
// caveat is shown. site/app.js learned the same lesson in paintCounts when a
// drilled division drew one ribbon.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s's", n, noun)
	}
	return fmt.Sprintf("%d %ss'", n, noun)
}

// spendingSides is the fund groups this document decomposes: the ones with a
// division beneath one of their funds.
//
// ONE COMPUTATION FOR BOTH THE COUNT AND THE MARKS, which is what the caveat
// needs. It said "the other 5 groups" while marking six of them, general
// included -- a list that contradicted the sentence it sat under, because the
// count excluded the exception and the marks did not.
func spendingSides(nodes []Node) map[string]bool {
	parent := map[string]string{}
	for _, n := range nodes {
		if n.Tier == tierFund {
			parent[n.ID] = n.Parent
		}
	}
	out := map[string]bool{}
	for _, n := range nodes {
		if n.Tier == tierDepartment {
			if g := parent[n.Parent]; g != "" {
				out[g] = true
			}
		}
	}
	return out
}

// truncatedGroups is the fund groups whose money ends at their funds, sorted.
//
// COUNTED RATHER THAN WRITTEN DOWN, because the published columns do not agree:
// fund-flows-2024-actual carries a seventh fund group, permanent, so six of its
// seven stop short, while the other three carry six and five stop. A literal is
// right about one column and wrong on the page the site actually draws -- which
// is how "the other six fund groups" shipped on a chart where five stop.
func truncatedGroups(nodes []Node) []string {
	decomposed := spendingSides(nodes)
	out := []string{}
	for _, n := range nodes {
		if strings.HasPrefix(n.ID, prefixFundGroup) && !decomposed[n.ID] {
			out = append(out, n.ID)
		}
	}
	slices.Sort(out)
	return out
}

// appliesToTruncatedGroups names the marks the only-the-general-fund caveat is
// about: every group whose money ends at its funds, and the fund that is the
// exception to them. How many that is depends on the column and is not written
// down anywhere -- see truncatedGroups.
//
// THE EXCEPTION IS NAMED AS A FUND, NOT AS A GROUP, and that is what keeps the
// list and the sentence in step. fund-group/general was in here, so a caveat
// counting the groups that stop printed an About list with the exception among
// them. fund/100 marks the same thing without being counted -- and on a chart
// where that fund is folded away, caveatsFor resolves it to fund-group/general
// anyway, so the exception is still marked where a reader can see it.
//
// DERIVED FROM THE DOCUMENT, and a hard-coded list is what taught me why.
// ValidateCaveats refuses an id the document does not carry, and the smaller
// fixtures in fundflows_test.go build documents with one or two groups, so a
// fixed list of six failed every one of them at build time.
func appliesToTruncatedGroups(nodes []Node) []string {
	out := truncatedGroups(nodes)
	for _, n := range nodes {
		if n.ID == prefixFund+"100" {
			out = append(out, n.ID)
			break
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
//
// THE LINE IS THE GRAIN AND THE CATEGORY IS ITS PARENT. pp.127-140 print
// thirteen property-tax rows into the General Fund and the key used to hold
// only their category, so all thirteen netted into one cell and the document
// drew the page's own decomposition as a single ribbon. line is a
// data/taxonomy.yaml line slug (`taxes/property/eraf`), resolved from the row
// label the fact carries, and it is EMPTY for a transfer: pp.131-140 print
// "Transfers In" as a row of the schedule, but its node is the flow endpoint
// every transfer arrives from and that endpoint is not a line of anything.
type revKey struct {
	kind      mapping.Kind
	category  string
	line      string
	fundGroup string
	fund      int
}

// expKey addresses an expenditure cell: what a division spends, by object.
//
// THE FUND IS NOT IN THE KEY and the guard in netFundFlows is what makes that
// safe: pp.167-170 are one fund, so a second one appearing would put two cells
// on one (division, object) address. It is refused by name rather than absorbed.
type expKey struct {
	division string
	category string
}

// rollupKey addresses a revenue line's rollup into its category: one printed row,
// under one link kind.
//
// PER KIND AND NOT PER LINE, because a kind is a claim about the money and not a
// label on the ribbon. pp.127-140 print one row reaching the five Internal
// Service Funds as an internal service charge and the rest of the city as
// external revenue, and a single rollup would have to publish one of those two
// answers for both -- naming internal service charges as money crossing the
// city's boundary, which is the false claim link-kinds-match-their-facts was
// written for. Two links on one pair is what checkDistinctLinks and
// site/app.js's foldDocument both allow exactly when the kinds differ.
type rollupKey struct {
	line string
	kind LinkKind
}

// lineRollup is one line's own sum and the endpoint that carries its category.
//
// THE ENDPOINT RIDES ALONG FOR revenueEndpoint's REASON: the target is the
// line's parent, and a parent cannot be cut out of the child's slug because a
// category slug is itself one or two segments.
type lineRollup struct {
	cellSum
	src endpoint
}

// netFundFlows sums the selected facts into the two cell maps, refusing anything
// it cannot address.
//
// EVERY GUARD IS A REFUSAL AND NOT A SKIP, this package's own netCells' rule: a
// fact this document cannot place is a mapping defect, and dropping it publishes
// a smaller city with no error anywhere.
func (f *fundFlows) netFundFlows(facts []fact.Fact) (map[revKey]*cellSum, map[expKey]*cellSum, error) {
	rev := map[revKey]*cellSum{}
	exp := map[expKey]*cellSum{}
	expFund := 0
	for i := range facts {
		fa := &facts[i]
		switch fa.Scope {
		case ScopeRevenueByFund:
			if fa.Category == "" {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s carries no category", fa.ID),
					"a revenue node is a category, so a fact without one has no source end")
			}
			if fa.Fund == 0 {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) names no fund", fa.ID, fa.Category),
					"this document's tier 3 IS the fund; 0 is the no-fund sentinel and "+
						"would collapse every fund group into one box")
			}
			if fa.FundGroup == "" {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) names no fund group", fa.ID, fa.Category),
					"the fund group decides whether a flow crosses the city's boundary")
			}
			line, lineErr := f.revenueLine(fa)
			if lineErr != nil {
				return nil, nil, lineErr
			}
			add(rev, revKey{fa.Kind, fa.Category, line, fa.FundGroup, fa.Fund}, fa)
		case scopeExpenditureByDepartment:
			if fa.Department == "" {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s carries no department", fa.ID),
					"this document's tier 4 IS the department")
			}
			if fa.Category == "" {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) carries no category", fa.ID, fa.Department),
					"an object-category node is a category")
			}
			// THE FUND IS REQUIRED, for the reason the revenue side's is:
			// 0 is the no-fund sentinel, and an expenditure side of fund-0
			// facts would build a document whose every department node is
			// parented to fund/100 and whose every division link is sourced
			// from it -- attributing the whole of the spending to the General
			// Fund on no evidence at all. Nothing downstream would see it: the
			// amounts are unchanged, so expenditure-detail-ties-to-spine still
			// ties, and fact-funds-resolve only examines facts that DO name a
			// fund. Refused here, which also lets expFund use 0 as an honest
			// "not yet seen".
			if fa.Fund == 0 {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: fact %s (%s) names no fund", fa.ID, fa.Department),
					"this document parents every department to the fund that pays it, and "+
						"0 is the no-fund sentinel rather than a fund")
			}
			// ONE FUND ON THE WHOLE EXPENDITURE SIDE. The tier-5 id carries the
			// division and not the fund, so two funds spending on one
			// division/object would land two cells on one link and
			// checkDistinctLinks would report it as a duplicate rather than as
			// what it is.
			if expFund == 0 {
				expFund = fa.Fund
			}
			if fa.Fund != expFund {
				return nil, nil, cmdutil.WithHint(
					fmt.Errorf("fund-flows: the expenditure side names funds %d and %d",
						expFund, fa.Fund),
					"pp.167-170 are a General Fund schedule and this document's department "+
						"axis assumes it; pp.72-75 give a per-fund expenses column and are "+
						"the schedule a wider key would be for")
			}
			add(exp, expKey{fa.Department, fa.Category}, fa)
		default:
			return nil, nil, fmt.Errorf("fund-flows: fact %s is in scope %q, which this "+
				"document does not select", fa.ID, fa.Scope)
		}
	}
	if expFund != 0 && expFund != generalFund {
		return nil, nil, fmt.Errorf("fund-flows: the expenditure side is fund %d, want %d",
			expFund, generalFund)
	}
	return rev, exp, nil
}

// revenueLine is the data/taxonomy.yaml line a revenue fact's printed row names,
// and "" for a transfer, whose node is a flow endpoint rather than a line.
//
// A ROW THE REGISTRY CANNOT PLACE IS REFUSED BY NAME, netFundFlows' rule and
// not an exception to it. Falling back to the category would be the tempting
// half-measure and it is the worst of the three outcomes: the money would still
// tie to the spine, every check over amounts would stay green, and the page
// would draw a category whose children are some of its rows and whose remainder
// is silently folded into a ribbon that looks like one more row. Either the
// document decomposes a category or it does not.
//
// AMBIGUITY IS REFUSED SEPARATELY FROM ABSENCE, because the two are different
// defects in data/taxonomy.yaml -- a row nobody declared, against two lines
// claiming one printed spelling -- and a reader fixing one would look in the
// wrong place for the other.
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
// THE KIND IS DECIDED BY THE FACT'S KIND FIRST AND BY THE FUND GROUP SECOND, and
// getting that order wrong published a false claim rather than an ugly one.
// boundaryKind alone answers "does this cross the city's boundary" from the fund
// group, which is the right question for REVENUE and the wrong one for a
// TRANSFER: a transfer is money moving between two city funds and crosses no
// boundary whatever group receives it. Measured on the shipped document before
// this was fixed: 8 transfers/in links carrying $21,045,597 published as
// "external", the same money sankey.json publishes as internal_transfer, and
// internal_transfer absent from the drill-down entirely. A client summing
// external to get external revenue over-counted by exactly that.
func revenueLinkKind(k revKey) LinkKind {
	if k.kind == mapping.KindTransferIn {
		return KindInternalTransfer
	}
	return boundaryKind(k.fundGroup)
}

// revenueEndpoint is the source end of a revenue flow: a tier-1 line for a
// revenue row, and the tier-0 flow endpoint for a transfer.
//
// THE CELL'S LINE IS GUARANTEED BY netFundFlows, which refuses a revenue fact
// it cannot resolve, so the empty-line arm below guards a second kind admitted
// to the revenue map rather than any fact the corpus holds -- the same thing
// the `default` arm guards, one step earlier.
//
// THE PARENT RIDES ON THE ENDPOINT because nothing downstream can recover it.
// A line's id is `revenue-line/<line slug>` and the slug carries its category
// as a prefix, so the parent LOOKS derivable by cutting at the last slash -- and
// is not: a category slug is itself one or two segments (`taxes/property`
// against `licenses-and-permits`), so the cut lands inside the category for
// half the taxonomy. The cell knows which category the row was netted under;
// this carries that answer rather than guessing it back out of a string.
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
func (f *fundFlows) fundEndpoint(number int) (endpoint, error) {
	if _, ok := f.Labels.FundType(number); !ok {
		return endpoint{}, cmdutil.WithHint(
			fmt.Errorf("fund-flows: fund %d is in no data/funds.yaml entry", number),
			"a fund node's parent is its type, so a fund the registry does not list "+
				"cannot be placed in the hierarchy")
	}
	return endpoint{id: prefixFund + strconv.Itoa(number), tier: tierFund, role: roleFund}, nil
}

func (*fundFlows) divisionEndpoint(division string) endpoint {
	return endpoint{id: prefixDept + division, tier: tierDepartment, role: roleDepartment}
}

// objectEndpoint is a tier-5 node, and its id carries the DIVISION.
//
// Node.Parent is one string and `wages-and-benefits` is spent by 22 divisions,
// so a bare expenditure/wages-and-benefits node cannot have 22 parents. The
// prefix stays `expenditure/` because that is the id form the contract's table
// gives tier 5; slugLabel reads the last segment, so the label falls out right.
func (*fundFlows) objectEndpoint(division, category string) endpoint {
	return endpoint{id: prefixExpenditure + division + "/" + category, slug: category,
		tier: tierObjectCategory, role: roleObjectCategory}
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
			// THE NODE IS PUBLISHED AND THE TIER IS OURS, so Derived stays
			// false -- setting it would claim the city did not print the fund --
			// and the disclosure rides on the two fields beside it.
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
	if e.tier == tierObjectCategory {
		// expenditure/<division>/<object> -> dept/<division>
		// THE DIVISION IS THE FIRST SEGMENT, so this cuts at the FIRST slash
		// and not the last. A division slug cannot contain one, but a taxonomy
		// category routinely can -- taxes/property, transfers/in,
		// fund-balance/ending -- so the moment an object category gains a slash
		// a last-index cut would land inside the category and yield a parent
		// like dept/police/services-and-supplies, which resolves to nothing.
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
// A CATEGORY IS NOT BUILT WHERE IT IS DRAWN INTO. The (1,0) rollup names it as a
// target and builds nothing, because a line whose cells cancel draws no rollup
// and would leave its category with no box for the lines to fold into -- so the
// node has to come from the hierarchy rather than from a link, which is what
// this does. p127's ERAF and RPTTF reduction are the rows that make that case
// reachable rather than theoretical.
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

// label resolves a node's words: a built-in first, then the registry, then a
// readable transform of the id -- Sankey.label's order, for its reasons.
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

// sortedRevKeys, sortedRollupKeys and sortedExpKeys are total orders over the cell
// maps, so node creation and the caveats' arithmetic do not depend on map
// iteration order. Links are re-sorted afterwards, but a node's first-touch, a
// division's accumulation and a line's are all done here.
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

// sortedKeys orders a string-keyed map. Same shape as internal/export's, and
// spelled again rather than exported from there: this package does not import
// the packager and must not start.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
