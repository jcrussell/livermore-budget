package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// The spine's vocabulary for its right-hand column, spelled here rather than
// imported, which is this package's habit wherever the point is an independent
// second reading: internal/project composes both from its own constants, and a
// check taking them from the producer would agree with it by construction.
//
// THE ROLE IS THE ONE THAT MATTERS. Three of the spine's tier-5 nodes are flow
// endpoints rather than object categories, so the prefix alone would not tell
// them apart -- `transfers/out` is at tier 5 and carries no `expenditure/`.
const (
	spendingObjectPrefix = "expenditure/"
	roleObjectCategory   = "object_category"
)

// SpendingGaps is the declared gap between the spine's object-category cells and
// what pp.85-125's division rows come to, as the site's drill seam holds it: the
// tier-5 node id, and the reason the two documents do not close.
//
// DERIVED FROM departmentwideExceptions AND NOT WRITTEN AGAIN. That table is the
// declaration, and it is the one the check below consults; this is the same set
// spelled for a consumer whose vocabulary is node ids rather than category keys.
// Two spellings of a set that must agree drift; one declaration with two readers
// cannot.
//
// THE COLUMN IS IN THE SENTENCE AND NOT IN THE KEY, because the seam it feeds
// has no year axis: a drill step's residual is a map from node id to reason, and
// the same step is declared for every year the chart can open. An entry
// therefore names its own column in words, which is what a reader of the mark
// needs anyway -- the figure is right in one budget year and zero in the other,
// and a sentence that did not say which would be wrong half the time.
func SpendingGaps() map[string]string {
	out := make(map[string]string, len(departmentwideExceptions))
	for _, e := range departmentwideExceptions {
		id := spendingCategoryNode(e.category)
		reason := fmt.Sprintf("FY%d %s: the citywide spine publishes %s in this object "+
			"category and Budget Book pp.85-125's division rows come to %s, a difference of "+
			"%s. %s (%s)", e.year, e.basis, e.spineCents, e.detailCents, e.discrepancy(),
			e.reason, e.bead)
		if prev, dup := out[id]; dup {
			// TWO COLUMNS' GAPS ON ONE NODE ARE BOTH NAMED, in year order, because
			// the key cannot tell them apart and a map that kept the last one
			// would publish whichever the slice happened to end with. There is one
			// entry today; the day there are two, a reader sees both rather than a
			// figure that is right for a year they are not looking at.
			out[id] = prev + " " + reason
			continue
		}
		out[id] = reason
	}
	return out
}

// spendingCategoryNode is the tier-5 node id an object category is drawn at.
func spendingCategoryNode(category string) string { return spendingObjectPrefix + category }

// spendingWindowReconciles asserts the two documents the object-category window
// splices still describe the same money: for every fiscal column both the spine
// and the departmentwide cross-tab publish, each object category's spine inflow
// equals the divisions' outflow plus the gap declared for that cell, to the cent.
//
//	spine_in(C) == spending_out(C) + declared_gap(C)
//
// where spine_in is every spine link into the object-category node and
// spending_out every cross-tab link out of the node of the same id.
//
// IT IS A DOCUMENT-LEVEL TWIN OF departmentwide-ties-to-spine AND NOT A SECOND
// COPY OF IT. That check reads the FACT STORE through detailSums and never
// builds a graph, which is why the coverage it gives did not move when this lane
// retired the unprojectedScopes entry. What no check saw until this one is the
// two GRAPHS disagreeing: a window splices a spine document and a cross-tab
// document on the node the reader clicked, and a projection that dropped a cell,
// mis-netted one or drew a category the other does not have would leave the
// centre of that window with two different totals on its two sides. Both are
// worth having, and neither subsumes the other -- see WHAT IT CANNOT WITNESS.
//
// THE GAP IS READ FROM departmentwideExceptions AND NEVER COMPUTED FROM THE TWO
// SIDES, which is fundingSourcesException's rule (internal/check/fundingsources.go)
// applied at graph level. A gap taken as "whatever the difference is" absorbs the
// next dropped division silently and reports a pass; a gap that is a declared
// figure fails the moment either side moves by anything other than exactly it.
//
// THE SUBJECT SET IS THE SPINE'S OBJECT CATEGORIES, BY ROLE. Three of the spine's
// seven tier-5 nodes -- transfers/out, fund-balance/contribution and
// fund-balance/reserve-increase -- are flow endpoints rather than object
// categories, and pp.85-125 decompose none of them: the Transfers Out row those
// pages do print is zero in both budget years, and no fund-balance row is printed
// there at all. Those nodes carry a role of their own on the spine, so the
// restriction is the role rather than a list of ids to keep in step. The
// cross-tab side is then checked in the other direction too: a category IT draws
// that the spine has no object-category node for is a finding, because this
// document must decompose the spine and never extend it.
//
// WHAT IT CANNOT WITNESS. Both documents are built from one fact slice in one
// process, so it cannot witness a wrong figure -- a perturbed fact moves the
// spine cell and the division cell together, and fact-offset-points-at-token is
// the check that sees that. It also cannot see WHICH division spent the money:
// the identity sums them away exactly as departmentwide-ties-to-spine does, so a
// dollar moved from Patrol to Horizons inside one object category leaves every
// subject here unchanged. What it catches is the two documents drifting apart --
// a selector dropped from one schedule, a cell netted at a different grain, a
// zero-value rule that stopped dropping a link -- and a declared gap that has
// stopped being the difference it declares.
type spendingWindowReconciles struct{}

