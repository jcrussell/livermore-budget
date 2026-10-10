package export

import (
	"fmt"
	"slices"
)

// validateOverviewTiersCarried refuses a view's own chart drawing a tier no
// document of any year the view lists carries a node at, and then one that
// some year's document does not.
func (v View) validateOverviewTiersCarried(yearStems []string, ix ColumnIndex) error {
	if t := undrawnTier(v.Overview.drawnTiers(), yearStems, ix); t >= 0 {
		return fmt.Errorf(
			"view %q's chart draws tier %d, and %v carries nodes at tiers %v and none "+
				"at that one in any year the view lists; the column would be empty on "+
				"every year's chart", v.Path, t, yearStems, tiersOf(yearStems, ix))
	}
	for _, year := range yearStems {
		if t := undrawnTier(v.Overview.drawnTiers(), []string{year}, ix); t >= 0 {
			return fmt.Errorf(
				"view %q's chart draws tier %d, and %q, its document in that year, carries "+
					"nodes at tiers %v and none at that one; that year's chart would be laid "+
					"out around an empty column", v.Path, t, year, ix.Tiers(year))
		}
	}
	return nil
}

// projectionAmongYears refuses a view whose projection is not among its year
// stems: the page's non-per-year metadata would come from a document the
// reader can never select.
func (v View) projectionAmongYears() error {
	if !slices.Contains(v.yearStems(), v.Projection) {
		return fmt.Errorf("view %q renders projection %q, which is not among its year stems %v",
			v.Path, v.Projection, v.YearStems)
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

// undrawnTier is the first of tiers that none of the documents carries a node
// at, or -1.
func undrawnTier(tiers []int, stems []string, ix ColumnIndex) int {
	has := tiersOf(stems, ix)
	for _, t := range tiers {
		if !slices.Contains(has, t) {
			return t
		}
	}
	return -1
}
