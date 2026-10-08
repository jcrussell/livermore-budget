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
// not a skip, unless its rule declares that cell blank, and
// structure.Balance.BlankExemptsSourcesUses says which blanks set the balance aside. Two
// facts on one line of a balance are a finding, and the balance is not held;
// nor is one printing a figure on a line its rule declares blank, which
// fund-balance-identity reports.
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
	// fund-balance-identity reports what declaredBlanks finds.
	read := declaredBlanks(s, declared)
	blanks := read.lines
	var findings []Finding
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
		l, onLine := decl.LineOf(f)
		// A figure on a declared-blank line is fund-balance-identity's one
		// finding; its balance is set aside below, so it is read here as
		// nothing, not as a term or a second figure.
		if onLine && blanks[k][l] {
			continue
		}
		// SourcesUses.Add sums what it is given, so a second fact on a line
		// would be added to the first.
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
	exempted, setAside := 0, 0
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
				blank = blank || decl.BlankExemptsSourcesUses(l)
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
		// A balance printing a figure on a declared-blank line still answers
		// for its missing lines above; it has no single value for that line,
		// so sources = uses is not held of it.
		if read.contradicted[k] {
			setAside++
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
		// it pins, and is no balance that holds. So is one a declared blank
		// exempts: every line it prints was held to its scope's declaration.
		subjects: len(sides) + exempted + setAside,
		unit:     "balances",
		held: fmt.Sprintf("%d balances, %d of them with sources minus uses equal to its change, %d held "+
			"apart by declared exceptions, %d exempted by declared blank cells, and %d set aside for "+
			"printing a figure on a declared-blank line",
			len(sides)+exempted+setAside, holding, heldApart, exempted, setAside),
		nothing:  "no balance prints its sources and uses",
		findings: findings,
	}.result(), nil
}
