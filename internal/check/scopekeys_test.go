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

// TestTheFineAddressIsTheProjectionsKeyPlusWhatSelectionFixes pins what the
// two addresses claim to be.
//
// netCells has one caller -- sankey -- which hands it selectFacts' output and
// refuses more than one column, so every fact reaching it already agrees on
// fiscal year and basis. The grain a projection can realize is therefore
// netCells' key plus those two, and since the key carries the fund that is
// cellAddress, the FINE one. mergeableAddress is that key without the fund:
// coarser than any projection now nets on, and kept as the stricter arm
// because two schedules restating money at a fund-less grain are still two
// schedules restating money.
//
// Comparing names read from the source, because project.cellKey is unexported
// and reflect cannot see it across a package boundary.
func TestTheFineAddressIsTheProjectionsKeyPlusWhatSelectionFixes(t *testing.T) {
	netKey := fieldsOfStruct(t, "../project/sankey.go", "cellKey")
	fineKey := fieldsOfStruct(t, "scopepairs.go", "cellAddress")
	got := fieldsOfStruct(t, "scopepairs.go", "mergeableAddress")

	want := append(append([]string{}, netKey...), "fiscalyear", "basis")
	if diff := cmp.Diff(want, fineKey); diff != "" {
		t.Errorf("cellAddress is not project.cellKey plus (fiscal_year, basis) "+
			"(-want +got):\n%s\nIf netCells was re-keyed, or sankey stopped fixing the "+
			"column, follow it here deliberately.", diff)
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
// worked example -- the spine carries no fund where the detail carries fund
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
	// Takes the *testing.T it is called with: calling t.Fatal on the OUTER t
	// from inside a subtest does not stop the subtest, so the cleanup below it
	// would be skipped while execution carried on.
	run := func(t *testing.T) []Finding {
		t.Helper()
		res, err := (&projectionScopesAreDisjoint{}).Run(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		return res.Findings
	}
	if got := run(t); len(got) > 0 {
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
		for _, f := range run(t) {
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
			// defer and not a bare delete: run() can abort this subtest, and a
			// bare delete on the line after would leave a package-global
			// mutated for every test that follows.
			got := func() []Finding {
				defer delete(disjointScopes, p)
				return run(t)
			}()
			if len(got) > 0 {
				t.Errorf("declaring %s disjoint was refused, and the declaration is TRUE: %v", p, got)
			}
		}
	})
}

// TestTheMergeableMeasurementSkipsFactsNetCellsRefuses pins the filter in
// sharedMergeableKeys, which shipped without one.
//
// netCells errors rather than merging on a fact carrying a department, or
// missing a category or a fund group, so such a fact cannot take part in a
// collision -- it stops the build instead. Counting it overstates the hazard and
// reinstates the false-refusal class: every expenditure-by-department fact
// carries a department, so without the filter that scope reports collisions it
// cannot have, and a true disjointness declaration for it would be refused.
//
// Deleting the filter leaves go test and fisc verify green, which is why the
// claim needs a test rather than a run.
func TestTheMergeableMeasurementSkipsFactsNetCellsRefuses(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// expenditure-by-department and NOT departmentwide-expenditures: both carry
	// departments, but only this pair's keys overlap the spine's at all, so it
	// is the only one whose measurement the filter changes -- 8 shared keys
	// unfiltered against 0 filtered. Picking the other scope gives a test that
	// passes with the filter deleted, which is what the first draft of this did.
	pair := pairOf(project.PublishedScope, expenditureDetailScope)

	// The scope must really be department-bearing, or this proves nothing.
	departmentBearing := 0
	for i := range s.Facts {
		if s.Facts[i].Scope == expenditureDetailScope && s.Facts[i].Department != "" {
			departmentBearing++
		}
	}
	if departmentBearing == 0 {
		t.Fatalf("no fact in scope %q carries a department, so the filter has "+
			"nothing to exclude and this test could not fail", expenditureDetailScope)
	}

	if got := len(sharedMergeableKeys(s.Facts)[pair]); got != 0 {
		t.Errorf("%s shares %d mergeable key(s); every fact of that scope carries a "+
			"department, and netCells refuses those rather than merging them, so "+
			"the pair cannot collide at any grain", pair, got)
	}

	// The FINE arm applies it too. That is latent on the committed store -- no
	// pair's fine count moves either way -- so it is asserted as the shared
	// property rather than as a number, and canMerge is what both call.
	for i := range s.Facts {
		if f := &s.Facts[i]; f.Department != "" && canMerge(f) {
			t.Errorf("canMerge admits %s, which carries department %q; "+
				"project.netCells errors on it rather than merging", f.ID, f.Department)
			break
		}
	}
	if len(sharedKeys(s.Facts)[pair]) != 0 {
		t.Errorf("%s shares a fine address built from department-bearing facts, "+
			"so the two arms disagree about what can collide", pair)
	}

	// And the filter must not be a blanket one: a pair that CAN collide still does.
	live := pairOf(project.PublishedScope, revenueDetailScope)
	if got := len(sharedMergeableKeys(s.Facts)[live]); got == 0 {
		t.Errorf("%s shares no mergeable key, so the filter is excluding facts "+
			"netCells would accept", live)
	}
}
