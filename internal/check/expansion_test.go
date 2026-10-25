package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// withExpansion is the committed corpus with the FY2026-27 expansion document
// replaced by a copy edit has changed.
func withExpansion(t *testing.T, edit func(d *project.Document)) *Subject {
	t.Helper()
	s := mutable(t)
	for i, p := range s.Projections {
		if p.Name != project.ExpansionProjection || p.Graph == nil || p.Options.Columns[0].FiscalYear != 2027 {
			continue
		}
		d := *p.Graph
		d.Nodes = slices.Clone(d.Nodes)
		d.Links = slices.Clone(d.Links)
		for j := range d.Links {
			d.Links[j].FactIDs = slices.Clone(d.Links[j].FactIDs)
		}
		edit(&d)
		s.Projections[i].Graph = &d
		return s
	}
	t.Fatal("the committed corpus builds no FY2026-27 expansion")
	return nil
}

// linkAt is the index of the link from source to target.
func linkAt(t *testing.T, d *project.Document, source, target string) int {
	t.Helper()
	for i, l := range d.Links {
		if l.Source == source && l.Target == target {
			return i
		}
	}
	t.Fatalf("no link %s -> %s", source, target)
	return -1
}

// legsOf is project.IsLeg over d.
func legsOf(d *project.Document) func(project.Link) bool {
	nodes := map[string]project.Node{}
	for _, n := range d.Nodes {
		nodes[n.ID] = n
	}
	sides := project.PrimarySides(d.Splits)
	return func(l project.Link) bool { return project.IsLeg(l, nodes, sides) }
}

// leafAt is the index of the leaf a page prints from source to target,
// wherever the stitch draws it: the one link repeating it that is no leg.
func leafAt(t *testing.T, d *project.Document, source, target string) int {
	t.Helper()
	isLeg := legsOf(d)
	for i, l := range d.Links {
		if l.Repeats == "" && l.Source == source && l.Target == target {
			return i
		}
		if s, tg := project.LeafEnds(l.Repeats); s == source && tg == target && !isLeg(l) {
			return i
		}
	}
	t.Fatalf("no leaf %s -> %s", source, target)
	return -1
}

// legAt is the index of the hub leg at hub repeating the leaf from source to
// target.
func legAt(t *testing.T, d *project.Document, hub, source, target string) int {
	t.Helper()
	isLeg := legsOf(d)
	for i, l := range d.Links {
		s, tg := project.LeafEnds(l.Repeats)
		if s != source || tg != target {
			continue
		}
		if isLeg(l) && (l.Source == hub || l.Target == hub) {
			return i
		}
	}
	t.Fatalf("no leg at %s repeating %s -> %s", hub, source, target)
	return -1
}

// spineFact is the one fact the FY2026-27 spine cites from source to target.
func spineFact(t *testing.T, s *Subject, source, target string) string {
	t.Helper()
	for _, p := range s.documentsNamed(project.PublishedProjection) {
		if p.Options.Columns[0].FiscalYear != 2027 {
			continue
		}
		for _, l := range p.Graph.Links {
			if l.Source == source && l.Target == target {
				return l.FactIDs[0]
			}
		}
	}
	t.Fatalf("the spine draws no %s -> %s", source, target)
	return ""
}

const (
	isRemainder = "fund-group-only/internal-service"
	isRow       = "expenditure/fund-group/internal-service/services-and-supplies"
	isCell      = "fund-group/internal-service -> expenditure/services-and-supplies internal_service"
)

const (
	salesHub  = "revenue/taxes/sales"
	salesLine = "revenue-line/taxes/sales/sales-and-use-tax"
	salesLeaf = salesLine + " -> fund/100 external"
)

