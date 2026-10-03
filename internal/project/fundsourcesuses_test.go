package project

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// committedStore is facts/facts.jsonl and the registry beside it, read once.
var committedStore = sync.OnceValues(func() (struct {
	facts []fact.Fact
	reg   *registry.Registry
}, error) {
	var out struct {
		facts []fact.Fact
		reg   *registry.Registry
	}
	f, err := os.Open(filepath.Join("..", "..", "facts", "facts.jsonl"))
	if err != nil {
		return out, err
	}
	defer f.Close()
	if out.facts, err = fact.Read(f); err != nil {
		return out, err
	}
	out.reg, err = registry.Load(os.DirFS(filepath.Join("..", "..", "data")))
	return out, err
})

// fundSourcesUsesOf builds the committed store's document of one column.
func fundSourcesUsesOf(t *testing.T, year int, basis mapping.Basis) *Document {
	t.Helper()
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	u := &fundSourcesUses{Labels: store.reg}
	for _, o := range u.Slices(store.facts, "fund-sources-uses test") {
		if o.Columns[0] == (Column{FiscalYear: year, Basis: basis}) {
			doc, err := u.Document(store.facts, o)
			if err != nil {
				t.Fatalf("Document: %v", err)
			}
			if _, err := u.Build(store.facts, o); err != nil {
				t.Fatalf("Build: %v", err)
			}
			return doc
		}
	}
	t.Fatalf("no FY%d %s slice", year, basis)
	return nil
}

// TestTheGeneralFundReadsAsPrinted holds the General Fund's FY2026 block to
// Budget Book p186-p187 and p66: its draw is p66's printed CHANGE IN WORKING
// CAPITAL (1,034,154), beginning 1,766,613 less ending 732,459.
func TestTheGeneralFundReadsAsPrinted(t *testing.T) {
	doc := fundSourcesUsesOf(t, 2026, mapping.BasisAdopted)
	got := map[string]int64{}
	for _, l := range doc.Links {
		if l.Source == "fund/100" {
			got["-> "+l.Target] = l.ValueCents
		}
		if l.Target == "fund/100" {
			got[l.Source+" ->"] = l.ValueCents
		}
	}
	want := map[string]int64{
		"revenue/revenues ->":              15787347000,
		"transfers/in ->":                  48040000,
		"fund-balance/draw ->":             103415400,
		"-> expenditure/expenses":          14465080200,
		"-> transfers/out":                 1003779700,
		"-> fund-balance/reserve-increase": 469942500,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fund/100's links (-want +got):\n%s", diff)
	}
	var general *Node
	for i := range doc.Nodes {
		if doc.Nodes[i].ID == "fund/100" {
			general = &doc.Nodes[i]
		}
	}
	if general == nil || general.Balances == nil || general.Balances.Beginning == nil || general.Balances.Ending == nil {
		t.Fatalf("fund/100 carries no balances: %+v", general)
	}
	if b, e := general.Balances.Beginning.ValueCents, general.Balances.Ending.ValueCents; b != 176661300 || e != 73245900 {
		t.Errorf("fund/100's balances are %d and %d, want 176661300 and 73245900", b, e)
	}
	if general.Parent != "fund-group/general" {
		t.Errorf("fund/100's parent is %q", general.Parent)
	}
}

// TestTheCIPBlockIsNotSelected: pp.190, 196, 202 and 208's Capital
// Improvement Program block carries enterprise fund groups, so a selection
// admitting it publishes FY2026 enterprise revenues of 71,573,173 where p198
// prints 67,514,261.
func TestTheCIPBlockIsNotSelected(t *testing.T) {
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	cip := map[string]bool{}
	for _, c := range structure.AllCuts() {
		if c.Scope == FundSourcesUsesScope {
			for _, r := range c.Rules {
				cip[r] = true
			}
		}
	}
	for _, o := range (&fundSourcesUses{}).Slices(store.facts, "test") {
		for _, f := range SelectFacts(store.facts, o) {
			if !cip[f.RuleID] {
				t.Errorf("%s selects fact %s of rule %s, which no cut of its scope reads", o.Columns[0], f.ID, f.RuleID)
			}
		}
	}

	doc := fundSourcesUsesOf(t, 2026, mapping.BasisAdopted)
	parent := map[string]string{}
	for _, n := range doc.Nodes {
		parent[n.ID] = n.Parent
	}
	var enterprise int64
	for _, l := range doc.Links {
		if l.Source == "revenue/revenues" && parent[l.Target] == "fund-group/enterprise" {
			enterprise += l.ValueCents
		}
	}
	if enterprise != 6751426100 {
		t.Errorf("FY2026 enterprise revenues are %d cents, want p198's 6751426100", enterprise)
	}
}

// TestEveryFundsSourcesEqualItsUses in the adopted columns, where the pages
// print every balance and no rounding.
func TestEveryFundsSourcesEqualItsUses(t *testing.T) {
	for _, year := range []int{2026, 2027} {
		doc := fundSourcesUsesOf(t, year, mapping.BasisAdopted)
		if got := FundImbalance(doc.Links); len(got) != 0 {
			t.Errorf("FY%d: funds whose sources and uses differ: %v", year, got)
		}
		for _, c := range doc.Metadata.Caveats {
			if c.ID == "sources-and-uses-differ" {
				t.Errorf("FY%d carries %q with every fund balanced", year, c.ID)
			}
		}
	}
}

