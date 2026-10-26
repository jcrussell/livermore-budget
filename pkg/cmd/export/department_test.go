package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// departmentFixtureColumns are the columns site/department.test.mjs reads
// beyond the index's: FY2023-24 actual, where pp.85-125 prints Transfers Out.
var departmentFixtureColumns = []string{"fy2024-actual"}

// TestTheDepartmentFixtureHoldsWhatTheClientReads pins
// testdata/client/department.config.json and testdata/fy2024-actual.column.json
// to what Go writes over the client subset, build stamps aside.
func TestTheDepartmentFixtureHoldsWhatTheClientReads(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	holdConfig(t, dir, "department.html", departmentConfig)
	files := map[string]string{}
	for _, stem := range departmentFixtureColumns {
		files[stem+".json"] = stem + ".column.json"
	}
	leaveCopies(t, dir, files)
	var fCfg map[string]any
	if err := json.Unmarshal(departmentConfig.read(t), &fCfg); err != nil {
		t.Fatalf("the fixture's config does not decode: %v", err)
	}
	var paths []string
	for _, y := range fCfg["years"].([]any) {
		paths = append(paths, y.(map[string]any)["path"].(string))
	}
	for _, stem := range append(slices.Clone(clientFixtureColumns), departmentFixtureColumns...) {
		if !slices.Contains(paths, stem+".json") {
			t.Errorf("the department fixture's years %v do not open the pinned column %s", paths, stem)
		}
	}
	stamp, _ := fCfg["exported_by"].(string)
	facts := storeByID(t)
	for _, stem := range departmentFixtureColumns {
		servedCol, err := os.ReadFile(filepath.Join(dir, stem+".json")) // #nosec G304 -- the temp site.
		if err != nil {
			t.Fatal(err)
		}
		fixtureCol, err := os.ReadFile(filepath.Join(root, "testdata", stem+".column.json")) // #nosec G304 -- testdata/.
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		for _, f := range columnFixtureFaults(stem, servedCol, fixtureCol, stamp, facts) {
			t.Error(f)
		}
	}
}

// TestTheDepartmentPageDeclaresEveryDepartmentwideException: the cells the
// department page holds apart are exactly structure.BudgetBookExceptions' for
// pp.85-125 at a department, each with its reason.
// Mutation: pass "general-fund-departments" in departmentsHeldApart and the
// page holds apart the General Fund's two rounding cells instead.
func TestTheDepartmentPageDeclaresEveryDepartmentwideException(t *testing.T) {
	var want []string
	for _, e := range structure.BudgetBookExceptions() {
		if e.Cut != structure.CutDepartmentwide {
			continue
		}
		for _, c := range e.Cells {
			// A cell of the departmentwide cut held apart at a coarser grain
			// names no department and is no department's.
			if c.Coords[structure.AxisDepartment] == "" {
				continue
			}
			want = append(want, fmt.Sprintf("department/%s@%d-%s", c.Coords[structure.AxisDepartment], c.Year, c.Basis))
		}
	}
	var got []string
	for _, h := range departmentsHeldApart() {
		if h.Reason == "" {
			t.Errorf("%s in FY%d %s is held apart with no reason", h.Department, h.FiscalYear, h.Basis)
		}
		got = append(got, fmt.Sprintf("%s@%d-%s", h.Department, h.FiscalYear, h.Basis))
	}
	slices.Sort(want)
	slices.Sort(got)
	if len(want) == 0 {
		t.Fatal("no exception holds the departmentwide cut apart, so this test asserts nothing")
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("held-apart cells (-exceptions +page):\n%s", diff)
	}
}

// TestTheHeldReasonIsTrueOfWhatThePageDraws: the held FY2023-24 cell's reason
// must say Maintenance's Transfers Out is counted in public works' spending.
// Mutation: restore the reason to "the departmentwide cut leaves out
// Maintenance's Transfers Out, ..." and this goes red.
func TestTheHeldReasonIsTrueOfWhatThePageDraws(t *testing.T) {
	var found bool
	for _, h := range departmentsHeldApart() {
		if h.Department != "department/public-works" || h.FiscalYear != 2024 {
			continue
		}
		found = true
		if strings.Contains(h.Reason, "leaves out") {
			t.Errorf("public works FY2024 is held apart because the spending side %q, and the page draws "+
				"Transfers Out inside that side's sum", h.Reason)
		}
		if !strings.Contains(h.Reason, "counted with it, as the department page counts it") {
			t.Errorf("public works FY2024's reason %q does not say the page counts Transfers Out", h.Reason)
		}
	}
	if !found {
		t.Fatal("public works FY2024 is not held apart, so this test asserts nothing")
	}
}
