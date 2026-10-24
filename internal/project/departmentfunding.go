package project

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/schema"
)

// DepartmentFundingProjection is this document's name and file stem.
const DepartmentFundingProjection = "department-funding"

// DepartmentFundingScope is the schedule this document is of: Budget Book
// pp.85-125's LOWER block, Department Funding Sources.
//
// The upper block of the same pages is a separate scope: the two blocks are
// two readings of one figure, and adding them doubles the city's expenditure.
const DepartmentFundingScope = structure.ScopeDepartmentFundingSources

// DepartmentFundingScopes is the schedule set, as [Options.Scopes] holds it.
func DepartmentFundingScopes() []string { return []string{DepartmentFundingScope} }

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
// It is the only published schedule saying what a fund other than the General
// Fund pays for. The tier-3 ids are the drill-down's own, because a reader
// arrives here by clicking a fund. Tier 4 is `department/`, not `dept/`
// (divisions), because five slugs name both. A department is parentless: it
// is paid for by several funds. A zero cell draws no link and stays in
// facts_uncited.
type departmentFunding struct {
	// Labels is optional: a nil registry degrades to a slug-derived label.
	Labels labels
}

var (
	_ Projection = (*departmentFunding)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*departmentFunding) Name() string { return DepartmentFundingProjection }

