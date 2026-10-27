package project

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// stubFundFlows answers the four methods a fund- and department-keyed document
// needs. Every map is supplied by the test, so nothing here is inferred from a
// fact -- which is the property TestTheFundParentComesFromTheRegistry turns on.
type stubFundFlows struct {
	stubLabels
	names     map[int]string
	types     map[int]string
	tiers     map[int]string
	notes     map[int]string
	divisions map[string]string
	// departments is the ALL-CAPS tier, kept apart from divisions because
	// five slugs name both.
	departments map[string]string
	lines       map[lineKey][]string
}

// lineKey is the three-part question the projection asks the registry about a
// printed revenue row.
type lineKey struct{ parent, printed, kind string }

func (s stubFundFlows) FundName(n int) (string, bool) { v, ok := s.names[n]; return v, ok }
func (s stubFundFlows) FundType(n int) (string, bool) { v, ok := s.types[n]; return v, ok }
func (s stubFundFlows) ConstraintTier(n int) string   { return s.tiers[n] }
func (s stubFundFlows) RestrictionNote(n int) string  { return s.notes[n] }

// FundGroup answers from types, as the registry answers from funds.yaml's
// types, so a group no fund has is refused.
func (s stubFundFlows) FundGroup(name string) bool {
	for _, t := range s.types {
		if t == name {
			return true
		}
	}
	return false
}

func (s stubFundFlows) DivisionLabel(d string) (string, bool) {
	v, ok := s.divisions[d]
	return v, ok
}

// DepartmentLabel answers from its own map, so a fixture can tell which tier
// a projection asked for.
func (s stubFundFlows) DepartmentLabel(d string) (string, bool) {
	v, ok := s.departments[d]
	return v, ok
}

// LinesPrintedAs answers only what a test declared, so a forgotten row fails
// loudly.
func (s stubFundFlows) LinesPrintedAs(parent, printed, kind string) []string {
	return s.lines[lineKey{parent, printed, kind}]
}

// printedRow is the row label this fixture prints for a category, and lineOf
// the line slug it resolves to: one row per category.
func printedRow(category string) string { return "Printed " + category }
func lineOf(category string) string     { return category + "/printed" }

func fundFlowsLabels() stubFundFlows {
	return stubFundFlows{
		stubLabels: stubLabels{"taxes/property": "Property Taxes",
			"wages-and-benefits": "Wages & Benefits"},
		names:     map[int]string{100: "General Fund", 500: "Water"},
		types:     map[int]string{100: "general", 500: "enterprise"},
		tiers:     map[int]string{100: "discretionary", 500: "restricted-by-law"},
		notes:     map[int]string{100: "Available for any general city service.", 500: "Rates."},
		divisions: map[string]string{"police": "Police"},
		lines: map[lineKey][]string{
			{"taxes/property", printedRow("taxes/property"), "revenue"}: {lineOf("taxes/property")},
		},
	}
}

func fundFlowsOptions() Options {
	return Options{
		Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
		Scopes:  FundFlowsScopes(),
		Version: "fund-flows test",
	}
}

// fundFlowsFact is one fact at an address. The amount is what nets.
// factID is the id of the fixture fact `tag` names.
func factID(tag string) string { return fmt.Sprintf("fisc-f-%012x", tag[0]) }

// fundFlowsFact builds one fact of this file's fixture. tag is a handle and
// the id is built from it, so the id has the shape
// schema/fact-id.schema.json requires.
func fundFlowsFact(scope string, kind vocab.Kind, category, department, group string,
	fund *int, cents int64, tag string) fact.Fact {
	id := factID(tag)
	return fact.Fact{
		ID: id, DocID: testDoc, Scope: scope, Kind: kind, Category: category,
		Department: department, FundGroup: group, Fund: fund, RowLabel: printedRow(category),
		FiscalYear: testYear, Basis: testBasis, AmountCents: cents, Page: 127,
	}
}

func fundFlowsFacts() []fact.Fact {
	const rev, exp = ScopeRevenueByFund, scopeExpenditureByDepartment
	return []fact.Fact{
		fundFlowsFact(rev, vocab.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1000, "a"),
		fundFlowsFact(rev, vocab.KindRevenue, "taxes/property", "", "enterprise", fact.FundNumber(500), 2000, "b"),
		// A printed zero: a fact that earns no link.
		fundFlowsFact(rev, vocab.KindTransferIn, "transfers/in", "", "general", fact.FundNumber(100), 0, "c"),
		fundFlowsFact(exp, vocab.KindExpenditure, "wages-and-benefits", "police", "general", fact.FundNumber(100), 600, "d"),
		fundFlowsFact(exp, vocab.KindExpenditure, "services-and-supplies", "police", "general", fact.FundNumber(100), 400, "e"),
	}
}

func buildFundFlows(t *testing.T, facts []fact.Fact, l labels) *Document {
	t.Helper()
	doc, err := (&fundFlows{Labels: l}).Document(facts, fundFlowsOptions())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	return doc
}

// TestTheFundParentComesFromTheRegistryAndNotTheFact: with data/funds.yaml's
// type and the fact's fund_group disagreeing, the fund node follows the registry.
func TestTheFundParentComesFromTheRegistryAndNotTheFact(t *testing.T) {
	labels := fundFlowsLabels()
	labels.types[500] = "capital" // the registry says capital...
	facts := fundFlowsFacts()     // ...and the fact says enterprise.

	doc := buildFundFlows(t, facts, labels)
	var got string
	for _, n := range doc.Nodes {
		if n.ID == "fund/500" {
			got = n.Parent
		}
	}
	if got != "fund-group/capital" {
		t.Errorf("fund/500's parent is %q, want %q: the parent is the registry's type, "+
			"never the fact's fund_group", got, "fund-group/capital")
	}
}

