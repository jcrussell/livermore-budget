package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/structure"
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
// THE VACUITY GUARDS ARE THE POINT. The client's tests hold the client to
// this file, and a file in which every column's ids are empty, in which no
// column holds more than a cap the client would fold, or which answers for
// one year, is one they could report PASS against without the comparison
// they exist for ever running. Each guard below names the shape it refuses and
// where the committed corpus supplies the opposite.
//
// WHAT HOLDS A COLUMN'S MEMBERSHIP, now that the client reads this artifact
// rather than recomputing one to compare against it. Three of these guards
// read the documents and the step declarations
// and never the walk: every id a column draws is a node a document holds at
// that tier, every id it carries is one the document marks derived or the
// step declares an endpoint, and every id it draws that a later step opens
// is answered by a rung of its own. The third leans on export.Openable,
// which the walk also asks, so it is completeness read from the parent
// rather than a second reading of the documents.
//
// AND WHAT THEY DO NOT REFUSE, which is why the replay below exists: a
// mark's cents is not guarded here, nor which of its step's declared tiers it
// stands at, nor how many of the declared endpoints it names, and an id
// dropped from a column no later step opens leaves no trace in this test.
// Those four are
// TestTheRungArtifactIsWhatTheReachPrimitivesAnswer's. The mutations, and
// which guard took each one, are in docs/rung-walk-witness-evidence.md.
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
	// THE STAMP IS BLANKED ON BOTH SIDES, as the projection goldens' is. It
	// carries the commit and the build date, so comparing it would make this
	// fixture stale on every build and say nothing about the walk.
	stamp := regexp.MustCompile(`"generated_by": "[^"]*"`)
	blank := []byte(`"generated_by": ""`)
	if readErr != nil || !bytes.Equal(stamp.ReplaceAll(got, blank), stamp.ReplaceAll(want, blank)) {
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
	var documented, documentedKept, countedPlain, carriedDerived, carriedDeclared, answeredBelow int
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
		// THE DOCUMENTS THIS COLUMN IS READ AGAINST, and there are two kinds:
		// each step's own for this year, which is where its outward columns
		// come from, and the year's own, which the overview the first rungs
		// open from was folded from. The guards below ask these documents
		// what they hold; they do not ask the walk again.
		stems := stepStemsFor(t, spine, built.Projections, col.Stem)
		records := map[string]map[string]export.GraphNode{}
		for _, stem := range append(slices.Clone(stems), col.Stem) {
			if _, read := records[stem]; read {
				continue
			}
			raw, wasBuilt := built.Projections[stem]
			if !wasBuilt {
				t.Fatalf("column %q reads %q, which was not built", col.Stem, stem)
			}
			g, err := export.DecodeGraph(raw)
			if err != nil {
				t.Fatalf("column %q: %s: %v", col.Stem, stem, err)
			}
			byID := map[string]export.GraphNode{}
			for _, n := range g.Nodes {
				byID[n.ID] = n
			}
			records[stem] = byID
		}
		// WHAT EACH STEP OFFERS A READER TO OPEN, read once per column off
		// export.Openable -- the same answer the walk asks for, which is why
		// the completeness guard below is completeness from the parent and
		// not an independent reading of the documents.
		opens := make([][]string, len(spine.Steps))
		for i, st := range spine.Steps {
			o, err := export.Openable(spine, i, st, stems[i], built.Projections[stems[i]])
			if err != nil {
				t.Fatalf("column %q: step %q: %v", col.Stem, st.Key, err)
			}
			opens[i] = o
		}
		// WHAT EACH RUNG HOLDS, READ ONCE AND KEYED BY PATH, which is the
		// artifact's shape now that no column budget multiplies it: a path
		// answered twice is two answers to one question, and the second pass
		// needs the first pass's whole file to ask about a rung's parent.
		holds := map[string][]string{}
		answers := map[string]string{}
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
			answers[key] = r.Step
		}
		for _, r := range col.Rungs {
			key := strings.Join(r.Path, " > ")
			si := slices.IndexFunc(spine.Steps, func(x export.DrillStep) bool { return x.Key == r.Step })
			if si < 0 {
				t.Fatalf("%s %s: step %q is not declared on the spine", col.Stem, key, r.Step)
			}
			s := spine.Steps[si]
			at := func(format string, args ...any) {
				t.Helper()
				t.Errorf("%s %s: %s", col.Stem, key, fmt.Sprintf(format, args...))
			}
			// WHICH DOCUMENT AN ID CAME OFF IS NOT ONE DOCUMENT PER RUNG. An
			// outward column is this step's own document read at the tier the
			// column names. The centre and the flank are the chart on screen,
			// whose records this step's document need not hold at all: every
			// fund group a kept flank draws on an object-category rung is one
			// department-spending never mentions, so holding the kept half to
			// this step's own document fails on the committed corpus. It is
			// held to the documents this column reads instead, which is the
			// weaker claim of the two and is said here rather than implied.
			// The count is in docs/rung-walk-witness-evidence.md.
			recordOf := func(d drawnTier, id string) (export.GraphNode, bool) {
				if d.Role == roleOutward {
					n, held := records[stems[si]][id]
					return n, held && n.Tier == d.Tier
				}
				for _, byID := range records {
					if n, held := byID[id]; held && n.Tier == d.Tier {
						return n, true
					}
				}
				return export.GraphNode{}, false
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
				// EVERY ID A COLUMN DRAWS IS A NODE A DOCUMENT HOLDS AT THAT
				// TIER, which is the arm that refuses an id moved to a column
				// the document does not print it in. A tier is a property of
				// the documents and the walk computes none of it, so this is
				// a claim the walk cannot satisfy by agreeing with itself.
				for _, id := range slices.Concat(d.IDs, d.Carried) {
					n, held := recordOf(d, id)
					switch {
					case held && d.Role == roleOutward:
						documented++
					case held:
						documentedKept++
					case d.Role == roleOutward:
						at("tier %d draws %q, and %q holds no node of that id at tier %d", d.Tier, id, stems[si], d.Tier)
						continue
					default:
						at("tier %d draws %q, and no document this column reads holds a node of that id at tier %d", d.Tier, id, d.Tier)
						continue
					}
					// WHAT A COLUMN CARRIES IS WHAT MAKES A NODE CARRIED, and
					// it is the document and the declaration that say which:
					// a node the document marks derived, or an endpoint the
					// step declares. Counting one of those, or carrying a row
					// the document prints, are the two halves of the same
					// defect -- a reader is shown the same nodes either way
					// and the opened node's parts come to a different figure.
					_, declared := s.Residual[id]
					switch carried := slices.Contains(d.Carried, id); {
					case carried && n.Derived:
						carriedDerived++
					case carried && declared:
						carriedDeclared++
					case carried:
						at("tier %d carries %q, which no document this column reads marks derived and the step declares no endpoint for", d.Tier, id)
					case n.Derived:
						at("tier %d counts %q as a part of the opened node, and the document marks it derived", d.Tier, id)
					default:
						countedPlain++
					}
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
			// AND THE MIRROR, WHICH IS THE HALF THE PARENTAGE GUARD LEAVES
			// OPEN: a walk that stopped walking satisfies it, because every
			// rung it did write down still names its parent. So each id this
			// rung draws at a tier a later step opens, with the role that
			// step declares and where that step's document decomposes it,
			// has to be answered by a rung one path longer. Suppressing a
			// whole step's rungs took this artifact from 99 and 97 to 45 and
			// 45 with every other gate green (fisc-u8di).
			//
			// IT ASKS export.Openable, WHICH THE WALK ALSO ASKS. That makes
			// it completeness read from the parent rather than a second
			// reading of the documents: it cannot witness the openable set
			// itself being wrong, only the walk answering fewer rungs than
			// that set offers.
			for j, s2 := range spine.Steps {
				if !slices.Contains(s2.After, r.Step) {
					continue
				}
				for _, d := range r.Draws {
					if d.Tier != s2.From {
						continue
					}
					// A LATER STEP OPENING A KEPT COLUMN IS REFUSED RATHER
					// THAN READ: the role it would be matched on lives on the
					// chart on screen, and recordOf reads a kept id off
					// whichever document holds it. No shipped step does this
					// -- every child step opens its parent's outward column
					// -- so the shape is a pinned zero and not a case.
					if d.Role != roleOutward {
						at("tier %d is a %s column and step %q opens tier %d after %q, which this guard reads no node record for", d.Tier, d.Role, s2.Key, s2.From, r.Step)
						continue
					}
					for _, id := range slices.Concat(d.IDs, d.Carried) {
						n, held := recordOf(d, id)
						if !held || !slices.Contains(opens[j], id) {
							continue
						}
						if s2.Role != "" && n.Role != s2.Role {
							continue
						}
						below := key + " > " + id
						if answers[below] != s2.Key {
							at("draws %q at tier %d, which step %q opens, and this column answers no rung of that step below it", id, d.Tier, s2.Key)
							continue
						}
						answeredBelow++
					}
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
	// THE MEMBERSHIP GUARDS' OWN FIVE SHAPES, each supplied by the corpus and
	// each a way this artifact could hold nothing: a column whose ids were all
	// unknown to every document, a kept half drawn from no document, a carried
	// id that is neither derived nor declared, a counted id that is neither,
	// and a rung that opens nothing further. Where a count is zero the arm
	// above it compared nothing and reported PASS.
	if documented == 0 || documentedKept == 0 {
		t.Errorf("%d outward id(s) and %d kept id(s) were found in a document at the tier their column names, so the membership arm read nothing", documented, documentedKept)
	}
	if carriedDerived == 0 || carriedDeclared == 0 {
		t.Errorf("%d carried id(s) are derived and %d are declared endpoints, and the corpus draws both, so one half of what makes a node carried is unexercised", carriedDerived, carriedDeclared)
	}
	if countedPlain == 0 {
		t.Error("no column counts a printed row of its own, so a row moved out of a column's ids could not be seen")
	}
	if answeredBelow == 0 {
		t.Error("no id a rung draws is opened by a rung below it, so a walk that stopped walking would satisfy every guard here")
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

// TestTheRungArtifactIsWhatTheReachPrimitivesAnswer holds every id and every
// figure the artifact writes down to export.ReachOf, export.ResidualOf and
// export.GapOf CALLED HERE, and never to a second run of rungsOf. A rung's
// path and the step that answered it are taken as the artifact states them;
// what that step's columns hold and what marks stand beside them are
// recomputed from the documents, so a perturbation of what the emitter wrote
// down has only one side moving.
//
// WHY THIS IS NOT THE RE-DERIVATION internal/check/check.go REFUSES. Two
// spellings of one computation in one process witness nothing, and this is
// not that. The primitives are held by their own fixture tests over
// hand-written graphs in internal/export, which is the layer below; what is
// spelled a second time here is the ASSEMBLY -- which column is asked of
// which document on which side, which record answers for each id, and which
// figures are written down. A bug in ReachOf is that layer's to catch, and a
// bug in what rungsOf wrote down is this one's.
//
// THE FOUR MUTATIONS IT EXISTS FOR, none of which any other gate sees once
// testdata/rungs.json is regenerated from the mutated walk: an id dropped
// from a column no later step opens, a mark moved to another tier the step
// declares, every derived mark's cents perturbed by one, and an endpoint
// dropped from a residual's ends. The matrix is in
// docs/rung-walk-witness-evidence.md.
//
// THE WINDOW IS REBUILT HERE TOO, and it has to be: a mark is arithmetic
// over the chart a rung leaves on screen, so a replay that asked the
// artifact for that chart would be asking the mutated side for the answer.
// Each rung's chart is this test's own, built from its parent's, and the
// overview the first rungs open from is export.Fold of the year's document.
//
// WHAT IT DOES NOT WITNESS, since the replay takes them as given: which
// rungs exist at all -- the paths and the steps are the artifact's, and
// their completeness is TestTheRungArtifactIsWhatGoComputes' parentage
// guards -- and the primitives themselves being wrong.
func TestTheRungArtifactIsWhatTheReachPrimitivesAnswer(t *testing.T) {
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
	served, ok := built.Files[rungsServedPath]
	if !ok {
		t.Fatalf("buildAll ships no %s, so there is no rung answer to hold to the documents", rungsServedPath)
	}
	var doc rungsDoc
	if err := json.Unmarshal(served, &doc); err != nil {
		t.Fatalf("decode %s: %v", rungsServedPath, err)
	}
	var replayed, outwardIDs, flankIDs, residuals, gaps, markCents, endsPlural int
	for _, col := range doc.Columns {
		stems := stepStemsFor(t, spine, built.Projections, col.Stem)
		read := func(stem string) export.Graph {
			t.Helper()
			g, err := export.DecodeGraph(built.Projections[stem])
			if err != nil {
				t.Fatalf("column %q: %s: %v", col.Stem, stem, err)
			}
			return g
		}
		documents := map[string]export.Graph{}
		of := func(stem string) export.Graph {
			if g, seen := documents[stem]; seen {
				return g
			}
			documents[stem] = read(stem)
			return documents[stem]
		}
		year := of(col.Stem)
		// THE OVERVIEW IS export.Fold OF THE YEAR'S DOCUMENT, which is what
		// shapeFor draws before anything is opened. The primitive is called
		// here rather than overviewOf for the reason the walk is not re-run:
		// overviewOf is the emitter's assembly.
		overview, err := export.Fold(year, spine.RenderTiers)
		if err != nil {
			t.Fatalf("column %q: overview: %v", col.Stem, err)
		}
		// WHAT EACH RUNG LEAVES ON SCREEN AND WHAT IT WAS SHAPED FROM, this
		// test's own, keyed by path so a child reads its parent's: the chart
		// is the window this replay built and from is the document that
		// rung's step drew, which is what a residual's ribbons come off where
		// the window does not draw them.
		screens, froms := map[string]export.Graph{}, map[string]export.Graph{}
		// SHALLOWEST FIRST, so a parent's window exists before its children
		// ask for it. The artifact's own order already is, by path string,
		// and sorting rather than relying on that keeps the replay's
		// prerequisite stated where it is needed.
		order := slices.Clone(col.Rungs)
		slices.SortStableFunc(order, func(a, b rung) int { return len(a.Path) - len(b.Path) })
		for _, r := range order {
			key := strings.Join(r.Path, " > ")
			si := slices.IndexFunc(spine.Steps, func(x export.DrillStep) bool { return x.Key == r.Step })
			if si < 0 {
				t.Errorf("%s %s: step %q is not declared on the spine", col.Stem, key, r.Step)
				continue
			}
			s := spine.Steps[si]
			screen, from := overview, year
			if len(r.Path) > 1 {
				above := strings.Join(r.Path[:len(r.Path)-1], " > ")
				var answered bool
				if screen, answered = screens[above]; !answered {
					t.Errorf("%s %s: opens under the path %q, which this column answers no rung for, so there is no chart to replay it against", col.Stem, key, above)
					continue
				}
				from = froms[above]
			}
			opened, drawing := r.Path[len(r.Path)-1], of(stems[si])
			var ck export.ColumnKey
			if len(s.Gaps) > 0 {
				if ck, err = export.ColumnKeyOf(built.Projections[stems[si]]); err != nil {
					t.Errorf("%s %s: %v", col.Stem, key, err)
					continue
				}
			}
			draws, marks, next, err := replayRung(s, drawing, screen, from, ck, opened)
			if err != nil {
				t.Errorf("%s %s: %v", col.Stem, key, err)
				continue
			}
			screens[key], froms[key] = next, drawing
			replayed++
			if diff := cmp.Diff(draws, r.Draws); diff != "" {
				t.Errorf("%s %s: the columns export.ReachOf answers over the documents are not the ones the artifact writes down (-recomputed +artifact):\n%s", col.Stem, key, diff)
			}
			if diff := cmp.Diff(marks, r.Marks); diff != "" {
				t.Errorf("%s %s: the marks export.ResidualOf and export.GapOf answer are not the ones the artifact writes down (-recomputed +artifact):\n%s", col.Stem, key, diff)
			}
			for _, d := range draws {
				switch d.Role {
				case roleOutward:
					outwardIDs += len(d.IDs) + len(d.Carried)
				case roleFlank:
					flankIDs += len(d.IDs) + len(d.Carried)
				}
			}
			for _, m := range marks {
				switch m.Role {
				case export.RoleResidual:
					residuals++
					if len(m.Ends) > 1 {
						endsPlural++
					}
				case export.RoleGap:
					gaps++
				}
				if m.InCents+m.OutCents != 0 {
					markCents++
				}
			}
		}
	}
	// EACH OF THESE IS A SHAPE IN WHICH THE COMPARISONS ABOVE COMPARE
	// NOTHING, and the corpus supplies the opposite of every one: a replay
	// that answered no rung, columns holding no id on either side of the
	// window, and the two marks, whose cents and whose endpoint list are
	// three of the four mutations this test exists for. A count of zero
	// against any of them is a PASS reported over an empty comparison.
	if replayed == 0 {
		t.Error("no rung was replayed, so nothing here was held to the documents")
	}
	if outwardIDs == 0 || flankIDs == 0 {
		t.Errorf("%d outward id(s) and %d kept id(s) were recomputed, so one side of the window is compared against nothing and an id dropped from it could not be seen", outwardIDs, flankIDs)
	}
	if residuals == 0 || gaps == 0 {
		t.Errorf("%d residual mark(s) and %d gap mark(s) were recomputed, and the corpus draws both, so a mark moved to another declared tier could not be seen", residuals, gaps)
	}
	if markCents == 0 {
		t.Error("no recomputed mark carries a figure, so a perturbed cent could not be seen")
	}
	if endsPlural == 0 {
		t.Error("no recomputed residual names more than one endpoint, so an endpoint dropped from a mark's ends could not be seen")
	}
}

// replayRung is one rung recomputed from the primitives: what every column
// the step declares holds, the marks that stand beside them, and the chart
// the rung leaves on screen for its children to open from. It asks
// rungWalker.answer's question a second time in this test's own words, over
// the same export.ReachOf, export.ResidualOf and export.GapOf.
//
// THE REFUSALS answer MAKES ARE NOT REPEATED -- a neighbour beside the
// opened node in a centre column, a flank that sends nothing in, a mark
// surviving onto a kept tier. A corpus that provoked one of those would have
// failed the walk before an artifact existed to replay, and those shapes
// have refusal tests of their own; what is repeated here is only what gets
// written down.
func replayRung(s export.DrillStep, drawing, screen, from export.Graph, ck export.ColumnKey, opened string) ([]drawnTier, []drawnMark, export.Graph, error) {
	nearIsSource, outward, centre := s.Side == export.SideSource, slices.Clone(s.Tiers), len(s.Keep) > 0
	var keptTiers []int
	if centre {
		var flank bool
		if nearIsSource, outward, flank = export.Flank(s); !flank {
			return nil, nil, export.Graph{}, fmt.Errorf("step %q keeps %v of tiers %v, which is not a flank", s.Key, s.Keep, s.Tiers)
		}
		idx := slices.Index(s.Tiers, s.From)
		if keptTiers = s.Tiers[idx:]; nearIsSource {
			keptTiers = s.Tiers[:idx+1]
		}
	}
	// THE DOCUMENT IS ASKED FOR THE CENTRE AND EVERYTHING BEYOND IT, and the
	// chart on screen for the flank and the centre on the other side, which
	// is the pair of questions windowFor asks.
	half := slices.Clone(outward)
	if centre {
		half = append([]int{s.From}, half...)
	}
	fresh, err := export.ReachOf(drawing, opened, nearIsSource, half)
	if err != nil {
		return nil, nil, export.Graph{}, fmt.Errorf("the document does not answer %q into tiers %v: %w", opened, half, err)
	}
	var kept export.Reach
	if centre {
		if kept, err = export.ReachOf(screen, opened, !nearIsSource, keptTiers); err != nil {
			return nil, nil, export.Graph{}, fmt.Errorf("the chart on screen does not answer the flank of %q: %w", opened, err)
		}
	}
	record := func(g export.Graph) map[string]export.GraphNode {
		byID := map[string]export.GraphNode{}
		for _, n := range g.Nodes {
			byID[n.ID] = n
		}
		return byID
	}
	freshNodes, keptNodes := record(fresh.Drawn), record(kept.Drawn)
	// THE KEPT HALF GOES IN WHOLE AND FIRST, so its record of the opened node
	// is the one the window carries, and the fresh half's ribbons arrive at
	// the ends the fold left them at.
	window := export.IndexGraph(kept.Drawn)
	for _, t := range outward {
		for _, id := range fresh.At[t] {
			window.Add(freshNodes[id])
		}
	}
	if !centre {
		window.Add(freshNodes[opened])
	}
	for _, l := range fresh.Drawn.Links {
		if err := window.Link(l); err != nil {
			return nil, nil, export.Graph{}, fmt.Errorf("the window of %q: %w", opened, err)
		}
	}
	// WHAT THE CLIENT DRAWS BUT DOES NOT COUNT is the document's word and the
	// step's: a node the document marks derived, or an endpoint the step
	// declares a residual for. Sorting the ids first leaves both lists
	// sorted, which is the order the artifact writes them in.
	split := func(ids []string, byID map[string]export.GraphNode) (own, lent []string) {
		own = []string{}
		for _, id := range slices.Sorted(slices.Values(ids)) {
			_, declared := s.Residual[id]
			if byID[id].Derived || declared {
				lent = append(lent, id)
				continue
			}
			own = append(own, id)
		}
		return own, lent
	}
	draws := make([]drawnTier, 0, len(s.Tiers))
	for _, t := range s.Tiers {
		role, ids, byID := roleOutward, fresh.At[t], freshNodes
		switch {
		case centre && t == s.From:
			role, ids, byID = roleCentre, []string{opened}, keptNodes
		case slices.Contains(s.Keep, t):
			role, ids, byID = roleFlank, kept.At[t], keptNodes
		case !slices.Contains(outward, t):
			return nil, nil, export.Graph{}, fmt.Errorf("step %q draws tier %d, which is neither its centre, a flank it keeps, nor a tier it opens into", s.Key, t)
		}
		own, lent := split(ids, byID)
		draws = append(draws, drawnTier{Tier: t, Role: role, IDs: own, Carried: lent})
	}
	// THE MARKS GO ON LAST AND IN ORDER, the residual over the whole spliced
	// window and the gap over the chart the residual left, which is the order
	// shapeFor applies them in. A residual is carried only across a document
	// switch, which is carryResidual's own gate.
	residual := s.Residual
	if s.Projection == "" {
		residual = nil
	}
	var marks []drawnMark
	drawn := window.Graph()
	if c, ok, err := export.ResidualOf(drawn, from, drawing, opened, s.Tiers, residual, s.ResidualGrain); err != nil {
		return nil, nil, export.Graph{}, fmt.Errorf("the residual of %q: %w", opened, err)
	} else if ok {
		if window, err = spliceMark(drawn, c); err != nil {
			return nil, nil, export.Graph{}, fmt.Errorf("the residual of %q: %w", opened, err)
		}
		marks, drawn = append(marks, drawnMark(c.Mark)), window.Graph()
	}
	if c, ok, err := export.GapOf(drawn, from, drawing, ck, opened, s.Tiers, s.Gaps); err != nil {
		return nil, nil, export.Graph{}, fmt.Errorf("the gap of %q: %w", opened, err)
	} else if ok {
		if window, err = spliceMark(drawn, c); err != nil {
			return nil, nil, export.Graph{}, fmt.Errorf("the gap of %q: %w", opened, err)
		}
		marks = append(marks, drawnMark(c.Mark))
	}
	slices.SortFunc(marks, func(a, b drawnMark) int { return strings.Compare(a.ID, b.ID) })
	return draws, marks, window.Graph(), nil
}

// spliceMark is drawn with one mark applied: the ribbons it re-points taken
// out by index, its nodes added and its ribbons merged in.
//
// THIS TEST'S OWN, NOT rungs.go's carry, for the reason the whole replay is
// written twice: a seam shared with the emitter is one a mutation moves both
// sides of. The two drifting apart is a red test rather than a quiet
// agreement.
func spliceMark(drawn export.Graph, c export.Carry) (*export.Chart, error) {
	kept := export.Graph{Nodes: drawn.Nodes}
	for i, l := range drawn.Links {
		if !slices.Contains(c.Splice, i) {
			kept.Links = append(kept.Links, l)
		}
	}
	chart := export.IndexGraph(kept)
	for _, n := range c.Nodes {
		chart.Add(n)
	}
	for _, l := range c.Links {
		if err := chart.Link(l); err != nil {
			return nil, err
		}
	}
	return chart, nil
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

// TestRungsRefuseADriftTheStepDoesNotDeclare is the refusals a declaration
// can provoke, inert on the committed declarations and shown firing on a
// one-field change: the object-category step licensing a gap on another node
// alone, or services-and-supplies' gap in the column that ties, or at another
// figure, while p.67 and pp.85-125 still print FY 2026-27's cell 250,000
// dollars apart.
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
			s.Gaps = map[string]export.Gaps{"expenditure/debt-services": {{FiscalYear: 1999, Basis: "adopted", Cents: 1, Reason: "A reason for a column not drawn."}}}
		},
			`opens "expenditure/services-and-supplies": the chart above sends 13050208700 into "expenditure/services-and-supplies" and this one draws 13025208700 of it, a difference of 25000000 cents that no declaration on this step accounts for in FY2027 adopted`},
		{"the gap licensed in the other column", "object-category", func(s *export.DrillStep) {
			g := s.Gaps["expenditure/services-and-supplies"]
			s.Gaps = map[string]export.Gaps{"expenditure/services-and-supplies": {{FiscalYear: 2026, Basis: g[0].Basis, Cents: g[0].Cents, Reason: g[0].Reason}}}
		},
			`declares a gap of 25000000 cents on "expenditure/services-and-supplies" in FY2026 adopted and the chart balances there`},
		{"the gap licensed at another figure", "object-category", func(s *export.DrillStep) {
			g := s.Gaps["expenditure/services-and-supplies"]
			s.Gaps = map[string]export.Gaps{"expenditure/services-and-supplies": {{FiscalYear: g[0].FiscalYear, Basis: g[0].Basis, Cents: g[0].Cents + 1, Reason: g[0].Reason}}}
		},
			`declares a gap of 25000001 cents on "expenditure/services-and-supplies" in FY2027 adopted and the charts differ there by 25000000`},
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

// stepStemsFor is the test's own resolution of what each step draws for one
// year, asked of the index the packager resolves through. It is a wrapper and
// not a second spelling: export.StepStems is the function under test's own.
func stepStemsFor(t *testing.T, spine export.View, projections map[string][]byte, year string) []string {
	t.Helper()
	_, ix, err := export.ColumnsOf(projections, "fisc test")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	stems, err := export.StepStems(spine.Steps, year, ix)
	if err != nil {
		t.Fatalf("StepStems for %q: %v", year, err)
	}
	return stems
}

// TestTheFundStepsSentenceIsItsArithmetic holds the fund step's Description
// to the identity it states, over every published column. What fund/100
// takes in less what its divisions draw is what pp.66-67 print leaving the
// general group other than through its divisions -- transfers out and the
// fund-balance rows -- LESS the residual the group's own rung carries in,
// read off the rungs artifact rather than recomputed. Measured off this
// test, in cents: 1,322,266,800 = 1,473,722,200 - 151,455,400 in FY2025-26
// and 1,534,356,800 = 1,583,030,300 - 48,673,500 in FY2026-27. A sentence
// naming the first term alone, which is what the old wording did, is off by
// the residual in both years, and the words are pinned here beside the
// arithmetic so that neither can move without the other.
func TestTheFundStepsSentenceIsItsArithmetic(t *testing.T) {
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
	si := slices.IndexFunc(spine.Steps, func(s export.DrillStep) bool { return s.Key == "fund" })
	if si < 0 {
		t.Fatal("no fund step on the spine")
	}
	const twoTerms = "less the money the group takes in that no fund receives"
	if !strings.Contains(spine.Steps[si].Description, twoTerms) {
		t.Fatalf("the fund step's description does not say %q, so the arithmetic below is not what the reader is told", twoTerms)
	}
	var doc rungsDoc
	if err := json.Unmarshal(built.Files[rungsServedPath], &doc); err != nil {
		t.Fatalf("decode %s: %v", rungsServedPath, err)
	}
	const group, fund = "fund-group/general", "fund/100"
	checked := 0
	for _, col := range doc.Columns {
		stems := stepStemsFor(t, spine, built.Projections, col.Stem)
		year, err := export.DecodeGraph(built.Projections[col.Stem])
		if err != nil {
			t.Fatalf("%s: %v", col.Stem, err)
		}
		drawn, err := export.DecodeGraph(built.Projections[stems[si]])
		if err != nil {
			t.Fatalf("%s: %s: %v", col.Stem, stems[si], err)
		}
		var in, out, leaving int64
		for _, l := range drawn.Links {
			switch {
			case l.Source == group && l.Target == fund:
				in += l.ValueCents
			case l.Source == fund:
				out += l.ValueCents
			}
		}
		for _, l := range year.Links {
			if l.Source == group && !strings.HasPrefix(l.Target, "expenditure/") {
				leaving += l.ValueCents
			}
		}
		ri := slices.IndexFunc(col.Rungs, func(r rung) bool { return slices.Equal(r.Path, []string{group}) })
		if ri < 0 {
			t.Fatalf("%s: no rung opens %s", col.Stem, group)
		}
		var residual int64
		for _, m := range col.Rungs[ri].Marks {
			if m.Role == export.RoleResidual {
				residual += m.InCents
			}
		}
		if in == 0 || out == 0 || leaving == 0 || residual == 0 {
			t.Fatalf("%s: in %d, out %d, leaving %d, residual %d; a zero term proves nothing", col.Stem, in, out, leaving, residual)
		}
		t.Logf("%s: fund/100 takes %d and its divisions draw %d, a difference of %d; the group's transfers out and fund-balance rows come to %d less %d carried in, which is %d",
			col.Stem, in, out, in-out, leaving, residual, leaving-residual)
		if in-out != leaving-residual {
			t.Errorf("%s: the fund step's sentence states in - out = leaving - residual, and %d - %d != %d - %d",
				col.Stem, in, out, leaving, residual)
		}
		checked++
	}
	if checked != len(doc.Columns) || checked == 0 {
		t.Fatalf("checked %d of %d columns", checked, len(doc.Columns))
	}
}

// TestAGapIsAnsweredOnlyOnTheColumnItsExceptionPins holds each column's gap
// marks to the exceptions structure declares, per (year, basis, cell): the
// FY2025-26 column answers no gap, because no pin names it, and the
// FY2026-27 column answers services-and-supplies at the pin's own difference
// rather than at a literal. The licence GapOf reads is keyed by node alone
// (fisc-sixz), so this is the arm that sees a gap drawn on a column the
// exception does not pin, which the artifact would otherwise carry with the
// other year's prose.
func TestAGapIsAnsweredOnlyOnTheColumnItsExceptionPins(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	var doc rungsDoc
	if err := json.Unmarshal(built.Files[rungsServedPath], &doc); err != nil {
		t.Fatalf("decode %s: %v", rungsServedPath, err)
	}
	pinned := 0
	for _, col := range doc.Columns {
		var meta struct {
			Column struct {
				FiscalYear int    `json:"fiscal_year"`
				Basis      string `json:"basis"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(built.Projections[col.Stem], &meta); err != nil {
			t.Fatalf("%s: %v", col.Stem, err)
		}
		if meta.Column.FiscalYear == 0 || meta.Column.Basis == "" {
			t.Fatalf("%s: the spine document names no fiscal year or basis, so no pin can be matched to it", col.Stem)
		}
		want := map[string]int64{}
		for _, e := range structure.BudgetBookExceptions() {
			if e.Cut != "departmentwide" || e.Against != "spine" || e.At != structure.LevelCategory {
				continue
			}
			for _, p := range e.Cells {
				if p.Year == meta.Column.FiscalYear && p.Basis == meta.Column.Basis {
					want["expenditure/"+p.Coords[structure.AxisCategory]] = p.Against.Cents - p.Cut.Cents
					pinned++
				}
			}
		}
		got := map[string]int64{}
		for _, r := range col.Rungs {
			for _, m := range r.Marks {
				if m.Role == export.RoleGap {
					got[r.Path[len(r.Path)-1]] = m.InCents - m.OutCents
				}
			}
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s (FY%d %s): the gap marks answered are not the cells the exceptions pin for this column (-pinned +answered):\n%s",
				col.Stem, meta.Column.FiscalYear, meta.Column.Basis, diff)
		}
	}
	if pinned == 0 {
		t.Fatal("no exception pins a departmentwide cell on any published column, so no gap was held to anything")
	}
}

// TestARungsAmountIsTheFigureTheOverviewLabelsTheNodeWith is what makes
// rung.Amounts evidence rather than a second spelling of the drawing.
//
// THE WHOLE POINT OF THE FIELD IS THAT THE RIBBONS DISAGREE WITH IT. A
// category a schedule prints a reduction under has its reduction drawn forward
// at its magnitude, so the ribbons arriving at the mark come to the figure plus
// twice the reductions -- 10,343,009,200 against a published 6,945,941,400 on
// FY2025-26. An amount re-derived from those same ribbons by the test would
// agree with the walk by construction and witness nothing.
//
// SO IT IS COMPARED AGAINST THE OTHER DOCUMENT. The spine publishes that node's
// own cell one click earlier, from pp.66-67, and the drill-down reads
// pp.127-140: two schedules, independently mapped, and the amount is right only
// if it is the figure the reader was just shown. That is the comparison, and it
// is the same one the window itself makes -- the two sides of a category mark
// are one figure read from two schedules.
func TestARungsAmountIsTheFigureTheOverviewLabelsTheNodeWith(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}

	// The spine's own cell for a node: what the overview labels it with, which
	// is everything leaving it at the citywide grain.
	spineCell := func(t *testing.T, stem, node string) int64 {
		t.Helper()
		raw, ok := built[stem]
		if !ok {
			t.Fatalf("buildProjections did not build %q", stem)
		}
		g, decodeErr := export.DecodeGraph(raw)
		if decodeErr != nil {
			t.Fatalf("decode %s: %v", stem, decodeErr)
		}
		var sum int64
		seen := false
		for _, l := range g.Links {
			if l.Source == node {
				sum += l.ValueCents
				seen = true
			}
		}
		if !seen {
			t.Fatalf("the %s spine draws nothing out of %q", stem, node)
		}
		return sum
	}

	raw, err := os.ReadFile(filepath.Join(root, rungsPath))
	if err != nil {
		t.Fatalf("read %s: %v", rungsPath, err)
	}
	var art struct {
		Columns []struct {
			Stem  string `json:"stem"`
			Rungs []struct {
				Path    []string         `json:"path"`
				Amounts map[string]int64 `json:"amounts"`
			} `json:"rungs"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(raw, &art); err != nil {
		t.Fatalf("decode %s: %v", rungsPath, err)
	}

	named := 0
	for _, col := range art.Columns {
		for _, r := range col.Rungs {
			for node, cents := range r.Amounts {
				named++
				if want := spineCell(t, col.Stem, node); cents != want {
					t.Errorf("%s rung %v names %s at %d; the spine publishes that node's own "+
						"cell at %d. The two are read from different schedules and the "+
						"window exists to show they agree",
						col.Stem, r.Path, node, cents, want)
				}
			}
		}
	}
	// THE VACUITY GUARD, in this file's own idiom: an artifact naming no amount
	// would pass every line above. Budget Book p127 prints ERAF and the RPTTF
	// reduction under Property Taxes, and both published spine years open into
	// that category, so the committed corpus supplies exactly two.
	if named != 2 {
		t.Errorf("%d amount(s) named across the artifact, want 2 -- Property Taxes on each "+
			"published year. A walk naming none would report nothing and pass", named)
	}
}

// TestARungsAmountOfZeroIsRefusedAtTheWrite holds the schema's lower bound on
// a rung's amounts: a mark drawn at zero or below sets the scale for its
// whole column, and the client draws whatever figure it is answered.
//
// OVER THE BUILT ANSWER AND NOT A LITERAL, so the document that reaches the
// schema is one every other key of which is what buildAll writes; the one
// change is the figure.
func TestARungsAmountOfZeroIsRefusedAtTheWrite(t *testing.T) {
	built, err := buildAll(repoRootForTest(t))
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	spine, err := spineView(built)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []int64{0, -1} {
		rungs, err := rungsOf(built.Projections, spine)
		if err != nil {
			t.Fatalf("rungsOf: %v", err)
		}
		if _, err = encodeRungs(rungs); err != nil {
			t.Fatalf("the built answer does not encode: %v", err)
		}
		changed := ""
		for _, col := range rungs.Columns {
			for _, r := range col.Rungs {
				for id := range r.Amounts {
					r.Amounts[id] = bad
					changed = col.Stem + " " + strings.Join(r.Path, " > ") + " " + id
					break
				}
				if changed != "" {
					break
				}
			}
			if changed != "" {
				break
			}
		}
		if changed == "" {
			t.Fatal("no rung answers an amount, so the bound cannot be exercised")
		}
		_, err = encodeRungs(rungs)
		if err == nil {
			t.Errorf("an amount of %d at %s was written; the client would draw a mark at that figure", bad, changed)
			continue
		}
		if !strings.Contains(err.Error(), "amounts") {
			t.Errorf("the refusal of an amount of %d does not name amounts: %v", bad, err)
		}
	}
}

// TestAMarkWithoutItsWordsIsRefusedAtTheWrite holds the schema's per-role
// requirements on a mark: a gap states what its figure is and cites the pages
// of both totals, and a residual names the endpoints it carries. Each row
// blanks one field of one mark of the built answer, which encodes whole.
func TestAMarkWithoutItsWordsIsRefusedAtTheWrite(t *testing.T) {
	built, err := buildAll(repoRootForTest(t))
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	spine, err := spineView(built)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, role string
		blank      func(*drawnMark)
	}{
		{"a gap with no source note", export.RoleGap, func(m *drawnMark) { m.SourceNote = "" }},
		{"a gap citing no page", export.RoleGap, func(m *drawnMark) { m.Locators = nil }},
		{"a gap with no rationale", export.RoleGap, func(m *drawnMark) { m.Rationale = "" }},
		{"a residual naming no endpoint", export.RoleResidual, func(m *drawnMark) { m.Ends = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rungs, err := rungsOf(built.Projections, spine)
			if err != nil {
				t.Fatalf("rungsOf: %v", err)
			}
			if _, err = encodeRungs(rungs); err != nil {
				t.Fatalf("the built answer does not encode: %v", err)
			}
			blanked := false
			for _, col := range rungs.Columns {
				for _, r := range col.Rungs {
					for i := range r.Marks {
						if !blanked && r.Marks[i].Role == tc.role {
							tc.blank(&r.Marks[i])
							blanked = true
						}
					}
				}
			}
			if !blanked {
				t.Fatalf("no rung answers a %s mark, so nothing was blanked", tc.role)
			}
			if _, err = encodeRungs(rungs); err == nil || !strings.Contains(err.Error(), "does not match") {
				t.Errorf("encodeRungs with %s: err = %v, want the schema's refusal", tc.name, err)
			}
		})
	}
}
