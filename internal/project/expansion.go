package project

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/schema"
)

// ExpansionProjection is the stitched schedule's name and file stem.
const ExpansionProjection = "expansion"

// ExpansionScopes is every schedule the stitch reads, the spine first.
func ExpansionScopes() []string {
	return []string{
		PublishedScope, ScopeRevenueByFund, scopeExpenditureByDepartment, scopeExpenditureByFund,
		TransfersByFundScope, CIPFundingScope, FundSourcesUsesScope, DepartmentSpendingScope,
	}
}

// SplitExtend is the one form a split takes: the node stays where it is and
// its members are drawn in a column of their own after it, each reached by
// links from the node.
const SplitExtend = "extend"

// SplitForms is the closed set of split forms.
func SplitForms() []string { return []string{SplitExtend} }

// SplitAfter and SplitBefore are the sides of its node a split's members
// stand on: after it where the node's links to them run from it, before it
// where they run from them into it, as a hub's do at a node taking nothing
// in on the spine.
const (
	SplitAfter  = "after"
	SplitBefore = "before"
)

// SplitSides is the closed set of split sides.
func SplitSides() []string { return []string{SplitAfter, SplitBefore} }

// Split is one way a node of the expansion schedule opens. Its members are
// the nodes whose Parent is Node and whose Alternative is Key on an
// alternative split, or "" on the primary one.
type Split struct {
	Node string `json:"node"`
	Key  string `json:"key"`
	Form string `json:"form"`
	// Alternative is false on the one primary split a node has, whose
	// members and links are read whenever the node is opened by it, and
	// true on a split a reader chooses instead of it, whose members and
	// links carry its Key.
	Alternative bool `json:"alternative"`
	// Side is SplitAfter or SplitBefore: which way every link between
	// Node and a member of this split runs, and so the column its members
	// stand in. [IsLeg] reads a hub's legs by it.
	Side string `json:"side"`
}

// sideOf is the side a split of d stands its members on at a node that
// does, or does not, take money in on the spine.
func sideOf(d splitDecl, takesIn bool) string {
	if d.hub && !takesIn {
		return SplitBefore
	}
	return SplitAfter
}

// SideOf is the side the stitch stands the members of a node's split on: a
// node of role, split by key (the primary split's whatever its key), at a
// node that does or does not take money in on the spine. It is false for a
// split no role declares.
func SideOf(role, key string, alternative, takesIn bool) (string, bool) {
	d, ok := splitOf(role, key, alternative)
	if !ok {
		return "", false
	}
	return sideOf(d, takesIn), true
}

// PrimarySides is each node's primary split's side, by node.
func PrimarySides(splits []Split) map[string]string {
	out := map[string]string{}
	for _, sp := range splits {
		if !sp.Alternative {
			out[sp.Node] = sp.Side
		}
	}
	return out
}

// IsLeg answers whether a link of the expansion schedule is a hub leg: it
// repeats a leaf and runs between a node and its own primary member, and is
// not that leaf re-pointed onto the node -- the node at the end its split's
// side re-points (the target after, the source before), the far end the
// leaf's own. So a flow between two members of a hub, re-pointed onto it, is
// a leaf, while a leg run against its side stays a leg for
// expansion-splits-tie to name. sides is [PrimarySides] of the schedule.
func IsLeg(l Link, nodes map[string]Node, sides map[string]string) bool {
	if l.Repeats == "" || l.Alternative != "" {
		return false
	}
	member := func(id, parent string) bool {
		m, ok := nodes[id]
		return ok && m.Parent == parent && m.Alternative == ""
	}
	if !member(l.Target, l.Source) && !member(l.Source, l.Target) {
		return false
	}
	src, tgt := LeafEnds(l.Repeats)
	if member(l.Source, l.Target) && sides[l.Target] == SplitAfter && l.Source == src {
		return false
	}
	if member(l.Target, l.Source) && sides[l.Source] == SplitBefore && l.Target == tgt {
		return false
	}
	return true
}

// splitDecl is what a node of one role splits into.
type splitDecl struct {
	role        string
	key         string
	form        string
	alternative bool
	// hub is an extended split whose links to its members repeat the leaves
	// at those members, one leg per leaf ([extendHubs]), rather than citing
	// a total of their own.
	hub bool
	// restates is, on a split citing totals of its own, which leaves those
	// totals state again ([declareRestating]).
	restates restating
}

// restating is which leaves a totals split's links to its members state
// again: each member's onward leaves, or together every leaf arriving at the
// node.
type restating int

const (
	restatesNothing restating = iota
	restatesOnward
	restatesInflow
)

// expansionSplits is every split the stitch declares, by the role of the node
// that splits. Each is declared once, here; the builder attaches the members.
var expansionSplits = []splitDecl{
	{RoleRevenueSource, "lines", SplitExtend, false, true, restatesNothing},
	{RoleTransferIn, "payers", SplitExtend, false, true, restatesNothing},
	{RoleFundGroup, "funds", SplitExtend, false, true, restatesNothing},
	{RoleGeneralFund, "divisions", SplitExtend, false, false, restatesOnward},
	{RoleObjectCategory, "object-rows", SplitExtend, false, true, restatesNothing},
	{RoleObjectCategory, "divisions", SplitExtend, true, false, restatesInflow},
	{RoleTransferOut, "receivers", SplitExtend, false, true, restatesNothing},
}

// SplitKeys is every split key the stitch declares, each once, in
// declaration order.
func SplitKeys() []string {
	var out []string
	for _, d := range expansionSplits {
		if !slices.Contains(out, d.key) {
			out = append(out, d.key)
		}
	}
	return out
}

// splitOf is the declaration for a role's primary split, or its alternative
// named key.
func splitOf(role, key string, alternative bool) (splitDecl, bool) {
	for _, d := range expansionSplits {
		if d.role == role && d.alternative == alternative && (!alternative || d.key == key) {
			return d, true
		}
	}
	return splitDecl{}, false
}

// alternativeDivisions is the key of the object categories' alternative split
// into pp.85-125's divisions.
const alternativeDivisions = "divisions"

