package export

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// viewsFile is the site's views, read from the root of the same filesystem
// registry.Load reads the vocabulary from. It is the one source of every
// authored field of an [export.View]; what Go fills is the year control.
const viewsFile = "views.yaml"

// viewsSchemaVersion is the only views.yaml version this command reads.
const viewsSchemaVersion = 1

// viewsDoc is the whole of views.yaml. Every key the file may carry is
// declared, because the decoder refuses an unknown one.
type viewsDoc struct {
	SchemaVersion int        `yaml:"schema_version"`
	Views         []viewDecl `yaml:"views"`
}

// viewDecl is one page as authored. Its fields are [export.View]'s authored
// ones under the file's keys; YearControl is the one declaration Go fills
// from the built documents rather than copies.
type viewDecl struct {
	Path       string `yaml:"path"`
	Nav        string `yaml:"nav"`
	Template   string `yaml:"template"`
	Projection string `yaml:"projection"`
	// YearControl lists every built document of Projection as a year, oldest
	// first, which is [export.View.YearStems].
	YearControl bool   `yaml:"year_control"`
	Title       string `yaml:"title"`
	// Description is [export.View.Description].
	Description string        `yaml:"description"`
	Lede        string        `yaml:"lede"`
	Sections    []sectionDecl `yaml:"sections"`
	Overview    *chartDecl    `yaml:"overview"`
}

// sectionDecl is one [export.Section] as authored. Rows maps a printed row
// label to the taxonomy category whose document term the row displays, or to
// "" to display the printed label.
type sectionDecl struct {
	Heading   string            `yaml:"heading"`
	Kind      string            `yaml:"kind"`
	FundGroup string            `yaml:"fund_group"`
	Rows      map[string]string `yaml:"rows"`
}

// chartDecl is an [export.Chart] as authored: the form and the form's hints
// under its own key, which decode as [export.SankeyHints] itself. Projection
// is the overview's [export.View.Draws].
type chartDecl struct {
	Form       string              `yaml:"form"`
	Projection string              `yaml:"projection"`
	Sankey     *export.SankeyHints `yaml:"sankey"`
	Bars       *export.BarsHints   `yaml:"bars"`
}

// viewsError reports a views.yaml this command cannot load. Entry names the
// view, already rendered; it is empty for a file-level problem.
type viewsError struct {
	Entry string
	Field string
	Msg   string
}

func (e *viewsError) Error() string {
	var b strings.Builder
	b.WriteString(viewsFile)
	if e.Entry != "" {
		b.WriteString(": " + e.Entry)
	}
	if e.Field != "" {
		b.WriteString(": " + e.Field)
	}
	b.WriteString(": " + e.Msg)
	return b.String()
}

// loadViews reads views.yaml from the root of fsys and resolves every name it
// carries: a projection against the published projections and a section row
// against the registry's categories. It refuses an unknown key and an unknown
// name, and nothing else; what a declared view must satisfy is validated at
// the write.
func loadViews(fsys fs.FS, reg *registry.Registry) ([]viewDecl, error) {
	b, err := fs.ReadFile(fsys, viewsFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", viewsFile, err)
	}
	return decodeViews(b, reg)
}

// decodeViews is loadViews over the file's bytes.
func decodeViews(b []byte, reg *registry.Registry) ([]viewDecl, error) {
	var doc viewsDoc
	dec := yaml.NewDecoder(bytes.NewReader(b))
	// An unknown key is a hard error, as in internal/registry and
	// internal/mapping: a misspelled `split_caps:` that decoded quietly would
	// ship every split uncapped and nothing to say so.
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &viewsError{Msg: "file is empty"}
		}
		return nil, &viewsError{Msg: err.Error()}
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, &viewsError{Msg: "file contains more than one YAML document"}
	} else if !errors.Is(err, io.EOF) {
		return nil, &viewsError{Msg: err.Error()}
	}
	if doc.SchemaVersion != viewsSchemaVersion {
		return nil, &viewsError{Field: "schema_version",
			Msg: fmt.Sprintf("got %d, want %d", doc.SchemaVersion, viewsSchemaVersion)}
	}
	projections := publishedProjections()
	for _, v := range doc.Views {
		entry := fmt.Sprintf("view %q", v.Path)
		if v.Projection != "" && !slices.Contains(projections, v.Projection) {
			return nil, &viewsError{Entry: entry, Field: "projection",
				Msg: unknownName(v.Projection, projections)}
		}
		if v.Overview != nil && v.Overview.Projection != "" && !slices.Contains(projections, v.Overview.Projection) {
			return nil, &viewsError{Entry: entry, Field: "overview.projection",
				Msg: unknownName(v.Overview.Projection, projections)}
		}
		for _, s := range v.Sections {
			for printed, slug := range s.Rows {
				if slug == "" {
					continue
				}
				if _, ok := reg.Category(slug); !ok {
					return nil, &viewsError{Entry: entry,
						Field: fmt.Sprintf("section %q row %q", s.Heading, printed),
						Msg:   fmt.Sprintf("%q names no category in %s", slug, registry.TaxonomyFile)}
				}
			}
		}
	}
	return doc.Views, nil
}

