// Package project turns facts into the JSON documents the site renders.
//
// A projection is published in the output directory: inside the column it
// folds into, <output>/fy<year>-<basis>.json, or as <output>/data/<stem>.json
// where it states no column. The browser fetches the file; it is never
// inlined into the page.
//
// A [Projection] takes facts and options and returns bytes; it does no I/O.
//
// Every key is present on every object, in declaration order, except a field
// tagged omitempty, each for the reason on its field. Absent strings are "",
// absent slices are [], and no key is null except a pointer field whose doc
// says when, such as [Series.Fund].
//
// Money is an integer count of cents. amount.Cents is deliberately not
// marshalled, so a future MarshalJSON on it cannot change a published shape.
//
// Output is deterministic: two Build calls over the same facts return
// byte-identical output.
//
// Each document's published shape is a schema under schema/ (a graph's is
// projection.schema.json, a series' series.schema.json), and the FY2026 spine
// transcribed by hand is projecttest's. What each document reads off the
// printed pages, and how it reconciles with them, is under docs/outputs/.
package project

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// SchemaVersion is the version stamped on every projection document. A
// consumer reads it first and refuses a document it does not understand,
// rather than discovering a renamed key halfway through rendering.
const SchemaVersion = 1

// The slice of the fact store the site publishes, the one declaration
// `fisc export` builds and `fisc verify` checks.
const (
	// PublishedFiscalYear is the year the site opens on, a default and not a
	// limit: see [PublishedFiscalYears].
	PublishedFiscalYear = 2026
	PublishedBasis      = vocab.BasisAdopted
	PublishedScope      = structure.ScopeAllFundsGross
	// PublishedProjection is the stem of the document the page is built from.
	// internal/export keeps its own copy, and a test asserts they agree.
	PublishedProjection = "sankey"
)

// PublishedFiscalYears is every year the site publishes a document for, in the
// order a reader should meet them, as a fresh slice no caller can extend.
// Which slices are built is each projection's own Slices.
func PublishedFiscalYears() []int {
	return []int{2026, 2027}
}

// PublishedDocument is one file the site publishes, and the slice of the fact
// store it must be built over.
//
// Each entry carries its stem, since [Stem] depends on how many slices a
// projection declares at runtime. Each states its columns rather than reading
// them off the facts, so a corpus that loses a column fails the check instead
// of agreeing with it by construction.
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
	Kinds []vocab.Kind
	// ThroughCuts is [Options.ThroughCuts]: a caller rebuilding the document's
	// selection from this declaration selects what the document drew.
	ThroughCuts bool
	// Columns is every (fiscal year, basis) pair the document must cover.
	Columns []Column
}

// String names the document the way a report should: the stem a reader can
// fetch, then the slice behind it.
func (d PublishedDocument) String() string {
	return fmt.Sprintf("%s (%s %s)", d.Stem, Describe(d.Columns),
		strings.Join(d.Scopes, ", "))
}

// PublishedDocuments is every file the site publishes, in the order a reader
// should meet them, as a fresh slice. Graph entries come from
// [publishedGraphs]; the series documents are stated here. Nothing consults
// the facts, so the facts can be reported for not covering it.
func PublishedDocuments() []PublishedDocument {
	var out []PublishedDocument
	for _, g := range publishedGraphs() {
		declared := g.options()
		for _, o := range declared {
			out = append(out, PublishedDocument{
				Projection:  g.projection,
				Stem:        stemOrPanic(g.projection, o, declared),
				Scopes:      slices.Clone(g.scopes),
				Kinds:       slices.Clone(g.kinds),
				ThroughCuts: g.throughCuts,
				Columns:     slices.Clone(o.Columns),
			})
		}
	}

	out = append(out, PublishedDocument{
		Projection: TrendsProjection,
		Stem:       TrendsProjection,
		Scopes:     []string{TrendsScope},
		Columns:    TrendsColumns(),
	})

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
	out = append(out, PublishedDocument{
		Projection: CIPProjectsProjection,
		Stem:       CIPProjectsProjection,
		Scopes:     []string{CIPProjectsScope},
		Columns:    CIPProjectsColumns(),
	})
	out = append(out, PublishedDocument{
		Projection: DebtServiceProjection,
		Stem:       DebtServiceProjection,
		Scopes:     DebtServiceScopes(),
		Columns:    DebtServiceColumns(),
	})
	return out
}

// publishedGraph is one graph projection's published declaration: the
// schedule set it draws, the kinds it selects out of them, and every column
// the site publishes it over, stated for [PublishedDocuments]' reason.
type publishedGraph struct {
	projection  string
	scopes      []string
	kinds       []vocab.Kind
	throughCuts bool
	columns     []Column
}

