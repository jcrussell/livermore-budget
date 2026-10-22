package beadid

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCitedTellsABeadIDFromTheOtherThingsNamedfisc(t *testing.T) {
	// `fisc-` is an overloaded prefix in this tree rather than a namespace, so
	// most of these are shapes that are not a citation.
	tests := []struct {
		line string
		want []string
	}{
		{"see fisc-kc3j for the lane", []string{"fisc-kc3j"}},
		{"`fisc-yj4w.7` declines two tier sets.", []string{"fisc-yj4w.7"}},
		{"fisc-1wr.5.1 is deferred", []string{"fisc-1wr.5.1"}},
		{"the pair (fisc-abc, fisc-def)", []string{"fisc-abc", "fisc-def"}},
		// The period ending a sentence is not part of the id, but a dotted
		// child's is; the pattern has already taken the second when it applies.
		{"see fisc-abc.", []string{"fisc-abc"}},
		// A byob id's whole hyphenated slug is the id; a trailing hyphen is not.
		{"byob-layout.1 is a decision", []string{"byob-layout.1"}},
		{"byob-command-shape.1 is one id", []string{"byob-command-shape.1"}},
		{"byob-layout -- and then", []string{"byob-layout"}},
		{"byob-release.8 and byob-release.10", []string{"byob-release.8", "byob-release.10"}},
		// Not a citation: something longer runs into it on one side.
		{"a-byob-layout.1 thing", nil},
		{"byob-layout.1x", nil},

		// A fact id and a series id: the SECOND hyphen is what says so.
		{`"fact_id": "fisc-f-0ba7b00dfaf7"`, nil},
		{`"series_id": "fisc-s-ce9117881328"`, nil},
		{`link.FactIDs = []string{"fisc-f-notafact"}`, nil},
		{"const IDPrefix = \"fisc-f-\"", nil},

		// A marker file and a scratch directory: the leading dot says so.
		{`const ExportMarkerName = ".fisc-export"`, nil},
		{`os.MkdirTemp(target, ".fisc-write-probe-*")`, nil},
		{"every fixedPaths key -- .fisc-export, app.js", nil},

		// Neither end may run into a word.
		{"prefisc-abc", nil},
		{"fisc-abcZ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, Cited(tt.line)); diff != "" {
				t.Errorf("Cited(%q) (-want +got):\n%s", tt.line, diff)
			}
		})
	}
}
