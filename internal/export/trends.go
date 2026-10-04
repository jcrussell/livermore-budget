package export

import (
	"encoding/json"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

type trendsPageData struct {
	chrome
	Columns []columnRef
	Series  []seriesRef
	Facts   int
	Count   int
}

// columnRef is one printed column as the table heads it.
type columnRef struct {
	Label string
	// Basis is the word the chip and every cell tooltip print, which is
	// [basisLabelFor]'s answer and not the document's basis enum.
	Basis string
	// Group is the column's comparable_group, rendered as a data attribute so
	// the boundary between measurements is in the markup rather than only in a
	// caveat. New says this column starts a new group, which is where a reader
	// should not carry a comparison across.
	Group string
	New   bool
}

// seriesRef is one printed row as the table renders it.
type seriesRef struct {
	Label    string
	Fund     string
	Group    string
	Category string
	Cells    []cellRef
	// Mark is the row's four columns drawn beside the figures they are.
	Mark markRef
}

// cellRef is one column of one row: the point published there, or the absence
// of one.
type cellRef struct {
	Value string
	// Missing says the series publishes no point in this column. The cell is
	// still rendered, because the alternative is what this type exists to
	// prevent — see buildCells.
	Missing bool
	// Negative marks a contra row — the General Fund's ERAF and RPTTF Reduction
	// are printed in parentheses and published signed — so the stylesheet can
	// colour the cell and fill the mark's bar downward.
	//
	// THE CELL STILL RENDERS A MINUS SIGN, DELIBERATELY. Rendering
	// (14,086,438) as the city prints it was considered and refused, because
	// screen readers do not announce parentheses at default punctuation
	// settings — the cell would be read aloud as a POSITIVE figure, and the
	// minus sign is the only part of it that carries direction to a reader who
	// is not looking at the colour. Nine cells ship. Accounting convention is a
	// visual convention, and this page has one reader it cannot see.
	//
	// So what the flag drives is .contra: the cell's colour, and .mark-bar.contra
	// in the mark beside it, where direction really is geometric and the bar
	// hanging below the baseline says it without punctuation.
	Negative bool
	// New repeats the column's group boundary onto the body cell, so the rule
	// between two measurements runs down the table rather than stopping at the
	// header.
	New bool
	// Cents is the figure the cell shows, carried alongside its rendered Value
	// so the mark beside it can be scaled without re-parsing the string the
	// packager just formatted. It is the DOCUMENT's number, read and not
	// recomputed, and it is integer cents like every other amount here.
	Cents int64
	// Page and Href cite the figure itself. Every point carries its own page in
	// the document, which is what makes a single cell citable.
	Page int
	Href string
}

// basisLabelFor is the word a column chip and a cell tooltip print for a basis.
//
// It is deliberately NOT the basis itself. Basis is component 7 of fact.MakeID,
// so every figure in the ACFR's ten-year schedules carries vocab.BasisAudited
// in its identity and cannot be re-based without rewriting every one of those
// fact ids and moving facts/facts.jsonl. The page is a different question from
// the identity: those schedules sit in the section the document's own caveat
// quotes the ACFR calling "(Unaudited)", and a page may not print a word one of
// its own caveats withdraws two paragraphs below.
//
// The document's caveats decide, so the label and the refusal read one source
// rather than a list of page names that would have to be kept in step by hand.
// Only the audited basis is rewritten: a document may ship this caveat over a
// column on some other basis, and relabelling that one would be the same defect
// pointing the other way.
func basisLabelFor(caveats []caveatMeta, basis string) string {
	if basis != string(vocab.BasisAudited) {
		return basis
	}
	for _, c := range caveats {
		if c.ID == project.UnauditedCaveatID {
			return "unaudited"
		}
	}
	return basis
}

// trendsMetadata is as much of a revenue-trends document as the page renders.
//
// It is a separate struct from projectionMetadata rather than a superset of it,
// because the two documents genuinely differ: this one has columns and no
// fiscal year, and no headline at all. Sharing one struct would mean a page
// reading a field its document never publishes and getting a zero.
type trendsMetadata struct {
	GeneratedBy string            `json:"generated_by"`
	Scope       string            `json:"scope"`
	Sources     []sourceMeta      `json:"sources"`
	Columns     []trendColumnMeta `json:"columns"`
	Counts      trendCountsMeta   `json:"counts"`
	Caveats     []caveatMeta      `json:"caveats"`
}

// trendCountsMeta is the document's own accounting of itself, which
// reconcileSeriesCounts holds the rendered page to.
type trendCountsMeta struct {
	Facts  int `json:"facts"`
	Series int `json:"series"`
	Points int `json:"points"`
}

// trendColumnMeta is one column as the document declares it.
type trendColumnMeta struct {
	FiscalYear      int    `json:"fiscal_year"`
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
	ComparableGroup string `json:"comparable_group"`
}

// trendPoint is one published point as this page reads it.
type trendPoint struct {
	FiscalYear  int    `json:"fiscal_year"`
	Basis       string `json:"basis"`
	AmountCents int64  `json:"amount_cents"`
	DocID       string `json:"doc_id"`
	Page        int    `json:"page"`
}

// trendsBody is the series this page tabulates.
type trendsBody struct {
	Series []struct {
		Label         string       `json:"label"`
		Fund          *int         `json:"fund"`
		FundName      string       `json:"fund_name"`
		FundGroup     string       `json:"fund_group"`
		Kind          string       `json:"kind"`
		CategoryLabel string       `json:"category_label"`
		Points        []trendPoint `json:"points"`
	} `json:"series"`
}

// buildTrendsPage renders the revenue-trends view.
//
// EVERY FIGURE IS READ OUT OF THE DOCUMENT AND NONE IS COMPUTED HERE. The
// packager's standing rule (see the package doc) is that it does not recompute
// what a projection published, and a table is the case where that is most
// tempting to break: a total column, a growth percentage, a per-fund subtotal
// would each be one line. None of them is published, and the contract says why —
// growth from an actual to an adopted figure is not a quantity this project can
// compute, and a total this document does not carry is a total the city did not
// print on the page these series came from.
func buildTrendsPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (trendsPageData, error) {
	caveatsPath := caveatsPathOf(o)
	raw := o.Projections[v.Projection]
	doc, err := decodeDocument(v.Projection, raw)
	if err != nil {
		return trendsPageData{}, err
	}
	var meta trendsMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return trendsPageData{}, fmt.Errorf("decode %s metadata: %w", v.Projection, err)
	}
	if len(meta.Columns) == 0 {
		return trendsPageData{}, fmt.Errorf("%s metadata publishes no columns", v.Projection)
	}
	var body trendsBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return trendsPageData{}, fmt.Errorf("decode %s series: %w", v.Projection, err)
	}

	columns := make([]columnRef, 0, len(meta.Columns))
	for i, c := range meta.Columns {
		columns = append(columns, columnRef{
			Label: c.FiscalYearLabel,
			Basis: basisLabelFor(meta.Caveats, c.Basis),
			Group: c.ComparableGroup,
			// The FIRST column of a group does not start a boundary a reader
			// could carry a comparison across; every later one does.
			New: i > 0 && c.ComparableGroup != meta.Columns[i-1].ComparableGroup,
		})
	}

	series := make([]seriesRef, 0, len(body.Series))
	rendered := 0
	for _, s := range body.Series {
		fund := s.FundName
		if fund == "" {
			// A fund the registry does not name renders as its number, which is
			// what the document's own fallback intends: the number is on the
			// page and is never nothing. A series under no fund at all has no
			// row header in this view, and is refused rather than headed "".
			if s.Fund == nil {
				return trendsPageData{}, fmt.Errorf("%s series %q names no fund and no "+
					"fund name; this view's row header is the fund", v.Projection, s.Label)
			}
			fund = fmt.Sprintf("Fund %d", *s.Fund)
		}
		cells, placed, err := buildCells(s.Points, columns, meta.Columns, pageTextBase)
		if err != nil {
			return trendsPageData{}, fmt.Errorf("%s series %q: %w", v.Projection, s.Label, err)
		}
		rendered += placed
		series = append(series, seriesRef{
			Label:    s.Label,
			Fund:     fund,
			Group:    s.FundGroup,
			Category: s.CategoryLabel,
			Cells:    cells,
			Mark:     buildMark(cells, columns),
		})
	}

	if err := reconcileSeriesCounts(v.Projection, rendered, len(body.Series), meta.Counts); err != nil {
		return trendsPageData{}, err
	}

	sources, _ := sourcesFor(meta.Sources, byID, pageTextBase, o.RecordsBase)
	refs := projectionRefs(o, ix)
	title := v.Title
	if title == "" {
		title = "City of Livermore revenue by fund"
	}
	return trendsPageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  refs,
			DataPath:     ix.PublishedPath(v.Projection),
			Scope:        meta.Scope,
			Caveats:      caveatRefs(meta.Caveats, v.Projection, caveatsPath),
			CaveatsPath:  caveatsPath,
		},
		Columns: columns,
		Series:  series,
		Facts:   meta.Counts.Facts,
		Count:   meta.Counts.Series,
	}, nil
}

