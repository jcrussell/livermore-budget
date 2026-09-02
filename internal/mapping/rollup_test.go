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
//
// IT ALSO PINS THE SIGN, which is fisc-7a6 and which this test did not do
// before. Asserting only that the two figures appear passes whichever direction
// the difference is printed in, and CheckRollup printed the negation of the
// quantity it had computed -- inherited, along with the comment justifying the
// direction, from compareTotals, which had the same inversion. A test that
// names both totals and shrugs at the number between them is how that survived
// a review; see TestTheUndeclaredFailureStatesTheDeltaToPasteIn for the same
// defect at the site where the number is meant to be pasted into a declaration.
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

// cityCouncilPage is Budget Book p167's CITY COUNCIL section, transcribed with
// its printed FY2023-24 Actual figures.
//
// It is the single-division case, and there are six of them on pp.167-170. The
// department prints a TOTAL of its own three lines below the division's, with
// the same figure -- which is why refusing a one-rule rollup looked reasonable
// and was wrong: they are two printed lines, and only one of them was asserted.
//
// Note the two anchors differ in case, not in words: "Total" and "TOTAL". That
// is what keeps them apart on this page, and it is also why the aliasing test
// below can reach the resolver at all.
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
	f, err := Parse(strings.NewReader(strings.Replace(cityCouncilRules, "#ANCHOR", anchor, 1)),
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

// A ROLLUP MAY COVER ONE RULE, and six of pp.167-170's eleven department totals
// need it to.
//
// City Council, City Attorney, Library, Innovation, General Services and Fire
// each have exactly one division, and each prints a <DEPARTMENT> TOTAL that is a
// SECOND LINE with its own figures. The parser used to refuse this on the
// reasoning that such a total "is that rule's total_row", which is true of the
// number and false of the line, and it left six printed totals asserted by
// nothing.
//
// The second case is what makes the first a real assertion rather than a
// tautology: change what the department line prints and the rollup fails.
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

// The invariant that replaced the count: a rollup may not be anchored on a line
// that is already one of its covered rules' totals.
//
// This is the hazard letting covers==1 through would otherwise open. `" Total"`
// is not equal to `"Total"`, so the parser's string guard passes it; on a page
// with one division it occurs exactly once, so the anchor-uniqueness guard
// passes it too. It then resolves to the division's OWN printed Total -- an
// assertion of a figure against itself, green whatever the object rows say.
//
// The parser cannot see this: which part bears a rule's total, and where it
// lands on the page, needs the pages.
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
// panicked; a crash is not a closed failure, whatever it stops.
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
func TestAWhitespaceUnassertableIsRefused(t *testing.T) {
	src := strings.Replace(adminServicesRules, "    #COVERS",
		adminCovers+"\n    unassertable: \" \"", 1)
	if _, err := Parse(strings.NewReader(src), "admin.yaml"); err == nil {
		t.Fatal("accepted a whitespace-only unassertable")
	} else if !strings.Contains(err.Error(), "is whitespace; omit it or give the reason") {
		t.Errorf("error = %v", err)
	}
}

// TestARollupWhoseRulesStateDifferentColumnsFailsClosed is fisc-7fe, and the
// fixture is built so that the BUG PRODUCES A CLEAN TIE.
//
// Both covered rules state ONE column, so the width guard is satisfied. But
// div-one's total is stated over FY2026 and div-two's over FY2027, and the
// printed DEPARTMENT TOTAL of $400 is exactly 100 + 300. With only a length
// test, this rollup adds one year to another and reports success -- the
// plausible wrong value this project exists to refuse, arrived at by a check
// whose whole job was to prevent one.
//
// WHY THE PARSER CANNOT SEE IT. validateRollups compares each covered rule's
// Parts[0].Columns and they agree here: div-two's FIRST part is its labels_from
// continuation on page 2, declared over FY2026, while the part that BEARS its
// printed total is page 3, declared over FY2027. Which part bears the total
// needs the pages, which is the reason CheckRollup takes its columns from the
// bearer -- and then, until this test, compared only how many there were.
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
          - {fund_group: general, fiscal_year: 2027}

rollups:
  - id: dept-total
    page: 1
    total_row: "DEPARTMENT TOTAL"
    covers: [div-one, div-two]
`
	f, err := Parse(strings.NewReader(src), "columns.yaml")
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

// TestARollupAcrossTwoBasesFailsClosed is the half of fisc-7fe that comparing
// Column values alone would have missed, and it was found reviewing the fix.
//
// Column.Basis is an OVERRIDE, empty on almost every column in this corpus.
// fact.FromValues falls back to the rule's basis, so these two rules publish
// facts on different bases while their column blocks are byte-identical and
// every Column compares equal. Summing a revised total into an adopted one is
// the same defect as summing FY2026 into FY2027 -- one printed figure added to
// another that means something else -- reached through the other declaration.
//
// The fixture ties at $400 under the bug, so the wrong implementation reports
// success rather than a different failure.
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
	f, err := Parse(strings.NewReader(src), "bases.yaml")
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

// TestARollupCoveringNothingErrorsRatherThanPanicking is fisc-t9h.
//
// The covers list is emptied AFTER parsing, which is not a contrivance: it is
// what a second caller looks like. The parser refuses a rollup declaring
// neither covers nor unassertable and pkg/cmd/build skips CheckRollup for an
// unassertable one, so both of today's guards live outside this function --
// and CheckRollup is exported. A verify-side structural sweep over
// Subject.Resolvers reaches it by doing nothing wrong.
//
// A panic is not a closed failure, whatever it stops. Same class as the
// Parts[0] panic fisc-3bl fixed here, one caller further away.
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

// TestFirstDifferingColumnRefusesRunsOfDifferentWidth calls the helper directly,
// which is the only way to reach the branch this test is about (fisc-oz4).
//
// CheckRollup's own width guard runs before it, so no rule file can produce a
// length mismatch here. That is exactly why the branch was wrong for so long: it
// was written to make the helper safe for its NEXT caller, and it made it
// crash instead. The old code returned min(len(a), len(b)) and the caller
// indexed BOTH slices at that value, so 3 columns against 5 returned 3 and
// panicked on a[3] -- one past the last valid index of the shorter run.
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
			// WHY THE OLD RETURN VALUE WAS UNUSABLE IS HISTORY AND IS NOT
			// ASSERTED HERE, after three attempts to assert it produced three
			// tautologies. min(len(a), len(b)) is a valid index into the LONGER
			// run and exactly one past the end of the shorter, so the caller's
			// first indexing operation succeeded and its second panicked -- but
			// that is a property of `min`, not of anything in this tree. The
			// code it describes is gone, and AGENTS.md's rule applies: a claim
			// whose baseline no longer exists cannot be checked.
			//
			// What IS checkable is above and is proven by mutation: restoring
			// the min() return makes `comparable` true and this test fails.
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
