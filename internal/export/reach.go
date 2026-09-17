package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Graph is as much of a projection document as a reach needs: which tier,
// role and parent each node has, and which nodes each ribbon joins at what
// value.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// GraphNode is one node of a [Graph]: its id, the tier it sits at, the role
// the document printed it with, and the id of its parent or "" at a root.
type GraphNode struct {
	ID     string `json:"id"`
	Tier   int    `json:"tier"`
	Role   string `json:"role"`
	Parent string `json:"parent"`
}

// GraphLink is one ribbon of a [Graph], from Source to Target at ValueCents.
type GraphLink struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	ValueCents int64  `json:"value_cents"`
}

// DecodeGraph reads a projection document down to its [Graph], and refuses
// one with no nodes: every question asked of a graph here is a question about
// what a document draws, and a document drawing nothing answers none of them.
func DecodeGraph(raw []byte) (Graph, error) {
	var g Graph
	if err := json.Unmarshal(raw, &g); err != nil {
		return Graph{}, fmt.Errorf("decode graph: %w", err)
	}
	if len(g.Nodes) == 0 {
		return Graph{}, fmt.Errorf("decode graph: no nodes")
	}
	return g, nil
}

// Reach is what a document draws at each of a set of tiers when one node is
// opened into them: At holds, per drawn tier, the ids the folded chart
// touches, sorted; In and Out hold the cents into and out of each node over
// the kept ribbons before the fold, which is the column the client ranks a
// cap over.
type Reach struct {
	At  map[int][]string
	In  map[string]int64
	Out map[string]int64
}

// maxHops is how far up a parent chain a placement walks before giving up,
// which is the client's own bound; node-hierarchy-well-formed refuses a cycle
// Go-side, so on a verified store the bound is never the reason a walk stops.
const maxHops = 9

// ReachOf is the client's filter, fold and prune as one rule: what site/app.js
// draws of document g when node opened is opened into tiers, on the side its
// step declares.
//
// THE RULE IS THE HIERARCHY'S, NOT THE RIBBONS'. A ribbon is kept when its
// near end -- its source when nearIsSource, its target otherwise, which is
// filterFromNode against filterToNode -- is the opened node or a descendant of
// it by parent chain, and both of its ends have an ancestor at a drawn tier to
// be folded to. Every kept ribbon is then folded to those ancestors, a ribbon
// folding to one node is dropped as a flow inside what is now one box, and a
// node no folded ribbon touches is pruned. What is left at each tier is At.
//
// WHY NOT A WALK OUTWARD ALONG RIBBONS FROM THE OPENED NODE: the transfers
// document's tier 0 is transfers/in, which no ribbon touches in either
// direction; the eight payer ends at tier 2 are its children, and the client
// reaches them through the parent chain. On every step that keeps a flank the
// two readings coincide on the committed corpus, and the rung artifact under
// testdata/ is the measurement; this is the one that also answers the step
// that keeps none.
//
// THE OPENED NODE HAS TO EXIST IN g, and that is the whole of what the client
// asks of it: filterLinks refuses an id the document does not carry and reads
// nothing else about the node, not even its tier. A caller that wants the
// opened node at a particular tier of g asks that itself.
//
// EMPTY IS AN ANSWER HERE AND A REFUSAL IN EVERY CALLER. A Reach whose At is
// empty is a node the document does not decompose under these tiers, which
// stepView.Opens leaves out and a rung refuses by name; returning it rather
// than an error is what lets one function answer both "which nodes open" and
// "what does this one draw".
func ReachOf(g Graph, opened string, nearIsSource bool, tiers []int) (Reach, error) {
	byID := make(map[string]GraphNode, len(g.Nodes))
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	if _, ok := byID[opened]; !ok {
		return Reach{}, fmt.Errorf("the document does not carry node %q", opened)
	}
	drawn := make(map[int]bool, len(tiers))
	for _, t := range tiers {
		drawn[t] = true
	}
	// ancestor is the parent chain of id up to maxHops, id first, or an error
	// on a parent the document does not carry, which is the one shape the
	// client refuses to answer "no" for.
	ancestor := func(id string) ([]GraphNode, error) {
		var chain []GraphNode
		at, ok := byID[id]
		for hops := 0; ok && hops < maxHops; hops++ {
			chain = append(chain, at)
			if at.Parent == "" {
				break
			}
			up, found := byID[at.Parent]
			if !found {
				return nil, fmt.Errorf("node %q names parent %q, which the document does not carry", at.ID, at.Parent)
			}
			at, ok = up, true
		}
		return chain, nil
	}
	inside := map[string]bool{}
	foldTo := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		chain, err := ancestor(n.ID)
		if err != nil {
			return Reach{}, err
		}
		for _, a := range chain {
			if a.ID == opened {
				inside[n.ID] = true
			}
			if _, placed := foldTo[n.ID]; !placed && drawn[a.Tier] {
				foldTo[n.ID] = a.ID
			}
		}
	}
	r := Reach{At: map[int][]string{}, In: map[string]int64{}, Out: map[string]int64{}}
	touched := map[string]bool{}
	for _, l := range g.Links {
		near := l.Target
		if nearIsSource {
			near = l.Source
		}
		if !inside[near] {
			continue
		}
		fs, ok := foldTo[l.Source]
		if !ok {
			continue
		}
		ft, ok := foldTo[l.Target]
		if !ok {
			continue
		}
		v := l.ValueCents
		if v < 0 {
			v = -v
		}
		r.In[l.Target] += v
		r.Out[l.Source] += v
		if fs == ft {
			continue
		}
		touched[fs] = true
		touched[ft] = true
	}
	for id := range touched {
		t := byID[id].Tier
		r.At[t] = append(r.At[t], id)
	}
	for _, t := range slices.Sorted(maps.Keys(r.At)) {
		slices.Sort(r.At[t])
	}
	return r, nil
}
