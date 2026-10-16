package check

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// These two checks audit the stitched expansion schedule against the cuts it
// is stitched from, cuts-tie-along-the-lattice's doctrine at the grain a
// reader opens. They sum leaf links per printed cell and compare; neither
// chooses what a reader sees, which is the client's one fold.

// stitched is one expansion document beside the spine of its column.
type stitched struct {
	doc   linked
	spine *project.Document
	nodes map[string]project.Node
	facts map[string]fact.Fact
}

// stitchedDocuments pairs every expansion document with its column's spine,
// with a finding for one whose column the spine does not draw.
func stitchedDocuments(s *Subject) ([]stitched, []Finding, error) {
	var out []stitched
	var findings []Finding
	for _, p := range s.linkedDocuments() {
		if p.Name != project.ExpansionProjection {
			continue
		}
		var spine *project.Document
		for _, q := range s.documentsNamed(project.PublishedProjection) {
			if slices.Equal(q.Options.Columns, p.Options.Columns) {
				spine = q.Graph
			}
		}
		if spine == nil {
			findings = append(findings, finding(p.String(), "no spine is built for this column, so nothing says what its roots print"))
			continue
		}
		slice, err := p.slice(s.Facts)
		if err != nil {
			return nil, nil, err
		}
		nodes := make(map[string]project.Node, len(p.Nodes))
		for _, n := range p.Nodes {
			nodes[n.ID] = n
		}
		out = append(out, stitched{doc: p, spine: spine, nodes: nodes, facts: factIndex(slice)})
	}
	return out, findings, nil
}

// rootOf is the root a node's primary parent chain reaches, or "" for a
// chain that breaks or loops.
func (x stitched) rootOf(id string) string {
	for hops := 0; hops <= len(x.nodes); hops++ {
		n, ok := x.nodes[id]
		if !ok {
			return ""
		}
		if n.Parent == "" {
			return id
		}
		id = n.Parent
	}
	return ""
}

// memberOf is the member of the split (node, alternative) whose subtree holds
// id, or "".
func (x stitched) memberOf(id, node, alternative string) string {
	for hops := 0; hops <= len(x.nodes); hops++ {
		n, ok := x.nodes[id]
		if !ok || n.Parent == "" {
			return ""
		}
		if n.Parent == node && n.Alternative == alternative {
			return id
		}
		id = n.Parent
	}
	return ""
}

// atLeaf is a link read at the ends of the leaf it repeats, which is where
// its facts are printed. A hub leg -- a link between a node and its own
// primary member, either way -- is a copy of that leaf and is read at the
// leaf's ends whole; a leaf is read at its own end only where it is drawn at
// that end's primary parent, the one move the stitch makes, so an end drawn
// anywhere else is read as drawn. A link repeating nothing is itself.
func atLeaf(l project.Link, nodes map[string]project.Node) project.Link {
	if l.Repeats == "" {
		return l
	}
	parent := func(id string) string {
		if n := nodes[id]; n.Alternative == "" {
			return n.Parent
		}
		return ""
	}
	src, tgt := project.LeafEnds(l.Repeats)
	if l.Alternative == "" && (parent(l.Target) == l.Source || parent(l.Source) == l.Target) {
		l.Source, l.Target = src, tgt
		return l
	}
	if l.Source != src && parent(src) == l.Source {
		l.Source = src
	}
	if l.Target != tgt && parent(tgt) == l.Target {
		l.Target = tgt
	}
	return l
}

// printedSide is the facts a link cites from the spine's own scope, and the
// rest.
func (x stitched) printedSide(l project.Link) (printed, rest []string) {
	for _, id := range l.FactIDs {
		if x.facts[id].Scope == project.PublishedScope {
			printed = append(printed, id)
		} else {
			rest = append(rest, id)
		}
	}
	return printed, rest
}

// expansionOpensAsTheSpine asserts the expansion's root frontier is the spine:
// its roots are the spine's nodes, and its primary leaf links, each end
// folded to its root, draw exactly the spine's cells at the spine's cents, a
// figure of pp.66-67 a leaf cites being one its own cell prints. A printed
// cell a derived leg folds into is drawn printed only as the whole set of
// legs naming it in Completes, so every leg of it names it, and the legs
// naming a cell fold to it, sum to its cents and cite its printed figures.
//
// MUTATION: re-parent fund/101 (capital) to fund-group/general in the stitch
// and this check names the two groups' moved cells; drop the 250,000
// remainder and it names FY2026-27's internal service Services & Supplies;
// drop Completes from one of that cell's legs and it names the cell.
type expansionOpensAsTheSpine struct{}

