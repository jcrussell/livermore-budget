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
	builtAt := map[string]project.Options{}
	for _, p := range project.Registry(reg) {
		declared := slicesOf(p, facts, version)
		for _, o := range declared {
			stem := stemFor(p.Name(), o, len(declared))
			// Two documents landing on one stem would write one file and drop
			// the other in silence, and a reader would have no way to tell which
			// of the two they were looking at.
			if prev, ok := builtAt[stem]; ok {
				return nil, fmt.Errorf(
					"the %s projection wants two documents at the stem %q: %s and %s",
					p.Name(), stem, project.Describe(prev.Columns), project.Describe(o.Columns))
			}
			builtAt[stem] = o
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
func assertPublishedBuilt(builtAt map[string]project.Options) error {
	for _, d := range project.PublishedDocuments() {
		o, ok := builtAt[d.Stem]
		if !ok {
			return fmt.Errorf(
				"the site publishes %s and no projection built it; the documents "+
					"built were: %s", d, joinComma(builtStems(builtAt)))
		}
		if missing := project.MissingColumns(d, o); len(missing) > 0 {
			return fmt.Errorf(
				"the site publishes %s and the document built at that stem covers %s, "+
					"missing %s", d, project.Describe(o.Columns), project.Describe(missing))
		}
	}
	return nil
}

// builtStems is the stems built, in order, for a refusal that has to say what
// it did build.
func builtStems(builtAt map[string]project.Options) []string {
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
			Scope:   project.PublishedScope,
			Version: version,
		})
	}
	return out
}

// stemFor is the file stem one of a projection's documents is written under.
//
// THE QUESTION IS WHETHER THE PROJECTION PUBLISHES ONE DOCUMENT PER YEAR, and it
// is answered by how many slices the projection declared, not by how many
// columns any one of them carries. A projection declaring SEVERAL slices is
// publishing several documents that must be told apart, and the way this site
// tells them apart is the fiscal year: the spine keeps data/sankey.json for the
// opening year and data/sankey-2027.json for the next (project.PublishedStem). A
// projection declaring ONE slice publishes ONE document and needs no
// distinguishing suffix, so it takes its name verbatim.
//
// AN EARLIER VERSION ASKED THE COLUMN COUNT and was wrong in a way no test then
// covered: a store carrying exactly one revenue-by-fund column -- one year
// mapped, or four columns dropping to one -- would have made the trends document
// single-column, sent it through PublishedStem, and shipped it as
// revenue-trends-2024.json while data/revenue-trends.json, the path
// docs/revenue-trends-contract.md promises, silently did not exist. The column
// count is a property of a document; the stem is a property of a SET of them.
//
// The error in the other direction is the one the contract names: a multi-column
// document put through PublishedStem is written once per published year, as two
// byte-identical files one of which claims a year it does not cover. Both
// directions are pinned by TestStemForDistinguishesAYearFromAWhole.
func stemFor(name string, o project.Options, slices int) string {
	if slices == 1 {
		return name
	}
	return project.PublishedStem(name, o.Columns[0].FiscalYear)
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

// yearStems is the document stems for one projection, one per published fiscal
// year, in the order a reader should meet them.
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
	years := project.PublishedFiscalYears()
	out := make([]string, 0, len(years))
	for _, y := range years {
		stem := project.PublishedStem(name, y)
		if _, ok := projections[stem]; ok {
			out = append(out, stem)
		}
	}
	return out
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
	return out
}
