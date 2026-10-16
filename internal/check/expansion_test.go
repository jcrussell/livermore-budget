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

// leafAt is the index of the leaf a page prints from source to target,
// wherever the stitch draws it: a hub leg repeating it runs between a node
// and its own member, and the leaf is the one link repeating it that does not.
func leafAt(t *testing.T, d *project.Document, source, target string) int {
	t.Helper()
	parent := map[string]string{}
	for _, n := range d.Nodes {
		if n.Alternative == "" {
			parent[n.ID] = n.Parent
		}
	}
	for i, l := range d.Links {
		if l.Repeats == "" && l.Source == source && l.Target == target {
			return i
		}
		if s, tg := project.LeafEnds(l.Repeats); s == source && tg == target &&
			parent[l.Target] != l.Source && parent[l.Source] != l.Target {
			return i
		}
	}
	t.Fatalf("no leaf %s -> %s", source, target)
	return -1
}

// legAt is the index of the hub leg at hub repeating the leaf from source to
// target: from the hub to a member, or from a member to a hub taking nothing
// in.
func legAt(t *testing.T, d *project.Document, hub, source, target string) int {
	t.Helper()
	parent := map[string]string{}
	for _, n := range d.Nodes {
		if n.Alternative == "" {
			parent[n.ID] = n.Parent
		}
	}
	for i, l := range d.Links {
		s, tg := project.LeafEnds(l.Repeats)
		if s != source || tg != target {
			continue
		}
		if (l.Source == hub && parent[l.Target] == hub) || (l.Target == hub && parent[l.Source] == hub) {
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
		check Check
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
			green, err := c.check.Run(t.Context(), committed(t))
			if err != nil {
				t.Fatal(err)
			}
			if green.Status != StatusPass {
				t.Fatalf("%s over the committed corpus is %s: %v", c.check.ID(), green.Status, green.Findings)
			}
			s := withExpansion(t, func(d *project.Document) { c.edit(t, committed(t), d) })
			if c.bare != nil {
				c.bare(s)
			}
			red, err := c.check.Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			named := false
			for _, f := range red.Findings {
				named = named || strings.Contains(f.Subject+" "+f.Detail, c.want)
			}
			if red.Status != StatusFail || !named {
				t.Errorf("%s is %s with findings %v, want a failure naming %q", c.check.ID(), red.Status, red.Findings, c.want)
			}
		})
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
			res, err := (&linkValuesTieToFacts{}).Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			want := "which only the expansion schedule's links carry"
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), want) {
				t.Errorf("link-values-tie-to-facts is %s with findings %v, want a failure naming %q", res.Status, res.Findings, want)
			}
		})
	}
}
