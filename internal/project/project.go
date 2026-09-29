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
// structural change. Absent strings are "", absent slices are []. The one key
// a document may omit is [Metadata.Headline], for the reason on that field.
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
	"slices"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
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
	PublishedScope      = structure.ScopeAllFundsGross
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
	// Scopes is the schedule set it is of, matching [Options.Scopes]. Compared
	// as a SET -- see [MissingColumns].
	Scopes []string
	// Kinds is the kind set it selects out of those schedules, matching
	// [Options.Kinds]: empty for every kind the schedules carry.
	Kinds []mapping.Kind
	// Columns is every (fiscal year, basis) pair the document must cover. A
	// document missing one of these was built, but not over what the site
	// promised, and that is a finding rather than silence.
	Columns []Column
}

// String names the document the way a report should: the stem a reader can
// fetch, then the slice behind it.
func (d PublishedDocument) String() string {
	return fmt.Sprintf("%s (%s %s)", d.Stem, Describe(d.Columns),
		strings.Join(d.Scopes, ", "))
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
			Scopes:     []string{PublishedScope},
			Columns:    []Column{{FiscalYear: year, Basis: PublishedBasis}},
		})
	}
	out = append(out, PublishedDocument{
		Projection: TrendsProjection,
		Stem:       TrendsProjection,
		Scopes:     []string{TrendsScope},
		Columns:    TrendsColumns(),
	})

	for _, g := range publishedGraphs()[1:] {
		declared := g.options()
		for _, o := range declared {
			out = append(out, PublishedDocument{
				Projection: g.projection,
				Stem:       stemOrPanic(g.projection, o, declared),
				Scopes:     slices.Clone(g.scopes),
				Kinds:      slices.Clone(g.kinds),
				Columns:    slices.Clone(o.Columns),
			})
		}
	}

	// The two ACFR ten-year schedules, one document each: two row axes, two
	// scopes, and seriesSpec's one-schedule rule keeps them apart. Their
	// columns are stated by [HistoryColumns], for TrendsColumns' reason.
	out = append(out, PublishedDocument{
		Projection: ChangesProjection,
		Stem:       ChangesProjection,
		Scopes:     []string{ChangesScope},
		Columns:    HistoryColumns(),
	})
	out = append(out, PublishedDocument{
		Projection: FundBalancesProjection,
		Stem:       FundBalancesProjection,
		Scopes:     []string{FundBalancesScope},
		Columns:    HistoryColumns(),
	})
	return out
}

// publishedGraph is one graph projection's published declaration: the
// schedule set it draws, the kinds it selects out of them, and every column
// the site publishes it over.
//
// Stated rather than read off the facts, for [PublishedDocuments]' reason: a
// declaration that consulted the store could not report that the store stopped
// covering it.
type publishedGraph struct {
	projection string
	scopes     []string
	kinds      []mapping.Kind
	columns    []Column
}

// options is one Options per published column, as the projection's Slices
// would declare them over a corpus carrying every one.
func (g publishedGraph) options() []Options {
	out := make([]Options, 0, len(g.columns))
	for _, c := range g.columns {
		out = append(out, Options{Columns: []Column{c}, Scopes: slices.Clone(g.scopes), Kinds: slices.Clone(g.kinds)})
	}
	return out
}

// publishedGraphs is every graph the site publishes, the spine first. The
// detail schedules publish all four printed columns: each is the only document
// drawing its pages, and two of four would be half a schedule with nothing
// saying which half. p76 prints four and its rules skip the two historical
// ones, which miss the page's own grand total by millions; p222 prints its
// revised column beside no p76 column.
func publishedGraphs() []publishedGraph {
	return []publishedGraph{
		{projection: PublishedProjection, scopes: []string{PublishedScope}, columns: adoptedColumns()},
		{projection: FundFlowsProjection, scopes: FundFlowsScopes(), columns: budgetBookDetailColumns()},
		{projection: DepartmentSpendingProjection, scopes: DepartmentSpendingScopes(), columns: budgetBookDetailColumns()},
		{projection: DepartmentFundingProjection, scopes: DepartmentFundingScopes(), columns: budgetBookDetailColumns()},
		{projection: TransfersByFundProjection, scopes: TransfersByFundScopes(), columns: adoptedColumns()},
		{projection: TransfersOutProjection, scopes: TransfersOutScopes(), kinds: transferKinds, columns: adoptedColumns()},
	}
}

