package registry

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestEveryFileUnderDataIsValidatedBySomething: every file under data/ is on a
// hand-maintained list of what validates it, so a file no loader reads fails
// here. It walks data/extracted/ and data/pdf/ too, since
// artifacts-match-manifest catches strays only inside a document's directory.
func TestEveryFileUnderDataIsValidatedBySomething(t *testing.T) {
	const hint = "add a loader for it and add it here, or it is committed data " +
		"nothing validates"

	t.Run("data/", func(t *testing.T) {
		files, dirs := readDir(t, realData)
		// Sorted, because readDir's result is: the constants are not
		// alphabetical by construction, and a fifth one -- which this test's
		// own doc comment invites -- would otherwise fail with an ordering
		// diff that reads as a data problem.
		wantFiles := []string{DepartmentsFile, FundsFile, SourcesFile, TaxonomyFile, "views.yaml"}
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
		// The whole path is checked, not the basename: LoadSources accepts
		// any fs.ValidPath, so `file: data/pdfs/x.pdf` loads clean.
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
