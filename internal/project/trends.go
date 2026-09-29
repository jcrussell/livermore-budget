package project

import (
	"sort"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
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
// publishes this document over.
//
// IT IS STATED AND NOT DERIVED, and the difference is the whole reason it
// exists. [Trends.Slices] reads its columns off the fact store, exhaustively --
// so the document and the corpus agree by construction, and a corpus that lost
// FY2023-24 entirely would build a three-column document that every check passes
// over. trend-series-are-complete compares each series against the columns its
// document was BUILT over, so all 231 series would be complete over three and
// the check goes green while counts.facts falls from 924 to 693 (fisc-7dt).
//
// This list is the declared floor that closes that hole from the publishing
// side: it says what the SITE PROMISES, which a corpus cannot contradict without
// something going red. It is not a claim about what the corpus used to hold --
// the class of constant this project refuses, because no page states it -- and
// it is the same species of declaration as [PublishedFiscalYears], which is also
// stated, also author-chosen, and also load-bearing for exactly this reason.
//
// The bases are the ones the schedule prints and are NOT one measurement: an
// actual, a mid-year re-forecast, and two years of one adopted two-year budget.
// docs/revenue-trends-contract.md carries the table. FY2024 here is the Budget
// Book's own restatement, never [mapping.BasisAudited], which is a different
// basis and is fisc-4ua.4's business.
//
// A fifth mapped column is a change to what the site publishes, so it belongs
// here, in a commit that says so. TestPublishedDocumentsAreWhatTheCorpusBuilds
// is what refuses to let one arrive silently.
func TrendsColumns() []Column {
	return []Column{
		{FiscalYear: 2024, Basis: mapping.BasisActual},
		{FiscalYear: 2025, Basis: mapping.BasisRevised},
		{FiscalYear: 2026, Basis: mapping.BasisAdopted},
		{FiscalYear: 2027, Basis: mapping.BasisAdopted},
	}
}

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
	Labels labels
}

var (
	_ Projection = (*Trends)(nil)
	_ Sliced     = (*Trends)(nil)
)

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

// trendsMetadata is this document's metadata block.
//
// It embeds [Envelope] and carries NONE of the spine's fiscal_year,
// fiscal_year_label, basis or headline. Those are singular or spine-specific
// and this document is of four columns and three bases; publishing them zeroed
// to keep a familiar shape would be exactly the absent-is-not-zero error this
// project refuses everywhere else.
type trendsMetadata struct {
	Envelope
	// Columns are the printed columns, in the order the schedule prints them.
	Columns []trendColumn `json:"columns"`
	Sources []Source      `json:"sources"`
	Counts  trendCounts   `json:"counts"`
	Caveats []Caveat      `json:"caveats"`
}

// trendCounts is how much of the corpus this document accounts for.
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
	//
	// Fund is null where the row sits under no numbered fund: the ACFR's
	// fund-balance schedules print a fund's components and an aggregate across
	// funds, never a fund. It is fact.Fact.Fund carried through, for that
	// field's reason -- 0 would be a fund, and no fund is numbered 0.
	Fund      *int   `json:"fund"`
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
	return t.spec().build(facts, o)
}

// Document builds the document without encoding it, so `fisc verify` can check
// it without parsing back the JSON it is trying to validate. It is Trends's
// sankey.Document.
func (t *Trends) Document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	return t.spec().document(facts, o)
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
var (
	caveatGeneralFundTransfersIn = Caveat{
		ID:      "no-general-fund-transfers-in-row",
		Summary: "pp.127-130 print no General Fund Transfers In row, so that total sits below the city's own summary.",
		Text: "Budget Book pp.127-130 print no General Fund " +
			"Transfers In row, so this schedule's General Fund total is below the city's own " +
			"summary in every column: by $480,400 in FY2025-26 and $486,735 in FY2026-27, which " +
			"are the transfers pp.66-67 do print, itemised by payer on p76. No series here is " +
			"short; the row is not in this schedule.",
		AppliesTo: []string{},
	}
	caveatCapitalReserves = Caveat{
		ID:      "capital-reserves-are-not-in-this-schedule",
		Summary: "General Fund CIP Reserves has no section on pp.131-140, so FY2024-25's Capital column is short by $4,125,627.",
		Text: "The Capital Funds column for FY2024-25 sums to $30,713,648 " +
			"against the $34,839,275 the city prints on p63, because General Fund CIP Reserves " +
			"has no section on pp.131-140. The $4,125,627 difference is a fund the schedule " +
			"does not carry, not a figure it gets wrong.",
		AppliesTo: []string{},
	}
	caveatComparability = Caveat{
		ID:      "four-columns-are-four-measurements",
		Summary: "The four columns are four different measurements, so growth across them is not published.",
		Text: "The four columns are four different measurements: FY2023-24 is " +
			"money that moved, FY2024-25 is a mid-year re-forecast, and the two later years are " +
			"intentions adopted together. Each column carries the basis it was produced on and " +
			"the group it may be compared within; growth across groups is not published, because " +
			"it is not a quantity this document can compute.",
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
