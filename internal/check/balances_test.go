package check

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// balanceLine is one printed cell of a hand-built balance: its kind, its
// category, and what it says.
type balanceLine struct {
	kind     mapping.Kind
	category string
	cents    int64
}

// byFundRow is one pp.186-209 row as the scope publishes it: the six flows
// and the two stocks, and no change line. It balances:
// 1,000 + 500 + 100 - 300 - 50 - 20 - 30 = 1,200.
func byFundRow() []balanceLine {
	return []balanceLine{
		{mapping.KindFundBalance, structure.CategoryFundBalanceBeginning, 100_000},
		{mapping.KindRevenue, "taxes", 50_000},
		{mapping.KindTransferIn, "transfers/in", 10_000},
		{mapping.KindExpenditure, "capital-projects", 30_000},
		{mapping.KindTransferOut, "transfers/out", 5_000},
		{mapping.KindTransferOut, "transfers/out-to-cip", 2_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceReserveIncrease, 3_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceEnding, 120_000},
	}
}

// balanceAt is where a hand-built balance sits.
type balanceAt struct {
	scope, group string
	fund         int
	year         int
	basis        mapping.Basis
}

// facts renders lines at one place, each with an id from the tuple `fisc
// build` hashes, so a finding naming one names one that could exist.
func (at balanceAt) facts(lines []balanceLine) []fact.Fact {
	var out []fact.Fact
	for _, l := range lines {
		rule := "balance-" + at.scope
		rowPath := fact.RowPath(mapping.Row{Category: l.category})
		col := mapping.Column{FundGroup: at.group, Fund: at.fund}
		colPath := fact.ColumnPath(col, at.scope)
		f := fact.Fact{
			ID:          fact.MakeID(testDoc, rule, rowPath, l.category, colPath, at.year, at.basis),
			DocID:       testDoc,
			RuleID:      rule,
			Kind:        l.kind,
			Basis:       at.basis,
			Scope:       at.scope,
			FiscalYear:  at.year,
			RowPath:     rowPath,
			RowLabel:    l.category,
			Category:    l.category,
			ColumnPath:  colPath,
			FundGroup:   at.group,
			AmountCents: l.cents,
		}
		if at.fund != 0 {
			n := at.fund
			f.Fund = &n
		}
		out = append(out, f)
	}
	return out
}

// fund101 is General Fund CIP Reserves' FY2026 row.
var fund101 = balanceAt{structure.ScopeFundBalancesByFund, "capital", 101, 2026, mapping.BasisAdopted}

// runBalance runs one check over hand-built facts and nothing else: both
// balance checks read the facts and the rule files and no other input.
//
// THE TREE'S EXCEPTIONS ARE WITHHELD unless the test declares its own: they
// name pp.186-209's balances, on the scope these facts are built on, so over
// a hand-built store every one of them is stale and a finding.
func runBalance(t *testing.T, c Check, facts []fact.Fact, files ...*mapping.File) Result {
	t.Helper()
	if !declaredBalanceExceptions {
		prev := balanceExceptions
		balanceExceptions = func() []structure.BalanceException { return nil }
		defer func() { balanceExceptions = prev }()
	}
	res, err := c.Run(t.Context(), &Subject{Facts: facts, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// said is a result's findings as one string, for a test asserting on prose.
func said(res Result) string { return strings.Join(findingLines(res), "\n") }

func wantPass(t *testing.T, res Result, subjects int) {
	t.Helper()
	if res.Status != StatusPass || res.Subjects != subjects {
		t.Fatalf("%s over %d (%s), findings %s; want pass over %d",
			res.Status, res.Subjects, res.Summary, said(res), subjects)
	}
}

func wantFail(t *testing.T, res Result, want ...string) {
	t.Helper()
	got := said(res)
	if res.Status != StatusFail {
		t.Fatalf("%s (%s), findings %s; want fail saying %q", res.Status, res.Summary, got, want)
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("findings %s do not say %q", got, w)
		}
	}
}

// replace returns lines with the one of category c set to cents.
func replace(lines []balanceLine, c string, cents int64) []balanceLine {
	out := slices.Clone(lines)
	for i := range out {
		if out[i].category == c {
			out[i].cents = cents
		}
	}
	return out
}

// without returns lines less the one of category c.
func without(lines []balanceLine, c string) []balanceLine {
	return slices.DeleteFunc(slices.Clone(lines), func(l balanceLine) bool { return l.category == c })
}

func TestAFundRowThatBalancesPassesBothIdentities(t *testing.T) {
	facts := fund101.facts(byFundRow())
	wantPass(t, runBalance(t, &fundGroupSourcesEqualUses{}, facts), 1)
	wantPass(t, runBalance(t, &fundBalanceIdentity{}, facts), 1)
}

// TestAFundRowOffByACentFails is the arithmetic of a scope that prints no
// change line: the change is ending - beginning, two printed figures.
func TestAFundRowOffByACentFails(t *testing.T) {
	facts := fund101.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 120_001))
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, facts), "capital fund 101 FY2026 adopted", "(off by -$0.01)")
}