var _ Check = (*expansionOpensAsTheSpine)(nil)

func (*expansionOpensAsTheSpine) ID() string { return "expansion-opens-as-the-spine" }
func (*expansionOpensAsTheSpine) Tier() int  { return 1 }
func (*expansionOpensAsTheSpine) Full() bool { return false }
func (*expansionOpensAsTheSpine) Description() string {
	return "the expansion schedule's roots are the spine's nodes, and its leaf links folded to " +
		"their roots draw exactly the spine's cells at the spine's cents, citing no figure of " +
		"pp.66-67 another cell prints, and every leg of a printed cell a derived leg folds into " +
		"names it as the set completing it"
}

func (*expansionOpensAsTheSpine) Run(_ context.Context, s *Subject) (Result, error) {
	docs, findings, err := stitchedDocuments(s)
	if err != nil {
		return Result{}, err
	}
	cells := 0
	for _, x := range docs {
		subject := x.doc.String()
		var roots []string
		for _, n := range x.doc.Nodes {
			if n.Parent == "" {
				roots = append(roots, n.ID)
			}
		}
		var spineIDs []string
		for _, n := range x.spine.Nodes {
			spineIDs = append(spineIDs, n.ID)
		}
		sort.Strings(roots)
		sort.Strings(spineIDs)
		if !slices.Equal(roots, spineIDs) {
			findings = append(findings, finding(subject, "its roots are %v and the spine's nodes are %v", roots, spineIDs))
		}

		type cell struct {
			cents   int64
			printed []string
			legs    int
			derived bool
		}
		drawn := map[string]*cell{}
		// completing is each cell a leg names in Completes, over the legs
		// naming it, wherever they fold.
		completing := map[string]*cell{}
		at := func(m map[string]*cell, k string) *cell {
			if m[k] == nil {
				m[k] = &cell{}
			}
			return m[k]
		}
		for _, l := range x.doc.Links {
			if l.Alternative != "" {
				if l.Completes != "" {
					findings = append(findings, finding(subject, "%s -> %s is a link of the %s alternative and names %q in completes",
						l.Source, l.Target, l.Alternative, l.Completes))
				}
				continue
			}
			src, tgt := x.rootOf(l.Source), x.rootOf(l.Target)
			if src == "" || tgt == "" {
				findings = append(findings, finding(subject, "%s -> %s has an end whose parents reach no root", l.Source, l.Target))
				continue
			}
			k := project.CellKey(src, tgt, l.Kind)
			if l.Completes != "" && l.Completes != k {
				findings = append(findings, finding(subject, "%s -> %s names %q in completes and folds to %s",
					l.Source, l.Target, l.Completes, k))
			}
			if src == tgt {
				continue
			}
			printed, _ := x.printedSide(l)
			add := func(c *cell) {
				c.cents += l.ValueCents
				c.printed = append(c.printed, printed...)
				c.legs++
				c.derived = c.derived || l.Derived
			}
			add(at(drawn, k))
			if l.Completes != "" {
				add(at(completing, l.Completes))
			}
		}
		for _, l := range x.spine.Links {
			cells++
			k := project.CellKey(l.Source, l.Target, l.Kind)
			c := drawn[k]
			delete(drawn, k)
			if c == nil {
				findings = append(findings, finding(subject, "the spine draws %s at %s and no leaf folds to it", k,
					amount.Cents(l.ValueCents)))
				continue
			}
			if c.cents != l.ValueCents {
				findings = append(findings, finding(subject, "%s: the leaves fold to %s and the spine draws %s (off by %s)",
					k, amount.Cents(c.cents), amount.Cents(l.ValueCents), amount.Cents(c.cents-l.ValueCents)))
			}
			for _, id := range c.printed {
				if !slices.Contains(l.FactIDs, id) {
					findings = append(findings, finding(subject, "%s: a leaf cites %s, a figure of pp.66-67 this cell does not print",
						k, id))
				}
			}
			whole := completing[k]
			delete(completing, k)
			named := 0
			if whole != nil {
				named = whole.legs
			}
			if c.derived && !l.Derived && named != c.legs {
				findings = append(findings, finding(subject, "%s: a derived leg folds into this printed cell and %d of its %d legs name it in completes",
					k, named, c.legs))
			}
			if whole == nil {
				continue
			}
			if whole.cents != l.ValueCents {
				findings = append(findings, finding(subject, "%s: the legs completing it carry %s and the spine draws %s",
					k, amount.Cents(whole.cents), amount.Cents(l.ValueCents)))
			}
			if !slices.Equal(sortedCopy(slices.Compact(sortedCopy(whole.printed))), sortedCopy(l.FactIDs)) {
				findings = append(findings, finding(subject, "%s: the legs completing it cite %v of pp.66-67 and the cell prints %v",
					k, slices.Compact(sortedCopy(whole.printed)), l.FactIDs))
			}
		}
		for _, k := range sortedStrings(completing) {
			findings = append(findings, finding(subject, "%d leg(s) name %s in completes, a cell the spine does not draw",
				completing[k].legs, k))
		}
		for _, k := range sortedStrings(drawn) {
			findings = append(findings, finding(subject, "the leaves fold to %s at %s, a cell the spine does not draw",
				k, amount.Cents(drawn[k].cents)))
		}
	}
	return conclusion{
		subjects: cells,
		unit:     "spine cells",
		held: fmt.Sprintf("%d spine cells over %d expansion document(s), each the sum of the leaves "+
			"folded to it, to the cent", cells, len(docs)),
		nothing:  "no expansion schedule is built, so no root frontier has been read",
		findings: findings,
	}.result(), nil
}

