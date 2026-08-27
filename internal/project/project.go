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
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// SchemaVersion is the version stamped on every projection document. A
// consumer reads it first and refuses a document it does not understand,
// rather than discovering a renamed key halfway through rendering.
const SchemaVersion = 1

// The slice of the fact store the site publishes.
//
// Both budget years live in one facts.jsonl, so the year is not optional: a
// projection built over both doubles every figure and still balances (see
// [Options]). These are declared here, beside the type that carries them, because
// two commands need the same answer — `fisc export` builds this slice and `fisc
// verify` checks it — and while there were two copies, verify could pass a graph
// the site does not publish and nothing would say so.
//
// The version is deliberately not here: it is a property of the binary, not of
// the slice.
const (
	// PublishedFiscalYear is the year the site OPENS on. It is not the only
	// year published — see [PublishedFiscalYears] — and the distinction is the
	// difference between a default and a limit.
	PublishedFiscalYear = 2026
	PublishedBasis      = mapping.BasisAdopted
	PublishedScope      = "all-funds-gross"
	// PublishedProjection is the stem of the document the page is built from.
	//
	// It is here with the slice rather than only in internal/export because it
	// is the fourth part of the same declaration: since each projection is built
	// over the slices IT declares, naming a year, a basis and a schedule no
	// longer names a document. internal/export keeps its own copy for the page
	// it renders, and the two agreeing is what a test asserts.
	PublishedProjection = "sankey"
)

// PublishedFiscalYears is every year the site publishes a document for, in the
// order a reader should meet them.
//
// It is a FUNCTION returning a fresh slice rather than a package variable,
// because a variable would let any caller reorder or extend the published set
// by accident, and the whole point of this declaration is that `fisc export`
// and `fisc verify` cannot disagree about what it contains.
//
// BOTH YEARS WERE ALWAYS PROJECTED AND CHECKED. internal/check derives one
// projection per (fiscal year, basis) the spine carries, so FY2027 has been
// building and passing every graph check since the two-year budget book was
// mapped. What it never was, is exported: this list is what closes that gap,
// and until it existed the site published one of the two documents that already
// passed (fisc-kwq).
func PublishedFiscalYears() []int {
	return []int{2026, 2027}
}

// PublishedDocument is one file the site publishes, and the slice of the fact
// store it must be built over.
//
// THE PUBLISHED SET IS A LIST OF DOCUMENTS, NOT ONE DOCUMENT AND A YEAR LIST.
// Until 2026-08-25 those were the same thing: every published file was a year of
// the spine, so naming a projection, a basis and a scope and then iterating
// [PublishedFiscalYears] named all of them. revenue-trends ended that. It is one
// document, at its own scope, spanning four columns and no year at all, and a
// year list cannot express it. Everything downstream that asked "is the
// published slice among the built ones" had to become "is every published
// DOCUMENT among them".
//
// EACH ENTRY CARRIES ITS STEM, and that is not redundancy with the projection
// name. A stem is not derivable from a declaration: the packager's stemFor asks
// how many slices a projection declared AT RUNTIME -- one slice takes the name
// verbatim, several are suffixed by year through [PublishedStem] -- so a
// declaration that omitted the stem would force `fisc export` to re-derive that
// rule against a static list, which is the second copy this whole declaration
// exists to prevent. docs/revenue-trends-contract.md is explicit that the trends
// document must NOT go through PublishedStem: it would write two byte-identical
// files, one of which is a lie about which year it covers.
//
// EACH ENTRY STATES ITS COLUMNS RATHER THAN DERIVING THEM, and that is the
// property that makes the check able to fail. [Trends.Slices] reads its columns
// off the fact store, exhaustively and on purpose. A published set that did the
// same would agree with the corpus by construction: it would go quiet in exactly
// the state it exists to catch, because a corpus that lost a column would be
// declared to publish one column fewer. Stating them is what turns "the corpus
// changed" into "the corpus no longer contains what the site promised".
type PublishedDocument struct {
	// Projection is the [Projection.Name] of the projection that must build it.
	Projection string
	// Stem is the file stem it is published under, without the .json.
	Stem string
	// Scope is the schedule it is of, matching [Options.Scope].
	Scope string
	// Columns is every (fiscal year, basis) pair the document must cover. A
	// document missing one of these was built, but not over what the site
	// promised, and that is a finding rather than silence.
	Columns []Column
}

