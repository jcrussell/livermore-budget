package export

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// foldFixture is three tiers of which 0 and 2 are drawn: a root at 0 with two
// children at 1, one of which has two children at 2. Every ribbon below
// exercises one rule of the fold, and the test names which.
func foldFixture() Graph {
	return Graph{
		Nodes: []GraphNode{
			{ID: "a", Tier: 0},
			{ID: "b", Tier: 1, Parent: "a"},
			{ID: "e", Tier: 1, Parent: "a"},
			{ID: "c", Tier: 2, Parent: "b", Role: "leaf"},
			{ID: "d", Tier: 2, Parent: "b", Derived: true},
		},
		Links: []GraphLink{
			{Source: "a", Target: "c", ValueCents: 100},          // drawn to drawn: kept as is
			{Source: "b", Target: "c", ValueCents: 10},           // b folds to a: merges into a -> c
			{Source: "a", Target: "c", ValueCents: 1, Kind: "x"}, // another kind: kept apart
			{Source: "a", Target: "b", ValueCents: 5},            // b folds to a: a flow inside one box
			{Source: "a", Target: "d", ValueCents: -7},           // a reduction: summed as printed
			{Source: "a", Target: "e", ValueCents: 3},            // e folds to a: e is touched by nothing
		},
	}
}

// TestFoldIsTheClientsFoldDocument is Fold's four rules on one graph, and
// the mutations that show each: drop the fs == ft rule and a -> a appears;
// drop the parent re-point and c keeps parent b, which the chart does not
// hold; merge without the kind and the two a -> c ribbons become one; skip
// the prune and e is drawn with no ribbon.
func TestFoldIsTheClientsFoldDocument(t *testing.T) {
	got, err := Fold(foldFixture(), []int{0, 2})
	if err != nil {
		t.Fatal(err)
	}
	want := Graph{
		Nodes: []GraphNode{
			{ID: "a", Tier: 0},
			{ID: "c", Tier: 2, Parent: "a", Role: "leaf"},
			{ID: "d", Tier: 2, Parent: "a", Derived: true},
		},
		Links: []GraphLink{
			{Source: "a", Target: "c", ValueCents: 110},
			{Source: "a", Target: "c", ValueCents: 1, Kind: "x"},
			{Source: "a", Target: "d", ValueCents: -7},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Fold (-want +got):\n%s", diff)
	}
}

// TestFoldRefusesARibbonEndNoDrawnTierPlaces mirrors foldDocument's refusal of
// a node with no ancestor at a drawn tier.
func TestFoldRefusesARibbonEndNoDrawnTierPlaces(t *testing.T) {
	g := foldFixture()
	g.Nodes = append(g.Nodes, GraphNode{ID: "z", Tier: 3})
	g.Links = append(g.Links, GraphLink{Source: "a", Target: "z", ValueCents: 1})
	_, err := Fold(g, []int{0, 2})
	if err == nil || !strings.Contains(err.Error(), `"z" has no ancestor at a drawn tier`) {
		t.Fatalf("Fold with an unplaceable end: err = %v", err)
	}
}

// TestReachOfCarriesTheFoldedChart is that Drawn is Fold over exactly the
// ribbons the filter keeps.
func TestReachOfCarriesTheFoldedChart(t *testing.T) {
	g := foldFixture()
	// A ribbon whose near end is outside b's subtree, which the filter drops
	// and the fold would otherwise draw.
	g.Links = append(g.Links, GraphLink{Source: "e", Target: "c", ValueCents: 1000})
	r, err := ReachOf(g, "b", true, []int{0, 2})
	if err != nil {
		t.Fatal(err)
	}
	// Opened into b on the source side: b -> c is the one ribbon whose source
	// is inside b, and it folds to a -> c.
	want := Reach{
		At: map[int][]string{0: {"a"}, 2: {"c"}},
		Drawn: Graph{
			Nodes: []GraphNode{{ID: "a", Tier: 0}, {ID: "c", Tier: 2, Parent: "a", Role: "leaf"}},
			Links: []GraphLink{{Source: "a", Target: "c", ValueCents: 10}},
		},
	}
	if diff := cmp.Diff(want, r); diff != "" {
		t.Errorf("ReachOf (-want +got):\n%s", diff)
	}
}

// TestFoldRefusesAParentCycle is a two-node parent ring, which a chain
// truncated at the hop bound would read as ending at a root.
func TestFoldRefusesAParentCycle(t *testing.T) {
	g := Graph{
		Nodes: []GraphNode{{ID: "a", Tier: 0}, {ID: "x", Tier: 1, Parent: "y"}, {ID: "y", Tier: 1, Parent: "x"}},
		Links: []GraphLink{{Source: "a", Target: "x", ValueCents: 1}},
	}
	_, err := Fold(g, []int{0, 1})
	if err == nil || !strings.Contains(err.Error(), "does not reach a root within") {
		t.Fatalf("Fold over a parent cycle: err = %v", err)
	}
}