var _ Check = (*spendingWindowReconciles)(nil)

func (*spendingWindowReconciles) ID() string { return "spending-window-reconciles" }
func (*spendingWindowReconciles) Tier() int  { return 1 }
func (*spendingWindowReconciles) Full() bool { return false }
func (*spendingWindowReconciles) Description() string {
	return "for every fiscal column both the spine and the departmentwide cross-tab publish, " +
		"each object category's spine inflow equals the divisions' outflow plus the declared " +
		"gap, to the cent"
}

func (*spendingWindowReconciles) Run(_ context.Context, s *Subject) (Result, error) {
	return runSpendingWindow(s, departmentwideExceptions)
}

// runSpendingWindow is Run with the gap table as a parameter, so a test can
// damage the declaration without touching the package's.
func runSpendingWindow(s *Subject, gaps []departmentwideException) (Result, error) {
	spines := map[project.Column]projection{}
	for _, p := range s.graphs() {
		if p.Name != project.PublishedProjection {
			continue
		}
		if err := spendingOneColumn(spines, p); err != nil {
			return Result{}, err
		}
	}
	windows := map[project.Column]projection{}
	for _, p := range s.departmentSpendingDocuments() {
		if err := spendingOneColumn(windows, p); err != nil {
			return Result{}, err
		}
	}

	var columns []project.Column
	for c := range spines {
		if _, ok := windows[c]; ok {
			columns = append(columns, c)
		}
	}
	sort.Slice(columns, func(i, j int) bool {
		if columns[i].FiscalYear != columns[j].FiscalYear {
			return columns[i].FiscalYear < columns[j].FiscalYear
		}
		return columns[i].Basis < columns[j].Basis
	})

	// The declared gaps, keyed on the cell they are of. Built once rather than
	// scanned per subject so an entry that matches no reconciled cell can be
	// reported at the end: an exception nothing applies reconciles nothing while
	// the summary advertises it as live.
	declared := map[categoryKey]departmentwideException{}
	for _, e := range gaps {
		declared[e.key()] = e
	}
	applied := map[categoryKey]bool{}

	var findings []Finding
	var summaries []string
	subjects := 0
	for _, c := range columns {
		spineIn := spendingInflow(spines[c].Graph.Nodes, spines[c].Graph.Links)
		out := spendingOutflow(windows[c].DepartmentSpending.Links)

		// THE UNION, NOT THE SPINE'S KEYS. A node only the cross-tab draws is a
		// document extending the spine rather than decomposing it, and a
		// spine-keyed loop could not see it.
		for _, id := range sortedStrings(unionOf(spineIn, out)) {
			key := categoryKey{c.FiscalYear, c.Basis, strings.TrimPrefix(id, spendingObjectPrefix)}
			sp, ok := spineIn[id]
			if !ok {
				findings = append(findings, finding(fmt.Sprintf("%s, %s", c, id),
					"the divisions of Budget Book pp.85-125 spend %s under this node and the "+
						"spine draws no object category of that id. The cross-tab must "+
						"decompose the spine's right-hand column, never extend it",
					amount.Cents(out[id])))
				continue
			}
			subjects++
			gap := int64(0)
			if e, isDeclared := declared[key]; isDeclared {
				applied[key] = true
				gap = int64(e.discrepancy())
			}
			if sp == out[id]+gap {
				continue
			}
			findings = append(findings, finding(fmt.Sprintf("%s, %s", c, id),
				"the spine sends %s into this category, pp.85-125's divisions take %s out of "+
					"it and the declared gap is %s, leaving %s unaccounted. These are the two "+
					"sides of the node a reader clicks to open the spending window, and a "+
					"reader who adds the divisions up must land on the figure the centre "+
					"shows", amount.Cents(sp), amount.Cents(out[id]), amount.Cents(gap),
				amount.Cents(sp-out[id]-gap)))
		}

		summaries = append(summaries, fmt.Sprintf("%s: %d object %s reconcile to the cent "+
			"between the spine and pp.85-125's divisions", c, len(spineIn),
			plural(len(spineIn), "category", "categories")))
	}

	// EVERY DECLARED GAP MUST HAVE BEEN APPLIED, departmentwide-ties-to-spine's
	// rule at graph level: an entry naming a cell no reconciled column produces
	// is inert while the summary reports the reconciliation as complete.
	for _, e := range gaps {
		if applied[e.key()] {
			continue
		}
		if !spendingColumnReconciled(columns, e) {
			continue
		}
		findings = append(findings, finding(e.key().String(),
			"a declared gap names this cell and the two documents of that column draw no "+
				"such object category, so it is held apart from nothing while the summary "+
				"reports the column reconciled. Fix its key, or delete the entry (%s)", e.bead))
	}

	held := joinSemicolon(summaries)
	for _, e := range gaps {
		if !applied[e.key()] {
			continue
		}
		held += fmt.Sprintf("; %s is held apart by the declared %s, which is what the window's "+
			"centre must show as carried rather than absorb (%s)",
			e.key(), e.discrepancy(), e.bead)
	}

	return conclusion{
		subjects: subjects,
		unit:     "object categories",
		held:     held,
		nothing: "no fiscal column is published by both the spine and the departmentwide " +
			"cross-tab, so there is no object category whose two grains could be compared",
		findings: findings,
	}.result(), nil
}

