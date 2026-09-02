package registry

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestEveryFileUnderDataIsValidatedBySomething is the meta-property no other
// test has, and it is one level up from the ones that do.
//
// Each committed file under data/ is strictly validated by a loader:
// funds/taxonomy/departments.yaml by Load (and read off disk by
// TestLoadRealRegistries), sources.yaml by LoadSources (and by
// TestLoadSourcesReadsTheCommittedRegistry), and each extracted manifest by
// five structural checks that fisc verify --strict runs over the real tree.
// What NOTHING catches is a file appearing that no loader knows about.
//
// MEASURED, and it is not hypothetical: dropping data/reconciliations.yaml -- a
// filename check.Result's doc comment named as a future one when this was written,
// and which that comment now records will never exist, tier 2 having landed in
// internal/mapping at build time instead -- plus a malformed data/junk.yaml leaves
// fisc verify fully green, and go test ./... green too. Committed data that nothing
// reads and nothing validates is the same failure this package exists to refuse,
// one level up: an unvalidated thing agreeing with nothing at all, reported as a
// pass.
//
// IT WALKS TWO LEVELS, NOT ONE, because the boundary is not where it looks.
// artifacts-match-manifest hashes every file its manifest lists and reports every
// file it does not, so a stray under data/extracted/<doc>/ IS caught. A stray at
// data/extracted/ itself, or at data/pdf/, is NOT -- measured, both leave fisc
// verify fully green. So the two known directories get their own arm.
//
// This is a hand-maintained list that fails closed, the same trade
// tools/jscheck's TEMPLATE_IDS and KNOWN_SELECTORS make: adding data is meant
// to be a decision, and a decision leaves a diff here.
func TestEveryFileUnderDataIsValidatedBySomething(t *testing.T) {
	const hint = "add a loader for it and add it here, or it is committed data " +
		"nothing validates"

	t.Run("data/", func(t *testing.T) {
		files, dirs := readDir(t, realData)
		// Sorted, because readDir's result is: the constants are not
		// alphabetical by construction, and a fifth one -- which this test's
		// own doc comment invites -- would otherwise fail with an ordering
		// diff that reads as a data problem.
		wantFiles := []string{DepartmentsFile, FundsFile, SourcesFile, TaxonomyFile}
		sort.Strings(wantFiles)
		if diff := cmp.Diff(wantFiles, files); diff != "" {
			t.Errorf("files directly under data/ (-want +got):\n%s\n%s", diff, hint)
		}
		if diff := cmp.Diff([]string{"extracted", "pdf"}, dirs); diff != "" {
			t.Errorf("directories under data/ (-want +got):\n%s\n%s", diff, hint)
		}
	})

	// The two directories are checked against sources.yaml rather than against
	// a second hand-written list, so a fourth document is ONE edit and not
	// three -- and a document added to the registry with no PDF or no
	// extraction reddens here rather than at whatever reads it first.
	srcs, err := LoadSources(os.DirFS(realData))
	if err != nil {
		t.Fatalf("LoadSources(%s): %v", realData, err)
	}
	var wantPDFs, wantDirs []string
	for _, s := range srcs {
		// THE WHOLE PATH IS CHECKED, not just the basename. LoadSources only
		// requires fs.ValidPath, so `file: data/pdfs/x.pdf` loads clean; a
		// basename comparison would then pass the very test that exists to tie
		// data/pdf to the registry, and the only reader of the full path is a
		// --full check that CI runs monthly. This is the claim the directory
		// layout actually rests on.
		if want := "data/pdf/" + filepath.Base(s.File); s.File != want {
			t.Errorf("sources.yaml %s has file %q, want %q; every PDF lives in data/pdf",
				s.ID, s.File, want)
		}
		wantPDFs = append(wantPDFs, filepath.Base(s.File))
		wantDirs = append(wantDirs, s.ID)
	}
	sort.Strings(wantPDFs)
	sort.Strings(wantDirs)

	t.Run("data/pdf", func(t *testing.T) {
		files, dirs := readDir(t, filepath.Join(realData, "pdf"))
		if diff := cmp.Diff(wantPDFs, files); diff != "" {
			t.Errorf("files under data/pdf (-want +got):\n%s\nevery PDF is one "+
				"sources.yaml names, with its size and sha256", diff)
		}
		if len(dirs) != 0 {
			t.Errorf("data/pdf holds directories %v, want none", dirs)
		}
	})

	t.Run("data/extracted", func(t *testing.T) {
		files, dirs := readDir(t, filepath.Join(realData, "extracted"))
		if len(files) != 0 {
			t.Errorf("data/extracted holds loose files %v, want none; a file here is "+
				"under no manifest, so artifacts-match-manifest cannot see it", files)
		}
		if diff := cmp.Diff(wantDirs, dirs); diff != "" {
			t.Errorf("directories under data/extracted (-want +got):\n%s\neach is named "+
				"for a sources.yaml document id", diff)
		}
	})
}

// readDir returns the regular file names and directory names of one directory,
// each sorted.
func readDir(t *testing.T, dir string) (files, dirs []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		} else {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	sort.Strings(dirs)
	return files, dirs
}
