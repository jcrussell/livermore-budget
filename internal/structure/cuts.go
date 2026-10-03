package structure

import (
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// The cut names other packages select by.
const (
	CutSpine          = "spine"
	CutRevenueDetail  = "revenue-detail"
	CutDepartmentwide = "departmentwide"
	CutFundingSources = "funding-sources"

	CutFundBalanceFlows    = "fund-balance-flows"
	CutFundBalanceRevenues = "fund-balance-revenues"
	CutFundBalanceExpenses = "fund-balance-expenses"
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
			// pp.186-209, each fund's transfers and balances. The Capital
			// Improvement Program Funds block is p222's money, and residue.
			//
			// The pages print each fund's balances and Reserve
			// Increase/(Use) and no change line, so the spine's CHANGE IN
			// WORKING CAPITAL is not compared here. It is held through
			// fund-balance-identity on pp.66-67 (beginning + change = ending)
			// and this cut's beginning and ending, which are.
			Name:  CutFundBalanceFlows,
			Scope: ScopeFundBalancesByFund,
			Rules: fundBalancesRules(false),
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindTransferIn, mapping.KindTransferOut, mapping.KindFundBalance},
			Lines: []Line{
				{mapping.KindTransferIn, "transfers/in"},
				{mapping.KindTransferOut, "transfers/out"},
				{mapping.KindTransferOut, "transfers/out-to-cip"},
				LineBeginning,
				{mapping.KindFundBalance, CategoryFundBalanceReserveIncrease},
				LineEnding,
			},
			Unprinted: []Line{LineChange},
			Bases:     budgetBookDetail,
		},
		{
			// pp.186-209, each fund's Revenues, printed whole: a fund's
			// total, which every per-fund revenue schedule decomposes. Two
			// cuts and not one because a placeholder carries one value.
			Name:         CutFundBalanceRevenues,
			Scope:        ScopeFundBalancesByFund,
			Rules:        fundBalancesRules(false),
			Level:        LevelFund,
			Kinds:        []mapping.Kind{mapping.KindRevenue},
			Placeholders: []Axis{AxisCategory},
			Bases:        budgetBookDetail,
		},
		{
			// pp.186-209, each fund's Expenses, printed whole.
			Name:         CutFundBalanceExpenses,
			Scope:        ScopeFundBalancesByFund,
			Rules:        fundBalancesRules(false),
			Level:        LevelFund,
			Kinds:        []mapping.Kind{mapping.KindExpenditure},
			Placeholders: []Axis{AxisCategory},
			Bases:        budgetBookDetail,
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
	}, {
		Name:  "a-fund-balance-transfer-in-is-the-revenue-schedules",
		A:     CutFundBalanceFlows,
		B:     CutRevenueDetail,
		Kinds: []mapping.Kind{mapping.KindTransferIn},
		Reason: "pp.186-209 print each fund's Transfers In in a column of its sources, and " +
			"pp.127-140 print the same figure as a row of the fund's revenue section",
	}, {
		Name:  "a-fund-balance-transfer-is-p76s",
		A:     CutFundBalanceFlows,
		B:     "transfers-detail",
		Kinds: []mapping.Kind{mapping.KindTransferIn, mapping.KindTransferOut},
		Categories: []KindCategory{
			{mapping.KindTransferIn, "transfers/in"}, {mapping.KindTransferOut, "transfers/out"}},
		Reason: "pp.198-209 print each fund's Transfers In and Transfers Out as columns, and " +
			"p76 lists the same movements one by one, at both ends",
	}, {
		Name:       "a-fund-balance-transfer-to-the-cip-is-p222s",
		A:          CutFundBalanceFlows,
		B:          "cip-transfers-out",
		Kinds:      []mapping.Kind{mapping.KindTransferOut},
		Categories: []KindCategory{{mapping.KindTransferOut, "transfers/out-to-cip"}},
		Reason: "pp.192-209 print each fund's Transfers Out to CIP as a column, and p222 " +
			"prints the same figure as the fund's row of the CIP's funding sources",
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
	return append([]Exception{
		printsNoGFTransferInP76(2026, 48040000, 48040000, "p0076.txt:19, :23, :25 and :27, the four transfers "+
			"to General Fund, 19,250 + 250,000 + 77,250 + 133,900 = 480,400"),
		printsNoGFTransferInP76(2027, 48673500, 48673500, "p0076.txt:19, :23, :25 and :27, the four transfers "+
			"to General Fund, 19,250 + 250,000 + 79,568 + 137,917 = 486,735"),
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
	}, fundBalanceExceptions()...)
}

