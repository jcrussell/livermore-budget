package export

import (
	"encoding/json"
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
	case v.Description != "" && templates[v.Template].form == "":
		return fmt.Errorf(
			"view %q describes its chart and renders template %q, which draws none; "+
				"the sentence would be dropped in silence", v.Path, v.Template)
	}
	drawn, err := v.drawnStems(ix)
	if err != nil {
		return err
	}
	if v.Overview.sides() != nil {
		return v.validateSides(drawn, built)
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

// validateSides refuses a sides chart the client could not draw: a side with
// no heading, a tier declared twice, and, in any year's drawn document, a
// tier with no root or a root whose outside ribbons both arrive and leave.
func (v View) validateSides(drawn []string, built map[string][]byte) error {
	h := v.Overview.sides()
	for j, s := range h.Sides {
		switch {
		case s.Heading == "":
			return fmt.Errorf("view %q draws tier %d as a side with no heading", v.Path, s.Tier)
		case s.Tier < 0:
			return fmt.Errorf("view %q draws tier %d as a side; a tier is not negative", v.Path, s.Tier)
		case slices.ContainsFunc(h.Sides[:j], func(o Side) bool { return o.Tier == s.Tier }):
			return fmt.Errorf("view %q draws tier %d as a side twice", v.Path, s.Tier)
		}
	}
	for _, stem := range drawn {
		var d decoded
		if err := json.Unmarshal(built[stem], &d); err != nil {
			return fmt.Errorf("decode %s for view %q's sides: %w", stem, v.Path, err)
		}
		if len(d.Splits) == 0 {
			return fmt.Errorf("view %q draws %s as sides, and it declares no splits, so nothing would open", v.Path, stem)
		}
		parent := make(map[string]string, len(d.Nodes))
		for _, n := range d.Nodes {
			parent[n.ID] = n.Parent
		}
		inside := func(root, id string) bool {
			for hops := 0; id != "" && hops < 12; hops++ {
				if id == root {
					return true
				}
				id = parent[id]
			}
			return false
		}
		for _, s := range h.Sides {
			roots := 0
			for _, n := range d.Nodes {
				if n.Parent != "" || n.Tier != s.Tier {
					continue
				}
				roots++
				var leaving, arriving bool
				for _, l := range d.Links {
					if l.Source == n.ID && !inside(n.ID, l.Target) {
						leaving = true
					}
					if l.Target == n.ID && !inside(n.ID, l.Source) {
						arriving = true
					}
				}
				if leaving && arriving {
					return fmt.Errorf("view %q draws tier %d as a side, and in %s its root %s both takes in and pays out, "+
						"so no one figure is its size", v.Path, s.Tier, stem, n.ID)
				}
			}
			if roots == 0 {
				return fmt.Errorf("view %q draws tier %d as a side, and %s has no root at it; every node there hangs under another",
					v.Path, s.Tier, stem)
			}
		}
	}
	return nil
}
