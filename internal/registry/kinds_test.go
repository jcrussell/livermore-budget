package registry_test

// This file pins registry.Load to the vocabulary: a data/taxonomy.yaml
// kinds: member is accepted exactly when it is a vocab.Kind, and the refusal
// offers the whole set.
//
// THE PIN IS BEHAVIOURAL, NOT A SLICE COMPARISON. Load reads the set from
// internal/vocab, so there is no second list to compare; what can still go
// wrong is a filter in front of that set, a refusal that stops naming the
// alternatives, or an arm that accepts an empty list. Driving Load with each
// vocab.Kind pins all three, and fails in the terms a reader hits -- a file
// that will not load -- rather than in terms of two slices.

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

// TestLoadRefusesAKindVocabDoesNotDefine is the other half. It also pins the
// message, because a reader hitting this has a typo in a YAML file and the
// list of what they could have meant is the whole remedy.
//
// "transfer" is not an arbitrary bad string: it is the exact value all four
// transfer categories carried until 6216eda, and it was reported as a pass for
// as long as it was there (fisc-ttq).
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

// TestRegistryOffersNoKindVocabLacks closes the pin's other direction, and
// it is not symmetry for its own sake.
//
// The two tests above prove registry ACCEPTS every vocab.Kind. They say
// nothing about a value registry would offer that vocab has never defined:
// an extra "grant" accepted here leaves the whole suite green, and
// `kinds: [grant]` would then load as a category NO FACT CAN EVER MATCH --
// silently, because fact-kind-matches-category only consults a category some
// fact reached. That is the fisc-ttq shape from the other side, and it is
// exactly what this file exists to prevent.
//
// The refusal message is the only view of the list from out here, and it
// already has to enumerate the alternatives for the reader's sake, so this
// reads the set back off it. Coupling the test to that wording is deliberate:
// the message IS the remedy, and it should not be free to change silently.
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
