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

// name is the chart as a refusal names it.
func (p parentChart) name() string {
	if p.key == "" {
		return "the view's own chart"
	}
	return fmt.Sprintf("step %q", p.key)
}

// validateSteps refuses a drill tree a reader could not walk, and a step that
// would open nothing, fold nothing, say nothing, draw a schedule some listed
// year's column does not carry, or draw one year's document under another
// year's chart.
// Every step is placed against every parent it names. A cycle cannot be
// declared, since After names only earlier steps, so nothing here detects one.
//
// It returns the first refusal it meets: which one a tree carrying several
// defects reports depends on the order of the calls it makes, and the tests
// pin those messages.
func (v View) validateSteps(ix ColumnIndex) error {
	index, err := v.stepIndex()
	if err != nil {
		return err
	}
	parentsOf, err := v.stepParents(index)
	if err != nil {
		return err
	}
	// The document each step draws, in every year the view lists, is one
	// answer: the one the page builder opens the rungs with. Nothing here
	// derives it a second way.
	byYear, err := v.stepStemsByYear(ix)
	if err != nil {
		return err
	}
	t := drillTree{
		index:     index,
		columns:   v.openableColumns(ix),
		parentsOf: parentsOf,
		byYear:    byYear,
		docs:      byYear[v.Projection],
		yearStems: v.yearStems(),
	}
	if err := v.validateOverviewTiersCarried(t.yearStems, ix); err != nil {
		return err
	}
	for i, s := range v.Steps {
		if err := v.validateStep(i, s, t, ix); err != nil {
			return err
		}
	}
	return nil
}

// drillTree is what validateSteps learns of a view's steps before it places
// any one of them, and what validateStep reads back for each.
type drillTree struct {
	// index is each step's position by its key.
	index map[string]int
	// columns is [View.openableColumns].
	columns map[string]bool
	// parentsOf is the charts each step opens from, in After's order.
	parentsOf [][]parentChart
	// byYear is the document each step draws, keyed by year stem, and docs
	// is byYear's entry for the view's own document.
	byYear map[string][]string
	docs   []string
	// yearStems is [View.yearStems].
	yearStems []string
}

// stemsOf is every document step i draws across the years the view lists,
// once each.
func (t drillTree) stemsOf(i int) []string {
	var out []string
	for _, year := range t.yearStems {
		if stem := t.byYear[year][i]; !slices.Contains(out, stem) {
			out = append(out, stem)
		}
	}
	return out
}

// placedParents is step i's parents, each given the document it draws: the
// view's own for its chart, and for a step the one that step resolves to.
// The entries of parentsOf are written in place.
func (t drillTree) placedParents(i int, projection string) []parentChart {
	parents := t.parentsOf[i]
	for k := range parents {
		parents[k].doc = projection
		if parents[k].key != "" {
			parents[k].doc = t.docs[t.index[parents[k].key]]
		}
	}
	return parents
}

// above is every document the charts this step opens from draw, across the
// years the view lists.
func (t drillTree) above(parents []parentChart) []string {
	var out []string
	for _, p := range parents {
		if p.key == "" {
			out = append(out, t.yearStems...)
		} else {
			out = append(out, t.stemsOf(t.index[p.key])...)
		}
	}
	return out
}

// stepIndex is each step's position by its key. It refuses a step with no
// key and two steps sharing one.
//
// Keys first, as a pass of their own, so After resolves against a set
// already known to name one step each.
func (v View) stepIndex() (map[string]int, error) {
	index := make(map[string]int, len(v.Steps))
	for i, s := range v.Steps {
		if s.Key == "" {
			return nil, fmt.Errorf(
				"view %q declares step %d with no key; a step nothing can name is a chart no "+
					"other step can ever be declared to open from", v.Path, i)
		}
		if j, ok := index[s.Key]; ok {
			return nil, fmt.Errorf(
				"view %q declares steps %d and %d both keyed %q; After would name two charts "+
					"and a rung would open from whichever was declared first",
				v.Path, j, i, s.Key)
		}
		index[s.Key] = i
	}
	return index, nil
}