// expansionSplitsTie asserts every split of the expansion schedule ties to
// its parent's printed cells, by facts and by cents: a root's against the
// spine's cells it sits in, fund/100's divisions against p172's General Fund
// by object, an alternative against what its node takes in. Within a cell no
// member cites a fact twice; a remainder cites its cell's own printed facts
// and subtracts exactly the members it is the difference of; and a remainder
// of a difference is worth what a declared structure.Exception holds that
// cell apart by. Every split extends. A split whose links to its members
// cite totals of their own reaches each member by one, and the totals naming
// one list of leaves in Restates carry those leaves' cents
// ([restatingFindings]); a hub split's legs each repeat one leaf
// ([hubFindings]), and together carry what the node takes in on the spine,
// or gives out where it takes nothing in.
//
// MUTATION: cite the 250,000 remainder's printed side from the enterprise
// group's Services & Supplies and this check names the cell; make fund/100's
// link to one division a cent short and it names that division; drop a leaf
// from what one division's total restates and it names the list's cents;
// name two divisions' leaves on each of their totals and it names the total;
// name leaves on a hub leg and it names the leg; drop one
// revenue/taxes/sales leg and it names the line's leaf and the hub's
// outflow; run that leg out of the hub and it names the leg; make one object
// category's leg a cent short and it names the pairing; leave one line's
// leaf on its fund and it names a leg with no leaf, or on its line and it
// names the leaf; run one division's total into its fund and it names the
// split as drawn both ways.
type expansionSplitsTie struct{}

var _ Check = (*expansionSplitsTie)(nil)

func (*expansionSplitsTie) ID() string { return "expansion-splits-tie" }
func (*expansionSplitsTie) Tier() int  { return 1 }
func (*expansionSplitsTie) Full() bool { return false }
func (*expansionSplitsTie) Description() string {
	return "every split of the expansion schedule ties to its parent's printed cells by facts and " +
		"cents, each remainder citing its cell's own figures and worth what a declared exception " +
		"holds that cell apart by, and each hub leg repeating exactly one leaf at its member"
}

// tieCell is one printed cell of a split's parent against its members.
type tieCell struct {
	printed      int64
	printedFacts []string
	members      int64
	plain        []string
	// remainders are the members' remainder links in this cell.
	remainders []project.Link
}

func (c *expansionSplitsTie) Run(_ context.Context, s *Subject) (Result, error) {
	docs, findings, err := stitchedDocuments(s)
	if err != nil {
		return Result{}, err
	}
	splits := 0
	for _, x := range docs {
		// Restates is a total's alone: a leaf or a leg naming leaves would be
		// stated in their place by a sum over ribbons.
		for _, l := range x.doc.Links {
			if m := x.nodes[l.Target]; len(l.Restates) > 0 &&
				(l.Repeats != "" || m.Parent != l.Source || m.Alternative != l.Alternative) {
				findings = append(findings, finding(x.doc.String(), "%s -> %s names leaves it restates and is no split's total",
					l.Source, l.Target))
			}
		}
		// A leaf's repeats is its cell's one key: a second leaf at it would
		// be read as the first wherever a sum states each cell once.
		repeated := map[string]int{}
		for _, l := range x.doc.Links {
			if l.Repeats != "" && !x.isLeg(l) {
				repeated[l.Repeats]++
			}
		}
		for _, k := range sortedStrings(repeated) {
			if repeated[k] > 1 {
				findings = append(findings, finding(x.doc.String(), "%d leaves repeat %q, want 1", repeated[k], k))
			}
		}
		for _, sp := range x.doc.Graph.Splits {
			splits++
			subject := fmt.Sprintf("%s split of %s by %s", x.doc, sp.Node, sp.Key)
			findings = append(findings, c.split(s, x, sp, subject)...)
		}
	}
	return conclusion{
		subjects: splits,
		unit:     "splits",
		held: fmt.Sprintf("%d splits over %d expansion document(s), each tying to its parent's "+
			"printed cells by facts and cents", splits, len(docs)),
		nothing:  "no expansion schedule declares a split",
		findings: findings,
	}.result(), nil
}

