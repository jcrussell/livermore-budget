package check

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestCompareRefusesWhatTheLatticeCannotTie pins why the committed corpus's
// uncompared pairs are refused, each with structure.ErrNotComparable.
func TestCompareRefusesWhatTheLatticeCannotTie(t *testing.T) {
	t.Parallel()
	s := committed(t)
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
		if !errors.Is(err, structure.ErrNotComparable) {
			t.Errorf("Compare(%s, %s) = %v, which is not structure.ErrNotComparable", tc.a, tc.b, err)
		}
	}
}

// TestTheCutsCheckGoesRed runs the mutations through the registered check, so
// what is proven is the line fisc verify prints.
func TestTheCutsCheckGoesRed(t *testing.T) {
	t.Parallel()
	s := committed(t)
	facts := s.Facts

	run := func(t *testing.T, facts []fact.Fact) Result {
		t.Helper()
		mutated := *s
		mutated.Facts = facts
		return runCheck(t, &cutsTieAlongTheLattice{}, &mutated)
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

	// compareAt refuses a pair whose side holds a fact on no line it
	// declares, or whose other side prints one on a line it neither prints
	// nor leaves unprinted. Either refusal is the data's, so the check
	// fails on it rather than counting the pair among those it cannot relate.
	var flows structure.Cut
	for _, c := range structure.AllCuts() {
		if c.Name == structure.CutFundBalanceFlows {
			flows = c
		}
	}
	for _, tc := range []struct {
		name, scope string
		rules       []string
		want        string
	}{
		{"a fact a cut admits on no line it declares fails the pair", flows.Scope, flows.Rules,
			`is on no line "fund-balance-flows" declares`},
		{"a fact the other side prints on a line the cut does not declare fails the pair", structure.ScopeAllFundsGross, nil,
			`"fund-balance-flows" declares neither printed nor unprinted`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planted := slices.Clone(facts)
			moved := false
			for i := range planted {
				f := &planted[i]
				if f.Scope == tc.scope && (tc.rules == nil || slices.Contains(tc.rules, f.RuleID)) &&
					f.Kind == vocab.KindFundBalance && f.Category == structure.CategoryFundBalanceEnding {
					f.Category = "fund-balance/on-no-line"
					moved = true
					break
				}
			}
			if !moved {
				t.Fatal("no ending balance to move off its line")
			}
			// The pair's exceptions go too: with the pair refused they would
			// be inert, and that finding is not the one under test.
			mutated := *s
			mutated.Facts = planted
			mutated.Exceptions = slices.DeleteFunc(slices.Clone(s.Exceptions), func(e structure.Exception) bool {
				return e.Cut == structure.CutFundBalanceFlows && e.Against == structure.CutSpine
			})
			res := runCheck(t, &cutsTieAlongTheLattice{}, &mutated)
			if res.Status != StatusFail {
				t.Fatalf("status = %s, want a failure; summary: %s", res.Status, res.Summary)
			}
			if !hasFinding(res, "spine ~ fund-balance-flows: ", tc.want) {
				t.Errorf("no finding on spine ~ fund-balance-flows says %q:\n  %v", tc.want, res.Findings)
			}
		})
	}

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
		if !hasFinding(res, "pp.127-130-print-no-general-fund-transfer-in-2026", "excuses nothing") {
			t.Errorf("no finding refuses the exception by name:\n  %v", res.Findings)
		}
	})

	t.Run("a whole schedule dropped is red on its rules' grain and its residue, not compared as nothing", func(t *testing.T) {
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			if facts[i].Scope != "departmentwide-expenditures" {
				kept = append(kept, facts[i])
			}
		}
		res := run(t, kept)
		// The empty cut is skipped, the grain arm refuses its rules by name, and
		// the residue declared on the dropped scope is stale: a scope the store
		// no longer carries does not excuse its residue.
		if diff := cmp.Diff(StatusFail, res.Status); diff != "" {
			t.Fatalf("status (-want +got):\n%s", diff)
		}
		if len(res.Findings) != 2 || res.Findings[0].Subject != "grain" ||
			!strings.Contains(res.Findings[0].Detail, "dw-") ||
			!strings.Contains(res.Findings[0].Detail, "no fact") ||
			res.Findings[1].Subject != "coverage" ||
			!strings.Contains(res.Findings[1].Detail, "residue (departmentwide-expenditures, dw-maintenance, transfer_out) matches no fact") {
			t.Errorf("want the grain arm's refusal naming a dw- rule and the coverage arm's refusal "+
				"of dw-maintenance's residue, got:\n  %v", res.Findings)
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
		res := runCheck(t, &cutsTieAlongTheLattice{}, &mutated)
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

// TestTheCutsCheckHoldsTheStoreToEveryCut plants what each arm beyond the
// comparison exists for.
func TestTheCutsCheckHoldsTheStoreToEveryCut(t *testing.T) {
	t.Parallel()
	s := mutable(t)

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
		res := runCheck(t, &cutsTieAlongTheLattice{}, &mutated)
		if res.Status != StatusFail || !hasFinding(res, "cuts: ", `cut "acfr-general-fund-summary" carries no fact`) {
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
			res := runCheck(t, &cutsTieAlongTheLattice{}, &mutated)
			if !hasFinding(res, "tier: ", fmt.Sprintf(`cut %q names departments`, tc.cut)) ||
				!hasFinding(res, "tier: ", fmt.Sprintf("%q", tc.to)) {
				t.Fatalf("status %s, want the tier arm naming %s in %s:\n  %v", res.Status, tc.to, tc.cut, res.Findings)
			}
		})
	}

	t.Run("an exception ValidateExceptions refuses is reported", func(t *testing.T) {
		withExceptions(t, s, append(structure.BudgetBookExceptions(), structure.Exception{
			Name: "invalid", Cut: "revenue-detail", Against: "spine", At: structure.LevelFundGroupByCategory,
		}))
		res := runCheck(t, &cutsTieAlongTheLattice{}, s)
		if !hasFinding(res, "exceptions: ", `exception "invalid"`) {
			t.Fatalf("status %s, want the exceptions arm:\n  %v", res.Status, res.Findings)
		}
	})

	t.Run("an exception on a pair no comparison relates is refused as inert", func(t *testing.T) {
		pin := structure.Pin{Year: 2026, Basis: "adopted",
			Coords: map[structure.Axis]string{structure.AxisCategory: "transfers/in"},
			Cut:    structure.Sum{Cents: 1, Present: true}, Against: structure.Sum{Cents: 2, Present: true}}
		withExceptions(t, s, append(structure.BudgetBookExceptions(), structure.Exception{
			Name: "inert", Cut: "revenue-detail", Against: "departmentwide", At: structure.LevelCategory,
			Cells: []structure.Pin{pin}, Residual: 1, Printed: "nowhere", Reason: "a plant", Bead: "none",
		}))
		res := runCheck(t, &cutsTieAlongTheLattice{}, s)
		if len(res.Findings) != 1 || res.Findings[0].Subject != "inert" ||
			!strings.Contains(res.Findings[0].Detail, "no comparison relates that pair") {
			t.Fatalf("status %s, want exactly the inert arm:\n  %v", res.Status, res.Findings)
		}
	})
}