// TestTransfersOutToCIPIsAUse holds that SourcesUses.Add books
// transfers/out-to-cip as a transfer out: drop it from the ending and the row
// is off by exactly its 20.00.
func TestTransfersOutToCIPIsAUse(t *testing.T) {
	facts := fund101.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 122_000))
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, facts), "(off by -$20.00)")
}

func TestAFundRowCarryingAChangeLineFails(t *testing.T) {
	lines := append(byFundRow(), balanceLine{mapping.KindFundBalance, structure.CategoryFundBalanceChange, 20_000})
	wantFail(t, runBalance(t, &fundBalanceIdentity{}, fund101.facts(lines)), structure.CategoryFundBalanceChange)
}

func TestAFundRowMissingAStockFails(t *testing.T) {
	for _, c := range []string{structure.CategoryFundBalanceBeginning, structure.CategoryFundBalanceEnding} {
		t.Run(c, func(t *testing.T) {
			facts := fund101.facts(without(byFundRow(), c))
			wantFail(t, runBalance(t, &fundBalanceIdentity{}, facts), "missing "+c)
			wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, facts), "missing "+c)
		})
	}
}

// TestAFundRowMissingAFlowFails is "carries any line, must carry all" for a
// flow: a dropped Transfers Out to CIP column leaves the arithmetic holding
// wherever the page prints a dash, so absence itself is the finding.
func TestAFundRowMissingAFlowFails(t *testing.T) {
	facts := fund101.facts(without(byFundRow(), "transfers/out-to-cip"))
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, facts), "missing transfer_out transfers/out-to-cip")
}

// TestTwoFundsOfOneGroupAreTwoBalances is why the fund is in the key: fund
// 101 a cent long and fund 102 a cent short net to nothing as one group.
func TestTwoFundsOfOneGroupAreTwoBalances(t *testing.T) {
	fund102 := fund101
	fund102.fund = 102
	facts := append(fund101.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 120_001)),
		fund102.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 119_999))...)
	res := runBalance(t, &fundGroupSourcesEqualUses{}, facts)
	wantFail(t, res, "capital fund 101 FY2026 adopted", "capital fund 102 FY2026 adopted")
	if len(res.Findings) != 2 {
		t.Errorf("findings %s, want one per fund", said(res))
	}
	wantPass(t, runBalance(t, &fundBalanceIdentity{}, facts), 2)
}

// twoYears is fund 101 in FY2026 and FY2027, the second year beginning where
// the first ends.
func twoYears(next int64) []fact.Fact {
	fy2027 := fund101
	fy2027.year = 2027
	row := replace(replace(byFundRow(), structure.CategoryFundBalanceBeginning, next),
		structure.CategoryFundBalanceEnding, next+20_000)
	return append(fund101.facts(byFundRow()), fy2027.facts(row)...)
}