// split is the findings on one split.
func (c *expansionSplitsTie) split(s *Subject, x stitched, sp project.Split, subject string) []Finding {
	node, ok := x.nodes[sp.Node]
	if !ok {
		return []Finding{finding(subject, "the split's node is not a node of the document")}
	}
	alt := ""
	if sp.Alternative {
		alt = sp.Key
	}
	var members []string
	for _, n := range x.doc.Nodes {
		if n.Parent == sp.Node && n.Alternative == alt {
			members = append(members, n.ID)
		}
	}
	if len(members) == 0 {
		return []Finding{finding(subject, "no node is a member of it")}
	}
	if sp.Form != project.SplitExtend {
		return []Finding{finding(subject, "no printed cell is declared for a %s split of a node at this place, "+
			"so nothing holds its members", sp.Form)}
	}
	var findings []Finding
	// One way between the node and its members, which side of the node they
	// stand on: the client refuses a split drawn both ways at layout.
	from, into := 0, 0
	for _, l := range x.doc.Links {
		if l.Source == sp.Node && slices.Contains(members, l.Target) {
			from++
		}
		if l.Target == sp.Node && slices.Contains(members, l.Source) {
			into++
		}
	}
	if from > 0 && into > 0 {
		findings = append(findings, finding(subject, "the node's links to its members run both ways, %d from it and %d into it",
			from, into))
	}
	// The node's links to its members in this split's set: a total of its
	// own to each, or legs each repeating one leaf, either way.
	totals := map[string]int{}
	var legs []project.Link
	for _, l := range x.doc.Links {
		if l.Alternative != alt {
			continue
		}
		toMember := l.Source == sp.Node && slices.Contains(members, l.Target)
		fromMember := l.Target == sp.Node && slices.Contains(members, l.Source)
		if l.Repeats != "" && (toMember || fromMember) {
			legs = append(legs, l)
			continue
		}
		if toMember {
			totals[l.Target]++
		}
	}
	hub := len(legs) > 0
	if hub && len(totals) > 0 {
		findings = append(findings, finding(subject, "the node reaches %d member(s) by a total of its own and others by %d legs repeating a leaf",
			len(totals), len(legs)))
	}
	if !hub {
		for _, m := range members {
			if totals[m] != 1 {
				findings = append(findings, finding(subject, "the node extends into %s and reaches it by %d links, want 1",
					m, totals[m]))
			}
		}
		findings = append(findings, c.restatingFindings(x, sp, members, alt, subject)...)
	}

	if hub && (node.Parent != "" || sp.Alternative) {
		return append(findings, finding(subject, "the node reaches its members by %d legs repeating a leaf, "+
			"and only a root's primary split holds legs to the leaves they repeat", len(legs)))
	}
	var cells map[string]*tieCell
	switch {
	// rootCells reads the primary members alone, so an alternative is held
	// to what its node takes in.
	case node.Parent == "" && !sp.Alternative:
		if !hub {
			return append(findings, finding(subject, "a root's primary split reaches its members by no leg, "+
				"so nothing repeats the leaves at them"))
		}
		findings = append(findings, c.hubFindings(x, sp, members, legs, subject)...)
		cells = c.rootCells(x, sp)
	case node.Parent == "":
		cells = c.extendedRootCells(x, sp, alt)
	case strings.HasPrefix(sp.Node, project.PrefixFund) && !sp.Alternative:
		var f []Finding
		cells, f = c.fundCells(s, x, sp, members, subject)
		findings = append(findings, f...)
	default:
		return append(findings, finding(subject, "no printed cell is declared for a %s split of a node at this place, "+
			"so nothing holds its members", sp.Form))
	}
	for _, k := range sortedStrings(cells) {
		findings = append(findings, c.cell(s, x, subject, k, cells[k])...)
	}
	return findings
}