// PrefixGroupOnly is a node carrying money pp.66-67 print for a fund group and
// no fund page attributes to a fund; PrefixNoDivision one carrying an object
// category's money pp.66-67 print and no division on pp.85-125 does. Both
// are derived.
const (
	PrefixGroupOnly  = "fund-group-only/"
	PrefixNoDivision = "no-division/"
)

// groupObjectID is an object category's money a fund group's column of
// pp.66-67 prints and its funds' rows do not, beside the funds' own rows.
func groupObjectID(group, category string) string {
	return PrefixExpenditure + PrefixFundGroup + group + "/" + category
}

// IsRemainder answers whether a node id is one the stitch derives to carry a
// difference between a printed cell and its members.
func IsRemainder(id string) bool {
	return strings.HasPrefix(id, PrefixGroupOnly) || strings.HasPrefix(id, PrefixNoDivision) ||
		strings.HasPrefix(id, PrefixExpenditure+PrefixFundGroup)
}

// expansion stitches one schedule per spine column out of the cuts that
// decompose the spine, for the client to fold to a frontier of node ids.
//
// # The shape
//
//	roots     the spine's nodes, parent ""
//	revenue/<category>    <- revenue-line/<line>            hub, lines
//	transfers/in          <- transfer-from/<payer>          hub, payers
//	fund-group/<type>     <- fund/<n>, fund-group-only/<type> hub, funds
//	fund/100              <- dept/<division>                totals, divisions
//	expenditure/<object>  <- expenditure/<division>/<object>,
//	                         expenditure/fund/<n>/<object>,
//	                         expenditure/fund-group/<type>/<object>
//	                                                         hub, object rows
//	                      <- dept/<division>/<object>, no-division/<object>
//	                                                         totals, divisions (alternative)
//	transfers/out         <- transfer-to/<receiver>         hub, receivers
//
// Every split extends: its node stays drawn and its members stand in a column
// beside it. A split marked totals reaches each member by one link citing
// that member's own total, naming the leaves it states again
// ([declareRestating]). A hub split reaches its members by legs that repeat
// the leaves at them ([extendHubs]): a revenue category's and transfers/in's
// the leaves leaving each member, which are re-pointed to start at the node,
// a fund group's, an object category's and transfers/out's the leaves
// arriving at each, which are re-pointed to end at it. Every other primary
// link is a leaf link, whose facts a page prints at its own ends. The roots' cells are the leaves summed, which
// expansion-opens-as-the-spine holds to the spine, and each split's members
// are held to their parent's printed cells, and each leg to its leaf, by
// expansion-splits-tie.
//
// A remainder node carries money a parent's printed cell holds and no member's
// does. A fund group whose funds' balances move in opposite directions draws
// its change in working capital on one, citing pp.66-67's row, since netting
// fund by fund is a ribbon from a fund to its own column. Any other remainder
// is derived from the structure.Exception declaring its cell, which is the
// one source of its existence; its value is the difference of the printed
// facts it cites, and the build refuses one that is not the exception's.
type expansion struct {
	Labels labels
}

var (
	_ Projection = (*expansion)(nil)
	_ Sliced     = (*expansion)(nil)
)

// Name is [Projection]'s, and it is this document's file stem.
func (*expansion) Name() string { return ExpansionProjection }

// Slices is one Options per column every schedule it stitches prints: a
// column missing one would leave a spine cell its leaves do not reach.
func (*expansion) Slices(facts []fact.Fact, version string) []Options {
	return columnsCarrying(facts, ExpansionScopes(), ExpansionScopes(), nil, version)
}

// Build is [Projection]'s entry point.
func (e *expansion) Build(facts []fact.Fact, o Options) ([]byte, error) {
	doc, err := e.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(doc, e.Name(), schema.Projection)
}

// stitch is one column's schedule under construction.
type stitch struct {
	nodes map[string]Node
	links []Link
	// apart is every fund whose group's change is drawn whole on the group's
	// remainder while the fund's own is not drawn: its in and out differ.
	apart []string
}

// add records a node under a parent and an alternative, refusing a second
// placement of one id: every node has one parent per split.
func (s *stitch) add(n Node, parent, alternative string) error {
	n.Parent = parent
	n.Alternative = alternative
	if have, ok := s.nodes[n.ID]; ok {
		if have != n {
			return fmt.Errorf("expansion: node %q is placed twice: %+v and %+v", n.ID, have, n)
		}
		return nil
	}
	s.nodes[n.ID] = n
	return nil
}

