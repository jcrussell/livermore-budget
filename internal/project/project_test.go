package project

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

func TestOptionsValidate(t *testing.T) {
	ok := Options{
		Columns: []Column{{FiscalYear: 2026, Basis: vocab.BasisAdopted}},
		Scopes:  []string{"all-funds-gross"},
		Version: "dev",
	}
	col := func(o Options, year int, basis vocab.Basis) Options {
		o.Columns = []Column{{FiscalYear: year, Basis: basis}}
		return o
	}

	cases := []struct {
		name string
		o    Options
		want string // substring of the expected error; "" means valid
	}{
		{"valid", ok, ""},
		{"no columns", Options{Scopes: ok.Scopes, Version: ok.Version}, "at least one column is required"},
		{"no fiscal year", col(ok, 0, vocab.BasisAdopted), "fiscal year is required"},
		{"negative fiscal year", col(ok, -1, vocab.BasisAdopted), "fiscal year is required"},
		{"no basis", col(ok, 2026, ""), `basis "" is not one of`},
		{"unknown basis", col(ok, 2026, "guessed"), `basis "guessed" is not one of`},
		// A repeated column is refused because anything summing the document
		// would count it twice -- the same doubling Options exists to prevent,
		// reached from inside one document instead of across two.
		{"duplicate column", func() Options {
			o := ok
			o.Columns = []Column{{2026, vocab.BasisAdopted}, {2026, vocab.BasisAdopted}}
			return o
		}(), "column FY2026 adopted is listed twice"},
		// Several DISTINCT columns are valid at this level. Options is the type
		// a trends document shares with a Sankey; refusing more than one here
		// would make the multi-column document unrepresentable, and it is
		// Sankey.Graph that refuses to be of two.
		{"several columns", func() Options {
			o := ok
			o.Columns = []Column{{2026, vocab.BasisAdopted}, {2027, vocab.BasisAdopted}}
			return o
		}(), ""},
		{"no scope", func() Options { o := ok; o.Scopes = nil; return o }(),
			"at least one scope is required"},
		{"an empty scope", func() Options { o := ok; o.Scopes = []string{""}; return o }(),
			"a scope may not be empty"},
		// A repeated scope selects the same facts once, so nothing doubles --
		// but every reader that COUNTS scopes would see a document claiming
		// more schedules than it has.
		{"a repeated scope", func() Options {
			o := ok
			o.Scopes = []string{PublishedScope, PublishedScope}
			return o
		}(), "is listed twice"},
		{"no version", func() Options { o := ok; o.Version = ""; return o }(), "version is required"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.o.validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("got %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("got no error, want one containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	got := Registry(nil)
	// The names are asserted rather than the count alone, and in order: the
	// first is the document the site opens on, and a registry that quietly
	// reordered would move which document `fisc export` writes to data/sankey.json.
	want := []string{PublishedProjection, TrendsProjection, FundFlowsProjection,
		DepartmentSpendingProjection, DepartmentFundingProjection, TransfersByFundProjection,
		TransfersOutProjection, FundSourcesUsesProjection, ChangesProjection, FundBalancesProjection}
	if len(got) != len(want) {
		t.Fatalf("got %d projections, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name() != name {
			t.Errorf("projection %d is %q, want %q", i, got[i].Name(), name)
		}
	}
	// Two calls must not hand back the same instance: a caller that sets a
	// label registry on one must not be configuring everybody else's.
	if Registry(nil)[0] == got[0] {
		t.Error("got the same projection instance twice, want a fresh one per call")
	}
}

// TestTheOpeningPublishedYearOpensTheYearList pins a coherence property of this
// package's own constants, and it is the last edge of fisc-rmx.
//
// Stem gives the bare name to the slice at PublishedFiscalYear, while
// PublishedDocuments walks PublishedFiscalYears(). Nothing made the constant the
// first entry of the list, and editing one without the other is a source change
// no corpus can cause and no test then noticed: with the list at {2027} and the
// constant still 2026, the declaration names a document "sankey" over FY2027
// while every derivation gives that stem to FY2026. `fisc export` does refuse
// it, clearly -- but at the far end of the pipeline, about a file, rather than
// here, about the two lines that disagree.
func TestTheOpeningPublishedYearOpensTheYearList(t *testing.T) {
	years := PublishedFiscalYears()
	if len(years) == 0 {
		t.Fatal("PublishedFiscalYears is empty")
	}
	if years[0] != PublishedFiscalYear {
		t.Errorf("PublishedFiscalYears()[0] = %d, want PublishedFiscalYear (%d): Stem gives "+
			"the bare name to the opening year, so the two must be the same year or the "+
			"site declares one stem and every derivation computes another",
			years[0], PublishedFiscalYear)
	}
}

func TestFiscalYearLabel(t *testing.T) {
	cases := map[int]string{2026: "FY 2025-26", 2027: "FY 2026-27", 2000: "FY 1999-00", 2100: "FY 2099-00"}
	for year, want := range cases {
		if got := fiscalYearLabel(year); got != want {
			t.Errorf("fiscalYearLabel(%d) = %q, want %q", year, got, want)
		}
	}
}

func TestDollars(t *testing.T) {
	cases := map[int64]string{
		5961273400: "$59,612,734",
		0:          "$0",
		-100:       "-$1",
		123:        "$1.23",
	}
	for cents, want := range cases {
		if got := amount.Cents(cents).Dollars(); got != want {
			t.Errorf("amount.Cents(%d).Dollars() = %q, want %q", cents, got, want)
		}
	}
}

func TestSlugLabel(t *testing.T) {
	cases := map[string]string{
		"revenue/taxes/property":         "Property",
		"expenditure/wages-and-benefits": "Wages And Benefits",
		"fund-group/internal-service":    "Internal Service",
		"":                               "",
	}
	for id, want := range cases {
		if got := slugLabel(id); got != want {
			t.Errorf("slugLabel(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestOnlyScopeRefusesASetItCannotDescribe is the guard that replaced a field
// read.
//
// WHILE Options.Scopes WAS A STRING, "this document is of one schedule" was true
// by construction and nobody had to assert it. A set can hold two, so the three
// places that need a single scope -- Sankey.Graph, Trends.Document and
// envelope() -- each had to grow the same refusal, and the one that forgot would
// publish Scopes[0] into a singular metadata.scope: one schedule named as the
// whole of a document built over two. That is a false claim in a published file,
// and no check reads the JSON closely enough to catch it.
//
// The refusal is written once, here, and the three call it.
func TestOnlyScopeRefusesASetItCannotDescribe(t *testing.T) {
	one := Options{
		Columns: []Column{{FiscalYear: 2026, Basis: vocab.BasisAdopted}},
		Scopes:  []string{PublishedScope},
		Version: "test",
	}
	got, err := one.onlyScope()
	if err != nil || got != PublishedScope {
		t.Fatalf("OnlyScope over one scope = %q, %v, want %q, nil", got, err, PublishedScope)
	}

	two := one
	two.Scopes = []string{PublishedScope, TrendsScope}
	if _, err := two.onlyScope(); err == nil {
		t.Fatal("OnlyScope over two scopes = nil error, want a refusal")
	} else {
		// The message has to name BOTH, or a reader cannot tell which
		// declaration handed the wrong options over.
		for _, want := range []string{PublishedScope, TrendsScope} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("got %q, want it to name %q", err, want)
			}
		}
	}

	// Validate does not refuse the set -- a two-scope Options is legal, it is
	// only this document SHAPE that cannot describe one. Asserting that keeps
	// the two refusals from being collapsed into one by a later reader.
	if err := two.validate(); err != nil {
		t.Errorf("Validate over two scopes = %v, want nil: the set is legal", err)
	}
}

// TestASingleGrainDocumentRefusesTwoScopes is the same guard reached through the
// two projections that publish a singular metadata.scope.
//
// The lattice refuses the spine beside pp.127-140 first, with both cuts named;
// two summable schedules pass it and reach the sankey's own refusal.
func TestASingleGrainDocumentRefusesTwoScopes(t *testing.T) {
	both := []string{PublishedScope, TrendsScope}

	so := testOptions()
	so.Scopes = both
	if _, err := (&sankey{}).Document(spineFacts(t, testYear), so); err == nil {
		t.Error("Sankey.Graph over the spine and its decomposition = nil error, want a refusal")
	} else {
		for _, want := range []string{"not an antichain", `"revenue-detail"`, `"spine"`, "counts that money twice"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Sankey.Graph = %q, want the lattice to say %q", err, want)
			}
		}
	}

	so.Scopes = []string{TrendsScope, "expenditure-by-department"}
	if _, err := (&sankey{}).Document(spineFacts(t, testYear), so); err == nil {
		t.Error("Sankey.Graph over two summable schedules = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "one schedule") {
		t.Errorf("Sankey.Graph = %q, want it to say the document is of one schedule", err)
	}

	to := trendsOptions()
	to.Scopes = both
	if _, err := (&Trends{}).Document(trendsFixture(t), to); err == nil {
		t.Error("Trends.Document over two scopes = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "one schedule") {
		t.Errorf("Trends.Document = %q, want it to say the document is of one schedule", err)
	}
}

// TestTierOfRefusesACoinedForm pins the id forms' shape rules: a declared
// prefix with something after it, and a fund id that is a fund number.
func TestTierOfRefusesACoinedForm(t *testing.T) {
	for id, want := range map[string]int{
		"fund/100": tierFund, "fund/642": tierFund, "revenue/taxes/property": tierRevenueSource,
		NodeFundBalanceDraw: tierRevenueSource, NodeFundBalanceContribution: tierObjectCategory,
	} {
		if got, ok := TierOf(id); !ok || got != want {
			t.Errorf("TierOf(%q) = %d, %v; want %d, true", id, got, ok, want)
		}
	}
	for _, id := range []string{"fund/0", "fund/-1", "fund/+100", "fund/0100", "fund/general", "fund/", "fund", "revenue/", "dept", "fund-balance/change", "coined/x"} {
		if tier, ok := TierOf(id); ok {
			t.Errorf("TierOf(%q) = %d, true; want it refused", id, tier)
		}
	}
}

// TestSelectFactsRefusesAViewThatCannotBuild: a ThroughCuts selection over a
// scope no cut reads is an error, not an empty slice a caller that skipped
// validate would draw as an empty chart or pass a check over. The fact is in
// every other selector, so an empty answer is the swallow and not the filter.
func TestSelectFactsRefusesAViewThatCannotBuild(t *testing.T) {
	col := Column{FiscalYear: 2026, Basis: vocab.BasisAdopted}
	facts := []fact.Fact{{ID: "in", Scope: "no-such-scope", Kind: vocab.KindRevenue,
		FiscalYear: col.FiscalYear, Basis: col.Basis}}
	o := Options{Columns: []Column{col}, Scopes: []string{"no-such-scope"}, ThroughCuts: true}
	got, err := SelectFacts(facts, o)
	if err == nil || !strings.Contains(err.Error(), "which no declared cut reads") {
		t.Fatalf("SelectFacts = %v, %v; want a refusal naming the scope no cut reads", got, err)
	}
	o.ThroughCuts = false
	if got, err := SelectFacts(facts, o); err != nil || len(got) != 1 {
		t.Errorf("SelectFacts without ThroughCuts = %v, %v; want the one fact", got, err)
	}
}

// TestSelectFactsAppliesEverySelector holds the one fact selection the
// builders and the checks share to its three selectors: a fact outside the
// scopes, the kinds or the columns is not selected, and one inside all three
// is.
func TestSelectFactsAppliesEverySelector(t *testing.T) {
	col := Column{FiscalYear: 2026, Basis: vocab.BasisAdopted}
	mk := func(id, scope string, kind vocab.Kind, year int) fact.Fact {
		return fact.Fact{ID: id, Scope: scope, Kind: kind, FiscalYear: year, Basis: col.Basis}
	}
	facts := []fact.Fact{
		mk("in", TransfersByFundScope, vocab.KindTransferIn, 2026),
		mk("other-scope", PublishedScope, vocab.KindTransferIn, 2026),
		mk("other-kind", TransfersByFundScope, vocab.KindRevenue, 2026),
		mk("other-column", TransfersByFundScope, vocab.KindTransferIn, 2027),
	}
	o := Options{Columns: []Column{col}, Scopes: []string{TransfersByFundScope}, Kinds: transferKinds}
	selected, err := SelectFacts(facts, o)
	if err != nil {
		t.Fatalf("SelectFacts: %v", err)
	}
	var got []string
	for _, f := range selected {
		got = append(got, f.ID)
	}
	if !slices.Equal(got, []string{"in"}) {
		t.Errorf("SelectFacts = %v, want only the fact inside every selector", got)
	}
}

// TestEveryLinkKindHasWords holds the page's words for link kinds to the set:
// a kind with none would reach the reader as its snake_case name.
func TestEveryLinkKindHasWords(t *testing.T) {
	for _, k := range LinkKinds() {
		if LinkKindLabel(k) == "" {
			t.Errorf("link kind %q has no words for the page", k)
		}
	}
	if len(linkKindLabels) != len(LinkKinds()) {
		t.Errorf("%d kinds have words and %d kinds exist; a label for no kind is a promise about nothing",
			len(linkKindLabels), len(LinkKinds()))
	}
}
