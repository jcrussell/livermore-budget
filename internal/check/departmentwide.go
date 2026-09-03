package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// departmentwideScope is the scope Budget Book pp.85-125's Expenditures by
// Category blocks are mapped at, spelled here for the reason detail.go gives:
// the hazard this check exists for is a scope string two readers spell
// differently.
const departmentwideScope = "departmentwide-expenditures"

// The restriction is the KIND half only, and both the presence of that half and
// the absence of the other are claims about the pages.
//
// The kind half excludes dw-maintenance's Transfers Out row, which is the one
// row on these eleven pages that is not an expenditure. Without it the union
// would meet the spine's transfers/out cells, which decompose the whole city's
// transfers and not one division's.
//
// There is no fund-group half because these facts carry NO FUND GROUP AT ALL --
// see the doc comment below. Pinning one would match nothing.
var departmentwideRestriction = detailRestriction{
	Kinds: []mapping.Kind{mapping.KindExpenditure},
}

// categoryKey is one cell this check compares: an object category inside a
// column.
//
// IT IS detailKey WITHOUT THE FUND GROUP, and it is the mirror image of
// fundingsources.go's groupKey, which is detailKey without the category. The two
// schedules are the two blocks of the same eleven pages and they decompose a
// department's money along opposite axes: the lower block by PAYING FUND with
// one category, the upper block by OBJECT CATEGORY with no fund at all. Each
// check drops the axis its schedule does not print.
//
// Keyed on the full detailKey these two sides would share no key: every
// departmentwide fact carries fund_group "" and every spine fact carries one of
// six groups, so all sixteen cells would be one-sided and the check would fail
// on every one while the arithmetic ties exactly.
type categoryKey struct {
	year     int
	basis    mapping.Basis
	category string
}

func (k categoryKey) String() string {
	return fmt.Sprintf("FY%d %s %s", k.year, k.basis, k.category)
}

func (k categoryKey) yb() yearBasis { return yearBasis{k.year, k.basis} }

// byCategory folds detailSums' cells onto the object category, summing away the
// fund group.
//
// It reuses detailSums rather than re-walking the fact store so both sides are
// built by the code every other detail check uses, and so reconciledPairs --
// which reads only the year and basis -- can be fed the unfolded maps unchanged.
func byCategory(m map[detailKey]cellSum) map[categoryKey]cellSum {
	out := map[categoryKey]cellSum{}
	for k, c := range m {
		g := categoryKey{k.year, k.basis, k.category}
		acc := out[g]
		acc.cents += c.cents
		acc.present = acc.present || c.present
		out[g] = acc
	}
	return out
}

// departmentwideException is a cell reconciled against a PRINTED figure instead
// of against the spine, because the spine's own page is wrong. It is
// fundingSourcesException on the other axis, and it carries the same rules:
// the cell is still examined and can still fail three ways, and BOTH figures are
// read off a page rather than either being derived from the other.
type departmentwideException struct {
	category string
	year     int
	basis    mapping.Basis

	spineCents   amount.Cents
	printedCents amount.Cents

	bead   string
	reason string
}

func (e departmentwideException) key() categoryKey {
	return categoryKey{e.year, e.basis, e.category}
}

// departmentwideExceptions is the whole list, and it is one entry long: seven of
// the eight reconciled cells tie to the cent with no exception, no tolerance and
// no constant.
//
// IT IS THE SAME $250,000 fundingSourcesExceptions DECLARES AND IT ARRIVES ON A
// DIFFERENT AXIS, which is why the entry is worth having rather than being a
// second copy of one fact. That check sums pp.85-125's LOWER block by paying
// fund and lands the discrepancy on the internal-service GROUP; this one sums
// the UPPER block of the same pages by object and lands it on SERVICES AND
// SUPPLIES. Neither could locate it alone. Together they say the missing cell is
// internal-service x services-and-supplies x FY2026-27, which is exactly where
// fisc-av0w says it is from a third direction -- the line-item schedule
// pp.172-183.
var departmentwideExceptions = []departmentwideException{{
	category: "services-and-supplies", year: 2027, basis: mapping.BasisAdopted,

	spineCents:   amount.Cents(13050208700),
	printedCents: amount.Cents(13025208700),

	bead: "fisc-av0w",
	reason: "p0067's Internal Service Funds column prints Services & Supplies 16,796,010 " +
		"for FY2026-27, and the five internal service funds' own printed rows on " +
		"pp.172-183 sum to 250,000 less. The spine carries p0067's figure, so the " +
		"citywide services-and-supplies cell it publishes is 250,000 above what the " +
		"eleven departmentwide pages print between them. These pages are an INDEPENDENT " +
		"witness rather than a sixth copy of the same schedule: they decompose the money " +
		"by department, division and object with no fund dimension at all, and they still " +
		"put the difference in this category and this year and in no other of the eight " +
		"cells. The other seven tie to the cent",
}}

