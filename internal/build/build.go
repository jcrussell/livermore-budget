// Package build carries version information stamped in at link time.
package build

import (
	"runtime/debug"
	"strings"
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
//
// VCSKnown IS WHAT KEEPS Modified FALSE FROM MEANING "CLEAN". The two states a
// bool cannot tell apart are "the tree was clean" and "nothing recorded whether
// the tree was clean", and a build with -buildvcs=false is the second: measured,
// `GOFLAGS=-buildvcs=false make build` on a dirty tree emits no vcs settings at
// all while ldflags still supply a commit, so a bare Modified would publish that
// commit as though it were the source of the running code. String says
// "+unknown" there rather than nothing.
type Info struct {
	Version  string
	Commit   string
	Date     string
	Modified bool
	VCSKnown bool
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
// a repository -- leaves VCSKnown false, which String renders as "+unknown" on
// any commit it does have. Reporting Modified false there would be a claim
// nothing measured, and the linker can supply a commit independently of the
// metadata, so "no settings" does not imply "no commit".
func fromSettings(i Info, settings []debug.BuildSetting) Info {
	for _, s := range settings {
		if strings.HasPrefix(s.Key, "vcs.") {
			i.VCSKnown = true
		}
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
		switch {
		case i.Modified:
			s += "+dirty"
		case !i.VCSKnown:
			s += "+unknown"
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
