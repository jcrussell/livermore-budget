package project

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/schema"
)

// committedDocs is every graph the committed store builds for one adopted
// column, by projection name.
func committedDocs(t *testing.T, year int) map[string]*Document {
	t.Helper()
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*Document{}
	for _, p := range Registry(store.reg, structure.BudgetBookExceptions()) {
		g, ok := p.(interface {
			Projection
			Document([]fact.Fact, Options) (*Document, error)
		})
		if !ok {
			continue
		}
		for _, o := range g.Slices(store.facts, "expansion test") {
			if o.Columns[0] != (Column{FiscalYear: year, Basis: vocab.BasisAdopted}) {
				continue
			}
			d, err := g.Document(store.facts, o)
			if err != nil {
				t.Fatalf("%s FY%d: %v", p.Name(), year, err)
			}
			out[p.Name()] = d
		}
	}
	return out
}

// fundGroupOf is the fund group data/funds.yaml types a fund id in.
func fundGroupOf(t *testing.T, id string) string {
	t.Helper()
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, PrefixFund))
	if err != nil {
		t.Fatalf("%q is no fund id", id)
	}
	g, ok := store.reg.FundType(n)
	if !ok {
		t.Fatalf("fund %d is in no data/funds.yaml entry", n)
	}
	return PrefixFundGroup + g
}

// leavesOf is a stitch's links less its hub legs ([IsLeg]), each read at the
// ends of the leaf it repeats: where a page prints it, wherever the stitch
// draws it.
func leavesOf(d *Document) []Link {
	nodes := map[string]Node{}
	for _, n := range d.Nodes {
		nodes[n.ID] = n
	}
	sides := PrimarySides(d.Splits)
	var out []Link
	for _, l := range d.Links {
		if l.Repeats == "" {
			out = append(out, l)
			continue
		}
		if IsLeg(l, nodes, sides) {
			continue
		}
		l.Source, l.Target = LeafEnds(l.Repeats)
		out = append(out, l)
	}
	return out
}

// TestTheContestedRemainderIsTheExceptions: FY2026-27's 250,000 internal
// service Services & Supplies remainder is the declaring exception's, drawn to
// a row of the group's own, and the divisions alternative carries its sibling.
func TestTheContestedRemainderIsTheExceptions(t *testing.T) {
	byName := map[string]structure.Exception{}
	for _, e := range structure.BudgetBookExceptions() {
		byName[e.Name] = e
	}
	for _, c := range []struct {
		year   int
		want   map[string]int64
		absent bool
	}{
		{year: 2026, absent: true},
		{year: 2027, want: map[string]int64{
			"fund-group-only/internal-service -> expenditure/fund-group/internal-service/services-and-supplies": byName["p0067-internal-service-is-250000-high-by-fund"].Residual,
			"expenditure/services-and-supplies -> no-division/services-and-supplies":                            byName["p0067-internal-service-is-250000-high-by-object"].Residual,
			"fund-balance/draw -> fund-group-only/internal-service":                                             -byName["p0067-internal-service-ending-is-250000-low"].Residual,
		}},
	} {
		x := committedDocs(t, c.year)[ExpansionProjection]
		got := map[string]int64{}
		for _, l := range leavesOf(x) {
			if (IsRemainder(l.Source) || IsRemainder(l.Target)) && len(l.FactIDs) > 1 {
				if !l.Derived {
					t.Errorf("FY%d %s -> %s carries a difference and is not derived", c.year, l.Source, l.Target)
				}
				got[l.Source+" -> "+l.Target] = l.ValueCents
			}
		}
		if c.absent && len(got) > 0 {
			t.Errorf("FY%d draws remainders of a difference %v, and every cell of FY2025-26 ties", c.year, got)
		}
		for k, v := range c.want {
			if v == 0 || got[k] != v {
				t.Errorf("FY%d %s is %d, want the exception's %d", c.year, k, got[k], v)
			}
		}
	}
}

