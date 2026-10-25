package project

import (
	"encoding/json"
	"fmt"
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
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

const acfrDoc = "livermore-acfr-fy2025"

// historyFacts renders one printed row of an ACFR ten-year schedule across
// every column of [HistoryColumns].
func historyFacts(t *testing.T, scope, rule, label string, kind vocab.Kind,
	category, group string, cents int64,
) []fact.Fact {
	t.Helper()
	cols := HistoryColumns()
	out := make([]fact.Fact, 0, len(cols))
	for i, c := range cols {
		f := printedFact(acfrDoc, rule, mapping.Row{Category: category}, label,
			mapping.Column{FundGroup: group}, scope, kind, c, cents+int64(i))
		f.Page, f.Offset, f.Token = 167, 10*i, "x"
		out = append(out, f)
	}
	return out
}

// balancesFixture is p167's shape in miniature: two blocks with the same row
// label, told apart only by rule and column path.
func balancesFixture(t *testing.T) []fact.Fact {
	t.Helper()
	var out []fact.Fact
	out = append(out, historyFacts(t, FundBalancesScope, "gf-balances", "Committed",
		vocab.KindFundBalance, "fund-balance/committed", "general", 100)...)
	out = append(out, historyFacts(t, FundBalancesScope, "other-balances", "Committed",
		vocab.KindFundBalance, "fund-balance/committed", "", 200)...)
	return out
}

func historyOptions(scope string) Options {
	return Options{Columns: HistoryColumns(), Scopes: []string{scope}, Version: "test"}
}

// TestFundBalancesKeepsTheTwoBlocksApart: p167 prints "Committed" in both blocks
// under fund 0, so the series id (rule and column path) must tell them apart.
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
			t.Errorf("series %s has %d points, want all ten years", s.SeriesID, len(s.Points))
		}
	}
}

// TestHistorySlicesTakeTheWholeScope holds both ACFR
// projections: one slice, every column the store carries, or the document
// draws part of a scope and says nothing about the part it left.
func TestHistorySlicesTakeTheWholeScope(t *testing.T) {
	balances := balancesFixture(t)
	changes := historyFacts(t, ChangesScope, "revenues", "Sales taxes",
		vocab.KindRevenue, "taxes/sales", "", 300)

	for _, tc := range []struct {
		p     Projection
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
		vocab.KindFundBalance, "fund-balance/excess-of-revenues", "", 0)
	facts[3].AmountCents = -1_261_509_800
	d, err := (&FundBalanceChanges{}).Document(facts, historyOptions(ChangesScope))
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if got := d.Series[0].Points[3].AmountCents; got != -1_261_509_800 {
		t.Errorf("FY2019 excess = %d, want the printed negative", got)
	}
}

