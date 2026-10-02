package structure

import (
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// The four fund-balance rows pp.66-67 print, by category. Change is a flow;
// beginning and ending are stocks; reserve increase is money set aside.
const (
	CategoryFundBalanceChange          = "fund-balance/change"
	CategoryFundBalanceBeginning       = "fund-balance/beginning"
	CategoryFundBalanceEnding          = "fund-balance/ending"
	CategoryFundBalanceReserveIncrease = "fund-balance/reserve-increase"
)

// SourcesUses is one balance's terms of pp.66-67's identity: revenue +
// transfers in - expenditure - transfers out - reserve increase is the change
// in working capital. Every transfer_out is a transfer out, pp.186-209's
// Transfers Out to CIP among them. Terms counts the facts booked to the left
// side and Changes the change rows; Beginning and Ending are the stocks.
type SourcesUses struct {
	Revenue, TransfersIn, Expenditure, TransfersOut, Reserve, Change int64
	Beginning, Ending                                                int64
	Terms, Changes                                                   int
}

// Add books one fact as its term. It reports false for a fact that is no term
// of the identity; a stock is a term of nothing and is booked as a stock.
func (s *SourcesUses) Add(f *fact.Fact) bool {
	switch {
	case f.Kind == mapping.KindRevenue:
		s.Revenue += f.AmountCents
	case f.Kind == mapping.KindExpenditure:
		s.Expenditure += f.AmountCents
	case f.Kind == mapping.KindTransferIn:
		s.TransfersIn += f.AmountCents
	case f.Kind == mapping.KindTransferOut:
		s.TransfersOut += f.AmountCents
	case f.Category == CategoryFundBalanceReserveIncrease:
		s.Reserve += f.AmountCents
	case f.Category == CategoryFundBalanceChange:
		s.Change += f.AmountCents
		s.Changes++
		return true
	case f.Category == CategoryFundBalanceBeginning:
		s.Beginning += f.AmountCents
		return true
	case f.Category == CategoryFundBalanceEnding:
		s.Ending += f.AmountCents
		return true
	default:
		return false
	}
	s.Terms++
	return true
}

// Net is the left side of the identity, which Change must equal.
func (s SourcesUses) Net() int64 {
	return s.Revenue + s.TransfersIn - s.Expenditure - s.TransfersOut - s.Reserve
}

// Sources and Uses are the two sides pp.66-67 print a control total for,
// TOTAL SOURCES and TOTAL USES: a draw on balance is a source and a
// contribution to it a use, so the two are equal where the identity holds.
func (s SourcesUses) Sources() int64 { return s.Revenue + s.TransfersIn + max(0, -s.Change) }

// Uses is Sources' other side.
func (s SourcesUses) Uses() int64 {
	return s.Expenditure + s.TransfersOut + s.Reserve + max(0, s.Change)
}