// withExceptions declares exceptions on s in place of the ones it carries, for
// one test.
func withExceptions(t *testing.T, s *Subject, exceptions []structure.Exception) {
	t.Helper()
	prev := s.Exceptions
	s.Exceptions = exceptions
	t.Cleanup(func() { s.Exceptions = prev })
}

// TestTheDepartmentSchedulesTieInEveryColumn plants in the actual and revised
// columns, which the spine does not print and only the ties reach.
func TestTheDepartmentSchedulesTieInEveryColumn(t *testing.T) {
	t.Parallel()
	s := committed(t)
	for _, tc := range []struct {
		name, pair, cell string
		plant            func(facts []fact.Fact) []fact.Fact
	}{
		{"funding-city-council's actual and revised columns swapped", "departmentwide ~ funding-sources",
			"FY2024 actual department[department=city-council]", func(facts []fact.Fact) []fact.Fact {
				for i := range facts {
					f := &facts[i]
					if f.RuleID == "funding-city-council" && f.FiscalYear == 2024 {
						f.FiscalYear, f.Basis = 2025, vocab.BasisRevised
					} else if f.RuleID == "funding-city-council" && f.FiscalYear == 2025 {
						f.FiscalYear, f.Basis = 2024, vocab.BasisActual
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
			res := runCheck(t, &cutsTieAlongTheLattice{}, &mutated)
			if !hasFinding(res, tc.pair+": ", tc.cell) {
				t.Fatalf("status %s, want %s refusing %s:\n  %v", res.Status, tc.pair, tc.cell, res.Findings)
			}
		})
	}
}

// TestASplitWithAnEmptySideLeavesItsPairsToTheLattice drops p222's facts: the
// split then holds nothing, so p76 is compared with the spine on transfers out
// again and the CIP's share is a finding rather than a pair compared by nothing.
func TestASplitWithAnEmptySideLeavesItsPairsToTheLattice(t *testing.T) {
	t.Parallel()
	s := mutable(t)
	kept := s.Facts[:0:0]
	for _, f := range s.Facts {
		if f.Scope != "cip-funding-sources" {
			kept = append(kept, f)
		}
	}
	s.Facts = kept
	res := runCheck(t, &cutsTieAlongTheLattice{}, s)
	b := said(res)
	for _, want := range []string{"transfers-detail -> spine", "category=transfers/out fund_group=capital"} {
		if !strings.Contains(b, want) {
			t.Errorf("no finding says %q:\n%s", want, b)
		}
	}
	if strings.Contains(res.Summary, "held only by a split") {
		t.Errorf("the summary still counts a pair held by a split that holds nothing:\n%s", res.Summary)
	}
}
