package export

import (
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
	if !IsResidual(r) || IsGap(r) || IsAggregate(r) {
		t.Errorf("%q is a residual and nothing else", r)
	}
	if !IsGap(g) || IsResidual(g) || IsAggregate(g) {
		t.Errorf("%q is a gap and nothing else", g)
	}
	if IsResidual("fund-group/general") || IsGap("fund-group/general") {
		t.Error("a document node is neither mark")
	}
}
