package project

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
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
	_ Sliced     = (*departmentFunding)(nil)
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
	selected := SelectFacts(facts, o)

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
		dst := endpoint{id: PrefixDepartment + k.department, role: roleWholeDepartment}
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

	c, uncited := tally(selected, links, len(out))
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
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-funding: fact %s carries no department", fa.ID),
				"this document's tier 4 IS the department, and pp.85-125 print one funding "+
					"schedule per department")
		}
		// A fund is required: this block IS the fund axis, the inverse of
		// departmentSpending's guard.
		if fa.Fund == nil || fa.FundGroup == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-funding: fact %s (%s) names fund %s and fund group %q",
					fa.ID, fa.Department, fact.FundString(fa.Fund), fa.FundGroup),
				"pp.85-125's lower block prints one row per paying fund, so a fact of this "+
					"scope with no fund has nothing to hang a ribbon from")
		}
		k := fundingKey{fund: *fa.Fund, department: fa.Department}
		// One fund group per cell, refused rather than last-wins: the group
		// decides the link's kind.
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

// fundEndpoint is the paying end of a cell, at the drill-down's id and role.
func (d *departmentFunding) fundEndpoint(number int) (endpoint, error) {
	role := roleFund
	if number == generalFund {
		role = roleGeneralFund
	}
	e := endpoint{id: PrefixFund + strconv.Itoa(number), role: role}
	if d.Labels == nil {
		return e, nil
	}
	// A fund the registry does not know is refused, not drawn parentless.
	t, ok := d.Labels.FundType(number)
	if !ok || t == "" {
		return endpoint{}, cmdutil.WithHint(
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
	if _, ok := nodes[e.id]; ok {
		return
	}
	n := Node{ID: e.id, Label: d.label(e), Tier: e.tier(), Role: e.role, Parent: e.parent}
	if e.tier() == tierFund {
		if number, err := strconv.Atoi(e.id[len(PrefixFund):]); err == nil {
			annotateFund(&n, d.Labels, number)
		}
	}
	nodes[e.id] = n
}

// addFundGroups adds the tier-2 node above every fund, which this document
// parents to and does not otherwise build.
//
// No link touches it; it is there for the fold and the fund's colour.
func (d *departmentFunding) addFundGroups(nodes map[string]Node) error {
	for _, id := range sortedKeys(nodes) {
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
			Role: roleFundGroup}
	}
	return nil
}

// label resolves a node's words: a built-in, then the registry, then the id.
func (d *departmentFunding) label(e endpoint) string {
	if l, ok := builtinLabels[e.id]; ok {
		return l
	}
	if d.Labels != nil {
		switch e.tier() {
		case tierFund:
			if n, err := strconv.Atoi(e.id[len(PrefixFund):]); err == nil {
				if name, ok := d.Labels.FundName(n); ok && name != "" {
					return name
				}
			}
		case tierDepartment:
			l, _ := d.Labels.DepartmentLabel(e.id[len(PrefixDepartment):])
			return l
		}
	}
	return slugLabel(e.id)
}

// departmentFundingCaveats are the things a reader of this file has to be told.
// Each is about the whole schedule, so none names a node.
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
			ID: "what-a-fund-pays-departments-is-not-what-it-takes-in",
			Summary: "What a fund pays departments here is not what that fund takes in, " +
				"and the two are printed by different schedules.",
			Text: "What a fund takes in is printed by Budget Book pp.127-140. What leaves " +
				"the fund here is what pp.85-125 print the " +
				"city's departments drawing on it. Those are two schedules and not one " +
				"figure read twice, and either side may be the larger.",
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

// sortedFundingKeys is a total order over the cell map, so a node's first touch
// does not depend on map iteration order.
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
