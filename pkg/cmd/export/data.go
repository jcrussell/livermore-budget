package export

import (
	"fmt"
	"os"
	"path/filepath"

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
	// verify` checks the same triple, and two copies would let it pass a graph
	// this command does not publish.
	opts := project.Options{
		FiscalYear: project.PublishedFiscalYear,
		Basis:      project.PublishedBasis,
		Scope:      project.PublishedScope,
		Version:    build.Get().String(),
	}
	out := map[string][]byte{}
	for _, p := range project.Registry(reg) {
		b, err := p.Build(facts, opts)
		if err != nil {
			return nil, fmt.Errorf("build the %s projection: %w", p.Name(), err)
		}
		out[p.Name()] = b
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
