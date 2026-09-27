package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// cutsTieAlongTheLattice asserts that the published schedules are cuts of one
// hierarchy: every pair structure.Compare relates ties at the grain it meets,
// every declared structure.Tie holds, and a cell that does not tie is held
// apart only by a structure.Exception pinning both sides to the pages.
//
// The failure it exists for is silent: a rule written at the spine's scope
// lands pp.127-140's rows in the spine's own cells and doubles General Fund
// property tax with every graph check green, because those checks tie the
// graph to the facts and not the facts to each other.
//
// Peers, and pairs Compare cannot relate, are refused and counted, not
// compared; the peers are peersOverlapOnlyByDeclaredIdentity's.
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

	// A cut at a level its facts do not sit at, or carrying no fact, is refused
	// before any pair is compared. Without rule files -- a fixture -- the empty
	// cuts are named and skipped, and the summary says no level was checked.
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

	// A fact outside every cut is outside every comparison and every view.
	residue := structure.BudgetBookResidue()
	coverage, uncovered := structure.Covered(s.Facts, cuts, residue)
	for _, f := range coverage {
		findings = append(findings, finding("coverage", "%s", f))
	}

	var (
		subjects  int
		clauses   []string
		refused   int
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
			held = append(held, e.Name)
		}
		clause := fmt.Sprintf("%s at %s: %d cells over %s, %d one-sided at zero",
			c.Name(), c.At, r.Subjects, joinComma(c.Columns), r.AgreeAtZero)
		if n := len(r.Excused); n > 0 {
			clause += fmt.Sprintf(", %d held apart", n)
		}
		clauses = append(clauses, clause)
	}
	ties := structure.BudgetBookTies()
	tied := map[[2]string]bool{}
	for _, t := range ties {
		tied[[2]string{t.A, t.B}], tied[[2]string{t.B, t.A}] = true, true
	}
	for i, a := range cuts {
		for _, b := range cuts[i+1:] {
			if isEmpty[a.Name] || isEmpty[b.Name] {
				continue
			}
			c, err := structure.Compare(s.Facts, a, b)
			if err != nil {
				if !tied[[2]string{a.Name, b.Name}] {
					refused++
				}
				continue
			}
			reconcile(c)
		}
	}
	department := func(division string) string {
		d, _ := s.Vocabulary.Division(division)
		return d.Department
	}
	for _, t := range ties {
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

	// An exception on a pair no comparison relates is inert, and refused.
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
	summary := fmt.Sprintf("%d cells over %d comparison(s) of %d cut(s): %s",
		subjects, len(clauses), len(cuts)-len(empty), strings.Join(clauses, "; "))
	if len(held) > 0 {
		summary += fmt.Sprintf(". %d exception(s) held apart: %s", len(held), joinComma(held))
	}
	if uncovered > 0 {
		summary += fmt.Sprintf(". %d fact(s) fall in no cut, under %d declared residue(s)", uncovered, len(residue))
	}
	if refused > 0 {
		summary += fmt.Sprintf(". %d pair(s) no comparison or tie relates", refused)
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
