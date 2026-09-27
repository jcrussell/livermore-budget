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
// buildAll serves at rungsServedPath, rebuilt and compared rather than
// updated in place, and refuses an artifact the client's tests could pass
// against vacuously. The membership guards read the documents and the step
// declarations, never the walk; a mark's cents, tier and endpoints, and an id
// dropped from a column no later step opens, are
// TestTheRungArtifactIsWhatTheReachPrimitivesAnswer's. Which guard took which
// mutation is in docs/rung-walk-witness-evidence.md.
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
	// The stamp carries the commit and build date, so it is blanked on both sides.
	stamp := regexp.MustCompile(`"generated_by": "[^"]*"`)
	blank := []byte(`"generated_by": ""`)
	if readErr != nil || !bytes.Equal(stamp.ReplaceAll(got, blank), stamp.ReplaceAll(want, blank)) {
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
	// A pinned zero: no step declares both marks, so their order is unwitnessed.
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
		// Each step's document for this year, plus the year's own, which the
		// overview was folded from.
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
		// export.Openable is what the walk asks too, so the completeness guard
		// below is completeness from the parent, not an independent reading.
		opens := make([][]string, len(spine.Steps))
		for i, st := range spine.Steps {
			parents, err := export.FlankDocuments(spine, st, col.Stem, stems, built.Projections)
			if err != nil {
				t.Fatalf("column %q: step %q: %v", col.Stem, st.Key, err)
			}
			o, err := export.Openable(spine, i, st, stems[i], built.Projections[stems[i]], parents)
			if err != nil {
				t.Fatalf("column %q: step %q: %v", col.Stem, st.Key, err)
			}
			opens[i] = o
		}
		// Keyed by path: the second pass asks about a rung's parent.
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
			// An outward id is held to this step's document; a kept id only to
			// some document this column reads, the weaker claim, because the
			// chart on screen holds nodes this step's document need not
			// (docs/rung-walk-witness-evidence.md).
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
			// Every declared column is answered, in order, empty rather than dropped.
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
				if d.IDs == nil {
					at("tier %d's ids are omitted rather than empty", d.Tier)
				}
				if !slices.IsSorted(d.IDs) || len(slices.Compact(slices.Clone(d.IDs))) != len(d.IDs) {
					at("tier %d's ids are not sorted and unique: %v", d.Tier, d.IDs)
				}
				if !slices.IsSorted(d.Carried) || len(slices.Compact(slices.Clone(d.Carried))) != len(d.Carried) {
					at("tier %d's carried ids are not sorted and unique: %v", d.Tier, d.Carried)
				}
				// A declared endpoint is carried or absent, never counted.
				for _, id := range d.IDs {
					if _, declared := s.Residual[id]; declared {
						at("tier %d counts %q as a part of the opened node, and the step declares it a residual endpoint", d.Tier, id)
					}
					if slices.Contains(d.Carried, id) {
						at("tier %d both counts and carries %q", d.Tier, id)
					}
					distinct[id] = true
				}
				// A tier is the documents' property, not the walk's, so this
				// refuses an id moved to a column the document does not print it in.
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
					// Carried iff the document marks it derived or the step
					// declares it an endpoint.
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
				if c, capped := capOf(s, d.Tier); capped && drawn > c {
					overCap++
				} else if !capped && len(d.IDs) > 0 {
					uncappedIDs++
				}
			}
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
			// The rung above holds the node this one opened.
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
			// The mirror, which a walk that stopped walking fails: each id a
			// later step opens is answered by a rung one path longer. It
			// cannot witness export.Openable itself being wrong (fisc-u8di).
			for j, s2 := range spine.Steps {
				if !slices.Contains(s2.After, r.Step) {
					continue
				}
				for _, d := range r.Draws {
					if d.Tier != s2.From {
						continue
					}
					// A pinned zero: no shipped step opens a kept column, and
					// recordOf has no role to match a kept id on.
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
	// Vacuity guards: each zero is a shape the arms above pass without comparing.
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
	if flankPlural == 0 {
		t.Error("no flank draws more than one id, so an id perturbed on a flank could not be seen")
	}
	if flankCarried == 0 {
		t.Error("no flank carries a declared endpoint, so the residual's two spine feeders are compared on no column")
	}
	if emptyFlank == 0 {
		t.Error("no flank draws nothing of its own, so an omitted ids would be indistinguishable from an empty one")
	}
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
	// The gap is p0067's services-and-supplies against pp.85-125's rows in
	// FY2026-27; FY2025-26 ties to the cent.
	if len(gapYears) == 0 {
		t.Error("no column answers a gap mark, so the arm has no gap to hold the client to")
	}
	// Both years have a fund group whose draw or transfers in pp.127-140 print
	// for no fund.
	if len(residualYears) != len(doc.Columns) {
		t.Errorf("%d of %d columns answer a residual mark, and every published column carries a group the fund schedule does not fully decompose", len(residualYears), len(doc.Columns))
	}
}

// TestTheRungArtifactIsWhatTheReachPrimitivesAnswer holds every id and figure
// the artifact writes down to export.ReachOf, export.ResidualOf and
// export.GapOf called here, never to a second run of rungsOf, so a
// perturbation of the emitter has one side moving. What is spelled twice is
// the assembly, not the primitives, which have fixture tests of their own. The
// window is rebuilt here too, since a mark is arithmetic over it. It takes
// which rungs exist as given. Its four mutations are in
// docs/rung-walk-witness-evidence.md.
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
		// The primitive, not overviewOf, which is the emitter's assembly.
		overview, err := export.Fold(year, spine.RenderTiers)
		if err != nil {
			t.Fatalf("column %q: overview: %v", col.Stem, err)
		}
		// Keyed by path so a child reads its parent's chart and document.
		screens, froms := map[string]export.Graph{}, map[string]export.Graph{}
		// Shallowest first, so a parent's window exists before its children.
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
	// Vacuity guards: each zero is a comparison above that compared nothing.
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

// replayRung is one rung recomputed from the primitives: its columns, its
// marks, and the chart it leaves on screen for its children. rungWalker.answer's
// refusals are not repeated; they have tests of their own.
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
	// The kept half goes in first, so its record of the opened node wins.
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
	// Needs, keeping the widened columns from the front: an outward id first
	// reached once Widen[k] is kept needs Widen[k].
	reachedWith := func(k int) (export.Reach, error) {
		narrow := slices.DeleteFunc(slices.Clone(half), func(t int) bool { return slices.Contains(s.Widen[k:], t) })
		return export.ReachOf(drawing, opened, nearIsSource, narrow)
	}
	for i := range draws {
		if draws[i].Role != roleOutward || slices.Contains(s.Widen, draws[i].Tier) {
			continue
		}
		for _, id := range slices.Concat(draws[i].IDs, draws[i].Carried) {
			for k := 0; k < len(s.Widen); k++ {
				r, err := reachedWith(k)
				if err != nil {
					return nil, nil, export.Graph{}, err
				}
				if slices.Contains(r.At[draws[i].Tier], id) {
					break
				}
				next, err := reachedWith(k + 1)
				if err != nil {
					return nil, nil, export.Graph{}, err
				}
				if slices.Contains(next.At[draws[i].Tier], id) {
					if draws[i].Needs == nil {
						draws[i].Needs = map[string]int{}
					}
					draws[i].Needs[id] = s.Widen[k]
					break
				}
			}
		}
	}
	// Residual then gap, shapeFor's order; a residual only across a document switch.
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

// spliceMark is drawn with one mark applied. It is this test's own rather
// than the emitter's, so a mutation there moves only one side.
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

// TestRungsAnswerAColumnTheDocumentDrawsNothingIn: a declared column the
// document folds no node to is answered with an empty id list, not dropped.
// Hand-written because no committed document supplies the shape.
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

// TestRungsRefuseADriftTheStepDoesNotDeclare shows each gap refusal firing on
// a one-field change to the committed declaration. Dropping the whole map is
// not the mutation: a step declaring no gap makes no balance claim.
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

// TestRungsRefuseAWindowTheClientWouldNotDraw shows the flank refusal in
// rungWalker.answer firing on a one-field change to the shipped spine.
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

// TestRungsRefuseAKeptHalfTheArtifactHasNoShapeFor: a second node beside the
// opened one in the kept centre column is refused. The whole walk runs,
// because the shape comes from the overview's fold.
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

// stepStemsFor is export.StepStems for one year over the packager's index.
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
// to the identity it states in every published column: what fund/100 takes in
// less what its divisions draw equals what pp.66-67 print leaving the general
// group other than through its divisions, less the residual its rung carries
// in. The words and the arithmetic are pinned together.
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

// proseDenial is the rule the served prose is held to: a step description,
// a residual or gap reason or a caveat says what its own chart draws and
// from which pages, and denies nothing about what any other schedule prints.
// It is a tripwire, not a parser: it does not chase paraphrases.
var proseDenial = regexp.MustCompile(`(?i)no published|not broken down|no schedule|nothing published|prints? none|in any published|nowhere`)

// residualFigure is a figure in a residual reason, once its page citations
// are gone: one reason is shown under every column, so any figure in it is
// one column's.
var (
	pageCite       = regexp.MustCompile(`\bpp?\.\s?\d+(-\d+)?`)
	residualFigure = regexp.MustCompile(`(?i)[\d$]|\b(hundred|thousand|million|billion|dollars?)\b`)
)

// TestServedProseDeniesNothingAndResidualsQuoteNoFigure holds the text of
// every HTML page and every string in every JSON file one real export serves
// to proseDenial, and every residual reason to residualFigure. Page texts and
// fact records are the city's own words and are not read.
func TestServedProseDeniesNothingAndResidualsQuoteNoFigure(t *testing.T) {
	dir := exportedSite(t)
	type text struct{ where, words string }
	var prose []text
	var strs func(where string, v any)
	strs = func(where string, v any) {
		switch v := v.(type) {
		case string:
			prose = append(prose, text{where, v})
		case []any:
			for _, e := range v {
				strs(where, e)
			}
		case map[string]any:
			for k, e := range v {
				strs(where+" "+k, e)
			}
		}
	}
	pages, docs := 0, 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		raw, err := os.ReadFile(path) // #nosec G304 -- a temp dir this test wrote.
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".html":
			pages++
			prose = append(prose, text{rel, htmlTag.ReplaceAllString(htmlStyle.ReplaceAllString(string(raw), " "), " ")})
		case ".json":
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			docs++
			strs(rel, v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := buildAll(repoRootForTest(t))
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	var residuals []text
	for _, v := range views(built) {
		for _, s := range v.Steps {
			for id, why := range s.Residual {
				residuals = append(residuals, text{v.Path + " step " + s.Key + " residual " + id, why})
			}
		}
	}
	if pages == 0 || docs == 0 || len(residuals) == 0 {
		t.Fatalf("%d pages, %d JSON files and %d residual reasons served; the rule reached nothing", pages, docs, len(residuals))
	}
	for _, p := range prose {
		if m := proseDenial.FindString(p.words); m != "" {
			i := strings.Index(p.words, m)
			t.Errorf("%s says %q: ...%s...", p.where, m, p.words[max(0, i-120):min(len(p.words), i+120)])
		}
	}
	for _, r := range residuals {
		if m := residualFigure.FindString(pageCite.ReplaceAllString(r.words, "")); m != "" {
			t.Errorf("%s quotes %q, which is one column's: %s", r.where, m, r.words)
		}
	}
}

var (
	htmlStyle = regexp.MustCompile(`(?is)<style.*?</style>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
)

// TestAGapIsAnsweredOnlyOnTheColumnItsExceptionPins holds each column's gap
// marks to the exceptions structure pins per (year, basis, cell). GapOf's
// licence is keyed by node alone (fisc-sixz), so this is what sees a gap drawn
// on a column no exception pins.
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

// TestARungsAmountIsTheFigureTheOverviewLabelsTheNodeWith holds rung.Amounts to
// the spine's own cell from pp.66-67, not to the drill-down's ribbons, which
// disagree with it by twice the reductions drawn forward. Two independently
// mapped schedules must agree.
func TestARungsAmountIsTheFigureTheOverviewLabelsTheNodeWith(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}

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
	// Budget Book p127 prints reductions under Property Taxes, opened in both years.
	if named != 2 {
		t.Errorf("%d amount(s) named across the artifact, want 2 -- Property Taxes on each "+
			"published year. A walk naming none would report nothing and pass", named)
	}
}

// TestARungsAmountOfZeroIsRefusedAtTheWrite holds the schema's lower bound on
// a rung's amounts, over the built answer with only the figure changed.
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
	var gapCites []export.Locator
	{
		rungs, err := rungsOf(built.Projections, spine)
		if err != nil {
			t.Fatalf("rungsOf: %v", err)
		}
		for _, col := range rungs.Columns {
			for _, r := range col.Rungs {
				for _, m := range r.Marks {
					if m.Role == export.RoleGap && gapCites == nil {
						gapCites = m.Locators
					}
				}
			}
		}
		if len(gapCites) == 0 {
			t.Fatal("no gap mark cites a page, so there is no locator to plant on a residual")
		}
	}
	for _, tc := range []struct {
		name, role string
		blank      func(*drawnMark)
	}{
		{"a gap with no source note", export.RoleGap, func(m *drawnMark) { m.SourceNote = "" }},
		{"a gap citing no page", export.RoleGap, func(m *drawnMark) { m.Locators = nil }},
		{"a gap with no rationale", export.RoleGap, func(m *drawnMark) { m.Rationale = "" }},
		{"a residual naming no endpoint", export.RoleResidual, func(m *drawnMark) { m.Ends = nil }},
		{"a residual citing locators of its own", export.RoleResidual, func(m *drawnMark) { m.Locators = gapCites }},
		{"a gap naming an endpoint", export.RoleGap, func(m *drawnMark) { m.Ends = []string{"transfers/in"} }},
		{"a gap on both sides", export.RoleGap, func(m *drawnMark) { m.InCents, m.OutCents = 1, 1 }},
		{"a gap on neither side", export.RoleGap, func(m *drawnMark) { m.InCents, m.OutCents = 0, 0 }},
		{"a gap of negative cents", export.RoleGap, func(m *drawnMark) { m.InCents, m.OutCents = -m.InCents-m.OutCents, 0 }},
		{"a gap of negative out cents", export.RoleGap, func(m *drawnMark) { m.InCents, m.OutCents = 0, -m.InCents-m.OutCents }},
		{"a residual on neither side", export.RoleResidual, func(m *drawnMark) { m.InCents, m.OutCents = 0, 0 }},
		{"a residual of negative in cents", export.RoleResidual, func(m *drawnMark) { m.InCents = -m.InCents - m.OutCents }},
		{"a residual of negative out cents", export.RoleResidual, func(m *drawnMark) { m.OutCents = -m.InCents - m.OutCents }},
		{"a residual with a note of its own", export.RoleResidual, func(m *drawnMark) { m.SourceNote = "Derived." }},
		{"a gap at a tier the rung draws no column at", export.RoleGap, func(m *drawnMark) { m.Tier = 99 }},
		{"a residual at a tier the rung draws no column at", export.RoleResidual, func(m *drawnMark) { m.Tier = 99 }},
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
			want := "does not match"
			if strings.Contains(tc.name, "tier") {
				want = "which the rung draws no column at"
			}
			if _, err = encodeRungs(rungs); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("encodeRungs with %s: err = %v, want one containing %q", tc.name, err, want)
			}
		})
	}
}
