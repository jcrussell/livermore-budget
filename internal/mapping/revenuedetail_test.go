package mapping

import (
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// revenueDetailScope is the scope Budget Book pp.127-140 are mapped at (fisc-u2v).
const revenueDetailScope = "revenue-by-fund"

// revenueDetailPages is the schedule: pp.127-130 are the General Fund and
// pp.131-140 the other five fund groups. They are ONE scope because they cannot
// double-count each other — the General Fund has no section on pp.131-140 and
// pp.127-130 carry no other fund, so they are disjoint BY FUND rather than by
// convention — and because only together do they reproduce all six of the
// spine's columns.
var revenueDetailPages = []int{127, 128, 129, 130, 131, 132, 133, 134, 135, 136,
	137, 138, 139, 140}

// groupCatYear is the tuple the reconciliation keys on, and it is deliberately
// internal/project's cellKey minus the slice: the check this test stands behind
// asserts equality on precisely the tuple a doubling would have collided on.
type groupCatYear struct {
	group    string
	category string
	year     int
}

func revenueDetail(t *testing.T) (*File, *Resolver) {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, revenueDetailPages...), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return f, r
}

// readRevenueDetail sums pp.127-140 per (fund group, category, fiscal year),
// from a rule file the caller supplies so a mutation test can pass a damaged one.
func readRevenueDetail(t *testing.T, f *File, r *Resolver) map[groupCatYear]amount.Cents {
	t.Helper()
	out := map[groupCatYear]amount.Cents{}
	for i := range f.Rules {
		ru := &f.Rules[i]
		if ru.Scope != revenueDetailScope {
			continue
		}
		for j := range ru.Parts {
			vals, _, err := r.Values(ru, &ru.Parts[j])
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, ru.Parts[j].Page, err)
			}
			for _, v := range vals {
				out[groupCatYear{v.Column.FundGroup, v.Row.Category, v.Column.FiscalYear}] += v.Cents
			}
		}
	}
	return out
}

// readSpineRevenue reads pp.66-67's revenue and transfer-in blocks through their
// own rules, so the comparison is page against page with no figure typed in.
func readSpineRevenue(t *testing.T) map[groupCatYear]amount.Cents {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, 66, 67), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	out := map[groupCatYear]amount.Cents{}
	for i := range f.Rules {
		ru := &f.Rules[i]
		if ru.Scope != publishedSpineScope {
			continue
		}
		if ru.Kind != KindRevenue && ru.Kind != KindTransferIn {
			continue
		}
		for j := range ru.Parts {
			vals, _, err := r.Values(ru, &ru.Parts[j])
			if err != nil {
				t.Fatalf("Values(%s, p%d): %v", ru.ID, ru.Parts[j].Page, err)
			}
			for _, v := range vals {
				out[groupCatYear{v.Column.FundGroup, v.Row.Category, v.Column.FiscalYear}] += v.Cents
			}
		}
	}
	return out
}

// TestRevenueDetailTiesToTheSpineOffThePages is the same arithmetic
// revenue-detail-ties-to-spine does, done against the DOCUMENTS rather than
// against the fact store.
//
// It exists so the check and the corpus cannot drift together. The check reads
// Subject.Facts, which `fisc build` produced from these same rules; if the rules
// were wrong, both would be wrong in the same direction and the check would
// still be green. This test reads pp.127-140 and pp.66-67 through their own
// rules and compares the two, with no figure typed in.
//
// THE ONE DECLARED EXCEPTION IS ASSERTED, NOT SKIPPED. pp.127-130 print no
// General Fund Transfers In row, so (general, transfers/in) is spine-only. The
// test states the amount the spine publishes there, because an exception nobody
// measures is an exception that can quietly grow.
func TestRevenueDetailTiesToTheSpineOffThePages(t *testing.T) {
	f, r := revenueDetail(t)
	detail := readRevenueDetail(t, f, r)
	spine := readSpineRevenue(t)

	// The spine decides. pp.66-67 print only the two adopted years, so the
	// schedule's FY2024 actual and FY2025 revised columns have no counterpart
	// and are reconciled by nothing here — a gap in the DOCUMENT.
	years := map[int]bool{}
	for k := range spine {
		years[k.year] = true
	}
	if len(years) != 2 || !years[2026] || !years[2027] {
		t.Fatalf("the spine publishes years %v, want exactly FY2026 and FY2027; this test's "+
			"shape assumes pp.66-67 print no actual or revised column", years)
	}

	// The exception, by name, and only this one.
	const exceptCategory, exceptGroup = "transfers/in", "general"
	exceptTotal := amount.Cents(0)

	// The union, in both directions. A key on one side only must be zero on
	// that side; a key on both must be equal.
	seen := map[groupCatYear]bool{}
	for k := range detail {
		seen[k] = true
	}
	for k := range spine {
		seen[k] = true
	}
	compared, oneSided := 0, 0
	for k := range seen {
		if !years[k.year] {
			continue
		}
		if k.category == exceptCategory && k.group == exceptGroup {
			exceptTotal += spine[k]
			if got := detail[k]; got != 0 {
				t.Errorf("FY%d %s %s: pp.127-140 publish %s, so the declared exception is "+
					"a false claim about the schedule", k.year, k.group, k.category, got)
			}
			continue
		}
		compared++
		d, sp := detail[k], spine[k]
		if _, inDetail := detail[k]; !inDetail {
			oneSided++
		} else if _, inSpine := spine[k]; !inSpine {
			oneSided++
		}
		if d != sp {
			t.Errorf("FY%d %s %s: pp.127-140 sum to %s and pp.66-67 print %s, a difference "+
				"of %s", k.year, k.group, k.category, d, sp, d-sp)
		}
	}

	// A zero here would mean the loop above compared nothing and the test is a
	// decoration; six groups over eleven categories over two years is the size
	// this schedule is.
	if compared < 60 {
		t.Errorf("compared %d cells, want at least 60; this schedule crosses six fund "+
			"groups against eleven revenue categories in two years", compared)
	}
	if oneSided == 0 {
		t.Error("no key is one-sided, so the union asserted nothing the intersection " +
			"would not have; p67 prints a dash for taxes/property under capital and " +
			"pp.131-140 print no such row")
	}

	// p76 prints what pp.127-130 omit: 480,400 and 486,735, which is
	// p0066.txt:24 to the cent. Naming it here is what keeps the exemption from
	// growing quietly into "General Fund transfers are not reconciled".
	if want := amount.Cents(48_040_000 + 48_673_500); exceptTotal != want {
		t.Errorf("the exempted General Fund transfers in total %s over both budget years, "+
			"want %s; the one declared exception changed size", exceptTotal, want)
	}
}

