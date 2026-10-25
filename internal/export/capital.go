package export

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

// capitalPageData is the capital template's input: the project listing as a
// table, every cell a figure the document cites, and the funds its filter
// offers. Nothing here is summed; site/capital.js sums the rows a reader has
// left shown.
type capitalPageData struct {
	chrome
	Columns []columnRef
	Funds   []capitalFund
	Rows    []capitalRow
	Facts   int
	Count   int
}

// capitalFund is one option of the fund filter.
type capitalFund struct {
	Number string
	Label  string
}

// capitalRow is one printed project line.
type capitalRow struct {
	// Fund is the fund number the row's data-fund carries and the filter
	// matches; FundLabel is what the Fund cell shows.
	Fund      string
	FundLabel string
	// Project is the printed label less the fund number it opens with.
	Project string
	Cells   []cellRef
}

// capitalPoint is a point as this page reads it: trendPoint and the token the
// page printed, which a zero renders as.
type capitalPoint struct {
	trendPoint
	Token string `json:"token"`
}

// capitalBody is the series this page tabulates.
type capitalBody struct {
	Series []struct {
		Label    string         `json:"label"`
		Fund     *int           `json:"fund"`
		FundName string         `json:"fund_name"`
		Points   []capitalPoint `json:"points"`
	} `json:"series"`
}

// buildCapitalPage renders the capital view over its series document.
//
// buildHistoryPage's rules apply: every figure is read out of the document and
// the counts must reconcile. A zero renders as the token the page printed, a
// dash, rather than as "$0", which the city did not print.
func buildCapitalPage(p pageCtx) (capitalPageData, error) {
	o, v, ix := p.o, p.v, p.ix
	cite := newCellCites(p.byID, p.pageTextBase)
	d, err := decodeSeriesDocument[capitalBody](v.Projection, o.Projections[v.Projection])
	if err != nil {
		return capitalPageData{}, err
	}
	meta, body, columns := d.meta, d.body, d.cols
	// The top bar opens on every year, which is the table whole; a year
	// narrows it to the column whose header carries the year's key.
	sel := yearSelect{Options: []yearOption{{Label: "Every year", Selected: true}}}
	for i, c := range meta.Columns {
		columns[i].Key = YearKey(c.FiscalYear, c.Basis)
		sel.Options = append(sel.Options, yearOptionOf(c.FiscalYear, c.FiscalYearLabel, c.Basis))
	}

	var funds []capitalFund
	listed := map[string]bool{}
	rows := make([]capitalRow, 0, len(body.Series))
	rendered := 0
	for _, s := range body.Series {
		if s.Fund == nil {
			return capitalPageData{}, fmt.Errorf("%s series %q names no fund; this view "+
				"files every project under the fund paying for it", v.Projection, s.Label)
		}
		number := strconv.Itoa(*s.Fund)
		project, ok := strings.CutPrefix(s.Label, number+" ")
		if !ok || strings.TrimSpace(project) == "" {
			return capitalPageData{}, fmt.Errorf("%s series %q does not open with its fund "+
				"number %s; the Fund and Project cells would disagree", v.Projection, s.Label, number)
		}
		label := number
		if s.FundName != "" {
			label = number + " " + s.FundName
		}
		if !listed[number] {
			listed[number] = true
			funds = append(funds, capitalFund{Number: number, Label: label})
		}
		points := make([]trendPoint, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, p.trendPoint)
		}
		cells, placed, cellsErr := buildCells(points, columns, meta.Columns, cite)
		if cellsErr != nil {
			return capitalPageData{}, fmt.Errorf("%s series %q: %w", v.Projection, s.Label, cellsErr)
		}
		for i := range cells {
			if cells[i].Missing || cells[i].Cents != 0 {
				continue
			}
			c := meta.Columns[i]
			for _, p := range s.Points {
				if p.FiscalYear == c.FiscalYear && p.Basis == c.Basis {
					if p.Token == "" {
						return capitalPageData{}, fmt.Errorf("%s series %q publishes a zero in %s "+
							"with no token", v.Projection, s.Label, c.FiscalYearLabel)
					}
					cells[i].Value = p.Token
				}
			}
		}
		rendered += placed
		rows = append(rows, capitalRow{Fund: number, FundLabel: label, Project: project, Cells: cells})
	}
	if err = reconcileSeriesCounts(v.Projection, rendered, len(body.Series), meta.Counts); err != nil {
		return capitalPageData{}, err
	}

	ch, _, err := p.chrome(chrome{
		Title:      cmp.Or(v.Title, "Livermore's capital projects"),
		DataPath:   ix.PublishedPath(v.Projection),
		Scope:      meta.Scope,
		YearSelect: sel,
	}, meta.Sources, cite)
	if err != nil {
		return capitalPageData{}, err
	}
	return capitalPageData{
		chrome:  ch,
		Columns: columns,
		Funds:   funds,
		Rows:    rows,
		Facts:   meta.Counts.Facts,
		Count:   meta.Counts.Series,
	}, nil
}
