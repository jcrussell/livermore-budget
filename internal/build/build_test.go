package build

import (
	"runtime/debug"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestFromSettingsReadsTheDirtyMarker is the guard for fisc-qz2w: a binary built
// from a working tree with uncommitted changes used to report the last commit's
// hash as though it were the source of the running code.
//
// That hash is not decoration here. It reaches published output through
// check.Report.GeneratedBy and export's document GeneratedBy, so a dirty build
// broke the provenance chain silently -- the hash was well-formed, it just
// described code that was not what ran.
//
// The settings are synthetic because Get reads the running binary's own
// metadata: a test cannot dirty the tree and rebuild itself.
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
			want: Info{Commit: rev, Date: "2026-09-04T15:37:19Z"},
		},
		{
			name: "a dirty build is marked",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: rev},
				{Key: "vcs.time", Value: "2026-09-04T15:37:19Z"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: Info{Commit: rev, Date: "2026-09-04T15:37:19Z", Modified: true},
		},
		{
			name: "no vcs settings claims no commit rather than a clean one",
			// -buildvcs=false, or a build outside a repository. Modified stays
			// false, but so does Commit, so nothing is asserted about a tree.
			settings: []debug.BuildSetting{{Key: "GOARCH", Value: "amd64"}},
			want:     Info{},
		},
		{
			name: "a dirty tree marks a commit the linker supplied",
			// Get falls through to here whenever EITHER ldflags field is
			// missing. vcs.modified describes the tree the Makefile's
			// `git rev-parse` read, so it applies to that commit too.
			in: Info{Commit: rev},
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0000000000000000000000000000000000000000"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: Info{Commit: rev, Modified: true},
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

// TestTheBuildMetadataIsReadEvenWhenTheLinkerSuppliedEverything is the guard
// for the defect the first version of this fix shipped: Get returned early once
// Commit and Date were both set from ldflags, which made the whole vcs.modified
// arm unreachable in every binary `make build` produces -- the build that ships.
//
// THE PRE-EXISTING `git describe --dirty` DOES NOT COVER THAT GAP, which is why
// this is not merely redundant plumbing. Measured on this repository: with an
// untracked file in the tree, `git describe --tags --always --dirty` prints a
// bare hash with no marker, while Go's own vcs.modified prints true. So the
// ldflags path had NO dirty signal for the commonest way to dirty a tree.
//
// It asserts on the metadata being CONSULTED, not on a call count: a reader
// that is never called cannot make the tree's state reach the version line.
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
	want := Info{Version: "v1.2.3", Commit: ld, Date: "2026-09-04T15:37:19Z", Modified: true}
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

	clean := Info{Version: "dev", Commit: rev, Date: "2026-09-04T15:37:19Z"}
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

// TestStringMarksADirtyBuildWithNoCommit covers the arm the truncation branch
// cannot reach: Modified true with Commit empty, which is what a dirty build
// looks like when the linker supplied a Date but no Commit.
func TestStringMarksADirtyBuildWithNoCommit(t *testing.T) {
	i := Info{Version: "dev", Modified: true}
	if got, want := i.String(), "dev (dirty)"; got != want {
		t.Errorf("Info{Modified: true}.String() = %q, want %q", got, want)
	}
}
