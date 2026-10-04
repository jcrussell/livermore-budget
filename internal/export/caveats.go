package export

import (
	"encoding/json"
	"fmt"
)

// documentCaveats is as much of ANY document as the caveats page needs.
// Shape-blind, for documentSources' reason.
type documentCaveats struct {
	Metadata struct {
		FiscalYearLabel string       `json:"fiscal_year_label"`
		Basis           string       `json:"basis"`
		Caveats         []caveatMeta `json:"caveats"`
	} `json:"metadata"`
}

// caveatMeta is a decoded caveat.
//
// A SEPARATE TYPE FROM project.Caveat, like every other decode struct in this
// file, because this package consumes projections as bytes and does not import
// internal/project. The field set is the contract, and it is pinned by
// TestEveryNameThisPackageDecodesIsOneAProjectionStates rather than by the two
// declarations happening to agree.
type caveatMeta struct {
	ID        string   `json:"id"`
	Summary   string   `json:"summary"`
	Text      string   `json:"text"`
	AppliesTo []string `json:"applies_to"`
}

// caveatRef is one caveat as a PAGE shows it: a line, and somewhere to go for
// the rest; the text is deliberately absent. Href is empty when the site has
// no caveats page, and the template then renders plain text.
type caveatRef struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Href    string `json:"href"`
}

// caveatRefs composes the page-facing form. base is the caveats view's path, or
// "" when the site has no such page.
//
// The anchor is per (caveat, document), not per caveat: one id can carry
// different text in different documents, so stem is required.
func caveatRefs(metas []caveatMeta, stem, base string) []caveatRef {
	out := make([]caveatRef, 0, len(metas))
	for _, m := range metas {
		ref := caveatRef{ID: m.ID, Summary: m.Summary}
		if base != "" {
			ref.Href = base + "#" + caveatAnchor(stem, m.ID)
		}
		out = append(out, ref)
	}
	return out
}

// caveatAnchor is the one spelling of the fragment, so the page that emits the
// id and the pages that link to it cannot disagree about its form.
func caveatAnchor(stem, id string) string { return "caveat-" + stem + "--" + id }

// caveatsPathOf is the caveats view's path, or "" when the site has none.
//
// Derived from the view set, so it cannot drift out of step with it.
func caveatsPathOf(o *Options) string {
	for _, v := range o.views() {
		if v.Template == CaveatsTemplate {
			return v.Path
		}
	}
	return ""
}

// caveatsPageData is every published document's caveats, in full.
//
// GROUPED BY DOCUMENT AND NOT BY CAVEAT, which is forced rather than chosen.
// One id can carry different text in different documents -- transfer-legs-
// unpaired has three sentences and the FY2027 spine carries a contested-total
// entry FY2026 does not -- so a page keyed on id alone would have to pick one
// text and would be wrong about the others.
type caveatsPageData struct {
	chrome
	Documents []caveatDocument
	// Count is every caveat on the page, across documents, so the lede can say
	// how many without the template summing a nested range.
	Count int
}

// caveatDocument is one published document's section of the caveats page.
type caveatDocument struct {
	Stem     string
	Label    string
	DataPath string
	Entries  []caveatEntry
	// Drawn is whether a view that DRAWS A CHART renders this document, so the
	// page can promise a chart flag only where there is a chart.
	//
	// NOT "any view renders it", which is what this said and what
	// templateDrawsAChart exists to correct: trends.html renders revenue-trends
	// and ships no app.js, so a caveat on that document can be listed and never
	// chipped on a mark.
	//
	// False for every document no view renders (the fund-flows columns
	// unviewedDocuments declares) and for every one rendered as a TABLE —
	// revenue-trends and the two ACFR history documents. This page lists all
	// of their caveats anyway, because a caveat is owed to whoever fetches the
	// file. What it must not do is tell that reader the charts flag these
	// marks.
	Drawn bool
}

// caveatEntry is one caveat, rendered whole. Anchor is what every other page's
// summary links to.
type caveatEntry struct {
	ID        string
	Anchor    string
	Summary   string
	Text      string
	AppliesTo []string
}

