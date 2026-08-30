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

// TestTheIdenticalLegsArmFiresOnTheShapeEveryPublishedRuleUses is fisc-i38, and
// the reason it needed its own test is that the case above passes for the wrong
// reason.
//
// base() declares FundGroup ON THE ROW. No published rule does: on Budget Book
// p76 a SECTION is the receiving fund group, so the group lives on the COLUMN
// and the row declares only a fund. The arm compared the counterpart against
// row.EffectiveColumn(Column{}) -- the row's own fund and group with no printed
// column behind them -- and two guards above require cp.FundGroup != "", so the
// group clause could only hold when the row also declared a group. On all 22 of
// p76's rows it was false, and the arm was unreachable while its own comment
// claimed it caught "the one an author writing a two-legged row actually makes".
//
// The fixture here is the p76 shape. Under the old comparison it parsed clean
// and failed much later in `fisc build` as "id ... claimed twice: rule X ...
// rule X", which does not read as "your counterpart duplicates its own near
// leg".
func TestTheIdenticalLegsArmFiresOnTheShapeEveryPublishedRuleUses(t *testing.T) {
	// Group on the COLUMN, fund on the ROW -- p76 exactly.
	rule := &Rule{
		ID: "p76-transfers-in-general", Kind: KindTransferIn,
		Parts: []Part{{
			Page: 76,
			Columns: []Column{
				{FundGroup: "general", FiscalYear: 2026},
				{FundGroup: "general", FiscalYear: 2027},
			},
		}},
	}
	// The counterpart repeats the near leg on BOTH axes a fact's identity is
	// built from: the category (its row path) and the fund (its column path).
	// This is the mistake an author writing a two-legged row makes -- copying
	// the row and changing only the kind, which is in neither path.
	row := Row{
		Label: "Transfer From Low Income Hsng", LabelTail: "to General Fund",
		Category: "transfers/in", Fund: 100,
		Counterpart: &Counterpart{
			Category: "transfers/in", Kind: KindTransferOut,
			Fund: 100, FundGroup: "general",
		},
	}

	// The row declares NO fund group, which is the whole point: the old
	// comparison had nothing to match cp.FundGroup against.
	if row.FundGroup != "" {
		t.Fatal("the fixture declares a fund group on the row, so it does not " +
			"reproduce the shape this test is about")
	}

	err := checkCounterpart(rule, row, errfLike)
	if err == nil {
		t.Fatal("a counterpart duplicating its own near leg was accepted; the " +
			"identical-legs arm is unreachable again")
	}
	for _, want := range []string{"same category and the same fund", "column 1", "page 76"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q; the message has to name the "+
				"column, because the collision is per column", err, want)
		}
	}

	// A counterpart that differs from every column is still accepted: the arm
	// must reject the collision, not the shape.
	ok := row
	cp := *row.Counterpart
	cp.Category = "transfers/out"
	cp.Fund, cp.FundGroup = 200, "special-revenue"
	ok.Counterpart = &cp
	if err := checkCounterpart(rule, ok, errfLike); err != nil {
		t.Errorf("a well-formed p76-shaped counterpart was refused: %v", err)
	}

	// AND THE COLLISION IS PER COLUMN, not per rule: a counterpart matching the
	// group of only ONE of a rule's columns still collides on that column's
	// figure, and a check that compared against the first column alone would
	// miss it.
	twoGroups := &Rule{
		ID: "r", Kind: KindTransferIn,
		Parts: []Part{{Page: 76, Columns: []Column{
			{FundGroup: "enterprise", FiscalYear: 2026},
			{FundGroup: "general", FiscalYear: 2026},
		}}},
	}
	if err := checkCounterpart(twoGroups, row, errfLike); err == nil {
		t.Error("a counterpart colliding on the second column only was accepted")
	} else if !strings.Contains(err.Error(), "column 2") {
		t.Errorf("error %q does not name column 2", err)
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
