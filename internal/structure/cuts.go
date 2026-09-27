package structure

import "github.com/jcrussell/livermore-budget/internal/mapping"

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
			Name:      "spine",
			Scope:     "all-funds-gross",
			Level:     LevelFundGroupByCategory,
			Kinds:     everyKind,
			Reference: true,
			Bases:     []mapping.Basis{mapping.BasisAdopted},
		},
		{
			// pp.127-140, revenue and transfers in per fund.
			Name:  "revenue-detail",
			Scope: "revenue-by-fund",
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn},
			Bases: budgetBookDetail,
		},
		{
			// p76, both legs of every transfer, per fund.
			Name:  "transfers-detail",
			Scope: "transfers-by-fund",
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindTransferIn, mapping.KindTransferOut},
			Bases: []mapping.Basis{mapping.BasisAdopted},
		},
		{
			// pp.167-170, General Fund expenditure by department and object.
			// `general` is a footprint, not a placeholder.
			Name:           "general-fund-departments",
			Scope:          "expenditure-by-department",
			Level:          LevelFundByDepartmentByCategory,
			Kinds:          []mapping.Kind{mapping.KindExpenditure},
			FundGroups:     []string{"general"},
			DepartmentTier: "division",
			Bases:          budgetBookDetail,
		},
		{
			// pp.85-125, expenditure by department and object across every
			// fund, with no fund axis printed anywhere on the pages.
			Name:           "departmentwide",
			Scope:          "departmentwide-expenditures",
			Level:          LevelDepartmentByCategory,
			Kinds:          []mapping.Kind{mapping.KindExpenditure},
			DepartmentTier: "division",
			Bases:          budgetBookDetail,
		},
		{
			// pp.85-125, which department's money comes from which fund. The
			// rows name departments where the other schedules name divisions.
			Name:           "funding-sources",
			Scope:          "department-funding-sources",
			Level:          LevelFundByDepartment,
			Kinds:          []mapping.Kind{mapping.KindExpenditure},
			Placeholders:   []Axis{AxisCategory},
			DepartmentTier: "department",
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
			Scope:      "acfr-general-fund-summary",
			Level:      LevelFundGroupByCategory,
			Kinds:      everyKind,
			FundGroups: []string{"general"},
			Bases:      audited,
		},
		{
			// pp.168-169, ten audited years of all governmental funds combined,
			// with no fund group printed anywhere.
			Name:  "acfr-changes-in-fund-balances",
			Scope: "acfr-changes-in-fund-balances",
			Level: LevelCategory,
			Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindExpenditure, mapping.KindFundBalance},
			Bases: audited,
		},
		{
			// p167, the General Fund's GASB 54 components with the group named.
			Name:       "acfr-fund-balances/general",
			Scope:      "acfr-fund-balances",
			Rules:      []string{"acfr-p0167-general-fund-balances"},
			Level:      LevelFundGroupByCategory,
			Kinds:      []mapping.Kind{mapping.KindFundBalance},
			FundGroups: []string{"general"},
			Bases:      audited,
		},
		{
			// p167, every other governmental fund's components, aggregated.
			Name:  "acfr-fund-balances/other-governmental",
			Scope: "acfr-fund-balances",
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
		A:     "revenue-detail",
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
		{Name: "a-department-spends-what-funds-it", A: "departmentwide", B: "funding-sources", At: LevelDepartment},
		{Name: "the-general-fund-row-is-the-general-fund-schedule", A: "general-fund-departments",
			B: "funding-sources", At: LevelDepartment},
	}
}

func init() {
	if err := ValidateIdentities(AllCuts(), BudgetBookIdentities()); err != nil {
		panic("internal/structure: " + err.Error())
	}
}

