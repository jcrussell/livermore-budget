package cmdutil

import (
	"sync"

	"github.com/jcrussell/livermore-budget/internal/repo"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// Factory holds every cross-cutting dependency, built once in main and
// threaded into each command constructor. Expensive dependencies are lazy
// closures, so `fisc --help` reads no filesystem.
type Factory struct {
	IOStreams *iostreams.IOStreams

	// RepoRoot resolves the repository root, which anchors every data path.
	RepoRoot func() (string, error)
}

// New returns a Factory wired to the real process streams.
func New() *Factory {
	return &Factory{
		IOStreams: iostreams.System(),
		RepoRoot:  sync.OnceValues(repo.Root),
	}
}
