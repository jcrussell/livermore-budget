package project

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
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
func fundFlowsFact(scope string, kind mapping.Kind, category, department, group string,
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
		fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1000, "a"),
		fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "enterprise", fact.FundNumber(500), 2000, "b"),
		// A printed zero: a fact that earns no link.
		fundFlowsFact(rev, mapping.KindTransferIn, "transfers/in", "", "general", fact.FundNumber(100), 0, "c"),
		fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police", "general", fact.FundNumber(100), 600, "d"),
		fundFlowsFact(exp, mapping.KindExpenditure, "services-and-supplies", "police", "general", fact.FundNumber(100), 400, "e"),
	}
}

func buildFundFlows(t *testing.T, facts []fact.Fact, l labels) *FundFlowsDocument {
	t.Helper()
	doc, err := (&fundFlows{Labels: l}).Document(facts, fundFlowsOptions())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	return doc
}

// TestTheFundParentComesFromTheRegistryAndNotTheFact is the claim without which
// the whole hierarchy is a tautology.
//
// fisc-gkv put it in writing: "the tier-3 node's parent comes from
// data/funds.yaml type: and NOT from the fact's fund_group, or
// aggregation-invariance is a tautology". The two are independent records of one
// claim -- funds.yaml's type: is transcribed from the appendix pp.253-257, while
// a fact's fund_group is read off the section header its row sits under on
// pp.131-140 -- and comparing two independent records is the only version of the
// comparison that says anything.
//
// The test makes them DISAGREE and asserts the node follows the registry. If the
// projection ever reads fund_group instead, this is the only thing in the tree
// that notices: every check stays green, because fact-funds-resolve pins the two
// equal over the committed corpus and a tautology passes.
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

