package project

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
)

// DepartmentSpendingProjection is this document's name and file stem.
const DepartmentSpendingProjection = "department-spending"

// DepartmentSpendingScope is the schedule this document is of: Budget Book
// pp.85-125's UPPER block, Expenditures by Category.
//
// The lower block of the same pages is keyed on DEPARTMENT, not division, and
// is [departmentFunding]'s; adding the two blocks doubles the city's spending.
const DepartmentSpendingScope = "departmentwide-expenditures"

// DepartmentSpendingScopes is the schedule set, as [Options.Scopes] holds it.
func DepartmentSpendingScopes() []string { return []string{DepartmentSpendingScope} }

// departmentSpendingCounts is how much of the corpus this document accounts for.
//
// Every printed cell is one link, so there is no facts_cited_twice.
// facts = facts_cited + facts_uncited, every uncited fact a printed zero.
type departmentSpendingCounts struct {
	Facts        int `json:"facts"`
	FactsCited   int `json:"facts_cited"`
	FactsUncited int `json:"facts_uncited"`
	Nodes        int `json:"nodes"`
	Links        int `json:"links"`
}

// departmentSpendingMetadata is this document's metadata block, of one
// schedule and one column.
type departmentSpendingMetadata struct {
	Envelope
	FiscalYear      int                      `json:"fiscal_year"`
	FiscalYearLabel string                   `json:"fiscal_year_label"`
	Basis           string                   `json:"basis"`
	Sources         []Source                 `json:"sources"`
	Counts          departmentSpendingCounts `json:"counts"`
	Caveats         []Caveat                 `json:"caveats"`
}

// DepartmentSpendingDocument is the whole published file.
type DepartmentSpendingDocument struct {
	SchemaVersion int                        `json:"schema_version"`
	Projection    string                     `json:"projection"`
	Metadata      departmentSpendingMetadata `json:"metadata"`
	Nodes         []Node                     `json:"nodes"`
	Links         []Link                     `json:"links"`
}

// departmentSpending draws Budget Book pp.85-125's Expenditures by Category
// block: which divisions spend the city's money under each object category,
// citywide and across every fund.
//
// # The shape
//
//	tier 5  expenditure/<object>   parent ""
//	          |  one link per printed cell, PARTITION, value the cell
//	tier 4  dept/<division>        parent ""
//
// The tier-5 ids are the spine's own, because a reader opens this document by
// clicking an object category on sankey.json. Both ends are parentless: these
// rows carry no fund. The links descend 5 -> 4 and carry Link.Partition, since
// one printed matrix read either way round is not money moving. A zero cell
// draws no link and stays in facts_uncited.
type departmentSpending struct {
	// Labels is optional: a nil registry degrades to a slug-derived label.
	Labels labels
}

var (
	_ Projection = (*departmentSpending)(nil)
	_ Sliced     = (*departmentSpending)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*departmentSpending) Name() string { return DepartmentSpendingProjection }

// Slices is one Options per column the schedule carries: all four printed
// columns, not only the two the spine publishes.
func (*departmentSpending) Slices(facts []fact.Fact, version string) []Options {
	seen := map[Column]bool{}
	for i := range facts {
		if facts[i].Scope == DepartmentSpendingScope {
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
			Columns: []Column{c}, Scopes: DepartmentSpendingScopes(), Version: version,
		})
	}
	return out
}

// Build is [Projection]'s entry point.
func (d *departmentSpending) Build(facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := d.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, d.Name(), schema.Projection)
}