// Document builds the stitched schedule and returns it, so `fisc verify`
// reads the same structure `fisc export` writes.
func (e *expansion) Document(facts []fact.Fact, o Options) (*Document, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("expansion options: %w", err)
	}
	if !sameScopes(o.Scopes, ExpansionScopes()) || len(o.Kinds) != 0 || o.ThroughCuts {
		return nil, hint.With(
			fmt.Errorf("expansion: scopes are %q, kinds %v and through-cuts %v, want %q, every kind and false",
				o.ScopeList(), o.Kinds, o.ThroughCuts, Options{Scopes: ExpansionScopes()}.ScopeList()),
			"the stitch reads each schedule through the document already built over it, and "+
				"selects every fact any of them cites")
	}
	if len(o.Columns) != 1 {
		return nil, fmt.Errorf("expansion: a stitch is of one column, got %d", len(o.Columns))
	}
	if e.Labels == nil {
		return nil, fmt.Errorf("expansion: no registry is attached, and a fund's group is data/funds.yaml's")
	}
	col := o.Columns[0]
	selected, selErr := SelectFacts(facts, o)
	if selErr != nil {
		return nil, fmt.Errorf("expansion: %w", selErr)
	}
	byID := factIndexOf(selected)

	docs, cutErr := e.cuts(facts, o)
	if cutErr != nil {
		return nil, cutErr
	}
	st := &stitch{nodes: map[string]Node{}}
	spine := docs[PublishedProjection]
	roots := map[string]Node{}
	for _, n := range spine.Nodes {
		roots[n.ID] = n
		if err := st.add(n, "", ""); err != nil {
			return nil, err
		}
	}
	nodesOf := func(d *Document) map[string]Node {
		out := make(map[string]Node, len(d.Nodes))
		for _, n := range d.Nodes {
			out[n.ID] = n
		}
		return out
	}
	fsu := docs[FundSourcesUsesProjection]
	fsuNodes := nodesOf(fsu)
	// A fund hangs from the group data/funds.yaml types it, which must be a
	// root: a fund whose group the spine does not draw has nowhere to fold.
	// It carries pp.186-209's balances from fund-sources-uses' node, the one
	// pointer whichever schedule placed it, so the placements stay equal.
	addFund := func(n Node) error {
		n.Balances = fsuNodes[n.ID].Balances
		number, convErr := strconv.Atoi(strings.TrimPrefix(n.ID, PrefixFund))
		if convErr != nil {
			return fmt.Errorf("expansion: %q is no fund id", n.ID)
		}
		t, ok := e.Labels.FundType(number)
		if !ok {
			return fmt.Errorf("expansion: fund %d is in no data/funds.yaml entry", number)
		}
		group := PrefixFundGroup + t
		if _, ok := roots[group]; !ok {
			return hint.With(fmt.Errorf("expansion: fund %d is in %s, which the spine does not draw in %s",
				number, group, col),
				"every leaf folds to a spine node, so a fund of a group pp.66-67 give no column has no root")
		}
		return st.add(n, group, "")
	}
	link := func(l Link) { st.links = append(st.links, l) }

	// Revenue: pp.127-140's lines into their funds. The transfers in they
	// print are p76's legs below, which print the General Fund's too.
	ff := docs[FundFlowsProjection]
	ffNodes := nodesOf(ff)
	for _, l := range ff.Links {
		switch {
		case strings.HasPrefix(l.Source, PrefixRevenueLine) && strings.HasPrefix(l.Target, PrefixFund):
			line := ffNodes[l.Source]
			if err := st.add(line, line.Parent, ""); err != nil {
				return nil, err
			}
			if err := addFund(ffNodes[l.Target]); err != nil {
				return nil, err
			}
			link(l)
		case strings.HasPrefix(l.Source, PrefixDept) && strings.HasPrefix(l.Target, PrefixExpenditure),
			strings.HasPrefix(l.Source, PrefixFund) && strings.HasPrefix(l.Target, PrefixExpenditure):
			// An object row hangs from its object category, its owner kept
			// as the link's source.
			category, catErr := categoryOf(l, byID)
			if catErr != nil {
				return nil, catErr
			}
			parent := PrefixExpenditure + category
			if _, ok := roots[parent]; !ok {
				return nil, fmt.Errorf("expansion: %s spends on %q, which the spine draws no node for", l.Source, parent)
			}
			owner := ffNodes[l.Source]
			if strings.HasPrefix(l.Source, PrefixDept) {
				if err := st.add(owner, owner.Parent, ""); err != nil {
					return nil, err
				}
			} else if err := addFund(owner); err != nil {
				return nil, err
			}
			row := ffNodes[l.Target]
			if err := st.add(row, parent, ""); err != nil {
				return nil, err
			}
			link(l)
		case strings.HasPrefix(l.Source, PrefixFund) && strings.HasPrefix(l.Target, PrefixDept):
			// fund/100 extends into its divisions: the link carrying each
			// division's total is the one an opened fund draws.
			if err := addFund(ffNodes[l.Source]); err != nil {
				return nil, err
			}
			division := ffNodes[l.Target]
			if err := st.add(division, l.Source, ""); err != nil {
				return nil, err
			}
			link(l)
		}
	}

	// Transfers in, by the fund paying them: p76's receiving legs.
	tin := docs[TransfersByFundProjection]
	tinNodes := nodesOf(tin)
	for _, l := range tin.Links {
		if !strings.HasPrefix(l.Source, PrefixTransferFrom) {
			continue
		}
		payer := tinNodes[l.Source]
		if err := st.add(payer, NodeTransfersIn, ""); err != nil {
			return nil, err
		}
		if err := addFund(tinNodes[l.Target]); err != nil {
			return nil, err
		}
		link(l)
	}

	// Transfers out, by the fund receiving them: p76's paying legs and p222's
	// transfers to the CIP.
	tout := docs[TransfersOutProjection]
	toutNodes := nodesOf(tout)
	for _, l := range tout.Links {
		if !strings.HasPrefix(l.Target, PrefixTransferTo) {
			continue
		}
		if err := addFund(toutNodes[l.Source]); err != nil {
			return nil, err
		}
		receiver := toutNodes[l.Target]
		if err := st.add(receiver, NodeTransfersOut, ""); err != nil {
			return nil, err
		}
		link(l)
	}

	// pp.186-209: each fund's addition to reserves, and each fund's change in
	// balance where its group's funds all move one way.
	changes := map[string][]Link{}
	for _, l := range fsu.Links {
		switch {
		case l.Target == CategoryFundBalanceReserveIncrease:
			if err := addFund(fsuNodes[l.Source]); err != nil {
				return nil, err
			}
			link(l)
		case l.Source == NodeFundBalanceDraw:
			changes[e.groupOf(l.Target)] = append(changes[e.groupOf(l.Target)], l)
		case l.Target == NodeFundBalanceContribution:
			changes[e.groupOf(l.Source)] = append(changes[e.groupOf(l.Source)], l)
		}
	}
	remainders := &groupRemainders{roots: roots, st: st}
	for _, l := range spine.Links {
		var group string
		switch {
		case l.Source == NodeFundBalanceDraw:
			group = l.Target
		case l.Target == NodeFundBalanceContribution:
			group = l.Source
		default:
			continue
		}
		if err := e.changeOf(st, remainders, l, group, changes[group], fsuNodes, byID, col, addFund); err != nil {
			return nil, err
		}
		delete(changes, group)
	}
	// A group the spine prints no change for draws none fund by fund either,
	// so its funds' changes must net to nothing.
	for _, group := range sortedKeys(changes) {
		var net int64
		for _, l := range changes[group] {
			if l.Source == NodeFundBalanceDraw {
				net -= l.ValueCents
			} else {
				net += l.ValueCents
			}
		}
		if net != 0 {
			return nil, fmt.Errorf("expansion: %q's funds change by %d cents in %s and pp.66-67 print no change for it",
				group, net, col)
		}
	}

	// An object category's cell a fund group's own rows do not reach, held
	// apart by an exception.
	if err := e.objectRemainders(st, remainders, spine, byID, col); err != nil {
		return nil, err
	}

	// The alternative: pp.85-125's divisions extending each object category.
	if err := e.divisionsAlternative(st, docs[DepartmentSpendingProjection], spine, roots, byID, col); err != nil {
		return nil, err
	}

	takesIn := map[string]bool{}
	for _, l := range spine.Links {
		takesIn[l.Target] = true
	}
	if err := extendHubs(st, takesIn); err != nil {
		return nil, err
	}
	if err := declareRestating(st); err != nil {
		return nil, err
	}
	declareCompleting(st, spine)

	splits, splitErr := splitsOf(st.nodes, st.links, takesIn)
	if splitErr != nil {
		return nil, splitErr
	}
	sortLinks(st.links)
	if err := checkDistinctAlternativeLinks(st.links); err != nil {
		return nil, err
	}
	out := sortedNodes(st.nodes)

	c, uncited := tally(selected, st.links, out)
	if err := refuseUncited(e.Name(), uncited, drawnElsewhere); err != nil {
		return nil, err
	}
	var apart []string
	for _, id := range st.apart {
		if _, drawn := st.nodes[id]; drawn {
			apart = append(apart, id)
		}
	}
	slices.Sort(apart)
	cavs := expansionCaveats(c.FactsCitedTwice, out, apart)
	if err := validateCaveats(cavs, nodeIDs(out)); err != nil {
		return nil, fmt.Errorf("%s: %w", col, err)
	}
	return &Document{
		SchemaVersion: SchemaVersion,
		Projection:    e.Name(),
		Metadata: Metadata{
			GeneratedBy:     o.Version,
			FiscalYear:      col.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(col.FiscalYear),
			Basis:           string(col.Basis),
			Scopes:          ExpansionScopes(),
			Currency:        "USD",
			Units:           "cents",
			Sources:         SourcesOf(selected),
			Counts:          c,
			Caveats:         cavs,
		},
		Nodes:  out,
		Links:  st.links,
		Splits: splits,
	}, nil
}

