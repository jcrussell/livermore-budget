package structure

import (
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// The cut names other packages select by.
const (
	CutSpine          = "spine"
	CutRevenueDetail  = "revenue-detail"
	CutDepartmentwide = "departmentwide"
	CutFundingSources = "funding-sources"
)

// BudgetBookCuts are the Budget Book's schedules as cuts of one hierarchy,
// each a claim about its pages.
func BudgetBookCuts() []Cut {
	// The detail schedules print an actual, a revised and two adopted columns;
	// pp.66-67 and p76 print the two adopted columns only.
	budgetBookDetail := []mapping.Basis{mapping.BasisActual, mapping.BasisRevised, mapping.BasisAdopted}
	return []Cut{
		{
			// pp.66-67, the citywide control totals every other Budget Book
			// schedule decomposes.
			Name:      CutSpine,
			Scope:     ScopeAllFundsGross,
			Level:     LevelFundGroupByCategory,
			Kinds:     everyKind,
			Reference: true,
			Bases:     []mapping.Basis{mapping.BasisAdopted},
		},
		{
			// pp.127-140, revenue and transfers in per fund.
			Name:  CutRevenueDetail,
			Scope: ScopeRevenueByFund,
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn},
			Bases: budgetBookDetail,
		},
		{
			// p76, both legs of every transfer, per fund.
			Name:  "transfers-detail",
			Scope: ScopeTransfersByFund,
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindTransferIn, mapping.KindTransferOut},
			Bases: []mapping.Basis{mapping.BasisAdopted},
		},
		{
			// p222, what each operating fund transfers to the CIP. Part of the
			// spine's TRANSFER OUT, with p76, under BudgetBookSplits.
			Name:  "cip-transfers-out",
			Scope: ScopeCIPFundingSources,
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindTransferOut},
			Bases: []mapping.Basis{mapping.BasisRevised, mapping.BasisAdopted},
		},
		{
			// p222, what each CIP fund receives: those transfers, its grants,
			// and the balance it draws.
			Name:  "cip-funds",
			Scope: ScopeCIPFundingSources,
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindTransferIn, mapping.KindRevenue, mapping.KindFundBalance},
			Bases: []mapping.Basis{mapping.BasisRevised, mapping.BasisAdopted},
			Outside: "p0204 prints the Capital Improvement Program Funds on a line of their own, " +
				"beside Total Operating Budget, and pp.66-67 total the operating budget alone",
		},
		{
			// pp.167-170, General Fund expenditure by department and object.
			// `general` is a footprint, not a placeholder.
			Name:           "general-fund-departments",
			Scope:          ScopeExpenditureByDepartment,
			Level:          LevelFundByDepartmentByCategory,
			Kinds:          []mapping.Kind{mapping.KindExpenditure},
			FundGroups:     []string{registry.FundTypeGeneral},
			DepartmentTier: TierDivision,
			Bases:          budgetBookDetail,
		},
		{
			// p172, the General Fund's expenditure by object: pp.167-170's
			// money a grain coarser, held equal by their containment.
			Name:       "general-fund-by-category",
			Scope:      ScopeGeneralFundByCategory,
			Level:      LevelFundByCategory,
			Kinds:      []mapping.Kind{mapping.KindExpenditure},
			FundGroups: []string{registry.FundTypeGeneral},
			Bases:      budgetBookDetail,
		},
		{
			// pp.173-183, expenditure by fund and object for every other
			// operating fund. The footprint is what lets fund-flows draw it
			// beside general-fund-departments.
			Name:       "fund-expenditures",
			Scope:      ScopeExpenditureByFund,
			Level:      LevelFundByCategory,
			Kinds:      []mapping.Kind{mapping.KindExpenditure},
			FundGroups: allFundTypesBut(registry.FundTypeGeneral),
			Bases:      budgetBookDetail,
		},
		{
			// pp.85-125, expenditure by department and object across every
			// fund, with no fund axis printed anywhere on the pages.
			Name:           CutDepartmentwide,
			Scope:          ScopeDepartmentwideExpenditures,
			Level:          LevelDepartmentByCategory,
			Kinds:          []mapping.Kind{mapping.KindExpenditure},
			DepartmentTier: TierDivision,
			Bases:          budgetBookDetail,
		},
		{
			// pp.85-125, which department's money comes from which fund. The
			// rows name departments where the other schedules name divisions.
			Name:           CutFundingSources,
			Scope:          ScopeDepartmentFundingSources,
			Level:          LevelFundByDepartment,
			Kinds:          []mapping.Kind{mapping.KindExpenditure},
			Placeholders:   []Axis{AxisCategory},
			DepartmentTier: TierDepartment,
			Bases:          budgetBookDetail,
		},
	}
}