// Document builds the cross-tab and returns it, so `fisc verify` reads the same
// structure `fisc export` writes rather than re-parsing the JSON.
func (d *departmentSpending) Document(facts []fact.Fact, o Options) (*DepartmentSpendingDocument, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("department-spending options: %w", err)
	}
	if !sameScopes(o.Scopes, DepartmentSpendingScopes()) {
		return nil, cmdutil.WithHint(
			fmt.Errorf("department-spending: scopes are %q, want %q", o.ScopeList(),
				Options{Scopes: DepartmentSpendingScopes()}.ScopeList()),
			"this document is of pp.85-125's upper block alone; the lower block carries no "+
				"department, and the two together are the city's expenditure twice")
	}
	if len(o.Columns) != 1 {
		return nil, cmdutil.WithHint(
			fmt.Errorf("department-spending: a cross-tab is of one column, got %d", len(o.Columns)),
			"two budget years in one matrix add every cell to its own successor")
	}
	col := o.Columns[0]
	selected := selectFacts(facts, o)

	cells, err := netDepartmentSpending(selected)
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	links := make([]Link, 0, len(cells))
	cited := map[string]bool{}
	zero := map[string]bool{}
	for _, k := range sortedSpendKeys(cells) {
		c := cells[k]
		if c.cents == 0 {
			// A printed dash is a fact and is not a flow.
			for _, id := range c.factIDs {
				zero[id] = true
			}
			continue
		}
		src, srcErr := spendingObjectEndpoint(k)
		if srcErr != nil {
			return nil, srcErr
		}
		dst := endpoint{id: prefixDept + k.division, tier: tierDepartment, role: roleDepartment}
		if d.Labels != nil {
			if l, ok := d.Labels.DivisionLabel(k.division); !ok || l == "" {
				return nil, fmt.Errorf("department-spending: data/departments.yaml lists no "+
					"division %q, so the row has no division to draw", k.division)
			}
		}
		d.addNode(nodes, src)
		d.addNode(nodes, dst)
		for _, id := range c.factIDs {
			cited[id] = true
		}
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			Kind: spendingLinkKind(k.kind), FactIDs: c.factIDs,
			Locators: c.locs.sources(),
			// Every link here is a cell of one matrix read along its second axis.
			Partition: true,
		})
	}

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
				fmt.Errorf("department-spending: fact %s is carried by no link and is not a "+
					"printed zero", id),
				"facts = facts_cited + facts_uncited is this document's published identity, "+
					"and a fact reaching no link for another reason is a division's spending "+
					"dropped in silence")
		}
	}

	cavs := departmentSpendingCaveats()
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}

	return &DepartmentSpendingDocument{
		SchemaVersion: SchemaVersion,
		Projection:    d.Name(),
		Metadata: departmentSpendingMetadata{
			Envelope: Envelope{
				GeneratedBy: o.Version,
				Scope:       DepartmentSpendingScope,
				Currency:    "USD",
				Units:       "cents",
			},
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Sources:         sourcesOf(selected),
			Counts: departmentSpendingCounts{
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

// spendKey addresses one printed cell: what a division spends under an object
// heading.
//
// There is no fund in it because there is no fund on the page. The kind is in
// it for p124's Transfers Out row under Maintenance, the one row that is not an
// expenditure.
type spendKey struct {
	kind     mapping.Kind
	division string
	category string
}

// netDepartmentSpending sums the selected facts into the cell map, refusing
// anything it cannot address.
func netDepartmentSpending(facts []fact.Fact) (map[spendKey]*cellSum, error) {
	out := map[spendKey]*cellSum{}
	for i := range facts {
		fa := &facts[i]
		if fa.Scope != DepartmentSpendingScope {
			return nil, fmt.Errorf("department-spending: fact %s is in scope %q, which this "+
				"document does not select", fa.ID, fa.Scope)
		}
		if fa.Department == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-spending: fact %s carries no department", fa.ID),
				"this document's tier 4 IS the division, and pp.85-125's upper block prints "+
					"every row under one")
		}
		if fa.Category == "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-spending: fact %s (%s) carries no category", fa.ID,
					fa.Department),
				"this document's tier 5 IS the object category the row is printed under")
		}
		// A fund is refused rather than required: these rows have no fund
		// axis, so a fact carrying one belongs to another schedule.
		if fa.Fund != nil || fa.FundGroup != "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-spending: fact %s (%s) names fund %s and fund group %q",
					fa.ID, fa.Department, fact.FundString(fa.Fund), fa.FundGroup),
				"pp.85-125's upper block prints what a division spends whatever pays for it, "+
					"so a fact of this scope with a fund is a row of another schedule under "+
					"this one's name")
		}
		add(out, spendKey{fa.Kind, fa.Department, fa.Category}, fa)
	}
	return out, nil
}

// spendingObjectEndpoint is the tier-5 end of a cell, at the spine's id:
// `expenditure/<slug>`, or `transfers/out` for the Transfers Out row.
func spendingObjectEndpoint(k spendKey) (endpoint, error) {
	switch k.kind {
	case mapping.KindExpenditure:
		return endpoint{id: prefixExpenditure + k.category, slug: k.category,
			tier: tierObjectCategory, role: roleObjectCategory}, nil
	case mapping.KindTransferOut:
		return endpoint{id: k.category, slug: k.category,
			tier: tierObjectCategory, role: roleTransferOut}, nil
	default:
		return endpoint{}, cmdutil.WithHint(
			fmt.Errorf("department-spending: kind %q has no object end in this document", k.kind),
			"pp.85-125's upper block prints expenditure rows and one Transfers Out row; a "+
				"third kind means the schedule or the mapping changed under this projection")
	}
}

