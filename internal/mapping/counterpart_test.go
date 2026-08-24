package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// A counterpart is the far end of a figure that moves money between two funds:
// one printed number that is 257,012 leaving fund 200 AND 257,012 arriving at
// fund 100. Before it, the engine could not express that at all -- the resolver
// yields one Value per printed token and fact.FromValues built one Fact per
// Value, an unbranched chain with no fan-out anywhere in it. fisc-4rh decided
// the two-fact SHAPE and checked it against fact.MakeID; nothing checked
// whether the engine could emit two facts, and it could not.
//
// The tests below are in two halves. The first is what the rule file refuses.
// The second is the one that matters and is not obvious: THE DOCUMENT CANNOT
// CHECK A COUNTERPART, so every guard on one has to be somewhere else.

// TestACounterpartIsRefusedWhenItCouldNotBeToldApart covers the four arms of
// checkCounterpart, each of which is a way to declare a far leg that is not one.
func TestACounterpartIsRefusedWhenItCouldNotBeToldApart(t *testing.T) {
	base := func() Row {
		return Row{
			Label: "Transfer From Low Income Hsng", LabelTail: "to General Fund",
			Category: "transfers/in", Fund: 100, FundGroup: "general",
			Counterpart: &Counterpart{
				Category: "transfers/out", Kind: KindTransferOut,
				Fund: 200, FundGroup: "special-revenue",
			},
		}
	}
	tests := []struct {
		name string
		mut  func(*Row)
		want string
	}{
		{"no category", func(r *Row) { r.Counterpart.Category = "" }, "counterpart has no category"},
		{"no kind", func(r *Row) { r.Counterpart.Kind = "" }, "counterpart kind"},
		{"bad kind", func(r *Row) { r.Counterpart.Kind = "transfer" }, "counterpart kind"},
		{"no fund group", func(r *Row) { r.Counterpart.FundGroup = "" }, "counterpart has no fund_group"},
		// A GROUP ALONE IS NOT ENOUGH, and nothing downstream would say so:
		// fact.FromValues' guard fires only when both are absent, and
		// row-funds-match-their-anchors has no number to compare its printed
		// anchor against, so the leg would publish fund 0 with every check
		// green. The near leg is deliberately different -- p76's LAVWMA row
		// receives into a joint powers authority that is no City fund.
		{"a group but no fund", func(r *Row) { r.Counterpart.Fund = 0 },
			"counterpart declares fund_group"},
		{"skipped row", func(r *Row) { r.Skip = true }, "declares a counterpart"},
		{"row with a department", func(r *Row) { r.Department = "police" }, "carries department"},
		{"same category and fund", func(r *Row) {
			r.Counterpart.Category = r.Category
			r.Counterpart.Fund, r.Counterpart.FundGroup = r.Fund, r.FundGroup
		}, "same category and the same fund"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := base()
			tt.mut(&row)
			rule := &Rule{ID: "r", Kind: KindTransferIn}
			err := checkCounterpart(rule, row, errfLike)
			if err == nil {
				t.Fatalf("no error; a counterpart that is %s must be refused", tt.name)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}

	// And the shape that is correct is accepted, so the arms above are
	// rejecting the defect rather than the feature.
	if err := checkCounterpart(&Rule{ID: "r"}, base(), errfLike); err != nil {
		t.Errorf("a well-formed counterpart was refused: %v", err)
	}
}

// TestTheDocumentCannotCheckACounterpart is the failability proof, and it is
// written the way this project's evidence has to be written: by running the
// real page and showing WHAT STILL PASSES.
//
// The claim is about placement. The fan-out lives in fact.FromValues, which is
// downstream of everything that compares our read against the city's own
// arithmetic -- StatedTotals, CheckTotals, stated_total_deltas, rollups, the
// gap and unmapped-rest guards. All of those consume []Value. So a counterpart
// declared under the WRONG fund entirely leaves every one of them byte for byte
// unchanged, and the page goes on tying to its own printed total.
//
// That cuts both ways and both halves are worth stating. It is why a
// counterpart cannot BREAK a schedule's tie to its document, which is what
// makes the seam safe. It is also why a schedule that uses one owes a
// reconciliation against something OUTSIDE its own page -- for p76 that is
// pp.66-67's TRANSFER OUT row (fisc-aes), and without it the payer side would
// be a hand-typed claim no arithmetic touches.
func TestTheDocumentCannotCheckACounterpart(t *testing.T) {
	read := func(t *testing.T, withCounterparts bool) ([]Value, amount.Cents) {
		t.Helper()
		f, err := Load("testdata/transfers-p76.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if withCounterparts {
			for i := range f.Rules {
				for j := range f.Rules[i].Rows {
					// Deliberately absurd: every payer on the page declared as
					// the same wrong fund. If the document could see a
					// counterpart at all, this would be caught.
					f.Rules[i].Rows[j].Counterpart = &Counterpart{
						Category: "transfers/out", Kind: KindTransferOut,
						Fund: 999, FundGroup: "internal-service",
					}
				}
			}
		}
		r, err := NewResolver(budgetDoc(t, p76Page), f)
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		var all []Value
		var total amount.Cents
		for i := range f.Rules {
			rule := &f.Rules[i]
			vals, _, err := r.Values(rule, &rule.Parts[0])
			if err != nil {
				t.Fatalf("rule %s: %v", rule.ID, err)
			}
			all = append(all, vals...)
			for _, v := range vals {
				if v.Column.FiscalYear == 2026 {
					total += v.Cents
				}
			}
		}
		return all, total
	}

	plain, plainTotal := read(t, false)
	counterparted, cpTotal := read(t, true)

	if len(plain) == 0 {
		t.Fatal("p76 produced no values, so this test compares nothing")
	}
	if len(plain) != len(counterparted) {
		t.Fatalf("the resolver yielded %d values without counterparts and %d with; "+
			"the fan-out has moved upstream of Values and every printed total "+
			"in the corpus is now counting facts instead of figures",
			len(plain), len(counterparted))
	}
	for i := range plain {
		if plain[i].Cents != counterparted[i].Cents ||
			plain[i].Offset != counterparted[i].Offset ||
			plain[i].ColumnIndex != counterparted[i].ColumnIndex {
			t.Fatalf("value %d differs once a counterpart is declared: %+v vs %+v",
				i, plain[i], counterparted[i])
		}
	}

	// The page's own printed grand total, unmoved. p0076.txt prints
	// $21,525,997 beneath the last section and the read reproduces it to the
	// cent either way -- with every payer on the page declared as fund 999.
	if plainTotal != p76StatedFY2026 || cpTotal != p76StatedFY2026 {
		t.Errorf("FY2026 column sums to %s without counterparts and %s with, "+
			"want the printed %s in both", plainTotal, cpTotal, p76StatedFY2026)
	}
}

// errfLike is the error shape validate's closure produces, spelled here so the
// counterpart arms can be exercised without parsing a whole file.
func errfLike(ruleID, field, format string, args ...any) error {
	return &ParseError{Path: "counterpart_test", RuleID: ruleID, Field: field,
		Msg: fmt.Sprintf(format, args...)}
}
