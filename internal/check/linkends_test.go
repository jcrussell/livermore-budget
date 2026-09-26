package check

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestEveryIdFormSaysWhatItNames holds endNames to hierarchyTiers, so a form
// added to one is declared in the other rather than passed by omission.
func TestEveryIdFormSaysWhatItNames(t *testing.T) {
	if diff := cmp.Diff(slices.Sorted(maps.Keys(hierarchyTiers)), slices.Sorted(maps.Keys(endNames))); diff != "" {
		t.Errorf("hierarchyTiers and endNames name different forms (-tiers +names):\n%s", diff)
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
	// plant rewrites the first link of doc matching (source, target) and
	// restores it when the subtest ends.
	plant := func(t *testing.T, doc, source, target string, edit func(l *project.Link)) {
		t.Helper()
		links := docLinks(t, doc)
		for i := range links {
			if links[i].Source == source && links[i].Target == target {
				was := links[i]
				edit(&links[i])
				t.Cleanup(func() { links[i] = was })
				return
			}
		}
		t.Fatalf("%s draws no %s -> %s", doc, source, target)
	}
	for _, tc := range []struct {
		name, doc, source, target string
		edit                      func(l *project.Link)
		want                      string
	}{
		{"two departments swapped", "department-funding", "fund/100", "department/city-attorney",
			func(l *project.Link) { l.Target = "department/city-manager" },
			`department/city-manager names "city-manager"`},
		{"a fund's division re-pointed", "fund-flows", "fund/100", "dept/administrative-services",
			func(l *project.Link) { l.Target = "dept/building-and-safety" },
			`dept/building-and-safety names "building-and-safety"`},
		{"a transfer's receiver re-pointed", "transfers-by-fund", "transfer-from/100", "fund/210",
			func(l *project.Link) { l.Target = "fund/310" },
			`fund/310 names "310"`},
		{"a transfer's payer end re-pointed", "transfers-by-fund", "transfer-from/100", "fund/210",
			func(l *project.Link) { l.Source = "transfer-from/286" },
			`transfer-from/286 names fund 286 and the other leg`},
		{"a fund group into another group's fund", "fund-flows", "fund-group/capital", "fund/510",
			func(l *project.Link) { l.Target = "fund/100" },
			`fund/100 names "100"`},
		{"an id form nothing declares", "fund-flows", "fund/100", "dept/administrative-services",
			func(l *project.Link) { l.Target = "division/administrative-services" },
			"is no declared id form"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plant(t, tc.doc, tc.source, tc.target, tc.edit)
			res, err := c.Run(t.Context(), s)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != StatusFail || !strings.Contains(findingDetails(res), tc.want) {
				t.Fatalf("status %s, findings %v; want a fail saying %q", res.Status, res.Findings, tc.want)
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
