package export

import (
	"cmp"
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
// the document printed it with, the id of its parent or "" at a root, and
// whether the document marks it derived -- which the client reads as "not
// one of the opened node's parts" wherever the node is drawn.
type GraphNode struct {
	ID   string `json:"id"`
	Tier int    `json:"tier"`
	// Label is the city's own word for the node, decoded because a mark's
	// prose names the node it stands beside and that sentence is Go's: a
	// residual says which flow the schedule does not split, and a gap says
	// which cell the two documents disagree about.
	Label   string `json:"label"`
	Role    string `json:"role"`
	Parent  string `json:"parent"`
	Derived bool   `json:"derived"`
}

// GraphLink is one ribbon of a [Graph], from Source to Target at ValueCents,
// of one Kind; the fold keeps kinds apart, as foldDocument does.
type GraphLink struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	ValueCents int64  `json:"value_cents"`
	Kind       string `json:"kind"`
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
// opened into them: At holds, per drawn tier, the ids the chart touches,
// sorted, and Drawn is the chart itself, which is what a rung leaves on
// screen and the next rung reads its flank off.
//
// NO PRE-FOLD TOTAL RIDES ALONG, and a caller that wants one computes it
// itself. The cents into and out of each node before the fold are the key
// site/app.js's capColumn ranks a column by, and ranking a column against a
// cap is fitting, which is the client's (fisc-lwh5). A second spelling of it
// here would be a figure Go computes for a reader it does not have, free to
// drift from the one that draws.
type Reach struct {
	At    map[int][]string
	Drawn Graph
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
// be folded to. Every kept ribbon is then folded to those ancestors by [Fold],
// a ribbon folding to one node is dropped as a flow inside what is now one
// box, and a node no folded ribbon touches is pruned. What is left at each
// tier is At.
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
	byID := indexNodes(g)
	if _, ok := byID[opened]; !ok {
		return Reach{}, fmt.Errorf("the document does not carry node %q", opened)
	}
	chains, err := ancestry(g, byID)
	if err != nil {
		return Reach{}, err
	}
	drawn := drawnSet(tiers)
	inside := map[string]bool{}
	placeable := map[string]bool{}
	for id, chain := range chains {
		for _, a := range chain {
			if a.ID == opened {
				inside[id] = true
			}
			if drawn[a.Tier] {
				placeable[id] = true
			}
		}
	}
	r := Reach{At: map[int][]string{}}
	var kept []GraphLink
	for _, l := range g.Links {
		near := l.Target
		if nearIsSource {
			near = l.Source
		}
		if !inside[near] || !placeable[l.Source] || !placeable[l.Target] {
			continue
		}
		kept = append(kept, l)
	}
	folded, err := Fold(Graph{Nodes: g.Nodes, Links: kept}, tiers)
	if err != nil {
		return Reach{}, err
	}
	r.Drawn = folded
	for _, n := range folded.Nodes {
		r.At[n.Tier] = append(r.At[n.Tier], n.ID)
	}
	return r, nil
}

// Fold is the client's foldDocument: each ribbon of g to its ends' nearest
// ancestors at a drawn tier, ribbons between one pair of folded ends and of
// one kind merged into one with their cents summed as printed, a ribbon
// folding to one node dropped, and a node no folded ribbon touches pruned.
// Each node kept is re-parented to its nearest ancestor at a drawn tier that
// the fold kept, or to "" where none was, so the chart's own hierarchy is
// the one a later "inside" walks. Nodes come back sorted by id and links by
// source, target and kind, so two folds of one graph are one byte sequence.
//
// A RIBBON END WITH NO ANCESTOR AT A DRAWN TIER IS REFUSED, as foldDocument
// refuses it, rather than dropped: a caller that wants such ribbons left out
// filters them first, which is what [ReachOf] does and filterLinks does.
//
// It is the overview of a view with no Root -- shapeFor folds the document
// to the render tiers and filters nothing -- and the second half of every
// reach.
func Fold(g Graph, tiers []int) (Graph, error) {
	byID := indexNodes(g)
	chains, err := ancestry(g, byID)
	if err != nil {
		return Graph{}, err
	}
	drawn := drawnSet(tiers)
	foldTo := make(map[string]string, len(g.Nodes))
	for id, chain := range chains {
		for _, a := range chain {
			if drawn[a.Tier] {
				foldTo[id] = a.ID
				break
			}
		}
	}
	merged := map[[3]string]int64{}
	for _, l := range g.Links {
		fs, ok := foldTo[l.Source]
		if !ok {
			return Graph{}, fmt.Errorf("link %q -> %q: %q has no ancestor at a drawn tier %v", l.Source, l.Target, l.Source, tiers)
		}
		ft, ok := foldTo[l.Target]
		if !ok {
			return Graph{}, fmt.Errorf("link %q -> %q: %q has no ancestor at a drawn tier %v", l.Source, l.Target, l.Target, tiers)
		}
		if fs == ft {
			continue
		}
		merged[[3]string{fs, ft, l.Kind}] += l.ValueCents
	}
	touched := map[string]bool{}
	for ends := range merged {
		touched[ends[0]] = true
		touched[ends[1]] = true
	}
	var out Graph
	for _, id := range slices.Sorted(maps.Keys(touched)) {
		n := byID[id]
		n.Parent = ""
		for _, a := range chains[id][1:] {
			if drawn[a.Tier] && touched[a.ID] {
				n.Parent = a.ID
				break
			}
		}
		out.Nodes = append(out.Nodes, n)
	}
	keys := slices.SortedFunc(maps.Keys(merged), func(a, b [3]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]), cmp.Compare(a[2], b[2]))
	})
	for _, k := range keys {
		out.Links = append(out.Links, GraphLink{Source: k[0], Target: k[1], ValueCents: merged[k], Kind: k[2]})
	}
	return out, nil
}

func indexNodes(g Graph) map[string]GraphNode {
	byID := make(map[string]GraphNode, len(g.Nodes))
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	return byID
}

func drawnSet(tiers []int) map[int]bool {
	drawn := make(map[int]bool, len(tiers))
	for _, t := range tiers {
		drawn[t] = true
	}
	return drawn
}

// ancestry is the parent chain of every node up to maxHops, the node itself
// first, or an error on a parent the document does not carry, which is the
// one shape the client refuses to answer "no" for.
func ancestry(g Graph, byID map[string]GraphNode) (map[string][]GraphNode, error) {
	chains := make(map[string][]GraphNode, len(g.Nodes))
	for _, n := range g.Nodes {
		var chain []GraphNode
		at, ok := n, true
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
		chains[n.ID] = chain
	}
	return chains, nil
}