// reconcileSeriesCounts refuses a series document whose own counts disagree
// with the cells the page just laid out (fisc-4j5).
//
// `fisc export` runs no checks by design and this is the one thing it insists
// on. A series legitimately SHORT A COLUMN still reconciles: internal/check
// declares that state through incompleteSeries, and a short series carries
// fewer points and says so in its own counts.
//
// rendered IS HELD TO counts.facts AS WELL AS counts.points, because facts is
// the number the lede prints and the two are computed independently -- facts
// off the projection's selection, points off the series actually built. A
// document that dropped a series publishes points below facts, and reconciling
// only points once shipped a lede claiming 924 figures over a table carrying
// 920 (fisc-5tu).
func reconcileSeriesCounts(projection string, rendered, series int, counts trendCountsMeta) error {
	if rendered != counts.Points {
		return fmt.Errorf(
			"%s declares counts.points %d and its series carry %d; the document has "+
				"lost a figure between the projection that built it and this page",
			projection, counts.Points, rendered)
	}
	if rendered != counts.Facts {
		return fmt.Errorf(
			"%s prints counts.facts %d in its lede and its series carry %d cells; "+
				"the page would claim more figures than it shows",
			projection, counts.Facts, rendered)
	}
	if series != counts.Series {
		return fmt.Errorf(
			"%s declares counts.series %d and carries %d",
			projection, counts.Series, series)
	}
	return nil
}

