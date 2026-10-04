// Package repo is the committed layout of the repository: the paths every
// command and every check means by "the facts", "the rules" and "the
// extraction", and the one way of finding the root they are relative to.
package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcrussell/livermore-budget/internal/hint"
)

// The committed layout, as slash-separated paths relative to the repository
// root.
//
// These live here rather than in each command because three packages have to
// name the same files: `fisc build` writes the fact store, `fisc export` reads
// it, and `fisc verify` checks it. Separate copies fail semi-open — verify would
// report on a different file than build wrote, and every check in it would pass —
// which is a hole no test in either package would notice.
const (
	// FactsPath is the fact store. It is `fisc build`'s --output default rather
	// than a fixed destination, because a draft rule set may be built elsewhere;
	// it is the file everything else means by "the facts".
	FactsPath = "facts/facts.jsonl"
	// MappingsDir holds the curated rule files.
	MappingsDir = "mappings"
	// DataDir holds the curated registries, the source registry and the
	// committed extraction. Not to be confused with export's output data
	// directory, which is a published path and not a repository one.
	DataDir = "data"
	// ExtractedDir is where tools/extract.py writes, one directory per
	// document. It is spelled once, here, because a command that joined its own
	// copy of the path would drift from the reader that has to find the result:
	// internal/corpus opens an extraction under it and internal/check sweeps
	// every file in it for drift.
	ExtractedDir = DataDir + "/extracted"
)

// marker is the file whose presence identifies the repository root. It is
// deliberately a data file rather than go.mod: fisc reads committed artifacts,
// so the tree it cares about is the one holding the source registry.
const marker = DataDir + "/sources.yaml"

// Root walks up from the working directory looking for the marker. Walking up
// (rather than requiring the user to stand in the root) means fisc works from
// any subdirectory, the way git does.
func Root() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	start := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", hint.With(
				fmt.Errorf("no %s found in %q or any parent directory", marker, start),
				"run fisc from inside the livermore-budget repository",
			)
		}
		dir = parent
	}
}
