package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/build"
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// factsPath is the fact store this export reads. It is cmdutil's rather than
// this package's: `fisc build` writes that path and `fisc verify` checks it, and
// a site built from a different file than either would still export cleanly.
const factsPath = cmdutil.FactsPath

// buildAll is the default Builder: everything the site ships.
//
// It is a thin seam on purpose. buildProjections keeps its own signature and
// its own tests -- it answers "what does the projection pipeline produce",
// which is a question worth asking without an export around it -- and this
// function answers the wider one the command actually needs. The split is also
// what keeps the ~10 direct callers of buildProjections in the test suite
// unchanged.
func buildAll(repoRoot string) (result, error) {
	// ONE READ OF THE STORE, used twice: the projections are built from the
	// decoded facts and the shards are compared against the raw bytes. Reading
	// it twice would make that comparison a claim about two files that happen
	// to have the same name.
	raw, facts, err := readFactStore(repoRoot)
	if err != nil {
		return result{}, err
	}
	projections, err := buildProjectionsFrom(repoRoot, facts)
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
	}, nil
}

// buildProjections is the projection half of buildAll: read the committed fact store and
// run every registered projection over it.
//
// It reads facts/facts.jsonl rather than re-running the mapping engine, and
// that is the point rather than a shortcut. facts.jsonl is the audit trail; a
// site built from anything else could disagree with the file the provenance
// claims it came from. `fisc build` regenerates it byte-deterministically, so
// the two can be compared in CI (fisc-1wr.6).
func buildProjections(repoRoot string) (map[string][]byte, error) {
	_, facts, err := readFactStore(repoRoot)
	if err != nil {
		return nil, err
	}
	return buildProjectionsFrom(repoRoot, facts)
}

// readFactStore reads facts/facts.jsonl BOTH ways: the raw bytes as committed
// and the decoded facts.
//
// Both, from one read, because the two are compared. buildFactAssets re-encodes
// the facts and asserts the result is the raw bytes back, and that assertion is
// worth nothing if the two came from different reads of a file something could
// have changed in between.
func readFactStore(repoRoot string) ([]byte, []fact.Fact, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(factsPath))
	// #nosec G304 -- the path is the repository root fisc found by walking up
	// from the working directory, joined to a constant; it is not user input.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, cmdutil.Hintf(fmt.Errorf("read the fact store: %w", err),
			"run `fisc build` to generate %s", factsPath)
	}
	facts, err := fact.Read(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", factsPath, err)
	}
	return raw, facts, nil
}