func TestACarryForwardThatTiesPasses(t *testing.T) {
	res := runBalance(t, &fundBalanceIdentity{}, twoYears(120_000))
	wantPass(t, res, 2)
	if !strings.Contains(res.Summary, "1 carry-forward") {
		t.Errorf("summary %q does not count the carry-forward it held", res.Summary)
	}
}

// TestAYearOnTwoBasesHasNoSingleEnding is the carry-forward's fail-closed
// arm: FY2026 printed both revised and adopted leaves no one ending to carry.
func TestAYearOnTwoBasesHasNoSingleEnding(t *testing.T) {
	revised := fund101
	revised.basis = mapping.BasisRevised
	facts := append(twoYears(120_000), revised.facts(byFundRow())...)
	wantFail(t, runBalance(t, &fundBalanceIdentity{}, facts), "prints FY2026 on 2 bases")
}

func TestACarryForwardBreakFails(t *testing.T) {
	wantFail(t, runBalance(t, &fundBalanceIdentity{}, twoYears(120_001)),
		"capital fund 101", "FY2026 adopted ends at $1,200.00", "FY2027 adopted begins at $1,200.01")
}

// TestTheSpineStillPrintsItsChange is the spine's behaviour under the
// generalised checks: a column with its stocks and no change line fails both.
func TestTheSpineStillPrintsItsChange(t *testing.T) {
	spine := balanceAt{structure.ScopeAllFundsGross, "general", 0, 2026, mapping.BasisAdopted}
	lines := []balanceLine{
		{mapping.KindFundBalance, structure.CategoryFundBalanceBeginning, 100_000},
		{mapping.KindRevenue, "taxes/property", 50_000},
		{mapping.KindExpenditure, "wages-and-benefits", 30_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceChange, 20_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceEnding, 120_000},
	}
	wantPass(t, runBalance(t, &fundBalanceIdentity{}, spine.facts(lines)), 1)
	wantPass(t, runBalance(t, &fundGroupSourcesEqualUses{}, spine.facts(lines)), 1)

	noChange := spine.facts(without(lines, structure.CategoryFundBalanceChange))
	wantFail(t, runBalance(t, &fundBalanceIdentity{}, noChange), "missing "+structure.CategoryFundBalanceChange)
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, noChange), "missing "+structure.CategoryFundBalanceChange)
}

// TestABalanceInAnUndeclaredScopeFails is the table's own fail-closed arm: a
// scope printing a balance line that structure.FundBalances does not declare
// has no identity to hold it to, and is a finding rather than a skip.
func TestABalanceInAnUndeclaredScopeFails(t *testing.T) {
	at := fund101
	at.scope = "some-new-schedule"
	wantFail(t, runBalance(t, &fundBalanceIdentity{}, at.facts(byFundRow())), `"some-new-schedule"`)
}

// withBalanceExceptions declares exceptions the tree does not, for one test.
func withBalanceExceptions(t *testing.T, exceptions ...structure.BalanceException) {
	t.Helper()
	if err := structure.ValidateBalanceExceptions(structure.FundBalances(), exceptions); err != nil {
		t.Fatal(err)
	}
	prev := balanceExceptions
	balanceExceptions = func() []structure.BalanceException { return exceptions }
	declaredBalanceExceptions = true
	t.Cleanup(func() { balanceExceptions, declaredBalanceExceptions = prev, false })
}

// declaredBalanceExceptions says a test has declared its own exceptions.
var declaredBalanceExceptions bool

// carryBreak is the shape of fund 101's real break, ending one year and
// beginning the next at different figures, declared.
func carryBreak(left, right int64) structure.BalanceException {
	return structure.BalanceException{
		Identity: structure.BalanceCarryForward,
		At: structure.BalanceAt{DocID: testDoc, Scope: structure.ScopeFundBalancesByFund,
			FundGroup: "capital", Fund: "101", Year: 2026, Basis: mapping.BasisAdopted},
		Left: left, Right: right,
		Printed: "fixture", Reason: "fixture", Bead: "fisc-3eh2",
	}
}

