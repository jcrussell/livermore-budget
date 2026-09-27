package project

import (
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// detailScope is the scope Budget Book pp.167-170 are mapped at (fisc-brx).
const detailScope = "expenditure-by-department"

// generalFundWages is what p66 prints for General Fund Wages & Benefits in
// FY2025-26, and the spine fixture carries the same figure. Doubling it is what
// this scope exists to prevent, so both numbers are named.
const (
	generalFundWages        = 8_180_101_100
	generalFundWagesDoubled = 16_360_202_200
)

// departmentWages is the pp.167-170 decomposition of that one cell: the 23
// divisions' Wages & Benefits rows, summing to exactly what the spine publishes.
//
// Only the divisions with a wages line are here — Fire Administration prints
// none, because the fire service is contracted — and the figures are the
// document's, so the sum is the document's too.
var departmentWages = []struct {
	division string
	cents    int64
}{
	{"city-council", 7_470_100},
	{"city-manager", 278_908_700},
	{"city-clerk", 95_074_300},
	{"city-attorney", 212_067_100},
	{"library", 575_493_400},
	{"innovation-and-economic-devel", 123_350_200},
	{"general-services", 0},
	{"administrative-services", 83_488_200},
	{"finance", 247_241_800},
	{"human-resources", 180_818_400},
	{"police-administration", 556_082_900},
	{"patrol", 2_356_083_700},
	{"support-services", 557_137_100},
	{"special-operations", 1_012_246_900},
	{"community-development-admin", 125_582_800},
	{"building-and-safety", 344_023_300},
	{"engineering", 488_448_400},
	{"housing-and-human-services", 141_709_600},
	{"planning", 340_968_400},
	{"public-works-administration", 100_487_100},
	{"maintenance", 345_198_700},
	{"environmental-services", 8_220_000},
}

// detailFacts renders the department decomposition at a chosen scope, with or
// without the department field, so a test can state which of the two guards it
// is about.
func detailFacts(t *testing.T, scope string, withDepartment bool) []fact.Fact {
	t.Helper()
	out := make([]fact.Fact, 0, len(departmentWages))
	for _, d := range departmentWages {
		row := mapping.Row{Category: "wages-and-benefits"}
		label := "Wages & Benefits"
		if withDepartment {
			row.Department = d.division
		} else {
			// The author who writes the division into the LABEL instead. The
			// facts are otherwise identical and netCells never sees a
			// department.
			label = d.division + " Wages & Benefits"
		}
		rowPath := fact.RowPath(row)
		columnPath := fact.ColumnPath(mapping.Column{FundGroup: "general"}, scope)
		out = append(out, fact.Fact{
			ID: fact.MakeID(testDoc, "div-"+d.division, rowPath, label, columnPath,
				testYear, testBasis),
			DocID:       testDoc,
			Page:        167,
			RuleID:      "div-" + d.division,
			Kind:        mapping.KindExpenditure,
			Basis:       testBasis,
			Scope:       scope,
			FiscalYear:  testYear,
			RowPath:     rowPath,
			RowLabel:    label,
			Category:    "wages-and-benefits",
			Department:  row.Department,
			ColumnPath:  columnPath,
			FundGroup:   "general",
			Sign:        mapping.SignPositive,
			Units:       "dollars",
			AmountCents: d.cents,
		})
	}
	return out
}

// TestDepartmentDetailDoesNotEnterTheSpine is the strongest form of the claim
// the scope exists to make: 196 facts join the store and the published document
// does not move by one byte.
//
// The byte-identity clause is what makes it strong. Asserting the one link would
// catch the one link; asserting the whole graph says that nothing anywhere in
// the chart noticed, and it fails on ten links rather than on one if the scope
// guard breaks.
func TestDepartmentDetailDoesNotEnterTheSpine(t *testing.T) {
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	all := append(withCIPLeg(t, spineFacts(t, testYear)), detailFacts(t, detailScope, true)...)

	got, err := (&sankey{Labels: goldenLabels}).Build(all, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Build with %d department facts added differs from %s; the detail is the "+
			"same money as the spine and must not reach the chart",
			len(departmentWages), goldenPath)
	}

	// And the one cell they would have landed on, named: still the spine's
	// figure, still citing exactly the one spine fact.
	g := buildGraph(t, all, testOptions())
	l := linkBetween(t, g, "fund-group/general", "expenditure/wages-and-benefits")
	if l.ValueCents != generalFundWages {
		t.Errorf("General Fund wages = %d, want %d", l.ValueCents, generalFundWages)
	}
	if len(l.FactIDs) != 1 {
		t.Errorf("link cites %d facts, want 1 (the spine's); the detail must not be "+
			"summed into it", len(l.FactIDs))
	}
}

// TestDetailAtSpineScopeErrorsOutOfNetCells covers the second guard, and it is
// the loud one.
//
// A rule author who writes `scope: all-funds-gross` on a department-bearing rule
// does NOT get a doubled General Fund with every check green: netCells refuses
// the fact and the build stops. fisc-brx measured this and decided the refusal
// stays; the only thing that changed is that verify reports it instead of
// exiting 1 with no report.
func TestDetailAtSpineScopeErrorsOutOfNetCells(t *testing.T) {
	all := append(spineFacts(t, testYear), detailFacts(t, testScope, true)...)

	_, err := (&sankey{Labels: goldenLabels}).Graph(all, testOptions())
	if err == nil {
		t.Fatal("a department-bearing fact at the spine's scope built a graph; the spine " +
			"has no department tier and must refuse one")
	}
	if !strings.Contains(err.Error(), "department") {
		t.Errorf("error = %v, want it to name the department the fact carries", err)
	}
}

// TestTheDepartmentlessVariantDoublesTheGeneralFund is why the scope is required
// even though netCells refuses a department.
//
// internal/mapping requires a row to carry a category OR a department, not both,
// so an author may write the object rows with `category:` alone and put the
// division in the row LABEL. Those facts carry no department, netCells never
// sees one, and the graph builds — doubled, silently. The department field is a
// guard only for authors who use it; the scope is the guard that does not depend
// on that choice.
//
// The wrong number is pinned by name, this repository's house style for a figure
// a mistake would produce.
func TestTheDepartmentlessVariantDoublesTheGeneralFund(t *testing.T) {
	all := append(spineFacts(t, testYear), detailFacts(t, testScope, false)...)

	g, err := (&sankey{Labels: goldenLabels}).Graph(all, testOptions())
	if err != nil {
		t.Fatalf("Graph: %v; the departmentless variant is accepted, which is the point", err)
	}
	l := linkBetween(t, g, "fund-group/general", "expenditure/wages-and-benefits")
	if l.ValueCents != generalFundWagesDoubled {
		t.Fatalf("General Fund wages = %d, want %d; this test states the DEFECT, so a "+
			"change here means the doubling stopped happening and the scope guard's "+
			"reason needs re-checking", l.ValueCents, generalFundWagesDoubled)
	}
	if l.ValueCents != 2*generalFundWages {
		t.Errorf("the doubled figure is not twice the spine's; the fixture drifted")
	}
}
