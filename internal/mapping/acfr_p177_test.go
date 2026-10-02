package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// ACFR p177, the ten-year schedule of outstanding debt by type — the page
// fisc-9tn4 was settled on, and the first consumer of the quantity channel.
//
// Every test here reads the page THROUGH the published rule
// (acfr-p0177-debt-by-type), so this file and the mapping cannot become two
// drifting readings of one page: the arithmetic below is over the tokens the
// rule's own anchors and grammars produced. The rule publishes nothing
// (fisc-xmh2, fisc-7jtl carry the blockers), so its amount cells are reachable
// only by lifting the skips in memory — which is also what makes these tests
// the page's arithmetic guard until publication lands and the check moves to
// internal/check.
const publishedACFR = "../../mappings/livermore-acfr-fy2025.yaml"

const acfrDebtRuleID = "acfr-p0177-debt-by-type"

// acfrDebtResolver loads the published ACFR mapping and resolves it against
// the committed p177 fixture pair. mutate, if non-nil, edits the rule before
// any read — the in-memory mutations these proofs are built on.
func acfrDebtResolver(t *testing.T, mutate func(*Rule)) (*Resolver, *Rule) {
	t.Helper()
	f, err := Load(publishedACFR)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var rule *Rule
	for i := range f.Rules {
		if f.Rules[i].ID == acfrDebtRuleID {
			rule = &f.Rules[i]
		}
	}
	if rule == nil {
		t.Fatalf("%s declares no rule %q", publishedACFR, acfrDebtRuleID)
	}
	if mutate != nil {
		mutate(rule)
	}
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{177}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, rule
}

// unskip lifts every publication gate the committed rule declares — row skips
// and amount-column skips — leaving the two quantity columns as they are. What
// remains is exactly the read the rule performs, with its amount cells visible.
func unskip(rule *Rule) {
	for i := range rule.Rows {
		rule.Rows[i].Skip = false
	}
	for i := range rule.Parts {
		for j := range rule.Parts[i].Columns {
			rule.Parts[i].Columns[j].Skip = false
		}
	}
}

// TestACFRDebtRulePublishesNothing pins the committed state: the rule reads
// the whole page — a resolution error here would say it no longer does — and
// yields not one Value. Every fact is blocked, and each blocker is a bead
// (fisc-xmh2, fisc-7jtl); un-skipping anything without settling those would
// publish debt under a false kind with a false year.
func TestACFRDebtRulePublishesNothing(t *testing.T) {
	r, rule := acfrDebtResolver(t, nil)
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("the committed rule published %d values, want 0", len(values))
	}
}

// acfrDebtGrid is the unskipped read laid out by year: for each of the ten
// rows, the eight amount columns in printed order (seven debt columns, then
// the city's own Total Primary Government).
func acfrDebtGrid(t *testing.T) map[int][]amount.Cents {
	t.Helper()
	r, rule := acfrDebtResolver(t, unskip)
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if want := 10 * 8; len(values) != want {
		t.Fatalf("unskipped read yields %d values, want %d (ten rows over "+
			"eight amount columns; the two quantity columns must yield none)", len(values), want)
	}

	grid := map[int][]amount.Cents{}
	for _, v := range values {
		if v.ColumnIndex >= 8 {
			t.Fatalf("column %d (%s) yielded a value; a non-amount column must "+
				"not, whatever the skips say", v.ColumnIndex+1, v.Column.Quantity)
		}
		year := 0
		for _, c := range v.Row.Label {
			year = year*10 + int(c-'0')
		}
		if grid[year] == nil {
			grid[year] = make([]amount.Cents, 8)
		}
		grid[year][v.ColumnIndex] = v.Cents
	}
	return grid
}

// TestACFRDebtScheduleTiesExceptOneRow is the schedule checking our reading of
// it: nine of the ten rows tie to the city's own printed total EXACTLY, which
// is what makes the tenth a fact about the document rather than a suspicion
// about the parser.
//
// FY2024's printed total is short by 176,292 — exactly its own Financed
// Purchases figure, the column the city started using that year. Naming the
// column is the whole claim; a difference that merely happened to be 176,292
// would prove nothing.
func TestACFRDebtScheduleTiesExceptOneRow(t *testing.T) {
	const (
		shortYear                         = 2024
		financedPurchasesCol              = 4
		shortBy              amount.Cents = 17629200
	)
	grid := acfrDebtGrid(t)
	if len(grid) != 10 {
		t.Fatalf("got %d year rows, want 10", len(grid))
	}
	for year, row := range grid {
		var sum amount.Cents
		for _, c := range row[:7] {
			sum += c
		}
		diff := sum - row[7]
		switch {
		case year == shortYear:
			if diff != shortBy {
				t.Errorf("FY%d: rows sum to %s against a printed %s, a difference of %s; want %s",
					year, sum, row[7], diff, shortBy)
			}
			if got := row[financedPurchasesCol]; got != shortBy {
				t.Errorf("FY%d: the difference is %s but Financed Purchases is %s, "+
					"so the total is not simply missing that column",
					year, shortBy, got)
			}
		case diff != 0:
			t.Errorf("FY%d: rows sum to %s against a printed %s, off by %s",
				year, sum, row[7], diff)
		}
	}
}

