package export

import (
	"fmt"
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
// COLUMNS A BUDGET LEFT DRAWN. carryResidual and markGap both index
// step.tiers, so a mark can be placed at a column a narrow budget dropped,
// and this reproduces that rather than correcting it: the artifact is Go's
// reading of what the client draws, and a placement Go quietly repaired
// would be one the arm could never see disagree.
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

// GapOf is markGap's arithmetic: what the drawn chart's ribbons send into
// opened, less what they send out of it, stated as one mark where the step
// declares a reason for it. ok is false where the step declares no gap at
// all or the centre balances; a non-zero difference on a node the
// declaration does not name is an error, which is the client's throw and
// the only teeth a declaration has.
//
// SIGNED, OVER THE DRAWN CHART AS IT STANDS. The sums are of ValueCents as
// the fold left them, reductions negative, which is what the client reads
// before markContra makes them positive; after that pass a centre taking a
// category's gross against the spine's net reads as a shortfall of twice
// the reductions. Reach.In and Reach.Out are the wrong input here for the
// same reason: they are absolute-valued and pre-fold.
//
// THE SHORT SIDE DECIDES WHERE THE MARK GOES: too little leaving stands at
// the last of tiers, too little arriving at the first, and the one ribbon
// runs from opened to the mark or from the mark to opened accordingly. An
// empty declaration is no declaration, which is what the client is handed:
// the step's map is omitted from the JSON when empty.
func GapOf(drawn Graph, opened string, tiers []int, gaps map[string]string) (Carry, bool, error) {
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
	if gap == 0 {
		return Carry{}, false, nil
	}
	if gaps[opened] == "" {
		return Carry{}, false, fmt.Errorf("the chart above sends %d into %q and this one draws %d of it, a difference of %d cents that no declaration on this step accounts for; the two documents have drifted apart", into, opened, outOf, abs(gap))
	}
	if len(tiers) == 0 {
		return Carry{}, false, fmt.Errorf("the step draws no tiers, so the gap on %q has no column to stand in", opened)
	}
	m := Mark{ID: GapID(opened), Role: RoleGap}
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

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
