package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/project"
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
// A SEPARATE TYPE FROM project.Caveat because this package consumes
// projections as bytes. The field set is the contract, and it is pinned by
// TestEveryNameThisPackageDecodesIsOneAProjectionStates rather than by the two
// declarations happening to agree.
type caveatMeta struct {
	ID        string   `json:"id"`
	Summary   string   `json:"summary"`
	Text      string   `json:"text"`
	Theme     string   `json:"theme"`
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

// caveatsHere is the one line a page states its caveats in: the themes of
// its section that are limits, each counted and linked to that theme's list
// in the section; ThemeChecked qualifies nothing and is left to the section.
// With no caveats page Href is "" and the "caveats-line" template lists
// Groups' summaries instead, so a site without the page still states them.
//
// Groups and Entries are the caveats page's own section for this view
// (caveatIndex.views), so the line and the section cannot disagree.
type caveatsHere struct {
	// Count is how many limits apply, the entries of Groups.
	Count   int
	Href    string
	Groups  []caveatGroup
	Entries []*caveatEntry
}

// caveatKey is one caveat of one document. An id is unique within a
// document (caveatIndex refuses a repeat), so the pair names one caveat.
type caveatKey struct{ stem, id string }

// caveatIndex is every distinct caveat the published documents carry, each
// id once, and every view's share of them.
//
// DISTINCT IS THE ID. buildCaveatIndex refuses one id under two summaries or
// themes, so an entry has one heading; where documents carry the id with
// different texts (transfer-legs-unpaired's turns on that column's arithmetic)
// each text is a variant under it, told apart by the years carrying it.
type caveatIndex struct {
	// base is the caveats view's path, or "" when the site has none.
	base    string
	entries []*caveatEntry
	byRef   map[caveatKey]*caveatEntry
	// themes is every theme some entry is of, in project.CaveatThemes order.
	themes []caveatGroup
	// views is every view's section, in nav order, empty ones included; two
	// views stating the same entries share one *caveatSection.
	views []*caveatView
	// unviewed are the entries no view states and of no data-files theme,
	// which has its own list: every document carrying one is one no view
	// states caveats for, either drawn by no view or only stitched into an
	// expansion one draws.
	unviewed []*caveatEntry
}

// caveatEntry is one distinct caveat, rendered whole. Anchor is what every
// other page's link lands on.
type caveatEntry struct {
	ID      string
	Anchor  string
	Summary string
	Theme   project.CaveatTheme
	// Variants are its texts, in year order; one, with no Of, where every
	// document carrying it says the same.
	Variants []*caveatVariant
	// Pages are the views that state it, in nav order.
	Pages []navItem
}

// DataFiles is whether e is of project.ThemeDataFiles, which the template asks.
func (e *caveatEntry) DataFiles() bool { return e.Theme == project.ThemeDataFiles }

// Checked is whether e is of project.ThemeChecked, which no page's line
// states, so the template names the pages it was checked for instead.
func (e *caveatEntry) Checked() bool { return e.Theme == project.ThemeChecked }

// caveatVariant is one text of an entry and the documents carrying it.
type caveatVariant struct {
	// Of is the fiscal years of the documents carrying this text, "" when
	// the entry has one variant.
	Of   string
	Text string
	// About groups the documents that ship this text by what it is about in
	// each of them, in the order the documents sort.
	About   []caveatAbout
	carrier []string
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

// caveatGroup is the entries of one theme, in some list of the caveats page.
type caveatGroup struct {
	Theme   project.CaveatTheme
	Heading string
	Blurb   string
	// Anchor is this group's fragment on the caveats page.
	Anchor  string
	Entries []*caveatEntry
}

// caveatSection is one section of the caveats page's by-page list: the
// views that state exactly these entries, and the entries by theme.
type caveatSection struct {
	Pages   []navItem
	Anchor  string
	Groups  []caveatGroup
	Entries []*caveatEntry
}

// caveatView is one view and the section stating its caveats, nil for a
// view stating none.
type caveatView struct {
	Path    string
	Section *caveatSection
}

// caveatsPathOf is the caveats view's path, or "" when the site has none.
//
// Derived from the view set, so it cannot drift out of step with it.
func caveatsPathOf(o *Options) string { return templatePathOf(o, CaveatsTemplate) }

// caveatSectionPrefix, caveatEntryPrefix and caveatThemePrefix are the
// fragment spellings the caveats page emits, so no two kinds can claim one id.
const (
	caveatSectionPrefix = "caveats-on-"
	caveatEntryPrefix   = "caveat-"
	caveatThemePrefix   = "caveats-of-"
)

// slugUnsafe is everything a view path holds that a fragment slug does not.
var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// viewSlug is a view's path as a fragment: "index.html" is "index",
// "a/b.html" is "a-b".
func viewSlug(path string) string {
	s := strings.ToLower(strings.TrimSuffix(path, ".html"))
	return strings.Trim(slugUnsafe.ReplaceAllString(s, "-"), "-")
}

// caveatsOfView is the documents whose caveats a view states: those
// [View.ReachedStems] names less the schedules an expansion is stitched from,
// for a view that renders a document, none otherwise. A chart page states the
// schedule it draws and the spine beside it; the stitched documents' caveats
// are about marks the chart does not chip (site/app.js's caveatsFor reads the
// drawn schedule's alone). The one place this is decided, so the caveats
// page's section and the view's own line count the same documents.
func caveatsOfView(v View, ix ColumnIndex) ([]string, error) {
	if !templateRendersADocument(v.Template) {
		return nil, nil
	}
	reached, err := v.ReachedStems(ix)
	if err != nil || v.Draws != project.ExpansionProjection {
		return reached, err
	}
	keep := map[string]bool{}
	for _, year := range v.yearStems() {
		keep[year] = true
		column, folded := ix.Column(year)
		if !folded {
			continue
		}
		if stem, ok := ix.Stem(column, v.Draws); ok {
			keep[stem] = true
		}
		read, err := v.readStems(year, ix)
		if err != nil {
			return nil, err
		}
		for _, stem := range read {
			keep[stem] = true
		}
	}
	out := reached[:0:0]
	for _, stem := range reached {
		if keep[stem] {
			out = append(out, stem)
		}
	}
	return out, nil
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

// groupByTheme is entries in project.CaveatThemes order, then their own,
// one group per theme present. anchor is the fragment a group's is built on.
func groupByTheme(entries []*caveatEntry, anchor string) []caveatGroup {
	var out []caveatGroup
	for _, t := range project.CaveatThemes() {
		g := caveatGroup{
			Theme: t, Heading: project.CaveatThemeHeading(t), Blurb: project.CaveatThemeBlurb(t),
			Anchor: anchor + string(t),
		}
		for _, e := range entries {
			if e.Theme == t {
				g.Entries = append(g.Entries, e)
			}
		}
		if len(g.Entries) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// buildCaveatIndex reads every published document's caveats once.
//
// It refuses a caveat with no id, summary or text or with a theme of no
// [project.CaveatThemes], a data-files or checked caveat naming nodes, a
// document carrying one id twice, one id under two summaries or themes, two
// ids under one summary, two texts of one id sharing a year, and two
// fragments the caveats page would emit alike.
// project.ValidateCaveats never sees a document decoded from bytes, which is
// every document here.
func buildCaveatIndex(o *Options, ix ColumnIndex, nav []navItem) (*caveatIndex, error) {
	cx := &caveatIndex{base: caveatsPathOf(o), byRef: map[caveatKey]*caveatEntry{}}
	flagged, err := flaggedStems(o, ix)
	if err != nil {
		return nil, err
	}

	// labelOf is each document's fiscal year and basis, or its stem for a
	// document stating neither.
	labelOf := map[string]string{}
	byID := map[string]*caveatEntry{}
	// heading is each id's summary and theme, and the first document stating
	// them: the caveats page heads an id once.
	type heading struct{ summary, theme, stem string }
	headingOf := map[string]heading{}
	// summaryOf is the id each summary heads.
	summaryOf := map[string]string{}
	appliesOf := map[*caveatVariant]map[string][]string{}
	// carried is each document's entries, in the order it states them.
	carried := map[string][]*caveatEntry{}
	for _, stem := range slices.Sorted(maps.Keys(o.Projections)) {
		var dc documentCaveats
		if err := json.Unmarshal(o.Projections[stem], &dc); err != nil {
			return nil, fmt.Errorf("decode %s caveats: %w", stem, err)
		}
		labelOf[stem] = dc.label()
		if labelOf[stem] == "" {
			labelOf[stem] = stem
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
			case project.CaveatThemeHeading(project.CaveatTheme(c.Theme)) == "":
				return nil, fmt.Errorf(
					"%s caveat %q has theme %q, which is none of project.CaveatThemes, so the caveats page has nowhere to list it",
					stem, c.ID, c.Theme)
			}
			if (c.Theme == string(project.ThemeDataFiles) || c.Theme == string(project.ThemeChecked)) && len(c.AppliesTo) > 0 {
				return nil, fmt.Errorf(
					"%s caveat %q is of theme %s and names nodes; no page counts it as a limit, so a chart must "+
						"not chip it on a mark", stem, c.ID, c.Theme)
			}
			key := caveatKey{stem, c.ID}
			if _, dup := cx.byRef[key]; dup {
				return nil, fmt.Errorf(
					"%s carries caveat %q twice, so a link to it could land on either", stem, c.ID)
			}
			if h, ok := headingOf[c.ID]; !ok {
				headingOf[c.ID] = heading{c.Summary, c.Theme, stem}
			} else if h.summary != c.Summary || h.theme != c.Theme {
				return nil, fmt.Errorf(
					"caveat %q is headed %q under %s in %s and %q under %s in %s; the caveats page lists an id "+
						"once, so a figure that varies by document belongs in its text",
					c.ID, h.summary, h.theme, h.stem, c.Summary, c.Theme, stem)
			}
			if prev, ok := summaryOf[c.Summary]; ok && prev != c.ID {
				return nil, fmt.Errorf(
					"caveats %q and %q share the summary %q, so the caveats page would head two different "+
						"caveats alike and a reader could not tell which a link means",
					min(prev, c.ID), max(prev, c.ID), c.Summary)
			}
			summaryOf[c.Summary] = c.ID
			e, seen := byID[c.ID]
			if !seen {
				theme := project.CaveatTheme(c.Theme)
				e = &caveatEntry{
					ID: c.ID, Anchor: caveatEntryPrefix + c.ID, Summary: c.Summary,
					Theme: theme,
				}
				byID[c.ID] = e
				cx.entries = append(cx.entries, e)
			}
			at := slices.IndexFunc(e.Variants, func(v *caveatVariant) bool { return v.Text == c.Text })
			if at < 0 {
				e.Variants = append(e.Variants, &caveatVariant{Text: c.Text})
				at = len(e.Variants) - 1
			}
			va := e.Variants[at]
			va.carrier = append(va.carrier, stem)
			if appliesOf[va] == nil {
				appliesOf[va] = map[string][]string{}
			}
			appliesOf[va][stem] = c.AppliesTo
			cx.byRef[key] = e
			carried[stem] = append(carried[stem], e)
		}
	}

	// Anchors, and every fragment the page emits, refused when two collide.
	// The template's fixed ids are claimed first.
	fragments := map[string]string{
		"caveats-kinds":                 "the kinds list",
		"caveats-in-data-no-page-draws": "the list no page states",
	}
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
		if err := claim(e.Anchor, fmt.Sprintf("caveat %q", e.ID)); err != nil {
			return nil, err
		}
		ofs := map[string]bool{}
		for _, va := range e.Variants {
			for _, stem := range va.carrier {
				f := caveatFile{Stem: stem, Label: labelOf[stem], DataPath: ix.PublishedPath(stem)}
				applies := appliesOf[va][stem]
				// Flagged only splits a group that names nodes: the page says
				// nothing of it otherwise, and two groups would read alike.
				flags := len(applies) > 0 && flagged[stem]
				at := slices.IndexFunc(va.About, func(a caveatAbout) bool {
					return slices.Equal(a.AppliesTo, applies) && a.Flagged == flags
				})
				if at < 0 {
					va.About = append(va.About, caveatAbout{AppliesTo: applies, Flagged: flags})
					at = len(va.About) - 1
				}
				va.About[at].Files = append(va.About[at].Files, f)
			}
			if len(e.Variants) < 2 {
				continue
			}
			var of []string
			for _, stem := range va.carrier {
				if label := labelOf[stem]; !slices.Contains(of, label) {
					of = append(of, label)
				}
			}
			for _, label := range of {
				if ofs[label] {
					return nil, fmt.Errorf(
						"caveat %q carries %d texts and two of them are of %s, so the caveats page would head "+
							"two texts with one year; say it once, or give each schedule its own id",
						e.ID, len(e.Variants), label)
				}
				ofs[label] = true
			}
			slices.Sort(of)
			va.Of = strings.Join(of, ", ")
		}
		// A year's label opens "FY ", so years sort in order, and before any
		// stem standing for a document of no year.
		slices.SortStableFunc(e.Variants, func(a, b *caveatVariant) int { return strings.Compare(a.Of, b.Of) })
	}

	cx.themes = groupByTheme(cx.entries, caveatThemePrefix)
	for _, g := range cx.themes {
		for _, f := range []string{g.Anchor, g.Anchor + "-h"} {
			if err := claim(f, fmt.Sprintf("theme %q", g.Theme)); err != nil {
				return nil, err
			}
		}
	}

	var sections []*caveatSection
	for i, v := range o.Views {
		stems, err := caveatsOfView(v, ix)
		if err != nil {
			return nil, err
		}
		var entries []*caveatEntry
		for _, stem := range stems {
			// An unbuilt document is the view's builder's to refuse, in
			// its own words; nothing of it can be stated here.
			if _, built := o.Projections[stem]; !built {
				continue
			}
			for _, e := range carried[stem] {
				if e.Theme != project.ThemeDataFiles && !slices.Contains(entries, e) {
					entries = append(entries, e)
				}
			}
		}
		cv := &caveatView{Path: v.Path}
		cx.views = append(cx.views, cv)
		if len(entries) == 0 {
			continue
		}
		page := navItem{Label: nav[i].Label, Path: v.Path}
		for _, e := range entries {
			e.Pages = append(e.Pages, page)
		}
		for _, s := range sections {
			if sameEntries(s.Entries, entries) {
				cv.Section = s
				s.Pages = append(s.Pages, page)
				break
			}
		}
		if cv.Section == nil {
			anchor := caveatSectionPrefix + viewSlug(v.Path)
			if err := claim(anchor, fmt.Sprintf("view %q", v.Path)); err != nil {
				return nil, err
			}
			s := &caveatSection{Pages: []navItem{page}, Anchor: anchor, Entries: entries,
				Groups: groupByTheme(entries, anchor+"--")}
			for _, g := range s.Groups {
				if err := claim(g.Anchor, fmt.Sprintf("view %q's %s", v.Path, g.Theme)); err != nil {
					return nil, err
				}
			}
			sections = append(sections, s)
			cv.Section = s
		}
	}
	for _, e := range cx.entries {
		if len(e.Pages) == 0 && e.Theme != project.ThemeDataFiles {
			cx.unviewed = append(cx.unviewed, e)
		}
	}
	return cx, nil
}

// sameEntries is whether a and b hold the same entries, in any order.
func sameEntries(a, b []*caveatEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for _, e := range a {
		if !slices.Contains(b, e) {
			return false
		}
	}
	return true
}

// label is a document's fiscal year and basis as the caveats page names
// it, or "" for a document stating neither.
func (dc documentCaveats) label() string {
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

// here is one view's line: its section's themes, counted, and the section.
func (cx *caveatIndex) here(path string) caveatsHere {
	for _, cv := range cx.views {
		if cv.Path != path || cv.Section == nil {
			continue
		}
		s := cv.Section
		out := caveatsHere{Entries: s.Entries}
		for _, g := range s.Groups {
			if g.Theme != project.ThemeChecked {
				out.Groups = append(out.Groups, g)
				out.Count += len(g.Entries)
			}
		}
		if cx.base != "" {
			out.Href = cx.base
		}
		return out
	}
	return caveatsHere{}
}

// caveatsPageData is every distinct caveat, once, and each view's share.
type caveatsPageData struct {
	chrome
	// Themes are every entry by theme, in project.CaveatThemes order.
	Themes []caveatGroup
	// Sections are the by-page list, one per distinct set of entries, in
	// nav order of the first view stating it.
	Sections []*caveatSection
	// Unviewed are the caveats no page states, carried only by documents no
	// page states caveats for (drawn by no view, or only stitched into an
	// expansion one draws), and owed to whoever fetches the file.
	Unviewed []*caveatEntry
	Entries  []*caveatEntry
	// Documents is how many published documents carry a caveat.
	Documents int
}

// buildCaveatsPage lists every distinct caveat once, under an anchor every
// other page's link lands on, by theme, and a section per set of views
// stating the same caveats. It names no projection: it is an index across
// documents. It refuses an empty page; buildCaveatIndex refuses the rest.
func buildCaveatsPage(o *Options, v View, nav []navItem, cx *caveatIndex, ix ColumnIndex) (caveatsPageData, error) {
	if len(cx.entries) == 0 {
		return caveatsPageData{}, fmt.Errorf(
			"view %q renders the caveats index and no published document carries a caveat, "+
				"so the site would ship a nav entry to a page with nothing on it", v.Path)
	}
	var sections []*caveatSection
	for _, cv := range cx.views {
		if cv.Section != nil && !slices.Contains(sections, cv.Section) {
			sections = append(sections, cv.Section)
		}
	}
	carrying := map[string]bool{}
	for _, e := range cx.entries {
		for _, va := range e.Variants {
			for _, stem := range va.carrier {
				carrying[stem] = true
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
			SourcesPath: templatePathOf(o, ProvenanceTemplate),
			// NO Sources: this page shows no figure, so it cites no page.
			// NO DataPath either, for buildProvenancePage's reason: this page
			// draws no projection.
		},
		Themes:    cx.themes,
		Sections:  sections,
		Unviewed:  cx.unviewed,
		Entries:   cx.entries,
		Documents: len(carrying),
	}, nil
}
