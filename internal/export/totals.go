package export

import "fmt"

// totalsYear is one year's headline figures on the totals page.
type totalsYear struct {
	Label   string
	Figures []figure
	Caveats []caveatRef
}

// totalsPageData is the totals page: every year of the spine's headline
// figures, newest first.
type totalsPageData struct {
	chrome
	Years []totalsYear
}

// buildTotalsPage renders tilesFor over every year the view lists, server-side:
// the page ships no script, so no year switch repaints it.
//
// Scope and builder are refused per year as buildSankeyPage refuses them,
// because the footer states one of each.
func buildTotalsPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (totalsPageData, error) {
	_, meta, err := decodeSankey(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return totalsPageData{}, err
	}
	stems := v.YearStems
	if len(stems) == 0 {
		stems = []string{v.Projection}
	}
	caveatsPath := caveatsPathOf(o)
	years := make([]totalsYear, 0, len(stems))
	var cited []sourceMeta
	for _, stem := range stems {
		m := meta
		if stem != v.Projection {
			if _, m, err = decodeSankey(stem, o.Projections[stem]); err != nil {
				return totalsPageData{}, err
			}
		}
		if m.Scope() != meta.Scope() || m.GeneratedBy != meta.GeneratedBy {
			return totalsPageData{}, fmt.Errorf(
				"view %q opens on %q (scope %q, built by %q) and its year stem %q is scope %q, built by %q; "+
					"the footer states one of each", v.Path, v.Projection, meta.Scope(), meta.GeneratedBy,
				stem, m.Scope(), m.GeneratedBy)
		}
		cited = append(cited, m.Sources...)
		hero, figures := tilesFor(m)
		// Newest first: the stems are oldest first.
		years = append([]totalsYear{{
			Label:   ledeOf(m.FiscalYearLabel, m.Basis),
			Figures: append([]figure{hero}, figures...),
			Caveats: caveatRefs(m.Caveats, stem, caveatsPath),
		}}, years...)
	}
	sources, _ := sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)
	title := v.Title
	if title == "" {
		title = "The totals behind the budget flows"
	}
	return totalsPageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  projectionRefs(o, ix),
			Scope:        meta.Scope(),
			CaveatsPath:  caveatsPath,
		},
		Years: years,
	}, nil
}
