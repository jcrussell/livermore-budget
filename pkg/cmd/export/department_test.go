package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/schema"
)

const departmentConfigOpen, departmentConfigClose = `<script type="application/json" id="department-config">`, "</script>"

// splitDepartmentPage cuts department.html into its markup either side of its
// config, build stamps blanked, and the config itself.
func splitDepartmentPage(page []byte) (markup, config []byte, err error) {
	before, rest, ok := bytes.Cut(page, []byte(departmentConfigOpen))
	if !ok {
		return nil, nil, fmt.Errorf("carries no department config")
	}
	body, after, ok := bytes.Cut(rest, []byte(departmentConfigClose))
	if !ok {
		return nil, nil, fmt.Errorf("never closes the department config")
	}
	markup = slices.Concat(before, []byte(departmentConfigOpen+"…"+departmentConfigClose), after)
	return packagedBy.ReplaceAll(markup, []byte("Packaged by")), body, nil
}

// departmentFixtureColumns are the columns site/department.test.mjs reads
// beyond the index's: FY2023-24 actual, the one column printing a Transfers
// Out row in pp.85-125's upper block and the one where departments' two blocks
// are held apart.
var departmentFixtureColumns = []string{"fy2024-actual"}

// TestTheDepartmentFixtureHoldsWhatTheClientReads pins
// testdata/department.golden.html and testdata/fy2024-actual.column.json,
// which site/department.test.mjs boots over with the index's pinned columns,
// to what Go writes over the client subset: the same markup and the same
// decoded config, build stamps aside; a config the schema accepts whose years
// include every pinned column; and the column held as the index's are.
//
// On a failure the export's files are left where a cp -f takes them.
func TestTheDepartmentFixtureHoldsWhatTheClientReads(t *testing.T) {
	dir := clientExport(t)
	root := repoRootForTest(t)
	served, err := os.ReadFile(filepath.Join(dir, "department.html")) // #nosec G304 -- the temp site.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		out := filepath.Join(root, "bin", "client-fixtures")
		if mkErr := os.MkdirAll(out, 0o750); mkErr != nil {
			t.Error(mkErr)
			return
		}
		files := map[string]string{"department.html": "department.golden.html"}
		for _, stem := range departmentFixtureColumns {
			files[stem+".json"] = stem + ".column.json"
		}
		var copies []string
		for _, from := range slices.Sorted(maps.Keys(files)) {
			b, readErr := os.ReadFile(filepath.Join(dir, from)) // #nosec G304 -- the temp site.
			if readErr == nil {
				readErr = os.WriteFile(filepath.Join(out, files[from]), b, 0o600)
			}
			if readErr != nil {
				t.Error(readErr)
				return
			}
			copies = append(copies, "cp -f "+filepath.Join(out, files[from])+" testdata/"+files[from])
		}
		t.Logf("regenerate with\n  %s\nand read the diff before committing it", strings.Join(copies, "\n  "))
	})
	fixture, err := os.ReadFile(filepath.Join(root, "testdata", "department.golden.html")) // #nosec G304 -- testdata/.
	if err != nil {
		t.Fatal(err)
	}
	sMarkup, sBody, err := splitDepartmentPage(served)
	if err != nil {
		t.Fatal("the served page " + err.Error())
	}
	fMarkup, fBody, err := splitDepartmentPage(fixture)
	if err != nil {
		t.Fatal("the fixture " + err.Error())
	}
	if !bytes.Equal(sMarkup, fMarkup) {
		t.Errorf("the markup outside the config is not the template's:\n%s",
			cmp.Diff(strings.Split(string(sMarkup), "\n"), strings.Split(string(fMarkup), "\n")))
	}
	var sCfg, fCfg map[string]any
	if err := json.Unmarshal(sBody, &sCfg); err != nil {
		t.Fatalf("the served config does not decode: %v", err)
	}
	if err := json.Unmarshal(fBody, &fCfg); err != nil {
		t.Fatalf("the fixture's config does not decode: %v", err)
	}
	if err := schema.Validate(schema.Department, fCfg); err != nil {
		t.Errorf("the fixture's config does not match %s: %v", schema.Department, err)
	}
	delete(sCfg, "exported_by")
	delete(fCfg, "exported_by")
	if diff := cmp.Diff(sCfg, fCfg); diff != "" {
		t.Errorf("the fixture's config is not the one Go writes (-go +fixture):\n%s", diff)
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
	var stamp struct {
		ExportedBy string `json:"exported_by"`
	}
	if err := json.Unmarshal(fBody, &stamp); err != nil {
		t.Fatal(err)
	}
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
		for _, f := range columnFixtureFaults(stem, servedCol, fixtureCol, stamp.ExportedBy, facts) {
			t.Error(f)
		}
	}
}

// TestTheDepartmentPageDeclaresEveryDepartmentwideException: the cells the
// department page prints as held apart are exactly the cells
// structure.BudgetBookExceptions holds pp.85-125's upper block apart from its
// lower one at a department, one per pinned cell, each with its reason.
//
// Mutation: pass "general-fund-departments" for the cut in
// departmentsHeldApart and the page holds apart the General Fund schedule's
// two rounding cells instead of the departmentwide five.
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

// TestTheHeldReasonIsTrueOfWhatThePageDraws: the department page draws
// Maintenance's Transfers Out inside public works' spending sum, so the
// reason it prints for FY2023-24's held cell must say the row is counted,
// not that the spending side leaves it out.
//
// Mutation: restore the exception's reason to "the departmentwide cut leaves
// out Maintenance's Transfers Out, ..." and this goes red.
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
