package project

import (
	"strings"
	"testing"

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
}

func (s stubFundFlows) FundName(n int) (string, bool) { v, ok := s.names[n]; return v, ok }
func (s stubFundFlows) FundType(n int) (string, bool) { v, ok := s.types[n]; return v, ok }
func (s stubFundFlows) ConstraintTier(n int) string   { return s.tiers[n] }
func (s stubFundFlows) RestrictionNote(n int) string  { return s.notes[n] }
func (s stubFundFlows) DivisionLabel(d string) (string, bool) {
	v, ok := s.divisions[d]
	return v, ok
}

func fundFlowsLabels() stubFundFlows {
	return stubFundFlows{
		stubLabels: stubLabels{"taxes/property": "Property Taxes",
			"wages-and-benefits": "Wages & Benefits"},
		names:     map[int]string{100: "General Fund", 500: "Water"},
		types:     map[int]string{100: "general", 500: "enterprise"},
		tiers:     map[int]string{100: "discretionary", 500: "restricted-by-law"},
		notes:     map[int]string{100: "Available for any general city service.", 500: "Rates."},
		divisions: map[string]string{"police": "Police"},
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
func fundFlowsFact(scope string, kind mapping.Kind, category, department, group string,
	fund int, cents int64, id string) fact.Fact {
	return fact.Fact{
		ID: id, DocID: testDoc, Scope: scope, Kind: kind, Category: category,
		Department: department, FundGroup: group, Fund: fund,
		FiscalYear: testYear, Basis: testBasis, AmountCents: cents, Page: 127,
	}
}

func fundFlowsFacts() []fact.Fact {
	const rev, exp = ScopeRevenueByFund, ScopeExpenditureByDepartment
	return []fact.Fact{
		fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", 100, 1000, "a"),
		fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "enterprise", 500, 2000, "b"),
		// A printed zero: a fact that earns no link.
		fundFlowsFact(rev, mapping.KindTransferIn, "transfers/in", "", "general", 100, 0, "c"),
		fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police", "general", 100, 600, "d"),
		fundFlowsFact(exp, mapping.KindExpenditure, "services-and-supplies", "police", "general", 100, 400, "e"),
	}
}

func buildFundFlows(t *testing.T, facts []fact.Fact, l Labels) *FundFlowsDocument {
	t.Helper()
	doc, err := (&FundFlows{Labels: l}).Document(facts, fundFlowsOptions())
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
	if !containsString(doc.Metadata.Caveats, ConstraintTierCaveat()) {
		t.Error("the document publishes constraint tiers and its caveats do not disclose " +
			"that they are our reading")
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
	// Both expenditure facts are behind their own object link AND the
	// fund-to-department link that totals them.
	if c.FactsCitedTwice != 2 {
		t.Errorf("cited twice = %d, want 2: the two object rows are also in the division "+
			"total", c.FactsCitedTwice)
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
	const rev, exp = ScopeRevenueByFund, ScopeExpenditureByDepartment
	cases := []struct {
		name  string
		facts []fact.Fact
		opts  func(Options) Options
		want  string
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
				fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", 0, 1, "z"),
			},
			want: "names no fund",
		},
		{
			name: "an expenditure fact naming no department",
			facts: []fact.Fact{
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "", "general", 100, 1, "z"),
			},
			want: "carries no department",
		},
		{
			// The tier-5 id carries the DIVISION and not the fund, so two funds
			// spending on one division/object would collide on one link.
			name: "two funds on the expenditure side",
			facts: []fact.Fact{
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police", "general", 100, 1, "y"),
				fundFlowsFact(exp, mapping.KindExpenditure, "wages-and-benefits", "police", "enterprise", 500, 1, "z"),
			},
			want: "names funds",
		},
		{
			name: "a fund the registry does not list",
			facts: []fact.Fact{
				fundFlowsFact(rev, mapping.KindRevenue, "taxes/property", "", "general", 999, 1, "z"),
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
			_, err := (&FundFlows{Labels: fundFlowsLabels()}).Document(c.facts, o)
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
	_, err := (&FundFlows{}).Document(fundFlowsFacts(), fundFlowsOptions())
	if err == nil || !strings.Contains(err.Error(), "no registry") {
		t.Fatalf("Document with no Labels = %v, want a refusal naming the registry", err)
	}
}

// TestFundFlowsIsDeterministic: two builds of the same facts are byte-identical,
// which is the contract every projection in this package holds to.
func TestFundFlowsIsDeterministic(t *testing.T) {
	p := &FundFlows{Labels: fundFlowsLabels()}
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
		"general", 100, 5, "x")
	extra.FiscalYear = testYear + 1
	facts = append(facts, extra)

	got := (&FundFlows{Labels: fundFlowsLabels()}).Slices(facts, "test")
	if len(got) != 1 {
		t.Fatalf("slices = %d, want 1: only one column is printed by both schedules", len(got))
	}
	if got[0].Columns[0].FiscalYear != testYear {
		t.Errorf("slice covers FY%d, want FY%d", got[0].Columns[0].FiscalYear, testYear)
	}
}

func containsString(all []string, want string) bool {
	for _, s := range all {
		if s == want {
			return true
		}
	}
	return false
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

// TestTheExpenditureSideRefusesAFundlessFact: 0 is the no-fund sentinel, and an
// expenditure side of fund-0 facts would parent every department to fund/100 and
// source every division link from it -- attributing the whole of the spending to
// the General Fund on no evidence. Nothing downstream would see it: the amounts
// are unchanged, so expenditure-detail-ties-to-spine still ties, and
// fact-funds-resolve only examines facts that DO name a fund.
func TestTheExpenditureSideRefusesAFundlessFact(t *testing.T) {
	facts := []fact.Fact{
		fundFlowsFact(ScopeExpenditureByDepartment, mapping.KindExpenditure,
			"wages-and-benefits", "police", "general", 0, 600, "d"),
	}
	_, err := (&FundFlows{Labels: fundFlowsLabels()}).Document(facts, fundFlowsOptions())
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
		fundFlowsFact(ScopeRevenueByFund, mapping.KindRevenue, "taxes/property", "", "general", 100, 1, "a"),
		fundFlowsFact(ScopeExpenditureByDepartment, mapping.KindExpenditure,
			"fund-balance/ending", "police", "general", 100, 600, "d"),
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
