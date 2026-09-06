package project

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

const acfrDoc = "livermore-acfr-fy2025"

// historyFacts renders one printed row of an ACFR ten-year schedule across
// every column of [HistoryColumns].
func historyFacts(t *testing.T, scope, rule, label string, kind mapping.Kind,
	category, group string, cents int64,
) []fact.Fact {
	t.Helper()
	row := mapping.Row{Category: category}
	rowPath := fact.RowPath(row)
	columnPath := fact.ColumnPath(mapping.Column{FundGroup: group}, scope)
	cols := HistoryColumns()
	out := make([]fact.Fact, 0, len(cols))
	for i, c := range cols {
		out = append(out, fact.Fact{
			ID: fact.MakeID(acfrDoc, rule, rowPath, label, columnPath,
				c.FiscalYear, c.Basis),
			DocID:       acfrDoc,
			Page:        167,
			Offset:      10 * i,
			Token:       "x",
			RuleID:      rule,
			Kind:        kind,
			Basis:       c.Basis,
			Scope:       scope,
			FiscalYear:  c.FiscalYear,
			RowPath:     rowPath,
			RowLabel:    label,
			Category:    category,
			ColumnPath:  columnPath,
			FundGroup:   group,
			Sign:        mapping.SignPositive,
			Units:       "dollars",
			AmountCents: cents + int64(i),
		})
	}
	return out
}

// balancesFixture is p167's shape in miniature: the two blocks print the same
// row label, and only the rule and the column path tell them apart -- both
// carry fund 0, unlike the trends schedule, whose same-label rows sit in
// different funds.
func balancesFixture(t *testing.T) []fact.Fact {
	t.Helper()
	var out []fact.Fact
	out = append(out, historyFacts(t, FundBalancesScope, "gf-balances", "Committed",
		mapping.KindFundBalance, "fund-balance/committed", "general", 100)...)
	out = append(out, historyFacts(t, FundBalancesScope, "other-balances", "Committed",
		mapping.KindFundBalance, "fund-balance/committed", "", 200)...)
	return out
}

func historyOptions(scope string) Options {
	return Options{Columns: HistoryColumns(), Scopes: []string{scope}, Version: "test"}
}

// TestFundBalancesKeepsTheTwoBlocksApart is this document's identity claim:
// p167 prints "Committed" in both its blocks, and both rows carry fund 0, so
// neither the label nor a fund number can tell them apart. The series id --
// which hashes the rule and the column path -- must.
func TestFundBalancesKeepsTheTwoBlocksApart(t *testing.T) {
	d, err := (&FundBalances{}).Document(balancesFixture(t), historyOptions(FundBalancesScope))
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if len(d.Series) != 2 {
		t.Fatalf("got %d series, want 2 -- one per printed block", len(d.Series))
	}
	if d.Series[0].SeriesID == d.Series[1].SeriesID {
		t.Fatal("the two blocks' identical row labels share a series id")
	}
	groups := []string{d.Series[0].FundGroup, d.Series[1].FundGroup}
	if !reflect.DeepEqual(groups, []string{"general", ""}) &&
		!reflect.DeepEqual(groups, []string{"", "general"}) {
		t.Errorf("fund groups = %v, want one block general and one blank -- the blank is "+
			"how a page tells the aggregate from the General Fund", groups)
	}
	for _, s := range d.Series {
		if len(s.Points) != 10 {
			t.Errorf("series %s has %d points, want all ten audited years", s.SeriesID, len(s.Points))
		}
	}
}

// TestHistorySlicesTakeTheWholeScope is fisc-rmw's constraint on both ACFR
// projections: one slice, every column the store carries, or the scope's
// unprojectedScopes entry could have stayed green over a half-drawn schedule.
func TestHistorySlicesTakeTheWholeScope(t *testing.T) {
	balances := balancesFixture(t)
	changes := historyFacts(t, ChangesScope, "revenues", "Sales taxes",
		mapping.KindRevenue, "taxes/sales", "", 300)

	for _, tc := range []struct {
		p     Sliced
		facts []fact.Fact
		scope string
	}{
		{&FundBalances{}, balances, FundBalancesScope},
		{&FundBalanceChanges{}, changes, ChangesScope},
	} {
		got := tc.p.Slices(tc.facts, "test")
		if len(got) != 1 {
			t.Fatalf("%s: got %d slices, want exactly 1", tc.scope, len(got))
		}
		if got[0].ScopeList() != tc.scope {
			t.Errorf("scope = %q, want %q", got[0].ScopeList(), tc.scope)
		}
		if !reflect.DeepEqual(got[0].Columns, HistoryColumns()) {
			t.Errorf("%s columns = %v, want all ten in year order", tc.scope, got[0].Columns)
		}
		// And nothing of the other schedule: the spine-only stores throughout
		// this package are the everyday form of this.
		if extra := tc.p.Slices(spineFacts(t, testYear), "test"); len(extra) != 0 {
			t.Errorf("%s: got %d slices over a spine-only store, want none", tc.scope, len(extra))
		}
	}
}