// TestAFundDrawnApartFromItsChangeIsToldSo: each fund of a group whose change
// is drawn whole is a caveat target, nets to its own change, and carries
// fund-sources-uses' balances and derived change.
func TestAFundDrawnApartFromItsChangeIsToldSo(t *testing.T) {
	for _, year := range []int{2026, 2027} {
		docs := committedDocs(t, year)
		exp, fsu := docs[ExpansionProjection], docs[FundSourcesUsesProjection]
		drawn := map[string]Node{}
		for _, n := range exp.Nodes {
			drawn[n.ID] = n
		}
		fsuNodes := map[string]Node{}
		for _, n := range fsu.Nodes {
			fsuNodes[n.ID] = n
		}
		// A group is drawn whole where its change lands on its remainder and
		// no fund of it draws its own: a one-way group's remainder carries
		// only what pp.66-67 print beyond its funds' changes.
		whole, byFund := map[string]bool{}, map[string]bool{}
		for _, l := range leavesOf(exp) {
			switch {
			case l.Source == NodeFundBalanceDraw && strings.HasPrefix(l.Target, PrefixGroupOnly):
				whole[PrefixFundGroup+strings.TrimPrefix(l.Target, PrefixGroupOnly)] = true
			case l.Target == NodeFundBalanceContribution && strings.HasPrefix(l.Source, PrefixGroupOnly):
				whole[PrefixFundGroup+strings.TrimPrefix(l.Source, PrefixGroupOnly)] = true
			case l.Source == NodeFundBalanceDraw && strings.HasPrefix(l.Target, PrefixFund):
				byFund[fundGroupOf(t, l.Target)] = true
			case l.Target == NodeFundBalanceContribution && strings.HasPrefix(l.Source, PrefixFund):
				byFund[fundGroupOf(t, l.Source)] = true
			}
		}
		for g := range byFund {
			delete(whole, g)
		}
		change := map[string]int64{}
		var want []string
		for _, l := range fsu.Links {
			fund, signed := l.Source, l.ValueCents
			switch {
			case l.Source == NodeFundBalanceDraw:
				fund, signed = l.Target, -l.ValueCents
			case l.Target != NodeFundBalanceContribution:
				continue
			}
			change[fund] = signed
			if _, ok := drawn[fund]; ok && whole[fundGroupOf(t, fund)] {
				want = append(want, fund)
			}
		}
		slices.Sort(want)
		if len(want) == 0 {
			t.Fatalf("FY%d: no drawn fund is of a group drawn whole, so this asserts nothing", year)
		}

		var got []string
		for _, c := range exp.Metadata.Caveats {
			if c.ID != "some-fund-group-money-is-not-by-fund" {
				continue
			}
			for _, id := range c.AppliesTo {
				if strings.HasPrefix(id, PrefixFund) {
					got = append(got, id)
				}
			}
			if !strings.Contains(c.Text, "in and out differ by its own change in balance") {
				t.Errorf("FY%d: the caveat's text does not say a fund's in and out differ: %q", year, c.Text)
			}
		}
		if !slices.Equal(want, got) {
			t.Errorf("FY%d: the caveat reaches funds %v, want the drawn funds of the groups drawn whole %v", year, got, want)
		}

		flow := map[string]int64{}
		for _, l := range leavesOf(exp) {
			if l.Alternative != "" {
				continue
			}
			flow[l.Target] += l.ValueCents
			flow[l.Source] -= l.ValueCents
		}
		for _, id := range want {
			if flow[id] != change[id] {
				t.Errorf("FY%d %s: in less out is %d and its change is %d", year, id, flow[id], change[id])
			}
		}

		balanced, changed := 0, 0
		for _, n := range exp.Nodes {
			if !strings.HasPrefix(n.ID, PrefixFund) {
				continue
			}
			src := fsuNodes[n.ID].Balances
			if diff := cmp.Diff(src, n.Balances); diff != "" {
				t.Errorf("FY%d %s: balances are not fund-sources-uses' (-want +got):\n%s", year, n.ID, diff)
				continue
			}
			if n.Balances == nil {
				continue
			}
			balanced++
			c := n.Balances.Change
			if c == nil {
				continue
			}
			changed++
			if !c.Derived || c.Rationale == "" || c.SourceNote == "" {
				t.Errorf("FY%d %s: change %+v is not marked derived with its words", year, n.ID, c)
			}
			if c.ValueCents != change[n.ID] {
				t.Errorf("FY%d %s: change is %d and fund-sources-uses draws %d", year, n.ID, c.ValueCents, change[n.ID])
			}
			cites := []string{n.Balances.Beginning.FactID, n.Balances.Ending.FactID}
			slices.Sort(cites)
			if !slices.Equal(cites, c.FactIDs) {
				t.Errorf("FY%d %s: change cites %v, want its balances' %v", year, n.ID, c.FactIDs, cites)
			}
		}
		for _, id := range want {
			if b := drawn[id].Balances; b == nil || b.Beginning == nil || b.Ending == nil || b.Change == nil {
				t.Errorf("FY%d %s: drawn apart from its change and carries balances %+v", year, id, b)
			}
		}
		t.Logf("FY%d: %d funds drawn apart from their change carry the caveat; %d drawn funds carry balances, %d a change",
			year, len(want), balanced, changed)
	}
}

