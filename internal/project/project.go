// Package project turns facts into the JSON documents the site renders.
//
// One projection is one file under <output>/data/, so `fisc export -o dist`
// writes dist/data/sankey.json and `fisc export -o site` writes
// site/data/sankey.json: one code path, one layout. The browser fetches the
// file; it is never inlined into the page, because a provenance document you
// cannot curl on its own is not much of an audit trail.
//
// # The seam
//
// [Projection] exists now, with exactly one implementation, because the site
// will grow a second and a third projection and retrofitting an interface
// under three concrete builders is the expensive version of this change. A
// projection takes facts and options and returns bytes; it does no I/O, does
// not know where its file goes, and does not know what wrote the facts.
//
// # Rules every projection follows
//
// Every key is present on every object, in declaration order — no omitempty,
// no null — the same discipline as [fact.Fact] and for the same reason: a key
// that vanishes when it is empty makes a diff between two releases read as a
// structural change. Absent strings are "", absent slices are [].
//
// Money is an integer count of cents, never a float and never a string.
// amount.Cents is deliberately not marshalled: it has a String method and
// someone will one day give it a MarshalJSON, at which point every published
// figure would silently change shape.
//
// Output is deterministic. Two Build calls over the same facts return
// byte-identical output, which is what lets a rebuild-and-diff check mean
// something.
//
// The frozen sankey.json contract is docs/sankey-contract.md, with the worked
// FY2026 example in testdata/sankey.golden.json. The contract file says it
// moves into this doc comment once this package exists; that move is a
// separate change from the one that created the package.
package project

import (
	"errors"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// SchemaVersion is the version stamped on every projection document. A
// consumer reads it first and refuses a document it does not understand,
// rather than discovering a renamed key halfway through rendering.
const SchemaVersion = 1

// Options are the slice of the corpus a projection is built from.
//
// The three selectors are not decoration. Every fiscal year the city publishes
// lives in the same facts.jsonl, and a projection that forgets to filter on
// one of them doubles every figure while still balancing perfectly — the
// failure mode that no internal consistency check can catch, because two years
// of a balanced schedule are also balanced.
type Options struct {
	// FiscalYear is the year to project. There is no "all years" value: a
	// Sankey of two budgets is not a chart of anything.
	FiscalYear int
	// Basis keeps a budgeted figure from being mixed with an audited one,
	// which is the most common way a civic budget chart misleads.
	Basis mapping.Basis
	// Scope selects the schedule a fact came from. It is also the guard that
	// keeps a second schedule's facts out of a projection built for the
	// citywide spine: a department-by-category page is a different scope, and
	// its rows would otherwise be added on top of the spine's.
	Scope string
	// Version is build.Get().String(), published as metadata.generated_by so
	// a reader can tell which binary produced the file.
	Version string
}

// Validate reports options that cannot produce a meaningful document.
//
// It is strict about the three selectors and about the version because each
// one appears in the published metadata: a document that says which year,
// basis and scope it covers, and which binary wrote it, is auditable, and one
// that leaves any of them blank is a chart with no caption.
func (o Options) Validate() error {
	if o.FiscalYear <= 0 {
		return fmt.Errorf("fiscal year is required (got %d)", o.FiscalYear)
	}
	switch o.Basis {
	case mapping.BasisAdopted, mapping.BasisRevised, mapping.BasisActual,
		mapping.BasisAudited, mapping.BasisProjected:
	default:
		return fmt.Errorf("basis %q is not one of adopted, revised, actual, audited, projected", o.Basis)
	}
	if o.Scope == "" {
		return errors.New("scope is required")
	}
	if o.Version == "" {
		return errors.New("version is required for metadata.generated_by")
	}
	return nil
}

// Projection is one JSON document under <output>/data/.
//
// Build returns bytes rather than writing them so that the packager owns every
// path decision and the projection owns none, and so a test can assert on the
// exact document without a filesystem.
type Projection interface {
	// Name is the file's stem: "sankey" becomes sankey.json.
	Name() string
	// Build renders the projection as canonical, deterministic JSON.
	Build(facts []fact.Fact, o Options) ([]byte, error)
}

// Registry returns the projections a build emits, in a stable order, each
// wired to l for its labels.
//
// It is a function rather than a package variable so no caller can append to
// the published set, and so each call hands back projections with no shared
// state. l may be nil, in which case a projection falls back to slug-derived
// labels rather than failing — but it is a parameter rather than a field the
// caller may forget to set, because forgetting it is silent: the page renders,
// and every node is labelled "Use Of Money And Property" instead of the words
// the city printed.
func Registry(l Labels) []Projection {
	return []Projection{&Sankey{Labels: l}}
}
