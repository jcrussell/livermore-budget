package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// openableDoc builds the smallest document openableNodes can be asked about:
// a coarse column, two nodes at the opened tier, and a fine column only one of
// them reaches.
func openableDoc(t *testing.T, links ...[2]string) []byte {
	t.Helper()
	type node struct {
		ID   string `json:"id"`
		Tier int    `json:"tier"`
	}
	type link struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	doc := struct {
		Nodes []node `json:"nodes"`
		Links []link `json:"links"`
	}{
		Nodes: []node{{"g", 2}, {"opens", 3}, {"shut", 3}, {"d", 4}},
	}
	for _, l := range links {
		doc.Links = append(doc.Links, link{l[0], l[1]})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// TestOpenableNodesReadsTheRibbonOutOfANode: a fund is fed by its group at a
// tier the step draws, so a reading without direction would declare every fund
// openable. windowFor asks for the half away from the kept flank.
func TestOpenableNodesReadsTheRibbonOutOfANode(t *testing.T) {
	v := View{Path: "index.html"}
	step := DrillStep{From: 3, Tiers: []int{2, 3, 4}, Keep: []int{2}}
	// The third ribbon, `d -> shut` from a column beyond the opened node, is what
	// makes the direction load-bearing: without it the mutation passes. Latent on
	// the committed corpus, whose ribbons all run coarse-to-fine across a centre.
	raw := openableDoc(t, [2]string{"g", "opens"}, [2]string{"g", "shut"},
		[2]string{"opens", "d"}, [2]string{"d", "shut"})

	got, err := openableNodes(v, 0, step, "stem", raw, nil)
	if err != nil {
		t.Fatalf("openableNodes: %v", err)
	}
	if diff := cmp.Diff([]string{"opens"}, got); diff != "" {
		t.Errorf("a left flank opens (-want +got):\n%s\n\"shut\" is fed by the group and pays "+
			"nothing on, so the chart has nothing to open it into", diff)
	}
}

// TestOpenableNodesReadsWhichEndTheFlankIsAt: `centre == len(Keep)` is true of
// a left flank and, by arithmetic, of the revenue-category step's right one
// (Keep {2}, Tiers {1,0,2}). This document is one where the backwards reading
// would succeed, so the arm is pinned by its answer.
func TestOpenableNodesReadsWhichEndTheFlankIsAt(t *testing.T) {
	v := View{Path: "index.html"}
	// Ribbons run both ways across the opened tier: "shut" pays the flank and
	// "opens" is paid by the fine column. Read as a left flank this document
	// would answer "shut"; read as the right flank it declares, "opens".
	raw := openableDoc(t, [2]string{"d", "opens"}, [2]string{"shut", "g"})
	got, err := openableNodes(v, 0, DrillStep{From: 3, Tiers: []int{4, 3, 2}, Keep: []int{2}},
		"stem", raw, nil)
	if err != nil {
		t.Fatalf("openableNodes: %v", err)
	}
	if diff := cmp.Diff([]string{"opens"}, got); diff != "" {
		t.Errorf("a right flank opens (-want +got):\n%s", diff)
	}
}

// hierarchyDoc is the transfers document's shape at its smallest: a root no
// ribbon touches, whose two children at tier 2 each pay one fund at tier 3,
// and a fund that pays on to a tier the step does not draw.
func hierarchyDoc(t *testing.T) []byte {
	t.Helper()
	type node struct {
		ID     string `json:"id"`
		Tier   int    `json:"tier"`
		Parent string `json:"parent,omitempty"`
	}
	type link struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	raw, err := json.Marshal(struct {
		Nodes []node `json:"nodes"`
		Links []link `json:"links"`
	}{
		Nodes: []node{{"root", 0, ""}, {"payer-a", 2, "root"}, {"payer-b", 2, "root"},
			{"fund-x", 3, ""}, {"fund-y", 3, ""}, {"far", 5, ""}},
		Links: []link{{"payer-a", "fund-x"}, {"payer-b", "fund-y"}, {"fund-x", "far"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// TestOpenableNodesReadsAStepThatKeepsNothingThroughTheHierarchy: "root"
// touches no ribbon, as transfers/in touches none, and is decomposed by its
// children, so a ribbon reading would never offer it. SideSource answers the
// nodes whose subtree pays, "" those whose subtree is paid. "fund-x" pays "far",
// which no drawn tier places, so it is in neither set.
func TestOpenableNodesReadsAStepThatKeepsNothingThroughTheHierarchy(t *testing.T) {
	v := View{Path: "index.html"}
	cases := []struct {
		name string
		step DrillStep
		want []string
	}{
		{"the paying side", DrillStep{From: 0, Tiers: []int{2, 3}, Side: SideSource}, []string{"payer-a", "payer-b", "root"}},
		{"the paid side", DrillStep{From: 0, Tiers: []int{2, 3}}, []string{"fund-x", "fund-y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := openableNodes(v, 0, tc.step, "stem", hierarchyDoc(t), nil)
			if err != nil {
				t.Fatalf("openableNodes: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("a step that keeps nothing opens (-want +got):\n%s", diff)
			}
		})
	}
	// A step whose tiers place no end of any ribbon is refused here too.
	_, err := openableNodes(v, 3, DrillStep{From: 0, Tiers: []int{7, 8}, Side: SideSource}, "stem", hierarchyDoc(t), nil)
	if err == nil {
		t.Fatal("openableNodes accepted a step whose tiers place nothing")
	}
	for _, want := range []string{"index.html", "step 3", "no reader could ever reach"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestOpenableNodesRefusesAnEmptySetRatherThanShippingOne: `opens` carries
// omitempty, so an empty set would ship as the absent key the client defaults
// open on.
func TestOpenableNodesRefusesAnEmptySetRatherThanShippingOne(t *testing.T) {
	_, err := openableNodes(View{Path: "index.html"}, 2,
		DrillStep{From: 3, Tiers: []int{2, 3, 4}, Keep: []int{2}},
		"stem", openableDoc(t, [2]string{"g", "opens"}), nil)
	if err == nil {
		t.Fatal("openableNodes accepted a document with nothing beyond the opened tier")
	}
	for _, want := range []string{"index.html", "step 2", "no reader could ever reach"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}
