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
	return get(Info{Version: Version, Commit: Commit, Date: Date}, debug.ReadBuildInfo)
})

// get is Get with the metadata source injected, so a test can supply settings
// the running binary does not have.
//
// THE BUILD METADATA IS READ EVEN WHEN THE LINKER SUPPLIED EVERYTHING, and that
// is the whole point of this function. An earlier version returned early once
// Commit and Date were both set, which made the vcs.modified arm below
// unreachable in every binary the Makefile produces -- so the dirty marker
// worked only for `go build` and not for the build that ships. The Makefile's
// `git describe --dirty` does not cover the gap: measured, it reports a clean
// hash for a tree carrying an UNTRACKED file, where vcs.modified reports true.
// Nothing here overrides a linker-supplied field; fromSettings fills only what
// is empty.
func get(i Info, read func() (*debug.BuildInfo, bool)) Info {
	bi, ok := read()
	if !ok {
		return i
	}
	return fromSettings(i, bi.Settings)
}

// fromSettings fills from the Go build metadata whatever the linker flags left
// empty, and records whether the tree was dirty when the binary was built.
//
// Get reads the running binary's own metadata, and a test cannot dirty the
// working tree and rebuild itself; synthetic settings are the only way to
// assert the marker.
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
			// Makefile's `git rev-parse` read, so it applies to a
			// linker-supplied commit as much as to a discovered one.
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
