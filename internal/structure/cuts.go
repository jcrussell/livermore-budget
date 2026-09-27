package structure

import "github.com/jcrussell/livermore-budget/internal/mapping"

// BudgetBookCuts are the Budget Book's schedules as cuts of one hierarchy.
//
// EACH IS A CLAIM ABOUT THE PAGES. The level says which axes the schedule
// totals over; the kinds and fund groups say which money it covers. Both are
// declared rather than measured, because a footprint read off the facts shrinks
// with them: a schedule that stops printing a fund group would drop that group
// from its own footprint, and the cells it stopped printing would never be
// looked at.
//
// WHAT THIS REPLACES is one file per pair -- each with its own restriction,
// its own key type and its own copy of the comparison. The comparison is
// Contain and the pairs fall out of the lattice.
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
			// The single fund group is a footprint and not a placeholder: the
			// pages are of the General Fund, and `general` is a value five
			// other schedules also use.
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
			// pp.171-176, which department's money comes from which fund. The
			// category field carries the scope's own name on every row; see
			// Cut.Placeholders for why that is not an axis. The rows name
			// DEPARTMENTS where every other schedule with the axis names
			// divisions: data/departments.yaml, "TWO TIERS".
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

// ACFRCuts are the ACFR's schedules as cuts of the same hierarchy.
//
// EVERY COLUMN IS AUDITED, and that is what keeps them apart from the Budget
// Book by declaration rather than by luck. p41's General Fund summary sits at
// the spine's own level and pp.127-140 by fund decompose that level, so the
// lattice offers a containment between two documents with no column in
// common; the declared bases refuse it by name.
//
// ONE SCOPE PRINTS TWO GRAINS AND IS TWO CUTS. p167 prints the General Fund's
// fund balance components with the fund group named, and every other
// governmental fund's aggregated with no group at all -- a coarser grain under
// one scope -- so each rule is its own cut, selected by rule id.
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

// AllCuts is every declared cut, Budget Book and ACFR: one cut per scope, and
// two for the scope that prints two grains. Measured against the store by
// TestEveryScopeInTheStoreIsACut.
func AllCuts() []Cut {
	return append(BudgetBookCuts(), ACFRCuts()...)
}

// BudgetBookIdentities are the same-figure relations between the Budget Book's
// peers, and there is one.
//
// pp.127-140 print each fund's Transfers In inside that fund's block, at the
// RECEIVING end; p76 prints every transfer with both its legs, and its
// receiving leg is the same movement at the same fund. Measured over the
// committed store: 22 shared cells over the two adopted columns, 42,183,495.00
// on each side, identical to the cent. Both readings stay, each with its own
// locator chain, because p76 is the citation a reader clicking a transfer OUT
// lands on and pp.127-140 is the one a reader of a fund's revenue lands on;
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
// reach, held in every column both sides print. pp.85-125 print each
// department's Total Department Expenditures and Total Department Funding
// Sources as one figure, and each department's General Fund funding row is the
// department's total on pp.167-170.
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
// not tie, each pinned on both sides to what its schedule prints and to the
// printed figure the two differ by.
//
// THREE ARGUMENTS, TEN CELLS, AND NONE IS A TOLERANCE. Every other cell of
// every comparison ties to the cent.
//
// WHAT GENERALISES IS THE ARITHMETIC AND NOT THE ARGUMENT. The comparison is
// one function over every pair of cuts; what is worth reading about each
// schedule is the argument for ITS cells -- which rows the pages do not print,
// which figure they print instead and where -- and that is carried here, one
// exception at a time, rather than in a check per schedule.
//
// pp.127-130 PRINT NO GENERAL FUND TRANSFER IN. p130's `Total General Fund` is
// a revenue total, not a sources total, so the spine's General Fund TRANSFER IN
// -- p0066.txt:24, 480,400 and 486,735 -- has no counterpart in the revenue
// schedule. It is not unaccounted for: p76 prints the same movement and
// transfers-detail carries it, which is the peer overlap the identity check
// witnesses. Two cells, one per published column.
//
// pp.66-67's TRANSFER OUT INCLUDES TRANSFERS TO THE CIP THAT p76 DOES NOT LIST;
// its grand total is the transfers-in side. pp.72-75 print the to-CIP figures
// under a heading of their own, per major fund and as ONE aggregate for every
// non-major fund, and those printed figures are the residuals here. Every one
// is read off the page: internal-service and the non-major aggregate verbatim
// (p0073.txt:53 and :56, p0075.txt:53 and :56); enterprise as the difference
// of two figures on one page (:55 Total Major Funds less :53), because
// general and Low Income Housing pay no transfer out in either budget year and
// the major list is otherwise the four enterprises and Internal Service. The
// non-major aggregate is one exception over two cells rather than two
// exceptions, because splitting 28,693,590 between capital and special-revenue
// would be a figure derived by difference from p67 while the pair is what the
// page prints. What that gives up: money moved between those two groups is
// invisible here. general and debt-service pay a printed "-" to the CIP and
// tie without an exception. Eight cells over six exceptions.
//
// p0067's INTERNAL SERVICE COLUMN IS WRONG BY 250,000 AND THE CORPUS PUBLISHES
// IT AS PRINTED. Its Services & Supplies prints 16,796,010 (p0067.txt:31) and
// the five internal service funds' own rows on pp.172-183 sum to 250,000 less;
// five schedules print the group's FY2026-27 expenditure as 26,294,515
// (p0183.txt:64, p0075.txt:53, p0205, p0209, and p0061 as 26,906,515 with the
// 612,000 to-CIP added) and only p0067 prints 26,544,515 (p0067.txt:34).
// fisc-av0w decided the store keeps p0067's figure, because 16,546,010 is
// printed on no page and a corrected fact would be a computed one. The
// discrepancy is held apart on two axes: pp.171-176 by paying fund land it on
// the internal-service GROUP, where both figures are printed; pp.85-125 by
// object land it on SERVICES AND SUPPLIES, where neither sum is a figure any
// page prints, and that entry is grounded in the first by SameResidualAs.
// Neither axis could locate the cell alone; together they put it at
// internal-service x services-and-supplies x FY2026-27, which is where
// pp.172-183 put it from a third direction.
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
