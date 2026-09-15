package structure_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// committedFacts reads the fact store this repository publishes. Nothing here
// runs the extractor, opens a PDF or reaches the network.
func committedFacts(t *testing.T) []fact.Fact {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	f, err := os.Open(filepath.Join(root, "facts", "facts.jsonl"))
	if err != nil {
		t.Fatalf("open the committed fact store: %v", err)
	}
	defer f.Close()
	facts, err := fact.Read(f)
	if err != nil {
		t.Fatalf("read the committed fact store: %v", err)
	}
	return facts
}

// TestMeetIsTheGrainTwoCutsAgreeAt pins the four shapes the comparison depends
// on, including the two where the meet is NEITHER input.
func TestMeetIsTheGrainTwoCutsAgreeAt(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b structure.Level
		want structure.Level
	}{
		{"a finer cut meets the coarser one at the coarser one",
			structure.LevelFundByCategory, structure.LevelFundGroupByCategory, structure.LevelFundGroupByCategory},
		{"pp.85-125 and the spine share only the object category",
			structure.LevelDepartmentByCategory, structure.LevelFundGroupByCategory, structure.LevelCategory},
		{"pp.171-176 print no category, so they meet the spine at the fund group",
			structure.LevelFundByDepartment, structure.LevelFundGroupByCategory, structure.LevelFundGroup},
		{"the finest level meets the spine at the spine",
			structure.LevelFundByDepartmentByCategory, structure.LevelFundGroupByCategory, structure.LevelFundGroupByCategory},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := structure.Meet(c.a, c.b)
			if err != nil {
				t.Fatalf("Meet(%q, %q): %v", c.a, c.b, err)
			}
			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Errorf("Meet(%q, %q) (-want +got):\n%s", c.a, c.b, diff)
			}
		})
	}
}

// TestALevelDoesNotRefineItself is the arm that keeps two peers out of the
// containment comparison. Two cuts at one level state figures about the same
// money at the same grain; whether they AGREE is a different question with a
// different failure mode, and Contain must not answer it.
func TestALevelDoesNotRefineItself(t *testing.T) {
	for _, l := range structure.Levels() {
		if structure.Refines(l, l) {
			t.Errorf("level %q refines itself", l)
		}
	}
}

// TestEveryRuleInTheStoreSitsAtADeclaredLevel is the claim that makes the
// lattice a description of these documents rather than a hopeful model: every
// rule that produced a fact populates an axis union the lattice names.
//
// It also pins the split the store's ONE two-grain scope forces. The ACFR's
// fund-balance schedule prints the General Fund with its group named and the
// other governmental funds aggregated, so `scope` cannot carry the grain.
func TestEveryRuleInTheStoreSitsAtADeclaredLevel(t *testing.T) {
	facts := committedFacts(t)
	byRule, err := structure.LevelOfRule(facts)
	if err != nil {
		t.Fatalf("derive each rule's level: %v", err)
	}
	for rule, l := range byRule {
		if !structure.Declared(l) {
			t.Errorf("rule %q sits at %q, which is not a declared level", rule, l)
		}
	}
	levels := map[string]map[structure.Level]bool{}
	for i := range facts {
		f := &facts[i]
		if levels[f.Scope] == nil {
			levels[f.Scope] = map[structure.Level]bool{}
		}
		levels[f.Scope][byRule[f.RuleID]] = true
	}
	if n := len(levels["acfr-fund-balances"]); n != 2 {
		t.Errorf("acfr-fund-balances sits at %d level(s), want 2: the schedule prints the "+
			"General Fund by name and the other governmental funds aggregated, which is why "+
			"a scope cannot carry a grain", n)
	}
}

// TestEveryDeclaredCutSitsAtTheLevelItDeclares checks each Cut against the
// store instead of trusting it, and holds the placeholder rule to its reason.
func TestEveryDeclaredCutSitsAtTheLevelItDeclares(t *testing.T) {
	facts := committedFacts(t)
	byRule, err := structure.LevelOfRule(facts)
	if err != nil {
		t.Fatalf("derive each rule's level: %v", err)
	}
	for _, c := range structure.BudgetBookCuts() {
		derived, ok := c.DerivedLevel(byRule, facts)
		if !ok {
			t.Errorf("cut %q selects facts at no single level", c.Name)
			continue
		}
		want := derived
		for _, a := range c.Placeholders {
			want = structure.Drop(want, a)
		}
		if diff := cmp.Diff(want, c.Level); diff != "" {
			t.Errorf("cut %q declares a level its facts do not support (-want +got):\n%s"+
				"\n  derived %q, placeholders %v", c.Name, diff, derived, c.Placeholders)
		}
	}
}

