package export

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/jcrussell/livermore-budget/internal/fact"
)

// rootOutsideOverview is the first step opening from the view's own chart at
// a tier the view does not draw, or -1 when every root is placed.
func (v View) rootOutsideOverview() int {
	for i, s := range v.Steps {
		if slices.Contains(s.After, "") && !slices.Contains(v.Overview.drawnTiers(), s.From) {
			return i
		}
	}
	return -1
}

// parentChart is one chart a step opens from: the key naming it, "" for the
// view's own, with the column order it draws (not a set: a kept flank's side
// is a position in it) and the document it draws them of.
type parentChart struct {
	key   string
	tiers []int
	doc   string
	// keep is that chart's own kept flank, or nil where it keeps none.
	keep []int
}

// validateSteps refuses a drill tree a reader could not walk, and a step that
// would fold nothing, say nothing, draw a schedule some listed year's column
// does not carry, or draw one year's document under another year's chart.
// Every step is placed against every parent it names. A cycle cannot be
// declared, since After names only earlier steps, so nothing here detects one.
func (v View) validateSteps(ix ColumnIndex) error {
	// Keys first, as a pass of their own, so After resolves against a set
	// already known to name one step each.
	index := make(map[string]int, len(v.Steps))
	for i, s := range v.Steps {
		if s.Key == "" {
			return fmt.Errorf(
				"view %q declares step %d with no key; a step nothing can name is a chart no "+
					"other step can ever be declared to open from", v.Path, i)
		}
		if j, ok := index[s.Key]; ok {
			return fmt.Errorf(
				"view %q declares steps %d and %d both keyed %q; After would name two charts "+
					"and a rung would open from whichever was declared first",
				v.Path, j, i, s.Key)
		}
		index[s.Key] = i
	}
	// The columns a reader can open, which a gap licence must name one of: the
	// view's years, or its own document where it lists none.
	years := map[string]bool{}
	for _, stem := range append([]string{v.Projection}, v.YearStems...) {
		if col, folded := ix.Column(stem); folded {
			years[col] = true
		}
	}
	// The charts each step opens from, as a pass of their own, so the document
	// each draws can be asked of [StepStems] over a tree it will not refuse
	// for its shape. "" is the view's own chart, whose From validate places.
	parentsOf := make([][]parentChart, len(v.Steps))
	for i, s := range v.Steps {
		if len(s.After) == 0 {
			return fmt.Errorf(
				"view %q declares step %d opening from no chart at all; a step that opens "+
					"from the view's own chart says so with \"\", and an empty list is a rung "+
					"hanging off nothing", v.Path, i)
		}
		parents := make([]parentChart, 0, len(s.After))
		for k, a := range s.After {
			if slices.Contains(s.After[:k], a) {
				return fmt.Errorf(
					"view %q's step %d opens from %q twice; one chart reaching a step is one "+
						"edge, and every arm below would place the same chart twice and the "+
						"other parents once", v.Path, i, a)
			}
			if a == "" {
				parents = append(parents, parentChart{tiers: v.Overview.drawnTiers()})
				continue
			}
			j, ok := index[a]
			switch {
			case !ok:
				return fmt.Errorf(
					"view %q's step %d opens from %q, which no step declares as its key; the "+
						"breadcrumb would carry a rung hanging off a chart this view never draws",
					v.Path, i, a)
			case j >= i:
				return fmt.Errorf(
					"view %q's step %d opens from %q, which is step %d; After names an EARLIER "+
						"step, and that is what makes a cycle undeclarable rather than something "+
						"this has to detect", v.Path, i, a, j)
			}
			parents = append(parents, parentChart{key: a, tiers: v.Steps[j].drawnTiers(),
				keep: v.Steps[j].keptFlank()})
		}
		parentsOf[i] = parents
	}
	// The document each step draws, in every year the view lists, is one
	// answer: the one the page builder opens the rungs with. Nothing here
	// derives it a second way.
	byYear, err := v.stepStemsByYear(ix)
	if err != nil {
		return err
	}
	docs := byYear[v.Projection]
	for i, s := range v.Steps {
		parents := parentsOf[i]
		for k := range parents {
			parents[k].doc = v.Projection
			if parents[k].key != "" {
				parents[k].doc = docs[index[parents[k].key]]
			}
		}
		doc := docs[i]
		switch {
		case !slices.Contains(ChartForms(), s.Form):
			return fmt.Errorf(
				"view %q declares step %d in form %q, which is not one of %v; the client "+
					"would have no renderer to hand the opened node to", v.Path, i, s.Form, ChartForms())
		case s.Form == SankeyForm && s.Sankey == nil:
			return fmt.Errorf(
				"view %q declares step %d as a sankey with no sankey hints; what it draws "+
					"once a node opens is a declaration and not an inference", v.Path, i)
		case s.Tail == "":
			return fmt.Errorf(
				"view %q declares step %d with no tail noun, so a capped column would be "+
					"labelled \"24 smaller\" and stop there", v.Path, i)
		case s.Back == "":
			return fmt.Errorf(
				"view %q declares step %d with no back label, so the breadcrumb out of an "+
					"opened node would be a button with no words in it", v.Path, i)
		case s.Noun == "":
			return fmt.Errorf(
				"view %q declares step %d with no noun, so two rungs of one trail drawing "+
					"the same words could not be told apart", v.Path, i)
		case s.Description == "":
			return fmt.Errorf(
				"view %q declares step %d with no description, so a reader who cannot see "+
					"the chart would be told the opening state's over a chart it no longer "+
					"draws", v.Path, i)
		case !endsASentence(s.Description):
			return fmt.Errorf(
				"view %q gives step %d a description ending %q rather than in a sentence "+
					"terminator; the client appends the way back and the table pointer after "+
					"it, and an unterminated one runs into them", v.Path, i, lastRune(s.Description))
		case markKey(s.Residual) != "":
			return fmt.Errorf(
				"view %q's step %d declares a residual on %q, which is a mark the client "+
					"makes and no chart above sends flow from", v.Path, i, markKey(s.Residual))
		case markKey(s.Gaps) != "":
			return fmt.Errorf(
				"view %q's step %d licenses a gap on %q, which is a mark the client makes "+
					"and no document prints a total for", v.Path, i, markKey(s.Gaps))
		case s.Projection == "" && len(s.Residual) > 0:
			return fmt.Errorf(
				"view %q's step %d carries a residual of %d endpoint(s) and draws the "+
					"document before it; a residual is what one document prints at a grain "+
					"the other does not, and a step that switches no document has no second "+
					"grain", v.Path, i, len(s.Residual))
		case s.Projection == "" && len(s.Gaps) > 0:
			return fmt.Errorf(
				"view %q's step %d declares a gap on %d node(s) and draws the document "+
					"before it; a gap is one cell two documents print at two figures, and a "+
					"step that switches no document has only one", v.Path, i, len(s.Gaps))
		}
		// Every parent draws the tier this step opens from, whatever the form.
		for _, p := range parents {
			if p.key != "" && !slices.Contains(p.tiers, s.From) {
				return fmt.Errorf(
					"view %q's step %d opens from tier %d, and step %q draws tiers "+
						"%v, which do not include it; the breadcrumb would carry a rung nothing "+
						"on the chart can reach", v.Path, i, s.From, p.key, p.tiers)
			}
		}
		// One arm per form; a second form adds its own here.
		if s.Form == SankeyForm {
			if err := v.validateSankeyStep(i, s, parents, doc); err != nil {
				return err
			}
		}
		// One step per (After, From, Role), per SHARED parent rather than per
		// equal After list, and a role-less step takes the whole tier.
		for j, o := range v.Steps[:i] {
			if o.From != s.From {
				continue
			}
			shared, found := "", false
			for _, a := range s.After {
				if slices.Contains(o.After, a) {
					shared, found = a, true
					break
				}
			}
			if !found {
				continue
			}
			where := "the view's own chart"
			if shared != "" {
				where = fmt.Sprintf("step %q's chart", shared)
			}
			if o.Role == s.Role {
				return fmt.Errorf(
					"view %q declares steps %d and %d both opening tier %d of %s "+
						"in role %q; a node there would open into two different charts",
					v.Path, j, i, s.From, where, s.Role)
			}
			if o.Role == "" || s.Role == "" {
				return fmt.Errorf(
					"view %q declares steps %d and %d both opening tier %d of %s, "+
						"in roles %q and %q; a step with no role opens EVERY node at its tier, "+
						"so it cannot share one with a step that names which nodes open",
					v.Path, j, i, s.From, where, o.Role, s.Role)
			}
		}
		// Residual and gap SHAPE -- a grain exactly where a residual is, no
		// empty id or reason, no licence of 0 cents -- is page.schema.json's,
		// held at the write; what follows is what a schema cannot say.
		for _, id := range slices.Sorted(maps.Keys(s.Gaps)) {
			seen := map[[2]string]bool{}
			for _, g := range s.Gaps[id] {
				if !years[ColumnPath(g.FiscalYear, g.Basis)] {
					return fmt.Errorf(
						"view %q's step %d licenses a gap on node %q for %s, a column the view "+
							"lists no year of; the licence would match no chart the reader can open",
						v.Path, i, id, fact.ColumnLabel(g.FiscalYear, g.Basis))
				}
				col := [2]string{strconv.Itoa(g.FiscalYear), g.Basis}
				if seen[col] {
					return fmt.Errorf(
						"view %q's step %d declares two gaps on node %q for %s; the client "+
							"holds a column's chart to one licence, and would keep one and drop "+
							"the other in silence", v.Path, i, id, fact.ColumnLabel(g.FiscalYear, g.Basis))
				}
				seen[col] = true
			}
		}
	}
	return nil
}

