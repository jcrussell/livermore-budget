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

// TestGrainIsRequiredExactlyWhereARulePublishes is Rule.Grain's two refusals,
// over each of the three ways a rule publishes nothing.
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
		// The part omits the only row that could publish, so the rule
		// publishes no fact, the same as if the row were skipped.
		{"a rule whose only publishing row is omitted may not declare one",
			strings.Replace(grainRule(grain, amountRow, amountCol), "        columns:\n",
				"        omitted_rows: [\"Taxes\"]\n        columns:\n", 1),
			`grain: is "category", but every row or every column`},
		{"a rule whose only publishing row is omitted and declares none is accepted",
			strings.Replace(grainRule("", amountRow, amountCol), "        columns:\n",
				"        omitted_rows: [\"Taxes\"]\n        columns:\n", 1), ""},
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
