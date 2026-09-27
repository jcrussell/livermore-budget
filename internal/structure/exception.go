package structure

import (
	"fmt"
	"sort"
)

// A Pin is one cell of an exception: where it is, and what each side says.
// A side with no such cell is a different claim from a printed zero.
type Pin struct {
	Year  int
	Basis string
	// Coords are the meet's axis values, spelled by Coords.
	Coords map[Axis]string
	// Cut and Against are what the comparison's two sides say here.
	Cut, Against Sum
}

// An Exception holds one or more cells of one comparison apart from the tie,
// with every figure it rests on declared. It is not a tolerance: it pins both
// sides, and fails when either moves, when the cell stops existing, or when
// the cell ties.
type Exception struct {
	// Name is how another exception grounds itself in this one.
	Name string
	// Cut and Against name the comparison's two cuts, in Compare's order:
	// Against is the side whose columns decided what was compared.
	Cut, Against string
	// At is the level the pair meets at, declared so that a lattice change
	// moving the meet refuses the exception instead of re-addressing it.
	At Level
	// Cells are the cells held apart; several only where the pages print a
	// single figure for their sum.
	Cells []Pin
	// Residual is what Against carries over Cut across the cells, in cents: a
	// printed figure, or the difference of two on one page. Declared, not
	// derived from the pins, so the pins are held to the page.
	Residual int64
	// Printed says where the residual, or both pinned sides, are printed.
	Printed string
	// SameResidualAs names an exception whose Residual this one's must equal,
	// grounding a cell neither of whose figures is printed.
	SameResidualAs string
	// Reason is why the schedules differ, about the document and not the
	// pipeline.
	Reason string
	// Bead is the work that retires the exception, or the decision that keeps
	// it.
	Bead string
}

// Key is the cell a pin names, at the exception's level.
func (e Exception) Key(p Pin) Key {
	return Key{Year: p.Year, Basis: p.Basis, Level: e.At, Coords: Coords(e.At, p.Coords)}
}

// pinned is the difference the pins declare, for the arm that holds it to
// Residual.
func (e Exception) pinned() int64 {
	var d int64
	for _, p := range e.Cells {
		d += p.Against.Cents - p.Cut.Cents
	}
	return d
}

// ValidateExceptions refuses a declaration that could not hold anything apart.
// It reads the declarations alone; the store is Reconcile's.
func ValidateExceptions(exceptions []Exception) error {
	byName := map[string]Exception{}
	for _, e := range exceptions {
		if e.Name == "" {
			return fmt.Errorf("an exception on %s -> %s has no name", e.Cut, e.Against)
		}
		if _, dup := byName[e.Name]; dup {
			return fmt.Errorf("exception %q is declared twice", e.Name)
		}
		byName[e.Name] = e
	}
	for _, e := range exceptions {
		if !Declared(e.At) {
			return fmt.Errorf("exception %q is at %q, which is not a declared level", e.Name, e.At)
		}
		if len(e.Cells) == 0 {
			return fmt.Errorf("exception %q names no cell", e.Name)
		}
		if e.Residual == 0 {
			return fmt.Errorf("exception %q declares a residual of zero, so it holds nothing apart", e.Name)
		}
		if e.Printed == "" {
			return fmt.Errorf("exception %q does not say where its figures are printed", e.Name)
		}
		for _, p := range e.Cells {
			if p.Cut.Cents == p.Against.Cents {
				return fmt.Errorf("exception %q pins %s to %s on both sides; a cell that ties needs "+
					"no exception, and one declared over it would excuse a figure that later moved",
					e.Name, e.Key(p), cents(p.Cut.Cents))
			}
			for a := range p.Coords {
				if !hasAxis(e.At, a) {
					return fmt.Errorf("exception %q names axis %q, which %q does not carry", e.Name, a, e.At)
				}
			}
		}
		if got := e.pinned(); got != e.Residual {
			return fmt.Errorf("exception %q pins sides that differ by %s and declares a printed "+
				"residual of %s; one of the three figures is mistyped", e.Name, cents(got), cents(e.Residual))
		}
		if e.SameResidualAs != "" {
			g, ok := byName[e.SameResidualAs]
			if !ok {
				return fmt.Errorf("exception %q is grounded in %q, which is not declared", e.Name, e.SameResidualAs)
			}
			if g.Residual != e.Residual {
				return fmt.Errorf("exception %q holds %s apart and is grounded in %q, which holds %s "+
					"apart; they are one discrepancy seen on two axes and must agree",
					e.Name, cents(e.Residual), g.Name, cents(g.Residual))
			}
		}
	}
	return nil
}

