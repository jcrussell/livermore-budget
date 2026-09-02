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
// TIER 1 IS ABSENT AND THE ABSENCE IS ASSERTED, not merely unlisted. The
// contract used to give tier 1 as a `constraint/<tier>` node and no longer does:
// a constraint tier is a property of a FUND and the fund groups do not partition
// along it (data/funds.yaml has capital = 3 committed + 43 restricted-by-law), so
// the layer cannot exist and the number is left unused rather than renumbering
// tiers 2-5, which are published in node.tier today. A node claiming tier 1 is
// therefore a node claiming a layer nothing can define.
var hierarchyTiers = map[string]int{
	"revenue":     0,
	"fund-group":  2,
	"fund":        3,
	"dept":        4,
	"expenditure": 5,
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
type nodeTiersAreDeclared struct{}

var _ Check = (*nodeTiersAreDeclared)(nil)

func (*nodeTiersAreDeclared) ID() string { return "node-tiers-are-declared" }
func (*nodeTiersAreDeclared) Tier() int  { return 1 }
func (*nodeTiersAreDeclared) Full() bool { return false }
func (*nodeTiersAreDeclared) Description() string {
	return "every node's tier is the one docs/sankey-contract.md's table gives for its id " +
		"form, and every link runs from a coarser tier to a finer one"
}

func (*nodeTiersAreDeclared) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	nodes := 0
	links := 0

	for _, p := range s.LinkedDocuments() {
		tierOf := map[string]int{}
		for _, n := range p.Nodes {
			nodes++
			tierOf[n.ID] = n.Tier

			// ONE FINDING PER NODE, MOST SPECIFIC FIRST. The three arms
			// overlap -- no declared form has tier 1, so a node claiming it
			// always mismatches its form too -- and reporting both would give
			// one defect two findings that read as two separate ones.
			want, ok := declaredTier(n.ID)
			switch {
			case !ok:
				findings = append(findings, finding(p.String(),
					"node %q is of no id form the contract declares. Its table names "+
						"revenue/, fund-group/, fund/<number>, dept/ and expenditure/, plus "+
						"five flow endpoints by name; a coined form has no tier and no place "+
						"in the fold", n.ID))
			case n.Tier == unusedTier:
				findings = append(findings, finding(p.String(),
					"node %q claims tier %d, which the contract leaves UNUSED (its id form "+
						"declares tier %d). Tier 1 was the constraint tier, and that layer "+
						"cannot exist: a constraint tier is a property of a fund and the "+
						"fund groups do not partition along it", n.ID, unusedTier, want))
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
			if src >= dst {
				findings = append(findings, finding(p.String(),
					"link %q -> %q runs from tier %d to tier %d. Every flow in this diagram "+
						"goes from a coarser tier to a finer one, and one that does not is "+
						"drawn as a ribbon running against every other ribbon on the page",
					l.Source, l.Target, src, dst))
			}
		}
	}

	return conclusion{
		subjects: nodes,
		unit:     "nodes",
		held: fmt.Sprintf("%d nodes over %d graph document(s), each at the tier its id form "+
			"declares, and %d links each running from a coarser tier to a finer one; the "+
			"declared forms are %s, plus %d flow endpoints named individually",
			nodes, len(s.LinkedDocuments()), links, describeForms(), len(endpointTiers)),
		nothing:  "no projection carries a node, so no tier has been read",
		findings: findings,
	}.result(), nil
}

// unusedTier is the number docs/sankey-contract.md leaves unassigned. Named
// rather than written as a literal 1, because the whole point is that it means
// "nothing", and a bare 1 beside tiers 0 and 2 reads like an omission.
const unusedTier = 1

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
	// ZERO IS REFUSED WITH THE NON-NUMBERS, because 0 is this codebase's
	// no-fund sentinel rather than a fund: fact.ColumnPath omits the segment
	// entirely when Fund == 0, and every one of the spine's facts ships
	// "fund":0 meaning "this schedule has no fund axis". `fund/0` is therefore
	// the same defect as `fund/general` wearing a number.
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