// TestAFundGroupsLegsCarryEachFundsInflow: a group's hub legs follow each fund's
// inflow, so a fund that only takes in (211, 280, 300) is reached.
func TestAFundGroupsLegsCarryEachFundsInflow(t *testing.T) {
	for _, year := range []int{2026, 2027} {
		x := committedDocs(t, year)[ExpansionProjection]
		in, out := map[string]int64{}, map[string]int64{}
		for _, l := range leavesOf(x) {
			if l.Alternative == "" {
				in[l.Target] += l.ValueCents
				out[l.Source] += l.ValueCents
			}
		}
		parent := map[string]string{}
		for _, n := range x.Nodes {
			parent[n.ID] = n.Parent
		}
		legs := map[string]int64{}
		for _, l := range x.Links {
			if l.Repeats != "" && strings.HasPrefix(l.Source, PrefixFundGroup) && parent[l.Target] == l.Source {
				legs[l.Target] += l.ValueCents
			}
		}
		var unreached []string
		for _, n := range x.Nodes {
			if !strings.HasPrefix(n.Parent, PrefixFundGroup) || n.Alternative != "" {
				continue
			}
			if legs[n.ID] != in[n.ID] {
				t.Errorf("FY%d %s: its group's legs carry %d and it takes in %d", year, n.ID, legs[n.ID], in[n.ID])
			}
			if in[n.ID] == 0 {
				unreached = append(unreached, n.ID)
			}
		}
		for _, id := range []string{"fund/211", "fund/280", "fund/300"} {
			if out[id] != 0 || legs[id] == 0 {
				t.Errorf("FY%d %s gives out %d and its group's legs carry %d, want an inflow-only fund reached", year, id, out[id], legs[id])
			}
		}
		t.Logf("FY%d: members a fund group reaches by no leg, nothing arriving at them: %v", year, unreached)
	}
}

// TestASourcesLegsRunFromItsMembers: a hub split whose node takes nothing in
// draws legs member -> node, re-points each leaf leaving a member to start at
// the node, and declares its members before it; other splits stand after.
// Mutation: draw a source's legs node -> member and leave the leaf on its
// member, and this names every leg; declare every split after its node in
// sideOf and this names every source.
func TestASourcesLegsRunFromItsMembers(t *testing.T) {
	for _, year := range []int{2026, 2027} {
		docs := committedDocs(t, year)
		x, spine := docs[ExpansionProjection], docs[PublishedProjection]
		takesIn := map[string]bool{}
		for _, l := range spine.Links {
			takesIn[l.Target] = true
		}
		parent, role := map[string]string{}, map[string]string{}
		for _, n := range x.Nodes {
			if n.Alternative == "" {
				parent[n.ID] = n.Parent
				role[n.ID] = n.Role
			}
		}
		source := func(id string) bool {
			d, ok := splitOf(role[id], "", false)
			return id != "" && ok && d.hub && !takesIn[id] && slices.ContainsFunc(x.Splits, func(sp Split) bool {
				return sp.Node == id && !sp.Alternative
			})
		}
		for _, sp := range x.Splits {
			want := SplitAfter
			if !sp.Alternative && source(sp.Node) {
				want = SplitBefore
			}
			if sp.Side != want {
				t.Errorf("FY%d: the split of %s by %s stands its members %q it, want %q", year, sp.Node, sp.Key, sp.Side, want)
			}
		}
		legs, leaves := 0, 0
		for _, l := range x.Links {
			if l.Alternative != "" || l.Repeats == "" {
				continue
			}
			from, _ := LeafEnds(l.Repeats)
			switch {
			case source(parent[l.Source]) && l.Target == parent[l.Source]:
				legs++
				if from != l.Source {
					t.Errorf("FY%d: the leg %s -> %s repeats %q, which does not leave its member", year, l.Source, l.Target, l.Repeats)
				}
			case source(parent[l.Target]) && l.Source == parent[l.Target]:
				t.Errorf("FY%d: the leg %s -> %s runs from a node that takes nothing in to its member", year, l.Source, l.Target)
			case source(parent[from]) && parent[l.Target] != l.Source:
				leaves++
				if l.Source != parent[from] {
					t.Errorf("FY%d: the leaf %s -> %s repeating %q leaves a member of %s and does not start at it",
						year, l.Source, l.Target, l.Repeats, parent[from])
				}
			}
		}
		if legs == 0 || legs != leaves {
			t.Errorf("FY%d: %d legs run into a source and %d leaves start at one, want the same, more than none", year, legs, leaves)
		}
		t.Logf("FY%d: %d legs run from a source's members into it, each repeating a leaf re-pointed to start at it", year, legs)
	}
}

