package structure

import (
	"fmt"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// A Restatement declares that a residue prints, line by line, the money of a
// cut the reference puts outside its totals. The lattice compares an Outside
// cut with no other, and a residue is in no cut, so without this nothing in
// fisc verify relates the two.
type Restatement struct {
	Name string
	// Scope and Rules select the residue's facts; every (rule, kind) a line
	// reads must be a declared Residue.
	Scope string
	Rules []string
	// Against is the Outside cut whose money the residue restates. Only the
	// columns it prints are compared.
	Against string
	// At is the level both sides are summed to.
	At    Level
	Lines []RestatedLine
	// Unrestated are the residue's lines Against prints no counterpart of. A
	// residue fact on neither a restated line nor one of these refuses the
	// hold, so a line the residue adds is not left out unseen.
	Unrestated []Line
	// Reason is why the residue is the Against cut's money, about the pages.
	Reason string
}

// A RestatedLine is one of Against's lines and the residue's lines that sum
// to it: Plus less Minus.
type RestatedLine struct {
	Plus, Minus []Line
	Against     Line
}

// BudgetBookRestatements is every declared restatement.
func BudgetBookRestatements() []Restatement {
	return []Restatement{{
		Name:    "the-cip-funds-block-is-p222s",
		Scope:   ScopeFundBalancesByFund,
		Rules:   fundBalancesRules(true),
		Against: "cip-funds",
		At:      LevelFund,
		Lines: []RestatedLine{
			{Plus: []Line{{vocab.KindTransferIn, "transfers/in"}}, Against: Line{vocab.KindTransferIn, "transfers/in"}},
			{Plus: []Line{{vocab.KindRevenue, "revenues"}}, Against: Line{Kind: vocab.KindRevenue}},
			// p222 prints the balance a CIP fund draws, positive; pp.186-209
			// print its opening and closing balances.
			{Plus: []Line{LineBeginning}, Minus: []Line{LineEnding},
				Against: Line{vocab.KindFundBalance, "fund-balance/use-for-cip"}},
		},
		// p222 prints what a CIP fund receives and the balance it draws; what
		// it spends and transfers on is not on p222 fund by fund.
		Unrestated: []Line{
			{vocab.KindExpenditure, "expenses"},
			{vocab.KindTransferOut, "transfers/out"},
			{vocab.KindTransferOut, "transfers/out-to-cip"},
			{vocab.KindFundBalance, CategoryFundBalanceReserveIncrease},
		},
		Reason: cipFundsBlockResidue,
	}}
}

// ValidateRestatements holds declarations to the cuts and the residue: the
// Against cut is declared and Outside, the level is one Against refines, and
// every line reads only declared residue and a kind Against prints.
func ValidateRestatements(cuts []Cut, residue []Residue, rs []Restatement) error {
	byName := map[string]Cut{}
	for _, c := range cuts {
		byName[c.Name] = c
	}
	for _, r := range rs {
		against, ok := byName[r.Against]
		switch {
		case !ok:
			return fmt.Errorf("restatement %q: %q is not a declared cut", r.Name, r.Against)
		case against.Outside == "":
			return fmt.Errorf("restatement %q: %q is not declared Outside, so the lattice already "+
				"compares it", r.Name, r.Against)
		case !Declared(r.At) || !Refines(against.Level, r.At):
			return fmt.Errorf("restatement %q: level %q is not one %q (%s) can be summed to",
				r.Name, r.At, r.Against, against.Level)
		case len(r.Rules) == 0 || len(r.Lines) == 0:
			return fmt.Errorf("restatement %q declares no rules or no lines", r.Name)
		}
		for _, l := range r.Lines {
			if !containsKind(against.Kinds, l.Against.Kind) {
				return fmt.Errorf("restatement %q: %q prints no %s", r.Name, r.Against, l.Against.Kind)
			}
			for _, side := range append(slices.Clone(l.Plus), l.Minus...) {
				for _, rule := range r.Rules {
					declared := slices.ContainsFunc(residue, func(d Residue) bool {
						return d.Scope == r.Scope && d.Rule == rule && d.Kind == side.Kind
					})
					if !declared {
						return fmt.Errorf("restatement %q reads %s of rule %s, which is not declared residue",
							r.Name, side, rule)
					}
				}
			}
		}
	}
	return nil
}

// HoldRestatement compares each line of a restatement in every column Against
// prints, one Comparison per line, named for the restatement and the line.
func HoldRestatement(facts []fact.Fact, cuts []Cut, r Restatement) ([]Comparison, error) {
	var against Cut
	for _, c := range cuts {
		if c.Name == r.Against {
			against = c
		}
	}
	if against.Name == "" {
		return nil, fmt.Errorf("restatement %q: %q is not a declared cut", r.Name, r.Against)
	}
	for i := range facts {
		f := &facts[i]
		if f.Scope != r.Scope || !slices.Contains(r.Rules, f.RuleID) || onLine(r.Unrestated, f) {
			continue
		}
		restated := slices.ContainsFunc(r.Lines, func(l RestatedLine) bool {
			return onLine(l.Plus, f) || onLine(l.Minus, f)
		})
		if !restated {
			return nil, fmt.Errorf("restatement %q: fact %s (%s %s) is on no line it restates or "+
				"declares unrestated, so the hold would leave it out unseen", r.Name, f.ID, f.Kind, f.Category)
		}
	}
	// The columns are every one Against prints, on any line: a line's own
	// reference cells would let a column where Against prints nothing on
	// that line drop the residue's figures there unseen.
	columns := map[string]bool{}
	for i := range facts {
		if f := &facts[i]; against.admits(f) {
			columns[KeyOf(f, r.At).Column()] = true
		}
	}
	var out []Comparison
	for _, l := range r.Lines {
		cells, ref := map[Key]Sum{}, map[Key]Sum{}
		add := func(m map[Key]Sum, f *fact.Fact, sign int64) {
			k := KeyOf(f, r.At)
			s := m[k]
			s.Cents += sign * f.AmountCents
			s.Present = true
			m[k] = s
		}
		for i := range facts {
			f := &facts[i]
			if against.admits(f) && l.Against.Matches(f) {
				add(ref, f, 1)
			}
			if f.Scope != r.Scope || !slices.Contains(r.Rules, f.RuleID) {
				continue
			}
			switch {
			case onLine(l.Plus, f):
				add(cells, f, 1)
			case onLine(l.Minus, f):
				add(cells, f, -1)
			}
		}
		c := Comparison{
			Cut:      Cut{Name: fmt.Sprintf("%s (%s)", r.Name, l.Against)},
			Against:  against,
			Relation: Agreement,
			At:       r.At,
		}
		c.tally(cells, ref, func(k Key) bool { return columns[k.Column()] })
		out = append(out, c)
	}
	return out, nil
}
