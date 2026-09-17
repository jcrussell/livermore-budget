package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
// to this file, and a file in which no cap ever engages, in which every
// column's ids are empty, or which answers for one budget or one year, is one
// the arm could report PASS against without the comparison it exists for
// ever running. Each guard below names the shape it refuses and where the
// committed corpus supplies the opposite.
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
	var engaged, unfoldedOverCap, uncappedIDs, plural, unflanked int
	distinct := map[string]bool{}
	widened := false
	for _, col := range doc.Columns {
		if len(col.Rungs) == 0 {
			t.Fatalf("column %q has no rung, so there is nothing to hold the client to", col.Stem)
		}
		drawsAt := map[string]map[int]int{}
		for _, r := range col.Rungs {
			widths[r.Width] = true
			key := strings.Join(r.Path, " > ")
			if drawsAt[key] == nil {
				drawsAt[key] = map[int]int{}
			}
			drawsAt[key][r.Width] = len(r.Draws)
			s, ok := stepByKey(spine.Steps, r.Step)
			if !ok {
				t.Fatalf("%s %s at %d: step %q is not declared on the spine", col.Stem, key, r.Width, r.Step)
			}
			at := func(format string, args ...any) {
				t.Helper()
				t.Errorf("%s %s at %d columns: %s", col.Stem, key, r.Width, fmt.Sprintf(format, args...))
			}
			tiers := make([]int, len(r.Draws))
			for i, d := range r.Draws {
				tiers[i] = d.Tier
			}
			if !isSubsequence(tiers, s.Tiers) || len(slices.Compact(slices.Sorted(slices.Values(tiers)))) != len(tiers) {
				at("draws tiers %v, which is not an ordered subsequence of the step's %v", tiers, s.Tiers)
			}
			var centres, flanks int
			for _, d := range r.Draws {
				switch d.Role {
				case roleCentre:
					centres++
					if !slices.Equal(d.IDs, []string{r.Path[len(r.Path)-1]}) || d.Candidates != 1 {
						at("the centre draws %v of %d, not the opened node alone", d.IDs, d.Candidates)
					}
				case roleFlank:
					flanks++
					if d.Unanswered == "" {
						at("flank tier %d carries no reason", d.Tier)
					}
					if d.Cap != 0 || d.Candidates != 0 || d.IDs != nil || d.Hidden != 0 {
						at("flank tier %d carries an answer beside its reason: %+v", d.Tier, d)
					}
				case roleOutward:
				default:
					at("tier %d has role %q", d.Tier, d.Role)
				}
				if d.Unanswered != "" {
					if d.Role != roleFlank {
						at("tier %d is %s and carries reason %q", d.Tier, d.Role, d.Unanswered)
					}
					if _, known := doc.Reasons[d.Unanswered]; !known {
						at("tier %d carries reason %q, which the reasons table does not name", d.Tier, d.Unanswered)
					}
					continue
				}
				if len(d.IDs) != d.Candidates-d.Hidden {
					at("tier %d draws %d ids of %d candidates with %d hidden", d.Tier, len(d.IDs), d.Candidates, d.Hidden)
				}
				if d.Hidden != 0 && (d.Cap <= 0 || d.Hidden != d.Candidates-d.Cap || len(d.IDs) != d.Cap) {
					at("tier %d hides %d of %d under a cap of %d, drawing %d", d.Tier, d.Hidden, d.Candidates, d.Cap, len(d.IDs))
				}
				if !slices.IsSorted(d.IDs) || len(slices.Compact(slices.Clone(d.IDs))) != len(d.IDs) {
					at("tier %d's ids are not sorted and unique: %v", d.Tier, d.IDs)
				}
				for _, id := range d.IDs {
					distinct[id] = true
				}
				if len(d.IDs) > 1 {
					plural++
				}
				switch {
				case d.Cap > 0 && d.Hidden > 0:
					engaged++
				case d.Cap > 0 && d.Hidden == 0 && d.Candidates > d.Cap:
					unfoldedOverCap++
				case d.Cap == 0 && len(d.IDs) > 0:
					uncappedIDs++
				}
			}
			if len(s.Keep) > 0 && centres != 1 {
				at("draws %d centre column(s)", centres)
			}
			if len(s.Keep) == 0 {
				unflanked++
				if centres != 0 {
					at("keeps no flank and draws %d centre column(s)", centres)
				}
			}
			if flanks != len(s.Keep) {
				at("draws %d flank column(s) and the step keeps %d", flanks, len(s.Keep))
			}
		}
		for _, byWidth := range drawsAt {
			if a, ok := byWidth[3]; ok {
				if b, ok := byWidth[4]; ok && a != b {
					widened = true
				}
			}
		}
	}
	for _, w := range rungWidths {
		if !widths[w] {
			t.Errorf("no rung is answered at %d columns", w)
		}
	}
	// EACH OF THESE IS A SHAPE THE ARM COULD PASS WITHOUT COMPARING ANYTHING.
	// The corpus supplies them today at, respectively, special-revenue's
	// funds under fund-group; enterprise's 9 funds under the same cap of 8;
	// fund-departments' tier 4, which no step caps; fund's tier 5, which
	// only the fourth column buys; and transfers/in under the transfers step,
	// which keeps no flank.
	//
	// THE LAST TWO GUARDS ARE ONE REGRESSION SEEN FROM BOTH SIDES. The walk
	// once skipped every step that keeps no flank and wrote the skip into the
	// artifact, and the arm let every rung under a skipped step by. A walk
	// that quietly did so again would leave every other guard here green and
	// the transfers rung unanswered, so the artifact must answer under such a
	// step and must declare nothing skipped.
	if engaged == 0 {
		t.Error("no cap engages on any rung at any width, so a cap perturbed in this artifact could not be seen")
	}
	if unfoldedOverCap == 0 {
		t.Error("no column sits between its cap and cap+1, so the fold's threshold is not witnessed")
	}
	if uncappedIDs == 0 {
		t.Error("no uncapped column carries ids, so the comparison reaches nothing a cap does not")
	}
	if plural == 0 || len(distinct) < 2 {
		t.Errorf("%d column(s) draw more than one id and %d distinct ids are drawn in all, so an id perturbed in this artifact could not be seen", plural, len(distinct))
	}
	if !widened {
		t.Error("no path is answered at both widths with a different number of columns, so a walk that ignored Widen would be invisible")
	}
	if unflanked == 0 {
		t.Error("no rung is answered under a step that keeps no flank, so the transfers step is unanswered and the arm has nothing to hold the client to there")
	}
	if len(doc.Skipped) != 0 {
		t.Errorf("the artifact declares %d step(s) skipped and the walk reaches every declared step, so a rung under one would pass the arm unanswered: %+v", len(doc.Skipped), doc.Skipped)
	}
}