// TestTheGeneralFundDepartmentsDecomposeTheSpine is the reproduction that
// matters: one generic comparison, driven off the lattice, produces the
// arithmetic expenditure-detail-ties-to-spine produces from a hand-written
// key and a hand-written restriction.
//
// EIGHT CELLS AND NONE ONE-SIDED. pp.167-170 are the General Fund's four object
// categories over two published columns, and every one has a spine cell facing
// it. A one-sided cell here would be a category one schedule prints and the
// other does not.
func TestTheGeneralFundDepartmentsDecomposeTheSpine(t *testing.T) {
	facts := committedFacts(t)
	fine, coarse := cutNamed(t, "general-fund-departments"), cutNamed(t, "spine")
	got, err := structure.Contain(facts, fine, coarse)
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("pp.167-170 do not decompose the spine:\n  %v", got.Findings)
	}
	if diff := cmp.Diff([]int{8, 0}, []int{got.Subjects, got.OneSided}); diff != "" {
		t.Errorf("cells compared and one-sided (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(structure.LevelFundGroupByCategory, got.At); diff != "" {
		t.Errorf("compared at the wrong level (-want +got):\n%s", diff)
	}
}

// TestARefusedPairIsNamedRatherThanCompared is the arm that keeps this from
// reporting a clean comparison of two things that do not compare. Each case is
// a pair the lattice offers and the documents do not support.
func TestARefusedPairIsNamedRatherThanCompared(t *testing.T) {
	facts := committedFacts(t)
	for _, c := range []struct{ name, fine, coarse, want string }{
		{"expenditure against revenue", "general-fund-departments", "revenue-detail",
			"no money is described by both"},
		{"expenditure against transfers", "general-fund-departments", "transfers-detail",
			"no money is described by both"},
		{"a General Fund schedule against one with no fund axis to restrict",
			"general-fund-departments", "departmentwide",
			"carries no fund group axis"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := structure.Contain(facts, cutNamed(t, c.fine), cutNamed(t, c.coarse))
			if err == nil {
				t.Fatalf("comparing %q against %q was allowed", c.fine, c.coarse)
			}
			if !contains(err.Error(), c.want) {
				t.Errorf("refusal does not say why:\n  got  %v\n  want it to mention %q", err, c.want)
			}
		})
	}
}

// TestTheKnownExceptionsAreTheWholeResidual pins what the generic comparison
// reports that the hand-written checks declare as exceptions. It is the honest
// half of the reproduction: the machinery generalises and the ARGUMENTS do not.
//
// REVENUE. pp.127-140 print no General Fund transfer in. p76 does --
// internal/check/revenuedetail.go declares it and prints the figure, 480,400 in
// FY2026 -- so the spine has a cell the revenue schedule never had. Two cells,
// one per published column.
//
// TRANSFERS. pp.66-67's TRANSFER OUT includes transfers to CIP that p76 does not
// list, which internal/check/transfersdetail.go carries as a toCIP add-back
// across three clauses. This comparison is one clause, so it reports them.
func TestTheKnownExceptionsAreTheWholeResidual(t *testing.T) {
	facts := committedFacts(t)
	for _, c := range []struct {
		name         string
		fine, coarse string
		wantFindings int
	}{
		{"pp.127-140 against the spine: the General Fund transfer in p76 prints and they do not",
			"revenue-detail", "spine", 2},
		{"p76 against the spine: the transfers to CIP pp.66-67 include and p76 does not list",
			"transfers-detail", "spine", 8},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := structure.Contain(facts, cutNamed(t, c.fine), cutNamed(t, c.coarse))
			if err != nil {
				t.Fatalf("contain: %v", err)
			}
			if len(got.Findings) != c.wantFindings {
				t.Errorf("residual is %d cells, want %d:\n  %v",
					len(got.Findings), c.wantFindings, got.Findings)
			}
		})
	}
}

