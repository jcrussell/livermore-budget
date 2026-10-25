package structure_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// compared runs Compare over the committed store for two named cuts.
func compared(t *testing.T, facts []fact.Fact, a, b string) structure.Comparison {
	t.Helper()
	ca, cb, held := structure.SplitPair(cutNamed(t, a), cutNamed(t, b), structure.BudgetBookSplits())
	if held {
		t.Fatalf("%q and %q are compared only by a split", a, b)
	}
	got, err := structure.Compare(facts, ca, cb)
	if err != nil {
		t.Fatalf("compare %q against %q: %v", a, b, err)
	}
	return got
}

func exceptionNamed(t *testing.T, name string) structure.Exception {
	t.Helper()
	for _, e := range structure.BudgetBookExceptions() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no declared exception named %q", name)
	return structure.Exception{}
}

// TestAnExceptionGoesRed removes one guarantee of the exception channel per
// arm and reads what Reconcile then says.
func TestAnExceptionGoesRed(t *testing.T) {
	facts := committedFacts(t)

	t.Run("a pinned amount that moves is named with both figures", func(t *testing.T) {
		e := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		e.Cells[0].Against.Cents += 100
		e.Residual += 100
		r := structure.Reconcile(compared(t, facts, "funding-sources", "spine"), []structure.Exception{e})
		if len(r.Findings) != 1 {
			t.Fatalf("%d findings, want 1:\n  %s", len(r.Findings), strings.Join(r.Findings, "\n  "))
		}
		for _, want := range []string{e.Name, "$26,544,516.00", "$26,544,515.00", "delete the exception"} {
			if !strings.Contains(r.Findings[0], want) {
				t.Errorf("finding does not say %q:\n  %s", want, r.Findings[0])
			}
		}
		if len(r.Excused) != 0 {
			t.Errorf("a refused exception was also held apart: %v", r.Excused)
		}
	})

	// The Cut side's own arm. Mutation: drop Reconcile's cell.Cut != p.Cut
	// case and the Against comparison below it matches, so the moved pin is
	// excused silently.
	t.Run("a pinned cut amount that moves is named with both figures", func(t *testing.T) {
		e := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		e.Cells[0].Cut.Cents += 100
		e.Residual -= 100
		r := structure.Reconcile(compared(t, facts, "funding-sources", "spine"), []structure.Exception{e})
		if len(r.Findings) != 1 || !strings.Contains(r.Findings[0], `pins "funding-sources"`) {
			t.Fatalf("findings = %v, want the moved cut pin named", r.Findings)
		}
		if len(r.Excused) != 0 {
			t.Errorf("a refused exception was also held apart: %v", r.Excused)
		}
	})

	// Mutation: drop Reconcile's e.At != c.At arm and the exception's cells
	// are looked up at a level the pair does not meet at.
	t.Run("an exception declared at another level is refused", func(t *testing.T) {
		e := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		e.At = structure.LevelDepartment
		r := structure.Reconcile(compared(t, facts, "funding-sources", "spine"), []structure.Exception{e})
		// Two findings: the refusal, and the cell it no longer excuses.
		if len(r.Findings) != 2 || !strings.Contains(r.Findings[0], "is declared at") {
			t.Fatalf("findings = %v, want the exception refused for its level and its cell unexcused", r.Findings)
		}
		if len(r.Excused) != 0 {
			t.Errorf("a refused exception was also held apart: %v", r.Excused)
		}
	})

	t.Run("a mistyped residual is refused before the store is consulted", func(t *testing.T) {
		e := exceptionNamed(t, "pp.127-130-print-no-general-fund-transfer-in-2026")
		e.Residual += 1
		err := structure.ValidateExceptions([]structure.Exception{e})
		if err == nil || !strings.Contains(err.Error(), e.Name) || !strings.Contains(err.Error(), "mistyped") {
			t.Fatalf("ValidateExceptions = %v, want a refusal naming the exception", err)
		}
	})

	t.Run("a row the exception says the reference prints, deleted, refuses the exception", func(t *testing.T) {
		kept := make([]fact.Fact, 0, len(facts))
		dropped := 0
		for i := range facts {
			f := &facts[i]
			if f.Scope == "all-funds-gross" && f.FiscalYear == 2026 && string(f.Basis) == "adopted" &&
				f.FundGroup == "general" && f.Category == "transfers/in" {
				dropped++
				continue
			}
			kept = append(kept, facts[i])
		}
		if dropped != 1 {
			t.Fatalf("dropped %d facts, want the spine's one General Fund TRANSFER IN cell", dropped)
		}
		r := structure.Reconcile(compared(t, kept, "revenue-detail", "spine"), structure.BudgetBookExceptions())
		if len(r.Findings) != 1 || !strings.Contains(r.Findings[0], "excuses nothing") ||
			!strings.Contains(r.Findings[0], "pp.127-130-print-no-general-fund-transfer-in-2026") {
			t.Fatalf("findings = %v, want the one exception refused as excusing nothing", r.Findings)
		}
	})

	t.Run("a cell that ties refuses the exception declared over it", func(t *testing.T) {
		planted := make([]fact.Fact, len(facts))
		copy(planted, facts)
		moved := 0
		for i := range planted {
			f := &planted[i]
			if f.Scope == "all-funds-gross" && f.FiscalYear == 2027 && string(f.Basis) == "adopted" &&
				f.FundGroup == "internal-service" && f.Category == "services-and-supplies" && f.Kind == "expenditure" {
				f.AmountCents -= 25000000
				moved++
			}
		}
		if moved != 1 {
			t.Fatalf("moved %d facts, want p0067's one Services & Supplies cell", moved)
		}
		e := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		r := structure.Reconcile(compared(t, planted, "funding-sources", "spine"), []structure.Exception{e})
		if len(r.Findings) != 1 || !strings.Contains(r.Findings[0], "false claim") {
			t.Fatalf("findings = %v, want the exception refused as a false claim over a cell that ties", r.Findings)
		}
	})

	t.Run("a cross-axis grounding that names a different figure is refused", func(t *testing.T) {
		byGroup := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		byObject := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-object")
		byObject.Cells[0].Cut.Cents -= 100
		byObject.Residual += 100
		err := structure.ValidateExceptions([]structure.Exception{byGroup, byObject})
		if err == nil || !strings.Contains(err.Error(), "two axes") {
			t.Fatalf("ValidateExceptions = %v, want the grounding refused", err)
		}
	})

	t.Run("cells that net to zero are one figure printed in two places, and a lone cell never nets", func(t *testing.T) {
		e := exceptionNamed(t, "pp.127-140-print-three-funds-revenue-as-the-state-grant-funds-2024")
		if len(e.Cells) < 2 || e.Residual != 0 {
			t.Fatalf("%s holds %d cells apart by %d; want a move between cells", e.Name, len(e.Cells), e.Residual)
		}
		if err := structure.ValidateExceptions([]structure.Exception{e}); err != nil {
			t.Fatalf("ValidateExceptions refused a move between cells: %v", err)
		}
		e.Cells = e.Cells[:1]
		err := structure.ValidateExceptions([]structure.Exception{e})
		if err == nil || !strings.Contains(err.Error(), "residual of zero") {
			t.Fatalf("ValidateExceptions = %v, want a lone cell's zero residual refused", err)
		}
	})

	t.Run("an exception whose two sides do not differ is refused at declaration", func(t *testing.T) {
		e := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		e.Cells[0].Cut = e.Cells[0].Against
		err := structure.ValidateExceptions([]structure.Exception{e})
		if err == nil || !strings.Contains(err.Error(), "needs no exception") {
			t.Fatalf("ValidateExceptions = %v, want a refusal", err)
		}
	})
}

