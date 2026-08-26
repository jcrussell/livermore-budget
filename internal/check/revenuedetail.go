package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// revenueDetailScope is the scope Budget Book pp.127-140 are mapped at, spelled
// here for the reason expenditureDetailScope is: the hazard the check exists for
// is a scope string that internal/check and mappings/ spell differently.
const revenueDetailScope = "revenue-by-fund"

// transfersDetailScope is p76's scope (fisc-aes), named here before anything
// writes it because this check's one exception hands a cell to it. A string that
// is only ever compared and never written is exactly where a typo survives, so
// it is spelled once and the exception below fails loudly if it stops matching.
const transfersDetailScope = "transfers-by-fund"

// The restriction is the KIND HALF ONLY, and the absence of the fund-group half
// is the load-bearing part rather than an omission.
//
// pp.127-140 span all six fund groups — pp.127-130 are the General Fund and
// pp.131-140 are the other five — so pinning one would silently drop the
// schedule's larger half. Copying the department lane's restriction here would
// do exactly that; detail.go:29-30 says so from the other side.
//
// TRANSFER_IN IS IN THE RESTRICTION because the schedule prints it. Eleven fund
// blocks on pp.131-140 carry a Transfers In row inside one printed
// `Total <fund>`, and four more are transfers-only. Leaving the kind out would
// publish 21,045,597 of FY2026 transfers at this scope with nothing reconciling
// them, which is the hole the scope guard exists to prevent, one kind over.
//
// The schedule prints no Transfers Out at all, and no expenditure or fund
// balance, so those keys stay out and the check never meets the ~$254M of spine
// expenditure the department lane's comment measures.
var revenueDetailRestriction = detailRestriction{
	Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn},
}

// revenueDetailException is a cell inside the restriction that pp.127-140 do not
// print, together with the scope that reconciles it instead.
//
// AN EXCEPTION NAMES ITS SUCCESSOR OR IT IS A HOLE. fisc-u2v's 2026-08-23
// correction sets the rule for all three detail lanes: an exception either names
// the scope that covers the key and fails if that scope carries facts and not
// this one, or it says in words that the cell is unreconciled. It may not simply
// be silent, because a bare exemption survives the scope that was to retire it
// being deferred or dropped, and the cell then belongs to no check at all.
//
// So this type carries both halves, and Run below implements four arms over
// them rather than one skip.
type revenueDetailException struct {
	kind      mapping.Kind
	category  string
	fundGroup string

	// coveredBy is the scope that reconciles this key instead, and bead is the
	// work that will make that true. While that scope carries no facts the
	// cell is genuinely unreconciled and this check says so on every run.
	coveredBy string
	bead      string

	// reason is why the schedule does not print the row. It is published in
	// the summary, so it is about the DOCUMENT and not about our pipeline.
	reason string
}

// matches ignores year and basis: the schedule either prints a row or it does
// not, and pp.127-130 do not print this one in any column.
func (e revenueDetailException) matches(k detailKey) bool {
	return k.category == e.category && k.fundGroup == e.fundGroup
}

func (e revenueDetailException) String() string {
	return fmt.Sprintf("(%s, %s, %s)", e.kind, e.category, e.fundGroup)
}

// revenueDetailExceptions is the whole list, and it is one entry long.
//
// The corpus-wide budget across the three detail scopes is one, and this is it:
// pp.167-170 need none (detail.go), and fisc-aes records that p76's IN side
// needs none either.
var revenueDetailExceptions = []revenueDetailException{{
	kind: mapping.KindTransferIn, category: "transfers/in", fundGroup: "general",

	coveredBy: transfersDetailScope,
	bead:      "fisc-5gk.3.1",
	reason: "pp.127-130 print TOTAL REVENUES for the General Fund and no Transfers In " +
		"row at all — p130's `Total General Fund` is a revenue total, not a sources " +
		"total — so the spine's General Fund transfer in has no counterpart in this " +
		"schedule. It is not unaccounted for: p76 prints it, 480,400 in FY2026 and " +
		"486,735 in FY2027, which is p0066.txt:24 to the cent",
}}