// Slices is one Options per column the schedule carries, all four printed
// columns. The two historical ones tie to nothing on the spine, which prints
// no actual or revised column; a caveat says so.
func (*departmentFunding) Slices(facts []fact.Fact, version string) []Options {
	return columnsCarrying(facts, DepartmentFundingScopes(), DepartmentFundingScopes(), nil, version)
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
func (d *departmentFunding) Document(facts []fact.Fact, o Options) (*Document, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("department-funding options: %w", err)
	}
	if !sameScopes(o.Scopes, DepartmentFundingScopes()) {
		return nil, hint.With(
			fmt.Errorf("department-funding: scopes are %q, want %q", o.ScopeList(),
				Options{Scopes: DepartmentFundingScopes()}.ScopeList()),
			"this document is of pp.85-125's lower block alone; the upper block carries no "+
				"fund, and the two together are the city's expenditure twice")
	}
	if len(o.Columns) != 1 {
		return nil, hint.With(
			fmt.Errorf("department-funding: a column is one budget year, got %d", len(o.Columns)),
			"two budget years in one graph add every cell to its own successor")
	}
	col := o.Columns[0]
	selected, err := SelectFacts(facts, o)
	if err != nil {
		return nil, fmt.Errorf("department-funding: %w", err)
	}

	cells, err := netDepartmentFunding(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	links := make([]Link, 0, len(cells))
	for _, k := range sortedFundingKeys(cells) {
		c := cells[k]
		if c.cents == 0 {
			// A printed dash is a fact and is not a flow.
			continue
		}
		src, srcErr := d.fundEndpoint(k.fund)
		if srcErr != nil {
			return nil, srcErr
		}
		dst := endpoint{id: PrefixDepartment + k.department, role: RoleWholeDepartment}
		if d.Labels != nil {
			if l, ok := d.Labels.DepartmentLabel(k.department); !ok || l == "" {
				return nil, fmt.Errorf("department-funding: data/departments.yaml lists no "+
					"department %q, so the row has no department to draw", k.department)
			}
		}
		d.addNode(nodes, src)
		d.addNode(nodes, dst)
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			// These rows print the fund, so the paying fund group can answer
			// the boundary question the upper block cannot.
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

	c, uncited := tally(selected, links, out)
	if err := refuseUncited(d.Name(), uncited, nil); err != nil {
		return nil, err
	}

	cavs := departmentFundingCaveats()
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &Document{
		SchemaVersion: SchemaVersion,
		Projection:    d.Name(),
		Metadata: Metadata{
			GeneratedBy:     o.Version,
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Scopes:          DepartmentFundingScopes(),
			Currency:        "USD",
			Units:           "cents",
			Sources:         SourcesOf(selected),
			Counts:          c,
			Caveats:         cavs,
		},
		Nodes: out,
		Links: links,
	}, nil
}

// fundingKey addresses one printed cell: what one fund pays towards one
// department.
//
// The fact's fund group decides the link kind but is not in the address, so
// a disagreement with data/funds.yaml cannot split one cell into two links.
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
			return nil, hint.With(
				fmt.Errorf("department-funding: fact %s carries no department", fa.ID),
				"this document's tier 4 IS the department, and pp.85-125 print one funding "+
					"schedule per department")
		}
		// A fund is required: this block IS the fund axis, the inverse of
		// departmentSpending's guard.
		if fa.Fund == nil || fa.FundGroup == "" {
			return nil, hint.With(
				fmt.Errorf("department-funding: fact %s (%s) names fund %s and fund group %q",
					fa.ID, fa.Department, fact.FundString(fa.Fund), fa.FundGroup),
				"pp.85-125's lower block prints one row per paying fund, so a fact of this "+
					"scope with no fund has nothing to hang a ribbon from")
		}
		k := fundingKey{fund: *fa.Fund, department: fa.Department}
		// One fund group per cell, refused rather than last-wins: the group
		// decides the link's kind.
		if g, ok := groups[k]; ok && g != fa.FundGroup {
			return nil, hint.With(
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

// fundEndpoint is the paying end of a cell, at the drill-down's id and role.
func (d *departmentFunding) fundEndpoint(number int) (endpoint, error) {
	role := RoleFund
	if number == generalFund {
		role = RoleGeneralFund
	}
	e := endpoint{id: PrefixFund + strconv.Itoa(number), role: role}
	if d.Labels == nil {
		return e, nil
	}
	// A fund the registry does not know is refused, not drawn parentless.
	t, ok := d.Labels.FundType(number)
	if !ok || t == "" {
		return endpoint{}, hint.With(
			fmt.Errorf("department-funding: data/funds.yaml records no type for fund %d", number),
			"a fund's fund group is its parent edge, and a node parented to `fund-group/` is "+
				"parented to nothing")
	}
	e.parent = PrefixFundGroup + t
	return e, nil
}

// addNode records a node the first time a link touches it, and hangs the
// constraint tier on a fund through annotateFund.
func (d *departmentFunding) addNode(nodes map[string]Node, e endpoint) {
	addFundNode(nodes, e, d.label(e), d.Labels)
}

// addFundGroups adds the tier-2 node above every fund, which this document
// parents to and does not otherwise build.
//
// No link touches it; it is there for the fold and the fund's colour.
func (d *departmentFunding) addFundGroups(nodes map[string]Node) error {
	for _, id := range slices.Sorted(maps.Keys(nodes)) {
		p := nodes[id].Parent
		if p == "" {
			continue
		}
		if _, ok := nodes[p]; ok {
			continue
		}
		if len(p) <= len(PrefixFundGroup) || p[:len(PrefixFundGroup)] != PrefixFundGroup {
			return fmt.Errorf("department-funding: node %q is parented to %q, which this "+
				"document does not build and cannot infer -- only a fund group is added "+
				"on demand", id, p)
		}
		nodes[p] = Node{ID: p, Label: d.label(endpoint{id: p}), Tier: endpoint{id: p}.tier(),
			Role: RoleFundGroup}
	}
	return nil
}

// label is nodeLabel over this document's registry.
func (d *departmentFunding) label(e endpoint) string { return nodeLabel(d.Labels, e.id, e.slug) }

// departmentFundingCaveats are the things a reader of this file has to be told.
// Each is about the whole schedule, so none names a node.
func departmentFundingCaveats() []Caveat {
	return []Caveat{
		ConstraintTierCaveat(),
		{
			ID:      "a-department-here-is-not-a-division",
			Theme:   ThemeSameMoneyTwice,
			Summary: "A department here is not the division of the same name elsewhere; don't add them.",
			Text: "Budget Book pp.85-125 print two blocks per page: spending by DIVISION above, and " +
				"each DEPARTMENT's funding sources below. Five names are both a department and a " +
				"division beneath it: City Council, City Manager, City Attorney, General Services " +
				"and Administrative Services. A mark here is the whole department across every " +
				"fund; a mark of the same name on the General Fund's breakdown is one division of it.",
			AppliesTo: []string{},
		},
		{
			ID:      "what-a-fund-pays-departments-is-not-what-it-takes-in",
			Theme:   ThemeSameMoneyTwice,
			Summary: "What a fund pays departments is not what it takes in; two different pages print them.",
			Text: "What a fund takes in is printed on Budget Book pp.127-140; what departments draw " +
				"from it is printed on pp.85-125. They are two pages, not one figure read twice, " +
				"and either may be the larger.",
			AppliesTo: []string{},
		},
		{
			ID:      "two-of-the-four-columns-tie-to-no-citywide-total",
			Theme:   ThemeKindsOfYear,
			Summary: "The FY2023-24 actual and FY2024-25 revised columns are checked only department by department.",
			Text: "Budget Book pp.66-67 print citywide totals for the two adopted years only, so the " +
				"actual and revised columns here are checked against each department's own printed " +
				"total and nothing citywide. Five of the eleven departments miss that total by " +
				"exactly one dollar, all in the FY2023-24 actual column; the mapping declares each " +
				"as the city's own rounding.",
			AppliesTo: []string{},
		},
	}
}

// sortedFundingKeys is a total order over the cell map, so a node's first touch
// does not depend on map iteration order.
func sortedFundingKeys(m map[fundingCell]*cellSum) []fundingCell {
	return slices.SortedFunc(maps.Keys(m), func(a, b fundingCell) int {
		return cmp.Or(cmp.Compare(a.fund, b.fund), cmp.Compare(a.department, b.department), cmp.Compare(a.fundGroup, b.fundGroup))
	})
}
