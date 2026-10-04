package cmdutil

import (
	"sync"

	"github.com/jcrussell/livermore-budget/internal/repo"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// Factory holds every cross-cutting dependency a command might need. It is
// built once in main and threaded into each command constructor, so no
// command reaches for a package-level global.
//
// Cheap dependencies are eager fields. Expensive ones are func() (T, error)
// closures, invoked only by commands that actually need them — which is what
// keeps `fisc --help` from reading the filesystem.
type Factory struct {
	IOStreams *iostreams.IOStreams

	// RepoRoot resolves the repository root, which anchors every data path.
	// Lazy because --help and --version have no business looking for it.
	RepoRoot func() (string, error)
}

// New returns a Factory wired to the real process streams.
func New() *Factory {
	return &Factory{
		IOStreams: iostreams.System(),
		RepoRoot:  sync.OnceValues(repo.Root),
	}
}
