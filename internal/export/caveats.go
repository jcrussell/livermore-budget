package export

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
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

// caveatsHere is the one line a page states its caveats in: how many, and
// the caveats page's section for this page. With no caveats page Href is ""
// and the "caveats-line" template lists Entries' headings instead, each
// summary with its Of, so a site without the page still states them and
// tells apart two that share a summary.
//
// Count is len(Entries), and Entries is the caveats page's own section for
// this view (caveatIndex.views), so the line and the section cannot disagree.
type caveatsHere struct {
	Count   int
	Href    string
	Entries []*caveatEntry
}

// caveatKey is one caveat of one document. An id is unique within a
// document (caveatIndex refuses a repeat), so the pair names one caveat.
type caveatKey struct{ stem, id string }

// caveatIndex is every distinct caveat the published documents carry, each
// once, and every view's share of them.
//
// DISTINCT IS (id, summary, text), NOT id: one id carries different text in
// different columns (transfer-legs-unpaired's sentence turns on whether that
// column's legs balance), and one text under different summaries
// (some-fund-group-money-is-not-by-fund counts the groups in its summary).
// Merging either would publish one column's sentence as another's.
type caveatIndex struct {
	// base is the caveats view's path, or "" when the site has none.
	base    string
	entries []*caveatEntry
	byRef   map[caveatKey]*caveatEntry
	// views is every view's section, in nav order, empty ones included.
	views []*caveatView
	// unviewed are the entries no view states: every document carrying one
	// is a document no view reaches.
	unviewed []*caveatEntry
}

// caveatEntry is one distinct caveat, rendered whole. Anchor is what every
// other page's link lands on.
type caveatEntry struct {
	ID      string
	Anchor  string
	Summary string
	// Of tells apart entries sharing one summary: the fiscal years of the
	// documents carrying this one (its stem for one stating no year), or,
	// when two entries under the summary would name the same years, every
	// entry's carrying stems. "" when no other entry shares the summary.
	Of   string
	Text string
	// About groups the documents that ship this caveat by what it is about
	// in each of them, in the order the documents sort.
	About []caveatAbout
	// Pages are the views that state it, in nav order.
	Pages []navItem
}

// caveatAbout is the documents that ship one caveat about one set of nodes.
type caveatAbout struct {
	AppliesTo []string
	Files     []caveatFile
	// Flagged is whether a chart flags these marks: every file of the group
	// is a schedule a Sankey page draws, whose caveats site/app.js chips on
	// the marks they name. A document only read beside the chart (the spine
	// a Flows page opens on) is not flagged, and neither is any ledger or
	// table.
	Flagged bool
}

// caveatFile is one published document carrying a caveat.
type caveatFile struct {
	Stem     string
	Label    string
	DataPath string
}

// caveatView is one view's section of the caveats page.
type caveatView struct {
	Label   string
	Path    string
	Anchor  string
	Entries []*caveatEntry
}

// caveatsPathOf is the caveats view's path, or "" when the site has none.
//
// Derived from the view set, so it cannot drift out of step with it.
func caveatsPathOf(o *Options) string { return templatePathOf(o, CaveatsTemplate) }

// caveatSectionPrefix and caveatEntryPrefix are the two fragment spellings
// the caveats page emits, so a view's section and a caveat's entry cannot
// claim one id.
const (
	caveatSectionPrefix = "caveats-on-"
	caveatEntryPrefix   = "caveat-"
)

// slugUnsafe is everything a view path holds that a fragment slug does not.
var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// viewSlug is a view's path as a fragment: "index.html" is "index",
// "a/b.html" is "a-b".
func viewSlug(path string) string {
	s := strings.ToLower(strings.TrimSuffix(path, ".html"))
	return strings.Trim(slugUnsafe.ReplaceAllString(s, "-"), "-")
}

// caveatsOfView is the documents whose caveats a view states:
// [View.ReachedStems] for a view that renders a document, none otherwise.
// The one place this is decided, so the caveats page's section and the
// view's own line count the same documents.
func caveatsOfView(v View, ix ColumnIndex) ([]string, error) {
	if !templateRendersADocument(v.Template) {
		return nil, nil
	}
	return v.ReachedStems(ix)
}

// flaggedStems are the documents whose caveats a chart chips on its marks:
// the schedule each year of a Sankey view draws (site/app.js's caveatsFor
// reads the drawn schedule's caveats and no other's), as [View.drawnStems]
// names it and refusing where it refuses.
func flaggedStems(o *Options, ix ColumnIndex) (map[string]bool, error) {
	out := map[string]bool{}
	for _, v := range o.Views {
		if v.Template != SankeyTemplate {
			continue
		}
		drawn, err := v.drawnStems(ix)
		if err != nil {
			return nil, err
		}
		for _, stem := range drawn {
			out[stem] = true
		}
	}
	return out, nil
}

