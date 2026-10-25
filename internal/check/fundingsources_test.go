package check

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// These tests read pp.85-125 with a scanner that shares no code with
// internal/mapping and compare them against figures typed out of pp.66-67,
// so a rule that mis-reads a column leaves this file red.

const budgetDoc = "livermore-budget-fy2026-2027"

// fundingPages are the fourteen pages (eleven departments) that print
// DEPARTMENTWIDE EXPENDITURES WITH FUNDING SOURCES. Pairs 101/102, 119/120 and
// 124/125 split a block: no total on the first, no heading on the second.
var fundingPages = []struct {
	page      int
	continued bool // the heading is on the previous page
	endsHere  bool // this page prints Total Department Funding Sources
}{
	{85, false, true}, {89, false, true}, {93, false, true}, {97, false, true},
	{101, false, false}, {102, true, true},
	{105, false, true}, {108, false, true}, {111, false, true}, {115, false, true},
	{119, false, false}, {120, true, true},
	{124, false, false}, {125, true, true},
}

// spineExpenditureByGroup is pp.66-67's TOTAL EXPENDITURES row per fund group
// and budget year, typed from the pages rather than read off the fact store.
// The internal-service FY2027 figure is p0067's outlier (fisc-av0w).
var spineExpenditureByGroup = map[string][2]amount.Cents{
	// group:            FY2026            FY2027
	"capital":          {amount.Cents(106135500), amount.Cents(96993400)},
	"debt-service":     {amount.Cents(698459700), amount.Cents(696989800)},
	"enterprise":       {amount.Cents(5705373000), amount.Cents(5754797000)},
	"general":          {amount.Cents(14465080200), amount.Cents(14901457900)},
	"internal-service": {amount.Cents(2507736700), amount.Cents(2654451500)},
	"special-revenue":  {amount.Cents(1926756100), amount.Cents(1180800000)},
}

// fundingRowRE: a data row is a label, two or more spaces, then figures split
// on whitespace for amount.Parse. Safe only because these pages print no
// standalone "$" token.
var fundingRowRE = regexp.MustCompile(`^(.*?)\s\s+([$\-\d].*)$`)

// readFundingRows scans one page's Department Funding Sources block into
// (label, four values) rows without consulting mappings/. continued and
// endsHere are declared by the caller, so a page that stops printing its
// total is a failure and not a shorter list.
func readFundingRows(t *testing.T, text string, continued, endsHere bool) [][5]string {
	t.Helper()
	var out [][5]string
	started := continued
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "Total Department Funding Sources") {
			return out
		}
		if strings.HasPrefix(s, "Department Funding Sources") {
			started = true
			continue
		}
		if !started {
			continue
		}
		m := fundingRowRE.FindStringSubmatch(s)
		if m == nil {
			// The continuation pages open with the department heading and the
			// column header line, neither of which is a data row.
			continue
		}
		vals := strings.Fields(m[2])
		if len(vals) != 4 {
			t.Fatalf("%q yields %d value tokens, not 4; every row on these pages "+
				"prints four", s, len(vals))
		}
		row := [5]string{strings.TrimSpace(m[1])}
		copy(row[1:], vals)
		out = append(out, row)
	}
	if endsHere {
		t.Fatal("no Total Department Funding Sources on a page that should print one")
	}
	if !started {
		t.Fatal("no Department Funding Sources heading on a page that should print one")
	}
	return out
}

func fundingCents(t *testing.T, tok string) amount.Cents {
	t.Helper()
	c, err := amount.Parse(tok, amount.Dollars)
	if err != nil {
		t.Fatalf("Parse(%q): %v", tok, err)
	}
	return c
}

