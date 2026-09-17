package export

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// TestTheRungArtifactIsWhatGoComputes pins testdata/rungs.json to rungsOf
// over the committed store, byte for byte, the way facts.jsonl is pinned to
// `fisc build`: rebuild and compare, never in place. On a difference the
// computed artifact is written under bin/ and the test says how to
// copy it over, so the regeneration is one visible step with a diff to read
// rather than an -update flag.
//
// THE VACUITY GUARDS ARE THE POINT. tools/jscheck/rungs.mjs holds the client
// to this file, and a file in which no cap ever engages, or which answers for
// one budget or one year, is one the arm could report PASS against without
// the comparison it exists for ever running.
func TestTheRungArtifactIsWhatGoComputes(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	var spine *export.View
	for _, v := range views(built) {
		if v.Path == export.IndexPath {
			v := v
			spine = &v
		}
	}
	if spine == nil {
		t.Fatal("no view at the index path; there is no spine to walk")
	}
	doc, err := rungsOf(built.Projections, *spine)
	if err != nil {
		t.Fatalf("rungsOf: %v", err)
	}
	got, err := encodeRungs(doc)
	if err != nil {
		t.Fatal(err)
	}
	want, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rungsPath)))
	if readErr != nil || !bytes.Equal(got, want) {
		// bin/ is where `fisc build --output bin/facts-rebuilt.jsonl` lands
		// too: gitignored, inside the checkout, and still there after the
		// test returns, which a t.TempDir is not.
		rebuilt := filepath.Join(root, "bin", "rungs-rebuilt.json")
		if err := os.MkdirAll(filepath.Dir(rebuilt), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rebuilt, got, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("%s is not what Go computes over the committed store (%v); the computed "+
			"artifact is at %s -- regenerate with `cp -f %s %s` and read the diff before committing it",
			rungsPath, readErr, rebuilt, rebuilt, rungsPath)
	}

	if len(doc.Columns) != len(spine.YearStems) || len(doc.Columns) == 0 {
		t.Fatalf("the artifact answers for %d column(s) and the spine lists %d year(s)", len(doc.Columns), len(spine.YearStems))
	}
	widths := map[int]bool{}
	engaged := 0
	for _, col := range doc.Columns {
		if len(col.Rungs) == 0 {
			t.Fatalf("column %q has no rung, so there is nothing to hold the client to", col.Stem)
		}
		for _, r := range col.Rungs {
			widths[r.Width] = true
			for _, c := range r.Caps {
				if c.Hidden > 0 {
					engaged++
				}
			}
		}
	}
	for _, w := range rungWidths {
		if !widths[w] {
			t.Errorf("no rung is answered at %d columns", w)
		}
	}
	if engaged == 0 {
		t.Fatal("no cap engages on any rung at any width, so a cap perturbed in this artifact could not be seen")
	}
}
