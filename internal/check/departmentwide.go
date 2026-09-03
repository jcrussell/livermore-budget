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

// departmentwideException is a cell held apart from the ordinary comparison
// because the spine's own page is wrong.
//
// IT IS NOT fundingSourcesException ON THE OTHER AXIS, AND THE DIFFERENCE IS THE
// ONE THIS PROJECT CARES MOST ABOUT. That type's two figures are PRINTED --
// 26,294,515 appears on eight extracted pages and 26,544,515 on two -- so its
// entry asserts a page against a page. NEITHER FIGURE HERE IS PRINTED ANYWHERE.
// grep data/extracted for 130,252,087 or 130,502,087 and there are no hits: they
// are citywide object-category totals, which this corpus never prints, and they
// exist only as sums this check computes. An earlier draft of this file copied
// the other type's wording and claimed both were read off a page. They are not,
// and saying so in a string `fisc verify` prints on every run is the
// published-versus-derived invariant broken in published text.
//
// SO THE ENTRY IS PINNED FROM TWO DIRECTIONS INSTEAD. detailCents and spineCents
// are regression pins on our own arithmetic: the cell still fails if either side
// moves, which is what stops the exception hiding a later mapping error. What
// makes it more than a pair of magic numbers is the third arm -- their
// DIFFERENCE must equal the discrepancy fundingSourcesExceptions declares
// between two figures that ARE printed. That is where the grounding lives, and
// it is one edge rather than a second copy: correct p0067 and delete that entry,
// and this one goes red demanding the same.
type departmentwideException struct {
	category string
	year     int
	basis    mapping.Basis

	// spineCents and detailCents are what the two sides SUM TO, not figures any
	// page prints. See the type comment before adding a third.
	spineCents  amount.Cents
	detailCents amount.Cents

	bead   string
	reason string
}

// discrepancy is what this entry holds apart, and it must be grounded in printed
// figures rather than in this file.
func (e departmentwideException) discrepancy() amount.Cents {
	return e.spineCents - e.detailCents
}

// printedDiscrepancy is the same quantity taken from the check whose figures ARE
// printed, and is what grounds every entry above.
//
// It returns ok=false rather than zero when no funding-sources exception names
// the year: zero is a legitimate difference and would let a missing ground pass
// as agreement.
func printedDiscrepancy(year int, basis mapping.Basis) (amount.Cents, bool) {
	for _, e := range fundingSourcesExceptions {
		if e.year == year && e.basis == basis {
			return e.spineCents - e.printedCents, true
		}
	}
	return 0, false
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

	spineCents:  amount.Cents(13050208700),
	detailCents: amount.Cents(13025208700),

	bead: "fisc-av0w",
	reason: "p0067's Internal Service Funds column prints Services & Supplies 16,796,010 " +
		"for FY2026-27, and the five internal service funds' own printed rows on " +
		"pp.172-183 sum to 250,000 less. The spine carries p0067's figure, so the citywide " +
		"services-and-supplies cell it publishes is 250,000 above what pp.85-125's division " +
		"rows come to. These pages are an INDEPENDENT witness rather than a sixth copy of " +
		"the same schedule: they decompose the money by department, division and object " +
		"with no fund dimension at all, and they still put the difference in this category " +
		"and this year and in none of the other seven cells, which tie to the cent",
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
//     one thing and not two: each division's own printed Division Total, which
//     is these rules' total_row and is compared at build time. Five of the 29
//     miss it by exactly one dollar, every one in the FY2023-24 Actual column,
//     declared as stated_total_deltas.
//
//     Total Department Expenditures does NOT hold them, and it is worth saying
//     so because it looks as though it should. No rule and no rollup reads that
//     row -- all 29 rules stop at Division Total. That its printed value equals
//     the sum of its page's printed Division Totals in all 44 (page, column)
//     cells is a measurement taken by hand while writing this lane, not a
//     guarantee anything re-checks. A rollup over it is the obvious next
//     guard and this lane did not build one.
//
//   - dw-maintenance's Transfers Out row, 266,798 in FY2023-24 Actual and a
//     printed dash in all three later columns. The restriction excludes it by
//     kind, and it is why the eleven pages' printed totals exceed p0183's Grand
//     Total by 266,799 in that column and tie exactly in the other three --
//     p0183 is an EXPENDITURE total and correctly leaves a transfer out of it.
//
//     The remaining 1 is a difference between two figures the CITY printed, in
//     two different schedules, and this lane does not know which is right. It is
//     NOT the rounding the dw-* rules declare as stated_total_deltas: those
//     reconcile our sum of a block's rows against that block's own printed
//     total, which is a different quantity and lives inside one schedule.
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
			if d.cents != e.detailCents {
				findings = append(findings, finding(k.String(),
					"declared to reconcile against %s, what pp.85-125's rows sum to in this "+
						"category, and the detail now sums to %s. %s",
					e.detailCents, d.cents, e.reason))
			}
			// THE ARM THAT GROUNDS THE OTHER TWO. Neither figure above is
			// printed, so on their own they pin our arithmetic to itself. What
			// they are allowed to differ BY is the discrepancy declared by the
			// check whose figures the city really does print.
			if want, ok := printedDiscrepancy(k.year, k.basis); !ok {
				findings = append(findings, finding(k.String(),
					"holds %s apart from the spine, and no funding-sources exception names "+
						"this column any more. Neither figure in this entry is printed, so "+
						"that entry is the whole of its grounding: delete this one too, or "+
						"say what prints the difference (%s)", e.discrepancy(), e.bead))
			} else if e.discrepancy() != want {
				findings = append(findings, finding(k.String(),
					"holds %s apart from the spine where funding-sources-tie-to-spine holds "+
						"%s, and they are the same $250,000 seen on two axes. One of the two "+
						"entries has been re-pointed without the other (%s)",
					e.discrepancy(), want, e.bead))
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
		held += fmt.Sprintf("; %s is held apart at %s against the spine's %s. NEITHER FIGURE IS "+
			"PRINTED -- this corpus prints no citywide object-category total, so both are "+
			"sums this check computes; what is printed is the %s between them, which "+
			"funding-sources-tie-to-spine reconciles against pages that carry it: %s (%s)",
			e.key(), e.detailCents, e.spineCents, e.discrepancy(), e.reason, e.bead)
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
