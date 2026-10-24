package export

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestTheSidesFixturesAreWhatGoWrites holds testdata/ledger.golden.html,
// which site/sides.test.mjs runs the form module over, and
// testdata/caveats.golden.html, which site/theme.test.mjs opens fragments
// on, to the pages Go writes over clientSubset, the build stamp aside. The
// ledger carries no figure: it fetches the pinned columns, so its fixture
// pins the config and the markup the client fills.
func TestTheSidesFixturesAreWhatGoWrites(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	for _, name := range []string{"ledger", "caveats"} {
		served, err := os.ReadFile(filepath.Join(dir, name+".html"))
		if err != nil {
			t.Fatal(err)
		}
		fixture, err := os.ReadFile(filepath.Join(root, "testdata", name+".golden.html"))
		if err == nil && bytes.Equal(packagedBy.ReplaceAll(served, []byte("Packaged by")),
			packagedBy.ReplaceAll(fixture, []byte("Packaged by"))) {
			continue
		}
		out := filepath.Join(root, "bin", "client-fixtures")
		if mkErr := os.MkdirAll(out, 0o750); mkErr != nil {
			t.Fatal(mkErr)
		}
		if wErr := os.WriteFile(filepath.Join(out, name+".golden.html"), served, 0o600); wErr != nil {
			t.Fatal(wErr)
		}
		if err != nil {
			t.Errorf("read the %s fixture: %v", name, err)
			continue
		}
		t.Errorf("testdata/%s.golden.html is not the page Go writes (-go +fixture):\n%s\n"+
			"regenerate with\n  cp -f %s testdata/%s.golden.html\nand read the diff before committing it", name,
			cmp.Diff(strings.Split(string(packagedBy.ReplaceAll(served, nil)), "\n"),
				strings.Split(string(packagedBy.ReplaceAll(fixture, nil)), "\n")),
			filepath.Join(out, name+".golden.html"), name)
	}
}
