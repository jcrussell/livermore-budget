package export

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestChartSplicesAsWindowForDoes is the three rules a carried chart is
// assembled under: the first record of an id wins, ribbons between one pair
// of ends of one kind merge, and a ribbon joining a node to itself is
// dropped -- the shape a folded node's ribbons take once re-pointed at the
// tail they were folded into.
func TestChartSplicesAsWindowForDoes(t *testing.T) {
	c := IndexGraph(Graph{
		Nodes: []GraphNode{{ID: "centre", Tier: 1, Parent: "flank"}, {ID: "flank", Tier: 0}},
		Links: []GraphLink{{Source: "flank", Target: "centre", ValueCents: 5}},
	})
	c.Add(GraphNode{ID: "centre", Tier: 1}) // the fresh half's record: loses
	c.Add(GraphNode{ID: AggregateID(2), Tier: 2, Role: RoleAggregate, Parent: "centre"})
	c.Add(GraphNode{ID: "part", Tier: 2, Parent: "centre"})
	for _, l := range []GraphLink{
		{Source: "centre", Target: "part", ValueCents: 3},
		{Source: "centre", Target: AggregateID(2), ValueCents: 1},
		{Source: "centre", Target: AggregateID(2), ValueCents: 1},
		{Source: AggregateID(2), Target: AggregateID(2), ValueCents: 9},
	} {
		if err := c.Link(l); err != nil {
			t.Fatal(err)
		}
	}
	want := Graph{
		Nodes: []GraphNode{
			{ID: AggregateID(2), Tier: 2, Role: RoleAggregate, Parent: "centre"},
			{ID: "centre", Tier: 1, Parent: "flank"},
			{ID: "flank", Tier: 0},
			{ID: "part", Tier: 2, Parent: "centre"},
		},
		Links: []GraphLink{
			{Source: "centre", Target: AggregateID(2), ValueCents: 2},
			{Source: "centre", Target: "part", ValueCents: 3},
			{Source: "flank", Target: "centre", ValueCents: 5},
		},
	}
	if diff := cmp.Diff(want, c.Graph()); diff != "" {
		t.Errorf("Chart.Graph (-want +got):\n%s", diff)
	}
	if !c.Within("centre", "part") || !c.Within("flank", "part") || c.Within("part", "centre") || !c.Within("part", "part") {
		t.Error("Within does not walk the chart's own parent chain")
	}
	if !IsAggregate(AggregateID(2)) || IsAggregate("part") {
		t.Error("IsAggregate does not recognise the tail's id")
	}
}

// TestChartRefusesARibbonToANodeItDoesNotHold: the walk that assembles a
// chart has already refused the fold that could orphan a ribbon, so an
// absent end here is its own bookkeeping gone wrong and not a case to drop.
func TestChartRefusesARibbonToANodeItDoesNotHold(t *testing.T) {
	c := IndexGraph(Graph{Nodes: []GraphNode{{ID: "a"}}})
	err := c.Link(GraphLink{Source: "a", Target: "gone"})
	if err == nil || !strings.Contains(err.Error(), `"gone", which the chart does not hold`) {
		t.Fatalf("Link to an absent end: err = %v", err)
	}
}