// rootCells is a root's primary cells: each spine link it sits in, against
// the primary leaves below its members and those re-pointed onto it, folded
// at their other end to a root. Its legs are no leaf of a cell.
func (*expansionSplitsTie) rootCells(x stitched, sp project.Split) map[string]*tieCell {
	cells := map[string]*tieCell{}
	at := func(k string) *tieCell {
		if cells[k] == nil {
			cells[k] = &tieCell{}
		}
		return cells[k]
	}
	for _, l := range x.spine.Links {
		var k string
		switch sp.Node {
		case l.Source:
			k = sp.Node + " -> " + l.Target + " " + string(l.Kind)
		case l.Target:
			k = l.Source + " -> " + sp.Node + " " + string(l.Kind)
		default:
			continue
		}
		t := at(k)
		t.printed += l.ValueCents
		t.printedFacts = append(t.printedFacts, l.FactIDs...)
	}
	for _, l := range x.doc.Links {
		if l.Alternative != "" || x.isLeg(l) {
			continue
		}
		var k string
		switch {
		case l.Target == sp.Node:
			other := x.rootOf(l.Source)
			if other == sp.Node {
				continue
			}
			k = other + " -> " + sp.Node + " " + string(l.Kind)
		case l.Source == sp.Node:
			other := x.rootOf(l.Target)
			if other == sp.Node {
				continue
			}
			k = sp.Node + " -> " + other + " " + string(l.Kind)
		case x.memberOf(l.Source, sp.Node, "") != "":
			other := x.rootOf(l.Target)
			if other == sp.Node {
				continue
			}
			k = sp.Node + " -> " + other + " " + string(l.Kind)
		case x.memberOf(l.Target, sp.Node, "") != "":
			other := x.rootOf(l.Source)
			if other == sp.Node {
				continue
			}
			k = other + " -> " + sp.Node + " " + string(l.Kind)
		default:
			continue
		}
		at(k).add(atLeaf(l, x.nodes))
	}
	return cells
}

// restatingFindings holds a totals split's links to the leaves they name in
// Restates: every link names exactly the leaves it states again -- its
// member's onward leaves on a primary split, every leaf arriving at the node
// on an alternative -- each a leaf's Repeats, and the links naming one list
// together carry exactly those leaves' cents, so a sum stating the list's
// leaves in their place states the same money.
func (*expansionSplitsTie) restatingFindings(x stitched, sp project.Split, members []string, alt,
	subject string) []Finding {
	var findings []Finding
	leaves := map[string]int64{}
	var leafLinks []project.Link
	for _, l := range x.doc.Links {
		if l.Alternative == "" && l.Repeats != "" && !x.isLeg(l) {
			leaves[l.Repeats] = l.ValueCents
			leafLinks = append(leafLinks, l)
		}
	}
	// stated is the leaves a total to member m states again.
	stated := func(m string) []string {
		var keys []string
		for _, o := range leafLinks {
			if (sp.Alternative && o.Target == sp.Node) || (!sp.Alternative && o.Source == m) {
				keys = append(keys, o.Repeats)
			}
		}
		slices.Sort(keys)
		return slices.Compact(keys)
	}
	type group struct {
		keys  []string
		cents int64
	}
	groups := map[string]*group{}
	for _, l := range x.doc.Links {
		if l.Alternative != alt || l.Source != sp.Node || !slices.Contains(members, l.Target) {
			continue
		}
		if len(l.Restates) == 0 {
			findings = append(findings, finding(subject, "%s -> %s is a total of its own and names no leaf it states again",
				l.Source, l.Target))
			continue
		}
		want := stated(l.Target)
		named := slices.Compact(slices.Sorted(slices.Values(l.Restates)))
		// Held as written: the client keys a list by its join, so one list
		// in two orders is two keys.
		if !slices.Equal(named, l.Restates) {
			findings = append(findings, finding(subject, "%s -> %s names the leaves it restates out of ascending order",
				l.Source, l.Target))
		}
		if !slices.Equal(named, want) {
			extra, missing := 0, 0
			for _, k := range named {
				if !slices.Contains(want, k) {
					extra++
				}
			}
			for _, k := range want {
				if !slices.Contains(named, k) {
					missing++
				}
			}
			findings = append(findings, finding(subject, "%s -> %s restates %d leaf(s) it does not state again and leaves out %d it does",
				l.Source, l.Target, extra, missing))
		}
		k := strings.Join(l.Restates, "\n")
		if groups[k] == nil {
			groups[k] = &group{keys: l.Restates}
		}
		groups[k].cents += l.ValueCents
	}
	for _, k := range sortedStrings(groups) {
		g := groups[k]
		var cents int64
		for _, key := range g.keys {
			c, ok := leaves[key]
			if !ok {
				findings = append(findings, finding(subject, "a total restates %q, which no leaf repeats", key))
			}
			cents += c
		}
		if cents != g.cents {
			findings = append(findings, finding(subject, "the totals restating %d leaves carry %s and the leaves %s (off by %s)",
				len(g.keys), amount.Cents(g.cents), amount.Cents(cents), amount.Cents(g.cents-cents)))
		}
	}
	return findings
}

