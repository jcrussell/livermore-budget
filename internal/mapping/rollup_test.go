package mapping

import (
	"strings"
	"testing"
)

// adminServicesPage is Budget Book p168's ADMINISTRATIVE SERVICES section,
// FY2023-24 Actual. Its object rows sum to 6,311,563 and its printed division
// and department totals to 6,311,564: Finance's $1 lives in its printed Total,
// so only a rollup over printed subtotals ties.
const adminServicesPage = `ADMINISTRATIVE SERVICES

Administrative Services
      Wages & Benefits          567,096
      Services & Supplies        80,664
      Total                    $647,760

Finance
      Wages & Benefits        2,543,421
      Services & Supplies     1,129,921
      Total                  $3,673,343

Human Resources
      Wages & Benefits        1,688,328
      Services & Supplies       302,133
      Total                  $1,990,461

ADMINISTRATIVE SERVICES TOTAL          $6,311,564
`

// adminServicesRules maps the three divisions, one rule each. Finance declares
// the city's $1 of rounding, stated minus mapped, so +100.
const adminServicesRules = `schema_version: 1
doc_id: admin-doc

rules:
  - id: div-administrative-services
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits}
      - {label: "Services & Supplies", category: services-and-supplies}
    parts:
      - page: 168
        section: "Administrative Services\n"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}

  - id: div-finance
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits}
      - {label: "Services & Supplies", category: services-and-supplies}
    parts:
      - page: 168
        section: "Finance\n"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
        stated_total_deltas:
          - column: 1
            delta_cents: 100
            note: >-
              The city's own rounding: the two object rows print 3,673,342 and
              the document states 3,673,343. The dollar is absorbed again by
              ADMINISTRATIVE SERVICES TOTAL one level up.

  - id: div-human-resources
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits}
      - {label: "Services & Supplies", category: services-and-supplies}
    parts:
      - page: 168
        section: "Human Resources\n"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}

rollups:
  - id: dept-administrative-services
    page: 168
    total_row: "ADMINISTRATIVE SERVICES TOTAL"
    #COVERS
`

const adminCovers = `    covers: [div-administrative-services, div-finance, div-human-resources]`

