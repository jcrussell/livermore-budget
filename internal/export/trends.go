package export

import (
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// columnRef is one printed column as the table heads it.
type columnRef struct {
	Label string
	// Basis is [basisLabelFor]'s word, not the document's basis enum.
	Basis string
	// Group is the column's comparable_group; New says this column starts a
	// new group, across which a reader should not compare.
	Group string
	New   bool
	// Key is the column's [YearKey], or "" where no page selects by it.
	Key string
}

// seriesRef is one printed row as the table renders it.
type seriesRef struct {
	Label    string
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
	// Missing says the series publishes no point in this column; the cell
	// still renders (buildCells).
	Missing bool
	// Negative marks a contra row, printed in parentheses and published
	// signed; it drives .contra. The cell still renders a minus sign, because
	// screen readers do not announce parentheses.
	Negative bool
	// New repeats the column's group boundary onto the body cell.
	New bool
	// Cents is the document's figure, read and not recomputed, so the mark
	// scales without re-parsing Value.
	Cents int64
	// Page and Href cite the figure itself; PDF is "" where the registry
	// names no PDF.
	Page int
	Href string
	PDF  string
}

// basisLabelFor is the word a column chip and a cell tooltip print for a
// basis. The ACFR's ten-year schedules carry vocab.BasisAudited in their fact
// ids but sit in a section the ACFR calls "(Unaudited)", so an audited column
// under project.UnauditedCaveatID prints "unaudited". Only the audited basis
// is rewritten.
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

// trendsMetadata is as much of a revenue-trends document as the page renders:
// columns, and no fiscal year or headline.
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

// reconcileSeriesCounts refuses a series document whose own counts disagree
// with the cells the page just laid out. A series short a column still
// reconciles (internal/check's incompleteSeries). rendered is held to
// counts.facts as well as counts.points: facts is what the lede prints, and
// a dropped series leaves points below facts.
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

// buildCells lays one series' points out against the document's columns,
// keyed on (fiscal_year, basis) rather than position, so a gap renders as a
// gap instead of sliding later figures under another year's header. A point
// in no column is dropped; the returned count of points placed lets the
// caller reconcile against counts.points.
func buildCells(points []trendPoint, columns []columnRef, meta []trendColumnMeta, cite *cellCites,
) ([]cellRef, int, error) {
	type key struct {
		year  int
		basis string
	}
	byColumn := make(map[key]trendPoint, len(points))
	for _, p := range points {
		k := key{p.FiscalYear, p.Basis}
		// Two points in one column: an assignment would lose one silently.
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
			// Not the city's dash: a row this column does not carry. The
			// title tells them apart (AGENTS.md, Provenance invariants).
			cell.Missing = true
			cell.Value = "—"
			out = append(out, cell)
			continue
		}
		cell.Value = amount.Cents(p.AmountCents).Dollars()
		cell.Cents = p.AmountCents
		cell.Negative = p.AmountCents < 0
		ref, err := cite.page(p.DocID, p.Page)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", fact.ColumnLabel(p.FiscalYear, p.Basis), err)
		}
		cell.Page = p.Page
		cell.Href = ref.TextURL
		cell.PDF = ref.PDFURL
		out = append(out, cell)
		placed++
	}
	return out, placed, nil
}
