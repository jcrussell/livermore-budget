package project

import (
	"reflect"
	"strings"
	"testing"

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
