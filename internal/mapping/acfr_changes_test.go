package mapping

import (
	"strings"
	"testing"
)

// ACFR pp.167-169, the first published batch of the Statistical Section
// (fisc-oakx.3). Every test reads the pages THROUGH the published rules, the
// same discipline as acfr_p177_test.go, so this file and the mapping cannot
// become two readings of one page.

// acfrStatResolver loads the published ACFR mapping, finds ruleID, applies
// mutate (the in-memory mutations the proofs below are built on), and resolves
// against the committed pp.167-169 fixtures.
func acfrStatResolver(t *testing.T, ruleID string, mutate func(*Rule)) (*Resolver, *Rule) {
	t.Helper()
	f, err := Load(publishedACFR)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var rule *Rule
	for i := range f.Rules {
		if f.Rules[i].ID == ruleID {
			rule = &f.Rules[i]
		}
	}
	if rule == nil {
		t.Fatalf("%s declares no rule %q", publishedACFR, ruleID)
	}
	if mutate != nil {
		mutate(rule)
	}
	r, err := NewResolver(testDoc(t, acfrFixtures, []int{167, 168, 169}), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, rule
}

// TestACFRStatisticalRulesPublishAndTie pins the committed shape of the batch:
// how many values each rule publishes, and that every rule with a printed
// total ties to it EXACTLY in all ten columns — no declared delta, no
// tolerance. The exact ties are also what pin amountRun's detached-mark
// handling against a real page: p167 prints its totals as "$ 47,139,536" per
// column, so reverting that drop reddens both p167 rules here with "no run of
// 10 consecutive amounts follows the anchor".
func TestACFRStatisticalRulesPublishAndTie(t *testing.T) {
	for _, tc := range []struct {
		ruleID string
		values int
		total  bool
	}{
		{"acfr-p0167-general-fund-balances", 50, true},
		{"acfr-p0167-other-governmental-fund-balances", 40, true},
		{"acfr-p0168-revenues", 90, true},
		{"acfr-p0168-expenditures", 120, true},
		{"acfr-p0168-excess-of-revenues", 10, false},
		{"acfr-p0169-other-financing", 0, false},
	} {
		t.Run(tc.ruleID, func(t *testing.T) {
			r, rule := acfrStatResolver(t, tc.ruleID, nil)
			values, _, err := r.Values(rule, &rule.Parts[0])
			if err != nil {
				t.Fatalf("Values: %v", err)
			}
			if len(values) != tc.values {
				t.Fatalf("published %d values, want %d", len(values), tc.values)
			}
			if !tc.total {
				return
			}
			res, err := r.CheckTotals(rule, &rule.Parts[0])
			if err != nil {
				t.Fatalf("CheckTotals: %v", err)
			}
			if res.Columns != 10 || res.Declared != 0 || res.Tolerated != 0 {
				t.Errorf("columns=%d declared=%d tolerated=%d, want 10 exact ties",
					res.Columns, res.Declared, res.Tolerated)
			}
		})
	}
}

// TestACFRChangesRowQuantityIsLoadBearing is the row-arm mutation proof
// fisc-oakx.2 left owed on the real p169: remove the percentage row's quantity
// and the read goes red on the row's own token, not on some downstream count.
// Read by MESSAGE, deliberately — an error for any other reason would be a
// gate firing elsewhere, which an exit code cannot distinguish.
func TestACFRChangesRowQuantityIsLoadBearing(t *testing.T) {
	r, rule := acfrStatResolver(t, "acfr-p0169-other-financing", func(ru *Rule) {
		for i := range ru.Rows {
			if ru.Rows[i].Label == "percentage of noncapital" {
				ru.Rows[i].Quantity = ""
			}
		}
	})
	_, _, err := r.Values(rule, &rule.Parts[0])
	if err == nil {
		t.Fatal("the percentage row read cleanly as amounts; the quantity is doing nothing")
	}
	if !strings.Contains(err.Error(), `"6.6%"`) {
		t.Fatalf("err = %v, want the row's own token \"6.6%%\" named", err)
	}
}

// TestACFRFundBalancesWrongBlockIsRefused is the wrong-fund proof: p167's two
// blocks print IDENTICAL row labels except that All Other Governmental Funds
// has no Assigned line, so a rule resolved against the wrong block would match
// every label it lists. What refuses it is shape, in both directions.
func TestACFRFundBalancesWrongBlockIsRefused(t *testing.T) {
	t.Run("the aggregate rule over the General Fund block", func(t *testing.T) {
		// The General Fund block prints an Assigned row this rule does not
		// list, so it surfaces as unexplained text between Committed and
		// Unassigned — checkGap's refusal, before any total is consulted.
		r, rule := acfrStatResolver(t, "acfr-p0167-other-governmental-fund-balances",
			func(ru *Rule) {
				ru.Parts[0].Section = "General Fund"
				ru.Parts[0].StopAt = "Total general fund"
			})
		_, _, err := r.Values(rule, &rule.Parts[0])
		if err == nil {
			t.Fatal("the aggregate rule read the General Fund block cleanly")
		}
		if !strings.Contains(err.Error(), "Assigned") ||
			!strings.Contains(err.Error(), "is not mapped") {
			t.Fatalf("err = %v, want the unmapped Assigned row named", err)
		}
	})
	t.Run("the General Fund rule over the aggregate block", func(t *testing.T) {
		r, rule := acfrStatResolver(t, "acfr-p0167-general-fund-balances",
			func(ru *Rule) {
				ru.Parts[0].Section = "All Other Governmental Funds"
				ru.Parts[0].StopAt = "Total all other governmental funds"
			})
		_, _, err := r.Values(rule, &rule.Parts[0])
		if err == nil {
			t.Fatal("the General Fund rule read the aggregate block cleanly")
		}
		if !strings.Contains(err.Error(), `row "Assigned" does not occur`) {
			t.Fatalf("err = %v, want the missing Assigned row named", err)
		}
	})
}
