package registry_test

// This file is package registry_test, not package registry, and that is the
// whole point of it: it is the only place in the tree that may import BOTH
// internal/registry and internal/mapping.
//
// internal/registry cannot import internal/mapping. Every test file in that
// package is `package mapping` and transfers_p76_test.go imports registry, so
// the edge closes a cycle in the TEST build -- `go build ./...` passes and
// `go vet ./...` fails with "import cycle not allowed in test", which is the
// worst way to find out. So registry re-spells the five kinds, and this file
// is the pin that keeps the copy honest.
//
// THE PIN IS BEHAVIOURAL, NOT A SLICE COMPARISON. registry.factKinds is
// unexported and invisible from here, and exporting it purely to be compared
// would widen the API for a test. Driving Load with each mapping.Kind is
// strictly better anyway: it pins the error message too, and it fails in the
// terms a reader hits -- a file that will not load -- rather than in terms of
// two slices.

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/registry"
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

// TestLoadAcceptsEveryKindMappingDefines is one half of the pin: adding a
// sixth mapping.Kind without adding it to registry.factKinds makes a taxonomy
// that internal/mapping would happily produce facts for unloadable.
func TestLoadAcceptsEveryKindMappingDefines(t *testing.T) {
	for _, k := range mapping.Kinds() {
		t.Run(string(k), func(t *testing.T) {
			if err := loadWithKinds(t, string(k)); err != nil {
				t.Errorf("Load with kinds: [%s] = %v, want nil; internal/mapping defines "+
					"that kind and internal/registry does not accept it", k, err)
			}
		})
	}
}

// TestLoadRefusesAKindMappingDoesNotDefine is the other half. It also pins the
// message, because a reader hitting this has a typo in a YAML file and the
// list of what they could have meant is the whole remedy.
//
// "transfer" is not an arbitrary bad string: it is the exact value all four
// transfer categories carried until 6216eda, and it was reported as a pass for
// as long as it was there (fisc-ttq).
func TestLoadRefusesAKindMappingDoesNotDefine(t *testing.T) {
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
			for _, k := range mapping.Kinds() {
				if !strings.Contains(got, string(k)) {
					t.Errorf("Load error = %q, want it to offer %q as an alternative", got, k)
				}
			}
		})
	}
}

// TestTheRefusalIsNotJustAnEmptyCheck guards the way this could go quietly
// wrong: a factKinds that had drifted to hold every string would accept
// everything, and the test above would still pass on the five real kinds. An
// empty kinds: list must still be refused by its own separate arm.
func TestTheRefusalIsNotJustAnEmptyCheck(t *testing.T) {
	err := loadWithKinds(t, "")
	if err == nil {
		t.Fatal("Load with kinds: [] = nil error, want a refusal")
	}
	if want := "kinds: is required"; !strings.Contains(err.Error(), want) {
		t.Errorf("Load error = %q, want it to contain %q", err, want)
	}
}
