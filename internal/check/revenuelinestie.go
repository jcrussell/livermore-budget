package check

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// spineGrain is the level arm 1 compares at: the spine's own, so a cell is a
// structure.Key both this check and cuts-tie-along-the-lattice spell the same
// way.
const spineGrain = structure.LevelFundGroupByCategory

// revenueLinesTieToTheirCategories asserts the drill-down's line tier decomposes
// the spine's revenue rather than restating or reclassifying it:
//
//  1. Per (kind, category, fund group), the links into funds sum to the spine's
//     cell, the fund group read from data/funds.yaml and never from the fund
//     node's own parent edge.
//  2. Every line node is parented to the category data/taxonomy.yaml declares
//     it under, and that category is a node of the same document.
//
// Both are the only verify-time guard of what they claim: a line re-pointed
// onto a sibling category, or with its parent blanked, passes
// node-hierarchy-well-formed. A spine cell printed as zero has no link and ties
// by construction, so the summary counts those apart. A wrong amount moves both
// sides together; that is fact-offset-points-at-token's to catch.
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
	// Vacuous on no drill-down document, which published-projection-built
	// refuses; a drill-down with no links is red here, not vacuous.
	docs := s.documentsNamed(project.FundFlowsProjection)
	if len(docs) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: "no projection built a drill-down, so there is no revenue line to tie " +
				"to its category",
			Findings: []Finding{},
		}, nil
	}

	var findings []Finding
	detail := map[structure.Key]structure.Sum{}
	lines, drawn := 0, 0

	for _, p := range docs {
		doc := p.Graph
		byID := make(map[string]project.Node, len(doc.Nodes))
		for _, n := range doc.Nodes {
			byID[n.ID] = n
		}

		lineFindings, n := c.checkParents(p.String(), doc.Nodes, byID, s.Vocabulary)
		findings = append(findings, lineFindings...)
		lines += n

		// internal/project builds no other shape; summing two columns' links
		// into one cell would tie against neither.
		if len(p.Options.Columns) != 1 {
			findings = append(findings, finding(p.String(),
				"this document is of %d columns and a flow diagram is of one, so its links "+
					"cannot be keyed to a fiscal year at all", len(p.Options.Columns)))
			continue
		}
		col := p.Options.Columns[0]

		for _, l := range doc.Links {
			if !strings.HasPrefix(l.Target, project.PrefixFund) {
				continue
			}
			// A group's rollup into its funds is the same money one grain
			// coarser; counting it doubles every cell.
			if strings.HasPrefix(l.Source, project.PrefixFundGroup) {
				continue
			}
			drawn++
			group, ok := fundGroupOf(s.Vocabulary, l.Target)
			if !ok {
				findings = append(findings, finding(p.String(),
					"link %q -> %q names a fund data/funds.yaml does not list, so the group "+
						"whose spine cell it sums into cannot be decided",
					l.Source, l.Target))
				continue
			}
			category, ok := inflowCategory(byID, l.Source)
			if !ok {
				findings = append(findings, finding(p.String(),
					"link %q -> %q reaches a fund from a node that is neither a %s line this "+
						"document carries nor %q, so it sums into no cell",
					l.Source, l.Target, project.PrefixRevenueLine, project.NodeTransfersIn))
				continue
			}
			k := structure.Key{Year: col.FiscalYear, Basis: string(col.Basis), Level: spineGrain,
				Coords: structure.Coords(spineGrain, map[structure.Axis]string{
					structure.AxisFundGroup: group, structure.AxisCategory: category})}
			cell := detail[k]
			cell.Cents += l.ValueCents
			cell.Present = true
			detail[k] = cell
		}
	}

	spine, err := spineCells(s.Facts)
	if err != nil {
		return Result{}, err
	}
	reconcile, unmatched := reconciledColumns(detail, spine)
	exempt, notes, exceptionFindings := lineExceptions(detail, spine, reconcile)
	findings = append(findings, exceptionFindings...)

	cmp := compareCells(detail, spine, reconcile, project.FundFlowsProjection, exempt)
	findings = append(findings, cmp.findings...)

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pair(s), summed from %d flows "+
		"into funds, %d of them a printed zero no link is drawn for; %d revenue-line nodes "+
		"under their data/taxonomy.yaml category", cmp.subjects, len(reconcile), drawn,
		silentZeros(detail, spine, reconcile, exempt), lines)
	if cmp.exempt > 0 {
		held += fmt.Sprintf("; %d cell(s) held apart, not among the %d: %s",
			cmp.exempt, cmp.subjects, strings.Join(notes, ", "))
	}
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d pair(s) with no spine column: %s", len(unmatched), strings.Join(sortedStrings(unmatched), ", "))
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