// TestASplitIsRefusedAtTheWrite: a split declaring a value schema/enums.schema.json
// does not list is refused at the write, naming the field.
func TestASplitIsRefusedAtTheWrite(t *testing.T) {
	d := committedDocs(t, 2026)[ExpansionProjection]
	if d == nil || len(d.Splits) == 0 {
		t.Fatal("the committed store builds no FY2026 expansion schedule with a split, so this test holds nothing")
	}
	if _, err := encode(d, ExpansionProjection, schema.Projection); err != nil {
		t.Fatalf("the committed schedule is refused at the write: %v", err)
	}
	for _, tc := range []struct {
		name, field string
		values      []string
		set         func(*Split, string)
	}{
		// A split in any form but extend is refused by split_form.
		{"ASplitInAnyFormButExtendIsRefusedAtTheWrite", "form", []string{"refine", "sideways", ""},
			func(s *Split, v string) { s.Form = v }},
		// A split with another side, or none, is refused by split_side.
		{"ASplitOnNoSideIsRefusedAtTheWrite", "side", []string{"left", ""},
			func(s *Split, v string) { s.Side = v }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.values {
				bad := *d
				bad.Splits = slices.Clone(d.Splits)
				tc.set(&bad.Splits[0], v)
				_, err := encode(&bad, ExpansionProjection, schema.Projection)
				switch {
				case err == nil:
					t.Errorf("a split of %s declaring %s %q was written", bad.Splits[0].Node, tc.field, v)
				case !strings.Contains(err.Error(), tc.field):
					t.Errorf("a split declaring %s %q was refused, but not for its %s: %v", tc.field, v, tc.field, err)
				}
			}
		})
	}
}

// TestExtendHubsRefuses: each row is a flow extendHubs must refuse rather than
// re-point onto a hub.
func TestExtendHubsRefuses(t *testing.T) {
	const group = "fund-group/enterprise"
	for _, tc := range []struct {
		name           string
		source, target string
		takesIn        bool
		want           string
	}{
		// Re-pointed onto the group, a flow between two members would be a
		// circular link, so the stitch refuses it.
		// Mutation: drop extendHubs' refusal and the stitch draws fund/610 ->
		// fund-group/enterprise beside the group's leg to fund/610.
		{"AFlowBetweenAHubsMembersIsRefused", "fund/610", "fund/600", true,
			"runs between two members of " + group},
		// A flow between a hub and its own member, on either side, would run
		// from the hub into itself.
		// Mutation: drop extendHubs' refusal of it and the stitch draws
		// fund-group/enterprise -> fund-group/enterprise, after the hub and
		// before it.
		{"ALeafBetweenAHubAndItsOwnMemberIsRefused/after the hub", group, "fund/600", true,
			"would be drawn from it back into it"},
		{"ALeafBetweenAHubAndItsOwnMemberIsRefused/before the hub", "fund/600", group, false,
			"would be drawn from it back into it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &stitch{nodes: map[string]Node{
				group:      {ID: group, Role: RoleFundGroup},
				"fund/600": {ID: "fund/600", Role: RoleFund, Parent: group},
				"fund/610": {ID: "fund/610", Role: RoleFund, Parent: group},
			}, links: []Link{{Source: tc.source, Target: tc.target, ValueCents: 1, Kind: KindExternal}}}
			err := extendHubs(st, map[string]bool{group: tc.takesIn})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("extendHubs is %v over %s -> %s, want it refused; links %+v", err, tc.source, tc.target, st.links)
			}
		})
	}
}