// TestHistoryRefusesAnotherSchedulesScope guards the pair against each other:
// the two schedules share ten audited columns, so options are the only thing
// standing between a fund-balances document and the changes schedule's facts.
func TestHistoryRefusesAnotherSchedulesScope(t *testing.T) {
	if _, err := (&FundBalances{}).Document(balancesFixture(t),
		historyOptions(ChangesScope)); err == nil {
		t.Fatal("FundBalances over the changes scope = nil error, want a refusal")
	}
	if _, err := (&FundBalanceChanges{}).Document(balancesFixture(t),
		historyOptions(FundBalancesScope)); err == nil {
		t.Fatal("FundBalanceChanges over the balances scope = nil error, want a refusal")
	}
}

// TestChangesKeepsANegativeExcessSigned: the printed excess is negative in two
// of the ten years (2019 and 2022 on the real page), and a series is a printed
// row, so the sign ships rather than being netted away.
func TestChangesKeepsANegativeExcessSigned(t *testing.T) {
	facts := historyFacts(t, ChangesScope, "excess", "over (under) expenditures",
		mapping.KindFundBalance, "fund-balance/excess-of-revenues", "", 0)
	facts[3].AmountCents = -1_261_509_800
	d, err := (&FundBalanceChanges{}).Document(facts, historyOptions(ChangesScope))
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if got := d.Series[0].Points[3].AmountCents; got != -1_261_509_800 {
		t.Errorf("FY2019 excess = %d, want the printed negative", got)
	}
}

// TestHistoryDocumentsShareTheTrendsShape pins what lets `fisc verify` cover
// these documents without new checks: both Document methods return a
// *TrendsDocument, which is the shape trend-points-tie-to-facts and
// trend-series-are-complete read.
func TestHistoryDocumentsShareTheTrendsShape(t *testing.T) {
	d, err := (&FundBalances{}).Document(balancesFixture(t), historyOptions(FundBalancesScope))
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if d.Projection != FundBalancesProjection {
		t.Errorf("projection = %q, want %q", d.Projection, FundBalancesProjection)
	}
	if got := d.Metadata.Scope; got != FundBalancesScope {
		t.Errorf("scope = %q, want %q", got, FundBalancesScope)
	}
	if len(d.Metadata.Columns) != 10 {
		t.Errorf("got %d columns, want ten", len(d.Metadata.Columns))
	}
	// One comparable group: every column is audited, so unlike the trends
	// document a comparison may be carried across any pair of them.
	for _, c := range d.Metadata.Columns {
		if c.ComparableGroup != string(mapping.BasisAudited) {
			t.Errorf("column FY%d comparable_group = %q, want %q",
				c.FiscalYear, c.ComparableGroup, mapping.BasisAudited)
		}
	}
	for _, name := range []string{"FundBalances", "FundBalanceChanges"} {
		var caveats []Caveat
		if name == "FundBalances" {
			caveats = fundBalancesCaveats()
		} else {
			caveats = changesCaveats()
		}
		if err := validateCaveats(caveats, map[string]struct{}{}); err != nil {
			t.Errorf("%s caveats: %v", name, err)
		}
	}
}

