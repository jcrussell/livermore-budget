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
	// departments is the ALL-CAPS tier, kept apart from divisions for the
	// reason data/departments.yaml keeps two namespaces: five slugs name both,
	// so one map would answer either and a projection reading the wrong tier
	// would be invisible here.
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

// DepartmentLabel answers from a map of its own, so a fixture can give a
// department and a division the same slug and still tell which tier a
// projection asked for.
func (s stubFundFlows) DepartmentLabel(d string) (string, bool) {
	v, ok := s.departments[d]
	return v, ok
}

// LinesPrintedAs answers only what a test declared. A miss is the registry's
// "no line is printed as this", which the projection refuses -- so a fixture
// that forgets a row fails loudly rather than drawing it under its category.
func (s stubFundFlows) LinesPrintedAs(parent, printed, kind string) []string {
	return s.lines[lineKey{parent, printed, kind}]
}

// printedRow is the row label this fixture prints for a category, and
// lineOf is the line slug it resolves to. Every schedule row is printed under
// some category, so one row per category is what the fixture needs everywhere
// the line TIER is not itself the subject.
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
// factID is the id of the fixture fact `tag` names, so an assertion can read by
// the handle it was written with while the document carries an id of the shape
// a store holds.
func factID(tag string) string { return fmt.Sprintf("fisc-f-%012x", tag[0]) }

// fundFlowsFact builds one fact of this file's fixture.
//
// tag IS A HANDLE AND THE ID IS BUILT FROM IT, rather than the handle being the
// id: schema/fact-id.schema.json says a fact id is `fisc-f-` and twelve hex
// digits, so a link citing "a" is a link no store can hold and no published
// document can contain. A fixture shaped like something that cannot occur is
// one whose tests pass against a document the tree would refuse.
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

// TestTheTruncatedGroupCountIsTheDocumentsOwn is the seven-group column that
// justifies computing this rather than writing it down.
//
// THE CAVEAT SAID "the other six fund groups" AND SIX IS TRUE OF ONE COLUMN.
// fund-flows-2024-actual carries a seventh fund group, permanent, so six of its
// seven stop short; the other three published columns carry six and FIVE stop --
// including FY2025-26, which is the one the site draws. The page shipped a
// figure that was wrong about the chart beside it.
//
// AND THE FIX WAS ASSERTED BY NOTHING. Replacing groupsWithNoSpendingSide's
// body with `return 5` -- the literal the correction was about -- left every Go
// test passing, which is the same shape as the defect: a number nobody checks.
func TestTheTruncatedGroupCountIsTheDocumentsOwn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups []string
		want   int
	}{
		// Six groups, one of which (general) has divisions beneath it.
		{"six groups, five stop", []string{"general", "capital", "enterprise",
			"special-revenue", "debt-service", "internal-service"}, 5},
		// The FY2023-24 shape: a seventh group, permanent, and six stop.
		{"seven groups, six stop", []string{"general", "capital", "enterprise",
			"special-revenue", "debt-service", "internal-service", "permanent"}, 6},
		{"one group, none stop", []string{"general"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := []Node{
				// The General Fund alone is decomposed: a fund with a division
				// beneath it is what "has a spending side" means here.
				{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
				{ID: prefixDept + "patrol", Tier: tierDepartment, Parent: prefixFund + "100"},
			}
			for _, g := range tc.groups {
				nodes = append(nodes, Node{ID: prefixFundGroup + g, Tier: tierFundGroup})
			}
			got := truncatedGroups(nodes)
			if len(got) != tc.want {
				t.Errorf("truncatedGroups = %v (%d), want %d", got, len(got), tc.want)
			}
			// AND THE MARKS ARE THE SAME SET PLUS THE EXCEPTION. The count and
			// the applies_to list came from two computations and disagreed: the
			// caveat said "the other 5 groups" and marked six, general among
			// them. One function feeds both now, and this is what says so.
			marks := appliesToTruncatedGroups(nodes)
			want := append(append([]string{}, got...), prefixFund+"100")
			slices.Sort(want)
			if diff := cmp.Diff(want, marks); diff != "" {
				t.Errorf("applies_to (-want +got):\n%s", diff)
			}
			for _, m := range marks {
				if m == prefixFundGroup+"general" {
					t.Error("the marks include fund-group/general, which is the exception " +
						"the sentence excludes from its count; fund/100 is how it is named")
				}
			}
		})
	}
}

