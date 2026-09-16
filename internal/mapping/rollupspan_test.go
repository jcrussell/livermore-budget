package mapping

import (
	"errors"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// rollupSpanFile is two rules that agree on everything a rollup already
// compares -- units, and Parts[0].Columns byte for byte -- so the only thing
// left to disagree about is what this file tests.
//
// IT IS SYNTHETIC ON PURPOSE, and that is a finding rather than a convenience.
// No mutation of mappings/livermore-budget-fy2026-2027.yaml can isolate either
// guard, measured both ways: adding any rule to a covers: list breaks
// CheckRollup's arithmetic ("the 11 covered rules state $142,177,200.00, the
// document states $142,028,002.00"), and pointing a dept-* rollup at a funding-*
// rule is refused by the existing column-equality guard, because div-* columns
// carry {fund_group, fund} and funding-* columns carry none. Either way the
// mutation goes red on a gate that is not the one under test -- green because
// the gate fired, read from the other side.
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

// TestARollupMayNotCoverTwoScopes is the refusal, and it is refused rather than
// declared because no printed line in this corpus wants the shape.
//
// A rollup summing a detail-scope total into a spine-scope one is fisc-u2v's
// doubling raised to the rollup: pp.85-125 and pp.66-67 are the same expenditure
// counted two ways, so the sum adds up and misstates the city. A tie there says
// the arithmetic worked.
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
		// THE HINT IS WHERE THE REASON LIVES, and it is asserted separately
		// because ErrHint keeps it out of Error() -- a message that read
		// "scope X and scope Y" with no reason attached would tell an author
		// what the parser noticed and not why it is refused.
		var h *cmdutil.ErrHint
		if !errors.As(err, &h) {
			t.Fatalf("got %T, want an *ErrHint saying why two scopes are refused", err)
		}
		if !strings.Contains(h.Hint, "counted twice") {
			t.Errorf("hint = %q, want it to name the double count", h.Hint)
		}
	})
}

// TestARollupSpanningTwoKindsMustSaySo is the opposite answer to the scope
// refusal above, and the asymmetry is the whole of fisc-qj4.
//
// p140 prints "Total Sources", which is literally revenue plus transfers in --
// a real printed line over two kinds. Refusing mixed kinds outright would choose
// a rule the corpus has not asked for. So the rule file must state it.
//
// THE DECLARATION CANNOT CATCH A WRONG KIND and this test does not pretend it
// can: it is required to EQUAL what the covered rules carry, so it can only
// restate them. What goes red without it is silence.
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

// TestKindsOnAnUnassertableRollupIsRefused closes the one place the field could
// be typed and read by nothing.
//
// validateRollups short-circuits on `if declines { continue }` before reaching
// validateRollupKinds, so `kinds:` beside `unassertable:` was accepted in
// silence. That is not a hypothetical corner: the corpus's only unassertable
// rollup is p140's "Total Sources", which is revenue plus transfers in -- the
// mixed-kind line Rollup.Kinds' own doc comment is built around. The most
// likely place for someone to type the field was the one place nothing read it.
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
	var h *cmdutil.ErrHint
	if !errors.As(err, &h) || !strings.Contains(h.Hint, "covers none") {
		t.Errorf("hint = %+v, want it to say an unassertable rollup covers no rules", h)
	}
}

// TestTheCommittedRollupsSpanOneKindAndOneScope is the measurement the two
// guards land green against, kept so that a rule file edit which quietly makes a
// rollup mixed shows up here as well as at the parse.
func TestTheCommittedRollupsSpanOneKindAndOneScope(t *testing.T) {
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load(%s): %v", publishedSpine, err)
	}
	byID := map[string]*Rule{}
	for i := range f.Rules {
		byID[f.Rules[i].ID] = &f.Rules[i]
	}
	assertable := 0
	for i := range f.Rollups {
		ro := &f.Rollups[i]
		if len(ro.Covers) == 0 {
			continue
		}
		assertable++
		kinds, scopes := map[Kind]bool{}, map[string]bool{}
		for _, id := range ro.Covers {
			kinds[byID[id].Kind] = true
			scopes[byID[id].Scope] = true
		}
		if len(kinds) != 1 || len(scopes) != 1 {
			t.Errorf("rollup %q spans %d kind(s) and %d scope(s); if that is now "+
				"correct, declare kinds: and read the scope refusal again",
				ro.ID, len(kinds), len(scopes))
		}
		if len(ro.Kinds) != 0 {
			t.Errorf("rollup %q declares kinds: over covered rules that agree, "+
				"which the parser refuses", ro.ID)
		}
	}
	if assertable != 13 {
		t.Errorf("%d assertable rollups, want 13; one of the 14 is unassertable", assertable)
	}
}
