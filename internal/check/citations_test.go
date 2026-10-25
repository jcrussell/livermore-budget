package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// citationSubject is the smallest subject fact-citations-are-declared reads: the
// facts, and the rules a counterpart could be declared on.
func citationSubject(rules []mapping.Rule, facts []fact.Fact) *Subject {
	return &Subject{
		Facts: facts,
		Files: []*mapping.File{{DocID: "doc", Rules: rules}},
	}
}

func citeFact(id, rule, label string, kind vocab.Kind, cents int64) fact.Fact {
	return fact.Fact{
		ID: id, DocID: "doc", Page: 41, Offset: 4336, Token: "0.53",
		RuleID: rule, RowLabel: label, Kind: kind, AmountCents: cents,
	}
}

// TestOnePrintedFigureIsTwoFactsOnlyWhenARowDeclaresIt: a shared citation
// passes when one row declares both ends (Budget Book p76) and fails when two
// rules read one anchor (ACFR p41).
func TestOnePrintedFigureIsTwoFactsOnlyWhenARowDeclaresIt(t *testing.T) {
	t.Parallel()
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
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferOut, 54780000),
		},
	}, {
		name:  "two rules over one section anchor is the ACFR p41 hazard",
		rules: []mapping.Rule{{ID: "acfr-fund-balances"}, {ID: "acfr-other-financing"}},
		facts: []fact.Fact{
			citeFact("a", "acfr-fund-balances", "Transfers in", vocab.KindTransferIn, 53000000),
			citeFact("b", "acfr-other-financing", "Transfers in", vocab.KindTransferIn, 53000000),
		},
		wantFail: true,
		wantText: "both publish token",
	}, {
		name:  "one rule twice on a row that declares nothing is still a double-publish",
		rules: noCounterpart,
		facts: []fact.Fact{
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferOut, 54780000),
		},
		wantFail: true,
		wantText: "declares no counterpart",
	}, {
		name:  "a third fact on one figure is not a pair however it is declared",
		rules: withCounterpart,
		facts: []fact.Fact{
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferOut, 54780000),
			citeFact("c", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferIn, 54780000),
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
			citeFact("a", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferIn, 54780000),
			citeFact("b", "p76-transfers", "Transfer From Host Comm Impact", vocab.KindTransferOut, 99900000),
		},
		wantFail: true,
		wantText: "for one printed token",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			res := runCheck(t, &factCitationsAreDeclared{}, citationSubject(tc.rules, tc.facts))
			failed := len(res.Findings) > 0
			if failed != tc.wantFail {
				t.Fatalf("findings = %v, want failure %v (summary: %s)", res.Findings, tc.wantFail, res.Summary)
			}
			if !tc.wantFail {
				return
			}
			if !hasFinding(res, tc.wantText) {
				t.Errorf("findings = %s\nwant one containing %q", said(res), tc.wantText)
			}
		})
	}
}

// TestATransferPrintedAgainstItsKindMustSaySo holds the netted transfer to its
// declaration, and holds the store's negative revenue and fund_balance facts,
// which declare nothing, to passing.
func TestATransferPrintedAgainstItsKindMustSaySo(t *testing.T) {
	t.Parallel()
	f := func(kind vocab.Kind, sign vocab.Sign, cents int64) fact.Fact {
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
			f(vocab.KindTransferOut, vocab.SignPositive, 1014659800), false},
		{"a transfer_out printed parenthesised and declaring it, the ACFR convention",
			f(vocab.KindTransferOut, vocab.SignNetted, -2572000000), false},
		{"a transfer_out printed parenthesised and NOT declaring it is fisc-fdxx",
			f(vocab.KindTransferOut, vocab.SignPositive, -2572000000), true},
		{"a transfer_in declaring netted while running with its kind is the reverse error",
			f(vocab.KindTransferIn, vocab.SignNetted, 54780000), true},
		// A transfer fact is zero where a column prints "-". Zero runs with
		// every direction, so a netted row's zero cells are not the reverse error.
		{"a netted transfer whose column is zero is not a contradiction",
			f(vocab.KindTransferOut, vocab.SignNetted, 0), false},
		{"a negative revenue is a magnitude, not an orientation: p127's (20,033)",
			f(vocab.KindRevenue, vocab.SignPositive, -2003300), false},
		{"a negative fund_balance is a balance that fell, and the spine carries six",
			f(vocab.KindFundBalance, vocab.SignPositive, -103415400), false},
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

// TestTheOrientationSummaryDoesNotCountAZeroAsNetted pins the summary fisc
// verify prints: a zero declares no direction, so it is not counted as netted.
func TestTheOrientationSummaryDoesNotCountAZeroAsNetted(t *testing.T) {
	t.Parallel()
	nettedAt := func(cents int64) fact.Fact {
		return fact.Fact{
			ID: "f", DocID: "doc", Page: 41, Token: "-", RowLabel: "Transfers (out)",
			Kind: vocab.KindTransferOut, Sign: vocab.SignNetted, AmountCents: cents,
		}
	}
	run := func(f fact.Fact) string {
		res, err := (&factTransferOrientationIsDeclared{}).Run(
			t.Context(), &Subject{Facts: []fact.Fact{f}})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Findings) > 0 {
			t.Fatalf("unexpected findings: %v", res.Findings)
		}
		return res.Summary
	}
	if got := run(nettedAt(0)); !strings.Contains(got, "1 transfer facts, 0 printed against") {
		t.Errorf("summary for a zero-valued netted transfer = %q\n"+
			"want it to count 0 as printed against its kind's direction", got)
	}
	if got := run(nettedAt(-2572000000)); !strings.Contains(got, "1 transfer facts, 1 printed against") {
		t.Errorf("summary for a negative netted transfer = %q\nwant it to count 1", got)
	}
}
