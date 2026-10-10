package export

import (
	"fmt"
	"slices"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// drawnStems is the document the view's own chart draws in each year the
// view lists, in [View.yearStems] order: the year itself, or the
// [View.Draws] schedule of that year's column. It refuses a year whose
// column carries no such schedule.
func (v View) drawnStems(ix ColumnIndex) ([]string, error) {
	years := v.yearStems()
	if v.Draws == "" {
		return years, nil
	}
	out := make([]string, 0, len(years))
	for _, year := range years {
		col, folded := ix.Column(year)
		stem, ok := ix.Stem(col, v.Draws)
		if !folded || !ok {
			return nil, fmt.Errorf(
				"view %q draws schedule %q, and %q, a year it lists, is of a column that "+
					"carries none; that year's chart would have nothing to draw",
				v.Path, v.Draws, year)
		}
		out = append(out, stem)
	}
	return out, nil
}

// validateOverview refuses what the view's own chart declares and could not
// draw: a schedule of its own on a view that draws no chart, and a canvas
// growth or split cap it could not honour.
func (v View) validateOverview(built map[string][]byte, ix ColumnIndex) error {
	switch {
	case v.Draws != "" && v.Overview.Form == "":
		return fmt.Errorf(
			"view %q draws schedule %q and declares no chart to draw it in", v.Path, v.Draws)
	case v.Description != "" && !templateDrawsAChart(v.Template):
		return fmt.Errorf(
			"view %q describes its chart and renders template %q, which draws none; "+
				"the sentence would be dropped in silence", v.Path, v.Template)
	}
	if _, err := v.drawnStems(ix); err != nil {
		return err
	}
	h := v.Overview.Sankey
	if v.Overview.Form != SankeyForm || h == nil {
		return nil
	}
	if err := v.validateGrowth(); err != nil {
		return err
	}
	return v.validateSplitCaps()
}

// validateGrowth refuses a canvas growth the client could not lay out by: a
// size of no px and a ceiling below its floor.
func (v View) validateGrowth() error {
	h := v.Overview.Sankey
	g := h.Grow
	switch {
	case g == nil:
		return nil
	case g.NodePx < 1 || g.Floor < 1:
		return fmt.Errorf("view %q grows its canvas %dpx a node from a %dpx floor; "+
			"a canvas of no px draws nothing", v.Path, g.NodePx, g.Floor)
	case g.Ceiling < g.Floor:
		return fmt.Errorf("view %q grows its canvas to a %dpx ceiling below its %dpx floor",
			v.Path, g.Ceiling, g.Floor)
	}
	return nil
}

// validateSplitCaps refuses a split cap that could not be honoured: a cap of
// no member, one naming no tail, a split key twice, and a key the stitch
// declares no split under, which would fold nothing, in silence.
func (v View) validateSplitCaps() error {
	caps := v.Overview.Sankey.SplitCaps
	keys := project.SplitKeys()
	for j, c := range caps {
		switch {
		case c.Cap < 1:
			return fmt.Errorf("view %q caps split %q at %d; a split drawn as no member is not a chart",
				v.Path, c.Split, c.Cap)
		case c.Tail == "":
			return fmt.Errorf("view %q caps split %q with no tail; nothing names what its folded members are",
				v.Path, c.Split)
		case !slices.Contains(keys, c.Split):
			return fmt.Errorf("view %q caps split %q, and the expansion schedule declares splits %v; "+
				"the cap would fold nothing, in silence", v.Path, c.Split, keys)
		case slices.ContainsFunc(caps[:j], func(o SplitCap) bool { return o.Split == c.Split }):
			return fmt.Errorf("view %q caps split %q twice", v.Path, c.Split)
		}
	}
	return nil
}
