package project

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
)

// DepartmentFundingProjection is this document's name and file stem.
const DepartmentFundingProjection = "department-funding"

// DepartmentFundingScope is the schedule this document is of: Budget Book
// pp.85-125's LOWER block, Department Funding Sources.
//
// THE UPPER BLOCK OF THE SAME ELEVEN PAGES IS NOT IN IT, and the two are kept
// apart by what they are printed by rather than by preference. The upper block
// prints what a DIVISION spends under each object heading and carries no fund;
// this one prints, once per DEPARTMENT, which funds pay for it. Adding them
// together would double the city's expenditure -- the two blocks are two
// readings of one figure, which is why each has a ties-to-spine check of its
// own and neither borrows the other's.
const DepartmentFundingScope = "department-funding-sources"

// DepartmentFundingScopes is the schedule set, as [Options.Scopes] holds it.
func DepartmentFundingScopes() []string { return []string{DepartmentFundingScope} }

// departmentFundingCounts is how much of the corpus this document accounts for.
//
// IT IS NOT [FundFlowsCounts] and there is no facts_cited_twice, for the reason
// [departmentSpendingCounts] gives: every printed cell here is one link and one
// link only. A department's rows are not summed into anything the document also
// draws, because the column above a department is the fund the cell is already
// addressed by.
//
// facts = facts_cited + facts_uncited is this document's identity, and every
// uncited fact is a cell the city printed as a dash or a zero.
type departmentFundingCounts struct {
	Facts        int `json:"facts"`
	FactsCited   int `json:"facts_cited"`
	FactsUncited int `json:"facts_uncited"`
	Nodes        int `json:"nodes"`
	Links        int `json:"links"`
}

// departmentFundingMetadata is this document's metadata block. It embeds
// [Envelope] for [departmentSpendingMetadata]'s reason: one schedule, one
// column, and a plural `scopes` key would advertise a second schedule.
type departmentFundingMetadata struct {
	Envelope
	FiscalYear      int                     `json:"fiscal_year"`
	FiscalYearLabel string                  `json:"fiscal_year_label"`
	Basis           string                  `json:"basis"`
	Sources         []Source                `json:"sources"`
	Counts          departmentFundingCounts `json:"counts"`
	Caveats         []Caveat                `json:"caveats"`
}

// DepartmentFundingDocument is the whole published file.
type DepartmentFundingDocument struct {
	SchemaVersion int                       `json:"schema_version"`
	Projection    string                    `json:"projection"`
	Metadata      departmentFundingMetadata `json:"metadata"`
	Nodes         []Node                    `json:"nodes"`
	Links         []Link                    `json:"links"`
}

// departmentFunding draws Budget Book pp.85-125's Department Funding Sources
// block: which funds pay for each of the city's eleven departments.
//
// # The shape
//
//	tier 2  fund-group/<type>     added as a parent, drawn by no link here
//	tier 3  fund/<number>         parent fund-group/<type>
//	          |  one link per printed cell, value the cell
//	tier 4  department/<slug>     parent "" -- a department draws on many funds
//
// IT IS WHAT OPENS THE SIXTY DEAD-END FUNDS. pp.167-170 are the General Fund's
// schedule and decompose that fund alone, so before this document existed every
// other fund the drill-down draws was the end of the chain. These eleven pages
// are the only published schedule that says what any other fund pays for.
//
// THE TIER-3 IDS ARE THE DRILL-DOWN'S OWN, for the reason departmentSpending's
// tier-5 ids are the spine's: a reader arrives here by clicking a fund, and the
// node they clicked is the centre of what they are shown. A second id form for
// the same fund would make that centre a different box wearing the same words.
//
// TIER 4 IS `department/` AND NOT `dept/`, WHICH IS THE ONE NEW ID FORM THIS
// DOCUMENT COINS. `dept/` is tier 4 and holds DIVISIONS -- pp.167-170's row
// groups, and the tier docs/general-fund-drilldown-contract.md is explicit
// about -- while this schedule's axis is the eleven ALL-CAPS departments.
// data/departments.yaml keeps those as two namespaces on purpose, and FIVE
// SLUGS ARE IN BOTH: city-council, city-manager, city-attorney, general-services
// and administrative-services each name a department and a division beneath it.
// So `dept/city-council` would mean the department in this file and the division
// in the drill-down, and an id form is read by cutting at the first slash --
// which is exactly the collision `revenue-line/` was given its own prefix to
// avoid.
//
// A DEPARTMENT IS PARENTLESS AND A FUND IS NOT. The schedule prints one row per
// (department, fund) pair, so a fund folds into its group -- data/funds.yaml
// says which -- while a department is paid for by several funds and has no
// single one to fold into. The client draws a parentless end muted, which is
// correct: a department here is not the property of any one fund group.
//
// A ZERO CELL IS A FACT AND NOT A FLOW, fundFlows' rule and departmentSpending's
// at the same place. The facts survive and are counted in facts_uncited.
type departmentFunding struct {
	// Labels supplies the city's words for a fund number and a department slug.
	// Optional, as the other projections' is: a nil registry degrades to a
	// slug-derived label rather than to no document.
	Labels labels
}

