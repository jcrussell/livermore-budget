package check

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// This file is the arithmetic behind pp.85-125, and it is deliberately NOT a
// second call into the machinery that publishes them.
//
// The doctrine internal/check/check.go states is that two functions over
// identical input inside one process cannot witness a wrong amount. Summing the
// committed facts and comparing them to the check that already sums the
// committed facts would be exactly that. So the tests below READ THE PAGES,
// with a scanner of their own that shares no code with internal/mapping, and
// compare what the city printed against figures typed out of pp.66-67. A rule
// that mis-read a column would move the facts and leave this file red.

const budgetDoc = "livermore-budget-fy2026-2027"

// fundingPages are the fourteen pages that print DEPARTMENTWIDE EXPENDITURES
// WITH FUNDING SOURCES, in printed order, each with where its block begins and
// ends. Three departments run onto a second page -- Community Development
// (101/102), Police (119/120) and Public Works (124/125) -- so the block on the
// first of each pair has no printed total and the block on the second has no
// printed heading. Fourteen pages, eleven departments.
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

// spineExpenditureByGroup is pp.66-67's TOTAL EXPENDITURES row, typed out of the
// pages, per fund group and budget year. It is the thing pp.85-125 must
// reproduce and it is written down rather than read off the fact store, so that
// a mis-mapped spine cannot make both sides of this comparison agree.
//
// The internal-service FY2027 figure is p0067's, which fisc-av0w records as the
// book's one self-contradicting cell; see TestP0067IsTheOutlierAndFivePagesDisagree.
var spineExpenditureByGroup = map[string][2]amount.Cents{
	// group:            FY2026            FY2027
	"capital":          {amount.Cents(106135500), amount.Cents(96993400)},
	"debt-service":     {amount.Cents(698459700), amount.Cents(696989800)},
	"enterprise":       {amount.Cents(5705373000), amount.Cents(5754797000)},
	"general":          {amount.Cents(14465080200), amount.Cents(14901457900)},
	"internal-service": {amount.Cents(2507736700), amount.Cents(2654451500)},
	"special-revenue":  {amount.Cents(1926756100), amount.Cents(1180800000)},
}

// A data row is a label, a run of two or more spaces, then the figures. Go's
// regexp has no lookahead, so the figures are split on whitespace and each field
// is handed to amount.Parse -- which is stricter than any pattern this file
// could spell and is the same parser the pipeline uses. These pages carry no
// standalone "$" token, which is what makes a whitespace split safe here and
// would not make it safe on a page that does. This used to cite fisc-yun, which
// is CLOSED -- mapping.dropCurrencyMarks handles a lone mark in the labelled
// read. What that fix does not do is make a hand-rolled split in a TEST safe,
// which is the claim this comment is actually making.
var fundingRowRE = regexp.MustCompile(`^(.*?)\s\s+([$\-\d].*)$`)

// readFundingRows scans one page's Department Funding Sources block and returns
// its printed (label, four values) rows.
//
// IT IS A SECOND READER AND NOT A WRAPPER. It knows only what a person reading
// the page knows: the block starts at the heading, ends at the total, and every
// data row is a label, a run of two or more spaces, and four figures. It does
// not consult mappings/, so it cannot inherit a mistake from it.
//
// continued says the block began on the previous page and this one carries no
// heading; endsHere says the page prints the total that closes it. Both are
// declared by the caller rather than inferred, so a page that stopped printing
// its total is a failure and not a shorter list.
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

