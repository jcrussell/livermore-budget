package check

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheCommittedCutsTieAlongTheLattice pins, by name, which pairs the
// committed corpus compares and at which grain, and why the others are refused:
// a comparison turned into a refusal leaves the check green with one fewer.
func TestTheCommittedCutsTieAlongTheLattice(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res := resultFor(t, runOne(t, s, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
	if res.Status != StatusPass {
		t.Fatalf("status = %s, findings:\n  %v", res.Status, res.Findings)
	}
	for _, want := range []string{
		"20 comparison(s) of 17 cut(s)",
		"departmentwide ~ funding-sources at department: 39 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		"general-fund-departments ~ funding-sources at department: 42 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		"revenue-detail -> spine at fund-group-by-category",
		"transfers-detail -> spine at fund-group-by-category",
		"general-fund-departments -> spine at fund-group-by-category",
		// pp.172-183 meet the spine in its two columns, and p172's General Fund
		// meets pp.167-170 in all four.
		"general-fund-by-category -> spine at fund-group-by-category: 8 cells over FY2026 adopted, FY2027 adopted, 0 one-sided at zero",
		"fund-expenditures -> spine at fund-group-by-category: 41 cells over FY2026 adopted, FY2027 adopted, 10 one-sided at zero, 1 held apart",
		"general-fund-departments -> general-fund-by-category at fund-by-category: 15 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted, 0 one-sided at zero, 1 held apart",
		"departmentwide ~ spine at category",
		"funding-sources ~ spine at fund-group",
		// The split takes transfers out, so p76 meets the spine on transfers in.
		"transfers-detail -> spine at fund-group-by-category: 14 cells over FY2026 adopted, FY2027 adopted, 6 one-sided at zero",
		"a-transfer-out-is-p76-or-to-the-cip -> spine at fund-group: 12 cells over FY2026 adopted, FY2027 adopted, 2 one-sided at zero",
		"1 pair(s) held only by a split",
		"cip-funds outside the reference, its funds carried by no other cut, and compared with none",
		// pp.186-209 meet the spine through the split on transfers out, and
		// as a containment on transfers in and balances; their two totals
		// cuts meet it at the fund group, and every per-fund schedule at the
		// fund.
		"fund-balance-flows -> spine at fund-group-by-category: 47 cells over FY2026 adopted, FY2027 adopted",
		"a-fund-transfers-out-or-to-the-cip -> spine at fund-group: 14 cells over FY2026 adopted, FY2027 adopted, 2 one-sided at zero",
		"fund-balance-revenues ~ spine at fund-group: 14 cells over FY2026 adopted, FY2027 adopted",
		"fund-balance-expenses ~ spine at fund-group: 13 cells over FY2026 adopted, FY2027 adopted",
		"revenue-detail -> fund-balance-revenues at fund: 298 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		"general-fund-departments -> fund-balance-expenses at fund: 4 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		"general-fund-by-category -> fund-balance-expenses at fund: 3 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		"fund-expenditures -> fund-balance-expenses at fund: 291 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		"funding-sources -> fund-balance-expenses at fund: 298 cells over FY2024 actual, FY2025 revised, FY2026 adopted, FY2027 adopted",
		// 136 pairs of 17 cuts: 16 compared, 16 with the outside cut, 1 held
		// only by a split, 2 held by a declared tie, and these.
		"101 pair(s) no comparison or tie relates",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary does not say %q", want)
		}
	}
	for _, absent := range []string{"carry no fact", "No rule file is loaded"} {
		if strings.Contains(res.Summary, absent) {
			t.Errorf("summary says %q over the committed corpus", absent)
		}
	}

	cut := map[string]structure.Cut{}
	for _, c := range structure.AllCuts() {
		cut[c.Name] = c
	}
	for _, tc := range []struct{ a, b, want string }{
		{"revenue-detail", "transfers-detail", `both are at "fund-by-category"`},
		{"general-fund-departments", "funding-sources", `"general-fund-departments" names departments at the "division" tier and "funding-sources" at the "department" tier`},
		{"general-fund-departments", "departmentwide", `"departmentwide" is at "department-by-category", which carries no fund group axis`},
		{"general-fund-departments", "transfers-detail", "no money is described by both"},
		{"revenue-detail", "departmentwide", "neither is the reference"},
		{"funding-sources", "acfr-changes-in-fund-balances", "share no common coarsening"},
		// The ACFR against the Budget Book: a lattice containment with no
		// column in common, refused by the declared bases.
		{"revenue-detail", "acfr-general-fund-summary", `"revenue-detail" prints [actual revised adopted] columns and "acfr-general-fund-summary" prints [audited]; there is no column both print`},
	} {
		_, err := structure.Compare(s.Facts, cut[tc.a], cut[tc.b])
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Compare(%s, %s) = %v, want %q", tc.a, tc.b, err, tc.want)
		}
	}
}

