package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// fundGroupSourcesEqualUses asserts each balance of a scope that prints its
// sources and uses (structure.FundBalances) balances: revenue + transfers in -
// expenditure - transfers out - reserve increase == its change, exactly. The
// change is the printed fund-balance/change where the scope prints one, and
// ending - beginning, two printed figures, where it does not.
//
// A balance carrying any line of its scope and missing another is a finding,
// not a skip, unless its rule declares that cell blank: a blank is absent and
// summed as nothing. A blank change leaves no change to hold, and so does a
// blank stock on a scope whose change is ending - beginning; where the scope
// prints its change, a blank stock still leaves that figure to hold. Two
// facts on one line of a balance are a finding, and the balance is not held.
// The id names fund groups because the spine's balances are fund groups;
// pp.186-209's are funds within them, keyed by both.
type fundGroupSourcesEqualUses struct{}

var _ Check = (*fundGroupSourcesEqualUses)(nil)

func (*fundGroupSourcesEqualUses) ID() string { return "fund-group-sources-equal-uses" }
func (*fundGroupSourcesEqualUses) Tier() int  { return 1 }
func (*fundGroupSourcesEqualUses) Full() bool { return false }
func (*fundGroupSourcesEqualUses) Description() string {
	return "every fund group or fund balance of a scope printing its sources and uses satisfies revenue + " +
		"transfers in - expenditure - transfers out - reserve increase == its change, printed or ending - " +
		"beginning, exactly, except where a declared exception pins both sides"
}

// sourcesUsesColumn is one balance's terms, the fact each line it prints
// carries, and whether a line carries two.
type sourcesUsesColumn struct {
	terms   structure.SourcesUses
	printed map[structure.Line]string
	twice   bool
}

func (*fundGroupSourcesEqualUses) Run(_ context.Context, s *Subject) (Result, error) {
	declared := structure.FundBalances()
	blanks, findings := declaredBlanks(s, declared)
	columns := map[structure.BalanceAt]*sourcesUsesColumn{}
	for i := range s.Facts {
		f := &s.Facts[i]
		decl, ok := structure.BalanceOf(declared, f.Scope)
		if !ok || !decl.SourcesUses {
			continue
		}
		k := structure.BalanceAtOf(f)
		c := columns[k]
		if c == nil {
			c = &sourcesUsesColumn{printed: map[structure.Line]string{}}
			columns[k] = c
		}
		// SourcesUses.Add sums what it is given, so a second fact on a line
		// would be added to the first.
		l, onLine := lineOf(decl, f)
		if prev := c.printed[l]; onLine && prev != "" {
			findings = append(findings, finding(f.ID,
				"%s prints %s twice, as %s and %s; sources = uses has no single value for it, so this "+
					"balance is excluded from it", k, l, prev, f.ID))
			c.twice = true
			continue
		}
		if !c.terms.Add(f) {
			findings = append(findings, finding(f.ID,
				"%s carries kind %q and category %q, which is no term of sources = uses", k, f.Kind, f.Category))
			continue
		}
		if onLine {
			c.printed[l] = f.ID
		}
	}
	keys := make([]structure.BalanceAt, 0, len(columns))
	for k := range columns {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	sides := map[structure.BalanceAt][2]int64{}
	exempted := 0
	for _, k := range keys {
		c := columns[k]
		if c.twice {
			continue
		}
		decl, _ := structure.BalanceOf(declared, k.Scope)
		var missing []string
		blank := false
		for _, l := range decl.Lines {
			switch {
			case c.printed[l] != "":
			case blanks[k][l]:
				if l == structure.LineChange ||
					!decl.PrintsChange() && (l == structure.LineBeginning || l == structure.LineEnding) {
					blank = true
				}
			default:
				missing = append(missing, l.String())
			}
		}
		if len(missing) > 0 {
			findings = append(findings, finding(k.String(),
				"prints a line of sources = uses and is missing %s, so its sources and uses balance "+
					"against less than the page prints", strings.Join(missing, ", ")))
			continue
		}
		if blank {
			exempted++
			continue
		}
		change := c.terms.Ending - c.terms.Beginning
		if decl.PrintsChange() {
			change = c.terms.Change
		}
		sides[k] = [2]int64{c.terms.Net(), change}
	}

	held, stale := structure.HoldBalances(structure.BalanceSourcesUses, sides, s.BalanceExceptions)
	for _, f := range stale {
		findings = append(findings, finding("structure.BalanceExceptions", "%s", f))
	}
	holding, heldApart := 0, 0
	for _, k := range keys {
		d, ok := sides[k]
		net, change := d[0], d[1]
		switch {
		case !ok:
			continue
		case held[k]:
			heldApart++
			continue
		case net == change:
			holding++
			continue
		}
		c := columns[k]
		its := fmt.Sprintf("its %s is %s", project.CategoryFundBalanceChange, amount.Cents(change))
		if decl, _ := structure.BalanceOf(declared, k.Scope); !decl.PrintsChange() {
			its = fmt.Sprintf("its ending %s - beginning %s is %s", amount.Cents(c.terms.Ending),
				amount.Cents(c.terms.Beginning), amount.Cents(change))
		}
		findings = append(findings, finding(k.String(),
			"revenue %s + transfers in %s - expenditure %s - transfers out %s - reserve increase %s "+
				"= %s, and %s (off by %s)",
			amount.Cents(c.terms.Revenue), amount.Cents(c.terms.TransfersIn), amount.Cents(c.terms.Expenditure),
			amount.Cents(c.terms.TransfersOut), amount.Cents(c.terms.Reserve), amount.Cents(net),
			its, amount.Cents(net-change)))
	}
	return conclusion{
		// An exception's balance is examined, both its sides compared to what
		// it pins, and is no balance that holds.
		subjects: len(sides),
		unit:     "balances",
		held: fmt.Sprintf("%d balances, each with sources minus uses equal to its change, %d held "+
			"apart by declared exceptions, and %d exempted by declared blank cells", holding, heldApart, exempted),
		nothing:  "no balance prints its sources and uses",
		findings: findings,
	}.result(), nil
}
