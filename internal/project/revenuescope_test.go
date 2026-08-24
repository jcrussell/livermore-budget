package project

import (
	"os"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// revenueScope is the scope Budget Book pp.127-140 are mapped at (fisc-u2v).
// Spelled here rather than imported: internal/check declares it unexported and
// this package is below it.
const revenueScope = "revenue-by-fund"

// What p66 prints for General Fund Property Taxes in FY2025-26, and what the
// chart would publish if pp.127-140 were mapped at the spine's scope. Both are
// named, because the wrong one is this bead's whole subject.
const (
	generalFundPropertyTaxes        = 6_414_376_200
	generalFundPropertyTaxesDoubled = 12_828_752_400
)

// p127PropertyTaxes is the thirteen printed detail lines under Property Taxes on
// Budget Book p127, FY2025-26, in the order the page prints them. The figures
// are the document's, so their sum is the document's too — 64,143,762, which is
// p0066.txt:13 to the cent.
//
// ERAF and RPTTF Reduction are the contra rows, ~26% of gross property tax
// booked as deductions. They are here rather than dropped because a Sankey
// cannot draw a negative link and it is exactly the netting of them into the
// category that makes the detail sum to the spine's one figure.
var p127PropertyTaxes = []struct {
	label string
	cents int64
}{
	{"Current Year - Secured", 6_206_747_000},
	{"Prior Year - Secured", 45_839_400},
	{"ERAF", -1_517_500_000},
	{"RPTTF Reduction", -181_033_900},
	{"Current Year - Unsecured", 238_330_600},
	{"Prior Year - Unsecured", 3_431_000},
	{"Supple - Sec Roll Current", 90_000_000},
	{"VLF Comp Fund", 802_930_600},
	{"Unitary Utility Tax", 62_500_000},
	{"Aircraft Taxes", 4_500_000},
	{"RPTTF Receipts & Other PropTax", 630_000_000},
	{"St Homeowner Prop Tax Re", 22_000_000},
	{"Pen & Int On Delinq Tax", 6_631_500},
}

// revenueDetailFacts renders p127's property-tax block at a chosen scope.
//
// Every fact carries a FUND, which is what the schedule's columns are, and it is
// the reason this lane has no second guard: netCells refuses a fact carrying a
// DEPARTMENT and has no such refusal for a fund, so these flow straight into the
// spine's own cells if the scope lets them.
func revenueDetailFacts(t *testing.T, scope string) []fact.Fact {
	t.Helper()
	out := make([]fact.Fact, 0, len(p127PropertyTaxes))
	for _, r := range p127PropertyTaxes {
		row := mapping.Row{Category: "taxes/property"}
		rowPath := fact.RowPath(row)
		columnPath := fact.ColumnPath(mapping.Column{FundGroup: "general", Fund: 100}, scope)
		sign := mapping.SignPositive
		if r.cents < 0 {
			sign = mapping.SignContra
		}
		out = append(out, fact.Fact{
			ID: fact.MakeID(testDoc, "gf-rev-property-taxes", rowPath, r.label, columnPath,
				testYear, testBasis),
			DocID:       testDoc,
			Page:        127,
			RuleID:      "gf-rev-property-taxes",
			Kind:        mapping.KindRevenue,
			Basis:       testBasis,
			Scope:       scope,
			FiscalYear:  testYear,
			RowPath:     rowPath,
			RowLabel:    r.label,
			Category:    "taxes/property",
			ColumnPath:  columnPath,
			FundGroup:   "general",
			Fund:        100,
			Sign:        sign,
			Units:       "dollars",
			AmountCents: r.cents,
		})
	}
	return out
}

// TestRevenueDetailDoesNotEnterTheSpine is the strongest form of the claim the
// scope makes: 924 facts join the store and the published document does not move
// by one byte.
//
// Byte-identity rather than one link, for the reason the department lane gives:
// asserting the one link catches the one link, while asserting the whole graph
// says nothing anywhere in the chart noticed, and fails on ten links rather than
// one if the guard breaks.
func TestRevenueDetailDoesNotEnterTheSpine(t *testing.T) {
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	all := append(spineFacts(t, testYear), revenueDetailFacts(t, revenueScope)...)

	got, err := (&Sankey{Labels: goldenLabels}).Build(all, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Build with %d revenue detail facts added differs from %s; the detail is "+
			"the same money as the spine and must not reach the chart",
			len(p127PropertyTaxes), goldenPath)
	}

	g := buildGraph(t, all, testOptions())
	l := linkBetween(t, g, "revenue/taxes/property", "fund-group/general")
	if l.ValueCents != generalFundPropertyTaxes {
		t.Errorf("General Fund property taxes = %d, want %d", l.ValueCents,
			generalFundPropertyTaxes)
	}
	if len(l.FactIDs) != 1 {
		t.Errorf("link cites %d facts, want 1 (the spine's); the thirteen detail lines "+
			"must not be summed into it", len(l.FactIDs))
	}
}

// TestRevenueDetailAtSpineScopeWouldDoubleTheGeneralFund pins the defect, which
// is this repository's house style for a figure a mistake would produce —
// transfers_p76_test.go pins the $38,086,737 residual, statedtotaldelta_test.go
// the +$1.
//
// AND IT IS A SHARPER STATEMENT HERE THAN ON THE DEPARTMENT LANE. There, a
// mis-scoped rule usually hits netCells' refusal of a department-bearing fact
// and stops loudly; only an author who put the division in the row label slips
// past. Here there is no refusal at all: netCells has no fund guard, so EVERY
// mis-scoped revenue rule builds a graph, doubled, silently. The scope string
// and revenue-detail-ties-to-spine are the whole of the guard.
func TestRevenueDetailAtSpineScopeWouldDoubleTheGeneralFund(t *testing.T) {
	all := append(spineFacts(t, testYear), revenueDetailFacts(t, testScope)...)

	g, err := (&Sankey{Labels: goldenLabels}).Graph(all, testOptions())
	if err != nil {
		t.Fatalf("Graph: %v; a fund-bearing fact at the spine's scope is ACCEPTED, which "+
			"is the point of this test", err)
	}
	l := linkBetween(t, g, "revenue/taxes/property", "fund-group/general")
	if l.ValueCents != generalFundPropertyTaxesDoubled {
		t.Fatalf("General Fund property taxes = %d, want %d; this test states the DEFECT, "+
			"so a change here means the doubling stopped happening and the scope guard's "+
			"reason needs re-checking", l.ValueCents, generalFundPropertyTaxesDoubled)
	}
	if l.ValueCents != 2*generalFundPropertyTaxes {
		t.Errorf("the doubled figure is not twice the spine's; the fixture drifted")
	}
	// And the doubling is INVISIBLE to the graph checks, which is why the
	// figure has to be pinned here rather than left to them: the link cites
	// fourteen facts and equals their sum exactly.
	if len(l.FactIDs) != 1+len(p127PropertyTaxes) {
		t.Errorf("link cites %d facts, want %d; the detail was summed into the spine cell "+
			"and link-values-tie-to-facts would report that as green",
			len(l.FactIDs), 1+len(p127PropertyTaxes))
	}
}