// buildCaveatsPage lists every published document's caveats, in full, with an
// anchor per (document, caveat) that every other page's summary links to.
//
// It names no projection: it is an index across documents. It refuses an
// empty page, a caveat with no id, summary or text, and two entries claiming
// one anchor -- a repeated id in one document, or the "--" separator making
// two compositions ambiguous. project.ValidateCaveats never sees a document
// decoded from bytes, which is every document here.
func buildCaveatsPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (caveatsPageData, error) {
	caveatsPath := caveatsPathOf(o)
	// Which documents a chart actually draws: through a view's projection,
	// its year stems, or its steps' documents.
	drawn := map[string]bool{}
	for _, v := range o.views() {
		// A view that draws no chart flags nothing, whatever it renders.
		if !templateDrawsAChart(v.Template) {
			continue
		}
		if v.Projection != "" {
			drawn[v.Projection] = true
		}
		for _, stem := range v.YearStems {
			drawn[stem] = true
		}
		// A document a chart opens into is drawn too, resolved per year.
		for _, stem := range v.DrawnStems(ix) {
			drawn[stem] = true
		}
	}
	anchors := map[string]string{}
	docs := make([]caveatDocument, 0, len(o.Projections))
	count := 0
	for _, stem := range sortedKeys(o.Projections) {
		var dc documentCaveats
		if err := json.Unmarshal(o.Projections[stem], &dc); err != nil {
			return caveatsPageData{}, fmt.Errorf("decode %s caveats: %w", stem, err)
		}
		if len(dc.Metadata.Caveats) == 0 {
			continue
		}
		entries := make([]caveatEntry, 0, len(dc.Metadata.Caveats))
		for _, c := range dc.Metadata.Caveats {
			switch {
			case c.ID == "":
				return caveatsPageData{}, fmt.Errorf(
					"%s carries a caveat with no id, and the id is the anchor every other page links to", stem)
			case c.Summary == "":
				return caveatsPageData{}, fmt.Errorf(
					"%s caveat %q has no summary, and the summary is what the other pages show in its place", stem, c.ID)
			case c.Text == "":
				return caveatsPageData{}, fmt.Errorf(
					"%s caveat %q has no text, so this page would publish a heading over nothing", stem, c.ID)
			}
			a := caveatAnchor(stem, c.ID)
			if prev, dup := anchors[a]; dup {
				return caveatsPageData{}, fmt.Errorf(
					"caveat anchor %q is claimed by both %s and %s; an anchor is a published "+
						"URL fragment and a link to a repeated one lands on whichever the browser finds first",
					a, prev, stem)
			}
			anchors[a] = stem
			entries = append(entries, caveatEntry{
				ID: c.ID, Anchor: a, Summary: c.Summary, Text: c.Text, AppliesTo: c.AppliesTo,
			})
		}
		count += len(entries)
		label := dc.Metadata.FiscalYearLabel
		if label != "" && dc.Metadata.Basis != "" {
			label += " " + dc.Metadata.Basis
		}
		docs = append(docs, caveatDocument{
			Stem:     stem,
			Label:    label,
			DataPath: ix.PublishedPath(stem),
			Entries:  entries,
			Drawn:    drawn[stem],
		})
	}
	if count == 0 {
		return caveatsPageData{}, fmt.Errorf(
			"view %q renders the caveats index and no published document carries a caveat, "+
				"so the site would ship a nav entry to a page with nothing on it", v.Path)
	}

	title := v.Title
	if title == "" {
		title = "What this site's figures do not say"
	}
	return caveatsPageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
			CaveatsPath: caveatsPath,
			// NO Sources, AND THAT IS THE POINT rather than an omission.
			// TestEachViewsFooterCitesItsOwnSources enforces that a page must
			// not advertise pages it never showed a figure from, and this page
			// shows no figures at all -- it publishes sentences about
			// documents, each of which links to the document itself. Naming
			// the union of every document's pages here would be this site's
			// broadest false provenance claim.
			//
			// NO DataPath either, for buildProvenancePage's reason: this page
			// draws no projection.
		},
		Documents: docs,
		Count:     count,
	}, nil
}
