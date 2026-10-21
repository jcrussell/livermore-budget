package check

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// categoryExcessOfRevenues is the printed subtotal this identity recomputes.
const categoryExcessOfRevenues = "fund-balance/excess-of-revenues"

// excessOfRevenuesIdentity asserts that in each of excessScopes, total revenues
// minus total expenditures equals the printed Excess of Revenues over (under)
// expenditures, per column, at zero tolerance.
//
// This is the schedule's corroboration tie, and it is a check rather than a
// build-time total because all three terms are published facts: the city prints
// the excess independently of the detail rows it summarises, so the store is
// checked against a figure the document states rather than against our own sum.
// Measured off p168 before the check was written: the identity holds in all ten
// columns, 2016's 149,526,096 - 114,652,048 = 34,874,048 through 2025's
// 221,212,743 - 186,516,649 = 34,696,094. ACFR p121 holds it in all three
// columns, the original budget's 146,975,502 - 144,228,303 = 2,747,199 among
// them.
//
// WHAT IT WITNESSES that the two per-block printed totals cannot: cross-block
// column agreement. Each block's total_row ties a column against its own rows,
// so a whole column read into the wrong year ties anyway; this identity fails
// unless the revenue, expenditure and excess reads agree about which column is
// which. On pp.168-169 it cannot witness all three shifting TOGETHER — no
// geometry guard exists on a bare-year-headed page (fisc-wiyg); p121 declares
// column_headers, so its guard does. It cannot witness an
// amount that is wrong against the page, which is fact-offset-points-at-token's
// claim, not this one's.
//
// A YEAR CARRYING ANY OF THE THREE SIDES MUST CARRY ALL THREE, the same
// fail-closed arm as fundBalanceIdentity: a rule that stopped publishing the
// excess row would otherwise leave the identity nothing to violate, and the
// check would go quietly to fewer subjects. An excess line at a scope not in
// excessScopes is a finding for the same reason.
type excessOfRevenuesIdentity struct{}

var _ Check = (*excessOfRevenuesIdentity)(nil)

func (*excessOfRevenuesIdentity) ID() string { return "excess-of-revenues-identity" }
func (*excessOfRevenuesIdentity) Tier() int  { return 1 }
func (*excessOfRevenuesIdentity) Full() bool { return false }
func (*excessOfRevenuesIdentity) Description() string {
	return "in the ACFR's ten-year Changes in Fund Balances schedule and its General Fund " +
		"budgetary comparison, total revenues minus total expenditures equals the printed " +
		"excess row, per column, exactly"
}

// excessScopes are the schedules printing an excess line, each one fund scope:
// pp.168-169 all governmental funds combined, p121 the General Fund alone.
var excessScopes = []string{structure.ScopeACFRChangesInFundBalances, structure.ScopeACFRGeneralFundBudgetActual}

// excessColumn is one column of a schedule: one (fiscal year, basis) pair at
// one of excessScopes. The fund dimension is deliberately absent — each scope
// is a single fund scope.
type excessColumn struct {
	scope         string
	year, through int
	basis         vocab.Basis
}

func (k excessColumn) String() string {
	return k.scope + " " + fact.BandLabel(k.year, k.through, k.basis)
}

// excessSide is what one column accumulated from one side of the identity.
type excessSide struct {
	cents amount.Cents
	rows  int
}

func (*excessOfRevenuesIdentity) Run(_ context.Context, s *Subject) (Result, error) {
	rev := map[excessColumn]excessSide{}
	exp := map[excessColumn]excessSide{}
	excess := map[excessColumn]excessSide{}
	excessID := map[excessColumn]string{}
	duplicated := map[excessColumn]bool{}
	var order []excessColumn
	seen := map[excessColumn]bool{}
	var findings []Finding

	note := func(k excessColumn) {
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}
	for _, f := range s.Facts {
		if !slices.Contains(excessScopes, f.Scope) {
			if f.Category == categoryExcessOfRevenues {
				findings = append(findings, finding(f.ID,
					"scope %q prints an excess line and is not one of the identity's scopes, "+
						"so nothing recomputes it", f.Scope))
			}
			continue
		}
		k := excessColumn{f.Scope, f.FiscalYear, fact.BandOf(&f), f.Basis}
		switch {
		case f.Category == categoryExcessOfRevenues:
			note(k)
			if prev := excess[k]; prev.rows > 0 {
				// Two excess facts on one column leave the identity without a
				// single right-hand side; the column is excluded, not guessed.
				findings = append(findings, finding(excessID[k],
					"%s publishes the excess line twice, as %s and %s; the identity has "+
						"no single value to check, so this column is excluded from it",
					k, prev.cents, amount.Cents(f.AmountCents)))
				duplicated[k] = true
				continue
			}
			excess[k] = excessSide{amount.Cents(f.AmountCents), 1}
			excessID[k] = f.ID
		case f.Kind == vocab.KindRevenue:
			note(k)
			side := rev[k]
			side.cents += amount.Cents(f.AmountCents)
			side.rows++
			rev[k] = side
		case f.Kind == vocab.KindExpenditure:
			note(k)
			side := exp[k]
			side.cents += amount.Cents(f.AmountCents)
			side.rows++
			exp[k] = side
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i].String() < order[j].String() })

	complete := 0
	for _, k := range order {
		var missing []string
		for _, side := range []struct {
			name string
			rows int
		}{
			{"revenue rows", rev[k].rows},
			{"expenditure rows", exp[k].rows},
			{"the printed excess line", excess[k].rows},
		} {
			if side.rows == 0 {
				missing = append(missing, side.name)
			}
		}
		if len(missing) > 0 {
			subject := excessID[k]
			if subject == "" {
				subject = k.String()
			}
			findings = append(findings, finding(subject,
				"%s carries part of this identity and is missing %s; a column that "+
					"publishes any side must publish all three, or a dropped rule would "+
					"leave the identity nothing to violate", k, strings.Join(missing, ", ")))
			continue
		}
		// A duplicated column still answers for its missing sides above; only
		// the identity, which has no single excess to hold, is not asked of it.
		if duplicated[k] {
			continue
		}
		complete++
		if got := rev[k].cents - exp[k].cents; got != excess[k].cents {
			findings = append(findings, finding(excessID[k],
				"%s: %d revenue rows sum to %s and %d expenditure rows to %s, a difference "+
					"of %s, but the document prints an excess of %s — off by %s",
				k, rev[k].rows, rev[k].cents, exp[k].rows, exp[k].cents,
				rev[k].cents-exp[k].cents, excess[k].cents,
				excess[k].cents-(rev[k].cents-exp[k].cents)))
		}
	}

	return conclusion{
		subjects: len(order),
		unit:     "columns",
		held: fmt.Sprintf("%d columns of %s, each with total revenues minus total "+
			"expenditures equal to the printed excess to the cent", complete, strings.Join(excessScopes, " and ")),
		nothing:  fmt.Sprintf("no fact is in scope %s", strings.Join(excessScopes, " or ")),
		findings: findings,
	}.result(), nil
}