func TestADeclaredCarryForwardBreakPasses(t *testing.T) {
	withBalanceExceptions(t, carryBreak(120_000, 120_001))
	wantPass(t, runBalance(t, &fundBalanceIdentity{}, twoYears(120_001)), 2)
}

func TestACarryForwardExceptionMustStillDescribeTheStore(t *testing.T) {
	for _, tt := range []struct {
		name  string
		facts []fact.Fact
		e     structure.BalanceException
		want  string
	}{
		{"a break that no longer exists", twoYears(120_000), carryBreak(120_000, 120_001), "holds at $1,200.00"},
		{"a side that moved", twoYears(120_002), carryBreak(120_000, 120_001), "the store says $1,200.00 and $1,200.02"},
		{"a balance the scope does not print", fund101.facts(byFundRow()), carryBreak(120_000, 120_001), "matches no balance"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			withBalanceExceptions(t, tt.e)
			wantFail(t, runBalance(t, &fundBalanceIdentity{}, tt.facts), tt.want)
		})
	}
}

// TestAnExceptionOnAScopeTheStoreDoesNotCarryIsNotStale is residue's rule: a
// declaration for pp.186-209 must not redden a store, such as the spine-only
// fixture, that carries none of them.
func TestAnExceptionOnAScopeTheStoreDoesNotCarryIsNotStale(t *testing.T) {
	withBalanceExceptions(t, carryBreak(120_000, 120_001))
	spine := fund101
	spine.scope, spine.fund = structure.ScopeAllFundsGross, 0
	lines := []balanceLine{
		{mapping.KindFundBalance, structure.CategoryFundBalanceBeginning, 100_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceChange, 20_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceEnding, 120_000},
	}
	wantPass(t, runBalance(t, &fundBalanceIdentity{}, spine.facts(lines)), 1)
}

// rowDelta is a FY2024 row the document rounds a dollar off its identity.
func rowDelta(net, change int64) structure.BalanceException {
	return structure.BalanceException{
		Identity: structure.BalanceSourcesUses,
		At: structure.BalanceAt{DocID: testDoc, Scope: structure.ScopeFundBalancesByFund,
			FundGroup: "capital", Fund: "101", Year: 2026, Basis: mapping.BasisAdopted},
		Left: net, Right: change,
		Printed: "fixture", Reason: "fixture", Bead: "fisc-3eh2",
	}
}

func TestADeclaredRowDeltaPassesAndAStaleOneFails(t *testing.T) {
	off := fund101.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 120_100))
	withBalanceExceptions(t, rowDelta(20_000, 20_100))
	wantPass(t, runBalance(t, &fundGroupSourcesEqualUses{}, off), 1)
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, fund101.facts(byFundRow())), "holds at $200.00")
}

// blankCells is a pp.186-209 rule reading County Measure D's FY2027 row on
// one page, as p207 prints it: every column but Reserve Increase/(Use), and,
// when ending is set, not the ending balance either, as p187 prints
// Community Benefit Fund's.
func blankCells(t *testing.T, ending bool) *mapping.File {
	t.Helper()
	cells := []string{`{label: "County Measure D", column: "Increase/(Use)", note: "the page leaves it blank"}`}
	if ending {
		cells = append(cells, `{label: "County Measure D", column: "6/30/27", note: "the page leaves it blank"}`)
	}
	return blankRule(t, structure.ScopeFundBalancesByFund, blankShape{cells: cells})
}

// blankShape is what a test adds to blankRule's County Measure D rule: its
// omitted cells, a row after County Measure D, and a column after the
// ending balance with its header.
type blankShape struct {
	cells          []string
	row            string
	header, column string
}

