package project

import (
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// TrendsScope is the schedule this projection draws: Budget Book pp.127-140,
// Revenue Sources by Fund.
//
// It is NOT the spine's scope, and that is the whole point of the projection
// existing separately. Those pages are the per-fund, line-item decomposition of
// pp.66-67's REVENUE rows -- the same money, printed twice -- so a document
// drawing both would double the city's revenue while balancing perfectly.
const TrendsScope = "revenue-by-fund"

// TrendsProjection is the file stem, and the name every check and the packager
// refer to this document by.
const TrendsProjection = "revenue-trends"

// Trends is the four printed columns of pp.127-140 as one series per printed
// row.
//
// It is the first document this project publishes that is NOT a graph: it has
// no nodes and no links, so nothing in internal/check's graph checks reads it,
// and it is not defective for that. The contract is
// docs/revenue-trends-contract.md.
type Trends struct {
	// Labels resolves the fund name and the category label the document
	// publishes. Nil is allowed and means both fall back, so a build never
	// fails for want of a registry -- see Sankey.Labels for the same rule.
	Labels Labels
}

var (
	_ Projection = (*Trends)(nil)
	_ Sliced     = (*Trends)(nil)
)

// Name is the file stem: revenue-trends becomes revenue-trends.json.
func (*Trends) Name() string { return TrendsProjection }

// Slices is ONE Options carrying EVERY column the schedule prints.
//
// That is the opposite of Sankey.Slices, which returns one Options per column,
// and the difference is the point of project.Sliced existing: a Sankey of two
// budgets is not a chart of anything, while a trend of one printed row across
// four columns is a chart of exactly one thing. Neither answer generalises to
// the other, which is why the projection is asked rather than told.
//
// IT SELECTS THE SCOPE EXHAUSTIVELY -- every column the store carries, not just
// the two adopted years the spine prints a column for. Not for completeness:
// internal/check's staleDeclarations retires an unprojectedScopes entry only
// when a scope is drawn in FULL, so a document covering half the scope would
// leave that entry green and false while half its facts were already in a chart.
//
// An empty result means the store carries no fact of this schedule, which is a
// statement about the corpus and not an error.
func (*Trends) Slices(facts []fact.Fact, version string) []Options {
	seen := map[Column]bool{}
	for _, f := range facts {
		if f.Scope == TrendsScope {
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
	return []Options{{Columns: cols, Scope: TrendsScope, Version: version}}
}

// TrendsDocument is the whole published file. It is exported so a check can
// examine the structure without parsing back the JSON it is validating, exactly
// as Sankey.Graph is.
type TrendsDocument struct {
	SchemaVersion int            `json:"schema_version"`
	Projection    string         `json:"projection"`
	Metadata      TrendsMetadata `json:"metadata"`
	Series        []Series       `json:"series"`
}

// TrendsMetadata is this document's metadata block.
//
// It embeds [Envelope] and carries NONE of the spine's fiscal_year,
// fiscal_year_label, basis or headline. Those are singular or spine-specific
// and this document is of four columns and three bases; publishing them zeroed
// to keep a familiar shape would be exactly the absent-is-not-zero error this
// project refuses everywhere else.
type TrendsMetadata struct {
	Envelope
	// Columns are the printed columns, in the order the schedule prints them.
	Columns []TrendColumn `json:"columns"`
	Sources []Source      `json:"sources"`
	Counts  TrendCounts   `json:"counts"`
	Caveats []string      `json:"caveats"`
}

// TrendCounts is how much of the corpus this document accounts for.
//
// THERE IS NO facts_cited HERE, and its absence is the considered answer rather
// than an omission. [Counts] publishes one because on the spine it is genuinely
// smaller than facts -- a zero-valued cell earns no link and a stock row earns
// none either -- so the gap is a quantity a check can assert. In this document
// every fact in the slice becomes a point: no netting, nothing dropped for being
// zero, no stocks. A facts_cited here would be a third name for a number already
// published twice, which is the "never name a key for a concept the code does
// not compute" rule failing from the other direction.
//
// WHAT facts AND points BEING EQUAL IS WORTH is that they are computed
// independently -- facts off the selection, points off the series actually
// built -- so a document that dropped a series publishes points below facts. The
// same divergence is caught directly, and with the row named, by
// trend-points-tie-to-facts; this is the form of it a reader can see in the file
// without running anything.
type TrendCounts struct {
	Facts  int `json:"facts"`
	Series int `json:"series"`
	Points int `json:"points"`
}

// TrendColumn is one printed column, with the claim about what may be compared
// with what.
type TrendColumn struct {
	FiscalYear      int    `json:"fiscal_year"`
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
	// ComparableGroup names the columns this one may be compared with
	// arithmetically. Two columns are comparable when they were produced by the
	// same process, so it is COMPUTED FROM THE BASIS and today equals it: the
	// two adopted years share a group and the actual and revised columns each
	// stand alone.
	//
	// It is a separate key from Basis because it is a separate claim -- basis
	// says how a figure was produced, this says what it may be set beside -- and
	// because it is what a consumer enforces on. The document publishes no
	// growth or CAGR of its own (see the contract), so this is the only thing
	// standing between a reader and a percentage across an actual and an
	// intention.
	ComparableGroup string `json:"comparable_group"`
}

// Series is one printed row across every column it appears in.
type Series struct {
	SeriesID string `json:"series_id"`
	// Label is the row's printed label, the city's own words.
	Label string `json:"label"`
	// Fund and FundName identify which fund's section the row sits in. They are
	// published because Label is NOT unique across funds: "Property Taxes" is
	// printed by four of them and "Use of Money & Prop" by thirty-eight, so a
	// series named by its row alone is ambiguous on sight.
	Fund      int    `json:"fund"`
	FundName  string `json:"fund_name"`
	FundGroup string `json:"fund_group"`
	Kind      string `json:"kind"`
	Category  string `json:"category"`
	// CategoryLabel is the taxonomy's words for Category, published rather than
	// looked up client-side so the site does not have to ship data/taxonomy.yaml
	// beside the document to render a legend.
	CategoryLabel string  `json:"category_label"`
	Points        []Point `json:"points"`
}

// Point is one printed cell: one fact, cited in full.
//
// EVERY POINT CARRIES ITS OWN PROVENANCE rather than deferring to the series.
// It is what lets a single figure be cited on its own, and it is what makes the
// comparability problem visible in the data instead of only in prose -- a point
// knows which basis it was produced on without its reader consulting a column
// header.
type Point struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	// AmountCents is SIGNED. The eight contra rows this schedule prints -- the
	// General Fund's ERAF and RPTTF Reduction, printed in parentheses -- carry
	// negative amounts, and unlike the Sankey this document does not net them
	// into a parent category, because a series is a printed ROW and those two
	// rows are printed.
	AmountCents int64  `json:"amount_cents"`
	FactID      string `json:"fact_id"`
	DocID       string `json:"doc_id"`
	Page        int    `json:"page"`
	Offset      int    `json:"offset"`
	Token       string `json:"token"`
	Derived     bool   `json:"derived"`
}

// Build renders the document as canonical, deterministic JSON.
func (t *Trends) Build(facts []fact.Fact, o Options) ([]byte, error) {
	d, err := t.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(d, t.Name())
}

// Document builds the document without encoding it, so `fisc verify` can check
// it without parsing back the JSON it is trying to validate. It is Trends's
// Sankey.Graph.
func (t *Trends) Document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("revenue-trends options: %w", err)
	}
	if o.Scope != TrendsScope {
		return nil, cmdutil.WithHint(
			fmt.Errorf("revenue-trends: scope is %q, want %q", o.Scope, TrendsScope),
			"this document is of one schedule; a projection built over another "+
				"schedule's facts would publish them under this one's contract")
	}

	selected := selectFacts(facts, o)

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
				FundName:      t.fundName(f.Fund),
				FundGroup:     f.FundGroup,
				Kind:          string(f.Kind),
				Category:      f.Category,
				CategoryLabel: t.categoryLabel(f.Category),
				Points:        []Point{},
			}
			byID[id] = s
			order = append(order, id)
		}
		// A series is one printed ROW, so every fact of it must agree about
		// which row that is. Disagreement means two different rows hashed to one
		// series id, which is the one thing an identity must not do -- fail
		// closed rather than publish a series whose label describes half of it.
		if s.FundGroup != f.FundGroup || s.Fund != f.Fund ||
			s.Kind != string(f.Kind) || s.Category != f.Category {
			return nil, fmt.Errorf(
				"revenue-trends: series %s is two different rows: %s p%d %q is %s/%s/fund %d/%s, "+
					"but an earlier fact of the same series is %s/%s/fund %d/%s",
				id, f.DocID, f.Page, f.RowLabel, f.Kind, f.Category, f.Fund, f.FundGroup,
				s.Kind, s.Category, s.Fund, s.FundGroup)
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

	return &TrendsDocument{
		SchemaVersion: SchemaVersion,
		Projection:    t.Name(),
		Metadata: TrendsMetadata{
			Envelope: envelope(o),
			Columns:  trendColumns(o.Columns),
			Sources:  sourcesOf(selected),
			Counts: TrendCounts{
				Facts:  len(selected),
				Series: len(series),
				Points: points,
			},
			Caveats: trendsCaveats(),
		},
		Series: series,
	}, nil
}

