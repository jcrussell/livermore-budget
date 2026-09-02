package check

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// transfersDetailScope is declared beside revenue-detail-ties-to-spine's
// exception, which names it, and is used here.

// THE THIRD DETAIL LANE, AND THE FIRST WHOSE TWO SIDES ARE RECONCILED
// DIFFERENTLY. Budget Book p76 publishes both ends of every transfer it prints:
// a receiving leg keyed to the destination fund and a paying leg keyed to the
// payer, from one printed figure. Those two halves do not tie to pp.66-67 the
// same way, and pretending they do is the failure this file is written against.
//
// THE IN SIDE TIES EXACTLY, with no constant and no exception. Each section's
// total equals the spine's TRANSFER IN cell for that fund group to the cent, in
// both budget years.
//
// THE OUT SIDE CANNOT, and the reason is the document's. pp.66-67's TRANSFER
// OUT row includes transfers to the Capital Improvement Program, and p76 does
// not list them -- its grand total IS the transfers-in side. pp.72-75 print
// those to-CIP figures under a heading, and they are not mapped, so the check
// hand-types them. That is the whole of what makes this lane different from the
// other two.
const (
	// nonMajorGroup is the fund-group key clause (c) compares under. It is not
	// a fund group the corpus uses; it is deliberately unspellable as one, so a
	// collapsed cell cannot be mistaken in a finding for a real group's.
	nonMajorGroup = "capital+special-revenue"
)

// toCIP is what pp.66-67's TRANSFER OUT includes and p76 does not list: the
// transfers each fund group makes to the Capital Improvement Program, in cents,
// per (fund group, fiscal year).
//
// EVERY FIGURE HERE IS READ OFF A PAGE, AND NONE IS DERIVED AS spine MINUS
// detail. That distinction is the difference between a check and a tautology.
// TestP76SourcesDecomposeTheResidualByFundType computes exactly that difference
// and asserts these same numbers; copying its arithmetic into the comparand would
// make this check assert 0 == 0 forever.
//
//   - internal-service and the non-major aggregate are printed verbatim:
//     p0073.txt:53 and :56 for FY2026, p0075.txt:53 and :56 for FY2027. (The
//     40,000 and 612,000 also appear one line earlier, at :51, as a single
//     fund's row -- only one internal service fund pays a to-CIP transfer, so
//     the two lines carry the same figure. The GROUP total is the one cited.)
//   - general and debt-service are printed as "-", which this corpus spells
//     zero.
//   - ENTERPRISE IS THE ONE THAT IS NOT PRINTED ON ITS OWN LINE, and saying so
//     is the point. pp.72-75 print a Total Major Funds to-CIP -- 9,393,147 and
//     14,832,000 at p0073.txt:55 and p0075.txt:55 -- over a major-fund list that
//     is General Fund, Low Income Housing, the four enterprises and Internal
//     Service. Subtracting the printed internal-service line leaves enterprise,
//     because general is zero and Low Income Housing pays no transfer out in
//     either budget year. It is a difference of two figures on one page, not a
//     difference against our own read.
//
// AND pp.72-75's MAJOR/NON-MAJOR SPLIT IS NOT THIS CHECK'S CLAUSE SPLIT. Those
// pages put Low Income Housing (special-revenue) on the MAJOR side and both
// debt-service and capital on the NON-MAJOR side; clauses (b) and (c) divide
// the groups the other way. The two coincide only because Low Income Housing,
// debt-service and permanent all pay zero transfers out in both budget years.
// The day any of them pays one, this table stops being readable off those pages
// in this shape and the clause boundaries have to move with it.
var toCIP = map[string]map[int]amount.Cents{
	"general":          {2026: 0, 2027: 0},
	"debt-service":     {2026: 0, 2027: 0},
	"enterprise":       {2026: 935314700, 2027: 1422000000},
	"internal-service": {2026: 4000000, 2027: 61200000},
	nonMajorGroup:      {2026: 2869359000, 2027: 3593025100},
}

// transfersClause is one of the three comparisons this check makes. They are
// three rather than one because the two sides of a transfer reconcile
// differently and the non-major pair cannot be split at all.
type transfersClause struct {
	name        string
	restriction detailRestriction
	// collapse folds capital and special-revenue into one compared cell. Only
	// clause (c) sets it; see collapseNonMajor for why that is an exception to
	// detailRestriction's filter/key rule rather than a relaxation of it.
	collapse bool
	// toCIPGroups are the keys of toCIP this clause adds back, and it is a
	// separate field rather than "all of them" because a constant added under a
	// group the clause does not compare produces a detail cell the spine cannot
	// have. That is not a hypothetical: adding the whole table to both out
	// clauses put capital+special-revenue's 28,693,590 into the major clause
	// and enterprise's 9,353,147 into the non-major one, and the check reported
	// six findings while every real cell tied to the cent.
	toCIPGroups []string
}

