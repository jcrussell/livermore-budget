package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/build"
	"github.com/jcrussell/livermore-budget/internal/check"
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// structurePath is where the structure for one fiscal year lands: at the site
// root beside the store's own downloads, and NOT under data/, which is the
// projections' directory and one file per projection by contract.
//
// ONE FILE PER YEAR, AND NO MONOLITH. A year is the one dimension the reader
// already switches on, so a part is the whole of what one column of the site
// reads, and a client that fetches the year on screen fetches nothing it will
// not draw. structure.PartitionByYear says why that is not per-interaction
// slicing, and TestTheStructureShipsOnTheAssetChannelAndIsMeasured re-measures
// each part against the whole under -v.
func structurePath(year int) string {
	return fmt.Sprintf("structure-%d.json", year)
}

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
	reg, err := loadRegistry(repoRoot)
	if err != nil {
		return result{}, err
	}
	parts, err := buildStructure(reg, facts)
	if err != nil {
		return result{}, err
	}
	for path, b := range parts {
		assets.Files[path] = b
	}
	// THE RUNG ANSWER SHIPS TOO. Go walks every rung of the spine to build
	// it, and until it shipped the walk's only reader was
	// the test that pinned testdata/rungs.json to it; the client drew each
	// rung from a derivation of its own instead. A projections-only result
	// is enough to name the spine, which is all spineView needs.
	spine, err := spineView(result{Projections: projections})
	if err != nil {
		return result{}, err
	}
	rungs, err := rungsOf(projections, spine)
	if err != nil {
		return result{}, err
	}
	served, err := encodeRungs(rungs)
	if err != nil {
		return result{}, err
	}
	assets.Files[rungsServedPath] = served
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

// buildStructure is the structure over the store as the site ships it: one
// document per fiscal year, keyed by structurePath, each the whole's
// scaffolding with that year's facts and the views that admit any of them.
// Compact JSON, as every projection is, so its size is comparable with theirs.
func buildStructure(reg *registry.Registry, facts []fact.Fact) (map[string][]byte, error) {
	doc, err := structureOf(reg, facts)
	if err != nil {
		return nil, err
	}
	parts, err := structure.PartitionByYear(doc)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(parts))
	for _, p := range parts {
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		out[structurePath(p.FiscalYear)] = b
	}
	return out, nil
}

// structureOf is the unpartitioned structure over the store: every summing
// document's view, each fact those views admit once with its provenance. It
// is what buildStructure partitions and what a test holds the parts to.
func structureOf(reg *registry.Registry, facts []fact.Fact) (structure.Document, error) {
	return structure.Build(facts, documentViews(reg, facts, build.Get().String()))
}

