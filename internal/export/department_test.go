package export_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// deptDocs are a department-funding and a department-spending document of
// FY2026-27 adopted: two departments, police with two divisions and council
// with one, each paid by the General Fund and police by an internal service
// fund too, and Horizons spending a Transfers Out row beside its wages.
func deptDocs() map[string][]byte {
	meta := func(scope string, pages string) string {
		return fmt.Sprintf(`"metadata":{"generated_by":"fisc (test)","fiscal_year":2027,`+
			`"fiscal_year_label":"FY 2026-27","basis":"adopted","scopes":[%q],"currency":"USD","units":"cents",`+
			`"sources":[{"doc_id":"livermore-budget-fy2026-2027","pages":[%s]}],"counts":{"facts":1,"nodes":1,"links":1},`+
			`"caveats":[{"id":"a-%s-caveat","summary":"A summary.","text":"A text.","applies_to":[]}]}`, scope, pages, scope)
	}
	link := func(from, to string, cents int64, kind string, page int) string {
		return fmt.Sprintf(`{"source":%q,"target":%q,"value_cents":%d,"kind":%q,"fact_ids":["fisc-f-%s"],`+
			`"locators":[{"doc_id":"livermore-budget-fy2026-2027","pages":[%d]}]}`,
			from, to, cents, kind, fmt.Sprintf("%x", sha256.Sum256([]byte(from+to)))[:12], page)
	}
	funding := `{"schema_version":1,"projection":"department-funding","nodes":[` +
		`{"id":"fund-group/general","label":"General Fund","tier":2,"role":"fund_group"},` +
		`{"id":"fund-group/internal-service","label":"Internal Service Funds","tier":2,"role":"fund_group"},` +
		`{"id":"fund/100","label":"General Fund","tier":3,"role":"general_fund","parent":"fund-group/general"},` +
		`{"id":"fund/730","label":"Fleet","tier":3,"role":"fund","parent":"fund-group/internal-service"},` +
		`{"id":"department/police-department","label":"Police Department","tier":4,"role":"whole_department"},` +
		`{"id":"department/city-council","label":"City Council","tier":4,"role":"whole_department"}],` +
		`"links":[` + link("fund/100", "department/police-department", 500_00, "external", 119) + `,` +
		link("fund/730", "department/police-department", 30_00, "internal_service", 120) + `,` +
		link("fund/100", "department/city-council", 20_00, "external", 85) + `],` +
		meta("department-funding-sources", "85, 119, 120") + `}`
	spending := `{"schema_version":1,"projection":"department-spending","nodes":[` +
		`{"id":"expenditure/wages-and-benefits","label":"Wages & Benefits","tier":5,"role":"object_category"},` +
		`{"id":"transfers/out","label":"Transfers Out","tier":5,"role":"transfer_out"},` +
		`{"id":"dept/patrol","label":"Patrol","tier":4,"role":"department"},` +
		`{"id":"dept/horizons","label":"Horizons","tier":4,"role":"department"},` +
		`{"id":"dept/city-council","label":"City Council","tier":4,"role":"department"}],` +
		`"links":[` + link("expenditure/wages-and-benefits", "dept/patrol", 400_00, "external", 119) + `,` +
		link("expenditure/wages-and-benefits", "dept/horizons", 130_00, "external", 119) + `,` +
		link("transfers/out", "dept/horizons", 10_00, "internal_transfer", 119) + `,` +
		link("expenditure/wages-and-benefits", "dept/city-council", 20_00, "external", 85) + `],` +
		meta("departmentwide-expenditures", "85, 119") + `}`
	return map[string][]byte{"department-funding-2027": []byte(funding), "department-spending-2027": []byte(spending)}
}

// deptMembership is deptDocs' membership.
func deptMembership() []export.Department {
	return []export.Department{
		{ID: "department/city-council", Label: "City Council",
			Divisions: []export.Division{{ID: "dept/city-council", Label: "City Council"}}},
		{ID: "department/police-department", Label: "Police Department",
			Divisions: []export.Division{{ID: "dept/patrol", Label: "Patrol"}, {ID: "dept/horizons", Label: "Horizons"}}},
	}
}