// TestTheSpendingFundsParentComesFromTheRegistryToo: the same on the pp.173-183
// side. Mutation: parent a fund node by the fact's fund_group, and it goes red.
func TestTheSpendingFundsParentComesFromTheRegistryToo(t *testing.T) {
	facts := append(fundFlowsFacts(), fundFlowsFact(scopeExpenditureByFund,
		vocab.KindExpenditure, "wages-and-benefits", "", "capital", fact.FundNumber(500), 300, "s"))
	doc := buildFundFlows(t, facts, fundFlowsLabels()) // the registry says enterprise
	var got string
	for _, n := range doc.Nodes {
		if n.ID == "fund/500" {
			got = n.Parent
		}
	}
	if got != "fund-group/enterprise" {
		t.Errorf("fund/500's parent is %q, want %q: the parent is the registry's type, "+
			"never the fact's fund_group", got, "fund-group/enterprise")
	}
}

// TestEveryFundNodeDisclosesItsConstraintTier checks the tier disclosure
// at the producer as well as by the check that reads the document.
func TestEveryFundNodeDisclosesItsConstraintTier(t *testing.T) {
	doc := buildFundFlows(t, fundFlowsFacts(), fundFlowsLabels())
	seen := 0
	for _, n := range doc.Nodes {
		if n.ConstraintTier == "" {
			continue
		}
		seen++
		if n.SourceNote == "" || n.Rationale == "" {
			t.Errorf("node %q carries tier %q with source_note %q and rationale %q; both "+
				"are required, because the node is published and the tier is ours",
				n.ID, n.ConstraintTier, n.SourceNote, n.Rationale)
		}
		// And NOT derived: the city printed the fund.
		if n.Derived {
			t.Errorf("node %q is marked derived; that claims the city did not print the "+
				"fund, which is the opposite error", n.ID)
		}
	}
	if seen == 0 {
		t.Fatal("no node carries a constraint tier, so this test asserts nothing")
	}
	// The id and the text are both asserted, so a reworded, weaker disclosure under
	// the same anchor goes red.
	want := ConstraintTierCaveat()
	i := slices.IndexFunc(doc.Metadata.Caveats, func(c Caveat) bool { return c.ID == want.ID })
	switch {
	case i < 0:
		t.Error("the document publishes constraint tiers and its caveats do not disclose " +
			"that they are our reading")
	case doc.Metadata.Caveats[i].Text != want.Text:
		t.Errorf("caveat %q is present and its text is not the declared disclosure:\ngot  %q\nwant %q",
			want.ID, doc.Metadata.Caveats[i].Text, want.Text)
	}
}

// TestTheStoppedGroupCountIsTheDocumentsOwn computes which groups end at
// their funds rather than writing a count down: the published columns differ
// in which groups they carry.
func TestTheStoppedGroupCountIsTheDocumentsOwn(t *testing.T) {
	general := []Node{
		{ID: PrefixFund + "100", Tier: tierFund, Parent: PrefixFundGroup + "general"},
		{ID: PrefixDept + "patrol", Tier: tierDepartment, Parent: PrefixFund + "100"},
	}
	for _, tc := range []struct {
		name       string
		noDivision bool
		extra      []Node
		want       []string
	}{
		{"every group spends", false, []Node{
			{ID: PrefixFund + "600", Tier: tierFund, Parent: PrefixFundGroup + "enterprise"},
			{ID: PrefixExpenditure + "fund/600/wages-and-benefits", Tier: tierObjectCategory, Parent: PrefixFund + "600"},
		}, []string{}},
		{"a group with revenue and no spending stops", false, []Node{
			{ID: PrefixFund + "600", Tier: tierFund, Parent: PrefixFundGroup + "enterprise"},
			{ID: PrefixFund + "470", Tier: tierFund, Parent: PrefixFundGroup + "permanent"},
			{ID: PrefixExpenditure + "fund/600/wages-and-benefits", Tier: tierObjectCategory, Parent: PrefixFund + "600"},
		}, []string{PrefixFundGroup + "permanent"}},
		// No spending drawn at all: every group stops, and the caveat is true
		// of every one of them.
		{"no group spends", true, []Node{
			{ID: PrefixFund + "600", Tier: tierFund, Parent: PrefixFundGroup + "enterprise"},
		}, []string{PrefixFundGroup + "enterprise", PrefixFundGroup + "general"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := append(append([]Node{}, general...), tc.extra...)
			if tc.noDivision {
				nodes = append([]Node{general[0]}, tc.extra...)
			}
			for _, g := range []string{"general", "enterprise", "permanent"} {
				for _, n := range nodes {
					if n.Parent == PrefixFundGroup+g {
						nodes = append(nodes, Node{ID: PrefixFundGroup + g, Tier: tierFundGroup})
						break
					}
				}
			}
			if diff := cmp.Diff(tc.want, truncatedGroups(nodes)); diff != "" {
				t.Errorf("truncatedGroups (-want +got):\n%s", diff)
			}
			var marks []string
			for _, c := range fundFlowsCaveats(0, nodes) {
				if c.ID == "some-funds-show-no-spending" {
					marks = c.AppliesTo
				}
			}
			if len(tc.want) == 0 && marks != nil {
				t.Errorf("no group stops and the caveat is published, marking %v", marks)
			}
			if len(tc.want) > 0 && !cmp.Equal(tc.want, marks) {
				t.Errorf("the caveat marks %v, want the groups it counts, %v", marks, tc.want)
			}
		})
	}
}

