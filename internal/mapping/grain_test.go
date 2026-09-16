package mapping

import (
	"strings"
	"testing"
)

// grainRule is one rule in the smallest accepted shape, with the row and
// column lists and the grain line left to each case.
func grainRule(grain, rows, columns string) string {
	return `schema_version: 1
doc_id: grain-doc

rules:
  - id: grain-rule
    kind: revenue
    basis: audited
    scope: grain-scope
` + grain + `    units: dollars
    rows:
` + rows + `
    parts:
      - page: 7
        section: "Header"
        columns:
` + columns + "\n"
}

// TestGrainIsRequiredExactlyWhereARulePublishes is Rule.Grain's two refusals.
//
// A grain is a claim about a table; a rule that reads a table and publishes
// nothing from it -- acfr-p0177-debt-by-type and acfr-p0169-other-financing,
// whose every row is skipped -- makes no claim the store could bear out, so a
// grain there is refused rather than tolerated. Every row skipped, every
// column skipped and every column non-amount are three spellings of the same
// silence, and all three are covered here so that a fourth cannot pass on
// resemblance to one of them.
func TestGrainIsRequiredExactlyWhereARulePublishes(t *testing.T) {
	const grain = "    grain: category\n"
	amountRow := `      - {label: "Taxes", category: taxes/other}`
	skippedRow := `      - {label: "Taxes", skip: true}`
	amountCol := `          - {fiscal_year: 2025}`
	skippedCol := `          - {fiscal_year: 2025, skip: true}`
	ratioCol := `          - {quantity: percentage}`

	cases := []struct {
		name string
		src  string
		want string // "" means the file must parse
	}{
		{"a publishing rule declares its grain", grainRule(grain, amountRow, amountCol), ""},
		{"a publishing rule without one is refused",
			grainRule("", amountRow, amountCol), `grain: is required on a rule that publishes`},
		{"a rule whose every row is skipped may not declare one",
			grainRule(grain, skippedRow, amountCol), `grain: is "category", but every row or every column`},
		{"a rule whose every row is skipped and declares none is accepted",
			grainRule("", skippedRow, amountCol), ""},
		{"a rule whose every column is skipped may not declare one",
			grainRule(grain, amountRow, skippedCol), `grain: is "category", but every row or every column`},
		{"a rule whose every column is a non-amount quantity may not declare one",
			grainRule(grain, amountRow, ratioCol), `grain: is "category", but every row or every column`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(strings.NewReader(tc.src), "grain.yaml")
			if tc.want == "" {
				if err != nil {
					t.Fatalf("parse: %v, want accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parse: %v, want an error containing %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "grain-rule") {
				t.Errorf("the refusal does not name the rule: %v", err)
			}
		})
	}
}