// buildCells lays one series' points out against the document's COLUMNS.
//
// POSITIONAL WAS WRONG AND THE FAILURE IS THE ONE THIS PROJECT EXISTS TO
// PREVENT. An earlier version emitted one cell per point, in the order the
// document listed them, against headers built from metadata.columns. Those two
// lists agree only while every series is complete -- and a series short a column
// is a state the project explicitly supports: internal/check declares it through
// incompleteSeries, and `fisc export` runs no checks, so a document with a gap
// can be packaged. Under the positional version every figure after the gap slid
// one column left, printing FY2026's money under FY2025 with a citation link to
// FY2026's page. A plausible wrong value, published, with provenance that
// disagrees with it.
//
// So the cells are keyed on (fiscal_year, basis) -- the column identity the
// document itself publishes -- and a column with no point gets a rendered gap
// rather than a shifted neighbour. A gap a reader can see is the honest form of
// something the document does not say.
//
// A point in NO published column cannot occur through internal/project, which
// filters facts on the same column set. If one ever arrives it is dropped here,
// so this reports how many points it actually PLACED and the caller reconciles
// that against the document's own counts.points. That reconciliation used to be
// promised by this comment and performed by nobody: counts.points was decoded
// and read nowhere, so a 1-column document carrying a 2-point series exported
// successfully, rendered one cell, lost the other figure silently, and printed
// a lede saying "2 figures in all" (fisc-4j5).
func buildCells(points []trendPoint, columns []columnRef, meta []trendColumnMeta,
	pageTextBase func(string) string,
) ([]cellRef, int, error) {
	type key struct {
		year  int
		basis string
	}
	byColumn := make(map[key]trendPoint, len(points))
	for _, p := range points {
		k := key{p.FiscalYear, p.Basis}
		// TWO POINTS IN ONE COLUMN IS THE SAME DEFECT FROM THE OTHER SIDE. A
		// plain assignment keeps the last and loses the first as quietly as a
		// dropped column does, and the two need not even agree -- so the count
		// below would still reconcile while a figure had vanished.
		if prev, dup := byColumn[k]; dup {
			return nil, 0, fmt.Errorf(
				"two points publish %s: %s and %s; one of them would be dropped",
				fact.ColumnLabel(p.FiscalYear, p.Basis), amount.Cents(prev.AmountCents).Dollars(), amount.Cents(p.AmountCents).Dollars())
		}
		byColumn[k] = p
	}

	out := make([]cellRef, 0, len(columns))
	placed := 0
	for i, c := range meta {
		cell := cellRef{New: columns[i].New}
		p, ok := byColumn[key{c.FiscalYear, c.Basis}]
		if !ok {
			// The em dash is the city's own mark for a cell it did not print,
			// and this is not that: this is a row the schedule does not carry in
			// this column at all. The title says which, because the two are
			// indistinguishable on the page otherwise and a published zero is a
			// fact while an absence is not (AGENTS.md, Provenance invariants).
			cell.Missing = true
			cell.Value = "—"
			out = append(out, cell)
			continue
		}
		cell.Value = amount.Cents(p.AmountCents).Dollars()
		cell.Cents = p.AmountCents
		cell.Negative = p.AmountCents < 0
		cell.Page = p.Page
		cell.Href = pageTextBase(p.DocID) + pageTextFile(p.Page)
		out = append(out, cell)
		placed++
	}
	return out, placed, nil
}
