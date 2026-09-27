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
// `revenue-line/` is its own prefix because declaredTier cuts at the first
// slash and a category slug already has one or two segments. `transfer-from/`
// and `transfer-to/` are the two ends of a p76 movement, one node per fund:
// fund to fund would run tier 3 to tier 3, which d3-sankey cannot lay out.
var hierarchyTiers = map[string]int{
	"revenue":       0,
	"revenue-line":  1,
	"fund-group":    2,
	"transfer-from": 2,
	"fund":          3,
	"dept":          4,
	// `department/` (pp.85-125's departments) is a separate form from `dept/`
	// (pp.167-170's divisions) because five slugs, city-council among them,
	// are in both namespaces.
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
// A descending link is refused unless it is not a flow: a `revenue-line/` node
// rolled up into its own `revenue/` parent, or a [project.Link.Partition] --
// the projection's claim that it read one printed matrix along its second
// axis. A document declares a partition on every link or none, and along one
// pair of tiers, so the flag cannot exempt a single flow.
type nodeTiersAreDeclared struct{}

var _ Check = (*nodeTiersAreDeclared)(nil)

func (*nodeTiersAreDeclared) ID() string { return "node-tiers-are-declared" }
func (*nodeTiersAreDeclared) Tier() int  { return 1 }
func (*nodeTiersAreDeclared) Full() bool { return false }
func (*nodeTiersAreDeclared) Description() string {
	return "every node's tier is the one docs/sankey-contract.md's table gives for its id " +
		"form, and every link runs from a coarser tier to a finer one unless it is a revenue " +
		"line rolled up into its category or a document-wide partition along one pair of tiers"
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

			// One finding per node: an undeclared form has no tier to compare.
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

		partitioned, plain := 0, 0
		axes := map[[2]int]bool{}
		for _, l := range p.Links {
			links++
			src, sok := tierOf[l.Source]
			dst, dok := tierOf[l.Target]
			// A dangling end is reported only here: graph-acyclic sees a leaf,
			// and link-values-tie-to-facts never reads the nodes.
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
			if l.Partition {
				partitions++
				partitioned++
				axes[[2]int{src, dst}] = true
			} else {
				plain++
			}
			if src < dst && !isRollup(l.Target, l.Source, parentOf) {
				continue
			}
			switch {
			case isRollup(l.Source, l.Target, parentOf):
				rollups++
			case isRollup(l.Target, l.Source, parentOf):
				findings = append(findings, finding(p.String(),
					"link %q -> %q runs from a revenue category into its own line. A line "+
						"rolls up into its category and never the reverse; the client drops a "+
						"reversed rollup without a word, so the line's ribbon vanishes and its "+
						"category falls short by its amount", l.Source, l.Target))
			case l.Partition:
			default:
				findings = append(findings, finding(p.String(),
					"link %q -> %q runs from tier %d to tier %d, is not a revenue line "+
						"rolled up into its own category, and declares no partition. A flow in "+
						"this diagram goes from a coarser tier to a finer one, from a revenue "+
						"line into its category, or along the second axis of one printed "+
						"table; one that does none of the three is drawn as a ribbon running "+
						"against every other ribbon on the page, or dropped by the client",
					l.Source, l.Target, src, dst))
			}
		}

		// The partition is the document's claim, held across its links.
		if partitioned > 0 && plain > 0 {
			findings = append(findings, finding(p.String(),
				"this document declares a partition on %d of its %d links and none on the "+
					"other %d; one printed matrix has no cell that is a flow",
				partitioned, partitioned+plain, plain))
		}
		if len(axes) > 1 {
			pairs := make([]string, 0, len(axes))
			for a := range axes {
				pairs = append(pairs, fmt.Sprintf("%d -> %d", a[0], a[1]))
			}
			sort.Strings(pairs)
			findings = append(findings, finding(p.String(),
				"this document's partition links run between %d pairs of tiers (%s). One "+
					"printed matrix has two axes, so its cells run between one pair; links "+
					"partitioning across more tiers than that are flows wearing the flag",
				len(axes), strings.Join(pairs, ", ")))
		}
	}

	return conclusion{
		subjects: nodes,
		unit:     "nodes",
		held: fmt.Sprintf("%d nodes over %d graph document(s), each at its id form's tier; %d "+
			"links, each coarser to finer or one of %d rollups and %d partitions",
			nodes, len(s.linkedDocuments()), links, rollups, partitions),
		nothing:  "no projection carries a node, so no tier has been read",
		findings: findings,
	}.result(), nil
}

// isRollup is a revenue line added into the category it is printed under, the
// only child -> parent link any document draws.
func isRollup(source, target string, parentOf map[string]string) bool {
	return strings.HasPrefix(source, "revenue-line/") && strings.HasPrefix(target, "revenue/") &&
		parentOf[source] == target
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
	// No fund is numbered 0, so `fund/0` is refused with them.
	if prefix == "fund" {
		n, err := strconv.Atoi(rest)
		if err != nil || n == 0 {
			return 0, false
		}
	}
	return t, true
}