// TestTheExpansionChecksCanFail holds each check green over the committed
// corpus and red under each edit a stitch could make, naming what it reads.
func TestTheExpansionChecksCanFail(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		check local
		edit  func(t *testing.T, s *Subject, d *project.Document)
		bare  func(s *Subject)
		want  string
	}{
		{
			name: "a fund under another group moves two groups' cells", check: &expansionOpensAsTheSpine{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				for i := range d.Nodes {
					if d.Nodes[i].ID == "fund/101" {
						d.Nodes[i].Parent = "fund-group/general"
					}
				}
			},
			want: "fund-group/general",
		},
		{
			name: "the 250,000 dropped leaves the cell short", check: &expansionOpensAsTheSpine{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := leafAt(t, d, isRemainder, isRow)
				d.Links = slices.Delete(d.Links, i, i+1)
			},
			want: "fund-group/internal-service -> expenditure/services-and-supplies",
		},
		{
			name: "a leaf citing another cell's printed figure", check: &expansionOpensAsTheSpine{},
			edit: func(t *testing.T, s *Subject, d *project.Document) {
				i := leafAt(t, d, project.NodeFundBalanceDraw, "fund-group-only/capital")
				d.Links[i].FactIDs = []string{spineFact(t, s, "fund-group/enterprise", project.NodeFundBalanceContribution)}
			},
			want: "a figure of pp.66-67 this cell does not print",
		},
		{
			name: "a leg of a printed cell a derived leg completes naming nothing", check: &expansionOpensAsTheSpine{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[leafAt(t, d, "fund/700", "expenditure/fund/700/services-and-supplies")].Completes = ""
			},
			want: isCell + ": a derived leg folds into this printed cell and 5 of its 6 legs name it in completes",
		},
		{
			name: "a leg completing a printed cell a cent short", check: &expansionOpensAsTheSpine{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[leafAt(t, d, isRemainder, isRow)].ValueCents--
			},
			want: isCell + ": the legs completing it carry",
		},
		{
			name: "a leg naming a cell it does not fold to", check: &expansionOpensAsTheSpine{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[leafAt(t, d, "fund/600", "expenditure/fund/600/services-and-supplies")].Completes = isCell
			},
			want: "names \"" + isCell + "\" in completes and folds to",
		},
		{
			name: "a division's link a cent short of its rows", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[linkAt(t, d, "fund/100", "dept/police-administration")].ValueCents--
			},
			want: "fund/100 -> dept/police-administration",
		},
		{
			name: "a division's total restating one leaf too few", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := linkAt(t, d, "fund/100", "dept/patrol")
				d.Links[i].Restates = d.Links[i].Restates[1:]
			},
			want: "the totals restating 1 leaves carry",
		},
		{
			// The same leaves, so a sorted comparison passes; the client keys
			// a list by its join as written.
			name: "a division's total naming its leaves in descending order", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := linkAt(t, d, "fund/100", "dept/patrol")
				if len(d.Links[i].Restates) < 2 {
					t.Fatalf("fund/100 -> dept/patrol restates %d leaf, so no order can be reversed", len(d.Links[i].Restates))
				}
				d.Links[i].Restates = slices.Clone(d.Links[i].Restates)
				slices.Reverse(d.Links[i].Restates)
			},
			want: "fund/100 -> dept/patrol names the leaves it restates out of ascending order",
		},
		{
			name: "a division's total naming no leaf it restates", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[linkAt(t, d, "fund/100", "dept/patrol")].Restates = nil
			},
			want: "fund/100 -> dept/patrol is a total of its own and names no leaf it states again",
		},
		{
			// Each list's cents still tie: the two totals together carry
			// what the two divisions' leaves do.
			name: "two divisions' totals each naming both divisions' leaves", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				a, b := linkAt(t, d, "fund/100", "dept/patrol"), linkAt(t, d, "fund/100", "dept/police-administration")
				both := slices.Sorted(slices.Values(append(slices.Clone(d.Links[a].Restates), d.Links[b].Restates...)))
				d.Links[a].Restates, d.Links[b].Restates = both, slices.Clone(both)
			},
			want: "fund/100 -> dept/patrol restates",
		},
		{
			name: "an object category's divisions restating a leaf no link repeats", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				for i, l := range d.Links {
					if l.Source == "expenditure/wages-and-benefits" && l.Alternative != "" {
						d.Links[i].Restates = append(slices.Clone(l.Restates), "nowhere -> nothing external")
					}
				}
			},
			want: "a total restates \"nowhere -> nothing external\", which no leaf repeats",
		},
		{
			name: "a hub leg naming leaves it restates", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := legAt(t, d, salesHub, salesLine, "fund/100")
				d.Links[i].Restates = []string{salesLeaf}
			},
			want: "names leaves it restates and is no split's total",
		},
		{
			name: "the remainder citing another group's cell", check: &expansionSplitsTie{},
			edit: func(t *testing.T, s *Subject, d *project.Document) {
				i := leafAt(t, d, isRemainder, isRow)
				own := spineFact(t, s, "fund-group/internal-service", "expenditure/services-and-supplies")
				other := spineFact(t, s, "fund-group/enterprise", "expenditure/services-and-supplies")
				for j, id := range d.Links[i].FactIDs {
					if id == own {
						d.Links[i].FactIDs[j] = other
					}
				}
			},
			want: "the remainder " + isRemainder + " -> " + isRow + " cites",
		},
		{
			name: "a sales tax line's leg dropped, its leaf unrepeated", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := legAt(t, d, salesHub, salesLine, "fund/100")
				d.Links = slices.Delete(d.Links, i, i+1)
			},
			want: "the leaf " + salesHub + " -> fund-group/general repeating " + salesLeaf + " is repeated by 0 legs",
		},
		{
			name: "a sales tax line's leg dropped, the hub short of its outflow", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := legAt(t, d, salesHub, salesLine, "fund/100")
				d.Links = slices.Delete(d.Links, i, i+1)
			},
			want: "on the spine " + salesHub + " gives out",
		},
		{
			name: "an object category's leg a cent short of its leaf", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[legAt(t, d, "expenditure/capital-outlay", "fund/620", "expenditure/fund/620/capital-outlay")].ValueCents--
			},
			want: "the leg expenditure/capital-outlay -> expenditure/fund/620/capital-outlay repeating " +
				"fund/620 -> expenditure/fund/620/capital-outlay external carries",
		},
		{
			name: "a line's leaf left on its fund", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[leafAt(t, d, salesLine, "fund/100")].Target = "fund/100"
			},
			want: "the leg fund-group/general -> fund/100 repeating " + salesLeaf + " has 0 leaves re-pointed onto the node",
		},
		{
			name: "a fund's divisions reached by forged legs, restating nothing", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				parent := map[string]string{}
				for _, n := range d.Nodes {
					if n.Alternative == "" {
						parent[n.ID] = n.Parent
					}
				}
				forged := 0
				for i, l := range d.Links {
					if l.Source == "fund/100" && l.Alternative == "" && parent[l.Target] == "fund/100" {
						d.Links[i].Repeats, d.Links[i].Restates = l.Restates[0], nil
						forged++
					}
				}
				if forged == 0 {
					t.Fatal("fund/100 reaches no division")
				}
			},
			want: "split of fund/100 by divisions the node reaches its members by",
		},
		{
			name: "a leaf re-pointed onto a hub repeating another node's leaf", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				was := project.NodeFundBalanceDraw + " -> fund-group-only/capital fund_balance"
				forged := project.NodeFundBalanceContribution + " -> fund-group-only/capital fund_balance"
				for i, l := range d.Links {
					if l.Repeats == was {
						d.Links[i].Repeats = forged
					}
				}
			},
			want: "the leaf " + project.NodeFundBalanceDraw + " -> fund-group/capital re-pointed onto the node repeats",
		},
		{
			name: "a sales tax line's leg run out of its hub", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				l := &d.Links[legAt(t, d, salesHub, salesLine, "fund/100")]
				l.Source, l.Target = l.Target, l.Source
			},
			want: "the leg " + salesHub + " -> " + salesLine + " repeating " + salesLeaf +
				" runs out of the node, and a leaf leaves its member the other way",
		},
		{
			name: "a line's leaf left on its line", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[leafAt(t, d, salesLine, "fund/100")].Source = salesLine
			},
			want: "the leaf " + salesLine + " -> fund-group/general repeating \"" + salesLeaf +
				"\" leaves a member and is not re-pointed onto the node",
		},
		{
			// A line stands before its source and takes nothing in; the
			// owed loop reads a source hub's links at their sources.
			name: "a leaf arriving at a sales tax line", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				into := d.Links[leafAt(t, d, salesLine, "fund/100")]
				into.Source, into.Target = "fund/100", salesLine
				into.Repeats = "fund/100 -> " + salesLine + " external"
				d.Links = append(d.Links, into)
			},
			want: "the leaf fund/100 -> " + salesLine + " repeating \"fund/100 -> " + salesLine +
				" external\" arrives at the member " + salesLine + ", which stands before the node and takes nothing in",
		},
		{
			name: "a link repeating no leaf arriving at a sales tax line", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				into := d.Links[leafAt(t, d, salesLine, "fund/100")]
				into.Source, into.Target, into.Repeats = "fund/100", salesLine, ""
				d.Links = append(d.Links, into)
			},
			want: "the link fund/100 -> " + salesLine + " arrives at the member " + salesLine +
				", which stands before the node and takes nothing in",
		},
		{
			name: "a link repeating a leaf into a sales tax line", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				into := d.Links[leafAt(t, d, salesLine, "fund/100")]
				into.Source, into.Target = "fund/100", "fund-group/general"
				into.Repeats = "fund/100 -> " + salesLine + " external"
				d.Links = append(d.Links, into)
			},
			want: "the link fund/100 -> fund-group/general repeats \"fund/100 -> " + salesLine +
				" external\", a leaf into the member " + salesLine + ", which stands before the node and takes nothing in",
		},
		{
			name: "a leaf re-pointed off a source hub repeating a leaf to another node", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				forged := salesLine + " -> fund/200 external"
				d.Links[legAt(t, d, salesHub, salesLine, "fund/100")].Repeats = forged
				d.Links[leafAt(t, d, salesLine, "fund/100")].Repeats = forged
			},
			want: "the leaf " + salesHub + " -> fund-group/general re-pointed onto the node repeats \"" + salesLine +
				" -> fund/200 external\", whose other end is another node's",
		},
		{
			name: "a leaf re-pointed off a source hub onto a fund repeating a leaf to one of its divisions", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				forged := salesLine + " -> dept/patrol external"
				d.Links[legAt(t, d, salesHub, salesLine, "fund/100")].Repeats = forged
				i := leafAt(t, d, salesLine, "fund/100")
				d.Links[i].Repeats = forged
				d.Links[i].Target = "fund/100"
			},
			want: "the leaf " + salesHub + " -> fund/100 re-pointed onto the node repeats \"" + salesLine +
				" -> dept/patrol external\", whose other end is another node's",
		},
		{
			// An inflow hub re-points only the leaves arriving at its
			// members, so a leaf leaving one stays on that member.
			name: "a leaf onto an inflow hub re-pointed off a fund onto its group, also an inflow hub", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[leafAt(t, d, "fund/620", "expenditure/fund/620/capital-outlay")].Source = "fund-group/enterprise"
			},
			want: "the leaf fund-group/enterprise -> expenditure/capital-outlay re-pointed onto the node repeats",
		},
		{
			name: "a division's link run back into its fund, against its split's side", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				back := d.Links[linkAt(t, d, "fund/100", "dept/patrol")]
				back.Source, back.Target, back.Restates = back.Target, back.Source, nil
				d.Links = append(d.Links, back)
			},
			want: "split of fund/100 by divisions 1 of the node's links to its members run against its declared side \"after\"",
		},
		{
			// The client stands members on the declared side and reads no
			// link to infer it, so a side the spine contradicts is a chart
			// drawn the wrong way round.
			name: "an inflow hub declaring its members before it", check: &expansionSplitsTie{},
			edit: func(_ *testing.T, _ *Subject, d *project.Document) {
				d.Splits = slices.Clone(d.Splits)
				for i := range d.Splits {
					if d.Splits[i].Node == "fund-group/enterprise" {
						d.Splits[i].Side = project.SplitBefore
					}
				}
			},
			want: "split of fund-group/enterprise by funds the split declares its members \"before\" its node, want \"after\"",
		},
		{
			// A line of a source stands before it, so a split of the line
			// opening after it would stand its members in the source's
			// column, which the client refuses at layout.
			name: "a member of a source opening back toward it", check: &expansionSplitsTie{},
			edit: func(_ *testing.T, _ *Subject, d *project.Document) {
				d.Splits = append(slices.Clone(d.Splits),
					project.Split{Node: salesLine, Key: "lines", Form: project.SplitExtend, Side: project.SplitAfter})
				d.Nodes = append(d.Nodes, project.Node{ID: salesLine + "/part", Label: "Part", Tier: 0, Parent: salesLine})
			},
			want: "split of " + salesLine + " by lines the node stands \"before\" " + salesHub + " and opens \"after\", back toward " + salesHub,
		},
		{
			name: "two leaves repeating one leaf", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := leafAt(t, d, salesLine, "fund/100")
				half := d.Links[i]
				half.ValueCents /= 2
				d.Links[i].ValueCents -= half.ValueCents
				d.Links = append(d.Links, half)
			},
			want: "2 leaves repeat \"" + salesLeaf + "\", want 1",
		},
		{
			name: "a flow endpoint parenting the members of no split it declares", check: &nodeHierarchyWellFormed{},
			edit: func(_ *testing.T, _ *Subject, d *project.Document) {
				d.Splits = slices.DeleteFunc(slices.Clone(d.Splits), func(sp project.Split) bool {
					return sp.Node == project.NodeTransfersIn
				})
			},
			want: "is parented to \"" + project.NodeTransfersIn + "\", a flow endpoint",
		},
		{
			name: "an alternative split of a root in a form other than extend, held to nothing", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				const group, member = "fund-group/internal-service", "fund/again"
				d.Splits = append(slices.Clone(d.Splits),
					project.Split{Node: group, Key: "again", Form: "sideways", Alternative: true})
				d.Nodes = append(d.Nodes, project.Node{ID: member, Label: "Again", Tier: 1, Parent: group, Alternative: "again"})
				l := d.Links[leafAt(t, d, isRemainder, isRow)]
				l.Source, l.Alternative, l.ValueCents = member, "again", l.ValueCents+1
				d.Links = append(d.Links, l)
			},
			want: "no printed cell is declared for a sideways split",
		},
		{
			name: "two onward links each citing both categories, binned so they cancel", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				in := linkAt(t, d, "fund/100", "dept/patrol")
				svc := leafAt(t, d, "dept/patrol", "expenditure/patrol/services-and-supplies")
				wages := leafAt(t, d, "dept/patrol", "expenditure/patrol/wages-and-benefits")
				s, w := d.Links[svc].FactIDs[0], d.Links[wages].FactIDs[0]
				d.Links[svc].FactIDs = []string{w, s}
				d.Links[wages].FactIDs = []string{s, w}
				d.Links[in].FactIDs = append(slices.Clone(d.Links[in].FactIDs), s, w)
			},
			want: "cites facts of 2 categories",
		},
		{
			name: "a difference no declared exception holds apart", check: &expansionSplitsTie{},
			edit: func(*testing.T, *Subject, *project.Document) {},
			bare: func(s *Subject) { s.Exceptions = nil },
			want: "no declared exception holds this cell apart",
		},
		{
			name: "a division dropped from the alternative", check: &expansionSplitsTie{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := linkAt(t, d, "expenditure/wages-and-benefits", "dept/patrol/wages-and-benefits")
				d.Links = slices.Delete(d.Links, i, i+1)
			},
			want: "into expenditure/wages-and-benefits",
		},
		// The exemptions the stitch is given elsewhere reach no further than
		// what it declares.
		{
			name: "members of a split the document does not declare", check: &nodeHierarchyWellFormed{},
			edit: func(_ *testing.T, _ *Subject, d *project.Document) { d.Splits = nil },
			want: "expenditure/wages-and-benefits",
		},
		{
			name: "a primary link wearing the partition flag", check: &nodeTiersAreDeclared{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[linkAt(t, d, "fund/100", "dept/police-administration")].Partition = true
			},
			want: "declares a partition",
		},
		{
			name: "a p76 transfer missing its receiving leg", check: &transferLegsPair{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				i := leafAt(t, d, "transfer-from/100", "fund/210")
				d.Links = slices.Delete(d.Links, i, i+1)
			},
			want: "names 1 legs, want 2",
		},
		{
			name: "a p222 leg the expansion draws alone, drawn twice", check: &transferLegsPair{},
			edit: func(t *testing.T, s *Subject, d *project.Document) {
				scopeOf := map[string]string{}
				for _, f := range s.Facts {
					scopeOf[f.ID] = f.Scope
				}
				for _, l := range d.Links {
					if l.TransferID != "" && citesOnly(l, scopeOf, project.CIPFundingScope) {
						d.Links = append(d.Links, l)
						return
					}
				}
				t.Fatal("the expansion draws no p222 leg alone")
			},
			want: "a p222 leg the expansion draws alone names 2 legs, want 1",
		},
		{
			name: "a p222 leg the expansion draws alone, drawn receiving", check: &transferLegsPair{},
			edit: func(t *testing.T, s *Subject, d *project.Document) {
				scopeOf := map[string]string{}
				for _, f := range s.Facts {
					scopeOf[f.ID] = f.Scope
				}
				for i, l := range d.Links {
					if l.TransferID != "" && citesOnly(l, scopeOf, project.CIPFundingScope) {
						d.Links[i].Source, d.Links[i].Target = l.Target, l.Source
						return
					}
				}
				t.Fatal("the expansion draws no p222 leg alone")
			},
			want: "cites p222 alone and is not a paying leg",
		},
		{
			name: "a division's alternative row named for another division", check: &linkEndsMatchTheirFacts{},
			edit: func(t *testing.T, _ *Subject, d *project.Document) {
				d.Links[linkAt(t, d, "expenditure/wages-and-benefits", "dept/patrol/wages-and-benefits")].Target =
					"dept/finance/wages-and-benefits"
			},
			want: "dept/finance/wages-and-benefits",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			green := runCheck(t, c.check, committed(t))
			if green.Status != StatusPass {
				t.Fatalf("%s over the committed corpus is %s: %v", c.check.meta().id, green.Status, green.Findings)
			}
			s := withExpansion(t, func(d *project.Document) { c.edit(t, committed(t), d) })
			if c.bare != nil {
				c.bare(s)
			}
			red := runCheck(t, c.check, s)
			named := slices.ContainsFunc(red.Findings, func(f Finding) bool {
				return strings.Contains(f.Subject+" "+f.Detail, c.want)
			})
			if red.Status != StatusFail || !named {
				t.Errorf("%s is %s with findings %v, want a failure naming %q", c.check.meta().id, red.Status, red.Findings, c.want)
			}
		})
	}
}