// deptOptions is a site with the spine and the department page over deptDocs.
func deptOptions(t *testing.T, dir string) export.Options {
	t.Helper()
	projections := map[string][]byte{export.PrimaryProjection: goldenSankey(t)}
	for k, v := range deptDocs() {
		projections[k] = v
	}
	return export.Options{
		Dir:         dir,
		Projections: projections,
		Views: append(spineOnly(), export.View{Path: "department.html", Nav: "Department",
			Template: export.DepartmentTemplate, Projection: "department-funding-2027",
			YearStems: []string{"department-funding-2027"}, Reads: []string{project.DepartmentSpendingProjection},
			Title: "A department title", Lede: "A department lede."}),
		Docs:        budgetDocs(),
		Departments: deptMembership(),
		DepartmentsHeldApart: []export.HeldApart{{Department: "department/police-department",
			FiscalYear: 2027, Basis: "adopted", Reason: "a declared reason"}},
	}
}

// departmentConfigOf cuts the department config out of a served page.
func departmentConfigOf(t *testing.T, html string) map[string]any {
	t.Helper()
	_, rest, ok := strings.Cut(html, `<script type="application/json" id="department-config">`)
	if !ok {
		t.Fatal("department.html carries no department config")
	}
	body, _, _ := strings.Cut(rest, "</script>")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("decode the department config: %v", err)
	}
	return cfg
}

// TestDepartmentPageCarriesItsMembershipAndItsTies holds the department view
// to the chrome it renders, the module it boots, the sentences it says in its
// body, and a config stating every department with its divisions and, per
// year, whether each ties or the declared reason it does not.
//
// Mutation: write a held-apart cell's side as tying in buildDepartmentPage
// and police reads as tying in a column its checks declare it does not.
func TestDepartmentPageCarriesItsMembershipAndItsTies(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(deptOptions(t, dir)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "department.html")
	for _, want := range []string{
		"<h1>A department title</h1>",
		"A department lede.",
		`<script type="module">import { boot } from "./department.js"; boot();</script>`,
		`<span aria-current="page">Department</span>`,
		`<a href="index.html">`,
		`id="sources-view"`,
		`<option value="department/police-department">Police Department</option>`,
		`<option value="fy2027-adopted.json" selected>FY 2026-27 adopted</option>`,
		// The two sentences a reader is owed in the page body.
		"The General Fund row is the General Fund's share alone",
		"A fund marked <em>internal service</em> is one the city fills by charging its own\n    departments",
		"<strong>Each figure is one fund's share</strong>",
		"<strong>Each figure is the whole department's\n    cost</strong>",
		"so neither sum is presented as the department's total",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("department.html does not carry %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "department.js")); err != nil {
		t.Errorf("department.html loads ./department.js and the site does not carry it: %v", err)
	}
	cfg := departmentConfigOf(t, html)
	if err := schema.Validate(schema.Department, cfg); err != nil {
		t.Errorf("the served config does not match %s: %v", schema.Department, err)
	}
	type side struct {
		Department string `json:"department"`
		Ties       bool   `json:"ties"`
		Reason     string `json:"reason"`
	}
	var got struct {
		Years []struct {
			Path  string `json:"path"`
			Sides []side `json:"sides"`
		} `json:"years"`
		Departments []struct {
			ID        string `json:"id"`
			Divisions []struct {
				ID string `json:"id"`
			} `json:"divisions"`
		} `json:"departments"`
		Docs map[string]struct {
			Pages map[string]any `json:"pages"`
		} `json:"docs"`
	}
	b, _ := json.Marshal(cfg)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Years) != 1 || got.Years[0].Path != "fy2027-adopted.json" {
		t.Fatalf("years = %+v, want fy2027-adopted.json alone", got.Years)
	}
	if diff := cmp.Diff([]side{
		{Department: "department/city-council", Ties: true},
		{Department: "department/police-department", Reason: "a declared reason"},
	}, got.Years[0].Sides); diff != "" {
		t.Errorf("sides (-want +got):\n%s", diff)
	}
	if len(got.Departments) != 2 || len(got.Departments[1].Divisions) != 2 {
		t.Errorf("departments = %+v, want council's one division and police's two", got.Departments)
	}
	for _, p := range []string{"85", "119", "120"} {
		if _, ok := got.Docs["livermore-budget-fy2026-2027"].Pages[p]; !ok {
			t.Errorf("the config carries no links for p%s, which a link cites", p)
		}
	}
}

