package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// cutsTieAlongTheLattice asserts that the Budget Book's schedules are cuts of
// one hierarchy: every finer cut sums to the coarser cut it decomposes, every
// pair that meets below both agrees with the spine at the grain they share,
// and the cells that do not tie are held apart by a declared exception that
// pins both sides to what the pages print.
//
// ONE COMPARISON, DRIVEN OFF THE LATTICE, IN PLACE OF ONE FILE PER PAIR. The
// pairs are not listed here: every two cuts are handed to structure.Compare,
// which relates them the way the declared levels say they stand, and refuses a
// pair it cannot relate with the reason. What is declared is the cuts -- each
// a claim about its pages -- and the exceptions; what is measured is
// everything else.
//
// THE FAILURE THIS EXISTS FOR IS SILENT AND HAS BEEN MEASURED. Each detail
// schedule reproduces a side of pp.66-67 exactly, which is what makes a
// mis-scoped rule invisible: written at the spine's scope, pp.127-140's rows
// land in the spine's own cells and the published General Fund property tax
// doubles with every graph check green, because those checks tie the graph to
// the facts it was built from and not to the document. This check ties the
// facts to each other across schedules, which is the only place the doubling
// shows.
//
// AN EXCEPTION IS NOT A TOLERANCE. Each pins both sides of one cell and names
// the printed figure they differ by, and it is refused when either side moves,
// when the cell stops existing, or when the cell ties; see structure.Exception.
// fisc-av0w's 250,000 is carried that way on two axes, never absorbed into a
// bound that would also absorb a missing fund.
//
// WHAT IT DOES NOT COVER. Two cuts at one level are peers, and whether they
// agree on the cells they share is a different question with a different
// failure mode; Compare refuses them by name and the refusal is reported, not
// hidden. Columns the reference does not publish -- pp.66-67 print no actual or
// revised column -- are gaps in the documents and are named per comparison.
type cutsTieAlongTheLattice struct{}

var _ Check = (*cutsTieAlongTheLattice)(nil)

func (*cutsTieAlongTheLattice) ID() string { return "cuts-tie-along-the-lattice" }
func (*cutsTieAlongTheLattice) Tier() int  { return 1 }
func (*cutsTieAlongTheLattice) Full() bool { return false }
func (*cutsTieAlongTheLattice) Description() string {
	return "every Budget Book cut sums to the coarser cut it decomposes, or agrees with the spine " +
		"at the grain both decompose, to the cent, except the cells a declared exception pins on " +
		"both sides to a printed residual"
}

func (*cutsTieAlongTheLattice) Run(_ context.Context, s *Subject) (Result, error) {
	cuts := structure.BudgetBookCuts()
	exceptions := structure.BudgetBookExceptions()

	var findings []Finding
	if err := structure.ValidateExceptions(exceptions); err != nil {
		findings = append(findings, finding("exceptions", "%v", err))
	}

	// THE CUTS ARE HELD TO THE STORE BEFORE THEY ARE COMPARED. A cut declaring
	// a level its facts do not sit at would be compared at a meet the pages
	// never printed; that is refused by name here rather than reported as a
	// hundred one-sided cells. The rule files are what carry each rule's
	// grain, and a subject with none loaded says so rather than skipping this
	// arm silently.
	levelsChecked := false
	if len(s.Files) > 0 {
		byRule, err := structure.LevelOfRule(s.Facts, s.Files)
		if err != nil {
			findings = append(findings, finding("grain", "%v", err))
		} else {
			levelsChecked = true
			if _, err := structure.ValidateCuts(s.Facts, byRule, cuts); err != nil {
				findings = append(findings, finding("cuts", "%v", err))
			}
		}
	}

	// A CUT NO FACT FALLS IN IS NOT COMPARED, and is named. Over the committed
	// corpus the grain arm above has already gone red on it -- a rule with a
	// grain and no fact -- so this is the fixture's path, where the miniature
	// spine has no schedule behind it, and the summary says which.
	var empty []string
	isEmpty := map[string]bool{}
	for _, c := range cuts {
		none := true
		for i := range s.Facts {
			if s.Facts[i].Scope == c.Scope {
				none = false
				break
			}
		}
		if none {
			empty = append(empty, c.Name)
			isEmpty[c.Name] = true
		}
	}

	var (
		subjects  int
		clauses   []string
		refused   []string
		held      []string
		consulted = map[string]bool{}
	)
	for i, a := range cuts {
		for _, b := range cuts[i+1:] {
			if isEmpty[a.Name] || isEmpty[b.Name] {
				continue
			}
			c, err := structure.Compare(s.Facts, a, b)
			if err != nil {
				refused = append(refused, err.Error())
				continue
			}
			r := structure.Reconcile(c, exceptions)
			for _, name := range r.Consulted {
				consulted[name] = true
			}
			subjects += r.Subjects
			for _, f := range r.Findings {
				findings = append(findings, finding(c.Name(), "%s", f))
			}
			for _, e := range r.Excused {
				held = append(held, fmt.Sprintf("%s holds %s apart on %s (%d cell(s)): %s. Printed: %s (%s)",
					e.Name, structure.Cents(e.Residual), c.Name(), len(e.Cells), e.Reason, e.Printed, e.Bead))
			}
			clause := fmt.Sprintf("%s at %s: %d cells over %s, %d one-sided and agreeing at zero",
				c.Name(), c.At, r.Subjects, joinComma(c.Columns), c.OneSided)
			if n := len(r.Excused); n > 0 {
				clause += fmt.Sprintf(", %d exception(s) held apart and NOT among the %d", n, r.Subjects)
			}
			clauses = append(clauses, clause)
		}
	}

	// EVERY DECLARED EXCEPTION MUST HAVE BEEN CONSULTED BY SOME COMPARISON. One
	// naming a pair no comparison relates -- a cut renamed, a lattice edge
	// removed -- is silently inert while the summary advertises it, which is
	// the shape structure.Reconcile refuses cell by cell and this arm refuses
	// pair by pair.
	for _, e := range exceptions {
		if consulted[e.Name] || isEmpty[e.Cut] || isEmpty[e.Against] {
			continue
		}
		findings = append(findings, finding(e.Name,
			"this exception is declared on %s against %s and no comparison relates that pair, so it "+
				"holds nothing apart; remove it, or declare the cuts so the pair is compared (%s)",
			e.Cut, e.Against, e.Bead))
	}

	sort.Strings(empty)
	summary := fmt.Sprintf("%d cells over %d comparison(s) of %d cut(s), each side summed at the grain "+
		"the pair meets: %s", subjects, len(clauses), len(cuts)-len(empty), strings.Join(clauses, "; "))
	if len(held) > 0 {
		summary += ". Held apart: " + strings.Join(held, "; ")
	}
	if len(refused) > 0 {
		summary += fmt.Sprintf(". %d pair(s) are not comparisons and were refused by name: %s",
			len(refused), strings.Join(refused, "; "))
	}
	if len(empty) > 0 {
		summary += fmt.Sprintf(". %d cut(s) carry no fact and were not compared: %s",
			len(empty), joinComma(empty))
	}
	if !levelsChecked {
		summary += ". No rule file is loaded, so no cut's level was checked against its facts"
	}
	nothing := "no cut but the spine carries a fact, so there is no schedule to decompose"
	if len(empty) == 0 {
		nothing = "the cuts carry facts and no pair of them is a comparison"
	}
	return conclusion{
		subjects: subjects,
		unit:     "cells",
		held:     summary,
		nothing:  nothing,
		findings: findings,
	}.result(), nil
}