// isHub answers whether a node is a hub split's node: a leg runs between it
// and its own primary member.
func (x stitched) isHub(id string) bool {
	for _, l := range x.doc.Links {
		if x.isLeg(l) && ((l.Source == id && x.nodes[l.Target].Parent == id) ||
			(l.Target == id && x.nodes[l.Source].Parent == id)) {
			return true
		}
	}
	return false
}

// isLeg answers whether a link is a hub leg: it repeats a leaf and runs
// between a node and its own primary member, from the node to the member or,
// on a node taking nothing in, from the member to the node.
func (x stitched) isLeg(l project.Link) bool {
	if l.Repeats == "" || l.Alternative != "" {
		return false
	}
	member := func(id, parent string) bool {
		m, ok := x.nodes[id]
		return ok && m.Parent == parent && m.Alternative == ""
	}
	return member(l.Target, l.Source) || member(l.Source, l.Target)
}

// hubFindings holds a hub split's legs to the leaves they repeat. Where the
// node takes money in on the spine, every primary leaf arriving at a member
// is re-pointed to end at the node, naming its own ends in repeats, and is
// repeated by exactly one leg from the node to that member; where it takes
// nothing in, every primary leaf leaving a member is re-pointed to start at
// the node and repeated by exactly one leg from that member to the node. A
// re-pointed leaf's other end is the leaf's own, or that end's parent where
// it too is a hub's primary member. A leg repeats exactly one leaf, with its cents,
// facts, kind and derived flag, and the legs together carry what the node
// takes in on the spine, or gives out where it takes nothing in.
func (*expansionSplitsTie) hubFindings(x stitched, sp project.Split, members []string, legs []project.Link,
	subject string) []Finding {
	var findings []Finding
	inflow := false
	var spineCents int64
	for _, l := range x.spine.Links {
		if l.Target == sp.Node {
			inflow = true
		}
	}
	for _, l := range x.spine.Links {
		if (inflow && l.Target == sp.Node) || (!inflow && l.Source == sp.Node) {
			spineCents += l.ValueCents
		}
	}
	// near is a link's end at the node's side of the flow, far its other.
	near := func(source, target string) (string, string) {
		if inflow {
			return target, source
		}
		return source, target
	}
	way := map[bool]string{true: "arrive at", false: "leave"}[inflow]
	isMember := func(id string) bool { return slices.Contains(members, id) }
	leaves := map[string][]project.Link{}
	for _, l := range x.doc.Links {
		if l.Alternative == "" && l.Repeats != "" && !x.isLeg(l) {
			leaves[l.Repeats] = append(leaves[l.Repeats], l)
		}
	}
	repeating := map[string]int{}
	var legCents int64
	for _, g := range legs {
		repeating[g.Repeats]++
		legCents += g.ValueCents
		at := fmt.Sprintf("the leg %s -> %s repeating %s", g.Source, g.Target, g.Repeats)
		member, hub := near(g.Source, g.Target)
		if hub != sp.Node {
			findings = append(findings, finding(subject, "%s runs %s the node, and a leaf %s its member the other way", at,
				map[bool]string{true: "into", false: "out of"}[inflow], map[bool]string{true: "arrives at", false: "leaves"}[inflow]))
			continue
		}
		if leafNear, _ := near(project.LeafEnds(g.Repeats)); leafNear != member {
			findings = append(findings, finding(subject, "%s names a leaf that does not %s its member", at, way))
			continue
		}
		var found []project.Link
		for _, l := range leaves[g.Repeats] {
			if n, _ := near(l.Source, l.Target); n == sp.Node {
				found = append(found, l)
			}
		}
		if len(found) != 1 {
			findings = append(findings, finding(subject, "%s has %d leaves re-pointed onto the node, want 1", at, len(found)))
			continue
		}
		l := found[0]
		if l.ValueCents != g.ValueCents || l.Kind != g.Kind || l.Derived != g.Derived ||
			!slices.Equal(sortedCopy(l.FactIDs), sortedCopy(g.FactIDs)) {
			findings = append(findings, finding(subject, "%s carries %s %s over %v (derived %v) and its leaf %s -> %s carries %s %s over %v (derived %v)",
				at, amount.Cents(g.ValueCents), g.Kind, g.FactIDs, g.Derived,
				l.Source, l.Target, amount.Cents(l.ValueCents), l.Kind, l.FactIDs, l.Derived))
		}
	}
	// Every leaf the node owes a leg: each at it or a member on its side.
	for _, l := range x.doc.Links {
		if l.Alternative != "" || x.isLeg(l) {
			continue
		}
		n, f := near(l.Source, l.Target)
		if n != sp.Node && !isMember(n) {
			continue
		}
		leafNear, leafFar := near(project.LeafEnds(l.Repeats))
		switch {
		case n != sp.Node || !isMember(leafNear):
			findings = append(findings, finding(subject, "the leaf %s -> %s repeating %q %s a member and is not re-pointed onto the node",
				l.Source, l.Target, l.Repeats, map[bool]string{true: "arrives at", false: "leaves"}[inflow]))
		case leafFar != f && (x.nodes[leafFar].Parent != f || x.nodes[leafFar].Alternative != "" || !x.isHub(f)):
			findings = append(findings, finding(subject, "the leaf %s -> %s re-pointed onto the node repeats %q, whose other end is another node's",
				l.Source, l.Target, l.Repeats))
		case repeating[l.Repeats] != 1:
			findings = append(findings, finding(subject, "the leaf %s -> %s repeating %s is repeated by %d legs, want 1",
				l.Source, l.Target, l.Repeats, repeating[l.Repeats]))
		}
	}
	if legCents != spineCents {
		way := map[bool]string{true: "takes in", false: "gives out"}[inflow]
		findings = append(findings, finding(subject, "the legs carry %s and on the spine %s %s %s (off by %s)",
			amount.Cents(legCents), sp.Node, way, amount.Cents(spineCents),
			amount.Cents(legCents-spineCents)))
	}
	return findings
}