// buildProjectionsFrom is the pipeline itself, over facts already read. It
// still takes repoRoot because the label registry is loaded from data/ here, in
// the composition root, and handed to internal/project.
func buildProjectionsFrom(repoRoot string, facts []fact.Fact) (map[string][]byte, error) {
	// The label registry is loaded here, in the composition root, and passed
	// in: internal/project is deliberately decoupled from internal/registry
	// and reaches it through an interface it declares itself.
	reg, err := loadRegistry(repoRoot)
	if err != nil {
		return nil, err
	}

	// The slices are internal/project's declaration, not this command's: `fisc
	// verify` checks the same slices, and two copies would let it pass a graph
	// this command does not publish.
	//
	// THE LOOP IS OVER PROJECTIONS, NOT OVER YEARS. It used to be the cartesian
	// product of PublishedFiscalYears() and Registry(), which was right while
	// every projection was one year of the citywide spine and hard-errors on the
	// first projection that is not: a trends document is of one slice spanning
	// four columns of a different schedule, and the old slicesContain refusal
	// rejected it before a single file was written (fisc-neh). fisc-744 fixed
	// this same shape in internal/check and left this copy behind, which is why
	// the failure was still waiting here.
	version := build.Get().String()
	out := map[string][]byte{}
	builtAt := map[string]builtDoc{}
	for _, p := range project.Registry(reg) {
		declared := slicesOf(p, facts, version)
		for _, o := range declared {
			stem, err := stemFor(p.Name(), o, declared)
			if err != nil {
				return nil, err
			}
			// Two documents landing on one stem would write one file and drop
			// the other in silence, and a reader would have no way to tell which
			// of the two they were looking at.
			if prev, ok := builtAt[stem]; ok {
				// NAMING BOTH PROJECTIONS, because builtAt spans the whole
				// registry loop and the two documents need not come from one
				// projection. While this said "the %s projection wants two
				// documents" it named only the CURRENT one, so a collision
				// between, say, the spine and a future one-slice projection read
				// as a fault in whichever happened to be second.
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

	// THE PUBLISHED SET IS STILL ASSERTED, and it is the half of the old
	// cartesian product worth keeping. `fisc export` and `fisc verify` share one
	// declaration of which slices the site publishes; what this catches is the
	// two of them diverging -- a projection whose declared slices no longer
	// include a published one would otherwise be checked under its own slices
	// and published under one it says it is not of.
	//
	// It fails closed and names both sides, because the fix is never obvious
	// from a missing file: either the projection wants slices this command
	// cannot publish, or the published set moved and the projection was not
	// told. What it no longer does is demand that EVERY projection be of the
	// published spine slice, which is the part that refused a second schedule.
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

// assertPublishedBuilt is the export side of published-projection-built.
//
// It is a function of the stems built rather than inline in the loop above so a
// test can hand it a published set that was not built, which is the state the
// real repository is never in and the only one worth asserting about.
//
// THE STEM EXISTING IS NOT THE WHOLE ASSERTION. A document built over fewer
// columns than the site publishes it over lands at the right path and is the
// wrong file: revenue-trends.json carrying three of its four printed columns is
// a chart a reader cannot tell from a complete one, and every check downstream
// compares each series against the columns the DOCUMENT declares, so it agrees
// with itself. project.MissingColumns is the comparison, shared with
// internal/check rather than spelled twice.
func assertPublishedBuilt(builtAt map[string]builtDoc) error {
	for _, d := range project.PublishedDocuments() {
		b, ok := builtAt[d.Stem]
		if !ok {
			return fmt.Errorf(
				"the site publishes %s and no projection built it; the documents "+
					"built were: %s", d, joinComma(builtStems(builtAt)))
		}
		if missing := project.MissingColumns(d, b.opts); len(missing) > 0 {
			return fmt.Errorf(
				"the site publishes %s and the document built at that stem covers %s, "+
					"missing %s", d, project.Describe(b.opts.Columns), project.Describe(missing))
		}
	}
	return nil
}

// builtStems is the stems built, in order, for a refusal that has to say what
// it did build.
func builtStems(builtAt map[string]builtDoc) []string {
	out := make([]string, 0, len(builtAt))
	for stem := range builtAt {
		out = append(out, stem)
	}
	sort.Strings(out)
	return out
}

// slicesOf is the slices one projection is built over.
//
// A projection that says which slices it is of is asked; one that does not is
// built over the published spine slices, one per published fiscal year, which is
// what every projection got before project.Sliced existed.
func slicesOf(p project.Projection, facts []fact.Fact, version string) []project.Options {
	if sl, ok := p.(project.Sliced); ok {
		return sl.Slices(facts, version)
	}
	years := project.PublishedFiscalYears()
	out := make([]project.Options, 0, len(years))
	for _, year := range years {
		out = append(out, project.Options{
			Columns: []project.Column{{FiscalYear: year, Basis: project.PublishedBasis}},
			Scopes:  []string{project.PublishedScope},
			Version: version,
		})
	}
	return out
}

// builtDoc is a document that has claimed a stem, and which projection claimed
// it. The name is carried so a collision can name BOTH sides.
type builtDoc struct {
	name string
	opts project.Options
}

// stemFor is [project.Stem], and it is a one-line delegation on purpose.
//
// THE RULE USED TO LIVE HERE, and fisc-rmx is what moved it. A stem is a PATH,
// and three commands need to agree about paths: this one writes the files,
// `fisc verify` says one of them was not built, and project.PublishedDocuments
// declares what the site serves. While the rule was spelled here and
// approximated there, it was reachable for the two to disagree -- and they did,
// on any spine carrying two bases for one fiscal year.
//
// The wrapper survives rather than the call sites being rewritten because the
// error message wants this command's words, and because export_test.go's cases
// are the ones that pin both directions of the naming rule.
func stemFor(name string, o project.Options, declared []project.Options) (string, error) {
	return project.Stem(name, o, declared)
}

// joinComma renders a list the way a message should. It is spelled here rather
// than reached for from internal/check, which has its own: a command and a check
// package sharing a formatting helper would couple them for nothing.
func joinComma(s []string) string { return strings.Join(s, ", ") }

// sourceRegistry is as much of data/sources.yaml as the site needs. The file
// is the only place a URL is asserted, so the page must read its citation
// targets out of it rather than composing them from a document id.
type sourceRegistry struct {
	Sources []struct {
		ID        string `yaml:"id"`
		Title     string `yaml:"title"`
		Publisher string `yaml:"publisher"`
		// URL is the stable canonical address. url_versioned carries the
		// CMS cache-buster and is deliberately not published: it changes
		// whenever the city re-uploads, which would rot every citation.
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

// yearStems is the built document stems for one projection, in the order a
// reader should meet them. It reads the stems project.PublishedDocuments
// declares rather than rebuilding them from a year, and lists only those built,
// so the year control never offers a 404.
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
// with its reason. The build gates never ask whether a reader can reach a
// document, so this declaration is what makes an unreachable one a decision;
// unviewedDocuments are documents the site publishes and no page renders, each
// with its reason. The build gates never ask whether a reader can reach a
// document, so this declaration is what makes an unreachable one a decision;
// assertPublishedReachable refuses an entry a view renders or that names no
// published document. Each entry is a column pp.66-67 print no year for, so no
// spine year opens into it.
var unviewedDocuments = map[string]string{
	project.FundFlowsProjection + "-2024-actual":  fundFlowsNoSpineColumn,
	project.FundFlowsProjection + "-2025-revised": fundFlowsNoSpineColumn,

	project.DepartmentSpendingProjection + "-2024-actual":  spendingNoSpineColumn,
	project.DepartmentSpendingProjection + "-2025-revised": spendingNoSpineColumn,

	project.DepartmentFundingProjection + "-2024-actual":  fundingNoSpineColumn,
	project.DepartmentFundingProjection + "-2025-revised": fundingNoSpineColumn,
}

const fundingNoSpineColumn = "a published column of pp.85-125's Department Funding Sources " +
	"with no spine year to open it from: the fund-departments step joins on Column and " +
	"pp.66-67 print no actual or revised column. It is published because this is the only " +
	"document drawing that block, and two of the four columns would be half a schedule with " +
	"nothing saying which half. caveats.html lists its caveats but does not render it"

const spendingNoSpineColumn = "a published column of the departmentwide cross-tab with no " +
	"spine year to open it from: the object-category step joins on Column and pp.66-67 " +
	"print no actual or revised column. It is published because this is the only document " +
	"drawing pp.85-125, and two of the four columns would be half a schedule with nothing " +
	"saying which half. caveats.html lists its caveats but does not render it"

const fundFlowsNoSpineColumn = "a published column of the General Fund drill-down with no " +
	"spine year to open it from: index.html opens fund-flows joining on Column and pp.66-67 " +
	"print no actual or revised column. caveats.html lists its caveats but does not render " +
	"it. Reaching it means a spine-less way into fund-flows, and FY2023-24 also carries a " +
	"seventh fund group, permanent, which the client draws muted rather than dropping " +
	"(fisc-zojk)"

// assertPublishedReachable refuses a built published document no view can
// reach unless unviewedDocuments declares it. A document is reached through a
// view's projection, its year stems, or a step's drawn stems: the spine's second
// year only through the year control, and fund-flows only by opening a node.
func assertPublishedReachable(vs []export.View, built map[string][]byte) error {
	_, ix, err := export.ColumnsOf(built, "")
	if err != nil {
		return err
	}
	reachable := make(map[string]struct{}, len(vs))
	for _, v := range vs {
		reachable[v.Projection] = struct{}{}
		for _, stem := range v.YearStems {
			reachable[stem] = struct{}{}
		}
		for _, stem := range v.DrawnStems(ix) {
			reachable[stem] = struct{}{}
		}
	}
	for _, d := range project.PublishedDocuments() {
		// Unbuilt is assertPublishedBuilt's finding, and a caller's own Builder
		// may build only some documents.
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

	// An entry naming no published document is stale too. A mistyped one is
	// caught above; a leftover one would be silent.
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

// spendingGaps is project.SpendingGaps as the step declares it.
func spendingGaps() (map[string]export.Gaps, error) {
	declared, err := project.SpendingGaps()
	if err != nil {
		return nil, err
	}
	out := map[string]export.Gaps{}
	for id, gaps := range declared {
		for _, g := range gaps {
			out[id] = append(out[id], export.Gap(g))
		}
	}
	return out, nil
}

// stepByKey is the declared step with this key, and whether one was declared.
// Steps are looked up by key, never index: the blocks append conditionally.
func stepByKey(steps []export.DrillStep, key string) (export.DrillStep, bool) {
	for _, s := range steps {
		if s.Key == key {
			return s, true
		}
	}
	return export.DrillStep{}, false
}

// opensInto reports whether the opening document was built and the step's
// projection built one for the same column, which decides whether the step is
// declared at all. It matches on Column, not declared order: fund-flows' bare
// stem is third in PublishedDocuments. Which document each year draws is
// export.ColumnIndex's question.
func opensInto(opening, step string, projections map[string][]byte) bool {
	if _, ok := projections[opening]; !ok {
		return false
	}
	docs := project.PublishedDocuments()
	for _, d := range docs {
		if d.Stem != opening {
			continue
		}
		for _, e := range docs {
			if e.Projection != step || !slices.Equal(d.Columns, e.Columns) {
				continue
			}
			if _, ok := projections[e.Stem]; ok {
				return true
			}
		}
	}
	return false
}

// views is the site's pages, in nav order, the page it opens on first. It lives
// in the composition root because naming a view means knowing what a projection
// is of, which internal/export must not.
//
// A view whose document was not built is dropped rather than refused: `fisc
// verify` already fails a missing published document, and a nav entry to an
// unwritten page is what must never ship.
func views(built result) ([]export.View, error) {
	residual, err := project.FundFlowsResidual()
	if err != nil {
		return nil, err
	}
	gaps, err := spendingGaps()
	if err != nil {
		return nil, err
	}
	projections := built.Projections
	spine := export.View{
		Path:       export.IndexPath,
		Nav:        "Budget flows",
		Template:   export.SankeyTemplate,
		Projection: export.PrimaryProjection,
		YearStems:  yearStems(export.PrimaryProjection, projections),
		// Declared rather than left to d3's inference so a step's kept flank has a
		// column to be adjacent to. On both spine goldens the layout is identical
		// either way, which the client's layout test pins.
		RenderTiers: []int{0, 2, 5},
	}
	// The spine opens into fund-flows, and fund-flows into itself (fisc-ko1j).
	//
	// Every tier set and cap below is measured against the committed goldens with
	// the shipped d3-sankey at app.js's constants; the client's tests pin the
	// figures. Caps are per tier: one cap on the finest tier leaves
	// special-revenue's fund column undrawable.
	//
	// Steps are declared only when the opening year's document was built, since a
	// step to an unwritten file is a click that 404s; a second year missing its
	// document is refused by View.validate.
	if opensInto(export.PrimaryProjection, project.FundFlowsProjection, projections) {
		spine.Steps = []export.DrillStep{
			{
				// After the spine's chart only: a group kept on a revenue- or object-category
				// window is drawn at its share of that centre while this step draws its whole
				// decomposition, leaving node height with no ribbon. validateSteps refuses it.
				Key:        "fund-group",
				After:      []string{""},
				From:       2,
				Projection: project.FundFlowsProjection,
				// [revenue categories | this group | its funds]: the spine draws tier 0 left
				// of tier 2, so the window pushes right.
				Keep: []int{0},
				// Two more columns where there is room: the divisions that spend each fund,
				// and the object categories they spend on. Only the General Fund fills tier
				// 4 (pp.167-170), so other groups' windows draw nothing there and the
				// client drops the column (fisc-84y5); every group fills tier 5, the General
				// Fund through its divisions and every other fund from pp.173-183.
				Tiers: []int{0, 2, 3, 4, 5},
				Widen: []int{4, 5},
				// Special-revenue's 31-32 funds fold to 8 with no sub-pixel ribbon; uncapped,
				// up to 9 of its ribbons lie under a pixel. The tier-4 cap is the fund step's,
				// inert against fund/100's 23 divisions, and the tier-5 cap is the fund
				// step's too.
				// Tier 5 is a fund's or a division's own object rows, not the
				// four categories, so its tail counts rows.
				Caps: []export.TierCap{{Tier: 3, Cap: 8}, {Tier: 4, Cap: 24, Tail: "divisions"},
					{Tier: 5, Cap: 8, Tail: "object rows"}},
				Noun: "fund group",
				// pp.127-140 print no fund-balance row, so a draw the spine sends into a
				// group reaches no fund here.
				ResidualGrain: "fund",
				Back:          "All fund groups",
				Tail:          "funds",
				// project.FundFlowsResidual is derived from the cuts and exceptions
				// internal/structure declares, shipped rather than respelled so the
				// client's set is one thing. Each such ribbon is re-pointed past the group
				// onto one derived node, so the group takes in exactly what its funds take
				// in.
				Residual: residual,
				// The client's tests pin these figures per column: fund/100 is about half the
				// fund column, and the smallest fund under 1/30,000 of it, in both years.
				Description: "The revenue categories on the left are the citywide chart's " +
					"own cells; this fund group is the mark in the middle, and its own funds " +
					"are on the right, rescaled to the group's total — the citywide " +
					"chart cannot show them, because the General Fund alone is half the " +
					"fund column and the smallest fund is less than a thirty-thousandth " +
					"of it. Money Budget Book pp.127-140 print for no fund at all passes " +
					"the group's mark to a node of its own beside the funds, so what the " +
					"group takes in here is what its funds take in. Every fund a " +
					"department draws on opens further: the General Fund into the " +
					"divisions that spend it, from Budget Book pp.167-170, and every " +
					"other fund into the departments it pays for, from pp.85-125. Any " +
					"other fund ends the drill. Where " +
					"there is room for more columns, the General Fund's divisions " +
					"from pp.167-170 are drawn beyond its funds, and beyond them the " +
					"object categories each fund spends on: the General Fund's through " +
					"its divisions, every other fund's straight from pp.173-183, which " +
					"print no division.",
			},
			{
				// Role general_fund: pp.167-170 decompose fund 100 alone, so no other fund
				// offers a click this step cannot answer.
				Key:   "fund",
				After: []string{"fund-group"},
				From:  3,
				Role:  project.RoleGeneralFund,
				// [the group | this fund | the divisions that spend it].
				Keep: []int{2},
				// Tier 5 is the object categories each division spends on, drawn at the end
				// away from the kept flank. Tiers is where columns are drawn, Widen which a
				// narrow client drops first; validateSteps refuses either alone.
				Tiers: []int{2, 3, 4, 5},
				Widen: []int{5},
				// The division cap is inert (23 divisions) and pinned so. The tier-5 cap makes
				// the widened column drawable: uncapped, its 44 cells leave 11 nodes of no
				// height.
				Caps: []export.TierCap{{Tier: 4, Cap: 24}, {Tier: 5, Cap: 8, Tail: "object rows"}},
				Noun: "fund",
				Back: "All funds",
				Tail: "divisions",
				// The difference has two terms and the sentence names both: what leaves the
				// group other than through its divisions, less the residual carried in above.
				// TestTheFundStepsSentenceIsItsArithmetic holds the words to that identity.
				Description: "The fund group this fund belongs to is on the left and the " +
					"divisions that spend it are on the right — that fund's rows of " +
					"Budget Book pp.167-170, rescaled to its total. The two sides of the " +
					"fund in the middle are not one figure: what it takes in is its revenue " +
					"and what leaves it here is what its divisions spend. The difference is " +
					"what pp.66-67 print for the fund group as a whole — the money the city " +
					"transfers out and sets aside in its balances and reserves — less the " +
					"money the group takes in that no fund receives, which the chart above " +
					"carries to a node of its own beside the funds.",
			},
			{
				Key:   "division",
				After: []string{"fund"},
				From:  4,
				// [the fund | this division | what it spends on].
				Keep:  []int{3},
				Tiers: []int{3, 4, 5},
				Caps:  []export.TierCap{{Tier: 5, Cap: 8}},
				Noun:  "division",
				Back:  "All divisions",
				Tail:  "categories",
				Description: "The fund that pays for this division is on the left and the " +
					"object categories it spends on are on the right — that division's " +
					"cells of Budget Book pp.167-170, rescaled to its total.",
			},
			{
				// A second edge out of the spine's chart, told apart from fund-group by
				// From. The role closes transfers/in and fund-balance/draw, which share tier 0
				// and open into nothing pp.127-140 print.
				Key:        "revenue-category",
				After:      []string{""},
				From:       0,
				Role:       project.RoleRevenueSource,
				Projection: project.FundFlowsProjection,
				// A window centred on the category: its pp.127-140 lines on one side, the fund
				// groups it reaches on the other. The spine draws tier 2 right of tier 0, so
				// the window pushes left.
				Keep:  []int{2},
				Tiers: []int{1, 0, 2},
				// Five of the ten categories print more than nine lines, so the line cap
				// engages; no category reaches more than five of the six fund groups.
				Caps: []export.TierCap{{Tier: 1, Cap: 8}},
				Noun: "revenue category",
				Back: "All revenue categories",
				Tail: "lines",
				Description: "The lines Budget Book pp.127-140 print under this revenue " +
					"category are on the left; the fund groups its money reaches are on the " +
					"right, as the citywide chart draws them. The category itself is the mark " +
					"in the middle, and the two sides of it are the same figure read from two " +
					"schedules. A line the schedule prints as a reduction is drawn in red at " +
					"its printed size and named as one, and the category's own mark is the " +
					"figure net of them \u2014 the same one the citywide chart labels it with.",
			},
		}
	}
	// The spine's right-hand column opens into department-spending, guarded apart
	// from fund-flows so a corpus with pp.85-125 and not pp.127-140 keeps it.
	//
	// Role object_category closes transfers/out and the two fund-balance rows,
	// which pp.85-125 do not decompose. The cap engages: services-and-supplies
	// reaches 29 divisions. The gap is project.SpendingGaps, read not copied:
	// FY2026-27's services-and-supplies falls 250,000 short of p0067's figure
	// (fisc-av0w), drawn as a mark of its own.
	if opensInto(export.PrimaryProjection, project.DepartmentSpendingProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "object-category",
				After:      []string{""},
				From:       5,
				Role:       project.RoleObjectCategory,
				Projection: project.DepartmentSpendingProjection,
				Keep:       []int{2},
				Tiers:      []int{2, 5, 4},
				Caps:       []export.TierCap{{Tier: 4, Cap: 8}},
				Noun:       "object category",
				Back:       "All object categories",
				Tail:       "divisions",
				Gaps:       gaps,
				Description: "The fund groups that pay for this object category are on the " +
					"left; the divisions that spend it are on the right \u2014 Budget Book " +
					"pp.85-125's rows for this category, every division in the city that " +
					"has one, rescaled to the category's total. The two columns are read " +
					"from different schedules, and the right-hand one prints what a " +
					"division spends whatever pays for it: it carries no fund at all, so " +
					"no division here takes the colour of a fund group.",
			},
		}...)
	}
	// The spine's transfers/in opens into p76, keeping no flank: the column beside
	// tier 0 on the spine is tier 2, which this step draws itself, and
	// validateSteps refuses one tier at two columns.
	//
	// It opens a source, so Side is declared and the client filters from the node;
	// filtering to a source yields an empty graph d3-sankey dies on. Role
	// transfer_in partitions tier 0 with revenue-category's revenue_source. Only
	// p76's receiving legs are drawn; transfers-out draws the paying side. No cap
	// and no gap: 8 payers and 9 receivers, and the legs sum to the spine's
	// transfers/in to the cent.
	if opensInto(export.PrimaryProjection, project.TransfersByFundProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "transfers",
				After:      []string{""},
				From:       0,
				Side:       export.SideSource,
				Role:       project.RoleTransferIn,
				Projection: project.TransfersByFundProjection,
				Tiers:      []int{2, 3},
				Noun:       "money coming in",
				Back:       "All money coming in",
				Tail:       "funds",
				Description: "Budget Book p76, Summary of Transfers: the funds that pay each " +
					"transfer the city makes to itself are on the left, and the funds that " +
					"receive them are on the right. One ribbon is one figure the page prints, " +
					"and a fund that both pays and receives is drawn once on each side, under " +
					"the same name. This is the money coming IN, which is what the mark on " +
					"the citywide chart counts.",
			},
		}...)
	}
	// The spine's transfers/out opens into the transfers-out network: p76's
	// paying legs and p222's transfers to the CIP, which together are the
	// spine's TRANSFER OUT by fund group (structure.BudgetBookSplits). The
	// payers' funds are on the left and each receiver's end on the right, every
	// one of them folded into transfers/out. Role transfer_out partitions tier 5
	// with object-category's object_category. No gap: the legs sum to the
	// spine's figure. Both columns are capped.
	if opensInto(export.PrimaryProjection, project.TransfersOutProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "transfers-out",
				After:      []string{""},
				From:       5,
				Role:       project.RoleTransferOut,
				Projection: project.TransfersOutProjection,
				Tiers:      []int{3, 5},
				Caps:       []export.TierCap{{Tier: 3, Cap: 10}, {Tier: 5, Cap: 10}},
				Noun:       "money going out",
				Back:       "All money going out",
				Tail:       "funds",
				Description: "Budget Book p76 and p222: the funds that pay each transfer the " +
					"city makes are on the left, and the funds that receive them are on the " +
					"right. p76 lists the transfers between operating funds and p222 the " +
					"transfers to the Capital Improvement Program, whose funds are not on the " +
					"citywide chart; the two lists together are the Transfers Out it counts. " +
					"One ribbon is one figure a page prints.",
			},
		}...)
	}
	// Every other fund opens into the departments it pays for, from pp.85-125's
	// lower block: a sibling of the fund step told apart by Role, so fund 100 keeps
	// opening into its divisions. Guarded on the fund-group step, the window it
	// opens from.
	//
	// No cap (the widest fund draws 5 departments), no gap (the two sides are two
	// schedules and not meant to be equal), and no residual (the kept flank is
	// carried verbatim).
	_, openable := stepByKey(spine.Steps, "fund-group")
	if openable && opensInto(export.PrimaryProjection, project.DepartmentFundingProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "fund-departments",
				After:      []string{"fund-group"},
				From:       3,
				Role:       project.RoleFund,
				Projection: project.DepartmentFundingProjection,
				// [the group | this fund | the departments it pays for].
				Keep:  []int{2},
				Tiers: []int{2, 3, 4},
				Noun:  "fund",
				Back:  "All funds",
				Tail:  "departments",
				// NO DIRECTION AND NO ACCOUNT OF THE DIFFERENCE: either side
				// is the larger on some fund in both committed columns.
				Description: "The fund group this fund belongs to is on the left and the " +
					"city departments it pays for are on the right \u2014 that fund's rows " +
					"of Budget Book pp.85-125, rescaled to its total. The two sides of the " +
					"fund in the middle are read from two different schedules and are not " +
					"one figure: what it takes in, from pp.127-140, and what the " +
					"departments draw on it here. Either side may be the larger. " +
					"A department here is the WHOLE department across every fund that " +
					"pays it, which is a coarser thing than the divisions the General Fund " +
					"opens into \u2014 five names belong to both tiers, so do not read one " +
					"as the other.",
			},
		}...)
	}
	out := []export.View{spine}

	// The tables come after the charts they belong to.
	if _, ok := projections[project.TrendsProjection]; ok {
		out = append(out, export.View{
			Path:       "trends.html",
			Nav:        "Revenue tables",
			Template:   export.TrendsTemplate,
			Projection: project.TrendsProjection,
			Title:      "Where Livermore's revenue comes from, fund by fund",
			Lede: "Every revenue line the city prints for each of its funds, across four " +
				"budget columns. The columns are not one measurement: FY 2023-24 is money " +
				"that moved, FY 2024-25 is a mid-year re-forecast, and the two later years " +
				"are intentions adopted together.",
		})
	}
	// THE TWO ACFR TEN-YEAR TABLES, server-rendered like the trends page. The
	// section headings are the blocks the schedules print, and each entry's
	// Kind/FundGroup pair is what the document's own series carry —
	// buildHistoryPage refuses a mismatch in either direction.
	if _, ok := projections[project.ChangesProjection]; ok {
		out = append(out, export.View{
			Path:       "history.html",
			Nav:        "Ten-year history",
			Template:   export.HistoryTemplate,
			Projection: project.ChangesProjection,
			Sections: []export.Section{
				{Heading: "Revenues", Kind: string(mapping.KindRevenue)},
				{Heading: "Expenditures", Kind: string(mapping.KindExpenditure)},
				// CLOSED, unlike the two blocks above, because this heading
				// names one printed row rather than a category: any other
				// fund_balance series with no fund group reaching this
				// projection must be refused, not absorbed under it. The
				// display label is p168's own three wrapped lines joined —
				// the fact's row_label stays the printed tail, and only the
				// Line cell says the whole phrase.
				{Heading: "Excess of revenues over (under) expenditures", Kind: string(mapping.KindFundBalance),
					Rows: map[string]string{
						"over (under) expenditures": "Excess of Revenues over (under) expenditures",
					}},
			},
			Title: "Ten years of Livermore's money, as the city reports it",
			Lede: "What the city's governmental funds actually took in and spent, " +
				"FY2015-16 through FY2024-25, from the ten-year schedules in the " +
				"city's annual financial report — not budgets or intentions, but " +
				"the money that moved.",
		})
	}
	if _, ok := projections[project.FundBalancesProjection]; ok {
		out = append(out, export.View{
			Path:       "balances.html",
			Nav:        "Fund balances",
			Template:   export.HistoryTemplate,
			Projection: project.FundBalancesProjection,
			Sections: []export.Section{
				{Heading: "General Fund", Kind: string(mapping.KindFundBalance), FundGroup: "general"},
				{Heading: "All Other Governmental Funds", Kind: string(mapping.KindFundBalance)},
			},
			Title: "What Livermore's funds held at each year's end",
			Lede: "The balance of the General Fund and of all other governmental " +
				"funds at each June 30, FY2015-16 through FY2024-25, split into the " +
				"GASB 54 categories that say how spendable each dollar is.",
		})
	}
	// The caveats index names no projection and is unconditional: it depends on
	// no single document, and buildCaveatsPage refuses an empty one rather than
	// link a blank page. Appended before provenance so the two indexes end the nav.
	out = append(out, export.View{
		Path:     "caveats.html",
		Nav:      "Caveats",
		Template: export.CaveatsTemplate,
		Title:    "What these figures do not say",
		Lede: "Every page here shows figures the city printed, and every one of them has " +
			"edges: a schedule that stops short, a total the city's own book contradicts, a " +
			"classification that is ours rather than theirs. Those are collected here in " +
			"full, so the pages that draw the money can say them in a line and point at " +
			"this one. Nothing on this page is a hedge; each is a specific thing a " +
			"specific document does not do.",
	})

	// The provenance index names no projection either. It is conditional on
	// Result.PageIndex, for the reason every view is conditional on its document.
	if len(built.PageIndex) > 0 {
		out = append(out, export.View{
			Path:     "provenance.html",
			Nav:      "Sources and data",
			Template: export.ProvenanceTemplate,
			Title:    "Every figure this site publishes, and the page it came from",
			Lede: "Every figure this site publishes, with the document, page and byte " +
				"offset it was read from — downloadable whole, or one page at a time.",
		})
	}
	return out, nil
}