// TestRevenueDetailReproducesEverySpineFundGroup states the claim fisc-u2v's
// decision rests on, per fund group rather than per cell: pp.127-140 are a
// COMPLETE alternative decomposition of the spine's revenue side, not a sample
// of it.
//
// The per-cell test above would pass over a schedule that had lost a whole fund
// group, provided the spine had lost it too. This one names the six.
func TestRevenueDetailReproducesEverySpineFundGroup(t *testing.T) {
	f, r := revenueDetail(t)
	detail := readRevenueDetail(t, f, r)
	spine := readSpineRevenue(t)

	groups := []string{"general", "enterprise", "capital", "debt-service",
		"special-revenue", "internal-service"}
	for _, year := range []int{2026, 2027} {
		for _, g := range groups {
			var d, sp amount.Cents
			for k, v := range detail {
				if k.group == g && k.year == year {
					d += v
				}
			}
			for k, v := range spine {
				if k.group == g && k.year == year {
					sp += v
				}
			}
			// The General Fund is short by exactly the Transfers In row
			// pp.127-130 do not print; every other group ties outright.
			if g == "general" {
				sp -= spine[groupCatYear{"general", "transfers/in", year}]
			}
			if d != sp {
				t.Errorf("FY%d %s: pp.127-140 sum to %s, pp.66-67 print %s, difference %s",
					year, g, d, sp, d-sp)
			}
			if d == 0 && g != "debt-service" {
				t.Errorf("FY%d %s sums to zero; only debt service does that", year, g)
			}
		}
	}
}

// TestPermanentFundsHaveNoSpineColumn keeps this schedule evidencing fisc-u8o.
//
// p134 prints a Permanent Funds section that pp.66-67 have no column for. The
// two schedules can only agree while it is ZERO in the reconciled years — which
// it is, and is NOT in FY2023-24. Asserting both halves is what makes this a
// statement about the document rather than about an empty read: if the page
// stopped printing the section at all, the first assertion would still pass and
// the second would not.
func TestPermanentFundsHaveNoSpineColumn(t *testing.T) {
	f, r := revenueDetail(t)
	detail := readRevenueDetail(t, f, r)

	sum := func(year int) amount.Cents {
		var out amount.Cents
		for k, v := range detail {
			if k.group == "permanent" && k.year == year {
				out += v
			}
		}
		return out
	}
	for _, year := range []int{2026, 2027} {
		if got := sum(year); got != 0 {
			t.Errorf("FY%d permanent funds sum to %s; the spine has no column to hold it "+
				"and the six-group tie only survives while this is zero", year, got)
		}
	}
	if got := sum(2024); got == 0 {
		t.Error("FY2024 permanent funds sum to zero, so this schedule no longer evidences " +
			"fisc-u8o and the two budget-year zeroes above prove nothing")
	}
}