// CellKey names the spine cell from source to target of kind, as a link's
// Completes spells it.
func CellKey(source, target string, kind LinkKind) string {
	return source + " -> " + target + " " + string(kind)
}

// extendHubs draws every hub split's node beside its members, by its side
// ([sideOf]) at a node takesIn says does or does not take money in on the
// spine. After its node, each primary leaf arriving at a member is
// re-pointed to end at the node, and a leg from the node to the member
// repeats it; before it, as at a source, each primary leaf leaving a member
// is re-pointed to start at the node, and a leg from the member to the node
// repeats it, and a leaf arriving at a member is refused. A leaf from a
// member of a hub before its node to a member of one after its node is
// re-pointed at both ends and repeated by a leg at each; a leaf leaving a
// member of a hub after its node keeps that end on its member. A leaf
// between two members of one hub, or between a hub's node and its own
// member, is refused: re-pointed, the first would run from a member back
// into its own node, against the split's side, and the second from the
// node into itself, neither of which the client can draw. Every leaf a leg repeats, and every leg, names that
// leaf's own ends in Repeats. A leg is a copy of one cited leaf: its cents,
// facts, kind, reduction words and derived flag are the leaf's, so the
// stitch publishes no sum, and a member nothing arrives at is reached by no
// leg.
//
// A hub split is declared only on a root, so a re-pointed end is a root and
// nothing above it opens.
func extendHubs(st *stitch, takesIn map[string]bool) error {
	hubOf := func(id string) (string, bool) {
		n := st.nodes[id]
		if n.Parent == "" || n.Alternative != "" {
			return "", false
		}
		d, ok := splitOf(st.nodes[n.Parent].Role, "", false)
		return n.Parent, ok && d.hub
	}
	after := func(hub string) bool {
		d, _ := splitOf(st.nodes[hub].Role, "", false)
		return sideOf(d, takesIn[hub]) == SplitAfter
	}
	var legs []Link
	for i := range st.links {
		l := &st.links[i]
		if l.Alternative != "" {
			continue
		}
		key := CellKey(l.Source, l.Target, l.Kind)
		if hub, ok := hubOf(l.Target); ok && hub == l.Source {
			return fmt.Errorf("expansion: %s runs from %s into its own member, and would be drawn from it back into it", key, hub)
		}
		if hub, ok := hubOf(l.Source); ok && hub == l.Target {
			return fmt.Errorf("expansion: %s runs from a member of %s into it, and would be drawn from it back into it", key, hub)
		}
		leg := func(source, target string) {
			g := *l
			g.Source, g.Target = source, target
			g.FactIDs = slices.Clone(l.FactIDs)
			g.Locators = slices.Clone(l.Locators)
			g.TransferID, g.Completes, g.Repeats = "", "", key
			legs = append(legs, g)
			l.Repeats = key
		}
		for _, end := range []string{l.Target, l.Source} {
			hub, ok := hubOf(end)
			if !ok {
				continue
			}
			if st.nodes[hub].Parent != "" {
				return fmt.Errorf("expansion: %s is a hub split's node and not a root", hub)
			}
			arriving := end == l.Target
			if arriving != after(hub) {
				if arriving {
					return fmt.Errorf("expansion: %s arrives at %s, a member of %s, which takes nothing in on the spine",
						key, end, hub)
				}
				continue
			}
			if arriving {
				leg(hub, end)
				l.Target = hub
			} else {
				leg(end, hub)
				l.Source = hub
			}
		}
		if hub, ok := hubOf(l.Source); ok && hub == l.Target {
			return fmt.Errorf("expansion: %s runs between two members of %s, and would be drawn from %s back into it",
				key, hub, l.Source)
		}
	}
	st.links = append(st.links, legs...)
	return nil
}

