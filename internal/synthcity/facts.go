package synthcity

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// Doc is the document every fact cites: the Budget Book, at the real pages
// its schedules print on, so a citation resolves to a page the site carries.
const Doc = "livermore-budget-fy2026-2027"

// Names is the fund names and division labels a fact prints, the registry's.
type Names interface {
	FundName(number int) (string, bool)
	DivisionLabel(slug string) (string, bool)
}

// spineGroups are pp.66-67's six columns, left to right.
var spineGroups = []string{"general", "enterprise", "capital", "debt-service", "special-revenue", "internal-service"}

// spineRevenue and spineExpenditure are pp.66-67's rows, in order, with the
// words the page prints them in.
var (
	spineRevenue = []struct{ category, label string }{
		{"taxes/property", "Property Taxes"}, {"taxes/other", "Other Taxes"},
		{"intergovernmental", "Intergovernmental"}, {"charges-for-services", "Charges for Services"},
		{"use-of-money-and-property", "Use of Money And Property"},
		{"contributions-outsourced", "Contributions Outsourced"},
		{"miscellaneous-revenue", "Miscellaneous Revenue"}, {"taxes/sales", "Sales Taxes"},
		{"fines-and-forfeitures", "Fines & Forfeitures"}, {"licenses-and-permits", "Licenses & Permits"},
	}
	spineExpenditure = []struct{ category, label string }{
		{wages, "Wages & Benefits"}, {supplies, "Services & Supplies"},
		{capital, "Capital Outlay"}, {debt, "Debt Services"},
	}
)

// objectLabel is the words an object category prints in on every schedule.
func objectLabel(category string) string {
	for _, r := range spineExpenditure {
		if r.category == category {
			return r.label
		}
	}
	return category
}

// writer renders facts, numbering each page's offsets across every column so
// no two facts land on one, as a page's own tokens do not.
type writer struct {
	col     Column
	offsets map[int]int
	// ending is each fund's closing balance in the column before, which is
	// the one it opens this column with.
	ending map[int]int64
	out    []fact.Fact
	err    error
}

func (w *writer) offset(page int) int {
	w.offsets[page] += 10
	return w.offsets[page]
}

// add is one printed figure. A negative figure prints in parentheses.
func (w *writer) add(rule string, row mapping.Row, label string, col mapping.Column, scope string,
	kind vocab.Kind, page, offset int, cents int64) {
	rowPath := fact.RowPath(row)
	columnPath := fact.ColumnPath(col, scope)
	f := fact.Fact{
		ID:          fact.MakeID(Doc, rule, rowPath, label, columnPath, w.col.Year, w.col.Basis),
		DocID:       Doc,
		Page:        page,
		Offset:      offset,
		Token:       token(cents),
		RuleID:      rule,
		Kind:        kind,
		Basis:       w.col.Basis,
		Scope:       scope,
		FiscalYear:  w.col.Year,
		RowPath:     rowPath,
		RowLabel:    label,
		Category:    row.Category,
		Department:  row.Department,
		ColumnPath:  columnPath,
		FundGroup:   col.FundGroup,
		Sign:        vocab.SignPositive,
		Units:       "dollars",
		AmountCents: cents,
	}
	if col.Fund != 0 {
		f.Fund = fact.FundNumber(col.Fund)
	}
	w.out = append(w.out, f)
}

// token is a figure as a page prints it: "-" for zero, whole dollars with
// commas, a negative in parentheses.
func token(cents int64) string {
	if cents == 0 {
		return "-"
	}
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := strconv.FormatInt(cents/100, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "(" + s + ")"
	}
	return s
}

// pct is the percentage of f's figures col prints: none outside f's Years,
// where its rows print as dashes, as the Budget Book prints a fund's rows in
// every column.
func (f Fund) pct(col Column) int64 {
	if len(f.Years) == 0 || slices.Contains(f.Years, col.Year) {
		return col.Pct
	}
	return 0
}

// Facts is the city as every schedule a column draws prints it, over every
// column of Columns.
func (c City) Facts(names Names) ([]fact.Fact, error) {
	w := &writer{offsets: map[int]int{}, ending: map[int]int64{}}
	for _, col := range Columns {
		w.col = col
		c.column(w, names)
		if w.err != nil {
			return nil, w.err
		}
	}
	return w.out, nil
}

// scaled is cents as col prints them.
func scaled(cents int64, col Column) int64 { return cents * col.Pct / 100 }

