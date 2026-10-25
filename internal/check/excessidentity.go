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
// expenditures, per column, at zero tolerance. All three terms are printed.
//
// It witnesses cross-block column agreement, which each block's own total_row
// cannot: a whole column read into the wrong year ties its own block. On
// pp.168-169 it cannot witness all three shifting together (fisc-wiyg).
//
// Where a column also prints its net change (p121), the excess plus the
// transfers, each as printed, must equal it — the flows, which SourcesUses
// does not tie for a scope printing Transfers (out) negative.
//
// A year carrying any of the three sides must carry all three, and an excess
// line at a scope not in excessScopes is a finding: either would otherwise
// shrink the subjects quietly.
type excessOfRevenuesIdentity struct{}

func (*excessOfRevenuesIdentity) meta() meta {
	return meta{id: "excess-of-revenues-identity", tier: 1,
		description: "in the ACFR's ten-year Changes in Fund Balances schedule and its General Fund " +
			"budgetary comparison, total revenues minus total expenditures equals the printed " +
			"excess row, and where a column prints its net change, the excess plus the transfers " +
			"equals it, per column, exactly; an excess line at any other scope is a finding"}
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
	transfers := map[excessColumn]excessSide{}
	change := map[excessColumn]excessSide{}
	excessID := map[excessColumn]string{}
	duplicated := map[excessColumn]bool{}
	unsummed := map[excessColumn]bool{}
	var order []excessColumn
	seen := map[excessColumn]bool{}
	var findings []Finding

	note := func(k excessColumn) {
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}
	add := func(m map[excessColumn]excessSide, k excessColumn, f *fact.Fact) {
		note(k)
		side := m[k]
		side.cents += amount.Cents(f.AmountCents)
		side.rows++
		m[k] = side
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
			add(rev, k, &f)
		case f.Kind == vocab.KindExpenditure:
			add(exp, k, &f)
		case f.Kind == vocab.KindTransferOut && f.Sign != vocab.SignNetted:
			// The flows are summed as printed, which nets only an out leg the
			// page prints negative.
			findings = append(findings, finding(f.ID,
				"%s prints a transfer out as a positive magnitude (sign %q), and the flows "+
					"are summed as printed, so this column's flows are not held to its change", k, f.Sign))
			unsummed[k] = true
		case f.Kind == vocab.KindTransferIn || f.Kind == vocab.KindTransferOut:
			add(transfers, k, &f)
		case f.Category == structure.CategoryFundBalanceChange:
			add(change, k, &f)
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i].String() < order[j].String() })

	complete, flows := 0, 0
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
		switch c := change[k]; {
		case c.rows == 0 && transfers[k].rows > 0:
			findings = append(findings, finding(excessID[k],
				"%s prints transfers and no net change, so the flows have nothing to equal", k))
		case c.rows == 0, unsummed[k]:
		case c.rows > 1:
			findings = append(findings, finding(excessID[k],
				"%s publishes the net change line %d times; the flows have no single "+
					"change to equal", k, c.rows))
		case transfers[k].rows == 0:
			findings = append(findings, finding(excessID[k],
				"%s prints a net change and no transfer line, so the flows cannot be "+
					"held to it", k))
		case excess[k].cents+transfers[k].cents == c.cents:
			flows++
		default:
			findings = append(findings, finding(excessID[k],
				"%s: the excess %s plus %d transfer rows' %s is %s, but the document "+
					"prints a net change of %s — off by %s",
				k, excess[k].cents, transfers[k].rows, transfers[k].cents,
				excess[k].cents+transfers[k].cents, c.cents,
				c.cents-(excess[k].cents+transfers[k].cents)))
		}
	}

	return conclusion{
		subjects: len(order),
		unit:     "columns",
		held: fmt.Sprintf("%d columns of %s, each with total revenues minus total "+
			"expenditures equal to the printed excess to the cent, and %d of them with the excess "+
			"plus transfers equal to the printed net change", complete, strings.Join(excessScopes, " and "), flows),
		nothing:  fmt.Sprintf("no fact is in scope %s", strings.Join(excessScopes, " or ")),
		findings: findings,
	}.result(), nil
}
