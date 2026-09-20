package export

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestGapOfIsMarkGapsArithmetic is markGap's rule on a hand-written chart,
// each row a branch of it: no declaration draws nothing; a centre that
// balances draws nothing; too little leaving stands at the last tier with
// the ribbon running out of the centre, too little arriving at the first
// with the ribbon running in; and a difference the declaration does not
// name is refused rather than drawn.
//
// THE SUMS ARE SIGNED. The fourth row's centre takes 100 and sends 60
// forward and 40 as a reduction, which is a chart that balances as printed
// and reads as 80 short once the reduction is drawn at its magnitude; the
// row pins that this reads it as printed.
func TestGapOfIsMarkGapsArithmetic(t *testing.T) {
	chart := func(links ...GraphLink) Graph {
		return Graph{
			Nodes: []GraphNode{{ID: "a", Tier: 0}, {ID: "c", Tier: 1}, {ID: "p", Tier: 2}, {ID: "q", Tier: 2}},
			Links: links,
		}
	}
	tiers := []int{0, 1, 2}
	declared := map[string]string{"c": "declared reason"}
	cases := []struct {
		name  string
		drawn Graph
		gaps  map[string]string
		want  Carry
		ok    bool
		err   string
	}{
		{
			name:  "no declaration at all",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60}),
			gaps:  nil,
		},
		{
			name:  "a centre that balances",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60}, GraphLink{Source: "c", Target: "q", ValueCents: 40}),
			gaps:  declared,
		},
		{
			name:  "too little leaving",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60}),
			gaps:  declared,
			want: Carry{
				Mark:  Mark{ID: "gap/c", Role: RoleGap, Tier: 2, InCents: 40},
				Nodes: []GraphNode{{ID: "gap/c", Tier: 2, Role: RoleGap, Derived: true}},
				Links: []GraphLink{{Source: "c", Target: "gap/c", ValueCents: 40}},
			},
			ok: true,
		},
		{
			name:  "a reduction read as printed",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 140}, GraphLink{Source: "c", Target: "q", ValueCents: -40}),
			gaps:  declared,
		},
		{
			name:  "too little arriving",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 70}, GraphLink{Source: "c", Target: "p", ValueCents: 100}),
			gaps:  declared,
			want: Carry{
				Mark:  Mark{ID: "gap/c", Role: RoleGap, Tier: 0, OutCents: 30},
				Nodes: []GraphNode{{ID: "gap/c", Tier: 0, Role: RoleGap, Derived: true}},
				Links: []GraphLink{{Source: "gap/c", Target: "c", ValueCents: 30}},
			},
			ok: true,
		},
		{
			name:  "a difference the declaration does not name",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60}),
			gaps:  map[string]string{"q": "another node's reason"},
			err:   `a difference of 40 cents that no declaration on this step accounts for`,
		},
		{
			name:  "a reason declared empty",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60}),
			gaps:  map[string]string{"c": ""},
			err:   `no declaration on this step accounts for`,
		},
		{
			name:  "a centre the chart does not draw",
			drawn: chart(GraphLink{Source: "a", Target: "p", ValueCents: 100}),
			gaps:  map[string]string{"x": "reason"},
			err:   `"x" is not a mark of the drawn chart`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opened := "c"
			if tc.name == "a centre the chart does not draw" {
				opened = "x"
			}
			got, ok, err := GapOf(tc.drawn, opened, tiers, tc.gaps)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want one containing %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("GapOf mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMarkIDsAreTwoPrefixes pins that a residual's id and a gap's id cannot
// be mistaken for each other or for a document node's: a check counting one
// must not find the other.
func TestMarkIDsAreTwoPrefixes(t *testing.T) {
	r, g := ResidualID("fund-group/general"), GapID("expenditure/services-and-supplies")
	if !IsResidual(r) || IsGap(r) {
		t.Errorf("%q is a residual and nothing else", r)
	}
	if !IsGap(g) || IsResidual(g) {
		t.Errorf("%q is a gap and nothing else", g)
	}
	if IsResidual("fund-group/general") || IsGap("fund-group/general") {
		t.Error("a document node is neither mark")
	}
}

// TestResidualOfIsCarryResiduals is carryResidual's rule on a hand-written
// pair of documents in the fund-group step's shape: a chart above that
// prints a group's inflow and outflow whole, and a step document that
// prints the same money by fund and carries no row for a draw or a
// transfer out. Each row is one branch of the rule.
func TestResidualOfIsCarryResiduals(t *testing.T) {
	// The chart above: revenue r, a draw e1 and transfers in tin at tier 0
	// feed group G at tier 2, which sends out to tier 5 and both ways to b.
	from := Graph{
		Nodes: []GraphNode{
			{ID: "r", Tier: 0}, {ID: "e1", Tier: 0, Role: "draw", Derived: true}, {ID: "tin", Tier: 0},
			{ID: "G", Tier: 2}, {ID: "b", Tier: 0}, {ID: "out", Tier: 5, Role: "transfer_out"},
		},
		Links: []GraphLink{
			{Source: "r", Target: "G", ValueCents: 100}, {Source: "e1", Target: "G", ValueCents: 10, Kind: "a"},
			{Source: "e1", Target: "G", ValueCents: 10, Kind: "b"}, {Source: "tin", Target: "G", ValueCents: 5},
			{Source: "G", Target: "out", ValueCents: 7}, {Source: "b", Target: "G", ValueCents: 3}, {Source: "G", Target: "b", ValueCents: 4},
		},
	}
	// The step document: G's funds f1 and f2 at tier 3, a division under f1
	// at tier 4, and transfers in decomposed to f2 -- so tin is never
	// carried, and out and b are carried only where the window decomposes G.
	doc := Graph{
		Nodes: []GraphNode{
			{ID: "r", Tier: 0}, {ID: "tin", Tier: 0}, {ID: "G", Tier: 2},
			{ID: "f1", Tier: 3, Parent: "G"}, {ID: "f2", Tier: 3, Parent: "G"}, {ID: "d1", Tier: 4, Parent: "f1"},
		},
		Links: []GraphLink{
			{Source: "r", Target: "f1", ValueCents: 60}, {Source: "r", Target: "f2", ValueCents: 40},
			{Source: "tin", Target: "f2", ValueCents: 5}, {Source: "f1", Target: "d1", ValueCents: 30},
		},
	}
	// The window at {0,2,3}: the kept flank's ribbons into G and the fresh
	// half's into its funds. e1's two ribbons are value-equal but for kind.
	window := Graph{
		Nodes: []GraphNode{
			{ID: "r", Tier: 0}, {ID: "e1", Tier: 0, Role: "draw", Derived: true}, {ID: "tin", Tier: 0}, {ID: "b", Tier: 0},
			{ID: "G", Tier: 2}, {ID: "f1", Tier: 3, Parent: "G"}, {ID: "f2", Tier: 3, Parent: "G"},
		},
		Links: []GraphLink{
			{Source: "b", Target: "G", ValueCents: 3}, {Source: "e1", Target: "G", ValueCents: 10, Kind: "a"},
			{Source: "e1", Target: "G", ValueCents: 10, Kind: "b"}, {Source: "r", Target: "G", ValueCents: 100},
			{Source: "tin", Target: "G", ValueCents: 5}, {Source: "G", Target: "f1", ValueCents: 60}, {Source: "G", Target: "f2", ValueCents: 40},
		},
	}
	// The same window widened to tier 4, where f1's division is drawn and
	// so G is decomposed.
	wide := Graph{
		Nodes: append(slices.Clone(window.Nodes), GraphNode{ID: "d1", Tier: 4, Parent: "f1"}),
		Links: append(slices.Clone(window.Links), GraphLink{Source: "f1", Target: "d1", ValueCents: 30}),
	}
	declared := map[string]string{"e1": "why", "tin": "why", "out": "why", "b": "why"}
	mark := func(tier int, in, out int64, ends ...string) Mark {
		return Mark{ID: "residual/G", Role: RoleResidual, Tier: tier, InCents: in, OutCents: out, Ends: ends}
	}
	node := GraphNode{ID: "residual/G", Tier: 3, Role: RoleResidual, Parent: "G", Derived: true}
	cases := []struct {
		name     string
		drawn    Graph
		tiers    []int
		residual map[string]string
		want     Carry
		ok       bool
		err      string
	}{
		{name: "no declaration", drawn: window, tiers: []int{0, 2, 3}},
		{
			// tin is decomposed to f2 and stays; out and b's outflow wait
			// on a window that decomposes G; e1 and b's inflow are carried,
			// e1's two ribbons spliced by index.
			name: "the inflow alone, the group not decomposed", drawn: window, tiers: []int{0, 2, 3}, residual: declared,
			want: Carry{
				Mark:  mark(3, 23, 0, "b", "e1"),
				Nodes: []GraphNode{node},
				Links: []GraphLink{
					{Source: "b", Target: "residual/G", ValueCents: 3},
					{Source: "e1", Target: "residual/G", ValueCents: 10, Kind: "a"},
					{Source: "e1", Target: "residual/G", ValueCents: 10, Kind: "b"},
				},
				Splice: []int{0, 1, 2},
			},
			ok: true,
		},
		{
			// The window draws no ribbon out of G, so out's and b's outflow
			// come off the chart above, out arrives as a node at the last
			// tier, and b -- on both sides -- keeps its drawn place.
			name: "the outflow through the chart above once the group is decomposed", drawn: wide, tiers: []int{0, 2, 3, 4}, residual: declared,
			want: Carry{
				Mark:  mark(3, 23, 11, "b", "e1", "out"),
				Nodes: []GraphNode{{ID: "out", Tier: 4, Role: "transfer_out"}, node},
				Links: []GraphLink{
					{Source: "b", Target: "residual/G", ValueCents: 3},
					{Source: "residual/G", Target: "b", ValueCents: 4},
					{Source: "e1", Target: "residual/G", ValueCents: 10, Kind: "a"},
					{Source: "e1", Target: "residual/G", ValueCents: 10, Kind: "b"},
					{Source: "residual/G", Target: "out", ValueCents: 7},
				},
				Splice: []int{0, 1, 2},
			},
			ok: true,
		},
		{
			// An endpoint not yet drawn and on both sides is placed by its
			// outflow, the client's last write.
			name: "an endpoint on both sides stands where its outflow puts it", tiers: []int{0, 2, 3, 4},
			drawn: Graph{Nodes: slices.DeleteFunc(slices.Clone(wide.Nodes), func(n GraphNode) bool { return n.ID == "b" }),
				Links: slices.DeleteFunc(slices.Clone(wide.Links), func(l GraphLink) bool { return l.Source == "b" })},
			residual: map[string]string{"b": "why"},
			want: Carry{
				Mark:  mark(3, 3, 4, "b"),
				Nodes: []GraphNode{{ID: "b", Tier: 4}, node},
				Links: []GraphLink{{Source: "b", Target: "residual/G", ValueCents: 3}, {Source: "residual/G", Target: "b", ValueCents: 4}},
			},
			ok: true,
		},
		{name: "nothing to carry draws nothing", drawn: window, tiers: []int{0, 2, 3}, residual: map[string]string{"tin": "why"}},
		{name: "no part at a declared tier", drawn: window, tiers: []int{0, 2}, residual: declared, err: `"G" has no part at a tier this step draws`},
		{name: "a node the document does not carry", drawn: window, tiers: []int{0, 2, 3}, residual: declared, err: `does not carry node "X"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opened := "G"
			if strings.HasPrefix(tc.name, "a node the document") {
				opened = "X"
			}
			got, ok, err := ResidualOf(tc.drawn, from, doc, opened, tc.tiers, tc.residual)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want one containing %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ResidualOf mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
