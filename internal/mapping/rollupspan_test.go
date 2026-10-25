package mapping

import (
	"errors"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/hint"
)

// rollupSpanFile is two rules agreeing on units and Parts[0].Columns, so only
// the field under test can disagree. Synthetic: every mutation of the
// published file goes red on CheckRollup's arithmetic or the column guard
// first.
const rollupSpanFile = `schema_version: 1
doc_id: livermore-budget-fy2026-2027
rules:
  - id: alpha
    kind: expenditure
    basis: adopted
    scope: expenditure-by-department
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total Alpha"
    parts:
      - page: 167
        section: "ALPHA"
        stop_at: "BUDGET FY"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2026}
    rows:
      - {label: "Wages", category: wages-and-benefits, department: city-council}
  - id: beta
    kind: #KIND
    basis: adopted
    scope: #SCOPE
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total Beta"
    parts:
      - page: 167
        section: "BETA"
        stop_at: "BUDGET FY"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2026}
    rows:
      - {label: "Supplies", category: services-and-supplies, department: city-council}
rollups:
  - id: alpha-beta-total
    page: 167
    total_row: "ALPHA AND BETA TOTAL"
    covers: [alpha, beta]
#KINDS
`

func parseRollupSpan(t *testing.T, kind, scope, kinds string) error {
	t.Helper()
	src := strings.Replace(rollupSpanFile, "#KIND", kind, 1)
	src = strings.Replace(src, "#SCOPE", scope, 1)
	src = strings.Replace(src, "#KINDS", kinds, 1)
	_, err := parse(strings.NewReader(src), "rollupspan.yaml")
	return err
}

const (
	sameKind  = "expenditure"
	sameScope = "expenditure-by-department"
)

// TestARollupMayNotCoverTwoScopes: pp.85-125 and pp.66-67 count one
// expenditure two ways, so a rollup across scopes would tie and misstate.
func TestARollupMayNotCoverTwoScopes(t *testing.T) {
	t.Run("one scope", func(t *testing.T) {
		if err := parseRollupSpan(t, sameKind, sameScope, ""); err != nil {
			t.Fatalf("refused a rollup whose covered rules share a scope: %v", err)
		}
	})

	t.Run("two scopes", func(t *testing.T) {
		err := parseRollupSpan(t, sameKind, "department-funding-sources", "")
		if err == nil {
			t.Fatal("accepted a rollup covering two scopes")
		}
		for _, want := range []string{
			`rule "alpha" is scope "expenditure-by-department"`,
			`rule "beta" is scope "department-funding-sources"`,
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want it to contain %q", err, want)
			}
		}
		// The reason is in the hint, which ErrHint keeps out of Error().
		var h *hint.ErrHint
		if !errors.As(err, &h) {
			t.Fatalf("got %T, want an *ErrHint saying why two scopes are refused", err)
		}
		if !strings.Contains(h.Hint, "counted twice") {
			t.Errorf("hint = %q, want it to name the double count", h.Hint)
		}
	})
}

// TestARollupSpanningTwoKindsMustSaySo: p140's "Total Sources" is revenue plus
// transfers in, so mixed kinds are declared, not refused. The declaration must
// equal the covered kinds, so it cannot catch a wrong kind, only silence.
func TestARollupSpanningTwoKindsMustSaySo(t *testing.T) {
	for _, tc := range []struct{ name, kind, kinds, want string }{
		{
			name: "mixed kinds and no declaration",
			kind: "revenue", kinds: "",
			want: `is required: the covered rules span "expenditure", "revenue"`,
		}, {
			name: "mixed kinds declared exactly",
			kind: "revenue", kinds: "    kinds: [expenditure, revenue]",
			want: "",
		}, {
			name: "mixed kinds declared partially",
			kind: "revenue", kinds: "    kinds: [revenue]",
			want: `names "revenue" and the covered rules span "expenditure", "revenue"`,
		}, {
			name: "one kind, declared anyway",
			kind: sameKind, kinds: "    kinds: [expenditure]",
			want: `is declared and every covered rule is kind "expenditure"`,
		}, {
			name: "a kind that is not one of the five",
			kind: "revenue", kinds: "    kinds: [banana]",
			want: `"banana" is not one of the five kinds`,
		}, {
			name: "the same kind twice",
			kind: "revenue", kinds: "    kinds: [revenue, revenue]",
			want: `"revenue" is listed twice`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := parseRollupSpan(t, tc.kind, sameScope, tc.kinds)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("refused an exact declaration over two kinds: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestKindsOnAnUnassertableRollupIsRefused: validateRollups skips unassertable
// rollups before validateRollupKinds, so `kinds:` beside one would be read by
// nothing.
func TestKindsOnAnUnassertableRollupIsRefused(t *testing.T) {
	const src = `schema_version: 1
doc_id: livermore-budget-fy2026-2027
rules:
  - id: alpha
    kind: expenditure
    basis: adopted
    scope: expenditure-by-department
    grain: fund-by-department-by-category
    units: dollars
    total_row: "Total Alpha"
    parts:
      - page: 167
        section: "ALPHA"
        stop_at: "BUDGET FY"
        columns:
          - {fund_group: general, fund: 100, fiscal_year: 2026}
    rows:
      - {label: "Wages", category: wages-and-benefits, department: city-council}
rollups:
  - id: total-sources
    page: 140
    total_row: "Total Sources"
    unassertable: >-
      exceeds the pages it closes by ~$57M, differently per column (fisc-wev)
    kinds: [expenditure, revenue]
`
	_, err := parse(strings.NewReader(src), "unassertable.yaml")
	if err == nil {
		t.Fatal("accepted kinds: on a rollup that covers no rules")
	}
	if !strings.Contains(err.Error(), "is declared alongside unassertable") {
		t.Errorf("error = %v, want it to name the pairing it refuses", err)
	}
	var h *hint.ErrHint
	if !errors.As(err, &h) || !strings.Contains(h.Hint, "covers none") {
		t.Errorf("hint = %+v, want it to say an unassertable rollup covers no rules", h)
	}
}