// TestDroppingARevenueRuleFails proves the reconciliation is failable, by doing
// what an author would actually do wrong: DELETE A RULE from the file.
//
// The mutation is textual and on the rule FILE rather than on a fact slice,
// because the failure being demonstrated is a missing rule. fisc-u2v DEFECT 2
// records that omitting a whole revenue block is the converse of the
// mistyped-scope incident, and that a loop over the detail's own keys would
// compare nothing and report green.
//
// TWO MECHANISMS CATCH IT AND THEY COVER DIFFERENT RULES, which is the thing
// worth stating: the ten General Fund rules are named by p130's rollup and go
// missing at PARSE time, while the sixty-nine fund rules are covered by no
// rollup at all — p140's Total Sources is unassertable (fisc-wev) — so nothing
// but the arithmetic notices them.
func TestDroppingARevenueRuleFails(t *testing.T) {
	src, err := os.ReadFile(publishedSpine)
	if err != nil {
		t.Fatalf("read published file: %v", err)
	}

	// A General Fund category: p130's `Total General Fund` rollup covers it by
	// id, so the file stops being loadable at all.
	t.Run("a rule a rollup covers fails at parse", func(t *testing.T) {
		cut := removeRule(t, string(src), "gf-rev-licenses-permits")
		if _, err := Parse(strings.NewReader(cut), publishedSpine); err == nil {
			t.Fatal("a file with gf-rev-licenses-permits removed parsed; gf-total-revenues " +
				"covers it and must not resolve without it")
		} else if !strings.Contains(err.Error(), "gf-rev-licenses-permits") {
			t.Errorf("error = %v, want it to name the missing rule", err)
		}
	})

	// A fund rule: no rollup names it, because the only printed total over the
	// 69 of them is p140's and that one reconciles against nothing. The file
	// parses, every remaining total still ties, and the ONLY thing that
	// notices is the tie against the spine.
	t.Run("a rule no rollup covers is caught only by the tie", func(t *testing.T) {
		cut := removeRule(t, string(src), "fund-rev-water")
		f, err := Parse(strings.NewReader(cut), publishedSpine)
		if err != nil {
			t.Fatalf("Parse: %v; no rollup covers fund-rev-water, so removing it must "+
				"leave a loadable file — that is the point of this case", err)
		}
		r, err := NewResolver(budgetDoc(t, revenueDetailPages...), f)
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		detail := readRevenueDetail(t, f, r)
		spine := readSpineRevenue(t)

		differs := false
		for k, sp := range spine {
			if k.year != 2026 || k.group != "enterprise" {
				continue
			}
			if detail[k] != sp {
				differs = true
			}
		}
		if !differs {
			t.Error("removing fund-rev-water left every FY2026 enterprise cell tying. " +
				"Nothing else in the pipeline sees this rule go missing: its own " +
				"printed total leaves with it, and no rollup names it")
		}
	})
}

// TestDroppingAFundZeroInBothBudgetYearsIsInvisibleToTheTie is the measurement
// behind rule-funds-match-their-headings' second clause, stated as a test so
// that clause cannot be simplified away.
//
// NINE of the schedule's 69 funds print zero in BOTH budget years. The
// reconciliation against the spine covers only those two years, because pp.66-67
// print no actual or revised column — so dropping one of the nine changes
// nothing it looks at, and up to $4.7M of FY2023-24 revenue leaves the corpus
// with every check green.
//
// This test asserts the BLIND SPOT, not a defect: the tie is correct, and it is
// correct that it cannot see this. What would be wrong is believing it could.
func TestDroppingAFundZeroInBothBudgetYearsIsInvisibleToTheTie(t *testing.T) {
	src, err := os.ReadFile(publishedSpine)
	if err != nil {
		t.Fatalf("read published file: %v", err)
	}
	// Transferable Development Cred: $4,723,774 in FY2023-24 and $7,055,955 in
	// FY2024-25, and a printed dash in both budget years.
	cut := removeRule(t, string(src), "fund-rev-transferable-development-cred")
	f, err := Parse(strings.NewReader(cut), publishedSpine)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(budgetDoc(t, revenueDetailPages...), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	full, fullR := revenueDetail(t)
	before := readRevenueDetail(t, full, fullR)
	after := readRevenueDetail(t, f, r)

	// The two reconciled years are untouched...
	for _, year := range []int{2026, 2027} {
		for k, v := range before {
			if k.year != year {
				continue
			}
			if after[k] != v {
				t.Errorf("FY%d %s %s moved from %s to %s; this fund was supposed to be "+
					"zero in both budget years", year, k.group, k.category, v, after[k])
			}
		}
	}
	// ...and FY2023-24 lost real money, which nothing above sees.
	var lost amount.Cents
	for k, v := range before {
		if k.year == 2024 {
			lost += v - after[k]
		}
	}
	if lost == 0 {
		t.Fatal("removing the rule cost nothing in FY2023-24 either, so this fund no " +
			"longer evidences the blind spot and the second clause of " +
			"rule-funds-match-their-headings needs a different example")
	}
	if want := amount.Cents(472_377_400); lost != want {
		t.Errorf("FY2023-24 lost %s, want %s (the fund's own printed total)", lost, want)
	}
}
