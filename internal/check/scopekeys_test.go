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

// TestNetCellAddressMirrorsTheProjection pins the claim netCellAddress's doc
// comment makes: that it is the key internal/project's netCells actually
// collides on.
//
// It is a MIRROR and not an import, because project.cellKey is unexported and
// because this check must keep measuring what netCells did even if netCells is
// re-keyed. So the two can drift, and this is what makes the drift loud: adding
// a field to cellKey without adding it here would leave ARM 2 validating
// disjointness declarations against a grain no projection uses, which is the
// fisc-tlbp defect arriving from the other direction.
//
// Comparing field NAMES read from the source rather than values, because the
// claim is structural and reflect cannot see an unexported type across a package
// boundary.
func TestNetCellAddressMirrorsTheProjection(t *testing.T) {
	want := fieldsOfStruct(t, "../project/sankey.go", "cellKey")
	got := fieldsOfStruct(t, "scopepairs.go", "netCellAddress")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("netCellAddress does not mirror project.cellKey (-cellKey +netCellAddress):\n%s\n"+
			"ARM 2 validates disjointness declarations against this grain; if netCells was "+
			"re-keyed, follow it here deliberately rather than leaving the two apart", diff)
	}
	if len(got) == 0 {
		t.Fatal("no fields compared, so this test could not fail")
	}
	// The finer address must be a strict superset: it is the key that answers
	// "could ANY projection collide these", so anything the coarse key carries
	// it must carry too.
	fine := fieldsOfStruct(t, "scopepairs.go", "cellAddress")
	set := map[string]bool{}
	for _, f := range fine {
		set[f] = true
	}
	for _, f := range got {
		if !set[f] {
			t.Errorf("netCellAddress carries %q and cellAddress does not, so the "+
				"finer key is not a refinement of the coarser one", f)
		}
	}
}

// TestADisjointnessDeclarationIsCheckedAtTheGrainProjectionsNetOn reproduces
// fisc-tlbp exactly: the declaration that was made, the measurement that
// justified it, and the collision it did not cover.
//
// The entry below is the one that really landed and was reverted. Its reason was
// measured with sharedKeys, which is true -- acfr-general-fund-summary and
// all-funds-gross share ZERO six-field addresses, because one is FY2025 audited
// and the other FY2026/FY2027 adopted, and fiscal_year and basis are in that key.
// Under the three-field key netCells collides on, they share 15. A declaration
// asserting a safety that does not hold is worse than the undeclared pair it
// replaced, because ARM 3 stops reporting it.
func TestADisjointnessDeclarationIsCheckedAtTheGrainProjectionsNetOn(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pair := pairOf(acfrGeneralFundScope, project.PublishedScope)

	// The committed store must be clean, or the assertion below proves nothing.
	clean, cleanErr := (&projectionScopesAreDisjoint{}).Run(t.Context(), s)
	if cleanErr != nil {
		t.Fatal(cleanErr)
	}
	if len(clean.Findings) > 0 {
		t.Fatalf("the committed corpus already has findings, so this test cannot "+
			"attribute one to the declaration: %v", clean.Findings)
	}
	if len(sharedKeys(s.Facts)[pair]) != 0 {
		t.Fatalf("%s shares a six-field address, so the old measurement no longer "+
			"holds and this test is not reproducing fisc-tlbp", pair)
	}
	if got := len(sharedNetKeys(s.Facts)[pair]); got == 0 {
		t.Fatalf("%s shares no netCells key, so there is nothing for the coarse "+
			"arm to catch and this test could not fail", pair)
	}

	disjointScopes[pair] = "they share no cell key"
	defer delete(disjointScopes, pair)

	res, err := (&projectionScopesAreDisjoint{}).Run(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("declaring the ACFR pair disjoint on a six-field measurement produced " +
			"no finding; ARM 2 is measuring only the finer key again")
	}
	var joined []string
	for _, f := range res.Findings {
		joined = append(joined, f.Detail)
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "at the grain a projection nets on") {
		t.Errorf("findings = %s\nwant the coarse-key arm to be the one that fired", all)
	}
	if !strings.Contains(all, "netCells") {
		t.Errorf("findings = %s\nwant the finding to name what collides", all)
	}
}
