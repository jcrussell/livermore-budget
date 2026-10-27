package export

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/build"
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/repo"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// factsPath is repo's, the path `fisc build` writes and `fisc verify` checks.
const factsPath = repo.FactsPath

// buildAll is the default Builder: everything the site ships.
func buildAll(repoRoot string) (result, error) {
	// One read of the store: the shards are compared against these raw bytes.
	raw, facts, err := readFactStore(repoRoot)
	if err != nil {
		return result{}, err
	}
	return buildFrom(repoRoot, raw, facts, structure.BudgetBookExceptions())
}

// buildFrom is buildAll over a store already read: raw must be facts as
// fact.Write encodes them, which buildFactAssets asserts.
func buildFrom(repoRoot string, raw []byte, facts []fact.Fact, exceptions []structure.Exception) (result, error) {
	projections, err := buildProjectionsFrom(repoRoot, facts, exceptions)
	if err != nil {
		return result{}, err
	}
	assets, err := buildFactAssets(raw, facts, generatedBy())
	if err != nil {
		return result{}, err
	}
	return result{
		Projections: projections,
		Files:       assets.Files,
		PageIndex:   assets.pageIndex(),
		Downloads:   assets.downloads(),
		RecordsBase: assets.recordsBase(),
		Exceptions:  exceptions,
	}, nil
}

// readFactStore reads facts/facts.jsonl as raw bytes and decoded facts from one
// read, since buildFactAssets asserts the two encode alike.
func readFactStore(repoRoot string) ([]byte, []fact.Fact, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(factsPath))
	// #nosec G304 -- the path is the repository root fisc found by walking up
	// from the working directory, joined to a constant; it is not user input.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, hint.Withf(fmt.Errorf("read the fact store: %w", err),
			"run `fisc build` to generate %s", factsPath)
	}
	facts, err := fact.Read(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", factsPath, err)
	}
	return raw, facts, nil
}

// buildProjectionsFrom is the pipeline itself, over facts already read.
func buildProjectionsFrom(repoRoot string, facts []fact.Fact, exceptions []structure.Exception) (map[string][]byte, error) {
	// The label registry is loaded here and passed in: internal/project
	// reaches it only through an interface it declares.
	reg, err := loadRegistry(repoRoot)
	if err != nil {
		return nil, err
	}

	// The slices are internal/project's declaration, which `fisc verify`
	// checks too. The loop is over projections, not years: a trends document
	// is one slice spanning four columns.
	version := build.Get().String()
	out := map[string][]byte{}
	builtAt := map[string]builtDoc{}
	for _, p := range project.Registry(reg, exceptions) {
		declared := p.Slices(facts, version)
		for _, o := range declared {
			stem, err := project.Stem(p.Name(), o, declared)
			if err != nil {
				return nil, err
			}
			// Two documents on one stem would silently drop one.
			if prev, ok := builtAt[stem]; ok {
				// Both projections are named: builtAt spans the whole
				// registry loop.
				return nil, fmt.Errorf(
					"the %s and %s projections both want the stem %q: %s and %s",
					prev.name, p.Name(), stem,
					project.Describe(prev.opts.Columns), project.Describe(o.Columns))
			}
			builtAt[stem] = builtDoc{name: p.Name(), opts: o}
			b, err := p.Build(facts, o)
			if err != nil {
				return nil, fmt.Errorf("build the %s projection for %s: %w",
					p.Name(), project.Describe(o.Columns), err)
			}
			out[stem] = b
		}
	}

	// The published set is asserted, so export and verify cannot diverge on
	// which slices the site publishes. It fails closed naming both sides.
	if err := assertPublishedBuilt(builtAt); err != nil {
		return nil, err
	}
	if _, ok := out[export.PrimaryProjection]; !ok {
		return nil, fmt.Errorf("no projection named %q was registered", export.PrimaryProjection)
	}
	return out, nil
}

