package check

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// The set of three is exhaustive on purpose.
//
// There is a FOURTH fund_balance category -- fund-balance/reserve-increase, the
// spine's ADDITION TO RESERVES -- and it is NOT a term of this identity. It is a
// movement inside the change, not a fourth line beside beginning and ending.
//
// Measured over the committed store: 12 facts carry it and only TWO are non-zero,
// general FY2026 (4,699,425) and FY2027 (3,332,607) -- p66 prints a dash for
// every other fund group. So summing it in breaks exactly those two of the
// twelve spine balances, not all of them. Two is enough to redden the check and
// enough to make the exclusion worth pinning.
var balanceCategories = []string{
	project.CategoryFundBalanceBeginning, project.CategoryFundBalanceChange, project.CategoryFundBalanceEnding,
}

// fundBalanceIdentity asserts that a fund balance's published lines agree:
// beginning + change == ending at zero tolerance where the scope prints a
// change line, and each year's ending is the next year's beginning.
//
// WHY IT IS A CHECK AND NOT A TEST. Both sides are in the fact store, and the
// arithmetic is the DOCUMENT's rather than ours -- the city prints all three
// lines and never asks a reader to add them. So this is the store checking
// itself against a figure the city published independently of the other two,
// which is the same species of claim as cuts-tie-along-the-lattice and a
// different one from CheckTotals, which ties rows to a total on their own page
// at build time.
//
// WHAT IT IS NOT. structure.SourcesUses is the RESIDUAL identity -- revenue +
// transfers in - expenditure - transfers out - reserve increase == change --
// which fund-group-sources-equal-uses holds, and
// TestTheSpineBalancesAtThePrintedControlTotals pins both sides to pp.66-67's
// printed TOTAL SOURCES and TOTAL USES. That says nothing about beginning and
// ending, which are two more printed cells on the same rows. fisc-7m2 exists
// because the two were being conflated, and the two claims are kept apart here so
// a failure says which one broke. A scope printing no change line
// (structure.FundBalances) has its change held there as ending - beginning, and
// here only its stocks, the absence of a change line, and its carry-forward.
//
// A BALANCE CARRYING ANY OF ITS SCOPE'S LINES MUST CARRY ALL OF THEM, or its
// rule must declare the cell blank, and that arm is the reason this check can
// see a dropped row rather than only a wrong one. Skipping an incomplete balance
// would fail open in exactly the direction that matters: a rule that stopped
// publishing its `change` line would leave the identity with nothing to violate.
// A scope structure.FundBalances does not declare is a finding for the same
// reason.
type fundBalanceIdentity struct{}

var _ Check = (*fundBalanceIdentity)(nil)

func (*fundBalanceIdentity) ID() string { return "fund-balance-identity" }
func (*fundBalanceIdentity) Tier() int  { return 1 }
func (*fundBalanceIdentity) Full() bool { return false }
func (*fundBalanceIdentity) Description() string {
	return "every published fund balance carries the balance lines its scope prints, satisfies " +
		"beginning + change == ending exactly where the scope prints a change, and ends each year " +
		"where the next begins, except where a declared exception pins both sides"
}

// balance is the lines of one fund balance, as collected from the store.
type balance struct {
	amounts    map[string]amount.Cents
	ids        map[string]string
	duplicates []string
}

// subject names the fact a finding about this balance should address,
// falling back to the balance's description: a finding whose subject is
// empty addresses nothing. One method, so every arm picks the same fact.
func (b *balance) subject(k structure.BalanceAt) string {
	for _, c := range balanceCategories {
		if id, ok := b.ids[c]; ok && id != "" {
			return id
		}
	}
	return k.String()
}

