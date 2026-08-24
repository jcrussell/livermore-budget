package check

import (
	"fmt"
	"slices"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// This file is the machinery every <kind>-detail-ties-to-spine check shares. The
// checks themselves stay one per file, because what is worth reading about each
// of them is the argument for ITS schedule — which restriction, which exceptions,
// what the asymmetries are — and that argument does not generalise even though
// the arithmetic does.
//
// It was extracted at the second instance rather than the first. Budget Book
// pp.167-170 (expenditure-by-department, fisc-5gk.2) landed alone; pp.127-140
// (revenue-by-fund, fisc-5gk.1) is the second and p76 (transfers-by-fund,
// fisc-5gk.3.1) is already specified, so the shape is known rather than guessed.

// detailRestriction is the slice of the fact store one detail check compares.
//
// IT IS A FILTER AND NOT A KEY, and the distinction is the whole reason this
// type exists rather than a pair of loose constants. FundGroup pins which facts
// are looked at; it never decides which cells are compared, because detailKey
// always carries the fund group. Collapse the two and a check whose schedule
// spans every group — pp.127-140 does — would compare six groups' facts under
// one key and tie on a sum that happens to match.
//
// Neither field is cosmetic on either side of the pair that exists today.
// Without Kinds, a revenue schedule's keys meet the spine's expenditure and
// fund-balance cells and the check fails at a quarter of a billion dollars
// before reaching anything it is about. Without FundGroup, a General Fund
// schedule meets the other five groups — about $109M of FY2026 expenditure.
// Which of the two a schedule needs is a property of the schedule, so it is
// declared beside the check that reads it.
type detailRestriction struct {
	// Kinds are the fact kinds the schedule prints. Required: a restriction
	// admitting every kind compares a detail schedule against the whole spine.
	Kinds []mapping.Kind

	// FundGroup pins the check to one fund group, for a schedule that covers
	// one. Empty means the schedule spans them all, which is a claim about the
	// pages and not a default.
	FundGroup string
}

// admits says whether a fact is one this check compares.
func (r detailRestriction) admits(f *fact.Fact) bool {
	if r.FundGroup != "" && f.FundGroup != r.FundGroup {
		return false
	}
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

// detailKey is one cell both scopes must agree on.
//
// It is deliberately the tuple internal/project's netCells keys on, minus the
// slice: these checks are the negative of that function's collision, so they
// assert equality on precisely the tuple a doubling would have happened on.
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
// pinning the unprojectedScopes entry count.
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
	scope string) (subjects int, findings []Finding) {
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
					"decompose the spine, never extend it", d.cents, scope))
		default:
			findings = append(findings, finding(k.String(),
				"the detail sums to %s and the spine publishes %s, a difference of %s; "+
					"these are the same money decomposed two ways and must tie to the cent",
				d.cents, sp.cents, d.cents-sp.cents))
		}
	}
	return subjects, findings
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
