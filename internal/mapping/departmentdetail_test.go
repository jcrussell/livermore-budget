package mapping

import (
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// The department schedule's scope, and the four pages it covers.
const departmentDetailScope = "expenditure-by-department"

// departmentDetailPages is the schedule, and it is also the doc departmentDetail
// builds. A rollup on any other page cannot be resolved against that doc, so the
// rollup sweep below filters on it rather than on "every rollup in the file" —
// which was the same claim while one schedule was mapped and became a different
// one the moment fisc-5gk.1 added rollups on p130 and p140.
var departmentDetailPages = map[int]bool{167: true, 168: true, 169: true, 170: true}

// TestPublishedDepartmentDetailResolves is to pp.167-170 what
// TestPublishedSpineResolves is to pp.66-67: every part of every rule resolves
// against the real pages, and every printed total the document offers is
// checked against what we read.
//
// The counts are the assertion. 23 division rules over 26 parts -- three
// divisions straddle a page break -- 49 object rows, 196 values, and twelve
// printed totals that cover more than one line of rules. A rule dropped or a
// row missed moves one of these numbers.
func TestPublishedDepartmentDetailResolves(t *testing.T) {
	f, r := departmentDetail(t)

	rules, parts, values := 0, 0, 0
	tied := 0
	for i := range f.Rules {
		ru := &f.Rules[i]
		if ru.Scope != departmentDetailScope {
			continue
		}
		rules++
		if ru.TotalRow == "" {
			t.Errorf("rule %s declares no total_row; every division prints its own Total, "+
				"and a rollup can only sum totals its covered rules print", ru.ID)
		}
		for j := range ru.Parts {
			p := &ru.Parts[j]
			parts++
			// Every part states a stop_at. A part whose rule prints a total
			// needs one -- StatedTotals searches the text AFTER the block --
			// and a part without one takes the running footer with it.
			if p.StopAt == "" {
				t.Errorf("rule %s p%d declares no stop_at", ru.ID, p.Page)
			}
			vals, _, err := r.Values(ru, p)
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, p.Page, err)
			}
			values += len(vals)
			for _, v := range vals {
				if v.Row.Department == "" {
					t.Errorf("rule %s row %q carries no department; the axis is the whole "+
						"reason this schedule is mapped", ru.ID, v.Row.Label)
				}
				// fisc-brx: without fund_group on every column, ColumnPath
				// falls back to the rule's scope and the reconciliation has
				// nothing to key on.
				if v.Column.FundGroup != "general" {
					t.Errorf("rule %s p%d column %d declares fund_group %q, want general",
						ru.ID, p.Page, v.ColumnIndex+1, v.Column.FundGroup)
				}
			}
		}
		if ru.TotalSpansParts {
			res, err := r.CheckSpanningTotals(ru)
			if err != nil {
				t.Errorf("CheckSpanningTotals(%s): %v", ru.ID, err)
				continue
			}
			tied += res.Columns
			continue
		}
		res, err := r.CheckTotals(ru, &ru.Parts[0])
		if err != nil {
			t.Errorf("CheckTotals(%s): %v", ru.ID, err)
			continue
		}
		tied += res.Columns
	}

	for _, tc := range []struct {
		name      string
		got, want int
	}{
		{"division rules", rules, 23},
		{"parts", parts, 26},
		{"values", values, 196},
		{"columns tied to a printed division total", tied, 92},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}

	// The twelve printed totals that cover more than one line of rules: eleven
	// <DEPARTMENT> TOTAL rows and the schedule's own closing total. Six of the
	// eleven cover a SINGLE division, which the parser refused until this lane.
	rollups, single := 0, 0
	for i := range f.Rollups {
		ro := &f.Rollups[i]
		if !departmentDetailPages[ro.Page] {
			continue
		}
		if ro.Unassertable != "" {
			t.Errorf("rollup %s is declared unassertable; every printed total on these "+
				"pages can be asserted", ro.ID)
			continue
		}
		res, err := r.CheckRollup(ro)
		if err != nil {
			t.Errorf("CheckRollup(%s): %v", ro.ID, err)
			continue
		}
		rollups++
		if res.Rules == 1 {
			single++
		}
	}
	if rollups != 12 {
		t.Errorf("rollups asserted = %d, want 12", rollups)
	}
	if single != 6 {
		t.Errorf("rollups covering one rule = %d, want 6 (City Council, City Attorney, "+
			"Library, Innovation, General Services, Fire)", single)
	}
}

