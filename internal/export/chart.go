package export

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// aggregatePrefix is the id prefix site/app.js draws a folded tail under, so
// a chart carried between rungs names the tail as the client names it.
const aggregatePrefix = "aggregate/tail/"

// RoleAggregate is the role capColumn gives a folded tail.
const RoleAggregate = "aggregate"

// AggregateID is the id capColumn draws the folded tail of one tier under.
func AggregateID(tier int) string { return aggregatePrefix + strconv.Itoa(tier) }

// IsAggregate is whether id names a folded tail rather than a document node.
func IsAggregate(id string) bool { return strings.HasPrefix(id, aggregatePrefix) }

// Chart is a [Graph] indexed by id: for reading a node's record and walking
// its hierarchy, and for assembling one chart from halves. The first record
// of an id wins, which is windowFor's splice, and ribbons between one pair
// of ends of one kind merge, which is the merge site/app.js's foldDocument
// makes of the same pair.
type Chart struct {
	Nodes map[string]GraphNode
	links map[[3]string]int64
}

// IndexGraph is g as a [Chart], every node and ribbon of it added.
func IndexGraph(g Graph) *Chart {
	c := &Chart{Nodes: map[string]GraphNode{}, links: map[[3]string]int64{}}
	for _, n := range g.Nodes {
		c.Add(n)
	}
	for _, l := range g.Links {
		c.links[[3]string{l.Source, l.Target, l.Kind}] += l.ValueCents
	}
	return c
}

// Add records n unless the chart already holds its id.
func (c *Chart) Add(n GraphNode) {
	if _, have := c.Nodes[n.ID]; !have {
		c.Nodes[n.ID] = n
	}
}

// Link merges l into the chart, dropping one that joins a node to itself as
// a flow inside one box. AN END THE CHART DOES NOT HOLD IS AN ERROR, not a
// ribbon quietly left out: a chart assembled with a ribbon hanging off
// nothing is one the client would draw with a ribbon hanging off nothing,
// and the error names which end at the rung that built it.
func (c *Chart) Link(l GraphLink) error {
	if l.Source == l.Target {
		return nil
	}
	for _, end := range []string{l.Source, l.Target} {
		if _, have := c.Nodes[end]; !have {
			return fmt.Errorf("ribbon %q -> %q names %q, which the chart does not hold", l.Source, l.Target, end)
		}
	}
	c.links[[3]string{l.Source, l.Target, l.Kind}] += l.ValueCents
	return nil
}

// Within is whether id is root or a descendant of it by the chart's own
// parent chain, which is withinNode over the chart on screen.
func (c *Chart) Within(root, id string) bool {
	at, ok := c.Nodes[id]
	for hops := 0; ok && hops < maxHops; hops++ {
		if at.ID == root {
			return true
		}
		if at.Parent == "" {
			return false
		}
		at, ok = c.Nodes[at.Parent]
	}
	return false
}

// Graph is the chart as a [Graph], nodes sorted by id and ribbons by source,
// target and kind, so one chart is one byte sequence however it was built.
func (c *Chart) Graph() Graph {
	var g Graph
	for _, id := range slices.Sorted(maps.Keys(c.Nodes)) {
		g.Nodes = append(g.Nodes, c.Nodes[id])
	}
	keys := slices.SortedFunc(maps.Keys(c.links), func(a, b [3]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]), cmp.Compare(a[2], b[2]))
	})
	for _, k := range keys {
		g.Links = append(g.Links, GraphLink{Source: k[0], Target: k[1], ValueCents: c.links[k], Kind: k[2]})
	}
	return g
}
