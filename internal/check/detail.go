package check

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// expenditureDetailScope is the scope Budget Book pp.167-170 are mapped at, and
// the string internal/check and mappings/ have to agree on. It is spelled here
// rather than in both places because the whole hazard this check exists for is a
// scope string that two readers spell differently.
const expenditureDetailScope = "expenditure-by-department"

// The restriction. Both halves are load-bearing and neither is cosmetic.
//
// WITHOUT THE KIND HALF the union below meets the spine's revenue, transfer and
// fund-balance keys — 120, 24 and 48 facts on the committed corpus — against a
// schedule that prints none of them, and fails at a quarter of a billion dollars
// before reaching an expenditure.
//
// WITHOUT THE FUND-GROUP HALF it meets the other five fund groups' expenditure,
// about $109M in FY2026, because pp.167-170 are a GENERAL FUND schedule.
// fisc-u2v's revenue-by-fund needs only the kind half, because pp.127-140 cover
// all six fund groups; copying that restriction here would be wrong.
const (
	expenditureDetailKind      = mapping.KindExpenditure
	expenditureDetailFundGroup = "general"
)

// expenditureDetailTiesToSpine asserts Budget Book pp.167-170 reconcile against
// the citywide spine rather than adding to it.
//
// THE FAILURE THIS EXISTS FOR IS SILENT. The 49 object rows on those pages sum,
// per object category, to exactly what p66 prints for the General Fund. That
// exactness is what makes a mistake invisible: a rule written at
// `all-funds-gross` puts them in the spine's own cells, internal/project's
// netCells keys on (kind, category, fundGroup) and sums them, and the published
// General Fund expenditure doubles to $289,301,604 with every graph check green
// — because those checks tie the graph to the facts it was built from, not to
// the document.
//
// So the scope keeps them out of the chart and this check ties them to it.
// Together they are the two halves of one claim: the detail is the same money,
// decomposed differently.
//
// TIER 1, ZERO TOLERANCE. There is nothing document-derived to tolerate here.
// Every reconciled figure ties to the cent, including the two zero cells — p66
// prints "-" for General Fund capital outlay and debt services in both budget
// years and the spine publishes those as zero-valued facts, while pp.167-170
// print "-" in the same categories in the same years. Both sides agree at zero
// for a documented reason, which is a tie and not a vacancy.
//
// THE UNION, NOT THE DETAIL'S KEYS. Iterating only the keys the DETAIL produces
// would make a dropped rule invisible: delete the two General Services rules and
// the Patrol/Support/Special-Operations capital outlay rows and both
// `capital-outlay` and `debt-services` vanish from the detail side entirely, so
// a detail-keyed loop compares nothing. Both are zero in the budget years, which
// is worse rather than better — the hole would stay invisible until the city
// next budgets capital outlay. A spine key inside the restriction with no detail
// counterpart is a FAILURE.
//
// ZERO DECLARED EXCEPTIONS, and that is a property of this schedule rather than
// a general rule. fisc-u2v's revenue lane needs one, for the General Fund
// Transfers In row pp.127-130 do not print; if one is ever needed here it must
// name the scope that covers the key instead and fail if that scope carries no
// fact there (fisc-aes and fisc-brx, decided jointly). The corpus-wide exception
// budget across the three detail scopes is zero.
type expenditureDetailTiesToSpine struct{}

var _ Check = (*expenditureDetailTiesToSpine)(nil)

func (*expenditureDetailTiesToSpine) ID() string { return "expenditure-detail-ties-to-spine" }
func (*expenditureDetailTiesToSpine) Tier() int  { return 1 }
func (*expenditureDetailTiesToSpine) Full() bool { return false }
func (*expenditureDetailTiesToSpine) Description() string {
	return "Budget Book pp.167-170's department detail sums, per object category, to exactly " +
		"what the citywide spine publishes for General Fund expenditure"
}

// cellSum is one side of the comparison: a sum and whether the scope said
// anything at all about the key.
//
// Present is carried separately from a zero sum because they are different
// claims. "The detail prints this category and it is zero" ties against a spine
// zero; "the detail has no such key" is the dropped-rule case, and collapsing
// the two would make the union pointless.
type cellSum struct {
	cents   amount.Cents
	present bool
}

type detailKey struct {
	year     int
	basis    mapping.Basis
	category string
}

