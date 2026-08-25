package export

import (
	"fmt"
	"os"
	"path/filepath"
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

	// The slice is internal/project's declaration, not this command's: `fisc
	// verify` checks the same slices, and two copies would let it pass a graph
	// this command does not publish.
	//
	// ONE DOCUMENT PER PUBLISHED YEAR. Both budget years have been projected and
	// checked since the two-year book was mapped -- internal/check derives one
	// projection per (fiscal year, basis) the spine carries -- and what FY2027
	// never was, is exported (fisc-kwq). The years come from the same list
	// published-projection-built reads, so a year added here cannot ship
	// unchecked and a year checked there cannot go unpublished.
	version := build.Get().String()
	out := map[string][]byte{}
	for _, year := range project.PublishedFiscalYears() {
		opts := project.Options{
			FiscalYear: year,
			Basis:      project.PublishedBasis,
			Scope:      project.PublishedScope,
			Version:    version,
		}
		for _, p := range project.Registry(reg) {
			// A projection that says which slices it is of must agree that it is
			// of THIS one. Without this the two commands diverge silently: `fisc
			// verify` builds each projection over the slices it declares, so a
			// projection whose slices do not include a published one would be
			// checked under its own and published under one it declared it is
			// not of -- the exact split the comment above says this shared
			// declaration exists to prevent, reopened the moment a projection
			// could disagree.
			//
			// It fails closed and names both sides, because the fix is never
			// obvious from a wrong figure: either the projection wants a slice
			// this command cannot publish, or the published set moved and the
			// projection was not told.
			if sl, ok := p.(project.Sliced); ok {
				want := sl.Slices(facts, version)
				if !slicesContain(want, opts) {
					return nil, fmt.Errorf(
						"the %s projection is of %d slice(s), none of them the published "+
							"FY%d %s %s this command publishes: %s",
						p.Name(), len(want), opts.FiscalYear, opts.Basis, opts.Scope,
						describeSlices(want))
				}
			}
			b, err := p.Build(facts, opts)
			if err != nil {
				return nil, fmt.Errorf("build the %s projection for FY%d: %w",
					p.Name(), year, err)
			}
			out[project.PublishedStem(p.Name(), year)] = b
		}
	}
	if _, ok := out[export.PrimaryProjection]; !ok {
		return nil, fmt.Errorf("no projection named %q was registered", export.PrimaryProjection)
	}
	return out, nil
}

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

// slicesContain reports whether want holds o, compared on the three selectors
// that decide which facts a projection reads. Version is deliberately not
// compared: it is stamped into metadata and is not a selector.
func slicesContain(want []project.Options, o project.Options) bool {
	for _, w := range want {
		if w.FiscalYear == o.FiscalYear && w.Basis == o.Basis && w.Scope == o.Scope {
			return true
		}
	}
	return false
}

// describeSlices renders the slices a projection declared, for the refusal
// above. A count alone would leave the operator to guess which year or schedule
// was wanted.
func describeSlices(o []project.Options) string {
	if len(o) == 0 {
		return "it declared none, so the fact store carries nothing it is of"
	}
	out := make([]string, 0, len(o))
	for _, s := range o {
		out = append(out, fmt.Sprintf("FY%d %s %s", s.FiscalYear, s.Basis, s.Scope))
	}
	return strings.Join(out, ", ")
}

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
