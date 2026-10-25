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
		{"a fund's total meets the spine at its fund group",
			structure.LevelFund, structure.LevelFundGroupByCategory, structure.LevelFundGroup},
		{"pp.127-140 and pp.85-125's funding sources meet at the fund",
			structure.LevelFundByCategory, structure.LevelFundByDepartment, structure.LevelFund},
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

// TestAFundTotalSitsBetweenTheFundGroupAndEveryPerFundLevel: pp.186-209 print
// a fund's Revenues and Expenses whole, so every level carrying the fund
// decomposes them, and they decompose the fund group.
func TestAFundTotalSitsBetweenTheFundGroupAndEveryPerFundLevel(t *testing.T) {
	for _, fine := range []structure.Level{
		structure.LevelFundByCategory,
		structure.LevelFundByDepartment,
		structure.LevelFundByDepartmentByCategory,
	} {
		if !structure.Refines(fine, structure.LevelFund) {
			t.Errorf("%q does not refine %q", fine, structure.LevelFund)
		}
	}
	if !structure.Refines(structure.LevelFund, structure.LevelFundGroup) {
		t.Errorf("%q does not refine %q", structure.LevelFund, structure.LevelFundGroup)
	}
	for _, other := range []structure.Level{
		structure.LevelFundGroupByCategory, structure.LevelCategory,
		structure.LevelDepartment, structure.LevelDepartmentByCategory,
	} {
		if structure.Refines(structure.LevelFund, other) || structure.Refines(other, structure.LevelFund) {
			t.Errorf("%q and %q are related; neither carries the other's axes", structure.LevelFund, other)
		}
	}
	if diff := cmp.Diff(structure.LevelFund,
		structure.Drop(structure.LevelFundByCategory, structure.AxisCategory)); diff != "" {
		t.Errorf("a per-fund cut whose category is a placeholder (-want +got):\n%s", diff)
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
			if !strings.Contains(err.Error(), c.want) {
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
		if !strings.Contains(got.Findings[0], "wages-and-benefits") ||
			!strings.Contains(got.Findings[0], "$1.00") {
			t.Errorf("the finding names neither the cell nor the difference:\n  %s", got.Findings[0])
		}
	})
}

// TestTallyCountsWithoutWording holds Tally to the split it documents: a
// disagreeing and a one-sided cell are counted and carried, and no finding
// names the two cuts a caller of Tally does not have.
func TestTallyCountsWithoutWording(t *testing.T) {
	a := structure.Key{Year: 2026, Basis: "adopted", Coords: "a"}
	b := structure.Key{Year: 2026, Basis: "adopted", Coords: "b"}
	c := structure.Tally(
		map[structure.Key]structure.Sum{a: {Cents: 1, Present: true}, b: {Cents: 2, Present: true}},
		map[structure.Key]structure.Sum{a: {Cents: 3, Present: true}},
		func(structure.Key) bool { return true })
	if c.Subjects != 2 || c.OneSided != 1 || len(c.Cells) != 2 {
		t.Errorf("Tally = %d subjects, %d one-sided, %d cells; want 2, 1, 2", c.Subjects, c.OneSided, len(c.Cells))
	}
	if len(c.Findings) != 0 {
		t.Errorf("Tally worded %q; its caller words its own findings", c.Findings)
	}
}

// TestACutIsComparedOnlyOnTheLinesItsPagesPrint holds pp.186-209 to the spine
// without the spine's CHANGE IN WORKING CAPITAL, which pp.186-209 print no
// line for, and refuses a comparison once a fact of the cut sits on no line it
// declares: leaving a line off the list must not drop it from the gate.
func TestACutIsComparedOnlyOnTheLinesItsPagesPrint(t *testing.T) {
	facts := committedFacts(t)
	flows, spine := allCutNamed(t, structure.CutFundBalanceFlows), allCutNamed(t, structure.CutSpine)
	c, err := structure.Compare(facts, flows, spine)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	held := 0
	for _, cell := range c.Cells {
		if strings.Contains(cell.Key.String(), "fund-balance/change") {
			t.Errorf("%s is compared; pp.186-209 print no change line", cell.Key)
		}
		if strings.Contains(cell.Key.String(), "fund-balance/ending") {
			held++
		}
	}
	if held == 0 {
		t.Error("no ending balance is compared; the change is held through beginning and ending, so both must be")
	}

	// The spine's change line is declared unprinted; undeclared, a spine
	// fact on it would leave the comparison unseen, and is refused.
	undeclared := flows
	undeclared.Unprinted = nil
	if _, err := structure.Compare(facts, undeclared, spine); err == nil ||
		!strings.Contains(err.Error(), "declares neither printed nor unprinted") {
		t.Errorf("Compare with the change line undeclared = %v, want refused", err)
	}

	var short []structure.Line
	for _, l := range flows.Lines {
		if l != structure.LineEnding {
			short = append(short, l)
		}
	}
	flows.Lines = short
	if _, err := structure.Compare(facts, flows, spine); err == nil ||
		!strings.Contains(err.Error(), "fund-balance/ending") {
		t.Errorf("Compare with the ending line undeclared = %v, want refused", err)
	}
	// Declared unprinted, the spine's ending passes; the cut's own ending
	// facts are then on no line it prints, and are refused.
	flows.Unprinted = append(flows.Unprinted, structure.LineEnding)
	if _, err := structure.Compare(facts, flows, spine); err == nil ||
		!strings.Contains(err.Error(), "is on no line") {
		t.Errorf("Compare with the ending line declared unprinted = %v, want its own facts refused", err)
	}
}