// The caveats this document ships, and the reason each is owed.
//
// NONE OF THEM IS ABOUT A SERIES BEING WRONG. Every one of the 924 figures is
// printed and every series has a point in every column. They are owed because a
// reader who SUMS this document into fund groups and sets it beside the city's
// own summary table -- p63 Table 2, Total Sources -- finds four of the seven
// groups tie exactly and three do not, and a document that leaves those
// differences to be rediscovered reads as wrong.
//
// The figures are read off pages, never derived as one total minus another;
// that distinction is what keeps a caveat a statement about the document rather
// than a restatement of its own arithmetic. Full reconciliation, all 28 cells,
// is in docs/revenue-trends-contract.md.
const (
	caveatGeneralFundTransfersIn = "Budget Book pp.127-130 print no General Fund " +
		"Transfers In row, so this schedule's General Fund total is below the city's own " +
		"summary in every column: by $480,400 in FY2025-26 and $486,735 in FY2026-27, which " +
		"are the transfers pp.66-67 do print, itemised by payer on p76. No series here is " +
		"short; the row is not in this schedule."
	caveatCapitalReserves = "The Capital Funds column for FY2024-25 sums to $30,713,648 " +
		"against the $34,839,275 the city prints on p63, because General Fund CIP Reserves " +
		"has no section on pp.131-140. The $4,125,627 difference is a fund the schedule " +
		"does not carry, not a figure it gets wrong."
	caveatComparability = "The four columns are four different measurements: FY2023-24 is " +
		"money that moved, FY2024-25 is a mid-year re-forecast, and the two later years are " +
		"intentions adopted together. Each column carries the basis it was produced on and " +
		"the group it may be compared within; growth across groups is not published, because " +
		"it is not a quantity this document can compute."
)

