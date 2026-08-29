package project

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
)

// TestLocatorSetGroupsSortsAndDeduplicates pins the one grouping rule this
// package has, because two readers depend on it agreeing with itself: a
// document's metadata.sources and every link's locators go through it, and the
// client composes a URL from both with the same function.
func TestLocatorSetGroupsSortsAndDeduplicates(t *testing.T) {
	facts := []fact.Fact{
		{DocID: "budget", Page: 67},
		{DocID: "acfr", Page: 177},
		{DocID: "budget", Page: 66},
		{DocID: "budget", Page: 67}, // the same page twice
		{DocID: "acfr", Page: 177},  // and in the other document
	}
	var l locatorSet
	for i := range facts {
		l.add(&facts[i])
	}
	want := []Source{
		{DocID: "acfr", Pages: []int{177}},
		{DocID: "budget", Pages: []int{66, 67}},
	}
	if diff := cmp.Diff(want, l.sources()); diff != "" {
		t.Errorf("sources() mismatch (-want +got):\n%s", diff)
	}
}

// TestAnEmptyLocatorSetIsNotNull guards the contract rather than the code.
// docs/sankey-contract.md requires every key present on every object with no
// null, so a link built from no facts must publish [] and not null.
func TestAnEmptyLocatorSetIsNotNull(t *testing.T) {
	var l locatorSet
	got := l.sources()
	if got == nil {
		t.Error("sources() over an empty set = nil, want an empty slice; a nil " +
			"encodes as null and the contract forbids it")
	}
	if len(got) != 0 {
		t.Errorf("sources() over an empty set = %v, want empty", got)
	}
}

// TestLocatorSetMergeUnions covers what the drill-down's tier-3-to-4 rollup
// needs: that link cites every fact of every cell beneath it, so its locators
// must be the union rather than the first cell's.
func TestLocatorSetMergeUnions(t *testing.T) {
	mk := func(pairs ...any) *locatorSet {
		var l locatorSet
		for i := 0; i < len(pairs); i += 2 {
			f := fact.Fact{DocID: pairs[i].(string), Page: pairs[i+1].(int)}
			l.add(&f)
		}
		return &l
	}
	a := mk("budget", 168)
	a.merge(mk("budget", 167, "budget", 168, "acfr", 3))
	want := []Source{
		{DocID: "acfr", Pages: []int{3}},
		{DocID: "budget", Pages: []int{167, 168}},
	}
	if diff := cmp.Diff(want, a.sources()); diff != "" {
		t.Errorf("merged sources() mismatch (-want +got):\n%s", diff)
	}

	// Merging into an empty set is the case the lazy map makes easy to get
	// wrong, and the rollup hits it on its first cell every time.
	var empty locatorSet
	empty.merge(mk("budget", 66))
	if diff := cmp.Diff([]Source{{DocID: "budget", Pages: []int{66}}}, empty.sources()); diff != "" {
		t.Errorf("merge into an empty set mismatch (-want +got):\n%s", diff)
	}
}

// TestEveryLinkLocatorIsAPageItsFactsWereReadFrom is the end-to-end claim, over
// the real spine fixture: a published link's locators are exactly the distinct
// (doc_id, page) of the facts it names, and nothing else.
//
// It re-derives the grouping from the facts rather than calling sourcesOf,
// because a check that asks the producer for the answer is not a check.
func TestEveryLinkLocatorIsAPageItsFactsWereReadFrom(t *testing.T) {
	facts := spineFacts(t, testYear)
	byID := make(map[string]fact.Fact, len(facts))
	for _, f := range facts {
		byID[f.ID] = f
	}
	g := buildGraph(t, facts, testOptions())

	if len(g.Links) == 0 {
		t.Fatal("the fixture built no links, so this test asserts nothing")
	}
	for _, l := range g.Links {
		if l.Locators == nil {
			t.Errorf("link %s -> %s has nil locators", l.Source, l.Target)
			continue
		}
		pages := map[string]map[int]bool{}
		for _, id := range l.FactIDs {
			f, ok := byID[id]
			if !ok {
				t.Fatalf("link %s -> %s cites fact %s, which the fixture does not carry",
					l.Source, l.Target, id)
			}
			if pages[f.DocID] == nil {
				pages[f.DocID] = map[int]bool{}
			}
			pages[f.DocID][f.Page] = true
		}
		var want []Source
		for _, doc := range sortedKeys(pages) {
			var ps []int
			for p := range pages[doc] {
				ps = append(ps, p)
			}
			sort.Ints(ps)
			want = append(want, Source{DocID: doc, Pages: ps})
		}
		if diff := cmp.Diff(want, l.Locators); diff != "" {
			t.Errorf("link %s -> %s locators mismatch (-want +got):\n%s",
				l.Source, l.Target, diff)
		}
	}
}

// TestTheSpineFixtureExercisesBothPages stops the test above passing over a
// graph where every link happens to cite one page. pp.66-67 put general and
// enterprise on 66 and the other four groups on 67, so the fixture really does
// span two pages -- and if it ever stops doing so, the locator assertion has
// become much weaker than it reads.
func TestTheSpineFixtureExercisesBothPages(t *testing.T) {
	g := buildGraph(t, spineFacts(t, testYear), testOptions())
	seen := map[int]int{}
	for _, l := range g.Links {
		for _, s := range l.Locators {
			for _, p := range s.Pages {
				seen[p]++
			}
		}
	}
	for _, p := range []int{66, 67} {
		if seen[p] == 0 {
			t.Errorf("no link cites page %d; spinePage puts general/enterprise on 66 "+
				"and the other four groups on 67, so both must appear", p)
		}
	}
	if len(seen) != 2 {
		t.Errorf("links cite pages %v, want exactly 66 and 67", seen)
	}
}