// buildCaveatIndex reads every published document's caveats once.
//
// It refuses a caveat with no id, summary or text, a document carrying one id
// twice, two fragments the caveats page would emit alike, and two entries
// whose headings (Summary and Of) would read alike. project.ValidateCaveats
// never sees a document decoded from bytes, which is every document here.
func buildCaveatIndex(o *Options, ix ColumnIndex, nav []navItem) (*caveatIndex, error) {
	cx := &caveatIndex{base: caveatsPathOf(o), byRef: map[caveatKey]*caveatEntry{}}
	flagged, err := flaggedStems(o, ix)
	if err != nil {
		return nil, err
	}

	type variant struct {
		id, summary, text string
	}
	byVariant := map[variant]*caveatEntry{}
	// firstStem is the first document carrying each entry, for its anchor.
	firstStem := map[*caveatEntry]string{}
	variants := map[string][]*caveatEntry{}
	appliesOf := map[*caveatEntry]map[string][]string{}
	carriers := map[*caveatEntry][]string{}
	// carried is each document's entries, in the order it states them.
	carried := map[string][]*caveatEntry{}
	for _, stem := range sortedKeys(o.Projections) {
		var dc documentCaveats
		if err := json.Unmarshal(o.Projections[stem], &dc); err != nil {
			return nil, fmt.Errorf("decode %s caveats: %w", stem, err)
		}
		for _, c := range dc.Metadata.Caveats {
			switch {
			case c.ID == "":
				return nil, fmt.Errorf(
					"%s carries a caveat with no id, and the id is the anchor every other page links to", stem)
			case c.Summary == "":
				return nil, fmt.Errorf(
					"%s caveat %q has no summary, and the summary is what the other pages show in its place", stem, c.ID)
			case c.Text == "":
				return nil, fmt.Errorf(
					"%s caveat %q has no text, so the caveats page would publish a heading over nothing", stem, c.ID)
			}
			key := caveatKey{stem, c.ID}
			if _, dup := cx.byRef[key]; dup {
				return nil, fmt.Errorf(
					"%s carries caveat %q twice, so a link to it could land on either", stem, c.ID)
			}
			vk := variant{c.ID, c.Summary, c.Text}
			e, seen := byVariant[vk]
			if !seen {
				e = &caveatEntry{ID: c.ID, Summary: c.Summary, Text: c.Text}
				byVariant[vk] = e
				firstStem[e] = stem
				variants[c.ID] = append(variants[c.ID], e)
				appliesOf[e] = map[string][]string{}
				cx.entries = append(cx.entries, e)
			}
			cx.byRef[key] = e
			carried[stem] = append(carried[stem], e)
			appliesOf[e][stem] = c.AppliesTo
			carriers[e] = append(carriers[e], stem)
		}
	}

	// Anchors, and every fragment the page emits, refused when two collide.
	fragments := map[string]string{}
	claim := func(fragment, by string) error {
		if prev, dup := fragments[fragment]; dup {
			return fmt.Errorf(
				"the caveats page fragment %q is claimed by both %s and %s; a fragment is a published "+
					"URL and a link to a repeated one lands on whichever the browser finds first",
				fragment, prev, by)
		}
		fragments[fragment] = by
		return nil
	}
	for _, e := range cx.entries {
		e.Anchor = caveatEntryPrefix + e.ID
		if len(variants[e.ID]) > 1 {
			e.Anchor += "--" + firstStem[e]
		}
		if err := claim(e.Anchor, fmt.Sprintf("caveat %q of %s", e.ID, firstStem[e])); err != nil {
			return nil, err
		}
		for _, stem := range carriers[e] {
			f := caveatFile{Stem: stem, Label: documentLabel(o, stem), DataPath: ix.PublishedPath(stem)}
			applies := appliesOf[e][stem]
			// Flagged only splits a group that names nodes: the page says
			// nothing of it otherwise, and two groups would read alike.
			flags := len(applies) > 0 && flagged[stem]
			at := slices.IndexFunc(e.About, func(a caveatAbout) bool {
				return slices.Equal(a.AppliesTo, applies) && a.Flagged == flags
			})
			if at < 0 {
				e.About = append(e.About, caveatAbout{AppliesTo: applies, Flagged: flags})
				at = len(e.About) - 1
			}
			e.About[at].Files = append(e.About[at].Files, f)
		}
	}

	// One summary over several texts (a count in the text differs by column)
	// would read as one link listed twice.
	bySummary := map[string][]*caveatEntry{}
	for _, e := range cx.entries {
		bySummary[e.Summary] = append(bySummary[e.Summary], e)
	}
	for _, summary := range sortedKeys(bySummary) {
		alike := bySummary[summary]
		if len(alike) < 2 {
			continue
		}
		// The years, unless two entries share them; then the stems, which
		// no two entries share.
		ofs := map[string]bool{}
		for _, e := range alike {
			var of []string
			for _, stem := range carriers[e] {
				label := documentLabel(o, stem)
				if label == "" {
					label = stem
				}
				if !slices.Contains(of, label) {
					of = append(of, label)
				}
			}
			e.Of = strings.Join(of, ", ")
			ofs[e.Of] = true
		}
		if len(ofs) < len(alike) {
			for _, e := range alike {
				e.Of = strings.Join(carriers[e], ", ")
			}
		}
		// Two entries under one summary carried by the same documents have
		// one Of either way, and would head two texts alike.
		headings := map[string]*caveatEntry{}
		for _, e := range alike {
			if prev, dup := headings[e.Of]; dup {
				return nil, fmt.Errorf(
					"caveats %q and %q of %s share the summary %q, so the caveats page would head "+
						"two different caveats alike and a reader could not tell which a link means",
					prev.ID, e.ID, e.Of, e.Summary)
			}
			headings[e.Of] = e
		}
	}

	for i, v := range o.Views {
		stems, err := caveatsOfView(v, ix)
		if err != nil {
			return nil, err
		}
		cv := &caveatView{Label: nav[i].Label, Path: v.Path, Anchor: caveatSectionPrefix + viewSlug(v.Path)}
		for _, stem := range stems {
			// An unbuilt document is the view's builder's to refuse, in
			// its own words; nothing of it can be stated here.
			if _, built := o.Projections[stem]; !built {
				continue
			}
			for _, e := range carried[stem] {
				if !slices.Contains(cv.Entries, e) {
					cv.Entries = append(cv.Entries, e)
					e.Pages = append(e.Pages, navItem{Label: cv.Label, Path: v.Path})
				}
			}
		}
		if len(cv.Entries) > 0 {
			if err := claim(cv.Anchor, fmt.Sprintf("view %q", v.Path)); err != nil {
				return nil, err
			}
		}
		cx.views = append(cx.views, cv)
	}
	for _, e := range cx.entries {
		if len(e.Pages) == 0 {
			cx.unviewed = append(cx.unviewed, e)
		}
	}
	return cx, nil
}

