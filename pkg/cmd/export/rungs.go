package export

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/schema"
)

// rungsPath is the committed copy of the rung answer, pinned byte for byte to
// what the site serves at rungsServedPath.
const rungsPath = "testdata/rungs.json"

// rungsServedPath is where the rung answer lands in the site: one file for
// every year and rung, so the client answers a drill by lookup, not fetch.
const rungsServedPath = export.RungsPath

// rungsSchemaVersion is the version a rungsDoc declares, spelled once. The
// arm reading the artifact refuses any other.
const rungsSchemaVersion = 1

// rungsDoc is Go's reading of the declared steps against the documents they
// draw: every figure is computed by export.ReachOf, not re-encoded from a
// declaration.
type rungsDoc struct {
	SchemaVersion int `json:"schema_version"`
	// GeneratedBy lets the client refuse an answer from a different run than the
	// page; the site publishes no cache-busting.
	GeneratedBy string       `json:"generated_by"`
	Columns     []rungColumn `json:"columns"`
}

// rungColumn is one published year, by the spine document's stem, which is
// how the client names a column.
type rungColumn struct {
	Stem  string `json:"stem"`
	Rungs []rung `json:"rungs"`
}

// A rung is one opened path: the step that opened its last node, what each
// column the step declares holds in draw order, and the marks the client adds
// that no document prints, sorted by id. It is answered once; fitting it to a
// screen is the client's.
type rung struct {
	Path  []string    `json:"path"`
	Step  string      `json:"step"`
	Draws []drawnTier `json:"draws"`
	Marks []drawnMark `json:"marks,omitempty"`
	// Amounts is the figure a mark prints, only for nodes whose drawn ribbons do
	// not add up to it: a node a schedule prints a reduction under, the reduction
	// drawn forward at its magnitude. It is the signed sum of the arriving
	// ribbons. The client draws a named node at this amount and lets the layout
	// size every other.
	Amounts map[string]int64 `json:"amounts,omitempty"`
}

// drawnMark is one node the client draws on a rung that no page prints: a gap
// (markGap) or a residual (carryResidual). Go answers which exists, its tier,
// its cents and its words. It sits on the rung, not a column, because its tier
// indexes the step's declared tiers. A gap has exactly one of InCents and
// OutCents, the side the short one stands on; Ends is a residual's declared
// endpoints, sorted.
//
// A mark is fold-invariant, since a fold preserves what arrives at and leaves
// the opened node, so it is answered once per rung.
type drawnMark struct {
	ID       string   `json:"id"`
	Role     string   `json:"role"`
	Tier     int      `json:"tier"`
	InCents  int64    `json:"in_cents,omitempty"`
	OutCents int64    `json:"out_cents,omitempty"`
	Ends     []string `json:"ends,omitempty"`
	// Converted from export.Mark, so a field added there and not here does not
	// compile.
	Label      string           `json:"label"`
	Rationale  string           `json:"rationale"`
	SourceNote string           `json:"source_note"`
	Locators   []export.Locator `json:"locators,omitempty"`
}

// drawnTier is one column of one rung, in column order (revenue-category's is
// [1,0,2]). Role is centre for the opened node's column, flank for a kept
// column, outward for one the node opens into.
//
// IDs, sorted, is every node the document draws at the tier as the opened
// node's parts; Carried is every drawn node the client does not count (derived,
// or named by the step's residual). IDs are unfolded: which fit a viewport is
// the client's (DrillStep.Caps, DrillStep.Widen). IDs is written even when
// empty, so a column drawing nothing reads apart from one not answered.
type drawnTier struct {
	Tier    int      `json:"tier"`
	Role    string   `json:"role"`
	IDs     []string `json:"ids"`
	Carried []string `json:"carried,omitempty"`
	// Needs is, for each id of this column whose every ribbon leads into a
	// widened column, the widened tier it is drawn with: a viewport that drops
	// that column leaves the id no ribbon to be drawn by. pp.173-183 print
	// spending for funds pp.127-140 print no revenue for, so in a group's
	// window such a fund reaches only the object categories.
	Needs map[string]int `json:"needs,omitempty"`
}

const (
	roleCentre  = "centre"
	roleFlank   = "flank"
	roleOutward = "outward"
)

// contraSum is the signed sum of the ribbons arriving at node, reported only
// where one is a reduction; everywhere else the layout already arrives at it.
// The arriving side is the one reductions are on.
func contraSum(g export.Graph, node string) (int64, bool) {
	var sum int64
	contra := false
	for _, l := range g.Links {
		if l.Target != node {
			continue
		}
		sum += l.ValueCents
		if l.ValueCents < 0 {
			contra = true
		}
	}
	return sum, contra
}

