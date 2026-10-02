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
// summed as nothing, and a blank stock or change leaves no change to hold.
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

// sourcesUsesColumn is one balance's terms and the lines it prints.
type sourcesUsesColumn struct {
	terms   structure.SourcesUses
	printed map[structure.Line]bool
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
			c = &sourcesUsesColumn{printed: map[structure.Line]bool{}}
			columns[k] = c
		}
		if !c.terms.Add(f) {
			findings = append(findings, finding(f.ID,
				"%s carries kind %q and category %q, which is no term of sources = uses", k, f.Kind, f.Category))
			continue
		}
		if l, ok := lineOf(decl, f); ok {
			if blanks[k][l] {
				findings = append(findings, finding(f.ID,
					"%s prints %s, and its rule declares that cell blank", k, l))
			}
			c.printed[l] = true
		}
	}
	keys := make([]structure.BalanceAt, 0, len(columns))
	for k := range columns {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	sides := map[structure.BalanceAt][2]int64{}
	for _, k := range keys {
		c := columns[k]
		decl, _ := structure.BalanceOf(declared, k.Scope)
		var missing []string
		blank := false
		for _, l := range decl.Lines {
			switch {
			case c.printed[l]:
			case blanks[k][l]:
				if l == structure.LineBeginning || l == structure.LineChange || l == structure.LineEnding {
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
			continue
		}
		change := c.terms.Ending - c.terms.Beginning
		if decl.PrintsChange() {
			change = c.terms.Change
		}
		sides[k] = [2]int64{c.terms.Net(), change}
	}

	held, stale := structure.HoldBalances(structure.BalanceSourcesUses, sides, carriedScopes(s.Facts), balanceExceptions())
	for _, f := range stale {
		findings = append(findings, finding("structure.BalanceExceptions", "%s", f))
	}
	checked := 0
	for _, k := range keys {
		d, ok := sides[k]
		if !ok {
			continue
		}
		// An exception's balance is examined: both its sides are compared to
		// what it pins.
		checked++
		if held[k] {
			continue
		}
		if net, change := d[0], d[1]; net != change {
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
	}
	return conclusion{
		subjects: checked,
		unit:     "balances",
		held:     fmt.Sprintf("%d balances, each with sources minus uses equal to its change", checked),
		nothing:  "no balance prints its sources and uses",
		findings: findings,
	}.result(), nil
}