func (*fundBalanceIdentity) Run(_ context.Context, s *Subject) (Result, error) {
	declared := structure.FundBalances()
	blanks, findings := declaredBlanks(s, declared)
	balances := map[structure.BalanceAt]*balance{}
	var order []structure.BalanceAt

	for i := range s.Facts {
		f := &s.Facts[i]
		if !slices.Contains(balanceCategories, f.Category) {
			continue
		}
		decl, ok := structure.BalanceOf(declared, f.Scope)
		if !ok {
			findings = append(findings, finding(f.ID,
				"scope %q prints a %s line and structure.FundBalances declares no fund balance for it, "+
					"so no identity holds it", f.Scope, f.Category))
			continue
		}
		if l, ok := lineOf(decl, f); !ok || l.Category != f.Category {
			findings = append(findings, finding(f.ID,
				"%s prints a %s line, and scope %q is declared to print none; its change is ending - "+
					"beginning, two printed figures, and a third would be a second answer",
				structure.BalanceAtOf(f), f.Category, f.Scope))
			continue
		}
		k := structure.BalanceAtOf(f)
		b := balances[k]
		if b == nil {
			b = &balance{amounts: map[string]amount.Cents{}, ids: map[string]string{}}
			balances[k] = b
			order = append(order, k)
		}
		// Two facts of one category on one balance leave the identity no
		// single answer. A finding, not an error: an error would leave every
		// other balance unexamined and report a defect in the store as a
		// failure of the harness. fisc-2x7y is this shape on ACFR p41.
		if prev, dup := b.amounts[f.Category]; dup {
			b.duplicates = append(b.duplicates, fmt.Sprintf(
				"%s twice, as %s and %s", f.Category, prev, amount.Cents(f.AmountCents)))
			continue
		}
		b.amounts[f.Category] = amount.Cents(f.AmountCents)
		b.ids[f.Category] = f.ID
	}

	sort.Slice(order, func(i, j int) bool { return order[i].String() < order[j].String() })

	complete := 0
	for _, k := range order {
		b := balances[k]
		decl, _ := structure.BalanceOf(declared, k.Scope)

		if len(b.duplicates) > 0 {
			findings = append(findings, finding(b.subject(k),
				"%s publishes %s; the identity has no single value to check, so this "+
					"balance is excluded from it",
				k, strings.Join(b.duplicates, " and ")))
			continue
		}

		var required, missing []string
		blank := false
		for _, l := range decl.Lines {
			if !slices.Contains(balanceCategories, l.Category) {
				continue
			}
			required = append(required, l.Category)
			_, printed := b.amounts[l.Category]
			switch {
			case printed:
			case blanks[k][l]:
				blank = true
			default:
				missing = append(missing, l.Category)
			}
		}
		if len(missing) > 0 {
			findings = append(findings, finding(b.subject(k),
				"%s publishes %d of the %d fund-balance lines its scope prints and is missing %s; "+
					"a balance that publishes any of them must publish all, or a dropped line "+
					"would leave this identity with nothing to violate",
				k, len(b.amounts), len(required), strings.Join(missing, ", ")))
			continue
		}
		complete++
		if blank || !decl.PrintsChange() {
			continue
		}
		beginning, change, ending := b.amounts[project.CategoryFundBalanceBeginning],
			b.amounts[project.CategoryFundBalanceChange], b.amounts[project.CategoryFundBalanceEnding]
		if got := beginning + change; got != ending {
			findings = append(findings, finding(b.ids[project.CategoryFundBalanceEnding],
				"%s: beginning %s + change %s = %s, but the document prints an ending "+
					"balance of %s, a difference of %s",
				k, beginning, change, got, ending, ending-got))
		}
	}

	carried, carryFindings := carryForward(balances, order, carriedScopes(s.Facts), balanceExceptions())
	findings = append(findings, carryFindings...)

	docs := map[string]bool{}
	for _, k := range order {
		docs[k.DocID] = true
	}
	return conclusion{
		subjects: len(order),
		unit:     "fund balances",
		held: fmt.Sprintf("%d fund balance(s) across %d document(s), each with every balance line "+
			"its scope prints published or declared blank, beginning + change equal to ending to the "+
			"cent wherever a change is printed, and %d carry-forward(s) each ending where the next "+
			"year begins", complete, len(docs), carried),
		nothing:  "no fact carries a beginning, change or ending fund balance",
		findings: findings,
	}.result(), nil
}

// carryForward holds ending(y) == beginning(y+1) for every series printing
// both, and returns how many it compared, an exception's included.
//
// MEASURED over the committed store, every one ties: the spine's six fund
// groups FY2026 -> FY2027, and nothing else, because ACFR p41 prints one
// audited year and no other scope prints a stock. So the clause covers every
// scope, and a break is declared in structure.BalanceExceptions.
//
// A series printing one year on two bases has no single ending to carry, and
// is a finding rather than a guess at which basis follows which.
func carryForward(balances map[structure.BalanceAt]*balance, order []structure.BalanceAt,
	carried map[string]bool, exceptions []structure.BalanceException) (int, []Finding) {
	var findings []Finding
	years := map[string]map[int][]structure.BalanceAt{}
	var series []string
	for _, k := range order {
		if len(balances[k].duplicates) > 0 {
			continue
		}
		s := k.Series()
		if years[s] == nil {
			years[s] = map[int][]structure.BalanceAt{}
			series = append(series, s)
		}
		years[s][k.Year] = append(years[s][k.Year], k)
	}

	sides := map[structure.BalanceAt][2]int64{}
	next := map[structure.BalanceAt]structure.BalanceAt{}
	for _, s := range series {
		ambiguous := false
		ys := make([]int, 0, len(years[s]))
		for y := range years[s] {
			ys = append(ys, y)
		}
		sort.Ints(ys)
		for _, y := range ys {
			if ks := years[s][y]; len(ks) > 1 {
				ambiguous = true
				findings = append(findings, finding(ks[0].String(),
					"%s prints FY%d on %d bases, so it has no single ending to carry forward", s, y, len(ks)))
			}
		}
		if ambiguous {
			continue
		}
		for _, y := range ys {
			ks := years[s][y]
			nk, ok := years[s][y+1]
			if !ok {
				continue
			}
			ending, okE := balances[ks[0]].amounts[project.CategoryFundBalanceEnding]
			beginning, okB := balances[nk[0]].amounts[project.CategoryFundBalanceBeginning]
			if okE && okB {
				sides[ks[0]] = [2]int64{int64(ending), int64(beginning)}
				next[ks[0]] = nk[0]
			}
		}
	}

	held, stale := structure.HoldBalances(structure.BalanceCarryForward, sides, carried, exceptions)
	for _, f := range stale {
		findings = append(findings, finding("structure.BalanceExceptions", "%s", f))
	}
	keys := make([]structure.BalanceAt, 0, len(sides))
	for k := range sides {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	n := 0
	for _, k := range keys {
		n++
		if held[k] {
			continue
		}
		if d := sides[k]; d[0] != d[1] {
			nk := next[k]
			findings = append(findings, finding(balances[nk].ids[project.CategoryFundBalanceBeginning],
				"%s: %s ends at %s and %s begins at %s, a difference of %s",
				k.Series(), fact.ColumnLabel(k.Year, k.Basis), amount.Cents(d[0]),
				fact.ColumnLabel(nk.Year, nk.Basis), amount.Cents(d[1]), amount.Cents(d[1]-d[0])))
		}
	}
	return n, findings
}