// options is one Options per published column, as the projection's Slices
// would declare them over a corpus carrying every one.
func (g publishedGraph) options() []Options {
	out := make([]Options, 0, len(g.columns))
	for _, c := range g.columns {
		out = append(out, Options{Columns: []Column{c}, Scopes: slices.Clone(g.scopes),
			Kinds: slices.Clone(g.kinds), ThroughCuts: g.throughCuts})
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
		{projection: TransfersOutProjection, scopes: TransfersOutScopes(), kinds: (&transfersByFund{Out: true}).kinds(), columns: adoptedColumns()},
		{projection: FundSourcesUsesProjection, scopes: FundSourcesUsesScopes(), throughCuts: true, columns: budgetBookDetailColumns()},
		{projection: ExpansionProjection, scopes: ExpansionScopes(), columns: adoptedColumns()},
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
		{FiscalYear: 2024, Basis: vocab.BasisActual},
		{FiscalYear: 2025, Basis: vocab.BasisRevised},
		{FiscalYear: 2026, Basis: vocab.BasisAdopted},
		{FiscalYear: 2027, Basis: vocab.BasisAdopted},
	}
}

// columnsCarrying is the Slices rule every graph projection answers with: one
// single-column Options per (fiscal year, basis) in which EVERY required scope
// carries a fact of the selected kinds, oldest first. One Options per column:
// a graph over two years doubles every figure and still balances.
func columnsCarrying(facts []fact.Fact, required, scopes []string, kinds []vocab.Kind, version string) []Options {
	sel := Options{Scopes: required, Kinds: kinds}
	seen := map[Column]map[string]bool{}
	for i := range facts {
		f := &facts[i]
		if !sel.HasScope(f.Scope) || !sel.HasKind(f.Kind) {
			continue
		}
		c := ColumnOf(f)
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
// it was actually built under do not cover: the one spelling `fisc verify` and
// `fisc export` share. A different scope set misses every column; scopes
// compare as a set, not in declaration order.
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

// sameScopes reports whether two scope sets name the same schedules. It sorts
// copies: the arguments' order is published.
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

// stemOrPanic is [Stem] where the arguments are this package's own literals and
// an error is a bug: publishedGraph.options gives each Options a column.
func stemOrPanic(name string, o Options, declared []Options) string {
	stem, err := Stem(name, o, declared)
	if err != nil {
		panic(err)
	}
	return stem
}

// Stem is the file stem one of a projection's documents is written under, the
// only place that rule is spelled.
//
// declared is every Options the projection declared, not a count, so no caller
// can arrive at the count differently. A projection declaring one document
// takes its name verbatim (data/revenue-trends.json). Otherwise the stem is a
// function of the whole column list (fisc-rmx); the opening published slice
// keeps the bare name (data/sankey.json), and a non-published basis is spelled
// out.
func Stem(name string, o Options, declared []Options) (string, error) {
	if len(declared) == 1 {
		return name, nil
	}
	if len(o.Columns) == 0 {
		return "", fmt.Errorf(
			"the %s projection declared %d documents and one of them has no columns, "+
				"so there is nothing to tell it apart by", name, len(declared))
	}
	if len(o.Columns) == 1 && o.Columns[0] == (Column{FiscalYear: PublishedFiscalYear, Basis: PublishedBasis}) {
		return name, nil
	}
	parts := make([]string, 0, len(o.Columns))
	for _, c := range o.Columns {
		part := strconv.Itoa(c.FiscalYear)
		if c.Through != 0 {
			part += "-" + strconv.Itoa(c.Through)
		}
		if c.Basis != PublishedBasis {
			part += "-" + string(c.Basis)
		}
		parts = append(parts, part)
	}
	return name + "-" + strings.Join(parts, "-"), nil
}

// Options are the slice of the corpus a projection is built from. Every year
// lives in one facts.jsonl, and a projection that forgets a selector doubles
// every figure while still balancing.
type Options struct {
	// Columns is the (fiscal year, basis) pairs to project, in the order a
	// reader should meet them. Never empty: there is no absent-means-every-year
	// (AGENTS.md, Provenance invariants). [Sankey] refuses more than one.
	Columns []Column
	// Scopes selects the schedules a fact may have come from, keeping a second
	// schedule's rows off the spine's. Schedules disjoint by kind may combine;
	// two that restate the same money double it, which a check refuses
	// (fisc-gkv point B), not this type. A single-schedule document reads
	// [Options.onlyScope].
	Scopes []string
	// Kinds narrows the slice to facts of these kinds, or is empty for every
	// kind the scopes carry. A document of one kind of money over a scope
	// printing several needs it: p222 prints transfers, grants and a balance
	// draw under one total, and the transfer network draws the transfers.
	Kinds []vocab.Kind
	// ThroughCuts narrows the slice to the facts the view over Scopes admits
	// (structure.ViewOf): a scope whose pages print a block no cut admits
	// leaves that block out. It is opt-in, because a document may draw a
	// scope's residue on purpose -- department-spending draws pp.85-125's
	// rows no cut reads -- and the checks select through the same Options the
	// document was built under.
	ThroughCuts bool
	// Version is build.Get().String(), published as metadata.generated_by so
	// a reader can tell which binary produced the file.
	Version string
}

// Column is one printed column of a schedule: a fiscal year read on one basis.
// Both components are the key; neither determines the other.
type Column struct {
	FiscalYear int
	Basis      vocab.Basis
	// Through is the last year of a band, and zero for a column of one year.
	// Budget Book p80 prints 2031-2035 as one figure, so a column naming
	// FY2031 alone does not select it: a document draws a band only by
	// naming the band (see SelectFacts).
	Through int
}

// ColumnOf is the column a fact is printed in, its band included. Every
// Column built from a fact is built here, so none can drop the band.
func ColumnOf(f *fact.Fact) Column {
	return Column{FiscalYear: f.FiscalYear, Basis: f.Basis, Through: fact.BandOf(f)}
}

// String renders a column the way a report should name one.
func (c Column) String() string { return fact.BandLabel(c.FiscalYear, c.Through, c.Basis) }

// Describe renders a set of columns for a message.
func Describe(cols []Column) string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.String())
	}
	return strings.Join(out, ", ")
}

