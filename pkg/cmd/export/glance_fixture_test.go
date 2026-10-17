package export

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestTheGlanceFixtureIsWhatGoWrites holds testdata/glance.golden.html, which
// site/glance.test.mjs runs the client over, to the glance page Go writes
// over clientSubset, the build stamp aside. clientSubset keeps every fund
// group with a figure in a budget year whole, so the fixture's rows are every
// cell the site shows, and its wholes the spine's own figures.
func TestTheGlanceFixtureIsWhatGoWrites(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	served, err := os.ReadFile(filepath.Join(dir, "glance.html"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "testdata", "glance.golden.html"))
	if err == nil && bytes.Equal(packagedBy.ReplaceAll(served, []byte("Packaged by")),
		packagedBy.ReplaceAll(fixture, []byte("Packaged by"))) {
		return
	}
	out := filepath.Join(root, "bin", "client-fixtures")
	if mkErr := os.MkdirAll(out, 0o750); mkErr != nil {
		t.Fatal(mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(out, "glance.golden.html"), served, 0o600); wErr != nil {
		t.Fatal(wErr)
	}
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	t.Errorf("testdata/glance.golden.html is not the page Go writes (-go +fixture):\n%s\n"+
		"regenerate with\n  cp -f %s testdata/glance.golden.html\nand read the diff before committing it",
		cmp.Diff(strings.Split(string(packagedBy.ReplaceAll(served, nil)), "\n"),
			strings.Split(string(packagedBy.ReplaceAll(fixture, nil)), "\n")),
		filepath.Join(out, "glance.golden.html"))
}
