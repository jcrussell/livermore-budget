package export

import (
	"bytes"
	"encoding/json"
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
	var engaged, unfoldedOverCap, uncappedIDs, plural, unflanked, flankPlural, flankCarried, emptyFlank int
	distinct := map[string]bool{}
	widened := false
	// THE MARKS ARE ANSWERED ON DIFFERENT STEPS AND NEVER MEET, which is a
	// pinned zero: no step declares both a residual and a gap, so no rung
	// carries two marks, and the order the two are applied in is a claim
	// nothing on the committed spine can contradict.
	for _, s := range spine.Steps {
		if len(s.Residual) > 0 && len(s.Gaps) > 0 {
			t.Errorf("step %q declares both a residual and a gap, which no rung in this artifact has been measured carrying together", s.Key)
		}
	}
	gapAt := map[int]bool{}
	gapYears := map[string]bool{}
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
				drawn := len(d.IDs) + len(d.Carried)
				switch d.Role {
				case roleCentre:
					centres++
					if !slices.Equal(slices.Concat(d.IDs, d.Carried), []string{r.Path[len(r.Path)-1]}) || d.Candidates != 1 {
						at("the centre draws %v and carries %v of %d, not the opened node alone", d.IDs, d.Carried, d.Candidates)
					}
				case roleFlank:
					flanks++
					// THE FLANK IS ANSWERED, NOT DECLINED, and never under a cap:
					// this walk refuses a cap on a kept tier, so a flank with a
					// hidden count is one it could not have written.
					if d.Cap != 0 || d.Hidden != 0 {
						at("flank tier %d carries a cap of %d and hides %d", d.Tier, d.Cap, d.Hidden)
					}
					if len(d.IDs) > 1 {
						flankPlural++
					}
					if len(d.Carried) > 0 {
						flankCarried++
					}
					if len(d.IDs) == 0 {
						emptyFlank++
					}
				case roleOutward:
				default:
					at("tier %d has role %q", d.Tier, d.Role)
				}
				// IDS IS A LIST EVEN WHEN EMPTY: an omitted field is what "not
				// answered" looked like, and the arm reading this file compares
				// the two shapes differently.
				if d.IDs == nil {
					at("tier %d's ids are omitted rather than empty", d.Tier)
				}
				if drawn != d.Candidates-d.Hidden {
					at("tier %d draws %d ids and carries %d of %d candidates with %d hidden", d.Tier, len(d.IDs), len(d.Carried), d.Candidates, d.Hidden)
				}
				if d.Hidden != 0 && (d.Cap <= 0 || d.Hidden != d.Candidates-d.Cap || drawn != d.Cap) {
					at("tier %d hides %d of %d under a cap of %d, drawing %d", d.Tier, d.Hidden, d.Candidates, d.Cap, drawn)
				}
				if !slices.IsSorted(d.IDs) || len(slices.Compact(slices.Clone(d.IDs))) != len(d.IDs) {
					at("tier %d's ids are not sorted and unique: %v", d.Tier, d.IDs)
				}
				if !slices.IsSorted(d.Carried) || len(slices.Compact(slices.Clone(d.Carried))) != len(d.Carried) {
					at("tier %d's carried ids are not sorted and unique: %v", d.Tier, d.Carried)
				}
				// A DECLARED ENDPOINT IS NEVER COUNTED AS A PART: it is carried
				// or absent, on every column of a step that declares it.
				for _, id := range d.IDs {
					if _, declared := s.Residual[id]; declared {
						at("tier %d counts %q as a part of the opened node, and the step declares it a residual endpoint", d.Tier, id)
					}
					if slices.Contains(d.Carried, id) {
						at("tier %d both counts and carries %q", d.Tier, id)
					}
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
			// A MARK IS THE OPENED NODE'S OWN, UNDER THE CLIENT'S ID, AND IS
			// COUNTED ON NO COLUMN: its role names which of the two it is,
			// its id is that role's prefix on the last node of the path,
			// and a gap has exactly one side, the short one.
			if len(r.Marks) > 1 {
				at("carries %d marks, and the spine declares the two marks on different steps", len(r.Marks))
			}
			opened := r.Path[len(r.Path)-1]
			for _, m := range r.Marks {
				switch m.Role {
				case export.RoleGap:
					gapAt[r.Width] = true
					gapYears[col.Stem] = true
					if m.ID != export.GapID(opened) {
						at("gap mark %q is not %q", m.ID, export.GapID(opened))
					}
					if (m.InCents == 0) == (m.OutCents == 0) {
						at("gap mark %q has in %d and out %d, and a gap has exactly one side", m.ID, m.InCents, m.OutCents)
					}
					if len(m.Ends) != 0 {
						at("gap mark %q names endpoints %v, and a gap carries none", m.ID, m.Ends)
					}
				default:
					at("mark %q has role %q", m.ID, m.Role)
				}
				if !slices.Contains(s.Tiers, m.Tier) {
					at("mark %q stands at tier %d, which the step does not declare", m.ID, m.Tier)
				}
				for _, d := range r.Draws {
					if slices.Contains(d.IDs, m.ID) || slices.Contains(d.Carried, m.ID) {
						at("tier %d lists the mark %q as a document node", d.Tier, m.ID)
					}
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
	// THE FLANK'S OWN THREE SHAPES, each supplied by the corpus: a flank of
	// several ids, where one id perturbed can be seen; a flank carrying a
	// declared endpoint, where the residual's subtraction is compared rather
	// than assumed on both sides; and a flank whose every mark is carried,
	// where "answered with nothing" has to be written to be read.
	if flankPlural == 0 {
		t.Error("no flank draws more than one id, so an id perturbed on a flank could not be seen")
	}
	if flankCarried == 0 {
		t.Error("no flank carries a declared endpoint, so the residual's two spine feeders are compared on no column")
	}
	if emptyFlank == 0 {
		t.Error("no flank draws nothing of its own, so an omitted ids would be indistinguishable from an empty one")
	}
	// THE GAP IS ANSWERED, AT BOTH BUDGETS, ON THE YEAR THAT DRAWS ONE: p0067's
	// services-and-supplies against pp.85-125's rows in FY2026-27, which ties
	// to the cent in FY2025-26. An artifact with no gap mark is one the arm
	// could hold the client's markGap to without ever comparing a figure.
	for _, w := range rungWidths {
		if !gapAt[w] {
			t.Errorf("no rung at %d columns answers a gap mark, so a gap perturbed in this artifact could not be seen there", w)
		}
	}
	if len(gapYears) == 0 {
		t.Error("no column answers a gap mark, so the arm has no gap to hold the client to")
	}
}

// TestRungsRefuseADriftTheStepDoesNotDeclare is the refusal a declaration
// can provoke, inert on the committed declarations and shown firing on a
// one-field change: the object-category step declaring a gap on another
// node and none on services-and-supplies, while p0067 and pp.85-125 still
// print FY2026-27's cell 250,000 dollars apart. The client meets the same
// drift as a throw in a browser; this is where it fails the build.
//
// DROPPING THE WHOLE MAP IS NOT THE MUTATION, measured: a step that declares
// no gap at all makes no claim that its nodes balance, so rungsOf builds
// with the difference unstated, which is markGap's own first rule. The
// claim with teeth is a declaration that names some node, which is a claim
// about every other node the step opens.
func TestRungsRefuseADriftTheStepDoesNotDeclare(t *testing.T) {
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
		perturb    func(*export.DrillStep)
		want       string
	}{
		{"a gap declared on another node alone", "object-category", func(s *export.DrillStep) {
			s.Gaps = map[string]string{"expenditure/debt-services": "a reason for a node that ties"}
		},
			`opens "expenditure/services-and-supplies": the chart above sends 13050208700 into "expenditure/services-and-supplies" and this one draws 13025208700 of it, a difference of 25000000 cents that no declaration on this step accounts for`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := spine
			v.Steps = slices.Clone(spine.Steps)
			i := slices.IndexFunc(v.Steps, func(s export.DrillStep) bool { return s.Key == tc.step })
			if i < 0 {
				t.Fatalf("no step %q on the spine", tc.step)
			}
			tc.perturb(&v.Steps[i])
			_, err := rungsOf(built.Projections, v)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("rungsOf with %s: err = %v, want one containing %q", tc.name, err, tc.want)
			}
		})
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

// TestRungsRefuseACapTheClientWouldApplyDifferently is the refusals in
// rungWalker.answer a declaration can provoke, each inert on the committed
// declarations and each shown firing on a one-field change to them: a cap on
// a kept flank, which sideOf would apply to a column this walk reads off the
// chart above; a cap on the centre, a column of one node; a fold on an
// outward tier with a tier drawn beyond it, which capColumn's orphaned()
// would rank from the folded column and this walk from the whole one; and a
// flank that sends nothing into the opened node, which filterLinks throws on
// rather than drawing empty.
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
		perturb    func(*export.DrillStep)
		want       string
	}{
		{"a cap on the kept flank", "fund-departments", func(s *export.DrillStep) { s.Caps = []export.TierCap{{Tier: 2, Cap: 8}} }, "caps tier 2, which is a flank it keeps"},
		{"a cap on the centre", "fund-departments", func(s *export.DrillStep) { s.Caps = []export.TierCap{{Tier: 3, Cap: 8}} }, "caps tier 3, which is its centre"},
		{"a fold with a deeper tier drawn", "fund", func(s *export.DrillStep) {
			s.Caps = []export.TierCap{{Tier: 4, Cap: 20}, {Tier: 5, Cap: 8}}
		}, "folds tier 4 (23 under a cap of 20) while drawing tier 5"},
		// Tier 0 of the fund-group rung is revenue, which flows into the fund
		// group and not into any fund, so a flank kept there sends nothing
		// into the opened fund.
		{"a flank that sends nothing into the opened node", "fund-departments", func(s *export.DrillStep) {
			s.Keep, s.Tiers = []int{0}, []int{0, 3, 4}
		}, "sends nothing between tiers [0 3] and it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := spine
			v.Steps = slices.Clone(spine.Steps)
			i := slices.IndexFunc(v.Steps, func(s export.DrillStep) bool { return s.Key == tc.step })
			if i < 0 {
				t.Fatalf("no step %q on the spine", tc.step)
			}
			tc.perturb(&v.Steps[i])
			_, err := rungsOf(built.Projections, v)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("rungsOf with %s: err = %v, want one containing %q", tc.name, err, tc.want)
			}
		})
	}
}

