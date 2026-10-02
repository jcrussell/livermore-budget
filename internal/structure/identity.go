package structure

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// An Identity says that two cuts at one level print the same figures for the
// kinds it names: a cell both produce is one movement read from two ends, and
// the two readings must agree to the cent. It is declared between two named
// cuts rather than as a lattice edge, which would be a cycle.
type Identity struct {
	Name string
	// A and B are the cut names, in either order.
	A, B string
	// Kinds are the fact kinds the two readings share. A cell of another kind
	// both cuts produce is an overlap the identity does not cover.
	Kinds []mapping.Kind
	// Categories narrows Kinds to the categories both readings print, where
	// one side prints a kind under a category the other never does: a cell
	// outside them is not this identity's, shared or one-sided. Empty means
	// every category.
	Categories []string
	// Reason is why the pages print one figure twice, about the documents.
	Reason string
}

func (id Identity) covers(a, b string, k mapping.Kind, category string) bool {
	joins := (id.A == a && id.B == b) || (id.A == b && id.B == a)
	return joins && containsKind(id.Kinds, k) && id.coversCategory(category)
}

func (id Identity) coversCategory(category string) bool {
	return len(id.Categories) == 0 || contains(id.Categories, category)
}

// ValidateIdentities holds a set of identities to a set of cuts. It reads
// declarations alone; the store is Peers'.
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
		if len(id.Categories) > 0 && !hasAxis(a.Level, AxisCategory) {
			return fmt.Errorf("identity %q names categories and %q is at %q, which carries no category axis",
				id.Name, a.Name, a.Level)
		}
		named := map[string]bool{}
		for _, c := range id.Categories {
			if c == "" || named[c] {
				return fmt.Errorf("identity %q names category %q empty or twice", id.Name, c)
			}
			named[c] = true
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
	// Findings is one line per cell or identity at fault.
	Findings []string
	// Excused names each exception every cell of which met an absence in
	// this pass. One meeting some of its cells excuses those and is left out.
	Excused []string
}

