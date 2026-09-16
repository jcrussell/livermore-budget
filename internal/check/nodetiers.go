package check

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// hierarchyTiers is the tier each node ID FORM sits at, from
// docs/sankey-contract.md's table.
//
// Keyed on the id prefix, because that is what the contract's "id form" column
// declares: `revenue/<slug>` is tier 0 and `fund/<number>` is tier 3, and the
// prefix is the whole of what makes an id one form rather than another. A node
// whose prefix is not here is a coined id form, which the contract forbids in as
// many words ("Do not coin new ones").
//
// TIER 1 IS A LINE AND WAS NEVER A CONSTRAINT TIER. The contract once gave it as
// a `constraint/<tier>` node between the revenue source and the fund group, and
// that layer could not exist: a constraint tier is a property of a FUND and the
// fund groups do not partition along it (data/funds.yaml has capital = 3
// committed + 43 restricted-by-law), so its parent edge had no single answer.
// A `revenue-line/` node's does -- the category the row is printed under -- which
// is the whole difference between an empty number and a layer.
//
// `revenue-line/` IS A SEPARATE PREFIX FROM `revenue/` BECAUSE OF THIS TABLE'S
// OWN KEY. declaredTier cuts an id at its FIRST slash, and a category slug is
// already one or two segments (`revenue/taxes/property` is tier 0), so a line
// nested under `revenue/` would be read as its own parent's form.
//
// `transfer-from/` AND `transfer-to/` ARE FORMS AND NOT ENDPOINTS, which is why
// they are here and not in endpointTiers beside `transfers/in`. That table is
// keyed by NAME because it holds five nodes; these two are one node per fund
// and could not be enumerated. They are the two ends of a printed movement
// rather than the funds themselves -- Budget Book p76 prints money moving
// between the city's own funds, and `fund/<a>` to `fund/<b>` runs tier 3 to
// tier 3, which the ordering claim below refuses and d3-sankey cannot lay out
// with both ends taking one column index. Their tiers are the spine's own for
// the same end of the chart: a payer's end at 2 with the fund groups, a
// receiver's at 5 with the object categories.
var hierarchyTiers = map[string]int{
	"revenue":       0,
	"revenue-line":  1,
	"fund-group":    2,
	"transfer-from": 2,
	"fund":          3,
	"dept":          4,
	// `department/` IS A SEPARATE FORM FROM `dept/`, AT THE SAME TIER, and the
	// table's own key is why. `dept/` holds pp.167-170's divisions and
	// `department/` holds pp.85-125's ALL-CAPS departments; data/departments.yaml
	// keeps those as two namespaces because the pages do, and five slugs are in
	// both -- city-council, city-manager, city-attorney, general-services and
	// administrative-services. declaredTier cuts an id at its FIRST slash, so
	// one prefix over both tiers would make `dept/city-council` mean the
	// department in one document and the division in another.
	"department":  4,
	"expenditure": 5,
	"transfer-to": 5,
}

// endpointTiers are the five flow endpoints, which sit OUTSIDE the hierarchy and
// therefore carry a tier by declaration rather than by form.
//
// They are ends of a flow rather than levels of a fold: no node is parented to
// one and none aggregates. Their tiers exist only so the diagram lays out left
// to right, which is why they are pinned by name here instead of by prefix --
// `fund-balance/draw` is tier 0 and `fund-balance/contribution` is tier 5, one
// prefix and two tiers, so a prefix rule could not express them and a reader
// deriving one from the other would be wrong half the time.
// SPELLED OUT RATHER THAN IMPORTED, which is this package's habit where the
// point is an independent second reading (fundGroupInternalService says the
// same about itself). internal/project exports two of these five as
// constants; taking them from there and the other three from the contract would
// make half the table agree with the producer by construction and the other half
// by assertion, which is the worst of both.
var endpointTiers = map[string]int{
	"transfers/in":                  0,
	"transfers/out":                 5,
	"fund-balance/draw":             0,
	"fund-balance/contribution":     5,
	"fund-balance/reserve-increase": 5,
}

