package export

import (
	"encoding/json"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// changeFigure is one chart of the change page: a series document's rows of
// one kind, each cell a printed point, which site/change.js groups by
// category and stacks per column. Go sums nothing; the table is what the
// page shows with no script.
type changeFigure struct {
	ID      string
	Heading string
	Lede    string
	// Chart is whether the client stacks this figure's rows; the excess rows
	// are listed and drawn as no chart.
	Chart   bool
	Columns []columnRef
	Rows    []changeRow
	// Document is the series document the rows are read from, for the
	// figure's own data link.
	Document string
}

// changeRow is one printed series as the table renders it.
type changeRow struct {
	Label    string
	Category string
	// Under is the fund a budget row belongs to, or "" for an audited row,
	// which pp.168 print for every governmental fund together.
	Under string
	Cells []cellRef
}

// changePageData is the change template's input.
type changePageData struct {
	chrome
	Figures []changeFigure
}

// changeDocument is one series document as the change page reads it.
type changeDocument struct {
	stem string
	meta trendsMetadata
	body trendsBody
	cols []columnRef
}

// decodeChangeDocument reads a series document the page draws, refusing one
// with no columns.
func decodeChangeDocument(stem string, raw []byte) (changeDocument, error) {
	doc, err := decodeDocument(stem, raw)
	if err != nil {
		return changeDocument{}, err
	}
	var d changeDocument
	d.stem = stem
	if err := json.Unmarshal(doc.Metadata, &d.meta); err != nil {
		return changeDocument{}, fmt.Errorf("decode %s metadata: %w", stem, err)
	}
	if len(d.meta.Columns) == 0 {
		return changeDocument{}, fmt.Errorf("%s metadata publishes no columns", stem)
	}
	if err := json.Unmarshal(raw, &d.body); err != nil {
		return changeDocument{}, fmt.Errorf("decode %s series: %w", stem, err)
	}
	for i, c := range d.meta.Columns {
		d.cols = append(d.cols, columnRef{
			Label: c.FiscalYearLabel,
			Basis: basisLabelFor(d.meta.Caveats, c.Basis),
			Group: c.ComparableGroup,
			New:   i > 0 && c.ComparableGroup != d.meta.Columns[i-1].ComparableGroup,
		})
	}
	return d, nil
}

// changeRows is every series of doc of one kind as rows, each cell built by
// buildCells. It refuses a figure with no row, which would be
// a chart of nothing under a heading.
func changeRows(d changeDocument, kind string, pageTextBase func(string) string) ([]changeRow, int, error) {
	var rows []changeRow
	placed, published := 0, 0
	for _, s := range d.body.Series {
		if s.Kind != kind {
			continue
		}
		if s.CategoryLabel == "" {
			return nil, 0, fmt.Errorf("%s series %q carries no category label, and the chart groups by it", d.stem, s.Label)
		}
		cells, n, err := buildCells(s.Points, d.cols, d.meta.Columns, pageTextBase)
		if err != nil {
			return nil, 0, fmt.Errorf("%s series %q: %w", d.stem, s.Label, err)
		}
		placed += n
		published += len(s.Points)
		rows = append(rows, changeRow{Label: s.Label, Category: s.CategoryLabel, Under: s.FundName, Cells: cells})
	}
	if len(rows) == 0 {
		return nil, 0, fmt.Errorf("%s publishes no series of kind %q", d.stem, kind)
	}
	// buildCells places a point only into a declared column and says nothing
	// of one it finds no column for, so a kind's figure is held to every
	// point of that kind here, whether or not the document as a whole is.
	if placed != published {
		return nil, 0, fmt.Errorf("%s publishes %d points of kind %q and %d of them fall in a declared column",
			d.stem, published, kind, placed)
	}
	return rows, placed, nil
}

// buildChangePage renders the change view: the ACFR's ten reported years of
// governmental-fund revenue and expenditure by category, the printed excess
// of one over the other, and the Budget Book's four columns of revenue by
// category. The view's own document is the ten-year schedule; the budget
// columns are read from the revenue-trends document, which must be built.
func buildChangePage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string, cx *caveatIndex,
) (changePageData, error) {
	if v.Projection != project.ChangesProjection {
		return changePageData{}, fmt.Errorf("view %q renders the change page over %q; the page's own document is %s",
			v.Path, v.Projection, project.ChangesProjection)
	}
	trendsRaw, ok := o.Projections[project.TrendsProjection]
	if !ok {
		return changePageData{}, fmt.Errorf("view %q draws the budget's columns from %s, which was not built",
			v.Path, project.TrendsProjection)
	}
	audited, err := decodeChangeDocument(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return changePageData{}, err
	}
	budget, err := decodeChangeDocument(project.TrendsProjection, trendsRaw)
	if err != nil {
		return changePageData{}, err
	}
	if audited.meta.GeneratedBy != budget.meta.GeneratedBy {
		return changePageData{}, fmt.Errorf("view %q reads documents built by %q and %q; one view is one build",
			v.Path, audited.meta.GeneratedBy, budget.meta.GeneratedBy)
	}

	type spec struct {
		id, heading, lede, kind string
		chart                   bool
		doc                     changeDocument
	}
	specs := []spec{
		{"audited-revenue", "What the city took in, ten years as reported",
			"Governmental funds' revenue by kind, each year as the city's annual financial report prints it. " +
				"Each column's height is our sum of the rows printed under it.",
			"revenue", true, audited},
		{"audited-spending", "What the city spent, ten years as reported",
			"Governmental funds' expenditure by function, each year as the city's annual financial report prints it.",
			"expenditure", true, audited},
		{"audited-excess", "What was left over, as the city prints it",
			"The excess of revenues over expenditures the report prints for each year, before money moved between " +
				"funds and other financing; a negative figure is a year the funds spent more than they took in. " +
				"Printed, not computed here.",
			"fund_balance", false, audited},
		{"budget-revenue", "What the budget plans to take in, four printed columns",
			"Every revenue line of every fund, by kind, across the Budget Book's four columns. A rule between two " +
				"columns marks two measurements that are not one: money that moved, a re-forecast, and two years of " +
				"one adopted budget. Money moved between the city's own funds is not counted.",
			"revenue", true, budget},
	}
	placed := map[string]int{}
	figures := make([]changeFigure, 0, len(specs))
	for _, s := range specs {
		rows, n, err := changeRows(s.doc, s.kind, pageTextBase)
		if err != nil {
			return changePageData{}, fmt.Errorf("view %q, %s: %w", v.Path, s.id, err)
		}
		placed[s.doc.stem] += n
		figures = append(figures, changeFigure{
			ID: s.id, Heading: s.heading, Lede: s.lede, Chart: s.chart,
			Columns: s.doc.cols, Rows: rows,
			Document: ix.PublishedPath(s.doc.stem),
		})
	}
	// Each document's own count holds the page to every point it publishes:
	// a point is placed in a figure or is of a kind leftOut declares the page
	// leaves out. The budget document's transfers in are money moved between
	// the city's own funds, which the budget figure's lede says it does not
	// count.
	leftOut := map[string]map[string]bool{budget.stem: {"transfer_in": true}}
	for _, d := range []changeDocument{audited, budget} {
		left := 0
		for _, s := range d.body.Series {
			if leftOut[d.stem][s.Kind] {
				left += len(s.Points)
			}
		}
		if placed[d.stem]+left != d.meta.Counts.Points {
			return changePageData{}, fmt.Errorf("view %q places %d of %s's %d points and leaves out %d; "+
				"a kind of its series is drawn nowhere", v.Path, placed[d.stem], d.stem, d.meta.Counts.Points, left)
		}
	}

	cited := append(append([]sourceMeta{}, audited.meta.Sources...), budget.meta.Sources...)
	sources, sourcesPath, _, serr := citedSources(o, v.Path, cited, byID, pageTextBase)
	if serr != nil {
		return changePageData{}, serr
	}
	title := v.Title
	if title == "" {
		title = "Livermore's budget over time"
	}
	return changePageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			Sources:     sources,
			SourcesPath: sourcesPath,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
			DataPath:    ix.PublishedPath(v.Projection),
			Scope:       audited.meta.Scope + ", " + budget.meta.Scope,
			CaveatsHere: cx.here(v.Path),
		},
		Figures: figures,
	}, nil
}
