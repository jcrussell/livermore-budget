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
// figure in it is computed by walking a document's ribbons, so a comparison
// against it is a comparison against what Go says the chart holds.
type rungsDoc struct {
	SchemaVersion int `json:"schema_version"`
	// Skipped is every step the artifact does not enumerate, by key and with
	// the reason, so the arm reading it can tell a rung Go declined from one
	// it missed.
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

// rungGraph is as much of a projection document as a rung needs: which tier
// and role each node has, and which nodes each ribbon joins at what value.
type rungGraph struct {
	Nodes []rungNode `json:"nodes"`
	Links []rungLink `json:"links"`
}

type rungNode struct {
	ID   string `json:"id"`
	Tier int    `json:"tier"`
	Role string `json:"role"`
}

type rungLink struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	ValueCents int64  `json:"value_cents"`
}

// rungsOf walks the spine's declared steps over the built documents, for
// every published year and every budget in rungWidths, and answers each rung
// it reaches.
//
// WHAT A RUNG OFFERS IS READ THE WAY THE CLIENT READS IT: a node opens under
// the first step whose After names the chart on screen, whose From is the
// node's tier, whose Role -- if it declares one -- is the node's own role as
// the document printed it, and whose document decomposes the node, which is
// export.Openable's answer and the one stepView.Opens ships. A step that
// keeps no flank has no window direction to walk and is skipped by name.
//
// WHAT A RUNG DRAWS IS DOCUMENT NODES. The client adds derived nodes of its
// own -- a residual, a gap, a folded tail -- and this walk counts none of
// them, so the day one of them competes with a cap the arm reading this
// artifact goes red rather than this function guessing which side wins.
func rungsOf(projections map[string][]byte, spine export.View) (rungsDoc, error) {
	doc := rungsDoc{SchemaVersion: 2, Reasons: rungReasons}
	skipped := map[string]string{}
	for _, year := range spine.YearStems {
		raw, ok := projections[year]
		if !ok {
			return rungsDoc{}, fmt.Errorf("rungs: the spine's year %q was not built", year)
		}
		chart, err := decodeRungGraph(raw)
		if err != nil {
			return rungsDoc{}, fmt.Errorf("rungs: %s: %w", year, err)
		}
		stems := stepStemsFor(spine, year)
		col := rungColumn{Stem: year}
		for _, width := range rungWidths {
			w := rungWalker{spine: spine, projections: projections, stems: stems, width: width, skipped: skipped}
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
	for _, key := range slices.Sorted(mapKeys(skipped)) {
		doc.Skipped = append(doc.Skipped, skippedStep{Step: key, Reason: skipped[key]})
	}
	return doc, nil
}

func mapKeys(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
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

func decodeRungGraph(raw []byte) (rungGraph, error) {
	var g rungGraph
	if err := json.Unmarshal(raw, &g); err != nil {
		return rungGraph{}, fmt.Errorf("decode graph: %w", err)
	}
	if len(g.Nodes) == 0 {
		return rungGraph{}, fmt.Errorf("decode graph: no nodes")
	}
	return g, nil
}

type rungWalker struct {
	spine       export.View
	projections map[string][]byte
	stems       []string
	width       int
	skipped     map[string]string
}

// walk answers every node the chart on screen offers, and the charts those
// open in turn.
func (w rungWalker) walk(chart []rungNode, openedKey string, path []string, out *[]rung) error {
	for i, s := range w.spine.Steps {
		if !slices.Contains(s.After, openedKey) {
			continue
		}
		if len(s.Keep) == 0 {
			w.skipped[s.Key] = "keeps no flank, so it has no window direction to walk a document in"
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
		keptLeft, outward, ok := export.Flank(s)
		if !ok {
			return fmt.Errorf("step %q keeps %v of tiers %v, which is not a flank", s.Key, s.Keep, s.Tiers)
		}
		g, err := decodeRungGraph(raw)
		if err != nil {
			return fmt.Errorf("step %q: %s: %w", s.Key, stem, err)
		}
		for _, n := range chart {
			if n.Tier != s.From || (s.Role != "" && n.Role != s.Role) || !slices.Contains(opens, n.ID) {
				continue
			}
			r, next, err := w.answer(g, s, n.ID, keptLeft, outward)
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

// answer is one rung at this walker's budget: what every column the window
// draws holds, and the chart the rung leaves on screen for the next step to
// open from.
func (w rungWalker) answer(g rungGraph, s export.DrillStep, opened string, keptLeft bool, outward []int) (rung, []rungNode, error) {
	// THE BUDGET DROPS WIDENED COLUMNS FROM THE END OF WIDEN'S ORDER, which is
	// DrillStep.Widen's contract, and a widened column the document leaves
	// empty is dropped too, which is the client's.
	active := slices.Clone(s.Tiers)
	for k, t := range s.Widen {
		if k >= w.width-3 {
			active = slices.DeleteFunc(active, func(x int) bool { return x == t })
		}
	}
	tier := make(map[string]int, len(g.Nodes))
	role := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		tier[n.ID] = n.Tier
		role[n.ID] = n.Role
	}
	inflow, outflow := map[string]int64{}, map[string]int64{}
	candidates := map[int][]string{}
	frontier := map[string]bool{opened: true}
	for _, t := range outward {
		if !slices.Contains(active, t) {
			break
		}
		next := map[string]bool{}
		for _, l := range g.Links {
			near, far := l.Source, l.Target
			if !keptLeft {
				near, far = far, near
			}
			if !frontier[near] || tier[far] != t {
				continue
			}
			next[far] = true
			v := l.ValueCents
			if v < 0 {
				v = -v
			}
			inflow[l.Target] += v
			outflow[l.Source] += v
		}
		candidates[t] = slices.Sorted(mapKeysBool(next))
		frontier = next
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
	// this walk reached the next tier from the frontier before the fold.
	// Inert on the corpus, measured: fund is the only step with two outward
	// tiers, and its 23 divisions sit under a cap of 24.
	for k, t := range outward[:len(outward)-1] {
		deeper := outward[k+1]
		if c, capped := capOn[t]; capped && len(candidates[t]) > c+1 && slices.Contains(active, deeper) {
			return rung{}, nil, fmt.Errorf("step %q folds tier %d (%d under a cap of %d) while drawing tier %d beyond it, which this walk reaches from the unfolded column and the client from the folded one", s.Key, t, len(candidates[t]), c, deeper)
		}
	}
	size := func(id string) int64 { return max(inflow[id], outflow[id]) }
	// ONE PASS IN COLUMN ORDER ANSWERS EVERY DRAWN TIER AND BUILDS THE CHART
	// THE RUNG LEAVES ON SCREEN from the same ranked slice, so what the
	// artifact says a column draws and what the next step is offered cannot
	// be two readings of the fold.
	draws := make([]drawnTier, 0, len(active))
	var next []rungNode
	for _, t := range active {
		switch {
		case t == s.From:
			draws = append(draws, drawnTier{Tier: t, Role: roleCentre, Candidates: 1, IDs: []string{opened}})
			next = append(next, rungNode{ID: opened, Tier: t, Role: role[opened]})
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
				next = append(next, rungNode{ID: id, Tier: t, Role: role[id]})
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

func mapKeysBool(m map[string]bool) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
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
