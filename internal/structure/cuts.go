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
	every := []mapping.Kind{
		mapping.KindRevenue, mapping.KindExpenditure,
		mapping.KindTransferIn, mapping.KindTransferOut, mapping.KindFundBalance,
	}
	return []Cut{
		{
			// pp.66-67, the citywide control totals every other Budget Book
			// schedule decomposes.
			Name:  "spine",
			Scope: "all-funds-gross",
			Level: LevelFundGroupByCategory,
			Kinds: every,
		},
		{
			// pp.127-140, revenue and transfers in per fund.
			Name:  "revenue-detail",
			Scope: "revenue-by-fund",
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn},
		},
		{
			// p76, both legs of every transfer, per fund.
			Name:  "transfers-detail",
			Scope: "transfers-by-fund",
			Level: LevelFundByCategory,
			Kinds: []mapping.Kind{mapping.KindTransferIn, mapping.KindTransferOut},
		},
		{
			// pp.167-170, General Fund expenditure by department and object.
			// The single fund group is a footprint and not a placeholder: the
			// pages are of the General Fund, and `general` is a value five
			// other schedules also use.
			Name:       "general-fund-departments",
			Scope:      "expenditure-by-department",
			Level:      LevelFundByDepartmentByCategory,
			Kinds:      []mapping.Kind{mapping.KindExpenditure},
			FundGroups: []string{"general"},
		},
		{
			// pp.85-125, expenditure by department and object across every
			// fund, with no fund axis printed anywhere on the pages.
			Name:  "departmentwide",
			Scope: "departmentwide-expenditures",
			Level: LevelDepartmentByCategory,
			Kinds: []mapping.Kind{mapping.KindExpenditure},
		},
		{
			// pp.171-176, which department's money comes from which fund. The
			// category field carries the scope's own name on every row; see
			// Cut.Placeholders for why that is not an axis.
			Name:         "funding-sources",
			Scope:        "department-funding-sources",
			Level:        LevelFundByDepartment,
			Kinds:        []mapping.Kind{mapping.KindExpenditure},
			Placeholders: []Axis{AxisCategory},
		},
	}
}