// fundBalancesRules is pp.186-209's rules: the Capital Improvement Program
// Funds block's four when cip is true, and every other when it is false.
func fundBalancesRules(cip bool) []string {
	var out []string
	for i, year := range []int{2024, 2025, 2026, 2027} {
		first := 186 + 6*i
		if cip {
			out = append(out, fmt.Sprintf("fund-balances-fy%d-p%04d", year, first+4))
			continue
		}
		out = append(out,
			fmt.Sprintf("fund-balances-fy%d-p%04d", year, first),
			fmt.Sprintf("fund-balances-fy%d-p%04d", year, first+2),
			fmt.Sprintf("fund-balances-fy%d-p%04d-above-cip", year, first+4))
	}
	return out
}

// fundBalanceExceptions are the cells of pp.186-209's comparisons that do not
// tie. FY2023-24's rounding, p0067's 250,000 and pp.127-130's missing General
// Fund transfers in are shapes BudgetBookExceptions holds on other schedules.
func fundBalanceExceptions() []Exception {
	present := func(c int64) Sum { return Sum{Cents: c, Present: true} }
	group := func(g, category string) map[Axis]string {
		return map[Axis]string{AxisFundGroup: g, AxisCategory: category}
	}
	fund := func(g, number string) map[Axis]string {
		return map[Axis]string{AxisFundGroup: g, AxisFund: number}
	}
	transfersIn := func(g, number string) map[Axis]string {
		return map[Axis]string{AxisFundGroup: g, AxisFund: number, AxisCategory: "transfers/in"}
	}
	return []Exception{
		carriesADollar("capital-beginning-2026", 2026, registry.FundTypeCapital, "fund-balance/beginning", 11071354400, 11071354500, 100,
			"p0067.txt:40 BEGINNING WORKING CAPITAL and p0200.txt:44 Total Capital Funds, both 110,713,545"),
		carriesADollar("capital-ending-2026", 2026, registry.FundTypeCapital, "fund-balance/ending", 10821333100, 10821333200, 100,
			"p0067.txt:42 ENDING WORKING CAPITAL and p0201.txt:46, Total Capital Funds' line, both 108,213,332"),
		carriesADollar("capital-beginning-2027", 2027, registry.FundTypeCapital, "fund-balance/beginning", 10821333100, 10821333200, 100,
			"p0067.txt:40 BEGINNING WORKING CAPITAL and p0206.txt:44 Total Capital Funds, both 108,213,332"),
		carriesADollar("capital-ending-2027", 2027, registry.FundTypeCapital, "fund-balance/ending", 9808391500, 9808391600, 100,
			"p0067.txt:42 ENDING WORKING CAPITAL and p0207.txt:46, Total Capital Funds' line, both 98,083,916"),
		carriesADollar("special-revenue-beginning-2026", 2026, registry.FundTypeSpecialRevenue, "fund-balance/beginning", 7729007400, 7729007300, -100,
			"p0067.txt:40 BEGINNING WORKING CAPITAL and p0200.txt:11 Total Special Revenue Funds, both 77,290,073"),
		carriesADollar("special-revenue-ending-2026", 2026, registry.FundTypeSpecialRevenue, "fund-balance/ending", 8616502300, 8616502200, -100,
			"p0067.txt:42 ENDING WORKING CAPITAL and p0201.txt:10, Total Special Revenue Funds' line, both 86,165,022"),
		carriesADollar("special-revenue-beginning-2027", 2027, registry.FundTypeSpecialRevenue, "fund-balance/beginning", 8616502300, 8616502200, -100,
			"p0067.txt:40 BEGINNING WORKING CAPITAL and p0206.txt:11 Total Special Revenue Funds, both 86,165,022"),
		carriesADollar("special-revenue-ending-2027", 2027, registry.FundTypeSpecialRevenue, "fund-balance/ending", 9518699500, 9518699400, -100,
			"p0067.txt:42 ENDING WORKING CAPITAL and p0207.txt:10, Total Special Revenue Funds' line, both 95,186,994"),

		{
			Name: "p0067-internal-service-ending-is-250000-low",
			Cut:  CutFundBalanceFlows, Against: CutSpine, At: LevelFundGroupByCategory,
			Cells: []Pin{{Year: 2027, Basis: "adopted", Coords: group(registry.FundTypeInternalService, "fund-balance/ending"),
				Cut: present(877908700), Against: present(852908700)}},
			Residual: -25000000,
			Printed: "both sides: p0067.txt:42 prints ENDING WORKING CAPITAL $8,529,087 for the Internal Service Funds, " +
				"and 8,779,087 is printed on p0209.txt:20, p0205.txt:17 and p0075.txt:53",
			Reason: "p0067's Internal Service Funds column prints FY2026-27 expenditure 250,000 above what the " +
				"funds' own rows sum to, and carries it into the column's ending working capital. The store " +
				"publishes p0067 as printed",
			Bead: "fisc-av0w",
		},
		{
			Name: "p0067-internal-service-is-250000-high-by-fund-total",
			Cut:  CutFundBalanceExpenses, Against: CutSpine, At: LevelFundGroup,
			Cells: []Pin{{Year: 2027, Basis: "adopted", Coords: map[Axis]string{AxisFundGroup: registry.FundTypeInternalService},
				Cut: present(2629451500), Against: present(2654451500)}},
			Residual:       25000000,
			SameResidualAs: "p0067-internal-service-is-250000-high-by-fund-group",
			Printed: "both sides: p0067.txt:34 prints TOTAL EXPENDITURES 26,544,515 for the Internal Service Funds, " +
				"and p0209.txt:20 prints Total Internal Service Funds Expenses 26,294,515",
			Reason: "p0067's Internal Service Funds column prints Services & Supplies of 16,796,010 for " +
				"FY2026-27, and the five internal service funds' Expenses on p209 sum to 250,000 less. " +
				"The store publishes p0067 as printed",
			Bead: "fisc-av0w",
		},

		roundsAnActual("pp.127-140-round-low-income-housing-revenue-2024", CutRevenueDetail, CutFundBalanceRevenues,
			registry.FundTypeSpecialRevenue, "200", 585291100, 585291200, 100,
			"p0135.txt:18 Total Low Income Housing Fund and p0186.txt:27 Revenues, both 5,852,912"),
		roundsAnActual("pp.127-140-round-airport-revenue-2024", CutRevenueDetail, CutFundBalanceRevenues,
			registry.FundTypeEnterprise, "600", 488652400, 488652500, 100,
			"p0131.txt:20 Total Airport and p0188.txt:48 Revenues, both 4,886,525"),
		roundsAnActual("p0172-rounds-general-fund-expenses-2024", "general-fund-by-category", CutFundBalanceExpenses,
			registry.FundTypeGeneral, "100", 12322819100, 12322819000, -100,
			"p0172.txt:24 Total General Fund and p0187.txt:9 Expenses, both 123,228,190"),
		roundsAnActual("pp.173-183-round-downtown-lmd-expenses-2024", "fund-expenditures", CutFundBalanceExpenses,
			registry.FundTypeSpecialRevenue, "310", 69456500, 69456400, -100,
			"p0179.txt:11 Total Downtown LMD and p0187.txt:60 Expenses, both 694,564"),
		roundsAnActual("pp.173-183-round-other-maintenance-cfds-expenses-2024", "fund-expenditures", CutFundBalanceExpenses,
			registry.FundTypeSpecialRevenue, "321", 17613700, 17613800, 100,
			"p0181.txt:52 Total Other Maintenance CFDs and p0187.txt:63 Expenses, both 176,138"),
		roundsAnActual("pp.173-183-round-airport-expenses-2024", "fund-expenditures", CutFundBalanceExpenses,
			registry.FundTypeEnterprise, "600", 300082800, 300082900, 100,
			"p0173.txt:20 Total Airport and p0189.txt:51 Expenses, both 3,000,829"),
		roundsAnActual("pp.173-183-round-water-expenses-2024", "fund-expenditures", CutFundBalanceExpenses,
			registry.FundTypeEnterprise, "640", 1693586800, 1693586900, 100,
			"p0174.txt:13 Total Water and p0189.txt:57 Expenses, both 16,935,869"),
		roundsAnActual("pp.173-183-round-facilities-rehab-expenses-2024", "fund-expenditures", CutFundBalanceExpenses,
			registry.FundTypeInternalService, "740", 248723700, 248723800, 100,
			"p0183.txt:22 Total Facilities Rehab Pgm and p0191.txt:9 Expenses, both 2,487,238"),

		{
			Name: "pp.127-140-print-three-funds-revenue-as-the-state-grant-funds-2024",
			Cut:  CutRevenueDetail, Against: CutFundBalanceRevenues, At: LevelFund,
			Cells: []Pin{
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "204"), Cut: present(0), Against: present(20854000)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "205"), Cut: present(0), Against: present(76100)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "240"), Cut: present(147300600), Against: present(126370500)},
			},
			Residual: 0,
			Printed: "p0137.txt:30 Total Grant - State Grant Fund 1,473,006, which p0186.txt:31, :32 and :44 print as " +
				"HHS Loan Fund 208,540, Cal Home Reuse 761 and Grant - State Grant Fund 1,263,705",
			Reason: "in the FY2023-24 Actual column pp.127-140 print HHS Loan Fund and Cal Home Reuse as dashes and " +
				"their revenue inside Grant - State Grant Fund's, and pp.186-209 print each fund's own",
			Bead: "fisc-3eh2",
		},
		{
			Name: "pp.173-183-print-four-funds-spending-as-the-state-grant-funds-2024",
			Cut:  "fund-expenditures", Against: CutFundBalanceExpenses, At: LevelFund,
			Cells: []Pin{
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "204"), Cut: present(0), Against: present(46511500)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "205"), Cut: present(0), Against: present(77800)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "206"), Cut: present(0), Against: present(54400)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "240"), Cut: present(113514100), Against: present(66870500)},
			},
			Residual: 100,
			Printed: "p0179.txt:68 Total Grant - State Grant Fund 1,135,142, which p0187.txt:30, :31, :32 and :43 print as " +
				"HHS Loan Fund 465,115, Cal Home Reuse 778, California Begin Program 544 and Grant - State Grant " +
				"Fund 668,705; p0179's rows miss its total by the dollar their rule's stated_total_deltas declare",
			Reason: "in the FY2023-24 Actual column pp.173-183 print HHS Loan Fund, Cal Home Reuse and California " +
				"Begin Program as dashes and their spending inside Grant - State Grant Fund's, and pp.186-209 print " +
				"each fund's own",
			Bead: "fisc-3eh2",
		},
		{
			Name: "pp.85-125-fund-four-funds-spending-from-the-state-grant-fund-2024",
			Cut:  CutFundingSources, Against: CutFundBalanceExpenses, At: LevelFund,
			Cells: []Pin{
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "204"), Cut: present(0), Against: present(46511500)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "205"), Cut: present(0), Against: present(77800)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "206"), Cut: present(0), Against: present(54400)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "240"), Cut: present(113514200), Against: present(66870500)},
			},
			Residual: 0,
			Printed: "p0101.txt prints HHS Loan Fund, Cal Home Reuse and California Begin Program as dashes and five " +
				"departments' Grant - State Grant Fund rows on pp.102-124 sum to p0179.txt:68's 1,135,142, which " +
				"p0187.txt:30, :31, :32 and :43 print as 465,115, 778, 544 and 668,705",
			Reason: "in the FY2023-24 Actual column pp.85-125 fund the three housing funds' spending from Grant - " +
				"State Grant Fund, as pp.173-183 do, and pp.186-209 print each fund's own",
			Bead: "fisc-3eh2",
		},
		{
			Name: "pp.85-125-fund-maintenances-transfer-out-2024",
			Cut:  CutFundingSources, Against: CutFundBalanceExpenses, At: LevelFund,
			Cells: []Pin{
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "310"), Cut: present(69457400), Against: present(69456400)},
				{Year: 2024, Basis: "actual", Coords: fund(registry.FundTypeSpecialRevenue, "311"), Cut: present(296560200), Against: present(269881400)},
			},
			Residual: -26679800,
			Printed: "p0124.txt:25 Transfers Out 266,798, which p0124.txt:61 and :63 fund in Downtown LMD 694,574 and " +
				"Other LMD 2,965,602, p0187.txt:60 and :61's Total Uses, and p0187 prints as Transfers Out of 10 and 266,788",
			Reason: "pp.85-125 fund Maintenance's FY2023-24 Transfers Out from Downtown LMD and Other LMD, so those " +
				"funds' funding sources are pp.186-209's Total Uses, whose Expenses leave the transfer out",
			Bead: "fisc-3eh2",
		},
		{
			Name: "pp.186-209-carry-no-police-donations-other-financing-2025",
			Cut:  CutRevenueDetail, Against: CutFundBalanceRevenues, At: LevelFund,
			Cells: []Pin{{Year: 2025, Basis: "revised", Coords: fund(registry.FundTypeSpecialRevenue, "331"),
				Cut: present(550000), Against: present(500000)}},
			Residual: -50000,
			Printed: "p0139.txt:56 Oth Financing Source 500 in p0139.txt:58's Total Police Donations 5,500, and " +
				"p0194.txt:10 Revenues 5,000 and Transfers In -",
			Reason: "pp.127-140 count a 500 Other Financing Source in Police Donations' FY2024-25 revenue, and " +
				"pp.186-209 carry it nowhere: 48,657 + 5,000 - 9,012 is the 44,645 p195 prints",
			Bead: "fisc-3eh2",
		},

		printsNoGFTransferIn("pp.127-130-print-no-general-fund-transfer-in-2024", 2024, "actual", 73745500, 73745500,
			"p0186.txt:10, General Fund Transfers In 737,455"),
		printsNoGFTransferIn("pp.127-130-print-no-general-fund-transfer-in-2025", 2025, "revised", 91420600, 91420600,
			"p0192.txt:10, General Fund Transfers In 914,206"),
		printsNoGFTransferIn("pp.127-130-print-no-general-fund-transfer-in-pp.186-209-2026", 2026, "adopted", 48040000, 48040000,
			"p0198.txt:10, General Fund Transfers In 480,400"),
		printsNoGFTransferIn("pp.127-130-print-no-general-fund-transfer-in-pp.186-209-2027", 2027, "adopted", 48673500, 48673500,
			"p0204.txt:10, General Fund Transfers In 486,735"),
		{
			Name: "pp.131-140-print-no-general-fund-cip-reserves-2025",
			Cut:  CutRevenueDetail, Against: CutFundBalanceFlows, At: LevelFundByCategory,
			Cells:    []Pin{{Year: 2025, Basis: "revised", Coords: transfersIn(registry.FundTypeCapital, "101"), Against: present(412562700)}},
			Residual: 412562700,
			Printed:  "p0194.txt:29, General Fund CIP Reserves Transfers In 4,125,627; pp.131-140 print no section for fund 101",
			Reason: "pp.131-140 print a section for every capital fund but General Fund CIP Reserves, whose one " +
				"transfer in is FY2024-25's",
			Bead: "fisc-zl9",
		},
	}
}

