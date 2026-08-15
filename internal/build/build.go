// Package build carries version information stamped in at link time.
package build

import (
	"runtime/debug"
	"sync"
)

// Injected via -ldflags -X. Defaults apply to `go run` and `go build` without
// the Makefile.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Info describes the running binary.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Get returns the build info, falling back to the Go build metadata when the
// linker flags were not supplied. Computed once; nothing mutates on the read
// path.
var Get = sync.OnceValue(func() Info {
	i := Info{Version: Version, Commit: Commit, Date: Date}
	if i.Commit != "" && i.Date != "" {
		return i
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return i
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if i.Commit == "" {
				i.Commit = s.Value
			}
		case "vcs.time":
			if i.Date == "" {
				i.Date = s.Value
			}
		}
	}
	return i
})

// String renders the version line shown by `fisc --version`.
func (i Info) String() string {
	s := i.Version
	if i.Commit != "" {
		c := i.Commit
		if len(c) > 12 {
			c = c[:12]
		}
		s += " (" + c + ")"
	}
	if i.Date != "" {
		s += " built " + i.Date
	}
	return s
}