// TestAGroupDrawnWholeIsHeldToItsFundsNet: changeOf holds a group drawn whole to
// its funds' net change and refuses one with no fund changing, before the stitch.
// Mutation: drop either arm and its row reaches the stitch instead of refusing.
func TestAGroupDrawnWholeIsHeldToItsFundsNet(t *testing.T) {
	col := Column{FiscalYear: 2026, Basis: vocab.BasisAdopted}
	group := PrefixFundGroup + "capital"
	spine := Link{Source: group, Target: NodeFundBalanceContribution, ValueCents: 300}
	draw := Link{Source: NodeFundBalanceDraw, Target: PrefixFund + "1", ValueCents: 100}
	add := Link{Source: PrefixFund + "2", Target: NodeFundBalanceContribution, ValueCents: 500}
	for _, tt := range []struct {
		name  string
		funds []Link
		want  string
	}{
		{"no fund changes", nil, "no fund of it changes"},
		{"funds net to another figure", []Link{draw, add}, "change by a net 400 cents"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := (*expansion)(nil).changeOf(nil, nil, spine, group, tt.funds, nil, nil, col, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("changeOf = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

// TestAFundChangingInAGroupWithNoPrintedChangeIsRefused, even where the group's
// funds net to nothing. Mutation: hold only the group's net to zero and the
// netting pair below draws.
func TestAFundChangingInAGroupWithNoPrintedChangeIsRefused(t *testing.T) {
	col := Column{FiscalYear: 2026, Basis: vocab.BasisAdopted}
	group := PrefixFundGroup + "capital"
	draw := Link{Source: NodeFundBalanceDraw, Target: PrefixFund + "1", ValueCents: 100}
	add := Link{Source: PrefixFund + "2", Target: NodeFundBalanceContribution, ValueCents: 100}
	zero := Link{Source: PrefixFund + "3", Target: NodeFundBalanceContribution}
	if err := unprintedChanges(map[string][]Link{group: {zero}}, col); err != nil {
		t.Fatalf("a fund changing by nothing was refused: %v", err)
	}
	err := unprintedChanges(map[string][]Link{group: {draw, add}}, col)
	if err == nil || !strings.Contains(err.Error(), "fund/1 changes by 100 cents") {
		t.Fatalf("unprintedChanges = %v, want fund/1's change refused", err)
	}
}

// TestTheExpansionSaysItsLinksDoubleCount: the expansion states how many printed
// figures sit behind more than one link.
// Mutation: drop an-opened-box-repeats-its-members from expansionCaveats and
// both years go red.
func TestTheExpansionSaysItsLinksDoubleCount(t *testing.T) {
	for _, year := range []int{2026, 2027} {
		exp := committedDocs(t, year)[ExpansionProjection]
		if exp.Metadata.Counts.FactsCitedTwice == 0 {
			t.Fatalf("FY%d: no fact is behind two links, so this asserts nothing", year)
		}
		want := countedFigures(exp.Metadata.Counts.FactsCitedTwice)
		found := false
		for _, c := range exp.Metadata.Caveats {
			if c.ID == "an-opened-box-repeats-its-members" {
				// The leaving arm is conditional, and a reader discounting
				// every leaving link would get the double count wrong.
				found = c.Theme == ThemeDataFiles && strings.Contains(c.Text, want) &&
					strings.Contains(c.Text, "or leaving it where the node takes nothing in")
			}
		}
		if !found {
			t.Errorf("FY%d: no data-files caveat says %q", year, want)
		}
	}
}