// everyKind is the kind set of a schedule that prints all five.
var everyKind = []mapping.Kind{
	mapping.KindRevenue, mapping.KindExpenditure,
	mapping.KindTransferIn, mapping.KindTransferOut, mapping.KindFundBalance,
}

// ACFRCuts are the ACFR's schedules as cuts of the same hierarchy. Their
// audited bases are what keep them from comparing against the Budget Book, and
// p167's one scope prints two grains, so it is two cuts selected by rule.
func ACFRCuts() []Cut {
	audited := []mapping.Basis{mapping.BasisAudited}
	return []Cut{
		{
			// p41, the General Fund's revenues, expenditures, transfers and
			// fund balance for one audited year, at the spine's own grain.
			Name:       "acfr-general-fund-summary",
			Scope:      ScopeACFRGeneralFundSummary,
			Level:      LevelFundGroupByCategory,
			Kinds:      everyKind,
			FundGroups: []string{registry.FundTypeGeneral},
			Bases:      audited,
		},
		{
			// pp.168-169, ten audited years of all governmental funds combined,
			// with no fund group printed anywhere.
			Name:  "acfr-changes-in-fund-balances",
			Scope: ScopeACFRChangesInFundBalances,
			Level: LevelCategory,
			Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindExpenditure, mapping.KindFundBalance},
			Bases: audited,
		},
		{
			// p167, the General Fund's GASB 54 components with the group named.
			Name:       "acfr-fund-balances/general",
			Scope:      ScopeACFRFundBalances,
			Rules:      []string{"acfr-p0167-general-fund-balances"},
			Level:      LevelFundGroupByCategory,
			Kinds:      []mapping.Kind{mapping.KindFundBalance},
			FundGroups: []string{registry.FundTypeGeneral},
			Bases:      audited,
		},
		{
			// p167, every other governmental fund's components, aggregated.
			Name:  "acfr-fund-balances/other-governmental",
			Scope: ScopeACFRFundBalances,
			Rules: []string{"acfr-p0167-other-governmental-fund-balances"},
			Level: LevelCategory,
			Kinds: []mapping.Kind{mapping.KindFundBalance},
			Bases: audited,
		},
	}
}

// AllCuts is every declared cut, Budget Book and ACFR.
func AllCuts() []Cut {
	return append(BudgetBookCuts(), ACFRCuts()...)
}

// BudgetBookIdentities are the same-figure relations between the Budget Book's
// peers. Both readings of a transfer in stay, each with its own locator chain;
// the identity is what lets a view take one and not the other by name.
func BudgetBookIdentities() []Identity {
	return []Identity{{
		Name:  "a-transfer-in-is-printed-at-both-ends",
		A:     CutRevenueDetail,
		B:     "transfers-detail",
		Kinds: []mapping.Kind{mapping.KindTransferIn},
		Reason: "pp.127-140 print a fund's Transfers In at the receiving fund and p76 prints the " +
			"same movements at the paying end; one figure, two schedules, two provenance chains",
	}}
}

// BudgetBookTies are the Budget Book's same-money relations the lattice cannot
// reach, held in every column both sides print.
func BudgetBookTies() []Tie {
	return []Tie{
		{Name: "a-department-spends-what-funds-it", A: CutDepartmentwide, B: CutFundingSources, At: LevelDepartment},
		{Name: "the-general-fund-row-is-the-general-fund-schedule", A: "general-fund-departments",
			B: CutFundingSources, At: LevelDepartment},
	}
}

func init() {
	if err := ValidateIdentities(AllCuts(), BudgetBookIdentities()); err != nil {
		panic("internal/structure: " + err.Error())
	}
	if err := ValidateSplits(AllCuts(), BudgetBookSplits()); err != nil {
		panic("internal/structure: " + err.Error())
	}
}