// rungsOf walks the spine's declared steps over the built documents, for every
// published year, and answers each rung it reaches the way the client reads
// and draws it: a node opens under a step whose After, From and Role match and
// whose document decomposes it (export.Openable), and each column is read
// through the hierarchy by export.ReachOf. Residual and gap marks are answered
// under Marks; the folded tail is not, since only a viewport makes it.
func rungsOf(projections map[string][]byte, spine export.View) (rungsDoc, error) {
	doc := rungsDoc{SchemaVersion: rungsSchemaVersion, GeneratedBy: generatedBy()}
	// The page's own resolution, by the same call, so the answer and the page
	// name the same document for a rung.
	_, ix, err := export.ColumnsOf(projections, "")
	if err != nil {
		return rungsDoc{}, fmt.Errorf("rungs: %w", err)
	}
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
		stems, err := export.StepStems(spine.Steps, year, ix)
		if err != nil {
			return rungsDoc{}, fmt.Errorf("rungs: %s: %w", year, err)
		}
		col := rungColumn{Stem: year}
		w := rungWalker{spine: spine, projections: projections, stems: stems, year: year}
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

// answerNeeds fills each outward column's Needs: dropping the widened columns
// from the end, as the client does, an id the narrower reach no longer draws
// needs the last widened column dropped.
func answerNeeds(g export.Graph, s export.DrillStep, opened string, nearIsSource bool,
	half, outward []int, full export.Reach, draws []drawnTier) error {
	needs := map[string]int{}
	for k := len(s.Widen) - 1; k >= 0; k-- {
		narrow := slices.DeleteFunc(slices.Clone(half), func(t int) bool { return slices.Contains(s.Widen[k:], t) })
		r, err := export.ReachOf(g, opened, nearIsSource, narrow)
		if err != nil {
			return fmt.Errorf("step %q opens %q without tier %d: %w", s.Key, opened, s.Widen[k], err)
		}
		for _, t := range narrow {
			// A widened column's own ids are drawn with it by definition.
			if !slices.Contains(outward, t) || slices.Contains(s.Widen, t) {
				continue
			}
			for _, id := range full.At[t] {
				if _, known := needs[id]; !known && !slices.Contains(r.At[t], id) {
					needs[id] = s.Widen[k]
				}
			}
		}
	}
	for i := range draws {
		for _, id := range slices.Concat(draws[i].IDs, draws[i].Carried) {
			if t, ok := needs[id]; ok {
				if draws[i].Needs == nil {
					draws[i].Needs = map[string]int{}
				}
				draws[i].Needs[id] = t
			}
		}
	}
	return nil
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

// overviewOf is the spine's chart before anything is opened: the document
// folded to the view's render tiers, as the client draws it.
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
	// year is the spine's own document stem, which a step after "" opens from.
	year string
}

// walk answers every node the chart on screen offers, and the charts those
// open in turn. from is the unfolded document the chart was shaped from, which
// a residual's ribbons come off. The side and outward tiers are read off the
// step as the client's shapeFor reads them. It descends the unfolded chart, so
// a node a fold hides is still answered (fisc-qics).
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
		parents, err := export.FlankDocuments(w.spine, s, w.year, w.stems, w.projections)
		if err != nil {
			return err
		}
		opens, err := export.Openable(w.spine, i, s, stem, raw, parents)
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
		var col export.ColumnKey
		if len(s.Gaps) > 0 {
			if col, err = export.ColumnKeyOf(raw); err != nil {
				return fmt.Errorf("step %q declares a gap and %s: %w", s.Key, stem, err)
			}
		}
		for _, n := range chart.Nodes {
			if n.Tier != s.From || (s.Role != "" && n.Role != s.Role) || !slices.Contains(opens, n.ID) {
				continue
			}
			r, next, err := w.answer(g, chart, from, col, s, n.ID, nearIsSource, outward)
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

// answer is one rung, and the chart it leaves on screen for the next step: the
// whole window, kept half spliced with fresh half, the kept record winning an
// id both hold. outward is every tier the step draws beyond its centre,
// nearest first, or all of them when it keeps no flank.
func (rungWalker) answer(g, screen, from export.Graph, col export.ColumnKey, s export.DrillStep, opened string, nearIsSource bool, outward []int) (rung, export.Graph, error) {
	centre := len(s.Keep) > 0
	// Every column this half declares, centre included, widened or not: Widen
	// fits a viewport and does not change what the document draws.
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
	// The kept half is the chart on screen read from the other end: a left
	// flank is what flows into the opened node.
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
		// The kept centre must be the opened node alone, and hold it: the client
		// refuses a window whose flank sends nothing into the node.
		if others := slices.DeleteFunc(slices.Clone(kept.At[s.From]), func(id string) bool { return id == opened }); len(others) > 0 {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q and the chart on screen draws %v beside it at tier %d, which is a column this walk has no shape for", s.Key, opened, others, s.From)
		}
		if !slices.Contains(kept.At[s.From], opened) {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q and the chart on screen sends nothing between tiers %v and it, so there is no flank to keep and the client would refuse the window", s.Key, opened, keptTiers)
		}
	}
	// The centre is the opened node alone: a ribbon into another node of that
	// tier would draw beside it, and this artifact has no shape for that.
	if centre {
		if others := slices.DeleteFunc(slices.Clone(reach.At[s.From]), func(id string) bool { return id == opened }); len(others) > 0 {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q and the document draws %v beside it at tier %d, which is a column this walk has no shape for", s.Key, opened, others, s.From)
		}
	}
	fresh := export.IndexGraph(reach.Drawn)
	next := export.IndexGraph(kept.Drawn)
	// Drawn but not counted: a derived node or one the residual declares, read
	// off the flank's record for flank ids and the document's for fresh ones.
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
	// One pass answers every declared tier and builds the chart the rung leaves,
	// so the two cannot be two readings of one document.
	var amounts map[string]int64
	draws := make([]drawnTier, 0, len(s.Tiers))
	for _, t := range s.Tiers {
		switch {
		case centre && t == s.From:
			own, lent := partition([]string{opened}, onScreen)
			draws = append(draws, drawnTier{Tier: t, Role: roleCentre, IDs: own, Carried: lent})
			if cents, ok := contraSum(reach.Drawn, opened); ok {
				if amounts == nil {
					amounts = map[string]int64{}
				}
				amounts[opened] = cents
			}
		case slices.Contains(s.Keep, t):
			// A mark of the rung above on a kept flank is refused: the two sides would
			// read it differently in silence.
			ids := kept.At[t]
			if i := slices.IndexFunc(ids, func(id string) bool { return export.IsResidual(id) || export.IsGap(id) }); i >= 0 {
				return rung{}, export.Graph{}, fmt.Errorf("step %q keeps tier %d and the chart on screen draws the mark %q there, which is a column this walk has no shape for", s.Key, t, ids[i])
			}
			own, lent := partition(ids, onScreen)
			draws = append(draws, drawnTier{Tier: t, Role: roleFlank, IDs: own, Carried: lent})
		case slices.Contains(outward, t):
			// An empty column is still a column: dropping it would say the step does
			// not declare it.
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
	if err := answerNeeds(g, s, opened, nearIsSource, half, outward, reach, draws); err != nil {
		return rung{}, export.Graph{}, err
	}
	if !centre {
		next.Add(fresh.Nodes[opened])
	}
	// The fresh ribbons arrive unfolded; Link refuses an end the window does
	// not hold rather than dropping it.
	for _, l := range reach.Drawn.Links {
		if err := next.Link(l); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
	}
	// Marks go on last, residual then gap, as the client's shapeFor applies
	// them, and each is added to the next rung's chart so a flank it survives onto
	// is refused above.
	var marks []drawnMark
	drawn := next.Graph()
	// A residual is carried only across a document switch, carryResidual's own
	// gate.
	residual := s.Residual
	if s.Projection == "" {
		residual = nil
	}
	if c, ok, err := export.ResidualOf(drawn, from, g, opened, s.Tiers, residual, s.ResidualGrain); err != nil {
		return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
	} else if ok {
		if next, err = carry(drawn, c); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
		marks = append(marks, drawnMark(c.Mark))
		drawn = next.Graph()
	}
	if c, ok, err := export.GapOf(drawn, from, g, col, opened, s.Tiers, s.Gaps); err != nil {
		return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
	} else if ok {
		if next, err = carry(drawn, c); err != nil {
			return rung{}, export.Graph{}, fmt.Errorf("step %q opens %q: %w", s.Key, opened, err)
		}
		marks = append(marks, drawnMark(c.Mark))
	}
	slices.SortFunc(marks, func(a, b drawnMark) int { return strings.Compare(a.ID, b.ID) })
	return rung{Step: s.Key, Draws: draws, Marks: marks, Amounts: amounts}, next.Graph(), nil
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
	// A mark's tier against its rung's columns is a cross-reference the schema
	// cannot state.
	for _, col := range doc.Columns {
		for _, r := range col.Rungs {
			for _, m := range r.Marks {
				if !slices.ContainsFunc(r.Draws, func(d drawnTier) bool { return d.Tier == m.Tier }) {
					return nil, fmt.Errorf("%s %s: mark %q stands at tier %d, which the rung draws no column at",
						col.Stem, strings.Join(r.Path, " > "), m.ID, m.Tier)
				}
			}
		}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')

	// Refused at the write, not the fetch: the schema is the one statement of
	// the shape.
	resolved, rerr := schema.Load(schema.Rungs)
	if rerr != nil {
		return nil, rerr
	}
	var v any
	if uerr := json.Unmarshal(b, &v); uerr != nil {
		return nil, fmt.Errorf("re-read the rung answer: %w", uerr)
	}
	if verr := resolved.Validate(v); verr != nil {
		return nil, fmt.Errorf("the rung answer this build produced does not match %s: %w",
			schema.Rungs, verr)
	}
	return b, nil
}
