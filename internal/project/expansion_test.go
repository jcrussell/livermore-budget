package project

import (
	"fmt"
	"slices"
	"sort"
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
	for _, p := range Registry(store.reg) {
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

// measuredCell is one cell of a split: what the parent prints and what its
// candidate members carry.
type measuredCell struct {
	parent, members int64
}

// measured is one split's cells, keyed by a description of the cell.
type measured map[string]*measuredCell

func (m measured) add(key string, parent, members int64) {
	c := m[key]
	if c == nil {
		c = &measuredCell{}
		m[key] = c
	}
	c.parent += parent
	c.members += members
}

// differences is the cells whose parent and members differ, sorted.
func (m measured) differences() []string {
	var out []string
	for k, c := range m {
		if c.parent != c.members {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// licence is how a difference is carried: the exception declaring it, the
// group-grain row a remainder carries, or "" for neither.
type licence func(cell string, diff int64) string

// TestEverySplitIsMeasuredAgainstItsParent is the measurement the stitch rests
// on, printed per split, per cell and per pinned year: the parent's printed
// cells against the members' printed cells, read from the documents built over
// each finer schedule. A difference is carried by a remainder citing the
// printed group-grain row, or held apart by a declared structure.Exception, or
// it is UNCARRIED and the split is not stitched. The test fails when the
// uncarried set moves, so a stitched split cannot start differing unseen and
// one left unstitched cannot start tying unseen.
func TestEverySplitIsMeasuredAgainstItsParent(t *testing.T) {
	uncarried := map[string][]string{}
	for _, year := range []int{2026, 2027} {
		col := Column{FiscalYear: year, Basis: vocab.BasisAdopted}
		docs := committedDocs(t, year)
		spine := docs[PublishedProjection]
		ff, fsu := docs[FundFlowsProjection], docs[FundSourcesUsesProjection]
		parentOf := map[string]string{}
		for _, n := range ff.Nodes {
			parentOf[n.ID] = n.Parent
		}
		report := func(split string, m measured, carried licence) {
			for _, k := range m.differences() {
				c := m[k]
				how := carried(k, c.parent-c.members)
				if how == "" {
					how = "UNCARRIED"
					uncarried[split] = append(uncarried[split], fmt.Sprintf("FY%d %s", year, k))
				}
				t.Logf("FY%d %-38s %-62s parent %15d members %15d diff %13d  %s",
					year, split, k, c.parent, c.members, c.parent-c.members, how)
			}
			t.Logf("FY%d %-38s %d cells, %d differ", year, split, len(m), len(m.differences()))
		}
		spineCell := func(m measured) {
			for _, l := range spine.Links {
				m.add(l.Source+" -> "+l.Target, l.ValueCents, 0)
			}
		}

		// fund group -> funds, with every spine cell a group sits in: the
		// members are each fund's printed cells, read off the schedules that
		// print a fund.
		funds := measured{}
		spineCell(funds)
		objectOf := func(id string) string {
			rest := strings.TrimPrefix(strings.TrimPrefix(id, PrefixExpenditure), PrefixFund)
			return PrefixExpenditure + rest[strings.LastIndex(rest, "/")+1:]
		}
		for _, l := range ff.Links {
			switch {
			case strings.HasPrefix(l.Source, PrefixRevenueLine) && strings.HasPrefix(l.Target, PrefixFund):
				funds.add(parentOf[l.Source]+" -> "+fundGroupOf(t, l.Target), 0, l.ValueCents)
			case strings.HasPrefix(l.Source, PrefixDept) && strings.HasPrefix(l.Target, PrefixExpenditure):
				funds.add(PrefixFundGroup+"general -> "+objectOf(l.Target), 0, l.ValueCents)
			case strings.HasPrefix(l.Source, PrefixFund) && strings.HasPrefix(l.Target, PrefixExpenditure):
				funds.add(fundGroupOf(t, l.Source)+" -> "+objectOf(l.Target), 0, l.ValueCents)
			}
		}
		for _, l := range docs[TransfersByFundProjection].Links {
			if strings.HasPrefix(l.Source, PrefixTransferFrom) {
				funds.add(NodeTransfersIn+" -> "+fundGroupOf(t, l.Target), 0, l.ValueCents)
			}
		}
		for _, l := range docs[TransfersOutProjection].Links {
			if strings.HasPrefix(l.Target, PrefixTransferTo) {
				funds.add(fundGroupOf(t, l.Source)+" -> "+NodeTransfersOut, 0, l.ValueCents)
			}
		}
		for _, l := range fsu.Links {
			switch {
			case l.Source == NodeFundBalanceDraw:
				funds.add(l.Source+" -> "+fundGroupOf(t, l.Target), 0, l.ValueCents)
			case l.Target == NodeFundBalanceContribution, l.Target == CategoryFundBalanceReserveIncrease:
				funds.add(fundGroupOf(t, l.Source)+" -> "+l.Target, 0, l.ValueCents)
			}
		}
		report("fund group -> funds", funds, func(cell string, diff int64) string {
			if strings.Contains(cell, "fund-balance/draw") || strings.Contains(cell, "fund-balance/contribution") {
				if name := pinned(col, cell, "fund-balance/ending", diff); name != "" {
					return "exception " + name + ", a remainder"
				}
				return "the group's change, pp.66-67's row, on its remainder"
			}
			if name := pinned(col, cell, "", diff); name != "" {
				return "exception " + name + ", a remainder"
			}
			return ""
		})

		// The endpoints, by fund: transfers in by the fund paying them,
		// transfers out by the fund receiving them, and the three balance
		// rows by the fund whose balance moves. Folded to the spine's grain,
		// so each cell here is the split's sum against the spine's figure.
		for _, end := range []string{NodeTransfersIn, NodeTransfersOut, NodeFundBalanceDraw,
			NodeFundBalanceContribution, CategoryFundBalanceReserveIncrease} {
			m := measured{}
			for k, c := range funds {
				if strings.HasPrefix(k, end+" -> ") || strings.HasSuffix(k, " -> "+end) {
					m.add(k, c.parent, c.members)
				}
			}
			report(end+" -> by fund", m, func(cell string, diff int64) string {
				if name := pinned(col, cell, "fund-balance/ending", diff); name != "" {
					return "exception " + name + ", a remainder"
				}
				return ""
			})
		}

		// fund/100 -> its divisions: p172's General Fund by object against
		// pp.167-170's division rows.
		store, err := committedStore()
		if err != nil {
			t.Fatal(err)
		}
		divisions := measured{}
		for _, f := range store.facts {
			if f.Scope == structure.ScopeGeneralFundByCategory && f.FiscalYear == year && f.Basis == vocab.BasisAdopted {
				divisions.add("fund/100 -> "+PrefixExpenditure+f.Category, f.AmountCents, 0)
			}
		}
		for _, l := range ff.Links {
			if strings.HasPrefix(l.Source, PrefixDept) {
				divisions.add("fund/100 -> "+objectOf(l.Target), 0, l.ValueCents)
			}
		}
		report("fund/100 -> divisions", divisions, func(string, int64) string { return "" })

		// A fund other than the General Fund -> the departments it pays for:
		// pp.85-125's funding sources against what pp.173-183 print it
		// spending.
		departments := measured{}
		paying := map[string]int{}
		for _, l := range docs[DepartmentFundingProjection].Links {
			departments.add(l.Source, 0, l.ValueCents)
			paying[l.Source]++
		}
		for _, l := range ff.Links {
			if strings.HasPrefix(l.Source, PrefixFund) && strings.HasPrefix(l.Target, PrefixExpenditure) {
				departments.add(l.Source, l.ValueCents, 0)
			}
		}
		delete(departments, PrefixFund+strconv.Itoa(generalFund))
		report("fund -> departments", departments, func(string, int64) string { return "" })
		var several []string
		for f, n := range paying {
			if n > 1 && f != PrefixFund+strconv.Itoa(generalFund) {
				several = append(several, fmt.Sprintf("%s pays %d", f, n))
			}
		}
		sort.Strings(several)
		t.Logf("FY%d fund -> departments: of %d funds, %d pay more than one department (%s); no page "+
			"prints a department's object rows by fund, so a department drawn between a fund and its "+
			"rows is our attribution on every fund and a split of rows nobody printed on these",
			year, len(paying)-1, len(several), strings.Join(several, ", "))
		uncarried["fund -> departments"] = append(uncarried["fund -> departments"],
			fmt.Sprintf("FY%d no page prints a department's rows by fund; %d funds pay several", year, len(several)))

		// revenue category -> lines is the revenue arm of the fund group's
		// cells above; its alternative into departments has nothing to
		// measure, because pp.85-125 print no revenue category.
		revenueByDepartment := 0
		for _, f := range store.facts {
			if f.Scope == DepartmentFundingScope && f.FiscalYear == year && f.Kind == vocab.KindRevenue {
				revenueByDepartment++
			}
		}
		t.Logf("FY%d revenue category -> departments: pp.85-125 print %d revenue facts; their funding "+
			"sources are funds, not revenue categories", year, revenueByDepartment)
		uncarried["revenue category -> departments"] = append(uncarried["revenue category -> departments"],
			fmt.Sprintf("FY%d no revenue category on pp.85-125", year))

		// expenditure category -> divisions, pp.85-125's departmentwide rows
		// against the category's printed cells.
		byDivision := measured{}
		for _, l := range spine.Links {
			if strings.HasPrefix(l.Target, PrefixExpenditure) {
				byDivision.add(l.Target, l.ValueCents, 0)
			}
		}
		for _, l := range docs[DepartmentSpendingProjection].Links {
			byDivision.add(l.Source, 0, l.ValueCents)
		}
		report("expenditure category -> divisions", byDivision, func(cell string, diff int64) string {
			_, ex, err := licensed(structure.CutDepartmentwide, structure.LevelCategory, col,
				map[structure.Axis]string{structure.AxisCategory: strings.TrimPrefix(cell, PrefixExpenditure)})
			if err == nil && ex.Cells[0].Against.Cents-ex.Cells[0].Cut.Cents == diff {
				return "exception " + ex.Name + ", a remainder"
			}
			return ""
		})
	}

	want := map[string][]string{
		"fund-balance/draw -> by fund": {
			"FY2026 fund-balance/draw -> fund-group/capital",
			"FY2026 fund-balance/draw -> fund-group/enterprise",
			"FY2026 fund-balance/draw -> fund-group/special-revenue",
			"FY2027 fund-balance/draw -> fund-group/capital",
			"FY2027 fund-balance/draw -> fund-group/enterprise",
			"FY2027 fund-balance/draw -> fund-group/special-revenue",
		},
		"fund-balance/contribution -> by fund": {
			"FY2026 fund-group/capital -> fund-balance/contribution",
			"FY2026 fund-group/enterprise -> fund-balance/contribution",
			"FY2026 fund-group/special-revenue -> fund-balance/contribution",
			"FY2027 fund-group/capital -> fund-balance/contribution",
			"FY2027 fund-group/enterprise -> fund-balance/contribution",
			"FY2027 fund-group/special-revenue -> fund-balance/contribution",
		},
		"fund -> departments": {
			"FY2026 no page prints a department's rows by fund; 1 funds pay several",
			"FY2027 no page prints a department's rows by fund; 1 funds pay several",
		},
		"revenue category -> departments": {
			"FY2026 no revenue category on pp.85-125",
			"FY2027 no revenue category on pp.85-125",
		},
	}
	for split, cells := range uncarried {
		if !slices.Equal(want[split], cells) {
			t.Errorf("%s: uncarried %q, want %q", split, cells, want[split])
		}
	}
	for split := range want {
		if _, ok := uncarried[split]; !ok {
			t.Errorf("%s: nothing uncarried, want %q", split, want[split])
		}
	}
}

// pinned names the exception on a cut the expansion reads against the spine
// pinning this "<a> -> <b>" spine cell's group and category (or, where
// category is given, that category of the group) with the difference diff, or
// "".
func pinned(col Column, cell, category string, diff int64) string {
	a, b, _ := strings.Cut(cell, " -> ")
	group, object := a, b
	if !strings.HasPrefix(group, PrefixFundGroup) {
		group, object = b, a
	}
	coords := map[structure.Axis]string{
		structure.AxisFundGroup: strings.TrimPrefix(group, PrefixFundGroup),
		structure.AxisCategory:  strings.TrimPrefix(object, PrefixExpenditure),
	}
	cut := "fund-expenditures"
	if category != "" {
		coords[structure.AxisCategory] = category
		cut = structure.CutFundBalanceFlows
	}
	p, ex, err := licensed(cut, structure.LevelFundGroupByCategory, col, coords)
	if err != nil {
		return ""
	}
	held := p.Against.Cents - p.Cut.Cents
	if category != "" && strings.HasPrefix(a, NodeFundBalanceDraw) {
		held = -held
	}
	if held != diff {
		return ""
	}
	return ex.Name
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

// TestTheContestedRemainderIsTheExceptions: the 250,000 FY2026-27's internal
// service Services & Supplies carries over its funds' rows is a remainder in
// FY2026-27 alone, worth what the exception declaring the cell holds apart,
// drawn from the group's remainder to a row of the group's own, and the
// divisions alternative carries the same exception's sibling.
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

// TestAFundDrawnApartFromItsChangeIsToldSo holds the expansion's funds of a
// group whose change is drawn whole on the group's remainder: each is a
// target of the caveat saying so, its in less its out is its own change, and
// it carries fund-sources-uses' balances and their derived change, the one
// figure the pin states that no page prints.
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

// TestAFundGroupsLegsCarryEachFundsInflow holds a fund group's hub legs to
// what each fund takes in: a fund nothing arrives at is reached by no leg,
// and one that only takes in -- 211, 280 and 300 in both years -- is reached,
// where legs repeating each fund's outflow would leave it out.
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

// TestASourcesLegsRunFromItsMembers holds a hub split whose node takes
// nothing in on the spine -- each revenue category and transfers/in, a hub
// being declared only on a root -- to
// legs running from its members into it, and to declaring its members
// before it, which the client stands them by: every leg is member -> node
// repeating a leaf that leaves that member, every primary leaf leaving a
// member is re-pointed to start at the node, and every other split stands
// its members after its node.
//
// MUTATION: draw a source's legs node -> member and leave the leaf on its
// member, as extendHubs does for no node, and this names every leg; declare
// every split after its node in sideOf and this names every source.
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

// TestASplitInAnyFormButExtendIsRefusedAtTheWrite holds the expansion
// schedule's write to its one split form: the committed schedule encodes, and
// the same schedule with one split declared in another form is refused by
// schema/enums.schema.json's split_form, naming the form.
func TestASplitInAnyFormButExtendIsRefusedAtTheWrite(t *testing.T) {
	d := committedDocs(t, 2026)[ExpansionProjection]
	if d == nil || len(d.Splits) == 0 {
		t.Fatal("the committed store builds no FY2026 expansion schedule with a split, so this test holds nothing")
	}
	if _, err := encode(d, ExpansionProjection, schema.Projection); err != nil {
		t.Fatalf("the committed schedule is refused at the write: %v", err)
	}
	for _, form := range []string{"refine", "sideways", ""} {
		bad := *d
		bad.Splits = slices.Clone(d.Splits)
		bad.Splits[0].Form = form
		_, err := encode(&bad, ExpansionProjection, schema.Projection)
		switch {
		case err == nil:
			t.Errorf("a split of %s declared %q was written", bad.Splits[0].Node, form)
		case !strings.Contains(err.Error(), "form"):
			t.Errorf("a split declared %q was refused, but not for its form: %v", form, err)
		}
	}
}

// TestASplitOnNoSideIsRefusedAtTheWrite holds the expansion schedule's write
// to its two split sides, which the client stands members by: the same
// schedule with one split declaring another side, or none, is refused by
// schema/enums.schema.json's split_side, naming the side.
func TestASplitOnNoSideIsRefusedAtTheWrite(t *testing.T) {
	d := committedDocs(t, 2026)[ExpansionProjection]
	if d == nil || len(d.Splits) == 0 {
		t.Fatal("the committed store builds no FY2026 expansion schedule with a split, so this test holds nothing")
	}
	for _, side := range []string{"left", ""} {
		bad := *d
		bad.Splits = slices.Clone(d.Splits)
		bad.Splits[0].Side = side
		_, err := encode(&bad, ExpansionProjection, schema.Projection)
		switch {
		case err == nil:
			t.Errorf("a split of %s declared on side %q was written", bad.Splits[0].Node, side)
		case !strings.Contains(err.Error(), "side"):
			t.Errorf("a split on side %q was refused, but not for its side: %v", side, err)
		}
	}
}

// TestAFlowBetweenAHubsMembersIsRefused: no page prints a flow from one
// enterprise fund to another today, and the stitch refuses to draw one,
// since re-pointed onto the group it would run from a member back into its
// own node, which the client lays out as a circular link.
//
// MUTATION: drop extendHubs' refusal and the stitch draws fund/610 ->
// fund-group/enterprise beside the group's leg to fund/610.
func TestAFlowBetweenAHubsMembersIsRefused(t *testing.T) {
	const group = "fund-group/enterprise"
	st := &stitch{nodes: map[string]Node{
		group:      {ID: group, Role: RoleFundGroup},
		"fund/600": {ID: "fund/600", Role: RoleFund, Parent: group},
		"fund/610": {ID: "fund/610", Role: RoleFund, Parent: group},
	}, links: []Link{{Source: "fund/610", Target: "fund/600", ValueCents: 1, Kind: KindExternal}}}
	err := extendHubs(st, map[string]bool{group: true})
	if err == nil || !strings.Contains(err.Error(), "runs between two members of "+group) {
		t.Fatalf("extendHubs is %v over a flow between two of %s's funds, want it refused", err, group)
	}
}

// TestALeafBetweenAHubAndItsOwnMemberIsRefused: no page prints a flow
// between a fund group and one of its own funds, and the stitch refuses to
// draw one on either side of the hub, since re-pointed onto the hub it would
// run from the hub into itself.
//
// MUTATION: drop extendHubs' refusal of a leaf between a hub's node and its
// member and the stitch draws fund-group/enterprise -> fund-group/enterprise,
// after the hub and before it.
func TestALeafBetweenAHubAndItsOwnMemberIsRefused(t *testing.T) {
	const group = "fund-group/enterprise"
	for _, c := range []struct {
		name           string
		source, target string
		takesIn        bool
	}{
		{"after the hub", group, "fund/600", true},
		{"before the hub", "fund/600", group, false},
	} {
		st := &stitch{nodes: map[string]Node{
			group:      {ID: group, Role: RoleFundGroup},
			"fund/600": {ID: "fund/600", Role: RoleFund, Parent: group},
		}, links: []Link{{Source: c.source, Target: c.target, ValueCents: 1, Kind: KindExternal}}}
		err := extendHubs(st, map[string]bool{group: c.takesIn})
		if err == nil || !strings.Contains(err.Error(), "would be drawn from it back into it") {
			t.Errorf("%s: extendHubs is %v over %s -> %s, want it refused; links %+v", c.name, err, c.source, c.target, st.links)
		}
	}
}

// TestAGroupDrawnWholeIsHeldToItsFundsNet: where a group's funds change both
// ways, the group's change is drawn whole on the caveat's word that it is the
// net of its funds' changes, so changeOf holds it to that, and refuses a
// group with no fund changing at all. Both refusals come before the stitch
// is touched, which is why nil serves for it here; the committed store
// builds the case that ties. Mutation: drop either arm and its row reaches the
// stitch instead of refusing.
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

// TestAFundChangingInAGroupWithNoPrintedChangeIsRefused: such a fund would be
// drawn with in and out differing by its own change and no caveat saying so,
// even where its group's funds net to nothing. Mutation: hold only the
// group's net to zero, as before, and the netting pair below draws.
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

// TestTheExpansionSaysItsLinksDoubleCount: an opened node repeats its
// members' links, so the committed expansion states how many printed figures
// sit behind more than one link, the document's own count.
//
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