// BudgetBookExceptions are the cells of the Budget Book's comparisons that do
// not tie, each pinned on both sides and to the printed figure the two differ
// by. None is a tolerance; each carries its own argument in Reason and Printed.
//
// p0067's 250,000 is held apart once per schedule that decomposes its cell:
// pp.85-125 by fund group and by object, which neither locates it alone, and
// pp.172-183 by both at once. fisc-av0w keeps p0067's figure as printed.
func BudgetBookExceptions() []Exception {
	general := map[Axis]string{AxisFundGroup: registry.FundTypeGeneral, AxisCategory: "transfers/in"}
	present := func(c int64) Sum { return Sum{Cents: c, Present: true} }
	absent := Sum{}
	rounded := func(cut, dept string, c, a int64, printed string) Exception {
		return Exception{
			Name: cut + "-rounds-" + dept + "-2024", Cut: cut, Against: CutFundingSources, At: LevelDepartment,
			Cells: []Pin{{Year: 2024, Basis: "actual", Coords: map[Axis]string{AxisDepartment: dept},
				Cut: present(c), Against: present(a)}},
			Residual: a - c,
			Printed:  printed + "; each side's rows miss it by the dollars their rules' stated_total_deltas declare",
			Reason:   "the two schedules print one total and round the rows under it differently in the FY2023-24 Actual column",
			Bead:     "fisc-2sd",
		}
	}
	return []Exception{
		rounded(CutDepartmentwide, "administrative-services", 1278595300, 1278595400,
			"p0097.txt:43 and :51, both 12,785,955"),
		rounded(CutDepartmentwide, "innovation-and-economic-development", 568058900, 568059000,
			"p0111.txt:27 and :37, both 5,680,590"),
		rounded(CutDepartmentwide, "library-department", 658380900, 658380800,
			"p0115.txt:19 and :31, both 6,583,809"),
		rounded(CutDepartmentwide, "police-department", 4346324000, 4346324100,
			"p0119.txt:47 and p0120.txt:13, both 43,463,240"),
		func() Exception {
			e := rounded(CutDepartmentwide, "public-works", 6258673700, 6285353700,
				"p0124.txt:45 and p0125.txt:33, both 62,853,536, and p0124.txt:25 Transfers Out 266,798")
			e.Reason = "the departmentwide cut leaves out Maintenance's Transfers Out, a declared residue the " +
				"funding sources include, and the two schedules round the rows under one total $2 apart"
			return e
		}(),
		rounded("general-fund-departments", "administrative-services", 631156300, 631156400,
			"p0168.txt:35 ADMINISTRATIVE SERVICES TOTAL and p0097.txt:47 General Fund, both 6,311,564"),
		rounded("general-fund-departments", "community-development", 1592517100, 1592517000,
			"p0169.txt:45 COMMUNITY DEVELOPMENT TOTAL and p0101.txt:51 General Fund, both 15,925,170"),
		{
			Name: "pp.127-130-print-no-general-fund-transfer-in-2026",
			Cut:  CutRevenueDetail, Against: CutSpine, At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2026, Basis: "adopted", Coords: general, Cut: absent, Against: present(48040000)}},
			Residual: 48040000,
			Printed:  "p0066.txt:24, TRANSFER IN 480,400 for the General Fund; pp.127-130 print TOTAL REVENUES and no Transfers In row",
			Reason: "pp.127-130 print TOTAL REVENUES for the General Fund and no Transfers In row at all, so " +
				"the spine's General Fund transfer in has no counterpart in this schedule; p76 prints the " +
				"same movement, and transfers-detail carries it",
			Bead: "fisc-5gk.3.1",
		},
		{
			Name: "pp.127-130-print-no-general-fund-transfer-in-2027",
			Cut:  CutRevenueDetail, Against: CutSpine, At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2027, Basis: "adopted", Coords: general, Cut: absent, Against: present(48673500)}},
			Residual: 48673500,
			Printed:  "p0066.txt:24, TRANSFER IN 486,735 for the General Fund; pp.127-130 print TOTAL REVENUES and no Transfers In row",
			Reason: "pp.127-130 print TOTAL REVENUES for the General Fund and no Transfers In row at all, so " +
				"the spine's General Fund transfer in has no counterpart in this schedule; p76 prints the " +
				"same movement, and transfers-detail carries it",
			Bead: "fisc-5gk.3.1",
		},
		{
			Name: "p0067-internal-service-is-250000-high-by-fund-group",
			Cut:  CutFundingSources, Against: CutSpine, At: LevelFundGroup,
			Cells: []Pin{{Year: 2027, Basis: "adopted", Coords: map[Axis]string{AxisFundGroup: registry.FundTypeInternalService},
				Cut: present(2629451500), Against: present(2654451500)}},
			Residual: 25000000,
			Printed: "both sides: p0067.txt:34 prints TOTAL EXPENDITURES 26,544,515 for the Internal Service Funds, " +
				"and 26,294,515 is printed on p0183.txt:64, p0075.txt:53, p0205 and p0209",
			Reason: "p0067's Internal Service Funds column prints Services & Supplies of 16,796,010 for " +
				"FY2026-27, and the five internal service funds' own rows on pp.172-183 sum to 250,000 less. " +
				"The store publishes p0067 as printed",
			Bead: "fisc-av0w",
		},
		{
			Name: "general-fund-departments-rounds-services-and-supplies-2024",
			Cut:  "general-fund-departments", Against: "general-fund-by-category", At: LevelFundByCategory,
			Cells: []Pin{{Year: 2024, Basis: "actual",
				Coords: map[Axis]string{AxisFundGroup: registry.FundTypeGeneral, AxisFund: "100", AxisCategory: "services-and-supplies"},
				Cut:    present(5445217000), Against: present(5445217100)}},
			Residual: 100,
			Printed: "p0172.txt:16 Services & Supplies 54,452,171; p0170.txt:23 and p0172.txt:22 both total " +
				"123,228,190, and each side's rows miss it by the dollars their rules' stated_total_deltas declare",
			Reason: "the two schedules print one General Fund total and round the rows under it differently " +
				"in the FY2023-24 Actual column",
			Bead: "fisc-2sd",
		},
		{
			Name: "p0067-internal-service-is-250000-high-by-fund",
			Cut:  "fund-expenditures", Against: CutSpine, At: LevelFundGroupByCategory,
			Cells: []Pin{{Year: 2027, Basis: "adopted",
				Coords: map[Axis]string{AxisFundGroup: registry.FundTypeInternalService, AxisCategory: "services-and-supplies"},
				Cut:    present(1654601000), Against: present(1679601000)}},
			Residual:       25000000,
			SameResidualAs: "p0067-internal-service-is-250000-high-by-fund-group",
			Printed: "p0067.txt:31 prints 16,796,010; the five Services & Supplies rows of p0183.txt:16-60 " +
				"sum to 16,546,010, which is arithmetic, and the entry is grounded by differing by exactly " +
				"what the fund-group cut holds apart",
			Reason: "p0067's Internal Service Funds column prints Services & Supplies of 16,796,010 for " +
				"FY2026-27, and the five internal service funds' own rows on p183 sum to 250,000 less. " +
				"The store publishes p0067 as printed",
			Bead: "fisc-av0w",
		},
		{
			Name: "p0067-internal-service-is-250000-high-by-object",
			Cut:  CutDepartmentwide, Against: CutSpine, At: LevelCategory,
			Cells: []Pin{{Year: 2027, Basis: "adopted", Coords: map[Axis]string{AxisCategory: "services-and-supplies"},
				Cut: present(13025208700), Against: present(13050208700)}},
			Residual:       25000000,
			SameResidualAs: "p0067-internal-service-is-250000-high-by-fund-group",
			Printed: "neither side, nor the difference: this corpus prints no citywide object-category total, so " +
				"both sums are arithmetic and the entry is grounded by differing by exactly what the fund-group " +
				"cut, whose figures are printed, holds apart",
			Reason: "Budget Book pp.85-125 print this spending by department, division and object with no " +
				"fund at all, and they differ from p.67 in this object category by $250,000 and in no " +
				"other. The chart draws p.67 as printed",
			Bead: "fisc-av0w",
		},
	}
}

// allFundTypesBut is every fund type data/funds.yaml may declare but one: a
// cut covering the city's other funds covers a type the registry adds.
func allFundTypesBut(except string) []string {
	var out []string
	for _, t := range registry.FundTypes() {
		if t != except {
			out = append(out, t)
		}
	}
	return out
}