// TestThePrintedFundingSourcesSumToTheSpineByFundGroup is this lane's whole
// argument, done from the pages.
//
// Every one of the 78 printed rows is resolved to a fund through
// data/funds.yaml, bucketed by that fund's TYPE, and summed. The result must be
// pp.66-67's own TOTAL EXPENDITURES for each of the six fund groups, in both
// budget years, to the cent -- with the single exception fisc-av0w owns.
//
// WHY THIS IS THE STRONGEST STATEMENT AVAILABLE ABOUT THE SCHEDULE: 254,095,412
// is all_funds_gross_expenditure_cents, the headline the site publishes. A
// schedule that reproduces the city's entire expenditure from a completely
// different decomposition -- by paying fund rather than by object category --
// and lands on the same number six times over is not agreeing by construction.
func TestThePrintedFundingSourcesSumToTheSpineByFundGroup(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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
			// r[3] is the FY2025-26 column and r[4] the FY2026-27 column;
			// r[1] and r[2] are the actual and revised columns the spine
			// prints no counterpart for.
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

	// THE PERMANENT GROUP IS ZERO IN BOTH BUDGET YEARS, which is the only reason
	// a six-column spine can reconcile a seven-group schedule. Doolan Canyon
	// Preserve Endow (470) pays 620,581 of Community Development in FY2023-24
	// and nothing after it, and pp.66-67 print no Permanent column at all
	// (fisc-u8o). Asserted rather than filtered out: the day the city budgets a
	// permanent fund into a department, this goes red instead of the money
	// vanishing from a comparison that has no bucket for it.
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
			// The one cell the book contradicts itself on. Named here rather
			// than skipped, so that a change to either figure is a failure.
			if g == "internal-service" && fy == 2027 &&
				got[i] == amount.Cents(2629451500) && want[i] == amount.Cents(2654451500) {
				continue
			}
			t.Errorf("FY%d %s: the departments' funding sources sum to %s and pp.66-67 "+
				"print %s, a difference of %s", fy, g, got[i], want[i], got[i]-want[i])
		}
	}

	// The grand total, because it is the figure the site publishes. FY2026 is
	// exact; FY2027 is short by the 250,000 fisc-av0w owns.
	if want := amount.Cents(25409541200); total[0] != want {
		t.Errorf("FY2026 total = %s, want %s (all_funds_gross_expenditure_cents)",
			total[0], want)
	}
	if want := amount.Cents(25260489600); total[1] != want {
		t.Errorf("FY2027 total = %s, want %s", total[1], want)
	}
}

// TestEveryFundingSourceFactMatchesThePrintedRow closes the loop the test above
// deliberately leaves open: that the facts the corpus publishes are the figures
// the pages print.
//
// The test above proves the PAGES reconcile. This one proves the MAPPING read
// them, cell for cell, and it is the arm that would catch a rule reading the
// wrong column -- a mistake that leaves the printed page untouched and every
// group sum intact if two columns were swapped in every rule at once.
func TestEveryFundingSourceFactMatchesThePrintedRow(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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
		if f.Scope != fundingSourcesScope {
			continue
		}
		seen++
		// THE YEAR HAS TO BE ONE OF THE FOUR, checked rather than looked up.
		// A bare map read aliases an unknown year to column index 0, so a
		// `fiscal_year: 2023` typo on the first column would satisfy the very
		// test written to catch a wrong-column read -- and nothing else catches
		// it either: parse.go rejects only year 0, and the tie check leaves an
		// unmatched (year, basis) pair unfailed by design.
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

		// THE PRINTED LABEL DECIDES THE FUND, and this is the only thing in the
		// tree that says so for this schedule.
		//
		// The arithmetic does not settle it: fact-funds-resolve catches a fund
		// whose TYPE disagrees with the group typed beside it, and
		// cuts-tie-along-the-lattice catches a fund that leaves its group, but
		// a same-type substitution passes both, and all four of Public Works'
		// CIP twins carry their operating fund's own type.
		//
		// The page settles it. Every one of the 63 distinct labels resolves
		// through data/funds.yaml to exactly one fund, so the number a rule
		// types is checkable against the line it was typed for.
		//
		// THIS IS NOT THE ONLY THING ASSERTING IT, and the difference is the
		// whole of fisc-90fp. These assertions alone make the 640 -> 641 swap red
		// under `go test` while `fisc verify` reports zero failures, which puts
		// the guarantee in one lane's test rather than in the gate. The eleven rules now declare row_labels_name_funds and
		// row-funds-match-their-anchors makes the same comparison, in the gate,
		// for any schedule that opts in. This stays because it is the fact-side
		// statement of it: it reads f.RowLabel off the published record, where
		// the check reads mapping.Row, and a defect between the row and the fact
		// would show here and not there.
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
			fundingSourcesScope, seen, want)
	}
}