// TestDepartmentPageRefusesASideLessThanTheDepartment: every way a side of a
// department could be summed over less than the department, or a page offer a
// department or a declaration the documents do not bear out, refuses the
// export by name.
//
// Mutation: drop the arm requiring every filed division to draw, and "a
// division the spending side draws nothing for" is refused only by the label
// arm after it, which reads the undrawn division's label as empty.
// Drop the arm holding a spending link's kind to its object end's role, and
// the four kind cases export.
func TestDepartmentPageRefusesASideLessThanTheDepartment(t *testing.T) {
	police := func(m []export.Department) *export.Department { return &m[1] }
	for _, tt := range []struct {
		name string
		edit func(o *export.Options)
		want string
	}{
		{"a drawn division no department files", func(o *export.Options) {
			p := police(o.Departments)
			p.Divisions = p.Divisions[:1]
		}, "dept/horizons, which no department's membership files"},
		{"a filed division the spending side draws nothing for", func(o *export.Options) {
			p := police(o.Departments)
			p.Divisions = append(p.Divisions, export.Division{ID: "dept/special-operations", Label: "Special Operations"})
		}, "draws nothing for dept/special-operations, a division of department/police-department"},
		{"a division filed under two departments", func(o *export.Options) {
			o.Departments[0].Divisions = append(o.Departments[0].Divisions, export.Division{ID: "dept/patrol", Label: "Patrol"})
		}, `division "dept/patrol" is filed under department/city-council and department/police-department`},
		{"a paid department the membership does not state", func(o *export.Options) {
			o.Departments = o.Departments[1:]
		}, "pays department/city-council, which the department page states no membership for"},
		{"a stated department nothing pays", func(o *export.Options) {
			o.Departments = append(o.Departments, export.Department{ID: "department/fire-department",
				Label: "Fire Department", Divisions: []export.Division{{ID: "dept/fire-administration", Label: "Fire"}}})
		}, "draws no fund paying department/fire-department"},
		{"a department label the document does not carry", func(o *export.Options) {
			police(o.Departments).Label = "Police"
		}, `labels department/police-department "Police Department" and the membership "Police"`},
		{"a division label the document does not carry", func(o *export.Options) {
			police(o.Departments).Divisions[0].Label = "Patrol Division"
		}, `labels dept/patrol "Patrol" and the membership "Patrol Division"`},
		{"a held-apart cell in a column the page does not open", func(o *export.Options) {
			o.DepartmentsHeldApart[0].FiscalYear = 2026
		}, "is held apart in fy2026-adopted.json, which the department page does not open"},
		{"a held-apart cell with no reason", func(o *export.Options) {
			o.DepartmentsHeldApart[0].Reason = ""
		}, "with no reason"},
		{"no membership at all", func(o *export.Options) {
			o.Departments = nil
		}, "the department page is given no departments"},
		// site/department.js says a spending row is not an expenditure by its
		// links' kind; these are the declarations it would read wrong.
		{"a Transfers Out row declared external", func(o *export.Options) {
			o.Projections["department-spending-2027"] = editSpendingKind(t, o, `"internal_transfer"`, `"external"`)
		}, `draws transfers/out -> dept/horizons as "external", and its object end's role "transfer_out" declares "internal_transfer"`},
		{"a Transfers Out row with no kind", func(o *export.Options) {
			o.Projections["department-spending-2027"] = editSpendingKind(t, o, `"kind":"internal_transfer",`, ``)
		}, `draws transfers/out -> dept/horizons as "", and its object end's role "transfer_out" declares "internal_transfer"`},
		{"an expenditure row declared a transfer", func(o *export.Options) {
			o.Projections["department-spending-2027"] = editSpendingKind(t, o,
				`"target":"dept/city-council","value_cents":2000,"kind":"external"`,
				`"target":"dept/city-council","value_cents":2000,"kind":"internal_transfer"`)
		}, `draws expenditure/wages-and-benefits -> dept/city-council as "internal_transfer"`},
		{"a spending row whose object end has no role the page draws", func(o *export.Options) {
			o.Projections["department-spending-2027"] = editSpendingKind(t, o, `"role":"transfer_out"`, `"role":"fund"`)
		}, `its object end's role "fund" declares ""`},
		// site/department.js splits a department's bar by its funds' groups.
		{"a paying fund with no fund group", func(o *export.Options) {
			o.Projections["department-funding-2027"] = editFunding(t, o, `,"parent":"fund-group/internal-service"`, ``)
		}, `draws fund/730 paying department/police-department, parented to "", which is not a fund group`},
		{"a paying fund parented to a group the document does not draw", func(o *export.Options) {
			o.Projections["department-funding-2027"] = editFunding(t, o, `"parent":"fund-group/internal-service"`,
				`"parent":"fund-group/enterprise"`)
		}, `parented to "fund-group/enterprise", which is not a fund group the document draws`},
		{"a paying fund parented to a node that is not a fund group", func(o *export.Options) {
			o.Projections["department-funding-2027"] = editFunding(t, o, `"parent":"fund-group/internal-service"`,
				`"parent":"department/city-council"`)
		}, `parented to "department/city-council", which is not a fund group the document draws`},
		{"a page reading no spending schedule", func(o *export.Options) {
			o.Views[1].Reads = nil
		}, "the page reads department-spending and nothing else"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := deptOptions(t, t.TempDir())
			tt.edit(&o)
			_, err := writeSite(o)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

// editSpendingKind is the options' department-spending document with old
// replaced by new exactly once.
func editSpendingKind(t *testing.T, o *export.Options, old, new string) []byte {
	t.Helper()
	doc := string(o.Projections["department-spending-2027"])
	if strings.Count(doc, old) != 1 {
		t.Fatalf("department-spending-2027 carries %q %d times, want once", old, strings.Count(doc, old))
	}
	return []byte(strings.Replace(doc, old, new, 1))
}

// editFunding is the options' department-funding document with old replaced
// by new exactly once.
func editFunding(t *testing.T, o *export.Options, old, new string) []byte {
	t.Helper()
	doc := string(o.Projections["department-funding-2027"])
	if strings.Count(doc, old) != 1 {
		t.Fatalf("department-funding-2027 carries %q %d times, want once", old, strings.Count(doc, old))
	}
	return []byte(strings.Replace(doc, old, new, 1))
}

// TestOnlyTheDepartmentPageReadsASecondSchedule: a view reading a schedule
// beside its own on a template that reads none is refused, and a year whose
// column carries no such schedule is refused, rather than the page reading
// nothing.
func TestOnlyTheDepartmentPageReadsASecondSchedule(t *testing.T) {
	o := deptOptions(t, t.TempDir())
	o.Views[0].Reads = []string{project.DepartmentSpendingProjection}
	if _, err := writeSite(o); err == nil || !strings.Contains(err.Error(), "reads no second schedule") {
		t.Errorf("a spine view reading department-spending: got %v", err)
	}
	o = deptOptions(t, t.TempDir())
	o.Views[1].Reads = []string{project.FundFlowsProjection, project.DepartmentSpendingProjection}
	if _, err := writeSite(o); err == nil || !strings.Contains(err.Error(), `reads schedule "fund-flows"`) {
		t.Errorf("a department view reading a schedule its column lacks: got %v", err)
	}
}
