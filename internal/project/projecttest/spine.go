// Package projecttest is the citywide spine of Budget Book pp.66-67, FY2026,
// transcribed by hand as facts, and the labels it is drawn with: the input a
// test builds the published sankey document from, in memory. It imports no
// projection, so internal/project's own tests can use it.
package projecttest

import (
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// The document, scope, year and basis every spine fact is of. Ids come from
// fact.MakeID over the mapping engine's tuple, so a fact here is the
// committed store's fact for the same cell.
const (
	Doc   = "livermore-budget-fy2026-2027"
	Scope = "all-funds-gross"
	Year  = 2026
	Basis = vocab.BasisAdopted
)

// spineGroups is the fund-group column order of spineRows, left to right as
// the two pages print them.
var spineGroups = [6]string{
	"general", "enterprise", "capital", "debt-service", "special-revenue", "internal-service",
}

// SpinePage says which page carries a fund group's column: p66 carries the row
// labels and the General Fund and Enterprise columns, p67 continues the same
// rows for the other four with nothing but numbers.
func SpinePage(group string) int {
	if group == "general" || group == "enterprise" {
		return 66
	}
	return 67
}

// spineRow is one printed row of the schedule across all six columns. Every
// cell is a fact, including the printed dashes, which earn no link.
type spineRow struct {
	kind     vocab.Kind
	rule     string
	category string
	label    string
	cents    [6]int64
}

var spineRows = []spineRow{
	{vocab.KindRevenue, "spine-revenues", "taxes/property", "Property Taxes", [6]int64{6414376200, 0, 0, 0, 531565200, 0}},
	{vocab.KindRevenue, "spine-revenues", "taxes/other", "Other Taxes", [6]int64{2380019600, 0, 0, 0, 17000000, 0}},
	{vocab.KindRevenue, "spine-revenues", "intergovernmental", "Intergovernmental", [6]int64{432816400, 0, 818845300, 0, 682483500, 1884483400}},
	{vocab.KindRevenue, "spine-revenues", "charges-for-services", "Charges for Services", [6]int64{613020700, 6225082400, 152942900, 0, 212929700, 0}},
	{vocab.KindRevenue, "spine-revenues", "use-of-money-and-property", "Use of Money And Property", [6]int64{848816800, 483653700, 134600000, 0, 74113200, 12500000}},
	{vocab.KindRevenue, "spine-revenues", "contributions-outsourced", "Contributions Outsourced", [6]int64{7636000, 0, 0, 0, 193400000, 0}},
	{vocab.KindRevenue, "spine-revenues", "miscellaneous-revenue", "Miscellaneous Revenue", [6]int64{218070800, 42690000, 1608200000, 0, 1135064400, 0}},
	{vocab.KindRevenue, "spine-revenues", "taxes/sales", "Sales Taxes", [6]int64{4108660600, 0, 0, 0, 0, 0}},
	{vocab.KindRevenue, "spine-revenues", "fines-and-forfeitures", "Fines & Forfeitures", [6]int64{38650000, 0, 0, 0, 0, 0}},
	{vocab.KindRevenue, "spine-revenues", "licenses-and-permits", "Licenses & Permits", [6]int64{725279900, 0, 0, 0, 0, 0}},
	{vocab.KindExpenditure, "spine-expenditures", "wages-and-benefits", "Wages & Benefits", [6]int64{8180101100, 1606653600, 18620500, 0, 371671700, 597065400}},
	{vocab.KindExpenditure, "spine-expenditures", "services-and-supplies", "Services & Supplies", [6]int64{6284979100, 3959324000, 87515000, 0, 1471493900, 1548918900}},
	{vocab.KindExpenditure, "spine-expenditures", "capital-outlay", "Capital Outlay", [6]int64{0, 69990000, 0, 0, 60000000, 254702400}},
	{vocab.KindExpenditure, "spine-expenditures", "debt-services", "Debt Services", [6]int64{0, 69405400, 0, 698459700, 23590500, 107050000}},
	{vocab.KindTransferIn, "spine-transfers-in", "transfers/in", "TRANSFER IN:", [6]int64{48040000, 1324700000, 0, 698459700, 81400000, 0}},
	{vocab.KindTransferOut, "spine-transfers-out", "transfers/out", "TRANSFER OUT:", [6]int64{1003779700, 1981314700, 2858474000, 0, 113705000, 4000000}},
	{vocab.KindFundBalance, "spine-fund-balance", "fund-balance/reserve-increase", "ADDITION TO RESERVES", [6]int64{469942500, 0, 0, 0, 0, 0}},
	// Signed as printed: the General Fund's FY2026 CHANGE IN WORKING CAPITAL
	// is "(1,034,154)". Three of the six columns are negative, which is what
	// makes the fund-balance/draw side of the decomposition testable here.
	{vocab.KindFundBalance, "spine-fund-balance", "fund-balance/change", "CHANGE IN WORKING CAPITAL", [6]int64{-103415400, 389438400, -250021300, 0, 887494900, -614753300}},
	// Stocks, not flows. They are facts the city printed and they must survive
	// into facts.jsonl, but a balance carried into the year is not money moving
	// and gets no link. Debt Service carries a literal $2.
	{vocab.KindFundBalance, "spine-fund-balance", "fund-balance/beginning", "BEGINNING WORKING CAPITAL", [6]int64{176661300, 10339445000, 11071354500, 200, 7729007300, 2183726500}},
	{vocab.KindFundBalance, "spine-fund-balance", "fund-balance/ending", "ENDING WORKING CAPITAL", [6]int64{73245900, 10728883400, 10821333200, 200, 8616502200, 1568973200}},
}

// SpineFacts is the spine as facts for one fiscal year, every cell printed
// positive in dollars on its SpinePage.
func SpineFacts(year int) []fact.Fact {
	var out []fact.Fact
	for _, r := range spineRows {
		for i, cents := range r.cents {
			f := PrintedFact(Doc, r.rule, mapping.Row{Category: r.category}, r.label,
				mapping.Column{FundGroup: spineGroups[i]}, Scope, r.kind, year, Basis, cents)
			f.Page = SpinePage(spineGroups[i])
			out = append(out, f)
		}
	}
	return out
}

// PrintedFact is one figure of a test document at a row and a column, its id
// hashed from the same tuple the mapping engine hashes. It is positive and in
// dollars until the caller says otherwise.
func PrintedFact(doc, rule string, row mapping.Row, label string, col mapping.Column, scope string,
	kind vocab.Kind, year int, basis vocab.Basis, cents int64) fact.Fact {
	rowPath := fact.RowPath(row)
	columnPath := fact.ColumnPath(col, scope)
	f := fact.Fact{
		ID:          fact.MakeID(doc, rule, rowPath, label, columnPath, year, basis),
		DocID:       doc,
		RuleID:      rule,
		Kind:        kind,
		Basis:       basis,
		Scope:       scope,
		FiscalYear:  year,
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
	return f
}

// Version is the generated_by a document built over the spine carries.
const Version = "projecttest spine (Budget Book pp.66-67, FY2026, transcribed)"

// WithCIPLeg adds one p222 transfer to the CIP carrying FY2025-26's
// 38,086,737, what the committed store's p222 legs sum to, which the spine's
// caveat reads.
func WithCIPLeg(facts []fact.Fact) []fact.Fact {
	leg := facts[0]
	leg.ID, leg.Scope, leg.Kind, leg.AmountCents = "p222-leg", structure.ScopeCIPFundingSources, vocab.KindTransferOut, 3808673700
	return append(facts, leg)
}
