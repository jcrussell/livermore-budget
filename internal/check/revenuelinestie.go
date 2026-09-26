package check

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// The id forms this check reads, spelled here because it works on the PUBLISHED
// document rather than on the constants that wrote it. internal/project's own
// prefixes are unexported for that reason, and a second reading of an id form is
// the point of every structural check in this package.
const (
	revenueLinePrefix = "revenue-line/"
	revenueNodePrefix = "revenue/"
	fundNodePrefix    = "fund/"
	fundGroupPrefix   = "fund-group/"
	transfersInNode   = "transfers/in"
)

// The cut names structure.BudgetBookCuts gives pp.127-140 and pp.66-67, which
// is how an exception names its pair. TestScopeConstantsNameDeclaredCuts holds
// them to the declarations.
const (
	revenueDetailCut = "revenue-detail"
	spineCut         = "spine"
)

// revenueDetailRestriction is the kind half of pp.127-140, and the spine side
// of arm 1 is summed inside it: the schedule prints revenue and transfers in
// and nothing else, and it spans every fund group, so there is no fund-group
// half. Eleven fund blocks on pp.131-140 carry a Transfers In row inside one
// printed `Total <fund>`, so leaving that kind out would leave the drill-down's
// transfer flows summed into no cell while the spine still publishes them.
var revenueDetailRestriction = detailRestriction{
	Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn},
}

// revenueLinesTieToTheirCategories asserts the drill-down's line tier decomposes
// the spine's revenue rather than restating or reclassifying it.
//
// TWO ARMS, AND BOTH ARE THE ONLY VERIFY-TIME GUARD OF WHAT THEY CLAIM. Measured
// on the published store before this check existed: re-pointing one line node
// onto a sibling category, and blanking a line node's parent outright, both leave
// `fisc verify` all-green. node-hierarchy-well-formed cannot see either — a
// re-pointed parent still resolves to a coarser node of the same document, and a
// blanked one removes the node from that check's subject set altogether, since
// its subjects are the PARENTED nodes. So neither arm below is a sharpening of
// an existing claim; each is the whole of one.
//
//  1. Per (kind, category, fund group), the links the drill-down draws into its
//     funds sum to the spine's own cell. THE FUND GROUP COMES FROM
//     data/funds.yaml through [Subject.Vocabulary] and not from the fund node's
//     own parent edge, so a document that mis-parented a fund cannot validate
//     itself here.
//  2. Every line node is parented to `revenue/` plus the category
//     data/taxonomy.yaml declares that line under, and that category is a node of
//     the same document. Sharper than "the parent resolves to some tier-0
//     category": a parent resolving to the WRONG category folds silently, which
//     is what arm 1 then prices.
//
// THE BASIS IS STATED IN THE SUMMARY because the two sides of arm 1 count
// different things. A link exists only for a nonzero cell — internal/project's
// drill-down keeps the facts of a printed dash and draws no ribbon — so a spine
// cell that is present and zero has no link and ties by construction.
// compareDetail's absent-vs-zero rule is right fact against fact, where both
// sides print rows, and would otherwise read here as a cell two figures agreed
// on. The count of those is reported rather than folded in.
//
// THE REVENUE SIDE IS CITED AT THREE GRAINS AND NEITHER ARM READS THE OTHER TWO.
// A row is drawn into its funds, rolled back up into its category, and rolled up
// again under the group its fund belongs to, so every revenue row behind a flow
// is cited more than once and FundFlowsCounts.FactsCitedTwice counts it -- which
// fund-flows-counts-reconcile re-derives from the links and this check does not
// touch. Arm 1 keys on the TARGET being a fund, which the (1,0) rollup fails and
// the (2,3) rollup PASSES: a group carries its funds' whole inflow, so it is
// skipped by source below rather than by the shape of the subject set.
//
// WHAT IT CANNOT WITNESS. Both sides are one fact slice in one process: the
// links were built from the facts this check sums, so a wrong AMOUNT moves both
// together and neither arm notices. That is fact-offset-points-at-token's claim
// and not this one. What these arms do witness is a line drawn under the wrong
// category, a line the drill-down stopped drawing, and the drill-down and the
// spine drifting apart.
type revenueLinesTieToTheirCategories struct{}

