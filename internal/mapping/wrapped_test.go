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

// TestAWrappedLabelBeforeTheFirstRowIsDeclarable is fisc-2jk, and it is the one
// shape a fragment used to be invisible in EITHER direction.
//
// checkGap's i == 0 branch admitted the text before the first mapped row
// without consulting WrappedLabels and without marking anything used. So a
// label wrapping there could not be declared -- declaring it failed as a stale
// declaration, "is declared but does not appear between this part's rows",
// because only the between-rows and after-last-row paths marked a fragment
// used -- and not declaring it passed silently. Neither answer states what the
// page does.
//
// The page is the same real block as the tests above, anchored one word
// earlier. pp.167-170 as mapped do not reach this: every wrapping division
// label there puts its head in the section anchor and its tail between two
// rows. Moving the anchor is all it takes.
func TestAWrappedLabelBeforeTheFirstRowIsDeclarable(t *testing.T) {
	early := strings.Replace(p167Innovation,
		`section: "Innovation & Economic "`, `section: "Innovation & "`, 1)

	t.Run("declared, it resolves", func(t *testing.T) {
		src := strings.Replace(early, "        #WRAPPED",
			`        wrapped_labels: ["Economic", "Devel"]`, 1)
		f, err := Parse(strings.NewReader(src), "p167.yaml")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		r, err := NewResolver(budgetDoc(t, 167), f)
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		vals, _, err := r.Values(&f.Rules[0], &f.Rules[0].Parts[0])
		if err != nil {
			t.Fatalf("a leading wrapped label was declared and still failed: %v", err)
		}
		// Eight values, the same two rows by four columns the later anchor
		// gives: declaring the fragment must not change what is read.
		if len(vals) != 8 {
			t.Errorf("got %d values, want 8", len(vals))
		}
	})

	// And the declaration is still a claim that fails when it stops being true.
	// Admitting the leading gap must not make this one corner permanently
	// unfalsifiable, which is the failure mode the fix could have introduced.
	t.Run("stale, it is still refused", func(t *testing.T) {
		src := strings.Replace(early, "        #WRAPPED",
			`        wrapped_labels: ["Economic", "Devel", "Nowhere On This Page"]`, 1)
		f, err := Parse(strings.NewReader(src), "p167.yaml")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		r, err := NewResolver(budgetDoc(t, 167), f)
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		if _, _, err := r.Values(&f.Rules[0], &f.Rules[0].Parts[0]); err == nil {
			t.Fatal("a fragment the page never prints was accepted")
		} else if !strings.Contains(err.Error(), "Nowhere On This Page") {
			t.Errorf("error does not name the stale declaration: %v", err)
		}
	})
}

// TestWrappedLabelsOnALabelsFromPartAreRefused is fisc-ekj.
//
// A labels_from part takes its row identity positionally from another page, so
// it routes to positionalValues -- which never reads WrappedLabels and never
// staleness-checks it. The declaration was therefore ACCEPTED AND INERT, the
// one shape a declaration in this repository must not have, and it was
// reachable on the shipped rule file rather than only in principle: five
// production parts take labels_from, p67 among them.
func TestWrappedLabelsOnALabelsFromPartAreRefused(t *testing.T) {
	const src = `schema_version: 1
doc_id: labelsfrom-doc

rules:
  - id: two-part
    kind: expenditure
    basis: adopted
    scope: all-funds-gross
    units: dollars
    rows:
      - {label: "Wages", category: wages-and-benefits}
    parts:
      - page: 1
        section: "Division One"
        stop_at: "END"
        columns:
          - {fund_group: general, fiscal_year: 2026}
      - page: 2
        labels_from: 1
        section: "CONTINUED"
        stop_at: "END"
        wrapped_labels: ["Anything At All"]
        columns:
          - {fund_group: general, fiscal_year: 2026}
`
	_, err := Parse(strings.NewReader(src), "labelsfrom.yaml")
	if err == nil {
		t.Fatal("wrapped_labels was accepted on a labels_from part, where nothing reads it")
	}
	for _, want := range []string{"wrapped_labels", "labels_from", "page 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}
