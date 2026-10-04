package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// categoryExcessOfRevenues is the printed subtotal this identity recomputes.
const categoryExcessOfRevenues = "fund-balance/excess-of-revenues"

// excessOfRevenuesIdentity asserts that in the ACFR's ten-year Changes in Fund
// Balances schedule, total revenues minus total expenditures equals the printed
// Excess of Revenues over (under) expenditures, per column, at zero tolerance.
//
// This is the schedule's corroboration tie, and it is a check rather than a
// build-time total because all three terms are published facts: the city prints
// the excess independently of the 21 detail rows it summarises, so the store is
// checked against a figure the document states rather than against our own sum.
// Measured off p168 before the check was written: the identity holds in all ten
// columns, 2016's 149,526,096 - 114,652,048 = 34,874,048 through 2025's
// 221,212,743 - 186,516,649 = 34,696,094.
//
// WHAT IT WITNESSES that the two per-block printed totals cannot: cross-block
// column agreement. Each block's total_row ties a column against its own rows,
// so a whole column read into the wrong year ties anyway; this identity fails
// unless the revenue, expenditure and excess reads agree about which column is
// which. It cannot witness all three shifting TOGETHER — no geometry guard
// exists on a bare-year-headed page (fisc-wiyg) — and it cannot witness an
// amount that is wrong against the page, which is fact-offset-points-at-token's
// claim, not this one's.
//
// A YEAR CARRYING ANY OF THE THREE SIDES MUST CARRY ALL THREE, the same
// fail-closed arm as fundBalanceIdentity: a rule that stopped publishing the
// excess row would otherwise leave the identity nothing to violate, and the
// check would go quietly from ten subjects to none.
type excessOfRevenuesIdentity struct{}

var _ Check = (*excessOfRevenuesIdentity)(nil)

func (*excessOfRevenuesIdentity) ID() string { return "excess-of-revenues-identity" }
func (*excessOfRevenuesIdentity) Tier() int  { return 1 }
func (*excessOfRevenuesIdentity) Full() bool { return false }
func (*excessOfRevenuesIdentity) Description() string {
	return "in the ACFR's ten-year Changes in Fund Balances schedule, total revenues minus " +
		"total expenditures equals the printed excess row, per column, exactly"
}

// excessColumn is one column of the schedule: one (fiscal year, basis) pair at
// the scope. The fund dimension is deliberately absent — the schedule is all
// governmental funds combined and its facts carry no fund group.
type excessColumn struct {
	year  int
	basis vocab.Basis
}

func (k excessColumn) String() string {
	return project.ChangesScope + " " + fact.ColumnLabel(k.year, k.basis)
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
		if f.Scope != project.ChangesScope {
			continue
		}
		k := excessColumn{f.FiscalYear, f.Basis}
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
		held: fmt.Sprintf("%d columns of %q, each with total revenues minus total "+
			"expenditures equal to the printed excess to the cent", complete, project.ChangesScope),
		nothing:  fmt.Sprintf("no fact is in scope %q", project.ChangesScope),
		findings: findings,
	}.result(), nil
}
