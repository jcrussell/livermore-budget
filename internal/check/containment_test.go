package check

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
)

// TestTheCommittedCutsTieAlongTheLattice pins what the one lattice-driven
// comparison covers over the committed corpus, BY NAME: which pairs are
// compared, at which grain, and which are refused with which reason. A
// mutation that turns a comparison into a refusal -- a kind dropped from a cut,
// a tier changed -- leaves the check green with one comparison fewer, and this
// is the arm that sees it.
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
		"5 comparison(s) of 6 cut(s)",
		"revenue-detail -> spine at fund-group-by-category",
		"transfers-detail -> spine at fund-group-by-category",
		"general-fund-departments -> spine at fund-group-by-category",
		"departmentwide ~ spine at category",
		"funding-sources ~ spine at fund-group",
		// The refusals, each a claim about the documents.
		`"revenue-detail" against "transfers-detail": both are at "fund-by-category"`,
		`"general-fund-departments" names departments at the "division" tier and "funding-sources" at the "department" tier`,
		`"departmentwide" is at "department-by-category", which carries no fund group axis`,
		"no money is described by both",
		"neither is the reference",
		"share no common coarsening",
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
}

// TestTheCutsCheckGoesRed runs the mutations through the registered check
// rather than through the package, so what is proven is the line fisc verify
// prints.
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
		if res.Status != StatusFail || len(res.Findings) != 1 {
			t.Fatalf("status %s with %d findings, want one failure:\n  %v", res.Status, len(res.Findings), res.Findings)
		}
		if !strings.Contains(res.Findings[0].Detail, "taxes/property") || !strings.Contains(res.Findings[0].Detail, "$1.00") {
			t.Errorf("the finding names neither the cell nor the difference: %s", res.Findings[0].Detail)
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
		// A cut with no fact is skipped rather than compared as a side that
		// prints nothing, for the fixture's sake. What makes that safe over
		// the committed corpus is the grain arm: every one of the schedule's
		// rules still declares a grain, and a grain over zero facts is refused
		// by name before any pair is compared.
		if diff := cmp.Diff(StatusFail, res.Status); diff != "" {
			t.Fatalf("status (-want +got):\n%s", diff)
		}
		if len(res.Findings) != 1 || res.Findings[0].Subject != "grain" ||
			!strings.Contains(res.Findings[0].Detail, "dw-") ||
			!strings.Contains(res.Findings[0].Detail, "no fact") {
			t.Errorf("want exactly the grain arm's refusal naming a dw- rule, got:\n  %v", res.Findings)
		}
		// And the p0067 exception on that axis is not refused: its cut was
		// never compared, so it was never consulted, and the check says
		// nothing about it rather than something false.
		for _, f := range res.Findings {
			if strings.Contains(f.Detail, "by-object") {
				t.Errorf("an exception on an empty cut was reported: %s", f.Detail)
			}
		}
	})
}

// runOne runs a single check over a subject.
func runOne(t *testing.T, s *Subject, c Check) *Report {
	t.Helper()
	return Run(t.Context(), s, []Check{c}, ReportOptions{GeneratedBy: testVersion})
}
