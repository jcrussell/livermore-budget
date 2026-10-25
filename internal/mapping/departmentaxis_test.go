package mapping

import (
	"strings"
	"testing"
)

// p167Departments is Budget Book p167's CITY MANAGER and CITY ATTORNEY
// sections with their FY2023-24 Actuals: three divisions in two departments.
const p167Departments = `CITY MANAGER

City Manager          Wages & Benefits            2,175,746
                      Services & Supplies          340,367
                      Total                      $2,516,113

City Clerk            Wages & Benefits             970,912
                      Services & Supplies          728,300
                      Total                      $1,699,212

CITY MANAGER TOTAL                               $4,215,325

CITY ATTORNEY

City Attorney         Wages & Benefits            1,843,067
                      Services & Supplies          482,362
                      Total                      $2,325,429

CITY ATTORNEY TOTAL                              $2,325,429
`

// departmentRules maps the three divisions, one rule each, and rolls them up
// into their two departments. The #ROLLUPS placeholder is where a test states
// which department claims which divisions.
const departmentRules = `schema_version: 1
doc_id: dept-doc

rules:
  - id: div-city-manager
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits, department: city-manager}
      - {label: "Services & Supplies", category: services-and-supplies, department: city-manager}
    parts:
      - page: 167
        section: "City Manager  "
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}

  - id: div-city-clerk
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits, department: city-clerk}
      - {label: "Services & Supplies", category: services-and-supplies, department: city-clerk}
    parts:
      - page: 167
        section: "City Clerk"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}

  - id: div-city-attorney
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits, department: city-attorney}
      - {label: "Services & Supplies", category: services-and-supplies, department: city-attorney}
    parts:
      - page: 167
        section: "City Attorney"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}

rollups:
#ROLLUPS
`

// correctAttribution is what p167 prints: City Clerk under CITY MANAGER. A
// one-division department still covers its rule; see
// TestARollupMayCoverOneRule.
const correctAttribution = `  - id: dept-city-manager
    page: 167
    total_row: "CITY MANAGER TOTAL"
    covers: [div-city-manager, div-city-clerk]`

func departmentResolver(t *testing.T, rollups string) (*File, *Resolver) {
	t.Helper()
	src := strings.Replace(departmentRules, "#ROLLUPS", rollups, 1)
	f, err := parse(strings.NewReader(src), "departments.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{167: p167Departments}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// TestADivisionUnderTheWrongDepartmentIsCaughtOnlyByTheRollup: moving City
// Clerk to CITY ATTORNEY leaves division totals, category sums and labels
// green; only the printed department total notices.
func TestADivisionUnderTheWrongDepartmentIsCaughtOnlyByTheRollup(t *testing.T) {
	t.Run("as the document prints it", func(t *testing.T) {
		f, r := departmentResolver(t, correctAttribution)
		if _, err := r.CheckRollup(&f.Rollups[0]); err != nil {
			t.Fatalf("CheckRollup: %v; 2,516,113 + 1,699,212 = 4,215,325", err)
		}
	})

	t.Run("with City Clerk moved to the City Attorney", func(t *testing.T) {
		f, r := departmentResolver(t, `  - id: dept-city-manager
    page: 167
    total_row: "CITY MANAGER TOTAL"
    covers: [div-city-manager, div-city-attorney]

  - id: dept-city-attorney
    page: 167
    total_row: "CITY ATTORNEY TOTAL"
    covers: [div-city-clerk, div-city-attorney]`)

		// Every per-rule check still passes. This is the half that makes the
		// rollup worth having.
		for i := range f.Rules {
			rule := &f.Rules[i]
			if _, err := r.CheckTotals(rule, &rule.Parts[0]); err != nil {
				t.Fatalf("CheckTotals(%s): %v; a division's own total is unaffected "+
					"by which department claims it", rule.ID, err)
			}
		}
		// The department total is not.
		if _, err := r.CheckRollup(&f.Rollups[0]); err == nil {
			t.Error("CITY MANAGER TOTAL tied with the City Attorney's division " +
				"in place of the City Clerk's")
		}
	})
}

// TestTheCategorySumsAreBlindToTheDepartmentMove: category sums are identical
// under both attributions, since moving a division moves no figure.
func TestTheCategorySumsAreBlindToTheDepartmentMove(t *testing.T) {
	byCategory := func(f *File, r *Resolver) map[string]int64 {
		t.Helper()
		out := map[string]int64{}
		for i := range f.Rules {
			rule := &f.Rules[i]
			values, _, err := r.Values(rule, &rule.Parts[0])
			if err != nil {
				t.Fatalf("Values(%s): %v", rule.ID, err)
			}
			for _, v := range values {
				out[v.Row.Category] += int64(v.Cents)
			}
		}
		return out
	}

	fa, ra := departmentResolver(t, correctAttribution)
	fb, rb := departmentResolver(t, `  - id: dept-city-manager
    page: 167
    total_row: "CITY MANAGER TOTAL"
    covers: [div-city-manager, div-city-attorney]`)

	want, got := byCategory(fa, ra), byCategory(fb, rb)
	if len(want) == 0 {
		t.Fatal("no categories summed, so this test asserts nothing")
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("category %s: %d vs %d", k, v, got[k])
		}
	}
	// Not caught: a covers list wrong in a way that still sums, or a wrong
	// parent in the department registry.
}