// flows is one fund's figures in one column, derived from the spec.
type flows struct {
	revenue, spend, in, out int64
	byObject                map[string]int64
}

func (c City) column(w *writer, names Names) {
	col := w.col
	funds := map[int]Fund{}
	var order []int
	for _, f := range c.Funds {
		funds[f.Number] = f
		order = append(order, f.Number)
	}
	at := func(n int, cents int64) int64 { return cents * funds[n].pct(col) / 100 }
	name := func(n int) string {
		s, ok := names.FundName(n)
		if !ok {
			w.err = fmt.Errorf("synthcity: fund %d is in no data/funds.yaml", n)
		}
		return s
	}
	divisionOf := map[string]Division{}
	for _, d := range c.Divisions {
		divisionOf[d.Slug] = d
	}
	fl := map[int]*flows{}
	for _, n := range order {
		fl[n] = &flows{byObject: map[string]int64{}}
	}

	// pp.127-140, revenue by fund.
	for i, n := range order {
		f := funds[n]
		page := 127 + i%14
		rule := "fund-rev-" + strconv.Itoa(n)
		if n == 100 {
			rule = "gf-rev"
		}
		for _, l := range f.Revenue {
			cents := at(n, l.Cents)
			fl[n].revenue += cents
			w.add(rule, mapping.Row{Category: l.Category}, l.Label, mapping.Column{FundGroup: f.Group, Fund: n},
				structure.ScopeRevenueByFund, vocab.KindRevenue, page, w.offset(page), cents)
		}
	}

	// p76, transfers by fund: one row, its two legs at one offset.
	for _, t := range c.Transfers {
		from, fok := funds[t.From]
		to, tok := funds[t.To]
		if !fok || !tok {
			w.err = fmt.Errorf("synthcity: a transfer joins fund %d to %d, and the city has no such fund", t.From, t.To)
			return
		}
		cents := scaled(t.Cents, col)
		fl[t.From].out += cents
		fl[t.To].in += cents
		label := "Transfer From " + name(t.From) + " to " + name(t.To)
		rule := "p76-transfers-in-" + to.Group
		at := w.offset(76)
		w.add(rule, mapping.Row{Category: "transfers/in"}, label, mapping.Column{FundGroup: to.Group, Fund: t.To},
			structure.ScopeTransfersByFund, vocab.KindTransferIn, 76, at, cents)
		w.add(rule, mapping.Row{Category: "transfers/out"}, label, mapping.Column{FundGroup: from.Group, Fund: t.From},
			structure.ScopeTransfersByFund, vocab.KindTransferOut, 76, at, cents)
	}
	// pp.127-140 print a receiving fund's transfers in as a row of its own,
	// except the General Fund's, whose pages print none.
	for i, n := range order {
		if n == 100 || fl[n].in == 0 {
			continue
		}
		page := 127 + i%14
		w.add("fund-rev-"+strconv.Itoa(n), mapping.Row{Category: "transfers/in"}, "Transfers In",
			mapping.Column{FundGroup: funds[n].Group, Fund: n}, structure.ScopeRevenueByFund, vocab.KindTransferIn,
			page, w.offset(page), fl[n].in)
	}
	// p222: a zero pair from a fund p76 pays nothing from, so the column
	// carries the schedule and moves nothing to the CIP.
	cipAt := w.offset(222)
	w.add("p222-cip-funding-sources", mapping.Row{Category: "transfers/in"}, "811 Capital Projects",
		mapping.Column{FundGroup: "capital", Fund: 811}, structure.ScopeCIPFundingSources, vocab.KindTransferIn, 222, cipAt, 0)
	w.add("p222-cip-funding-sources", mapping.Row{Category: "transfers/out-to-cip"}, "811 Capital Projects",
		mapping.Column{FundGroup: "capital", Fund: 550}, structure.ScopeCIPFundingSources, vocab.KindTransferOut, 222, cipAt, 0)

	// Spending: pp.167-170 for the General Fund by division, pp.171-185 for
	// every other fund by object, pp.85-125 by division with every fund
	// together and by department fund by fund.
	type key struct{ division, category string }
	divisionwide := map[key]int64{}
	var divisionOrder []key
	type fkey struct {
		department string
		fund       int
	}
	funding := map[fkey]int64{}
	var fundingOrder []fkey
	gfDivision := map[key]int64{}
	var gfOrder []key
	for _, n := range order {
		f := funds[n]
		for _, s := range f.Spend {
			d, ok := divisionOf[s.Division]
			if !ok {
				w.err = fmt.Errorf("synthcity: fund %d spends through %q, which the city declares no division for", n, s.Division)
				return
			}
			cents := at(n, s.Cents)
			fl[n].spend += cents
			fl[n].byObject[s.Category] += cents
			k := key{s.Division, s.Category}
			if _, seen := divisionwide[k]; !seen {
				divisionOrder = append(divisionOrder, k)
			}
			divisionwide[k] += cents
			fk := fkey{d.Department, n}
			if _, seen := funding[fk]; !seen {
				fundingOrder = append(fundingOrder, fk)
			}
			funding[fk] += cents
			if n == 100 {
				if _, seen := gfDivision[k]; !seen {
					gfOrder = append(gfOrder, k)
				}
				gfDivision[k] += cents
			}
		}
	}
	for _, k := range gfOrder {
		w.add("div-"+k.division, mapping.Row{Category: k.category, Department: k.division}, objectLabel(k.category),
			mapping.Column{FundGroup: "general", Fund: 100}, structure.ScopeExpenditureByDepartment, vocab.KindExpenditure,
			167, w.offset(167), gfDivision[k])
	}
	if gf, ok := fl[100]; ok {
		for _, r := range spineExpenditure {
			if cents, ok := gf.byObject[r.category]; ok {
				w.add("fund-exp-general-fund", mapping.Row{Category: r.category}, r.label,
					mapping.Column{FundGroup: "general", Fund: 100}, structure.ScopeGeneralFundByCategory,
					vocab.KindExpenditure, 172, w.offset(172), cents)
			}
		}
	}
	for i, n := range order {
		if n == 100 {
			continue
		}
		page := 173 + i%12
		for _, r := range spineExpenditure {
			if cents, ok := fl[n].byObject[r.category]; ok {
				w.add("fund-exp-"+strconv.Itoa(n), mapping.Row{Category: r.category}, r.label,
					mapping.Column{FundGroup: funds[n].Group, Fund: n}, structure.ScopeExpenditureByFund,
					vocab.KindExpenditure, page, w.offset(page), cents)
			}
		}
	}
	for _, k := range divisionOrder {
		page := divisionOf[k.division].Page
		w.add("dw-"+k.division, mapping.Row{Category: k.category, Department: k.division}, objectLabel(k.category),
			mapping.Column{}, structure.ScopeDepartmentwideExpenditures, vocab.KindExpenditure, page, w.offset(page), divisionwide[k])
	}
	for _, k := range fundingOrder {
		page := 0
		for _, d := range c.Divisions {
			if d.Department == k.department {
				page = d.Page
				break
			}
		}
		w.add("funding-"+k.department,
			mapping.Row{Category: "department-funding-sources", Department: k.department}, name(k.fund),
			mapping.Column{FundGroup: funds[k.fund].Group, Fund: k.fund}, structure.ScopeDepartmentFundingSources,
			vocab.KindExpenditure, page, w.offset(page), funding[k])
	}

	// pp.186-209, fund balances: sources on the first page, uses on the next.
	first := 186 + 6*(col.Year-2024)
	type group struct{ beginning, ending int64 }
	groups := map[string]*group{}
	for _, n := range order {
		f := funds[n]
		page := first
		if f.Group != "general" && f.Group != "special-revenue" {
			page = first + 2
		}
		rule := fmt.Sprintf("fund-balances-fy%d-p%04d", col.Year, page)
		begin, carried := w.ending[n]
		if !carried {
			begin = f.Beginning
		}
		end := begin + fl[n].revenue + fl[n].in - fl[n].spend - fl[n].out
		if end < 0 {
			w.err = fmt.Errorf("synthcity: fund %d ends FY%d below zero", n, col.Year)
			return
		}
		w.ending[n] = end
		at := mapping.Column{FundGroup: f.Group, Fund: n}
		label := name(n)
		for _, r := range []struct {
			kind     vocab.Kind
			category string
			page     int
			cents    int64
		}{
			{vocab.KindFundBalance, "fund-balance/beginning", page, begin},
			{vocab.KindRevenue, "revenues", page, fl[n].revenue},
			{vocab.KindTransferIn, "transfers/in", page, fl[n].in},
			{vocab.KindExpenditure, "expenses", page + 1, fl[n].spend},
			{vocab.KindTransferOut, "transfers/out", page + 1, fl[n].out},
			{vocab.KindTransferOut, "transfers/out-to-cip", page + 1, 0},
			{vocab.KindFundBalance, "fund-balance/reserve-increase", page + 1, 0},
			{vocab.KindFundBalance, "fund-balance/ending", page + 1, end},
		} {
			w.add(rule, mapping.Row{Category: r.category}, label, at, structure.ScopeFundBalancesByFund, r.kind,
				r.page, w.offset(r.page), r.cents)
		}
		g := groups[f.Group]
		if g == nil {
			g = &group{}
			groups[f.Group] = g
		}
		g.beginning += begin
		g.ending += end
	}

	// p{first+4}, the Capital Improvement Program Funds block: p222's money,
	// restated, so its one fund prints the zeros p222 moves.
	cipRule := fmt.Sprintf("fund-balances-fy%d-p%04d", col.Year, first+4)
	for _, r := range []struct {
		kind     vocab.Kind
		category string
	}{
		{vocab.KindFundBalance, "fund-balance/beginning"}, {vocab.KindRevenue, "revenues"},
		{vocab.KindTransferIn, "transfers/in"}, {vocab.KindExpenditure, "expenses"},
		{vocab.KindTransferOut, "transfers/out"}, {vocab.KindTransferOut, "transfers/out-to-cip"},
		{vocab.KindFundBalance, "fund-balance/reserve-increase"}, {vocab.KindFundBalance, "fund-balance/ending"},
	} {
		w.add(cipRule, mapping.Row{Category: r.category}, name(811), mapping.Column{FundGroup: "capital", Fund: 811},
			structure.ScopeFundBalancesByFund, r.kind, first+4, w.offset(first+4), 0)
	}

	if !col.Spine {
		return
	}
	// pp.66-67, the spine: every row across all six columns, each cell the
	// sum of the group's funds.
	sum := func(group string, of func(int) int64) int64 {
		var s int64
		for _, n := range order {
			if funds[n].Group == group {
				s += of(n)
			}
		}
		return s
	}
	cell := func(rule, category, label, group string, kind vocab.Kind, cents int64) {
		page := 67
		if group == "general" || group == "enterprise" {
			page = 66
		}
		w.add(rule, mapping.Row{Category: category}, label, mapping.Column{FundGroup: group},
			structure.ScopeAllFundsGross, kind, page, w.offset(page), cents)
	}
	for _, r := range spineRevenue {
		for _, g := range spineGroups {
			cell("spine-revenues", r.category, r.label, g, vocab.KindRevenue, sum(g, func(n int) int64 {
				var s int64
				for _, l := range funds[n].Revenue {
					if l.Category == r.category {
						s += at(n, l.Cents)
					}
				}
				return s
			}))
		}
	}
	for _, r := range spineExpenditure {
		for _, g := range spineGroups {
			cell("spine-expenditures", r.category, r.label, g, vocab.KindExpenditure,
				sum(g, func(n int) int64 { return fl[n].byObject[r.category] }))
		}
	}
	for _, g := range spineGroups {
		cell("spine-transfers-in", "transfers/in", "TRANSFER IN:", g, vocab.KindTransferIn,
			sum(g, func(n int) int64 { return fl[n].in }))
	}
	for _, g := range spineGroups {
		cell("spine-transfers-out", "transfers/out", "TRANSFER OUT:", g, vocab.KindTransferOut,
			sum(g, func(n int) int64 { return fl[n].out }))
	}
	for _, g := range spineGroups {
		cell("spine-fund-balance", "fund-balance/reserve-increase", "ADDITION TO RESERVES", g, vocab.KindFundBalance, 0)
	}
	for _, g := range spineGroups {
		var b, e int64
		if gr := groups[g]; gr != nil {
			b, e = gr.beginning, gr.ending
		}
		cell("spine-fund-balance", "fund-balance/change", "CHANGE IN WORKING CAPITAL", g, vocab.KindFundBalance, e-b)
		cell("spine-fund-balance", "fund-balance/beginning", "BEGINNING WORKING CAPITAL", g, vocab.KindFundBalance, b)
		cell("spine-fund-balance", "fund-balance/ending", "ENDING WORKING CAPITAL", g, vocab.KindFundBalance, e)
	}
}