var (
	_ Projection = (*departmentFunding)(nil)
	_ Sliced     = (*departmentFunding)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*departmentFunding) Name() string { return DepartmentFundingProjection }

// Slices is one Options per column the schedule carries, [Sankey.Slices]'s rule.
//
// ALL FOUR PRINTED COLUMNS, for departmentSpending.Slices' two reasons: this is
// the only document that draws these rows at all, so unprojectedScopes' entry
// for them retires only when they are drawn exhaustively, and a reader asking
// which funds paid for Police in FY2024 is asking the question this document
// exists to answer.
//
// THE TWO HISTORICAL COLUMNS TIE TO NOTHING ON THE SPINE, and that is a fact
// about pp.66-67 rather than a weakness here: those pages print no actual and
// no revised column, so funding-sources-tie-to-spine has nothing to compare
// them against and the only figure they reconcile to is each department's own
// printed Total Department Funding Sources. The caveat below says so to a
// reader.
func (*departmentFunding) Slices(facts []fact.Fact, version string) []Options {
	seen := map[Column]bool{}
	for i := range facts {
		if facts[i].Scope == DepartmentFundingScope {
			seen[Column{FiscalYear: facts[i].FiscalYear, Basis: facts[i].Basis}] = true
		}
	}
	cols := make([]Column, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sortColumns(cols)
	out := make([]Options, 0, len(cols))
	for _, c := range cols {
		out = append(out, Options{
			Columns: []Column{c}, Scopes: DepartmentFundingScopes(), Version: version,
		})
	}
	return out
}

// Build is [Projection]'s entry point.
func (d *departmentFunding) Build(facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := d.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, d.Name(), schema.Projection)
}