// String names the document the way a report should: the stem a reader can
// fetch, then the slice behind it.
func (d PublishedDocument) String() string {
	return fmt.Sprintf("%s (%s %s)", d.Stem, Describe(d.Columns), d.Scope)
}

// PublishedDocuments is every file the site publishes, in the order a reader
// should meet them.
//
// A FUNCTION RETURNING A FRESH SLICE, for [PublishedFiscalYears]'s reason: a
// package variable would let any caller reorder or extend the published set by
// accident, and the point of this declaration is that `fisc export` and `fisc
// verify` cannot disagree about what it contains. The spine's entries are
// derived from that list so the two cannot drift; the trends entry is stated,
// because nothing else states it.
//
// Nothing here is checked against the corpus at declaration time, deliberately.
// A published set that consulted the facts could not report that the facts stop
// covering it, which is the only thing it is for.
func PublishedDocuments() []PublishedDocument {
	years := PublishedFiscalYears()
	out := make([]PublishedDocument, 0, len(years)+1)
	for _, year := range years {
		out = append(out, PublishedDocument{
			Projection: PublishedProjection,
			Stem:       stemOrPanic(PublishedProjection, spineOptions(year), spineSlices()),
			Scope:      PublishedScope,
			Columns:    []Column{{FiscalYear: year, Basis: PublishedBasis}},
		})
	}
	return append(out, PublishedDocument{
		Projection: TrendsProjection,
		Stem:       TrendsProjection,
		Scope:      TrendsScope,
		Columns:    TrendsColumns(),
	})
}

