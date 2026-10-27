package project

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// detailScope is the scope Budget Book pp.167-170 are mapped at.
const detailScope = "expenditure-by-department"

// generalFundWages is what p66 prints for General Fund Wages & Benefits in
// FY2025-26, and the spine fixture carries the same figure. Doubling it is what
// this scope exists to prevent, so both numbers are named.
const (
	generalFundWages        = 8_180_101_100
	generalFundWagesDoubled = 16_360_202_200
)

// departmentWages is the pp.167-170 decomposition of that cell: the divisions'
// Wages & Benefits rows, summing to what the spine publishes.
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
		f := printedFact(testDoc, "div-"+d.division, row, label, mapping.Column{FundGroup: "general"},
			scope, vocab.KindExpenditure, testColumn, d.cents)
		f.Page = 167
		out = append(out, f)
	}
	return out
}

// TestDepartmentDetailDoesNotEnterTheSpine: department detail joins the store
// and the published spine document does not move by one byte.
func TestDepartmentDetailDoesNotEnterTheSpine(t *testing.T) {
	want := spineBytes(t)
	all := append(withCIPLeg(t, spineFacts(t, testYear)), detailFacts(t, detailScope, true)...)

	got, err := (&sankey{Labels: goldenLabels}).Build(all, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Build with %d department facts added differs from the spine alone; the detail is the "+
			"same money as the spine and must not reach the chart",
			len(departmentWages))
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

// TestDetailAtSpineScopeErrorsOutOfNetCells: a department-bearing rule at
// `scope: all-funds-gross` is refused by netCells and verify reports it.
func TestDetailAtSpineScopeErrorsOutOfNetCells(t *testing.T) {
	all := append(spineFacts(t, testYear), detailFacts(t, testScope, true)...)

	_, err := (&sankey{Labels: goldenLabels}).Document(all, testOptions())
	if err == nil {
		t.Fatal("a department-bearing fact at the spine's scope built a graph; the spine " +
			"has no department tier and must refuse one")
	}
	if !strings.Contains(err.Error(), "department") {
		t.Errorf("error = %v, want it to name the department the fact carries", err)
	}
}

// TestTheDepartmentlessVariantDoublesTheGeneralFund: object rows written with
// `category:` alone carry no department, so only the scope stops a doubled
// General Fund; the wrong number is pinned by name.
func TestTheDepartmentlessVariantDoublesTheGeneralFund(t *testing.T) {
	all := append(spineFacts(t, testYear), detailFacts(t, testScope, false)...)

	g, err := (&sankey{Labels: goldenLabels}).Document(all, testOptions())
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