// Document builds the graph and returns it, so `fisc verify` reads the same
// structure `fisc export` writes rather than re-parsing the JSON.
func (d *departmentFunding) Document(facts []fact.Fact, o Options) (*DepartmentFundingDocument, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("department-funding options: %w", err)
	}
	if !sameScopes(o.Scopes, DepartmentFundingScopes()) {
		return nil, cmdutil.WithHint(
			fmt.Errorf("department-funding: scopes are %q, want %q", o.ScopeList(),
				Options{Scopes: DepartmentFundingScopes()}.ScopeList()),
			"this document is of pp.85-125's lower block alone; the upper block carries no "+
				"fund, and the two together are the city's expenditure twice")
	}
	if len(o.Columns) != 1 {
		return nil, cmdutil.WithHint(
			fmt.Errorf("department-funding: a column is one budget year, got %d", len(o.Columns)),
			"two budget years in one graph add every cell to its own successor")
	}
	col := o.Columns[0]
	selected := selectFacts(facts, o)

	cells, err := netDepartmentFunding(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	links := make([]Link, 0, len(cells))
	cited := map[string]bool{}
	zero := map[string]bool{}
	for _, k := range sortedFundingKeys(cells) {
		c := cells[k]
		if c.cents == 0 {
			// A printed dash is a fact and is not a flow.
			for _, id := range c.factIDs {
				zero[id] = true
			}
			continue
		}
		src, srcErr := d.fundEndpoint(k.fund)
		if srcErr != nil {
			return nil, srcErr
		}
		dst := endpoint{id: prefixDepartment + k.department, tier: tierDepartment,
			role: roleWholeDepartment}
		d.addNode(nodes, src)
		d.addNode(nodes, dst)
		for _, id := range c.factIDs {
			cited[id] = true
		}
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			// THE FUND GROUP AT THE PAYING END IS THE BOUNDARY QUESTION, and
			// these rows can answer it where pp.85-125's upper block cannot:
			// the schedule prints the fund, so an Internal Service Fund paying
			// a department is an internal service charge and says so. The cross-
			// tab beside this one publishes every expenditure ribbon as external
			// and carries a caveat explaining that it has no fund to ask with.
			Kind: boundaryKind(k.fundGroup), FactIDs: c.factIDs,
			Locators: c.locs.sources(),
		})
	}

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
		return nil, err
	}
	if err := d.addFundGroups(nodes); err != nil {
		return nil, err
	}
	out := sortedNodes(nodes)

	// EVERY UNCITED FACT MUST BE A PRINTED ZERO, refused here rather than
	// asserted downstream, for the reason fundFlows gives at the same place: a
	// fact that reached no link for any other reason is money this document
	// dropped, and a document publishing the identity while quietly failing it
	// is worse than one publishing no counts at all.
	uncited := 0
	for i := range selected {
		id := selected[i].ID
		if cited[id] {
			continue
		}
		uncited++
		if !zero[id] {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-funding: fact %s is carried by no link and is not a "+
					"printed zero", id),
				"facts = facts_cited + facts_uncited is this document's published identity, "+
					"and a fact reaching no link for another reason is a fund's contribution "+
					"to a department dropped in silence")
		}
	}

	cavs := departmentFundingCaveats()
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &DepartmentFundingDocument{
		SchemaVersion: SchemaVersion,
		Projection:    d.Name(),
		Metadata: departmentFundingMetadata{
			Envelope: Envelope{
				GeneratedBy: o.Version,
				Scope:       DepartmentFundingScope,
				Currency:    "USD",
				Units:       "cents",
			},
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Sources:         sourcesOf(selected),
			Counts: departmentFundingCounts{
				Facts:        len(selected),
				FactsCited:   len(cited),
				FactsUncited: uncited,
				Nodes:        len(out),
				Links:        len(links),
			},
			Caveats: cavs,
		},
		Nodes: out,
		Links: links,
	}, nil
}

// fundingKey addresses one printed cell: what one fund pays towards one
// department.
//
// THE FUND GROUP RIDES ALONG AND IS NOT PART OF THE ADDRESS. It is the fact's
// own, read off the section header the row sits under, and it decides the link
// kind; the node's PARENT comes from data/funds.yaml instead, so the two
// records stay independent and rule-funds-match-their-headings has something to
// compare. Keying on it as well would let one (fund, department) pair split
// into two links if the two records ever disagreed, which would hide exactly
// the disagreement that check exists to find.
type fundingKey struct {
	fund       int
	department string
}

// fundingCell is a cell key with the fund group the rows carried, kept beside
// the sum rather than inside the key.
type fundingCell struct {
	fundingKey
	fundGroup string
}

// netDepartmentFunding sums the selected facts into the cell map, refusing
// anything it cannot address.
//
// EVERY GUARD IS A REFUSAL AND NOT A SKIP, this package's rule: a fact this
// document cannot place is a mapping defect, and dropping it publishes a
// smaller city with no error anywhere.
func netDepartmentFunding(facts []fact.Fact) (map[fundingCell]*cellSum, error) {
	out := map[fundingCell]*cellSum{}
	groups := map[fundingKey]string{}
	for i := range facts {
		fa := &facts[i]
		if fa.Scope != DepartmentFundingScope {
			return nil, fmt.Errorf("department-funding: fact %s is in scope %q, which this "+
				"document does not select", fa.ID, fa.Scope)
		}
		if fa.Department == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-funding: fact %s carries no department", fa.ID),
				"this document's tier 4 IS the department, and pp.85-125 print one funding "+
					"schedule per department")
		}
		// A FUND IS REQUIRED, WHICH INVERTS departmentSpending'S GUARD ON
		// PURPOSE. That block has no fund axis and refuses a fact carrying one;
		// this block IS the fund axis, and a fact of this scope with no fund is
		// a row of the upper block wearing this one's name -- which would be
		// drawn here as though the page had attributed it to a fund it does not
		// name. An absent fund is refused with an absent group.
		if fa.Fund == nil || fa.FundGroup == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-funding: fact %s (%s) names fund %s and fund group %q",
					fa.ID, fa.Department, fact.FundString(fa.Fund), fa.FundGroup),
				"pp.85-125's lower block prints one row per paying fund, so a fact of this "+
					"scope with no fund has nothing to hang a ribbon from")
		}
		k := fundingKey{fund: *fa.Fund, department: fa.Department}
		// ONE FUND GROUP PER (fund, department) CELL, refused rather than
		// last-wins. The group decides the link's kind, so two rows of one cell
		// disagreeing about it would publish whichever was read last as the
		// classification of both.
		if g, ok := groups[k]; ok && g != fa.FundGroup {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-funding: fund %d under %s is printed under fund group "+
					"%q and %q", *fa.Fund, fa.Department, g, fa.FundGroup),
				"the fund group at the paying end decides whether a ribbon is an internal "+
					"service charge, and a cell with two of them has two answers")
		}
		groups[k] = fa.FundGroup
		add(out, fundingCell{fundingKey: k, fundGroup: fa.FundGroup}, fa)
	}
	return out, nil
}