// documentLabel is a document's fiscal year and basis as the caveats page
// names it, or "" for a document stating neither.
func documentLabel(o *Options, stem string) string {
	var dc documentCaveats
	if json.Unmarshal(o.Projections[stem], &dc) != nil {
		return ""
	}
	label := dc.Metadata.FiscalYearLabel
	if label != "" && dc.Metadata.Basis != "" {
		label += " " + dc.Metadata.Basis
	}
	return label
}

// href is where a page links one entry, or "" with no caveats page.
func (cx *caveatIndex) href(e *caveatEntry) string {
	if cx.base == "" {
		return ""
	}
	return cx.base + "#" + e.Anchor
}

// refs is one document's caveats as a page links them, each to its deduped
// entry. A caveat the index does not carry is refused rather than linked
// nowhere.
func (cx *caveatIndex) refs(metas []caveatMeta, stem string) ([]caveatRef, error) {
	out := make([]caveatRef, 0, len(metas))
	for _, m := range metas {
		e, ok := cx.byRef[caveatKey{stem, m.ID}]
		if !ok {
			return nil, fmt.Errorf("%s caveat %q is in no published document the caveats index read", stem, m.ID)
		}
		out = append(out, caveatRef{ID: e.ID, Summary: e.Summary, Href: cx.href(e)})
	}
	return out, nil
}

// here is one view's line: its section's entries, counted, and the section.
func (cx *caveatIndex) here(path string) caveatsHere {
	for _, cv := range cx.views {
		if cv.Path != path {
			continue
		}
		out := caveatsHere{Count: len(cv.Entries), Entries: cv.Entries}
		if cx.base != "" && len(cv.Entries) > 0 {
			out.Href = cx.base + "#" + cv.Anchor
		}
		return out
	}
	return caveatsHere{}
}

// caveatsPageData is every distinct caveat, once, and each view's share.
type caveatsPageData struct {
	chrome
	// Views are the sections of the views that state a caveat, in nav order.
	Views []*caveatView
	// Unviewed are the caveats no page states, carried only by documents no
	// page draws and owed to whoever fetches the file.
	Unviewed []*caveatEntry
	Entries  []*caveatEntry
	// Documents is how many published documents carry a caveat.
	Documents int
}

// buildCaveatsPage lists every distinct caveat once, under an anchor every
// other page's link lands on, and a section per view listing that view's.
// It names no projection: it is an index across documents. It refuses an
// empty page; buildCaveatIndex refuses the rest.
func buildCaveatsPage(o *Options, v View, nav []navItem, cx *caveatIndex, ix ColumnIndex) (caveatsPageData, error) {
	if len(cx.entries) == 0 {
		return caveatsPageData{}, fmt.Errorf(
			"view %q renders the caveats index and no published document carries a caveat, "+
				"so the site would ship a nav entry to a page with nothing on it", v.Path)
	}
	var views []*caveatView
	for _, cv := range cx.views {
		if len(cv.Entries) > 0 {
			views = append(views, cv)
		}
	}
	carrying := map[string]bool{}
	for _, e := range cx.entries {
		for _, a := range e.About {
			for _, f := range a.Files {
				carrying[f.Stem] = true
			}
		}
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
			// NO Sources: TestEachViewsFooterCitesItsOwnSources holds a page
			// to the pages it showed a figure from, and this page shows none.
			// NO DataPath either, for buildProvenancePage's reason: this page
			// draws no projection.
		},
		Views:     views,
		Unviewed:  cx.unviewed,
		Entries:   cx.entries,
		Documents: len(carrying),
	}, nil
}
