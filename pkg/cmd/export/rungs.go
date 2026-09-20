package export

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// rungsPath is the committed copy of Go's answer for every rung the drill
// walks. Relative to the repository root. It is pinned byte for byte to what
// the site serves at rungsServedPath, so a fixture and a served file cannot
// be two answers -- and since site/app.js READS that answer rather than
// deriving one, a change here is a change to what the site draws.
const rungsPath = "testdata/rungs.json"

// rungsServedPath is where the rung answer lands in the site: at the site
// root beside the per-year structures, and NOT under data/, by the same
// contract as structurePath. ONE FILE FOR EVERY YEAR AND EVERY RUNG, because
// a client holding it answers a drill by lookup rather than by fetch; a file
// per drill was fisc-kbuo candidate (b), and it lost.
//
// THE WRITER AND THE PAGE NAME IT FROM ONE CONSTANT. The packager puts the
// same string into window.FISC_CONFIG for the client to fetch, and a second
// spelling here would be a served file and a fetched URL free to drift apart
// by one character.
const rungsServedPath = export.RungsPath

// rungsSchemaVersion is the version a rungsDoc declares, spelled once. The
// arm reading the artifact refuses any other.
const rungsSchemaVersion = 5

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

// A rung is one opened path: the step that opened its last node; for every
// column that step declares, in the order it draws them, what that column
// holds; and the marks the client adds to the window that no document
// prints, sorted by id.
//
// ONE ANSWER AND NOT ONE PER COLUMN BUDGET. What a column holds is what the
// document draws there; how much of it a reader's screen has room for is the
// client's (AGENTS.md, "Go vets, JavaScript renders"), so a rung is answered
// once and fitted many times.
type rung struct {
	Path  []string    `json:"path"`
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
// declared tiers and not the columns a budget left drawn, so a mark can
// stand at a tier this rung's draws do not list. Ends is a residual's
// declared endpoints, sorted; a gap has exactly one of InCents and OutCents,
// which is the side the short one stands on.
//
// A MARK IS FOLD-INVARIANT, which is what lets it be answered once for a rung
// the client may fit several ways: a fold merges ribbons but preserves what
// arrives at and leaves the opened node, so ResidualOf and GapOf read the
// same cents off a folded chart and an unfolded one. Measured at 86f0fae,
// where the artifact still answered each path at two budgets: all 150 paths
// carried byte-identical marks at both, and so does every path of this
// artifact against that one -- including the three whose fold engaged under
// a mark.
type drawnMark struct {
	ID       string   `json:"id"`
	Role     string   `json:"role"`
	Tier     int      `json:"tier"`
	InCents  int64    `json:"in_cents,omitempty"`
	OutCents int64    `json:"out_cents,omitempty"`
	Ends     []string `json:"ends,omitempty"`
}

// drawnTier is one column of one rung. ONE LIST IN COLUMN ORDER, rather than
// a tier list beside an id list: two spellings of one shape drift, and
// revenue-category's order is [1,0,2], which no centre-first convention
// would have spelled.
//
// Role is centre for the opened node's own column, flank for a column of
// s.Keep, and outward for one the step opens the node into. A flank is read
// off the chart on screen, the half windowFor carries over, and is answered
// like any other column.
//
// IDs, sorted, is EVERY node the document draws at the tier as the opened
// node's own parts, and Carried every one it draws that the client does not
// count as one -- a node the document marks derived, or one the step's
// residual declaration names, which is isCarried's rule. A mark the client
// adds of its own is in neither list: it is the rung's Marks.
//
// A COLUMN SAYS WHAT IT HOLDS, NOT WHAT WOULD FIT. The ids are unfolded and
// no cap has been applied to them: DrillStep.Caps declares which columns MAY
// fold and DrillStep.Widen which a fourth column buys, both ship to the
// client in CONFIG.steps, and which of these ids a viewport leaves room for
// is decided there (AGENTS.md, "Go vets, JavaScript renders"). A fold Go
// pre-computed would be Go fitting a screen it cannot see, and it also hid
// the rungs under it from the walk that answers them (fisc-qics).
//
// IDS IS WRITTEN EVEN WHEN EMPTY, because a flank whose only mark is carried
// answers with nothing, and a column the document draws nothing at is still
// a column the step declares; both have to be told from "not answered" by a
// reader of the file.
type drawnTier struct {
	Tier    int      `json:"tier"`
	Role    string   `json:"role"`
	IDs     []string `json:"ids"`
	Carried []string `json:"carried,omitempty"`
}

const (
	roleCentre  = "centre"
	roleFlank   = "flank"
	roleOutward = "outward"
)

// rungsOf walks the spine's declared steps over the built documents, for
// every published year, and answers each rung it reaches.
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
// WHAT A COLUMN HOLDS IS DOCUMENT NODES, AND THE MARKS ARE ANSWERED APART.
// Two of the marks the client adds to a window are answered here: a
// residual, which it computes as export.ResidualOf does over the window and
// the document one depth up, and a gap, which it computes as export.GapOf
// does over the window once the residual has been spliced in. Both are
// answered under the rung's Marks with their tier and their cents. The third,
// the folded tail, is not answered at all and has no id here: it exists only
// where a reader's screen made the client fold, which is not a fact about
// the documents. A document node the client draws but does not count as one
// of the opened node's parts is answered under Carried, so the two endpoints
// a residual lends the fund group's flank are compared rather than
// subtracted on both sides.
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
		w := rungWalker{spine: spine, projections: projections, stems: stems}
		if err := w.walk(screen, chart, "", nil, &col.Rungs); err != nil {
			return rungsDoc{}, fmt.Errorf("rungs: %s: %w", year, err)
		}
		sort.Slice(col.Rungs, func(i, j int) bool {
			a, b := col.Rungs[i], col.Rungs[j]
			ap, bp := strings.Join(a.Path, " > "), strings.Join(b.Path, " > ")
			if ap != bp {
				return ap < bp
			}
			return a.Step < b.Step
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
// THERE IS NO ROOT TO FILTER TO ANY MORE. A view once restricted its chart to
// one node's subtree, which this walk refused rather than modelled; the field
// went with the template that was its only reader, so the overview is the
// whole document folded and nothing here has to say so.
func overviewOf(spine export.View, chart export.Graph) (export.Graph, error) {
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
//
// IT DESCENDS THE UNFOLDED CHART, and that is the whole of fisc-qics: while
// answer ranked a capped column down to its cap, every node the fold hid was
// a node this recursion never opened, so a reader who expanded the column and
// opened one stood on a rung nothing answered.
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

// answer is one rung: what every column the step declares holds, and the
// chart the rung leaves on screen for the next step to open from. It reads
// nothing off the walker, which is what a rung answered once rather than once
// per column budget means. screen is the chart the node was opened on and from the
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
func (rungWalker) answer(g, screen, from export.Graph, s export.DrillStep, opened string, nearIsSource bool, outward []int) (rung, export.Graph, error) {
	centre := len(s.Keep) > 0
	// THE DOCUMENT IS ASKED FOR EVERY COLUMN THIS HALF DECLARES, CENTRE
	// INCLUDED, which is what windowFor hands sideOf; a step with no centre is
	// asked for all of its columns. A widened column is asked for whether or
	// not a reader has bought it, because DrillStep.Widen drops columns to fit
	// a viewport and what the document draws there does not change when it
	// does.
	var half []int
	if centre {
		half = append(half, s.From)
	}
	half = append(half, outward...)
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
		idx := slices.Index(s.Tiers, s.From)
		keptTiers := s.Tiers[idx:]
		if nearIsSource {
			keptTiers = s.Tiers[:idx+1]
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
	// ONE PASS IN COLUMN ORDER ANSWERS EVERY DECLARED TIER AND BUILDS THE CHART
	// THE RUNG LEAVES ON SCREEN from the same slice, so what the artifact says
	// a column holds and what the next step is offered cannot be two readings
	// of one document. The kept half went in first, whole, so that its record
	// of the centre is the one carried, as windowFor's is.
	draws := make([]drawnTier, 0, len(s.Tiers))
	for _, t := range s.Tiers {
		switch {
		case centre && t == s.From:
			own, lent := partition([]string{opened}, onScreen)
			draws = append(draws, drawnTier{Tier: t, Role: roleCentre, IDs: own, Carried: lent})
		case slices.Contains(s.Keep, t):
			// A MARK OF THE RUNG ABOVE AT A KEPT TIER IS REFUSED: the client
			// draws it and counts it under neither set, and this rung's Marks
			// are its own, so a mark that survived onto the flank would be one
			// the two sides read differently in silence.
			ids := kept.At[t]
			if i := slices.IndexFunc(ids, func(id string) bool { return export.IsResidual(id) || export.IsGap(id) }); i >= 0 {
				return rung{}, export.Graph{}, fmt.Errorf("step %q keeps tier %d and the chart on screen draws the mark %q there, which is a column this walk has no shape for", s.Key, t, ids[i])
			}
			own, lent := partition(ids, onScreen)
			draws = append(draws, drawnTier{Tier: t, Role: roleFlank, IDs: own, Carried: lent})
		case slices.Contains(outward, t):
			// EVERY NODE THE DOCUMENT REACHES AT THE TIER, AND AN EMPTY COLUMN IS
			// STILL A COLUMN: reach.At carries no key for a tier nothing folds to,
			// which is a widened column the document leaves empty, and dropping it
			// here would spell "the step does not declare this column" for "the
			// document draws nothing in it".
			ids := reach.At[t]
			for _, id := range ids {
				next.Add(fresh.Nodes[id])
			}
			own, lent := partition(ids, inDocument)
			draws = append(draws, drawnTier{Tier: t, Role: roleOutward, IDs: own, Carried: lent})
		default:
			return rung{}, export.Graph{}, fmt.Errorf("step %q draws tier %d, which is neither its centre, a flank it keeps, nor a tier it opens into", s.Key, t)
		}
	}
	if !centre {
		next.Add(fresh.Nodes[opened])
	}
	// THE FRESH HALF'S RIBBONS ARRIVE WHOLE, at the ends export.Fold left them:
	// no cap re-points one at a tail here, because the column they end in is
	// answered unfolded, and Link refuses an end the spliced window does not
	// hold rather than dropping it.
	for _, l := range reach.Drawn.Links {
		if err := next.Link(l); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
	}
	// THE MARKS GO ON LAST, over the whole spliced window, in the order
	// shapeFor applies them: the residual first, off the window and the
	// document one depth up, and the gap after it, over the chart the
	// residual left -- what the opened node takes in against what it sends
	// out, once everything that is going to stand beside it does. A mark's
	// tier is the step's declared one and not a column set (drawnMark), and
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
	return rung{Step: s.Key, Draws: draws, Marks: marks}, next.Graph(), nil
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