// declareRestating names, on every link from a totals split's node to its
// member, the leaves it states again ([Link.Restates]), so a sum over drawn
// ribbons can state that money once. It runs after [extendHubs], whose
// Repeats are the names, and refuses a total that states no leaf.
func declareRestating(st *stitch) error {
	for i := range st.links {
		l := &st.links[i]
		m, ok := st.nodes[l.Target]
		if !ok || m.Parent != l.Source || m.Alternative != l.Alternative || l.Repeats != "" {
			continue
		}
		d, ok := splitOf(st.nodes[l.Source].Role, m.Alternative, m.Alternative != "")
		if !ok || d.restates == restatesNothing {
			continue
		}
		var keys []string
		for _, o := range st.links {
			if o.Alternative != "" || o.Repeats == "" {
				continue
			}
			if (d.restates == restatesOnward && o.Source == m.ID) || (d.restates == restatesInflow && o.Target == l.Source) {
				keys = append(keys, o.Repeats)
			}
		}
		if len(keys) == 0 {
			return fmt.Errorf("expansion: %s -> %s is a total of its own and states no leaf again", l.Source, l.Target)
		}
		slices.Sort(keys)
		l.Restates = slices.Compact(keys)
	}
	return nil
}

// declareCompleting names, on every primary link of a printed spine cell a
// derived leg folds into, the cell they re-form together: all of them is the
// printed figure, so no fewer may be drawn as it.
func declareCompleting(st *stitch, spine *Document) {
	rootOf := func(id string) string {
		for hops := 0; st.nodes[id].Parent != "" && hops <= len(st.nodes); hops++ {
			id = st.nodes[id].Parent
		}
		return id
	}
	legs := map[string][]int{}
	derived := map[string]bool{}
	for i, l := range st.links {
		src, tgt := rootOf(l.Source), rootOf(l.Target)
		if l.Alternative != "" || src == tgt {
			continue
		}
		k := CellKey(src, tgt, l.Kind)
		legs[k] = append(legs[k], i)
		derived[k] = derived[k] || l.Derived
	}
	for _, l := range spine.Links {
		k := CellKey(l.Source, l.Target, l.Kind)
		if l.Derived || !derived[k] {
			continue
		}
		for _, i := range legs[k] {
			st.links[i].Completes = k
		}
	}
}

// stitchedCut is one document the stitch reads and the options it is built
// over.
type stitchedCut struct {
	p interface {
		Name() string
		Document([]fact.Fact, Options) (*Document, error)
	}
	opts Options
}

// stitchedCuts is every document the stitch reads, each once.
func stitchedCuts(l labels) []stitchedCut {
	return []stitchedCut{
		{&sankey{Labels: l}, Options{Scopes: []string{PublishedScope}}},
		{&fundFlows{Labels: l}, Options{Scopes: FundFlowsScopes()}},
		{&transfersByFund{Labels: l}, Options{Scopes: TransfersByFundScopes()}},
		{&transfersByFund{Labels: l, Out: true}, Options{Scopes: TransfersOutScopes(), Kinds: transferKinds}},
		{&fundSourcesUses{Labels: l}, Options{Scopes: FundSourcesUsesScopes(), ThroughCuts: true}},
		{&departmentSpending{Labels: l}, Options{Scopes: DepartmentSpendingScopes()}},
	}
}

// ExpansionStitches is the projection of every document the expansion
// schedule of a column is stitched from, in the order the stitch reads them.
func ExpansionStitches() []string {
	var out []string
	for _, c := range stitchedCuts(nil) {
		out = append(out, c.p.Name())
	}
	return out
}

// cuts builds each document the stitch reads, over the same column.
func (e *expansion) cuts(facts []fact.Fact, o Options) (map[string]*Document, error) {
	out := map[string]*Document{}
	for _, part := range stitchedCuts(e.Labels) {
		opts := part.opts
		opts.Columns = o.Columns
		opts.Version = o.Version
		d, err := part.p.Document(facts, opts)
		if err != nil {
			return nil, fmt.Errorf("expansion: the %s it stitches: %w", part.p.Name(), err)
		}
		out[part.p.Name()] = d
	}
	return out, nil
}

// groupOf is the fund-group id of a fund id, or "" for one data/funds.yaml
// does not type.
func (e *expansion) groupOf(id string) string {
	number, err := strconv.Atoi(strings.TrimPrefix(id, PrefixFund))
	if err != nil {
		return ""
	}
	t, ok := e.Labels.FundType(number)
	if !ok {
		return ""
	}
	return PrefixFundGroup + t
}

