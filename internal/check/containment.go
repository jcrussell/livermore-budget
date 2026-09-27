package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// cutsTieAlongTheLattice asserts that the published schedules are cuts of one
// hierarchy: every finer cut sums to the coarser cut it decomposes, every pair
// that meets below both agrees with the spine at the grain they share, and the
// cells that do not tie are held apart by a declared exception that pins both
// sides to what the pages print.
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
// revised column -- are gaps in the documents and are named per comparison;
// only a declared structure.Tie holds a detail schedule's actual and revised.
type cutsTieAlongTheLattice struct{}

var _ Check = (*cutsTieAlongTheLattice)(nil)

func (*cutsTieAlongTheLattice) ID() string { return "cuts-tie-along-the-lattice" }
func (*cutsTieAlongTheLattice) Tier() int  { return 1 }
func (*cutsTieAlongTheLattice) Full() bool { return false }
func (*cutsTieAlongTheLattice) Description() string {
	return "every cut sums to the coarser cut it decomposes, or agrees with the spine " +
		"at the grain both decompose, or ties to a schedule printing the same money in every column " +
		"both print, to the cent, except the cells a declared exception pins on both sides to a printed residual"
}

// budgetBookExceptions is a seam so a test can declare an exception the tree
// does not.
var budgetBookExceptions = structure.BudgetBookExceptions

func (*cutsTieAlongTheLattice) Run(_ context.Context, s *Subject) (Result, error) {
	cuts := structure.AllCuts()
	exceptions := budgetBookExceptions()

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
	//
	// A CUT NO FACT FALLS IN IS REFUSED when the rule files are loaded: a cut
	// is a claim about pages, and a store that carries none of them has lost
	// a schedule. Which cuts those are is ValidateCuts' answer, never
	// re-decided by scope. Where ValidateCuts refused or did not run -- a
	// fixture loads no rule file -- the empty cuts are named and not compared.
	levelsChecked := false
	var empty []string
	emptyKnown := false
	if len(s.Files) > 0 {
		byRule, err := structure.LevelOfRule(s.Facts, s.Files)
		if err != nil {
			findings = append(findings, finding("grain", "%v", err))
		} else {
			levelsChecked = true
			names, err := structure.ValidateCuts(s.Facts, byRule, cuts)
			if err != nil {
				findings = append(findings, finding("cuts", "%v", err))
			} else {
				empty, emptyKnown = names, true
				for _, name := range names {
					findings = append(findings, finding("cuts", "cut %q carries no fact; the "+
						"schedule it declares was dropped from the store, or the cut should go", name))
				}
			}
		}
	}
	if !emptyKnown {
		empty = structure.EmptyCuts(s.Facts, cuts)
	}
	isEmpty := map[string]bool{}
	for _, name := range empty {
		isEmpty[name] = true
	}

	for _, m := range structure.TierMisfits(s.Facts, cuts, func(tier, slug string) bool {
		if tier == "division" {
			_, ok := s.Vocabulary.Division(slug)
			return ok
		}
		return s.Vocabulary.Department(slug)
	}) {
		findings = append(findings, finding("tier", "%s", m))
	}

	// EVERY FACT IS IN ONE CUT OR A DECLARED RESIDUE. A comparison covers the
	// cuts' facts and a view sums them; a fact outside every cut is outside
	// both, and this arm is what keeps that from being silent.
	residue := structure.BudgetBookResidue()
	coverage, uncovered := structure.Covered(s.Facts, cuts, residue)
	for _, f := range coverage {
		findings = append(findings, finding("coverage", "%s", f))
	}

	var (
		subjects  int
		clauses   []string
		refused   []string
		held      []string
		consulted = map[string]bool{}
	)
	reconcile := func(c structure.Comparison) {
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
			c.Name(), c.At, r.Subjects, joinComma(c.Columns), r.AgreeAtZero)
		if n := len(r.Excused); n > 0 {
			clause += fmt.Sprintf(", %d exception(s) held apart and NOT among the %d", n, r.Subjects)
		}
		clauses = append(clauses, clause)
	}
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
			reconcile(c)
		}
	}
	department := func(division string) string {
		d, _ := s.Vocabulary.Division(division)
		return d.Department
	}
	for _, t := range structure.BudgetBookTies() {
		if isEmpty[t.A] || isEmpty[t.B] {
			continue
		}
		c, err := structure.HoldTie(s.Facts, cuts, t, department)
		if err != nil {
			findings = append(findings, finding(t.Name, "%v", err))
			continue
		}
		reconcile(c)
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
	if uncovered > 0 {
		summary += fmt.Sprintf(". %d fact(s) fall in no cut, each under a declared residue:", uncovered)
		for _, r := range residue {
			summary += fmt.Sprintf(" (%s, %s, %s) %s;", r.Scope, r.Rule, r.Kind, r.Reason)
		}
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