// adoptedColumns are the two columns pp.66-67 and p76 print, oldest first.
func adoptedColumns() []Column {
	out := make([]Column, 0, len(PublishedFiscalYears()))
	for _, y := range PublishedFiscalYears() {
		out = append(out, Column{FiscalYear: y, Basis: PublishedBasis})
	}
	return out
}

// budgetBookDetailColumns are the four columns the Budget Book's detail
// schedules print, oldest first.
func budgetBookDetailColumns() []Column {
	return []Column{
		{FiscalYear: 2024, Basis: mapping.BasisActual},
		{FiscalYear: 2025, Basis: mapping.BasisRevised},
		{FiscalYear: 2026, Basis: mapping.BasisAdopted},
		{FiscalYear: 2027, Basis: mapping.BasisAdopted},
	}
}

// columnsCarrying is the Slices rule every graph projection answers with: one
// single-column Options per (fiscal year, basis) in which EVERY required scope
// carries a fact of the selected kinds, oldest first.
//
// The years are read off the facts rather than hard-coded, so mapping a
// revised column or a third budget year puts that document under verify's
// checks without anyone remembering to add it here. ONE OPTIONS PER COLUMN:
// every fiscal year the city publishes lives in the same facts.jsonl, and a
// graph built over two of them doubles every figure while still balancing.
func columnsCarrying(facts []fact.Fact, required, scopes []string, kinds []mapping.Kind, version string) []Options {
	sel := Options{Scopes: required, Kinds: kinds}
	seen := map[Column]map[string]bool{}
	for i := range facts {
		f := &facts[i]
		if !sel.HasScope(f.Scope) || !sel.HasKind(f.Kind) {
			continue
		}
		c := Column{FiscalYear: f.FiscalYear, Basis: f.Basis}
		if seen[c] == nil {
			seen[c] = map[string]bool{}
		}
		seen[c][f.Scope] = true
	}
	var cols []Column
	for c, have := range seen {
		every := true
		for _, sc := range required {
			every = every && have[sc]
		}
		if every {
			cols = append(cols, c)
		}
	}
	sortColumns(cols)
	out := make([]Options, 0, len(cols))
	for _, c := range cols {
		out = append(out, Options{
			Columns: []Column{c}, Scopes: slices.Clone(scopes), Kinds: slices.Clone(kinds), Version: version,
		})
	}
	return out
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
// The scope set is compared too. A document built at the right stem over the
// right years but a different SCHEDULE is a different document wearing the path,
// and the column list alone cannot see that.
//
// THE COMPARISON IS SET EQUALITY AND NOT SLICE EQUALITY. Two projections
// declaring the same two schedules in different orders are of the same money,
// and a document that had to be satisfied in declaration order would report a
// missing document over an ordering choice. Order still matters for the
// PUBLISHED text -- [Options.ScopeList] preserves it -- because a reader looking
// up a declaration will look for the order it was written in.
func MissingColumns(d PublishedDocument, o Options) []Column {
	if !sameScopes(d.Scopes, o.Scopes) {
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
		Scopes:  []string{PublishedScope},
	}
}

// sameScopes reports whether two scope sets name the same schedules.
//
// Sorted copies rather than a map, because these sets are one or two entries
// long and a map allocation per comparison would be the expensive way to answer
// a question about a pair of strings. The copies matter: sorting the arguments
// in place would reorder a projection's own declaration, which is published.
func sameScopes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := slices.Clone(a)
	y := slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// spineSlices is every document the spine publishes, as [sankey.Slices] would
// declare them over a corpus that covers the published years. It is what
// [PublishedDocuments] hands [Stem], so the declaration names its files by the
// same rule and over the same list `fisc export` will.
func spineSlices() []Options {
	return publishedGraphs()[0].options()
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
	// still balancing perfectly. Absent is not zero (AGENTS.md, Provenance invariants).
	Columns []Column
	// Scopes selects the schedules a fact may have come from. It is also the
	// guard that keeps a second schedule's facts out of a projection built for
	// the citywide spine: a department-by-category page is a different scope,
	// and its rows would otherwise be added on top of the spine's.
	//
	// IT IS A SET AND NOT A STRING because a drill-down is of more than one
	// schedule. pp.127-140 print revenue by fund and pp.167-170 print General
	// Fund expenditure by department, and a document following a dollar from
	// source to spend needs both. They are safe together because they are
	// DISJOINT BY KIND -- every key this package builds carries kind, so a
	// revenue cell and an expenditure cell can never collide.
	//
	// A SET IS NOT A LICENCE TO COMBINE ANY TWO. Two scopes that restate the
	// same money double it, silently, with every check green: measured on the
	// committed store, revenue-by-fund and transfers-by-fund both publish
	// transfer_in and overlap by 21,045,597 in FY2026. The rule is fisc-gkv
	// point B -- a scope carrying an <x>-ties-to-spine check is by that fact
	// ineligible to sit beside the spine scope -- and it is enforced by a
	// check rather than by this type, because a set is the wrong place to hold
	// a claim about arithmetic.
	//
	// MOST DOCUMENTS ARE OF EXACTLY ONE, and say so through [Options.OnlyScope]
	// rather than by indexing this field.
	Scopes []string
	// Kinds narrows the slice to facts of these kinds, or is empty for every
	// kind the scopes carry. A document of one kind of money over a scope
	// printing several needs it: p222 prints transfers, grants and a balance
	// draw under one total, and the transfer network draws the transfers.
	Kinds []mapping.Kind
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

// validate reports options that cannot produce a meaningful document.
//
// It is strict about the three selectors and about the version because each
// one appears in the published metadata: a document that says which year,
// basis and scope it covers, and which binary wrote it, is auditable, and one
// that leaves any of them blank is a chart with no caption.
func (o Options) validate() error {
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
	if len(o.Scopes) == 0 {
		return errors.New("at least one scope is required")
	}
	scopes := make(map[string]bool, len(o.Scopes))
	for _, s := range o.Scopes {
		if s == "" {
			return errors.New("a scope may not be empty")
		}
		// A repeated scope is not merely redundant: SelectFacts tests
		// membership, so the same fact would be selected once however many
		// times its scope is listed -- but every reader of this field that
		// COUNTS scopes (the report strings, the disjointness check) would see
		// a document claiming more schedules than it has.
		if scopes[s] {
			return fmt.Errorf("scope %q is listed twice", s)
		}
		scopes[s] = true
	}
	kinds := make(map[mapping.Kind]bool, len(o.Kinds))
	for _, k := range o.Kinds {
		if kinds[k] {
			return fmt.Errorf("kind %q is listed twice", k)
		}
		kinds[k] = true
	}
	if o.Version == "" {
		return errors.New("version is required for metadata.generated_by")
	}
	return nil
}

// onlyScope is the one schedule a single-schedule document is of, and the
// refusal of any other shape.
//
// IT EXISTS SO THE REFUSAL IS WRITTEN ONCE. sankey.Document, Trends.Document and
// envelope() each need "this document is of exactly one scope, and here it is",
// and while [Options.Scopes] was a string all three spelled it as a field read
// with no refusal at all -- correct only because the type could not hold a
// second scope. A set can, so the three would each have had to grow the same
// guard, and the one that did not would publish half a set as the whole of it:
// metadata.scope naming one schedule for a document built over two is a false
// claim that no check reads the JSON closely enough to catch.
//
// The caller still compares the returned scope against its OWN constant. This
// answers "how many", not "which one" -- a projection built over the wrong
// single schedule is a different error and each projection reports it in its
// own words.
func (o Options) onlyScope() (string, error) {
	if len(o.Scopes) != 1 {
		return "", cmdutil.WithHint(
			fmt.Errorf("this document is of one schedule, and these options name %d: %s",
				len(o.Scopes), o.ScopeList()),
			"a document spanning schedules needs its own projection type; the ones that "+
				"publish a single metadata.scope cannot describe two")
	}
	return o.Scopes[0], nil
}

// HasScope reports whether a fact carrying this scope is in the slice.
func (o Options) HasScope(scope string) bool {
	return slices.Contains(o.Scopes, scope)
}

// HasKind reports whether a fact of this kind is in the slice.
func (o Options) HasKind(k mapping.Kind) bool {
	return len(o.Kinds) == 0 || slices.Contains(o.Kinds, k)
}

// ScopeList is the scope set as one string, for a report line or an error.
//
// Joined with ", " and NOT sorted: the order is the order the projection
// declared, which is the order a reader of that declaration will look for. Two
// runs agree because the declaration is a literal, not a map walk.
func (o Options) ScopeList() string {
	return strings.Join(o.Scopes, ", ")
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
func Registry(l labels) []Projection {
	return []Projection{
		&sankey{Labels: l},
		&Trends{Labels: l},
		&fundFlows{Labels: l},
		&departmentSpending{Labels: l},
		&departmentFunding{Labels: l},
		&transfersByFund{Labels: l},
		&transfersByFund{Labels: l, Out: true},
		&FundBalanceChanges{Labels: l},
		&FundBalances{Labels: l},
	}
}