// extendedRootCells is an extended root's one cell: what it takes in on the
// spine against its links to its members.
func (*expansionSplitsTie) extendedRootCells(x stitched, sp project.Split, alt string) map[string]*tieCell {
	t := &tieCell{}
	for _, l := range x.spine.Links {
		if l.Target == sp.Node {
			t.printed += l.ValueCents
			t.printedFacts = append(t.printedFacts, l.FactIDs...)
		}
	}
	for _, l := range x.doc.Links {
		if l.Alternative == alt && l.Source == sp.Node {
			t.add(l)
		}
	}
	return map[string]*tieCell{"into " + sp.Node: t}
}

// fundCells is an extended fund's cells: what the pages printing a fund by
// object print for it, against its members' links onward by the category
// their facts carry; and each of its links to a member is that member's
// onward links, by facts and cents.
func (*expansionSplitsTie) fundCells(s *Subject, x stitched, sp project.Split, members []string,
	subject string) (map[string]*tieCell, []Finding) {
	var findings []Finding
	number, err := strconv.Atoi(strings.TrimPrefix(sp.Node, project.PrefixFund))
	if err != nil {
		return nil, []Finding{finding(subject, "%s is no fund id", sp.Node)}
	}
	col := x.doc.Options.Columns[0]
	cells := map[string]*tieCell{}
	at := func(category string) *tieCell {
		k := sp.Node + " -> " + project.PrefixExpenditure + category
		if cells[k] == nil {
			cells[k] = &tieCell{}
		}
		return cells[k]
	}
	for _, f := range s.Facts {
		if (f.Scope == structure.ScopeGeneralFundByCategory || f.Scope == structure.ScopeExpenditureByFund) &&
			f.Fund != nil && *f.Fund == number && f.Kind == vocab.KindExpenditure &&
			project.ColumnOf(&f) == col && f.AmountCents != 0 {
			t := at(f.Category)
			t.printed += f.AmountCents
			t.printedFacts = append(t.printedFacts, f.ID)
		}
	}
	onward := map[string][]project.Link{}
	for _, l := range x.doc.Links {
		if l.Alternative == "" && slices.Contains(members, l.Source) {
			onward[l.Source] = append(onward[l.Source], l)
		}
	}
	for _, l := range x.doc.Links {
		if l.Alternative != "" || l.Source != sp.Node || !slices.Contains(members, l.Target) {
			continue
		}
		var cents int64
		var ids, own []string
		for _, o := range onward[l.Target] {
			cents += o.ValueCents
			ids = append(ids, o.FactIDs...)
		}
		// A printed dash draws no row onward, and the total cites it anyway.
		for _, id := range l.FactIDs {
			if x.facts[id].AmountCents != 0 {
				own = append(own, id)
			}
		}
		sort.Strings(ids)
		if cents != l.ValueCents || !slices.Equal(ids, sortedCopy(own)) {
			findings = append(findings, finding(subject, "%s -> %s carries %s over %d facts and %s draws %s onward over %d",
				l.Source, l.Target, amount.Cents(l.ValueCents), len(l.FactIDs), l.Target, amount.Cents(cents), len(ids)))
		}
	}
	for _, m := range members {
		for _, o := range onward[m] {
			// A link citing two categories has no one cell to sit in, and
			// binning it whole under either can cancel against another.
			var categories []string
			for _, id := range o.FactIDs {
				if c := x.facts[id].Category; !slices.Contains(categories, c) {
					categories = append(categories, c)
				}
			}
			if len(categories) != 1 {
				findings = append(findings, finding(subject, "%s -> %s cites facts of %d categories %q, want 1",
					o.Source, o.Target, len(categories), categories))
				continue
			}
			at(categories[0]).add(atLeaf(o, x.nodes))
		}
	}
	return cells, findings
}