// MissingColumns is the columns a published document promises that the Options
// it was actually built under do not cover.
//
// It lives here rather than in either caller because BOTH have to make the
// comparison and neither may make it differently: `fisc verify` asks it of the
// fact store's projections and `fisc export` asks it of the documents it just
// wrote. Two spellings of "does this document cover what we said" is the class
// of divergence PublishedDocuments exists to remove.
//
// The scope is compared too. A document built at the right stem over the right
// years but a different SCHEDULE is a different document wearing the path, and
// the column list alone cannot see that.
func MissingColumns(d PublishedDocument, o Options) []Column {
	if o.Scope != d.Scope {
		return d.Columns
	}
	have := make(map[Column]bool, len(o.Columns))
	for _, c := range o.Columns {
		have[c] = true
	}
	var missing []Column
	for _, c := range d.Columns {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	return missing
}

// spineOptions is one published year of the spine, as the projection would be
// built over it. It exists so [PublishedDocuments] computes its stems through
// [Stem] rather than restating the rule.
func spineOptions(year int) Options {
	return Options{
		Columns: []Column{{FiscalYear: year, Basis: PublishedBasis}},
		Scope:   PublishedScope,
	}
}

// spineSlices is every document the spine publishes, as [Sankey.Slices] would
// declare them over a corpus that covers the published years. It is what
// [PublishedDocuments] hands [Stem], so the declaration names its files by the
// same rule and over the same list `fisc export` will.
func spineSlices() []Options {
	years := PublishedFiscalYears()
	out := make([]Options, 0, len(years))
	for _, y := range years {
		out = append(out, spineOptions(y))
	}
	return out
}

// stemOrPanic is [Stem] where the arguments are this package's own literals and
// an error is a bug here rather than a state a corpus can reach. Stem's only
// error is an Options with no columns, and spineOptions always has one; a
// returned error would have to be swallowed or would have to make
// PublishedDocuments fallible, and a published set that can fail to be stated
// is worse than a panic on a line no input reaches.
func stemOrPanic(name string, o Options, declared []Options) string {
	stem, err := Stem(name, o, declared)
	if err != nil {
		panic(err)
	}
	return stem
}

// Stem is the file stem one of a projection's documents is written under, and
// it is the ONLY place that rule is spelled. `fisc export` writes the files,
// `fisc verify` says one was not built, and [PublishedDocuments] declares them;
// three spellings of a path is three chances for the site to serve a document
// under a name nothing else expects.
//
// declared is EVERY Options the projection declared, and o must be one of them.
// It is the whole list rather than a count because a count is a number three
// callers can each arrive at differently -- and did: `fisc export` counted the
// slices a projection declared, internal/check counted the ones that BUILT and
// only within one scope, and PublishedDocuments counted published YEARS. Three
// spellings of the argument that the single spelling of the rule was supposed to
// remove. Passing the list makes the disagreement a compile error's worth of
// obvious instead of a stem nobody notices, and lets this function say so when a
// caller hands it an Options the projection never declared.
//
// A projection publishing ONE document needs no distinguishing suffix and takes
// its name verbatim -- which is what keeps data/revenue-trends.json at the path
// docs/revenue-trends-contract.md promises even in a corpus that carries a
// single column. The error in the other direction is the one that contract
// names: a multi-column document suffixed per year is written twice, as two
// byte-identical files one of which claims a year it does not cover.
//
// IT IS A FUNCTION OF THE WHOLE COLUMN LIST, and until fisc-rmx it was a
// function of Columns[0].FiscalYear alone. That was reachable and it took the
// whole export down rather than shipping something wrong: Sankey.Slices derives
// one slice per (fiscal year, BASIS) the spine carries, so the day an FY2026
// revised column is mapped beside the adopted one, two slices computed the stem
// "sankey" and the duplicate-stem guard refused every file including the ones
// that were fine. Sankey.Slices' own doc comment presents mapping a revised
// column as needing no other change; that is true again.
//
// The opening published slice keeps the bare name, so data/sankey.json stays
// the path docs/sankey-contract.md promises and every existing link to it keeps
// working. A basis that is not the published one is spelled out, because a year
// alone cannot tell two of them apart.
func Stem(name string, o Options, declared []Options) (string, error) {
	if len(declared) == 1 {
		return name, nil
	}
	if len(o.Columns) == 0 {
		return "", fmt.Errorf(
			"the %s projection declared %d documents and one of them has no columns, "+
				"so there is nothing to tell it apart by", name, len(declared))
	}
	if len(o.Columns) == 1 && o.Columns[0] == (Column{PublishedFiscalYear, PublishedBasis}) {
		return name, nil
	}
	parts := make([]string, 0, len(o.Columns))
	for _, c := range o.Columns {
		part := strconv.Itoa(c.FiscalYear)
		if c.Basis != PublishedBasis {
			part += "-" + string(c.Basis)
		}
		parts = append(parts, part)
	}
	return name + "-" + strings.Join(parts, "-"), nil
}

// Options are the slice of the corpus a projection is built from.
//
// The three selectors are not decoration. Every fiscal year the city publishes
// lives in the same facts.jsonl, and a projection that forgets to filter on
// one of them doubles every figure while still balancing perfectly — the
// failure mode that no internal consistency check can catch, because two years
// of a balanced schedule are also balanced.
type Options struct {
	// Columns is the (fiscal year, basis) pairs to project, in the order a
	// reader should meet them. It is never empty.
	//
	// IT IS A LIST AND NOT A PAIR because a document need not be of one year.
	// A Sankey of two budgets is not a chart of anything, so [Sankey] refuses
	// any Options with more than one column; a four-year trend of one printed
	// row is a chart of exactly one thing, and cannot be expressed at all if
	// the year is singular. [Sliced]'s doc comment promised that document
	// before this field could hold it (fisc-ai0).
	//
	// THE ABSENT-MEANS-EVERY-YEAR DESIGN IS DELIBERATELY NOT AVAILABLE. A zero
	// FiscalYear meaning "all of them" would be smaller, and it would put the
	// failure mode this whole type exists to prevent back inside it: every
	// fiscal year the city publishes lives in the same facts.jsonl, and a
	// projection that quietly selected two of them doubles every figure while
	// still balancing perfectly. Absent is not zero (docs/agents/conventions.md).
	Columns []Column
	// Scope selects the schedule a fact came from. It is also the guard that
	// keeps a second schedule's facts out of a projection built for the
	// citywide spine: a department-by-category page is a different scope, and
	// its rows would otherwise be added on top of the spine's.
	Scope string
	// Version is build.Get().String(), published as metadata.generated_by so
	// a reader can tell which binary produced the file.
	Version string
}

// Column is one printed column of a schedule: a fiscal year read on one basis.
//
// BOTH COMPONENTS ARE THE KEY, and neither is redundant. Measured on the
// committed store: FY2026 and FY2027 are BOTH adopted, so the basis alone does
// not identify a column; and within any one schedule the year-to-basis map is
// 1:1 (2024/actual, 2025/revised, 2026/adopted, 2027/adopted), so the year alone
// carries the basis only by accident of what is mapped today. A schedule
// printing an actual and a revised figure for the same year would break the
// second half, which is why the pair is stored rather than derived.
type Column struct {
	FiscalYear int
	Basis      mapping.Basis
}

// String renders a column the way a report should name one.
func (c Column) String() string { return fmt.Sprintf("FY%d %s", c.FiscalYear, c.Basis) }

// Describe renders a set of columns for a message. It is here rather than at
// each call site because three packages format the same list and a reader
// comparing two findings should not have to notice that they differ.
func Describe(cols []Column) string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.String())
	}
	return strings.Join(out, ", ")
}