var _ Check = (*revenueLinesTieToTheirCategories)(nil)

func (*revenueLinesTieToTheirCategories) ID() string {
	return "revenue-lines-tie-to-their-categories"
}
func (*revenueLinesTieToTheirCategories) Tier() int  { return 1 }
func (*revenueLinesTieToTheirCategories) Full() bool { return false }
func (*revenueLinesTieToTheirCategories) Description() string {
	return "the drill-down's revenue lines sum, per category and fund group, to exactly what " +
		"the citywide spine publishes, and each is parented to the category " +
		"data/taxonomy.yaml declares it under"
}

func (c *revenueLinesTieToTheirCategories) Run(_ context.Context, s *Subject) (Result, error) {
	// VACUOUS ON NO DRILL-DOWN, AND THAT IS NOT A HOLE: the drill-down is one of
	// project.PublishedDocuments, so a repository that stopped building it fails
	// published-projection-built rather than reaching here and going quiet. The
	// condition is deliberately the DOCUMENT and not its links -- a drill-down
	// that drew no flow into a fund would leave every spine cell one-sided and
	// red, which is the verdict that case deserves.
	docs := s.fundFlowsDocuments()
	if len(docs) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: "no projection built a drill-down, so there is no revenue line to tie " +
				"to its category",
			Findings: []Finding{},
		}, nil
	}

	var findings []Finding
	detail := map[detailKey]cellSum{}
	lines, drawn := 0, 0

	for _, p := range docs {
		doc := p.FundFlows
		byID := make(map[string]project.Node, len(doc.Nodes))
		for _, n := range doc.Nodes {
			byID[n.ID] = n
		}

		lineFindings, n := c.checkParents(p.String(), doc.Nodes, byID, s.Vocabulary)
		findings = append(findings, lineFindings...)
		lines += n

		// A DRILL-DOWN IS OF ONE COLUMN and internal/project refuses any other
		// shape, so this is a state the check cannot make sense of rather than a
		// claim about the corpus. Reported instead of assumed: summing two
		// columns' links into one cell would tie against neither.
		if len(p.Options.Columns) != 1 {
			findings = append(findings, finding(p.String(),
				"this document is of %d columns and a flow diagram is of one, so its links "+
					"cannot be keyed to a fiscal year at all", len(p.Options.Columns)))
			continue
		}
		col := p.Options.Columns[0]

		for _, l := range doc.Links {
			if !strings.HasPrefix(l.Target, fundNodePrefix) {
				continue
			}
			// THE GROUP'S ROLLUP IS THIS CELL'S OWN MONEY ONE GRAIN COARSER.
			// Run, not predicted: counted here it doubles every one of the 18
			// cells, and the check reports the spine as short by exactly the
			// figure it publishes -- a drift finding on a document that
			// reconciles.
			if strings.HasPrefix(l.Source, fundGroupPrefix) {
				continue
			}
			drawn++
			group, ok := fundGroupOf(s.Vocabulary, l.Target)
			if !ok {
				findings = append(findings, finding(p.String(),
					"link %q -> %q names a fund data/funds.yaml does not list, so the group "+
						"whose spine cell it should sum into cannot be decided. The fund's "+
						"own parent edge says one, and taking it from there would let a "+
						"mis-parented fund reconcile against the wrong group",
					l.Source, l.Target))
				continue
			}
			category, ok := inflowCategory(byID, l.Source)
			if !ok {
				findings = append(findings, finding(p.String(),
					"link %q -> %q reaches a fund from a node that is neither a %s line this "+
						"document carries nor %q. Every flow into a fund is one or the other, "+
						"and a third shape is money this check would sum into no cell at all",
					l.Source, l.Target, revenueLinePrefix, transfersInNode))
				continue
			}
			k := detailKey{col.FiscalYear, col.Basis, group, category}
			cell := detail[k]
			cell.cents += amount.Cents(l.ValueCents)
			cell.present = true
			detail[k] = cell
		}
	}

	spine := detailSums(s.Facts, spineScope, revenueDetailRestriction)
	reconcile, unmatched := reconciledPairs(detail, spine)
	exempt, notes, exceptionFindings := lineExceptions(detail, spine, reconcile)
	findings = append(findings, exceptionFindings...)

	cmp := compareDetail(detail, spine, reconcile, project.FundFlowsProjection, exempt)
	findings = append(findings, cmp.findings...)

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pair(s) the spine publishes, "+
		"each the sum of the drill-down's %d flows into funds equal to the spine's own "+
		"figure to the cent, with every fund's group read from data/funds.yaml rather than "+
		"from the document; %d of those cells are a spine cell printed as zero that no link "+
		"is drawn for, which ties by construction and not by two figures agreeing; and %d "+
		"revenue-line nodes, each parented to the category data/taxonomy.yaml declares it "+
		"under", cmp.subjects, len(reconcile), drawn, silentZeros(detail, spine, reconcile, exempt),
		lines)
	if cmp.exempt > 0 {
		held += fmt.Sprintf("; %d further cell(s) are declared exceptions and are NOT among "+
			"the %d", cmp.exempt, cmp.subjects)
	}
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d further pair(s) the drill-down publishes have no spine "+
			"column and are not reconciled: %s", len(unmatched), describePairs(unmatched))
	}
	for _, n := range notes {
		held += "; " + n
	}

	return conclusion{
		subjects: cmp.subjects + lines,
		unit:     "cells and line nodes",
		held:     held,
		nothing: "no drill-down draws a flow into a fund and the spine publishes no revenue, " +
			"so there is no line to tie to a category",
		findings: findings,
	}.result(), nil
}