// TestP0067IsTheOutlierAndFivePagesDisagree is what makes the by-fund-group
// exception structure.BudgetBookExceptions declares a check rather than a
// tolerance.
//
// The entry claims two figures, and BOTH are printed. If either stops being
// printed the exception is a claim about a document that no longer says it, and
// this test is the only thing that would notice: the check itself compares the
// constants against the corpus, which is the same corpus the constants were
// read from.
//
// The pages, and what each prints for FY2026-27 internal service:
//
//	p0067:34  the spine's own column               26,544,515  <- the outlier
//	p0183:64  Citywide Expenditures, per fund      26,294,515
//	p0075:53  Fund Balance by Major/Non-Major      26,294,515
//	p0205:17  Fund Balances summary, Uses          26,294,515
//	p0209:20  Fund Balances detail, five funds     26,294,515
//	p0061:39  Table 1 Total Uses, + 612,000 to CIP 26,906,515
func TestP0067IsTheOutlierAndFivePagesDisagree(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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
	// p0061 prints the same quantity plus the 612,000 transfer to the CIP, which
	// is why it corroborates rather than repeats.
	if !strings.Contains(page(61), "26,906,515") {
		t.Errorf("p0061 no longer prints 26,906,515 (= %s + 612,000 to CIP)", agreed)
	}
	// And p0067 prints NONE of the agreed figure, which is the whole claim:
	// the disagreement is a cell, not a rounding.
	if strings.Contains(page(67), agreed) {
		t.Errorf("p0067 prints %s after all; the exception says it does not", agreed)
	}

	// p0183's figure is not one number but five, and summing the printed
	// per-fund totals is what makes it independent evidence rather than a
	// second copy of the same subtotal. 16,546,010 -- the Services & Supplies
	// column those five funds add up to, and the 250,000 difference from
	// p0067's printed 16,796,010 -- is NOT printed anywhere, which is why it is
	// derived here and cited nowhere as a page figure.
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
	// And the same page's Grand Total is what the departmentwide pages sum to,
	// which is the corroboration that matters most: two schedules built on
	// different axes closing on one number.
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

// TestTheScopeIsWhatStopsTheDoubling is the measured form of the argument the
// rule file makes in words, and it is what the cut model refuses at the grain
// rather than at the sum.
//
// Re-scoping these rules to all-funds-gross is a one-word edit in the YAML. What
// it produces is not an error: internal/project's netCells has no refusal for a
// fact carrying a fund the way it does for one carrying a department, so the
// facts flow into the spine's own cells and the city's expenditure doubles.
// The cut check is what sees it, and it sees it BEFORE any cell is summed: the
// spine cut declares fund-group-by-category, the moved facts put a second
// grain under its scope, and structure.ValidateCuts refuses the cut by name as
// selecting facts at no single level. Measured off the same run, the cells the
// comparison would then have reported are named too: the moved facts land on
// the spine under their own placeholder category and every real cut is
// one-sided against it.
func TestTheScopeIsWhatStopsTheDoubling(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := *base
	s.Facts = append([]fact.Fact(nil), base.Facts...)
	moved := 0
	for i := range s.Facts {
		if s.Facts[i].Scope == fundingSourcesScope {
			s.Facts[i].Scope = spineScope
			moved++
		}
	}
	if moved != 312 {
		t.Fatalf("moved %d facts, want 312", moved)
	}

	res, err := (&cutsTieAlongTheLattice{}).Run(t.Context(), &s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("with pp.85-125's lower block re-scoped to the spine the cut check reported "+
			"%s, not fail: %s", res.Status, res.Summary)
	}
	details := findingDetails(res)
	for _, want := range []string{`cut "spine"`, "no single level"} {
		if !strings.Contains(details, want) {
			t.Errorf("no finding says %s; the refusal should name the cut and its grain:\n%s",
				want, details)
		}
	}
	// And the doubling it would otherwise publish, measured on the same facts:
	// the spine's General Fund FY2026 expenditure is exactly twice p66's.
	var general amount.Cents
	for i := range s.Facts {
		f := &s.Facts[i]
		if f.Scope == spineScope && f.Kind == mapping.KindExpenditure &&
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
