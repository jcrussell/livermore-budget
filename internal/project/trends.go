package project

import (
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TrendsScope is the schedule this projection draws: Budget Book pp.127-140,
// Revenue Sources by Fund.
//
// It is NOT the spine's scope, and that is the whole point of the projection
// existing separately. Those pages are the per-fund, line-item decomposition of
// pp.66-67's REVENUE rows -- the same money, printed twice -- so a document
// drawing both would double the city's revenue while balancing perfectly.
const TrendsScope = structure.ScopeRevenueByFund

// TrendsProjection is the file stem, and the name every check and the packager
// refer to this document by.
const TrendsProjection = "revenue-trends"

// TrendsColumns is the four columns pp.127-140 print, which is what the site
// publishes this document over. Stated, not derived: [Trends.Slices] reads its
// columns off the facts, so this is the floor a corpus that lost a column
// cannot pass. FY2024 is the Budget Book's actual, never [vocab.BasisAudited]
// (fisc-4ua.4). TestPublishedDocumentsAreWhatTheCorpusBuilds refuses a fifth
// column arriving silently.
func TrendsColumns() []Column {
	return []Column{
		{FiscalYear: 2024, Basis: vocab.BasisActual},
		{FiscalYear: 2025, Basis: vocab.BasisRevised},
		{FiscalYear: 2026, Basis: vocab.BasisAdopted},
		{FiscalYear: 2027, Basis: vocab.BasisAdopted},
	}
}

// Trends is the four printed columns of pp.127-140 as one series per printed
// row: not a graph. The contract is docs/outputs/revenue-trends.md.
type Trends struct {
	// Labels resolves the fund name and the category label the document
	// publishes. Nil is allowed and means both fall back, so a build never
	// fails for want of a registry -- see Sankey.Labels for the same rule.
	Labels labels
}

var _ Projection = (*Trends)(nil)

// Name is the file stem: revenue-trends becomes revenue-trends.json.
func (*Trends) Name() string { return TrendsProjection }

// spec is this document's identity, handed to the shared series builder.
func (t *Trends) spec() seriesSpec {
	return seriesSpec{
		name:    TrendsProjection,
		scope:   TrendsScope,
		labels:  t.Labels,
		caveats: trendsCaveats,
	}
}

// Slices selects the whole schedule as one document; see seriesSpec.slices.
func (t *Trends) Slices(facts []fact.Fact, version string) []Options {
	return t.spec().slices(facts, version)
}

// TrendsDocument is the whole published file. It is exported so a check can
// examine the structure without parsing back the JSON it is validating, exactly
// as sankey.Document is.
type TrendsDocument struct {
	SchemaVersion int            `json:"schema_version"`
	Projection    string         `json:"projection"`
	Metadata      trendsMetadata `json:"metadata"`
	Series        []Series       `json:"series"`
}

// trendsMetadata is this document's metadata block. It carries none of the
// spine's singular keys, which zeroed would be absent read as zero.
type trendsMetadata struct {
	Envelope
	// Columns are the printed columns, in the order the schedule prints them.
	Columns []trendColumn `json:"columns"`
	Sources []Source      `json:"sources"`
	Counts  trendCounts   `json:"counts"`
	Caveats []Caveat      `json:"caveats"`
}

// trendCounts is how much of the corpus this document accounts for. No
// facts_cited: every fact becomes a point. facts and points are computed
// independently, off the selection and off the built series.
type trendCounts struct {
	Facts  int `json:"facts"`
	Series int `json:"series"`
	Points int `json:"points"`
}

// trendColumn is one printed column, with the claim about what may be compared
// with what.
type trendColumn struct {
	FiscalYear      int    `json:"fiscal_year"`
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
	// ComparableGroup names the columns this one may be compared with
	// arithmetically. Computed from the basis, and a separate key because it is
	// a separate claim, the one a consumer enforces on.
	ComparableGroup string `json:"comparable_group"`
}

// Series is one printed row across every column it appears in.
type Series struct {
	SeriesID string `json:"series_id"`
	// Label is the row's printed label, the city's own words.
	Label string `json:"label"`
	// Fund and FundName identify which fund's section the row sits in, since
	// Label is not unique across funds. Fund is null where the row sits under
	// no numbered fund, as fact.Fact.Fund.
	Fund      *int   `json:"fund"`
	FundName  string `json:"fund_name"`
	FundGroup string `json:"fund_group"`
	Kind      string `json:"kind"`
	Category  string `json:"category"`
	// CategoryLabel is the taxonomy's words for Category.
	CategoryLabel string  `json:"category_label"`
	Points        []Point `json:"points"`
}

// Point is one printed cell: one fact, cited in full, its basis included.
type Point struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	// AmountCents is signed: a parenthesized contra row is a printed row and
	// is not netted.
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
	return t.spec().build(facts, o)
}

