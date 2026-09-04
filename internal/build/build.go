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
//
// Modified is separate from Commit rather than a suffix on it because String
// truncates the hash to twelve characters, which would cut a "+dirty" suffix
// off the end and report a dirty build as a clean one -- the exact failure this
// field exists to prevent.
type Info struct {
	Version  string
	Commit   string
	Date     string
	Modified bool
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
	return fromSettings(i, bi.Settings)
})

// fromSettings fills from the Go build metadata whatever the linker flags left
// empty, and records whether the tree was dirty when the binary was built.
//
// SPLIT OUT OF Get SO A TEST CAN VARY THE INPUT. Get reads the running
// binary's own metadata, and a test cannot dirty the working tree and rebuild
// itself; synthetic settings are the only way to assert the marker.
//
// A build with no vcs settings at all -- `-buildvcs=false`, or a build outside
// a repository -- reports Modified false and Commit empty together, so it
// claims no commit rather than claiming a clean one.
func fromSettings(i Info, settings []debug.BuildSetting) Info {
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			if i.Commit == "" {
				i.Commit = s.Value
			}
		case "vcs.time":
			if i.Date == "" {
				i.Date = s.Value
			}
		case "vcs.modified":
			// Read whatever the commit came from. The setting describes the
			// tree the binary was built from, which is the same tree the
			// Makefile's `git rev-parse` read.
			i.Modified = s.Value == "true"
		}
	}
	return i
}

// String renders the version line shown by `fisc --version`.
func (i Info) String() string {
	s := i.Version
	if i.Commit != "" {
		c := i.Commit
		if len(c) > 12 {
			c = c[:12]
		}
		s += " (" + c
		if i.Modified {
			s += "+dirty"
		}
		s += ")"
	} else if i.Modified {
		s += " (dirty)"
	}
	if i.Date != "" {
		s += " built " + i.Date
	}
	return s
}
