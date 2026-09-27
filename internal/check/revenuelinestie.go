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

// The id forms this check reads, spelled here rather than imported: it reads
// the published document, not the constants that wrote it.
const (
	revenueLinePrefix = "revenue-line/"
	revenueNodePrefix = "revenue/"
	fundNodePrefix    = "fund/"
	fundGroupPrefix   = "fund-group/"
	transfersInNode   = "transfers/in"
)

// The cut names an exception pairs; TestScopeConstantsNameDeclaredCuts holds
// them to structure.BudgetBookCuts.
const (
	revenueDetailCut = "revenue-detail"
	spineCut         = "spine"
)

// revenueDetailRestriction is what pp.127-140 print: revenue and transfers in,
// across every fund group. Transfers In sits inside a printed `Total <fund>`
// on pp.131-140, so dropping that kind leaves the drill-down's transfer flows
// summed into no cell.
var revenueDetailRestriction = detailRestriction{
	Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn},
}

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
			if !strings.HasPrefix(l.Target, fundNodePrefix) {
				continue
			}
			// A group's rollup into its funds is the same money one grain
			// coarser; counting it doubles every cell.
			if strings.HasPrefix(l.Source, fundGroupPrefix) {
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

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pair(s), summed from %d flows "+
		"into funds, %d of them a printed zero no link is drawn for; %d revenue-line nodes "+
		"under their data/taxonomy.yaml category", cmp.subjects, len(reconcile), drawn,
		silentZeros(detail, spine, reconcile, exempt), lines)
	if cmp.exempt > 0 {
		held += fmt.Sprintf("; %d cell(s) held apart, not among the %d: %s",
			cmp.exempt, cmp.subjects, strings.Join(notes, ", "))
	}
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d pair(s) with no spine column: %s", len(unmatched), describePairs(unmatched))
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

// inflowCategory is the spine category a flow into a fund belongs to, read off
// the line's parent edge -- the document's claim, so a re-pointed line lands in
// the wrong cell rather than nowhere. A transfer in has no line and is drawn
// from the tier-0 endpoint straight into the fund.
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

// lineExceptions holds apart the revenue-detail-against-spine cells a
// structure.Exception declares pp.127-140 do not print -- the declaration
// cuts-tie-along-the-lattice uses, shared rather than restated. This check's
// own arm refuses one the drill-down draws a link for; whether the spine's
// figure matches the pin is the containment check's.
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
			// Only a cell the schedule has no row for is held apart here.
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
						"has stopped describing the document", k))
				continue
			}
			exempted[k] = true
			notes = append(notes, fmt.Sprintf("%s (%s, %s)", e.Name, k, sp.cents))
		}
	}
	if len(exempted) == 0 {
		return nil, notes, findings
	}
	return func(k detailKey) bool { return exempted[k] }, notes, findings
}

// silentZeros is how many reconciled cells the spine prints as zero and the
// drill-down draws no link for. detailComparison.oneSided is not that count: it
// counts a key either side is missing.
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