// TestAFlowBetweenAHubsMembersIsALeaf plants a flow between two enterprise
// funds re-pointed onto their group, which runs against the split's side and
// must be named.
//
// Mutation: skip a leaf re-pointed onto the node in sideFindings' count; the
// plant passes.
func TestAFlowBetweenAHubsMembersIsALeaf(t *testing.T) {
	t.Parallel()
	const (
		group = "fund-group/enterprise"
		line  = "revenue-line/charges-for-services/charges-for-services"
		flow  = "fund/610 -> fund/600 external"
	)
	s := withExpansion(t, func(d *project.Document) {
		d.Links[legAt(t, d, group, line, "fund/600")].Repeats = flow
		i := leafAt(t, d, line, "fund/600")
		d.Links[i].Source, d.Links[i].Target, d.Links[i].Repeats = "fund/610", group, flow
	})
	res := runCheck(t, &expansionSplitsTie{}, s)
	against := false
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, "the leg fund/610 -> "+group) || strings.Contains(f.Detail, "the leaf fund/610 -> "+group) {
			t.Errorf("%s: %s", f.Subject, f.Detail)
		}
		against = against || (strings.Contains(f.Subject, "split of "+group+" ") &&
			f.Detail == "1 of the node's links to its members run against its declared side \"after\"")
	}
	if !against {
		t.Errorf("expansion-splits-tie is %s and does not name the leaf drawn back into %s: %v", res.Status, group, res.Findings)
	}
}

