package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// nodeTiersAreDeclared asserts every node is of an id form project.TierOf
// declares, and every link runs from a coarser tier to a finer one.
//
// project.TierOf is the one tier table. A node of no declared form, or whose
// published tier is not its form's -- a Node literal that names a tier rather
// than taking it from the table -- is a finding here. It does not
// witness that the tier table is the right model of the city's finances, and it
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
	return "every node is of an id form the tier table declares and at that form's tier, and every link runs from a coarser tier to a finer one unless it is a revenue " +
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

			want, ok := project.TierOf(n.ID)
			switch {
			case !ok:
				findings = append(findings, finding(p.String(),
					"node %q is of no declared id form (%s, or one of the endpoints %s); "+
						"a coined form has no tier and no place in the fold",
					n.ID, strings.Join(project.IDForms(), ", "), strings.Join(project.Endpoints(), ", ")))
			case n.Tier != want:
				findings = append(findings, finding(p.String(),
					"node %q carries tier %d and its id form's is %d. The tier is what "+
						"the client folds on and orders the columns by, so the node is drawn "+
						"in the wrong column", n.ID, n.Tier, want))
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
		held: fmt.Sprintf("%d nodes over %d graph document(s), each at its declared id form's tier; %d "+
			"links, each coarser to finer or one of %d rollups and %d partitions",
			nodes, len(s.linkedDocuments()), links, rollups, partitions),
		nothing:  "no projection carries a node, so no tier has been read",
		findings: findings,
	}.result(), nil
}

// isRollup is a revenue line added into the category it is printed under, the
// only child -> parent link any document draws.
func isRollup(source, target string, parentOf map[string]string) bool {
	return strings.HasPrefix(source, project.PrefixRevenueLine) && strings.HasPrefix(target, project.PrefixRevenue) &&
		parentOf[source] == target
}
