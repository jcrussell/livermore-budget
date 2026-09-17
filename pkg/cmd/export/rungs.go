package export

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// rungsPath is the committed artifact tools/jscheck/rungs.mjs reads: Go's
// answer for every rung the drill walks, at every column budget, so the
// client's answer can be held to it. Relative to the repository root.
const rungsPath = "testdata/rungs.json"

// rungWidths are the column budgets the artifact answers for: the narrow
// window every page opens at, and the one a fourth column is bought on.
var rungWidths = []int{3, 4}

// rungsDoc is the artifact. It is Go's reading of the declared steps against
// the documents they draw, and NOT a re-encoding of the declarations: every
// figure in it is computed by export.ReachOf over a document, so a comparison
// against it is a comparison against what Go says the chart holds.
type rungsDoc struct {
	SchemaVersion int `json:"schema_version"`
	// Skipped is every step the artifact does not enumerate, by key and with
	// the reason. THE WALK SKIPS NOTHING, so it is written empty: the arm
	// reading it treats a rung under any other step as one Go must answer,
	// and a step that ever has to be left out again must be named here, by
	// this walk, for the arm to let it by. It is not omitted when empty so
	// that a reader of the file sees the allowance is empty rather than
	// wondering whether it moved.
	Skipped []skippedStep `json:"skipped"`
	// Reasons is the one sentence behind each drawnTier.Unanswered code, so a
	// column Go declines to answer says why once rather than on every rung,
	// and an arm reading a code this table does not carry can refuse it.
	Reasons map[string]string `json:"reasons"`
	Columns []rungColumn      `json:"columns"`
}

type skippedStep struct {
	Step   string `json:"step"`
	Reason string `json:"reason"`
}

// rungColumn is one published year, by the spine document's stem, which is
// how tools/jscheck names a column.
type rungColumn struct {
	Stem  string `json:"stem"`
	Rungs []rung `json:"rungs"`
}

// A rung is one opened path at one column budget: the step that opened its
// last node and, for every column the window draws, in the order it draws
// them, what that column holds.
type rung struct {
	Path  []string    `json:"path"`
	Width int         `json:"width"`
	Step  string      `json:"step"`
	Draws []drawnTier `json:"draws"`
}

// drawnTier is one column of one rung. ONE LIST IN COLUMN ORDER, rather than
// a tier list beside a cap list beside an id list: three spellings of one
// shape drift, and revenue-category's order is [1,0,2], which no
// centre-first convention would have spelled.
//
// Role is centre for the opened node's own column, flank for a column of
// s.Keep, and outward for one the step opens the node into. A flank carries
// Unanswered and nothing else: it is the half of the chart on screen that
// the client's windowFor carries over, which this walk does not model, and
// an empty IDs beside the reason would invite a green against an empty
// client column. Cap is 0 where the step declares none, which is the value
// validateSteps refuses on a declared cap.
//
// Candidates is how many document nodes the window reaches at the tier;
// IDs, sorted, is which of them the column draws as themselves and Hidden
// how many the folded tail stands for. THE FOLD ENGAGES ONLY ABOVE CAP+1,
// because folding one node into a tail of one draws the same number of
// marks and loses a name. That threshold is the client's, measured at the
// shipped site: special-revenue's 32 funds draw eight and a tail of 24, and
// a column of nine under a cap of eight draws all nine.
type drawnTier struct {
	Tier       int      `json:"tier"`
	Role       string   `json:"role"`
	Unanswered string   `json:"unanswered,omitempty"`
	Cap        int      `json:"cap,omitempty"`
	Candidates int      `json:"candidates,omitempty"`
	IDs        []string `json:"ids,omitempty"`
	Hidden     int      `json:"hidden,omitempty"`
}

const (
	roleCentre  = "centre"
	roleFlank   = "flank"
	roleOutward = "outward"
)

// reasonKeptFlank is the one Unanswered code the walk emits today, keyed
// into rungsDoc.Reasons.
const reasonKeptFlank = "kept-flank"