// blankRule is County Measure D's FY2027 row on p207, in scope, with shape's
// additions.
func blankRule(t *testing.T, scope string, shape blankShape) *mapping.File {
	t.Helper()
	rows := `      - {label: "County Measure D", fund_group: capital, fund: 305}
`
	if shape.row != "" {
		rows += "      - " + shape.row + "\n"
	}
	headers := `"7/1/26", "Revenues", "Transfers In", "Expenses", "Transfers Out",
                         "Transfers Out to CIP", "Increase/(Use)", "6/30/27"`
	columns := ""
	if shape.header != "" {
		headers += `, "` + shape.header + `"`
		columns = "          - " + shape.column + "\n"
	}
	cells := ""
	for _, c := range shape.cells {
		cells += "          - " + c + "\n"
	}
	src := `schema_version: 1
doc_id: ` + testDoc + `
rules:
  - id: by-fund-capital
    kind: fund_balance
    basis: adopted
    scope: ` + scope + `
    grain: fund-by-category
    units: dollars
    rows:
` + rows + `    parts:
      - page: 207
        section: "Capital Funds\n"
        stop_at: "Total Capital Funds"
        omitted_cells:
` + cells + `        column_headers: [` + headers + `]
        columns:
          - {fiscal_year: 2027, category: fund-balance/beginning}
          - {fiscal_year: 2027, kind: revenue, category: taxes}
          - {fiscal_year: 2027, kind: transfer_in, category: transfers/in}
          - {fiscal_year: 2027, kind: expenditure, category: capital-projects}
          - {fiscal_year: 2027, kind: transfer_out, category: transfers/out}
          - {fiscal_year: 2027, kind: transfer_out, category: transfers/out-to-cip}
          - {fiscal_year: 2027, category: fund-balance/reserve-increase}
          - {fiscal_year: 2027, category: fund-balance/ending}
` + columns
	files, err := mapping.LoadDir(fstest.MapFS{"mappings/blank.yaml": &fstest.MapFile{Data: []byte(src)}}, "mappings")
	if err != nil {
		t.Fatal(err)
	}
	return files[0]
}

// measureD is County Measure D's FY2027 balance in the scope.
var measureD = balanceAt{structure.ScopeFundBalancesByFund, "capital", 305, 2027, mapping.BasisAdopted}

// TestADeclaredBlankCellIsAbsentAndNotMissing is how the checks tell a cell
// the page leaves blank from a line a rule stopped publishing: the rule's own
// omitted_cells declaration. The blank flow is summed as absent, not as zero,
// and not as a finding; an undeclared one is still a finding.
func TestADeclaredBlankCellIsAbsentAndNotMissing(t *testing.T) {
	noReserve := measureD.facts(replace(without(byFundRow(), structure.CategoryFundBalanceReserveIncrease),
		structure.CategoryFundBalanceEnding, 123_000))
	wantPass(t, runBalance(t, &fundGroupSourcesEqualUses{}, noReserve, blankCells(t, false)), 1)
	wantPass(t, runBalance(t, &fundBalanceIdentity{}, noReserve, blankCells(t, false)), 1)
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, noReserve),
		"missing "+structure.CategoryFundBalanceReserveIncrease)

	// A blank stock leaves no change to hold the flows to, so the balance is
	// declared out of both identities rather than failed or summed.
	noEnding := measureD.facts(without(without(byFundRow(), structure.CategoryFundBalanceReserveIncrease),
		structure.CategoryFundBalanceEnding))
	for _, c := range []Check{&fundGroupSourcesEqualUses{}, &fundBalanceIdentity{}} {
		if res := runBalance(t, c, noEnding, blankCells(t, true)); res.Status == StatusFail {
			t.Errorf("%s: %s, findings %s; a declared blank ending is not a missing one", c.ID(), res.Status, said(res))
		}
		wantFail(t, runBalance(t, c, noEnding, blankCells(t, false)), "missing "+structure.CategoryFundBalanceEnding)
	}

	// And a declared blank the store prints anyway is a contradiction.
	printed := measureD.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 123_000))
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, printed, blankCells(t, false)), "declares that cell blank")
}

