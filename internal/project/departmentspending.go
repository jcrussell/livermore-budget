package project

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// DepartmentSpendingProjection is this document's name and file stem.
const DepartmentSpendingProjection = "department-spending"

// DepartmentSpendingScope is the schedule this document is of: Budget Book
// pp.85-125's UPPER block, Expenditures by Category.
//
// THE LOWER BLOCK OF THE SAME ELEVEN PAGES IS NOT IN IT, and that is a fact
// about the facts rather than a preference. Department Funding Sources rows
// carry no department at all -- they are {label, category, fund, fund_group} --
// so a document keyed on a division cannot place one of them. Adding the two
// blocks together would also double the city's expenditure, which is what the
// two ties-to-spine checks over these pages each reconcile separately.
const DepartmentSpendingScope = "departmentwide-expenditures"

// DepartmentSpendingScopes is the schedule set, as [Options.Scopes] holds it.
func DepartmentSpendingScopes() []string { return []string{DepartmentSpendingScope} }

// departmentSpendingCounts is how much of the corpus this document accounts for.
//
// IT IS NOT [FundFlowsCounts] and there is no facts_cited_twice, which is the
// difference between the two documents rather than an omission. The drill-down
// holds the same money at two grains -- a fund-to-division link sums the
// division's object rows -- so a fact there is behind two links and the overlap
// has to be published as a number. Here every printed cell is one link and one
// link only: there is no summing link above the cell, because the column above
// a division is the object category the cell is already addressed by.
//
// facts = facts_cited + facts_uncited is this document's identity, and every
// uncited fact is a cell the city printed as a dash or a zero. The projection
// REFUSES any other uncited fact rather than publishing a smaller city, so the
// identity holds by construction and the numbers state it for a reader.
type departmentSpendingCounts struct {
	Facts        int `json:"facts"`
	FactsCited   int `json:"facts_cited"`
	FactsUncited int `json:"facts_uncited"`
	Nodes        int `json:"nodes"`
	Links        int `json:"links"`
}

// departmentSpendingMetadata is this document's metadata block.
//
// It embeds [Envelope] rather than [MultiScopeEnvelope]: this document is of one
// schedule, and a plural `scopes` key holding a single entry would advertise a
// second schedule it does not draw. fiscal_year and basis are here because it is
// of one column; a cross-tab of two budget years adds every cell to its
// successor and still looks like a table.
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
// THE TIER-5 IDS ARE THE SPINE'S OWN, deliberately. A reader opens this document
// by clicking an object category on sankey.json, and the node they clicked is
// the centre of what they are shown; a second id form for the same category
// would make the centre a different box wearing the same words.
//
// BOTH ENDS ARE PARENTLESS, AND THAT IS THE PAGES RATHER THAN AN OMISSION. These
// rows print what a division spends whatever pays for it -- they carry no fund
// and no fund group, which is why departmentwide-ties-to-spine can tie them only
// to the spine's object categories summed over all six groups. So there is no
// fund for a division to hang from here, and no group for a category. The client
// draws a ribbon with a group-less end muted, which is correct and is what the
// no-fund-axis caveat below says in words.
//
// THE LINKS RUN 5 -> 4, WHICH DESCENDS, AND THEY SAY SO. pp.85-125 print one
// matrix, divisions down and object categories across; a chart can read it
// either way round and neither reading is money moving. Link.Partition is that
// claim on the wire, and node-tiers-are-declared admits a descending link only
// where it is carried.
//
// A ZERO CELL IS A FACT AND NOT A FLOW, fundflows' rule at the same place: 28
// divisions print a Wages & Benefits row and two of them print a dash in FY2026,
// so a link for every cell would draw ribbons the schedule prints as nothing.
// The facts survive and are counted in facts_uncited.
type departmentSpending struct {
	// Labels supplies the city's words for a division slug and an object
	// category. It is optional, as Sankey's is: a nil registry degrades to a
	// slug-derived label rather than to no document.
	Labels labels
}

var (
	_ Projection = (*departmentSpending)(nil)
	_ Sliced     = (*departmentSpending)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*departmentSpending) Name() string { return DepartmentSpendingProjection }