var rungReasons = map[string]string{
	reasonKeptFlank: "the column is the kept flank of the chart on screen, which the client's " +
		"windowFor carries over from the rung above rather than reading off the document this " +
		"step draws; this walk reads only that document, so it does not say which nodes the " +
		"flank holds",
}

// rungsOf walks the spine's declared steps over the built documents, for
// every published year and every budget in rungWidths, and answers each rung
// it reaches.
//
// WHAT A RUNG OFFERS IS READ THE WAY THE CLIENT READS IT: a node opens under
// the first step whose After names the chart on screen, whose From is the
// node's tier, whose Role -- if it declares one -- is the node's own role as
// the document printed it, and whose document decomposes the node, which is
// export.Openable's answer and the one stepView.Opens ships.
//
// WHAT A RUNG DRAWS IS READ THE WAY THE CLIENT DRAWS IT, through the
// hierarchy and not along the ribbons: export.ReachOf keeps the ribbons whose
// near end is inside the opened node by parent chain, folds them to the drawn
// tiers and prunes what nothing touches. A step that keeps a flank is asked
// for the half beyond its centre, the way windowFor asks its document; a step
// that keeps none is asked for every column it draws, on the side it
// declares, the way sideOf does -- so the transfers step, whose opened node
// touches no ribbon and whose payer ends are its children, is answered like
// any other rather than skipped.
//
// WHAT A RUNG DRAWS IS DOCUMENT NODES. The client adds derived nodes of its
// own -- a residual, a gap, a folded tail -- and this walk counts none of
// them, so the day one of them competes with a cap the arm reading this
// artifact goes red rather than this function guessing which side wins.
func rungsOf(projections map[string][]byte, spine export.View) (rungsDoc, error) {
	doc := rungsDoc{SchemaVersion: 2, Skipped: []skippedStep{}, Reasons: rungReasons}
	for _, year := range spine.YearStems {
		raw, ok := projections[year]
		if !ok {
			return rungsDoc{}, fmt.Errorf("rungs: the spine's year %q was not built", year)
		}
		chart, err := export.DecodeGraph(raw)
		if err != nil {
			return rungsDoc{}, fmt.Errorf("rungs: %s: %w", year, err)
		}
		stems := stepStemsFor(spine, year)
		col := rungColumn{Stem: year}
		for _, width := range rungWidths {
			w := rungWalker{spine: spine, projections: projections, stems: stems, width: width}
			if err := w.walk(chart.Nodes, "", nil, &col.Rungs); err != nil {
				return rungsDoc{}, fmt.Errorf("rungs: %s at %d columns: %w", year, width, err)
			}
		}
		sort.Slice(col.Rungs, func(i, j int) bool {
			a, b := col.Rungs[i], col.Rungs[j]
			if a.Width != b.Width {
				return a.Width < b.Width
			}
			return strings.Join(a.Path, " > ") < strings.Join(b.Path, " > ")
		})
		doc.Columns = append(doc.Columns, col)
	}
	return doc, nil
}

// stepStemsFor is the document each step draws for one year: its own for the
// year, or the step before it's where it names none -- the same resolution
// the packager makes when it ships one entry per step.
func stepStemsFor(spine export.View, year string) []string {
	stems := make([]string, len(spine.Steps))
	prev := year
	for i, s := range spine.Steps {
		if s.Projection != "" {
			prev = s.YearProjections[year]
		}
		stems[i] = prev
	}
	return stems
}

type rungWalker struct {
	spine       export.View
	projections map[string][]byte
	stems       []string
	width       int
}