// Peers compares two cuts at one level, kind by kind, and reports every cell
// both produce. Findings: a shared cell no identity covers; a covered cell
// whose readings differ; a covered non-zero cell only one side prints, unless
// an exception pins the other absent; an identity no cell bears out; and a
// category an identity names that no shared cell bears.
func Peers(facts []fact.Fact, a, b Cut, identities []Identity, exceptions []Exception) (Overlap, error) {
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
	met := map[string]map[int]bool{}
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
		sums := map[string]map[Level]map[Key]Sum{}
		sumAt := func(c Cut, l Level, key Key) (Sum, error) {
			if sums[c.Name] == nil {
				sums[c.Name] = map[Level]map[Key]Sum{}
			}
			if sums[c.Name][l] == nil {
				cells, err := project(facts, c, l, one)
				if err != nil {
					return Sum{}, err
				}
				sums[c.Name][l] = cells
			}
			return sums[c.Name][l][key], nil
		}
		for _, key := range UnionKeys(as, bs) {
			// An identity naming categories sits at a level carrying the
			// category axis (ValidateIdentities), so the key spells it.
			category := key.coord(AxisCategory)
			identity := ""
			for _, id := range identities {
				if id.covers(a.Name, b.Name, k, category) {
					identity = id.Name
					break
				}
			}
			sa, sb := as[key], bs[key]
			if !sa.Present || !sb.Present {
				if identity != "" && a.prints(mapping.Basis(key.Basis)) && b.prints(mapping.Basis(key.Basis)) &&
					sa.Cents+sb.Cents != 0 {
					present, missing := a, b
					if !sa.Present {
						present, missing = b, a
					}
					declared, by, err := absenceDeclared(facts, present, missing, one, key, exceptions, sumAt)
					if err != nil {
						return Overlap{}, err
					}
					for _, c := range by {
						if met[c.name] == nil {
							met[c.name] = map[int]bool{}
						}
						met[c.name][c.cell] = true
					}
					if !declared {
						out.Findings = append(out.Findings, fmt.Sprintf(
							"%s %s: identity %q says %q and %q print one figure, and %q prints %s here "+
								"where %q has no such cell, and no exception declares the absence",
							key, k, identity, a.Name, b.Name, present.Name, amount.Cents(sa.Cents+sb.Cents).String(), missing.Name))
					}
				}
				continue
			}
			cell := Shared{Key: key, Kind: k, A: sa, B: sb, Identity: identity}
			if identity != "" {
				borne[identity] = true
				borne[identity+"\x00"+category] = true
			}
			out.Shared = append(out.Shared, cell)
			switch {
			case cell.Identity == "":
				out.Findings = append(out.Findings, fmt.Sprintf(
					"%s %s: %q and %q both publish here (%s and %s) and no identity says they are one "+
						"figure; a total selecting both would count it twice",
					key, k, a.Name, b.Name, amount.Cents(sa.Cents).String(), amount.Cents(sb.Cents).String()))
			case sa.Cents != sb.Cents:
				out.Findings = append(out.Findings, fmt.Sprintf(
					"%s %s: identity %q says %q and %q print one figure here, and they print %s and "+
						"%s, a difference of %s; the pages disagree",
					key, k, cell.Identity, a.Name, b.Name, amount.Cents(sa.Cents).String(), amount.Cents(sb.Cents).String(), amount.Cents(sa.Cents-sb.Cents).String()))
			}
		}
	}
	// An exception excuses only when this pass met every cell it pins.
	for _, e := range exceptions {
		if len(met[e.Name]) == len(e.Cells) {
			out.Excused = append(out.Excused, e.Name)
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
			continue
		}
		for _, c := range id.Categories {
			if !borne[id.Name+"\x00"+c] {
				out.Findings = append(out.Findings, fmt.Sprintf(
					"identity %q names category %q, and %q and %q share no cell of it; a category "+
						"no cell bears is a claim nothing holds, so remove it",
					id.Name, c, a.Name, b.Name))
			}
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

// pinnedCell is one cell of a declared exception.
type pinnedCell struct {
	name string
	cell int
}

// absenceDeclared says whether, for every fact of present at key, an
// exception pins missing absent at the cell that fact falls in at the
// exception's level, and pins its other side at what present sums to there;
// and names the cells that did, when every fact was. An exception declared at
// the pair's own level is between peers and excuses only the pair it names;
// one at another level is a comparison's, and excuses the absence wherever
// present prints the figure it pins.
func absenceDeclared(facts []fact.Fact, present, missing Cut, r restriction, key Key, exceptions []Exception,
	sumAt func(Cut, Level, Key) (Sum, error)) (bool, []pinnedCell, error) {
	seen := false
	var by []pinnedCell
	for i := range facts {
		f := &facts[i]
		if !present.admits(f) || !r.admits(f) || KeyOf(f, key.Level) != key {
			continue
		}
		seen = true
		pinned := false
		for _, e := range exceptions {
			for j, p := range e.Cells {
				k := e.Key(p)
				if KeyOf(f, e.At) != k {
					continue
				}
				var other Sum
				var otherCut string
				switch {
				case e.Cut == missing.Name && !p.Cut.Present:
					other, otherCut = p.Against, e.Against
				case e.Against == missing.Name && !p.Against.Present:
					other, otherCut = p.Cut, e.Cut
				default:
					continue
				}
				if e.At == key.Level && otherCut != present.Name {
					continue
				}
				got, err := sumAt(present, e.At, k)
				if err != nil {
					return false, nil, err
				}
				if other == got {
					pinned = true
					by = append(by, pinnedCell{e.Name, j})
				}
			}
		}
		if !pinned {
			return false, nil, nil
		}
	}
	return seen, by, nil
}

// A View is a set of cuts a total may be taken over: an antichain of the
// lattice, and where two of its cuts are joined by an identity, a declaration
// of which reading the total takes.
type View struct {
	Name string
	Cuts []Cut
	// Readings maps an identity name to the cut whose reading the view
	// takes. Required for every identity joining two cuts of the view.
	Readings map[string]string
}

// NewView refuses a set that is not summable: a cut named twice, an identity
// joining two of its cuts with no reading, a reading for an identity joining
// none, or two cuts one of which decomposes the other. The antichain is over
// money both print, not levels alone: revenue by fund and expenditure by
// fund, department and object are summable together, and so are two cuts
// whose declared fund groups are disjoint.
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
			if !kindsMeet(a, b) || !footprintsMeet(a, b) {
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
			if !ok || !containsKind(id.Kinds, f.Kind) || !id.coversCategory(f.Category) {
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

// CutsOf is every declared cut whose scope is one of those named.
func CutsOf(scopes []string) []Cut {
	var out []Cut
	for _, c := range AllCuts() {
		if contains(scopes, c.Scope) {
			out = append(out, c)
		}
	}
	return out
}

// ViewOf is the view a document's scope set names, refused as NewView refuses
// and also when a scope has no cut.
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

// footprintsMeet is false only when both cuts declare fund groups and share
// none. An empty footprint is every group, so it meets anything.
func footprintsMeet(a, b Cut) bool {
	if len(a.FundGroups) == 0 || len(b.FundGroups) == 0 {
		return true
	}
	for _, g := range a.FundGroups {
		if contains(b.FundGroups, g) {
			return true
		}
	}
	return false
}

func sharedKinds(a, b Cut) []mapping.Kind {
	var out []mapping.Kind
	for _, k := range a.Kinds {
		if containsKind(b.Kinds, k) {
			out = append(out, k)
		}
	}
	return out
}
