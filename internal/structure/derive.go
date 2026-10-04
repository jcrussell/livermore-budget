package structure

import (
	"sort"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// Reference is the cut declared the reference among cuts: the one whose
// columns every agreement is held to, pp.66-67. false when none is.
func Reference(cuts []Cut) (Cut, bool) {
	for _, c := range cuts {
		if c.Reference {
			return c, true
		}
	}
	return Cut{}, false
}

// Undecomposed is the kinds the reference prints and no cut of the view
// prints, sorted: money a document drawn over the view can carry from the
// reference's chart only whole, since no page it reads splits it any finer.
func Undecomposed(view []Cut, reference Cut) []vocab.Kind {
	printed := map[vocab.Kind]bool{}
	for _, c := range view {
		for _, k := range c.Kinds {
			printed[k] = true
		}
	}
	var out []vocab.Kind
	for _, k := range reference.Kinds {
		if !printed[k] {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// AbsentCells is every pinned cell of an exception on (a cut of the view ->
// against) that pins the view's side absent: a cell the reference prints and
// the view's pages have no row for, which a document drawn over the view can
// carry from the reference's chart only whole. Pins come back in declaration
// order, each with the exception it is declared on.
func AbsentCells(exceptions []Exception, view []Cut, against Cut) []AbsentCell {
	inView := map[string]bool{}
	for _, c := range view {
		inView[c.Name] = true
	}
	var out []AbsentCell
	for _, e := range exceptions {
		if !inView[e.Cut] || e.Against != against.Name {
			continue
		}
		for _, p := range e.Cells {
			if !p.Cut.Present {
				out = append(out, AbsentCell{Exception: e.Name, Pin: p})
			}
		}
	}
	return out
}

// An AbsentCell is one pin a view's cut has no row for, and the exception
// that declares it.
type AbsentCell struct {
	Exception string
	Pin       Pin
}

// ExceptionsOn is the exceptions declared on one comparison at one level, in
// declaration order.
func ExceptionsOn(exceptions []Exception, cut, against string, at Level) []Exception {
	var out []Exception
	for _, e := range exceptions {
		if e.Cut == cut && e.Against == against && e.At == at {
			out = append(out, e)
		}
	}
	return out
}
