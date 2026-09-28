package export

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// residualPrefix is the id prefix site/app.js draws a residual node under,
// and gapPrefix the one it draws a gap node under: each followed by the id
// of the node the reader opened. Two prefixes, so a check counting one mark
// never finds the other.
const (
	residualPrefix = "residual/"
	gapPrefix      = "gap/"
)

// RoleResidual is the role carryResidual gives its mark, and RoleGap the one
// markGap gives its own.
const (
	RoleResidual = "residual"
	RoleGap      = "gap"
)

// ResidualID is the id carryResidual draws the residual of one opened node
// under.
func ResidualID(opened string) string { return residualPrefix + opened }

// GapID is the id markGap draws the gap of one opened node under.
func GapID(opened string) string { return gapPrefix + opened }

// IsResidual is whether id names a residual mark rather than a document node.
func IsResidual(id string) bool { return strings.HasPrefix(id, residualPrefix) }

// IsGap is whether id names a gap mark rather than a document node.
func IsGap(id string) bool { return strings.HasPrefix(id, gapPrefix) }

// Mark is one node the client draws that no page prints: which, where, and
// how many cents arrive at it and leave it over the drawn chart's ribbons.
//
// Tier is read off the step's declared tiers, as the client's carryResidual
// and markGap index step.tiers, even where the client drops that column for
// want of room. Ends is a residual's alone: the declared endpoints whose flow
// it carries, sorted.
type Mark struct {
	ID       string
	Role     string
	Tier     int
	InCents  int64
	OutCents int64
	Ends     []string
	// Label, Rationale and SourceNote are the words a reader meets on the
	// mark; they are claims about the documents, so Go composes them.
	Label      string
	Rationale  string
	SourceNote string
	// Locators is a gap's citations: the pages of the two totals it subtracts.
	Locators []Locator
}

// Carry is what one mark adds to the drawn chart and what it takes out: the
// mark's record, the nodes to add -- the mark itself, and for a residual the
// endpoints it brings along from the chart above -- the ribbons to add, and
// Splice, the indices into the drawn chart's links that those ribbons
// replace, sorted. A gap splices nothing; a residual re-points what the
// chart already draws and splices the originals out. Splice is by index
// because value-equal ribbons exist and the client splices by identity.
type Carry struct {
	Mark   Mark
	Nodes  []GraphNode
	Links  []GraphLink
	Splice []int
}

// Gap licenses one gap mark: the column it may stand in, the signed cents it
// comes to there -- what the chart above sends into the opened node less what
// the drawn document draws out of it -- and why the two documents differ, as
// terminated sentences. The cents is the one figure of a gap the client cannot
// sum for itself: it is structure's, and the client holds the drawn difference
// to it.
type Gap struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	Cents      int64  `json:"cents"`
	Reason     string `json:"reason"`
}

// Gaps is one node's licences, one per column it differs in.
type Gaps []Gap