// openableColumns is the columns a reader can open, which a gap licence must
// name one of: the view's years, or its own document where it lists none.
func (v View) openableColumns(ix ColumnIndex) map[string]bool {
	years := map[string]bool{}
	for _, stem := range append([]string{v.Projection}, v.YearStems...) {
		if col, folded := ix.Column(stem); folded {
			years[col] = true
		}
	}
	return years
}

// stepParents is the charts each step opens from. It refuses a step opening
// from no chart, from one chart twice, from a key no step declares, or from a
// step that is not an earlier one.
//
// The charts each step opens from, as a pass of their own, so the document
// each draws can be asked of [StepStems] over a tree it will not refuse
// for its shape. "" is the view's own chart, whose From validate places.
func (v View) stepParents(index map[string]int) ([][]parentChart, error) {
	parentsOf := make([][]parentChart, len(v.Steps))
	for i, s := range v.Steps {
		if len(s.After) == 0 {
			return nil, fmt.Errorf(
				"view %q declares step %d opening from no chart at all; a step that opens "+
					"from the view's own chart says so with \"\", and an empty list is a rung "+
					"hanging off nothing", v.Path, i)
		}
		parents := make([]parentChart, 0, len(s.After))
		for k, a := range s.After {
			if slices.Contains(s.After[:k], a) {
				return nil, fmt.Errorf(
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
				return nil, fmt.Errorf(
					"view %q's step %d opens from %q, which no step declares as its key; the "+
						"breadcrumb would carry a rung hanging off a chart this view never draws",
					v.Path, i, a)
			case j >= i:
				return nil, fmt.Errorf(
					"view %q's step %d opens from %q, which is step %d; After names an EARLIER "+
						"step, and that is what makes a cycle undeclarable rather than something "+
						"this has to detect", v.Path, i, a, j)
			}
			parents = append(parents, parentChart{key: a, tiers: v.Steps[j].drawnTiers(),
				keep: v.Steps[j].keptFlank()})
		}
		parentsOf[i] = parents
	}
	return parentsOf, nil
}

// validateOverviewTiersCarried refuses a view's own chart drawing a tier no
// document of any year the view lists carries a node at.
//
// A declaration is held against the documents it draws, in every year
// the view lists: the view's own for its chart, and for each step the
// one it resolves to, which the kept flank is not drawn of.
func (v View) validateOverviewTiersCarried(yearStems []string, ix ColumnIndex) error {
	if t := undrawnTier(v.Overview.drawnTiers(), nil, yearStems, ix); t >= 0 {
		return fmt.Errorf(
			"view %q's chart draws tier %d, and %v carries nodes at tiers %v and none "+
				"at that one in any year the view lists; the column would be empty on "+
				"every year's chart", v.Path, t, yearStems, tiersOf(yearStems, ix))
	}
	return nil
}

// validateStep places step i against the tree validateSteps learned, one
// concern per call, and returns the first refusal it meets.
func (v View) validateStep(i int, s DrillStep, t drillTree, ix ColumnIndex) error {
	parents := t.placedParents(i, v.Projection)
	doc := t.docs[i]
	if err := v.validateStepDeclares(i, s); err != nil {
		return err
	}
	if err := v.validateStepMarks(i, s, parents, doc); err != nil {
		return err
	}
	if err := v.validateStepTiersCarried(i, s, t.stemsOf(i), ix); err != nil {
		return err
	}
	if err := v.validateMarksCarried(i, s, t.above(parents), ix); err != nil {
		return err
	}
	if err := v.validateParentsDrawFrom(i, s, parents); err != nil {
		return err
	}
	if err := v.validateRoleCarried(i, s, t, parents, ix); err != nil {
		return err
	}
	// One arm per form; a second form adds its own here.
	if s.Form == SankeyForm {
		if err := v.validateSankeyStep(i, s, parents, doc); err != nil {
			return err
		}
	}
	if err := v.validateOneStepPerRole(i, s); err != nil {
		return err
	}
	return v.validateGapColumns(i, s, t.columns)
}

// validateStepDeclares refuses a step whose form has no renderer, a sankey
// with no sankey hints, and a step missing its tail noun, back label, noun or
// description, or whose description ends in no sentence terminator.
func (v View) validateStepDeclares(i int, s DrillStep) error {
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
	}
	return nil
}