// spendingLinkKind classifies a cell.
//
// A transfer is internal; everything else is `external`, because these rows
// carry no fund group to ask boundaryKind with. A caveat says so.
func spendingLinkKind(k mapping.Kind) LinkKind {
	if k == mapping.KindTransferOut {
		return KindInternalTransfer
	}
	return KindExternal
}

// addNode records a node the first time a link touches it.
func (d *departmentSpending) addNode(nodes map[string]Node, e endpoint) {
	if _, ok := nodes[e.id]; ok {
		return
	}
	nodes[e.id] = Node{ID: e.id, Label: d.label(e), Tier: e.tier, Role: e.role}
}

// label resolves a node's words: a built-in, then the registry, then the id.
func (d *departmentSpending) label(e endpoint) string {
	if l, ok := builtinLabels[e.id]; ok {
		return l
	}
	if d.Labels != nil {
		if e.tier == tierDepartment {
			if l, ok := d.Labels.DivisionLabel(e.id[len(prefixDept):]); ok && l != "" {
				return l
			}
		}
		if e.slug != "" {
			if l, ok := d.Labels.Label(e.slug); ok && l != "" {
				return l
			}
		}
	}
	return slugLabel(e.id)
}

// departmentSpendingCaveats are the things a reader of this file has to be told.
// Each is about the whole schedule, so none names a node.
func departmentSpendingCaveats() []Caveat {
	return []Caveat{
		{
			ID: "no-fund-axis-on-these-pages",
			Summary: "These rows say what a division spends, not what pays for it: the " +
				"schedule has no fund and no fund group.",
			Text: "Budget Book pp.85-125's upper block prints what each division spends " +
				"under each object heading whatever fund pays for it, so no row here carries " +
				"a fund or a fund group. That is the page rather than a gap -- the fund " +
				"breakdown is the LOWER block of the same eleven pages, Department Funding " +
				"Sources, which this site draws as its own chart. The two are not the same " +
				"grain and are not to be joined: that block prints one row per DEPARTMENT " +
				"and this one per DIVISION. A division drawn here therefore belongs to no " +
				"fund group, and a chart that colours by group leaves it unshaded.",
			AppliesTo: []string{},
		},
		{
			ID: "the-ribbons-are-a-cross-tab",
			Summary: "Each ribbon is a cell of one printed table read along a second axis, " +
				"not money moving in the direction drawn.",
			Text: "These pages print one matrix: divisions down, object categories across. " +
				"A chart can read it either way round and neither reading is money moving, " +
				"so every ribbon here carries the partition flag and the chart says so on " +
				"each one. Summing a whole column is the printed total of that column and " +
				"nothing more; there is no flow to follow from one end of this document to " +
				"the other.",
			AppliesTo: []string{},
		},
		{
			ID: "the-boundary-is-not-classified-here",
			Summary: "Every expenditure ribbon is published as external because these rows " +
				"name no fund group to classify it by.",
			Text: "Elsewhere in this project a flow is external or an internal service " +
				"charge according to the fund group at its far end, read off the section " +
				"header the row sits under. These rows have no such header, so that question " +
				"cannot be asked of them and every expenditure ribbon is published as " +
				"external. Internal Service Fund spending is INSIDE these figures rather " +
				"than beside them. Do not add a " +
				"figure from this document to an external total taken from another.",
			AppliesTo: []string{},
		},
	}
}

// sortedSpendKeys is a total order over the cell map, so a node's first touch
// does not depend on map iteration order.
func sortedSpendKeys(m map[spendKey]*cellSum) []spendKey {
	out := make([]spendKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.kind != b.kind:
			return a.kind < b.kind
		case a.division != b.division:
			return a.division < b.division
		default:
			return a.category < b.category
		}
	})
	return out
}

// sortColumns orders a column list the way every Slices implementation does:
// fiscal year, then basis.
func sortColumns(cols []Column) {
	sort.Slice(cols, func(i, j int) bool {
		if cols[i].FiscalYear != cols[j].FiscalYear {
			return cols[i].FiscalYear < cols[j].FiscalYear
		}
		return cols[i].Basis < cols[j].Basis
	})
}
