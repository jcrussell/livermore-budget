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
// client's answer can be held to it. Relative to the repository root. It is
// pinned byte for byte to what the site serves at rungsServedPath, so a
// fixture and a served file cannot be two answers.
const rungsPath = "testdata/rungs.json"

// rungsServedPath is where the rung answer lands in the site: at the site
// root beside the per-year structures, and NOT under data/, by the same
// contract as structurePath. ONE FILE FOR EVERY BUDGET AND EVERY YEAR,
// because rungWidths are both answered in it and a client holding it
// answers a change of column budget by lookup rather than by fetch; a file
// per budget or per drill was fisc-kbuo candidate (b), and it lost.
const rungsServedPath = "rungs.json"

// rungsSchemaVersion is the version a rungsDoc declares, spelled once. The
// arm reading the artifact refuses any other.
const rungsSchemaVersion = 4

// rungWidths are the column budgets the artifact answers for: the narrow
// window every page opens at, and the one a fourth column is bought on.
var rungWidths = []int{3, 4}

// rungsDoc is the artifact. It is Go's reading of the declared steps against
// the documents they draw, and NOT a re-encoding of the declarations: every
// figure in it is computed by export.ReachOf over a document, so a comparison
// against it is a comparison against what Go says the chart holds.
type rungsDoc struct {
	SchemaVersion int          `json:"schema_version"`
	Columns       []rungColumn `json:"columns"`
}

// rungColumn is one published year, by the spine document's stem, which is
// how tools/jscheck names a column.
type rungColumn struct {
	Stem  string `json:"stem"`
	Rungs []rung `json:"rungs"`
}

// A rung is one opened path at one column budget: the step that opened its
// last node; for every column the window draws, in the order it draws them,
// what that column holds; and the marks the client adds to the window that
// no document prints, sorted by id.
type rung struct {
	Path  []string    `json:"path"`
	Width int         `json:"width"`
	Step  string      `json:"step"`
	Draws []drawnTier `json:"draws"`
	Marks []drawnMark `json:"marks,omitempty"`
}

// drawnMark is one node the client draws on a rung that no page prints: the
// gap markGap states between what the chart above sends into the opened
// node and what the drawn document breaks it into, and the residual
// carryResidual stands beside the opened node's parts. Go computes which
// mark exists, the tier it stands at and the cents that arrive at it and
// leave it, and NOT its prose: the rationale and the source note are built
// from labels and locators the walk does not decode, and
// tools/jscheck/drill.mjs holds those.
//
// ON THE RUNG AND NOT ON A COLUMN, because both marks index the step's
// declared tiers and not the columns the budget left drawn, so a mark can
// stand at a tier this rung's draws do not list. Ends is a residual's
// declared endpoints, sorted; a gap has exactly one of InCents and OutCents,
// which is the side the short one stands on.
type drawnMark struct {
	ID       string   `json:"id"`
	Role     string   `json:"role"`
	Tier     int      `json:"tier"`
	InCents  int64    `json:"in_cents,omitempty"`
	OutCents int64    `json:"out_cents,omitempty"`
	Ends     []string `json:"ends,omitempty"`
}

// drawnTier is one column of one rung. ONE LIST IN COLUMN ORDER, rather than
// a tier list beside a cap list beside an id list: three spellings of one
// shape drift, and revenue-category's order is [1,0,2], which no
// centre-first convention would have spelled.
//
// Role is centre for the opened node's own column, flank for a column of
// s.Keep, and outward for one the step opens the node into. A flank is read
// off the chart on screen, the half windowFor carries over, and is answered
// like any other column. Cap is 0 where the step declares none, which is
// the value validateSteps refuses on a declared cap.
//
// Candidates is how many document nodes the window reaches at the tier;
// IDs, sorted, is which of them the column draws as the opened node's own
// parts, Carried which it draws but the client does not count as one -- a
// node the document marks derived, or one the step's residual declaration
// names, which is isCarried's rule -- and Hidden how many the folded tail
// stands for. A mark the client adds of its own is in neither list: it is
// the rung's Marks. IDS IS WRITTEN EVEN WHEN EMPTY, because a flank whose only
// mark is carried answers with nothing, and "answered with nothing" has to
// be told from "not answered" by a reader of the file. THE FOLD ENGAGES
// ONLY ABOVE CAP+1, because folding one node into a tail of one draws the
// same number of marks and loses a name. That threshold is the client's,
// measured at the shipped site: special-revenue's 32 funds draw eight and a
// tail of 24, and a column of nine under a cap of eight draws all nine.
type drawnTier struct {
	Tier       int      `json:"tier"`
	Role       string   `json:"role"`
	Cap        int      `json:"cap,omitempty"`
	Candidates int      `json:"candidates,omitempty"`
	IDs        []string `json:"ids"`
	Carried    []string `json:"carried,omitempty"`
	Hidden     int      `json:"hidden,omitempty"`
}