// TestOnlyTheGeneralFundHasDivisionsIsPublishedOnlyWhereTrue holds the
// divisions caveat to its sentence: the General Fund alone has a division
// beneath it, and some other group is drawn straight to its categories.
func TestOnlyTheGeneralFundHasDivisionsIsPublishedOnlyWhereTrue(t *testing.T) {
	const id = "only-the-general-fund-has-divisions"
	has := func(cs []Caveat) (Caveat, bool) {
		for _, c := range cs {
			if c.ID == id {
				return c, true
			}
		}
		return Caveat{}, false
	}
	groups := []Node{
		{ID: PrefixFundGroup + "general", Tier: tierFundGroup},
		{ID: PrefixFundGroup + "enterprise", Tier: tierFundGroup},
		{ID: PrefixFund + "100", Tier: tierFund, Parent: PrefixFundGroup + "general"},
		{ID: PrefixFund + "600", Tier: tierFund, Parent: PrefixFundGroup + "enterprise"},
	}
	division := Node{ID: PrefixDept + "patrol", Tier: tierDepartment, Parent: PrefixFund + "100"}
	direct := Node{ID: PrefixExpenditure + "fund/600/wages-and-benefits", Tier: tierObjectCategory,
		Parent: PrefixFund + "600"}

	both := append(append([]Node{}, groups...), division, direct)
	c, ok := has(fundFlowsCaveats(0, both))
	if !ok {
		t.Fatalf("a column with the General Fund's divisions and another fund's categories carries no %q", id)
	}
	if diff := cmp.Diff([]string{PrefixFundGroup + "enterprise", PrefixFund + "100"}, c.AppliesTo); diff != "" {
		t.Errorf("applies_to (-want +got):\n%s", diff)
	}
	if _, ok := has(fundFlowsCaveats(0, append(append([]Node{}, groups...), division))); ok {
		t.Errorf("a column where no other fund reaches its categories carries %q", id)
	}
	if _, ok := has(fundFlowsCaveats(0, append(append([]Node{}, groups...), direct))); ok {
		t.Errorf("a column with no division at all carries %q", id)
	}
	elsewhere := append(append([]Node{}, both...),
		Node{ID: PrefixDept + "airport", Tier: tierDepartment, Parent: PrefixFund + "600"})
	if _, ok := has(fundFlowsCaveats(0, elsewhere)); ok {
		t.Errorf("a column with divisions under enterprise too carries %q, which says the opposite", id)
	}
}

// TestTheStoppedCountIsPluralised is the one-group column: "1 fund group's",
// never "1 fund groups'".
func TestTheStoppedCountIsPluralised(t *testing.T) {
	nodes := []Node{
		{ID: PrefixFundGroup + "general", Tier: tierFundGroup},
		{ID: PrefixFundGroup + "capital", Tier: tierFundGroup},
		{ID: PrefixFund + "100", Tier: tierFund, Parent: PrefixFundGroup + "general"},
		{ID: PrefixDept + "patrol", Tier: tierDepartment, Parent: PrefixFund + "100"},
	}
	found := false
	for _, c := range fundFlowsCaveats(0, nodes) {
		if c.ID != "some-funds-show-no-spending" {
			continue
		}
		found = true
		if !strings.Contains(c.Text, "1 fund group's") || strings.Contains(c.Text, "1 fund groups") {
			t.Errorf("the caveat does not read \"1 fund group's\":\n  %s\n  %s", c.Summary, c.Text)
		}
	}
	if !found {
		t.Fatal("capital stops at its funds and no caveat says so")
	}
}

// TestTheCountsIdentityHolds is the arithmetic the document publishes.
func TestTheCountsIdentityHolds(t *testing.T) {
	doc := buildFundFlows(t, fundFlowsFacts(), fundFlowsLabels())
	c := doc.Metadata.Counts
	if c.Facts != 5 {
		t.Errorf("facts = %d, want 5", c.Facts)
	}
	if c.FactsCited+c.FactsUncited != c.Facts {
		t.Errorf("%d cited + %d uncited != %d facts", c.FactsCited, c.FactsUncited, c.Facts)
	}
	// The one printed zero is the whole of the gap.
	if c.FactsUncited != 1 {
		t.Errorf("uncited = %d, want 1: the transfers_in row is a printed zero", c.FactsUncited)
	}
	// Both expenditure facts are behind their object link and the
	// fund-to-department link; both revenue facts behind their fund flow and
	// their line's rollup. The printed zero is behind neither.
	if c.FactsCitedTwice != 4 {
		t.Errorf("cited twice = %d, want 4: the two object rows are also in the division "+
			"total, and the two revenue rows are also in their line's rollup",
			c.FactsCitedTwice)
	}
}