// validateStepMarks refuses a residual or a gap keyed on a mark the client
// makes, and a residual or a gap on a step that switches no document.
func (v View) validateStepMarks(i int, s DrillStep, parents []parentChart, doc string) error {
	// A chart this step opens from that draws the document this step
	// draws, or -1: against it, the step switches no document, whatever
	// Projection spells.
	same := slices.IndexFunc(parents, func(p parentChart) bool { return p.doc == doc })
	switch {
	case markKey(s.Residual) != "":
		return fmt.Errorf(
			"view %q's step %d declares a residual on %q, which is a mark the client "+
				"makes and no chart above sends flow from", v.Path, i, markKey(s.Residual))
	case markKey(s.Gaps) != "":
		return fmt.Errorf(
			"view %q's step %d licenses a gap on %q, which is a mark the client makes "+
				"and no document prints a total for", v.Path, i, markKey(s.Gaps))
	case same >= 0 && len(s.Residual) > 0:
		return fmt.Errorf(
			"view %q's step %d carries a residual of %d endpoint(s) and draws %q, the "+
				"document %s draws; a residual is what one document prints at a grain "+
				"the other does not, and a step that switches no document has no second "+
				"grain", v.Path, i, len(s.Residual), doc, parents[same].name())
	case same >= 0 && len(s.Gaps) > 0:
		return fmt.Errorf(
			"view %q's step %d declares a gap on %d node(s) and draws %q, the document "+
				"%s draws; a gap is one cell two documents print at two figures, and a "+
				"step that switches no document has only one", v.Path, i, len(s.Gaps), doc,
			parents[same].name())
	}
	return nil
}

// validateStepTiersCarried refuses a step drawing a tier, its kept flank
// aside, that none of the documents it draws carries a node at in any year
// the view lists.
func (v View) validateStepTiersCarried(i int, s DrillStep, drawn []string, ix ColumnIndex) error {
	if t := undrawnTier(s.drawnTiers(), s.keptFlank(), drawn, ix); t >= 0 {
		return fmt.Errorf(
			"view %q's step %d draws tier %d, and %v carries nodes at tiers %v and none "+
				"at that one in any year the view lists; the column would be empty on "+
				"every chart this step draws", v.Path, i, t, drawn, tiersOf(drawn, ix))
	}
	return nil
}