// isSubsequence is whether sub's elements appear in of, in order.
func isSubsequence(sub, of []int) bool {
	i := 0
	for _, x := range of {
		if i < len(sub) && sub[i] == x {
			i++
		}
	}
	return i == len(sub)
}

// TestRungsRefuseACapTheClientWouldApplyDifferently is the two refusals in
// rungWalker.answer, each inert on the committed declarations and each shown
// firing on a one-field change to them: a cap on a kept flank, which sideOf
// would apply and this walk would leave unanswered; and a fold on an outward
// tier with a tier drawn beyond it, which capColumn's orphaned() would rank
// from the folded column and this walk from the whole one.
func TestRungsRefuseACapTheClientWouldApplyDifferently(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	var spine export.View
	for _, v := range views(built) {
		if v.Path == export.IndexPath {
			spine = v
		}
	}
	cases := []struct {
		name, step string
		caps       []export.TierCap
		want       string
	}{
		{"a cap on the kept flank", "fund-departments", []export.TierCap{{Tier: 2, Cap: 8}}, "caps tier 2, which is a flank it keeps"},
		{"a fold with a deeper tier drawn", "fund", []export.TierCap{{Tier: 4, Cap: 20}, {Tier: 5, Cap: 8}}, "folds tier 4 (23 under a cap of 20) while drawing tier 5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := spine
			v.Steps = slices.Clone(spine.Steps)
			i := slices.IndexFunc(v.Steps, func(s export.DrillStep) bool { return s.Key == tc.step })
			if i < 0 {
				t.Fatalf("no step %q on the spine", tc.step)
			}
			v.Steps[i].Caps = tc.caps
			_, err := rungsOf(built.Projections, v)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("rungsOf with %s: err = %v, want one containing %q", tc.name, err, tc.want)
			}
		})
	}
}