// TestRungsRefuseAKeptHalfTheArtifactHasNoShapeFor is the two refusals no
// committed document can provoke, each on a spine of one hand-written
// document: a folded tail at a kept tier, which the client would draw with a
// hidden count under its label and this artifact has no cap to explain; and
// a second node beside the opened one in the kept half's centre column,
// which the client would draw beside the node the reader clicked -- the
// mirror of the fresh half's refusal.
//
// The walk is run rather than answer called, because both shapes arise from
// what an earlier rung left on screen: the tail from a cap engaging on the
// rung above, the neighbour from the overview's own fold.
func TestRungsRefuseAKeptHalfTheArtifactHasNoShapeFor(t *testing.T) {
	encode := func(g export.Graph) []byte {
		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	cases := []struct {
		name  string
		doc   export.Graph
		steps []export.DrillStep
		want  string
	}{
		{
			name: "a folded tail at a kept tier",
			// Three payers into x, capped to one on the first rung, so the
			// chart it leaves holds x, one payer and a tail of two; the
			// second rung opens x again with the payer tier kept.
			doc: export.Graph{
				Nodes: []export.GraphNode{{ID: "x", Tier: 0}, {ID: "y1", Tier: 1}, {ID: "y2", Tier: 1}, {ID: "y3", Tier: 1}, {ID: "z", Tier: 2}},
				Links: []export.GraphLink{
					{Source: "y1", Target: "x", ValueCents: 30}, {Source: "y2", Target: "x", ValueCents: 20},
					{Source: "y3", Target: "x", ValueCents: 10}, {Source: "x", Target: "z", ValueCents: 5},
				},
			},
			steps: []export.DrillStep{
				{Key: "payers", After: []string{""}, From: 0, Tiers: []int{0, 1}, Caps: []export.TierCap{{Tier: 1, Cap: 1}}},
				{Key: "again", After: []string{"payers"}, From: 0, Keep: []int{1}, Tiers: []int{1, 0, 2}},
			},
			want: `keeps tier 1 and the chart on screen draws the folded tail "aggregate/tail/1" there`,
		},
		{
			name: "a neighbour beside the kept centre",
			// g2 sends a ribbon into f, which is g1's child, so the kept
			// half of g1's window folds that ribbon to g2 -> g1 and draws g2
			// in the centre column beside g1.
			doc: export.Graph{
				Nodes: []export.GraphNode{{ID: "r", Tier: 0}, {ID: "g1", Tier: 1}, {ID: "g2", Tier: 1}, {ID: "f", Tier: 2, Parent: "g1"}},
				Links: []export.GraphLink{
					{Source: "r", Target: "g1", ValueCents: 10}, {Source: "r", Target: "g2", ValueCents: 10},
					{Source: "g1", Target: "f", ValueCents: 5}, {Source: "g2", Target: "f", ValueCents: 5},
				},
			},
			steps: []export.DrillStep{
				{Key: "open", After: []string{""}, From: 1, Keep: []int{0}, Tiers: []int{0, 1, 2}},
			},
			want: `opens "g1" and the chart on screen draws [g2] beside it at tier 1`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spine := export.View{Path: export.IndexPath, YearStems: []string{"d"}, RenderTiers: []int{0, 1, 2}, Steps: tc.steps}
			_, err := rungsOf(map[string][]byte{"d": encode(tc.doc)}, spine)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("rungsOf with %s: err = %v, want one containing %q", tc.name, err, tc.want)
			}
		})
	}
}