// spendingInflow is every spine link into a node the spine calls an object
// category, summed per node.
//
// THE ROLE IS THE RESTRICTION AND THE TIER IS NOT. transfers/out and the two
// fund-balance endpoints sit at tier 5 beside the object categories and are not
// object categories: they are ends of a flow, and pp.85-125 decompose none of
// them. Selecting on tier would put three undecomposable nodes into a set whose
// identity is that they ARE decomposed.
func spendingInflow(nodes []project.Node, links []project.Link) map[string]int64 {
	object := map[string]bool{}
	for _, n := range nodes {
		if n.Role == roleObjectCategory {
			object[n.ID] = true
		}
	}
	out := map[string]int64{}
	for _, l := range links {
		if object[l.Target] {
			out[l.Target] += l.ValueCents
		}
	}
	// A node the spine draws with no inbound link at all is still a subject:
	// absent is not zero, and a category that lost every link is exactly the
	// drift this compares.
	for id := range object {
		if _, ok := out[id]; !ok {
			out[id] = 0
		}
	}
	return out
}

// spendingOutflow is every cross-tab link out of a tier-5 node, summed per node.
//
// KEYED ON THE SOURCE END because that is the end the window's centre is: the
// document draws expenditure/<object> -> dept/<division>, so a category's whole
// decomposition is the links it sources.
func spendingOutflow(links []project.Link) map[string]int64 {
	out := map[string]int64{}
	for _, l := range links {
		out[l.Source] += l.ValueCents
	}
	return out
}

// unionOf is every key either side produced.
func unionOf(a, b map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

// spendingColumnReconciled says whether a declared gap's column is one both
// documents publish. A gap naming a column the spine does not print is not a
// stale entry: the loop above never reaches its cell.
func spendingColumnReconciled(columns []project.Column, e departmentwideException) bool {
	for _, c := range columns {
		if c.FiscalYear == e.year && c.Basis == e.basis {
			return true
		}
	}
	return false
}

// spendingOneColumn indexes a projection by its single column, refusing a
// projection of any other width or a second projection of the same column: both
// are states this check cannot make sense of rather than claims about the corpus.
func spendingOneColumn(index map[project.Column]projection, p projection) error {
	if len(p.Options.Columns) != 1 {
		return fmt.Errorf("spending-window-reconciles: %s is of %d columns, and a graph is "+
			"of one", p, len(p.Options.Columns))
	}
	c := p.Options.Columns[0]
	if prev, dup := index[c]; dup {
		return fmt.Errorf("spending-window-reconciles: %s and %s are both of %s", prev, p, c)
	}
	index[c] = p
	return nil
}
