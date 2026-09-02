package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// tieredSubject is the miniature spine with one graph, ready to be damaged.
func tieredSubject(t *testing.T) *Subject {
	t.Helper()
	s := testSubject(t)
	if len(s.Graphs()) == 0 {
		t.Fatal("the fixture built no graph, so nothing below asserts anything")
	}
	return s
}

func runNodeTiers(t *testing.T, s *Subject) Result {
	t.Helper()
	res, err := (&nodeTiersAreDeclared{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// nodeIndex is the position of a node with the given id prefix, so a test can
// damage one without hard-coding which of the fixture's nodes it is.
func nodeIndex(t *testing.T, g *project.Graph, prefix string) int {
	t.Helper()
	for i, n := range g.Nodes {
		if strings.HasPrefix(n.ID, prefix) {
			return i
		}
	}
	t.Fatalf("the fixture carries no node with prefix %q", prefix)
	return -1
}

// TestNodeTiersAreDeclaredIsFailable damages the graph four ways, one per claim.
//
// EACH CASE IS INVISIBLE TO EVERY OTHER CHECK, which is the argument for this
// one existing. A node at the wrong tier still cites the right facts, so
// link-values-tie-to-facts is green; it introduces no cycle, so graph-acyclic is
// green; the counts are unchanged, so counts-reconcile is green. The only thing
// that moves is where the client draws the box and which parent it folds into.
func TestNodeTiersAreDeclaredIsFailable(t *testing.T) {
	// The undamaged fixture passes, or the cases below prove nothing.
	if res := runNodeTiers(t, tieredSubject(t)); res.Status != StatusPass {
		t.Fatalf("the undamaged fixture is %s, want PASS: %v", res.Status, res.Findings)
	}

	cases := []struct {
		name   string
		damage func(t *testing.T, g *project.Graph)
		want   string
	}{
		{
			// The case the check exists for: the id says one layer and the
			// field says another.
			name: "a node at the wrong tier for its id form",
			damage: func(t *testing.T, g *project.Graph) {
				g.Nodes[nodeIndex(t, g, "fund-group/")].Tier = 3
			},
			want: "declares tier 2",
		},
		{
			// docs/sankey-contract.md: "Do not coin new ones."
			name: "a coined id form",
			damage: func(t *testing.T, g *project.Graph) {
				g.Nodes[nodeIndex(t, g, "revenue/")].ID = "programme/police"
			},
			want: "no id form the contract declares",
		},
		{
			// Tier 1 was the constraint tier and the layer cannot exist, so
			// claiming it is claiming a layer nothing can define.
			name: "the unused tier",
			damage: func(t *testing.T, g *project.Graph) {
				g.Nodes[nodeIndex(t, g, "fund-group/")].ID = "fund/1"
				g.Nodes[nodeIndex(t, g, "fund/1")].Tier = unusedTier
			},
			want: "leaves UNUSED",
		},
		{
			// A backward ribbon. d3 will draw it, which is why no rendering
			// test would catch it either.
			name: "a link running from a finer tier to a coarser one",
			damage: func(t *testing.T, g *project.Graph) {
				g.Links[0].Source, g.Links[0].Target = g.Links[0].Target, g.Links[0].Source
			},
			want: "coarser tier to a finer one",
		},
		{
			// THE GAP: this was a `continue` blamed on
			// graph-acyclic and link-values-tie-to-facts, and neither reports
			// it. findCycle only looks for cycles and a dangling end is a leaf;
			// linkValuesTieToFacts never reads Graph.Nodes. So a typo'd
			// endpoint passed every check in the tree.
			name: "a link naming a node the graph does not carry",
			damage: func(t *testing.T, g *project.Graph) {
				g.Links[0].Target = "fund-group/genrl"
			},
			want: "not a node of this graph",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := tieredSubject(t)
			c.damage(t, s.Graphs()[0].Graph)
			res := runNodeTiers(t, s)
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want FAIL", res.Status)
			}
			var saw bool
			for _, f := range res.Findings {
				if strings.Contains(f.Detail, c.want) {
					saw = true
				}
			}
			if !saw {
				t.Errorf("no finding mentions %q; got %v", c.want, res.Findings)
			}
		})
	}
}

// TestAFundNodeNamesAFundNumber is the one id form whose SHAPE carries meaning
// beyond its prefix.
//
// `fund/<number>` is the contract's form, and the number is what makes the node
// a fund rather than a word beginning with "fund". Without this,
// `fund/general` would pass as a tier-3 node naming no fund at all -- and it is
// a plausible mistake, because `fund-group/general` is a real node one hyphen
// away.
func TestAFundNodeNamesAFundNumber(t *testing.T) {
	for _, id := range []string{"fund/100", "fund/642"} {
		if _, ok := declaredTier(id); !ok {
			t.Errorf("declaredTier(%q) = not a declared form, want tier 3", id)
		}
	}
	// fund/0 is refused with the non-numbers: 0 is this codebase's NO-FUND
	// sentinel, not a fund. fact.ColumnPath omits the segment entirely when
	// Fund == 0 and every spine fact ships "fund":0 meaning "this schedule has
	// no fund axis", so `fund/0` is `fund/general` wearing a number.
	for _, id := range []string{"fund/0", "fund/general", "fund/", "fund", "revenue/", "dept"} {
		if tier, ok := declaredTier(id); ok {
			t.Errorf("declaredTier(%q) = %d, true; want it refused", id, tier)
		}
	}
}