func cutNamed(t *testing.T, name string) structure.Cut {
	t.Helper()
	for _, c := range structure.BudgetBookCuts() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no declared cut named %q", name)
	return structure.Cut{}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestTheCutsAgreeOnWhatTheDocumentsPrint is the arithmetic the whole design
// rests on: the same money, summed over different antichains of one hierarchy,
// comes to the same figure. If these ever disagree, the scopes are not cuts of
// one structure and the lattice is a fiction.
//
// Every figure is FY2026 adopted, read from the committed store.
func TestTheCutsAgreeOnWhatTheDocumentsPrint(t *testing.T) {
	facts := committedFacts(t)
	sum := func(scope string, kinds ...string) int64 {
		var total int64
		for i := range facts {
			f := &facts[i]
			if f.Scope != scope || f.FiscalYear != 2026 || string(f.Basis) != "adopted" {
				continue
			}
			for _, k := range kinds {
				if string(f.Kind) == k {
					total += f.AmountCents
				}
			}
		}
		return total
	}

	// REVENUE UNDER TWO CUTS: pp.66-67 by fund group, pp.127-140 by fund.
	const revenue = 29_996_900_700
	if diff := cmp.Diff([]int64{revenue, revenue},
		[]int64{sum("all-funds-gross", "revenue"), sum("revenue-by-fund", "revenue")}); diff != "" {
		t.Errorf("revenue under the fund-group cut and the fund cut (-want +got):\n%s", diff)
	}

	// AND PER FUND GROUP, because two totals agreeing is a much weaker claim
	// than six pairs agreeing: an error that moves money between groups nets
	// out of the total and survives it.
	groups := map[string][2]int64{}
	for i := range facts {
		f := &facts[i]
		if f.FiscalYear != 2026 || string(f.Basis) != "adopted" || string(f.Kind) != "revenue" {
			continue
		}
		g := groups[f.FundGroup]
		switch f.Scope {
		case "all-funds-gross":
			g[0] += f.AmountCents
		case "revenue-by-fund":
			g[1] += f.AmountCents
		default:
			continue
		}
		groups[f.FundGroup] = g
	}
	if len(groups) != 6 {
		t.Errorf("revenue reaches %d fund groups, want 6", len(groups))
	}
	for g, pair := range groups {
		if pair[0] != pair[1] {
			t.Errorf("fund group %q: the spine publishes %d and pp.127-140 sum to %d, a "+
				"difference of %d", g, pair[0], pair[1], pair[0]-pair[1])
		}
	}

	// EXPENDITURE UNDER THREE CUTS, along three different axes: pp.66-67 by
	// fund group and object, pp.85-125 by department and object, pp.171-176 by
	// fund and department.
	const expenditure = 25_409_541_200
	if diff := cmp.Diff([]int64{expenditure, expenditure, expenditure},
		[]int64{
			sum("all-funds-gross", "expenditure"),
			sum("departmentwide-expenditures", "expenditure"),
			sum("department-funding-sources", "expenditure"),
		}); diff != "" {
		t.Errorf("expenditure under three independent decompositions (-want +got):\n%s", diff)
	}
}

// TestTheComparisonGoesRed is the mutation. Each case reproduces a real
// guarantee by removing it and naming what the comparison then says; a case
// that stayed green would be a guarantee this check does not actually hold.
func TestTheComparisonGoesRed(t *testing.T) {
	facts := committedFacts(t)
	spine := cutNamed(t, "spine")

	t.Run("a finer cut that loses its footprint meets money it never printed", func(t *testing.T) {
		// pp.167-170 are the General Fund. Drop that and the comparison faces
		// the other five groups' expenditure, which those pages never printed.
		fine := cutNamed(t, "general-fund-departments")
		fine.FundGroups = nil
		got, err := structure.Contain(facts, fine, spine)
		if err != nil {
			t.Fatalf("contain: %v", err)
		}
		if len(got.Findings) == 0 {
			t.Fatal("a General Fund schedule compared against every fund group reports clean")
		}
	})

	t.Run("a cut whose level stops refining the spine is refused, not compared", func(t *testing.T) {
		// The mutation the whole grain argument is about: put pp.127-140 at the
		// spine's own grain and the two stop being coarse and fine. They are
		// then peers, whose agreement is a different question with a different
		// failure mode, and Contain must decline rather than tie.
		fine := cutNamed(t, "revenue-detail")
		fine.Level = structure.LevelFundGroupByCategory
		if _, err := structure.Contain(facts, fine, spine); err == nil {
			t.Fatal("two cuts at one level were compared as a containment")
		}
	})

	t.Run("a figure that moves is named with its difference", func(t *testing.T) {
		planted := make([]fact.Fact, len(facts))
		copy(planted, facts)
		moved := 0
		for i := range planted {
			f := &planted[i]
			if f.Scope == "expenditure-by-department" && f.FiscalYear == 2026 &&
				string(f.Basis) == "adopted" && f.Category == "wages-and-benefits" {
				f.AmountCents += 100
				moved++
				break
			}
		}
		if moved == 0 {
			t.Fatal("no General Fund wages row to move; the fixture no longer covers this")
		}
		got, err := structure.Contain(planted, cutNamed(t, "general-fund-departments"), spine)
		if err != nil {
			t.Fatalf("contain: %v", err)
		}
		if len(got.Findings) != 1 {
			t.Fatalf("moving one cell by a dollar produced %d findings, want 1:\n  %v",
				len(got.Findings), got.Findings)
		}
		if !contains(got.Findings[0], "wages-and-benefits") ||
			!contains(got.Findings[0], "$1.00") {
			t.Errorf("the finding names neither the cell nor the difference:\n  %s", got.Findings[0])
		}
	})
}
