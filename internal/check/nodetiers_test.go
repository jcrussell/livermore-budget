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
	if len(s.graphs()) == 0 {
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
			// A LINE LEFT AT ITS PARENT'S TIER, which is the tier-1 form's own
			// failure mode and the one that reads as nothing at all: the node
			// has the right id, the right parent and the right facts, and the
			// client draws it in its category's column rather than beside it.
			// Renaming the node carries its links with it, so the only claim
			// that moves is the tier.
			name: "a revenue line at its category's tier",
			damage: func(t *testing.T, g *project.Graph) {
				i := nodeIndex(t, g, "revenue/")
				was := g.Nodes[i].ID
				now := "revenue-line/" + strings.TrimPrefix(was, "revenue/") + "/eraf"
				g.Nodes[i].ID = now
				for j := range g.Links {
					if g.Links[j].Source == was {
						g.Links[j].Source = now
					}
				}
			},
			want: "declares tier 1",
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
			c.damage(t, s.graphs()[0].Graph)
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

// TestARollupIsTheOnlyDescendingLinkAllowed is the exemption and its edge, in
// one test because either alone is a different check.
//
// THE DRILL-DOWN ADDS EACH PRINTED ROW BACK INTO THE CATEGORY IT IS PRINTED
// UNDER, which is a tier-1 node pointing at a tier-0 one. It is the edge the
// client already folds along, so it draws no backward ribbon in any view: a
// column order is a view's own declaration and the two ends land in whichever
// order that declaration puts them. What must stay refused is every OTHER
// descending link, and the difference between the two is one comparison -- so a
// test that only proved the exemption would pass on a check that had stopped
// looking at direction at all.
func TestARollupIsTheOnlyDescendingLinkAllowed(t *testing.T) {
	rolled := func(t *testing.T, target string) *Subject {
		t.Helper()
		s := tieredSubject(t)
		g := s.graphs()[0].Graph
		category := g.Nodes[nodeIndex(t, g, "revenue/")].ID
		// The id form is `revenue-line/` and the slug it carries is the
		// category's own, which is what declaredTier reads it as tier 1 by.
		line := project.Node{ID: "revenue-line/" + strings.TrimPrefix(category, "revenue/") +
			"/eraf", Tier: 1, Parent: category}
		g.Nodes = append(g.Nodes, line)
		if target == "" {
			target = category
		}
		g.Links = append(g.Links, project.Link{Source: line.ID, Target: target,
			ValueCents: 1, Kind: project.KindExternal, FactIDs: []string{"x"}})
		return s
	}

	if res := runNodeTiers(t, rolled(t, "")); res.Status != StatusPass {
		t.Errorf("a line rolled up into its own category is %s, want PASS: %v",
			res.Status, res.Findings)
	}

	// THE SAME LINK ONE NODE OVER. transfers/in is tier 0 and is nobody's
	// parent, so this is a descending link that folds into nothing -- and it
	// is the shape a mis-pointed rollup would have.
	res := runNodeTiers(t, rolled(t, "transfers/in"))
	if res.Status != StatusFail {
		t.Fatalf("a line pointing at a tier-0 node that is not its parent is %s, want FAIL",
			res.Status)
	}
	var saw bool
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, "is not") && strings.Contains(f.Detail, "own parent") {
			saw = true
		}
	}
	if !saw {
		t.Errorf("no finding says the target is not the source's own parent; got %v",
			res.Findings)
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