// fundEndpoint is the paying end of a cell, and its id is THE DRILL-DOWN'S.
//
// THE GENERAL FUND KEEPS ITS OWN ROLE HERE TOO. fund/100 is `general_fund`
// wherever it is drawn, because 100 IS the General Fund and not because a
// particular column decomposes it -- the same claim fundFlows makes, and the
// reason the drill step that opens this document gates on `fund` and leaves
// fund/100 opening into pp.167-170's divisions instead.
func (d *departmentFunding) fundEndpoint(number int) (endpoint, error) {
	role := roleFund
	if number == generalFund {
		role = roleGeneralFund
	}
	e := endpoint{id: prefixFund + strconv.Itoa(number), tier: tierFund, role: role}
	if d.Labels == nil {
		return e, nil
	}
	// A FUND THE REGISTRY DOES NOT KNOW IS REFUSED, not drawn parentless.
	// fact-funds-resolve already guarantees every fact's fund is in
	// data/funds.yaml, so this cannot fire on the committed corpus -- what it
	// prevents is a fund silently losing its group, and with it its colour and
	// its place in the fold, on a corpus where that guarantee has lapsed.
	t, ok := d.Labels.FundType(number)
	if !ok || t == "" {
		return endpoint{}, cmdutil.WithHint(
			fmt.Errorf("department-funding: data/funds.yaml records no type for fund %d", number),
			"a fund's fund group is its parent edge, and a node parented to `fund-group/` is "+
				"parented to nothing")
	}
	e.parent = prefixFundGroup + t
	return e, nil
}

// addNode records a node the first time a link touches it, and hangs the
// constraint tier and its disclosure on a fund, as fundFlows does.
func (d *departmentFunding) addNode(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	n := Node{ID: e.id, Label: d.label(e), Tier: e.tier, Role: e.role, Parent: e.parent}
	if e.tier == tierFund && d.Labels != nil {
		if number, err := strconv.Atoi(e.id[len(prefixFund):]); err == nil {
			// THE NODE IS PUBLISHED AND THE TIER IS OURS, so Derived stays
			// false and the disclosure rides on the two fields beside it.
			if tier := d.Labels.ConstraintTier(number); tier != "" {
				n.ConstraintTier = tier
				n.SourceNote = "data/funds.yaml, our reading of Budget Book pp.258-261"
				n.Rationale = d.Labels.RestrictionNote(number)
			}
		}
	}
	nodes[e.id] = n
}

// addFundGroups adds the tier-2 node above every fund, which this document
// parents to and does not otherwise build.
//
// THE GROUP IS A PARENT AND NOT A COLUMN HERE. No link touches it: the chart
// that opens this document keeps the fund's group from the chart the reader
// came from, and what this file needs the node for is the fold -- a fund folded
// out of a capped column has to have a box to fold into, and the client colours
// a fund by walking to it.
func (d *departmentFunding) addFundGroups(nodes map[string]Node) error {
	for _, id := range sortedKeys(nodes) {
		p := nodes[id].Parent
		if p == "" {
			continue
		}
		if _, ok := nodes[p]; ok {
			continue
		}
		if len(p) <= len(prefixFundGroup) || p[:len(prefixFundGroup)] != prefixFundGroup {
			return fmt.Errorf("department-funding: node %q is parented to %q, which this "+
				"document does not build and cannot infer -- only a fund group is added "+
				"on demand", id, p)
		}
		nodes[p] = Node{ID: p, Label: d.label(endpoint{id: p}), Tier: tierFundGroup,
			Role: roleFundGroup}
	}
	return nil
}