// TestHistoryDocumentsShipTheUnauditedCaveat pins that both ten-year pages
// disclose their section's own label -- p161 reads "Statistical Section
// (Unaudited)" -- rather than each list carrying it by accident of the other.
// Dropping the caveat from either list goes red here.
func TestHistoryDocumentsShipTheUnauditedCaveat(t *testing.T) {
	for name, caveats := range map[string][]Caveat{
		"fundBalancesCaveats": fundBalancesCaveats(),
		"changesCaveats":      changesCaveats(),
	} {
		found := false
		for _, c := range caveats {
			if c.ID == caveatStatisticalSectionIsUnaudited.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not carry %q; the section's (Unaudited) label must reach that page", name, caveatStatisticalSectionIsUnaudited.ID)
		}
		if err := validateCaveats(caveats, map[string]struct{}{}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestHistoryIsDeterministic is what lets a rebuild-and-diff mean anything.
func TestHistoryIsDeterministic(t *testing.T) {
	facts := balancesFixture(t)
	p := &FundBalances{}
	first, err := p.Build(facts, historyOptions(FundBalancesScope))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := p.Build(facts, historyOptions(FundBalancesScope))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(first) != string(second) {
		t.Error("two builds over the same facts differ")
	}
	if strings.Contains(string(first), "null") {
		t.Error("the document contains a null; absent strings are \"\" and absent slices []")
	}
}

// fixturePage reads one page fixture: verbatim corpus bytes, sha256-equal to
// the extraction manifest's record for the page it was copied from
// (testdata/README.md), so a token read here is a token `fisc build` reads.
func fixturePage(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pages", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return strings.Split(string(b), "\n")
}

// firstAmountOnLine finds the first line of a fixture page containing label
// and parses the first token amount.Parse accepts at the given units. On both
// statements this test reads, the first amount column is the General Fund's.
func firstAmountOnLine(t *testing.T, name, label string, u amount.Units) amount.Cents {
	t.Helper()
	for _, line := range fixturePage(t, name) {
		if !strings.Contains(line, label) {
			continue
		}
		for _, f := range strings.Fields(line) {
			if c, err := amount.Parse(f, u); err == nil {
				return c
			}
		}
		t.Fatalf("%s: line %q carries no token amount.Parse accepts", name, label)
	}
	t.Fatalf("%s: no line contains %q", name, label)
	return 0
}

// commaDollars renders whole-dollar cents the way these documents print them.
func commaDollars(c amount.Cents) string {
	s := strconv.FormatInt(int64(c)/100, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// TestP167ComponentsTieToP54NotP41 re-derives the arithmetic behind the
// p167-does-not-tie-to-p41 caveat from fixture pages through amount.Parse,
// the parser every fact goes through. Three legs: p167's five 2025 General
// Fund components sum to one figure, p54's audited statement prints that
// figure to the dollar, and p41's in-millions condensation prints a balance
// the figure is not. The caveat must quote that sum and name p54: a caveat
// citing only p41 by the statement's bare name tells readers the audited ACFR
// disagrees with the schedule it agrees with exactly.
func TestP167ComponentsTieToP54NotP41(t *testing.T) {
	components := []string{"Nonspendable", "Restricted", "Committed", "Assigned", "Unassigned"}
	var sum amount.Cents
	seen := 0
	inGeneralFund := false
	for _, line := range fixturePage(t, "acfr-p0167.txt") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "General Fund" {
			inGeneralFund = true
			continue
		}
		if strings.HasPrefix(trimmed, "Total general fund") {
			break
		}
		fields := strings.Fields(trimmed)
		if !inGeneralFund || len(fields) == 0 || !slices.Contains(components, fields[0]) {
			continue
		}
		// 2025 is the schedule's last printed column.
		c, err := amount.Parse(fields[len(fields)-1], amount.Dollars)
		if err != nil {
			t.Fatalf("p167 %s, 2025 column: %v", fields[0], err)
		}
		sum += c
		seen++
	}
	if seen != len(components) {
		t.Fatalf("found %d of p167's five General Fund component rows, want all five", seen)
	}

	p54 := firstAmountOnLine(t, "acfr-p0054.txt", "FUND BALANCES- ENDING", amount.Dollars)
	if sum != p54 {
		t.Errorf("p167's 2025 components sum to %d cents against p54's ending balance %d; "+
			"the caveat says the audited statement agrees to the dollar", sum, p54)
	}

	p41 := firstAmountOnLine(t, "acfr-p0041.txt", "Fund Balances- Ending", amount.Millions)
	if sum == p41 {
		t.Errorf("p167's components sum equals p41's printed balance (%d cents); "+
			"the caveat exists because MD&A's condensation does not tie", p41)
	}

	c := caveatBalancesDoNotTieToP41
	if want := "$" + commaDollars(sum); !strings.Contains(c.Text, want) {
		t.Errorf("caveat text does not quote %s, the figure p167's components and p54 share", want)
	}
	if !strings.Contains(c.Text, "(p54)") {
		t.Error("caveat text does not name p54, the audited statement that agrees to the dollar")
	}
	for name, caveats := range map[string][]Caveat{
		"fundBalancesCaveats": fundBalancesCaveats(),
		"changesCaveats":      changesCaveats(),
	} {
		if err := validateCaveats(caveats, map[string]struct{}{}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
