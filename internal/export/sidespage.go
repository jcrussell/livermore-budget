package export

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/schema"
)

// sidesConfigVersion is the version a sides config declares.
const sidesConfigVersion = 1

// sidesConfig is what site/sides.js reads from ledger.html before it fetches
// a column, shaped by schema/sides.schema.json. It carries no figure: every
// figure is a link of the column a year names, folded by the client to the
// frontier it opens.
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

// sidesPageData is the ledger template's input.
type sidesPageData struct {
	chrome
	Description string
	Years       []sidesYear
	Sides       []Side
	OpensLede   string
	// ConfigJSON is the page's config. json.Marshal escapes <, > and &, so the
	// blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// encodeSidesConfig renders the config and refuses bytes that do not match
// the published schema.
func encodeSidesConfig(cfg sidesConfig) ([]byte, error) {
	blob, err := json.Marshal(cfg)
	return schemaChecked(blob, err, schema.Sides, "sides config")
}

// buildSidesPage renders a ledger view: the spine's years as the year
// control, the drawn schedule's declared sides as sections the client fills,
// and the config the client folds by. The chart's figures are the
// drawn schedule's; the year control is the spine's.
func buildSidesPage(p pageCtx) (sidesPageData, error) {
	o, v, ix := p.o, p.v, p.ix
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
	var (
		years []sidesYear
		sel   yearSelect
		cited []sourceMeta
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
					"one view is one scope and one build", v.Path, v.Projection, meta.Scope(), meta.GeneratedBy,
				stem, m.Scope(), m.GeneratedBy)
		}
		chart, chartErr := drawnDocument(v, stem, meta.GeneratedBy, o.Projections, ix)
		if chartErr != nil {
			return sidesPageData{}, chartErr
		}
		cited = append(cited, m.Sources...)
		cited = append(cited, chart.doc.Metadata.Sources...)
		sel.newest(m.FiscalYear, m.FiscalYearLabel, m.Basis)
		years = append(years, sidesYear{
			Path:  ColumnPath(m.FiscalYear, m.Basis),
			Label: m.FiscalYearLabel,
			Basis: m.Basis,
			Lede:  ledeOf(m.FiscalYearLabel, m.Basis),
		})
	}
	if len(years) == 0 {
		return sidesPageData{}, fmt.Errorf("view %q opens no year", v.Path)
	}
	open := years[len(years)-1]
	ch, docs, err := p.chrome(chrome{
		Title:      cmp.Or(v.Title, "Livermore's budget, side by side"),
		DataPath:   open.Path,
		Scope:      meta.Scope(),
		YearSelect: sel,
	}, cited, nil)
	if err != nil {
		return sidesPageData{}, err
	}
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
		chrome:      ch,
		Description: description,
		Years:       years,
		Sides:       slices.Clone(hints.Sides),
		OpensLede:   open.Lede,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and &, so it cannot terminate the script element it sits in.
		ConfigJSON: template.JS(blob),
	}, nil
}
