package structure

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TWO CUTS AT ONE LEVEL ARE PEERS, and a cell both produce is one figure read
// twice. pp.127-140 print a fund's transfers in at the RECEIVING fund and p76
// prints the same movements at the PAYING end: same money, same amount, two
// readings with two provenance chains. Neither decomposes the other, so the
// containment comparison refuses the pair, and a total over both would count
// the movement twice while every check downstream re-summed the same facts
// and agreed.
//
// AN IDENTITY IS A NEW RELATION AND NOT A ROW IN refinements. An equal-axes
// edge would pass validateLattice, because notSubset is non-strict, but
// declaring it both ways round trips the cycle check; the lattice stays
// directional and the identity is declared at the INSTANCE level, between two
// named cuts, for the kinds it covers.

// An Identity says that two cuts at one level print the same figures for the
// kinds it names: a cell both produce is one movement read from two ends, and
// the two readings must agree to the cent.
type Identity struct {
	Name string
	// A and B are the cut names, in either order.
	A, B string
	// Kinds are the fact kinds the two readings share. A cell of another kind
	// both cuts produce is an overlap the identity does not cover.
	Kinds []mapping.Kind
	// Reason is why the pages print one figure twice, about the documents.
	Reason string
}

func (id Identity) covers(a, b string, k mapping.Kind) bool {
	joins := (id.A == a && id.B == b) || (id.A == b && id.B == a)
	return joins && containsKind(id.Kinds, k)
}

// ValidateIdentities holds a set of identities to a set of cuts: both cuts
// exist and differ, sit at one level, and each print every kind the identity
// names. It reads declarations alone; whether the store bears an identity
// out is Peers' business.
func ValidateIdentities(cuts []Cut, identities []Identity) error {
	byName := map[string]Cut{}
	for _, c := range cuts {
		byName[c.Name] = c
	}
	seen := map[string]bool{}
	for _, id := range identities {
		if id.Name == "" {
			return fmt.Errorf("an identity between %q and %q has no name", id.A, id.B)
		}
		if seen[id.Name] {
			return fmt.Errorf("identity %q is declared twice", id.Name)
		}
		seen[id.Name] = true
		a, ok := byName[id.A]
		if !ok {
			return fmt.Errorf("identity %q names cut %q, which is not declared", id.Name, id.A)
		}
		b, ok := byName[id.B]
		if !ok {
			return fmt.Errorf("identity %q names cut %q, which is not declared", id.Name, id.B)
		}
		if id.A == id.B {
			return fmt.Errorf("identity %q relates %q to itself", id.Name, id.A)
		}
		if a.Level != b.Level {
			return fmt.Errorf("identity %q relates %q (%s) to %q (%s); two readings of one figure "+
				"sit at one level, and a pair at two is a containment or nothing", id.Name, a.Name, a.Level, b.Name, b.Level)
		}
		if len(id.Kinds) == 0 {
			return fmt.Errorf("identity %q covers no kind", id.Name)
		}
		for _, k := range id.Kinds {
			if !containsKind(a.Kinds, k) || !containsKind(b.Kinds, k) {
				return fmt.Errorf("identity %q covers %q, which %q or %q does not print", id.Name, k, a.Name, b.Name)
			}
		}
		if id.Reason == "" {
			return fmt.Errorf("identity %q gives no reason", id.Name)
		}
	}
	return nil
}

// A Shared cell is one key both peers produced.
type Shared struct {
	Key  Key
	Kind mapping.Kind
	A, B Sum
	// Identity is the declared identity covering this cell, or "" for an
	// overlap nothing declares.
	Identity string
}

// An Overlap is what one pass over two peers produced.
type Overlap struct {
	A, B Cut
	At   Level
	// Shared is every cell both cuts produced, in a stable order.
	Shared []Shared
	// Findings is one line per overlap no identity covers, per covered cell
	// whose readings differ, and per identity this pair bears out nowhere.
	Findings []string
}