// TestALineRollsUpIntoItsCategoryOncePerKind: one printed row can reach the
// Internal Service Funds and the rest of the city, so the (1,0) rollup is per kind.
func TestALineRollsUpIntoItsCategoryOncePerKind(t *testing.T) {
	labels := fundFlowsLabels()
	labels.names[700] = "Fleet Maintenance"
	labels.types[700] = "internal-service"
	facts := append(fundFlowsFacts(), fundFlowsFact(ScopeRevenueByFund, vocab.KindRevenue,
		"taxes/property", "", "internal-service", fact.FundNumber(700), 500, "f"))

	got := map[LinkKind]Link{}
	for _, l := range buildFundFlows(t, facts, labels).Links {
		if l.Target != PrefixRevenue+"taxes/property" {
			continue
		}
		if l.Source != PrefixRevenueLine+lineOf("taxes/property") {
			t.Errorf("%s -> %s: the category's inflow comes from something other than its "+
				"own line", l.Source, l.Target)
		}
		if _, seen := got[l.Kind]; seen {
			t.Errorf("two %s rollups on one pair", l.Kind)
		}
		got[l.Kind] = l
	}
	if len(got) != 2 {
		t.Fatalf("the line rolls up %d time(s), want one per kind: %v", len(got), got)
	}
	if v := got[KindExternal].ValueCents; v != 3000 {
		t.Errorf("the external rollup is %d, want 3000 = 1000 general + 2000 enterprise", v)
	}
	if v := got[KindInternalService].ValueCents; v != 500 {
		t.Errorf("the internal-service rollup is %d, want 500", v)
	}
	if diff := cmp.Diff([]string{factID("a"), factID("b")}, got[KindExternal].FactIDs); diff != "" {
		t.Errorf("the external rollup cites the wrong rows (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{factID("f")}, got[KindInternalService].FactIDs); diff != "" {
		t.Errorf("the internal-service rollup cites the wrong rows (-want +got):\n%s", diff)
	}
	// A rollup of printed rows is published, not derived.
	for k, l := range got {
		if l.Derived {
			t.Errorf("the %s rollup is published as derived", k)
		}
	}
}

// TestAPrintedZeroIsNotInItsLinesRollup: unlike the division total, a line's
// rollup does not cite its printed dashes. Only the citation can show it; the
// value is the same either way.
func TestAPrintedZeroIsNotInItsLinesRollup(t *testing.T) {
	labels := fundFlowsLabels()
	labels.names[600] = "Capital Projects"
	labels.types[600] = "capital"
	facts := append(fundFlowsFacts(), fundFlowsFact(ScopeRevenueByFund, vocab.KindRevenue,
		"taxes/property", "", "capital", fact.FundNumber(600), 0, "z"))

	doc := buildFundFlows(t, facts, labels)
	var rollup *Link
	for i := range doc.Links {
		if doc.Links[i].Target == PrefixRevenue+"taxes/property" {
			rollup = &doc.Links[i]
		}
	}
	if rollup == nil {
		t.Fatal("the line rolls up into nothing, so the category has no inflow at all")
	}
	if diff := cmp.Diff([]string{factID("a"), factID("b")}, rollup.FactIDs); diff != "" {
		t.Errorf("the rollup's citation is not the rows that flowed (-want +got):\n%s", diff)
	}
	if rollup.ValueCents != 3000 {
		t.Errorf("the rollup is %d, want 3000: a dash adds nothing", rollup.ValueCents)
	}
	// The dash is accounted for in the identity.
	c := doc.Metadata.Counts
	if c.FactsUncited != 2 || c.FactsCited+c.FactsUncited != c.Facts {
		t.Errorf("%d cited + %d uncited of %d facts, want 2 uncited: the transfers_in row "+
			"and the dashed capital row", c.FactsCited, c.FactsUncited, c.Facts)
	}
}

// TestTheDivisionTotalIsTheSumOfItsObjectRows pins the middle link, which is the
// one that makes the document renderable at a coarse depth.
func TestTheDivisionTotalIsTheSumOfItsObjectRows(t *testing.T) {
	doc := buildFundFlows(t, fundFlowsFacts(), fundFlowsLabels())
	var mid *Link
	for i := range doc.Links {
		if doc.Links[i].Source == "fund/100" && doc.Links[i].Target == "dept/police" {
			mid = &doc.Links[i]
		}
	}
	if mid == nil {
		t.Fatal("no fund/100 -> dept/police link; the department node would have no inbound " +
			"flow and would render with a value of zero")
	}
	if mid.ValueCents != 1000 {
		t.Errorf("the division link is %d, want 1000 = 600 + 400", mid.ValueCents)
	}
	if len(mid.FactIDs) != 2 {
		t.Errorf("the division link cites %v, want both object rows", mid.FactIDs)
	}
}

// TestFundFlowsRefusesWhatItCannotPlace covers every guard, because each one is
// a mapping defect that would otherwise publish a smaller city with no error.
func TestFundFlowsRefusesWhatItCannotPlace(t *testing.T) {
	const rev, exp = ScopeRevenueByFund, scopeExpenditureByDepartment
	cases := []struct {
		name   string
		facts  []fact.Fact
		opts   func(Options) Options
		labels func(stubFundFlows) stubFundFlows
		want   string
	}{
		{
			name:  "one scope where three are required",
			facts: fundFlowsFacts(),
			opts:  func(o Options) Options { o.Scopes = []string{rev}; return o },
			want:  "want",
		},
		{
			name:  "two columns",
			facts: fundFlowsFacts(),
			opts: func(o Options) Options {
				o.Columns = append(o.Columns, Column{FiscalYear: testYear + 1, Basis: testBasis})
				return o
			},
			want: "one column",
		},
		{
			name: "a revenue fact naming no fund",
			facts: []fact.Fact{
				fundFlowsFact(rev, vocab.KindRevenue, "taxes/property", "", "general", nil, 1, "z"),
			},
			want: "names no fund",
		},
		{
			name: "an expenditure fact naming no department",
			facts: []fact.Fact{
				fundFlowsFact(exp, vocab.KindExpenditure, "wages-and-benefits", "", "general", fact.FundNumber(100), 1, "z"),
			},
			want: "carries no department",
		},
		{
			// pp.167-170 draw the General Fund through its divisions; a second
			// reading of it from expenditure-by-fund would draw it twice.
			name: "the General Fund from expenditure-by-fund",
			facts: []fact.Fact{
				fundFlowsFact(scopeExpenditureByFund, vocab.KindExpenditure, "wages-and-benefits", "", "general", fact.FundNumber(100), 1, "z"),
			},
			want: "draws fund 100 from expenditure-by-fund",
		},
		{
			name: "a fund the registry does not list, spending",
			facts: []fact.Fact{
				fundFlowsFact(scopeExpenditureByFund, vocab.KindExpenditure, "wages-and-benefits", "", "capital", fact.FundNumber(999), 1, "z"),
			},
			want: "fund 999 is in no data/funds.yaml entry",
		},
		{
			// A printed dash is a fact and not a flow, and its fund is still placed.
			name: "a fund the registry does not list, spending a printed dash",
			facts: []fact.Fact{
				fundFlowsFact(scopeExpenditureByFund, vocab.KindExpenditure, "wages-and-benefits", "", "capital", fact.FundNumber(999), 0, "z"),
			},
			want: "fund 999 is in no data/funds.yaml entry",
		},
		{
			// The tier-5 id carries the DIVISION and not the fund, so two funds
			// spending on one division/object would collide on one link.
			name: "two funds on the expenditure side",
			facts: []fact.Fact{
				fundFlowsFact(exp, vocab.KindExpenditure, "wages-and-benefits", "police", "general", fact.FundNumber(100), 1, "y"),
				fundFlowsFact(exp, vocab.KindExpenditure, "wages-and-benefits", "police", "enterprise", fact.FundNumber(500), 1, "z"),
			},
			want: "names funds",
		},
		{
			// A row the taxonomy does not declare is refused, not drawn under
			// its category.
			name: "a revenue row no line is printed as",
			facts: []fact.Fact{
				fundFlowsFact(rev, vocab.KindRevenue, "taxes/sales", "", "general", fact.FundNumber(100), 1, "z"),
			},
			want: "no data/taxonomy.yaml line under",
		},
		{
			// A line resolves under a category the taxonomy does not declare:
			// its parent is refused, not built as a tier-0 box.
			name: "a line parented to a category the taxonomy does not declare",
			facts: []fact.Fact{
				fundFlowsFact(rev, vocab.KindRevenue, "typo", "", "general", fact.FundNumber(100), 1, "z"),
			},
			labels: func(l stubFundFlows) stubFundFlows {
				l.lines[lineKey{"typo", printedRow("typo"), "revenue"}] = []string{lineOf("typo")}
				return l
			},
			want: `parented to "revenue/typo", and data/taxonomy.yaml declares no category "typo"`,
		},
		{
			name: "a revenue row two lines claim",
			facts: []fact.Fact{
				fundFlowsFact(rev, vocab.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1, "z"),
			},
			labels: func(l stubFundFlows) stubFundFlows {
				l.lines = map[lineKey][]string{
					{"taxes/property", printedRow("taxes/property"), "revenue"}: {
						"taxes/property/one", "taxes/property/two"},
				}
				return l
			},
			want: "is printed by 2 lines",
		},
		{
			name: "a revenue fact carrying no row label",
			facts: []fact.Fact{
				func() fact.Fact {
					f := fundFlowsFact(rev, vocab.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1, "z")
					f.RowLabel = ""
					return f
				}(),
			},
			want: "carries no row label",
		},
		{
			name: "a fund the registry does not list",
			facts: []fact.Fact{
				fundFlowsFact(rev, vocab.KindRevenue, "taxes/property", "", "general", fact.FundNumber(999), 1, "z"),
			},
			want: "is in no data/funds.yaml entry",
		},
		{
			name: "a division the registry does not list",
			facts: []fact.Fact{
				fundFlowsFact(exp, vocab.KindExpenditure, "wages-and-benefits", "police-department", "general", fact.FundNumber(100), 1, "z"),
			},
			want: `lists no division "police-department"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := fundFlowsOptions()
			if c.opts != nil {
				o = c.opts(o)
			}
			l := fundFlowsLabels()
			if c.labels != nil {
				l = c.labels(l)
			}
			_, err := (&fundFlows{Labels: l}).Document(c.facts, o)
			if err == nil {
				t.Fatal("Document = nil error, want a refusal")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want it to mention %q", err, c.want)
			}
		})
	}
}

// TestAFundGroupParentIsOneFundsYamlUses drives addParents directly: every
// fund's parent is read from FundType, so no fact can reach this arm with a
// group funds.yaml does not use, and only a node handed in here can.
func TestAFundGroupParentIsOneFundsYamlUses(t *testing.T) {
	f := &fundFlows{Labels: fundFlowsLabels()}
	err := f.addParents(map[string]Node{
		"fund/500": {ID: "fund/500", Parent: PrefixFundGroup + "typo"},
	})
	want := `parented to "fund-group/typo", and no data/funds.yaml fund has type "typo"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("addParents = %v, want a refusal mentioning %q", err, want)
	}
	nodes := map[string]Node{
		"fund/500": {ID: "fund/500", Parent: PrefixFundGroup + "enterprise"},
	}
	if err := f.addParents(nodes); err != nil {
		t.Fatalf("addParents over a declared group: %v", err)
	}
	if _, ok := nodes[PrefixFundGroup+"enterprise"]; !ok {
		t.Errorf("addParents built no %q node", PrefixFundGroup+"enterprise")
	}
}

// TestFundFlowsNeedsARegistry: the parent edge comes from data/funds.yaml, so a
// nil Labels is a hierarchy with nothing in it rather than a readable fallback.
// This is where it differs from Sankey, whose Labels is genuinely optional.
func TestFundFlowsNeedsARegistry(t *testing.T) {
	_, err := (&fundFlows{}).Document(fundFlowsFacts(), fundFlowsOptions())
	if err == nil || !strings.Contains(err.Error(), "no registry") {
		t.Fatalf("Document with no Labels = %v, want a refusal naming the registry", err)
	}
}

// TestFundFlowsSlicesDeclareOnlyColumnsBothSchedulesCarry: a column one schedule
// prints and the other does not would draw a whole revenue side against an empty
// expenditure side, which looks like a city that stopped spending.
func TestFundFlowsSlicesDeclareOnlyColumnsBothSchedulesCarry(t *testing.T) {
	facts := fundFlowsFacts()
	// A second column, revenue only.
	extra := fundFlowsFact(ScopeRevenueByFund, vocab.KindRevenue, "taxes/property", "",
		"general", fact.FundNumber(100), 5, "x")
	extra.FiscalYear = testYear + 1
	facts = append(facts, extra)

	got := (&fundFlows{Labels: fundFlowsLabels()}).Slices(facts, "test")
	if len(got) != 1 {
		t.Fatalf("slices = %d, want 1: only one column is printed by both schedules", len(got))
	}
	if got[0].Columns[0].FiscalYear != testYear {
		t.Errorf("slice covers FY%d, want FY%d", got[0].Columns[0].FiscalYear, testYear)
	}
}

// TestATransferInLinkIsNotExternal: a transfer moves between city funds and
// crosses no boundary, whatever group receives it; link-kinds-match-their-facts
// is the check side.
func TestATransferInLinkIsNotExternal(t *testing.T) {
	facts := fundFlowsFacts()
	for i := range facts {
		if facts[i].Kind == vocab.KindTransferIn {
			facts[i].AmountCents = 5000 // a printed zero earns no link at all
		}
	}
	doc := buildFundFlows(t, facts, fundFlowsLabels())

	seen := 0
	for _, l := range doc.Links {
		if l.Source != NodeTransfersIn {
			continue
		}
		seen++
		if l.Kind != KindInternalTransfer {
			t.Errorf("%s -> %s is %q, want %q: a transfer crosses no boundary",
				l.Source, l.Target, l.Kind, KindInternalTransfer)
		}
	}
	if seen == 0 {
		t.Fatal("no transfers/in link was emitted, so this test asserts nothing")
	}

	// And a revenue link into the same fund group is still external, so the fix
	// is a distinction rather than a blanket relabelling.
	for _, l := range doc.Links {
		if strings.HasPrefix(l.Source, PrefixRevenue) && l.Target == "fund/100" {
			if l.Kind != KindExternal {
				t.Errorf("%s -> %s is %q, want %q", l.Source, l.Target, l.Kind, KindExternal)
			}
		}
	}
}

// TestTheExpenditureSideRefusesAFundlessFact: fundless facts would be
// attributed to the General Fund on no evidence, and no amount check would
// see it.
func TestTheExpenditureSideRefusesAFundlessFact(t *testing.T) {
	facts := []fact.Fact{
		fundFlowsFact(scopeExpenditureByDepartment, vocab.KindExpenditure,
			"wages-and-benefits", "police", "general", nil, 600, "d"),
	}
	_, err := (&fundFlows{Labels: fundFlowsLabels()}).Document(facts, fundFlowsOptions())
	if err == nil || !strings.Contains(err.Error(), "names no fund") {
		t.Fatalf("Document = %v, want a refusal naming the missing fund", err)
	}
}

// TestATierFiveParentIsCutAtTheFirstSlash: a category may hold a slash
// (taxes/property), so a last-index cut would yield a parent resolving to nothing.
func TestATierFiveParentIsCutAtTheFirstSlash(t *testing.T) {
	labels := fundFlowsLabels()
	labels.stubLabels["fund-balance/ending"] = "Ending Balance"
	facts := []fact.Fact{
		fundFlowsFact(ScopeRevenueByFund, vocab.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1, "a"),
		fundFlowsFact(scopeExpenditureByDepartment, vocab.KindExpenditure,
			"fund-balance/ending", "police", "general", fact.FundNumber(100), 600, "d"),
	}
	doc := buildFundFlows(t, facts, labels)
	for _, n := range doc.Nodes {
		if n.Tier != 5 {
			continue
		}
		if n.Parent != "dept/police" {
			t.Errorf("node %q has parent %q, want %q: the division is the FIRST segment",
				n.ID, n.Parent, "dept/police")
		}
	}
}

// TestAReductionNamesTheCategoryTheSchedulePrintsItUnder: a ribbon cannot carry
// a minus sign, so Contra is what tells a reduction from an addition. p127's
// ERAF reaches the document twice, as a fund cell and as its line's rollup.
func TestAReductionNamesTheCategoryTheSchedulePrintsItUnder(t *testing.T) {
	const eraf = "ERAF"
	labels := fundFlowsLabels()
	// Its own printed row: sharing one with additions would net positive
	// and hide the defect.
	labels.lines[lineKey{"taxes/property", eraf, "revenue"}] = []string{
		PrefixRevenueLine + "taxes/property/eraf"}
	reduction := fundFlowsFact(ScopeRevenueByFund, vocab.KindRevenue,
		"taxes/property", "", "general", fact.FundNumber(100), -250, "r")
	reduction.RowLabel = eraf
	facts := append(fundFlowsFacts(), reduction)

	var named, silent, wrong []string
	for _, l := range buildFundFlows(t, facts, labels).Links {
		where := l.Source + " -> " + l.Target
		switch {
		case l.ValueCents < 0 && l.Contra == "":
			silent = append(silent, where)
		case l.ValueCents < 0:
			named = append(named, where)
			if l.Contra != "printed as a reduction of Property Taxes" {
				wrong = append(wrong, where+": "+l.Contra)
			}
		case l.Contra != "":
			wrong = append(wrong, where+" is positive and says "+l.Contra)
		}
	}
	if len(silent) > 0 {
		t.Errorf("%d negative link(s) name no schedule: %v", len(silent), silent)
	}
	if len(wrong) > 0 {
		t.Errorf("wrong sentence: %v", wrong)
	}
	// Both the cell and the line's rollup, so a pass covering only one
	// emitting site fails.
	if len(named) != 2 {
		t.Errorf("%d negative link(s) carry the sentence, want 2 -- the fund cell and "+
			"its line's rollup into the category: %v", len(named), named)
	}
}

// TestAFundsObjectCellIsNamedForItsFund: two funds' cells of one category are
// told apart by label, each naming its fund; a division's names its division.
func TestAFundsObjectCellIsNamedForItsFund(t *testing.T) {
	labels := fundFlowsLabels()
	labels.names[501] = "Water Replacement"
	labels.types[501] = "enterprise"
	labels.stubLabels["services-and-supplies"] = "Services & Supplies"
	facts := append(fundFlowsFacts(),
		fundFlowsFact(scopeExpenditureByFund, vocab.KindExpenditure, "services-and-supplies", "", "enterprise", fact.FundNumber(500), 700, "f"),
		fundFlowsFact(scopeExpenditureByFund, vocab.KindExpenditure, "services-and-supplies", "", "enterprise", fact.FundNumber(501), 300, "g"))
	doc := buildFundFlows(t, facts, labels)
	got := map[string]string{}
	for _, n := range doc.Nodes {
		if n.Tier == 5 {
			got[n.ID] = n.Label
		}
	}
	want := map[string]string{
		"expenditure/fund/500/services-and-supplies": "Water: Services & Supplies",
		"expenditure/fund/501/services-and-supplies": "Water Replacement: Services & Supplies",
		"expenditure/police/wages-and-benefits":      "Police: Wages & Benefits",
		"expenditure/police/services-and-supplies":   "Police: Services & Supplies",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("tier-5 labels (-want +got):\n%s", diff)
	}
}

// TestNoRevenueLineReadsAsItsCategory: a line printed as its whole category, as
// pp.131-140 print one, does not ship reading as the category inside itself.
// Mutation: dropping the labelCategoryNamedLines call fails every column.
func TestNoRevenueLineReadsAsItsCategory(t *testing.T) {
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"revenue-line/charges-for-services/charges-for-services":         "Charges for Services, one row",
		"revenue-line/taxes/property/property-taxes":                     "Property Taxes, one row",
		"revenue-line/use-of-money-and-property/use-of-money-and-prop":   "Use of Money And Property, one row",
		"revenue-line/miscellaneous-revenue/miscellaneous-rev":           "Miscellaneous Revenue, one row",
		"revenue-line/miscellaneous-revenue/miscellaneous-revenue":       "Miscellaneous Revenue, the line so named",
		"revenue-line/charges-for-services/administrative-cost-recovery": "Administrative Cost Recovery",
	}
	met := map[string]bool{}
	for _, p := range []interface {
		Projection
		Document([]fact.Fact, Options) (*Document, error)
	}{&fundFlows{Labels: store.reg}, &expansion{Labels: store.reg, Exceptions: structure.BudgetBookExceptions()}} {
		for _, o := range p.Slices(store.facts, "revenue-line label test") {
			d, err := p.Document(store.facts, o)
			if err != nil {
				t.Fatalf("%s %v: %v", p.Name(), o.Columns, err)
			}
			byID := map[string]Node{}
			for _, n := range d.Nodes {
				byID[n.ID] = n
			}
			seen := map[[2]string]string{}
			for _, n := range d.Nodes {
				if !strings.HasPrefix(n.ID, PrefixRevenueLine) {
					continue
				}
				if up, ok := byID[n.Parent]; ok && up.Label == n.Label {
					t.Errorf("%s %v: %s reads %q, its category's own label", p.Name(), o.Columns, n.ID, n.Label)
				}
				k := [2]string{n.Parent, n.Label}
				if other, ok := seen[k]; ok {
					t.Errorf("%s %v: %s and %s both read %q under %s", p.Name(), o.Columns, other, n.ID, n.Label, n.Parent)
				}
				seen[k] = n.ID
				if w, ok := want[n.ID]; ok {
					met[p.Name()+" "+n.ID] = true
					if n.Label != w {
						t.Errorf("%s %v: %s reads %q, want %q", p.Name(), o.Columns, n.ID, n.Label, w)
					}
				}
			}
		}
		for id := range want {
			if !met[p.Name()+" "+id] {
				t.Errorf("%s draws %s in no column, so its wording is held by nothing", p.Name(), id)
			}
		}
	}
}

// termsLabels is stubFundFlows answering Terms from its own map, so a fixture
// can print a line under its category's name.
type termsLabels struct {
	stubFundFlows
	terms map[string][]string
}

func (s termsLabels) Terms(slug string) []string { return s.terms[slug] }

// namedLine is one revenue line of a labelCategoryNamedLines fixture: its slug
// under the category, the label it is drawn with, and the fund printing it.
type namedLine struct {
	slug, label string
	fund        int
}

// TestLabelCategoryNamedLines: labelCategoryNamedLines over one category and
// its lines, each row stating the mutation that turns it red.
func TestLabelCategoryNamedLines(t *testing.T) {
	charges := map[string][]string{
		"charges":           {"charges for services"},
		"charges/charges-a": {"charges for services"},
		"charges/charges-b": {"charges for services"},
	}
	for _, tc := range []struct {
		name, category, label string
		terms                 map[string][]string
		lines                 []namedLine
		// wantErr is a substring of the refusal; empty, the call succeeds and
		// each line in wantLabels reads as stated.
		wantErr    string
		wantLabels map[string]string
	}{{
		// A line labelled as its category in another case and spacing, sharing
		// no term with it, is refused.
		// Mutation: drop the refusal of a line reading as its parent, and the
		// node is returned reading "charges for services ".
		name: "ARevenueLineReadingAsItsCategoryIsRefused", category: "charges", label: "Charges for Services",
		terms: map[string][]string{"charges": {"charges for services"}, "charges/charges": {"chgs for svcs"}},
		lines: []namedLine{{"charges", "charges for services ", 100}},
		wantErr: `revenue line revenue-line/charges/charges reads "charges for services " ` +
			`under revenue/charges`,
	}, {
		// It shares the category's term, so it is relabelled as the category's
		// one row.
		// Mutation: compare terms byte for byte in sharesTerm, and the line is
		// refused for reading "Charges For Services" under "Charges for Services".
		name: "ARevenueLineSpelledAsItsCategoryInAnotherCaseIsRelabelled", category: "charges",
		label:      "Charges for Services",
		terms:      map[string][]string{"charges": {"Charges for Services"}, "charges/charges": {"Charges For Services"}},
		lines:      []namedLine{{"charges", "Charges For Services", 100}},
		wantLabels: map[string]string{"charges": "Charges for Services" + OneRow},
	}, {
		// Labels differing only in case and trailing space read alike.
		// Mutation: key the collision refusal on the exact label, and the two
		// lines are returned reading "Grant State" and "grant state ".
		name: "TwoRevenueLinesReadingAlikeInAnotherCaseAreRefused", category: "intergov",
		label: "Intergovernmental",
		lines: []namedLine{{"grant-a", "Grant State", 100}, {"grant-b", "grant state ", 100}},
		wantErr: `revenue lines revenue-line/intergov/grant-a and revenue-line/intergov/grant-b under ` +
			`revenue/intergov read "Grant State" and "grant state "`,
	}, {
		// Two lines that would both read "<category>, one row" are refused.
		// Mutation: drop the collision refusal from labelCategoryNamedLines and
		// the two nodes are returned reading alike.
		name: "TwoRevenueLinesReadingAlikeAreRefused", category: "charges", label: "Charges for Services",
		terms: charges,
		lines: []namedLine{{"charges-a", "Charges for Services", 100}, {"charges-b", "Charges for Svcs", 500}},
		wantErr: `revenue lines revenue-line/charges/charges-a and revenue-line/charges/charges-b under ` +
			`revenue/charges read "Charges for Services, one row" and "Charges for Services, one row"`,
	}, {
		// One fund printing both lines itemizes them, and the two read apart.
		// Mutation: never take the itemized branch, and both lines read
		// "Charges for Services, one row" and are refused.
		name: "OneFundPrintingBothLinesItemizesThem", category: "charges",
		label: "Charges for Services", terms: charges,
		lines: []namedLine{{"charges-a", "Charges for Services", 100}, {"charges-b", "Charges for Svcs", 100}},
		wantLabels: map[string]string{
			"charges-a": "Charges for Services" + SoNamed, "charges-b": "Charges for Svcs" + SoNamed},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fundFlows{Labels: termsLabels{
				stubFundFlows: stubFundFlows{stubLabels: stubLabels{tc.category: tc.label}},
				terms:         tc.terms,
			}}
			parent := PrefixRevenue + tc.category
			nodes := map[string]Node{parent: {ID: parent, Label: tc.label}}
			rev := map[revKey]*cellSum{}
			for _, l := range tc.lines {
				slug := tc.category + "/" + l.slug
				id := PrefixRevenueLine + slug
				nodes[id] = Node{ID: id, Label: l.label, Parent: parent}
				rev[revKey{kind: vocab.KindRevenue, category: tc.category, line: slug, fund: l.fund}] = &cellSum{cents: 1}
			}
			err := f.labelCategoryNamedLines(nodes, rev)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got %v, want an error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("got %v, want no error", err)
			}
			for slug, want := range tc.wantLabels {
				if diff := cmp.Diff(want, nodes[PrefixRevenueLine+tc.category+"/"+slug].Label); diff != "" {
					t.Errorf("%s label (-want +got):\n%s", slug, diff)
				}
			}
		})
	}
}

