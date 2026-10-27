package export

import (
	"bytes"
	"slices"
	"sync"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/synthcity"
)

// cityScopes are the schedules synthcity prints; every other scope of the
// committed store is kept, so the site's other documents still build.
func cityScopes() []string {
	return append(project.ExpansionScopes(), project.DepartmentFundingScopes()...)
}

// buildSynthCityFacts is synthcity.Default's facts in place of the committed
// store's for the schedules a column draws, and the store's for the rest,
// with the exceptions the city declares.
func buildSynthCityFacts(repoRoot string) ([]fact.Fact, []structure.Exception, error) {
	reg, err := loadRegistry(repoRoot)
	if err != nil {
		return nil, nil, err
	}
	city, err := synthcity.Default().Facts(reg)
	if err != nil {
		return nil, nil, err
	}
	facts, exceptions := city.Facts, city.Exceptions
	_, store, err := readFactStore(repoRoot)
	if err != nil {
		return nil, nil, err
	}
	scopes := cityScopes()
	for _, f := range store {
		if !slices.Contains(scopes, f.Scope) {
			facts = append(facts, f)
		}
	}
	fact.Sort(facts)
	return facts, exceptions, nil
}

// buildSynthCity is buildAll over buildSynthCityFacts, under the city's own
// exceptions.
func buildSynthCity(repoRoot string) (result, error) {
	facts, exceptions, err := buildSynthCityFacts(repoRoot)
	if err != nil {
		return result{}, err
	}
	var raw bytes.Buffer
	if err := fact.Write(&raw, facts); err != nil {
		return result{}, err
	}
	return buildFrom(repoRoot, raw.Bytes(), facts, exceptions)
}

var (
	cityOnce sync.Once
	cityDir  string
	cityErr  error
)

// cityExport is `fisc export` over buildSynthCity, written once.
func cityExport(t *testing.T) string {
	t.Helper()
	return sharedExport(t, &cityOnce, &cityDir, &cityErr, "city", buildSynthCity)
}

// cityByID is the store buildSynthCity exports, keyed by fact id.
func cityByID(t *testing.T) map[string]fact.Fact {
	t.Helper()
	built, _, err := buildSynthCityFacts(repoRootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]fact.Fact, len(built))
	for _, f := range built {
		out[f.ID] = f
	}
	return out
}