const (
	roleCentre  = "centre"
	roleFlank   = "flank"
	roleOutward = "outward"
)

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
// for the half beyond its centre, the way windowFor asks its document, and
// the chart on screen for the half before it, the way windowFor asks that;
// a step that keeps none is asked for every column it draws, on the side it
// declares, the way sideOf does -- so the transfers step, whose opened node
// touches no ribbon and whose payer ends are its children, is answered like
// any other rather than skipped.
//
// WHAT A COLUMN DRAWS IS DOCUMENT NODES, AND THE MARKS ARE ANSWERED APART.
// The client adds marks of its own to a window: a folded tail, which this
// walk carries under the client's own id so that a flank holding one is
// refused by name rather than answered as nothing; a residual, which it
// computes as export.ResidualOf does over the window and the document one
// depth up; and a gap, which it computes as export.GapOf does over the
// window once the residual has been spliced in. Both are answered under
// the rung's Marks with their tier and their cents, and the arm compares
// them by id against what the client draws. A document node the client
// draws but does not count as one of the opened node's parts is answered
// under Carried, so the two endpoints a residual lends the fund group's
// flank are compared rather than subtracted on both sides.
func rungsOf(projections map[string][]byte, spine export.View) (rungsDoc, error) {
	doc := rungsDoc{SchemaVersion: rungsSchemaVersion}
	for _, year := range spine.YearStems {
		raw, ok := projections[year]
		if !ok {
			return rungsDoc{}, fmt.Errorf("rungs: the spine's year %q was not built", year)
		}
		chart, err := export.DecodeGraph(raw)
		if err != nil {
			return rungsDoc{}, fmt.Errorf("rungs: %s: %w", year, err)
		}
		screen, err := overviewOf(spine, chart)
		if err != nil {
			return rungsDoc{}, fmt.Errorf("rungs: %s: %w", year, err)
		}
		stems := stepStemsFor(spine, year)
		col := rungColumn{Stem: year}
		for _, width := range rungWidths {
			w := rungWalker{spine: spine, projections: projections, stems: stems, width: width}
			if err := w.walk(screen, chart, "", nil, &col.Rungs); err != nil {
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

// spineView is the view at export.IndexPath, the one whose steps the rung
// answer walks. views only reads built.Projections to name it, so a result
// carrying the projections alone reaches it; a result whose views name no
// spine has nothing to walk, and says so.
func spineView(built result) (export.View, error) {
	for _, v := range views(built) {
		if v.Path == export.IndexPath {
			return v, nil
		}
	}
	return export.View{}, fmt.Errorf("rungs: no view at %q, so there is no spine to walk", export.IndexPath)
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

// overviewOf is the chart the spine's own page has on screen before anything
// is opened, read the way shapeFor reads it: the document folded to the
// view's render tiers, which is what activeTiers answers undrilled. Nothing
// moves on the committed spine -- its nodes all sit at its render tiers and
// every one is a ribbon's end -- and the first rungs are read off this chart
// rather than the raw document so that a spine that folds would be walked
// as the client draws it.
//
// A ROOT IS REFUSED BY NAME rather than modelled: shapeFor filters to it
// before folding, and no shipped view declares one, so the reading has
// nothing to be measured against.
func overviewOf(spine export.View, chart export.Graph) (export.Graph, error) {
	if spine.Root != "" {
		return export.Graph{}, fmt.Errorf("the spine declares root %q, which this walk does not filter the overview to", spine.Root)
	}
	if len(spine.RenderTiers) == 0 {
		return export.Graph{}, fmt.Errorf("the spine declares no render tiers, so the chart its first rungs open from cannot be read")
	}
	screen, err := export.Fold(chart, spine.RenderTiers)
	if err != nil {
		return export.Graph{}, fmt.Errorf("overview: %w", err)
	}
	if len(screen.Nodes) == 0 {
		return export.Graph{}, fmt.Errorf("overview: the document draws nothing at tiers %v", spine.RenderTiers)
	}
	return screen, nil
}

type rungWalker struct {
	spine       export.View
	projections map[string][]byte
	stems       []string
	width       int
}

// walk answers every node the chart on screen offers, and the charts those
// open in turn. from is the document the chart on screen was shaped from,
// unfolded -- the year's own at the top and the rung's own below that,
// which is docAt one depth up -- and is what a residual's ribbons come off
// where the window does not draw them.
//
// THE SIDE AND THE OUTWARD TIERS ARE READ OFF THE STEP THE WAY shapeFor READS
// THEM: a step that keeps a flank opens the node into the half beyond it,
// with the near end of a ribbon being its source when the flank is on the
// left; a step that keeps none draws all of its tiers, with the near end
// being the source when it declares SideSource and the target otherwise.
func (w rungWalker) walk(chart, from export.Graph, openedKey string, path []string, out *[]rung) error {
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
		for _, n := range chart.Nodes {
			if n.Tier != s.From || (s.Role != "" && n.Role != s.Role) || !slices.Contains(opens, n.ID) {
				continue
			}
			r, next, err := w.answer(g, chart, from, s, n.ID, nearIsSource, outward)
			if err != nil {
				return fmt.Errorf("%s > %s: %w", strings.Join(path, " > "), n.ID, err)
			}
			r.Path = append(slices.Clone(path), n.ID)
			*out = append(*out, r)
			if err := w.walk(next, g, s.Key, r.Path, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// answer is one rung at this walker's budget: what every column the step
// draws holds, and the chart the rung leaves on screen for the next step to
// open from. screen is the chart the node was opened on and from the
// document it was shaped from; outward is every tier the step draws beyond
// its centre, nearest first, or every tier it draws when it keeps no flank
// and has no centre.
//
// THE CHART LEFT ON SCREEN IS THE WHOLE WINDOW, kept half spliced with fresh
// half the way windowFor splices them, the kept record winning an id both
// hold -- and not the fresh half alone. The next step's flank is a column of
// this chart, and nothing in a declaration forbids that column being this
// rung's own flank; on the committed spine it is always this rung's centre,
// which is why a walk carrying only the fresh half would have gone
// unnoticed.
func (w rungWalker) answer(g, screen, from export.Graph, s export.DrillStep, opened string, nearIsSource bool, outward []int) (rung, export.Graph, error) {
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
		return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
	}
	if len(reach.At) == 0 {
		return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q into tiers %v and the document draws nothing there, which export.Openable said it would", s.Key, opened, half)
	}
	// THE KEPT HALF IS THE CHART ON SCREEN ASKED FOR THE FLANK AND THE CENTRE,
	// on the side windowFor asks it: a left flank is what flows INTO the
	// opened node, the mirror of the fresh half's filter, so the near end is
	// the one the fresh half does not use.
	var kept export.Reach
	if centre {
		idx := slices.Index(active, s.From)
		keptTiers := active[idx:]
		if nearIsSource {
			keptTiers = active[:idx+1]
		}
		kept, err = export.ReachOf(screen, opened, !nearIsSource, keptTiers)
		if err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q keeps the flank of %q: %w", s.Key, opened, err)
		}
		// THE KEPT HALF'S CENTRE IS THE OPENED NODE ALONE TOO, the mirror of
		// the fresh half's refusal below, and it has to hold the opened node
		// at all: a flank that sends nothing into the node is a window
		// windowFor refuses to draw, and the client's filterLinks says so by
		// throwing rather than drawing an empty column.
		if others := slices.DeleteFunc(slices.Clone(kept.At[s.From]), func(id string) bool { return id == opened }); len(others) > 0 {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q and the chart on screen draws %v beside it at tier %d, which is a column this walk has no shape for", s.Key, opened, others, s.From)
		}
		if !slices.Contains(kept.At[s.From], opened) {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q and the chart on screen sends nothing between tiers %v and it, so there is no flank to keep and the client would refuse the window", s.Key, opened, keptTiers)
		}
	}
	// THE CENTRE IS THE OPENED NODE ALONE, and the reach says whether it is:
	// a ribbon out of the opened node's subtree into ANOTHER node of the
	// centre's tier would be drawn by the client in the centre column, beside
	// the node the reader clicked, and this artifact has no shape for that.
	if centre {
		if others := slices.DeleteFunc(slices.Clone(reach.At[s.From]), func(id string) bool { return id == opened }); len(others) > 0 {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q and the document draws %v beside it at tier %d, which is a column this walk has no shape for", s.Key, opened, others, s.From)
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
			return rung{}, export.Graph{}, fmt.Errorf("step %q caps tier %d, which is a flank it keeps off the chart above and not a column of the document it draws", s.Key, c.Tier)
		}
		if centre && c.Tier == s.From {
			return rung{}, export.Graph{}, fmt.Errorf("step %q caps tier %d, which is its centre, a column of the one node the reader opened", s.Key, c.Tier)
		}
		if !slices.Contains(active, c.Tier) {
			continue
		}
		if _, reached := candidates[c.Tier]; !reached {
			return rung{}, export.Graph{}, fmt.Errorf("step %q caps tier %d, which its window does not open a node into", s.Key, c.Tier)
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
			return rung{}, export.Graph{}, fmt.Errorf("step %q folds tier %d (%d under a cap of %d) while drawing tier %d beyond it, which this walk reaches from the unfolded column and the client from the folded one", s.Key, t, len(candidates[t]), c, deeper)
		}
	}
	size := func(id string) int64 { return max(reach.In[id], reach.Out[id]) }
	fresh := export.IndexGraph(reach.Drawn)
	next := export.IndexGraph(kept.Drawn)
	// WHAT THE CLIENT DRAWS BUT DOES NOT COUNT is read the way isCarried and
	// the derived mark read it, off the record the chart carries: the flank's
	// records are the chart on screen's and the fresh half's are the
	// document's, which is which record windowFor keeps for each.
	carried := func(n export.GraphNode) bool {
		_, declared := s.Residual[n.ID]
		return n.Derived || declared
	}
	partition := func(ids []string, node func(string) export.GraphNode) (own, lent []string) {
		own = []string{}
		for _, id := range ids {
			if carried(node(id)) {
				lent = append(lent, id)
			} else {
				own = append(own, id)
			}
		}
		slices.Sort(own)
		slices.Sort(lent)
		return own, lent
	}
	onScreen := func(id string) export.GraphNode { return next.Nodes[id] }
	inDocument := func(id string) export.GraphNode { return fresh.Nodes[id] }
	// ONE PASS IN COLUMN ORDER ANSWERS EVERY DRAWN TIER AND BUILDS THE CHART
	// THE RUNG LEAVES ON SCREEN from the same ranked slice, so what the
	// artifact says a column draws and what the next step is offered cannot
	// be two readings of the fold. The kept half went in first, whole, so
	// that its record of the centre is the one carried, as windowFor's is.
	draws := make([]drawnTier, 0, len(active))
	remap := map[string]string{}
	for _, t := range active {
		switch {
		case centre && t == s.From:
			own, lent := partition([]string{opened}, onScreen)
			draws = append(draws, drawnTier{Tier: t, Role: roleCentre, Candidates: 1, IDs: own, Carried: lent})
		case slices.Contains(s.Keep, t):
			// A FOLDED TAIL AT A KEPT TIER IS REFUSED: the client counts what
			// it stands for under the tail's own label, and this artifact
			// has no field for a hidden count with no cap to explain it.
			ids := kept.At[t]
			if i := slices.IndexFunc(ids, export.IsAggregate); i >= 0 {
				return rung{}, export.Graph{}, fmt.Errorf("step %q keeps tier %d and the chart on screen draws the folded tail %q there, which is a column this walk has no shape for", s.Key, t, ids[i])
			}
			// A MARK OF THE RUNG ABOVE AT A KEPT TIER IS REFUSED THE SAME WAY:
			// the client draws it and counts it under neither set, and this
			// rung's Marks are its own, so a mark that survived onto the
			// flank would be one the two sides read differently in silence.
			if i := slices.IndexFunc(ids, func(id string) bool { return export.IsResidual(id) || export.IsGap(id) }); i >= 0 {
				return rung{}, export.Graph{}, fmt.Errorf("step %q keeps tier %d and the chart on screen draws the mark %q there, which is a column this walk has no shape for", s.Key, t, ids[i])
			}
			own, lent := partition(ids, onScreen)
			draws = append(draws, drawnTier{Tier: t, Role: roleFlank, Candidates: len(ids), IDs: own, Carried: lent})
		case slices.Contains(outward, t):
			ids, reached := candidates[t]
			if !reached {
				return rung{}, export.Graph{}, fmt.Errorf("step %q draws tier %d, which its window does not reach at %d columns", s.Key, t, w.width)
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
				// THE TAIL IS CARRIED AS THE NODE capColumn DRAWS, under its
				// id and parented as sideOf parents it: to the opened node
				// when every node of the column is inside it, else to
				// nothing. The ribbons of the nodes it folds are re-pointed
				// at it below, so the chart the next rung reads has the
				// tail's ribbons where the client has them.
				tail := export.AggregateID(t)
				parent := ""
				if slices.IndexFunc(ids, func(id string) bool { return !fresh.Within(opened, id) }) < 0 {
					parent = opened
				}
				for _, id := range ids[c:] {
					remap[id] = tail
				}
				next.Add(export.GraphNode{ID: tail, Tier: t, Role: export.RoleAggregate, Parent: parent})
				ids = ids[:c]
			}
			for _, id := range ids {
				next.Add(fresh.Nodes[id])
			}
			own, lent := partition(ids, inDocument)
			draws = append(draws, drawnTier{
				Tier: t, Role: roleOutward, Cap: capOn[t],
				Candidates: len(candidates[t]), IDs: own, Carried: lent,
				Hidden: len(candidates[t]) - len(ids),
			})
		default:
			return rung{}, export.Graph{}, fmt.Errorf("step %q draws tier %d, which is neither its centre, a flank it keeps, nor a tier it opens into", s.Key, t)
		}
	}
	if !centre {
		next.Add(fresh.Nodes[opened])
	}
	// THE FRESH HALF'S RIBBONS ARRIVE THROUGH THE CAP, as capColumn re-points
	// them: a folded node's end becomes the tail's, a ribbon that then joins
	// the tail to itself is a flow inside one box, and one whose end the
	// window no longer draws is dropped.
	for _, l := range reach.Drawn.Links {
		if to, folded := remap[l.Source]; folded {
			l.Source = to
		}
		if to, folded := remap[l.Target]; folded {
			l.Target = to
		}
		if err := next.Link(l); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
	}
	// THE MARKS GO ON LAST, over the whole spliced window, in the order
	// shapeFor applies them: the residual first, off the window and the
	// document one depth up, and the gap after it, over the chart the
	// residual left -- what the opened node takes in against what it sends
	// out, once everything that is going to stand beside it does. A mark's
	// tier is the step's declared one, not the budget's (drawnMark), and
	// each is added to the chart the next rung reads so that a flank it
	// survived onto is refused above rather than counted.
	var marks []drawnMark
	drawn := next.Graph()
	// A RESIDUAL IS CARRIED ONLY ACROSS A DOCUMENT SWITCH, which is
	// carryResidual's own gate; validateSteps refuses the declaration on a
	// same-document step of a shipped view, and this is what a spine that
	// never met validateSteps gets.
	residual := s.Residual
	if s.Projection == "" {
		residual = nil
	}
	if c, ok, err := export.ResidualOf(drawn, from, g, opened, s.Tiers, residual); err != nil {
		return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
	} else if ok {
		if next, err = carry(drawn, c); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
		marks = append(marks, drawnMark(c.Mark))
		drawn = next.Graph()
	}
	if c, ok, err := export.GapOf(drawn, opened, s.Tiers, s.Gaps); err != nil {
		return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
	} else if ok {
		if next, err = carry(drawn, c); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
		marks = append(marks, drawnMark(c.Mark))
	}
	slices.SortFunc(marks, func(a, b drawnMark) int { return strings.Compare(a.ID, b.ID) })
	return rung{Width: w.width, Step: s.Key, Draws: draws, Marks: marks}, next.Graph(), nil
}

// carry is drawn with one mark applied: the ribbons the mark re-points
// spliced out by index, its nodes added, and its ribbons merged in. A
// ribbon whose end the chart does not hold is the error Link makes of it,
// which is what refuses a mark placed beside nothing.
func carry(drawn export.Graph, c export.Carry) (*export.Chart, error) {
	kept := export.Graph{Nodes: drawn.Nodes}
	for i, l := range drawn.Links {
		if !slices.Contains(c.Splice, i) {
			kept.Links = append(kept.Links, l)
		}
	}
	next := export.IndexGraph(kept)
	for _, n := range c.Nodes {
		next.Add(n)
	}
	for _, l := range c.Links {
		if err := next.Link(l); err != nil {
			return nil, err
		}
	}
	return next, nil
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
