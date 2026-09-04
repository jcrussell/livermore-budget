package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// citationSubject is the smallest subject fact-citations-are-declared reads: the
// facts, and the rules a counterpart could be declared on.
func citationSubject(rules []mapping.Rule, facts []fact.Fact) *Subject {
	return &Subject{
		Facts: facts,
		Files: []*mapping.File{{DocID: "doc", Rules: rules}},
	}
}

func citeFact(id, rule, label string, kind mapping.Kind, cents int64) fact.Fact {
	return fact.Fact{
		ID: id, DocID: "doc", Page: 41, Offset: 4336, Token: "0.53",
		RuleID: rule, RowLabel: label, Kind: kind, AmountCents: cents,
	}
}

// TestOnePrintedFigureIsTwoFactsOnlyWhenARowDeclaresIt is the guard fisc-2x7y
// asked for, and it is written as a table because the check's whole job is to
// separate one shared citation from another.
//
// The declared case is Budget Book p76's shape -- one rule, one row, two ends of
// one movement -- and the store carries 44 of them, so a check that refused every
// shared citation would redden 88 committed facts. The undeclared case is ACFR
// p41's, where two rules read one section anchor and can publish its single
// printed figure twice with every other check satisfied.
func TestOnePrintedFigureIsTwoFactsOnlyWhenARowDeclaresIt(t *testing.T) {
	withCounterpart := []mapping.Rule{{
		ID: "p76-transfers",
		Rows: []mapping.Row{{
			Label:       "Transfer From Host Comm Impact",
			Counterpart: &mapping.Counterpart{},
		}},
	}}
	noCounterpart := []mapping.Rule{{
		ID:   "p76-transfers",
		Rows: []mapping.Row{{Label: "Transfer From Host Comm Impact"}},
	}}

	for _, tc := range []struct {
		name     string
		rules    []mapping.Rule
		facts    []fact.Fact
		wantFail bool
		wantText string
	}{{
		name:  "a row and its declared counterpart is what the mechanism is for",
		rules: withCounterpart,
		facts: []fact.Fact{
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferOut, 54780000),
		},
	}, {
		name:  "two rules over one section anchor is the ACFR p41 hazard",
		rules: []mapping.Rule{{ID: "acfr-fund-balances"}, {ID: "acfr-other-financing"}},
		facts: []fact.Fact{
			citeFact("a", "acfr-fund-balances", "Transfers in", mapping.KindTransferIn, 53000000),
			citeFact("b", "acfr-other-financing", "Transfers in", mapping.KindTransferIn, 53000000),
		},
		wantFail: true,
		wantText: "both publish token",
	}, {
		name:  "one rule twice on a row that declares nothing is still a double-publish",
		rules: noCounterpart,
		facts: []fact.Fact{
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferOut, 54780000),
		},
		wantFail: true,
		wantText: "declares no counterpart",
	}, {
		name:  "a third fact on one figure is not a pair however it is declared",
		rules: withCounterpart,
		facts: []fact.Fact{
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferOut, 54780000),
			citeFact("c", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferIn, 54780000),
		},
		wantFail: true,
		wantText: "a counterpart pair is two",
	}, {
		// The legs must be the SAME printed figure. Two amounts at one offset
		// means one of them was not read from there, which the counterpart
		// declaration does not license.
		name:  "a declared counterpart carrying a different amount is not one figure",
		rules: withCounterpart,
		facts: []fact.Fact{
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", mapping.KindTransferOut, 99900000),
		},
		wantFail: true,
		wantText: "for one printed token",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (&factCitationsAreDeclared{}).Run(t.Context(), citationSubject(tc.rules, tc.facts))
			if err != nil {
				t.Fatal(err)
			}
			failed := len(res.Findings) > 0
			if failed != tc.wantFail {
				t.Fatalf("findings = %v, want failure %v (summary: %s)", res.Findings, tc.wantFail, res.Summary)
			}
			if !tc.wantFail {
				return
			}
			var got []string
			for _, f := range res.Findings {
				got = append(got, f.Detail)
			}
			if !strings.Contains(strings.Join(got, "\n"), tc.wantText) {
				t.Errorf("findings = %v\nwant one containing %q", got, tc.wantText)
			}
		})
	}
}

// TestATransferPrintedAgainstItsKindMustSaySo pins fisc-fdxx's invariant, and
// pins the RESTRICTION as well as the rule: the last two cases are the negative
// revenue and fund_balance facts the committed store really carries, and a check
// written as "a negative amount must be declared" would redden all sixteen of
// them.
func TestATransferPrintedAgainstItsKindMustSaySo(t *testing.T) {
	f := func(kind mapping.Kind, sign mapping.Sign, cents int64) fact.Fact {
		return fact.Fact{
			ID: "f", DocID: "doc", Page: 41, Token: "(25.72)",
			RowLabel: "Transfers (out)", Kind: kind, Sign: sign, AmountCents: cents,
		}
	}
	for _, tc := range []struct {
		name     string
		fact     fact.Fact
		wantFail bool
	}{
		{"a transfer_out printed as a positive magnitude, the Budget Book convention",
			f(mapping.KindTransferOut, mapping.SignPositive, 1014659800), false},
		{"a transfer_out printed parenthesised and declaring it, the ACFR convention",
			f(mapping.KindTransferOut, mapping.SignNetted, -2572000000), false},
		{"a transfer_out printed parenthesised and NOT declaring it is fisc-fdxx",
			f(mapping.KindTransferOut, mapping.SignPositive, -2572000000), true},
		{"a transfer_in declaring netted while running with its kind is the reverse error",
			f(mapping.KindTransferIn, mapping.SignNetted, 54780000), true},
		{"a negative revenue is a magnitude, not an orientation: p127's (20,033)",
			f(mapping.KindRevenue, mapping.SignPositive, -2003300), false},
		{"a negative fund_balance is a balance that fell, and the spine carries six",
			f(mapping.KindFundBalance, mapping.SignPositive, -103415400), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (&factTransferOrientationIsDeclared{}).Run(
				t.Context(), &Subject{Facts: []fact.Fact{tc.fact}})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(res.Findings) > 0; got != tc.wantFail {
				t.Errorf("findings = %v, want failure %v (summary: %s)", res.Findings, tc.wantFail, res.Summary)
			}
		})
	}
}
