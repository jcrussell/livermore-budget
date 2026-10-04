package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// historyPageData is the history template's input. Like trendsPageData it
// carries no ConfigJSON and its page loads no app.js: every figure is rendered
// server-side, so the table works with JavaScript off.
type historyPageData struct {
	chrome
	Columns []columnRef
	// Sections are the printed blocks, in the order the view declares them,
	// each holding its rows in document order.
	Sections []historySection
	// HeadSpan is every column a section heading spans: the label, category
	// and mark columns plus one per printed column.
	HeadSpan int
	Facts    int
	Count    int
}

// historySection is one printed block of the table.
type historySection struct {
	Heading string
	Series  []seriesRef
}

// buildHistoryPage renders one ACFR ten-year view.
//
// buildTrendsPage's rules apply whole: every figure is read out of the
// document, none is computed here, and the counts must reconcile. What is new
// is the grouping — each series lands in the [View.Sections] entry matching
// its (kind, fund_group) exactly, and both a series no section claims and a
// section claiming no series are refused, because either one is a heading
// telling a reader something the document does not say. A section that closes
// itself with [Section.Rows] adds the refusal the key match cannot make: a
// series its key claims but its rows do not name.
func buildHistoryPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (historyPageData, error) {
	caveatsPath := caveatsPathOf(o)
	raw := o.Projections[v.Projection]
	doc, err := decodeDocument(v.Projection, raw)
	if err != nil {
		return historyPageData{}, err
	}
	var meta trendsMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return historyPageData{}, fmt.Errorf("decode %s metadata: %w", v.Projection, err)
	}
	if len(meta.Columns) == 0 {
		return historyPageData{}, fmt.Errorf("%s metadata publishes no columns", v.Projection)
	}
	var body trendsBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return historyPageData{}, fmt.Errorf("decode %s series: %w", v.Projection, err)
	}

	columns := make([]columnRef, 0, len(meta.Columns))
	for i, c := range meta.Columns {
		columns = append(columns, columnRef{
			Label: c.FiscalYearLabel,
			Basis: basisLabelFor(meta.Caveats, c.Basis),
			Group: c.ComparableGroup,
			New:   i > 0 && c.ComparableGroup != meta.Columns[i-1].ComparableGroup,
		})
	}

	// sectionKey is the (kind, fund_group) a series is claimed by. It is not
	// [Section] itself because Rows makes that struct uncomparable, and the
	// identity of a section deliberately excludes both its heading and its
	// rows: two sections may not share a key however differently they would
	// render.
	type sectionKey struct {
		kind, fundGroup string
	}
	sections := make([]historySection, len(v.Sections))
	at := make(map[sectionKey]int, len(v.Sections))
	for i, s := range v.Sections {
		sections[i] = historySection{Heading: s.Heading}
		key := sectionKey{kind: s.Kind, fundGroup: s.FundGroup}
		if _, dup := at[key]; dup {
			return historyPageData{}, fmt.Errorf(
				"view %q declares two sections for kind %q fund group %q; a series "+
					"cannot land under both headings", v.Path, s.Kind, s.FundGroup)
		}
		at[key] = i
	}

	named := make([]map[string]bool, len(v.Sections))
	rendered := 0
	for _, s := range body.Series {
		i, ok := at[sectionKey{kind: s.Kind, fundGroup: s.FundGroup}]
		if !ok {
			return historyPageData{}, fmt.Errorf(
				"%s series %q is kind %q in fund group %q, which no section of view %q "+
					"declares; a row under the wrong printed heading is a claim the "+
					"schedule does not make", v.Projection, s.Label, s.Kind, s.FundGroup, v.Path)
		}
		label := s.Label
		if rows := v.Sections[i].Rows; rows != nil {
			display, claims := rows[s.Label]
			if !claims {
				return historyPageData{}, fmt.Errorf(
					"%s series %q is kind %q in fund group %q, which lands in section %q "+
						"of view %q, and that section's rows do not name it; a heading "+
						"that enumerates its rows must refuse a new one rather than "+
						"absorb it", v.Projection, s.Label, s.Kind, s.FundGroup,
					v.Sections[i].Heading, v.Path)
			}
			if display != "" {
				label = display
			}
			if named[i] == nil {
				named[i] = make(map[string]bool, len(rows))
			}
			named[i][s.Label] = true
		}
		cells, placed, err := buildCells(s.Points, columns, meta.Columns, pageTextBase)
		if err != nil {
			return historyPageData{}, fmt.Errorf("%s series %q: %w", v.Projection, s.Label, err)
		}
		rendered += placed
		sections[i].Series = append(sections[i].Series, seriesRef{
			Label:    label,
			Group:    s.FundGroup,
			Category: s.CategoryLabel,
			Cells:    cells,
			Mark:     buildMark(cells, columns),
		})
	}
	// The counts first: a document that disagrees with itself is refused before
	// any question about how this view groups it.
	if err := reconcileSeriesCounts(v.Projection, rendered, len(body.Series), meta.Counts); err != nil {
		return historyPageData{}, err
	}
	for i, s := range sections {
		if len(s.Series) == 0 {
			return historyPageData{}, fmt.Errorf(
				"view %q declares section %q (kind %q, fund group %q) and %s carries no "+
					"such series; a heading over nothing says the schedule prints a block "+
					"it does not", v.Path, s.Heading, v.Sections[i].Kind,
				v.Sections[i].FundGroup, v.Projection)
		}
	}
	for i, s := range v.Sections {
		for _, key := range slices.Sorted(maps.Keys(s.Rows)) {
			if !named[i][key] {
				return historyPageData{}, fmt.Errorf(
					"view %q section %q names row %q and %s carries no such series; "+
						"a name that matches nothing checks nothing",
					v.Path, s.Heading, key, v.Projection)
			}
		}
	}

	sources, _ := sourcesFor(meta.Sources, byID, pageTextBase, o.RecordsBase)
	refs := projectionRefs(o, ix)
	title := v.Title
	if title == "" {
		title = "City of Livermore ten-year history"
	}
	return historyPageData{
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
		Columns:  columns,
		Sections: sections,
		HeadSpan: 3 + len(columns),
		Facts:    meta.Counts.Facts,
		Count:    meta.Counts.Series,
	}, nil
}
