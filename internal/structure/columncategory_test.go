package structure_test

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestAColumnCategoryRuleWithFundRowsIsFundByCategory: a rule whose category
// sits on its columns and whose fund sits on its rows publishes facts on the
// fund_group, fund and category axes, which is the grain it declares.
func TestAColumnCategoryRuleWithFundRowsIsFundByCategory(t *testing.T) {
	f, err := mapping.Load("../mapping/testdata/fund-balances-p186.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rule := &f.Rules[0]
	var values []mapping.Value
	for _, c := range rule.Parts[0].Columns {
		if !c.Skip {
			values = append(values, mapping.Value{Row: rule.Rows[0], Column: c, Page: 186})
		}
	}
	facts, err := fact.FromValues(f, rule, values)
	if err != nil {
		t.Fatalf("FromValues: %v", err)
	}
	byRule, err := structure.LevelOfRule(facts, []*mapping.File{f})
	if err != nil {
		t.Fatalf("LevelOfRule: %v", err)
	}
	if got := byRule[rule.ID]; got != structure.LevelFundByCategory {
		t.Errorf("level = %q, want %q", got, structure.LevelFundByCategory)
	}
}
