package export

import (
	"bytes"
	"sync"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/synthcity"
)

// buildSynthCityFacts is synthcity.Default's facts in place of the committed
// store's for every scope the city prints, and the store's for the rest, so
// the site's other documents still build, with the exceptions the city
// declares. The scopes are read off the city's facts: a scope it prints that
// the store also keeps would otherwise mix Livermore's figures into the
// city's, under the same ids.
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
	scopes := map[string]bool{}
	for _, f := range facts {
		scopes[f.Scope] = true
	}
	for _, f := range store {
		if !scopes[f.Scope] {
			facts = append(facts, f)
		}
	}
	if err := fact.CheckUniqueIDs(facts); err != nil {
		return nil, nil, err
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
