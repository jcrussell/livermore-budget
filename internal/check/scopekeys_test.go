package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// fieldsOfStruct returns the field names of a named struct type declared in a
// Go source file, in declaration order.
func fieldsOfStruct(t *testing.T, path, name string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var got []string
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != name {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		found = true
		for _, fld := range st.Fields.List {
			for _, id := range fld.Names {
				got = append(got, strings.ToLower(id.Name))
			}
		}
		return false
	})
	if !found {
		t.Fatalf("%s declares no struct type %q", path, name)
	}
	return got
}

// TestMergeableAddressIsTheProjectionsKeyPlusWhatSelectionFixes pins what
// mergeableAddress claims to be.
//
// netCells keys on three fields, but it has one caller -- sankey -- which hands
// it selectFacts' output and refuses more than one column, so every fact
// reaching it already agrees on fiscal year and basis. The grain a projection
// can realize is therefore netCells' key plus those two, and this asserts that
// relationship rather than a field list nobody would notice going stale.
//
// Comparing names read from the source, because project.cellKey is unexported
// and reflect cannot see it across a package boundary.
func TestMergeableAddressIsTheProjectionsKeyPlusWhatSelectionFixes(t *testing.T) {
	netKey := fieldsOfStruct(t, "../project/sankey.go", "cellKey")
	got := fieldsOfStruct(t, "scopepairs.go", "mergeableAddress")

	want := append(append([]string{}, netKey...), "fiscalyear", "basis")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mergeableAddress is not project.cellKey plus (fiscal_year, basis) "+
			"(-want +got):\n%s\nARM 2 validates disjointness declarations at this "+
			"grain. If netCells was re-keyed, or sankey stopped fixing the column, "+
			"follow it here deliberately.", diff)
	}
	if len(netKey) == 0 {
		t.Fatal("no fields read from project.cellKey, so this test could not fail")
	}

	// The fine address must be a strict superset: it is the key that answers
	// "could ANY document collide these", so it must carry everything the
	// reachable grain does, and exactly one field more.
	fine := fieldsOfStruct(t, "scopepairs.go", "cellAddress")
	set := map[string]bool{}
	for _, f := range fine {
		set[f] = true
	}
	for _, f := range got {
		if !set[f] {
			t.Errorf("mergeableAddress carries %q and cellAddress does not, so the "+
				"finer key is not a refinement of the coarser one", f)
		}
	}
	if len(fine) != len(got)+1 {
		t.Errorf("cellAddress has %d fields and mergeableAddress %d; the difference "+
			"should be exactly the fund, which is what a projection merges across",
			len(fine), len(got))
	}
}

// TestADisjointnessDeclarationIsCheckedAtTheGrainProjectionsCanReach is the
// guard fisc-tlbp asked for, and both halves matter.
//
// REFUSED: a pair that shares no cellAddress and does share a mergeableAddress.
// all-funds-gross and revenue-by-fund are that case and are the check's own
// worked example -- the spine carries fund 0 where the detail carries fund
// numbers, so they share zero six-field addresses and 74 five-field ones while
// being the same $299,969,007.
//
// ACCEPTED: the ACFR pairs. They share nothing at either grain, because that
// scope is FY2025 audited and everything else is FY2026-27 adopted, and
// selectFacts admits one (fiscal_year, basis) per projection. An earlier version
// of this check keyed on netCells' three fields alone and refused a TRUE
// declaration here, which would have blocked E10's ACFR-beside-the-spine
// document except by writing a false reason. That is why the acceptance is
// asserted and not assumed.
func TestADisjointnessDeclarationIsCheckedAtTheGrainProjectionsCanReach(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	run := func() []Finding {
		t.Helper()
		res, err := (&projectionScopesAreDisjoint{}).Run(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		return res.Findings
	}
	if got := run(); len(got) > 0 {
		t.Fatalf("the committed corpus already has findings, so this test cannot "+
			"attribute one to a declaration: %v", got)
	}

	fine, mergeable := sharedKeys(s.Facts), sharedMergeableKeys(s.Facts)

	t.Run("refused when the pair merges at the reachable grain", func(t *testing.T) {
		p := pairOf(project.PublishedScope, revenueDetailScope)
		if len(fine[p]) != 0 {
			t.Fatalf("%s shares a six-field address, so this no longer isolates the "+
				"coarse arm", p)
		}
		if len(mergeable[p]) == 0 {
			t.Fatal("the spine and the revenue detail no longer merge at the reachable " +
				"grain, so there is nothing for the coarse arm to catch")
		}
		disjointScopes[p] = "they share no cell key"
		defer delete(disjointScopes, p)

		var joined []string
		for _, f := range run() {
			joined = append(joined, f.Detail)
		}
		all := strings.Join(joined, "\n")
		if all == "" {
			t.Fatal("declaring the spine and the revenue detail disjoint produced no " +
				"finding; ARM 2 is measuring only the finer key again")
		}
		if !strings.Contains(all, "at the grain a projection can reach") {
			t.Errorf("findings = %s\nwant the coarse-key arm to be the one that fired", all)
		}
	})

	t.Run("accepted when the pair cannot meet in any column", func(t *testing.T) {
		for _, other := range []string{project.PublishedScope, revenueDetailScope, transfersDetailScope} {
			p := pairOf(acfrGeneralFundScope, other)
			if len(fine[p])+len(mergeable[p]) != 0 {
				t.Errorf("%s shares %d fine and %d mergeable key(s); the ACFR is FY2025 "+
					"audited and cannot meet an adopted column", p, len(fine[p]), len(mergeable[p]))
				continue
			}
			disjointScopes[p] = "FY2025 audited against FY2026-27 adopted: selectFacts " +
				"admits one (fiscal_year, basis) per projection, so no column holds both"
			got := run()
			delete(disjointScopes, p)
			if len(got) > 0 {
				t.Errorf("declaring %s disjoint was refused, and the declaration is TRUE: %v", p, got)
			}
		}
	})
}