// departmentwideTiesToSpine asserts Budget Book pp.85-125's Expenditures by
// Category blocks reconcile against the citywide spine rather than adding to it.
//
// THE FAILURE THIS EXISTS FOR IS THE SILENT ONE THE OTHER FOUR GUARD. The 73
// object rows on these eleven pages sum, per object category, to exactly what
// pp.66-67 publish citywide -- 254,095,412 in FY2026, which IS
// all_funds_gross_expenditure_cents. A rule written at `all-funds-gross` would
// put them in the spine's own cells and double the city's expenditure with every
// graph check green, because those checks tie the graph to the facts it was
// built from and not to the document.
//
// THESE FACTS CARRY NO FUND AND NO FUND GROUP, and that is the page rather than
// an omission. The upper block of each page prints what a department spends
// whatever pays for it; the fund breakdown is the lower block, which
// funding-sources-tie-to-spine reconciles. So this check can tie to the spine's
// object categories summed over all six groups and cannot tie to any one group's
// cell -- which is the reason it exists as a separate check rather than as a
// clause of that one.
//
// THE SECOND GUARD DOES REACH HERE, unlike on the lower block. netCells refuses
// a fact carrying a department, and every one of these 288 facts carries a
// division slug, so a rule mis-scoped to all-funds-gross fails loudly rather
// than doubling silently. That is the opposite of the funding-sources case and
// it is worth knowing which of the two schedules on these pages has which guard.
//
// TIER 1, ZERO TOLERANCE. Seven of the eight reconciled cells tie to the cent.
// The eighth is p0067's error, handled by naming both printed figures rather
// than by widening a bound -- see departmentwideExceptions. A tolerance sized to
// absorb 250,000 would also absorb a whole division.
//
// WHAT IT DOES NOT COVER, said here because the summary says it on every run:
//
//   - pp.66-67 print no actual or revised column, so half these facts -- the
//     FY2023-24 and FY2024-25 columns -- tie to nothing here. What holds them is
//     each division's own printed Division Total at build time, and above that
//     each page's Total Department Expenditures, which equals the sum of its
//     Division Totals in all 44 (page, column) cells. Five of the 29 divisions
//     miss their own Division Total by exactly one dollar, every one in the
//     FY2023-24 Actual column; those are declared as stated_total_deltas.
//
//   - dw-maintenance's Transfers Out row, 266,798 in FY2023-24 Actual and a
//     printed dash in all three later columns. The restriction excludes it by
//     kind, and it is why the eleven pages' printed totals exceed p0183's Grand
//     Total by 266,799 in that column and tie exactly in the other three --
//     p0183 is an EXPENDITURE total and correctly leaves a transfer out of it.
//     The remaining 1 is p0183's own rounding.
//
//   - Which DIVISION spent the money. This check sums them all away: a dollar
//     moved from Patrol to Horizons inside the same object category and year
//     leaves every cell here unchanged. fact-departments-resolve holds the
//     division slug to data/departments.yaml and each division's printed
//     Division Total holds its rows at build time, but no arithmetic here
//     distinguishes two divisions on one page.
type departmentwideTiesToSpine struct{}

var _ Check = (*departmentwideTiesToSpine)(nil)

func (*departmentwideTiesToSpine) ID() string { return "departmentwide-ties-to-spine" }
func (*departmentwideTiesToSpine) Tier() int  { return 1 }
func (*departmentwideTiesToSpine) Full() bool { return false }
func (*departmentwideTiesToSpine) Description() string {
	return "Budget Book pp.85-125's departmentwide expenditures sum, per object category, to " +
		"exactly what the citywide spine publishes for expenditure"
}