// Document builds the document without encoding it, so `fisc verify` can check
// it without parsing back the JSON it is trying to validate. It is Trends's
// sankey.Document.
func (t *Trends) Document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	return t.spec().document(facts, o)
}

// The caveats this document ships: where its fund-group sums do not tie to
// p63 Table 2. Their figures are read off pages, never derived as one total
// minus another. The reconciliation is in docs/outputs/revenue-trends.md.
var (
	caveatGeneralFundTransfersIn = Caveat{
		ID:      "no-general-fund-transfers-in-row",
		Theme:   ThemeLeftOut,
		Summary: "pp.127-130 print no General Fund Transfers In row, so that total is below the city's summary.",
		Text: "Budget Book pp.127-130 print no General Fund Transfers In row, so the General Fund " +
			"total here is below the city's summary: by $480,400 in FY2025-26 and $486,735 in " +
			"FY2026-27, the transfers pp.66-67 print and p76 itemises. Nothing here is short; " +
			"the row is simply not on these pages.",
		AppliesTo: []string{},
	}
	caveatCapitalReserves = Caveat{
		ID:      "capital-reserves-are-not-in-this-schedule",
		Theme:   ThemeLeftOut,
		Summary: "FY2024-25's Capital Funds column is $4,125,627 short, because one fund has no section.",
		Text: "The Capital Funds column for FY2024-25 adds up to $30,713,648 against the " +
			"$34,839,275 the city prints on p63, because General Fund CIP Reserves has no " +
			"section on pp.131-140. The $4,125,627 difference is a fund these pages don't " +
			"carry, not a wrong figure.",
		AppliesTo: []string{},
	}
	caveatComparability = Caveat{
		ID:      "four-columns-are-four-measurements",
		Theme:   ThemeKindsOfYear,
		Summary: "Actual, revised and adopted are different kinds of number, so no growth rate is given across them.",
		Text: "FY2023-24 is money that actually moved, FY2024-25 is a mid-year re-forecast, and " +
			"the two later years are plans adopted together. Each column says which it is; " +
			"growth from one kind to another is not published, because it would not mean " +
			"anything.",
		AppliesTo: []string{},
	}
)

// trendsCaveats is every caveat, unconditionally.
//
// UNCONDITIONAL IS THE POINT, unlike the Sankey's, whose caveats are predicated
// on what the graph turned out to contain. The first three are statements about
// what pp.127-140 DO NOT PRINT, and a document cannot detect the absence of a
// schedule it was never given. Dropping one when the corpus changes would need
// a check that the absence had ended, which is a thing to build when a page
// makes it possible and not a condition to guess at here.
//
// The fourth is [revenueSchedulePublishedTwiceCaveat], shared with the
// document that draws these rows a second time.
func trendsCaveats() []Caveat {
	return []Caveat{caveatGeneralFundTransfersIn, caveatCapitalReserves, caveatComparability,
		revenueSchedulePublishedTwiceCaveat()}
}

// trendColumns publishes the columns with their labels and comparable groups.
func trendColumns(cols []Column) []trendColumn {
	out := make([]trendColumn, 0, len(cols))
	for _, c := range cols {
		out = append(out, trendColumn{
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
// A point whose column the options do not declare cannot occur -- SelectFacts
// filtered on exactly that set -- so an unknown column sorts last rather than
// panicking, which keeps the ordering total under any future caller.
func sortPoints(points []Point, cols []Column) {
	rank := make(map[Column]int, len(cols))
	for i, c := range cols {
		rank[c] = i
	}
	at := func(p Point) int {
		if r, ok := rank[Column{FiscalYear: p.FiscalYear, Basis: vocab.Basis(p.Basis)}]; ok {
			return r
		}
		return len(cols)
	}
	sort.SliceStable(points, func(i, j int) bool { return at(points[i]) < at(points[j]) })
}

// fundBefore orders two fund coordinates that differ: no fund sorts ahead of
// every numbered one, and numbers sort as numbers.
func fundBefore(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil
	}
	return *a < *b
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
		if !fact.SameFund(series[i].Fund, series[j].Fund) {
			return fundBefore(series[i].Fund, series[j].Fund)
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