// TestAPairIsComparedOnlyWhereTheLatticeSaysHow pins the division/department
// vocabulary refusal and the refusal of a pair with no reference in it.
func TestAPairIsComparedOnlyWhereTheLatticeSaysHow(t *testing.T) {
	facts := committedFacts(t)

	t.Run("pp.167-170 name divisions and pp.171-176 name departments", func(t *testing.T) {
		_, err := structure.Compare(facts, cutNamed(t, "general-fund-departments"), cutNamed(t, "funding-sources"))
		if err == nil || !strings.Contains(err.Error(), `"division" tier`) ||
			!strings.Contains(err.Error(), `"department" tier`) {
			t.Fatalf("Compare = %v, want the tier mismatch refused by name", err)
		}
		// The mutation: declared at one tier, the split shows as arithmetic.
		fine := cutNamed(t, "general-fund-departments")
		fine.DepartmentTier = "department"
		got, err := structure.Compare(facts, fine, cutNamed(t, "funding-sources"))
		if err != nil {
			t.Fatalf("compare with the tiers equal: %v", err)
		}
		if len(got.Findings) < 100 {
			t.Errorf("with the tiers declared equal the pair reports %d findings; the divisions "+
				"and departments share almost no key, so this should be most of its cells", len(got.Findings))
		}
	})

	t.Run("two non-reference cuts that meet below both are refused", func(t *testing.T) {
		_, err := structure.Compare(facts, cutNamed(t, "revenue-detail"), cutNamed(t, "funding-sources"))
		if err == nil || !strings.Contains(err.Error(), "neither is the reference") {
			t.Fatalf("Compare = %v, want a refusal for want of a reference", err)
		}
	})

	t.Run("a cut with the department axis and no tier is refused", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		cuts := structure.BudgetBookCuts()
		for i := range cuts {
			if cuts[i].Name == "departmentwide" {
				cuts[i].DepartmentTier = ""
			}
		}
		if _, err := structure.ValidateCuts(facts, byRule, cuts); err == nil ||
			!strings.Contains(err.Error(), "departmentwide") || !strings.Contains(err.Error(), "tier") {
			t.Fatalf("ValidateCuts = %v, want the cut refused for naming no tier", err)
		}
	})

	t.Run("a cut declared at a level its facts are not at is refused by name", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		cuts := structure.BudgetBookCuts()
		for i := range cuts {
			if cuts[i].Name == "revenue-detail" {
				cuts[i].Level = structure.LevelFundGroupByCategory
			}
		}
		_, err = structure.ValidateCuts(facts, byRule, cuts)
		if err == nil {
			t.Fatal("pp.127-140 declared at the spine's own grain were accepted")
		}
		for _, want := range []string{"revenue-detail", `"fund-group-by-category"`, `"fund-by-category"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal does not name %s:\n  %v", want, err)
			}
		}
	})

	// Both mutations leave Drop(derived) == Level true, so only the
	// vocabulary arm can refuse them.
	t.Run("a placeholder axis carrying a second value is refused", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		planted := make([]fact.Fact, len(facts))
		copy(planted, facts)
		for i := range planted {
			if planted[i].Scope == "department-funding-sources" {
				planted[i].Category = "wages-and-benefits"
				break
			}
		}
		_, err = structure.ValidateCuts(planted, byRule, structure.BudgetBookCuts())
		if err == nil || !strings.Contains(err.Error(), `"funding-sources"`) ||
			!strings.Contains(err.Error(), "placeholder") || !strings.Contains(err.Error(), "2 values") {
			t.Fatalf("ValidateCuts = %v, want the placeholder refused for carrying two values", err)
		}
	})

	t.Run("a placeholder whose one value another schedule carries is refused as a footprint", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		planted := make([]fact.Fact, len(facts))
		copy(planted, facts)
		for i := range planted {
			if planted[i].Scope == "department-funding-sources" {
				planted[i].Category = "services-and-supplies"
			}
		}
		_, err = structure.ValidateCuts(planted, byRule, structure.BudgetBookCuts())
		if err == nil || !strings.Contains(err.Error(), `"funding-sources"`) ||
			!strings.Contains(err.Error(), `"services-and-supplies"`) || !strings.Contains(err.Error(), "footprint") {
			t.Fatalf("ValidateCuts = %v, want the placeholder refused as a footprint another scope carries", err)
		}
	})

	t.Run("a second reference is refused", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		cuts := structure.BudgetBookCuts()
		for i := range cuts {
			if cuts[i].Name == "departmentwide" {
				cuts[i].Reference = true
			}
		}
		if _, err := structure.ValidateCuts(facts, byRule, cuts); err == nil ||
			!strings.Contains(err.Error(), "reference") {
			t.Fatalf("ValidateCuts = %v, want two references refused", err)
		}
	})

	t.Run("a cut with no fact is returned, not refused", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		var kept []fact.Fact
		for i := range facts {
			if facts[i].Scope != "transfers-by-fund" {
				kept = append(kept, facts[i])
			}
		}
		empty, err := structure.ValidateCuts(kept, byRule, structure.BudgetBookCuts())
		if err != nil {
			t.Fatalf("ValidateCuts: %v", err)
		}
		if diff := cmp.Diff([]string{"transfers-detail"}, empty); diff != "" {
			t.Errorf("empty cuts (-want +got):\n%s", diff)
		}
	})

	t.Run("a cut that loses one rule's kind is refused, not compared as whole", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range []string{"acfr-p0041-gf-revenues", "acfr-p0041-gf-fund-balances", "acfr-p0041-gf-other-financing-sources"} {
			var kept []fact.Fact
			for i := range facts {
				if facts[i].RuleID != rule {
					kept = append(kept, facts[i])
				}
			}
			if len(kept) == len(facts) {
				t.Fatalf("%s carries no fact to drop", rule)
			}
			_, err := structure.ValidateCuts(kept, byRule, structure.AllCuts())
			if err == nil || !strings.Contains(err.Error(), `"acfr-general-fund-summary"`) ||
				!strings.Contains(err.Error(), "declares kind") {
				t.Errorf("with %s dropped, ValidateCuts = %v, want acfr-general-fund-summary refused for a declared kind it lost", rule, err)
			}
		}
	})

	t.Run("a column that loses a kind the cut's other columns carry is refused", func(t *testing.T) {
		byRule, err := structure.LevelOfRule(facts, committedFiles(t))
		if err != nil {
			t.Fatal(err)
		}
		var kept []fact.Fact
		for i := range facts {
			if facts[i].RuleID != "spine-fund-balance" || facts[i].FiscalYear != 2027 {
				kept = append(kept, facts[i])
			}
		}
		if len(kept) == len(facts) {
			t.Fatal("spine-fund-balance carries no FY2027 fact to drop")
		}
		_, err = structure.ValidateCuts(kept, byRule, structure.AllCuts())
		if err == nil || !strings.Contains(err.Error(), `cut "spine" declares kind "fund_balance" and its FY2027 adopted column`) {
			t.Errorf("ValidateCuts = %v, want the spine's FY2027 column refused for the kind it lost", err)
		}
	})
}