// walk answers every node the chart on screen offers, and the charts those
// open in turn.
//
// THE SIDE AND THE OUTWARD TIERS ARE READ OFF THE STEP THE WAY shapeFor READS
// THEM: a step that keeps a flank opens the node into the half beyond it,
// with the near end of a ribbon being its source when the flank is on the
// left; a step that keeps none draws all of its tiers, with the near end
// being the source when it declares SideSource and the target otherwise.
func (w rungWalker) walk(chart []export.GraphNode, openedKey string, path []string, out *[]rung) error {
	for i, s := range w.spine.Steps {
		if !slices.Contains(s.After, openedKey) {
			continue
		}
		stem := w.stems[i]
		raw, ok := w.projections[stem]
		if !ok {
			return fmt.Errorf("step %q draws %q, which was not built", s.Key, stem)
		}
		opens, err := export.Openable(w.spine, i, s, stem, raw)
		if err != nil {
			return err
		}
		nearIsSource, outward := s.Side == export.SideSource, slices.Clone(s.Tiers)
		if len(s.Keep) > 0 {
			var flank bool
			nearIsSource, outward, flank = export.Flank(s)
			if !flank {
				return fmt.Errorf("step %q keeps %v of tiers %v, which is not a flank", s.Key, s.Keep, s.Tiers)
			}
		}
		g, err := export.DecodeGraph(raw)
		if err != nil {
			return fmt.Errorf("step %q: %s: %w", s.Key, stem, err)
		}
		for _, n := range chart {
			if n.Tier != s.From || (s.Role != "" && n.Role != s.Role) || !slices.Contains(opens, n.ID) {
				continue
			}
			r, next, err := w.answer(g, s, n.ID, nearIsSource, outward)
			if err != nil {
				return fmt.Errorf("%s > %s: %w", strings.Join(path, " > "), n.ID, err)
			}
			r.Path = append(slices.Clone(path), n.ID)
			*out = append(*out, r)
			if err := w.walk(next, s.Key, r.Path, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// answer is one rung at this walker's budget: what every column the step
// draws holds, and the chart the rung leaves on screen for the next step to
// open from. outward is every tier the step draws beyond its centre, nearest
// first, or every tier it draws when it keeps no flank and has no centre.
func (w rungWalker) answer(g export.Graph, s export.DrillStep, opened string, nearIsSource bool, outward []int) (rung, []export.GraphNode, error) {
	centre := len(s.Keep) > 0
	// THE BUDGET DROPS WIDENED COLUMNS FROM THE END OF WIDEN'S ORDER, which is
	// DrillStep.Widen's contract, and a widened column the document leaves
	// empty is dropped too, which is the client's.
	active := slices.Clone(s.Tiers)
	for k, t := range s.Widen {
		if k >= w.width-3 {
			active = slices.DeleteFunc(active, func(x int) bool { return x == t })
		}
	}
	role := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		role[n.ID] = n.Role
	}
	// THE DOCUMENT IS ASKED FOR THE COLUMNS THIS HALF DRAWS, CENTRE INCLUDED,
	// which is what windowFor hands sideOf; a step with no centre is asked for
	// all of its active columns.
	var half []int
	if centre {
		half = append(half, s.From)
	}
	for _, t := range outward {
		if slices.Contains(active, t) {
			half = append(half, t)
		}
	}
	reach, err := export.ReachOf(g, opened, nearIsSource, half)
	if err != nil {
		return rung{}, nil, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
	}
	if len(reach.At) == 0 {
		return rung{}, nil, fmt.Errorf("step %q opens %q into tiers %v and the document draws nothing there, which export.Openable said it would", s.Key, opened, half)
	}
	// THE CENTRE IS THE OPENED NODE ALONE, and the reach says whether it is:
	// a ribbon out of the opened node's subtree into ANOTHER node of the
	// centre's tier would be drawn by the client in the centre column, beside
	// the node the reader clicked, and this artifact has no shape for that.
	if centre {
		if others := slices.DeleteFunc(slices.Clone(reach.At[s.From]), func(id string) bool { return id == opened }); len(others) > 0 {
			return rung{}, nil, fmt.Errorf("step %q opens %q and the document draws %v beside it at tier %d, which is a column this walk has no shape for", s.Key, opened, others, s.From)
		}
	}
	candidates := map[int][]string{}
	for _, t := range outward {
		if slices.Contains(active, t) {
			candidates[t] = reach.At[t]
		}
	}
	for _, t := range s.Widen {
		if slices.Contains(active, t) && len(candidates[t]) == 0 {
			active = slices.DeleteFunc(active, func(x int) bool { return x == t })
		}
	}
	capOn := map[int]int{}
	for _, c := range s.Caps {
		// A CAP ON THE KEPT FLANK IS REFUSED, NOT LEFT UNCOMPARED. The client's
		// sideOf applies every declared cap to both halves of the window, so a
		// cap here would fold a column this walk answers with a reason instead
		// of ids, and nothing would hold the client to it.
		if slices.Contains(s.Keep, c.Tier) {
			return rung{}, nil, fmt.Errorf("step %q caps tier %d, which is a flank it keeps off the chart above and not a column of the document it draws", s.Key, c.Tier)
		}
		if !slices.Contains(active, c.Tier) {
			continue
		}
		if _, reached := candidates[c.Tier]; !reached {
			return rung{}, nil, fmt.Errorf("step %q caps tier %d, which its window does not open a node into", s.Key, c.Tier)
		}
		capOn[c.Tier] = c.Cap
	}
	// A FOLD ABOVE A DRAWN DEEPER TIER IS REFUSED, because the two sides would
	// count that deeper tier differently in silence: the client's capColumn
	// removes a folded node's descendants before the next tier is ranked, and
	// this walk reached the next tier through the unfolded column.
	// Inert on the corpus, measured: fund is the only step with two outward
	// tiers, and its 23 divisions sit under a cap of 24.
	for k, t := range outward[:len(outward)-1] {
		deeper := outward[k+1]
		if c, capped := capOn[t]; capped && len(candidates[t]) > c+1 && slices.Contains(active, deeper) {
			return rung{}, nil, fmt.Errorf("step %q folds tier %d (%d under a cap of %d) while drawing tier %d beyond it, which this walk reaches from the unfolded column and the client from the folded one", s.Key, t, len(candidates[t]), c, deeper)
		}
	}
	size := func(id string) int64 { return max(reach.In[id], reach.Out[id]) }
	// ONE PASS IN COLUMN ORDER ANSWERS EVERY DRAWN TIER AND BUILDS THE CHART
	// THE RUNG LEAVES ON SCREEN from the same ranked slice, so what the
	// artifact says a column draws and what the next step is offered cannot
	// be two readings of the fold.
	draws := make([]drawnTier, 0, len(active))
	var next []export.GraphNode
	for _, t := range active {
		switch {
		case centre && t == s.From:
			draws = append(draws, drawnTier{Tier: t, Role: roleCentre, Candidates: 1, IDs: []string{opened}})
			next = append(next, export.GraphNode{ID: opened, Tier: t, Role: role[opened]})
		case slices.Contains(s.Keep, t):
			draws = append(draws, drawnTier{Tier: t, Role: roleFlank, Unanswered: reasonKeptFlank})
		case slices.Contains(outward, t):
			ids, reached := candidates[t]
			if !reached {
				return rung{}, nil, fmt.Errorf("step %q draws tier %d, which its window does not reach at %d columns", s.Key, t, w.width)
			}
			// THE RANKING IS THE CLIENT'S: by the larger of what flows in and
			// what flows out, largest first, ties by id. Which nodes survive a
			// fold is which nodes the next step can be asked to open.
			if c, capped := capOn[t]; capped && len(ids) > c+1 {
				ids = slices.Clone(ids)
				sort.SliceStable(ids, func(i, j int) bool {
					si, sj := size(ids[i]), size(ids[j])
					if si != sj {
						return si > sj
					}
					return ids[i] < ids[j]
				})
				ids = ids[:c]
			}
			for _, id := range ids {
				next = append(next, export.GraphNode{ID: id, Tier: t, Role: role[id]})
			}
			draws = append(draws, drawnTier{
				Tier: t, Role: roleOutward, Cap: capOn[t],
				Candidates: len(candidates[t]), IDs: slices.Sorted(slices.Values(ids)),
				Hidden: len(candidates[t]) - len(ids),
			})
		default:
			return rung{}, nil, fmt.Errorf("step %q draws tier %d, which is neither its centre, a flank it keeps, nor a tier it opens into", s.Key, t)
		}
	}
	return rung{Width: w.width, Step: s.Key, Draws: draws}, next, nil
}

// encodeRungs is the artifact's one encoding: indented, so a regenerated
// file's diff can be read by eye against the rule drawnTier states.
func encodeRungs(doc rungsDoc) ([]byte, error) {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