// stepStemsByYear is [StepStems] over every year the view lists, keyed by
// year stem: the view's own document where it lists none. Whether any node
// of a rung's document opens is the client's to answer, and site/*.test.mjs
// walks every rung.
func (v View) stepStemsByYear(ix ColumnIndex) (map[string][]string, error) {
	years := v.YearStems
	if len(years) == 0 {
		years = []string{v.Projection}
	}
	out := make(map[string][]string, len(years))
	for _, year := range years {
		stems, err := StepStems(v.Steps, year, ix)
		if err != nil {
			return nil, fmt.Errorf("view %q, year stem %q: %w", v.Path, year, err)
		}
		out[year] = stems
	}
	return out, nil
}

// validateSankeyStep is the Sankey form's half of a step's validation: the
// window's shape, its placement against every chart it opens from, and its
// caps. The generic half is validateSteps'.
func (v View) validateSankeyStep(i int, s DrillStep, parents []parentChart, doc string) error {
	h := s.Sankey
	repeated := repeatedTier(h.Tiers)
	keptCap := -1
	for _, c := range h.Caps {
		if slices.Contains(h.Keep, c.Tier) {
			keptCap = c.Tier
		}
	}
	switch {
	case keptCap >= 0:
		return fmt.Errorf(
			"view %q's step %d caps tier %d, which it keeps; a kept flank is the chart "+
				"above's column as that chart drew it, and a cap of this step's on it would "+
				"be applied by the draw and not by the offer", v.Path, i, keptCap)
	case len(h.Tiers) == 0:
		return fmt.Errorf(
			"view %q declares step %d with no tiers, so a node opened on it would be "+
				"drawn by the same tier set it was closed under", v.Path, i)
	case len(s.Gaps) > 0 && len(h.Widen) > 0:
		return fmt.Errorf(
			"view %q's step %d declares a gap on %d node(s) and widens tiers %v; the "+
				"client stands the gap mark at the step's first or last declared tier, and "+
				"a viewport that does not buy that tier would draw the mark in a column "+
				"that is not there", v.Path, i, len(s.Gaps), h.Widen)
	case h.Side != "" && h.Side != SideSource && h.Side != SideBoth:
		return fmt.Errorf(
			"view %q's step %d opens side %q; the sides are \"\", the node a link points "+
				"at, %q, the node it comes from, and %q, a node drawn between the two",
			v.Path, i, h.Side, SideSource, SideBoth)
	case h.Side == SideBoth && (len(s.Gaps) > 0 || len(s.Residual) > 0):
		return fmt.Errorf(
			"view %q's step %d opens a node on both sides and declares a gap or a residual; the "+
				"client stands either mark at one end of the step's tiers, and a node drawn between "+
				"two halves has no one end that mark belongs to", v.Path, i)
	case h.Side == SideBoth && !interior(h.Tiers, s.From):
		return fmt.Errorf(
			"view %q's step %d opens tier %d on both sides and draws tiers %v; a node drawn "+
				"between what enters it and what leaves it is a column with one on each side, "+
				"so the opened tier must be neither end, with lower tiers before it and higher "+
				"after", v.Path, i, s.From, h.Tiers)
	case len(h.Keep) > 0 && h.Side != "":
		return fmt.Errorf(
			"view %q's step %d keeps tier(s) %v and opens side %q; a window's opened node "+
				"is the TARGET of one half and the SOURCE of the other, so its side is "+
				"both and a step declaring one would be two declarations of one thing",
			v.Path, i, h.Keep, h.Side)
	case len(h.Widen) > 0 && len(h.Keep) == 0:
		return fmt.Errorf(
			"view %q's step %d widens by tier(s) %v and keeps no flank; a step that keeps "+
				"nothing draws the opened node's parts alone and has no centre to add a "+
				"column out from, so the widening names a construct this is not",
			v.Path, i, h.Widen)
	case repeated >= 0:
		return fmt.Errorf(
			"view %q's step %d draws tiers %v, which name tier %d twice; a tier is a "+
				"column, two columns of one tier is the same nodes drawn twice, and which "+
				"end a kept flank is at could not be read off the list either",
			v.Path, i, h.Tiers, repeated)
	case len(h.Keep) > 0 && len(h.Tiers) != len(h.Keep)+2+len(h.Widen):
		return fmt.Errorf(
			"view %q's step %d keeps tier(s) %v, widens by %d column(s) and draws tiers "+
				"%v; a window is its kept flank, the node that was opened and what it opens "+
				"into, one column each and one more for every widening -- %d columns here, "+
				"and not %d",
			v.Path, i, h.Keep, len(h.Widen), h.Tiers, len(h.Keep)+2+len(h.Widen), len(h.Tiers))
	}
	// Which end the flank is at is read off Tiers once; every arm after
	// this reads that answer.
	keptLeft := false
	if m := len(h.Keep); m > 0 {
		n := len(h.Tiers)
		switch {
		case slices.Equal(h.Tiers[:m], reversedTiers(h.Keep)):
			keptLeft = true
		case slices.Equal(h.Tiers[n-m:], h.Keep):
		default:
			return fmt.Errorf(
				"view %q's step %d keeps tier(s) %v and draws tiers %v, whose ends are not "+
					"that flank; the kept columns are the ones at ONE end of what the step "+
					"draws, outermost first, and a widening on the flank's side would push "+
					"them off it -- a flank two columns deep is declared by keeping two",
				v.Path, i, h.Keep, h.Tiers)
		}
		centre := m
		if !keptLeft {
			centre = n - 1 - m
		}
		if h.Tiers[centre] != s.From {
			return fmt.Errorf(
				"view %q's step %d keeps tier(s) %v and draws tiers %v, whose column %d is "+
					"tier %d and not the opened tier %d; the node the reader clicked is the "+
					"centre of a window", v.Path, i, h.Keep, h.Tiers, centre, h.Tiers[centre], s.From)
		}
		if w := len(h.Widen); w > 0 {
			drawn, want := h.Tiers[n-w:], h.Widen
			if !keptLeft {
				drawn, want = h.Tiers[:w], reversedTiers(h.Widen)
			}
			if !slices.Equal(drawn, want) {
				return fmt.Errorf(
					"view %q's step %d widens by tier(s) %v and draws tiers %v, whose %d "+
						"column(s) away from the kept flank are %v; a widened column is on the "+
						"opened node's side and the widening order is the order OUT from it, so "+
						"a client dropping the last of them draws a narrower window and not a "+
						"hole", v.Path, i, h.Widen, h.Tiers, w, drawn)
			}
		}
	}
	// Every parent places this step, not the first one that fits.
	for _, p := range parents {
		where := "the view's own chart"
		if p.key != "" {
			where = fmt.Sprintf("step %q", p.key)
		}
		// Same document only, and equality not containment (fisc-ke1f): a
		// strict subset of a widened parent's tiers is a narrower chart.
		if p.key != "" && doc == p.doc && slices.Equal(p.tiers, h.Tiers) {
			return fmt.Errorf(
				"view %q's step %d draws tiers %v of %q, the set step %q "+
					"already draws; opening a node would redraw the chart it was opened "+
					"from", v.Path, i, h.Tiers, doc, p.key)
		}
		// A kept flank is drawn at its share of the centre, not whole, so
		// nothing on one may open: the node would take one figure in and
		// send its whole decomposition out. Per parent, every flank column.
		if slices.Contains(p.keep, s.From) {
			return fmt.Errorf(
				"view %q's step %d opens tier %d of %s, which KEEPS that tier; a kept "+
					"flank is drawn at its share of that chart's centre rather than whole, "+
					"so this step would draw a node taking one figure in and sending its "+
					"whole decomposition out, with the difference left as node height "+
					"nothing accounts for", v.Path, i, s.From, where)
		}
		if len(h.Keep) == 0 {
			continue
		}
		// Every parent here has a column order and already contains From,
		// so fi >= 0. The flank walks out from the opened node.
		fi := slices.Index(p.tiers, s.From)
		step := -1
		if !keptLeft {
			step = 1
		}
		for n, k := range h.Keep {
			ki := slices.Index(p.tiers, k)
			drawnSide := "RIGHT"
			if ki < fi {
				drawnSide = "LEFT"
			}
			switch {
			case ki < 0:
				return fmt.Errorf(
					"view %q's step %d keeps tier %d and opens from %s, which draws tiers %v "+
						"and does not include it; the flank the reader came from has to be a "+
						"column they were looking at", v.Path, i, k, where, p.tiers)
			case n == 0 && ki != fi-1 && ki != fi+1:
				return fmt.Errorf(
					"view %q's step %d keeps tier %d and opens tier %d of %s, which draws "+
						"them as columns %d and %d of %v; a window slides by one column, and "+
						"which way it slides is the SIGN of that adjacency",
					v.Path, i, k, s.From, where, ki, fi, p.tiers)
			case n == 0 && ki != fi+step:
				return fmt.Errorf(
					"view %q's step %d keeps tier %d, which %s draws to the %s of the "+
						"opened tier %d, and draws tiers %v, which put it at the other end; "+
						"the kept flank stays on the side the reader saw it on",
					v.Path, i, k, where, drawnSide, s.From, h.Tiers)
			case ki != fi+step*(n+1):
				return fmt.Errorf(
					"view %q's step %d keeps tier(s) %v and opens tier %d of %s, which draws "+
						"tier %d as column %d of %v and the opened tier as column %d; a flank "+
						"is the columns BESIDE EACH OTHER walking out from the node that was "+
						"opened, so a gap in it is a column the reader was looking at dropped "+
						"out of the middle of the ones that stay",
					v.Path, i, h.Keep, s.From, where, k, ki, p.tiers, fi)
			}
		}
	}
	for j, c := range h.Caps {
		switch {
		case c.Cap < 1:
			return fmt.Errorf(
				"view %q's step %d caps tier %d at %d; the cap is what keeps a fine "+
					"column drawable and a column of one node is not a chart",
				v.Path, i, c.Tier, c.Cap)
		case !slices.Contains(h.Tiers, c.Tier):
			return fmt.Errorf(
				"view %q's step %d caps tier %d and draws tiers %v, which do not include "+
					"it; the cap would fold nothing, in silence", v.Path, i, c.Tier, h.Tiers)
		case slices.ContainsFunc(h.Caps[:j], func(o TierCap) bool { return o.Tier == c.Tier }):
			return fmt.Errorf("view %q's step %d caps tier %d twice", v.Path, i, c.Tier)
		}
	}
	return nil
}

// repeatedTier returns a tier the list names twice, or -1.
func repeatedTier(tiers []int) int {
	seen := make(map[int]bool, len(tiers))
	for _, t := range tiers {
		if seen[t] {
			return t
		}
		seen[t] = true
	}
	return -1
}

// reversedTiers is tiers back to front, in a copy: the callers hold the
// caller's own [DrillStep] fields.
func reversedTiers(tiers []int) []int {
	out := slices.Clone(tiers)
	slices.Reverse(out)
	return out
}

// endsASentence reports whether s closes with a sentence terminator.
func endsASentence(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '!', '?':
		return true
	}
	return false
}

// lastRune is the final character of s as a string.
func lastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	return string(r[len(r)-1])
}