// roundsADollar is a dollar of FY2023-24 rounding, one cell, both sides'
// printed figure named. Its residual is declared beside the pins rather than
// computed from them, so ValidateExceptions holds a mistyped pin to it.
func roundsADollar(name, cut, against string, at Level, year int, basis string, coords map[Axis]string,
	c, a, residual int64, printed, reason string) Exception {
	return Exception{
		Name: name, Cut: cut, Against: against, At: at,
		Cells: []Pin{{Year: year, Basis: basis, Coords: coords,
			Cut: Sum{Cents: c, Present: true}, Against: Sum{Cents: a, Present: true}}},
		Residual: residual,
		Printed:  printed,
		Reason:   reason,
		Bead:     "fisc-2sd",
	}
}

// carriesADollar is the dollar of FY2023-24 rounding pp.186-209 carry into a
// fund group's later balances against pp.66-67, as roundsADollar declares it.
// Its residual is declared beside the pins rather than computed from them, so
// ValidateExceptions holds a mistyped pin to it.
func carriesADollar(name string, year int, g, category string, c, a, residual int64, printed string) Exception {
	return roundsADollar("pp.186-209-carry-a-dollar-of-"+name, CutFundBalanceFlows, CutSpine,
		LevelFundGroupByCategory, year, "adopted", map[Axis]string{AxisFundGroup: g, AxisCategory: category}, c, a, residual,
		printed+"; pp.186-209's rows miss it by the dollar their rules' subtotal_deltas declare",
		"FY2023-24 actuals are printed rounded figure by figure, and pp.186-209 carry the dollar "+
			"a fund group's rows miss their printed total by into every later beginning and ending balance; "+
			"the group's printed total is pp.66-67's figure")
}

