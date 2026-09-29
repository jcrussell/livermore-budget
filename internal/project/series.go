package project

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/schema"
)

// seriesSpec is one series document's identity: the parts of a
// one-series-per-printed-row projection that differ between schedules. The
// building itself is shared, so [Trends] and the two ACFR history projections
// cannot disagree about grouping, ordering, or the fail-closed identity guard.
type seriesSpec struct {
	name    string
	scope   string
	labels  labels
	caveats func() []Caveat
}

// slices is ONE Options carrying EVERY column the schedule prints -- a trend
// of one printed row across its columns is a chart of exactly one thing, where
// [Sankey.Slices] returns one Options per column.
//
// IT SELECTS THE SCOPE EXHAUSTIVELY -- every column the store carries. A
// document covering half the scope publishes half its facts and says nothing
// about the half it left, which no check downstream can tell from a schedule
// the city printed at half the width.
//
// An empty result means the store carries no fact of this schedule, which is a
// statement about the corpus and not an error.
func (sp seriesSpec) slices(facts []fact.Fact, version string) []Options {
	seen := map[Column]bool{}
	for _, f := range facts {
		if f.Scope == sp.scope {
			seen[Column{FiscalYear: f.FiscalYear, Basis: f.Basis}] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	cols := make([]Column, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sort.Slice(cols, func(i, j int) bool {
		if cols[i].FiscalYear != cols[j].FiscalYear {
			return cols[i].FiscalYear < cols[j].FiscalYear
		}
		return cols[i].Basis < cols[j].Basis
	})
	return []Options{{Columns: cols, Scopes: []string{sp.scope}, Version: version}}
}

// document builds the series document without encoding it, so `fisc verify`
// can check the structure without parsing back the JSON it is validating.
func (sp seriesSpec) document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("%s options: %w", sp.name, err)
	}
	// Two refusals for the reason sankey.Document gives: how many schedules and
	// which schedule are different mistakes.
	scope, err := o.onlyScope()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sp.name, err)
	}
	if scope != sp.scope {
		return nil, cmdutil.WithHint(
			fmt.Errorf("%s: scope is %q, want %q", sp.name, scope, sp.scope),
			"this document is of one schedule; a projection built over another "+
				"schedule's facts would publish them under this one's contract")
	}

	selected := SelectFacts(facts, o)

	// Grouped by series, then each series' points ordered by the COLUMN ORDER
	// THE OPTIONS DECLARE rather than by anything read off the facts. That is
	// what makes a missing point detectable: a series short a column is short a
	// point in a known position, which trend-series-are-complete reports, rather
	// than a shorter list nothing can measure against.
	byID := map[string]*Series{}
	order := []string{}
	for i := range selected {
		f := selected[i]
		id := f.SeriesID()
		s := byID[id]
		if s == nil {
			s = &Series{
				SeriesID:      id,
				Label:         f.RowLabel,
				Fund:          f.Fund,
				FundName:      sp.fundName(f.Fund),
				FundGroup:     f.FundGroup,
				Kind:          string(f.Kind),
				Category:      f.Category,
				CategoryLabel: sp.categoryLabel(f.Category),
				Points:        []Point{},
			}
			byID[id] = s
			order = append(order, id)
		}
		// A series is one printed ROW, so every fact of it must agree about
		// which row that is. Disagreement means two different rows hashed to one
		// series id, which is the one thing an identity must not do -- fail
		// closed rather than publish a series whose label describes half of it.
		if s.FundGroup != f.FundGroup || !fact.SameFund(s.Fund, f.Fund) ||
			s.Kind != string(f.Kind) || s.Category != f.Category {
			return nil, fmt.Errorf(
				"%s: series %s is two different rows: %s p%d %q is %s/%s/fund %s/%s, "+
					"but an earlier fact of the same series is %s/%s/fund %s/%s",
				sp.name, id, f.DocID, f.Page, f.RowLabel, f.Kind, f.Category,
				fact.FundString(f.Fund), f.FundGroup,
				s.Kind, s.Category, fact.FundString(s.Fund), s.FundGroup)
		}
		s.Points = append(s.Points, Point{
			FiscalYear:  f.FiscalYear,
			Basis:       string(f.Basis),
			AmountCents: f.AmountCents,
			FactID:      f.ID,
			DocID:       f.DocID,
			Page:        f.Page,
			Offset:      f.Offset,
			Token:       f.Token,
			Derived:     f.Derived,
		})
	}

	series := make([]Series, 0, len(order))
	points := 0
	for _, id := range order {
		s := byID[id]
		sortPoints(s.Points, o.Columns)
		points += len(s.Points)
		series = append(series, *s)
	}
	sortSeries(series)

	env, err := envelope(o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sp.name, err)
	}

	// AN EMPTY NODE SET, NOT nil: this document publishes series rather than a
	// graph, so there is nothing for an AppliesTo to name, and nil would skip
	// the validation entirely. Measured before the fix: a bogus node id in
	// trendsCaveats left every Go test and `fisc verify` green, and published
	// "About <code>revenue/typo</code>" on caveats.html.
	cavs := sp.caveats()
	if err := validateCaveats(cavs, map[string]struct{}{}); err != nil {
		return nil, err
	}

	return &TrendsDocument{
		SchemaVersion: SchemaVersion,
		Projection:    sp.name,
		Metadata: trendsMetadata{
			Envelope: env,
			Columns:  trendColumns(o.Columns),
			Sources:  SourcesOf(selected),
			Counts: trendCounts{
				Facts:  len(selected),
				Series: len(series),
				Points: points,
			},
			Caveats: cavs,
		},
		Series: series,
	}, nil
}

// build renders the document as canonical, deterministic JSON.
func (sp seriesSpec) build(facts []fact.Fact, o Options) ([]byte, error) {
	d, err := sp.document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(d, sp.name, schema.Series)
}

// fundName is the city's name for a fund, falling back to "" so the client
// shows the number. A miss is not an error, as with a category label; a series
// under no numbered fund has no name to look up.
func (sp seriesSpec) fundName(fund *int) string {
	if sp.labels == nil || fund == nil {
		return ""
	}
	name, ok := sp.labels.FundName(*fund)
	if !ok {
		return ""
	}
	return name
}

// categoryLabel is the taxonomy's words for a slug, falling back to a
// slug-derived label so a newly mapped category renders as something readable.
func (sp seriesSpec) categoryLabel(slug string) string {
	if sp.labels != nil {
		if label, ok := sp.labels.Label(slug); ok {
			return label
		}
	}
	return slugLabel(slug)
}