func (*expenditureDetailTiesToSpine) Run(_ context.Context, s *Subject) (Result, error) {
	detail := detailSums(s.Facts, expenditureDetailScope)
	spine := detailSums(s.Facts, spineScope)

	if len(detail) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: fmt.Sprintf("no fact is in scope %q, so there is no department detail to "+
				"reconcile against the spine (Budget Book pp.167-170 are fisc-5gk.2)",
				expenditureDetailScope),
			Findings: []Finding{},
		}, nil
	}

	// THE SPINE DECIDES WHICH (fiscal year, basis) PAIRS MUST BE RECONCILED,
	// and the detail does not get a say. This is fisc-u2v's DEFECT 2 one axis
	// over, and it is not what that bead's wording says: "each pair present in
	// BOTH scopes" reads naturally as an intersection, and an intersection lets
	// the detail opt out of a column by dropping it. Measured — spine carrying
	// FY2026 and FY2027, detail carrying only FY2026 — an intersection passes
	// with one cell checked and $149,014,579 of General Fund expenditure
	// silently unreconciled.
	//
	// The asymmetry is real and runs one way only. pp.66-67 print no actual or
	// revised column, so the detail's FY2024 and FY2025 figures have nothing to
	// tie to and are published unreconciled; that is a gap in the DOCUMENT. A
	// spine column with no detail behind it is a gap in the MAPPING, which is
	// this check's business.
	//
	// Both sets are computed rather than named: a hard-coded pair list would
	// stop reconciling a column the day one is mapped, which is the same defect
	// class as pinning the unprojectedScopes entry count.
	reconcile := map[yearBasis]bool{}
	for k := range spine {
		reconcile[yearBasis{k.year, k.basis}] = true
	}
	unmatched := map[yearBasis]bool{}
	for k := range detail {
		if yb := (yearBasis{k.year, k.basis}); !reconcile[yb] {
			unmatched[yb] = true
		}
	}

	var findings []Finding
	subjects := 0
	for _, k := range unionKeys(detail, spine) {
		if !reconcile[yearBasis{k.year, k.basis}] {
			continue
		}
		d, sp := detail[k], spine[k]
		subjects++
		if d.cents == sp.cents {
			continue
		}
		switch {
		case !d.present:
			findings = append(findings, finding(k.String(),
				"the spine publishes %s here and the detail has no such row at all; a "+
					"category the schedule stopped printing is a rule that was dropped, "+
					"not a cell that is empty", sp.cents))
		case !sp.present:
			findings = append(findings, finding(k.String(),
				"the detail publishes %s here and the spine has no such cell; %q must "+
					"decompose the spine, never extend it", d.cents, expenditureDetailScope))
		default:
			findings = append(findings, finding(k.String(),
				"the detail sums to %s and the spine publishes %s, a difference of %s; "+
					"these are the same money decomposed two ways and must tie to the cent",
				d.cents, sp.cents, d.cents-sp.cents))
		}
	}

	// The unreconciled columns are named in the summary rather than left out of
	// it. They are the detail's own published figures with no spine column to
	// tie to, and a reader counting facts would otherwise have no way to tell
	// this check covers 98 of the schedule's 196.
	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pairs the spine publishes, "+
		"each the sum of pp.167-170's object rows equal to the spine's own General Fund "+
		"expenditure to the cent", subjects, len(reconcile))
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d further pair(s) the detail publishes have no spine column "+
			"and are not reconciled: %s", len(unmatched), describePairs(unmatched))
	}
	return conclusion{
		subjects: subjects,
		unit:     "cells",
		held:     held,
		nothing: fmt.Sprintf("scope %q carries facts but the spine publishes no General Fund "+
			"expenditure at all, so nothing could be reconciled", expenditureDetailScope),
		findings: findings,
	}.result(), nil
}

type yearBasis struct {
	year  int
	basis mapping.Basis
}

// describePairs renders a set of slices in a stable order.
func describePairs(pairs map[yearBasis]bool) string {
	out := make([]string, 0, len(pairs))
	for yb := range pairs {
		out = append(out, fmt.Sprintf("FY%d %s", yb.year, yb.basis))
	}
	sort.Strings(out)
	return joinComma(out)
}

// String names a cell the way a finding should: the slice, then the category.
func (k detailKey) String() string {
	return fmt.Sprintf("FY%d %s %s", k.year, k.basis, k.category)
}

// detailSums adds up one scope's facts inside the restriction.
func detailSums(facts []fact.Fact, scope string) map[detailKey]cellSum {
	out := map[detailKey]cellSum{}
	for _, f := range facts {
		if f.Scope != scope ||
			f.Kind != expenditureDetailKind ||
			f.FundGroup != expenditureDetailFundGroup {
			continue
		}
		k := detailKey{f.FiscalYear, f.Basis, f.Category}
		c := out[k]
		c.cents += amount.Cents(f.AmountCents)
		c.present = true
		out[k] = c
	}
	return out
}

// unionKeys is every key either side produced, in a stable order so two runs
// report the same findings in the same sequence.
func unionKeys(a, b map[detailKey]cellSum) []detailKey {
	keys := make([]detailKey, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, dup := a[k]; !dup {
			keys = append(keys, k)
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
	return slices.Clip(keys)
}
