package export

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// The committed store's builds and exports are the same bytes for every test
// in one process, and each costs seconds under -race, so each is made once.
// A build is handed out as a deep copy, since tests plant edits in it; an
// export directory is shared, and its callers only read it.
var (
	cacheDir string

	buildAllOnce = sync.OnceValues(func() (result, error) { return buildAll(repoRootOnce()) })
	projOnce     = sync.OnceValues(func() (map[string][]byte, error) {
		_, facts, err := readFactStore(repoRootOnce())
		if err != nil {
			return nil, err
		}
		return buildProjectionsFrom(repoRootOnce(), facts, structure.BudgetBookExceptions())
	})
	siteOnce     sync.Once
	siteDir      string
	siteErr      error
	repoRootOnce = sync.OnceValue(func() string { root, _ := filepath.Abs(filepath.Join("..", "..", "..")); return root })
)

// TestMain makes the directory the shared exports live in, which outlives
// any one test as t.TempDir would not, and removes it after the run.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fisc-export-test-")
	if err != nil {
		panic(err)
	}
	cacheDir = dir
	code := m.Run()
	_ = os.RemoveAll(cacheDir)
	os.Exit(code)
}

// cachedBuildAll is buildAll over the committed store, built once.
func cachedBuildAll(repoRoot string) (result, error) {
	if repoRoot != repoRootOnce() {
		return buildAll(repoRoot)
	}
	r, err := buildAllOnce()
	if err != nil {
		return result{}, err
	}
	r.Projections = cloneBytes(r.Projections)
	r.Files = cloneBytes(r.Files)
	r.PageIndex = slices.Clone(r.PageIndex)
	r.Downloads = slices.Clone(r.Downloads)
	r.RecordsBase = maps.Clone(r.RecordsBase)
	return r, nil
}

// buildProjections is the projection half of buildAll over the committed
// fact store, for tests that ask what the projections produce without an
// export around them.
func buildProjections(repoRoot string) (map[string][]byte, error) {
	if repoRoot != repoRootOnce() {
		_, facts, err := readFactStore(repoRoot)
		if err != nil {
			return nil, err
		}
		return buildProjectionsFrom(repoRoot, facts, structure.BudgetBookExceptions())
	}
	p, err := projOnce()
	return cloneBytes(p), err
}

func cloneBytes(m map[string][]byte) map[string][]byte {
	if m == nil {
		return nil
	}
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		out[k] = slices.Clone(v)
	}
	return out
}

// sharedExport runs exportRun once into a directory of its own under cacheDir.
func sharedExport(t *testing.T, once *sync.Once, dir *string, errp *error, name string, build func(string) (result, error)) string {
	t.Helper()
	once.Do(func() {
		opts, _, _, _ := testOptions(t)
		root := repoRootForTest(t)
		opts.RepoRoot = func() (string, error) { return root, nil }
		opts.Build = build
		opts.OutputDir = filepath.Join(cacheDir, name)
		*errp = exportRun(context.Background(), opts)
		*dir = opts.OutputDir
	})
	if *errp != nil {
		t.Fatalf("exportRun (%s): %v", name, *errp)
	}
	return *dir
}

// exportedSite is `fisc export` over the committed store, written once.
func exportedSite(t *testing.T) string {
	t.Helper()
	return sharedExport(t, &siteOnce, &siteDir, &siteErr, "site", cachedBuildAll)
}
