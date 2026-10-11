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
			Sliced
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
		for _, l := range x.Links {
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
		for _, l := range exp.Links {
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
			if !strings.Contains(c.Summary, "in and out differ by its own change in balance") {
				t.Errorf("FY%d: the caveat's summary does not say a fund's in and out differ: %q", year, c.Summary)
			}
		}
		if !slices.Equal(want, got) {
			t.Errorf("FY%d: the caveat reaches funds %v, want the drawn funds of the groups drawn whole %v", year, got, want)
		}

		flow := map[string]int64{}
		for _, l := range exp.Links {
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
