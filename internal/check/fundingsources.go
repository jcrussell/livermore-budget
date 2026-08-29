package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// fundingSourcesScope is the scope Budget Book pp.85-125's Department Funding
// Sources blocks are mapped at, spelled here for the reason detail.go gives:
// the hazard this check exists for is a scope string two readers spell
// differently.
const fundingSourcesScope = "department-funding-sources"

// The restriction is the KIND half only, and the absence of the fund-group half
// is a claim about the pages rather than a default.
//
// pp.85-125 name every fund that pays for a department, so their rows span all
// six of the spine's fund groups and a seventh it has no column for. Copying
// expenditure-detail-ties-to-spine's `FundGroups: []string{"general"}` here
// would drop five sixths of the schedule and leave the check tying $144M of a
// $254M reconciliation, green.
//
// The kind half is still load-bearing: without it the union meets the spine's
// revenue, transfer and fund-balance cells, which these pages print none of.
var fundingSourcesRestriction = detailRestriction{
	Kinds: []mapping.Kind{mapping.KindExpenditure},
}

// groupKey is one cell this check compares: a fund group inside a column.
//
// IT IS detailKey WITHOUT THE CATEGORY, and that is the whole difference between
// this check and the other three. pp.85-125 decompose a department's money by
// PAYING FUND, not by object, so a funding-source fact carries the single
// category `department-funding-sources` while the spine carries four object
// categories per group. Keyed on category the two sides would share no key at
// all and every cell would be one-sided — the check would fail on all twelve
// cells while the underlying arithmetic ties exactly.
type groupKey struct {
	year      int
	basis     mapping.Basis
	fundGroup string
}

func (k groupKey) String() string {
	return fmt.Sprintf("FY%d %s %s", k.year, k.basis, k.fundGroup)
}

func (k groupKey) yb() yearBasis { return yearBasis{k.year, k.basis} }

// byFundGroup folds detailSums' per-category cells into per-group ones.
//
// It reuses detailSums rather than re-walking the fact store so that both sides
// of this comparison are built by the same code every other detail check uses,
// and so reconciledPairs — which reads only the year and basis — can be fed the
// unfolded maps unchanged.
func byFundGroup(m map[detailKey]cellSum) map[groupKey]cellSum {
	out := map[groupKey]cellSum{}
	for k, c := range m {
		g := groupKey{k.year, k.basis, k.fundGroup}
		acc := out[g]
		acc.cents += c.cents
		acc.present = acc.present || c.present
		out[g] = acc
	}
	return out
}

// fundingSourcesException is a cell reconciled against a PRINTED figure instead
// of against the spine, because the spine's own page is wrong.
//
// IT IS NOT AN EXEMPTION. The cell is still examined and can still fail, in
// three ways: the detail may move off printedCents, the spine may move off
// spineCents, or the entry may be deleted, at which point the ordinary
// comparison fails on the difference. An exception that merely skipped the cell
// would be indistinguishable from the mapping having quietly lost $250,000.
//
// BOTH FIGURES ARE READ OFF A PAGE AND NEITHER IS DERIVED. That is the rule
// transfersdetail.go's toCIP table states and the difference between a check and
// a tautology: computing spineCents-minus-printedCents from the two sides would
// make this entry agree with whatever the corpus happens to say.
type fundingSourcesException struct {
	fundGroup string
	year      int
	basis     mapping.Basis

	// spineCents is what pp.66-67 print, and printedCents is what the rest of
	// the book prints for the same quantity. The exception asserts BOTH.
	spineCents   amount.Cents
	printedCents amount.Cents

	// bead is the work that decides what the corpus should publish, and reason
	// is why the two figures differ. reason is published in the summary on
	// every run, so it is about the DOCUMENT and not about our pipeline.
	bead   string
	reason string
}

func (e fundingSourcesException) key() groupKey {
	return groupKey{e.year, e.basis, e.fundGroup}
}

