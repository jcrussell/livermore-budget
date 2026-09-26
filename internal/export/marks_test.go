package export

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestGapOfIsMarkGapsArithmetic is markGap's rule on a hand-written chart,
// each row a branch of it: no declaration draws nothing; a centre that
// balances draws nothing; too little leaving stands at the last tier with
// the ribbon running out of the centre, too little arriving at the first
// with the ribbon running in; and a difference no licence names at this
// column and this figure is refused rather than drawn, as is a licence on a
// centre that balances.
//
// THE SUMS ARE SIGNED. The fourth row's centre takes 100 and sends 60
// forward and 40 as a reduction, which is a chart that balances as printed
// and reads as 80 short once the reduction is drawn at its magnitude; the
// row pins that this reads it as printed.
func TestGapOfIsMarkGapsArithmetic(t *testing.T) {
	chart := func(links ...GraphLink) Graph {
		return Graph{
			Nodes: []GraphNode{{ID: "a", Tier: 0}, {ID: "c", Tier: 1, Label: "Centre"}, {ID: "p", Tier: 2}, {ID: "q", Tier: 2}},
			Links: links,
		}
	}
	// The two documents the totals are read from, each citing its own page.
	cite := func(page int) []Locator { return []Locator{{DocID: "d", Pages: []int{page}}} }
	from := Graph{Nodes: []GraphNode{{ID: "a"}, {ID: "c"}, {ID: "z"}}, Links: []GraphLink{
		{Source: "a", Target: "c", Locators: cite(67)}, {Source: "a", Target: "z", Locators: cite(1)}}}
	doc := Graph{Nodes: []GraphNode{{ID: "c"}, {ID: "p"}}, Links: []GraphLink{{Source: "c", Target: "p", Locators: cite(90)}}}
	col := ColumnKey{FiscalYear: 2027, Basis: "adopted", Label: "FY 2026-27"}
	licence := func(cents int64) map[string]Gaps {
		return map[string]Gaps{"c": {{FiscalYear: 2026, Basis: "adopted", Cents: 5, Reason: "Another column's reason."},
			{FiscalYear: 2027, Basis: "adopted", Cents: cents, Reason: "Declared reason."}}}
	}
	short := chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60})
	cases := []struct {
		name  string
		drawn Graph
		gaps  map[string]Gaps
		want  Carry
		ok    bool
		err   string
	}{
		{
			name:  "no declaration at all",
			drawn: short,
			gaps:  nil,
		},
		{
			name:  "a centre that balances",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 60}, GraphLink{Source: "c", Target: "q", ValueCents: 40}),
			gaps:  map[string]Gaps{"q": {{FiscalYear: 2027, Basis: "adopted", Cents: 1, Reason: "Another node's reason."}}},
		},
		{
			name:  "too little leaving",
			drawn: short,
			gaps:  licence(40),
			want: Carry{
				Mark:  Mark{ID: "gap/c", Role: RoleGap, Tier: 2, InCents: 40, Locators: []Locator{{DocID: "d", Pages: []int{67, 90}}}},
				Nodes: []GraphNode{{ID: "gap/c", Tier: 2, Role: RoleGap, Derived: true}},
				Links: []GraphLink{{Source: "c", Target: "gap/c", ValueCents: 40}},
			},
			ok: true,
		},
		{
			name:  "a reduction read as printed",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 140}, GraphLink{Source: "c", Target: "q", ValueCents: -40}),
			gaps:  map[string]Gaps{"q": {{FiscalYear: 2027, Basis: "adopted", Cents: 1, Reason: "Another node's reason."}}},
		},
		{
			name:  "too little arriving",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 70}, GraphLink{Source: "c", Target: "p", ValueCents: 100}),
			gaps:  licence(-30),
			want: Carry{
				Mark:  Mark{ID: "gap/c", Role: RoleGap, Tier: 0, OutCents: 30, Locators: []Locator{{DocID: "d", Pages: []int{67, 90}}}},
				Nodes: []GraphNode{{ID: "gap/c", Tier: 0, Role: RoleGap, Derived: true}},
				Links: []GraphLink{{Source: "gap/c", Target: "c", ValueCents: 30}},
			},
			ok: true,
		},
		{
			name:  "a difference the declaration does not name",
			drawn: short,
			gaps:  map[string]Gaps{"q": {{FiscalYear: 2027, Basis: "adopted", Cents: 40, Reason: "Another node's reason."}}},
			err:   `a difference of 40 cents that no declaration on this step accounts for in FY2027 adopted`,
		},
		{
			name:  "a reason declared empty",
			drawn: short,
			gaps:  map[string]Gaps{"c": {{FiscalYear: 2027, Basis: "adopted", Cents: 40}}},
			err:   `no declaration on this step accounts for`,
		},
		{
			name:  "a licence in another column only",
			drawn: short,
			gaps:  map[string]Gaps{"c": {{FiscalYear: 2026, Basis: "adopted", Cents: 40, Reason: "Another column's reason."}}},
			err:   `no declaration on this step accounts for in FY2027 adopted`,
		},
		{
			name:  "a licence at another figure",
			drawn: short,
			gaps:  licence(41),
			err:   `declares a gap of 41 cents on "c" in FY2027 adopted and the charts differ there by 40`,
		},
		{
			name:  "a licence of the wrong sign",
			drawn: short,
			gaps:  licence(-40),
			err:   `declares a gap of -40 cents on "c" in FY2027 adopted and the charts differ there by 40`,
		},
		{
			name:  "a licence in this year on another basis",
			drawn: short,
			gaps:  map[string]Gaps{"c": {{FiscalYear: 2027, Basis: "revised", Cents: 40, Reason: "Another basis's reason."}}},
			err:   `no declaration on this step accounts for in FY2027 adopted`,
		},
		{
			name:  "a licence on a centre that balances",
			drawn: chart(GraphLink{Source: "a", Target: "c", ValueCents: 100}, GraphLink{Source: "c", Target: "p", ValueCents: 100}),
			gaps:  licence(40),
			err:   `declares a gap of 40 cents on "c" in FY2027 adopted and the chart balances there`,
		},
		{
			name:  "a centre the chart does not draw",
			drawn: chart(GraphLink{Source: "a", Target: "p", ValueCents: 100}),
			gaps:  map[string]Gaps{"x": {{FiscalYear: 2027, Basis: "adopted", Reason: "Reason."}}},
			err:   `"x" is not a mark of the drawn chart`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opened := "c"
			if tc.name == "a centre the chart does not draw" {
				opened = "x"
			}
			got, ok, err := GapOf(tc.drawn, from, doc, col, opened, tiers3, tc.gaps)
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
			// THE ARITHMETIC AND THE WORDS ARE ASSERTED APART. The struct
			// diff is about which mark exists, where it stands, what it is
			// worth and what it cites; a reworded sentence showing up as a
			// changed rule would train a reader to re-baseline this diff
			// without reading it.
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(Mark{},
				"Label", "Rationale", "SourceNote")); diff != "" {
				t.Errorf("GapOf mismatch (-want +got):\n%s", diff)
			}
			if !tc.ok {
				return
			}
			if got.Mark.Label != "Difference between the two schedules" {
				t.Errorf("label = %q", got.Mark.Label)
			}
			// THE FIGURES IN THE SENTENCE ARE THE MARK'S OWN, which is the half
			// a fixed-string comparison would not catch: a rationale quoting
			// the wrong side of the difference reads perfectly. The column is
			// named in the page's own words, the reason is this column's and
			// not another's, and a shortfall is not called a surplus.
			var into, outOf int64
			for _, l := range tc.drawn.Links {
				if l.Target == opened {
					into += l.ValueCents
				}
				if l.Source == opened {
					outOf += l.ValueCents
				}
			}
			side := " less."
			if into < outOf {
				side = " more than the " + dollars(into)
			}
			for _, want := range []string{"In FY 2026-27 adopted, ", dollars(into), dollars(outOf), "Centre", "Declared reason.", side} {
				if !strings.Contains(got.Mark.Rationale, want) {
					t.Errorf("rationale %q does not carry %q", got.Mark.Rationale, want)
				}
			}
			for _, refused := range []string{"Another column's reason.", ".00", "fisc-"} {
				if strings.Contains(got.Mark.Rationale, refused) {
					t.Errorf("rationale %q carries %q", got.Mark.Rationale, refused)
				}
			}
			if !strings.Contains(got.Mark.SourceNote, "Derived, not published") {
				t.Errorf("source note does not say the figure is derived: %q", got.Mark.SourceNote)
			}
		})
	}
}