// Peers compares two cuts at one level and reports every cell both produce.
//
// THREE ARMS, AND THE THIRD IS THE ONE AN EXEMPTION WOULD LACK. A shared cell
// no identity covers is a finding: two schedules printing one address is a
// doubling waiting for a total to select both. A shared cell under a declared
// identity whose amounts differ is a finding: the identity says they are one
// figure, and the pages disagree. And a declared identity producing NO shared
// cell is refused, for the reason an exemption for a cell nobody prints
// exempts nothing -- the declaration has stopped describing the corpus.
//
// PER KIND, because a key carries no kind and an identity covers kinds.
// pp.127-140 and p76 share transfers-in cells and nothing else; an overlap of
// another kind at the same address would be one the identity does not cover.
func Peers(facts []fact.Fact, a, b Cut, identities []Identity) (Overlap, error) {
	if a.Level != b.Level {
		return Overlap{}, fmt.Errorf("peers %q (%s) and %q (%s): not at one level", a.Name, a.Level, b.Name, b.Level)
	}
	at := a.Level
	r, err := restrict(a, b, at)
	if err != nil {
		return Overlap{}, fmt.Errorf("peers %q and %q: %w", a.Name, b.Name, err)
	}
	out := Overlap{A: a, B: b, At: at}
	borne := map[string]bool{}
	for _, k := range r.kinds {
		one := restriction{kinds: []mapping.Kind{k}, fundGroups: r.fundGroups}
		as, err := project(facts, a, at, one)
		if err != nil {
			return Overlap{}, err
		}
		bs, err := project(facts, b, at, one)
		if err != nil {
			return Overlap{}, err
		}
		for _, key := range unionKeys(as, bs) {
			sa, sb := as[key], bs[key]
			if !sa.Present || !sb.Present {
				continue
			}
			cell := Shared{Key: key, Kind: k, A: sa, B: sb}
			for _, id := range identities {
				if id.covers(a.Name, b.Name, k) {
					cell.Identity = id.Name
					borne[id.Name] = true
					break
				}
			}
			out.Shared = append(out.Shared, cell)
			switch {
			case cell.Identity == "":
				out.Findings = append(out.Findings, fmt.Sprintf(
					"%s %s: %q and %q both publish here (%s and %s) and no identity says they are one "+
						"figure; a total selecting both would count it twice",
					key, k, a.Name, b.Name, cents(sa.Cents), cents(sb.Cents)))
			case sa.Cents != sb.Cents:
				out.Findings = append(out.Findings, fmt.Sprintf(
					"%s %s: identity %q says %q and %q print one figure here, and they print %s and "+
						"%s, a difference of %s; the pages disagree",
					key, k, cell.Identity, a.Name, b.Name, cents(sa.Cents), cents(sb.Cents), cents(sa.Cents-sb.Cents)))
			}
		}
	}
	for _, id := range identities {
		if id.A != a.Name && id.B != a.Name {
			continue
		}
		if id.A != b.Name && id.B != b.Name {
			continue
		}
		if !borne[id.Name] {
			out.Findings = append(out.Findings, fmt.Sprintf(
				"identity %q says %q and %q print one figure for %v, and they share no cell of those "+
					"kinds; a declaration over no cell covers nothing, so remove it rather than leaving "+
					"it to excuse an overlap that appears later", id.Name, a.Name, b.Name, id.Kinds))
		}
	}
	sort.Slice(out.Shared, func(i, j int) bool {
		if out.Shared[i].Key != out.Shared[j].Key {
			ki, kj := out.Shared[i].Key, out.Shared[j].Key
			if ki.Year != kj.Year {
				return ki.Year < kj.Year
			}
			if ki.Basis != kj.Basis {
				return ki.Basis < kj.Basis
			}
			return ki.Coords < kj.Coords
		}
		return out.Shared[i].Kind < out.Shared[j].Kind
	})
	return out, nil
}

// A View is a set of cuts a total may be taken over: an antichain of the
// lattice, and where two of its cuts are joined by an identity, a declaration
// of which reading the total takes.
//
// A CUT SET THAT WOULD TRAVERSE BOTH READINGS IS REFUSED, not summed once by a
// preference table and not counted once silently. A total that is not the
// sum of its parts with no arithmetic on any page witnessing the collapse is
// what this refuses; the view says which reading it takes and the other's
// cells of the identity's kinds are left out, by name.
type View struct {
	Name string
	Cuts []Cut
	// Readings maps an identity name to the cut whose reading the view
	// takes. Required for every identity joining two cuts of the view.
	Readings map[string]string
}