// checkParents is arm 2, and it reads the taxonomy rather than the id.
//
// A LINE'S CATEGORY CANNOT BE CUT OUT OF ITS OWN SLUG. A category slug is itself
// one or two segments — `taxes/property` beside `licenses-and-permits` — and the
// taxonomy nests to any depth, so no fixed cut recovers the parent for the whole
// file. data/taxonomy.yaml is asked, which is also what makes this an
// independent reading: the projection carries the category the row was netted
// under, and the registry carries the one the line is declared under.
func (*revenueLinesTieToTheirCategories) checkParents(doc string, nodes []project.Node,
	byID map[string]project.Node, v Vocabulary) ([]Finding, int) {

	var findings []Finding
	lines := 0
	for _, n := range nodes {
		if !strings.HasPrefix(n.ID, revenueLinePrefix) {
			continue
		}
		lines++
		slug := strings.TrimPrefix(n.ID, revenueLinePrefix)
		entry, ok := v.Category(slug)
		if !ok {
			findings = append(findings, finding(doc,
				"node %q names no data/taxonomy.yaml entry, so the category it decomposes "+
					"is whatever its parent edge says and nothing can contradict it", n.ID))
			continue
		}
		want := revenueNodePrefix + entry.Parent
		if n.Parent != want {
			findings = append(findings, finding(doc,
				"node %q is parented to %q and data/taxonomy.yaml declares it under %q, so "+
					"it wants %q. A line whose parent is not its category is folded into "+
					"the wrong column of the chart, or out of the chart altogether, and "+
					"neither shape reaches node-hierarchy-well-formed: a re-pointed parent "+
					"still resolves to a coarser node, and a blank one drops the node from "+
					"that check's subjects",
				n.ID, n.Parent, entry.Parent, want))
			continue
		}
		if _, ok := byID[want]; !ok {
			findings = append(findings, finding(doc,
				"node %q is parented to %q, which data/taxonomy.yaml declares and this "+
					"document does not carry, so the line has no box to fold into", n.ID, want))
		}
	}
	return findings, lines
}

// fundGroupOf is the group data/funds.yaml gives the fund a `fund/<number>` id
// names.
func fundGroupOf(v Vocabulary, id string) (string, bool) {
	number, err := strconv.Atoi(strings.TrimPrefix(id, fundNodePrefix))
	if err != nil {
		return "", false
	}
	entry, ok := v.Fund(number)
	if !ok {
		return "", false
	}
	return entry.Type, true
}

