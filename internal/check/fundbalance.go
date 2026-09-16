package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
)

// categoryFundBalanceChange is the third line this identity is over. The other
// two come from internal/check/graph.go, which already declared them: this file
// first shipped a second spelling of both, one concept with two names in one
// package.
//
// THE CHANGE LINE STAYS HERE AND IS NOT ADDED TO THAT BLOCK, which is where the
// de-duplication first put it and where it falsified three sentences at once.
// graph.go introduces its constants as "the two rows pp.66-67 print that are
// STOCKS rather than flows ... the projection records the facts and draws no
// link", and as a restatement of "two unexported constants in internal/project".
// fund-balance/change is none of those: it is a flow, the projection DOES draw a
// link for it, and graph.go's own stock predicate excludes it. Three constants
// under a comment saying two, one of them the opposite of what it describes.
const categoryFundBalanceChange = "fund-balance/change"

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

// fundBalanceIdentity asserts that a fund balance's three published lines agree:
// beginning + change == ending, at zero tolerance.
//
// WHY IT IS A CHECK AND NOT A TEST. Both sides are in the fact store, and the
// arithmetic is the DOCUMENT's rather than ours -- the city prints all three
// lines and never asks a reader to add them. So this is the store checking
// itself against a figure the city published independently of the other two,
// which is the same species of claim as the detail-ties-to-spine checks and a
// different one from CheckTotals, which ties rows to a total on their own page
// at build time.
//
// WHAT IT IS NOT. internal/mapping's TestPublishedSpineBalancesPerFundGroup
// covers the RESIDUAL identity -- revenue + transfers in - expenditure -
// transfers out - reserve increase == change -- and pins both sides to pp.66-67's
// printed TOTAL SOURCES and TOTAL USES. That says nothing about beginning and
// ending, which are two more printed cells on the same rows. fisc-7m2 exists
// because the two were being conflated, and the two claims are kept apart here so
// a failure says which one broke.
//
// A GROUP CARRYING ANY OF THE THREE MUST CARRY ALL THREE, and that arm is the
// reason this check can see a dropped row rather than only a wrong one. Skipping
// an incomplete group would fail open in exactly the direction that matters: a
// rule that stopped publishing its `change` line would leave the identity with
// nothing to violate, and this check would go quietly from thirteen subjects to
// twelve. Measured over the committed corpus, all thirteen groups are complete.
type fundBalanceIdentity struct{}

var _ Check = (*fundBalanceIdentity)(nil)

func (*fundBalanceIdentity) ID() string { return "fund-balance-identity" }
func (*fundBalanceIdentity) Tier() int  { return 1 }
func (*fundBalanceIdentity) Full() bool { return false }
func (*fundBalanceIdentity) Description() string {
	return "every published fund balance satisfies beginning + change == ending, exactly"
}

// fundBalanceKey is one balance: one fund of one fund group of one document, on
// one (fiscal year, basis) column.
//
// The scope is in the key because two scopes may publish the same fund group's
// balance from different schedules, and adding a Budget Book cell to an ACFR one
// would be arithmetic across two documents that nothing licenses.
//
// THE FUND IS IN IT FOR A CASE THE CORPUS DOES NOT YET HAVE, and it is here now
// because getting it wrong later would be silent. Every fund-balance fact today
// carries no fund -- the spine and ACFR p41 both publish per fund GROUP -- but
// pp.68-75 print a balance per FUND, and without this field funds 100 and 101 of
// one group would collapse onto one key, be reported as a spurious duplicate,
// and BOTH be excluded from the identity. A check that quietly stops examining
// the rows a coverage lane just added is the failure this whole file is about.
//
// The fund is fact.FundString's rendering rather than the fact's pointer,
// because a pointer keys a map by address and two facts naming fund 100 would
// be two balances.
type fundBalanceKey struct {
	docID      string
	scope      string
	fundGroup  string
	fund       string
	fiscalYear int
	basis      string
}

func (k fundBalanceKey) String() string {
	group := k.fundGroup
	if group == "" {
		group = "(no fund group)"
	}
	if k.fund != fact.FundString(nil) {
		group = fmt.Sprintf("%s fund %s", group, k.fund)
	}
	return fmt.Sprintf("%s %s %s FY%d %s", k.docID, k.scope, group, k.fiscalYear, k.basis)
}

// balance is the three lines of one fund balance, as collected from the store.
type balance struct {
	amounts    map[string]amount.Cents
	ids        map[string]string
	duplicates []string
}