// validate reports options that cannot produce a meaningful document. Each
// selector and the version appear in the published metadata.
func (o Options) validate() error {
	if len(o.Columns) == 0 {
		return errors.New("at least one column is required")
	}
	seen := make(map[Column]bool, len(o.Columns))
	for _, c := range o.Columns {
		if c.FiscalYear <= 0 {
			return fmt.Errorf("fiscal year is required (got %d)", c.FiscalYear)
		}
		if c.Through != 0 && c.Through <= c.FiscalYear {
			return fmt.Errorf("column %s ends its band before it begins", c)
		}
		if !c.Basis.Valid() {
			return fmt.Errorf("basis %q is not one of %v", c.Basis, vocab.Bases())
		}
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
		// SelectFacts tests membership, but a reader that counts scopes
		// would see a repeat as another schedule.
		if scopes[s] {
			return fmt.Errorf("scope %q is listed twice", s)
		}
		scopes[s] = true
	}
	kinds := make(map[vocab.Kind]bool, len(o.Kinds))
	for _, k := range o.Kinds {
		if kinds[k] {
			return fmt.Errorf("kind %q is listed twice", k)
		}
		kinds[k] = true
	}
	if o.Version == "" {
		return errors.New("version is required for metadata.generated_by")
	}
	if o.ThroughCuts {
		if _, err := o.view(); err != nil {
			return fmt.Errorf("selecting through the cuts: %w", err)
		}
	}
	return nil
}

// view is the view a ThroughCuts selection admits facts through.
func (o Options) view() (structure.View, error) {
	return structure.ViewOf("selection", o.Scopes)
}

// onlyScope is the one schedule a single-schedule document is of, and the
// refusal of any other shape, written once so no document publishes one
// metadata.scope for two. It answers how many, not which: the caller compares
// the scope against its own constant.
func (o Options) onlyScope() (string, error) {
	if len(o.Scopes) != 1 {
		return "", hint.With(
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
func (o Options) HasKind(k vocab.Kind) bool {
	return len(o.Kinds) == 0 || slices.Contains(o.Kinds, k)
}

// ScopeList is the scope set as one string, in declared order, not sorted.
func (o Options) ScopeList() string {
	return strings.Join(o.Scopes, ", ")
}

// Projection is one JSON document under <output>/data/. Build returns bytes;
// the packager owns every path.
type Projection interface {
	// Name is the file's stem: "sankey" becomes sankey.json.
	Name() string
	// Build renders the projection as canonical, deterministic JSON.
	Build(facts []fact.Fact, o Options) ([]byte, error)
	// Slices returns the options this projection is built under, in a stable
	// order, given the whole fact store: a Sankey one per year, a trend one
	// spanning every year. An empty result means the store carries nothing
	// this projection is of, which is a statement about the corpus and not an
	// error.
	Slices(facts []fact.Fact, version string) []Options
}

// Registry returns the projections a build emits, in a stable order, each
// wired to l for its labels. l may be nil, falling back to slug-derived
// labels; it is a parameter so a caller cannot silently forget it.
func Registry(l labels) []Projection {
	return []Projection{
		&sankey{Labels: l},
		&Trends{Labels: l},
		&fundFlows{Labels: l},
		&departmentSpending{Labels: l},
		&departmentFunding{Labels: l},
		&transfersByFund{Labels: l},
		&transfersByFund{Labels: l, Out: true},
		&fundSourcesUses{Labels: l},
		&expansion{Labels: l},
		&FundBalanceChanges{Labels: l},
		&FundBalances{Labels: l},
		&CIPProjects{Labels: l},
		&DebtService{},
	}
}