// TestHistoryDocumentsShareTheTrendsShape: both Document methods return a
// *TrendsDocument, the shape the trend checks read.
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
		if c.ComparableGroup != string(vocab.BasisAudited) {
			t.Errorf("column FY%d comparable_group = %q, want %q",
				c.FiscalYear, c.ComparableGroup, vocab.BasisAudited)
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

// TestHistoryDocumentsShipTheUnauditedCaveat: both ten-year pages disclose p161's
// "Statistical Section (Unaudited)". Dropping it from either list goes red.
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

// TestHistoryPublishesOneNullPerSeries holds where the document publishes null.
func TestHistoryPublishesOneNullPerSeries(t *testing.T) {
	first, err := (&FundBalances{}).Build(balancesFixture(t), historyOptions(FundBalancesScope))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// fund is null on every series (fact.Fact.Fund is nil), not 0; absent strings
	// are "" and absent slices [].
	var doc map[string]any
	if err := json.Unmarshal(first, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	nulls := nullKeys(doc)
	series := doc["series"].([]any)
	want := make([]string, len(series))
	for i := range want {
		want[i] = "fund"
	}
	if len(series) == 0 {
		t.Fatal("the fixture built no series, so the null arm below asserts nothing")
	}
	if !reflect.DeepEqual(nulls, want) {
		t.Errorf("null-valued keys = %v, want %v: one fund per series and no other null",
			nulls, want)
	}
}

// nullKeys is every key whose value is null, in document order, so a test can
// say WHERE a document publishes null rather than only whether it does.
func nullKeys(v any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				if t[k] == nil {
					out = append(out, k)
					continue
				}
				walk(t[k])
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	return out
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

// printedDollars is commaDollars with the statements' sign convention:
// negatives print in parentheses, never with a minus sign.
func printedDollars(c amount.Cents) string {
	if c < 0 {
		return "(" + commaDollars(-c) + ")"
	}
	return commaDollars(c)
}

// millionsCell renders cents the way p41's in-millions table prints them:
// two decimals, so a cell's unit is $10,000.
func millionsCell(c amount.Cents) string {
	h := int64(c) / 1_000_000
	return fmt.Sprintf("%d.%02d", h/100, h%100)
}

// lastAmountBelowLabel returns the last amount.Parse token on the first line at
// or below label's line that carries one: pp.168-169 wrap labels above their
// amounts, and the wanted column prints last.
func lastAmountBelowLabel(t *testing.T, name, label string, u amount.Units) amount.Cents {
	t.Helper()
	lines := fixturePage(t, name)
	for i, line := range lines {
		if !strings.Contains(line, label) {
			continue
		}
		for _, below := range lines[i:] {
			var last amount.Cents
			found := false
			for _, f := range strings.Fields(below) {
				if c, err := amount.Parse(f, u); err == nil {
					last, found = c, true
				}
			}
			if found {
				return last
			}
		}
		t.Fatalf("%s: no amounts at or below the %q line", name, label)
	}
	t.Fatalf("%s: no line contains %q", name, label)
	return 0
}

// TestP167ComponentsTieToP54NotP41: p167's five 2025 General Fund components sum
// to what p54 prints, not to p41's $87.10; the caveat quotes the sum and names p54.
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

	// The p41 leg pins the printed figure, not a mere difference: a difference
	// is satisfied by any token at all, including the dash amount.Parse reads
	// as zero, so it could not witness the caveat's claim.
	p41 := firstAmountOnLine(t, "acfr-p0041.txt", "Fund Balances- Ending", amount.Millions)
	if want := amount.Cents(8_710_000_000); p41 != want {
		t.Errorf("p41's condensed statement prints an ending balance of %d cents, want %d "+
			"-- the printed $87.10; a drifted fixture and a dash read as zero both land here", p41, want)
	}
	// A cell of p41's table is $10,000, so rounding the sum to the nearest
	// 1,000,000 cents is what "where the sum rounds" means on that page.
	if rounded := (sum + 500_000) / 1_000_000 * 1_000_000; rounded == p41 {
		t.Errorf("p167's components sum (%d cents) rounds to p41's printed balance %d; "+
			"the caveat exists because MD&A's condensation does not tie", sum, p41)
	}

	c := caveatBalancesDoNotTieToP41
	if want := "$" + commaDollars(sum); !strings.Contains(c.Text, want) {
		t.Errorf("caveat text does not quote %s, the figure p167's components and p54 share", want)
	}
	if want := "$" + millionsCell(p41); !strings.Contains(c.Text, want) {
		t.Errorf("caveat text does not quote %s, the balance p41 prints", want)
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

// TestChanges2025ColumnTiesToP54: on five lines pp.168-169's 2025 column equals
// p54's Total Governmental Funds, quoted as printed, signs included; it ships on
// the changes document alone, never in the shared unaudited caveat.
func TestChanges2025ColumnTiesToP54(t *testing.T) {
	c := caveatChangesTieToP54
	ties := []struct{ row, page, label, p54Label string }{
		{"Total revenues", "acfr-p0168.txt", "Total revenues", "Total Revenues"},
		{"Total Expenditures", "acfr-p0168.txt", "Total Expenditures", "Total Expenditures"},
		{"excess", "acfr-p0168.txt", "Excess of Revenues", "OVER EXPENDITURES"},
		{"other financing", "acfr-p0169.txt", "Total other financing", "Total Other Financing Sources (Uses)"},
		{"net change", "acfr-p0169.txt", "Net change in", "Net Change in Fund Balance"},
	}
	for _, tc := range ties {
		schedule := lastAmountBelowLabel(t, tc.page, tc.label, amount.Dollars)
		audited := lastAmountBelowLabel(t, "acfr-p0054.txt", tc.p54Label, amount.Dollars)
		if schedule != audited {
			t.Errorf("%s: the schedule's 2025 column prints %d cents against p54's %d; "+
				"the caveat says the five lines tie to the dollar", tc.row, schedule, audited)
		}
		if want := "$" + printedDollars(schedule); !strings.Contains(c.Text, want) {
			t.Errorf("%s: caveat text does not quote %s, the figure both pages print", tc.row, want)
		}
	}

	found := false
	for _, cv := range changesCaveats() {
		if cv.ID == c.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("changesCaveats does not carry %q; change.html's 2025 column would go uncorroborated", c.ID)
	}
	for _, cv := range fundBalancesCaveats() {
		if cv.ID == c.ID {
			t.Errorf("fundBalancesCaveats carries %q, a claim about pp.168-169, which balances.html does not show", c.ID)
		}
	}
	if strings.Contains(caveatStatisticalSectionIsUnaudited.Text, "p167") {
		t.Error("the shared unaudited caveat names p167; it ships on change.html too, whose " +
			"pages are pp.168-169, so a corroboration must live per document")
	}
}
