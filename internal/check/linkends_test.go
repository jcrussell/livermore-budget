package check

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestEveryIdFormSaysWhatItNames holds endNames to project.IDForms, and
// every endpoint to a category, so a form or endpoint added to project is
// declared here rather than passed by omission.
func TestEveryIdFormSaysWhatItNames(t *testing.T) {
	if diff := cmp.Diff(project.IDForms(), slices.Sorted(maps.Keys(endNames))); diff != "" {
		t.Errorf("project's id forms and endNames differ (-project +names):\n%s", diff)
	}
	for _, id := range project.Endpoints() {
		if _, ok := project.EndpointCategory(id); !ok {
			t.Errorf("endpoint %q carries no category, so link-ends-match-their-facts cannot hold its facts", id)
		}
	}
}

// TestLinkEndsMatchTheirFactsIsFailable re-points one link per plant over the
// committed corpus. None moves a cent, so every other link check stays green.
func TestLinkEndsMatchTheirFactsIsFailable(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatal(err)
	}
	c := &linkEndsMatchTheirFacts{}
	res, err := c.Run(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusPass {
		t.Fatalf("the committed corpus is %s: %v", res.Status, res.Findings)
	}

	docLinks := func(t *testing.T, name string) []project.Link {
		t.Helper()
		for _, p := range s.linkedDocuments() {
			if p.Name == name {
				return p.Links
			}
		}
		t.Fatalf("no document %q", name)
		return nil
	}
	// find is the first link of doc's first document drawing source -> target.
	find := func(t *testing.T, doc, source, target string) *project.Link {
		t.Helper()
		links := docLinks(t, doc)
		for i := range links {
			if links[i].Source == source && links[i].Target == target {
				return &links[i]
			}
		}
		t.Fatalf("%s draws no %s -> %s", doc, source, target)
		return nil
	}
	type edit struct {
		doc, source, target string
		fn                  func(l *project.Link)
	}
	target := func(to string) func(l *project.Link) { return func(l *project.Link) { l.Target = to } }
	source := func(from string) func(l *project.Link) { return func(l *project.Link) { l.Source = from } }
	for _, tc := range []struct {
		name  string
		edits []edit
		want  string
	}{
		{"two departments swapped", []edit{{"department-funding", "fund/100", "department/city-attorney",
			target("department/city-manager")}},
			`department/city-manager names "city-manager"`},
		{"a department drawn as the division of one slug", []edit{{"department-funding", "fund/100",
			"department/city-attorney", target("dept/city-attorney")}},
			"dept/city-attorney names a division and fact"},
		{"a fund's division re-pointed", []edit{{"fund-flows", "fund/100", "dept/administrative-services",
			target("dept/building-and-safety")}},
			`dept/building-and-safety names "building-and-safety"`},
		{"a transfer's receiver re-pointed", []edit{{"transfers-by-fund", "transfer-from/100", "fund/210",
			target("fund/310")}},
			`fund/310 names "310"`},
		{"a transfer's payer end re-pointed", []edit{{"transfers-by-fund", "transfer-from/100", "fund/210",
			source("transfer-from/286")}},
			`transfer-from/286 names fund 286 and the other leg`},
		{"a transfer's receiver end re-pointed", []edit{{"transfers-by-fund", "fund/100", "transfer-to/210",
			target("transfer-to/310")}},
			`transfer-to/310 names fund 310 and the other leg`},
		{"a transfer's two legs collapsed into one", []edit{
			{"transfers-by-fund", "transfer-from/100", "fund/210", target("transfer-to/210")},
			{"transfers-by-fund", "fund/100", "transfer-to/210", source("transfer-from/100")}},
			`transfer-from/100 names fund 100 and the other leg`},
		{"a transfer end with no transfer id", []edit{{"transfers-by-fund", "transfer-from/100", "fund/210",
			func(l *project.Link) { l.TransferID = "" }}},
			"transfer-from/100 names a transfer's other leg and the link carries no transfer_id"},
		{"a fund group into another group's fund", []edit{{"fund-flows", "fund-group/capital", "fund/510",
			target("fund/100")}},
			`fund/100 names "100"`},
		{"a fund group into a fund no registry lists", []edit{{"fund-flows", "fund-group/capital", "fund/510",
			target("fund/999")}},
			"fund/999 is no fund data/funds.yaml lists"},
		{"a draw into another fund group", []edit{{"sankey", "fund-balance/draw", "fund-group/general",
			target("fund-group/special-revenue")}},
			`fund-group/special-revenue names "special-revenue"`},
		{"general's two object categories swapped", []edit{
			{"sankey", "fund-group/general", "expenditure/wages-and-benefits", target("expenditure/services-and-supplies")},
			{"sankey", "fund-group/general", "expenditure/services-and-supplies", target("expenditure/wages-and-benefits")}},
			`expenditure/services-and-supplies names "services-and-supplies"`},
		{"two revenue sources swapped into general", []edit{
			{"sankey", "revenue/taxes/sales", "fund-group/general", source("revenue/licenses-and-permits")},
			{"sankey", "revenue/licenses-and-permits", "fund-group/general", source("revenue/taxes/sales")}},
			`revenue/licenses-and-permits names "licenses-and-permits"`},
		{"capital's transfers out re-pointed to a contribution", []edit{{"sankey", "fund-group/capital",
			"transfers/out", target("fund-balance/contribution")}},
			`fund-balance/contribution carries category "fund-balance/change"`},
		{"a reserve increase re-pointed to a contribution", []edit{{"sankey", "fund-group/general",
			"fund-balance/reserve-increase", target("fund-balance/contribution")}},
			`fund-balance/contribution carries category "fund-balance/change"`},
		{"a contribution drawn as a draw", []edit{{"sankey", "fund-group/enterprise",
			"fund-balance/contribution", func(l *project.Link) {
				l.Source, l.Target = "fund-balance/draw", "fund-group/enterprise"
			}}},
			"fund-balance/draw cites facts summing to"},
		{"a draw drawn as a contribution", []edit{{"sankey", "fund-balance/draw", "fund-group/general",
			func(l *project.Link) {
				l.Source, l.Target = "fund-group/general", "fund-balance/contribution"
			}}},
			"fund-balance/contribution cites facts summing to"},
		{"a revenue line re-pointed to the category that shares its label", []edit{{"fund-flows",
			"revenue-line/charges-for-services/charges-for-services", "fund/210",
			source("revenue-line/charges-for-services")}},
			"revenue-line/charges-for-services is a line under"},
		{"a division's category swapped", []edit{{"department-spending", "expenditure/wages-and-benefits",
			"dept/city-attorney", source("expenditure/services-and-supplies")}},
			`expenditure/services-and-supplies names "services-and-supplies"`},
		{"one division's money in another division's box", []edit{{"fund-flows", "dept/city-attorney",
			"expenditure/city-attorney/wages-and-benefits", target("expenditure/city-council/wages-and-benefits")}},
			`expenditure/city-council/wages-and-benefits names "city-council/wages-and-benefits"`},
		{"one fund's spending in another fund's box", []edit{{"fund-flows", "fund/600",
			"expenditure/fund/600/wages-and-benefits", target("expenditure/fund/620/wages-and-benefits")}},
			`expenditure/fund/620/wages-and-benefits names "fund/620/wages-and-benefits"`},
		{"two revenue lines swapped into the general fund", []edit{
			{"fund-flows", "revenue-line/charges-for-services/library-fees", "fund/100",
				source("revenue-line/charges-for-services/weed-abatement")},
			{"fund-flows", "revenue-line/charges-for-services/weed-abatement", "fund/100",
				source("revenue-line/charges-for-services/library-fees")}},
			"revenue-line/charges-for-services/weed-abatement is not printed as"},
		{"an id form nothing declares", []edit{{"fund-flows", "fund/100", "dept/administrative-services",
			target("division/administrative-services")}},
			"is no declared id form"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Located before any is applied, so a swap's second edit finds
			// the link the first has not yet rewritten.
			var at []*project.Link
			for _, e := range tc.edits {
				at = append(at, find(t, e.doc, e.source, e.target))
			}
			for i, e := range tc.edits {
				was := *at[i]
				e.fn(at[i])
				t.Cleanup(func() { *at[i] = was })
			}
			res, err := c.Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), tc.want) {
				t.Fatalf("status %s, findings %v; want a fail saying %q", res.Status, res.Findings, tc.want)
			}
		})
	}

	// A link citing no fact holds its ends to nothing, so it is not counted.
	t.Run("links that cite no fact are not counted", func(t *testing.T) {
		for _, p := range s.linkedDocuments() {
			for i := range p.Links {
				was := p.Links[i].FactIDs
				p.Links[i].FactIDs = nil
				t.Cleanup(func() { p.Links[i].FactIDs = was })
			}
		}
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != StatusVacuous {
			t.Fatalf("status %s (%s), want vacuous", res.Status, res.Summary)
		}
	})

	// A ZERO SUM IS NEITHER A DRAW NOR A CONTRIBUTION.
	for _, end := range [][2]string{
		{"fund-balance/draw", "fund-group/general"},
		{"fund-group/enterprise", "fund-balance/contribution"},
	} {
		t.Run("a zero-sum "+end[0]+" -> "+end[1], func(t *testing.T) {
			l := find(t, "sankey", end[0], end[1])
			orig := s.Facts
			facts := slices.Clone(orig)
			s.Facts = facts
			t.Cleanup(func() { s.Facts = orig })
			for j := range facts {
				if slices.Contains(l.FactIDs, facts[j].ID) {
					facts[j].AmountCents = 0
				}
			}
			res, err := c.Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), "cites facts summing to 0 cents") {
				t.Fatalf("status %s, findings %v; want a fail on the zero sum", res.Status, res.Findings)
			}
		})
	}

	// THE REGISTRY ARM ALONE: the link and its facts agree on a group that
	// data/funds.yaml does not put the fund in.
	t.Run("a fund in a group data/funds.yaml does not put it in", func(t *testing.T) {
		links := docLinks(t, "fund-flows")
		i := slices.IndexFunc(links, func(l project.Link) bool {
			return l.Source == "fund-group/capital" && l.Target == "fund/510"
		})
		if i < 0 {
			t.Fatal("fund-flows draws no fund-group/capital -> fund/510")
		}
		was := links[i]
		links[i].Source = "fund-group/special-revenue"
		orig := s.Facts
		facts := slices.Clone(orig)
		s.Facts = facts
		t.Cleanup(func() { s.Facts = orig })
		for j := range facts {
			if slices.Contains(was.FactIDs, facts[j].ID) {
				facts[j].FundGroup = "special-revenue"
			}
		}
		t.Cleanup(func() { links[i] = was })
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != StatusFail || len(res.Findings) != 1 ||
			!strings.Contains(findingDetails(res), `puts fund 510 in "capital", not "special-revenue"`) {
			t.Fatalf("status %s, findings %v; want one fail naming the registry's group", res.Status, res.Findings)
		}
	})
}

// TestDepartmentTiersRefusesAScopeTwoCutsDisagreeOn: a scope declared at both
// tiers matches neither form.
func TestDepartmentTiersRefusesAScopeTwoCutsDisagreeOn(t *testing.T) {
	got := departmentTiers([]structure.Cut{
		{Scope: "a", DepartmentTier: "division"},
		{Scope: "a", DepartmentTier: "department"},
		{Scope: "a", DepartmentTier: "division"},
		{Scope: "b", DepartmentTier: "department"},
		{Scope: "c"},
	})
	if diff := cmp.Diff(map[string]string{"a": "", "b": "department"}, got); diff != "" {
		t.Errorf("departmentTiers (-want +got):\n%s", diff)
	}
}