// TestEveryFundNodeDisclosesItsConstraintTier is fisc-yor's requirement, checked
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
	// THE ID AND THE TEXT ARE BOTH ASSERTED, matching internal/check's arm.
	// Finding the id and stopping would pass over a document whose disclosure
	// had been reworded into something weaker while the anchor still resolved,
	// which is the quieter of the two failures.
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
		{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
		{ID: prefixDept + "patrol", Tier: tierDepartment, Parent: prefixFund + "100"},
	}
	for _, tc := range []struct {
		name       string
		noDivision bool
		extra      []Node
		want       []string
	}{
		{"every group spends", false, []Node{
			{ID: prefixFund + "600", Tier: tierFund, Parent: prefixFundGroup + "enterprise"},
			{ID: prefixExpenditure + "fund/600/wages-and-benefits", Tier: tierObjectCategory, Parent: prefixFund + "600"},
		}, []string{}},
		{"a group with revenue and no spending stops", false, []Node{
			{ID: prefixFund + "600", Tier: tierFund, Parent: prefixFundGroup + "enterprise"},
			{ID: prefixFund + "470", Tier: tierFund, Parent: prefixFundGroup + "permanent"},
			{ID: prefixExpenditure + "fund/600/wages-and-benefits", Tier: tierObjectCategory, Parent: prefixFund + "600"},
		}, []string{prefixFundGroup + "permanent"}},
		// No spending drawn at all: every group stops, and the caveat is true
		// of every one of them.
		{"no group spends", true, []Node{
			{ID: prefixFund + "600", Tier: tierFund, Parent: prefixFundGroup + "enterprise"},
		}, []string{prefixFundGroup + "enterprise", prefixFundGroup + "general"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := append(append([]Node{}, general...), tc.extra...)
			if tc.noDivision {
				nodes = append([]Node{general[0]}, tc.extra...)
			}
			for _, g := range []string{"general", "enterprise", "permanent"} {
				for _, n := range nodes {
					if n.Parent == prefixFundGroup+g {
						nodes = append(nodes, Node{ID: prefixFundGroup + g, Tier: tierFundGroup})
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
		{ID: prefixFundGroup + "general", Tier: tierFundGroup},
		{ID: prefixFundGroup + "enterprise", Tier: tierFundGroup},
		{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
		{ID: prefixFund + "600", Tier: tierFund, Parent: prefixFundGroup + "enterprise"},
	}
	division := Node{ID: prefixDept + "patrol", Tier: tierDepartment, Parent: prefixFund + "100"}
	direct := Node{ID: prefixExpenditure + "fund/600/wages-and-benefits", Tier: tierObjectCategory,
		Parent: prefixFund + "600"}

	both := append(append([]Node{}, groups...), division, direct)
	c, ok := has(fundFlowsCaveats(0, both))
	if !ok {
		t.Fatalf("a column with the General Fund's divisions and another fund's categories carries no %q", id)
	}
	if diff := cmp.Diff([]string{prefixFundGroup + "enterprise", prefixFund + "100"}, c.AppliesTo); diff != "" {
		t.Errorf("applies_to (-want +got):\n%s", diff)
	}
	if _, ok := has(fundFlowsCaveats(0, append(append([]Node{}, groups...), division))); ok {
		t.Errorf("a column where no other fund reaches its categories carries %q", id)
	}
	if _, ok := has(fundFlowsCaveats(0, append(append([]Node{}, groups...), direct))); ok {
		t.Errorf("a column with no division at all carries %q", id)
	}
	elsewhere := append(append([]Node{}, both...),
		Node{ID: prefixDept + "airport", Tier: tierDepartment, Parent: prefixFund + "600"})
	if _, ok := has(fundFlowsCaveats(0, elsewhere)); ok {
		t.Errorf("a column with divisions under enterprise too carries %q, which says the opposite", id)
	}
}

// TestTheStoppedCountIsPluralised is the one-group column: "1 fund group's",
// never "1 fund groups'".
func TestTheStoppedCountIsPluralised(t *testing.T) {
	nodes := []Node{
		{ID: prefixFundGroup + "general", Tier: tierFundGroup},
		{ID: prefixFundGroup + "capital", Tier: tierFundGroup},
		{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
		{ID: prefixDept + "patrol", Tier: tierDepartment, Parent: prefixFund + "100"},
	}
	found := false
	for _, c := range fundFlowsCaveats(0, nodes) {
		if c.ID != "some-funds-show-no-spending" {
			continue
		}
		found = true
		if !strings.Contains(c.Summary, "1 fund group's") || strings.Contains(c.Text, "1 fund groups") {
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
// Internal Service Funds as an internal service charge and the rest of the
// city as external revenue, so the (1,0) rollup is per kind. No other check
// holds this.
func TestALineRollsUpIntoItsCategoryOncePerKind(t *testing.T) {
	labels := fundFlowsLabels()
	labels.names[700] = "Fleet Maintenance"
	labels.types[700] = "internal-service"
	facts := append(fundFlowsFacts(), fundFlowsFact(ScopeRevenueByFund, mapping.KindRevenue,
		"taxes/property", "", "internal-service", fact.FundNumber(700), 500, "f"))

	got := map[LinkKind]Link{}
	for _, l := range buildFundFlows(t, facts, labels).Links {
		if l.Target != prefixRevenue+"taxes/property" {
			continue
		}
		if l.Source != prefixRevenueLine+lineOf("taxes/property") {
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
	facts := append(fundFlowsFacts(), fundFlowsFact(ScopeRevenueByFund, mapping.KindRevenue,
		"taxes/property", "", "capital", fact.FundNumber(600), 0, "z"))

	doc := buildFundFlows(t, facts, labels)
	var rollup *Link
	for i := range doc.Links {
		if doc.Links[i].Target == prefixRevenue+"taxes/property" {
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
				fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", nil, 1, "z"),
			},
			want: "names no fund",
		},
		{
			name: "an expenditure fact naming no department",
			facts: []fact.Fact{
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "", "general", fact.FundNumber(100), 1, "z"),
			},
			want: "carries no department",
		},
		{
			// pp.167-170 draw the General Fund through its divisions; a second
			// reading of it from expenditure-by-fund would draw it twice.
			name: "the General Fund from expenditure-by-fund",
			facts: []fact.Fact{
				fundFlowsFact(scopeExpenditureByFund, mapping.KindExpenditure, "wages-and-benefits", "", "general", fact.FundNumber(100), 1, "z"),
			},
			want: "draws fund 100 from expenditure-by-fund",
		},
		{
			name: "a fund filed under a group the registry does not put it in",
			facts: []fact.Fact{
				fundFlowsFact(scopeExpenditureByFund, mapping.KindExpenditure, "wages-and-benefits", "", "capital", fact.FundNumber(500), 1, "z"),
			},
			want: `files fund 500 under "capital"`,
		},
		{
			name: "a fund the registry does not list",
			facts: []fact.Fact{
				fundFlowsFact(scopeExpenditureByFund, mapping.KindExpenditure, "wages-and-benefits", "", "capital", fact.FundNumber(999), 1, "z"),
			},
			want: "fund 999 is in no data/funds.yaml entry",
		},
		{
			// The tier-5 id carries the DIVISION and not the fund, so two funds
			// spending on one division/object would collide on one link.
			name: "two funds on the expenditure side",
			facts: []fact.Fact{
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police", "general", fact.FundNumber(100), 1, "y"),
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police", "enterprise", fact.FundNumber(500), 1, "z"),
			},
			want: "names funds",
		},
		{
			// A row the taxonomy does not declare is refused, not drawn under
			// its category.
			name: "a revenue row no line is printed as",
			facts: []fact.Fact{
				fundFlowsFact(rev, mapping.KindRevenue, "taxes/sales", "", "general", fact.FundNumber(100), 1, "z"),
			},
			want: "no data/taxonomy.yaml line under",
		},
		{
			name: "a revenue row two lines claim",
			facts: []fact.Fact{
				fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1, "z"),
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
					f := fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1, "z")
					f.RowLabel = ""
					return f
				}(),
			},
			want: "carries no row label",
		},
		{
			name: "a fund the registry does not list",
			facts: []fact.Fact{
				fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", fact.FundNumber(999), 1, "z"),
			},
			want: "is in no data/funds.yaml entry",
		},
		{
			name: "a division the registry does not list",
			facts: []fact.Fact{
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police-department", "general", fact.FundNumber(100), 1, "z"),
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

// TestFundFlowsNeedsARegistry: the parent edge comes from data/funds.yaml, so a
// nil Labels is a hierarchy with nothing in it rather than a readable fallback.
// This is where it differs from Sankey, whose Labels is genuinely optional.
func TestFundFlowsNeedsARegistry(t *testing.T) {
	_, err := (&fundFlows{}).Document(fundFlowsFacts(), fundFlowsOptions())
	if err == nil || !strings.Contains(err.Error(), "no registry") {
		t.Fatalf("Document with no Labels = %v, want a refusal naming the registry", err)
	}
}

// TestFundFlowsIsDeterministic: two builds of the same facts are byte-identical,
// which is the contract every projection in this package holds to.
func TestFundFlowsIsDeterministic(t *testing.T) {
	p := &fundFlows{Labels: fundFlowsLabels()}
	first, err := p.Build(fundFlowsFacts(), fundFlowsOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for i := 0; i < 8; i++ {
		again, err := p.Build(fundFlowsFacts(), fundFlowsOptions())
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if string(again) != string(first) {
			t.Fatalf("build %d differs from the first; a map walk is leaking into the "+
				"output", i)
		}
	}
}

// TestFundFlowsSlicesDeclareOnlyColumnsBothSchedulesCarry: a column one schedule
// prints and the other does not would draw a whole revenue side against an empty
// expenditure side, which looks like a city that stopped spending.
func TestFundFlowsSlicesDeclareOnlyColumnsBothSchedulesCarry(t *testing.T) {
	facts := fundFlowsFacts()
	// A second column, revenue only.
	extra := fundFlowsFact(ScopeRevenueByFund, mapping.KindRevenue, "taxes/property", "",
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

// TestATransferInLinkIsNotExternal is the regression this document shipped and
// nothing caught.
//
// The tier-0 flows were classified by fund group alone -- boundaryKind, which
// asks "does this cross the city's boundary" and answers it correctly for
// REVENUE. A transfer is money moving between two city funds and crosses no
// boundary whatever group receives it, so 8 links carrying $21,045,597 went out
// as "external" while sankey.json published the same money as internal_transfer.
// Every check was green: values tied to facts, counts reconciled, the graph was
// acyclic. link-kinds-match-their-facts is the check side of this.
func TestATransferInLinkIsNotExternal(t *testing.T) {
	facts := fundFlowsFacts()
	for i := range facts {
		if facts[i].Kind == mapping.KindTransferIn {
			facts[i].AmountCents = 5000 // a printed zero earns no link at all
		}
	}
	doc := buildFundFlows(t, facts, fundFlowsLabels())

	seen := 0
	for _, l := range doc.Links {
		if l.Source != nodeTransfersIn {
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
		if strings.HasPrefix(l.Source, prefixRevenue) && l.Target == "fund/100" {
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
		fundFlowsFact(scopeExpenditureByDepartment, mapping.KindExpenditure,
			"wages-and-benefits", "police", "general", nil, 600, "d"),
	}
	_, err := (&fundFlows{Labels: fundFlowsLabels()}).Document(facts, fundFlowsOptions())
	if err == nil || !strings.Contains(err.Error(), "names no fund") {
		t.Fatalf("Document = %v, want a refusal naming the missing fund", err)
	}
}

// TestATierFiveParentIsCutAtTheFirstSlash: a division slug cannot contain one,
// but a taxonomy category routinely can (taxes/property, transfers/in), and a
// last-index cut would land inside the category and yield a parent that resolves
// to nothing.
func TestATierFiveParentIsCutAtTheFirstSlash(t *testing.T) {
	labels := fundFlowsLabels()
	labels.stubLabels["fund-balance/ending"] = "Ending Balance"
	facts := []fact.Fact{
		fundFlowsFact(ScopeRevenueByFund, mapping.KindRevenue, "taxes/property", "", "general", fact.FundNumber(100), 1, "a"),
		fundFlowsFact(scopeExpenditureByDepartment, mapping.KindExpenditure,
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
		prefixRevenueLine + "taxes/property/eraf"}
	reduction := fundFlowsFact(ScopeRevenueByFund, mapping.KindRevenue,
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