func loadRegistry(repoRoot string) (*registry.Registry, error) {
	reg, err := registry.Load(os.DirFS(filepath.Join(repoRoot, "data")))
	if err != nil {
		return nil, fmt.Errorf("load the data registries: %w", err)
	}
	return reg, nil
}

// assertPublishedBuilt is the export side of published-projection-built. The
// stem existing is not enough: a document built over fewer columns than
// published lands at the right path and agrees with itself, so
// project.MissingColumns, shared with internal/check, is compared too.
func assertPublishedBuilt(builtAt map[string]builtDoc) error {
	for _, d := range project.PublishedDocuments() {
		b, ok := builtAt[d.Stem]
		if !ok {
			return fmt.Errorf(
				"the site publishes %s and no projection built it; the documents "+
					"built were: %s", d, strings.Join(builtStems(builtAt), ", "))
		}
		if missing := project.MissingColumns(d, b.opts); len(missing) > 0 {
			return fmt.Errorf(
				"the site publishes %s and the document built at that stem covers %s, "+
					"missing %s", d, project.Describe(b.opts.Columns), project.Describe(missing))
		}
	}
	return nil
}

// builtStems is the stems built, sorted, for a refusal.
func builtStems(builtAt map[string]builtDoc) []string {
	return slices.Sorted(maps.Keys(builtAt))
}

// builtDoc is a document that has claimed a stem, and the projection that
// claimed it, so a collision names both.
type builtDoc struct {
	name string
	opts project.Options
}

// sourceRegistry is as much of data/sources.yaml as the site needs: the only
// place a URL is asserted.
type sourceRegistry struct {
	Sources []struct {
		ID        string `yaml:"id"`
		Title     string `yaml:"title"`
		Publisher string `yaml:"publisher"`
		// URL is the stable canonical address; url_versioned changes on every
		// re-upload and would rot every citation.
		URL string `yaml:"url"`
	} `yaml:"sources"`
}

// loadDocs reads the source registry into the shape internal/export cites.
func loadDocs(repoRoot string) ([]export.Doc, error) {
	path := filepath.Join(repoRoot, "data", "sources.yaml")
	// #nosec G304 -- as above: repository root plus a constant path.
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source registry: %w", err)
	}
	var reg sourceRegistry
	if err := yaml.Unmarshal(b, &reg); err != nil {
		return nil, fmt.Errorf("parse source registry %q: %w", path, err)
	}
	docs := make([]export.Doc, 0, len(reg.Sources))
	for _, s := range reg.Sources {
		docs = append(docs, export.Doc{
			ID:        s.ID,
			Title:     s.Title,
			Publisher: s.Publisher,
			PDFURL:    s.URL,
		})
	}
	return docs, nil
}

// generatedBy names this binary for the page footer.
func generatedBy() string { return "fisc " + build.Get().String() }

// yearStems is the built stems of project.PublishedDocuments for one
// projection, in reading order, so the year control never offers a 404.
func yearStems(name string, projections map[string][]byte) []string {
	docs := project.PublishedDocuments()
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		if d.Projection != name {
			continue
		}
		if _, ok := projections[d.Stem]; ok {
			out = append(out, d.Stem)
		}
	}
	return out
}

// unviewedDocuments are documents the site publishes and no page renders, each
// with its reason; assertPublishedReachable refuses an entry a view renders or
// that names no published document. Each is a column pp.66-67 print no year
// for.
var unviewedDocuments = map[string]string{
	publishedStem(project.FundFlowsProjection, actual2024):  fundFlowsNoSpineColumn,
	publishedStem(project.FundFlowsProjection, revised2025): fundFlowsNoSpineColumn,

	publishedStem(project.FundSourcesUsesProjection, actual2024):  sourcesUsesNoSpineColumn,
	publishedStem(project.FundSourcesUsesProjection, revised2025): sourcesUsesNoSpineColumn,
}

// The two columns pp.66-67 print no year for.
var (
	actual2024  = project.Column{FiscalYear: 2024, Basis: vocab.BasisActual}
	revised2025 = project.Column{FiscalYear: 2025, Basis: vocab.BasisRevised}
)

