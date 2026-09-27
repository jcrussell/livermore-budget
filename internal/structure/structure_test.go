package structure_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// committedFacts reads the fact store this repository publishes.
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

// committedFiles reads every rule file under mappings/.
func committedFiles(t *testing.T) []*mapping.File {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	files, err := mapping.LoadDir(os.DirFS(root), "mappings")
	if err != nil {
		t.Fatalf("load the committed rule files: %v", err)
	}
	return files
}

// ruleDeclaring finds one rule declared at the given level, so a test can
// mutate a declaration the store bears out rather than invent a rule.
func ruleDeclaring(t *testing.T, files []*mapping.File, level structure.Level) *mapping.Rule {
	t.Helper()
	for _, f := range files {
		for i := range f.Rules {
			if f.Rules[i].Grain == string(level) {
				return &f.Rules[i]
			}
		}
	}
	t.Fatalf("no committed rule declares grain %q; the fixture no longer covers this", level)
	return nil
}

// TestADeclaredGrainIsHeldToTheFactsThatBearItOut mutates the committed
// `grain:` declarations in memory, one way per arm, and reads the refusal off
// LevelOfRule with the rule named.
func TestADeclaredGrainIsHeldToTheFactsThatBearItOut(t *testing.T) {
	facts := committedFacts(t)

	t.Run("a coarser grain than the facts populate names the rule and both levels", func(t *testing.T) {
		files := committedFiles(t)
		r := ruleDeclaring(t, files, structure.LevelFundByCategory)
		r.Grain = string(structure.LevelFundGroupByCategory)
		_, err := structure.LevelOfRule(facts, files)
		if err == nil {
			t.Fatal("a rule declaring fund-group-by-category over facts that name a fund was accepted")
		}
		for _, want := range []string{r.ID, `"fund-group-by-category"`, `"fund-by-category"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal %q does not name %s", err, want)
			}
		}
	})

	t.Run("a grain naming no declared level is refused by name", func(t *testing.T) {
		files := committedFiles(t)
		r := ruleDeclaring(t, files, structure.LevelFundByCategory)
		r.Grain = "fund-by-anything"
		_, err := structure.LevelOfRule(facts, files)
		if err == nil || !strings.Contains(err.Error(), r.ID) ||
			!strings.Contains(err.Error(), `"fund-by-anything"`) {
			t.Fatalf("LevelOfRule = %v, want a refusal naming the rule and the level it invented", err)
		}
	})

	t.Run("a publishing rule that declares no grain is refused", func(t *testing.T) {
		files := committedFiles(t)
		r := ruleDeclaring(t, files, structure.LevelFundByCategory)
		r.Grain = ""
		_, err := structure.LevelOfRule(facts, files)
		if err == nil || !strings.Contains(err.Error(), r.ID) ||
			!strings.Contains(err.Error(), "declares no grain") {
			t.Fatalf("LevelOfRule = %v, want a refusal naming the rule that publishes without a grain", err)
		}
	})

	t.Run("a grain declared over no fact in the store is refused", func(t *testing.T) {
		files := committedFiles(t)
		r := ruleDeclaring(t, files, structure.LevelFundByCategory)
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			if facts[i].RuleID != r.ID {
				kept = append(kept, facts[i])
			}
		}
		if len(kept) == len(facts) {
			t.Fatalf("rule %q has no facts to drop; the fixture no longer covers this", r.ID)
		}
		_, err := structure.LevelOfRule(kept, files)
		if err == nil || !strings.Contains(err.Error(), r.ID) ||
			!strings.Contains(err.Error(), "no fact") {
			t.Fatalf("LevelOfRule = %v, want a refusal naming the rule whose grain nothing bears out", err)
		}
	})
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

// TestALevelDoesNotRefineItself keeps two peers out of the containment
// comparison.
func TestALevelDoesNotRefineItself(t *testing.T) {
	for _, l := range structure.Levels() {
		if structure.Refines(l, l) {
			t.Errorf("level %q refines itself", l)
		}
	}
}

// TestEveryRuleInTheStoreSitsAtADeclaredLevel: every rule that produced a
// fact populates an axis union the lattice names, and the ACFR fund-balance
// scope's two grains stay two.
func TestEveryRuleInTheStoreSitsAtADeclaredLevel(t *testing.T) {
	facts := committedFacts(t)
	byRule, err := structure.LevelOfRule(facts, committedFiles(t))
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
// store.
func TestEveryDeclaredCutSitsAtTheLevelItDeclares(t *testing.T) {
	facts := committedFacts(t)
	byRule, err := structure.LevelOfRule(facts, committedFiles(t))
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

// TestTheGeneralFundDepartmentsDecomposeTheSpine: pp.167-170's four object
// categories over the spine's two columns, each facing a spine cell.
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

// TestAnExcusedCellIsNotCountedAsAgreeingAtZero holds AgreeAtZero to the
// cells Reconcile compared.
func TestAnExcusedCellIsNotCountedAsAgreeingAtZero(t *testing.T) {
	facts := committedFacts(t)
	for _, name := range []string{"revenue-detail"} {
		c, err := structure.Compare(facts, cutNamed(t, name), cutNamed(t, "spine"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		r := structure.Reconcile(c, structure.BudgetBookExceptions())
		excusedOneSided := 0
		for _, e := range r.Excused {
			for _, p := range e.Cells {
				if !p.Cut.Present || !p.Against.Present {
					excusedOneSided++
				}
			}
		}
		if excusedOneSided == 0 {
			t.Fatalf("%s: no excused one-sided cell, so this proves nothing", name)
		}
		if diff := cmp.Diff(c.OneSided-excusedOneSided, r.AgreeAtZero); diff != "" {
			t.Errorf("%s: one-sided and agreeing at zero (-want +got):\n%s", name, diff)
		}
	}
}

// TestARefusedPairIsNamedRatherThanCompared: each case is a pair the lattice
// offers and the documents do not support.
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

// TestTheCutsAgreeOnWhatTheDocumentsPrint: the same FY2026 adopted money,
// summed over different antichains of one hierarchy, comes to the same figure.
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

	// And per fund group: money moved between groups nets out of the total.
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

// TestTheComparisonGoesRed removes one guarantee per case and names what the
// comparison then says.
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
		// pp.127-140 at the spine's own grain are peers, and Contain must
		// decline rather than tie.
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