// TestABlankBalanceDrawsNoChange: p187 leaves the Community Benefit Fund's
// FY2024 ending balance blank, so its change is not drawn and its printed
// beginning still rides on its node. FY2024's other funds miss the page
// identity by the city's rounding, which the document names fund by fund.
func TestABlankBalanceDrawsNoChange(t *testing.T) {
	doc := fundSourcesUsesOf(t, 2024, mapping.BasisActual)
	var blank []string
	for _, n := range doc.Nodes {
		if n.Balances == nil || (n.Balances.Beginning != nil && n.Balances.Ending != nil) {
			continue
		}
		blank = append(blank, n.ID)
		if n.Balances.Beginning == nil {
			t.Errorf("%s carries no beginning balance", n.ID)
		}
		for _, l := range doc.Links {
			if (l.Source == n.ID || l.Target == n.ID) && l.Derived {
				t.Errorf("%s has a blank balance and draws a change %s -> %s", n.ID, l.Source, l.Target)
			}
		}
	}
	if diff := cmp.Diff([]string{"fund/291"}, blank); diff != "" {
		t.Errorf("FY2024 funds with a blank balance (-want +got):\n%s", diff)
	}
	imbalance := FundImbalance(doc.Links)
	for _, c := range doc.Metadata.Caveats {
		if c.ID != "sources-and-uses-differ" {
			continue
		}
		if diff := cmp.Diff(slices.Sorted(maps.Keys(imbalance)), c.AppliesTo); diff != "" {
			t.Errorf("%q marks other funds than the unbalanced ones (-unbalanced +marked):\n%s", c.ID, diff)
		}
		if !strings.Contains(c.Text, "18 funds the printed figures themselves miss the identity, by at most $1,") {
			t.Errorf("%q does not name FY2024's rounding: %s", c.ID, c.Text)
		}
		return
	}
	t.Errorf("FY2024 carries no sources-and-uses-differ caveat")
}

// TestTheGrossChangeCaveatSumsTheDocumentsOwnLinks: FY2026's per-fund draws
// and contributions, gross, and netted by group to the spine's figures.
func TestTheGrossChangeCaveatSumsTheDocumentsOwnLinks(t *testing.T) {
	doc := fundSourcesUsesOf(t, 2026, mapping.BasisAdopted)
	for _, c := range doc.Metadata.Caveats {
		if c.ID != "each-fund-change-is-gross" {
			continue
		}
		for _, want := range []string{"36 funds draw $23,235,743", "25 funds contribute $26,323,176",
			"draws of $9,681,900 and contributions of $12,769,333"} {
			if !strings.Contains(c.Text, want) {
				t.Errorf("the caveat does not say %q: %s", want, c.Text)
			}
		}
		return
	}
	t.Fatal("FY2026 carries no each-fund-change-is-gross caveat")
}

// TestFundSourcesUsesRefusesASelectionNotThroughTheCuts: the CIP block is in
// scope and outside every cut, so only the flag leaves it out.
func TestFundSourcesUsesRefusesASelectionNotThroughTheCuts(t *testing.T) {
	store, err := committedStore()
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Columns: []Column{{2026, mapping.BasisAdopted}}, Scopes: FundSourcesUsesScopes(), Version: "test"}
	if _, err := (&fundSourcesUses{Labels: store.reg}).Document(store.facts, o); err == nil ||
		!strings.Contains(err.Error(), "through-cuts false") {
		t.Errorf("Document without ThroughCuts = %v, want a refusal", err)
	}
	o.ThroughCuts = true
	o.Scopes = []string{"no-such-scope"}
	if err := o.validate(); err == nil || !strings.Contains(err.Error(), "selecting through the cuts") {
		t.Errorf("validate over a scope no cut reads = %v, want a refusal", err)
	}
}

// TestChangeCentsSubtractsTheBeginning is the one spelling of a change.
func TestChangeCentsSubtractsTheBeginning(t *testing.T) {
	b := fact.Fact{Kind: mapping.KindFundBalance, Category: CategoryFundBalanceBeginning, AmountCents: 500}
	e := fact.Fact{Kind: mapping.KindFundBalance, Category: CategoryFundBalanceEnding, AmountCents: 200}
	c := fact.Fact{Kind: mapping.KindFundBalance, Category: CategoryFundBalanceChange, AmountCents: -300}
	r := fact.Fact{Kind: mapping.KindRevenue, Category: "revenues", AmountCents: 70}
	for _, tc := range []struct {
		facts []fact.Fact
		want  int64
	}{
		{[]fact.Fact{b, e}, -300},
		{[]fact.Fact{c}, -300},
		{[]fact.Fact{r, r}, 140},
	} {
		if got := ChangeCents(tc.facts); got != tc.want {
			t.Errorf("ChangeCents(%v) = %d, want %d", tc.facts, got, tc.want)
		}
	}
}