// BudgetBookExceptions are the cells of the Budget Book's comparisons that do
// not tie, each pinned on both sides and to the printed figure the two differ
// by. None is a tolerance; each carries its own argument in Reason and Printed.
//
// p0067's 250,000 is held apart on two axes, fund group and object, because
// fisc-av0w keeps p0067's figure as printed and neither axis locates the cell
// alone.
func BudgetBookExceptions() []Exception {
	general := map[Axis]string{AxisFundGroup: "general", AxisCategory: "transfers/in"}
	out := func(group string) map[Axis]string {
		return map[Axis]string{AxisFundGroup: group, AxisCategory: "transfers/out"}
	}
	present := func(c int64) Sum { return Sum{Cents: c, Present: true} }
	absent := Sum{}
	rounded := func(cut, dept string, c, a int64, printed string) Exception {
		return Exception{
			Name: cut + "-rounds-" + dept + "-2024", Cut: cut, Against: "funding-sources", At: LevelDepartment,
			Cells: []Pin{{Year: 2024, Basis: "actual", Coords: map[Axis]string{AxisDepartment: dept},
				Cut: present(c), Against: present(a)}},
			Residual: a - c,
			Printed:  printed + "; each side's rows miss it by the dollars their rules' stated_total_deltas declare",
			Reason:   "the two schedules print one total and round the rows under it differently in the FY2023-24 Actual column",
			Bead:     "fisc-2sd",
		}
	}
	return []Exception{
		rounded("departmentwide", "administrative-services", 1278595300, 1278595400,
			"p0097.txt:43 and :51, both 12,785,955"),
		rounded("departmentwide", "innovation-and-economic-development", 568058900, 568059000,
			"p0111.txt:27 and :37, both 5,680,590"),
		rounded("departmentwide", "library-department", 658380900, 658380800,
			"p0115.txt:19 and :31, both 6,583,809"),
		rounded("departmentwide", "police-department", 4346324000, 4346324100,
			"p0119.txt:47 and p0120.txt:13, both 43,463,240"),
		func() Exception {
			e := rounded("departmentwide", "public-works", 6258673700, 6285353700,
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
			Cut:  "revenue-detail", Against: "spine", At: LevelFundGroupByCategory,
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
			Cut:  "revenue-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2027, Basis: "adopted", Coords: general, Cut: absent, Against: present(48673500)}},
			Residual: 48673500,
			Printed:  "p0066.txt:24, TRANSFER IN 486,735 for the General Fund; pp.127-130 print TOTAL REVENUES and no Transfers In row",
			Reason: "pp.127-130 print TOTAL REVENUES for the General Fund and no Transfers In row at all, so " +
				"the spine's General Fund transfer in has no counterpart in this schedule; p76 prints the " +
				"same movement, and transfers-detail carries it",
			Bead: "fisc-5gk.3.1",
		},
		{
			Name: "p76-lists-no-transfer-to-the-cip-2026-enterprise",
			Cut:  "transfers-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2026, Basis: "adopted", Coords: out("enterprise"), Cut: present(1046000000), Against: present(1981314700)}},
			Residual: 935314700,
			Printed: "p0073.txt:55 Total Major Funds to the CIP 9,393,147, less p0073.txt:53 the internal service " +
				"line 40,000; the other major funds pay no transfer out to the CIP in FY2025-26",
			Reason: "pp.66-67's TRANSFER OUT includes the enterprise funds' transfers to the Capital Improvement " +
				"Program and p76 does not list them; pp.72-75 print the to-CIP figure",
			Bead: "fisc-aes",
		},
		{
			Name: "p76-lists-no-transfer-to-the-cip-2026-internal-service",
			Cut:  "transfers-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2026, Basis: "adopted", Coords: out("internal-service"), Cut: absent, Against: present(4000000)}},
			Residual: 4000000,
			Printed:  "p0073.txt:53, Internal Service Funds to the CIP 40,000",
			Reason: "p76 lists no internal service payer at all: the whole of the group's FY2025-26 transfer " +
				"out went to the Capital Improvement Program, which pp.72-75 print and p76 does not",
			Bead: "fisc-aes",
		},
		{
			Name: "p76-lists-no-transfer-to-the-cip-2026-non-major",
			Cut:  "transfers-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells: []Pin{
				{Year: 2026, Basis: "adopted", Coords: out("capital"), Cut: present(21115000), Against: present(2858474000)},
				{Year: 2026, Basis: "adopted", Coords: out("special-revenue"), Cut: present(81705000), Against: present(113705000)},
			},
			Residual: 2869359000,
			Printed:  "p0073.txt:56, Total Non-Major Funds to the CIP 28,693,590, one aggregate for capital and special-revenue together",
			Reason: "pp.72-75 print one to-CIP aggregate for every non-major fund and no split by group, so the " +
				"capital and special-revenue cells are held apart together against the one printed figure; " +
				"money moved between the two groups is invisible here",
			Bead: "fisc-aes",
		},
		{
			Name: "p76-lists-no-transfer-to-the-cip-2027-enterprise",
			Cut:  "transfers-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2027, Basis: "adopted", Coords: out("enterprise"), Cut: present(1046000000), Against: present(2468000000)}},
			Residual: 1422000000,
			Printed: "p0075.txt:55 Total Major Funds to the CIP 14,832,000, less p0075.txt:53 the internal service " +
				"line 612,000; the other major funds pay no transfer out to the CIP in FY2026-27",
			Reason: "pp.66-67's TRANSFER OUT includes the enterprise funds' transfers to the Capital Improvement " +
				"Program and p76 does not list them; pp.72-75 print the to-CIP figure",
			Bead: "fisc-aes",
		},
		{
			Name: "p76-lists-no-transfer-to-the-cip-2027-internal-service",
			Cut:  "transfers-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells:    []Pin{{Year: 2027, Basis: "adopted", Coords: out("internal-service"), Cut: absent, Against: present(61200000)}},
			Residual: 61200000,
			Printed:  "p0075.txt:53, Internal Service Funds to the CIP 612,000",
			Reason: "p76 lists no internal service payer at all: the whole of the group's FY2026-27 transfer " +
				"out went to the Capital Improvement Program, which pp.72-75 print and p76 does not",
			Bead: "fisc-aes",
		},
		{
			Name: "p76-lists-no-transfer-to-the-cip-2027-non-major",
			Cut:  "transfers-detail", Against: "spine", At: LevelFundGroupByCategory,
			Cells: []Pin{
				{Year: 2027, Basis: "adopted", Coords: out("capital"), Cut: present(21748500), Against: present(3604773600)},
				{Year: 2027, Basis: "adopted", Coords: out("special-revenue"), Cut: present(80055000), Against: present(90055000)},
			},
			Residual: 3593025100,
			Printed:  "p0075.txt:56, Total Non-Major Funds to the CIP 35,930,251, one aggregate for capital and special-revenue together",
			Reason: "pp.72-75 print one to-CIP aggregate for every non-major fund and no split by group, so the " +
				"capital and special-revenue cells are held apart together against the one printed figure; " +
				"money moved between the two groups is invisible here",
			Bead: "fisc-aes",
		},
		{
			Name: "p0067-internal-service-is-250000-high-by-fund-group",
			Cut:  "funding-sources", Against: "spine", At: LevelFundGroup,
			Cells: []Pin{{Year: 2027, Basis: "adopted", Coords: map[Axis]string{AxisFundGroup: "internal-service"},
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
			Name: "p0067-internal-service-is-250000-high-by-object",
			Cut:  "departmentwide", Against: "spine", At: LevelCategory,
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
