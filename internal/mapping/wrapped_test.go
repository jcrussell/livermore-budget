package mapping

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// p167Innovation maps one division of Budget Book p167, chosen because it is
// the smallest real block that cannot be read without a wrapped-label
// declaration. The page prints:
//
//	Innovation & Economic Wages & Benefits   1,028,282  1,068,663 ...
//	Devel
//	                      Services & Supplies  1,610,950  2,592,436 ...
//
// "Devel" is the tail of the division's own name, wrapped onto a line of its
// own, and it lands between the first row's last figure and the second row's
// label -- where checkGap refuses anything that is not whitespace.
const p167Innovation = `schema_version: 1
doc_id: livermore-budget-fy2026-2027

rules:
  - id: gf-innovation
    kind: expenditure
    basis: adopted
    scope: gf-expenditure-detail
    units: dollars
    rows:
      - {label: "Wages & Benefits", category: wages-and-benefits, department: innovation}
      - {label: "Services & Supplies", category: services-and-supplies, department: innovation}
    parts:
      - page: 167
        section: "Innovation & Economic "
        stop_at: "Total"
        #WRAPPED
        columns:
          - {fund_group: general, fiscal_year: 2024, basis: actual}
          - {fund_group: general, fiscal_year: 2025, basis: revised}
          - {fund_group: general, fiscal_year: 2026}
          - {fund_group: general, fiscal_year: 2027}
`

const wrappedDecl = `        wrapped_labels: ["Devel"]`

func innovationValues(t *testing.T, decl string) ([]Value, error) {
	t.Helper()
	src := strings.Replace(p167Innovation, "        #WRAPPED", decl, 1)
	f, err := Parse(strings.NewReader(src), "p167.yaml")
	if err != nil {
		return nil, err
	}
	r, err := NewResolver(budgetDoc(t, 167), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	vals, _, err := r.Values(&f.Rules[0], &f.Rules[0].Parts[0])
	return vals, err
}

// TestWrappedLabelIsReadWhenDeclared is fisc-0cs's acceptance criterion on the
// real page: the block resolves with the correct row count, and the figures are
// the ones the city printed either side of the wrap.
func TestWrappedLabelIsReadWhenDeclared(t *testing.T) {
	vals, err := innovationValues(t, wrappedDecl)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(vals) != 8 {
		t.Fatalf("got %d values, want 8 (two rows x four columns)", len(vals))
	}
	// Wages & Benefits FY2025-26 sits immediately BEFORE the wrap, Services &
	// Supplies FY2025-26 immediately after it. If the fragment were swallowed
	// as a row the second row's figures would shift.
	if got := vals[2].Cents; got != amount.Cents(123350200) {
		t.Errorf("Wages & Benefits FY2026 = %d, want 123350200", got)
	}
	if got := vals[6].Cents; got != amount.Cents(287031000) {
		t.Errorf("Services & Supplies FY2026 = %d, want 287031000", got)
	}
}

// TestUndeclaredWrappedLabelStillFailsClosed is the half that matters more.
// The bead's acceptance criterion is explicit that there must be no relaxation
// of the unmapped-row check for non-continuation text, so the same page with
// no declaration must still refuse.
func TestUndeclaredWrappedLabelStillFailsClosed(t *testing.T) {
	_, err := innovationValues(t, "")
	if err == nil {
		t.Fatal("resolved with no declaration; checkGap must still refuse it")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"Devel", "not mapped", "wrapped_labels"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}

// TestUnrelatedTextIsNotAdmittedByADeclaration: declaring one fragment does not
// open the gap to anything else. A real unmapped row on this page must still
// fail even when the part carries a wrapped_labels list.
func TestUnrelatedTextIsNotAdmittedByADeclaration(t *testing.T) {
	_, err := innovationValues(t, `        wrapped_labels: ["Services & Supplies"]`)
	if err == nil {
		t.Fatal("resolved; a declaration must not admit text other than itself")
	}
	if got := diagnosis(t, err); !strings.Contains(got, "Devel") {
		t.Errorf("error should still name the undeclared fragment:\n%s", got)
	}
}

// TestStaleWrappedLabelIsAnError keeps the declaration honest in the direction
// nothing else would catch: if the page stops wrapping there, the claim has
// become false and must be removed rather than pass silently. Same direction as
// a stated_total_delta that now ties exactly (fisc-2sd).
func TestStaleWrappedLabelIsAnError(t *testing.T) {
	_, err := innovationValues(t, `        wrapped_labels: ["Devel", "Administration"]`)
	if err == nil {
		t.Fatal("resolved; a declared fragment the page does not use must fail")
	}
	got := diagnosis(t, err)
	for _, want := range []string{"Administration", "declared but does not appear"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}
