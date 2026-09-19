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

// TestTheRungArtifactIsWhatGoComputes pins testdata/rungs.json to the bytes
// buildAll ships at rungsServedPath over the committed store, byte for
// byte, the way facts.jsonl is pinned to `fisc build`: rebuild and compare,
// never in place. On a difference the served artifact is written under bin/
// and the test says how to copy it over, so the regeneration is one visible
// step with a diff to read rather than an -update flag.
//
// THE SERVED BYTES, NOT A SECOND CALL OF rungsOf: a fixture pinned to a
// computation the export did not make would hold the client to an answer
// the site never served, and a buildAll that dropped the file would leave
// that pin green.
//
// THE VACUITY GUARDS ARE THE POINT. tools/jscheck/rungs.mjs holds the client
// to this file, and a file in which every column's ids are empty, in which no
// column holds more than a cap the client would fold, or which answers for
// one year, is one the arm could report PASS against without the comparison
// it exists for ever running. Each guard below names the shape it refuses and
// where the committed corpus supplies the opposite.
func TestTheRungArtifactIsWhatGoComputes(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	spine, err := spineView(built)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := built.Files[rungsServedPath]
	if !ok {
		t.Fatalf("buildAll ships no %s, so the store serves no rung answer; the asset channel carries %d files", rungsServedPath, len(built.Files))
	}
	var doc rungsDoc
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatalf("decode %s: %v", rungsServedPath, err)
	}
	if doc.SchemaVersion != rungsSchemaVersion {
		t.Fatalf("%s declares schema_version %d, want %d", rungsServedPath, doc.SchemaVersion, rungsSchemaVersion)
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
		t.Fatalf("%s is not what buildAll serves at %s over the committed store (%v); the served "+
			"artifact is at %s -- regenerate with `cp -f %s %s` and read the diff before committing it",
			rungsPath, rungsServedPath, readErr, rebuilt, rebuilt, rungsPath)
	}

	if len(doc.Columns) != len(spine.YearStems) || len(doc.Columns) == 0 {
		t.Fatalf("the artifact answers for %d column(s) and the spine lists %d year(s)", len(doc.Columns), len(spine.YearStems))
	}
	var overCap, uncappedIDs, plural, unflanked, flankPlural, flankCarried, emptyFlank, parented int
	distinct := map[string]bool{}
	// THE MARKS ARE ANSWERED ON DIFFERENT STEPS AND NEVER MEET, which is a
	// pinned zero: no step declares both a residual and a gap, so no rung
	// carries two marks, and the order the two are applied in is a claim
	// nothing on the committed spine can contradict.
	for _, s := range spine.Steps {
		if len(s.Residual) > 0 && len(s.Gaps) > 0 {
			t.Errorf("step %q declares both a residual and a gap, which no rung in this artifact has been measured carrying together", s.Key)
		}
	}
	gapYears := map[string]bool{}
	residualYears := map[string]bool{}
	for _, col := range doc.Columns {
		if len(col.Rungs) == 0 {
			t.Fatalf("column %q has no rung, so there is nothing to hold the client to", col.Stem)
		}
		// WHAT EACH RUNG HOLDS, READ ONCE AND KEYED BY PATH, which is the
		// artifact's shape now that no column budget multiplies it: a path
		// answered twice is two answers to one question, and the second pass
		// needs the first pass's whole file to ask about a rung's parent.
		holds := map[string][]string{}
		for _, r := range col.Rungs {
			key := strings.Join(r.Path, " > ")
			if _, twice := holds[key]; twice {
				t.Fatalf("column %q answers the path %q twice, and a rung is one opened path", col.Stem, key)
			}
			ids := []string{}
			for _, d := range r.Draws {
				ids = append(ids, d.IDs...)
				ids = append(ids, d.Carried...)
			}
			holds[key] = ids
		}
		for _, r := range col.Rungs {
			key := strings.Join(r.Path, " > ")
			s, ok := stepByKey(spine.Steps, r.Step)
			if !ok {
				t.Fatalf("%s %s: step %q is not declared on the spine", col.Stem, key, r.Step)
			}
			at := func(format string, args ...any) {
				t.Helper()
				t.Errorf("%s %s: %s", col.Stem, key, fmt.Sprintf(format, args...))
			}
			tiers := make([]int, len(r.Draws))
			for i, d := range r.Draws {
				tiers[i] = d.Tier
			}
			// EVERY COLUMN THE STEP DECLARES IS ANSWERED, IN ITS ORDER. A
			// column the document draws nothing in is answered empty rather
			// than dropped, so a walk that dropped one -- as it dropped a
			// widened column at a narrow budget -- is one id set short of the
			// step it claims to answer.
			if !slices.Equal(tiers, s.Tiers) {
				at("draws tiers %v and the step declares %v", tiers, s.Tiers)
			}
			var centres, flanks int
			for _, d := range r.Draws {
				drawn := len(d.IDs) + len(d.Carried)
				switch d.Role {
				case roleCentre:
					centres++
					if !slices.Equal(slices.Concat(d.IDs, d.Carried), []string{r.Path[len(r.Path)-1]}) {
						at("the centre draws %v and carries %v, not the opened node alone", d.IDs, d.Carried)
					}
				case roleFlank:
					flanks++
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
				// WHAT THE CLIENT IS LEFT TO FOLD, counted against the step's own
				// declaration rather than against a field of this file: a column
				// holding more than the cap DrillStep.Caps declares for its tier
				// is one the client folds and this artifact does not.
				if c, capped := capOf(s, d.Tier); capped && drawn > c {
					overCap++
				} else if !capped && len(d.IDs) > 0 {
					uncappedIDs++
				}
			}
			// A MARK IS THE OPENED NODE'S OWN, UNDER THE CLIENT'S ID, AND IS
			// COUNTED ON NO COLUMN: its role names which of the two it is,
			// its id is that role's prefix on the last node of the path, a
			// gap has exactly one side, the short one, and a residual names
			// the declared endpoints it carries, sorted, and carries cents.
			if len(r.Marks) > 1 {
				at("carries %d marks, and the spine declares the two marks on different steps", len(r.Marks))
			}
			opened := r.Path[len(r.Path)-1]
			for _, m := range r.Marks {
				switch m.Role {
				case export.RoleResidual:
					residualYears[col.Stem] = true
					if m.ID != export.ResidualID(opened) {
						at("residual mark %q is not %q", m.ID, export.ResidualID(opened))
					}
					if m.InCents+m.OutCents == 0 {
						at("residual mark %q carries nothing", m.ID)
					}
					if len(m.Ends) == 0 || !slices.IsSorted(m.Ends) || len(slices.Compact(slices.Clone(m.Ends))) != len(m.Ends) {
						at("residual mark %q's endpoints are not a sorted, unique, non-empty list: %v", m.ID, m.Ends)
					}
					for _, e := range m.Ends {
						if _, declared := s.Residual[e]; !declared {
							at("residual mark %q carries %q, which the step does not declare an endpoint", m.ID, e)
						}
					}
				case export.RoleGap:
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
			// THE RUNG ABOVE HOLDS THE NODE THIS ONE OPENED, and that is what
			// says the walk descends the whole chart rather than the part a
			// fold left: a column answered folded would be one whose hidden
			// members are opened by rungs no answer above them names.
			if len(r.Path) > 1 {
				above := strings.Join(r.Path[:len(r.Path)-1], " > ")
				switch held, answered := holds[above]; {
				case !answered:
					at("opens under the path %q, which this column answers no rung for", above)
				case !slices.Contains(held, opened):
					at("opens %q, and the rung at %q neither counts nor carries it in any column", opened, above)
				default:
					parented++
				}
			}
		}
	}
	// EACH OF THESE IS A SHAPE THE ARM COULD PASS WITHOUT COMPARING ANYTHING.
	// The corpus supplies them today at, respectively, special-revenue's 32
	// funds under fund-group's cap of 8; fund-departments' tier 4, which no
	// step caps; every fund opened out of a fund group; and transfers/in
	// under the transfers step, which keeps no flank.
	//
	// THE UNFLANKED GUARD IS A REGRESSION SEEN FROM BOTH SIDES. The walk once
	// skipped every step that keeps no flank and wrote the skip into the
	// artifact, and the arm let every rung under a skipped step by. A walk
	// that quietly did so again would leave every other guard here green and
	// the transfers rung unanswered, so the artifact must answer under such a
	// step.
	if overCap == 0 {
		t.Error("no column holds more than the cap its step declares for that tier, so the client has no fold left to exercise and this artifact could not show one")
	}
	if uncappedIDs == 0 {
		t.Error("no uncapped column carries ids, so the comparison reaches nothing a cap does not")
	}
	if parented == 0 {
		t.Error("no rung opens a node another rung answers, so nothing here witnesses the walk descending past one chart")
	}
	if plural == 0 || len(distinct) < 2 {
		t.Errorf("%d column(s) draw more than one id and %d distinct ids are drawn in all, so an id perturbed in this artifact could not be seen", plural, len(distinct))
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
	// THE GAP IS ANSWERED ON THE YEAR THAT DRAWS ONE: p0067's
	// services-and-supplies against pp.85-125's rows in FY2026-27, which ties
	// to the cent in FY2025-26. An artifact with no gap mark is one the arm
	// could hold the client's markGap to without ever comparing a figure.
	if len(gapYears) == 0 {
		t.Error("no column answers a gap mark, so the arm has no gap to hold the client to")
	}
	// THE RESIDUAL IS ANSWERED IN EVERY YEAR: the fund groups
	// whose draw or transfers in pp.127-140 print for no fund, which both
	// published columns have.
	if len(residualYears) != len(doc.Columns) {
		t.Errorf("%d of %d columns answer a residual mark, and every published column carries a group the fund schedule does not fully decompose", len(residualYears), len(doc.Columns))
	}
}

// TestRungsAnswerAColumnTheDocumentDrawsNothingIn is the shape no committed
// document supplies, on a spine of one hand-written document: a column the
// step declares that the document folds no node to. It is answered with an
// empty id list rather than dropped, because a dropped column reads as one
// the step never declared (drawnTier), and the walk no longer drops a widened
// column at all.
//
// MEASURED AGAINST THE CORPUS, which is why it is hand-written: every outward
// column every shipped step declares holds at least one node once the answer
// stops being cut to a budget, so the committed artifact witnesses this
// nowhere.
func TestRungsAnswerAColumnTheDocumentDrawsNothingIn(t *testing.T) {
	// g's parts reach tier 2 and nothing reaches the widened tier 3.
	doc := export.Graph{
		Nodes: []export.GraphNode{{ID: "r", Tier: 0}, {ID: "g", Tier: 1}, {ID: "f", Tier: 2, Parent: "g"}},
		Links: []export.GraphLink{{Source: "r", Target: "g", ValueCents: 10}, {Source: "g", Target: "f", ValueCents: 10}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	spine := export.View{
		Path: export.IndexPath, YearStems: []string{"d"}, RenderTiers: []int{0, 1, 2},
		Steps: []export.DrillStep{{Key: "open", After: []string{""}, From: 1, Keep: []int{0}, Tiers: []int{0, 1, 2, 3}, Widen: []int{3}}},
	}
	got, err := rungsOf(map[string][]byte{"d": raw}, spine)
	if err != nil {
		t.Fatal(err)
	}
	var tiers []int
	var wide *drawnTier
	for _, col := range got.Columns {
		for _, r := range col.Rungs {
			if !slices.Equal(r.Path, []string{"g"}) {
				continue
			}
			for i, d := range r.Draws {
				tiers = append(tiers, d.Tier)
				if d.Tier == 3 {
					wide = &r.Draws[i]
				}
			}
		}
	}
	if !slices.Equal(tiers, []int{0, 1, 2, 3}) {
		t.Fatalf("the rung at g draws tiers %v, and the step declares [0 1 2 3]", tiers)
	}
	if wide.IDs == nil || len(wide.IDs) != 0 || len(wide.Carried) != 0 || wide.Role != roleOutward {
		t.Errorf("tier 3 is answered %+v, and the document draws nothing there", *wide)
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
	spine, err := spineView(built)
	if err != nil {
		t.Fatal(err)
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

// capOf is the cap the step declares for one tier, and whether it declares
// one at all: the permission to fold that column, which the client spends and
// this artifact does not.
func capOf(s export.DrillStep, tier int) (int, bool) {
	if i := slices.IndexFunc(s.Caps, func(c export.TierCap) bool { return c.Tier == tier }); i >= 0 {
		return s.Caps[i].Cap, true
	}
	return 0, false
}

// TestRungsRefuseAWindowTheClientWouldNotDraw is the refusal in
// rungWalker.answer a declaration of the shipped spine can provoke, inert on
// the committed declarations and shown firing on a one-field change to them:
// a flank that sends nothing into the opened node, which filterLinks throws
// on rather than drawing empty.
//
// THE THREE CAP REFUSALS THIS TEST ALSO HELD ARE GONE WITH THEIR SUBJECT. A
// cap on a kept flank, a cap on the centre and a fold above a drawn deeper
// tier were each a shape in which Go's fold and the client's would disagree,
// and the walk no longer folds: DrillStep.Caps is a permission it ships and
// does not spend (drawnTier).
func TestRungsRefuseAWindowTheClientWouldNotDraw(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	spine, err := spineView(built)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, step string
		perturb    func(*export.DrillStep)
		want       string
	}{
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

// TestRungsRefuseAKeptHalfTheArtifactHasNoShapeFor is the refusal no
// committed document can provoke, on a spine of one hand-written document: a
// second node beside the opened one in the kept half's centre column, which
// the client would draw beside the node the reader clicked -- the mirror of
// the fresh half's refusal.
//
// The walk is run rather than answer called, because the shape arises from
// what an earlier rung left on screen: here the overview's own fold.
//
// IT HELD A SECOND CASE, A FOLDED TAIL AT A KEPT TIER, and that case is gone
// with its subject: it reached the refusal by capping a column on the rung
// above so the chart left on screen carried a tail, and no chart this walk
// builds carries one now (drawnTier).
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