// changeOf stitches one fund group's change in working capital: fund by fund
// where every fund's change runs the group's way, with any difference from
// pp.66-67 carried by a remainder an exception licenses; whole, on the
// group's remainder, where they do not.
func (*expansion) changeOf(st *stitch, rem *groupRemainders, spineLink Link, group string, funds []Link,
	fsuNodes map[string]Node, byID map[string]fact.Fact, col Column, addFund func(Node) error) error {
	draw := spineLink.Source == NodeFundBalanceDraw
	oneWay := len(funds) > 0
	var sum int64
	for _, l := range funds {
		oneWay = oneWay && (l.Source == NodeFundBalanceDraw) == draw
		sum += l.ValueCents
	}
	if !oneWay {
		r, err := rem.node(group)
		if err != nil {
			return err
		}
		l := spineLink
		if draw {
			l.Target = r
		} else {
			l.Source = r
		}
		st.links = append(st.links, l)
		for _, f := range funds {
			if f.Source == NodeFundBalanceDraw {
				st.apart = append(st.apart, f.Target)
			} else {
				st.apart = append(st.apart, f.Source)
			}
		}
		return nil
	}
	for _, l := range funds {
		fund := l.Target
		if !draw {
			fund = l.Source
		}
		if err := addFund(fsuNodes[fund]); err != nil {
			return err
		}
		st.links = append(st.links, l)
	}
	diff := spineLink.ValueCents - sum
	if diff == 0 {
		return nil
	}
	// The spine's ending balance differs from the funds' pages; the change
	// differs by the same figure, a draw by its negation.
	pin, ex, err := licensed(structure.CutFundBalanceFlows, structure.LevelFundGroupByCategory, col,
		map[structure.Axis]string{
			structure.AxisFundGroup: strings.TrimPrefix(group, PrefixFundGroup),
			structure.AxisCategory:  CategoryFundBalanceEnding,
		})
	if err != nil {
		return hint.With(fmt.Errorf("expansion: %s's funds change by %d cents and pp.66-67 print %d in %s: %w",
			group, sum, spineLink.ValueCents, col, err),
			"a difference no exception declares is not stitched; measure it and file it")
	}
	want := pin.Against.Cents - pin.Cut.Cents
	if draw {
		want = -want
	}
	if diff != want {
		return fmt.Errorf("expansion: %s's change differs from its funds' by %d cents in %s and exception %q holds %d apart",
			group, diff, col, ex.Name, want)
	}
	r, err := rem.node(group, ex)
	if err != nil {
		return err
	}
	ids := slices.Clone(spineLink.FactIDs)
	for _, l := range funds {
		ids = append(ids, l.FactIDs...)
	}
	l := remainderLink(ids, byID, spineLink.Kind)
	if draw {
		l.Source, l.Target = NodeFundBalanceDraw, r
	} else {
		l.Source, l.Target = r, NodeFundBalanceContribution
	}
	l.ValueCents = diff
	st.links = append(st.links, l)
	return nil
}

// objectRemainders stitches each fund group's object-category cell its funds'
// rows fall short of, as an exception on pp.172-183 against the spine
// declares it: a row of the group's own beside its funds', from the group's
// remainder.
func (e *expansion) objectRemainders(st *stitch, rem *groupRemainders, spine *Document,
	byID map[string]fact.Fact, col Column) error {
	for _, ex := range structure.ExceptionsOn(structure.BudgetBookExceptions(), "fund-expenditures",
		structure.CutSpine, structure.LevelFundGroupByCategory) {
		for _, p := range ex.Cells {
			if p.Year != col.FiscalYear || p.Basis != string(col.Basis) {
				continue
			}
			group := PrefixFundGroup + p.Coords[structure.AxisFundGroup]
			category := PrefixExpenditure + p.Coords[structure.AxisCategory]
			var cell *Link
			for i := range spine.Links {
				if spine.Links[i].Source == group && spine.Links[i].Target == category {
					cell = &spine.Links[i]
				}
			}
			if cell == nil {
				return fmt.Errorf("expansion: exception %q pins %s -> %s in %s, which the spine draws no link for",
					ex.Name, group, category, col)
			}
			ids := slices.Clone(cell.FactIDs)
			var rows int64
			for _, l := range st.links {
				if st.nodes[l.Target].Parent == category && e.ownerGroup(st, l.Source) == group {
					ids = append(ids, l.FactIDs...)
					rows += l.ValueCents
				}
			}
			diff := cell.ValueCents - rows
			if want := p.Against.Cents - p.Cut.Cents; diff != want {
				return fmt.Errorf("expansion: %s -> %s differs from its funds' rows by %d cents in %s and exception %q holds %d apart",
					group, category, diff, col, ex.Name, want)
			}
			r, err := rem.node(group, ex)
			if err != nil {
				return err
			}
			row := Node{
				ID:    groupObjectID(p.Coords[structure.AxisFundGroup], p.Coords[structure.AxisCategory]),
				Label: rem.label(group) + ": " + st.nodes[category].Label,
				Tier:  tierObjectCategory, Role: RoleObjectCategory,
				Derived: true, Rationale: remainderRationale(objectRemainderRationale, ex),
				SourceNote: ex.Printed,
			}
			if err := st.add(row, category, ""); err != nil {
				return err
			}
			l := remainderLink(ids, byID, cell.Kind)
			l.Source, l.Target, l.ValueCents = r, row.ID, diff
			st.links = append(st.links, l)
		}
	}
	return nil
}

// ownerGroup is the fund group a link's owner end belongs to.
func (*expansion) ownerGroup(st *stitch, id string) string {
	for at := id; at != ""; at = st.nodes[at].Parent {
		if strings.HasPrefix(at, PrefixFundGroup) {
			return at
		}
	}
	return ""
}

// divisionsAlternative stitches each object category's alternative split into
// the divisions pp.85-125 print spending it, every fund together.
func (*expansion) divisionsAlternative(st *stitch, ds, spine *Document, roots map[string]Node,
	byID map[string]fact.Fact, col Column) error {
	dsNodes := map[string]Node{}
	for _, n := range ds.Nodes {
		dsNodes[n.ID] = n
	}
	for _, l := range ds.Links {
		category := l.Source
		root, ok := roots[category]
		if !ok || root.Role != RoleObjectCategory {
			return fmt.Errorf("expansion: pp.85-125 spend from %q in %s, which is no object category the spine draws",
				category, col)
		}
		division := dsNodes[l.Target]
		member := Node{
			ID:    division.ID + "/" + strings.TrimPrefix(category, PrefixExpenditure),
			Label: division.Label + ": " + root.Label,
			Tier:  tierDepartment, Role: RoleDepartment,
		}
		if err := st.add(member, category, alternativeDivisions); err != nil {
			return err
		}
		l.Target = member.ID
		l.Alternative = alternativeDivisions
		st.links = append(st.links, l)
	}
	for _, ex := range structure.ExceptionsOn(structure.BudgetBookExceptions(), structure.CutDepartmentwide,
		structure.CutSpine, structure.LevelCategory) {
		for _, p := range ex.Cells {
			if p.Year != col.FiscalYear || p.Basis != string(col.Basis) {
				continue
			}
			category := PrefixExpenditure + p.Coords[structure.AxisCategory]
			var ids []string
			var printed, divided int64
			for _, l := range spine.Links {
				if l.Target == category {
					ids = append(ids, l.FactIDs...)
					printed += l.ValueCents
				}
			}
			for _, l := range ds.Links {
				if l.Source == category {
					ids = append(ids, l.FactIDs...)
					divided += l.ValueCents
				}
			}
			diff := printed - divided
			if want := p.Against.Cents - p.Cut.Cents; diff != want {
				return fmt.Errorf("expansion: %s's divisions fall %d cents short of the spine in %s and exception %q holds %d apart",
					category, diff, col, ex.Name, want)
			}
			root := roots[category]
			member := Node{
				ID:    PrefixNoDivision + p.Coords[structure.AxisCategory],
				Label: root.Label + ", by no division",
				Tier:  tierDepartment, Role: RoleDepartment,
				Derived: true, Rationale: remainderRationale(noDivisionRationale, ex), SourceNote: ex.Printed,
			}
			if err := st.add(member, category, alternativeDivisions); err != nil {
				return err
			}
			l := remainderLink(ids, byID, spendingLinkKind(vocab.KindExpenditure))
			l.Source, l.Target, l.ValueCents = category, member.ID, diff
			l.Partition = true
			l.Alternative = alternativeDivisions
			st.links = append(st.links, l)
		}
	}
	return nil
}

