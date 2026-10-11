package export

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestTheCapitalFixtureIsWhatGoWrites holds testdata/capital.golden.html,
// which site/capital.test.mjs runs the client over, to the capital page Go
// writes over clientSubset, the build stamp aside. clientSubset keeps
// cip-project-listing whole, so the fixture is every project the site shows.
func TestTheCapitalFixtureIsWhatGoWrites(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	served, err := os.ReadFile(filepath.Join(dir, "capital.html"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "testdata", "capital.golden.html"))
	if err == nil && bytes.Equal(packagedBy.ReplaceAll(served, []byte("Packaged by")),
		packagedBy.ReplaceAll(fixture, []byte("Packaged by"))) {
		return
	}
	out := filepath.Join(root, "bin", "client-fixtures")
	if mkErr := os.MkdirAll(out, 0o750); mkErr != nil {
		t.Fatal(mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(out, "capital.golden.html"), served, 0o600); wErr != nil {
		t.Fatal(wErr)
	}
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	t.Errorf("testdata/capital.golden.html is not the page Go writes (-go +fixture):\n%s\n"+
		"regenerate with\n  cp -f %s testdata/capital.golden.html\nand read the diff before committing it",
		cmp.Diff(strings.Split(string(packagedBy.ReplaceAll(served, nil)), "\n"),
			strings.Split(string(packagedBy.ReplaceAll(fixture, nil)), "\n")),
		filepath.Join(out, "capital.golden.html"))
}