// nodeTiersAreDeclared asserts every node sits at the tier its id form declares
// and every link runs from a coarser tier to a finer one.
//
// THE FIELD WAS PUBLISHED AND READ BY NOTHING. node.tier ships on every node
// of the two spine documents and no check in this package looked at it:
// aggregation-invariance reads only Parent, constraint-tier-vocabulary only
// ConstraintTier, graph-acyclic only links. That was nearly harmless while the
// spine emitted three tier values from three constants and a test pinned eight
// node ids by hand -- and stops being harmless the moment a document populates
// the hierarchy, because every tier value it writes would land unasserted.
//
// WHAT IT ACTUALLY WITNESSES, stated so the PASS line is not read as more than
// it is. It compares the contract's table against the code by something other
// than a reader: a node whose id says fund-group and whose tier says 3 is a
// finding here and is invisible everywhere else. It does NOT witness that the
// tier table is the right model of the city's finances -- no check can -- and it
// does not witness node.parent, which is the fold and belongs to
// aggregation-invariance.
//
// THE ORDERING CLAIM IS NOT ABOUT THE HIERARCHY. "Source tier is strictly less
// than target tier" holds for the flow endpoints too, which is why they are
// given tiers at all, and it is what makes d3 draw the diagram left to right
// with no backward ribbon. A link from a finer tier to a coarser one would
// render as a flow running against every other flow on the page, which is a
// picture that reads as a defect in the data rather than in the layout.
//
// TWO EXCEPTIONS, EACH NARROW BY CONSTRUCTION, AND NEITHER IS A TOLERANCE. A
// column order is a declaration a view makes and need not be ascending -- a
// window puts the clicked node in the middle -- so a ribbon runs backwards only
// in a view that drew it backwards. What has to be established per link is that
// the link is not a FLOW at all, and there are exactly two ways to establish it:
//
//   - A ROLLUP. The target is the SOURCE'S OWN PARENT, the edge the client
//     already folds along, so the "flow" is a node being added into the box it
//     is part of.
//   - A DECLARED PARTITION. [project.Link.Partition] says the projection read
//     one printed matrix along its second axis, so neither end is upstream of
//     the other and the direction drawn is the chart's choice. It is the
//     PROJECTION's claim and is on the wire: nothing in a graph distinguishes a
//     cross-tab from a chain by looking, so a check that inferred it would be
//     inferring what a published table means.
//
// ANY OTHER DESCENDING LINK IS STILL REFUSED, which is what keeps these
// exceptions rather than a repeal: a fund-to-revenue-category link is neither a
// fold nor a cross-tab, and there is no column order that makes it forward.
type nodeTiersAreDeclared struct{}

var _ Check = (*nodeTiersAreDeclared)(nil)

func (*nodeTiersAreDeclared) ID() string { return "node-tiers-are-declared" }
func (*nodeTiersAreDeclared) Tier() int  { return 1 }
func (*nodeTiersAreDeclared) Full() bool { return false }
func (*nodeTiersAreDeclared) Description() string {
	return "every node's tier is the one docs/sankey-contract.md's table gives for its id " +
		"form, and every link runs from a coarser tier to a finer one unless it is a rollup " +
		"into the source's own parent or a declared partition"
}