// TestABlankOnACellThatNeverPublishesIsNoBlank holds declaredBlanks to the
// predicate the mapping publishes by: a cell of a non-amount row or column
// yields no fact, so a blank declared there marks no line of the balance.
func TestABlankOnACellThatNeverPublishesIsNoBlank(t *testing.T) {
	t.Run("a quantity row", func(t *testing.T) {
		// The percentage row shares County Measure D's fund, so a blank on
		// its ending cell, read as an amount, would skip a wrong ending.
		file := blankRule(t, structure.ScopeFundBalancesByFund, blankShape{
			row:   `{label: "Share of Measure D", fund_group: capital, fund: 305, quantity: percentage}`,
			cells: []string{`{label: "Share of Measure D", column: "6/30/27", note: "the page leaves it blank"}`},
		})
		off := measureD.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 120_001))
		res := runBalance(t, &fundGroupSourcesEqualUses{}, off, file)
		wantFail(t, res, "(off by -$0.01)")
		if strings.Contains(said(res), "declares that cell blank") {
			t.Errorf("findings %s: a percentage cell is no blank on the balance", said(res))
		}
	})
	t.Run("a quantity column", func(t *testing.T) {
		file := blankRule(t, structure.ScopeFundBalancesByFund, blankShape{
			header: "% Change", column: `{quantity: percentage}`,
			cells: []string{`{label: "County Measure D", column: "% Change", note: "the page leaves it blank"}`},
		})
		facts := measureD.facts(byFundRow())
		wantPass(t, runBalance(t, &fundGroupSourcesEqualUses{}, facts, file), 1)
		wantPass(t, runBalance(t, &fundBalanceIdentity{}, facts, file), 1)
	})
}

// TestADeclaredBlankThatIsPrintedFailsOnEveryBalanceScope is the
// contradiction on a scope that does not hold sources = uses: ACFR p41's
// identity is fund-balance-identity's alone, and a printed fact on a line its
// rule declares blank must not pass there by the printed figure winning.
func TestADeclaredBlankThatIsPrintedFailsOnEveryBalanceScope(t *testing.T) {
	acfr := measureD
	acfr.scope = structure.ScopeACFRGeneralFundSummary
	file := blankRule(t, acfr.scope, blankShape{
		cells: []string{`{label: "County Measure D", column: "6/30/27", note: "the page leaves it blank"}`},
	})
	facts := acfr.facts([]balanceLine{
		{mapping.KindFundBalance, structure.CategoryFundBalanceBeginning, 100_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceChange, 20_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceEnding, 120_000},
	})
	wantFail(t, runBalance(t, &fundBalanceIdentity{}, facts, file), "declares that cell blank")
}

// TestABlankChangeIsNotCountedAsHeld: a balance whose scope prints a change
// line and whose rule declares that cell blank is complete, and beginning +
// change == ending was never evaluated on it, so the summary counts it apart
// from the balances that held the identity.
func TestABlankChangeIsNotCountedAsHeld(t *testing.T) {
	acfr := measureD
	acfr.scope = structure.ScopeACFRGeneralFundSummary
	file := blankRule(t, acfr.scope, blankShape{
		header: "Change", column: `{fiscal_year: 2027, category: fund-balance/change}`,
		cells: []string{`{label: "County Measure D", column: "Change", note: "the page leaves it blank"}`},
	})
	facts := acfr.facts([]balanceLine{
		{mapping.KindFundBalance, structure.CategoryFundBalanceBeginning, 100_000},
		{mapping.KindFundBalance, structure.CategoryFundBalanceEnding, 120_000},
	})
	res := runBalance(t, &fundBalanceIdentity{}, facts, file)
	wantPass(t, res, 1)
	const want = "0 of them beginning + change equal to ending to the cent, and 1 checked for completeness only"
	if !strings.Contains(res.Summary, want) {
		t.Errorf("summary %q, want %q", res.Summary, want)
	}
}

