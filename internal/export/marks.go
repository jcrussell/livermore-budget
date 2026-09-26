package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// residualPrefix is the id prefix site/app.js draws a residual node under,
// and gapPrefix the one it draws a gap node under: each followed by the id
// of the node the reader opened.
//
// TWO PREFIXES AND NOT ONE, because the two marks make different claims and
// a check counting one must not find the other. A residual is money the
// chart above prints that the drawn document carries no row for, copied
// across with its citations; a gap is one cell two schedules print at two
// figures, which no page prints at all.
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
// THE TIER IS THE STEP'S OWN, READ OFF ITS DECLARED TIERS AND NOT OFF THE
// COLUMNS THE CLIENT ENDS UP DRAWING. carryResidual and markGap both index
// step.tiers, so a mark can be placed at a column the client dropped for
// want of room, and this reproduces that rather than correcting it: the
// artifact is Go's reading of what the client draws, and a placement Go
// quietly repaired would be one the arm could never see disagree.
//
// A mark is the same at every width the client may choose, measured: every
// path the artifact answered at both three and four columns carried
// byte-identical marks at both, so these are emitted once per rung.
//
// Ends is a residual's alone: the declared endpoints whose flow it carries,
// sorted. A gap has one ribbon and no endpoint.
type Mark struct {
	ID       string
	Role     string
	Tier     int
	InCents  int64
	OutCents int64
	Ends     []string
	// Label, Rationale and SourceNote are the words a reader meets on the mark.
	//
	// THEY ARE CLAIMS ABOUT THE DOCUMENTS AND SO THEY ARE GO'S. A residual says
	// which flow a schedule does not split by fund and why; a gap says which
	// cell two documents disagree about and by how much. Composed in the client
	// they were sentences no Go check could be held against -- and one of them
	// said `fisc verify` holds a difference it does not hold (fisc-4lsx).
	//
	// THE LABEL IS DECLARED AND NOT DERIVED FROM A TIER. "Not broken down by
	// fund" was hard-coded in the client and right only because every residual
	// on the committed corpus stands on a fund-group rung; it is the step's
	// noun that makes it true, and the step is what names it here.
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
// chart already draws and splices the originals out.
//
// SPLICE IS BY INDEX AND NOT BY VALUE, because the client's is by identity
// and value-equal ribbons exist: two ends can carry the same cents of the
// same kind, and a splice by value would take both where the client takes
// one.
type Carry struct {
	Mark   Mark
	Nodes  []GraphNode
	Links  []GraphLink
	Splice []int
}

// Gap licenses one gap mark: the column it may stand in, the signed cents it
// comes to there -- what the chart above sends into the opened node less what
// the drawn document draws out of it -- and why the two documents differ, as
// terminated sentences.
type Gap struct {
	FiscalYear int
	Basis      string
	Cents      int64
	Reason     string
}

// Gaps is one node's licences, one per column it differs in. On the wire it
// is the reasons alone: the client reads only that a gap is declared.
type Gaps []Gap

// MarshalJSON writes the licences' reasons as one string.
func (g Gaps) MarshalJSON() ([]byte, error) {
	reasons := make([]string, 0, len(g))
	for _, l := range g {
		reasons = append(reasons, l.Reason)
	}
	return json.Marshal(strings.Join(reasons, " "))
}

// GapOf is markGap's arithmetic: what the drawn chart's ribbons send into
// opened, less what they send out of it, stated as one mark where the step
// licenses exactly that difference in this column. ok is false where the step
// declares no gap at all or the centre balances unlicensed; any other
// difference -- unlicensed, licensed in another column, or at another figure
// -- is an error, and so is a licence for a centre that balances.
//
// SIGNED, OVER THE DRAWN CHART AS IT STANDS. The sums are of ValueCents as
// the fold left them, reductions negative, which is what the client reads
// before markContra makes them positive; after that pass a centre taking a
// category's gross against the spine's net reads as a shortfall of twice
// the reductions.
//
// THE SHORT SIDE DECIDES WHERE THE MARK GOES: too little leaving stands at
// the last of tiers, too little arriving at the first, and the one ribbon
// runs from opened to the mark or from the mark to opened accordingly.
//
// The mark cites every page a ribbon touching opened was read from, in from
// (the chart above's document) and in doc (the drawn one): the pages of the
// two totals its rationale subtracts.
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
// WHOLE OR NOTHING PER DECLARED ENDPOINT, in sorted id order: an endpoint's
// flow into the opened node is carried only where doc carries nothing from
// that endpoint into any node inside the opened one, and its flow out only
// where the drawn chart decomposes the node at all -- some ribbon leaves a
// node inside it other than itself -- and doc carries nothing from inside
// to that endpoint. Where an endpoint is on both sides the outflow's
// placement wins, which is the client's last write to its map.
//
// THE RIBBONS ARE THE CHART ABOVE'S, TAKEN FROM WHERE THEY ARE DRAWN. An
// endpoint's ribbons come off the drawn chart where it draws any, and off
// from where it draws none, which is every endpoint the window's tiers do
// not hold; both are never taken, because a window keeps a flank of the
// chart above and its ribbons are already on screen pointing at the opened
// node -- taking the file's as well drew transfers/in at 960,800 against
// the 480,400 p0067 prints. A ribbon taken off drawn is spliced out by
// index; one taken off from replaces nothing.
//
// THE MARK STANDS AT THE SHALLOWEST DECLARED TIER OF ANY NODE INSIDE THE
// OPENED ONE, read off doc, and a node with no part at a declared tier is
// an error rather than a mark beside nothing. Its in and out differ by
// construction: the difference is what the drawn document does not break
// down, and the client sizes the mark at the larger.
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
	// THE ENDPOINTS COME WITH THEIR RIBBONS, at the first declared tier when
	// the flow arrives and the last when it leaves, parentless, with the
	// record the chart above holds for them otherwise; one the window
	// already draws keeps its place.
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
	// THE REASONS ARE THE STEP'S OWN WORDS, one per declared endpoint, in the
	// order the mark carries them. They are why a reader is told no part of the
	// opened node receives that flow, in the words the check that guards the
	// identity declares it in rather than in a paraphrase.
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
		Label: "Not broken down by " + grain,
		// NO PLURAL IS FORMED FROM THE GRAIN. "the opened node's parts" says
		// what "the funds" said without a rule for turning one word into
		// another, which is a rule this would get wrong on the first grain that
		// does not take an s.
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
