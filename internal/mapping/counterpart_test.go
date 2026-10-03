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
			// THE RULE NEEDS A PART. checkCounterpart's collision arm walks the
			// columns of every part, because a collision is per cell; a
			// hand-built Rule{} with no parts has no cell for it to find.
			// validateRule refuses a partless rule before it ever calls this,
			// so a no-part fixture was testing a state the parser cannot
			// produce -- and the row-only fallback that made it pass was itself
			// a false refusal on omitted rows and skipped columns.
			rule := &Rule{ID: "r", Kind: KindTransferIn,
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

	// THE ACCEPTED CASES CONSTRAIN ONE CLAUSE EACH, and they have to, because a
	// counterpart differing on all three axes exits at the category
	// short-circuit and never reaches the per-column loop at all. Measured: with
	// only such a case here, dropping `cp.Fund == near.Fund` from the loop left
	// the whole package green while the arm would have started refusing a
	// legitimate same-group, different-fund counterpart.
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
	return &parseError{Path: "counterpart_test", RuleID: ruleID, Field: field,
		Msg: fmt.Sprintf(format, args...)}
}

// TestTheIdenticalLegsArmIgnoresCellsThatPublishNothing is the second review
// pass over the guards lane, and it guards against a FALSE REFUSAL the fix for
// fisc-i38 introduced.
//
// That fix made the arm compare the counterpart against every column of every
// part. "Every" was too many: a skip: true column consumes its position and
// yields no fact, and a part whose omitted_rows drop this row prints no cell for
// it at all. Neither can collide with anything, because neither publishes
// anything -- so refusing on one is a refusal against a cell the document does
// not have.
//
// THE ROW DECLARES NO FUND HERE, and it has to be that way for the case to
// exist: Row.EffectiveColumn lets a row's own fund override the column's, so a
// row declaring fund 100 can never collide with a column declaring 200 whatever
// the counterpart says. The collision this arm is for needs the fund to come
// from the COLUMN -- which is the shape of every schedule that maps one fund per
// column, pp.127-140 among them.
//
// Latent on the committed corpus, since no skipped column there carries a fund.
// Reproduced through Parse before being fixed, which is why this goes through
// Parse rather than calling checkCounterpart with a hand-built Rule.
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

	// THE OTHER EXEMPTION: a part that does not print this row. Its columns are
	// live, and the row has no cell under any of them, so a counterpart matching
	// one of them collides with nothing. Written separately because the two arms
	// are independent -- removing the omitted-rows arm left the whole package
	// green until this case existed.
	const omitted = `schema_version: 1
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
        omitted_rows: [{label: "Transfer From Low Income Hsng"}]
        columns:
          - {fund_group: special-revenue, fund: 200, fiscal_year: 2026}
    rows:
      - label: "Transfer From Low Income Hsng"
        category: transfers/in
        counterpart: {category: transfers/in, kind: transfer_out, fund: 200, fund_group: special-revenue}
      - label: "Transfer From Water"
        category: transfers/in
`
	if _, err := parse(strings.NewReader(omitted), "omitted.yaml"); err != nil {
		t.Errorf("a counterpart colliding only with a column of a part that OMITS "+
			"this row was refused: %v\nthe row has no cell there to collide with", err)
	}

	// A row omitted from its ONLY part publishes no cell anywhere, so its
	// counterpart is refused for that, above this arm, and never as a
	// collision: the arm reads no cell of it.
	const omittedEverywhere = `schema_version: 1
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
        omitted_rows: [{label: "Transfer From Low Income Hsng"}]
        columns:
          - {fund_group: general, fiscal_year: 2026}
    rows:
      - label: "Transfer From Low Income Hsng"
        category: transfers/in
        fund: 200
        fund_group: special-revenue
        counterpart: {category: transfers/in, kind: transfer_out, fund: 200, fund_group: special-revenue}
      - label: "Transfer From Water"
        category: transfers/in
`
	_, everywhereErr := parse(strings.NewReader(omittedEverywhere), "everywhere.yaml")
	if everywhereErr == nil || !strings.Contains(everywhereErr.Error(), "counterpart on a row that publishes no cell") {
		t.Errorf("a counterpart on a row omitted from its ONLY part: %v\n"+
			"want it refused for publishing no cell, not as a collision", everywhereErr)
	}

	// And with the omission removed, p77's column is live for this row and the
	// same counterpart is refused -- so the exemption turns on the declaration
	// rather than on the second part existing at all.
	present := strings.Replace(omitted,
		"        omitted_rows: [{label: \"Transfer From Low Income Hsng\"}]\n", "", 1)
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