var transfersClauses = []transfersClause{{
	name: "in",
	// No fund-group filter: p76's sections span five groups and the spine
	// publishes a TRANSFER IN cell for six.
	restriction: detailRestriction{Kinds: []mapping.Kind{mapping.KindTransferIn}},
}, {
	name: "out-major",
	restriction: detailRestriction{
		Kinds:      []mapping.Kind{mapping.KindTransferOut},
		FundGroups: []string{"general", "enterprise", "internal-service", "debt-service"},
	},
	toCIPGroups: []string{"general", "enterprise", "internal-service", "debt-service"},
}, {
	name: "out-non-major",
	restriction: detailRestriction{
		Kinds:      []mapping.Kind{mapping.KindTransferOut},
		FundGroups: []string{"capital", "special-revenue", "permanent"},
	},
	collapse:    true,
	toCIPGroups: []string{nonMajorGroup},
}}

// clausesCoverEveryFundGroup is the list the two out clauses must partition
// between them, asserted by a test rather than trusted.
//
// permanent is in the non-major clause and was left out of both when this check
// was first written -- pp.66-67 print no Permanent column, so no spine cell
// would ever have raised it, and a permanent payer would have been compared by
// nothing at all while the check went on claiming "decompose the spine, never
// extend it". p76 already prints a Permanent section on the receiving side, so
// a permanent payer is one counterpart declaration away. It belongs with
// capital and special-revenue because pp.72-75 put it there: their non-major
// aggregate is every fund that is not on the major list.
var clausesCoverEveryFundGroup = []string{
	"general", "enterprise", "internal-service", "debt-service",
	"capital", "special-revenue", "permanent",
}

// collapseNonMajor compares capital and special-revenue as ONE cell, and it is
// a real exception to detailRestriction's "IT IS A FILTER AND NOT A KEY" rule
// rather than a quiet widening of it. The argument has to be made here, on its
// own terms, because the invariant it breaks is correct everywhere else.
//
// THE COLLAPSE IS THE DOCUMENT DECLINING TO PRINT THE SPLIT, NOT THE
// RESTRICTION LEAKING INTO THE KEY. pp.72-75 print a to-CIP figure per major
// fund and then ONE aggregate line for every non-major fund. p76 itemises
// 1,028,200 of FY2026 transfers out paid by non-major funds, and the spine
// prints 28,584,740 capital and 1,137,050 special-revenue; the pair reconciles
// against the printed 28,693,590 exactly, and a per-group constant would have
// to be derived by difference from p67 while the PAIR is published. That is
// this project's own published-is-not-derived rule, and it is the reason, not
// an impossibility claim -- both years do in fact split arithmetically.
//
// WHAT IS GIVEN UP, stated because a collapsed cell hides it: money moved
// between capital and special-revenue is invisible here. That is not
// hypothetical -- five of p76's payer labels match an operating fund AND its
// CIP twin, and every twin is type: capital, so a leg declared under the wrong
// twin moves exactly this way and this check stays green. Catching that is
// fisc-bhe's row-anchor fund check, not this one, and no mutation of a twin can
// prove this check failable.
// It folds THREE groups, not two: capital, special-revenue and permanent, which
// is what pp.72-75 mean by non-major. permanent contributes zero on both sides
// today and is here so that it cannot contribute silently.
func collapseNonMajor(in map[detailKey]cellSum) map[detailKey]cellSum {
	out := make(map[detailKey]cellSum, len(in))
	for k, v := range in {
		k.fundGroup = nonMajorGroup
		c := out[k]
		c.cents += v.cents
		c.present = c.present || v.present
		out[k] = c
	}
	return out
}

// addToCIP folds the printed to-CIP figures into the DETAIL side.
//
// Folding rather than comparing with an offset is what keeps cellSum.present
// meaning one thing. p76 lists no internal-service payer at all, so that key is
// absent from the detail while the spine publishes 40,000; compared as-is it
// hits compareDetail's "the detail has no such row at all" arm and fails, when
// the truth is that the whole of that group's transfers out went to the CIP.
// After folding, present means "the detail scope or a printed constant says
// something about this key", the comparison is on cents throughout, and
// oneSided still counts what it did.
//
// THE CONSTANTS ARE ADOPTED-COLUMN FIGURES AND ARE FOLDED INTO NO OTHER BASIS.
// pp.72-75 print one budget column per year; there is no revised or actual
// to-CIP figure on those pages and this table holds none. Matching on year
// alone put the adopted constant into every basis the spine happens to publish
// for that year -- so the day pp.66-67 gain a revised column, the check would
// have reported "the detail sums to $9,353,147" for a basis where p76 publishes
// nothing, blaming the mapping for a number the check invented. Nothing in the
// corpus reaches it today, which is exactly why it needs saying: reconciledPairs
// is deliberately dynamic, so the reachable set grows without this file
// changing.
func addToCIP(in map[detailKey]cellSum, groups []string,
	reconcile map[yearBasis]bool) map[detailKey]cellSum {

	out := maps.Clone(in)
	if out == nil {
		out = map[detailKey]cellSum{}
	}
	for _, group := range groups {
		for year, cents := range toCIP[group] {
			for yb := range reconcile {
				if yb.year != year || yb.basis != mapping.BasisAdopted {
					continue
				}
				k := detailKey{year, yb.basis, group, "transfers/out"}
				c := out[k]
				c.cents += cents
				c.present = true
				out[k] = c
			}
		}
	}
	return out
}

