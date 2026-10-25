package registry_test

// This file pins registry.Load to the vocabulary behaviourally: a
// data/taxonomy.yaml kinds: member is accepted exactly when it is a vocab.Kind,
// and the refusal offers the whole set.

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

const (
	pinFunds = `
schema_version: 1
funds:
  - {number: 100, name: "General", type: general, constraint_tier: discretionary, restriction_note: "p258."}
`
	pinDepartments = `
schema_version: 1
departments:
  - {slug: city-manager, label: "City Manager", document_term: "CITY MANAGER", pages: [167]}
divisions:
  - {slug: city-clerk, label: "City Clerk", department: city-manager, pages: [167]}
`
	pinTaxonomyFmt = `
schema_version: 1
categories:
  - {slug: pinned, label: "Pinned", kinds: [%s]}
`
)

// loadWithKinds runs the real Load over a minimal registry whose one category
// declares the given kinds: members verbatim.
func loadWithKinds(t *testing.T, kinds string) error {
	t.Helper()
	fsys := fstest.MapFS{
		registry.FundsFile:       &fstest.MapFile{Data: []byte(pinFunds)},
		registry.TaxonomyFile:    &fstest.MapFile{Data: []byte(fmt.Sprintf(pinTaxonomyFmt, kinds))},
		registry.DepartmentsFile: &fstest.MapFile{Data: []byte(pinDepartments)},
	}
	_, err := registry.Load(fsys)
	return err
}

// TestLoadAcceptsEveryKindVocabDefines is one half of the pin: a vocab.Kind
// the registry would not accept makes a taxonomy that the rule parser would
// happily produce facts for unloadable.
func TestLoadAcceptsEveryKindVocabDefines(t *testing.T) {
	for _, k := range vocab.Kinds() {
		t.Run(string(k), func(t *testing.T) {
			if err := loadWithKinds(t, string(k)); err != nil {
				t.Errorf("Load with kinds: [%s] = %v, want nil; internal/vocab defines "+
					"that kind and internal/registry does not accept it", k, err)
			}
		})
	}
}

// TestLoadRefusesAKindVocabDoesNotDefine, and its message lists the
// alternatives. "transfer" is the value four transfer categories once carried
// undetected.
func TestLoadRefusesAKindVocabDoesNotDefine(t *testing.T) {
	for _, bad := range []string{"transfer", "banana", "Revenue", "transfers_in"} {
		t.Run(bad, func(t *testing.T) {
			err := loadWithKinds(t, bad)
			if err == nil {
				t.Fatalf("Load with kinds: [%s] = nil error, want a refusal", bad)
			}
			got := err.Error()
			if !strings.Contains(got, fmt.Sprintf("got %q", bad)) {
				t.Errorf("Load error = %q, want it to name the offending value %q", got, bad)
			}
			for _, k := range vocab.Kinds() {
				if !strings.Contains(got, string(k)) {
					t.Errorf("Load error = %q, want it to offer %q as an alternative", got, k)
				}
			}
		})
	}
}

// TestRegistryOffersNoKindVocabLacks: registry offers no kind vocab has never
// defined, or `kinds: [grant]` would load a category no fact can match. It
// reads the set back off the refusal message, which is the remedy and must not
// change silently.
func TestRegistryOffersNoKindVocabLacks(t *testing.T) {
	err := loadWithKinds(t, "banana")
	if err == nil {
		t.Fatal("Load with kinds: [banana] = nil error, want a refusal")
	}
	_, list, ok := strings.Cut(err.Error(), "want one of ")
	if !ok {
		t.Fatalf("Load error = %q, want it to contain \"want one of \" so the "+
			"offered set can be read back", err)
	}
	var offered []string
	for _, k := range strings.Split(list, ", ") {
		offered = append(offered, strings.TrimSpace(k))
	}
	var want []string
	for _, k := range vocab.Kinds() {
		want = append(want, string(k))
	}
	slices.Sort(offered)
	slices.Sort(want)
	if !slices.Equal(offered, want) {
		t.Errorf("registry offers kinds %v, want exactly vocab.Kinds() %v; a kind "+
			"registry accepts and vocab never defines loads as a category no fact "+
			"can match", offered, want)
	}
}

// TestTheRefusalIsNotJustAnEmptyCheck guards the way this could go quietly
// wrong: a membership arm that accepted every string would pass the test
// above on the five real kinds. An empty kinds: list must still be refused by
// its own separate arm.
func TestTheRefusalIsNotJustAnEmptyCheck(t *testing.T) {
	err := loadWithKinds(t, "")
	if err == nil {
		t.Fatal("Load with kinds: [] = nil error, want a refusal")
	}
	if want := "kinds: is required"; !strings.Contains(err.Error(), want) {
		t.Errorf("Load error = %q, want it to contain %q", err, want)
	}
}
