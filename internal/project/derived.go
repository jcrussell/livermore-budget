package project

import (
	"errors"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// This file is where the flow endpoints the documents draw outside their
// hierarchy are spelled in node ids, and where a link at one is read in cents.

// flowEndpoint is one flow endpoint: the node id, the kind of money it ends,
// the category whose cell draws it, and whether the citywide spine draws it.
type flowEndpoint struct {
	id       string
	kind     vocab.Kind
	category string
	tier     int
	// spine is whether sankey.Document draws this endpoint; the others are
	// drawn only by the schedules beside it.
	spine bool
}

// flowEndpoints are the nodes the documents draw outside their hierarchy: a
// transfer's end at the category's own id, the reserve row at its own, and
// the two halves of a change in balance. A test holds the spine's to the
// spine's golden.
var flowEndpoints = []flowEndpoint{
	{NodeTransfersIn, vocab.KindTransferIn, NodeTransfersIn, tierRevenueSource, true},
	{NodeTransfersOut, vocab.KindTransferOut, NodeTransfersOut, tierObjectCategory, true},
	{CategoryFundBalanceReserveIncrease, vocab.KindFundBalance, CategoryFundBalanceReserveIncrease, tierObjectCategory, true},
	{NodeFundBalanceDraw, vocab.KindFundBalance, CategoryFundBalanceChange, tierRevenueSource, true},
	{NodeFundBalanceContribution, vocab.KindFundBalance, CategoryFundBalanceChange, tierObjectCategory, true},
	{NodeTransfersOutToCIP, vocab.KindTransferOut, NodeTransfersOutToCIP, tierObjectCategory, false},
}

// spineEndpoints is the flow endpoints the citywide spine draws.
func spineEndpoints() []flowEndpoint {
	var out []flowEndpoint
	for _, e := range flowEndpoints {
		if e.spine {
			out = append(out, e)
		}
	}
	return out
}

// EndpointCategories is each set of categories the facts behind a flow
// endpoint may carry, every fact of one link within one set, and whether id is
// one. A change in balance is read either off a printed change row or off the
// two balances it is the difference of; [ChangeCents] is its value either way.
func EndpointCategories(id string) ([][]string, bool) {
	for _, e := range flowEndpoints {
		if e.id != id {
			continue
		}
		out := [][]string{{e.category}}
		if e.category == CategoryFundBalanceChange {
			out = append(out, []string{CategoryFundBalanceBeginning, CategoryFundBalanceEnding})
		}
		return out, true
	}
	return nil, false
}

// LinkCents is the value a link's cited facts come to: their plain sum, except
// on a link at a change endpoint, which carries the change they state
// ([ChangeCents]), negated on a draw. Only those links may subtract a balance,
// so a link elsewhere citing a beginning balance beside its own fact does not
// tie by netting it off.
//
// A link at a remainder node carries the difference of a printed cell and the
// members it is not: the spine's facts it cites, read so, less the rest.
func LinkCents(source, target string, facts []fact.Fact) int64 {
	if IsRemainder(source) || IsRemainder(target) {
		var printed, members []fact.Fact
		for i := range facts {
			if facts[i].Scope == PublishedScope {
				printed = append(printed, facts[i])
			} else {
				members = append(members, facts[i])
			}
		}
		return endCents(source, target, printed) - endCents(source, target, members)
	}
	return endCents(source, target, facts)
}

// endCents is LinkCents' reading of one set of facts at a link's ends.
func endCents(source, target string, facts []fact.Fact) int64 {
	switch {
	case source == NodeFundBalanceDraw:
		return -ChangeCents(facts)
	case target == NodeFundBalanceContribution:
		return ChangeCents(facts)
	}
	var sum int64
	for i := range facts {
		sum += facts[i].AmountCents
	}
	return sum
}

// ChangeCents is the change in balance a set of facts states: each beginning
// balance subtracted, every other fact added as printed.
func ChangeCents(facts []fact.Fact) int64 {
	var sum int64
	for i := range facts {
		if facts[i].Kind == vocab.KindFundBalance && facts[i].Category == CategoryFundBalanceBeginning {
			sum -= facts[i].AmountCents
			continue
		}
		sum += facts[i].AmountCents
	}
	return sum
}

// referenceCut is the cut every derived licence is against.
func referenceCut() (structure.Cut, error) {
	reference, ok := structure.Reference(structure.AllCuts())
	if !ok {
		return structure.Cut{}, errors.New("project: no declared cut is the reference, so nothing says what a derived licence is against")
	}
	return reference, nil
}