// inflowCategory is the spine category a flow into a fund belongs to.
//
// THE LINE'S CATEGORY IS THE DOCUMENT'S OWN CLAIM, taken from the parent edge
// and not from the taxonomy, which is what makes a re-pointed line show up as
// money in the wrong cell rather than as nothing at all. checkParents is what
// says the claim is true; this arm prices it if it is not.
//
// A TRANSFER HAS NO LINE. pp.127-140 print a fund's Transfers In as a row of the
// fund and the taxonomy declares no line under `transfers/in`, so the drill-down
// draws that flow from the tier-0 endpoint straight into the fund. Leaving those
// links out would leave $21,045,597 of FY2026 inflow summed into no cell while
// the spine still publishes it, and every transfers/in cell would then read as a
// dropped rule.
func inflowCategory(byID map[string]project.Node, source string) (string, bool) {
	if source == transfersInNode {
		return transfersInNode, true
	}
	if !strings.HasPrefix(source, revenueLinePrefix) {
		return "", false
	}
	node, ok := byID[source]
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(node.Parent, revenueNodePrefix), true
}

// lineExceptions holds apart the cells structure.BudgetBookExceptions declares
// pp.127-140 do not print, which is the SAME declaration
// cuts-tie-along-the-lattice reconciles the revenue-detail cut against the
// spine with.
//
// SHARED RATHER THAN RESTATED. There is one such argument in the corpus -- the
// General Fund's Transfers In, which pp.127-130 do not print -- and a second
// copy of it here would be a declaration that could stop agreeing with the
// first without anything going red.
//
// WHAT IS THIS CHECK'S OWN BUSINESS is the arm below: the drill-down draws no
// such link, and if it ever does, the exception has become a false claim about
// the document and is excusing a real figure. Whether the spine's figure is the
// one the exception pins is the containment check's arm and not this one's, so
// a pin the spine does not produce is left to that check to refuse.
func lineExceptions(detail, spine map[detailKey]cellSum,
	reconcile map[yearBasis]bool) (func(detailKey) bool, []string, []Finding) {

	var notes []string
	var findings []Finding
	exempted := map[detailKey]bool{}

	for _, e := range budgetBookExceptions() {
		if e.Cut != revenueDetailCut || e.Against != spineCut {
			continue
		}
		for _, p := range e.Cells {
			// A cell the schedule prints at a figure of its own is drawn by
			// the drill-down and compared like any other; only a cell the
			// schedule has NO row for is held apart here.
			if p.Cut.Present {
				continue
			}
			k := detailKey{p.Year, mapping.Basis(p.Basis),
				p.Coords[structure.AxisFundGroup], p.Coords[structure.AxisCategory]}
			if !reconcile[yearBasis{k.year, k.basis}] {
				continue
			}
			sp, ok := spine[k]
			if !ok {
				continue
			}
			if detail[k].present {
				findings = append(findings, finding(e.Name,
					"the drill-down draws a flow into %s, so the exception it is exempted by "+
						"has stopped describing the document and is now excusing a figure the "+
						"chart publishes", k))
				continue
			}
			exempted[k] = true
			notes = append(notes, fmt.Sprintf("%s is drawn by no link and is held apart: %s. "+
				"It is the same declaration cuts-tie-along-the-lattice holds the "+
				"revenue-detail cut apart from the spine with (%s); %s is published by the "+
				"spine and reaches this chart through nothing", k, e.Reason, e.Bead, sp.cents))
		}
	}
	if len(exempted) == 0 {
		return nil, notes, findings
	}
	return func(k detailKey) bool { return exempted[k] }, notes, findings
}

// silentZeros is how many reconciled cells the spine prints as zero and the
// drill-down draws no link for. It is the count the summary's basis clause
// quotes, and it is computed rather than taken from detailComparison.oneSided:
// that field counts a key either side is missing, and on this comparison one of
// the two sides never emits a zero at all.
func silentZeros(detail, spine map[detailKey]cellSum, reconcile map[yearBasis]bool,
	exempt func(detailKey) bool) int {

	n := 0
	for k, sp := range spine {
		if !reconcile[yearBasis{k.year, k.basis}] || (exempt != nil && exempt(k)) {
			continue
		}
		if sp.present && sp.cents == 0 && !detail[k].present {
			n++
		}
	}
	return n
}
