package export

import (
	"encoding/json"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/schema"
)

// sidesConfigVersion is the version a sides config declares.
const sidesConfigVersion = 1

// sidesConfig is what site/sides.js reads from treemap.html and ledger.html
// before it fetches a column, shaped by schema/sides.schema.json. It carries
// no figure: every figure is a link of the column a year names, folded by
// the client to the frontier it opens.
type sidesConfig struct {
	SchemaVersion int                  `json:"schema_version"`
	ExportedBy    string               `json:"exported_by"`
	Primary       string               `json:"primary"`
	Overview      Chart                `json:"overview"`
	Years         []sidesYear          `json:"years"`
	Docs          map[string]clientDoc `json:"docs"`
}

// sidesYear is one column the page opens.
type sidesYear struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Basis string `json:"basis"`
	Lede  string `json:"lede"`
}

// sidesPageData is the treemap and ledger templates' input.
type sidesPageData struct {
	chrome
	Description string
	Years       []sidesYear
	Sides       []Side
	Opens       string
	OpensLede   string
	// ConfigJSON is the page's config. json.Marshal escapes <, > and &, so the
	// blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// encodeSidesConfig renders the config and refuses bytes that do not match
// the published schema.
func encodeSidesConfig(cfg sidesConfig) ([]byte, error) {
	blob, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode sides config: %w", err)
	}
	var v any
	if err := json.Unmarshal(blob, &v); err != nil {
		return nil, fmt.Errorf("re-read sides config: %w", err)
	}
	if err := schema.Validate(schema.Sides, v); err != nil {
		return nil, fmt.Errorf("the sides config this build produced does not match %s: %w", schema.Sides, err)
	}
	return blob, nil
}

// buildSidesPage renders a treemap or ledger view: the spine's years as the
// year control, the drawn schedule's declared sides as sections the client
// fills, and the config the client folds by. The chart's figures are the
// drawn schedule's; the footer and the year control are the spine's.
func buildSidesPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (sidesPageData, error) {
	hints := v.Overview.sides()
	if hints == nil {
		return sidesPageData{}, fmt.Errorf("view %q renders template %q with no %s hints", v.Path, v.Template, v.Overview.Form)
	}
	if v.Draws == "" {
		return sidesPageData{}, fmt.Errorf("view %q draws no schedule; a sides page opens the expansion schedule of each year's column", v.Path)
	}
	_, meta, err := decodeSankey(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return sidesPageData{}, err
	}
	caveatsPath := caveatsPathOf(o)
	var (
		years   []sidesYear
		cited   []sourceMeta
		caveats []caveatRef
	)
	for _, stem := range v.yearStems() {
		m := meta
		if stem != v.Projection {
			if _, m, err = decodeSankey(stem, o.Projections[stem]); err != nil {
				return sidesPageData{}, err
			}
		}
		if m.Scope() != meta.Scope() || m.GeneratedBy != meta.GeneratedBy {
			return sidesPageData{}, fmt.Errorf(
				"view %q opens on %q (scope %q, built by %q) and its year stem %q is scope %q, built by %q; "+
					"the footer states one of each", v.Path, v.Projection, meta.Scope(), meta.GeneratedBy,
				stem, m.Scope(), m.GeneratedBy)
		}
		chart, chartErr := drawnDocument(v, stem, meta.GeneratedBy, o.Projections, ix)
		if chartErr != nil {
			return sidesPageData{}, chartErr
		}
		cited = append(cited, m.Sources...)
		cited = append(cited, chart.doc.Metadata.Sources...)
		years = append(years, sidesYear{
			Path:  ColumnPath(m.FiscalYear, m.Basis),
			Label: m.FiscalYearLabel,
			Basis: m.Basis,
			Lede:  ledeOf(m.FiscalYearLabel, m.Basis),
		})
		// The footer lists the opening year's caveats, which is the last.
		caveats = append(caveatRefs(m.Caveats, stem, caveatsPath),
			caveatRefs(chart.doc.Metadata.Caveats, chart.stem, caveatsPath)...)
	}
	if len(years) == 0 {
		return sidesPageData{}, fmt.Errorf("view %q opens no year", v.Path)
	}
	sources, docs := sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)
	blob, err := encodeSidesConfig(sidesConfig{
		SchemaVersion: sidesConfigVersion,
		ExportedBy:    o.GeneratedBy,
		Primary:       v.Draws,
		Overview:      v.Overview,
		Years:         years,
		Docs:          docs,
	})
	if err != nil {
		return sidesPageData{}, err
	}
	open := years[len(years)-1]
	title := v.Title
	if title == "" {
		title = "Livermore's budget, side by side"
	}
	headings := make([]string, 0, len(hints.Sides))
	for _, s := range hints.Sides {
		headings = append(headings, strings.ToLower(s.Heading))
	}
	description := v.Description
	if description == "" {
		description = "The City of Livermore's budget as " + v.Overview.Form + " charts: " + strings.Join(headings, ", ") +
			", each opening in place into the parts the budget prints, every figure a sum of cells the city printed."
	}
	return sidesPageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  projectionRefs(o, ix),
			DataPath:     open.Path,
			Scope:        meta.Scope(),
			Caveats:      caveats,
			CaveatsPath:  caveatsPath,
		},
		Description: description,
		Years:       years,
		Sides:       slices.Clone(hints.Sides),
		Opens:       open.Path,
		OpensLede:   open.Lede,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and &, so it cannot terminate the script element it sits in.
		ConfigJSON: template.JS(blob),
	}, nil
}
