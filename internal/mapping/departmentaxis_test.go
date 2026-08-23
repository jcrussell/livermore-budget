package mapping

import (
	"strings"
	"testing"
)

// p167Departments is Budget Book p167's CITY MANAGER and CITY ATTORNEY
// sections, transcribed with their printed FY2023-24 Actual figures.
//
// Three divisions across two departments, which is the smallest shape that can
// state the defect: City Manager and City Clerk roll up to CITY MANAGER TOTAL,
// City Attorney alone to CITY ATTORNEY TOTAL.
//
//	City Manager    2,175,746 +   340,367 = $2,516,113
//	City Clerk        970,912 +   728,300 = $1,699,212
//	                            CITY MANAGER TOTAL $4,215,325
//	City Attorney   1,843,067 +   482,362 = $2,325,429
//	                            CITY ATTORNEY TOTAL $2,325,429
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

// correctAttribution is what p167 prints: City Clerk is a division of CITY
// MANAGER. A one-division department still declares its rollup unassertable
// rather than covering one rule, because a total over a single rule IS that
// rule's total_row.
const correctAttribution = `  - id: dept-city-manager
    page: 167
    total_row: "CITY MANAGER TOTAL"
    covers: [div-city-manager, div-city-clerk]`

func departmentResolver(t *testing.T, rollups string) (*File, *Resolver) {
	t.Helper()
	src := strings.Replace(departmentRules, "#ROLLUPS", rollups, 1)
	f, err := Parse(strings.NewReader(src), "departments.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{167: p167Departments}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// TestADivisionUnderTheWrongDepartmentIsCaughtOnlyByTheRollup is fisc-b9j.
//
// It is written as a pair, and the second half is the point. Moving City Clerk
// from CITY MANAGER to CITY ATTORNEY leaves EVERY OTHER CHECK GREEN:
//
//   - each division's own rows still tie to its own printed Total, because a
//     division's Total covers exactly its own rows and never names its
//     department;
//   - every category sum is unchanged, so an expenditure-detail tie keyed on
//     {kind, category, fund_group} -- which does not carry department -- still
//     holds to the cent;
//   - the row labels, categories and department slugs are all well formed, so
//     a vocabulary check sees nothing.
//
// The printed department total is the only thing that notices. That is why the
// rollup had to be built rather than the twelve totals declared unassertable.
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

// TestTheCategorySumsAreBlindToTheDepartmentMove states, as arithmetic rather
// than as prose, the reason the rollup is the only check that sees this.
//
// It sums the mapped values by category across all three rules under both
// attributions. The numbers are identical, because moving a division between
// departments moves no figure -- which is exactly why a check keyed on
// {kind, category, fund_group} cannot notice.
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
	// And state the residual risk this mitigation does NOT close, per
	// fisc-u54's discipline: a rollup whose covers list is wrong in a way that
	// still sums correctly is not caught, and neither is a department registry
	// entry whose parent relation is itself wrong. Nothing here asserts
	// otherwise.
}