func (*departmentwideTiesToSpine) Run(_ context.Context, s *Subject) (Result, error) {
	detailCells := detailSums(s.Facts, departmentwideScope, departmentwideRestriction)
	spineCells := detailSums(s.Facts, spineScope, departmentwideRestriction)

	// Vacuous when the scope is empty, for the reason detail.go gives at length:
	// the shared fixture carries spine expenditure cells with no departmentwide
	// counterpart, so a literal reading of "a spine key with no detail is a
	// failure" would redden every test that builds a subject.
	if len(detailCells) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: fmt.Sprintf("no fact is in scope %q, so there are no departmentwide "+
				"expenditures to reconcile against the spine (Budget Book pp.85-125 are "+
				"fisc-7q6)", departmentwideScope),
			Findings: []Finding{},
		}, nil
	}

	// The pairs are decided on the UNFOLDED maps, the same computation every
	// other detail check does: reconciledPairs reads only the year and the
	// basis, and folding away the fund group cannot change either.
	reconcile, unmatched := reconciledPairs(detailCells, spineCells)

	detail := byCategory(detailCells)
	spine := byCategory(spineCells)

	byException := map[categoryKey]departmentwideException{}
	for _, e := range departmentwideExceptions {
		byException[e.key()] = e
	}
	applied := map[categoryKey]bool{}

	var findings []Finding
	subjects, oneSided, exempt := 0, 0, 0
	for _, k := range unionCategoryKeys(detail, spine) {
		if !reconcile[k.yb()] {
			continue
		}
		d, sp := detail[k], spine[k]

		if e, ok := byException[k]; ok {
			// NOT COUNTED AMONG THE SUBJECTS, for fundingsources.go's reason:
			// `held` says every subject equals the spine to the cent and this
			// one deliberately does not.
			exempt++
			applied[k] = true
			if d.cents != e.printedCents {
				findings = append(findings, finding(k.String(),
					"declared to reconcile against %s, the figure the eleven departmentwide "+
						"pages print between them, and the detail sums to %s. %s",
					e.printedCents, d.cents, e.reason))
			}
			if sp.cents != e.spineCents {
				findings = append(findings, finding(k.String(),
					"declared against a spine cell of %s and the spine now publishes %s. If "+
						"p0067's figure has been corrected, delete this exception (%s) rather "+
						"than re-pointing it: the cell then ties on its own",
					e.spineCents, sp.cents, e.bead))
			}
			continue
		}

		subjects++
		if !d.present || !sp.present {
			oneSided++
		}
		if d.cents == sp.cents {
			continue
		}
		switch {
		case !d.present:
			findings = append(findings, finding(k.String(),
				"the spine publishes %s of expenditure in this object category and no "+
					"department prints a row of it; every dollar the city spends is spent by "+
					"a division under some object heading, so a category with no division "+
					"behind it is a rule that was dropped", sp.cents))
		case !sp.present:
			findings = append(findings, finding(k.String(),
				"the departments spend %s under this object category and the spine has no "+
					"such cell; %q must decompose the spine, never extend it",
				d.cents, departmentwideScope))
		default:
			findings = append(findings, finding(k.String(),
				"the departmentwide rows sum to %s and the spine publishes %s, a difference "+
					"of %s; these are the same money decomposed two ways and must tie to the "+
					"cent", d.cents, sp.cents, d.cents-sp.cents))
		}
	}

	// EVERY DECLARED EXCEPTION MUST HAVE BEEN APPLIED, for the reason
	// fundingsources.go gives: an entry naming a cell neither scope produces is
	// silently inert while `held` advertises it as a live reconciliation.
	for _, e := range departmentwideExceptions {
		if applied[e.key()] {
			continue
		}
		// A pair the spine does not publish is not a stale entry -- the union
		// loop skips an unreconciled (fiscal year, basis) before reaching the
		// exception arm.
		if !reconcile[e.key().yb()] {
			continue
		}
		findings = append(findings, finding(e.key().String(),
			"a declared exception names this cell and neither scope produces it, so it "+
				"reconciles nothing while the summary reports it as reconciled. Delete "+
				"the entry, or fix its key (%s)", e.bead))
	}

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pair(s) the spine publishes, "+
		"each the sum of pp.85-125's division object rows equal to the spine's own "+
		"expenditure for that object category to the cent; %d of those are a key only one "+
		"scope produces and both sides agree at zero", subjects, len(reconcile), oneSided)
	if exempt > 0 {
		held += fmt.Sprintf("; %d further cell(s) are declared exceptions and are NOT among "+
			"the %d", exempt, subjects)
	}
	for _, e := range departmentwideExceptions {
		if !applied[e.key()] {
			continue
		}
		held += fmt.Sprintf("; %s is reconciled against %s rather than against the spine's %s, "+
			"and both figures are printed: %s (%s)",
			e.key(), e.printedCents, e.spineCents, e.reason, e.bead)
	}
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d further pair(s) the detail publishes have no spine column "+
			"and are not reconciled: %s", len(unmatched), describePairs(unmatched))
	}
	return conclusion{
		subjects: subjects,
		unit:     "cells",
		held:     held,
		nothing: fmt.Sprintf("scope %q carries facts but the spine publishes no expenditure at "+
			"all, so nothing could be reconciled", departmentwideScope),
		findings: findings,
	}.result(), nil
}

// unionCategoryKeys is every key either side produced, in a stable order so two
// runs report the same findings in the same sequence.
//
// THE UNION, NOT THE DETAIL'S KEYS, for the reason compareDetail gives: an
// object category that vanished from the detail side entirely would be invisible
// to a detail-keyed loop, and a spine key inside the restriction with no
// counterpart is a FAILURE rather than a skip.
func unionCategoryKeys(a, b map[categoryKey]cellSum) []categoryKey {
	keys := make([]categoryKey, 0, len(a)+len(b))
	seen := map[categoryKey]bool{}
	for _, m := range []map[categoryKey]cellSum{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].year != keys[j].year {
			return keys[i].year < keys[j].year
		}
		if keys[i].basis != keys[j].basis {
			return keys[i].basis < keys[j].basis
		}
		return keys[i].category < keys[j].category
	})
	return keys
}