// Slices is one Options per column the schedule carries, [Sankey.Slices]'s rule:
// a cross-tab of two budget years adds every cell to its successor.
//
// ALL FOUR PRINTED COLUMNS, not the two the spine publishes. pp.85-125 print
// FY2023-24 Actual, FY2024-25 Revised and both adopted years, and two reasons
// make drawing all four the right answer. It is the only document that draws
// these pages at all, and unprojectedScopes' declaration for them retires only
// when they are drawn EXHAUSTIVELY -- draw two of four and staleDeclarations'
// partial arm fires and demands the reason be rewritten. And a reader asking
// which divisions spent the money in FY2024 is asking the question this document
// exists to answer; that the spine prints no actual column is a fact about
// pp.66-67 rather than about pp.85-125.
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
	return encode(doc, d.Name())
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
		d.addNode(nodes, src)
		d.addNode(nodes, dst)
		for _, id := range c.factIDs {
			cited[id] = true
		}
		links = append(links, Link{
			Source: src.id, Target: dst.id, ValueCents: c.cents,
			Kind: spendingLinkKind(k.kind), FactIDs: c.factIDs,
			Locators: c.locs.sources(),
			// THE FLAG IS SET HERE AND NOWHERE ELSE IN THIS PACKAGE. Every
			// link this document draws is a cell of one printed matrix read
			// along its second axis, so the claim is the document's rather
			// than any link's.
			Partition: true,
		})
	}

	sortLinks(links)
	if err := checkDistinctLinks(links); err != nil {
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
// THERE IS NO FUND IN IT BECAUSE THERE IS NO FUND ON THE PAGE. Every fact of
// this scope carries fund 0 and fund_group "", which is the upper block's whole
// point -- it prints what a department spends whatever pays for it. netCells'
// usual guard, "a fact naming no fund cannot be placed", is therefore the wrong
// guard here and its opposite is the right one: see netDepartmentSpending.
//
// The KIND is in the key so that the one row on these eleven pages which is not
// an expenditure keeps its own address. p124 prints a Transfers Out row under
// Maintenance, and a cell mixing it with that division's expenditure would be
// one link carrying two kinds -- which link-kinds-match-their-facts reports as a
// cell key that lost an axis.
type spendKey struct {
	kind     mapping.Kind
	division string
	category string
}

// netDepartmentSpending sums the selected facts into the cell map, refusing
// anything it cannot address.
//
// EVERY GUARD IS A REFUSAL AND NOT A SKIP, this package's rule: a fact this
// document cannot place is a mapping defect, and dropping it publishes a smaller
// city with no error anywhere.
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
		// A FUND IS REFUSED RATHER THAN REQUIRED, which inverts the drill-down's
		// guard on purpose. Every fact of this scope carries fund 0 and
		// fund_group "" because these rows have no fund axis; one that carried a
		// fund would be a fact of the LOWER block, or of pp.167-170, wearing
		// this scope -- and it would be drawn here as though the page had
		// printed it with no fund, which is the claim the no-fund-axis caveat
		// makes to every reader of the file.
		if fa.Fund != 0 || fa.FundGroup != "" {
			return nil, cmdutil.WithHint(
				fmt.Errorf("department-spending: fact %s (%s) names fund %d and fund group %q",
					fa.ID, fa.Department, fa.Fund, fa.FundGroup),
				"pp.85-125's upper block prints what a division spends whatever pays for it, "+
					"so a fact of this scope with a fund is a row of another schedule under "+
					"this one's name")
		}
		add(out, spendKey{fa.Kind, fa.Department, fa.Category}, fa)
	}
	return out, nil
}

// spendingObjectEndpoint is the tier-5 end of a cell, and its id is THE SPINE'S.
//
// An expenditure row's category becomes `expenditure/<slug>`; the Transfers Out
// row's becomes the flow endpoint `transfers/out`, which is the id and the tier
// the spine gives it and which endpointTiers pins at 5. Deriving one form for
// both would put a second box on the page for a node the reader may have clicked
// to get here.
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
// A TRANSFER IS NEVER EXTERNAL, which is the one classification these facts can
// support: money moving between two city funds crosses no boundary. Everything
// else is `external` and the boundary caveat below says why that is the weakest
// of this document's claims -- these rows carry no fund group, so the question
// boundaryKind answers cannot be asked of them, and the citywide internal
// service charge is inside these figures rather than beside them.
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

// label resolves a node's words: a built-in first, then the registry, then a
// readable transform of the id -- Sankey.label's order, for its reasons.
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

// departmentSpendingCaveats are the things a reader of this file has to be told,
// each a property of the document rather than a hedge about it.
//
// ALL THREE ARE UNCONDITIONAL AND NONE NAMES A NODE. Each is a statement about
// the SCHEDULE -- it has no fund axis, its ribbons are a cross-tab, its kinds
// cannot classify the boundary -- so marking particular marks would be marking
// every one of them, which marks none. ValidateCaveats is given the drawn node
// set anyway, and an empty applies_to means document-wide rather than
// not-yet-filled-in.
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
				"Sources, which no document draws yet. A division drawn here therefore " +
				"belongs to no fund group, and a chart that colours by group leaves it " +
				"unshaded.",
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
				"than beside them: the citywide spine puts it at 25,077,367 of 254,095,412 " +
				"in FY2025-26 and 26,544,515 of 252,854,896 in FY2026-27. Do not add a " +
				"figure from this document to an external total taken from another.",
			AppliesTo: []string{},
		},
	}
}

// sortedSpendKeys is a total order over the cell map, so node creation does not
// depend on map iteration order. Links are re-sorted afterwards, but a node's
// first touch is decided here.
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