// TestABalanceHeldApartIsNotCountedAsHolding is what each summary counts: a
// balance an exception holds apart breaks the identity, so it is named
// apart from the balances that satisfy it.
func TestABalanceHeldApartIsNotCountedAsHolding(t *testing.T) {
	withBalanceExceptions(t, carryBreak(120_000, 120_001), rowDelta(20_000, 20_100))
	t.Run("carry-forward", func(t *testing.T) {
		fy2028 := fund101
		fy2028.year = 2028
		facts := append(twoYears(120_001), fy2028.facts(replace(replace(byFundRow(),
			structure.CategoryFundBalanceBeginning, 140_001), structure.CategoryFundBalanceEnding, 160_001))...)
		res := runBalance(t, &fundBalanceIdentity{}, facts)
		wantPass(t, res, 3)
		const want = "1 carry-forward(s) each ending where the next year begins, and 1 held apart by declared exceptions"
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q, want %q", res.Summary, want)
		}
	})
	t.Run("sources-uses", func(t *testing.T) {
		fund102 := fund101
		fund102.fund = 102
		facts := append(fund101.facts(replace(byFundRow(), structure.CategoryFundBalanceEnding, 120_100)),
			fund102.facts(byFundRow())...)
		res := runBalance(t, &fundGroupSourcesEqualUses{}, facts)
		wantPass(t, res, 2)
		const want = "1 balances, each with sources minus uses equal to its change, 1 held apart by declared " +
			"exceptions, and 0 exempted by declared blank cells"
		if res.Summary != want {
			t.Errorf("summary %q, want %q", res.Summary, want)
		}
	})
}

// TestABlankStockOnAScopePrintingItsChangeStillHoldsSourcesUses: the spine
// prints its change, so a blank beginning leaves the flows a printed figure
// to net to, and a misfiled flow is still a finding.
func TestABlankStockOnAScopePrintingItsChangeStillHoldsSourcesUses(t *testing.T) {
	spine := measureD
	spine.scope = structure.ScopeAllFundsGross
	file := blankRule(t, spine.scope, blankShape{
		header: "Change", column: `{fiscal_year: 2027, category: fund-balance/change}`,
		cells: []string{`{label: "County Measure D", column: "7/1/26", note: "the page leaves it blank"}`},
	})
	lines := append(without(byFundRow(), structure.CategoryFundBalanceBeginning),
		balanceLine{mapping.KindFundBalance, structure.CategoryFundBalanceChange, 20_000})
	wantPass(t, runBalance(t, &fundGroupSourcesEqualUses{}, spine.facts(lines), file), 1)
	misfiled := spine.facts(replace(lines, "taxes", 50_001))
	wantFail(t, runBalance(t, &fundGroupSourcesEqualUses{}, misfiled, file), "(off by $0.01)")
}

// TestABalanceExemptedByABlankIsCounted: a blank stock leaves sources = uses
// no change to hold, and the summary names the balance it set aside, so a
// rule wrongly declaring a stock blank cannot shrink the population unseen.
func TestABalanceExemptedByABlankIsCounted(t *testing.T) {
	noEnding := measureD.facts(without(without(byFundRow(), structure.CategoryFundBalanceReserveIncrease),
		structure.CategoryFundBalanceEnding))
	res := runBalance(t, &fundGroupSourcesEqualUses{}, append(noEnding, fund101.facts(byFundRow())...),
		blankCells(t, true))
	const want = "1 balances, each with sources minus uses equal to its change, 0 held apart by declared " +
		"exceptions, and 1 exempted by declared blank cells"
	if res.Summary != want {
		t.Errorf("summary %q, want %q", res.Summary, want)
	}
}

// TestTwoFactsOnOneLineOfSourcesUsesFail: a second revenue figure on one
// fund's year would be summed into its revenue, here to the revenue that
// balances, so sources = uses refuses the balance and names both facts.
func TestTwoFactsOnOneLineOfSourcesUsesFail(t *testing.T) {
	facts := measureD.facts(append(replace(byFundRow(), "taxes", 49_999), balanceLine{mapping.KindRevenue, "fees", 1}))
	res := runBalance(t, &fundGroupSourcesEqualUses{}, facts)
	wantFail(t, res, facts[1].ID, facts[len(facts)-1].ID, "revenue twice")
	if res.Subjects != 0 {
		t.Errorf("examined %d balances, want 0: a balance with no single revenue is not compared", res.Subjects)
	}
}