// TestRepeatsAndRestatesAreTheExpansionsAlone plants each on a fund-flows
// link, its value unmoved: project.LinkCents reads repeats wherever it is
// drawn, and a link naming its own ends still ties to its facts.
func TestRepeatsAndRestatesAreTheExpansionsAlone(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		plant func(l *project.Link)
	}{
		{"repeats", func(l *project.Link) { l.Repeats = l.Source + " -> " + l.Target + " " + string(l.Kind) }},
		{"restates", func(l *project.Link) { l.Restates = []string{l.Source + " -> " + l.Target + " " + string(l.Kind)} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := isolated(t)
			if res, err := (&linkValuesTieToFacts{}).Run(t.Context(), s); err != nil || res.Status != StatusPass {
				t.Fatalf("link-values-tie-to-facts over the committed corpus is %v (%v): %v", res.Status, err, res.Findings)
			}
			planted := false
			for _, p := range s.linkedDocuments() {
				if p.Name == project.FundFlowsProjection && len(p.Links) > 0 {
					c.plant(&p.Links[0])
					planted = true
					break
				}
			}
			if !planted {
				t.Fatal("no fund-flows document draws a link")
			}
			res := runCheck(t, &linkValuesTieToFacts{}, s)
			want := "which only the expansion schedule's links carry"
			wantFail(t, res, want)
		})
	}
}

