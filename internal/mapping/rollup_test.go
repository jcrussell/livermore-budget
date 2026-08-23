package mapping

import (
	"strings"
	"testing"
)

// adminServicesPage is Budget Book p168's ADMINISTRATIVE SERVICES section,
// transcribed with its printed figures for the FY2023-24 Actual column.
//
// It is the measurement the whole rollup design rests on:
//
//	object rows          6,311,563   (567,096 + 80,664 + 2,543,421 +
//	                                  1,129,921 + 1,688,328 + 302,133)
//	printed div totals   6,311,564   (647,760 + 3,673,343 + 1,990,461)
//	printed dept total  $6,311,564
//
// Finance's own $1 is absorbed by its printed division Total and reappears one
// level up. A rollup summing LEAVES is off by that dollar; a rollup summing the
// printed subtotals ties exactly. One column, because the discrepancy is in
// that column and a second would only add arithmetic to read past.
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

// adminServicesRules maps the three divisions, one rule each, exactly as
// fisc-28s's A-prime row identity forces: the division is the rule.
//
// Finance declares the city's $1 of rounding, because its printed Total
// exceeds its two object rows by that much. STATED MINUS MAPPED, so +100.
const adminServicesRules = `schema_version: 1
doc_id: admin-doc

rules:
  - id: div-administrative-services
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
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
	f, err := Parse(strings.NewReader(src), "admin.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, map[int]string{168: adminServicesPage}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// TestARollupSumsPrintedTotalsAndNotLeaves is the regression test for the
// decision at the centre of fisc-3bl, and it is written so that the wrong
// implementation cannot pass it.
//
// The rollup ties. Summing the object rows instead would give 6,311,563
// against a printed 6,311,564 and fail -- so an implementation that reached
// for Values here would go red on a page where the document's arithmetic is
// entirely correct, and the only escape would be a delta declaration at the
// rollup level absorbing a structural artefact rather than a rounding.
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

// TestARollupFailsWhenARuleIsMissingFromCovers proves the check is failable in
// the way it exists to be: a division that belongs to the department but is not
// claimed by it.
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
		{"covering a single rule",
			"    covers: [div-finance]",
			"names one rule"},
		{"asserting and declining at once",
			adminCovers + "\n    unassertable: \"cannot be checked\"",
			"is declared alongside covers"},
		{"saying nothing at all",
			"    note: \"\"",
			"is empty and no reason is declared"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(adminServicesRules, "    #COVERS", tc.covers, 1)
			if _, err := Parse(strings.NewReader(src), "admin.yaml"); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestARollupNeedsEveryCoveredRuleToPrintATotal is the guard that keeps the
// chain's first link from being missing. A covered rule with no total_row has
// no stated total to contribute, so the rollup would silently be a check over
// fewer rules than it names.
func TestARollupNeedsEveryCoveredRuleToPrintATotal(t *testing.T) {
	src := strings.Replace(adminServicesRules, "    #COVERS", adminCovers, 1)
	// Take the total_row off Human Resources only.
	i := strings.Index(src, "  - id: div-human-resources")
	src = src[:i] + strings.Replace(src[i:], "    total_row: \"Total\"\n", "", 1)
	if _, err := Parse(strings.NewReader(src), "admin.yaml"); err == nil {
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
			f, err := Parse(strings.NewReader(src), "admin.yaml")
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

// TestARollupWhoseRulesStateDifferentWidthsFailsClosed covers the case the
// parser cannot: a covered rule whose TOTAL-BEARING part declares a different
// number of columns from the first rule's.
//
// Reaching it takes a rule whose bearing part is not Parts[0] -- here a
// label-less continuation is listed first and the labelled part that prints
// the total second, which the parser permits because it checks that
// labels_from points at a labelled part of the same rule and not at the order
// they appear in.
//
// It has to fail, and it has to fail CLOSED. An earlier draft took the column
// width from Parts[0] and indexed the sum with it, which read off the end and
// panicked; a crash is not a closed failure, whatever it stops. Found by
// /code-review.
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
	f, err := Parse(strings.NewReader(src), "widths.yaml")
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

// TestAWhitespaceUnassertableIsRefused closes a seam between the parser and the
// build: the first compared the reason trimmed and the second did not, so
// `unassertable: " "` alongside covers validated as "no reason given" and then
// suppressed the check as "a reason given". A rollup that asserts nothing while
// reporting an empty reason is the silence this field exists to prevent.
// Found by /code-review.
func TestAWhitespaceUnassertableIsRefused(t *testing.T) {
	src := strings.Replace(adminServicesRules, "    #COVERS",
		adminCovers+"\n    unassertable: \" \"", 1)
	if _, err := Parse(strings.NewReader(src), "admin.yaml"); err == nil {
		t.Fatal("accepted a whitespace-only unassertable")
	} else if !strings.Contains(err.Error(), "is whitespace; omit it or give the reason") {
		t.Errorf("error = %v", err)
	}
}