func adminResolver(t *testing.T, covers string) (*File, *Resolver) {
	t.Helper()
	src := strings.Replace(adminServicesRules, "    #COVERS", covers, 1)
	f, err := parse(strings.NewReader(src), "admin.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{168: adminServicesPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// TestARollupSumsPrintedTotalsAndNotLeaves holds that a rollup sums printed
// division totals. Mutation: sum the object rows and it fails 6,311,563
// against 6,311,564.
func TestARollupSumsPrintedTotalsAndNotLeaves(t *testing.T) {
	f, r := adminResolver(t, adminCovers)

	// The first link of the chain: each division's rows tie to its own printed
	// Total, Finance's by way of its declared dollar.
	for i := range f.Rules {
		rule := &f.Rules[i]
		if _, err := r.CheckTotals(rule, &rule.Parts[0]); err != nil {
			t.Fatalf("CheckTotals(%s): %v", rule.ID, err)
		}
	}

	// The second: those printed totals tie to the printed department total.
	res, err := r.CheckRollup(&f.Rollups[0])
	if err != nil {
		t.Fatalf("CheckRollup: %v\n"+
			"647,760 + 3,673,343 + 1,990,461 = 6,311,564, which is what p168 prints; "+
			"a failure here means the rollup summed the object rows, which give 6,311,563", err)
	}
	if res.Rules != 3 {
		t.Errorf("Rules = %d, want 3", res.Rules)
	}
	if res.Columns != 1 {
		t.Errorf("Columns = %d, want 1", res.Columns)
	}
}

// TestARollupFailsWhenARuleIsMissingFromCovers holds that a division left out
// of covers fails the rollup, and pins the delta's sign (fisc-7a6).
// Mutation: print the negated delta and the asserted number fails.
func TestARollupFailsWhenARuleIsMissingFromCovers(t *testing.T) {
	f, r := adminResolver(t,
		"    covers: [div-administrative-services, div-finance]")
	_, err := r.CheckRollup(&f.Rollups[0])
	if err == nil {
		t.Fatal("a rollup missing Human Resources tied; 647,760 + 3,673,343 is " +
			"not the printed 6,311,564")
	}
	for _, want := range []string{"4,321,103", "6,311,564"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
	// STATED MINUS SUMMED: the document states 6,311,564 and the two covered
	// rules state 4,321,103, so the rollup is short by Human Resources' own
	// printed total and the shortfall reads POSITIVE.
	if got, want := err.Error(), "off by $1,990,461.00"; !strings.Contains(got, want) {
		t.Errorf("error does not say %q, the missing division's own total: %v", want, err)
	}
	if strings.Contains(err.Error(), "off by -$1,990,461.00") {
		t.Errorf("the shortfall is reported with the sign inverted: %v", err)
	}
}

// TestAnUnassertableRollupIsDeclaredWithItsReason covers the other answer, the
// one p140's Total Sources needs: the document prints a total that no rule
// structure here can assert, and saying so is the requirement.
func TestAnUnassertableRollupIsDeclaredWithItsReason(t *testing.T) {
	f, _ := adminResolver(t,
		`    unassertable: >-
      exceeds the pages it closes by ~$57M, differently per column (fisc-wev)`)
	if got := f.Rollups[0].Unassertable; !strings.Contains(got, "fisc-wev") {
		t.Errorf("Unassertable = %q, want the declared reason", got)
	}
	if len(f.Rollups[0].Covers) != 0 {
		t.Error("an unassertable rollup covers no rules")
	}
}

// cityCouncilPage is Budget Book p167's CITY COUNCIL section, FY2023-24 Actual:
// a single division whose department TOTAL is a second printed line with the
// same figure. The anchors differ only in case, "Total" and "TOTAL".
const cityCouncilPage = `CITY COUNCIL

City Council
      Wages & Benefits           71,118
      Services & Supplies        78,080
      Total                    $149,198

CITY COUNCIL TOTAL             $149,198
`

const cityCouncilRules = `schema_version: 1
doc_id: council-doc

rules:
  - id: div-city-council
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits, department: city-council}
      - {label: "Services & Supplies", category: services-and-supplies, department: city-council}
    parts:
      - page: 167
        section: "City Council\n"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}

rollups:
  - id: dept-city-council
    page: 167
    total_row: "#ANCHOR"
    covers: [div-city-council]
`

func councilResolver(t *testing.T, anchor, page string) (*File, *Resolver) {
	t.Helper()
	f, err := parse(strings.NewReader(strings.Replace(cityCouncilRules, "#ANCHOR", anchor, 1)),
		"council.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{167: page}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// TestARollupMayCoverOneRule holds that a one-rule rollup is accepted and
// asserted, as six of pp.167-170's department totals need. Mutation: change
// what the department line prints and the second case fails.
func TestARollupMayCoverOneRule(t *testing.T) {
	f, r := councilResolver(t, "CITY COUNCIL TOTAL", cityCouncilPage)
	if len(f.Rollups[0].Covers) != 1 {
		t.Fatalf("Covers = %v, want the parser to accept one rule", f.Rollups[0].Covers)
	}
	res, err := r.CheckRollup(&f.Rollups[0])
	if err != nil {
		t.Fatalf("CheckRollup: %v; 71,118 + 78,080 = 149,198, which both lines print", err)
	}
	if res.Rules != 1 || res.Columns != 1 {
		t.Errorf("Rules, Columns = %d, %d, want 1, 1", res.Rules, res.Columns)
	}

	// Failable: the department line printing something the division's does not.
	bad := strings.Replace(cityCouncilPage,
		"CITY COUNCIL TOTAL             $149,198",
		"CITY COUNCIL TOTAL             $149,199", 1)
	f2, r2 := councilResolver(t, "CITY COUNCIL TOTAL", bad)
	if _, err := r2.CheckRollup(&f2.Rollups[0]); err == nil {
		t.Error("a one-rule rollup tied against a department total the division does not " +
			"state; it must be a real assertion, not a restatement")
	}
}

// TestARollupMayNotBeAnchoredOnACoveredRulesOwnTotal holds that `" Total"`,
// which passes the parser's string and uniqueness guards on a one-division
// page, is refused for resolving to the division's own Total.
func TestARollupMayNotBeAnchoredOnACoveredRulesOwnTotal(t *testing.T) {
	f, r := councilResolver(t, " Total", cityCouncilPage)
	if _, err := r.CheckRollup(&f.Rollups[0]); err == nil {
		t.Fatal("a rollup anchored on its covered rule's own Total was asserted; " +
			"it compares a figure with itself and can never fail")
	} else if !strings.Contains(err.Error(), "same printed line") {
		t.Errorf("error = %v, want it to say the two anchors name one line", err)
	}
}

// TestRollupsRefuseWhatTheyCannotMean states the parser's refusals. Every one
// is a declaration that would otherwise report a number nobody could act on.
func TestRollupsRefuseWhatTheyCannotMean(t *testing.T) {
	for _, tc := range []struct{ name, covers, want string }{
		{"naming a rule that does not exist",
			"    covers: [div-finance, div-nonesuch]",
			`no rule "div-nonesuch" in this file`},
		{"counting a rule twice",
			"    covers: [div-finance, div-finance, div-human-resources]",
			`"div-finance" is listed twice`},
		{"asserting and declining at once",
			adminCovers + "\n    unassertable: \"cannot be checked\"",
			"is declared alongside covers"},
		{"saying nothing at all",
			"    note: \"\"",
			"is empty and no reason is declared"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(adminServicesRules, "    #COVERS", tc.covers, 1)
			if _, err := parse(strings.NewReader(src), "admin.yaml"); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestARollupNeedsEveryCoveredRuleToPrintATotal holds that a covered rule
// with no total_row is refused rather than silently dropped from the sum.
func TestARollupNeedsEveryCoveredRuleToPrintATotal(t *testing.T) {
	src := strings.Replace(adminServicesRules, "    #COVERS", adminCovers, 1)
	// Take the total_row off Human Resources only.
	i := strings.Index(src, "  - id: div-human-resources")
	src = src[:i] + strings.Replace(src[i:], "    total_row: \"Total\"\n", "", 1)
	if _, err := parse(strings.NewReader(src), "admin.yaml"); err == nil {
		t.Fatal("accepted a rollup covering a rule that prints no total")
	} else if !strings.Contains(err.Error(), `rule "div-human-resources" declares no total_row`) {
		t.Errorf("error = %v", err)
	}
}

// TestARollupAnchorMustNameOnePrintedLine keeps the read off the wrong line.
// A rollup has no block to search after, unlike a rule's total_row, so an
// ambiguous anchor would take whichever occurrence came first.
func TestARollupAnchorMustNameOnePrintedLine(t *testing.T) {
	for _, tc := range []struct{ name, page, want string }{
		{"absent", strings.Replace(adminServicesPage,
			"ADMINISTRATIVE SERVICES TOTAL", "ADMINISTRATIVE SERVCIES TOTAL", 1),
			"does not occur on the page"},
		{"twice", adminServicesPage +
			"ADMINISTRATIVE SERVICES TOTAL          $6,311,564\n",
			"occurs more than once on the page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(adminServicesRules, "    #COVERS", adminCovers, 1)
			f, err := parse(strings.NewReader(src), "admin.yaml")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{168: tc.page}), f)
			if err != nil {
				t.Fatalf("NewResolver: %v", err)
			}
			if _, err := r.CheckRollup(&f.Rollups[0]); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestARollupWhoseRulesStateDifferentWidthsFailsClosed holds that a covered
// rule whose total-bearing part (not Parts[0]) declares a different column
// count errors rather than panicking.
func TestARollupWhoseRulesStateDifferentWidthsFailsClosed(t *testing.T) {
	pages := map[int]string{
		1: "Division One\nWages 100\nTotal $100\nDEPARTMENT TOTAL $400\n",
		2: "CONTINUED\n50\nEND\n",
		3: "Division Two\nWages 300 400\nTotal $300 $400\n",
	}
	const src = `schema_version: 1
doc_id: widths-doc

rules:
  - id: div-one
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 1
        section: "Division One"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2026}

  - id: div-two
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 2
        labels_from: 3
        section: "CONTINUED"
        stop_at: "END"
        columns:
          - {fund_group: general, fiscal_year: 2026}
      - page: 3
        section: "Division Two"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2026}
          - {fund_group: general, fiscal_year: 2027}

rollups:
  - id: dept-total
    page: 1
    total_row: "DEPARTMENT TOTAL"
    covers: [div-one, div-two]
`
	f, err := parse(strings.NewReader(src), "widths.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, pages), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	// The assertion is that this RETURNS rather than panicking.
	_, err = r.CheckRollup(&f.Rollups[0])
	if err == nil {
		t.Fatal("a rollup over rules stating 1 and 2 columns was accepted")
	}
	if !strings.Contains(err.Error(), "states 1 columns") {
		t.Errorf("error = %v, want it to name the two widths", err)
	}
}

// TestAWhitespaceUnassertableIsRefused holds that `unassertable: " "` is
// refused, as the parser and the build must both trim the reason.
func TestAWhitespaceUnassertableIsRefused(t *testing.T) {
	src := strings.Replace(adminServicesRules, "    #COVERS",
		adminCovers+"\n    unassertable: \" \"", 1)
	if _, err := parse(strings.NewReader(src), "admin.yaml"); err == nil {
		t.Fatal("accepted a whitespace-only unassertable")
	} else if !strings.Contains(err.Error(), "is whitespace; omit it or give the reason") {
		t.Errorf("error = %v", err)
	}
}

// TestARollupWhoseRulesStateDifferentColumnsFailsClosed is fisc-7fe: both
// rules state one column, but the bearing parts are FY2026 and FY2027 and the
// printed $400 is exactly 100 + 300. Mutation: compare only column counts and
// the rollup ties. The parser compares Parts[0] and cannot see it.
func TestARollupWhoseRulesStateDifferentColumnsFailsClosed(t *testing.T) {
	pages := map[int]string{
		1: "Division One\nWages 100\nTotal $100\nDEPARTMENT TOTAL $400\n",
		2: "CONTINUED\n50\nEND\n",
		3: "Division Two\nWages 300\nTotal $300\n",
	}
	const src = `schema_version: 1
doc_id: columns-doc

rules:
  - id: div-one
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 1
        section: "Division One"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2026}

  - id: div-two
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 2
        labels_from: 3
        section: "CONTINUED"
        stop_at: "END"
        columns:
          - {fund_group: general, fiscal_year: 2026}
      - page: 3
        section: "Division Two"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2027}

rollups:
  - id: dept-total
    page: 1
    total_row: "DEPARTMENT TOTAL"
    covers: [div-one, div-two]
`
	f, err := parse(strings.NewReader(src), "columns.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, pages), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, err = r.CheckRollup(&f.Rollups[0])
	if err == nil {
		t.Fatal("a rollup summing an FY2026 total into an FY2027 one reported a clean tie")
	}
	// Naming both rules and both columns, because "the columns differ" leaves
	// the author to work out which of the two declarations is the wrong one.
	for _, want := range []string{"div-one", "div-two", "FY2026", "FY2027", "column 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// TestARollupAcrossTwoBasesFailsClosed holds that two rules on different
// bases are refused although every Column compares equal: Column.Basis is an
// override and the rule's basis fills it. Mutation: compare Column values
// alone and it ties at $400.
func TestARollupAcrossTwoBasesFailsClosed(t *testing.T) {
	pages := map[int]string{
		1: "Division One\nWages 100\nTotal $100\nDEPARTMENT TOTAL $400\n",
		2: "Division Two\nWages 300\nTotal $300\n",
	}
	const src = `schema_version: 1
doc_id: bases-doc

rules:
  - id: div-one
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 1
        section: "Division One"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2026}

  - id: div-two
    kind: expenditure
    basis: revised
    scope: all-funds-gross
    grain: fund-group-by-category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 2
        section: "Division Two"
        stop_at: "Total"
        columns:
          - {fund_group: general, fiscal_year: 2026}

rollups:
  - id: dept-total
    page: 1
    total_row: "DEPARTMENT TOTAL"
    covers: [div-one, div-two]
`
	f, err := parse(strings.NewReader(src), "bases.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, pages), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, err = r.CheckRollup(&f.Rollups[0])
	if err == nil {
		t.Fatal("a rollup summing a revised total into an adopted one reported a clean tie")
	}
	for _, want := range []string{"adopted", "revised"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name the %s basis: %v", want, err)
		}
	}
}

// TestARollupCoveringNothingErrorsRatherThanPanicking is fisc-t9h: covers is
// emptied after parsing, as an exported caller outside the parser and build
// guards could reach CheckRollup.
func TestARollupCoveringNothingErrorsRatherThanPanicking(t *testing.T) {
	f, r := adminResolver(t, adminCovers)
	ro := &f.Rollups[0]
	ro.Covers = nil

	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("CheckRollup panicked on a rollup covering nothing: %v", p)
		}
	}()
	_, err := r.CheckRollup(ro)
	if err == nil {
		t.Fatal("a rollup covering nothing was accepted")
	}
	if !strings.Contains(err.Error(), ro.ID) {
		t.Errorf("error does not name the rollup %q: %v", ro.ID, err)
	}
}

// TestFirstDifferingColumnRefusesRunsOfDifferentWidth calls the helper
// directly, since CheckRollup's width guard runs first. Mutation: return
// min(len(a), len(b)) and the caller panics on the shorter run.
func TestFirstDifferingColumnRefusesRunsOfDifferentWidth(t *testing.T) {
	three := []Column{
		{FundGroup: "general", FiscalYear: 2026},
		{FundGroup: "general", FiscalYear: 2027},
		{FundGroup: "enterprise", FiscalYear: 2026},
	}
	five := append(append([]Column{}, three...),
		Column{FundGroup: "enterprise", FiscalYear: 2027},
		Column{FundGroup: "capital", FiscalYear: 2026})

	for _, tt := range []struct {
		name string
		a, b []Column
	}{
		{"the shorter run first", three, five},
		{"the longer run first", five, three},
	} {
		t.Run(tt.name, func(t *testing.T) {
			idx, comparable := firstDifferingColumn(tt.a, tt.b)
			if comparable {
				t.Fatalf("comparable = true for runs of %d and %d columns",
					len(tt.a), len(tt.b))
			}
			if idx != 0 {
				t.Errorf("idx = %d, want 0; an incomparable pair has no differing column", idx)
			}
			// Mutation: restore the min() return and `comparable` is true and
			// this test fails.
		})
	}

	// And equal widths still answer the question the helper is named for.
	if idx, comparable := firstDifferingColumn(three, three); !comparable || idx != -1 {
		t.Errorf("identical runs = (%d, %v), want (-1, true)", idx, comparable)
	}
	differs := append([]Column{}, three...)
	differs[1] = Column{FundGroup: "general", FiscalYear: 2028}
	if idx, comparable := firstDifferingColumn(three, differs); !comparable || idx != 1 {
		t.Errorf("runs differing at 1 = (%d, %v), want (1, true)", idx, comparable)
	}
}
