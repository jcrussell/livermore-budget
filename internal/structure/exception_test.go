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
	got, err := structure.Compare(facts, cutNamed(t, a), cutNamed(t, b))
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

// TestTheDeclaredExceptionsAreTheWholeResidual is the honest half of the
// reproduction, by name rather than by count: every cell of every Budget Book
// comparison that does not tie is held apart by a declared exception, and
// every declared exception holds a cell apart.
//
// REVENUE. pp.127-140 print no General Fund transfer in. p76 does, so the spine
// has a cell the revenue schedule never had. Two cells, one per published
// column.
//
// TRANSFERS. pp.66-67's TRANSFER OUT includes transfers to CIP that p76 does
// not list; pp.72-75 print them. Eight cells over six exceptions, because the
// non-major aggregate is printed once for two groups.
//
// p0067. The 250,000 the Internal Service column is high by, on the fund-group
// axis where both figures are printed and on the object axis where neither is.
func TestTheDeclaredExceptionsAreTheWholeResidual(t *testing.T) {
	facts := committedFacts(t)
	exceptions := structure.BudgetBookExceptions()
	if err := structure.ValidateExceptions(exceptions); err != nil {
		t.Fatalf("the declared exceptions do not validate: %v", err)
	}
	want := map[string][]string{
		"revenue-detail -> spine": {
			"pp.127-130-print-no-general-fund-transfer-in-2026",
			"pp.127-130-print-no-general-fund-transfer-in-2027",
		},
		"transfers-detail -> spine": {
			"p76-lists-no-transfer-to-the-cip-2026-enterprise",
			"p76-lists-no-transfer-to-the-cip-2026-internal-service",
			"p76-lists-no-transfer-to-the-cip-2026-non-major",
			"p76-lists-no-transfer-to-the-cip-2027-enterprise",
			"p76-lists-no-transfer-to-the-cip-2027-internal-service",
			"p76-lists-no-transfer-to-the-cip-2027-non-major",
		},
		"general-fund-departments -> spine": nil,
		"departmentwide ~ spine":            {"p0067-internal-service-is-250000-high-by-object"},
		"funding-sources ~ spine":           {"p0067-internal-service-is-250000-high-by-fund-group"},
	}
	fired := map[string]bool{}
	for pair, names := range want {
		a, b, _ := strings.Cut(pair, " -> ")
		if !strings.Contains(pair, " -> ") {
			a, b, _ = strings.Cut(pair, " ~ ")
		}
		r := structure.Reconcile(compared(t, facts, a, b), exceptions)
		if len(r.Findings) != 0 {
			t.Errorf("%s: %d finding(s) survive the declared exceptions:\n  %s",
				pair, len(r.Findings), strings.Join(r.Findings, "\n  "))
		}
		got := make([]string, 0, len(r.Excused))
		for _, e := range r.Excused {
			got = append(got, e.Name)
			fired[e.Name] = true
		}
		if diff := cmp.Diff(names, got, cmp.Transformer("nilIsEmpty", func(s []string) []string {
			if s == nil {
				return []string{}
			}
			return s
		})); diff != "" {
			t.Errorf("%s: exceptions held apart (-want +got):\n%s", pair, diff)
		}
	}
	for _, e := range exceptions {
		if !fired[e.Name] {
			t.Errorf("exception %q is declared and no comparison held it apart", e.Name)
		}
	}
}

