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
// figure of pp.66-67 a leaf cites being one its own cell prints.
//
// MUTATION: re-parent fund/101 (capital) to fund-group/general in the stitch
// and this check names the two groups' moved cells; drop the 250,000
// remainder and it names FY2026-27's internal service Services & Supplies.
type expansionOpensAsTheSpine struct{}

var _ Check = (*expansionOpensAsTheSpine)(nil)

func (*expansionOpensAsTheSpine) ID() string { return "expansion-opens-as-the-spine" }
func (*expansionOpensAsTheSpine) Tier() int  { return 1 }
func (*expansionOpensAsTheSpine) Full() bool { return false }
func (*expansionOpensAsTheSpine) Description() string {
	return "the expansion schedule's roots are the spine's nodes, and its leaf links folded to " +
		"their roots draw exactly the spine's cells at the spine's cents, citing no figure of " +
		"pp.66-67 another cell prints"
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
		}
		drawn := map[string]*cell{}
		key := func(src, tgt string, kind project.LinkKind) string { return src + " -> " + tgt + " " + string(kind) }
		for _, l := range x.doc.Links {
			if l.Alternative != "" {
				continue
			}
			src, tgt := x.rootOf(l.Source), x.rootOf(l.Target)
			if src == "" || tgt == "" {
				findings = append(findings, finding(subject, "%s -> %s has an end whose parents reach no root", l.Source, l.Target))
				continue
			}
			if src == tgt {
				continue
			}
			k := key(src, tgt, l.Kind)
			c := drawn[k]
			if c == nil {
				c = &cell{}
				drawn[k] = c
			}
			c.cents += l.ValueCents
			printed, _ := x.printedSide(l)
			c.printed = append(c.printed, printed...)
		}
		for _, l := range x.spine.Links {
			cells++
			k := key(l.Source, l.Target, l.Kind)
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
// by object, an extension against what its node takes in. Within a cell no
// member cites a fact twice; a remainder cites its cell's own printed facts
// and subtracts exactly the members it is the difference of; and a remainder
// of a difference is worth what a declared structure.Exception holds that
// cell apart by. An extended node reaches each member by one link and a
// refined node carries none.
//
// MUTATION: cite the 250,000 remainder's printed side from the enterprise
// group's Services & Supplies and this check names the cell; make fund/100's
// link to one division a cent short and it names that division.
type expansionSplitsTie struct{}

var _ Check = (*expansionSplitsTie)(nil)

func (*expansionSplitsTie) ID() string { return "expansion-splits-tie" }
func (*expansionSplitsTie) Tier() int  { return 1 }
func (*expansionSplitsTie) Full() bool { return false }
func (*expansionSplitsTie) Description() string {
	return "every split of the expansion schedule ties to its parent's printed cells by facts and " +
		"cents, each remainder citing its cell's own figures and worth what a declared exception " +
		"holds that cell apart by"
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
	var findings []Finding
	// The node's own links in this split's set: one to each member when it
	// extends, none when it is replaced.
	reached := map[string]int{}
	for _, l := range x.doc.Links {
		if l.Alternative != alt || (l.Source != sp.Node && l.Target != sp.Node) {
			continue
		}
		if sp.Form == project.SplitRefine {
			findings = append(findings, finding(subject, "the node is replaced by its members and carries %s -> %s itself",
				l.Source, l.Target))
			continue
		}
		reached[l.Target]++
	}
	if sp.Form == project.SplitExtend {
		for _, m := range members {
			if reached[m] != 1 {
				findings = append(findings, finding(subject, "the node extends into %s and reaches it by %d links, want 1",
					m, reached[m]))
			}
		}
	}

	var cells map[string]*tieCell
	switch {
	case node.Parent == "" && sp.Form == project.SplitRefine:
		cells = c.rootCells(x, sp)
	case node.Parent == "" && sp.Form == project.SplitExtend:
		cells = c.extendedRootCells(x, sp, alt)
	case strings.HasPrefix(sp.Node, project.PrefixFund) && sp.Form == project.SplitExtend:
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

// rootCells is a refined root's cells: each spine link it sits in, against
// the primary leaves below its members folded at their other end to a root.
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
		if l.Alternative != "" {
			continue
		}
		var k string
		switch {
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
		at(k).add(l)
	}
	return cells
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
			f.FiscalYear == col.FiscalYear && f.Basis == col.Basis && f.AmountCents != 0 {
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
			category := ""
			for _, id := range o.FactIDs {
				category = x.facts[id].Category
			}
			at(category).add(o)
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
