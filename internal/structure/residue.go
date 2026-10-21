package structure

import (
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// A Residue is a set of facts no cut admits, declared with the reason the
// pages put it outside every decomposition. Every fact is in exactly one cut
// or one residue, and a residue matching no fact is refused.
type Residue struct {
	Scope  string
	Rule   string
	Kind   vocab.Kind
	Reason string
}

// Matches says whether a fact is the one this residue declares.
func (r Residue) Matches(f *fact.Fact) bool { return r.matches(f) }

func (r Residue) matches(f *fact.Fact) bool {
	return f.Scope == r.Scope && f.RuleID == r.Rule && f.Kind == r.Kind
}

// BudgetBookResidue is every declared residue.
func BudgetBookResidue() []Residue {
	return append([]Residue{{
		Scope: ScopeDepartmentwideExpenditures,
		Rule:  "dw-maintenance",
		Kind:  vocab.KindTransferOut,
		Reason: "pp.85-125 print one Transfers Out row, under Maintenance, and no other: 266,798 in " +
			"FY2023-24 actual and a printed dash in every later column. It is one division's " +
			"transfer and not a decomposition of pp.66-67's citywide TRANSFER OUT, so the " +
			"departmentwide cut prints expenditure only and this row sits in no cut",
	}, {
		Scope:  ScopeDebtServiceByIssue,
		Rule:   "debt-service-principal",
		Kind:   vocab.KindExpenditure,
		Reason: debtServiceByIssueResidue,
	}, {
		Scope:  ScopeDebtServiceByIssue,
		Rule:   "debt-service-interest",
		Kind:   vocab.KindExpenditure,
		Reason: debtServiceByIssueResidue,
	}, {
		Scope: ScopeDebtServiceRequirements,
		Rule:  "debt-service-requirements",
		Kind:  vocab.KindExpenditure,
		Reason: "pp.80-81's DEBT SERVICE REQUIREMENTS are payments the city's debt agreements " +
			"schedule through 2055, by fund type and by year or five-year band: projected, " +
			"never an appropriation, and no other schedule prints their axis. Each column " +
			"ties to the schedule's own printed Total",
	}}, append(append(cipProjectListingResidue(), fundBalancesByFundResidue()...),
		acfrBudgetActualResidue()...)...)
}

// acfrBudgetActualResidue is ACFR p121, every kind of every rule reading it.
func acfrBudgetActualResidue() []Residue {
	var out []Residue
	for _, r := range []struct {
		rule  string
		kinds []vocab.Kind
	}{
		{"acfr-p0121-gf-budget-revenues", []vocab.Kind{vocab.KindRevenue}},
		{"acfr-p0121-gf-budget-expenditures", []vocab.Kind{vocab.KindExpenditure}},
		{"acfr-p0121-gf-budget-excess-of-revenues", []vocab.Kind{vocab.KindFundBalance}},
		{"acfr-p0121-gf-budget-other-financing-sources", []vocab.Kind{vocab.KindTransferIn, vocab.KindTransferOut}},
		{"acfr-p0121-gf-budget-fund-balances", []vocab.Kind{vocab.KindFundBalance}},
	} {
		for _, kind := range r.kinds {
			out = append(out, Residue{
				Scope:  ScopeACFRGeneralFundBudgetActual,
				Rule:   r.rule,
				Kind:   kind,
				Reason: acfrBudgetActualResidueReason,
			})
		}
	}
	return out
}

// acfrBudgetActualResidueReason is why ACFR p121 sits in no cut. A comparison
// is held to the coarser side's columns, and no Budget Book schedule prints
// FY2024-25 adopted or actual, so as a cut p121 would hold the Budget Book to
// two columns it never prints.
const acfrBudgetActualResidueReason = "ACFR p121 prints the General Fund's FY2024-25 original " +
	"budget, final budget and actual side by side, on a function axis for expenditure, and no " +
	"Budget Book schedule prints FY2024-25 adopted or actual. Its final budget revenues are the " +
	"Budget Book's FY2024-25 revised General Fund revenues in seven of nine lines, Total Revenues " +
	"148,800,148 in both; property taxes 60,571,418 and use of money and property 7,287,501 are " +
	"p127's 60,621,418 and p129's 7,237,501 with 50,000 moved between them, and its transfers in, " +
	"905,500, are not p192's 914,206. Its actual is p41's FY2024-25 to the $10,000 p41 rounds to, but " +
	"for the fund balances: p41 begins at 92.10 million, as restated, and ends at 87.10, where p121 " +
	"prints 92,047,744 and 87,043,576. " +
	"Its revenue, expenditure and transfer blocks tie to their printed totals in each column read"

// fundBalancesByFundResidue is pp.186-209's Capital Improvement Program
// Funds block, every kind of every rule reading it.
func fundBalancesByFundResidue() []Residue {
	var out []Residue
	for _, rule := range fundBalancesRules(true) {
		for _, kind := range []vocab.Kind{vocab.KindFundBalance, vocab.KindRevenue,
			vocab.KindTransferIn, vocab.KindExpenditure, vocab.KindTransferOut} {
			out = append(out, Residue{
				Scope:  ScopeFundBalancesByFund,
				Rule:   rule,
				Kind:   kind,
				Reason: cipFundsBlockResidue,
			})
		}
	}
	return out
}

// cipFundsBlockResidue is why pp.186-209's CIP funds sit in no cut: they are
// cip-funds' funds, which ValidateOutside lets no second cut carry.
const cipFundsBlockResidue = "pp.186-209's Capital Improvement Program Funds block is p222's money, " +
	"which pp.66-67 total outside the operating budget: its 35 funds' Transfers In are p222's legs fund " +
	"by fund in all three columns p222 prints, 92,968,752, 38,086,737 and 50,762,251 for FY2024-25 " +
	"to FY2026-27, their Revenues p222's grants, 11,339,751, 7,814,799 and 8,226,620, and the " +
	"balance they draw, 356,913, 1,183,087 and 0, p222's fund balance. FY2023-24, which p222 does " +
	"not print, carries 1,963,647 of opening balance and 3,143 of expenses"

// cipProjectListingResidue is pp.224-235, one rule per page pair. Their axis
// is the project, and their budget years are not p222's appropriations: the
// listing's FY2025-26 and FY2026-27 totals, 70,765,450 and 70,559,870, carry
// forward unspent appropriations that p222's 47,084,623 and 58,988,871 do not.
func cipProjectListingResidue() []Residue {
	var out []Residue
	for page := 224; page <= 234; page += 2 {
		out = append(out, Residue{
			Scope: ScopeCIPProjectListing,
			Rule:  fmt.Sprintf("cip-listing-p%04d", page),
			Kind:  vocab.KindExpenditure,
			Reason: "pp.224-235 list CIP spending by project, an axis no other schedule " +
				"has, and their FY2025-26 and FY2026-27 totals include carried-forward " +
				"appropriations, so they are not p222's new appropriations: 70,765,450 and " +
				"70,559,870 against 47,084,623 and 58,988,871. Every fund total ties to " +
				"the project rows above it, and the grand total does in every column but " +
				"FY2025-26 and FY2026-27, where it prints a dollar under them",
		})
	}
	return out
}

// debtServiceByIssueResidue is why pp.80-81 sit in no cut. Their axis is the
// debt issue, which no other schedule prints, and only their FY2024-25 grand
// total is pp.172-183's Debt Services summed over funds, 8,931,434 both ways:
// the budget years differ, chiefly in funds 224 and 402.
const debtServiceByIssueResidue = "pp.80-81 print debt service by issue, an axis no other " +
	"schedule has. Their FY2024-25 grand total, 8,931,434, is the funds' Debt Services on " +
	"pp.172-183 summed, the Interfund Loan's 117,500 being the General Fund's; their " +
	"budget years are not, differing by 119,578 and 110,926, chiefly in fund 224 " +
	"(137,684 and 130,003 more than the HUD Loans) and fund 402 (18,250 and 19,125 " +
	"less than the 2022 COPs). Each column ties to the schedule's own printed Total"

// Covered holds a store to its cuts: every fact is admitted by exactly one
// cut, or by no cut and exactly one declared residue, and every residue
// matches at least one fact. It returns one line per fact or residue at
// fault and the number of facts the residue covers.
func Covered(facts []fact.Fact, cuts []Cut, residue []Residue) (findings []string, uncovered int) {
	matched := make([]int, len(residue))
	for i := range facts {
		f := &facts[i]
		var in []string
		for _, c := range cuts {
			if c.admits(f) {
				in = append(in, c.Name)
			}
		}
		var declared []int
		for j, r := range residue {
			if r.matches(f) {
				declared = append(declared, j)
			}
		}
		switch {
		case len(in) > 1:
			findings = append(findings, fmt.Sprintf("fact %s (scope %q, rule %q, %s) is admitted by %d cuts %v; "+
				"a fact in two cuts is counted twice by any view holding both", f.ID, f.Scope, f.RuleID, f.Kind, len(in), in))
		case len(in) == 1 && len(declared) > 0:
			findings = append(findings, fmt.Sprintf("fact %s (scope %q, rule %q, %s) is admitted by cut %q and "+
				"declared a residue; one of the two is a false claim about the pages", f.ID, f.Scope, f.RuleID, f.Kind, in[0]))
		case len(in) == 0 && len(declared) == 0:
			findings = append(findings, fmt.Sprintf("fact %s (scope %q, rule %q, %s) is admitted by no cut and "+
				"declared no residue, so it is in no comparison and no view and nothing says why",
				f.ID, f.Scope, f.RuleID, f.Kind))
		case len(in) == 0 && len(declared) > 1:
			findings = append(findings, fmt.Sprintf("fact %s is declared a residue %d times", f.ID, len(declared)))
		case len(in) == 0:
			matched[declared[0]]++
			uncovered++
		}
	}
	for j, r := range residue {
		if matched[j] == 0 {
			findings = append(findings, fmt.Sprintf("residue (%s, %s, %s) matches no fact, so it excuses nothing; "+
				"remove it rather than leaving a declaration that has stopped describing the corpus",
				r.Scope, r.Rule, r.Kind))
		}
	}
	return findings, uncovered
}