// Validate reports options that cannot produce a meaningful document.
//
// It is strict about the three selectors and about the version because each
// one appears in the published metadata: a document that says which year,
// basis and scope it covers, and which binary wrote it, is auditable, and one
// that leaves any of them blank is a chart with no caption.
func (o Options) Validate() error {
	if len(o.Columns) == 0 {
		return errors.New("at least one column is required")
	}
	seen := make(map[Column]bool, len(o.Columns))
	for _, c := range o.Columns {
		if c.FiscalYear <= 0 {
			return fmt.Errorf("fiscal year is required (got %d)", c.FiscalYear)
		}
		switch c.Basis {
		case mapping.BasisAdopted, mapping.BasisRevised, mapping.BasisActual,
			mapping.BasisAudited, mapping.BasisProjected:
		default:
			return fmt.Errorf("basis %q is not one of adopted, revised, actual, audited, projected", c.Basis)
		}
		// A repeated column would be counted twice by anything summing the
		// document, which is the same doubling the field's doc comment is
		// about, reached by a different route.
		if seen[c] {
			return fmt.Errorf("column %s is listed twice", c)
		}
		seen[c] = true
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

// Sliced is a projection that says which slices of the fact store it is of.
//
// It exists because the alternative was `fisc verify` deciding for it, and that
// decision does not generalise. internal/check used to derive one slice per
// (fiscal year, basis) present within the SPINE scope and build every registered
// projection over every one of them — correct while the Sankey was the only
// projection, and wrong for the first one that is of a different schedule, which
// would be handed a slice containing none of its facts.
//
// The projection is the right place to answer it: it is the thing that knows
// which schedule it draws and whether one document per year is meaningful for
// it. A Sankey of two budgets is not a chart of anything, so Sankey returns one
// slice per year; a four-year trend line is a chart of exactly one thing, so a
// trends projection returns a single slice spanning all four.
//
// A projection that does not implement this is still built — over the slices the
// caller chose — so this is an opt-in refinement rather than a new obligation.
type Sliced interface {
	// Slices returns the options this projection wants to be built under, in a
	// stable order, given the whole fact store. An empty result means the store
	// carries nothing this projection is of, which is a statement about the
	// corpus and not an error.
	Slices(facts []fact.Fact, version string) []Options
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
	return []Projection{&Sankey{Labels: l}, &Trends{Labels: l}}
}