// GapOf is markGap's arithmetic: what the drawn chart's ribbons send into
// opened, less what they send out of it, stated as one mark where the step
// licenses exactly that difference in this column. ok is false where the step
// declares no gap at all or the centre balances unlicensed; any other
// difference -- unlicensed, licensed in another column, or at another figure
// -- is an error, and so is a licence for a centre that balances.
//
// The sums are signed, reductions negative, as the client reads them before
// markContra makes them positive; after that pass a centre would read short
// by twice the reductions. Too little leaving stands the mark at the last of
// tiers, too little arriving at the first. It cites every page a ribbon
// touching opened was read from, in from and in doc.
func GapOf(drawn, from, doc Graph, col ColumnKey, opened string, tiers []int, gaps map[string]Gaps) (Carry, bool, error) {
	if len(gaps) == 0 {
		return Carry{}, false, nil
	}
	if !slices.ContainsFunc(drawn.Nodes, func(n GraphNode) bool { return n.ID == opened }) {
		return Carry{}, false, fmt.Errorf("%q is not a mark of the drawn chart, so the gap this step declares has nothing to be stated against", opened)
	}
	var into, outOf int64
	for _, l := range drawn.Links {
		if l.Target == opened {
			into += l.ValueCents
		}
		if l.Source == opened {
			outOf += l.ValueCents
		}
	}
	gap := into - outOf
	where := fmt.Sprintf("FY%d %s", col.FiscalYear, col.Basis)
	var licence *Gap
	for _, l := range gaps[opened] {
		if l.FiscalYear == col.FiscalYear && l.Basis == col.Basis && l.Reason != "" {
			licence = &l
		}
	}
	switch {
	case gap == 0 && licence == nil:
		return Carry{}, false, nil
	case gap == 0:
		return Carry{}, false, fmt.Errorf("the step declares a gap of %d cents on %q in %s and the chart balances there", licence.Cents, opened, where)
	case licence == nil:
		return Carry{}, false, fmt.Errorf("the chart above sends %d into %q and this one draws %d of it, a difference of %d cents that no declaration on this step accounts for in %s; the two documents have drifted apart", into, opened, outOf, abs(gap), where)
	case licence.Cents != gap:
		return Carry{}, false, fmt.Errorf("the step declares a gap of %d cents on %q in %s and the charts differ there by %d", licence.Cents, opened, where, gap)
	}
	if len(tiers) == 0 {
		return Carry{}, false, fmt.Errorf("the step draws no tiers, so the gap on %q has no column to stand in", opened)
	}
	cites, err := citedAround(opened, from, doc)
	if err != nil {
		return Carry{}, false, err
	}
	if len(cites) == 0 {
		return Carry{}, false, fmt.Errorf("no ribbon touching %q cites a page, so the gap on it could cite none", opened)
	}
	centre := opened
	for _, n := range drawn.Nodes {
		if n.ID == opened && n.Label != "" {
			centre = n.Label
		}
	}
	lead := "In " + col.Label + " " + col.Basis + ", the chart above puts " + dollars(into) + " through " +
		centre + " and the schedule this chart is drawn from accounts for " + dollars(outOf) +
		" of it, " + dollars(gap) + " less."
	if gap < 0 {
		lead = "In " + col.Label + " " + col.Basis + ", the schedule this chart is drawn from accounts for " +
			dollars(outOf) + " through " + centre + ", " + dollars(-gap) + " more than the " + dollars(into) +
			" the chart above puts through it."
	}
	m := Mark{
		ID:    GapID(opened),
		Role:  RoleGap,
		Label: "Difference between the two schedules",
		Rationale: lead + " " + licence.Reason + " This mark is that " + dollars(abs(gap)) +
			", drawn so that the ribbons and the node agree; no page prints it as a figure of its own.",
		SourceNote: "Derived, not published: one document's total for this cell less the " +
			"other's. Each total is built from figures `fisc verify` ties to the pages the city " +
			"printed, and the difference is the one declared for this column; no page prints " +
			"it as a figure of its own.",
		Locators: cites,
	}
	var link GraphLink
	if gap > 0 {
		m.Tier, m.InCents = tiers[len(tiers)-1], gap
		link = GraphLink{Source: opened, Target: m.ID, ValueCents: gap}
	} else {
		m.Tier, m.OutCents = tiers[0], -gap
		link = GraphLink{Source: m.ID, Target: opened, ValueCents: -gap}
	}
	node := GraphNode{ID: m.ID, Tier: m.Tier, Role: RoleGap, Parent: "", Derived: true}
	return Carry{Mark: m, Nodes: []GraphNode{node}, Links: []GraphLink{link}}, true, nil
}

// citedAround is every page cited by a ribbon of gs with an end at opened or
// inside it, merged per document, documents and pages sorted.
func citedAround(opened string, gs ...Graph) ([]Locator, error) {
	pages := map[string]map[int]bool{}
	for _, g := range gs {
		chains, err := ancestry(g, indexNodes(g))
		if err != nil {
			return nil, err
		}
		inside := func(id string) bool {
			return slices.ContainsFunc(chains[id], func(a GraphNode) bool { return a.ID == opened })
		}
		for _, l := range g.Links {
			if !inside(l.Source) && !inside(l.Target) {
				continue
			}
			for _, loc := range l.Locators {
				if pages[loc.DocID] == nil {
					pages[loc.DocID] = map[int]bool{}
				}
				for _, p := range loc.Pages {
					pages[loc.DocID][p] = true
				}
			}
		}
	}
	var out []Locator
	for _, d := range slices.Sorted(maps.Keys(pages)) {
		out = append(out, Locator{DocID: d, Pages: slices.Sorted(maps.Keys(pages[d]))})
	}
	return out, nil
}