// TestACFRDebtRowIsNotCorruptedInTheCommittedCorpus is the flagship claim of
// amount.TestLeadingMinusIsReallyPositive, proved off the page: under poppler
// the FY2017 row's 512,946 stands alone, and the "-512,946" that test quotes
// — an empty column's dash glued on by xberg — is not in the corpus. The
// arithmetic proves the sign, and a leading minus is still refused.
func TestACFRDebtRowIsNotCorruptedInTheCommittedCorpus(t *testing.T) {
	row, ok := acfrDebtGrid(t)[2017]
	if !ok {
		t.Fatal("no FY2017 row, which is the row the amount package reconciles")
	}

	// The two empty columns are read as published zeros, which is the reading
	// the glued dash destroyed: "absent is not zero" cuts both ways, and here
	// the document prints the dash.
	zeros := 0
	for _, c := range row[:7] {
		if c == 0 {
			zeros++
		}
	}
	if zeros != 2 {
		t.Errorf("got %d zero columns in the FY2017 row, want 2 (the two the "+
			"city prints as dashes)", zeros)
	}

	const stated amount.Cents = 8264398000
	if row[7] != stated {
		t.Errorf("got a printed total of %s, want %s", row[7], stated)
	}
	if got, want := row[6], amount.Cents(51294600); got != want {
		t.Errorf("got %s in the column the glued dash landed on, want %s", got, want)
	}
	if _, err := amount.Parse("-512,946", amount.Dollars); err == nil {
		t.Error("Parse accepted the corrupted form; it must still fail closed")
	}
}

// TestACFRDebtPercentageColumnNeedsItsQuantity is fisc-oakx.2's second proof,
// kept as a regression: delete col 9's quantity and the read goes red on
// amount.Parse rejecting "2.5%" — checked by MESSAGE, not exit code, because a
// failure for any other reason (an anchor, a count) would be the
// green-because-the-gate-fired shape in red clothing.
func TestACFRDebtPercentageColumnNeedsItsQuantity(t *testing.T) {
	r, rule := acfrDebtResolver(t, func(rule *Rule) {
		rule.Parts[0].Columns[8].Quantity = ""
	})
	_, _, err := r.Values(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("the read succeeded without col 9's quantity; it must fail on the percentage")
	}
	for _, want := range []string{`cannot parse amount "2.5%"`, "not a recognized number", `row "2016" column 9`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestACFRDebtPerCapitaParsesCleanlyAsAmount is fisc-oakx.2's third proof, and
// it proves an ABSENCE: delete col 10's quantity and nothing goes red, because
// "$ 1,009" per capita is well-formed money. The YAML declaration is the only
// gate, and the corroboration that could refuse a mis-declaration — the
// cross-tie to p180's population — is fisc-54vn. If this test ever fails, a
// guard has appeared; retire fisc-54vn against it.
func TestACFRDebtPerCapitaParsesCleanlyAsAmount(t *testing.T) {
	r, rule := acfrDebtResolver(t, func(rule *Rule) {
		rule.Parts[0].Columns[9].Quantity = ""
	})
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("Values with col 10 declared as money: %v — a guard has appeared; "+
			"update this test and retire fisc-54vn against it", err)
	}
	if len(values) != 0 {
		t.Fatalf("published %d values, want 0 (the skips still hold)", len(values))
	}
}

// TestACFRDebtPageCarriesTheColumnGuard: p177 prints two "(1)" superscripts.
// The footnote's own is a line of its own in -layout and clusters into its
// sentence's geometry line, which splitSuperscripts gives back, so the page
// pairs. The header line's, after "Income", is on that line in both
// substrates, so it is a header word like any other: a header list that
// leaves it out is refused for a column it does not declare, and one naming
// "Income (1)" reads the committed rule guarded.
func TestACFRDebtPageCarriesTheColumnGuard(t *testing.T) {
	headers := func(income string) func(*Rule) {
		return func(rule *Rule) {
			rule.Parts[0].ColumnHeaders = columnHeaders{
				{Text: "Participation"}, {Text: "Payable"}, {Text: "SBITA"},
				{Text: "Participation"}, {Text: "Purchases"}, {Text: "Loan"},
				{Text: "SBITA"}, {Text: "Government"}, {Text: income}, {Text: "Capita"},
			}
		}
	}
	r, rule := acfrDebtResolver(t, headers("Income (1)"))
	if _, _, err := r.Values(rule, &rule.Parts[0]); err != nil {
		t.Fatalf("Values with column_headers: %v", err)
	}

	r, rule = acfrDebtResolver(t, headers("Income"))
	_, _, err := r.Values(rule, &rule.Parts[0])
	if err == nil || !strings.Contains(err.Error(), `"(1)" is printed on the header line`) {
		t.Fatalf("Values with the header line's superscript undeclared: %v, want it "+
			"refused as a column the part does not declare", err)
	}
}
