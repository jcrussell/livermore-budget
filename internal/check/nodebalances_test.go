package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestFundChangesAndBalancesAreFailable plants, over the committed corpus,
// the defects a fund's change link and its node balances can carry. None of
// the balance plants moves a link, so only node-balances-tie-to-facts sees
// them.
func TestFundChangesAndBalancesAreFailable(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	var doc *linked
	for _, p := range s.linkedDocuments() {
		if p.Name == project.FundSourcesUsesProjection && p.Options.Columns[0].FiscalYear == 2026 {
			doc = &p
		}
	}
	if doc == nil {
		t.Fatal("no FY2026 fund-sources-uses document was built")
	}
	node := func(t *testing.T, id string) *project.Node {
		t.Helper()
		for i := range doc.Nodes {
			if doc.Nodes[i].ID == id {
				return &doc.Nodes[i]
			}
		}
		t.Fatalf("no node %s", id)
		return nil
	}
	run := func(t *testing.T, c Check) Result {
		t.Helper()
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for _, c := range []Check{&linkValuesTieToFacts{}, &nodeBalancesTieToFacts{}} {
		if res := run(t, c); res.Status != StatusPass {
			t.Fatalf("%s over the committed corpus is %s: %v", c.ID(), res.Status, res.Findings)
		}
	}

	t.Run("a draw valued as the plain sum of its two balances", func(t *testing.T) {
		for i := range doc.Links {
			l := &doc.Links[i]
			if l.Source != project.NodeFundBalanceDraw || l.Target != "fund/100" {
				continue
			}
			// 1,766,613 + 732,459: what a check summing the facts would want.
			was := l.ValueCents
			l.ValueCents = 249907200
			t.Cleanup(func() { l.ValueCents = was })
			res := run(t, &linkValuesTieToFacts{})
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), "facts come to $1,034,154.00") {
				t.Fatalf("status %s, findings %v; want the General Fund's draw refused", res.Status, res.Findings)
			}
			return
		}
		t.Fatal("FY2026 draws nothing into fund/100")
	})

	link := func(t *testing.T, src, dst string) *project.Link {
		t.Helper()
		for i := range doc.Links {
			if doc.Links[i].Source == src && doc.Links[i].Target == dst {
				return &doc.Links[i]
			}
		}
		t.Fatalf("no link %s -> %s", src, dst)
		return nil
	}
	beginning := node(t, "fund/100").Balances.Beginning

	t.Run("an expenses link netting a beginning balance off", func(t *testing.T) {
		l := link(t, "fund/100", "expenditure/expenses")
		was, wasIDs := l.ValueCents, l.FactIDs
		l.FactIDs = append([]string{beginning.FactID}, l.FactIDs...)
		l.ValueCents -= beginning.ValueCents
		t.Cleanup(func() { l.ValueCents, l.FactIDs = was, wasIDs })
		res := run(t, &linkValuesTieToFacts{})
		if res.Status != StatusFail {
			t.Fatalf("status %s; a link outside a change endpoint subtracted a balance and tied", res.Status)
		}
	})

	t.Run("a draw citing its beginning balance alone", func(t *testing.T) {
		l := link(t, project.NodeFundBalanceDraw, "fund/100")
		was, wasIDs := l.ValueCents, l.FactIDs
		l.FactIDs, l.ValueCents = []string{beginning.FactID}, beginning.ValueCents
		t.Cleanup(func() { l.ValueCents, l.FactIDs = was, wasIDs })
		if res := run(t, &linkValuesTieToFacts{}); res.Status != StatusPass {
			t.Fatalf("link-values-tie-to-facts is %s; the plant must tie its value so only the ends can see it", res.Status)
		}
		res := run(t, &linkEndsMatchTheirFacts{})
		if res.Status != StatusFail || !strings.Contains(findingDetails(res), "exactly one") {
			t.Fatalf("status %s, findings %v; want the draw refused for citing one balance", res.Status, res.Findings)
		}
	})

	t.Run("a printed balance the node does not publish", func(t *testing.T) {
		n := node(t, "fund/100")
		was := *n.Balances
		n.Balances = &project.NodeBalances{Beginning: was.Beginning}
		t.Cleanup(func() { n.Balances = &was })
		res := run(t, &nodeBalancesTieToFacts{})
		if res.Status != StatusFail || !strings.Contains(findingDetails(res), "the fund's node does not publish it") {
			t.Fatalf("status %s, findings %v; want the General Fund's dropped ending named", res.Status, res.Findings)
		}
	})

	general := node(t, "fund/100")
	other := node(t, "fund/200")
	for _, tc := range []struct {
		name  string
		plant func(b *project.NodeBalances)
		want  string
	}{
		{"beginning and ending swapped", func(b *project.NodeBalances) {
			b.Beginning, b.Ending = b.Ending, b.Beginning
		}, `under "fund-balance/ending"`},
		{"a balance off by a cent", func(b *project.NodeBalances) {
			c := *b.Ending
			c.ValueCents++
			b.Ending = &c
		}, "prints $732,459.00"},
		{"another fund's balance", func(b *project.NodeBalances) {
			b.Ending = other.Balances.Ending
		}, "which is fund 200's"},
		{"a balance located on another page", func(b *project.NodeBalances) {
			c := *b.Ending
			c.Locators = []project.Source{{DocID: c.Locators[0].DocID, Pages: []int{1}}}
			b.Ending = &c
		}, "locators are"},
		{"a fact of no slice", func(b *project.NodeBalances) {
			c := *b.Ending
			c.FactID = "fisc-f-000000000000"
			b.Ending = &c
		}, "not in this projection's slice"},
		{"a change off by a cent", func(b *project.NodeBalances) {
			c := *b.Change
			c.ValueCents++
			b.Change = &c
		}, "come to -$1,034,154.00"},
		{"a change taken the wrong way round", func(b *project.NodeBalances) {
			c := *b.Change
			c.ValueCents = -c.ValueCents
			b.Change = &c
		}, "come to -$1,034,154.00"},
		{"a change not marked derived", func(b *project.NodeBalances) {
			c := *b.Change
			c.Derived = false
			b.Change = &c
		}, "not marked derived"},
		{"a change citing another fund's balance", func(b *project.NodeBalances) {
			c := *b.Change
			c.FactIDs = []string{b.Beginning.FactID, other.Balances.Ending.FactID}
			b.Change = &c
		}, "the node's balances are facts"},
		{"a change beside a blank balance", func(b *project.NodeBalances) {
			b.Ending = nil
		}, "a balance is blank"},
		{"two printed balances and no change", func(b *project.NodeBalances) {
			b.Change = nil
		}, "states no change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			was := *general.Balances
			planted := was
			tc.plant(&planted)
			general.Balances = &planted
			t.Cleanup(func() { general.Balances = &was })
			res := run(t, &nodeBalancesTieToFacts{})
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), tc.want) {
				t.Fatalf("status %s, findings %v; want a fail saying %q", res.Status, res.Findings, tc.want)
			}
		})
	}
}