// roundsAnActual is a fund total two schedules print in the FY2023-24 Actual
// column over rows each rounds differently, as roundsADollar declares it. Its
// residual is declared beside the pins rather than computed from them, so
// ValidateExceptions holds a mistyped pin to it.
func roundsAnActual(name, cut, against string, g, number string, c, a, residual int64, printed string) Exception {
	return roundsADollar(name, cut, against, LevelFund, 2024, "actual", map[Axis]string{AxisFundGroup: g, AxisFund: number},
		c, a, residual, printed+"; the rows under it miss it by the dollar their rules' stated_total_deltas declare",
		"the two schedules print one fund total in the FY2023-24 Actual column and round the rows under it differently")
}

// printsNoGFTransferIn is the General Fund transfer in pp.186-209 print and
// pp.127-130 do not. Its residual is the printed figure, declared rather than
// taken from the pin, so ValidateExceptions holds a mistyped pin to it.
func printsNoGFTransferIn(name string, year int, basis string, c, residual int64, printed string) Exception {
	return Exception{
		Name: name,
		Cut:  CutRevenueDetail, Against: CutFundBalanceFlows, At: LevelFundByCategory,
		Cells: []Pin{{Year: year, Basis: basis,
			Coords:  map[Axis]string{AxisFundGroup: registry.FundTypeGeneral, AxisFund: "100", AxisCategory: "transfers/in"},
			Against: Sum{Cents: c, Present: true}}},
		Residual: residual,
		Printed:  printed + "; pp.127-130 print TOTAL REVENUES and no Transfers In row",
		Reason: "pp.127-130 print TOTAL REVENUES for the General Fund and no Transfers In row at all, so " +
			"pp.186-209's General Fund transfer in has no counterpart in this schedule",
		Bead: "fisc-5gk.3.1",
	}
}

// printsNoGFTransferInP76 is the General Fund transfer in p76 prints and
// pp.127-130 do not: a peer absence, declared on the peer pair at its level,
// since the spine's exceptions excuse none. Its residual is the printed
// figure, declared rather than taken from the pin, so ValidateExceptions
// holds a mistyped pin to it.
func printsNoGFTransferInP76(year int, c, residual int64, printed string) Exception {
	return Exception{
		Name: fmt.Sprintf("pp.127-130-print-no-general-fund-transfer-in-p76-%d", year),
		Cut:  CutRevenueDetail, Against: "transfers-detail", At: LevelFundByCategory,
		Cells: []Pin{{Year: year, Basis: "adopted",
			Coords:  map[Axis]string{AxisFundGroup: registry.FundTypeGeneral, AxisFund: "100", AxisCategory: "transfers/in"},
			Against: Sum{Cents: c, Present: true}}},
		Residual: residual,
		Printed:  printed + "; pp.127-130 print TOTAL REVENUES and no Transfers In row",
		Reason: "pp.127-130 print TOTAL REVENUES for the General Fund and no Transfers In row at all, so " +
			"p76's transfers into the General Fund have no counterpart in this schedule",
		Bead: "fisc-5gk.3.1",
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