// TestTheAgreementAtTheMeetReproducesTheTwoAxisReconciliations is the arithmetic
// fisc-av0w rests on, produced by the generic comparison: pp.85-125 by object
// and pp.171-176 by fund each agree with the spine everywhere but the one cell
// p0067 prints wrong, and the two cells differ from the spine by the same
// 250,000.
func TestTheAgreementAtTheMeetReproducesTheTwoAxisReconciliations(t *testing.T) {
	facts := committedFacts(t)
	for _, c := range []struct {
		cut, at, key  string
		detail, spine int64
	}{
		{"departmentwide", "category", "FY2027 adopted category[category=services-and-supplies]",
			13025208700, 13050208700},
		{"funding-sources", "fund-group", "FY2027 adopted fund-group[fund_group=internal-service]",
			2629451500, 2654451500},
	} {
		t.Run(c.cut, func(t *testing.T) {
			got := compared(t, facts, c.cut, "spine")
			if diff := cmp.Diff(structure.Agreement, got.Relation); diff != "" {
				t.Errorf("relation (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(structure.Level(c.at), got.At); diff != "" {
				t.Errorf("level (-want +got):\n%s", diff)
			}
			var off []structure.Cell
			for _, cell := range got.Cells {
				if !cell.Ties() {
					off = append(off, cell)
				}
			}
			if len(off) != 1 {
				t.Fatalf("%d cells disagree, want exactly p0067's:\n  %v", len(off), got.Findings)
			}
			want := structure.Cell{
				Key:     structure.Key{Year: 2027, Basis: "adopted", Level: structure.Level(c.at)},
				Cut:     structure.Sum{Cents: c.detail, Present: true},
				Against: structure.Sum{Cents: c.spine, Present: true},
			}
			want.Key.Coords = off[0].Key.Coords
			if diff := cmp.Diff(want, off[0]); diff != "" {
				t.Errorf("the disagreeing cell (-want +got):\n%s", diff)
			}
			if off[0].Key.String() != c.key {
				t.Errorf("cell is %s, want %s", off[0].Key, c.key)
			}
			if d := off[0].Against.Cents - off[0].Cut.Cents; d != 25000000 {
				t.Errorf("difference is %d cents, want 25000000", d)
			}
		})
	}
}

// TestAnExceptionGoesRed is the mutation for the exception channel. Each arm
// removes one guarantee and reads what Reconcile then says; an arm that stayed
// green would be a way a declaration could excuse a figure it does not
// describe.
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
		for _, want := range []string{e.Name, "$26544516.00", "$26544515.00", "delete the exception"} {
			if !strings.Contains(r.Findings[0], want) {
				t.Errorf("finding does not say %q:\n  %s", want, r.Findings[0])
			}
		}
		if len(r.Excused) != 0 {
			t.Errorf("a refused exception was also held apart: %v", r.Excused)
		}
	})

	t.Run("a mistyped residual is refused before the store is consulted", func(t *testing.T) {
		e := exceptionNamed(t, "p76-lists-no-transfer-to-the-cip-2026-non-major")
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

	t.Run("an exception whose two sides do not differ is refused at declaration", func(t *testing.T) {
		e := exceptionNamed(t, "p0067-internal-service-is-250000-high-by-fund-group")
		e.Cells[0].Cut = e.Cells[0].Against
		err := structure.ValidateExceptions([]structure.Exception{e})
		if err == nil || !strings.Contains(err.Error(), "needs no exception") {
			t.Fatalf("ValidateExceptions = %v, want a refusal", err)
		}
	})
}

// TestAPairIsComparedOnlyWhereTheLatticeSaysHow pins the refusals the six
// cuts produce, each a claim about the documents rather than a gap: the
// division/department vocabulary split on the one pair that reaches it, and
// the pair with no reference in it.
func TestAPairIsComparedOnlyWhereTheLatticeSaysHow(t *testing.T) {
	facts := committedFacts(t)

	t.Run("pp.167-170 name divisions and pp.171-176 name departments", func(t *testing.T) {
		_, err := structure.Compare(facts, cutNamed(t, "general-fund-departments"), cutNamed(t, "funding-sources"))
		if err == nil || !strings.Contains(err.Error(), `"division" tier`) ||
			!strings.Contains(err.Error(), `"department" tier`) {
			t.Fatalf("Compare = %v, want the tier mismatch refused by name", err)
		}
		// THE MUTATION: declare both at one tier and the comparison runs and
		// reports what the vocabulary split looks like as arithmetic.
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

	// THE PLACEHOLDER DECLARATION IS MEASURED AGAINST THE STORE'S VOCABULARY.
	// Both mutations leave the derived level, the placeholder drop and the
	// declared level exactly as they are, so a validation that only asked
	// Drop(derived) == Level accepted both.
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
}
