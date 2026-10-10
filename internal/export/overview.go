package export

import (
	"encoding/json"
	"fmt"
	"slices"
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
// draw: a schedule of its own on a view that drills, or on one that draws no
// chart, and a canvas height, cap or dropped tier it could not honour.
func (v View) validateOverview(built map[string][]byte, ix ColumnIndex) error {
	switch {
	case v.Draws != "" && v.Overview.Form == "":
		return fmt.Errorf(
			"view %q draws schedule %q and declares no chart to draw it in", v.Path, v.Draws)
	case v.Draws != "" && len(v.Steps) > 0:
		return fmt.Errorf(
			"view %q draws schedule %q and declares %d step(s); a step opening from the "+
				"view's own chart is placed against %q's document, which this chart does not draw",
			v.Path, v.Draws, len(v.Steps), v.Projection)
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
	if t := undrawnTier(h.Drop, nil, stems, ix); t >= 0 {
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