// groupRemainders builds each fund group's remainder node once, its words the
// union of every reason it carries money for.
type groupRemainders struct {
	roots map[string]Node
	st    *stitch
}

// label is a fund group's words.
func (g *groupRemainders) label(group string) string { return g.roots[group].Label }

// node is the remainder of group, created or extended with the reason it
// carries money for: the group's netted change, or each exception it
// carries a difference for.
func (g *groupRemainders) node(group string, ex ...structure.Exception) (string, error) {
	if _, ok := g.roots[group]; !ok {
		return "", fmt.Errorf("expansion: %q is no fund group the spine draws", group)
	}
	id := PrefixGroupOnly + strings.TrimPrefix(group, PrefixFundGroup)
	n, ok := g.st.nodes[id]
	if !ok {
		n = Node{ID: id, Label: g.label(group) + ", as a group", Tier: tierFund, Role: RoleGroupRemainder,
			Parent: group, Derived: true}
	}
	reason, note := groupChangeRationale, groupChangeSourceNote
	for _, e := range ex {
		reason, note = remainderRationale(groupExceptionRationale, e), e.Printed
	}
	n.Rationale = joinOnce(n.Rationale, reason)
	n.SourceNote = joinOnce(n.SourceNote, note)
	g.st.nodes[id] = n
	return id, nil
}

// joinOnce appends s to a space-joined list of sentences unless it is
// already there.
func joinOnce(list, s string) string {
	switch {
	case list == "":
		return s
	case strings.Contains(list, s):
		return list
	}
	return list + " " + s
}

// The words a remainder carries. Each exception-derived one quotes its
// exception's Reason, the remainder's single source.
const (
	groupChangeRationale = "Our node for the fund group's change in working capital, which " +
		"Budget Book pp.66-67 print for the group as one signed figure: pp.186-209 print each " +
		"fund's balances, and within this group some rise and some fall, so the group's figure " +
		"is their net and no fund carries it. Netting fund by fund would draw a ribbon from one " +
		"fund to another in the same column."
	groupChangeSourceNote   = "Budget Book PDF pp.66-67, CHANGE IN WORKING CAPITAL"
	groupExceptionRationale = "Our node for money Budget Book pp.66-67 print in this fund " +
		"group's column and its funds' own pages do not: "
	objectRemainderRationale = "Our row for the part of this fund group's object category " +
		"that pp.66-67 print and its funds' rows on pp.172-183 do not: "
	noDivisionRationale = "Our node for the part of this object category that pp.66-67 print " +
		"and no division on pp.85-125 does: "
)

// remainderRationale is a remainder's words, ending with its exception's
// reason.
func remainderRationale(lead string, ex structure.Exception) string {
	return lead + ex.Reason + " (" + ex.Name + ")."
}

// remainderLink is a derived link citing the facts a remainder is the
// difference of, each once.
func remainderLink(ids []string, byID map[string]fact.Fact, kind LinkKind) Link {
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var locs locatorSet
	for _, id := range ids {
		f := byID[id]
		locs.add(&f)
	}
	return Link{Kind: kind, FactIDs: ids, Locators: locs.sources(), Derived: true}
}

// licensed is the one exception on (cut -> spine) at level pinning coords in
// col, or an error naming why there is not exactly one.
func licensed(cut string, at structure.Level, col Column, coords map[structure.Axis]string) (structure.Pin, structure.Exception, error) {
	var found []struct {
		p  structure.Pin
		ex structure.Exception
	}
	for _, ex := range structure.ExceptionsOn(structure.BudgetBookExceptions(), cut, structure.CutSpine, at) {
		for _, p := range ex.Cells {
			if p.Year != col.FiscalYear || p.Basis != string(col.Basis) || len(p.Coords) != len(coords) {
				continue
			}
			match := true
			for a, v := range coords {
				match = match && p.Coords[a] == v
			}
			if match {
				found = append(found, struct {
					p  structure.Pin
					ex structure.Exception
				}{p, ex})
			}
		}
	}
	if len(found) != 1 {
		return structure.Pin{}, structure.Exception{}, fmt.Errorf("%d exceptions on %s against %s pin %v", len(found), cut, structure.CutSpine, coords)
	}
	return found[0].p, found[0].ex, nil
}

// categoryOf is the one category an object row's facts carry.
func categoryOf(l Link, byID map[string]fact.Fact) (string, error) {
	category := ""
	for _, id := range l.FactIDs {
		c := byID[id].Category
		if category != "" && c != category {
			return "", fmt.Errorf("expansion: %s -> %s cites categories %q and %q", l.Source, l.Target, category, c)
		}
		category = c
	}
	if category == "" {
		return "", fmt.Errorf("expansion: %s -> %s cites no category", l.Source, l.Target)
	}
	return category, nil
}

