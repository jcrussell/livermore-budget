package structure

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// DocumentSchemaVersion is the version a [Document] declares. A client reads
// it before anything else and refuses one it does not know.
const DocumentSchemaVersion = 1

// A Document is the structure on the wire: the lattice, every declared cut and
// identity, the views the site's documents are, and every fact any view admits
// -- each fact ONCE, with its provenance, and each view naming its facts by
// index into that list.
//
// THE VIEW NAMES ITS FACTS; THE CLIENT DOES NOT RE-DERIVE ADMISSION. The cut
// and identity declarations ride along as the reason a fact is in a view, and
// a client is free to check them, but membership is shipped as [View.Admits]
// decided it. A client re-implementing that rule would be a second copy of
// the admission test that nothing keeps in step with this one.
//
// A VIEW ADMITTING NO FACT IS REFUSED, never shipped with an empty list, so
// an absent view and an empty one cannot mean one thing. That is the
// discipline stepView.Opens keeps, inherited by the same derivation.
type Document struct {
	SchemaVersion int            `json:"schema_version"`
	Levels        []LevelDecl    `json:"levels"`
	Cuts          []CutDecl      `json:"cuts"`
	Identities    []IdentityDecl `json:"identities"`
	Views         []ViewDecl     `json:"views"`
	Facts         []Record       `json:"facts"`
}

// A LevelDecl is one point of the lattice: its axes, and the levels it
// decomposes by a DECLARED edge. Only the immediate parents are listed, as
// the lattice declares them; a client wanting the closure walks the edges.
type LevelDecl struct {
	Name    Level   `json:"name"`
	Axes    []Axis  `json:"axes"`
	Refines []Level `json:"refines,omitempty"`
}

// A CutDecl is a [Cut] on the wire, field for field.
type CutDecl struct {
	Name           string          `json:"name"`
	Scope          string          `json:"scope"`
	Level          Level           `json:"level"`
	Kinds          []mapping.Kind  `json:"kinds"`
	FundGroups     []string        `json:"fund_groups,omitempty"`
	DepartmentTier string          `json:"department_tier,omitempty"`
	Reference      bool            `json:"reference,omitempty"`
	Bases          []mapping.Basis `json:"bases"`
	Rules          []string        `json:"rules,omitempty"`
	Placeholders   []Axis          `json:"placeholders,omitempty"`
}

// An IdentityDecl is an [Identity] on the wire, field for field.
type IdentityDecl struct {
	Name   string         `json:"name"`
	A      string         `json:"a"`
	B      string         `json:"b"`
	Kinds  []mapping.Kind `json:"kinds"`
	Reason string         `json:"reason"`
}

// A ViewDecl is one document's view: the scope set it declared, the cuts that
// set selects, the readings it takes, and the facts it admits as indices into
// [Document.Facts].
type ViewDecl struct {
	Name     string            `json:"name"`
	Scopes   []string          `json:"scopes"`
	Cuts     []string          `json:"cuts"`
	Readings map[string]string `json:"readings,omitempty"`
	Facts    []int             `json:"facts"`
}

// A Record is one fact as the structure carries it: its provenance -- the id
// and the locator that [fact.Fact] carries, verbatim -- and the coordinates
// a cut reads. The row and column paths are not here: a client reading the
// structure addresses a fact by its coordinates, not by the row it was on.
type Record struct {
	ID     string `json:"id"`
	DocID  string `json:"doc_id"`
	Page   int    `json:"page"`
	Offset int    `json:"offset"`
	Token  string `json:"token"`
	RuleID string `json:"rule_id"`

	Kind       mapping.Kind  `json:"kind"`
	Basis      mapping.Basis `json:"basis"`
	Scope      string        `json:"scope"`
	FiscalYear int           `json:"fiscal_year"`

	FundGroup  string `json:"fund_group"`
	Fund       *int   `json:"fund"`
	Department string `json:"department"`
	Category   string `json:"category"`
	RowLabel   string `json:"row_label"`

	Sign        mapping.Sign `json:"sign"`
	AmountCents int64        `json:"amount_cents"`
	Derived     bool         `json:"derived"`
}

// Scoped is one document's name and the scope set it is of: what
// [ViewOf] constructs a view from.
type Scoped struct {
	Name   string
	Scopes []string
}

// Build is the structure over these facts for these documents' views, each
// constructed by [ViewOf] and refused the way it refuses. A document named
// twice is refused, and so is a view that admits no fact.
func Build(facts []fact.Fact, docs []Scoped) (Document, error) {
	d := Document{
		SchemaVersion: DocumentSchemaVersion,
		Levels:        levelDecls(),
		Cuts:          cutDecls(AllCuts()),
		Identities:    identityDecls(BudgetBookIdentities()),
	}
	identities := BudgetBookIdentities()
	index := make(map[int]int, len(facts))
	seen := map[string]bool{}
	for _, doc := range docs {
		if seen[doc.Name] {
			return Document{}, fmt.Errorf("structure: document %q is named twice", doc.Name)
		}
		seen[doc.Name] = true
		v, err := ViewOf(doc.Name, doc.Scopes, nil)
		if err != nil {
			return Document{}, fmt.Errorf("structure: %w", err)
		}
		decl := ViewDecl{
			Name:     v.Name,
			Scopes:   append([]string(nil), doc.Scopes...),
			Readings: v.Readings,
		}
		for _, c := range v.Cuts {
			decl.Cuts = append(decl.Cuts, c.Name)
		}
		for i := range facts {
			if !v.Admits(&facts[i], identities) {
				continue
			}
			at, ok := index[i]
			if !ok {
				at = len(d.Facts)
				index[i] = at
				d.Facts = append(d.Facts, recordOf(&facts[i]))
			}
			decl.Facts = append(decl.Facts, at)
		}
		if len(decl.Facts) == 0 {
			return Document{}, fmt.Errorf(
				"structure: view %q over %v admits no fact, so it would ship an empty list "+
					"where an absent view means the same thing", v.Name, doc.Scopes)
		}
		d.Views = append(d.Views, decl)
	}
	return d, nil
}

func levelDecls() []LevelDecl {
	var out []LevelDecl
	for _, l := range Levels() {
		parents := append([]Level(nil), refinements[l]...)
		sort.Slice(parents, func(i, j int) bool { return parents[i] < parents[j] })
		out = append(out, LevelDecl{Name: l, Axes: Axes(l), Refines: parents})
	}
	return out
}

func cutDecls(cuts []Cut) []CutDecl {
	out := make([]CutDecl, 0, len(cuts))
	for _, c := range cuts {
		// A CONVERSION, NOT A LITERAL, so a field added to Cut stops compiling
		// here until CutDecl says whether it reaches the wire.
		out = append(out, CutDecl(c))
	}
	return out
}

func identityDecls(ids []Identity) []IdentityDecl {
	out := make([]IdentityDecl, 0, len(ids))
	for _, id := range ids {
		out = append(out, IdentityDecl(id))
	}
	return out
}

func recordOf(f *fact.Fact) Record {
	return Record{
		ID:          f.ID,
		DocID:       f.DocID,
		Page:        f.Page,
		Offset:      f.Offset,
		Token:       f.Token,
		RuleID:      f.RuleID,
		Kind:        f.Kind,
		Basis:       f.Basis,
		Scope:       f.Scope,
		FiscalYear:  f.FiscalYear,
		FundGroup:   f.FundGroup,
		Fund:        f.Fund,
		Department:  f.Department,
		Category:    f.Category,
		RowLabel:    f.RowLabel,
		Sign:        f.Sign,
		AmountCents: f.AmountCents,
		Derived:     f.Derived,
	}
}