// revenueDetailTiesToSpine asserts Budget Book pp.127-140 reconcile against the
// citywide spine rather than adding to it.
//
// THE FAILURE THIS EXISTS FOR IS THE LARGEST SILENT ONE IN THE CORPUS. Summed
// and keyed the way internal/project's netCells keys cells, pp.127-140 reproduce
// THE ENTIRE REVENUE SIDE of pp.66-67 exactly — all six fund groups, both budget
// years. That exactness is what makes a mistake invisible: a rule written at
// `all-funds-gross` puts 924 facts into the spine's own cells and the published
// General Fund property tax doubles from $64,143,762 to $128,287,524 with every
// graph check green, because those checks tie the graph to the facts it was
// built from and not to the document.
//
// AND UNLIKE THE DEPARTMENT LANE THERE IS NO SECOND GUARD. netCells refuses a
// fact carrying a department outright, so a mis-scoped pp.167-170 rule fails
// loudly. It has no such refusal for a fund: these facts would flow straight in.
// The scope string and this check are the whole of it.
//
// TIER 1, ZERO TOLERANCE. Measured over the committed corpus: 0 value
// differences. p127's known +$1 (fisc-2sd) is in the FY2023-24 Actual column,
// which the spine prints no counterpart for and this check therefore never
// reconciles; stated_total_deltas already absorbs it at build time.
//
// WHAT IT DOES NOT COVER, said here because the summary says it on every run:
// pp.66-67 print no actual or revised column, so the schedule's FY2024 and
// FY2025 halves — 462 of the 924 facts — tie to the schedule's own printed
// totals at build time and to nothing on the spine.
type revenueDetailTiesToSpine struct{}

var _ Check = (*revenueDetailTiesToSpine)(nil)

func (*revenueDetailTiesToSpine) ID() string { return "revenue-detail-ties-to-spine" }
func (*revenueDetailTiesToSpine) Tier() int  { return 1 }
func (*revenueDetailTiesToSpine) Full() bool { return false }
func (*revenueDetailTiesToSpine) Description() string {
	return "Budget Book pp.127-140's revenue by fund sums, per category and fund group, to " +
		"exactly what the citywide spine publishes"
}

func (*revenueDetailTiesToSpine) Run(_ context.Context, s *Subject) (Result, error) {
	detail := detailSums(s.Facts, revenueDetailScope, revenueDetailRestriction)
	spine := detailSums(s.Facts, spineScope, revenueDetailRestriction)

	// Vacuous when the scope is empty, for the reason detail.go gives at
	// length: the shared fixture carries spine revenue and transfer-in cells
	// with no counterpart here, so a literal reading of "a spine key with no
	// detail is a failure" would redden every test that builds a subject.
	if len(detail) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: fmt.Sprintf("no fact is in scope %q, so there is no revenue detail to "+
				"reconcile against the spine (Budget Book pp.127-140 are fisc-5gk.1)",
				revenueDetailScope),
			Findings: []Finding{},
		}, nil
	}

	reconcile, unmatched := reconciledPairs(detail, spine)

	exempt, notes, findings := resolveRevenueExceptions(s, detail, spine, reconcile)
	cmp := compareDetail(detail, spine, reconcile, revenueDetailScope, exempt)
	findings = append(findings, cmp.findings...)

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pairs the spine publishes, each "+
		"the sum of pp.127-140's rows equal to the spine's own figure to the cent; %d of "+
		"those are a key only one scope produces and both sides agree at zero",
		cmp.subjects, len(reconcile), cmp.oneSided)
	if cmp.exempt > 0 {
		held += fmt.Sprintf("; %d further cell(s) are declared exceptions and are NOT among "+
			"the %d", cmp.exempt, cmp.subjects)
	}
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d further pair(s) the detail publishes have no spine column "+
			"and are not reconciled: %s", len(unmatched), describePairs(unmatched))
	}
	for _, n := range notes {
		held += "; " + n
	}
	return conclusion{
		subjects: cmp.subjects,
		unit:     "cells",
		held:     held,
		nothing: fmt.Sprintf("scope %q carries facts but the spine publishes no revenue at "+
			"all, so nothing could be reconciled", revenueDetailScope),
		findings: findings,
	}.result(), nil
}

