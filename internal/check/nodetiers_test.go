package check

import (
	"slices"
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
			// A line left at its parent's tier: only the tier moves.
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
		{
			// A flag on one link of a flow diagram.
			name: "a partition declared on one link of a document that draws flows",
			damage: func(t *testing.T, g *project.Graph) {
				g.Links[0].Partition = true
			},
			want: "declares a partition on 1 of its",
		},
		{
			// The flag on every link, across more than one pair of tiers.
			name: "a partition declared on every link of a document spanning more than one pair of tiers",
			damage: func(t *testing.T, g *project.Graph) {
				for i := range g.Links {
					g.Links[i].Partition = true
				}
			},
			want: "pairs of tiers",
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
// one test: proving the exemption alone would pass a check that had stopped
// looking at direction.
func TestARollupIsTheOnlyDescendingLinkAllowed(t *testing.T) {
	rolled := func(t *testing.T, target string) *Subject {
		t.Helper()
		s := tieredSubject(t)
		g := s.graphs()[0].Graph
		category := g.Nodes[nodeIndex(t, g, "revenue/")].ID
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

	// A mis-pointed rollup: transfers/in is tier 0 and nobody's parent.
	res := runNodeTiers(t, rolled(t, "transfers/in"))
	if res.Status != StatusFail {
		t.Fatalf("a line pointing at a tier-0 node that is not its parent is %s, want FAIL",
			res.Status)
	}
	var saw bool
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, "is not a revenue line rolled up into its own category") {
			saw = true
		}
	}
	if !saw {
		t.Errorf("no finding says the target is not the source's own parent; got %v",
			res.Findings)
	}

	// A reversed parent -> child link is child -> own parent too; only a
	// revenue line may roll up.
	for _, rev := range []struct{ parent, child project.Node }{
		{project.Node{ID: "fund-group/capital", Tier: 2}, project.Node{ID: "fund/510", Tier: 3}},
		{project.Node{ID: "dept/police-patrol", Tier: 4},
			project.Node{ID: "expenditure/police-patrol/wages-and-benefits", Tier: 5}},
	} {
		s := tieredSubject(t)
		g := s.graphs()[0].Graph
		rev.child.Parent = rev.parent.ID
		g.Nodes = append(g.Nodes, rev.parent, rev.child)
		g.Links = append(g.Links, project.Link{Source: rev.child.ID, Target: rev.parent.ID,
			ValueCents: 1, Kind: project.KindExternal, FactIDs: []string{"x"}})
		if res := runNodeTiers(t, s); res.Status != StatusFail {
			t.Errorf("%s -> its own parent %s is %s, want FAIL", rev.child.ID, rev.parent.ID,
				res.Status)
		}
	}
}

// TestAPartitionIsTheOTHERDescendingLinkAllowed is the second exemption and
// its narrowness: the cross-tab's expenditure/<object> -> dept/<division>
// descends between parentless nodes and passes only on the flag, so the same
// link with the flag cleared must be refused by name.
func TestAPartitionIsTheOTHERDescendingLinkAllowed(t *testing.T) {
	crossTab := func(t *testing.T, partition bool) *Subject {
		t.Helper()
		col := project.Column{FiscalYear: testYear, Basis: project.PublishedBasis}
		// Both ends parentless, as the committed cross-tab publishes.
		doc := &project.DepartmentSpendingDocument{
			Nodes: []project.Node{
				{ID: "expenditure/wages-and-benefits", Tier: 5, Role: "object_category"},
				{ID: "expenditure/services-and-supplies", Tier: 5, Role: "object_category"},
				{ID: "dept/police-patrol", Tier: 4, Role: "department"},
			},
			Links: []project.Link{
				{Source: "expenditure/wages-and-benefits", Target: "dept/police-patrol",
					ValueCents: 1, Kind: project.KindExternal, FactIDs: []string{"x"},
					Partition: partition},
				{Source: "expenditure/services-and-supplies", Target: "dept/police-patrol",
					ValueCents: 1, Kind: project.KindExternal, FactIDs: []string{"y"},
					Partition: partition},
			},
		}
		return &Subject{Projections: []projection{{
			Name: project.DepartmentSpendingProjection,
			Options: project.Options{Columns: []project.Column{col},
				Scopes: project.DepartmentSpendingScopes(), Version: testVersion},
			DepartmentSpending: doc,
		}}}
	}

	if res := runNodeTiers(t, crossTab(t, true)); res.Status != StatusPass {
		t.Errorf("a declared partition running 5 -> 4 is %s, want PASS: %v",
			res.Status, res.Findings)
	}

	res := runNodeTiers(t, crossTab(t, false))
	if res.Status != StatusFail {
		t.Fatalf("the same link with partition cleared is %s, want FAIL", res.Status)
	}
	var saw bool
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, "declares no partition") &&
			strings.Contains(f.Detail, "rolled up into its own category") {
			saw = true
		}
	}
	if !saw {
		t.Errorf("no finding names both the missing parent edge and the missing partition "+
			"declaration; got %v", res.Findings)
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
	// fund/0 is refused with the non-numbers: no fund is numbered 0.
	for _, id := range []string{"fund/0", "fund/general", "fund/", "fund", "revenue/", "dept"} {
		if tier, ok := declaredTier(id); ok {
			t.Errorf("declaredTier(%q) = %d, true; want it refused", id, tier)
		}
	}
}

