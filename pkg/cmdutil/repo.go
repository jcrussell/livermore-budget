package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// repoMarker is the file whose presence identifies the repository root. It is
// deliberately a data file rather than go.mod: fisc reads committed artifacts,
// so the tree it cares about is the one holding the source registry.
const repoMarker = "data/sources.yaml"

// findRepoRoot walks up from the working directory looking for the marker.
// Walking up (rather than requiring the user to stand in the root) means fisc
// works from any subdirectory, the way git does.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	start := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, repoMarker)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", WithHint(
				fmt.Errorf("no %s found in %q or any parent directory", repoMarker, start),
				"run fisc from inside the livermore-budget repository",
			)
		}
		dir = parent
	}
}