func (*nodeTiersAreDeclared) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	nodes := 0
	links := 0
	rollups := 0
	partitions := 0

	for _, p := range s.linkedDocuments() {
		tierOf := map[string]int{}
		parentOf := map[string]string{}
		for _, n := range p.Nodes {
			nodes++
			tierOf[n.ID] = n.Tier
			parentOf[n.ID] = n.Parent

			// ONE FINDING PER NODE. A node whose form is undeclared has no
			// tier to be compared against, so the arms are ordered rather than
			// independent: reporting both would give one defect two findings
			// that read as two separate ones.
			want, ok := declaredTier(n.ID)
			switch {
			case !ok:
				findings = append(findings, finding(p.String(),
					"node %q is of no id form the contract declares. Its table names "+
						"revenue/, revenue-line/, fund-group/, fund/<number>, dept/ and "+
						"expenditure/, plus five flow endpoints by name; a coined form has "+
						"no tier and no place in the fold", n.ID))
			case n.Tier != want:
				findings = append(findings, finding(p.String(),
					"node %q carries tier %d and its id form declares tier %d. The tier is "+
						"what the client folds on and what orders the diagram's columns, so "+
						"a node at the wrong one is drawn in the wrong place and aggregated "+
						"into the wrong parent", n.ID, n.Tier, want))
			}
		}

		for _, l := range p.Links {
			links++
			src, sok := tierOf[l.Source]
			dst, dok := tierOf[l.Target]
			// A LINK NAMING A NODE THE GRAPH DOES NOT CARRY IS REPORTED HERE,
			// because nothing else in this package reports it. An earlier
			// version of this comment handed the case to graph-acyclic and
			// link-values-tie-to-facts and both refuse it: findCycle only looks
			// for cycles, and a dangling target is a leaf rather than a cycle;
			// linkValuesTieToFacts never reads Graph.Nodes at all. So a typo'd
			// endpoint passed every check in the tree, and the client would
			// draw a ribbon into a box that does not exist.
			//
			// It belongs here rather than in a check of its own: this is the
			// only place that has already indexed the nodes by id, and a tier
			// comparison cannot be made without resolving both ends anyway.
			if !sok || !dok {
				missing := l.Target
				if !sok {
					missing = l.Source
				}
				findings = append(findings, finding(p.String(),
					"link %q -> %q names %q, which is not a node of this graph. Nothing "+
						"else asserts a link's endpoints resolve -- graph-acyclic looks "+
						"only for cycles and a dangling end is a leaf -- so the client "+
						"would draw a ribbon into a box that is not there",
					l.Source, l.Target, missing))
				continue
			}
			if src < dst {
				continue
			}
			switch {
			case parentOf[l.Source] == l.Target:
				rollups++
			case l.Partition:
				partitions++
			default:
				findings = append(findings, finding(p.String(),
					"link %q -> %q runs from tier %d to tier %d, %q is not %q's own "+
						"parent, and the link declares no partition. A flow in this diagram "+
						"goes from a coarser tier to a finer one, from a node into the box "+
						"it folds into, or along the second axis of one printed table; one "+
						"that does none of the three is drawn as a ribbon running against "+
						"every other ribbon on the page",
					l.Source, l.Target, src, dst, l.Target, l.Source))
			}
		}
	}

	return conclusion{
		subjects: nodes,
		unit:     "nodes",
		held: fmt.Sprintf("%d nodes over %d graph document(s), each at the tier its id form "+
			"declares, and %d links each running from a coarser tier to a finer one or, for "+
			"%d of them, from a node into the box it folds into and, for %d, along the "+
			"second axis of one printed table; the declared forms are %s, plus %d flow "+
			"endpoints named individually",
			nodes, len(s.linkedDocuments()), links, rollups, partitions, describeForms(),
			len(endpointTiers)),
		nothing:  "no projection carries a node, so no tier has been read",
		findings: findings,
	}.result(), nil
}

// declaredTier is the tier an id form declares, and whether the form is one the
// contract names. The endpoint table is consulted FIRST: `fund-balance/draw` is
// an endpoint and `fund-balance` is not a hierarchy prefix, so the order is what
// makes the two tables non-overlapping rather than merely ordered.
func declaredTier(id string) (int, bool) {
	if t, ok := endpointTiers[id]; ok {
		return t, true
	}
	prefix, rest, ok := strings.Cut(id, "/")
	if !ok || rest == "" {
		return 0, false
	}
	t, ok := hierarchyTiers[prefix]
	if !ok {
		return 0, false
	}
	// A fund node's id form is `fund/<number>`, and the number is what makes it
	// one. Checked here rather than left to the reader because `fund/general`
	// would otherwise pass as a tier-3 node while naming no fund at all, one
	// hyphen away from the real `fund-group/general`.
	//
	// ZERO IS REFUSED WITH THE NON-NUMBERS, because no fund is numbered 0: a
	// fact with no fund publishes null, and the mapping side, which still
	// spells its absence 0 (fisc-12jt), never writes the segment at all --
	// fact.ColumnPath omits it. `fund/0` is therefore the same defect as
	// `fund/general` wearing a number.
	if prefix == "fund" {
		n, err := strconv.Atoi(rest)
		if err != nil || n == 0 {
			return 0, false
		}
	}
	return t, true
}

// describeForms lists the hierarchy prefixes and their tiers, for the summary.
func describeForms() string {
	out := make([]string, 0, len(hierarchyTiers))
	for prefix, tier := range hierarchyTiers {
		out = append(out, fmt.Sprintf("%s/ = %d", prefix, tier))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
