package export

// capitalPageData is the capital template's input: the chrome every view renders,
// and nothing yet. The page publishes no figure, so it cites no page and
// names no data file of its own.
type capitalPageData struct {
	chrome
}

// buildCapitalPage renders the capital view. It takes buildSite's whole argument
// list so that the page's figures, when it gains them, change this file and
// not the dispatch.
func buildCapitalPage(o *Options, v View, nav []navItem, _ map[string]Doc,
	ix ColumnIndex, _ func(string) string,
) (capitalPageData, error) {
	title := v.Title
	if title == "" {
		title = "Livermore's capital projects"
	}
	return capitalPageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
		},
	}, nil
}
