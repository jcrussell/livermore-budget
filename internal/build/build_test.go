package build

import (
	"runtime/debug"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestFromSettingsReadsTheDirtyMarker: a binary built from a dirty tree says
// so; the hash reaches published output through GeneratedBy. The settings are
// synthetic because Get reads the running binary's own metadata.
func TestFromSettingsReadsTheDirtyMarker(t *testing.T) {
	const rev = "44d173a10e6f382e3c42181825df1cd395f4a269"

	tests := []struct {
		name     string
		in       Info
		settings []debug.BuildSetting
		want     Info
	}{
		{
			name: "a clean build is not marked",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: rev},
				{Key: "vcs.time", Value: "2026-09-04T15:37:19Z"},
				{Key: "vcs.modified", Value: "false"},
			},
			want: Info{Commit: rev, Date: "2026-09-04T15:37:19Z", VCSKnown: true},
		},
		{
			name: "a dirty build is marked",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: rev},
				{Key: "vcs.time", Value: "2026-09-04T15:37:19Z"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: Info{Commit: rev, Date: "2026-09-04T15:37:19Z", Modified: true, VCSKnown: true},
		},
		{
			name: "no vcs settings leaves the tree state unknown, not clean",
			// -buildvcs=false, or a build outside a repository. Modified stays
			// false, and VCSKnown stays false to say that false was not measured.
			settings: []debug.BuildSetting{{Key: "GOARCH", Value: "amd64"}},
			want:     Info{},
		},
		{
			name: "no vcs settings does not make a linker-supplied commit clean",
			// The arm fisc-qz2w reaches again if VCSKnown goes away: measured,
			// `GOFLAGS=-buildvcs=false make build` on a tree with an untracked
			// file emitted no vcs settings while ldflags still supplied the
			// commit, and the version line claimed a clean build.
			in:       Info{Version: "v1", Commit: rev, Date: "d"},
			settings: []debug.BuildSetting{{Key: "GOARCH", Value: "amd64"}},
			want:     Info{Version: "v1", Commit: rev, Date: "d"},
		},
		{
			name: "a dirty tree marks a commit the linker supplied",
			// vcs.modified describes the tree the Makefile's `git rev-parse`
			// read, so it applies to a linker-supplied commit as much as to a
			// discovered one. The sibling test below covers the case where the
			// linker supplied BOTH fields, which is what the Makefile does.
			in: Info{Commit: rev},
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0000000000000000000000000000000000000000"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: Info{Commit: rev, Modified: true, VCSKnown: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fromSettings(tt.in, tt.settings)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("fromSettings() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestTheBuildMetadataIsReadEvenWhenTheLinkerSuppliedEverything: vcs.modified
// is consulted even when ldflags set Commit and Date, since `git describe
// --dirty` does not mark a tree dirtied only by an untracked file.
func TestTheBuildMetadataIsReadEvenWhenTheLinkerSuppliedEverything(t *testing.T) {
	const ld = "1111111111111111111111111111111111111111"

	var read bool
	fake := func() (*debug.BuildInfo, bool) {
		read = true
		return &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "2222222222222222222222222222222222222222"},
			{Key: "vcs.time", Value: "2020-01-01T00:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		}}, true
	}

	// Both linker fields set, which is exactly what the Makefile supplies.
	got := get(Info{Version: "v1.2.3", Commit: ld, Date: "2026-09-04T15:37:19Z"}, fake)

	if !read {
		t.Fatal("the build metadata was not read at all when ldflags supplied Commit and Date; " +
			"the dirty marker cannot fire on the path make build takes")
	}
	want := Info{Version: "v1.2.3", Commit: ld, Date: "2026-09-04T15:37:19Z", Modified: true, VCSKnown: true}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("get() mismatch (-want +got):\n%s", diff)
	}
	// The linker's values win over the metadata's; only Modified is new.
	if got.Commit != ld {
		t.Errorf("get() overrode the linker-supplied commit with %q", got.Commit)
	}
}

// TestGetSurvivesAbsentBuildInfo covers the arm where the metadata cannot be
// read at all: the linker's values stand and nothing claims a tree state.
func TestGetSurvivesAbsentBuildInfo(t *testing.T) {
	in := Info{Version: "v1", Commit: "abc", Date: "d"}
	got := get(in, func() (*debug.BuildInfo, bool) { return nil, false })
	if diff := cmp.Diff(in, got); diff != "" {
		t.Errorf("get() with no build info mismatch (-want +got):\n%s", diff)
	}
}

// TestStringSurvivesTruncation is the reason Modified is a field and not a
// "+dirty" suffix written onto Commit: String truncates the hash to twelve
// characters, so a suffix would be cut off and a dirty build would render
// identically to a clean one.
func TestStringSurvivesTruncation(t *testing.T) {
	const rev = "44d173a10e6f382e3c42181825df1cd395f4a269"

	// VCSKnown is what makes this one CLEAN rather than merely unmeasured;
	// without it String renders "+unknown" and the fixture would not be the
	// thing the test is named for.
	clean := Info{Version: "dev", Commit: rev, Date: "2026-09-04T15:37:19Z", VCSKnown: true}
	dirty := clean
	dirty.Modified = true

	if got, want := clean.String(), "dev (44d173a10e6f) built 2026-09-04T15:37:19Z"; got != want {
		t.Errorf("clean.String() = %q, want %q", got, want)
	}
	if got, want := dirty.String(), "dev (44d173a10e6f+dirty) built 2026-09-04T15:37:19Z"; got != want {
		t.Errorf("dirty.String() = %q, want %q", got, want)
	}
	if clean.String() == dirty.String() {
		t.Error("a dirty build renders identically to a clean one, which is fisc-qz2w")
	}
}

// TestStringDistinguishesUnknownFromClean: "clean", "dirty" and "not
// measured" (`-buildvcs=false` with an ldflags commit) all render differently.
func TestStringDistinguishesUnknownFromClean(t *testing.T) {
	const rev = "44d173a10e6f382e3c42181825df1cd395f4a269"
	base := Info{Version: "dev", Commit: rev}

	clean := base
	clean.VCSKnown = true
	dirty := clean
	dirty.Modified = true
	unknown := base // VCSKnown false

	got := []string{clean.String(), dirty.String(), unknown.String()}
	want := []string{
		"dev (44d173a10e6f)",
		"dev (44d173a10e6f+dirty)",
		"dev (44d173a10e6f+unknown)",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("String() mismatch (-want +got):\n%s", diff)
	}
	if unknown.String() == clean.String() {
		t.Error("an unmeasured tree renders as a clean one, which is fisc-qz2w " +
			"reachable through -buildvcs=false")
	}
}

// TestStringMarksADirtyBuildWithNoCommit: Modified true with Commit empty, the
// state a build inside a `git init`-ed repository with no commits carries.
func TestStringMarksADirtyBuildWithNoCommit(t *testing.T) {
	i := Info{Version: "dev", Modified: true}
	if got, want := i.String(), "dev (dirty)"; got != want {
		t.Errorf("Info{Modified: true}.String() = %q, want %q", got, want)
	}
}