// TestTheExpansionsFundBalancesAreFailable plants on a fund the expansion
// draws apart from its change, so node-balances-tie-to-facts is shown to read
// the expansion's nodes and not fund-sources-uses' alone, and to refuse a
// drawn fund that drops the balances its slice prints.
func TestTheExpansionsFundBalancesAreFailable(t *testing.T) {
	t.Parallel()
	s := isolated(t)
	var node *project.Node
	for _, p := range s.linkedDocuments() {
		if p.Name != project.ExpansionProjection || p.Options.Columns[0].FiscalYear != 2026 {
			continue
		}
		var apart []string
		for _, c := range p.Graph.Metadata.Caveats {
			if c.ID == "some-fund-group-money-is-not-by-fund" {
				apart = c.AppliesTo
			}
		}
		for i := range p.Nodes {
			n := &p.Nodes[i]
			if slices.Contains(apart, n.ID) && strings.HasPrefix(n.ID, project.PrefixFund) &&
				n.Balances != nil && n.Balances.Change != nil {
				node = n
				break
			}
		}
	}
	if node == nil {
		t.Fatal("no FY2026 expansion fund drawn apart from its change carries balances and a change")
	}
	run := func(t *testing.T) Result {
		t.Helper()
		res, err := (&nodeBalancesTieToFacts{}).Run(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := run(t); res.Status != StatusPass {
		t.Fatalf("over the committed corpus: %s %v", res.Status, res.Findings)
	}
	t.Run("a drawn fund carrying no balances", func(t *testing.T) {
		was := node.Balances
		node.Balances = nil
		t.Cleanup(func() { node.Balances = was })
		res := run(t)
		if res.Status != StatusFail || len(res.Findings) != 2 ||
			!strings.Contains(findingDetails(res), "the fund's node does not publish it") {
			t.Fatalf("status %s, findings %v; want %s's two balances named unpublished", res.Status, res.Findings, node.ID)
		}
	})
	for _, tc := range []struct {
		name  string
		plant func(b *project.NodeBalances)
		want  string
	}{
		{"a change off by a cent", func(b *project.NodeBalances) {
			c := *b.Change
			c.ValueCents++
			b.Change = &c
		}, project.ExpansionProjection},
		{"an ending balance off by a cent", func(b *project.NodeBalances) {
			c := *b.Ending
			c.ValueCents++
			b.Ending = &c
		}, project.ExpansionProjection},
	} {
		t.Run(tc.name, func(t *testing.T) {
			was := node.Balances
			planted := *was
			tc.plant(&planted)
			node.Balances = &planted
			t.Cleanup(func() { node.Balances = was })
			res := run(t)
			subjects := ""
			for _, f := range res.Findings {
				subjects += f.Subject + "\n"
			}
			if res.Status != StatusFail || len(res.Findings) != 1 ||
				!strings.HasPrefix(subjects, tc.want+" ") || !strings.Contains(subjects, " "+node.ID+" ") {
				t.Fatalf("status %s, findings %v; want %s on the expansion refused", res.Status, res.Findings, node.ID)
			}
			t.Logf("%s on %s: %v", tc.name, node.ID, res.Findings)
		})
	}
}