// TestThePrintedFundingSourcesSumToTheSpineByFundGroup: pp.85-125's 78 rows,
// bucketed by fund type through data/funds.yaml, sum to pp.66-67's TOTAL
// EXPENDITURES for each fund group in both years, to the cent, bar fisc-av0w.
func TestThePrintedFundingSourcesSumToTheSpineByFundGroup(t *testing.T) {
	t.Parallel()
	s := committed(t)
	doc, ok := s.Docs[budgetDoc]
	if !ok {
		t.Fatalf("no extraction for %s", budgetDoc)
	}

	sums := map[string][2]amount.Cents{}
	rows := 0
	labels := map[string]bool{}
	for _, fp := range fundingPages {
		text, err := doc.Page(fp.page)
		if err != nil {
			t.Fatalf("page %d: %v", fp.page, err)
		}
		for _, r := range readFundingRows(t, text, fp.continued, fp.endsHere) {
			entry, err := s.Vocabulary.FundByLabel(r[0])
			if err != nil {
				t.Fatalf("p%d: the page prints %q as a funding source and "+
					"data/funds.yaml resolves no such fund: %v", fp.page, r[0], err)
			}
			rows++
			labels[r[0]] = true
			acc := sums[entry.Type]
			// r[3] is FY2025-26 and r[4] FY2026-27; r[1] and r[2] are actual
			// and revised, which the spine does not print.
			acc[0] += fundingCents(t, r[3])
			acc[1] += fundingCents(t, r[4])
			sums[entry.Type] = acc
		}
	}

	if rows != 78 || len(labels) != 63 {
		t.Errorf("read %d rows naming %d distinct funds, want 78 and 63; the schedule's "+
			"shape changed and every figure below is about a different set of rows",
			rows, len(labels))
	}

	// The permanent group is zero in both budget years and pp.66-67 print no
	// Permanent column; asserted rather than filtered, so budgeted permanent
	// money goes red instead of vanishing.
	if got := sums["permanent"]; got[0] != 0 || got[1] != 0 {
		t.Errorf("permanent funds pay %s / %s in the budget years and pp.66-67 print no "+
			"Permanent column to reconcile that against (fisc-u8o)", got[0], got[1])
	}
	delete(sums, "permanent")

	var groups []string
	for g := range sums {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	if len(groups) != len(spineExpenditureByGroup) {
		t.Errorf("the pages name funds in %d groups (%s) and the spine publishes %d",
			len(groups), strings.Join(groups, ", "), len(spineExpenditureByGroup))
	}

	var total [2]amount.Cents
	for _, g := range groups {
		got, want := sums[g], spineExpenditureByGroup[g]
		total[0] += got[0]
		total[1] += got[1]
		for i, fy := range []int{2026, 2027} {
			if got[i] == want[i] {
				continue
			}
			// The one cell the book contradicts itself on, named so a change
			// to either figure fails.
			if g == "internal-service" && fy == 2027 &&
				got[i] == amount.Cents(2629451500) && want[i] == amount.Cents(2654451500) {
				continue
			}
			t.Errorf("FY%d %s: the departments' funding sources sum to %s and pp.66-67 "+
				"print %s, a difference of %s", fy, g, got[i], want[i], got[i]-want[i])
		}
	}

	// The grand total the site publishes: FY2026 exact, FY2027 short by
	// fisc-av0w's 250,000.
	if want := amount.Cents(25409541200); total[0] != want {
		t.Errorf("FY2026 total = %s, want %s (all_funds_gross_expenditure_cents)",
			total[0], want)
	}
	if want := amount.Cents(25260489600); total[1] != want {
		t.Errorf("FY2027 total = %s, want %s", total[1], want)
	}
}

// TestEveryFundingSourceFactMatchesThePrintedRow: each funding-source fact is
// the figure its page prints, cell for cell, which catches a rule reading the
// wrong column even where every group sum survives.
func TestEveryFundingSourceFactMatchesThePrintedRow(t *testing.T) {
	t.Parallel()
	s := committed(t)
	doc, ok := s.Docs[budgetDoc]
	if !ok {
		t.Fatalf("no extraction for %s", budgetDoc)
	}

	// (page, printed label, column index) -> the figure the page prints.
	printed := map[string]amount.Cents{}
	for _, fp := range fundingPages {
		text, err := doc.Page(fp.page)
		if err != nil {
			t.Fatalf("page %d: %v", fp.page, err)
		}
		for _, r := range readFundingRows(t, text, fp.continued, fp.endsHere) {
			for i := 0; i < 4; i++ {
				printed[fmt.Sprintf("%d\x1f%s\x1f%d", fp.page, r[0], i)] =
					fundingCents(t, r[i+1])
			}
		}
	}

	column := map[int]int{2024: 0, 2025: 1, 2026: 2, 2027: 3}
	seen := 0
	for i := range s.Facts {
		f := &s.Facts[i]
		if f.Scope != project.DepartmentFundingScope {
			continue
		}
		seen++
		// The year must be one of the four: a bare map read aliases an
		// unknown year to column 0, so a `fiscal_year: 2023` typo would pass.
		col, ok := column[f.FiscalYear]
		if !ok {
			t.Errorf("%s: p%d %q publishes FY%d and these pages print four columns, "+
				"FY2024 through FY2027", f.ID, f.Page, f.RowLabel, f.FiscalYear)
			continue
		}
		key := fmt.Sprintf("%d\x1f%s\x1f%d", f.Page, f.RowLabel, col)
		want, ok := printed[key]
		if !ok {
			t.Errorf("%s: p%d prints no funding-source row %q", f.ID, f.Page, f.RowLabel)
			continue
		}
		if got := amount.Cents(f.AmountCents); got != want {
			t.Errorf("%s: p%d %q FY%d publishes %s and the page prints %s",
				f.ID, f.Page, f.RowLabel, f.FiscalYear, got, want)
		}

		// The printed label decides the fund. A same-type substitution passes
		// fact-funds-resolve and cuts-tie-along-the-lattice; every one of the
		// 63 distinct labels resolves to exactly one fund in data/funds.yaml.
		// This is the fact-side twin of row-funds-match-their-anchors.
		entry, err := s.Vocabulary.FundByLabel(f.RowLabel)
		if err != nil {
			t.Errorf("%s: %q resolves to no fund: %v", f.ID, f.RowLabel, err)
			continue
		}
		if f.Fund == nil || entry.Number != *f.Fund {
			t.Errorf("%s: p%d prints %q, which is fund %d (%s), and the rule declares "+
				"fund %s", f.ID, f.Page, f.RowLabel, entry.Number, entry.Name, fact.FundString(f.Fund))
		}
		if entry.Type != f.FundGroup {
			t.Errorf("%s: p%d %q is fund %d, type %q in data/funds.yaml, and the rule "+
				"declares fund group %q", f.ID, f.Page, f.RowLabel, entry.Number,
				entry.Type, f.FundGroup)
		}
	}
	if want := 78 * 4; seen != want {
		t.Errorf("scope %q carries %d facts, want %d (78 rows x 4 columns)",
			project.DepartmentFundingScope, seen, want)
	}
}

// TestP0067IsTheOutlierAndFivePagesDisagree: both figures
// structure.BudgetBookExceptions claims are still printed. FY2026-27 internal
// service is 26,544,515 on p0067:34 and 26,294,515 on p0183, p0075, p0205 and
// p0209; p0061 prints 26,906,515 including the 612,000 to CIP.
func TestP0067IsTheOutlierAndFivePagesDisagree(t *testing.T) {
	t.Parallel()
	s := committed(t)
	doc, ok := s.Docs[budgetDoc]
	if !ok {
		t.Fatalf("no extraction for %s", budgetDoc)
	}
	page := func(n int) string {
		t.Helper()
		text, err := doc.Page(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		return text
	}

	// The exception's own two constants, spelled as the pages spell them.
	const outlier = "26,544,515"
	const agreed = "26,294,515"

	if !strings.Contains(page(67), outlier) {
		t.Errorf("p0067 no longer prints %s. If the city reissued the page, delete the "+
			"by-fund-group exception rather than re-pointing it (fisc-av0w)", outlier)
	}
	for _, p := range []int{183, 75, 205, 209} {
		if !strings.Contains(page(p), agreed) {
			t.Errorf("p%04d no longer prints %s, so the exception rests on one page fewer",
				p, agreed)
		}
	}
	// p0061 prints the same quantity plus the 612,000 transfer to the CIP.
	if !strings.Contains(page(61), "26,906,515") {
		t.Errorf("p0061 no longer prints 26,906,515 (= %s + 612,000 to CIP)", agreed)
	}
	// p0067 prints none of the agreed figure: the disagreement is a cell,
	// not a rounding.
	if strings.Contains(page(67), agreed) {
		t.Errorf("p0067 prints %s after all; the exception says it does not", agreed)
	}

	// p0183's figure is summed from five per-fund totals, so it is
	// independent evidence. Their Services & Supplies sum, 16,546,010, is
	// printed nowhere, so it is derived here and cited as no page figure.
	var fromFunds amount.Cents
	for _, tok := range []string{
		"$3,785,705", // Total Facilities Rehab Pgm
		"$5,484,232", // Total Fleet & Equipment Services
		"$6,517,952", // Total General Liability
		"$7,555,789", // Total Information Technology
		"$2,950,837", // Total Workers Comp Insurance
	} {
		if !strings.Contains(page(183), tok) {
			t.Errorf("p0183 no longer prints %s", tok)
		}
		fromFunds += fundingCents(t, tok)
	}
	if want := amount.Cents(2629451500); fromFunds != want {
		t.Errorf("p0183's five printed fund totals sum to %s, not %s", fromFunds, want)
	}
	if !strings.Contains(page(67), "16,796,010") {
		t.Error("p0067 no longer prints 16,796,010, the Services & Supplies figure that " +
			"is 250,000 more than the five funds on p0183 add up to")
	}
	// The same page's Grand Total is what the departmentwide pages sum to.
	for _, tok := range []string{"$254,095,412", "$252,604,896"} {
		if !strings.Contains(page(183), tok) {
			t.Errorf("p0183's Grand Total no longer prints %s, which is what pp.85-125's "+
				"Total Department Expenditures rows sum to", tok)
		}
	}
	if got := spineExpenditureByGroup["internal-service"][1] -
		amount.Cents(2629451500); got != amount.Cents(25000000) {
		t.Errorf("the two printed figures now differ by %s, not by $250,000", got)
	}
}

// TestTheScopeIsWhatStopsTheDoubling: re-scoping these rules to
// all-funds-gross is a one-word YAML edit that doubles the city's expenditure
// without an error. structure.ValidateCuts refuses it before any cell is
// summed: the moved facts put a second grain under the spine cut's scope.
func TestTheScopeIsWhatStopsTheDoubling(t *testing.T) {
	t.Parallel()
	base := committed(t)
	s := *base
	s.Facts = append([]fact.Fact(nil), base.Facts...)
	moved := 0
	for i := range s.Facts {
		if s.Facts[i].Scope == project.DepartmentFundingScope {
			s.Facts[i].Scope = spineScope
			moved++
		}
	}
	if moved != 312 {
		t.Fatalf("moved %d facts, want 312", moved)
	}

	res := runCheck(t, &cutsTieAlongTheLattice{}, &s)
	wantFail(t, res, `cut "spine"`, "no single level")
	// And the doubling it would otherwise publish, measured on the same facts:
	// the spine's General Fund FY2026 expenditure is exactly twice p66's.
	var general amount.Cents
	for i := range s.Facts {
		f := &s.Facts[i]
		if f.Scope == spineScope && f.Kind == vocab.KindExpenditure &&
			f.FundGroup == "general" && f.FiscalYear == 2026 {
			general += amount.Cents(f.AmountCents)
		}
	}
	if want := amount.Cents(2 * 14465080200); general != want {
		t.Errorf("General Fund FY2026 expenditure at the spine's scope = %s, want %s "+
			"(exactly double 144,650,802); if this is no longer a doubling the scope "+
			"guard is protecting against something else", general, want)
	}
}
