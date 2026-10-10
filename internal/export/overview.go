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
// height, canvas growth, cap, split cap or dropped tier it could not honour.
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
	stems, err := v.drawnStems(ix)
	if err != nil {
		return err
	}
	h := v.Overview.Sankey
	if v.Overview.Form != SankeyForm || h == nil {
		return nil
	}
	switch {
	case h.Height < 0:
		return fmt.Errorf("view %q lays its chart out on a canvas %dpx tall", v.Path, h.Height)
	case repeatedTier(h.Drop) >= 0:
		return fmt.Errorf("view %q drops tier %d twice", v.Path, repeatedTier(h.Drop))
	}
	if err := v.validateGrowth(); err != nil {
		return err
	}
	if err := v.validateSplitCaps(); err != nil {
		return err
	}
	for j, c := range h.Caps {
		switch {
		case c.Cap < 1:
			return fmt.Errorf("view %q's own chart caps tier %d at %d; a column of no node is not a chart",
				v.Path, c.Tier, c.Cap)
		case !slices.Contains(h.Tiers, c.Tier):
			return fmt.Errorf(
				"view %q's own chart caps tier %d and draws tiers %v, which do not include it; "+
					"the cap would fold nothing, in silence", v.Path, c.Tier, h.Tiers)
		case slices.ContainsFunc(h.Caps[:j], func(o TierCap) bool { return o.Tier == c.Tier }):
			return fmt.Errorf("view %q's own chart caps tier %d twice", v.Path, c.Tier)
		case c.Tail == "":
			return fmt.Errorf(
				"view %q's own chart caps tier %d with no tail; the overview has no step to "+
					"name what its folded rows are", v.Path, c.Tier)
		}
	}
	for _, t := range h.Drop {
		if slices.Contains(h.Tiers, t) {
			return fmt.Errorf(
				"view %q both draws and drops tier %d; one column cannot be drawn and not drawn",
				v.Path, t)
		}
	}
	if t := undrawnTier(h.Drop, stems, ix); t >= 0 {
		return fmt.Errorf(
			"view %q drops tier %d, at which %v carry no node; a drop of nothing "+
				"declares a restatement the documents do not make", v.Path, t, stems)
	}
	return validateDrop(v.Path, h.Drop, stems, built)
}

// validateDrop refuses a dropped tier any of whose links cites a fact no link
// the chart keeps cites: such a tier is not a restatement, and dropping it
// would take a printed figure off the chart and out of the flow table.
func validateDrop(view string, drop []int, stems []string, built map[string][]byte) error {
	if len(drop) == 0 {
		return nil
	}
	for _, stem := range stems {
		var d decoded
		if err := json.Unmarshal(built[stem], &d); err != nil {
			return fmt.Errorf("view %q: decode %s: %w", view, stem, err)
		}
		dropped := map[string]bool{}
		for _, n := range d.Nodes {
			if slices.Contains(drop, n.Tier) {
				dropped[n.ID] = true
			}
		}
		kept := map[string]bool{}
		for _, l := range d.Links {
			if !dropped[l.Source] && !dropped[l.Target] {
				for _, id := range l.FactIDs {
					kept[id] = true
				}
			}
		}
		for _, l := range d.Links {
			if !dropped[l.Source] && !dropped[l.Target] {
				continue
			}
			for _, id := range l.FactIDs {
				if !kept[id] {
					return fmt.Errorf(
						"view %q drops tiers %v, and %s's link %s -> %s cites fact %s, which no "+
							"link the chart keeps cites; dropping it would take a printed figure "+
							"off the chart and out of the flow table", view, drop, stem, l.Source, l.Target, id)
				}
			}
		}
	}
	return nil
}

// validateGrowth refuses a canvas growth the client could not lay out by: a
// size of no px, a ceiling below its floor, and a growth beside a fixed
// height, which would leave two canvases to choose between.
func (v View) validateGrowth() error {
	h := v.Overview.Sankey
	g := h.Grow
	switch {
	case g == nil:
		return nil
	case h.Height != 0:
		return fmt.Errorf("view %q declares both a %dpx canvas and a canvas that grows; "+
			"the chart is laid out on one", v.Path, h.Height)
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