// resolveRevenueExceptions settles each declared exception against the corpus
// and reports what it found.
//
// FOUR ARMS, because an exception can stop being true in three different ways
// and only one of them is "still pending":
//
//  1. THE SCHEDULE DOES PRINT THE ROW. The exemption is a false claim about the
//     document and must go, or a real detail-side figure is silently excused.
//     This is the arm an unconditional exemption would never have.
//  2. THE NAMED SCOPE COVERS THE KEY. The cell is reconciled, by another check,
//     and this one steps aside. Reported, not silent.
//  3. THE NAMED SCOPE EXISTS AND DOES NOT COVER THE KEY. Its schedule was
//     mapped and this cell was not in it, so the hand-off failed. FAIL.
//  4. THE NAMED SCOPE CARRIES NOTHING YET. The cell is genuinely unreconciled
//     and the summary says so by name on every run, with the bead that closes
//     it. fisc-u2v's 2026-08-23 correction allows exactly this — "if no scope
//     covers it, the exception must say in words that the cell is unreconciled"
//     — and arm 3 is what stops it outliving the work.
func resolveRevenueExceptions(s *Subject, detail, spine map[detailKey]cellSum,
	reconcile map[yearBasis]bool) (func(detailKey) bool, []string, []Finding) {

	var notes []string
	var findings []Finding
	exempted := map[detailKey]bool{}

	for _, e := range revenueDetailExceptions {
		// Only keys the spine actually publishes in a reconciled slice are at
		// stake; an exception for a cell nobody prints exempts nothing.
		var keys []detailKey
		for k := range spine {
			if e.matches(k) && reconcile[yearBasis{k.year, k.basis}] {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].year != keys[j].year {
				return keys[i].year < keys[j].year
			}
			return keys[i].basis < keys[j].basis
		})
		if len(keys) == 0 {
			findings = append(findings, finding(e.String(),
				"this exception is declared and the spine publishes no such cell in any "+
					"reconciled slice, so it exempts nothing; remove it rather than "+
					"leaving a declaration that has stopped describing the corpus"))
			continue
		}

		// (1) The schedule prints the row after all.
		printed := false
		for _, k := range keys {
			if detail[k].present {
				printed = true
			}
		}
		if printed {
			findings = append(findings, finding(e.String(),
				"scope %q publishes this cell, so the declared exception is a false claim "+
					"about Budget Book pp.127-140 and would silently excuse a real figure; "+
					"remove it and let the cell reconcile like every other",
				revenueDetailScope))
			continue
		}

		// THE HAND-OFF IS PER SLICE, NOT PER EXCEPTION. Counting "does the
		// successor cover this cell" over the whole fact store and then
		// exempting every key at once is the bug this loop is shaped to avoid:
		// a transfers-by-fund that lands carrying FY2026 only would exempt
		// FY2027 as well, the summary would claim both were reconciled, and
		// arm 3 -- which exists for exactly a failed hand-off -- would never
		// fire. A scope covers a cell in a slice or it does not.
		carries := 0
		covers := map[detailKey]int{}
		for i := range s.Facts {
			f := &s.Facts[i]
			if f.Scope != e.coveredBy {
				continue
			}
			carries++
			if f.Kind != e.kind || f.Category != e.category || f.FundGroup != e.fundGroup {
				continue
			}
			covers[detailKey{f.FiscalYear, f.Basis, f.FundGroup, f.Category}]++
		}

		// THE TEST IS PRESENCE, AND THE WORDING SAYS SO (fisc-8ka).
		//
		// covers[k] counts FACTS, never cents, so this arm establishes that the
		// covering scope publishes something on the key -- not that what it
		// publishes equals the spine. It used to report "reconciled by scope X
		// instead", which is a reconciliation this check did not perform and
		// would have gone on claiming if X published one cent under that key.
		//
		// A check must not assert another check's conclusion. What DOES compare
		// the amount is transfers-detail-ties-to-spine, clause (a): p76's
		// receiving legs against the spine's own TRANSFER IN cell, zero
		// tolerance. That check is the one entitled to the word "reconciled";
		// this one says where the cell went and stops.
		var handed, pending []detailKey
		for _, k := range keys {
			if covers[k] > 0 {
				handed = append(handed, k)
			} else {
				pending = append(pending, k)
			}
		}
		for _, k := range handed {
			exempted[k] = true
		}
		if len(handed) > 0 {
			notes = append(notes, fmt.Sprintf("%s is not printed by this schedule and is "+
				"handed off to scope %q in %s, which publishes the key; whether the "+
				"amounts agree is that scope's own check to make, not this one's",
				e.String(), e.coveredBy, describeSlices(handed)))
		}
		switch {
		case len(pending) == 0:
			// Fully handed off.
		case carries > 0:
			// (3) The successor was mapped and dropped these slices.
			findings = append(findings, finding(e.String(),
				"this cell is exempted here on the grounds that scope %q covers it, and "+
					"that scope carries %d facts and none on this key in %s. The hand-off "+
					"has failed and %s of the spine's own money is now reconciled by "+
					"nothing", e.coveredBy, carries, describeSlices(pending),
				describeCents(spine, pending)))
		default:
			// (4) Pending. Loud, and it becomes arm 3 the moment work lands.
			for _, k := range pending {
				exempted[k] = true
			}
			notes = append(notes, fmt.Sprintf("%s is NOT RECONCILED: %s. Scope %q will "+
				"reconcile it and carries no fact yet (%s), so %s is published by the "+
				"spine and checked against no detail",
				e.String(), e.reason, e.coveredBy, e.bead, describeCents(spine, pending)))
		}
	}
	if len(exempted) == 0 {
		return nil, notes, findings
	}
	return func(k detailKey) bool { return exempted[k] }, notes, findings
}

// describeSlices names the (fiscal year, basis) pairs a set of keys covers.
func describeSlices(keys []detailKey) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("FY%d %s", k.year, k.basis))
	}
	sort.Strings(out)
	return joinComma(out)
}

// describeCents renders the money at stake across an exception's keys, so a
// summary line or a finding says how much rather than only which cell.
func describeCents(sums map[detailKey]cellSum, keys []detailKey) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("FY%d %s %s", k.year, k.basis, sums[k].cents))
	}
	return joinComma(out)
}