// TestASplitsSideIsTheStitchs holds sideFindings to the side the stitch gives
// a split at a root the spine draws nothing into: a hub's members stand before
// it, a split citing its own totals after it.
//
// Mutation: want before for every root primary split at a node taking nothing
// in, hub or not; this names the general fund's divisions.
func TestASplitsSideIsTheStitchs(t *testing.T) {
	t.Parallel()
	x := stitched{spine: &project.Document{}}
	for _, c := range []struct {
		node   project.Node
		key    string
		naming bool
	}{
		{project.Node{ID: "fund/100", Role: project.RoleGeneralFund}, "divisions", false},
		{project.Node{ID: "fund-group/enterprise", Role: project.RoleFundGroup}, "funds", true},
	} {
		sp := project.Split{Node: c.node.ID, Key: c.key, Form: project.SplitExtend, Side: project.SplitAfter}
		got := x.sideFindings(sp, c.node, []string{c.node.ID + "/m"}, "", "subject")
		if named := len(got) > 0; named != c.naming {
			t.Errorf("the split of %s by %s declared after a root the spine draws nothing into: findings %v, want named %v",
				c.node.ID, c.key, got, c.naming)
		}
	}
}

// TestALinkIsReadWholeAtItsLeafOnlyAsALeg holds atLeaf to project.IsLeg: a
// fund's link into its own group that repeats a leaf elsewhere is read at the
// group, not whole at the leaf.
//
// Mutation: read every link between a node and its own primary member whole at
// its leaf; this reads it as fund/600 -> expenditure/capital-outlay.
func TestALinkIsReadWholeAtItsLeafOnlyAsALeg(t *testing.T) {
	t.Parallel()
	const group = "fund-group/enterprise"
	nodes := map[string]project.Node{
		group:      {ID: group, Role: project.RoleFundGroup},
		"fund/600": {ID: "fund/600", Role: project.RoleFund, Parent: group},
	}
	sides := map[string]string{group: project.SplitAfter}
	l := project.Link{Source: "fund/600", Target: group, Kind: project.KindExternal,
		Repeats: "fund/600 -> expenditure/capital-outlay external"}
	if got := atLeaf(l, nodes, sides); got.Source != "fund/600" || got.Target != group {
		t.Errorf("atLeaf reads %s -> %s repeating %q as %s -> %s, want it read where it is drawn",
			l.Source, l.Target, l.Repeats, got.Source, got.Target)
	}
	leg := project.Link{Source: group, Target: "fund/600", Kind: project.KindExternal,
		Repeats: "revenue-line/x -> fund/600 external"}
	if got := atLeaf(leg, nodes, sides); got.Source != "revenue-line/x" || got.Target != "fund/600" {
		t.Errorf("atLeaf reads the leg %s -> %s as %s -> %s, want it read at its leaf", leg.Source, leg.Target, got.Source, got.Target)
	}
}