// Reconciled is a comparison with its declared exceptions applied.
type Reconciled struct {
	Comparison
	// Excused are the exceptions that fired: each named a cell the comparison
	// produced, and both sides said what it pinned.
	Excused []Exception
	// Consulted names every exception declared on this pair, fired or refused,
	// so a caller can tell an inert declaration from one this pair settled.
	Consulted []string
	// Subjects is the cells compared and not held apart.
	Subjects int
	// AgreeAtZero is how many of those Subjects only one cut produced and
	// that tie, the other side being zero.
	AgreeAtZero int
	// Findings are the disagreements no exception covers, and the exceptions
	// that could not be applied.
	Findings []string
}

// Reconcile applies the exceptions declared for a comparison's pair to it.
// Every one must fire: naming a cell the comparison did not produce, or one
// that ties, is a finding.
func Reconcile(c Comparison, exceptions []Exception) Reconciled {
	out := Reconciled{Comparison: c}
	byKey := map[Key]Cell{}
	for _, cell := range c.Cells {
		byKey[cell.Key] = cell
	}
	held := map[Key]bool{}
	for _, e := range exceptions {
		if e.Cut != c.Cut.Name || e.Against != c.Against.Name {
			continue
		}
		out.Consulted = append(out.Consulted, e.Name)
		if e.At != c.At {
			out.Findings = append(out.Findings, fmt.Sprintf(
				"exception %q is declared at %q and the pair meets at %q; the declaration names "+
					"cells of a level this comparison does not produce (%s)", e.Name, e.At, c.At, e.Bead))
			continue
		}
		fired := true
		for _, p := range e.Cells {
			k := e.Key(p)
			cell, ok := byKey[k]
			switch {
			case !ok:
				out.Findings = append(out.Findings, fmt.Sprintf(
					"exception %q names %s and neither %q nor %q produces that cell, so it excuses "+
						"nothing; remove it rather than leaving a declaration that has stopped "+
						"describing the corpus (%s)", e.Name, k, c.Cut.Name, c.Against.Name, e.Bead))
				fired = false
			case cell.Ties():
				out.Findings = append(out.Findings, fmt.Sprintf(
					"exception %q names %s and the two sides agree at %s, so the declaration is a "+
						"false claim about the pages and would excuse a figure that later moved; "+
						"remove it and let the cell tie like every other (%s)",
					e.Name, k, cents(cell.Cut.Cents), e.Bead))
				fired = false
			case cell.Cut != p.Cut:
				out.Findings = append(out.Findings, fmt.Sprintf(
					"exception %q pins %q at %s for %s and it now says %s. %s (%s)",
					e.Name, c.Cut.Name, sum(p.Cut), k, sum(cell.Cut), e.Reason, e.Bead))
				fired = false
			case cell.Against != p.Against:
				out.Findings = append(out.Findings, fmt.Sprintf(
					"exception %q pins %q at %s for %s and it now says %s. If the page has been "+
						"corrected, delete the exception rather than re-pointing it: the cell then "+
						"ties on its own (%s)", e.Name, c.Against.Name, sum(p.Against), k,
					sum(cell.Against), e.Bead))
				fired = false
			}
			held[k] = true
		}
		if fired {
			out.Excused = append(out.Excused, e)
		}
	}
	for _, cell := range c.Cells {
		if held[cell.Key] {
			continue
		}
		out.Subjects++
		if !cell.Ties() {
			out.Findings = append(out.Findings, c.Finding(cell))
		} else if !cell.Cut.Present || !cell.Against.Present {
			out.AgreeAtZero++
		}
	}
	sort.Slice(out.Excused, func(i, j int) bool { return out.Excused[i].Name < out.Excused[j].Name })
	return out
}

func sum(s Sum) string {
	if !s.Present {
		return "no such cell"
	}
	return cents(s.Cents)
}