// TestAReversedRollupIsRefused reverses one revenue-line rollup in every
// document that draws one, over the committed corpus.
func TestAReversedRollupIsRefused(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatal(err)
	}
	if res := runNodeTiers(t, s); res.Status != StatusPass {
		t.Fatalf("the committed corpus is %s: %v", res.Status, res.Findings)
	}
	reversed := 0
	for _, p := range s.linkedDocuments() {
		for i := range p.Links {
			l := &p.Links[i]
			if strings.HasPrefix(l.Source, "revenue-line/") && strings.HasPrefix(l.Target, "revenue/") {
				l.Source, l.Target = l.Target, l.Source
				reversed++
				break
			}
		}
	}
	if reversed == 0 {
		t.Fatal("no document draws a rollup, so nothing was reversed")
	}
	res := runNodeTiers(t, s)
	got := 0
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, "runs from a revenue category into its own line") {
			got++
		}
	}
	if res.Status != StatusFail || got != reversed {
		t.Errorf("%d rollups reversed, %d refused as reversed (status %s): %v",
			reversed, got, res.Status, res.Findings)
	}
}

// TestEachRollupClauseIsNeeded plants one link per clause of isRollup that
// satisfies the other two, so dropping that clause lets it through.
func TestEachRollupClauseIsNeeded(t *testing.T) {
	for _, tc := range []struct {
		name           string
		source, target project.Node
		parented       bool
	}{
		{"a line into another category", project.Node{ID: "revenue-line/other/eraf", Tier: 1,
			Parent: "revenue/other"}, project.Node{ID: "revenue/taxes-x", Tier: 0}, false},
		{"a fund parented to a category", project.Node{ID: "fund/510", Tier: 3},
			project.Node{ID: "revenue/taxes-x", Tier: 0}, true},
		{"a line parented to an endpoint", project.Node{ID: "revenue-line/taxes-x/eraf", Tier: 1},
			project.Node{ID: "transfers/in", Tier: 0}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tieredSubject(t)
			g := s.graphs()[0].Graph
			if tc.parented {
				tc.source.Parent = tc.target.ID
			}
			for _, n := range []project.Node{tc.source, tc.target} {
				if !slices.ContainsFunc(g.Nodes, func(m project.Node) bool { return m.ID == n.ID }) {
					g.Nodes = append(g.Nodes, n)
				}
			}
			g.Links = append(g.Links, project.Link{Source: tc.source.ID, Target: tc.target.ID,
				ValueCents: 1, Kind: project.KindExternal, FactIDs: []string{"x"}})
			if res := runNodeTiers(t, s); res.Status != StatusFail {
				t.Errorf("%s -> %s is %s, want FAIL", tc.source.ID, tc.target.ID, res.Status)
			}
		})
	}
}
