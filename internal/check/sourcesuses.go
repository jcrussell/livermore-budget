package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
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
// fund-balance-identity reports. Where a scope declares control totals, each
// balance's two sides are also held to the TOTAL SOURCES and TOTAL USES its
// page prints (printedControlTotals), which the identity cannot see when two
// terms move together.
// The id names fund groups because the spine's balances are fund groups;
// pp.186-209's are funds within them, keyed by both.
type fundGroupSourcesEqualUses struct{}

func (*fundGroupSourcesEqualUses) meta() meta {
	return meta{id: "fund-group-sources-equal-uses", tier: 1,
		description: "every fund group or fund balance of a scope printing its sources and uses satisfies revenue + " +
			"transfers in - expenditure - transfers out - reserve increase == its change, printed or ending - " +
			"beginning, exactly, except where a declared exception pins both sides"}
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
	totals, totalFindings := printedControlTotals(s, declared, keys, columns)
	findings = append(findings, totalFindings...)

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
	summary := fmt.Sprintf("%d balances, %d of them with sources minus uses equal to its change, %d held "+
		"apart by declared exceptions, %d exempted by declared blank cells, and %d set aside for "+
		"printing a figure on a declared-blank line",
		len(sides)+exempted+setAside, holding, heldApart, exempted, setAside)
	if totals > 0 {
		summary += fmt.Sprintf("; %d printed control totals, each equal to its side's terms", totals)
	}
	return conclusion{
		// An exception's balance is examined, both its sides compared to what
		// it pins, and is no balance that holds. So is one a declared blank
		// exempts: every line it prints was held to its scope's declaration.
		subjects: len(sides) + exempted + setAside,
		unit:     "balances",
		held:     summary,
		nothing:  "no balance prints its sources and uses",
		findings: findings,
	}.result(), nil
}

// controlTotal is one figure printed on a TotalSources or TotalUses row, and
// where: two rules padding over one row each read it.
type controlTotal struct {
	cents int64
	at    string
}

// printedControlTotals holds each balance of a scope declaring control totals
// to the figures its pages print on those rows, read by label from the cells
// of the scope's own rules, since no rule maps the rows to a fact. The sources
// side is revenue + transfers in, the uses side expenditure + transfers out +
// reserve increase. A balance of a document and scope whose rules were read
// and that has no printed total is a finding, as is a printed total with no
// balance. It returns how many totals agreed.
//
// Mutation: hold the sources side to the TotalUses row and every balance
// whose change is not zero goes red.
func printedControlTotals(s *Subject, declared []structure.Balance, keys []structure.BalanceAt,
	columns map[structure.BalanceAt]*sourcesUsesColumn) (int, []Finding) {
	var findings []Finding
	printed := map[structure.BalanceAt]*[2]*controlTotal{}
	// read is each document and scope whose rules were read: a balance of
	// one no loaded rule prints is held to nothing here.
	type docScope struct{ doc, scope string }
	read := map[docScope]bool{}
	for _, file := range s.Files {
		for i := range file.Rules {
			rule := &file.Rules[i]
			decl, ok := structure.BalanceOf(declared, rule.Scope)
			if !ok || !decl.SourcesUses || decl.TotalSources == "" {
				continue
			}
			// Load gives every file a resolver; a subject built by hand
			// without one reads no page, and its balances face no total.
			r := s.Resolvers[file.Path]
			if r == nil {
				continue
			}
			read[docScope{file.DocID, rule.Scope}] = true
			for j := range rule.Parts {
				cells, err := r.Cells(rule, &rule.Parts[j])
				if err != nil {
					findings = append(findings, finding(rule.ID, "p%d: %v", rule.Parts[j].Page, err))
					continue
				}
				for _, v := range cells {
					side := -1
					switch v.Row.Label {
					case decl.TotalSources:
						side = 0
					case decl.TotalUses:
						side = 1
					}
					if side < 0 || v.Column.Skip {
						continue
					}
					col := v.Row.EffectiveColumn(v.Column)
					var fund *int
					if col.Fund != 0 {
						fund = &col.Fund
					}
					k := structure.BalanceAt{DocID: file.DocID, Scope: rule.Scope, FundGroup: col.FundGroup,
						Fund: fact.FundString(fund), Year: col.FiscalYear, Through: col.FiscalYearThrough,
						Basis: col.EffectiveBasis(rule)}
					if printed[k] == nil {
						printed[k] = &[2]*controlTotal{}
					}
					got := &controlTotal{cents: int64(v.Cents), at: fmt.Sprintf("p%d offset %d", v.Page, v.Offset)}
					if prev := printed[k][side]; prev != nil && *prev != *got {
						findings = append(findings, finding(k.String(), "%q is read as %s at %s and as %s at %s",
							v.Row.Label, amount.Cents(prev.cents), prev.at, amount.Cents(got.cents), got.at))
						continue
					}
					printed[k][side] = got
				}
			}
		}
	}
	agreed := 0
	for _, k := range keys {
		c := columns[k]
		decl, _ := structure.BalanceOf(declared, k.Scope)
		if decl.TotalSources == "" || !read[docScope{k.DocID, k.Scope}] {
			continue
		}
		p := printed[k]
		delete(printed, k)
		if c.twice {
			continue
		}
		for side, terms := range [2]int64{
			c.terms.Revenue + c.terms.TransfersIn,
			c.terms.Expenditure + c.terms.TransfersOut + c.terms.Reserve,
		} {
			label := [2]string{decl.TotalSources, decl.TotalUses}[side]
			switch {
			case p == nil || p[side] == nil:
				findings = append(findings, finding(k.String(), "no %q figure is read for this balance, so "+
					"its terms are held to no printed total", label))
			case p[side].cents != terms:
				findings = append(findings, finding(k.String(), "its terms sum to %s and %q prints %s at %s, "+
					"a difference of %s", amount.Cents(terms), label, amount.Cents(p[side].cents), p[side].at,
					amount.Cents(terms-p[side].cents)))
			default:
				agreed++
			}
		}
	}
	orphans := make([]string, 0, len(printed))
	for k := range printed {
		orphans = append(orphans, k.String())
	}
	sort.Strings(orphans)
	for _, k := range orphans {
		findings = append(findings, finding(k, "prints a control total and carries no fact of sources = uses"))
	}
	return agreed, findings
}
