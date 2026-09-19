package structure

import (
	"fmt"
	"slices"
)

// PartitionByYear is d as one [Document] per fiscal year, ascending: each
// part carries the whole's scaffolding -- SchemaVersion, Levels, Cuts and
// Identities -- verbatim, the whole's facts of that year in the whole's
// order, and each view's facts of that year re-indexed into the part's own
// list.
//
// THIS IS NOT THE Document.Slice THAT WENT WITH fisc-kbuo's CANDIDATE (b).
// That sliced per interaction -- one file per view, per drill, per column
// budget -- and lost because a client fetching on every click is a client
// that cannot answer offline what Go already answered. A year is a static
// partition along the one dimension the reader already switches on with
// the year control, so a part is the whole of what one column of the site
// reads, decided once at export and never re-cut by the client.
//
// A VIEW WITH NO FACT IN A YEAR IS OMITTED FROM THAT PART, never shipped
// with an empty list: the discipline [Build] keeps over the whole, kept per
// part for the same reason, that an absent view and an empty one cannot
// mean one thing. A part left with no view at all is refused rather than
// shipped as scaffolding alone, and a fact whose FiscalYear is zero is
// refused by index and id -- absent is not zero, and a year that cannot be
// named is a part that cannot be addressed. An empty d.Facts is refused
// for the same reason there is nothing to partition.
func PartitionByYear(d Document) ([]Document, error) {
	if len(d.Facts) == 0 {
		return nil, fmt.Errorf("structure: the document carries no fact, so there is no year to partition it by")
	}
	var years []int
	for i, f := range d.Facts {
		if f.FiscalYear == 0 {
			return nil, fmt.Errorf("structure: fact %d (%s) carries no fiscal year, so no part could carry it", i, f.ID)
		}
		if !slices.Contains(years, f.FiscalYear) {
			years = append(years, f.FiscalYear)
		}
	}
	slices.Sort(years)
	parts := make([]Document, 0, len(years))
	for _, year := range years {
		part := Document{
			SchemaVersion: d.SchemaVersion,
			FiscalYear:    year,
			Levels:        slices.Clone(d.Levels),
			Cuts:          slices.Clone(d.Cuts),
			Identities:    slices.Clone(d.Identities),
		}
		// THE INDEX IS REBUILT PER PART: a view's indices are into the whole's
		// list, and a part's list is shorter, so an index copied across would
		// name a fact of the wrong year or nothing at all.
		index := make(map[int]int)
		for i, f := range d.Facts {
			if f.FiscalYear != year {
				continue
			}
			index[i] = len(part.Facts)
			part.Facts = append(part.Facts, f)
		}
		for _, v := range d.Views {
			decl := ViewDecl{
				Name:     v.Name,
				Scopes:   slices.Clone(v.Scopes),
				Cuts:     slices.Clone(v.Cuts),
				Readings: v.Readings,
			}
			for _, at := range v.Facts {
				if at < 0 || at >= len(d.Facts) {
					return nil, fmt.Errorf("structure: view %q names fact %d, outside 0..%d", v.Name, at, len(d.Facts)-1)
				}
				if re, ok := index[at]; ok {
					decl.Facts = append(decl.Facts, re)
				}
			}
			if len(decl.Facts) == 0 {
				continue
			}
			part.Views = append(part.Views, decl)
		}
		if len(part.Views) == 0 {
			return nil, fmt.Errorf("structure: no view admits a fact of %d, so its part would be scaffolding alone", year)
		}
		parts = append(parts, part)
	}
	return parts, nil
}