// NewView refuses a set that is not summable: two cuts one of which
// decomposes the other over money both print, a cut named twice, or an
// identity joining two of its cuts with no reading declared. A reading naming
// an identity that joins no two cuts of the view is refused too, because it
// would excuse a traversal that is not happening.
//
// THE ANTICHAIN IS OVER MONEY BOTH CUTS PRINT, not over levels alone. pp.127-140
// by fund and pp.167-170 by fund, department and object are one above the
// other in the lattice, and a document following a dollar from source to
// spend holds both; they are summable together because one prints revenue and
// the other expenditure, and no fact is in both. What is refused is a pair one
// of which decomposes the other where their kinds meet -- the spine beside any
// detail schedule.
func NewView(name string, cuts []Cut, identities []Identity, readings map[string]string) (View, error) {
	names := map[string]Cut{}
	for _, c := range cuts {
		if _, dup := names[c.Name]; dup {
			return View{}, fmt.Errorf("view %q names cut %q twice", name, c.Name)
		}
		names[c.Name] = c
	}
	for i, a := range cuts {
		for _, b := range cuts[i+1:] {
			if !kindsMeet(a, b) {
				continue
			}
			fine, coarse := a, b
			if Refines(b.Level, a.Level) {
				fine, coarse = b, a
			}
			if Refines(fine.Level, coarse.Level) {
				return View{}, fmt.Errorf("view %q is not an antichain: %q (%s) decomposes %q (%s) and both "+
					"print %v, so a total over both counts that money twice",
					name, fine.Name, fine.Level, coarse.Name, coarse.Level, sharedKinds(a, b))
			}
		}
	}
	joined := map[string]bool{}
	for _, id := range identities {
		_, hasA := names[id.A]
		_, hasB := names[id.B]
		if !hasA || !hasB {
			continue
		}
		joined[id.Name] = true
		reading, ok := readings[id.Name]
		if !ok {
			return View{}, fmt.Errorf("view %q holds both %q and %q, which identity %q says print one "+
				"figure for %v; a total over both would count it twice, so the view must say which "+
				"reading it takes", name, id.A, id.B, id.Name, id.Kinds)
		}
		if reading != id.A && reading != id.B {
			return View{}, fmt.Errorf("view %q takes reading %q for identity %q, which joins %q and %q",
				name, reading, id.Name, id.A, id.B)
		}
	}
	for idName := range readings {
		if !joined[idName] {
			return View{}, fmt.Errorf("view %q declares a reading for identity %q, which joins no two "+
				"of its cuts", name, idName)
		}
	}
	return View{Name: name, Cuts: cuts, Readings: readings}, nil
}

// Admits says whether a fact is counted by the view: it falls in one of the
// view's cuts, and it is not the reading the view declined.
func (v View) Admits(f *fact.Fact, identities []Identity) bool {
	for _, c := range v.Cuts {
		if !c.admits(f) {
			continue
		}
		for _, id := range identities {
			reading, ok := v.Readings[id.Name]
			if !ok || !containsKind(id.Kinds, f.Kind) {
				continue
			}
			other := id.A
			if reading == id.A {
				other = id.B
			}
			if c.Name == other {
				return false
			}
		}
		return true
	}
	return false
}

// CutsOf is every declared cut whose scope is one of those named: one per
// scope, and two for the scope that prints two grains.
func CutsOf(scopes []string) []Cut {
	var out []Cut
	for _, c := range AllCuts() {
		if contains(scopes, c.Scope) {
			out = append(out, c)
		}
	}
	return out
}

// ViewOf is the view a document's scope set names, over the declared cuts
// and identities, refused the way NewView refuses. A scope no cut selects is
// refused too: a document over money the structure does not describe is not
// a view of it.
func ViewOf(name string, scopes []string, readings map[string]string) (View, error) {
	cuts := CutsOf(scopes)
	for _, sc := range scopes {
		found := false
		for _, c := range cuts {
			if c.Scope == sc {
				found = true
				break
			}
		}
		if !found {
			return View{}, fmt.Errorf("view %q selects scope %q, which no declared cut reads", name, sc)
		}
	}
	return NewView(name, cuts, BudgetBookIdentities(), readings)
}

func kindsMeet(a, b Cut) bool { return len(sharedKinds(a, b)) > 0 }

func sharedKinds(a, b Cut) []mapping.Kind {
	var out []mapping.Kind
	for _, k := range a.Kinds {
		if containsKind(b.Kinds, k) {
			out = append(out, k)
		}
	}
	return out
}