// TestAColumnThatDecomposesNothingDoesNotClaimTheGeneralFundIsSpecial is the
// latent arm of the caveat above.
//
// It opens "Only the General Fund has a spending side", and a column with no
// department rows has none -- so every group counts as truncated and the
// sentence would read "the other 6 groups' revenue ends at their funds" out of
// six, over a document that decomposes nothing at all. All four published
// columns carry pp.167-170 today, so this shape reaches no reader; it also
// reaches no test unless one builds it, and a caveat describing a distinction
// the document does not draw is worse than a missing one, because it reads as
// though the distinction was checked.
func TestAColumnThatDecomposesNothingDoesNotClaimTheGeneralFundIsSpecial(t *testing.T) {
	groups := []Node{
		{ID: prefixFundGroup + "general", Tier: tierFundGroup},
		{ID: prefixFundGroup + "capital", Tier: tierFundGroup},
		{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
	}
	has := func(cs []Caveat, id string) bool {
		for _, c := range cs {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	const id = "only-the-general-fund-is-decomposed"

	if got := fundFlowsCaveats(0, groups); has(got, id) {
		t.Errorf("a column with no division rows still carries %q; it says only the "+
			"General Fund has a spending side, and nothing here has one", id)
	}

	// AND A COLUMN THAT DECOMPOSES SOMETHING ELSE. "Anything is decomposed" was
	// the first guard and it is not what the sentence claims: a column with
	// divisions under a capital fund and none under the General Fund satisfies
	// it and makes "Only the General Fund has a spending side" false -- while
	// the applies_to beside it would list fund-group/general among the groups
	// the sentence counts as stopping, which is the contradiction this lane
	// removed.
	elsewhere := []Node{
		{ID: prefixFundGroup + "general", Tier: tierFundGroup},
		{ID: prefixFundGroup + "capital", Tier: tierFundGroup},
		{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
		{ID: prefixFund + "510", Tier: tierFund, Parent: prefixFundGroup + "capital"},
		{ID: prefixDept + "parks", Tier: tierDepartment, Parent: prefixFund + "510"},
	}
	if got := fundFlowsCaveats(0, elsewhere); has(got, id) {
		t.Errorf("a column decomposing capital and not the General Fund carries %q, "+
			"which says the opposite", id)
	}
	// And with one division it comes back, so the condition is not simply off.
	withDivision := append(append([]Node{}, groups...),
		Node{ID: prefixDept + "patrol", Tier: tierDepartment, Parent: prefixFund + "100"})
	if got := fundFlowsCaveats(0, withDivision); !has(got, id) {
		t.Errorf("a column that DOES decompose the General Fund carries no %q", id)
	}
}

// TestTheTruncatedCountIsPluralised is the one-group column.
//
// Every number in these caveats was a literal until this lane, and every
// literal was written for one column. The moment they became computed, a column
// with exactly one truncated group would publish "the other 1 groups' revenue
// ends at their funds" -- in the caveat, on the caveats page, and in every
// tooltip badge that shows the summary. No published column has that shape, so
// nothing but this reaches it.
func TestTheTruncatedCountIsPluralised(t *testing.T) {
	// Two groups, one decomposed: exactly one stops.
	nodes := []Node{
		{ID: prefixFundGroup + "general", Tier: tierFundGroup},
		{ID: prefixFundGroup + "capital", Tier: tierFundGroup},
		{ID: prefixFund + "100", Tier: tierFund, Parent: prefixFundGroup + "general"},
		{ID: prefixDept + "patrol", Tier: tierDepartment, Parent: prefixFund + "100"},
	}
	if n := len(truncatedGroups(nodes)); n != 1 {
		t.Fatalf("%d groups stop short, want 1; this test is about the singular", n)
	}
	for _, c := range fundFlowsCaveats(0, nodes) {
		if c.ID != "only-the-general-fund-is-decomposed" {
			continue
		}
		for _, bad := range []string{"1 groups", "1 fund groups"} {
			if strings.Contains(c.Summary, bad) || strings.Contains(c.Text, bad) {
				t.Errorf("the caveat says %q:\n  %s\n  %s", bad, c.Summary, c.Text)
			}
		}
		if !strings.Contains(c.Summary, "1 group's") {
			t.Errorf("the summary does not read \"1 group's\": %s", c.Summary)
		}
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
	// FOUR, AND THE TWO OVERLAPS ARE NOT THE SAME SHAPE. Both expenditure facts
	// are behind their own object link AND the fund-to-department link that
	// totals them; both revenue facts are behind their own flow into a fund AND
	// the rollup of their line into Property Taxes. The printed zero is behind
	// neither, which is what keeps it in facts_uncited above.
	if c.FactsCitedTwice != 4 {
		t.Errorf("cited twice = %d, want 4: the two object rows are also in the division "+
			"total, and the two revenue rows are also in their line's rollup",
			c.FactsCitedTwice)
	}
}

// TestALineRollsUpIntoItsCategoryOncePerKind is the (1,0) link's shape, and the
// per-kind half of it is held by nothing else in the tree.
//
// ONE PRINTED ROW REACHES THE CITY UNDER TWO KINDS. pp.127-140 print rows whose
// money lands in the five Internal Service Funds as an internal service charge
// and in the rest of the city as external revenue -- 2 of the 93 lines in both
// published columns. A single rollup would have to publish one of those two
// answers for all of it, and nothing else would notice: the value would still
// tie to the facts it cites, every one of those facts is a revenue row so
// link-kinds-match-their-facts stays quiet, and checkDistinctLinks refuses two
// links of ONE kind on a pair rather than a pair carrying two kinds.
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
	// PUBLISHED, NOT INFERRED. A rollup of rows the city printed is a reading of
	// the page and not a model of it, so the flag that would draw it dashed and
	// list it under "what we inferred" stays false.
	for k, l := range got {
		if l.Derived {
			t.Errorf("the %s rollup is published as derived", k)
		}
	}
}

// TestAPrintedZeroIsNotInItsLinesRollup holds the one place the tier-3-to-4 rule
// is deliberately not copied.
//
// A DASH IS A FACT AND NOT A FLOW on this side of the document: the division
// total sums every object cell including the zeros and cites them all, while a
// revenue row printed as a dash earns no link and is counted in facts_uncited.
// Rolling the zeros up would cite them, move that number and contradict
// TestThePrintedZeroRowsAreNotNodes -- and the value would not move a cent,
// which is why only a test of the CITATION can see it.
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
	// The dash is still a fact, and the identity is where it is accounted for.
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
			name:  "one scope where two are required",
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
			// A ROW THE TAXONOMY DOES NOT DECLARE. The tempting half-measure is
			// to draw it under its category, which ties to the spine and leaves
			// a ribbon that looks like one more row while being the remainder of
			// every row nobody declared.
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

// TestTheExpenditureSideRefusesAFundlessFact: an expenditure side of fundless
// facts would parent every department to fund/100 and source every division
// link from it -- attributing the whole of the spending to the General Fund on
// no evidence. Nothing downstream would see it: the amounts are unchanged, so
// cuts-tie-along-the-lattice still ties, and fact-funds-resolve only examines
// facts that DO name a fund.
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