// fundingSourcesExceptions is the whole list, and it is one entry long.
//
// Eleven of the twelve (fund group, budget year) cells tie to the cent with no
// exception, no tolerance and no constant. This is the twelfth.
var fundingSourcesExceptions = []fundingSourcesException{{
	fundGroup: "internal-service", year: 2027, basis: mapping.BasisAdopted,

	spineCents:   amount.Cents(2654451500),
	printedCents: amount.Cents(2629451500),

	bead: "fisc-av0w",
	reason: "p0067's Internal Service Funds column is the one cell in this book that " +
		"disagrees with the rest of it. The same book's line-item schedule, Citywide " +
		"Expenditures pp.172-183, prints every fund's object rows and one Total per fund " +
		"group; DERIVED from those printed lines, the six groups by four object categories " +
		"over two budget years give 48 cells and 47 agree with pp.66-67 to the dollar. " +
		"Neither pp.172-183 nor any other page prints a group-by-object subtotal, so that " +
		"48-cell grid is our arithmetic and not the city's -- what the pages print, and " +
		"what this exception rests on, are the Totals below. The cell that disagrees is " +
		"this group's Services & Supplies for FY2026-27, where p0067 prints 16,796,010 " +
		"and the five internal service funds' printed rows sum to 250,000 less. Five " +
		"published " +
		"schedules give this group's FY2026-27 expenditure as 26,294,515 and none gives " +
		"26,544,515: p0183:64, p0075:53, p0205:17, p0209:20, and p0061:39 (26,906,515 = " +
		"26,294,515 + the 612,000 transfer to the CIP). p0067's error runs on down its own " +
		"column — TOTAL USES 27,156,515 against 26,906,515 elsewhere, ENDING WORKING " +
		"CAPITAL 8,529,087 against 8,779,087 — while its revenue side reconciles exactly. " +
		"pp.85-125 agree with the other five. No FY2026-27 figure of 250,000 appears on " +
		"any internal service fund anywhere in the corpus, so the difference is not an " +
		"item either side is missing",
}}

// fundingSourcesTiesToSpine asserts Budget Book pp.85-125's Department Funding
// Sources reconcile against the citywide spine rather than adding to it.
//
// THE FAILURE THIS EXISTS FOR IS THE SAME SILENT ONE THE OTHER THREE GUARD, and
// on this schedule it is the whole city rather than one slice of it. Summed by
// the fund each row names, the 78 rows reproduce pp.66-67's TOTAL EXPENDITURES
// for eleven of the twelve (fund group, budget year) cells — every group in
// FY2026, and every group but internal-service in FY2027, whose twelfth cell is
// the exception declared above. The FY2026 total is 254,095,412, which IS
// all_funds_gross_expenditure_cents, the headline the site publishes. These
// pages give 252,604,896 for FY2027 and the headline there is 252,854,896,
// because the spine carries p0067's figure — do not read the schedule's own
// total as the headline in both years.
//
// A rule written at `all-funds-gross` would put these facts in the spine's own
// cells and double the city's expenditure with every graph check green, because
// those checks tie the graph to the facts it was built from and not to the
// document.
//
// AND THE SECOND GUARD DOES NOT REACH HERE EITHER. netCells refuses a fact
// carrying a department, which is what makes a mis-scoped pp.167-170 rule fail
// loudly; these rows carry no department, only a fund. As with pp.127-140 the
// scope string and this check are the whole of it.
//
// THAT THEY CARRY NO DEPARTMENT IS A CONSTRAINT AND NOT A CHOICE, which is
// worth knowing before reading the paragraph above as an oversight.
// fact-departments-resolve resolves the field against a DIVISION, and six of
// data/departments.yaml's eleven departments are not division slugs, so typing
// it here reddens six of the eleven rules. fisc-xudn owns the decision, and it
// has to be made before fisc-7q6 maps the same fourteen pages at the division
// tier and carries a department on every row.
//
// TIER 1, ZERO TOLERANCE, AND THE TOLERANCE QUESTION IS SETTLED BY THE PAGES.
// Eleven of the twelve reconciled cells tie to the cent. The twelfth is a defect
// in p0067 that five other schedules contradict, and it is handled by naming
// both printed figures rather than by widening a bound — see
// fundingSourcesExceptions. A tolerance sized to absorb 250,000 would also
// absorb a whole missing fund.
//
// WHAT IT DOES NOT COVER, said here because the summary says it on every run:
//
//   - pp.66-67 print no actual or revised column, so the schedule's FY2024 and
//     FY2025 halves — 156 of its 312 facts — tie to each department's own
//     printed Total Department Funding Sources at build time and to nothing on
//     the spine. Five of the eleven departments miss that printed total by
//     exactly one dollar, every one of them in the FY2023-24 Actual column and
//     nowhere else; those are declared as stated_total_deltas.
//
//   - The `permanent` group. Doolan Canyon Preserve Endow (470) pays 620,581 of
//     Community Development in FY2023-24 and nothing thereafter, and pp.66-67
//     print no Permanent column at all (fisc-u8o). It is reconciled by nothing
//     here — but it is zero in both budget years, so it is a group this check
//     names rather than a sum it is missing.
//
//   - The fund NUMBER on each row, by THIS check. It is hand-typed 78 times,
//     and what the arithmetic here catches is a fund of the wrong TYPE, because
//     that moves money between groups and breaks a sum. A same-type
//     substitution — Water 640 for CIP Water 641, and all four of Public Works'
//     operating/CIP twins keep their operating fund's type — moves no sum and is
//     invisible to it.
//
//     It is caught by row-funds-match-their-anchors instead, since these eleven
//     rules declare row_labels_name_funds and it reads the printed label
//     (fisc-90fp). rule-funds-match-their-headings still never enters: it reads
//     column funds and these rules declare none.
type fundingSourcesTiesToSpine struct{}