// TestDepartmentDetailTiesToTheSpineOffThePages is the claim the scope exists
// for, with BOTH SIDES READ FROM THE DOCUMENT.
//
// No figure is typed into this test. pp.167-170's 49 object rows are summed per
// object category, pp.66-67's General Fund expenditure block is read by its own
// rules, and the two are compared. That is the same arithmetic
// cuts-tie-along-the-lattice does over the fact store, done here against the
// pages themselves -- so the check and the corpus cannot drift together.
func TestDepartmentDetailTiesToTheSpineOffThePages(t *testing.T) {
	detail := detailByCategory(t)
	spine := spineGeneralFundExpenditure(t)

	// Only the two budget columns are reconciled: pp.66-67 print no actual or
	// revised column, so the detail's FY2024 and FY2025 figures are published
	// and have nothing to tie to. Naming that here is the point -- it is a gap
	// in the DOCUMENT, not one in the mapping.
	for _, year := range []int{2026, 2027} {
		for _, category := range []string{
			"wages-and-benefits", "services-and-supplies", "capital-outlay", "debt-services",
		} {
			k := catYear{category, year}
			d, s := detail[k], spine[k]
			if d != s {
				t.Errorf("FY%d %s: pp.167-170 sum to %s and pp.66-67 print %s; these are "+
					"the same money decomposed two ways", year, category, d, s)
			}
		}
	}

	// The zero cells are real ties and not vacancies: p66 prints "-" for
	// General Fund capital outlay and debt services in both budget years, and
	// pp.167-170 print "-" in the same categories in the same years while
	// printing 66,528 and 18,800 in FY2023-24. Asserting the FY2024 figures is
	// what stops "both sides are zero" from passing over two empty reads.
	for _, tc := range []struct {
		category string
		want     amount.Cents
	}{
		{"capital-outlay", dollars(66_528)},
		{"debt-services", dollars(18_800)},
	} {
		if got := detail[catYear{tc.category, 2024}]; got != tc.want {
			t.Errorf("FY2024 %s = %s, want %s; the budget years tie at zero and this is "+
				"what proves the rows are being read at all", tc.category, got, tc.want)
		}
		if got := detail[catYear{tc.category, 2026}]; got != 0 {
			t.Errorf("FY2026 %s = %s, want 0", tc.category, got)
		}
	}
}

// TestDroppingADivisionRuleFails proves the tie is failable in the way it exists
// to be, and it deletes the RULE rather than mutating a value.
//
// A missing rule is the failure the union-of-keys reconciliation is written for,
// and a fact-slice mutation would not exercise it: the rule file is where a
// division goes missing.
func TestDroppingADivisionRuleFails(t *testing.T) {
	src, err := os.ReadFile(publishedSpine)
	if err != nil {
		t.Fatal(err)
	}
	// Patrol is the largest division on the pages, so its absence cannot be
	// mistaken for rounding.
	cut := removeRule(t, string(src), "div-patrol")
	f, err := parse(strings.NewReader(cut), publishedSpine)
	if err == nil {
		// The rollup that covers it should refuse first: POLICE DEPARTMENT
		// TOTAL names a rule that is no longer there.
		t.Fatal("a file with div-patrol removed parsed; POLICE DEPARTMENT TOTAL covers it " +
			"and must not resolve without it")
	}
	if !strings.Contains(err.Error(), "div-patrol") {
		t.Errorf("error = %v, want it to name the missing rule", err)
	}
	_ = f
}

type catYear struct {
	category string
	year     int
}

// departmentDetail loads the published file over the four real pages.
func departmentDetail(t *testing.T) (*File, *Resolver) {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 167, 168, 169, 170), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// detailByCategory sums pp.167-170's object rows per (category, fiscal year).
func detailByCategory(t *testing.T) map[catYear]amount.Cents {
	t.Helper()
	f, r := departmentDetail(t)
	out := map[catYear]amount.Cents{}
	for i := range f.Rules {
		ru := &f.Rules[i]
		if ru.Scope != departmentDetailScope {
			continue
		}
		for j := range ru.Parts {
			vals, _, err := r.Values(ru, &ru.Parts[j])
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, ru.Parts[j].Page, err)
			}
			for _, v := range vals {
				out[catYear{v.Row.Category, v.Column.FiscalYear}] += v.Cents
			}
		}
	}
	return out
}

// spineGeneralFundExpenditure reads pp.66-67's General Fund expenditure block
// through its own rules, so the comparison is page against page.
func spineGeneralFundExpenditure(t *testing.T) map[catYear]amount.Cents {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 66, 67), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	out := map[catYear]amount.Cents{}
	for i := range f.Rules {
		ru := &f.Rules[i]
		if ru.Scope != publishedSpineScope || ru.Kind != KindExpenditure {
			continue
		}
		for j := range ru.Parts {
			vals, _, err := r.Values(ru, &ru.Parts[j])
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, ru.Parts[j].Page, err)
			}
			for _, v := range vals {
				if v.Column.FundGroup != "general" {
					continue
				}
				out[catYear{v.Row.Category, v.Column.FiscalYear}] += v.Cents
			}
		}
	}
	return out
}

// removeRule cuts one rule out of the YAML source, by id, keeping the rest
// byte-identical. It is deliberately textual: the point is to delete what an
// author would delete.
func removeRule(t *testing.T, src, id string) string {
	t.Helper()
	start := strings.Index(src, "  - id: "+id+"\n")
	if start < 0 {
		t.Fatalf("no rule %q in the published file", id)
	}
	rest := src[start+1:]
	next := strings.Index(rest, "\n  - id: ")
	if next < 0 {
		t.Fatalf("rule %q is the last in the file; this helper needs a following rule", id)
	}
	return src[:start] + rest[next+1:]
}