// add is one member link into a cell.
func (t *tieCell) add(l project.Link) {
	t.members += l.ValueCents
	if project.IsRemainder(l.Source) || project.IsRemainder(l.Target) {
		t.remainders = append(t.remainders, l)
		return
	}
	t.plain = append(t.plain, l.FactIDs...)
}

// cell is the findings on one printed cell of a split.
func (*expansionSplitsTie) cell(s *Subject, x stitched, subject, k string, t *tieCell) []Finding {
	var findings []Finding
	if t.members != t.printed {
		findings = append(findings, finding(subject, "%s: the members carry %s and the parent prints %s (off by %s)",
			k, amount.Cents(t.members), amount.Cents(t.printed), amount.Cents(t.members-t.printed)))
	}
	seen := map[string]bool{}
	for _, id := range t.plain {
		if seen[id] {
			findings = append(findings, finding(subject, "%s: two members cite %s", k, id))
		}
		seen[id] = true
	}
	for _, r := range t.remainders {
		printed, rest := x.printedSide(r)
		if !slices.Equal(sortedCopy(printed), sortedCopy(t.printedFacts)) {
			findings = append(findings, finding(subject, "%s: the remainder %s -> %s cites %v of pp.66-67 and the cell prints %v",
				k, r.Source, r.Target, printed, t.printedFacts))
		}
		if len(rest) == 0 {
			continue
		}
		if !slices.Equal(sortedCopy(rest), sortedCopy(t.plain)) {
			findings = append(findings, finding(subject, "%s: the remainder %s -> %s subtracts %d facts and the other members cite %d",
				k, r.Source, r.Target, len(rest), len(t.plain)))
		}
		if name := heldApart(s.Exceptions, x.doc.Options.Columns[0], k, r.ValueCents); name == "" {
			findings = append(findings, finding(subject, "%s: the remainder %s -> %s carries a difference of %s and no "+
				"declared exception holds this cell apart by it", k, r.Source, r.Target, amount.Cents(r.ValueCents)))
		}
	}
	return findings
}

// heldApart names an exception against the spine pinning, in col, a cell whose
// coordinates the cell key names -- its fund group and category, a balance
// cell's ending balance -- apart by cents either way, or "".
func heldApart(exceptions []structure.Exception, col project.Column, k string, cents int64) string {
	var names []string
	for _, part := range strings.Fields(strings.NewReplacer(" -> ", " ").Replace(k)) {
		switch {
		case strings.HasPrefix(part, project.PrefixFundGroup):
			names = append(names, strings.TrimPrefix(part, project.PrefixFundGroup))
		case part == project.NodeFundBalanceDraw, part == project.NodeFundBalanceContribution:
			names = append(names, project.CategoryFundBalanceEnding)
		case strings.HasPrefix(part, project.PrefixExpenditure):
			names = append(names, strings.TrimPrefix(part, project.PrefixExpenditure))
		}
	}
	for _, e := range exceptions {
		if e.Against != structure.CutSpine {
			continue
		}
		for _, p := range e.Cells {
			d := p.Against.Cents - p.Cut.Cents
			if p.Year != col.FiscalYear || p.Basis != string(col.Basis) || (d != cents && d != -cents) {
				continue
			}
			named := len(p.Coords) == len(names)
			for _, v := range p.Coords {
				named = named && slices.Contains(names, v)
			}
			if named {
				return e.Name
			}
		}
	}
	return ""
}

// sortedCopy is ids sorted, leaving ids as it was.
func sortedCopy(ids []string) []string {
	out := slices.Clone(ids)
	sort.Strings(out)
	return out
}