// transfersDetailTiesToSpine asserts Budget Book p76 decomposes pp.66-67's
// transfer rows rather than adding to them.
//
// THE FAILURE THIS EXISTS FOR IS SILENT, and it is measured rather than feared.
// Publishing these rules at scope all-funds-gross instead doubles the General
// Fund's transfers in from 480,400 to 960,800 and roughly doubles the published
// transfer residual -- while link-values-tie-to-facts, counts-reconcile and
// headline-transfer-residual all stay GREEN, because they tie the graph to the
// facts it was built from rather than to the document.
//
// TIER 1, ZERO TOLERANCE. p76 ties to the cent on both budget columns and to
// the spine's TRANSFER IN and TRANSFER OUT rows exactly; there is nothing
// document-derived to tolerate. The to-CIP figures are printed, not estimated.
//
// ONE CELL IS RECONCILED BY NOTHING AND THE SUMMARY SAYS SO. p76's Permanent
// section publishes a zero in both budget years, and pp.66-67 print no
// Permanent column at all (fisc-u8o). Both sides read zero, so it ties -- but
// it ties because the spine is silent, not because two figures agree, and a
// reader counting cells would otherwise have no way to tell.
type transfersDetailTiesToSpine struct{}

var _ Check = (*transfersDetailTiesToSpine)(nil)

func (*transfersDetailTiesToSpine) ID() string { return "transfers-detail-ties-to-spine" }
func (*transfersDetailTiesToSpine) Tier() int  { return 1 }
func (*transfersDetailTiesToSpine) Full() bool { return false }
func (*transfersDetailTiesToSpine) Description() string {
	return "Budget Book p76's 22 transfers sum, per fund group, to exactly what the citywide " +
		"spine publishes as TRANSFER IN, and to TRANSFER OUT once the to-CIP figures " +
		"pp.72-75 print are added back"
}

func (*transfersDetailTiesToSpine) Run(_ context.Context, s *Subject) (Result, error) {
	// VACUOUS WHEN THE DETAIL SCOPE IS EMPTY. The same arm the other two lanes
	// carry, and for the same reason: internal/check's shared fixture has a
	// spine with no p76 behind it, so a literal "a spine key with no detail is
	// a failure" would redden every test calling runChecks over a corpus with
	// nothing wrong in it.
	any := false
	for i := range s.Facts {
		if s.Facts[i].Scope == transfersDetailScope {
			any = true
			break
		}
	}
	if !any {
		return Result{
			Status: StatusVacuous,
			Summary: fmt.Sprintf("no fact is in scope %q, so there is no transfer detail to "+
				"reconcile against the spine (Budget Book p76 is fisc-5gk.3.1)",
				transfersDetailScope),
			Findings: []Finding{},
		}, nil
	}

	var (
		subjects int
		findings []Finding
		pairs    = map[yearBasis]bool{}
		unmatch  = map[yearBasis]bool{}
		clauses  []string
	)
	for _, cl := range transfersClauses {
		detail := detailSums(s.Facts, transfersDetailScope, cl.restriction)
		spine := detailSums(s.Facts, spineScope, cl.restriction)
		if cl.collapse {
			detail, spine = collapseNonMajor(detail), collapseNonMajor(spine)
		}
		reconcile, unmatched := reconciledPairs(detail, spine)
		if len(cl.toCIPGroups) > 0 {
			detail = addToCIP(detail, cl.toCIPGroups, reconcile)
		}
		cmp := compareDetail(detail, spine, reconcile, transfersDetailScope, nil)

		subjects += cmp.subjects
		findings = append(findings, cmp.findings...)
		maps.Copy(pairs, reconcile)
		maps.Copy(unmatch, unmatched)
		clauses = append(clauses, fmt.Sprintf("%s %d", cl.name, cmp.subjects))
	}

	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pairs the spine publishes, in "+
		"three clauses (%s): p76's receiving legs equal the spine's TRANSFER IN to the cent, "+
		"and its paying legs equal TRANSFER OUT once pp.72-75's printed to-CIP figures are "+
		"added back -- capital, special-revenue and permanent jointly, because those pages "+
		"print one aggregate for all non-major funds. The permanent in-leg is a published zero "+
		"reconciled against no spine column at all (fisc-u8o), so it agrees without being "+
		"checked", subjects, len(pairs), joinComma(clauses))
	if len(unmatch) > 0 {
		held += fmt.Sprintf("; %d further pair(s) the detail publishes have no spine column "+
			"and are not reconciled: %s", len(unmatch), describePairs(unmatch))
	}

	return conclusion{
		subjects: subjects,
		unit:     "cells",
		held:     held,
		nothing: fmt.Sprintf("scope %q carries facts but the spine publishes no transfers at "+
			"all, so nothing could be reconciled", transfersDetailScope),
		findings: findings,
	}.result(), nil
}

// declaredToCIPGroups is the group list toCIP covers, for the test that holds
// the table to the clauses.
func declaredToCIPGroups() []string { return slices.Sorted(maps.Keys(toCIP)) }
