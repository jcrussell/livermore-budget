package check

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestDetailRestrictionAdmitsASetOfFundGroups covers the widening from one
// group to a set, which the two lanes that exist today do not exercise: one
// pins a single group and the other pins none.
//
// The capability is here for p76, whose OUT side is reconciled in two clauses
// over one scope -- the four groups pp.72-75 print a to-CIP figure for, and the
// two they print only an aggregate for (fisc-j2l). What is asserted is that the
// field is still a FILTER: it decides which facts are looked at, and nothing
// about which cells are compared.
func TestDetailRestrictionAdmitsASetOfFundGroups(t *testing.T) {
	f := func(kind mapping.Kind, group string) *fact.Fact {
		return &fact.Fact{Kind: kind, FundGroup: group}
	}

	pair := detailRestriction{
		Kinds:      []mapping.Kind{mapping.KindTransferOut},
		FundGroups: []string{"capital", "special-revenue"},
	}
	for _, g := range []string{"capital", "special-revenue"} {
		if !pair.admits(f(mapping.KindTransferOut, g)) {
			t.Errorf("a set of two groups does not admit %q", g)
		}
	}
	for _, g := range []string{"general", "enterprise", "internal-service", "debt-service"} {
		if pair.admits(f(mapping.KindTransferOut, g)) {
			t.Errorf("a set of {capital, special-revenue} admits %q", g)
		}
	}
	// The kind clause is unchanged and still ANDs with the groups.
	if pair.admits(f(mapping.KindTransferIn, "capital")) {
		t.Error("a transfer_in was admitted by a transfer_out restriction in an " +
			"admitted fund group; the two clauses must both hold")
	}

	// Empty still means "spans them all", which is a claim about the pages and
	// not a default -- pp.127-140 rely on it.
	all := detailRestriction{Kinds: []mapping.Kind{mapping.KindRevenue}}
	for _, g := range []string{"general", "capital", "permanent", ""} {
		if !all.admits(f(mapping.KindRevenue, g)) {
			t.Errorf("an empty FundGroups did not admit %q", g)
		}
	}
}