// checkParents is arm 2. It asks data/taxonomy.yaml rather than cutting the
// slug: category slugs are one or two segments and the taxonomy nests to any
// depth.
func (*revenueLinesTieToTheirCategories) checkParents(doc string, nodes []project.Node,
	byID map[string]project.Node, v Vocabulary) ([]Finding, int) {

	var findings []Finding
	lines := 0
	for _, n := range nodes {
		if !strings.HasPrefix(n.ID, project.PrefixRevenueLine) {
			continue
		}
		lines++
		slug := strings.TrimPrefix(n.ID, project.PrefixRevenueLine)
		entry, ok := v.Category(slug)
		if !ok {
			findings = append(findings, finding(doc,
				"node %q names no data/taxonomy.yaml entry, so the category it decomposes "+
					"is whatever its parent edge says and nothing can contradict it", n.ID))
			continue
		}
		want := project.PrefixRevenue + entry.Parent
		if n.Parent != want {
			findings = append(findings, finding(doc,
				"node %q is parented to %q and data/taxonomy.yaml declares it under %q, so "+
					"it wants %q",
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
	number, err := strconv.Atoi(strings.TrimPrefix(id, project.PrefixFund))
	if err != nil {
		return "", false
	}
	entry, ok := v.Fund(number)
	if !ok {
		return "", false
	}
	return entry.Type, true
}

// inflowCategory is the spine category a flow into a fund belongs to, read off
// the line's parent edge -- the document's claim, so a re-pointed line lands in
// the wrong cell rather than nowhere. A transfer in has no line and is drawn
// from the tier-0 endpoint straight into the fund.
func inflowCategory(byID map[string]project.Node, source string) (string, bool) {
	if source == project.NodeTransfersIn {
		return project.NodeTransfersIn, true
	}
	if !strings.HasPrefix(source, project.PrefixRevenueLine) {
		return "", false
	}
	node, ok := byID[source]
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(node.Parent, project.PrefixRevenue), true
}

// lineExceptions holds apart the revenue-detail-against-spine cells a
// structure.Exception declares pp.127-140 do not print -- the declaration
// cuts-tie-along-the-lattice uses, shared rather than restated. This check's
// own arm refuses one the drill-down draws a link for; whether the spine's
// figure matches the pin is the containment check's.
func lineExceptions(detail, spine map[structure.Key]structure.Sum,
	reconcile map[string]bool) (func(structure.Key) bool, []string, []Finding) {

	var notes []string
	var findings []Finding
	exempted := map[structure.Key]bool{}

	for _, e := range budgetBookExceptions() {
		if e.Cut != structure.CutRevenueDetail || e.Against != structure.CutSpine || e.At != spineGrain {
			continue
		}
		for _, p := range e.Cells {
			// Only a cell the schedule has no row for is held apart here.
			if p.Cut.Present {
				continue
			}
			k := e.Key(p)
			if !reconcile[k.Column()] {
				continue
			}
			sp, ok := spine[k]
			if !ok {
				continue
			}
			if detail[k].Present {
				findings = append(findings, finding(e.Name,
					"the drill-down draws a flow into %s, so the exception it is exempted by "+
						"has stopped describing the document", k))
				continue
			}
			exempted[k] = true
			notes = append(notes, fmt.Sprintf("%s (%s, %s)", e.Name, k, amount.Cents(sp.Cents)))
		}
	}
	if len(exempted) == 0 {
		return nil, notes, findings
	}
	return func(k structure.Key) bool { return exempted[k] }, notes, findings
}

// silentZeros is how many reconciled cells the spine prints as zero and the
// drill-down draws no link for. cellComparison.oneSided is not that count: it
// counts a key either side is missing.
func silentZeros(detail, spine map[structure.Key]structure.Sum, reconcile map[string]bool,
	exempt func(structure.Key) bool) int {

	n := 0
	for k, sp := range spine {
		if !reconcile[k.Column()] || (exempt != nil && exempt(k)) {
			continue
		}
		if sp.Present && sp.Cents == 0 && !detail[k].Present {
			n++
		}
	}
	return n
}

// spineCells sums the spine's facts of the kinds pp.127-140 print into the
// cells of its own grain, keyed as cuts-tie-along-the-lattice keys them.
func spineCells(facts []fact.Fact) (map[structure.Key]structure.Sum, error) {
	var spine, detail structure.Cut
	for _, c := range structure.AllCuts() {
		switch c.Name {
		case structure.CutSpine:
			spine = c
		case structure.CutRevenueDetail:
			detail = c
		}
	}
	if spine.Name == "" || detail.Name == "" {
		return nil, fmt.Errorf("structure declares no %q or no %q cut", structure.CutSpine, structure.CutRevenueDetail)
	}
	// The spine's cells of the kinds the revenue detail prints.
	spine.Kinds = detail.Kinds
	return structure.CellsOf(facts, spine, spineGrain)
}

// reconciledColumns splits the (fiscal year, basis) columns the two sides
// carry into the ones this check reconciles and the ones it cannot.
//
// THE SPINE DECIDES, and the detail does not get a say: an intersection would
// let the detail opt out of a column by dropping it. A spine column with no
// detail behind it is a gap in the MAPPING, which this check exists for; a
// detail column with no spine column is a gap in the DOCUMENT -- pp.66-67
// print no actual or revised column -- and is returned separately so the
// summary can name it rather than leave a reader counting facts to guess.
func reconciledColumns(detail, spine map[structure.Key]structure.Sum) (reconcile, unmatched map[string]bool) {
	reconcile = map[string]bool{}
	for k := range spine {
		reconcile[k.Column()] = true
	}
	unmatched = map[string]bool{}
	for k := range detail {
		if col := k.Column(); !reconcile[col] {
			unmatched[col] = true
		}
	}
	return reconcile, unmatched
}

// compareCells is the comparison itself: every key either side produced,
// inside the columns the spine publishes, with the three ways a cell can
// disagree told apart because they need different fixes.
//
// THE UNION, NOT THE DETAIL'S KEYS. Iterating only the keys the DETAIL produces
// would make a dropped rule invisible: delete a whole block and its category
// vanishes from the detail side entirely, so a detail-keyed loop compares
// nothing and reports green. A spine key with no detail counterpart is a
// FAILURE.
func compareCells(detail, spine map[structure.Key]structure.Sum, reconcile map[string]bool,
	scope string, exempt func(structure.Key) bool) cellComparison {
	var out cellComparison
	tallied := structure.Tally(detail, spine, func(k structure.Key) bool {
		if !reconcile[k.Column()] {
			return false
		}
		if exempt != nil && exempt(k) {
			out.exempt++
			return false
		}
		return true
	})
	out.subjects, out.oneSided = tallied.Subjects, tallied.OneSided
	for _, cell := range tallied.Cells {
		if cell.Ties() {
			continue
		}
		d, sp := cell.Cut, cell.Against
		switch {
		case !d.Present:
			out.findings = append(out.findings, finding(cell.Key.String(),
				"the spine publishes %s here and the detail has no such row at all; a "+
					"category the schedule stopped printing is a rule that was dropped, "+
					"not a cell that is empty", amount.Cents(sp.Cents)))
		case !sp.Present:
			out.findings = append(out.findings, finding(cell.Key.String(),
				"the detail publishes %s here and the spine has no such cell; %q must "+
					"decompose the spine, never extend it", amount.Cents(d.Cents), scope))
		default:
			out.findings = append(out.findings, finding(cell.Key.String(),
				"the detail sums to %s and the spine publishes %s, a difference of %s; "+
					"these are the same money decomposed two ways and must tie to the cent",
				amount.Cents(d.Cents), amount.Cents(sp.Cents), amount.Cents(d.Cents-sp.Cents)))
		}
	}
	return out
}

// cellComparison is what one pass over the union produced. oneSided is
// carried because a key only one side produced ties when the other is zero,
// a real agreement and not a vacancy; exempt is separate again, since an
// exempted cell was neither compared nor agreed at zero but handed to another
// check.
type cellComparison struct {
	subjects int
	oneSided int
	exempt   int
	findings []Finding
}
