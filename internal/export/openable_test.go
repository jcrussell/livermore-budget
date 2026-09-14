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

// TestOpenableNodesReadsTheRibbonOutOfANode is the mutation this whole
// declaration exists to survive.
//
// A FUND IS FED BY ITS GROUP AT A TIER THE STEP DRAWS, so "touches a link whose
// other end is a column of this step" is true of every fund in the column and
// would declare all sixty openable — the exact state the set exists to end,
// arrived at through the set itself. What windowFor asks the step's document for
// is the half AWAY from the kept flank, so the ribbon has to run outward.
//
// Measured on the committed corpus with the direction removed: fund/511 is
// declared openable, drillDown(fund/511) fails, and the reader is shown a
// refusal banner over a mark drawn with the open affordance.
func TestOpenableNodesReadsTheRibbonOutOfANode(t *testing.T) {
	v := View{Path: "index.html"}
	step := DrillStep{From: 3, Tiers: []int{2, 3, 4}, Keep: []int{2}}
	// THE THIRD RIBBON IS WHAT MAKES THE DIRECTION LOAD-BEARING, and it took a
	// mutation to find that out. `outward` already excludes the flank's own
	// columns, so a document whose only other ribbon into "shut" comes from the
	// GROUP is answered the same way with the direction removed -- which is what
	// the first version of this fixture was, and the mutation passed it. What
	// separates the two readings is a ribbon pointing INTO the opened node from
	// a column BEYOND it: `d -> shut` is a 4-to-3 link, and read without a
	// direction it declares "shut" openable on a ribbon windowFor will not draw.
	//
	// LATENT ON THE COMMITTED CORPUS AND SAID SO. Every document the site
	// publishes runs its ribbons coarse-to-fine across each window's centre, so
	// no shipped step can tell the two rules apart; department-spending's
	// cross-tab is the shape that could, and it prints 5 -> 4 where the step
	// that opens it keeps tier 2 on the left.
	raw := openableDoc(t, [2]string{"g", "opens"}, [2]string{"g", "shut"},
		[2]string{"opens", "d"}, [2]string{"d", "shut"})

	got, err := openableNodes(v, 0, step, "stem", raw)
	if err != nil {
		t.Fatalf("openableNodes: %v", err)
	}
	if diff := cmp.Diff([]string{"opens"}, got); diff != "" {
		t.Errorf("a left flank opens (-want +got):\n%s\n\"shut\" is fed by the group and pays "+
			"nothing on, so the chart has nothing to open it into", diff)
	}
}

// TestOpenableNodesReadsWhichEndTheFlankIsAt pins the arm a first draft got
// wrong, and it is wrong in a way no committed document would have shown.
//
// `centre == len(Keep)` is true of a LEFT flank by construction and true of the
// revenue-category step's RIGHT one by arithmetic — Keep {2}, Tiers {1,0,2},
// centre 1 — so a derived keptLeft read that window backwards. It did not go
// quiet: it refused the whole site with "fund-flows draws no ribbon from tier 0
// into tier(s) [1]". A document where the backwards reading would have SUCCEEDED
// is what this asserts against, so the arm is pinned by its answer rather than
// by the accident of which failure it produced.
func TestOpenableNodesReadsWhichEndTheFlankIsAt(t *testing.T) {
	v := View{Path: "index.html"}
	// Ribbons run both ways across the opened tier: "shut" pays the flank and
	// "opens" is paid by the fine column. Read as a left flank this document
	// would answer "shut"; read as the right flank it declares, "opens".
	raw := openableDoc(t, [2]string{"d", "opens"}, [2]string{"shut", "g"})
	got, err := openableNodes(v, 0, DrillStep{From: 3, Tiers: []int{4, 3, 2}, Keep: []int{2}},
		"stem", raw)
	if err != nil {
		t.Fatalf("openableNodes: %v", err)
	}
	if diff := cmp.Diff([]string{"opens"}, got); diff != "" {
		t.Errorf("a right flank opens (-want +got):\n%s", diff)
	}
}

// TestOpenableNodesDeclaresNoSetForAStepThatKeepsNothing keeps the absent key's
// one meaning.
//
// From and Tiers belong to different hierarchies on a step that keeps no flank
// — the transfers step's From is the spine's tier 0 and the document it draws
// has no tier 0 at all — so a set read there would close a rung that works.
func TestOpenableNodesDeclaresNoSetForAStepThatKeepsNothing(t *testing.T) {
	got, err := openableNodes(View{Path: "index.html"}, 0,
		DrillStep{From: 0, Tiers: []int{2, 3}}, "stem", openableDoc(t))
	if err != nil {
		t.Fatalf("openableNodes: %v", err)
	}
	if got != nil {
		t.Errorf("openableNodes = %v, want nil for a step that keeps no flank", got)
	}
}

// TestOpenableNodesRefusesAnEmptySetRatherThanShippingOne is what lets the
// client default open on an absent key.
//
// `opens` carries omitempty, so nil and an empty slice ship the same absent key.
// A window whose document decomposes nothing at the opened tier is a rung no
// reader could ever reach, and reporting it by name here is what keeps the two
// states from being one.
func TestOpenableNodesRefusesAnEmptySetRatherThanShippingOne(t *testing.T) {
	_, err := openableNodes(View{Path: "index.html"}, 2,
		DrillStep{From: 3, Tiers: []int{2, 3, 4}, Keep: []int{2}},
		"stem", openableDoc(t, [2]string{"g", "opens"}))
	if err == nil {
		t.Fatal("openableNodes accepted a document with nothing beyond the opened tier")
	}
	for _, want := range []string{"index.html", "step 2", "no reader could ever reach"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}
