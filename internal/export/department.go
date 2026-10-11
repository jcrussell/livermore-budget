package export

// departmentPageData is the department template's input: the chrome every view renders,
// and nothing yet. The page publishes no figure, so it cites no page and
// names no data file of its own.
type departmentPageData struct {
	chrome
}

// buildDepartmentPage renders the department view. It takes buildSite's whole argument
// list so that the page's figures, when it gains them, change this file and
// not the dispatch.
func buildDepartmentPage(o *Options, v View, nav []navItem, _ map[string]Doc,
	ix ColumnIndex, _ func(string) string,
) (departmentPageData, error) {
	title := v.Title
	if title == "" {
		title = "Where Livermore's money goes, department by department"
	}
	return departmentPageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
		},
	}, nil
}
