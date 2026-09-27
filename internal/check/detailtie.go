package check

import (
	"fmt"
	"slices"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// The spine-side sums revenue-lines-tie-to-their-categories compares against:
// cells keyed on (column, fund group, category) inside a kind restriction,
// compared as a union so a dropped row fails one-sided rather than vanishing.
// Re-pointing it at structure.KeyOf retires this file (fisc-6714).

// detailRestriction is the slice of the fact store one comparison reads: the
// kinds the schedule prints.
//
// IT IS A FILTER AND NOT A KEY: detailKey carries the fund group regardless,
// so a schedule spanning every group is compared group by group.
type detailRestriction struct {
	// Kinds are the fact kinds the schedule prints. Required: a restriction
	// admitting every kind compares a detail schedule against the whole spine.
	Kinds []mapping.Kind
}

// admits says whether a fact is one this comparison reads.
func (r detailRestriction) admits(f *fact.Fact) bool {
	return slices.Contains(r.Kinds, f.Kind)
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

// detailKey is one cell both sides must agree on, at the spine's own grain.
type detailKey struct {
	year      int
	basis     mapping.Basis
	fundGroup string
	category  string
}

// String names a cell the way a finding should: the slice, then the group, then
// the category.
func (k detailKey) String() string {
	return fmt.Sprintf("FY%d %s %s %s", k.year, k.basis, k.fundGroup, k.category)
}

type yearBasis struct {
	year  int
	basis mapping.Basis
}

// detailSums adds up one scope's facts inside the restriction.
func detailSums(facts []fact.Fact, scope string, r detailRestriction) map[detailKey]cellSum {
	out := map[detailKey]cellSum{}
	for i := range facts {
		f := &facts[i]
		if f.Scope != scope || !r.admits(f) {
			continue
		}
		k := detailKey{f.FiscalYear, f.Basis, f.FundGroup, f.Category}
		c := out[k]
		c.cents += amount.Cents(f.AmountCents)
		c.present = true
		out[k] = c
	}
	return out
}

// reconciledPairs splits the (fiscal year, basis) slices two scopes carry into
// the ones this check reconciles and the ones it cannot.
//
// THE SPINE DECIDES, and the detail does not get a say. fisc-u2v (3) says "each
// pair present in BOTH scopes", which reads naturally as an intersection, and an
// intersection lets the detail opt out of a column by dropping it. Measured on
// the department lane — spine carrying FY2026 and FY2027, detail carrying only
// FY2026 — an intersection passes with one cell checked and $149,014,579 of
// General Fund expenditure silently unreconciled.
//
// The asymmetry is real and runs one way only. A spine column with no detail
// behind it is a gap in the MAPPING, which these checks exist for. A detail
// column with no spine column is a gap in the DOCUMENT — pp.66-67 print no
// actual or revised column — and is returned separately so the summary can name
// it rather than leave a reader counting facts to guess.
//
// Both sets are computed rather than named: a hard-coded pair list would stop
// reconciling a column the day one is mapped, which is the same defect class as
// pinning a declaration's entry count.
func reconciledPairs(detail, spine map[detailKey]cellSum) (reconcile, unmatched map[yearBasis]bool) {
	reconcile = map[yearBasis]bool{}
	for k := range spine {
		reconcile[yearBasis{k.year, k.basis}] = true
	}
	unmatched = map[yearBasis]bool{}
	for k := range detail {
		if yb := (yearBasis{k.year, k.basis}); !reconcile[yb] {
			unmatched[yb] = true
		}
	}
	return reconcile, unmatched
}

// compareDetail is the comparison itself: every key either scope produced,
// inside the pairs the spine publishes, with the three ways a cell can disagree
// told apart because they need different fixes.
//
// It returns the number of cells examined, which is what the check reports as
// its subject count.
//
// THE UNION, NOT THE DETAIL'S KEYS. Iterating only the keys the DETAIL produces
// would make a dropped rule invisible: delete a whole block and its category
// vanishes from the detail side entirely, so a detail-keyed loop compares
// nothing and reports green. A spine key inside the restriction with no detail
// counterpart is a FAILURE.
func compareDetail(detail, spine map[detailKey]cellSum, reconcile map[yearBasis]bool,
	scope string, exempt func(detailKey) bool) detailComparison {
	var out detailComparison
	for _, k := range unionKeys(detail, spine) {
		if !reconcile[yearBasis{k.year, k.basis}] {
			continue
		}
		if exempt != nil && exempt(k) {
			out.exempt++
			continue
		}
		d, sp := detail[k], spine[k]
		out.subjects++
		if !d.present || !sp.present {
			out.oneSided++
		}
		if d.cents == sp.cents {
			continue
		}
		findings := out.findings
		switch {
		case !d.present:
			findings = append(findings, finding(k.String(),
				"the spine publishes %s here and the detail has no such row at all; a "+
					"category the schedule stopped printing is a rule that was dropped, "+
					"not a cell that is empty", sp.cents))
		case !sp.present:
			findings = append(findings, finding(k.String(),
				"the detail publishes %s here and the spine has no such cell; %q must "+
					"decompose the spine, never extend it", d.cents, scope))
		default:
			findings = append(findings, finding(k.String(),
				"the detail sums to %s and the spine publishes %s, a difference of %s; "+
					"these are the same money decomposed two ways and must tie to the cent",
				d.cents, sp.cents, d.cents-sp.cents))
		}
		out.findings = findings
	}
	return out
}

// detailComparison is what one pass over the union produced.
//
// oneSided is carried because it is the count a reader needs to judge the
// held line and cannot recover from it: a key only one scope produced ties
// when the other side is zero, which is a real agreement and not a vacancy —
// p67 prints a dash for taxes/property under capital and pp.131-140 print no
// such row, so both say zero for a documented reason. On the committed corpus
// the revenue lane compares 134 cells of which 60 are one-sided, against 0 value
// differences, so a summary that did not distinguish them would report 134
// "cells" of which nearly half are two zeroes agreeing. fisc-u2v (3) is explicit
// that this must NOT become a table of exceptions, and a count is the honest
// middle: loud, and not a list.
//
// exempt is separate again, and separate from subjects: an exempted cell was
// neither compared nor agreed at zero, it was handed to another check. Counting
// it as a subject would overstate what this one covers.
type detailComparison struct {
	subjects int
	oneSided int
	exempt   int
	findings []Finding
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
		if keys[i].fundGroup != keys[j].fundGroup {
			return keys[i].fundGroup < keys[j].fundGroup
		}
		return keys[i].category < keys[j].category
	})
	return slices.Clip(keys)
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