// trendsCaveats is every caveat, unconditionally.
//
// UNCONDITIONAL IS THE POINT, unlike the Sankey's, whose caveats are predicated
// on what the graph turned out to contain. Each of these is a statement about
// what pp.127-140 DO NOT PRINT, and a document cannot detect the absence of a
// schedule it was never given. Dropping one when the corpus changes would need
// a check that the absence had ended, which is a thing to build when a page
// makes it possible and not a condition to guess at here.
func trendsCaveats() []string {
	return []string{caveatGeneralFundTransfersIn, caveatCapitalReserves, caveatComparability}
}

// trendColumns publishes the columns with their labels and comparable groups.
func trendColumns(cols []Column) []TrendColumn {
	out := make([]TrendColumn, 0, len(cols))
	for _, c := range cols {
		out = append(out, TrendColumn{
			FiscalYear:      c.FiscalYear,
			FiscalYearLabel: fiscalYearLabel(c.FiscalYear),
			Basis:           string(c.Basis),
			ComparableGroup: string(c.Basis),
		})
	}
	return out
}

// sortPoints puts a series' points in the document's column order.
//
// A point whose column the options do not declare cannot occur -- selectFacts
// filtered on exactly that set -- so an unknown column sorts last rather than
// panicking, which keeps the ordering total under any future caller.
func sortPoints(points []Point, cols []Column) {
	rank := make(map[Column]int, len(cols))
	for i, c := range cols {
		rank[c] = i
	}
	at := func(p Point) int {
		if r, ok := rank[Column{FiscalYear: p.FiscalYear, Basis: mapping.Basis(p.Basis)}]; ok {
			return r
		}
		return len(cols)
	}
	sort.SliceStable(points, func(i, j int) bool { return at(points[i]) < at(points[j]) })
}

// sortSeries puts the document in reading order: fund by number, and within a
// fund the order the schedule prints the rows.
//
// PRINTED ORDER IS READ OFF THE PAGE, as the page and the offset of the series'
// first point, not invented here. Two series cannot share one (page, offset) --
// a fact's offset is where its own token starts -- so the ordering is total and
// the series_id tiebreak below is a backstop rather than a load-bearing rule.
func sortSeries(series []Series) {
	at := func(s Series) (int, int) {
		page, offset := 0, 0
		for i, p := range s.Points {
			if i == 0 || p.Page < page || (p.Page == page && p.Offset < offset) {
				page, offset = p.Page, p.Offset
			}
		}
		return page, offset
	}
	sort.Slice(series, func(i, j int) bool {
		if series[i].Fund != series[j].Fund {
			return series[i].Fund < series[j].Fund
		}
		pi, oi := at(series[i])
		pj, oj := at(series[j])
		if pi != pj {
			return pi < pj
		}
		if oi != oj {
			return oi < oj
		}
		return series[i].SeriesID < series[j].SeriesID
	})
}

// fundName is the city's name for a fund, falling back to "" so the client
// shows the number. A miss is not an error, as with a category label.
func (t *Trends) fundName(number int) string {
	if t.Labels == nil {
		return ""
	}
	name, ok := t.Labels.FundName(number)
	if !ok {
		return ""
	}
	return name
}

// categoryLabel is the taxonomy's words for a slug, falling back to a
// slug-derived label so a newly mapped category renders as something readable.
func (t *Trends) categoryLabel(slug string) string {
	if t.Labels != nil {
		if label, ok := t.Labels.Label(slug); ok {
			return label
		}
	}
	return slugLabel(slug)
}
