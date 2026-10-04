package export

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
)

// viewsFile is the site's views and drill steps, read from the root of the
// same filesystem registry.Load reads the vocabulary from. It is the one
// source of every authored field of an [export.View]; what Go fills is the
// year control, which steps the built documents can open, and the residual
// and gap derivations a step names.
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
	YearControl bool          `yaml:"year_control"`
	Title       string        `yaml:"title"`
	Lede        string        `yaml:"lede"`
	Sections    []sectionDecl `yaml:"sections"`
	Overview    *chartDecl    `yaml:"overview"`
	Steps       []stepDecl    `yaml:"steps"`
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
// under its own key, which decode as [export.SankeyHints] itself.
type chartDecl struct {
	Form   string              `yaml:"form"`
	Sankey *export.SankeyHints `yaml:"sankey"`
}

// stepDecl is one [export.DrillStep] as authored, its chart inlined as on the
// wire. Residual and Gaps name a derivation rather than carry one: a figure
// written here would be a second spelling of what internal/project derives.
type stepDecl struct {
	Key           string              `yaml:"key"`
	After         []string            `yaml:"after"`
	From          int                 `yaml:"from"`
	Role          string              `yaml:"role"`
	Projection    string              `yaml:"projection"`
	Form          string              `yaml:"form"`
	Sankey        *export.SankeyHints `yaml:"sankey"`
	Noun          string              `yaml:"noun"`
	Back          string              `yaml:"back"`
	Tail          string              `yaml:"tail"`
	Description   string              `yaml:"description"`
	ResidualGrain string              `yaml:"residual_grain"`
	Residual      string              `yaml:"residual"`
	Gaps          string              `yaml:"gaps"`
}

// residualDerivations is every derivation a step's `residual` may name, each
// keyed by the projection whose document it is about.
var residualDerivations = map[string]func() (map[string]string, error){
	project.FundFlowsProjection: project.FundFlowsResidual,
}

// gapDerivations is every derivation a step's `gaps` may name, keyed as
// residualDerivations is.
var gapDerivations = map[string]func() (map[string][]project.Gap, error){
	project.DepartmentSpendingProjection: project.SpendingGaps,
}

// viewsError reports a views.yaml this command cannot load. Entry names the
// view or step, already rendered; it is empty for a file-level problem.
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
// carries: a projection against the published projections, a role against
// project.Roles, a residual or gaps derivation against the ones declared
// here, and a section row against the registry's categories. It refuses an
// unknown key and an unknown name, and nothing else; what a declared view must
// satisfy is validated at the write.
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
	// internal/mapping: a misspelled `widen:` that decoded quietly would ship
	// a step with no widened column and nothing to say so.
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
		for _, s := range v.Steps {
			entry := fmt.Sprintf("view %q step %q", v.Path, s.Key)
			if s.Projection != "" && !slices.Contains(projections, s.Projection) {
				return nil, &viewsError{Entry: entry, Field: "projection",
					Msg: unknownName(s.Projection, projections)}
			}
			if s.Role != "" && !slices.Contains(project.Roles(), s.Role) {
				return nil, &viewsError{Entry: entry, Field: "role",
					Msg: unknownName(s.Role, project.Roles())}
			}
			if _, ok := residualDerivations[s.Residual]; s.Residual != "" && !ok {
				return nil, &viewsError{Entry: entry, Field: "residual",
					Msg: unknownName(s.Residual, slices.Sorted(maps.Keys(residualDerivations)))}
			}
			if _, ok := gapDerivations[s.Gaps]; s.Gaps != "" && !ok {
				return nil, &viewsError{Entry: entry, Field: "gaps",
					Msg: unknownName(s.Gaps, slices.Sorted(maps.Keys(gapDerivations)))}
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
			Path:       d.Path,
			Nav:        d.Nav,
			Template:   d.Template,
			Projection: d.Projection,
			Title:      d.Title,
			Lede:       d.Lede,
			Overview:   d.Overview.chart(),
		}
		if d.YearControl {
			v.YearStems = yearStems(d.Projection, projections)
		}
		for _, s := range d.Sections {
			v.Sections = append(v.Sections, s.section(reg))
		}
		v.Steps, err = declaredSteps(d, projections)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// declaredSteps is the steps of one view the built documents can open, in
// declared order: a step is dropped when a chart it opens from was dropped or
// the schedule it draws was not built for the column the view's own document
// is of (opensInto). A step drawing no schedule of its own draws its
// parents', so it is dropped exactly when they are. Steps are looked up by
// key, never index, because the list is filtered.
//
// ONLY A DROPPED PARENT DROPS A STEP. An After naming a key no earlier step
// declares is kept, so validateSteps refuses it by name; dropping it here
// would ship the view without that step and everything below it, in silence.
func declaredSteps(d viewDecl, projections map[string][]byte) ([]export.DrillStep, error) {
	draws := map[string][]string{"": {d.Projection}}
	dropped := map[string]bool{}
	var out []export.DrillStep
	for _, s := range d.Steps {
		var schedules []string
		if s.Projection != "" {
			schedules = []string{s.Projection}
		}
		declared := true
		for _, parent := range s.After {
			if dropped[parent] {
				declared = false
				break
			}
			if s.Projection == "" {
				schedules = append(schedules, draws[parent]...)
			}
		}
		for _, sched := range schedules {
			if !opensInto(d.Projection, sched, projections) {
				declared = false
			}
		}
		if !declared {
			dropped[s.Key] = true
			continue
		}
		draws[s.Key] = schedules
		step, err := s.step()
		if err != nil {
			return nil, fmt.Errorf("%s: view %q step %q: %w", viewsFile, d.Path, s.Key, err)
		}
		out = append(out, step)
	}
	return out, nil
}

// step is the declaration as [export.DrillStep], its residual and gaps
// derived by the functions they name; decodeViews has already refused a name
// that is no derivation.
func (s stepDecl) step() (export.DrillStep, error) {
	step := export.DrillStep{
		Key:           s.Key,
		After:         s.After,
		From:          s.From,
		Role:          s.Role,
		Projection:    s.Projection,
		Chart:         (&chartDecl{Form: s.Form, Sankey: s.Sankey}).chart(),
		Back:          s.Back,
		Tail:          s.Tail,
		Noun:          s.Noun,
		Description:   s.Description,
		ResidualGrain: s.ResidualGrain,
	}
	if s.Residual != "" {
		residual, err := residualDerivations[s.Residual]()
		if err != nil {
			return export.DrillStep{}, err
		}
		step.Residual = residual
	}
	if s.Gaps != "" {
		gaps, err := gapDerivations[s.Gaps]()
		if err != nil {
			return export.DrillStep{}, err
		}
		step.Gaps = gaps
	}
	return step, nil
}

// chart is the declaration as [export.Chart]; a nil declaration is a view
// that draws no chart.
func (c *chartDecl) chart() export.Chart {
	if c == nil {
		return export.Chart{}
	}
	return export.Chart{Form: c.Form, Sankey: c.Sankey}
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
