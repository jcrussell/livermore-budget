package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// expenditureDetailScope is the scope Budget Book pp.167-170 are mapped at, and
// the string internal/check and mappings/ have to agree on. It is spelled here
// rather than in both places because the whole hazard this check exists for is a
// scope string that two readers spell differently.
const expenditureDetailScope = "expenditure-by-department"

// The restriction. Both halves are load-bearing and neither is cosmetic.
//
// WITHOUT THE KIND HALF the union meets the spine's revenue, transfer and
// fund-balance keys, against a schedule that prints none of them, and fails at a
// quarter of a billion dollars before reaching an expenditure.
//
// WITHOUT THE FUND-GROUP HALF it meets the other five fund groups' expenditure,
// about $109M in FY2026, because pp.167-170 are a GENERAL FUND schedule.
// fisc-u2v's revenue-by-fund needs only the kind half, because pp.127-140 cover
// all six fund groups; copying this restriction there would be wrong.
//
// Note which field does what: FundGroups FILTERS, and detailKey carries the fund
// group regardless. Here every key comes out "general" by construction, which is
// why the distinction is invisible in this check's findings and load-bearing in
// the next one's.
var expenditureDetailRestriction = detailRestriction{
	Kinds:      []mapping.Kind{mapping.KindExpenditure},
	FundGroups: []string{"general"},
}

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
// THE UNION, NOT THE DETAIL'S KEYS — see compareDetail. Deleting the two General
// Services rules and the Patrol/Support/Special-Operations capital outlay rows
// makes both `capital-outlay` and `debt-services` vanish from the detail side
// entirely, so a detail-keyed loop would compare nothing. Both are zero in the
// budget years, which is worse rather than better: the hole would stay invisible
// until the city next budgets capital outlay.
//
// ZERO DECLARED EXCEPTIONS, and that is a property of this schedule rather than
// a general rule. fisc-u2v's revenue lane needs one, for the General Fund
// Transfers In row pp.127-130 do not print; if one is ever needed here it must
// name the scope that covers the key instead and fail if that scope carries no
// fact there, or say in words that the cell is unreconciled (fisc-aes and
// fisc-brx, decided jointly). The corpus-wide exception budget across the three
// detail scopes is one, and it is not this schedule's.
type expenditureDetailTiesToSpine struct{}

var _ Check = (*expenditureDetailTiesToSpine)(nil)

func (*expenditureDetailTiesToSpine) ID() string { return "expenditure-detail-ties-to-spine" }
func (*expenditureDetailTiesToSpine) Tier() int  { return 1 }
func (*expenditureDetailTiesToSpine) Full() bool { return false }
func (*expenditureDetailTiesToSpine) Description() string {
	return "Budget Book pp.167-170's department detail sums, per object category, to exactly " +
		"what the citywide spine publishes for General Fund expenditure"
}

func (*expenditureDetailTiesToSpine) Run(_ context.Context, s *Subject) (Result, error) {
	detail := detailSums(s.Facts, expenditureDetailScope, expenditureDetailRestriction)
	spine := detailSums(s.Facts, spineScope, expenditureDetailRestriction)

	// VACUOUS WHEN THE DETAIL SCOPE IS EMPTY, and this arm is why the check can
	// be landed at all rather than a convenience. internal/check's shared
	// fixture carries a spine expenditure cell with no detail counterpart, so
	// under a literal "a spine key with no detail is a failure" every test
	// calling runChecks would go red over a corpus with nothing wrong in it.
	// No detail at all is a schedule nobody has mapped yet; partial detail is
	// measured against the whole spine.
	if len(detail) == 0 {
		return Result{
			Status: StatusVacuous,
			Summary: fmt.Sprintf("no fact is in scope %q, so there is no department detail to "+
				"reconcile against the spine (Budget Book pp.167-170 are fisc-5gk.2)",
				expenditureDetailScope),
			Findings: []Finding{},
		}, nil
	}

	reconcile, unmatched := reconciledPairs(detail, spine)
	// No exemptions: pp.167-170 print every category p66 does. The nil is the
	// declaration, and fisc-brx is where the argument for it lives.
	cmp := compareDetail(detail, spine, reconcile, expenditureDetailScope, nil)

	// The unreconciled columns are named in the summary rather than left out of
	// it. They are the detail's own published figures with no spine column to
	// tie to, and a reader counting facts would otherwise have no way to tell
	// this check covers 98 of the schedule's 196.
	held := fmt.Sprintf("%d cells over %d (fiscal year, basis) pairs the spine publishes, "+
		"each the sum of pp.167-170's object rows equal to the spine's own General Fund "+
		"expenditure to the cent", cmp.subjects, len(reconcile))
	if len(unmatched) > 0 {
		held += fmt.Sprintf("; %d further pair(s) the detail publishes have no spine column "+
			"and are not reconciled: %s", len(unmatched), describePairs(unmatched))
	}
	return conclusion{
		subjects: cmp.subjects,
		unit:     "cells",
		held:     held,
		nothing: fmt.Sprintf("scope %q carries facts but the spine publishes no General Fund "+
			"expenditure at all, so nothing could be reconciled", expenditureDetailScope),
		findings: cmp.findings,
	}.result(), nil
}