var tiers3 = []int{0, 1, 2}

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
		{name: "a carried endpoint with no reason", drawn: window, tiers: []int{0, 2, 3}, residual: map[string]string{"e1": " "}, err: `"e1" is carried onto the residual mark with no reason`},
		{name: "a node the document does not carry", drawn: window, tiers: []int{0, 2, 3}, residual: declared, err: `does not carry node "X"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opened := "G"
			if strings.HasPrefix(tc.name, "a node the document") {
				opened = "X"
			}
			got, ok, err := ResidualOf(tc.drawn, from, doc, opened, tc.tiers, tc.residual, "fund")
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
			// The arithmetic and the words apart, as the gap's cases are.
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(Mark{},
				"Label", "Rationale", "SourceNote")); diff != "" {
				t.Errorf("ResidualOf mismatch (-want +got):\n%s", diff)
			}
			if !tc.ok {
				return
			}
			// THE GRAIN IS THE STEP'S AND IS SAID TWICE, so a mark carrying the
			// label of one grain and the rationale of another cannot pass. That
			// was reachable while the client hard-coded the label and derived
			// nothing from the step.
			if got.Mark.Label != "Not broken down by fund" {
				t.Errorf("label = %q, want it named for the declared grain", got.Mark.Label)
			}
			if !strings.Contains(got.Mark.Rationale, "does not split by fund") ||
				!strings.Contains(got.Mark.Rationale, "no fund here receives or pays it") {
				t.Errorf("rationale does not say the grain the label names: %q", got.Mark.Rationale)
			}
			// AND EVERY DECLARED ENDPOINT'S REASON REACHES IT, in the step's own
			// words. A rationale naming the endpoints and dropping a reason
			// would read as complete.
			for _, e := range got.Mark.Ends {
				if why := tc.residual[e]; why != "" && !strings.Contains(got.Mark.Rationale, why) {
					t.Errorf("rationale drops %s's declared reason %q", e, why)
				}
			}
			// THE RESIDUAL'S NOTE IS THE CLIENT'S, because it renders the
			// citations of the flows it carries and neither Graph nor this
			// package's walk decodes a locator. fisc-tihl.
			if got.Mark.SourceNote != "" {
				t.Errorf("source note = %q, want empty: the residual's note is the client's", got.Mark.SourceNote)
			}
		})
	}
}