// factIndexOf indexes facts by id.
func factIndexOf(facts []fact.Fact) map[string]fact.Fact {
	out := make(map[string]fact.Fact, len(facts))
	for _, f := range facts {
		out[f.ID] = f
	}
	return out
}

// drawnElsewhere is a fact the stitch selects and draws through another
// schedule's figure, or decomposes: the spine's cells, which the leaves under
// them sum to; pp.127-140's transfers in, which are p76's; pp.186-209's
// columns, of which it draws reserves and balances only; and p222's receipts
// at the CIP funds, outside the operating budget.
func drawnElsewhere(f *fact.Fact) bool {
	switch f.Scope {
	case PublishedScope, FundSourcesUsesScope, CIPFundingScope:
		return true
	case ScopeRevenueByFund:
		return f.Kind == vocab.KindTransferIn
	}
	return false
}

// splitsOf is every split a node of the stitch has members in, in node order,
// each on its side at a node takesIn says does or does not take money in,
// refusing members under a node whose role declares no such split, or one
// whose split is not extend, and a node with no link to or from a member.
func splitsOf(nodes map[string]Node, links []Link, takesIn map[string]bool) ([]Split, error) {
	type key struct{ node, alternative string }
	members := map[key][]string{}
	for _, n := range nodes {
		if n.Parent != "" {
			members[key{n.Parent, n.Alternative}] = append(members[key{n.Parent, n.Alternative}], n.ID)
		}
	}
	reaches := map[key]bool{}
	for _, l := range links {
		if p, ok := nodes[l.Target]; ok && p.Parent == l.Source {
			reaches[key{l.Source, p.Alternative}] = true
		}
		if p, ok := nodes[l.Source]; ok && p.Parent == l.Target {
			reaches[key{l.Target, p.Alternative}] = true
		}
	}
	var out []Split
	for k := range members {
		parent, ok := nodes[k.node]
		if !ok {
			return nil, fmt.Errorf("expansion: %v are parented to %q, which is no node", members[k], k.node)
		}
		d, ok := splitOf(parent.Role, k.alternative, k.alternative != "")
		if !ok {
			return nil, fmt.Errorf("expansion: %q (role %s) has members %v and no split of its role declares them",
				k.node, parent.Role, members[k])
		}
		if d.form != SplitExtend || !reaches[k] {
			return nil, fmt.Errorf("expansion: %q splits by %s into %s and a link between it and a member is %v",
				k.node, d.form, d.key, reaches[k])
		}
		out = append(out, Split{Node: k.node, Key: d.key, Form: d.form, Alternative: d.alternative,
			Side: sideOf(d, takesIn[k.node])})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return !out[i].Alternative && out[j].Alternative
	})
	return out, nil
}

// checkDistinctAlternativeLinks is checkDistinctLinks within each split's
// links: two alternatives may each draw a pair once.
func checkDistinctAlternativeLinks(links []Link) error {
	by := map[string][]Link{}
	for _, l := range links {
		by[l.Alternative] = append(by[l.Alternative], l)
	}
	for _, ls := range by {
		if err := checkDistinctLinks(ls); err != nil {
			return err
		}
	}
	return nil
}

// expansionCaveats are the things a reader of the stitch has to be told.
func expansionCaveats(twice int, nodes []Node, apart []string) []Caveat {
	out := []Caveat{
		ConstraintTierCaveat(),
		revenueSchedulePublishedTwiceCaveat(),
		{
			ID: "the-expansion-is-the-spine-at-finer-grain",
			Summary: "Every cell of the citywide chart opens into the finer schedules that print it, " +
				"and they sum back to it to the cent.",
			Text: fmt.Sprintf("This document is the citywide chart's cells read from the finer "+
				"schedules that decompose each one: a revenue category from the lines Budget Book "+
				"pp.127-140 print, a fund group from its funds, the General Fund from its divisions "+
				"on pp.167-170, an object category from the rows pp.167-183 print or, instead, from "+
				"the divisions on pp.85-125, transfers from p76 and p222, and reserves and balances "+
				"from pp.186-209. Summed to the citywide chart's cells they equal pp.66-67. A node "+
				"marked derived carries money a page prints for a group or a category and no finer "+
				"page attributes. An opened node draws a link to each of its members: the General "+
				"Fund's to each division totals the rows drawn beyond it, and every other node's "+
				"repeats each link arriving at the member, or leaving it where the node takes "+
				"nothing in, with the same figure and facts. A remainder cites the members it is "+
				"the difference of. So %d fact(s) are behind more than one link and summing every "+
				"link double-counts.", twice),
			AppliesTo: []string{},
		},
	}
	var netted []string
	for _, n := range nodes {
		if strings.HasPrefix(n.ID, PrefixGroupOnly) {
			netted = append(netted, n.ID)
		}
	}
	if len(netted) > 0 {
		summary := fmt.Sprintf("%s money pp.66-67 print that no fund's own page carries, drawn "+
			"on a node of the group's own.", plural(len(netted), "fund group"))
		if len(apart) > 0 {
			summary += " A fund of such a group is drawn with what it takes in and spends, so its " +
				"in and out differ by its own change in balance."
		}
		out = append(out, Caveat{
			ID:      "some-fund-group-money-is-not-by-fund",
			Summary: summary,
			Text: "Budget Book pp.66-67 print each fund group's change in working capital as one " +
				"figure. Where some of the group's funds draw on their balances and others add to " +
				"them, pp.186-209 print each fund's own beginning and ending balance and the " +
				"group's change is the net of the funds' changes, which no single fund carries; it " +
				"is drawn on a node of the group's own. Each fund of such a group is drawn with " +
				"what it takes in and spends and not its own change, so its in and out differ by " +
				"that change: the fund states its two printed balances from pp.186-209 and our " +
				"subtraction of them. Where pp.66-67 print more for a group than its funds' pages " +
				"do, the difference is on the same node, with the exception declaring it named in " +
				"the node's note.",
			AppliesTo: append(netted, apart...),
		})
	}
	return out
}