// TestOneRowCaveatIsAboutEveryLineSoLabelled: each label's caveat names exactly
// the revenue lines carrying that label, and a document carries only the
// caveats of the labels it uses.
// Mutation: put the SoNamed lines in the OneRow caveat's applies_to and
// revenue-line/b is told it is its whole category; restore the summary's "the
// city prints no finer breakdown" and the pages arm goes red.
func TestOneRowCaveatIsAboutEveryLineSoLabelled(t *testing.T) {
	nodes := []Node{
		{ID: PrefixRevenueLine + "b", Label: "Miscellaneous Revenue" + SoNamed},
		{ID: PrefixRevenueLine + "a", Label: "Charges for Services" + OneRow},
		{ID: PrefixRevenueLine + "c", Label: "Current Year - Secured"},
		{ID: PrefixRevenue + "d", Label: "A category" + OneRow},
		{ID: PrefixRevenueLine + "e", Label: "Property Taxes" + OneRow},
	}
	about := func(cs []Caveat) map[string][]string {
		out := map[string][]string{}
		for _, c := range cs {
			out[c.ID] = c.AppliesTo
		}
		return out
	}
	cs := categoryNamedCaveats(nodes)
	want := map[string][]string{
		OneRowCaveatID:  {PrefixRevenueLine + "a", PrefixRevenueLine + "e"},
		SoNamedCaveatID: {PrefixRevenueLine + "b"},
	}
	if diff := cmp.Diff(want, about(cs)); diff != "" {
		t.Errorf("applies_to by caveat (-want +got):\n%s", diff)
	}
	if err := validateCaveats(cs, nodeIDs(nodes)); err != nil {
		t.Errorf("the caveats do not validate against their own nodes: %v", err)
	}
	// The code reads only pp.127-140, so the caveat names those pages in
	// both its sentences.
	for _, c := range cs {
		if c.ID == OneRowCaveatID && (!strings.Contains(c.Summary, "pp.127-140") || !strings.Contains(c.Text, "pp.127-140 print it as a single row")) {
			t.Errorf("%s claims beyond the pages read: %q / %q", c.ID, c.Summary, c.Text)
		}
	}
	if diff := cmp.Diff(map[string][]string{SoNamedCaveatID: {PrefixRevenueLine + "b"}},
		about(categoryNamedCaveats(nodes[:1]))); diff != "" {
		t.Errorf("a document with only a line so named (-want +got):\n%s", diff)
	}
	if cs := categoryNamedCaveats(nodes[2:4]); len(cs) != 0 {
		t.Errorf("a document with no relabelled line carries %d caveats", len(cs))
	}
}

// TestACellsFactIDsAscendWhateverOrderItsFactsArrive: a link's fact_ids ascend
// though the store is not sorted by id. Mutation: append in add instead of
// inserting in order and this goes red.
func TestACellsFactIDsAscendWhateverOrderItsFactsArrive(t *testing.T) {
	m := map[int]*cellSum{}
	for _, id := range []string{"fisc-f-c", "fisc-f-a", "fisc-f-b"} {
		add(m, 0, &fact.Fact{ID: id, AmountCents: 1})
	}
	if diff := cmp.Diff([]string{"fisc-f-a", "fisc-f-b", "fisc-f-c"}, m[0].factIDs); diff != "" {
		t.Errorf("fact ids (-want +got):\n%s", diff)
	}
}