// unknownName is the refusal for a name that resolves to nothing, naming what
// it could have been.
func unknownName(got string, known []string) string {
	return fmt.Sprintf("%q is not one of %s", got, strings.Join(known, ", "))
}

// publishedProjections is every projection name the site publishes a
// document of, in declared order and once each.
func publishedProjections() []string {
	var out []string
	for _, d := range project.PublishedDocuments() {
		if !slices.Contains(out, d.Projection) {
			out = append(out, d.Projection)
		}
	}
	return out
}

// views is the site's pages, in nav order, the page it opens on first, as
// data/views.yaml declares them and the built documents allow. It lives in
// the composition root because naming a view means knowing what a projection
// is of, which internal/export must not.
//
// A view whose document was not built is dropped rather than refused: `fisc
// verify` already fails a missing published document, and a nav entry to an
// unwritten page is what must never ship. The provenance index names no
// document and is dropped when there is no page index, for the same reason.
func views(repoRoot string, built result) ([]export.View, error) {
	reg, err := loadRegistry(repoRoot)
	if err != nil {
		return nil, err
	}
	decls, err := loadViews(os.DirFS(filepath.Join(repoRoot, "data")), reg)
	if err != nil {
		return nil, err
	}
	projections := built.Projections
	var out []export.View
	for _, d := range decls {
		if d.Projection != "" {
			if _, ok := projections[d.Projection]; !ok {
				continue
			}
		}
		if d.Template == export.ProvenanceTemplate && len(built.PageIndex) == 0 {
			continue
		}
		v := export.View{
			Path:        d.Path,
			Nav:         d.Nav,
			Template:    d.Template,
			Projection:  d.Projection,
			Title:       d.Title,
			Lede:        d.Lede,
			Overview:    d.Overview.chart(),
			Description: d.Description,
		}
		if d.Overview != nil {
			v.Draws = d.Overview.Projection
		}
		if !drawsBuilt(v, projections) {
			// THE SITE OPENS ON EXACTLY ONE INDEX, so where the schedule its
			// chart draws was not built it draws the view's own document, the
			// spine, with nothing to split; any other view is dropped.
			if v.Path != export.IndexPath {
				continue
			}
			v.Draws = ""
		}
		if d.YearControl {
			v.YearStems = yearStems(d.Projection, projections)
		}
		for _, s := range d.Sections {
			v.Sections = append(v.Sections, s.section(reg))
		}
		out = append(out, v)
	}
	return out, nil
}

// drawsBuilt reports whether the schedule a view's own chart draws was built
// for every year the view lists, where it is published for that year's
// column at all. A view drawing a schedule that was not built is dropped, as
// one whose own document was not built is, but for the index, which draws its
// own document instead; one never published for a year's column is kept for
// the write to refuse by name.
func drawsBuilt(v export.View, projections map[string][]byte) bool {
	if v.Draws == "" {
		return true
	}
	years := v.YearStems
	if len(years) == 0 {
		years = []string{v.Projection}
	}
	for _, year := range years {
		if len(publishedInto(year, v.Draws)) > 0 && !opensInto(year, v.Draws, projections) {
			return false
		}
	}
	return true
}

// chart is the declaration as [export.Chart]; a nil declaration is a view
// that draws no chart.
func (c *chartDecl) chart() export.Chart {
	if c == nil {
		return export.Chart{}
	}
	return export.Chart{Form: c.Form, Sankey: c.Sankey, Bars: c.Bars}
}

// section is the declaration as [export.Section], each row's category
// resolved to the term the document prints for it. loadViews has already
// refused a slug the registry does not know.
func (s sectionDecl) section(reg *registry.Registry) export.Section {
	out := export.Section{Heading: s.Heading, Kind: s.Kind, FundGroup: s.FundGroup}
	if s.Rows != nil {
		out.Rows = make(map[string]string, len(s.Rows))
		for printed, slug := range s.Rows {
			label := ""
			if slug != "" {
				c, _ := reg.Category(slug)
				label = c.DocumentTerm
			}
			out.Rows[printed] = label
		}
	}
	return out
}