// documentViews is every document the pipeline builds that sums, named with
// the scope set it is of: the views the structure carries.
//
// A SERIES PROJECTION PUBLISHES NO TOTAL AND IS NOT A VIEW, and it is told
// apart the way internal/check tells it apart: by the method that builds a
// series, not by whether its scopes name a cut. A view whose scopes name no
// cut is refused by structure.ViewOf, which is the direction a dropped cut
// should fail in.
//
// ONE VIEW PER PROJECTION. A projection declaring two scope sets across its
// slices would be one name for two views, and structure.Build refuses the
// name it sees twice rather than this function picking one.
func documentViews(reg *registry.Registry, facts []fact.Fact, version string) []structure.Scoped {
	var out []structure.Scoped
	for _, p := range project.Registry(reg) {
		if _, series := p.(interface {
			Document(facts []fact.Fact, o project.Options) (*project.TrendsDocument, error)
		}); series {
			continue
		}
		seen := map[string]bool{}
		for _, o := range slicesOf(p, facts, version) {
			if seen[o.ScopeList()] {
				continue
			}
			seen[o.ScopeList()] = true
			out = append(out, structure.Scoped{Name: p.Name(), Scopes: o.Scopes})
		}
	}
	return out
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

// yearStems is the document stems for one projection, in the order a reader
// should meet them.
//
// IT READS THE DECLARED STEMS RATHER THAN REBUILDING THEM, and that is fisc-rmx
// note (3). It used to walk PublishedFiscalYears() and recompute the stem from
// each year, which was a fourth spelling of the naming rule and was the one that
// bit: a stem that stemFor produced and this function could not reconstruct FROM
// A YEAR ALONE would be written to disk and offered to no reader, so the toggle
// would silently lose a document rather than fail. project.PublishedDocuments
// states each stem, so there is nothing left to reconstruct.
//
// It lists only stems a document was actually built for. The published list is
// what the site MEANS to publish; built is what it HAS, and the year control
// must name the second or it offers the reader a 404. The two agree whenever
// buildProjections wrote them -- it loops the same list -- and disagree when the
// caller supplied its own Builder, which the Options.Build seam exists to allow.
//
// A published year that produced no document is not silently dropped from the
// world by this: `fisc verify` fails published-projection-built for it, which is
// the check that exists to notice a year the site publishes and nothing looked
// at.
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

// unviewedDocuments are documents the site PUBLISHES and no page RENDERS, each
// with the reason and the bead that will give it one.
//
// AN ENTRY IS A DECLARATION, NOT A NOTE, and it exists because the gap it
// records shipped silently. `fisc export` wrote four fund-flows documents into
// data/, published every one of them in window.FISC_CONFIG.projections, and
// gave none of them a page. published-projection-built and assertPublishedBuilt
// both passed: they assert a published document was BUILT and neither asks
// whether a reader can reach it. `fisc verify` was green at 38 checks with four
// documents shipping as bytes nobody could open, and the test that should have
// caught it -- TestViewsNamesEveryDocumentTheSitePublishes -- asserted a view
// count of two over seven projections.
//
// AN ENTRY THAT HAS STOPPED BEING TRUE MUST GO RED, NOT QUIET, which is the
// same standard internal/check's staleDeclarations applies to
// unprojectedScopes: the moment a view names one of these stems, the entry is a
// false statement about the site, so assertPublishedReachable refuses it and
// the entry is deleted rather than left for whoever forgets.
//
// WHY THE FUND-FLOWS PAIR IS NOT A CHART PROBLEM ANY MORE. It was: the
// drill-down's 61-node fund column laid every node and every ribbon out at zero
// height, and c3a337d landed the fold that fixes it. index.html opens the spine
// into fund-flows now, joining the two documents on Column -- see opensInto --
// and that join is what those two cannot satisfy: see the const. Every entry
// left here is a printed column pp.66-67 have no year for, on one projection or
// the other.
var unviewedDocuments = map[string]string{
	project.FundFlowsProjection + "-2024-actual":  fundFlowsNoSpineColumn,
	project.FundFlowsProjection + "-2025-revised": fundFlowsNoSpineColumn,

	// THE CROSS-TAB'S TWO ADOPTED COLUMNS ARE GONE FROM HERE, and the two
	// historical ones are what is left. The object-category step opens the
	// spine's tier 5 into this projection and joins on Column, so each spine
	// year reaches its own column; the actual and revised columns have no spine
	// year to be opened from, which is the same shape as fund-flows' pair.
	project.DepartmentSpendingProjection + "-2024-actual":  spendingNoSpineColumn,
	project.DepartmentSpendingProjection + "-2025-revised": spendingNoSpineColumn,

	// AND THE FUNDING SOURCES' TWO HISTORICAL COLUMNS, which is the same shape
	// a third time and for the third schedule of the same eleven pages. The
	// fund-departments step opens tier 3 of the fund group's window into this
	// projection and joins on Column, so each spine year reaches its own
	// column and the two the spine does not print reach none.
	project.DepartmentFundingProjection + "-2024-actual":  fundingNoSpineColumn,
	project.DepartmentFundingProjection + "-2025-revised": fundingNoSpineColumn,
}

const fundingNoSpineColumn = "a published column of pp.85-125's Department Funding Sources " +
	"that the chart cannot reach even though its fund-departments step has landed. A fund " +
	"opens into this document one fiscal year at a time, joining on Column, and pp.66-67 " +
	"print no actual and no revised column -- so there is no spine year to open this one " +
	"from. It is published because pp.85-125 DO print those two columns and this is the " +
	"only document that draws that block: drawing two of the four would leave " +
	"internal/check's unprojectedScopes entry half true rather than retired. Those two " +
	"columns also tie to no citywide figure at all, which the document says in a caveat of " +
	"its own. caveats.html lists its caveats, which indexes the document rather than " +
	"rendering it and does not retire this entry"

const spendingNoSpineColumn = "a published column of the departmentwide cross-tab that the " +
	"chart cannot reach even though its object-category step has landed. The spine opens " +
	"into a document one " +
	"fiscal year at a time, joining on Column, and pp.66-67 print no actual and no revised " +
	"column -- so there is no spine year to open this one from. It is published because " +
	"pp.85-125 DO print those two columns and this is the only document that draws those " +
	"pages: drawing two of the four would leave internal/check's unprojectedScopes entry " +
	"half true rather than retired. caveats.html lists its caveats, which indexes the " +
	"document rather than rendering it and does not retire this entry"

const fundFlowsNoSpineColumn = "a published column of the General Fund drill-down that the chart " +
	"cannot reach. index.html opens the spine into fund-flows one fiscal year at a time, " +
	"joining the two documents on Column, and pp.66-67 print no actual and no revised " +
	"column -- so there is no spine year to open this one from. Note that caveats.html " +
	"DOES list its caveats -- it indexes every published document rather than drawing " +
	"one -- which is not the same as rendering it and does not retire this entry. " +
	"Reaching it means a spine-less way into fund-flows, and FY2023-24 also carries a " +
	"seventh fund group, permanent, which the client's palette has no hue for: it is " +
	"drawn and listed in the legend, muted, rather than dropped (fisc-zojk)"

// assertPublishedReachable is the half of the published-document contract that
// assertPublishedBuilt does not make: a document a reader can open.
//
// It takes the views rather than reading them, for assertPublishedBuilt's
// reason -- a test can hand it a set the real repository is never in.
//
// A DOCUMENT IS REACHABLE THROUGH A VIEW'S PROJECTION, THROUGH ITS YEAR STEMS,
// OR THROUGH A STEP'S DOCUMENTS, and all three arms are needed: the spine's
// second year has no view of its own and is reached only from the first
// view's year control, and both fund-flows documents the site draws are
// reached only by opening a node -- fund-flows-2027 by resolving the step's
// schedule in its own year's column, which is what export.View.DrawnStems
// answers and what the caveats page asks the same way.
func assertPublishedReachable(vs []export.View, built map[string][]byte) error {
	_, ix, err := export.ColumnsOf(built)
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
		// A document that was not BUILT is assertPublishedBuilt's finding, not
		// this one -- and views() drops a view whose document is missing on
		// purpose, so every unbuilt document would otherwise be reported here
		// as unreachable too. That matters beyond tidiness: Options.Build is a
		// documented seam for a caller supplying its own builder, and a caller
		// building one document must not be told the other six are unreachable.
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

	// AND AN ENTRY NAMING NO PUBLISHED DOCUMENT IS ITSELF STALE. Without this
	// arm the map is only half-checked: a stem that stops being published
	// leaves its declaration behind, still asserting something about a document
	// the site no longer has, and nothing would ever say so. A MISTYPED entry
	// is already caught -- the real document goes undeclared and the arm above
	// fires -- but a leftover one is silent, which is the shape this whole
	// declaration exists to refuse.
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

// stepByKey is the declared step with this key, and whether one was declared.
//
// A STEP IS NAMED AND NOT COUNTED, which is fisc-7e1g's rule arriving on the
// producing side. Every guarded block here appends, so the index of a step
// declared in an earlier block is a fact about how many blocks ran rather than
// about the step -- and a later block asking "is the fund-group chart there to
// open from" by index would be asking a different question on a corpus that
// lost a schedule.
func stepByKey(steps []export.DrillStep, key string) (export.DrillStep, bool) {
	for _, s := range steps {
		if s.Key == key {
			return s, true
		}
	}
	return export.DrillStep{}, false
}

// opensInto reports whether the step's projection published a document for the
// same column as the spine's opening one -- which decides whether the step is
// DECLARED at all, not which document any year of it draws.
//
// ON COLUMN, NOT ON DECLARED ORDER, and that is what dissolves fisc-zojk's
// first obstacle. yearStems walks PublishedDocuments() in declared order,
// which for fund-flows is 2024-actual, 2025-revised, 2026, 2027 -- so the bare
// stem is THIRD, and a view opening on it through a YearStems list is refused
// by View.validate. A step is not a YearStems list: nothing here asks which
// document comes first, only whether one covers the column the reader lands on.
//
// A BOOLEAN AND NOT A PER-YEAR MAP. Which document each year draws is
// export.ColumnIndex's, derived from the documents' own fiscal year and basis;
// this is the composition root's separate question -- whether the corpus it
// just built can support the rung -- and its answer decides a declaration
// rather than a lookup. The map was both at once, and a year it resolved
// wrongly satisfied every arm that guarded it.
//
// Only built documents on both sides, for yearStems' reason: a declaration
// must be supportable by files the site HAS, not by files it means to have.
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

// views is the site's pages, in nav order, the page it opens on first.
//
// IT LIVES IN THE COMMAND, not in internal/export, and that is what keeps that
// package's stated property true: it "consumes projections as filename stem ->
// JSON bytes and knows nothing about how they were built" and never imports
// internal/project. Naming a view means naming a projection and knowing what it
// is of, which is knowledge only the composition root has.
//
// A view whose document was not built is DROPPED rather than refused, and only
// here. The reason is the asymmetry between the two failures: `fisc verify`
// already fails when a published document is missing (published-projection-built,
// and fisc-w7d for the rest), so a missing document is caught by the gate; while
// refusing to export at all would mean a corpus that lost one schedule could not
// publish the others. What must never happen is a NAV ENTRY pointing at a page
// that was not written, and dropping the view is exactly what prevents that.
func views(built result) []export.View {
	projections := built.Projections
	spine := export.View{
		Path:       export.IndexPath,
		Nav:        "Budget flows",
		Template:   export.SankeyTemplate,
		Projection: export.PrimaryProjection,
		YearStems:  yearStems(export.PrimaryProjection, projections),
		// THE SPINE'S COLUMNS, DECLARED RATHER THAN INFERRED. Both published
		// spine documents carry exactly these three tiers, so the fold this
		// buys is a no-op on the corpus; what it buys is ADJACENCY. A step
		// that keeps one flank of the chart it opens from names a tier next to
		// the one it opens from, and "next to" has no answer on a chart whose
		// columns are d3's own inference -- View.RenderTiers says why, and
		// validateSteps refuses the kept flank without it.
		//
		// MEASURED, BOTH YEARS, BEFORE IT WAS DECLARED. Driven through
		// tools/jscheck's harness over the two committed spine goldens, the
		// drawn document and the laid geometry are identical either way: the
		// same nodes and links in the same order carrying the same values, and
		// every node's depth, x0, y0 and y1 and every ribbon's y0, y1 and
		// width unchanged to the digit. The reason is structural rather than
		// lucky -- the spine is a clean three-layer DAG whose tier 0 is pure
		// source and tier 5 pure sink, and sankeyJustify's own rule puts a
		// link-less sink in the LAST column, which is where indexOf puts tier
		// 5. tools/jscheck/layout.mjs keeps that measurement in the tree.
		RenderTiers: []int{0, 2, 5},
	}
	// THE SPINE OPENS INTO FUND-FLOWS, AND FUND-FLOWS INTO ITSELF: one page
	// where there were three. revenue.html drew fund-flows at {0,2} and opened
	// a group into {0,3}; spending.html drew fund/100 at {3,4} and opened a
	// division into {4,5}; index.html drew the spine and opened nothing. Those
	// were the levels of one chain laid side by side, and a reader following a
	// dollar from Property Taxes to Patrol had to notice three nav entries and
	// know which was which (fisc-ko1j, owner decisions of 2026-09-08).
	//
	// EVERY TIER SET AND EVERY CAP BELOW IS MEASURED, laying the graph out
	// with the shipped vendor/d3-sankey at app.js's own constants against the
	// two committed goldens. tools/jscheck/drill.mjs walks the chain on every
	// run and pins the figures: the depth-1 General Fund at 39 nodes and 37
	// links, of which the residual is one derived node, its four carried
	// endpoints and their four links, with 2 sub-pixel ribbons in FY2025-26
	// and 3 in FY2026-27; special-revenue's 32 funds folded to 8
	// (uncapped, 22 of its 49 ribbons are under a pixel, and the cap is what
	// makes the column drawable rather than the rescaling); the division
	// column's 23 under its cap of 24, so that cap is inert on the corpus and
	// pinned inert; and the worst depth-2 ribbon at 51px.
	//
	// CAPS ARE PER TIER because a single cap on the finest tier leaves
	// special-revenue's fund column uncapped at {0,3,4}: 40 nodes, 22
	// sub-pixel ribbons, 9 zero-height nodes -- undrawable.
	//
	// ONLY THE GENERAL FUND HAS TIER-4 NODES, so the other five groups' charts
	// end at their funds. spending.html said that in its lede; here it is a
	// property of the drawn chart, and step 0's description says it in fewer
	// words for the reader who cannot see the column stop.
	//
	// THE JOIN IS PER YEAR AND ON COLUMN -- see export.ColumnIndex. Both spine years
	// reach their own fund-flows column, which is what makes fund-flows-2027
	// reachable and retired its unviewedDocuments entry; the actual and
	// revised columns have no spine year and stay declared there.
	//
	// STEPS ARE DECLARED ONLY WHEN THE OPENING YEAR'S DOCUMENT WAS BUILT, for
	// the reason a view whose document was not built is dropped: a chain
	// pointing at a file that was not written is a click that 404s. A second
	// year missing its document is not dropped but REFUSED, by View.validate
	// naming the year -- a site that built one year's drill-down and not the
	// other's is a state assertPublishedBuilt already refuses in the real
	// pipeline, and hiding it under a custom Builder would be the silence
	// unviewedDocuments exists to refuse.
	if opensInto(export.PrimaryProjection, project.FundFlowsProjection, projections) {
		spine.Steps = []export.DrillStep{
			{
				// THE SPINE'S CHART AND NO OTHER, WHICH IS A MEASUREMENT AND
				// NOT A CHOICE. A revenue category's window and an object
				// category's both draw fund groups, and a group kept on one of
				// those flanks is drawn at its share of THAT centre -- the
				// spine's cell for one category into the group -- while this
				// step draws the group's whole decomposition on the other
				// side. Measured over both committed columns with
				// "revenue-category" in this list: the Contributions &
				// Outsourced window keeps fund-group/general at 76,360 and
				// opening it drew 157,873,470 leaving, 157,797,110 of node
				// height with no ribbon under it and nothing on the page
				// saying so. validateSteps refuses that declaration by name
				// now, so this list is held rather than remembered.
				Key:        "fund-group",
				After:      []string{""},
				From:       2,
				Projection: project.FundFlowsProjection,
				// A WINDOW WHOSE KEPT FLANK IS TIER 0, which is the case Keep
				// is a slice for: the spine draws its revenue categories to the
				// LEFT of its fund groups, so they stay the left column here
				// and the window pushes right into the group's own funds.
				// [revenue categories | this group | its funds], the owner's
				// decision of 2026-09-13 over the {0,3,4} chart this replaces,
				// which drew the funds and the divisions and left the group the
				// reader clicked off the screen entirely.
				Keep:  []int{0},
				Tiers: []int{0, 2, 3},
				// ONE CAP WHERE THERE WERE TWO, because the division column is
				// a step further out now. Measured off both committed goldens:
				// special-revenue draws 32 funds in FY2025-26 and 31 in
				// FY2026-27 and folds to 8 either way; capital 11, enterprise 9,
				// internal-service 5, debt-service 3 and general 1. Uncapped,
				// special-revenue's window lays 9 of its 42 ribbons under a
				// pixel in FY2025-26 and 7 of 41 in FY2026-27, with a smallest
				// of 0.12px and 0.05px; capped it draws 19 ribbons and none of
				// them is sub-pixel, in either column.
				Caps: []export.TierCap{{Tier: 3, Cap: 8}},
				Noun: "fund group",
				Back: "All fund groups",
				Tail: "funds",
				// THE RESIDUAL IS THE CHECK'S DECLARATION, READ, NOT COPIED.
				// drill-reconciles-across-documents declares which spine
				// endpoints pp.127-140 and 167-170 cannot decompose, each
				// with its reason, and proves the identity that set closes;
				// the chart carries the same set onto one derived node per
				// opened group. Two spellings of a set that must agree
				// drift, so this is the one place the client's set comes
				// from, and check.ResidualNodes exists for this line.
				//
				// THIS IMPORT IS NOT THE COUPLING joinComma REFUSES. That
				// rule is about a command and a check sharing a helper for
				// nothing; this is the command shipping a declaration the
				// check owns because the check is the only thing that can
				// fail on it. Spelling the five ids here instead would give
				// the site a set nothing verifies.
				//
				// IT IS WHAT MAKES THE CENTRE BALANCE, and that is new at
				// {0,2,3}. The group's own mark is now drawn, so the money
				// pp.127-140 print for no fund has to leave the reader's eye
				// somewhere: each declared endpoint's ribbon is re-pointed
				// past the group onto one derived node beside its funds, and
				// the group then takes in exactly what its funds take in.
				// Measured over both goldens, in dollars: general 1,514,554
				// (a 1,034,154 fund-balance draw and 480,400 of transfers in)
				// and 486,735 in FY2026-27, capital 2,500,213 and 10,129,416,
				// internal-service 6,147,533 and 7,160,645; special-revenue,
				// enterprise and debt-service nothing in either column, and
				// their groups tie to the cent with no mark at all.
				Residual: check.ResidualNodes(),
				// THE FIGURES IN THIS SENTENCE ARE MEASURED off both
				// committed goldens, and tools/jscheck/drill.mjs pins them
				// per column: fund/100 takes 49.18% of the fund column's
				// inflow in FY2026 and 50.79% in FY2027, and the smallest
				// fund -- fund/550 at $5,000, then fund/202 at $3,000 -- is
				// 1/31,575 and 1/54,786 of fund/100's. "Less than a
				// thirty-thousandth" is the bound both columns clear; the
				// exact ratio belongs to the check, not to a sentence that is
				// shown under either year. That gap is the reason a citywide
				// fund column is not drawn and a group is opened instead.
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
					"other fund into the departments it pays for, from pp.85-125. A fund " +
					"no department's funding schedule names ends the drill — not " +
					"missing, but not broken down in any published schedule.",
			},
			{
				// THE ROLE IS THE MIRROR OF THE OTHER TWO GATES, and it is
				// what keeps 60-odd funds from offering a click that cannot be
				// answered. pp.167-170 are the General Fund's schedule and no
				// other fund has a spending side at all, so `general_fund` --
				// which internal/project publishes on fund 100 because 100 IS
				// the General Fund, not because this column happens to
				// decompose it -- opens and `fund` does not. Run, not
				// predicted: without it every drawn fund is drillable and a
				// click on one banners "nothing flows between tiers 3, 4 for
				// node fund/200, so there is no chart to open it into", which
				// is Lane F's finding at the other end of the chart.
				Key:   "fund",
				After: []string{"fund-group"},
				From:  3,
				Role:  "general_fund",
				// [the group | this fund | the divisions that spend it]. The
				// step before it draws tier 2 to the LEFT of tier 3, so the
				// group stays the left column here.
				Keep: []int{2},
				// A FOURTH COLUMN WHERE THERE IS ROOM FOR ONE, and tier 5 is
				// what is there to draw: pp.167-170 print each of fund/100's 23
				// divisions against the object categories it spends on, so the
				// document carries 44 cells at tier 5 under those divisions and
				// one 4->5 link each. Measured over the committed goldens, both
				// columns: 6 nodes and 6 links at tiers {2,3}, 23 and 23 at
				// {3,4}, 44 and 44 at {4,5} -- every adjacent pair of these four
				// columns carries ribbons, which is the condition a sankey band
				// is counted under (tools/jscheck/layout.mjs bands()).
				//
				// BOTH DECLARATIONS, AND THEY SAY DIFFERENT THINGS. Tiers is
				// where the column is drawn -- at the end away from the kept
				// flank, which is what makes the widening's side derivable --
				// and Widen is which of those columns a client with less room
				// does without, in the order it drops them. validateSteps
				// refuses either one alone.
				Tiers: []int{2, 3, 4, 5},
				Widen: []int{5},
				// THE DIVISION CAP HAS NEVER ENGAGED AND IS PINNED INERT: 23
				// divisions against 24 in both committed columns. It is carried
				// at the width it was declared at rather than tightened,
				// because tightening it would fold a column no reader has ever
				// seen folded on the strength of no measurement.
				//
				// THE TIER-5 CAP IS WHAT MAKES THE WIDENED COLUMN DRAWABLE, and
				// it is the fund-group cap's argument one column further out.
				// Measured over the committed goldens at four columns, both
				// years: uncapped, the 44 cells lay out as 44 marks with 11
				// ribbons under a pixel and 11 nodes of no height at all --
				// which is the state that cap's comment calls undrawable.
				// Capped, the column draws 8 cells and a tail, no node is
				// height-less and the 2 ribbons left under a pixel are the
				// division column's own, at the same widths the three-column
				// window draws them.
				//
				// AND IT NAMES ITS OWN NOUN, because the step's counts
				// divisions: the tail read "36 smaller divisions" over a column
				// of object-category cells before this Tail was declared.
				Caps: []export.TierCap{{Tier: 4, Cap: 24}, {Tier: 5, Cap: 8, Tail: "categories"}},
				Noun: "fund",
				Back: "All funds",
				Tail: "divisions",
				// WHAT THIS CENTRE DOES NOT CLAIM, said in the chart's own
				// words because no mark can say it. A fund's revenue and its
				// spending are two schedules and they are not one cell printed
				// twice: what is left over is what the city transfers out and
				// adds to reserves, which pp.66-67 print for the GROUP and no
				// fund-level schedule attributes to a fund. Measured off both
				// goldens: fund/100 takes 157,873,470 and pays 144,650,802 to
				// its divisions in FY2025-26, and 164,358,147 against
				// 149,014,579 in FY2026-27.
				Description: "The fund group this fund belongs to is on the left and the " +
					"divisions that spend it are on the right — that fund's rows of " +
					"Budget Book pp.167-170, rescaled to its total. The two sides of the " +
					"fund in the middle are not one figure: what it takes in is its revenue " +
					"and what leaves it here is what its divisions spend, and the difference " +
					"is the money the city transfers out of the fund and adds to its " +
					"reserves, which pp.66-67 print for the fund group as a whole and no " +
					"published schedule breaks down by fund.",
			},
			{
				Key:   "division",
				After: []string{"fund"},
				From:  4,
				// [the fund | this division | what it spends on]. A window
				// where this drew two columns: the fund the division is paid
				// from stays on screen, which is the level a reader arrived
				// from and the one that says whose money this is.
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
				// A SECOND EDGE OUT OF THE SPINE'S CHART, not a rung of the
				// chain above it: After carries "" like the fund-group step's,
				// and the two are told apart by From. The role is declared
				// because transfers/in and fund-balance/draw share tier 0 with
				// the categories and open into nothing pp.127-140 print.
				Key:        "revenue-category",
				After:      []string{""},
				From:       0,
				Role:       "revenue_source",
				Projection: project.FundFlowsProjection,
				// A WINDOW, AND THE CATEGORY IS ITS CENTRE. The node the
				// reader clicked stays on the screen, in the middle column,
				// with the lines pp.127-140 print under it on one side and the
				// fund groups the spine draws it reaching on the other -- and
				// the two sides of that mark are one figure read from two
				// schedules. Tier 2 is to the RIGHT of tier 0 in the spine's
				// own {0,2,5}, so the kept flank is the last column here and
				// the window pushes left; validateSteps checks that adjacency
				// against the spine's declared order rather than the tier
				// numbers.
				Keep:  []int{2},
				Tiers: []int{1, 0, 2},
				// ONE COLUMN FOLDS NOW, WHERE TWO DID, AND THE CAP TAKES
				// THE STEP'S OWN NOUN. Measured over both goldens: five of
				// the ten categories print more than nine lines --
				// charges-for-services 19, other taxes 15, property taxes and
				// licenses and permits 14 each, miscellaneous 11 -- so the
				// line cap engages exactly where it did before. The fund
				// column it also capped is gone: the right-hand column is the
				// spine's own fund groups, and no category reaches more than
				// five of the six. With one cap left there is nothing for a
				// second noun to count, so Tail is not respelled here.
				Caps: []export.TierCap{{Tier: 1, Cap: 8}},
				Noun: "revenue category",
				Back: "All revenue categories",
				Tail: "lines",
				Description: "The lines Budget Book pp.127-140 print under this revenue " +
					"category are on the left; the fund groups its money reaches are on the " +
					"right, as the citywide chart draws them. The category itself is the mark " +
					"in the middle, and the two sides of it are the same figure read from two " +
					"schedules. A line the schedule prints as a reduction is drawn in red at " +
					"its printed size and named as one, so the category's own mark is the sum " +
					"of every ribbon into it before those reductions.",
			},
		}
	}
	// THE SPINE'S RIGHT-HAND COLUMN OPENS INTO A SECOND DOCUMENT, and it is
	// declared apart from the three above because it is joined to a different
	// projection: a corpus that built pp.85-125 and not pp.127-140 should lose
	// the fund-group drill and keep this one, which one guard over both could
	// not express.
	//
	// A WINDOW, AND THE ONLY ONE THE SITE SHIPS TODAY. Keep 2 with Tiers
	// {2,5,4} draws the fund groups that pay for the category the reader
	// clicked, the category itself, and the divisions that spend it. Tier 2 is
	// to the LEFT of tier 5 in the spine's own {0,2,5}, so the window pushes
	// right and the kept flank is the first column here -- validateSteps checks
	// that adjacency against the spine's declared order rather than against the
	// tier numbers.
	//
	// THE ROLE IS THE MIRROR OF THE REVENUE STEP'S. Three of the spine's seven
	// tier-5 nodes are flow ends rather than object categories -- transfers/out
	// and the two fund-balance rows -- and pp.85-125 decompose none of them:
	// the Transfers Out row those pages print is a dash in both budget columns.
	// Role closes them exactly as "revenue_source" closes transfers/in and
	// fund-balance/draw at the other end of the chart.
	//
	// THE CAP ENGAGES HERE, WHICH THE DIVISION CAP ON THE STEP ABOVE DOES NOT.
	// Measured off both committed columns: services-and-supplies reaches 29
	// divisions and wages-and-benefits 26, against 5 for debt-services and 5
	// (FY2025-26) or 4 (FY2026-27) for capital-outlay. Uncapped, the smallest
	// services ribbon lays out under a pixel; tools/jscheck/drill.mjs pins the
	// fold on all four.
	//
	// THE GAP IS THE CHECK'S DECLARATION, READ, NOT COPIED -- the argument the
	// residual above makes one field over. spending-window-reconciles proves
	// each object category's spine inflow equals pp.85-125's division rows plus
	// the declared gap, and check.SpendingGaps() is that same table keyed by
	// the node the chart draws it at. FY2026-27's services-and-supplies is the
	// one entry: p0067 publishes 130,502,087 where pp.85-125's rows come to
	// 130,252,087, which is fisc-av0w. The chart draws the 250,000 as a mark of
	// its own rather than letting the ribbons fall short of the node.
	if opensInto(export.PrimaryProjection, project.DepartmentSpendingProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "object-category",
				After:      []string{""},
				From:       5,
				Role:       "object_category",
				Projection: project.DepartmentSpendingProjection,
				Keep:       []int{2},
				Tiers:      []int{2, 5, 4},
				Caps:       []export.TierCap{{Tier: 4, Cap: 8}},
				Noun:       "object category",
				Back:       "All object categories",
				Tail:       "divisions",
				Gaps:       check.SpendingGaps(),
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
	// THE SPINE'S TRANSFERS IN OPENS INTO p76, AND THIS IS THE ONLY STEP ON THE
	// SITE THAT IS NOT A WINDOW. The three above keep a flank of the chart the
	// reader came from; this one keeps none, and the reason is positional
	// rather than editorial. A kept flank has to be a column ADJACENT to the
	// opened tier in the chart on screen, and on the spine's own {0,2,5} the
	// column beside tier 0 is tier 2 -- which is the tier this step's own
	// left-hand column draws. Keeping it would name one tier at two columns,
	// which validateSteps refuses by name.
	//
	// IT IS ALSO THE ONLY STEP THAT OPENS A SOURCE. transfers/in is a tier-0
	// node with nothing pointing at it, and filterToNode asked for it answers an
	// empty graph with no error -- the id is known, so the guard on an unknown
	// one does not fire -- and d3-sankey dies on the empty graph with a
	// RangeError. Side says which end opened, so the client picks filterFromNode
	// instead. DECLARED AND NOT INFERRED: "the opened tier is below every tier
	// this step draws, so it must be a source" is true of the columns that exist
	// and says nothing a third document would have to obey.
	//
	// THE ROLE IS WHAT MAKES IT REACHABLE. The revenue-category step also opens
	// tier 0 of this same chart after "", and validateSteps refuses two steps
	// sharing an (After, From, Role). That step names "revenue_source", which is
	// what leaves transfers/in and fund-balance/draw closed; this one names
	// transfers/in's own role, so the two partition the column instead of
	// colliding on it.
	//
	// ONE SIDE OF p76 IS DRAWN AND BOTH ARE PUBLISHED. The document carries a
	// receiving leg and a paying leg for every printed figure; this step's
	// {2,3} draws the receiving legs, whose subtree hangs off transfers/in. The
	// paying legs end at tier 5, which the spine pins transfers/out at, and a
	// node decomposing that would have to be FINER than its own parent -- the
	// tier order forbids it, and fisc-ko1j.12.10 is where the other half goes.
	//
	// NO CAP AND NO GAP. The drawn columns are 8 payer ends and 9 receiving
	// funds in both budget years, which is under any cap worth declaring, and
	// the receiving legs come to p76's printed grand total -- $21,525,997 in
	// FY2025-26 and $21,624,633 in FY2026-27 -- which is the spine's own
	// transfers/in to the cent, so the opened node has nothing to fall short by.
	if opensInto(export.PrimaryProjection, project.TransfersByFundProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "transfers",
				After:      []string{""},
				From:       0,
				Side:       export.SideSource,
				Role:       "transfer_in",
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
					"the citywide chart counts; what the city transfers OUT is larger, " +
					"because pp.72-75 print the transfers each fund makes to the Capital " +
					"Improvement Program under a heading of their own and p76 does not list " +
					"them.",
			},
		}...)
	}
	// THE FUND COLUMN OPENS AT LAST, AND IT IS THE STEP THIS CHART HAS BEEN
	// MISSING. pp.167-170 are the General Fund's schedule and decompose that
	// fund alone, so until this step existed every OTHER fund the drill-down
	// draws was the end of the chain -- which is what the `general_fund` role
	// on the step above exists to keep honest, by not offering a click it
	// cannot answer. pp.85-125's lower block is the only published schedule
	// that says what any other fund pays for.
	//
	// A SIBLING OF THAT STEP AND NOT A REPLACEMENT FOR IT. Both open tier 3 of
	// the fund group's window; they are told apart by Role, which validateSteps
	// requires to be distinct and non-empty on both. fund/100 is `general_fund`
	// wherever it is drawn, so it keeps opening into its 23 divisions -- a
	// finer answer than its eleven departments -- and this step takes the other
	// sixty.
	//
	// IT IS DECLARED APART FROM THE FUND-FLOWS BLOCK AND GUARDED ON IT, because
	// it needs two things and the two can fail separately: pp.85-125's lower
	// block, for the document it draws, and pp.127-140, for the chart it opens
	// FROM. A corpus that lost the revenue schedule would have no fund-group
	// window for this rung to hang off, and validateSteps would refuse the view
	// by name rather than the site dropping one page.
	//
	// APPENDED LAST RATHER THAN BESIDE THE STEP IT MIRRORS. tools/jscheck reads
	// declared steps by key now (fisc-7e1g) and not by position, so the order
	// is no longer load-bearing -- but an INSERT into the first literal moves
	// every index after it, and the cheapest way not to depend on having fixed
	// all of them is not to insert.
	//
	// NO CAP, MEASURED RATHER THAN ASSUMED. The widest fund this step opens
	// draws 5 departments (fund/240 in FY2023-24 actual; 3 in both adopted
	// columns), against the 11 on fund/100, which opens elsewhere. A cap
	// declared over a column that never reaches it is a fold no reader has ever
	// seen, pinned inert.
	//
	// NO GAP AND NO RESIDUAL. A gap would claim every other node this step
	// opens balances, and the two sides of a fund here are two schedules that
	// are not meant to be equal -- the caveat on the document says so, and the
	// description below says it on the chart. A residual is for an endpoint of
	// the chart above that the drawn document cannot decompose; the flank this
	// window keeps is carried verbatim off that chart, so there is none.
	//
	// ONE GUARD AND NOT TWO NESTED ONES, so the literal below sits at the
	// indent every other step literal here sits at. tools/jscheck/harness.mjs
	// slices step entries on a brace at a fixed depth and cross-checks the
	// count against the `Key:`, `From:` and `After:` fields it finds -- it
	// threw rather than dropping the step, which is the guard working, and
	// keeping the shape uniform is cheaper than widening the parse.
	_, openable := stepByKey(spine.Steps, "fund-group")
	if openable && opensInto(export.PrimaryProjection, project.DepartmentFundingProjection, projections) {
		spine.Steps = append(spine.Steps, []export.DrillStep{
			{
				Key:        "fund-departments",
				After:      []string{"fund-group"},
				From:       3,
				Role:       "fund",
				Projection: project.DepartmentFundingProjection,
				// [the group | this fund | the departments it pays for].
				// The step before it draws tier 2 to the LEFT of tier 3, so
				// the group stays the left column here -- the same window
				// the General Fund's opens into, one document over.
				Keep:  []int{2},
				Tiers: []int{2, 3, 4},
				Noun:  "fund",
				Back:  "All funds",
				Tail:  "departments",
				// THE DIFFERENCE IS NAMED IN BOTH DIRECTIONS, and the first
				// draft of this sentence was not. It said the difference is
				// what the city "transfers out of the fund and adds to its
				// reserves" -- the fund step's wording, which is true of
				// fund/100 and false of a fund that pays departments MORE than
				// its revenue. Measured over both committed columns: 7 of the
				// 54 funds this step opens in FY2025-26 and 5 of the 52 in
				// FY2026-27 pay out more than pp.127-140 give them, the widest
				// being fund/240 at 1,064,044 against 322,600. A sentence that
				// is wrong on one window in eight is worse than no sentence,
				// because only the reader who checks can tell which they have.
				Description: "The fund group this fund belongs to is on the left and the " +
					"city departments it pays for are on the right \u2014 that fund's rows " +
					"of Budget Book pp.85-125, rescaled to its total. The two sides of the " +
					"fund in the middle are read from two different schedules and are not " +
					"one figure: what it takes in is its revenue, from pp.127-140, and what " +
					"leaves it here is what the departments draw on it. Either side may be " +
					"the larger. The difference is money the city moves between its own " +
					"funds and into or out of accumulated balance, which pp.66-67 print for " +
					"the fund group as a whole and no published schedule breaks down by " +
					"fund. A department here is the WHOLE department across every fund that " +
					"pays it, which is a coarser thing than the divisions the General Fund " +
					"opens into \u2014 five names belong to both tiers, so do not read one " +
					"as the other.",
			},
		}...)
	}
	out := []export.View{spine}

	// THE TABLES COME AFTER THE CHARTS THEY BELONG TO. Listed before them the
	// nav read "Revenue tables" and then "Revenue" -- two entries beginning
	// with the same word, in an order that made the fuller answer look like the
	// footnote. The charts are what a reader came for; this is where they go
	// when a chart is not enough.
	if _, ok := projections[project.TrendsProjection]; ok {
		out = append(out, export.View{
			Path: "trends.html",
			// "Revenue tables" AND NOT "Revenue by fund", because a Revenue
			// page that draws a chart is coming and two nav entries both
			// beginning "Revenue" would leave a reader guessing which is
			// which. This one is the tables; that is the distinction worth
			// putting in the label.
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
				{Heading: "Revenues", Kind: "revenue"},
				{Heading: "Expenditures", Kind: "expenditure"},
				// CLOSED, unlike the two blocks above, because this heading
				// names one printed row rather than a category: any other
				// fund_balance series with no fund group reaching this
				// projection must be refused, not absorbed under it. The
				// display label is p168's own three wrapped lines joined —
				// the fact's row_label stays the printed tail, and only the
				// Line cell says the whole phrase.
				{Heading: "Excess of revenues over (under) expenditures", Kind: "fund_balance",
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
				{Heading: "General Fund", Kind: "fund_balance", FundGroup: "general"},
				{Heading: "All Other Governmental Funds", Kind: "fund_balance"},
			},
			Title: "What Livermore's funds held at each year's end",
			Lede: "The balance of the General Fund and of all other governmental " +
				"funds at each June 30, FY2015-16 through FY2024-25, split into the " +
				"GASB 54 categories that say how spendable each dollar is.",
		})
	}
	// THE CAVEATS INDEX, THE SECOND VIEW THAT NAMES NO PROJECTION. It lists
	// every published document's caveats in full, so the other pages can show
	// one line and link here instead of reprinting the whole paragraph
	// underneath a chart.
	//
	// UNCONDITIONAL, like the spine and unlike the three views between them,
	// and the asymmetry is a claim worth stating rather than an oversight.
	// Those three are conditional because a nav entry pointing at a page that
	// was not written is the failure views() exists to prevent -- and a
	// document that was not built cannot be rendered. The spine is
	// unconditional because a site with no index.html is not a site, and
	// Options.validate refuses one. This page is unconditional for a third
	// reason: it depends on no single document. Every builder in
	// internal/project emits caveats unconditionally: caveats() always appends
	// the stocks and permanent-funds pair, fundFlowsCaveats returns three and a
	// fourth on a column that decomposes a fund group, and trendsCaveats
	// returns four. So the empty case cannot arise from this
	// repository -- and buildCaveatsPage refuses it anyway, rather than
	// publishing a nav entry to a blank page, because "cannot arise here" is a
	// claim about today's corpus and the guard is about tomorrow's.
	//
	// It is appended BEFORE the provenance block so that the two indexes sit
	// together at the end of the nav, after the pages that draw something.
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

	// THE PROVENANCE INDEX, WHICH NAMES NO PROJECTION. It is an index of the
	// site's own record store, built from Result.PageIndex. It was the one view
	// whose template rendered no document -- see
	// export.templateRendersADocument, whose own comment anticipated a second
	// one -- and the caveats index above is now the other.
	//
	// Conditional on the index being non-empty for the same reason every other
	// view is conditional on its document: a nav entry pointing at a page that
	// was not written is the failure this function exists to prevent.
	if len(built.PageIndex) > 0 {
		out = append(out, export.View{
			Path:     "provenance.html",
			Nav:      "Sources and data",
			Template: export.ProvenanceTemplate,
			Title:    "Every figure this site publishes, and the page it came from",
			// ONE SENTENCE, and the rest of what this lede said is still on the
			// page: the locator rule it enumerated is stated at the top of
			// provenance.html.tmpl, where a reader meets the links it governs,
			// and repeating it in a header was the second copy.
			Lede: "Every figure this site publishes, with the document, page and byte " +
				"offset it was read from — downloadable whole, or one page at a time.",
		})
	}
	return out
}