// label resolves a node's words: a built-in first, then the registry, then a
// readable transform of the id -- Sankey.label's order, for its reasons.
func (d *departmentFunding) label(e endpoint) string {
	if l, ok := builtinLabels[e.id]; ok {
		return l
	}
	if d.Labels != nil {
		switch e.tier {
		case tierFund:
			if n, err := strconv.Atoi(e.id[len(prefixFund):]); err == nil {
				if name, ok := d.Labels.FundName(n); ok && name != "" {
					return name
				}
			}
		case tierDepartment:
			if l, ok := d.Labels.DepartmentLabel(e.id[len(prefixDepartment):]); ok && l != "" {
				return l
			}
		}
	}
	return slugLabel(e.id)
}

// departmentFundingCaveats are the things a reader of this file has to be told,
// each a property of the document rather than a hedge about it.
//
// NONE NAMES A NODE, for departmentSpendingCaveats' reason: each is a statement
// about the SCHEDULE -- which tier of departments.yaml it is printed by, what
// its totals are and are not, and which of its columns the spine can check --
// so marking particular marks would be marking every one of them.
func departmentFundingCaveats() []Caveat {
	return []Caveat{
		ConstraintTierCaveat(),
		{
			ID: "a-department-here-is-not-a-division",
			Summary: "These rows are printed once per DEPARTMENT; the divisions drawn " +
				"elsewhere on this site are a finer tier, and five names belong to both.",
			Text: "Budget Book pp.85-125 print two blocks per page and they are keyed on " +
				"different things. The upper block is Expenditures by Category, printed by " +
				"DIVISION; this one is Department Funding Sources, printed once per " +
				"DEPARTMENT. data/departments.yaml keeps those as two tiers because the " +
				"city's own pages do, and five names are in both -- City Council, City " +
				"Manager, City Attorney, General Services and Administrative Services each " +
				"name a department and a division beneath it. A mark here is the whole " +
				"department across every fund; a mark named the same on the General Fund's " +
				"drill-down is one division of it. Do not read one as the other, and do not " +
				"add them.",
			AppliesTo: []string{},
		},
		{
			ID: "a-fund-takes-in-more-than-it-pays-departments",
			Summary: "What a fund pays departments here is not what that fund takes in, " +
				"and the two are printed by different schedules.",
			Text: "The chart that opens this one draws a fund's REVENUE, from Budget Book " +
				"pp.127-140. What leaves the fund here is what pp.85-125 print the city's " +
				"departments drawing on it. Those are two schedules and not one figure read " +
				"twice: the difference is money the city transfers out of the fund and adds " +
				"to its reserves, which pp.66-67 print for the fund GROUP and no published " +
				"schedule attributes to a fund. A fund drawn here taking in more than it " +
				"pays out is that difference and is not a gap in these pages.",
			AppliesTo: []string{},
		},
		{
			ID: "two-of-the-four-columns-tie-to-no-citywide-total",
			Summary: "The FY2023-24 actual and FY2024-25 revised columns reconcile to each " +
				"department's own printed total and to nothing citywide.",
			Text: "Budget Book pp.66-67 print two adopted columns and no actual and no " +
				"revised one, so the citywide check this project runs over these rows " +
				"(cuts-tie-along-the-lattice) can compare only the two adopted years. " +
				"The other two columns reconcile to each department's own printed Total " +
				"Department Funding Sources at build time and to no citywide figure at all. " +
				"Five of the eleven departments miss that printed total by exactly one " +
				"dollar, every one of them in the FY2023-24 actual column and every one " +
				"declared in the mapping as the city's own rounding.",
			AppliesTo: []string{},
		},
	}
}

// sortedFundingKeys is a total order over the cell map, so node creation does
// not depend on map iteration order. Links are re-sorted afterwards, but a
// node's first touch is decided here.
func sortedFundingKeys(m map[fundingCell]*cellSum) []fundingCell {
	out := make([]fundingCell, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.fund != b.fund:
			return a.fund < b.fund
		case a.department != b.department:
			return a.department < b.department
		default:
			return a.fundGroup < b.fundGroup
		}
	})
	return out
}
