package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// A counterpart is the far end of a transfer: one printed figure that leaves
// one fund and arrives at another, emitted as two facts. The first half below
// is what the rule file refuses; the second is that the document cannot check
// a counterpart, so every guard on one has to live elsewhere.

// TestACounterpartIsRefusedWhenItCouldNotBeToldApart covers the four arms of
// checkCounterpart, each of which is a way to declare a far leg that is not one.
func TestACounterpartIsRefusedWhenItCouldNotBeToldApart(t *testing.T) {
	base := func() Row {
		return Row{
			Label: "Transfer From Low Income Hsng", LabelTail: "to General Fund",
			Category: "transfers/in", Fund: 100, FundGroup: "general",
			Counterpart: &Counterpart{
				Category: "transfers/out", Kind: vocab.KindTransferOut,
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
		// A group alone publishes fund 0 with every check green: the fan-out
		// guard fires only when both are absent. The near leg may lack a fund
		// (p76's LAVWMA row receives into a JPA).
		{"a group but no fund", func(r *Row) { r.Counterpart.Fund = 0 },
			"counterpart declares fund_group"},
		{"row with a department", func(r *Row) { r.Department = "police" }, "carries department"},
		{"row with a contra sign", func(r *Row) { r.Sign = vocab.SignContra }, "carries sign"},
		{"same category and fund", func(r *Row) {
			r.Counterpart.Category = r.Category
			r.Counterpart.Fund, r.Counterpart.FundGroup = r.Fund, r.FundGroup
		}, "same category and the same fund"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := base()
			tt.mut(&row)
			// The collision arm walks each part's columns, and validateRule
			// refuses a partless rule first, so the fixture needs a part.
			rule := &Rule{ID: "r", Kind: vocab.KindTransferIn,
				Parts: []Part{{Page: 76, Columns: []Column{{FiscalYear: 2026}}}}}
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
	wellFormed := &Rule{ID: "r",
		Parts: []Part{{Page: 76, Columns: []Column{{FiscalYear: 2026}}}}}
	if err := checkCounterpart(wellFormed, base(), errfLike); err != nil {
		t.Errorf("a well-formed counterpart was refused: %v", err)
	}
}

// TestTheIdenticalLegsArmFiresOnTheShapeEveryPublishedRuleUses holds the p76
// shape: the group lives on the COLUMN and the row declares only a fund.
// Mutation: compare against row.EffectiveColumn(Column{}) and the arm never
// fires; the duplicate surfaces in `fisc build` as "id ... claimed twice".
func TestTheIdenticalLegsArmFiresOnTheShapeEveryPublishedRuleUses(t *testing.T) {
	// Group on the COLUMN, fund on the ROW -- p76 exactly.
	rule := &Rule{
		ID: "p76-transfers-in-general", Kind: vocab.KindTransferIn,
		Parts: []Part{{
			Page: 76,
			Columns: []Column{
				{FundGroup: "general", FiscalYear: 2026},
				{FundGroup: "general", FiscalYear: 2027},
			},
		}},
	}
	// The counterpart repeats the near leg on both identity axes, category and
	// fund -- copying the row and changing only the kind.
	row := Row{
		Label: "Transfer From Low Income Hsng", LabelTail: "to General Fund",
		Category: "transfers/in", Fund: 100,
		Counterpart: &Counterpart{
			Category: "transfers/in", Kind: vocab.KindTransferOut,
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

	// Each accepted case constrains one clause: a counterpart differing on all
	// three axes exits at the category short-circuit. Mutation: drop
	// `cp.Fund == near.Fund` and only the same-group, different-fund case fails.
	for _, tt := range []struct {
		name string
		mut  func(*Counterpart)
	}{
		// Reaches the loop (same category) and must be accepted on the FUND.
		{"same category and group, a different fund", func(cp *Counterpart) { cp.Fund = 200 }},
		// Reaches the loop and must be accepted on the GROUP.
		{"same category and fund, a different group", func(cp *Counterpart) {
			cp.FundGroup = "special-revenue"
		}},
		// Exits at the short-circuit: the ordinary two-legged shape.
		{"a different category entirely", func(cp *Counterpart) {
			cp.Category = "transfers/out"
			cp.Fund, cp.FundGroup = 200, "special-revenue"
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accepted := row
			cp := *row.Counterpart
			tt.mut(&cp)
			accepted.Counterpart = &cp
			if err := checkCounterpart(rule, accepted, errfLike); err != nil {
				t.Errorf("a well-formed p76-shaped counterpart was refused: %v", err)
			}
		})
	}

	// The collision is per column: a counterpart matching only one column's
	// group still collides on that column's figure.
	twoGroups := &Rule{
		ID: "r", Kind: vocab.KindTransferIn,
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

// TestTheDocumentCannotCheckACounterpart runs the real page with a counterpart
// under the wrong fund and shows every total and rollup guard unchanged: the
// fan-out is downstream of everything that consumes []Value. So a schedule
// using one owes a reconciliation outside its page (p76 against pp.66-67's
// TRANSFER OUT row).
func TestTheDocumentCannotCheckACounterpart(t *testing.T) {
	read := func(t *testing.T, withCounterparts bool) ([]Value, amount.Cents) {
		t.Helper()
		f := loadPublishedP76(t)
		for i := range f.Rules {
			for j := range f.Rules[i].Rows {
				f.Rules[i].Rows[j].Counterpart = nil
				if withCounterparts {
					// Deliberately absurd: every payer on the page declared as
					// the same wrong fund. If the document could see a
					// counterpart at all, this would be caught.
					f.Rules[i].Rows[j].Counterpart = &Counterpart{
						Category: "transfers/out", Kind: vocab.KindTransferOut,
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
	return &parseError{Path: "counterpart_test", RuleID: ruleID, Field: field,
		Msg: fmt.Sprintf(format, args...)}
}

// TestTheIdenticalLegsArmIgnoresCellsThatPublishNothing holds that a skip: true
// column, or a part that does not print the row, cannot collide: neither
// publishes a cell. The row declares no fund so the fund comes from the column,
// as on pp.127-140. Goes through Parse, where the false refusal reproduced.
func TestTheIdenticalLegsArmIgnoresCellsThatPublishNothing(t *testing.T) {
	rule := func(secondColumn string) string {
		return `schema_version: 1
doc_id: livermore-budget-fy2026-2027
rules:
  - id: r
    kind: transfer_in
    basis: adopted
    grain: fund-by-category
    units: dollars
    parts:
      - page: 76
        section: "S"
        stop_at: "E"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2026}
          - ` + secondColumn + `
    rows:
      - label: "Transfer From Low Income Hsng"
        category: transfers/in
        counterpart: {category: transfers/in, kind: transfer_out, fund: 200, fund_group: special-revenue}
`
	}

	// The counterpart names fund 200 in special-revenue, and so does the second
	// column -- but that column is skipped, so it publishes nothing.
	skipped := rule("{fund_group: special-revenue, fund: 200, fiscal_year: 2026, skip: true}")
	if _, err := parse(strings.NewReader(skipped), "skip.yaml"); err != nil {
		t.Errorf("a counterpart colliding only with a SKIPPED column was refused: %v\n"+
			"a skipped column publishes no fact, so there is nothing to collide with", err)
	}

	// A non-amount column is read and publishes nothing, so a counterpart
	// naming its fund collides with nothing either.
	ratio := rule("{fund_group: special-revenue, fund: 200, fiscal_year: 2026, quantity: percentage}")
	if _, err := parse(strings.NewReader(ratio), "ratio.yaml"); err != nil {
		t.Errorf("a counterpart colliding only with a PERCENTAGE column was refused: %v\n"+
			"a non-amount column publishes no fact, so there is nothing to collide with", err)
	}

	// The same collision on a column the rule actually reads is still refused,
	// so the exemption above is about the CELL and not about the arm.
	live := rule("{fund_group: special-revenue, fund: 200, fiscal_year: 2026}")
	_, err := parse(strings.NewReader(live), "live.yaml")
	if err == nil {
		t.Fatal("a counterpart duplicating a LIVE column was accepted")
	}
	if !strings.Contains(err.Error(), "column 2") {
		t.Errorf("error %q does not name the colliding column", err)
	}

	// The other exemption: a part that does not print this row. Mutation:
	// remove the placement arm and only this case fails.
	const placed = `schema_version: 1
doc_id: livermore-budget-fy2026-2027
rules:
  - id: r
    kind: transfer_in
    basis: adopted
    grain: fund-by-category
    units: dollars
    parts:
      - page: 76
        section: "S"
        stop_at: "E"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2026}
      - page: 77
        section: "S"
        stop_at: "E"
        columns:
          - {fund_group: special-revenue, fund: 200, fiscal_year: 2026}
    rows:
      - label: "Transfer From Low Income Hsng"
        page: 76
        category: transfers/in
        counterpart: {category: transfers/in, kind: transfer_out, fund: 200, fund_group: special-revenue}
      - label: "Transfer From Water"
        category: transfers/in
`
	if _, err := parse(strings.NewReader(placed), "placed.yaml"); err != nil {
		t.Errorf("a counterpart colliding only with a column of a part that does not "+
			"print this row was refused: %v\nthe row has no cell there to collide with", err)
	}

	// And with the placement removed, p77's column is live for this row and
	// the same counterpart is refused -- so the exemption turns on the
	// declaration rather than on the second part existing at all.
	present := strings.Replace(placed, "        page: 76\n", "", 1)
	_, presentErr := parse(strings.NewReader(present), "present.yaml")
	if presentErr == nil {
		t.Fatal("a counterpart duplicating a column of a part that PRINTS this row " +
			"was accepted")
	}
	// Pinned to the identical-legs message, not merely to "some error": without
	// this the case stays green on any unrelated future parse failure, long
	// after the arm it is named for has stopped firing.
	if !strings.Contains(presentErr.Error(), "same fund as the row itself") ||
		!strings.Contains(presentErr.Error(), "page 77") {
		t.Errorf("error %q is not the identical-legs refusal on p77", presentErr)
	}
}