// validateMarksCarried refuses a residual endpoint or a gap node that names
// no node of the documents above in any year the view lists.
//
// A residual's endpoints and a gap's nodes are the chart above's, and
// the client draws an id matching nothing as nothing, in silence.
func (v View) validateMarksCarried(i int, s DrillStep, above []string, ix ColumnIndex) error {
	for _, id := range slices.Sorted(maps.Keys(s.Residual)) {
		if !carried(above, id, ix) {
			return fmt.Errorf(
				"view %q's step %d declares a residual on %q, which names no node of %v in "+
					"any year the view lists; the client would re-point nothing onto the "+
					"mark and draw the rest as though the endpoint balanced", v.Path, i, id, above)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(s.Gaps)) {
		if !carried(above, id, ix) {
			return fmt.Errorf(
				"view %q's step %d licenses a gap on %q, which names no node of %v in any "+
					"year the view lists; the licence would match no node a reader can open",
				v.Path, i, id, above)
		}
	}
	return nil
}

// validateParentsDrawFrom refuses a step opening from a tier some parent
// step does not draw.
//
// Every parent draws the tier this step opens from, whatever the form.
func (v View) validateParentsDrawFrom(i int, s DrillStep, parents []parentChart) error {
	for _, p := range parents {
		if p.key != "" && !slices.Contains(p.tiers, s.From) {
			return fmt.Errorf(
				"view %q's step %d opens from tier %d, and step %q draws tiers "+
					"%v, which do not include it; the breadcrumb would carry a rung nothing "+
					"on the chart can reach", v.Path, i, s.From, p.key, p.tiers)
		}
	}
	return nil
}

// validateRoleCarried refuses a step naming a role that no node at the tier
// it opens carries, in what some chart it opens from draws in any year the
// view lists.
//
// The client opens a node on a step only where the node's role is the step's,
// so such a step is a rung no reader can reach, shipped in silence.
func (v View) validateRoleCarried(i int, s DrillStep, t drillTree, parents []parentChart, ix ColumnIndex) error {
	if s.Role == "" {
		return nil
	}
	for _, p := range parents {
		stems := t.above([]parentChart{p})
		if !slices.ContainsFunc(stems, func(stem string) bool { return ix.CarriesRole(stem, s.From, s.Role) }) {
			return fmt.Errorf(
				"view %q's step %d opens role %q at tier %d of %s, and %v carries no node "+
					"in that role at that tier in any year the view lists; no node there "+
					"would open on this step", v.Path, i, s.Role, s.From, p.name(), stems)
		}
	}
	return nil
}

// validateOneStepPerRole refuses step i sharing a parent and a From with an
// earlier step in the same role, or with one where either step has no role.
//
// One step per (After, From, Role), per SHARED parent rather than per
// equal After list, and a role-less step takes the whole tier.
func (v View) validateOneStepPerRole(i int, s DrillStep) error {
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
		where := parentChart{key: shared}.name()
		if shared != "" {
			where += "'s chart"
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
	return nil
}

// validateGapColumns refuses a gap licence for a column the view lists no
// year of, and two licences on one node for one column.
//
// Residual and gap SHAPE -- a grain exactly where a residual is, no
// empty id or reason, no licence of 0 cents -- is page.schema.json's,
// held at the write; this is what a schema cannot say.
func (v View) validateGapColumns(i int, s DrillStep, columns map[string]bool) error {
	for _, id := range slices.Sorted(maps.Keys(s.Gaps)) {
		seen := map[[2]string]bool{}
		for _, g := range s.Gaps[id] {
			if !columns[ColumnPath(g.FiscalYear, g.Basis)] {
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
	return nil
}

// stepStemsByYear is [StepStems] over every year the view lists, keyed by
// year stem: the view's own document where it lists none. Whether any node
// of a rung's document opens is the client's to answer, and site/*.test.mjs
// walks every rung.
func (v View) stepStemsByYear(ix ColumnIndex) (map[string][]string, error) {
	years := v.yearStems()
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
	if err := v.validateSankeyColumns(i, s); err != nil {
		return err
	}
	if err := v.validateSankeySide(i, s); err != nil {
		return err
	}
	if err := v.validateSankeyWindow(i, s); err != nil {
		return err
	}
	keptLeft, err := v.keptFlankEnd(i, s)
	if err != nil {
		return err
	}
	if err := v.validateSankeyParents(i, s, parents, doc, keptLeft); err != nil {
		return err
	}
	return v.validateSankeyCaps(i, s.Sankey)
}

// validateSankeyColumns refuses a cap on a kept tier, a step with no tiers,
// and a widening on a step that declares a gap.
func (v View) validateSankeyColumns(i int, s DrillStep) error {
	h := s.Sankey
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
	}
	return nil
}

// validateSankeySide refuses a side that is not one of the sides, a node
// opened on both sides that carries a gap or a residual or is not interior to
// the tiers drawn, and a side declared on a step that keeps a flank.
func (v View) validateSankeySide(i int, s DrillStep) error {
	h := s.Sankey
	switch {
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
	}
	return nil
}

// validateSankeyWindow refuses a widening on a step that keeps no flank, a
// tier list naming one tier twice, and a window whose column count is not its
// flank, its centre, what that opens into and its widenings.
func (v View) validateSankeyWindow(i int, s DrillStep) error {
	h := s.Sankey
	repeated := repeatedTier(h.Tiers)
	switch {
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
	return nil
}

// keptFlankEnd answers whether the kept flank is at the left end of Tiers,
// false for a step that keeps none. It refuses a Tiers neither of whose ends
// is the flank, a window whose centre is not the opened tier, and widened
// columns that are not the ones away from the flank in the order out from
// the centre.
//
// Which end the flank is at is read off Tiers once, here; validateKeptFlank
// reads that answer.
func (v View) keptFlankEnd(i int, s DrillStep) (bool, error) {
	h := s.Sankey
	keptLeft := false
	m := len(h.Keep)
	if m == 0 {
		return keptLeft, nil
	}
	n := len(h.Tiers)
	switch {
	case slices.Equal(h.Tiers[:m], reversedTiers(h.Keep)):
		keptLeft = true
	case slices.Equal(h.Tiers[n-m:], h.Keep):
	default:
		return false, fmt.Errorf(
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
		return false, fmt.Errorf(
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
			return false, fmt.Errorf(
				"view %q's step %d widens by tier(s) %v and draws tiers %v, whose %d "+
					"column(s) away from the kept flank are %v; a widened column is on the "+
					"opened node's side and the widening order is the order OUT from it, so "+
					"a client dropping the last of them draws a narrower window and not a "+
					"hole", v.Path, i, h.Widen, h.Tiers, w, drawn)
		}
	}
	return keptLeft, nil
}

// validateSankeyParents runs validateSankeyParent against every chart the
// step opens from, in After's order, and refuses what it refuses.
//
// Every parent places this step, not the first one that fits.
func (v View) validateSankeyParents(i int, s DrillStep, parents []parentChart, doc string, keptLeft bool) error {
	for _, p := range parents {
		if err := v.validateSankeyParent(i, s, p, doc, keptLeft); err != nil {
			return err
		}
	}
	return nil
}

// validateSankeyParent refuses a step that redraws the chart it opens from
// and one that opens a tier that chart keeps, and on a step that keeps a
// flank, what validateKeptFlank refuses.
func (v View) validateSankeyParent(i int, s DrillStep, p parentChart, doc string, keptLeft bool) error {
	h := s.Sankey
	where := p.name()
	// Same document only, and equality not containment (fisc-ke1f): a
	// strict subset of a widened parent's tiers is a narrower chart. The
	// view's own chart is a parent like any other here: a first hop
	// drawing the overview's own columns of its own document redraws it.
	if doc == p.doc && slices.Equal(p.tiers, h.Tiers) {
		return fmt.Errorf(
			"view %q's step %d draws tiers %v of %q, the set %s "+
				"already draws; opening a node would redraw the chart it was opened "+
				"from", v.Path, i, h.Tiers, doc, where)
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
		return nil
	}
	return v.validateKeptFlank(i, s, p, keptLeft)
}

// validateKeptFlank refuses a kept tier the parent does not draw, a flank
// whose first column is not beside the opened tier or is beside it on the
// other side, and a flank with a gap in it.
//
// Every parent here has a column order and already contains From,
// so fi >= 0. The flank walks out from the opened node.
func (v View) validateKeptFlank(i int, s DrillStep, p parentChart, keptLeft bool) error {
	h := s.Sankey
	where := p.name()
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
	return nil
}

// validateSankeyCaps refuses a cap below one, a cap on a tier the step does
// not draw, and two caps on one tier.
func (v View) validateSankeyCaps(i int, h *SankeyHints) error {
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

// yearStems is every document this view draws as a year: [View.YearStems],
// or the view's own document where it lists none.
func (v View) yearStems() []string {
	if len(v.YearStems) == 0 {
		return []string{v.Projection}
	}
	return v.YearStems
}

// tiersOf is every tier any of the documents carries a node at, ascending.
func tiersOf(stems []string, ix ColumnIndex) []int {
	var out []int
	for _, stem := range stems {
		for _, t := range ix.Tiers(stem) {
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	slices.Sort(out)
	return out
}

// undrawnTier is the first of tiers, the kept ones aside, that none of the
// documents carries a node at, or -1. A kept flank is the chart above's
// column, drawn of that chart's document: the object-category step keeps
// the spine's fund groups and draws department-spending, which has no
// node at that tier.
func undrawnTier(tiers, keep []int, stems []string, ix ColumnIndex) int {
	has := tiersOf(stems, ix)
	for _, t := range tiers {
		if !slices.Contains(keep, t) && !slices.Contains(has, t) {
			return t
		}
	}
	return -1
}

// carried answers whether any of the documents states a node with this id.
func carried(stems []string, id string, ix ColumnIndex) bool {
	return slices.ContainsFunc(stems, func(stem string) bool { return ix.CarriesNode(stem, id) })
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