// subject names the fact a finding about this balance should address.
//
// IT IS ONE METHOD AND NOT A LOOP WRITTEN TWICE, which is the whole reason it
// exists. Picking the id by ranging b.ids was a defect -- `fisc verify --json`
// gave a different subject on different runs over one unchanged corpus -- and it
// was fixed in the missing-lines arm; the duplicate arm added by the same review
// pass then reintroduced it in a second form, hard-coding the BEGINNING line's
// id, which is "" on a balance that has no beginning line. Two arms, two ways to
// get one decision wrong, and the second was found only by a later pass.
//
// Falling back to the key's description rather than to "" matters: a finding
// whose subject is empty addresses nothing, and the report is what a reader
// greps.
func (b *balance) subject(k fundBalanceKey) string {
	for _, c := range []string{categoryFundBalanceBeginning, categoryFundBalanceChange, categoryFundBalanceEnding} {
		if id, ok := b.ids[c]; ok && id != "" {
			return id
		}
	}
	return k.String()
}

func (*fundBalanceIdentity) Run(_ context.Context, s *Subject) (Result, error) {
	balances := map[fundBalanceKey]*balance{}
	var order []fundBalanceKey

	for _, f := range s.Facts {
		switch f.Category {
		case categoryFundBalanceBeginning, categoryFundBalanceChange, categoryFundBalanceEnding:
		default:
			continue
		}
		k := fundBalanceKey{
			docID: f.DocID, scope: f.Scope, fundGroup: f.FundGroup, fund: fact.FundString(f.Fund),
			fiscalYear: f.FiscalYear, basis: string(f.Basis),
		}
		b := balances[k]
		if b == nil {
			b = &balance{amounts: map[string]amount.Cents{}, ids: map[string]string{}}
			balances[k] = b
			order = append(order, k)
		}
		// Two facts of one category on one balance means the identity has no
		// single answer, and silently keeping the last would pick whichever
		// sorted last.
		//
		// IT IS A FINDING AND NOT AN ERROR. Returning an error here would give
		// the whole check StatusError, so ONE duplicated line anywhere in the
		// corpus would leave all thirteen balances unexamined and would report a
		// defect in the STORE as a failure of the harness -- two different
		// things, and the report tells them apart on purpose. The duplicate is
		// recorded, the balance it belongs to is excluded from the identity, and
		// every other balance is still checked. This is also exactly the shape
		// fisc-2x7y documents for ACFR p41, so it is not hypothetical.
		if prev, dup := b.amounts[f.Category]; dup {
			b.duplicates = append(b.duplicates, fmt.Sprintf(
				"%s twice, as %s and %s", f.Category, prev, amount.Cents(f.AmountCents)))
			continue
		}
		b.amounts[f.Category] = amount.Cents(f.AmountCents)
		b.ids[f.Category] = f.ID
	}

	sort.Slice(order, func(i, j int) bool { return order[i].String() < order[j].String() })

	var findings []Finding
	complete := 0
	for _, k := range order {
		b := balances[k]

		if len(b.duplicates) > 0 {
			findings = append(findings, finding(b.subject(k),
				"%s publishes %s; the identity has no single value to check, so this "+
					"balance is excluded from it",
				k, strings.Join(b.duplicates, " and ")))
			continue
		}

		var missing []string
		for _, c := range []string{categoryFundBalanceBeginning, categoryFundBalanceChange, categoryFundBalanceEnding} {
			if _, ok := b.amounts[c]; !ok {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			findings = append(findings, finding(b.subject(k),
				"%s publishes %d of the three fund-balance lines and is missing %s; "+
					"a balance that publishes any of them must publish all three, or a "+
					"dropped line would leave this identity with nothing to violate",
				k, len(b.amounts), strings.Join(missing, ", ")))
			continue
		}

		complete++
		beginning, change, ending := b.amounts[categoryFundBalanceBeginning],
			b.amounts[categoryFundBalanceChange], b.amounts[categoryFundBalanceEnding]
		if got := beginning + change; got != ending {
			findings = append(findings, finding(b.ids[categoryFundBalanceEnding],
				"%s: beginning %s + change %s = %s, but the document prints an ending "+
					"balance of %s, a difference of %s",
				k, beginning, change, got, ending, ending-got))
		}
	}

	docs := map[string]bool{}
	for _, k := range order {
		docs[k.docID] = true
	}
	return conclusion{
		subjects: len(order),
		unit:     "fund balances",
		held: fmt.Sprintf("%d fund balance(s) across %d document(s), each with all three "+
			"of its beginning, change and ending lines published and beginning + change "+
			"equal to ending to the cent", complete, len(docs)),
		nothing:  "no fact carries a beginning, change or ending fund balance",
		findings: findings,
	}.result(), nil
}