// TestTheCutsCheckGoesRed runs the mutations through the registered check, so
// what is proven is the line fisc verify prints.
func TestTheCutsCheckGoesRed(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	facts := s.Facts

	run := func(t *testing.T, facts []fact.Fact) Result {
		t.Helper()
		mutated := *s
		mutated.Facts = facts
		return resultFor(t, runOne(t, &mutated, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
	}

	t.Run("a figure that moves is named with its difference", func(t *testing.T) {
		planted := make([]fact.Fact, len(facts))
		copy(planted, facts)
		moved := 0
		for i := range planted {
			f := &planted[i]
			if f.Scope == "revenue-by-fund" && f.FiscalYear == 2026 && string(f.Basis) == "adopted" &&
				f.Category == "taxes/property" && f.FundGroup == "general" {
				f.AmountCents += 100
				moved++
				break
			}
		}
		if moved == 0 {
			t.Fatal("no General Fund property tax row to move")
		}
		res := run(t, planted)
		// Once against the spine by category, once against pp.186-209's
		// General Fund Revenues.
		if res.Status != StatusFail || len(res.Findings) != 2 {
			t.Fatalf("status %s with %d findings, want two failures:\n  %v", res.Status, len(res.Findings), res.Findings)
		}
		if !strings.Contains(res.Findings[0].Detail, "taxes/property") || !strings.Contains(res.Findings[0].Detail, "$1.00") {
			t.Errorf("the finding names neither the cell nor the difference: %s", res.Findings[0].Detail)
		}
		if res.Findings[1].Subject != "revenue-detail -> fund-balance-revenues" ||
			!strings.Contains(res.Findings[1].Detail, "fund=100") || !strings.Contains(res.Findings[1].Detail, "$1.00") {
			t.Errorf("the second finding is not the General Fund's total on p198: %v", res.Findings[1])
		}
	})

	t.Run("a row an exception says the reference prints, deleted, refuses the exception", func(t *testing.T) {
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			f := &facts[i]
			if f.Scope == "all-funds-gross" && f.FiscalYear == 2026 && string(f.Basis) == "adopted" &&
				f.FundGroup == "general" && f.Category == "transfers/in" {
				continue
			}
			kept = append(kept, facts[i])
		}
		res := run(t, kept)
		if res.Status != StatusFail {
			t.Fatalf("status = %s, want a failure", res.Status)
		}
		var named bool
		for _, f := range res.Findings {
			if strings.Contains(f.Detail, "pp.127-130-print-no-general-fund-transfer-in-2026") &&
				strings.Contains(f.Detail, "excuses nothing") {
				named = true
			}
		}
		if !named {
			t.Errorf("no finding refuses the exception by name:\n  %v", res.Findings)
		}
	})

	t.Run("a whole schedule dropped is red on its rules' grain, not compared as nothing", func(t *testing.T) {
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			if facts[i].Scope != "departmentwide-expenditures" {
				kept = append(kept, facts[i])
			}
		}
		res := run(t, kept)
		// The empty cut is skipped, and the grain arm refuses its rules by name.
		if diff := cmp.Diff(StatusFail, res.Status); diff != "" {
			t.Fatalf("status (-want +got):\n%s", diff)
		}
		if len(res.Findings) != 1 || res.Findings[0].Subject != "grain" ||
			!strings.Contains(res.Findings[0].Detail, "dw-") ||
			!strings.Contains(res.Findings[0].Detail, "no fact") {
			t.Errorf("want exactly the grain arm's refusal naming a dw- rule, got:\n  %v", res.Findings)
		}
		// An exception on the empty cut was never consulted, and is not refused.
		for _, f := range res.Findings {
			if strings.Contains(f.Detail, "by-object") {
				t.Errorf("an exception on an empty cut was reported: %s", f.Detail)
			}
		}
	})

	// A rule deleted with its facts is invisible to the grain arm; only the cut
	// still naming it is left, and its scope's sibling still has facts, so
	// emptiness judged by scope would call it populated.
	t.Run("a cut naming a rule that left the mapping with its facts is refused, not counted as compared", func(t *testing.T) {
		const rule = "acfr-p0167-general-fund-balances"
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			if facts[i].RuleID != rule {
				kept = append(kept, facts[i])
			}
		}
		if len(kept) == len(facts) {
			t.Fatalf("the store carries no fact of %s", rule)
		}
		files := make([]*mapping.File, 0, len(s.Files))
		for _, f := range s.Files {
			c := *f
			c.Rules = nil
			for _, r := range f.Rules {
				if r.ID != rule {
					c.Rules = append(c.Rules, r)
				}
			}
			files = append(files, &c)
		}
		mutated := *s
		mutated.Facts = kept
		mutated.Files = files
		res := resultFor(t, runOne(t, &mutated, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
		if diff := cmp.Diff(StatusFail, res.Status); diff != "" {
			t.Fatalf("status (-want +got):\n%s", diff)
		}
		// Exactly one finding: with the cuts arm absent the check is green.
		if len(res.Findings) != 1 || res.Findings[0].Subject != "cuts" ||
			!strings.Contains(res.Findings[0].Detail, `"acfr-fund-balances/general" selects rule "`+rule+`"`) ||
			!strings.Contains(res.Findings[0].Detail, "produces no fact") {
			t.Errorf("want exactly the cuts arm refusing the cut by the rule it names, got:\n  %v", res.Findings)
		}
	})
}

// runOne runs a single check over a subject.
func runOne(t *testing.T, s *Subject, c Check) *Report {
	t.Helper()
	return Run(t.Context(), s, []Check{c}, ReportOptions{GeneratedBy: testVersion})
}

// TestTheCutsCheckHoldsTheStoreToEveryCut plants what each arm beyond the
// comparison exists for.
func TestTheCutsCheckHoldsTheStoreToEveryCut(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	subjectsNamed := func(res Result, subject, want string) bool {
		for _, f := range res.Findings {
			if f.Subject == subject && strings.Contains(f.Detail, want) {
				return true
			}
		}
		return false
	}

	t.Run("a cut whose every rule left the mapping with its facts is refused", func(t *testing.T) {
		const prefix = "acfr-p0041-"
		mutated := *s
		mutated.Facts = nil
		for i := range s.Facts {
			if !strings.HasPrefix(s.Facts[i].RuleID, prefix) {
				mutated.Facts = append(mutated.Facts, s.Facts[i])
			}
		}
		if len(mutated.Facts) == len(s.Facts) {
			t.Fatalf("the store carries no fact of a %s rule", prefix)
		}
		mutated.Files = nil
		for _, f := range s.Files {
			c := *f
			c.Rules = nil
			for _, r := range f.Rules {
				if !strings.HasPrefix(r.ID, prefix) {
					c.Rules = append(c.Rules, r)
				}
			}
			mutated.Files = append(mutated.Files, &c)
		}
		res := resultFor(t, runOne(t, &mutated, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
		if res.Status != StatusFail || !subjectsNamed(res, "cuts", `cut "acfr-general-fund-summary" carries no fact`) {
			t.Fatalf("status %s, want the empty cut refused by name:\n  %v", res.Status, res.Findings)
		}
	})

	for _, tc := range []struct{ scope, from, to, cut string }{
		{"department-funding-sources", "fire-department", "fire-administration", "funding-sources"},
		{"departmentwide-expenditures", "patrol", "police-department", "departmentwide"},
	} {
		t.Run(tc.from+" read as "+tc.to+" in "+tc.cut+" is refused at the cut's tier", func(t *testing.T) {
			mutated := *s
			mutated.Facts = make([]fact.Fact, len(s.Facts))
			copy(mutated.Facts, s.Facts)
			moved := 0
			for i := range mutated.Facts {
				f := &mutated.Facts[i]
				if f.Scope == tc.scope && f.Department == tc.from {
					f.Department = tc.to
					moved++
				}
			}
			if moved == 0 {
				t.Fatalf("no %s fact in %s", tc.from, tc.scope)
			}
			res := resultFor(t, runOne(t, &mutated, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
			if !subjectsNamed(res, "tier", fmt.Sprintf(`cut %q names departments`, tc.cut)) ||
				!subjectsNamed(res, "tier", fmt.Sprintf("%q", tc.to)) {
				t.Fatalf("status %s, want the tier arm naming %s in %s:\n  %v", res.Status, tc.to, tc.cut, res.Findings)
			}
		})
	}

	t.Run("an exception ValidateExceptions refuses is reported", func(t *testing.T) {
		withExceptions(t, append(structure.BudgetBookExceptions(), structure.Exception{
			Name: "invalid", Cut: "revenue-detail", Against: "spine", At: structure.LevelFundGroupByCategory,
		}))
		res := resultFor(t, runOne(t, s, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
		if !subjectsNamed(res, "exceptions", `exception "invalid"`) {
			t.Fatalf("status %s, want the exceptions arm:\n  %v", res.Status, res.Findings)
		}
	})

	t.Run("an exception on a pair no comparison relates is refused as inert", func(t *testing.T) {
		pin := structure.Pin{Year: 2026, Basis: "adopted",
			Coords: map[structure.Axis]string{structure.AxisCategory: "transfers/in"},
			Cut:    structure.Sum{Cents: 1, Present: true}, Against: structure.Sum{Cents: 2, Present: true}}
		withExceptions(t, append(structure.BudgetBookExceptions(), structure.Exception{
			Name: "inert", Cut: "revenue-detail", Against: "departmentwide", At: structure.LevelCategory,
			Cells: []structure.Pin{pin}, Residual: 1, Printed: "nowhere", Reason: "a plant", Bead: "none",
		}))
		res := resultFor(t, runOne(t, s, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
		if len(res.Findings) != 1 || res.Findings[0].Subject != "inert" ||
			!strings.Contains(res.Findings[0].Detail, "no comparison relates that pair") {
			t.Fatalf("status %s, want exactly the inert arm:\n  %v", res.Status, res.Findings)
		}
	})
}

// withExceptions declares exceptions in place of the tree's for one test.
func withExceptions(t *testing.T, exceptions []structure.Exception) {
	t.Helper()
	prev := budgetBookExceptions
	budgetBookExceptions = func() []structure.Exception { return exceptions }
	t.Cleanup(func() { budgetBookExceptions = prev })
}

// TestTheDepartmentSchedulesTieInEveryColumn plants in the actual and revised
// columns, which the spine does not print and only the ties reach.
func TestTheDepartmentSchedulesTieInEveryColumn(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, tc := range []struct {
		name, pair, cell string
		plant            func(facts []fact.Fact) []fact.Fact
	}{
		{"funding-city-council's actual and revised columns swapped", "departmentwide ~ funding-sources",
			"FY2024 actual department[department=city-council]", func(facts []fact.Fact) []fact.Fact {
				for i := range facts {
					f := &facts[i]
					if f.RuleID == "funding-city-council" && f.FiscalYear == 2024 {
						f.FiscalYear, f.Basis = 2025, mapping.BasisRevised
					} else if f.RuleID == "funding-city-council" && f.FiscalYear == 2025 {
						f.FiscalYear, f.Basis = 2024, mapping.BasisActual
					}
				}
				return facts
			}},
		{"an FY2024 funding fact deleted", "departmentwide ~ funding-sources",
			"FY2024 actual department[department=city-council]", func(facts []fact.Fact) []fact.Fact {
				return slices.DeleteFunc(facts, func(f fact.Fact) bool {
					return f.RuleID == "funding-city-council" && f.FiscalYear == 2024
				})
			}},
		{"an FY2025 departmentwide fact moved to another department's division", "departmentwide ~ funding-sources",
			"FY2025 revised department[department=city-council]", func(facts []fact.Fact) []fact.Fact {
				for i := range facts {
					f := &facts[i]
					if f.Scope == "departmentwide-expenditures" && f.FiscalYear == 2025 &&
						f.Department == "city-council" && f.AmountCents != 0 {
						f.Department = "patrol"
						break
					}
				}
				return facts
			}},
		{"an FY2025 General Fund department fact deleted", "general-fund-departments ~ funding-sources",
			"FY2025 revised department[department=city-council]", func(facts []fact.Fact) []fact.Fact {
				return slices.DeleteFunc(facts, func(f fact.Fact) bool {
					return f.Scope == "expenditure-by-department" && f.FiscalYear == 2025 &&
						f.Department == "city-council" && f.AmountCents != 0
				})
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := *s
			mutated.Facts = tc.plant(slices.Clone(s.Facts))
			res := resultFor(t, runOne(t, &mutated, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
			for _, f := range res.Findings {
				if f.Subject == tc.pair && strings.Contains(f.Detail, tc.cell) {
					return
				}
			}
			t.Fatalf("status %s, want %s refusing %s:\n  %v", res.Status, tc.pair, tc.cell, res.Findings)
		})
	}
}

// TestASplitWithAnEmptySideLeavesItsPairsToTheLattice drops p222's facts: the
// split then holds nothing, so p76 is compared with the spine on transfers out
// again and the CIP's share is a finding rather than a pair compared by nothing.
func TestASplitWithAnEmptySideLeavesItsPairsToTheLattice(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	kept := s.Facts[:0:0]
	for _, f := range s.Facts {
		if f.Scope != "cip-funding-sources" {
			kept = append(kept, f)
		}
	}
	s.Facts = kept
	res, err := (&cutsTieAlongTheLattice{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var b strings.Builder
	for _, f := range res.Findings {
		b.WriteString(f.Subject + ": " + f.Detail + "\n")
	}
	for _, want := range []string{"transfers-detail -> spine", "category=transfers/out fund_group=capital"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no finding says %q:\n%s", want, b.String())
		}
	}
	if strings.Contains(res.Summary, "held only by a split") {
		t.Errorf("the summary still counts a pair held by a split that holds nothing:\n%s", res.Summary)
	}
}