var _ Check = (*fundingSourcesTiesToSpine)(nil)

func (*fundingSourcesTiesToSpine) ID() string { return "funding-sources-tie-to-spine" }
func (*fundingSourcesTiesToSpine) Tier() int  { return 1 }
func (*fundingSourcesTiesToSpine) Full() bool { return false }
func (*fundingSourcesTiesToSpine) Description() string {
	return "Budget Book pp.85-125's department funding sources sum, per fund group, to exactly " +
		"what the citywide spine publishes for expenditure"
}

func (*fundingSourcesTiesToSpine) Run(_ context.Context, s *Subject) (Result, error) {
	detailCells := detailSums(s.Facts, fundingSourcesScope, fundingSourcesRestriction)
	spineCells := detailSums(s.Facts, spineScope, fundingSourcesRestriction)

	// Vacuous when the scope is empty, for the reason detail.go gives at length:
	// the shared fixture carries a spine expenditure cell with no funding-source
	// counterpart, so a literal reading of "a spine key with no detail is a
	// failure" would redden every test that builds a subject.
	if len(detailCells) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: fmt.Sprintf("no fact is in scope %q, so there are no department funding "+
				"sources to reconcile against the spine (Budget Book pp.85-125 are fisc-71j)",
				fundingSourcesScope),
			Findings: []Finding{},
		}, nil
	}

	// The pairs are decided on the UNFOLDED maps, which is the same computation
	// reconciledPairs does for every other detail check: it reads only the year
	// and the basis, and folding away the category cannot change either.
	reconcile, unmatched := reconciledPairs(detailCells, spineCells)

	detail := byFundGroup(detailCells)
	spine := byFundGroup(spineCells)

	byException := map[groupKey]fundingSourcesException{}
	for _, e := range fundingSourcesExceptions {
		byException[e.key()] = e
	}
	applied := map[groupKey]bool{}

	var findings []Finding
	subjects, oneSided, exempt := 0, 0, 0
	for _, k := range unionGroupKeys(detail, spine) {
		if !reconcile[k.yb()] {
			continue
		}
		d, sp := detail[k], spine[k]

		if e, ok := byException[k]; ok {
			// NOT COUNTED AMONG THE SUBJECTS, because `held` says every one of
			// them equals the spine to the cent and this one deliberately does
			// not. Counting it made the PASS line claim 14 cells tie where 13
			// do, which is the overstatement revenuedetail.go avoids by saying
			// its exceptions are "NOT among the 134". Reported separately
			// below. Found by the fourth review pass over this range.
			exempt++
			applied[k] = true
			// Both sides are asserted against a figure the book prints. Neither
			// is derived from the other, so this cell fails if either moves.
			if d.cents != e.printedCents {
				findings = append(findings, finding(k.String(),
					"declared to reconcile against %s, the figure pp.172-183 and four other "+
						"schedules print, and the detail sums to %s. %s",
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
				"the spine publishes %s of expenditure for this fund group and no department "+
					"names a fund in it; every dollar the city spends is spent by a "+
					"department, so a group with no payer is a rule that was dropped",
				sp.cents))
		case !sp.present:
			findings = append(findings, finding(k.String(),
				"the departments name %s paid by funds in this group and the spine has no "+
					"such cell; %q must decompose the spine, never extend it",
				d.cents, fundingSourcesScope))
		default:
			findings = append(findings, finding(k.String(),
				"the departments' funding sources sum to %s and the spine publishes %s, a "+
					"difference of %s; these are the same money decomposed two ways and must "+
					"tie to the cent", d.cents, sp.cents, d.cents-sp.cents))
		}
	}

	// EVERY DECLARED EXCEPTION MUST HAVE BEEN APPLIED. An entry is consulted
	// only from inside the union loop, so one naming a cell neither scope
	// produces -- a fund group that stopped appearing, a year the spine stopped
	// publishing, a typo in the key -- is silently inert while `held` goes on
	// advertising it as a live reconciliation. That is the same failure shape
	// staleDeclarations refuses for unprojectedScopes and declaredToCIPGroups
	// for transfersdetail.go's table, and it is a finding rather than a note
	// because a reader of the summary would have no way to tell.
	//
	// Found by /code-review over cd1192c.
	for _, e := range fundingSourcesExceptions {
		if applied[e.key()] {
			continue
		}
		// A PAIR THE SPINE DOES NOT PUBLISH IS NOT A STALE ENTRY. The union
		// loop skips an unreconciled (fiscal year, basis) before it reaches the
		// exception arm, so without this the check would tell a reader to delete
		// a still-valid entry the day pp.66-67 stopped printing an FY2027
		// column -- a change in the document, not a declaration going stale.
		// Found by the fourth review pass over this range.
		if !reconcile[e.key().yb()] {
			continue
		}
		findings = append(findings, finding(e.key().String(),
			"a declared exception names this cell and neither scope produces it, so it "+
				"reconciles nothing while the summary reports it as reconciled. Delete "+
				"the entry, or fix its key (%s)", e.bead))
	}

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pair(s) the spine publishes, "+
		"each the sum of pp.85-125's funding-source rows equal to the spine's own "+
		"expenditure for that fund group to the cent; %d of those are a key only one scope "+
		"produces and both sides agree at zero", subjects, len(reconcile), oneSided)
	if exempt > 0 {
		held += fmt.Sprintf("; %d further cell(s) are declared exceptions and are NOT among "+
			"the %d", exempt, subjects)
	}
	// ONLY THE ONES THAT FIRED. An entry the union loop never reached -- because
	// the spine stopped publishing its column, say -- would otherwise be printed
	// as a live reconciliation beside an `exempt` of zero and no holding-apart
	// clause, which is the check advertising coverage it did not provide. The
	// stale-entry arm above reports the other case, where the pair IS reconciled
	// and the cell is not produced. Found by the fifth review pass.
	for _, e := range fundingSourcesExceptions {
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
			"all, so nothing could be reconciled", fundingSourcesScope),
		findings: findings,
	}.result(), nil
}

// unionGroupKeys is every key either side produced, in a stable order so two
// runs report the same findings in the same sequence.
//
// THE UNION, NOT THE DETAIL'S KEYS, for the reason compareDetail gives: a fund
// group that vanished from the detail side entirely — every department's rule
// for it dropped — would be invisible to a detail-keyed loop, and a spine key
// inside the restriction with no counterpart is a FAILURE rather than a skip.
func unionGroupKeys(a, b map[groupKey]cellSum) []groupKey {
	keys := make([]groupKey, 0, len(a)+len(b))
	seen := map[groupKey]bool{}
	for _, m := range []map[groupKey]cellSum{a, b} {
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
		return keys[i].fundGroup < keys[j].fundGroup
	})
	return keys
}