// publishedStem is the stem of a projection's one-column published document,
// or "", which assertPublishedReachable refuses.
func publishedStem(projection string, col project.Column) string {
	for _, d := range project.PublishedDocuments() {
		if d.Projection == projection && len(d.Columns) == 1 && d.Columns[0] == col {
			return d.Stem
		}
	}
	return ""
}

const sourcesUsesNoSpineColumn = "a published column of pp.186-209's per-fund sources and " +
	"uses with no spine year to stitch it into: the expansion schedule is stitched per spine " +
	"column and pp.66-67 print no actual or revised column. It is published because this is the only " +
	"document drawing those pages, and two of the four columns would be half a schedule with " +
	"nothing saying which half. caveats.html lists its caveats but does not render it"

const fundFlowsNoSpineColumn = "a published column of pp.127-140, pp.167-170 and pp.173-183 with no " +
	"spine year to stitch it into: the expansion schedule is stitched per spine column and " +
	"pp.66-67 print no actual or revised column. caveats.html lists its caveats but does not render " +
	"it. Reaching it means a spine-less way into fund-flows, and FY2023-24 also carries a " +
	"seventh fund group, permanent, which the client draws muted rather than dropping " +
	"(fisc-zojk)"

// assertPublishedReachable refuses a built published document no view reaches
// (per [export.View.ReachedStems] over ix) unless unviewedDocuments declares
// it.
func assertPublishedReachable(vs []export.View, built map[string][]byte, ix export.ColumnIndex) error {
	reachable := make(map[string]struct{}, len(vs))
	for _, v := range vs {
		stems, err := v.ReachedStems(ix)
		if err != nil {
			return err
		}
		for _, stem := range stems {
			reachable[stem] = struct{}{}
		}
	}
	for _, d := range project.PublishedDocuments() {
		// Unbuilt is assertPublishedBuilt's finding.
		if _, ok := built[d.Stem]; !ok {
			continue
		}
		_, drawn := reachable[d.Stem]
		reason, declared := unviewedDocuments[d.Stem]
		switch {
		case drawn && declared:
			return fmt.Errorf(
				"the site publishes %s and a view now renders it, while unviewedDocuments "+
					"still declares it unrendered (%q); delete that entry", d, reason)
		case !drawn && !declared:
			return fmt.Errorf(
				"the site publishes %s and no view renders it, so it ships as bytes no "+
					"reader can open; give it a view or declare it in unviewedDocuments "+
					"with the bead that will", d)
		}
	}

	// A leftover entry naming no published document would be silent.
	published := make(map[string]struct{}, len(unviewedDocuments))
	for _, d := range project.PublishedDocuments() {
		published[d.Stem] = struct{}{}
	}
	for stem := range unviewedDocuments {
		if _, ok := published[stem]; !ok {
			return fmt.Errorf(
				"unviewedDocuments declares %q unrendered and the site publishes no such "+
					"document; delete that entry or correct its stem", stem)
		}
	}
	return nil
}

// opensInto reports whether the opening document was built and the drawn
// schedule's projection built one for the same column. It matches on Column,
// not declared order: fund-flows' bare stem is third in PublishedDocuments.
func opensInto(opening, drawn string, projections map[string][]byte) bool {
	if _, ok := projections[opening]; !ok {
		return false
	}
	for _, stem := range publishedInto(opening, drawn) {
		if _, ok := projections[stem]; ok {
			return true
		}
	}
	return false
}

// publishedInto is every stem PublishedDocuments declares of the drawn
// projection over the opening document's column, built or not. It joins on the
// declaration, not export.ColumnIndex, which knows only what was built and so
// cannot tell unbuilt from never published.
func publishedInto(opening, drawn string) []string {
	var out []string
	docs := project.PublishedDocuments()
	for _, d := range docs {
		if d.Stem != opening {
			continue
		}
		for _, e := range docs {
			if e.Projection == drawn && slices.Equal(d.Columns, e.Columns) {
				out = append(out, e.Stem)
			}
		}
	}
	return out
}