// ResidualOf is carryResidual's rule: the flows the chart above prints for
// the opened node that the document this rung draws does not decompose,
// re-pointed onto one mark beside the node's parts. drawn is the rung's
// window as spliced, from the document of the chart it was opened from,
// unfolded, and doc the document this step draws, unfolded. ok is false
// where the step declares no residual or nothing is carried.
//
// Whole or nothing per declared endpoint, in sorted id order: its inflow is
// carried only where doc carries nothing from it into the opened node, its
// outflow only where drawn decomposes the node and doc carries nothing from
// inside to it; where both, the outflow's placement wins, as in the client.
//
// An endpoint's ribbons come off drawn where it draws any (spliced out by
// index), else off from, never both: the window's flank is already on
// screen, and taking both doubled transfers/in against what p0067 prints.
//
// The mark stands at the shallowest declared tier of any node inside the
// opened one, read off doc; with none it is an error. Its in and out need
// not balance, and the client sizes it at the larger.
func ResidualOf(drawn, from, doc Graph, opened string, tiers []int, residual map[string]string, grain string) (Carry, bool, error) {
	if len(residual) == 0 {
		return Carry{}, false, nil
	}
	if len(tiers) == 0 {
		return Carry{}, false, fmt.Errorf("the step draws no tiers, so the residual of %q has no column to stand in", opened)
	}
	chains, err := ancestry(doc, indexNodes(doc))
	if err != nil {
		return Carry{}, false, err
	}
	inside := map[string]bool{}
	for id, chain := range chains {
		if slices.ContainsFunc(chain, func(a GraphNode) bool { return a.ID == opened }) {
			inside[id] = true
		}
	}
	if !inside[opened] {
		return Carry{}, false, fmt.Errorf("the document does not carry node %q, so nothing can be residual beside its parts", opened)
	}
	decomposed := slices.ContainsFunc(drawn.Links, func(l GraphLink) bool { return inside[l.Source] && l.Source != opened })
	carriesFrom := func(e string) bool {
		return slices.ContainsFunc(doc.Links, func(l GraphLink) bool { return l.Source == e && inside[l.Target] })
	}
	carriesTo := func(e string) bool {
		return slices.ContainsFunc(doc.Links, func(l GraphLink) bool { return l.Target == e && inside[l.Source] })
	}
	above := func(want func(GraphLink) bool) (hits []GraphLink, at []int) {
		for i, l := range drawn.Links {
			if want(l) {
				hits, at = append(hits, l), append(at, i)
			}
		}
		if len(hits) > 0 {
			return hits, at
		}
		for _, l := range from.Links {
			if want(l) {
				hits = append(hits, l)
			}
		}
		return hits, nil
	}
	id := ResidualID(opened)
	var c Carry
	var ends []string
	arrives := map[string]bool{}
	note := func(e string, in bool) {
		if _, seen := arrives[e]; !seen {
			ends = append(ends, e)
		}
		arrives[e] = in
	}
	for _, e := range slices.Sorted(maps.Keys(residual)) {
		if !carriesFrom(e) {
			hits, at := above(func(l GraphLink) bool { return l.Source == e && l.Target == opened })
			for _, l := range hits {
				l.Target = id
				c.Links = append(c.Links, l)
				note(e, true)
			}
			c.Splice = append(c.Splice, at...)
		}
		if decomposed && !carriesTo(e) {
			hits, at := above(func(l GraphLink) bool { return l.Source == opened && l.Target == e })
			for _, l := range hits {
				l.Source = id
				c.Links = append(c.Links, l)
				note(e, false)
			}
			c.Splice = append(c.Splice, at...)
		}
	}
	if len(c.Links) == 0 {
		return Carry{}, false, nil
	}
	slices.Sort(c.Splice)
	c.Splice = slices.Compact(c.Splice)
	// Endpoints come with their ribbons, at the first declared tier when the
	// flow arrives and the last when it leaves; one already drawn stays put.
	have := indexNodes(drawn)
	fromByID := indexNodes(from)
	for _, e := range ends {
		n, ok := fromByID[e]
		if _, drawnAlready := have[e]; !ok || drawnAlready {
			continue
		}
		n.Tier, n.Parent = tiers[len(tiers)-1], ""
		if arrives[e] {
			n.Tier = tiers[0]
		}
		c.Nodes = append(c.Nodes, n)
	}
	tier, placed := 0, false
	for _, n := range doc.Nodes {
		if n.ID != opened && inside[n.ID] && slices.Contains(tiers, n.Tier) && (!placed || n.Tier < tier) {
			tier, placed = n.Tier, true
		}
	}
	if !placed {
		return Carry{}, false, fmt.Errorf("%q has no part at a tier this step draws to stand the residual beside", opened)
	}
	sortedEnds := slices.Sorted(slices.Values(ends))
	label := func(n string) string {
		for _, g := range doc.Nodes {
			if g.ID == n && g.Label != "" {
				return g.Label
			}
		}
		for _, g := range from.Nodes {
			if g.ID == n && g.Label != "" {
				return g.Label
			}
		}
		return n
	}
	// The reasons are the step's own words, one per endpoint, in Ends order.
	reasons := make([]string, 0, len(sortedEnds))
	for _, e := range sortedEnds {
		why := residual[e]
		if strings.TrimSpace(why) == "" {
			return Carry{}, false, fmt.Errorf("%q is carried onto the residual mark with no reason declared for it", e)
		}
		reasons = append(reasons, label(e)+": "+why+".")
	}
	c.Mark = Mark{
		ID: id, Role: RoleResidual, Tier: tier, Ends: sortedEnds,
		Label: "Not split by " + grain + " here",
		// No plural is formed from the grain.
		Rationale: "Money the chart above prints for " + label(opened) + " as a whole and " +
			"that the schedule this chart is drawn from does not split by " + grain + ", so " +
			"no " + grain + " here receives or pays it. It is drawn beside the opened node's " +
			"parts rather than attributed to one of them, and what flows in and what flows " +
			"out need not balance: the difference is what that schedule does not break " +
			"down. " + strings.Join(reasons, " "),
	}
	for _, l := range c.Links {
		if l.Target == id {
			c.Mark.InCents += l.ValueCents
		} else {
			c.Mark.OutCents += l.ValueCents
		}
	}
	c.Nodes = append(c.Nodes, GraphNode{ID: id, Tier: tier, Role: RoleResidual, Parent: opened, Derived: true})
	return c, true, nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
