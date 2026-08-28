package export

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/build"
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// factsPath is the fact store this export reads. It is cmdutil's rather than
// this package's: `fisc build` writes that path and `fisc verify` checks it, and
// a site built from a different file than either would still export cleanly.
const factsPath = cmdutil.FactsPath

// buildProjections is the default Builder: read the committed fact store and
// run every registered projection over it.
//
// It reads facts/facts.jsonl rather than re-running the mapping engine, and
// that is the point rather than a shortcut. facts.jsonl is the audit trail; a
// site built from anything else could disagree with the file the provenance
// claims it came from. `fisc build` regenerates it byte-deterministically, so
// the two can be compared in CI (fisc-1wr.6).
func buildProjections(repoRoot string) (map[string][]byte, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(factsPath))
	// #nosec G304 -- the path is the repository root fisc found by walking up
	// from the working directory, joined to a constant; it is not user input.
	f, err := os.Open(path)
	if err != nil {
		return nil, cmdutil.Hintf(fmt.Errorf("read the fact store: %w", err),
			"run `fisc build` to generate %s", factsPath)
	}
	defer f.Close() //nolint:errcheck // read-only file; nothing to flush

	facts, err := fact.Read(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", factsPath, err)
	}

	// The label registry is loaded here, in the composition root, and passed
	// in: internal/project is deliberately decoupled from internal/registry
	// and reaches it through a one-method interface.
	reg, err := registry.Load(os.DirFS(filepath.Join(repoRoot, "data")))
	if err != nil {
		return nil, fmt.Errorf("load the data registries: %w", err)
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
// WHY THE THREE REMAINING ENTRIES ARE NOT A CHART PROBLEM ANY MORE. They were:
// the drill-down's 61-node fund column laid every node and every ribbon out at
// zero height, and c3a337d landed the fold that fixes it. drilldown.html renders
// fund-flows now. What the other three columns still lack is a YEAR CONTROL, and
// that is a different piece of work with a trap of its own -- see the const.
var unviewedDocuments = map[string]string{
	project.FundFlowsProjection + "-2024-actual":  fundFlowsUnviewed,
	project.FundFlowsProjection + "-2025-revised": fundFlowsUnviewed,
	project.FundFlowsProjection + "-2027":         fundFlowsUnviewed,
}

const fundFlowsUnviewed = "a published column of the General Fund drill-down that its page " +
	"cannot yet reach. drilldown.html renders fund-flows (FY2025-26) and lists no year " +
	"control, so these three ship as data no reader can open. Giving them one is not a " +
	"line in views(): yearStems walks PublishedDocuments() in declared order, which puts " +
	"2024-actual first, and View.validate refuses a view whose first stem is not its own " +
	"projection -- so the opening year has to be hoisted deliberately. FY2023-24 also " +
	"carries a seventh fund group, permanent, which FUND_ORDER has no hue for and " +
	"buildLegend no entry for, and site/style.css records that a seventh hue would " +
	"invalidate a measured CVD result"

// assertPublishedReachable is the half of the published-document contract that
// assertPublishedBuilt does not make: a document a reader can open.
//
// It takes the views rather than reading them, for assertPublishedBuilt's
// reason -- a test can hand it a set the real repository is never in.
//
// A DOCUMENT IS REACHABLE THROUGH A VIEW'S PROJECTION OR THROUGH ITS YEAR
// STEMS, and both arms are needed: the spine's second year has no view of its
// own and is reached only from the first view's year control.
func assertPublishedReachable(vs []export.View, built map[string][]byte) error {
	reachable := make(map[string]struct{}, len(vs))
	for _, v := range vs {
		reachable[v.Projection] = struct{}{}
		for _, stem := range v.YearStems {
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
func views(projections map[string][]byte) []export.View {
	out := []export.View{{
		Path:       export.IndexPath,
		Nav:        "Budget flows",
		Template:   export.SankeyTemplate,
		Projection: export.PrimaryProjection,
		YearStems:  yearStems(export.PrimaryProjection, projections),
	}}
	if _, ok := projections[project.TrendsProjection]; ok {
		out = append(out, export.View{
			Path:       "revenue.html",
			Nav:        "Revenue by fund",
			Template:   export.TrendsTemplate,
			Projection: project.TrendsProjection,
			Title:      "Where Livermore's revenue comes from, fund by fund",
			Lede: "Every revenue line the city prints for each of its funds, across four " +
				"budget columns. The columns are not one measurement: FY 2023-24 is money " +
				"that moved, FY 2024-25 is a mid-year re-forecast, and the two later years " +
				"are intentions adopted together.",
		})
	}
	// THE DRILL-DOWN OPENS ON FY2025-26 AND LISTS NO YEAR STEMS, which is a
	// smaller view than the four published columns could support and is
	// deliberate. See unviewedDocuments for the two things a year control here
	// has to solve first.
	//
	// RenderTiers is what makes this view drawable at all: 0 is the revenue
	// source, 2 the fund group, 4 the division. Tier 3 -- the 61 individual
	// funds -- folds into tier 2 in the client, because a 61-node column lays
	// every node and every ribbon out at zero height. Tier 5, the object
	// categories, folds into tier 4, because drawing it puts 29 of its 44 nodes
	// under one pixel and its labels are two strings repeated 44 times.
	// docs/general-fund-drilldown-contract.md's "Drawing it" section carries the
	// measurements; tools/jscheck/fold.mjs re-measures them on every run.
	if _, ok := projections[project.FundFlowsProjection]; ok {
		out = append(out, export.View{
			Path:        "drilldown.html",
			Nav:         "Fund and division",
			Template:    export.DrilldownTemplate,
			Projection:  project.FundFlowsProjection,
			RenderTiers: []int{0, 2, 4},
			Title:       "Which fund Livermore's money lands in, and which division spends it",
			Lede: "The citywide picture answers how big the budget is. This one answers " +
				"which fund a revenue source lands in, and which General Fund division " +
				"is given it \u2014 two schedules the city prints separately, over 18 pages. " +
				"It publishes no total, because the same money appears here at more than " +
				"one grain and any total would quietly count part of it twice.",
		})
	}
	return out
}
