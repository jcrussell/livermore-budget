package export

import (
	"bytes"
	"fmt"
	"html/template"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/site"
)

// caveatDoc is a shape-blind document of one year carrying caveats, as
// caveats JSON.
func caveatDoc(year, caveats string) []byte {
	return []byte(`{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"` + year + `",` +
		`"basis":"adopted","caveats":` + caveats + `}}`)
}

// caveatSite is three documents of three years, a view over each of the
// first two and the caveats view; "c" is read by no view.
func caveatSite(a, b, c string) (*Options, []navItem) {
	o := &Options{
		Projections: map[string][]byte{
			"a": caveatDoc("FY 2024-25", a), "b": caveatDoc("FY 2025-26", b), "c": caveatDoc("FY 2026-27", c),
		},
		Views: []View{
			{Path: "a.html", Nav: "A", Template: HistoryTemplate, Projection: "a"},
			{Path: "b.html", Nav: "B", Template: HistoryTemplate, Projection: "b"},
			{Path: "caveats.html", Nav: "Caveats", Template: CaveatsTemplate},
		},
	}
	nav := make([]navItem, 0, len(o.Views))
	for _, v := range o.Views {
		nav = append(nav, navItem{Label: v.Nav, Path: v.Path})
	}
	return o, nav
}

// TestTheCaveatIndexStatesEachCaveatOnce: one id is one entry under one
// anchor however many documents carry it, its texts variants told apart by
// year; two views stating the same entries share one section; each view's
// line counts that section. "In data no page states caveats for" holds only what no view
// states: c, which no view reaches, carries x as well as z, and a and b state
// x.
//
// Mutation: key entries on (id, text) and "y" is two entries, b's link
// landing on a's. Mutation: drop the variants' sort and y's texts are in
// document order, a's year after b's. Mutation: start a new section for every view and a.html and
// b.html each get one. Mutation: list as unviewed every entry an unreached
// document carries and x is in it beside the a and b section.
func TestTheCaveatIndexStatesEachCaveatOnce(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"y","summary":"Y.","theme":"our-reading","text":"In a.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"y","summary":"Y.","theme":"our-reading","text":"In b.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"z","summary":"Z.","theme":"left-out","text":"Only c.","applies_to":[]}]`,
	)
	// a is read first and is the later year, so the variants' order is the
	// years', not the documents'.
	o.Projections["a"] = caveatDoc("FY 2025-26", `[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"y","summary":"Y.","theme":"our-reading","text":"In a.","applies_to":[]}]`)
	o.Projections["b"] = caveatDoc("FY 2024-25", `[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"y","summary":"Y.","theme":"our-reading","text":"In b.","applies_to":[]}]`)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	type stated struct{ Anchor, Texts, Pages string }
	var got []stated
	for _, e := range cx.entries {
		var pages, texts []string
		for _, p := range e.Pages {
			pages = append(pages, p.Path)
		}
		for _, va := range e.Variants {
			texts = append(texts, va.Of+"|"+va.Text)
		}
		got = append(got, stated{e.Anchor, strings.Join(texts, " / "), strings.Join(pages, " ")})
	}
	want := []stated{
		{"caveat-x", "|Same.", "a.html b.html"},
		{"caveat-y", "FY 2024-25 adopted|In b. / FY 2025-26 adopted|In a.", "a.html b.html"},
		{"caveat-z", "|Only c.", ""},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("entries (-want +got):\n%s", diff)
	}

	for _, stem := range []string{"a", "b"} {
		refs, err := cx.refs([]caveatMeta{{ID: "x"}, {ID: "y"}}, stem, stem+".html")
		if err != nil {
			t.Fatalf("refs(%s): %v", stem, err)
		}
		if refs[0].Href != "caveats.html#caveat-x" || refs[1].Href != "caveats.html#caveat-y" {
			t.Errorf("%s links x to %q and y to %q, want caveats.html#caveat-x and caveats.html#caveat-y",
				stem, refs[0].Href, refs[1].Href)
		}
	}
	if _, err := cx.refs([]caveatMeta{{ID: "x"}}, "c-typo", "a.html"); err == nil {
		t.Error("a caveat of a document the index never read is linked rather than refused")
	}

	if cx.views[0].Section == nil || cx.views[0].Section != cx.views[1].Section {
		t.Fatal("a.html and b.html state the same caveats and do not share a section")
	}
	if s := cx.views[0].Section; s.Anchor != "caveats-on-a" || len(s.Pages) != 2 {
		t.Errorf("the shared section is %q over %d pages, want caveats-on-a over a.html and b.html", s.Anchor, len(s.Pages))
	}
	here := cx.here("b.html")
	var groups []string
	for _, g := range here.Groups {
		groups = append(groups, fmt.Sprintf("%s:%d", g.Anchor, len(g.Entries)))
	}
	if here.Count != 2 || here.Href != "caveats.html" ||
		!slices.Equal(groups, []string{"caveats-on-a--left-out:1", "caveats-on-a--our-reading:1"}) {
		t.Errorf("b.html's line = %d caveats at %q in %v, want 2 at caveats.html in left-out and our-reading",
			here.Count, here.Href, groups)
	}
	if got := cx.here("caveats.html"); got.Count != 0 || got.Href != "" || got.Groups != nil {
		t.Errorf("the caveats page's own line = %+v, want none", got)
	}
	var unviewed []string
	for _, e := range cx.unviewed {
		unviewed = append(unviewed, e.Anchor)
	}
	if diff := cmp.Diff([]string{"caveat-z"}, unviewed); diff != "" {
		t.Errorf("unviewed (-want +got):\n%s", diff)
	}
}

// TestTwoTextsOfOneYearAreRefused: two texts of one id carried by documents
// of the same years could not be told apart under one heading.
//
// Mutation: drop the Of refusal in buildCaveatIndex and it builds, heading
// both texts "FY 2025-26 adopted".
func TestTwoTextsOfOneYearAreRefused(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"One.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Two.","applies_to":[]}]`, `[]`)
	o.Projections["a"] = caveatDoc("FY 2025-26", `[{"id":"x","summary":"X.","theme":"left-out","text":"One.","applies_to":[]}]`)
	if _, err := buildCaveatIndex(o, ColumnIndex{}, nav); err == nil || !strings.Contains(err.Error(), "two of them are of FY 2025-26 adopted") {
		t.Errorf("buildCaveatIndex = %v, want two texts of one year refused", err)
	}
}

// TestADataFilesCaveatIsStatedOnNoPageAndACheckIsNoLimit: a data-files
// caveat is in no section, no count and not "In data no page states caveats for", which
// would say a page is owed it; a checked one is listed and not counted. Views
// whose lists differ get sections of their own.
//
// Mutation: drop the data-files test from the section loop and a.html's
// section lists d. Mutation: drop the data-files arm of the list split and
// d is stated nowhere on the caveats page. Mutation: keep the checked group in here and a.html's
// line is 2 in two themes.
func TestADataFilesCaveatIsStatedOnNoPageAndACheckIsNoLimit(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]},`+
			`{"id":"k","summary":"K.","theme":"checked","text":"T.","applies_to":[]},`+
			`{"id":"d","summary":"D.","theme":"data-files","text":"T.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`, `[]`)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	a, b := cx.views[0].Section, cx.views[1].Section
	if a == nil || b == nil || a == b {
		t.Fatal("a.html and b.html state different caveats and do not have a section each")
	}
	for _, e := range a.Entries {
		if e.ID == "d" {
			t.Error("a.html's section lists the data-files caveat")
		}
	}
	if here := cx.here("a.html"); here.Count != 1 || len(here.Groups) != 1 || len(here.Entries) != 2 {
		t.Errorf("a.html's line counts %d in %d themes of %d entries, want 1 limit in left-out of 2 entries",
			here.Count, len(here.Groups), len(here.Entries))
	}
	if len(cx.unviewed) != 0 {
		t.Errorf("%d caveats are listed as owed to a page, want none", len(cx.unviewed))
	}
	if len(cx.dataFiles) != 1 || cx.dataFiles[0].ID != "d" {
		t.Errorf("the data files' list is %d caveats, want d alone, which no other list states", len(cx.dataFiles))
	}
	var themes []string
	for _, g := range cx.themes {
		themes = append(themes, string(g.Theme))
	}
	if diff := cmp.Diff([]string{"left-out", "checked", "data-files"}, themes); diff != "" {
		t.Errorf("themes (-want +got):\n%s", diff)
	}
}

// TestTwoCaveatsTheIndexWouldHeadAlikeAreRefused: two ids under one summary
// would head two entries, and two fallback lines, alike.
//
// Mutation: drop the shared-summary refusal in buildCaveatIndex and both
// build, headed "S.".
func TestTwoCaveatsTheIndexWouldHeadAlikeAreRefused(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"S.","theme":"left-out","text":"One.","applies_to":[]},{"id":"y","summary":"S.","theme":"left-out","text":"Two.","applies_to":[]}]`,
		`[]`, `[]`,
	)
	_, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err == nil || !strings.Contains(err.Error(), "head two different caveats alike") {
		t.Errorf("buildCaveatIndex = %v, want two headings alike refused", err)
	}
}

// TestOneIDUnderTwoHeadingsIsRefused: the caveats page lists an id once, so
// a summary or theme that varies by document would publish one document's
// heading over another's text.
//
// Mutation: drop buildCaveatIndex's heading arm and both go red with
// "buildCaveatIndex = nil".
func TestOneIDUnderTwoHeadingsIsRefused(t *testing.T) {
	for name, b := range map[string]string{
		"two summaries": `[{"id":"x","summary":"4 groups.","theme":"left-out","text":"Same.","applies_to":[]}]`,
		"two themes":    `[{"id":"x","summary":"3 groups.","theme":"our-reading","text":"Same.","applies_to":[]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			o, nav := caveatSite(
				`[{"id":"x","summary":"3 groups.","theme":"left-out","text":"Same.","applies_to":[]}]`, b, `[]`)
			_, err := buildCaveatIndex(o, ColumnIndex{}, nav)
			if err == nil || !strings.Contains(err.Error(), "lists an id once") {
				t.Errorf("buildCaveatIndex = %v, want one id under two headings refused", err)
			}
		})
	}
}

// TestACaveatOfNoThemeIsRefused: the caveats page lists a caveat under its
// theme, so one of no theme would be listed nowhere.
//
// Mutation: drop buildCaveatIndex's theme arm and it goes red with
// "buildCaveatIndex = nil".
func TestACaveatOfNoThemeIsRefused(t *testing.T) {
	o, nav := caveatSite(`[{"id":"x","summary":"X.","theme":"caveat","text":"T.","applies_to":[]}]`, `[]`, `[]`)
	if _, err := buildCaveatIndex(o, ColumnIndex{}, nav); err == nil || !strings.Contains(err.Error(), "none of project.CaveatThemes") {
		t.Errorf("buildCaveatIndex = %v, want a theme of no set refused", err)
	}
}

// TestASiteWithNoCaveatsPageLinksNoCaveat: with no caveats view every href
// is empty and the line carries the headings the fallback list prints.
func TestASiteWithNoCaveatsPageLinksNoCaveat(t *testing.T) {
	o, nav := caveatSite(`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`, `[]`, `[]`)
	o.Views, nav = o.Views[:2], nav[:2]
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	here := cx.here("a.html")
	if here.Count != 1 || here.Href != "" || len(here.Entries) != 1 || here.Entries[0].Summary != "X." || cx.href("a.html", here.Entries[0]) != "" {
		t.Errorf("a.html's line = %+v, want one unlinked summary", here)
	}
}

// TestTheHeadingsRefusalNamesOnePairEveryRun: with two summaries each over
// two caveats headed alike, the refusal names the pair under the summary
// that sorts first, on every run.
//
// Mutation: range over bySummary itself and the refusal names "S." or "T."
// by map order, so twenty runs disagree.
func TestTheHeadingsRefusalNamesOnePairEveryRun(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"S.","theme":"left-out","text":"One.","applies_to":[]},{"id":"y","summary":"S.","theme":"left-out","text":"Two.","applies_to":[]},`+
			`{"id":"v","summary":"T.","theme":"left-out","text":"One.","applies_to":[]},{"id":"w","summary":"T.","theme":"left-out","text":"Two.","applies_to":[]}]`,
		`[]`, `[]`,
	)
	for range 20 {
		_, err := buildCaveatIndex(o, ColumnIndex{}, nav)
		if err == nil || !strings.Contains(err.Error(), `caveats "x" and "y"`) {
			t.Fatalf("buildCaveatIndex = %v, want the pair under \"S.\" named", err)
		}
	}
}

// TestASankeyViewWhoseYearDrawsNoScheduleIsRefused: a Sankey view drawing a
// schedule a year's column does not carry has no drawn document to flag, and
// the caveat index refuses it as [View.drawnStems] does rather than flagging
// nothing.
//
// Mutation: have flaggedStems skip a view drawnStems refuses and the index
// builds, flagging no document.
func TestASankeyViewWhoseYearDrawsNoScheduleIsRefused(t *testing.T) {
	o, nav := caveatSite(`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`, `[]`, `[]`)
	o.Views[0].Template, o.Views[0].Draws = SankeyTemplate, "fund-flows"
	_, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err == nil || !strings.Contains(err.Error(), "carries none") {
		t.Errorf("buildCaveatIndex = %v, want the year with no fund-flows refused", err)
	}
}

// TestASectionsThemeAnchorIsClaimedLikeAnyOther: a view's theme list is at
// "caveats-on-<view>--<theme>", which a view whose path is "<view>--<theme>"
// would compose as its own section.
//
// Mutation: drop the claim on a section's groups and it builds, one fragment
// on two lists.
func TestASectionsThemeAnchorIsClaimedLikeAnyOther(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`,
		`[{"id":"y","summary":"Y.","theme":"left-out","text":"T.","applies_to":[]}]`, `[]`)
	o.Views[1].Path, nav[1].Path = "a--left-out.html", "a--left-out.html"
	o.Views[0], o.Views[1] = o.Views[1], o.Views[0]
	nav[0], nav[1] = nav[1], nav[0]
	_, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err == nil || !strings.Contains(err.Error(), "is claimed by both") {
		t.Errorf("buildCaveatIndex = %v, want caveats-on-a--left-out claimed twice", err)
	}
}

// TestOneYearInTwoTextsIsRefused: a text of one id carried by two years and
// another carried by one of them would head two texts with that year.
//
// Mutation: compare whole Of strings rather than each year and it builds,
// "FY 2024-25 adopted" over one text and "FY 2024-25 adopted, FY 2025-26
// adopted" over another.
func TestOneYearInTwoTextsIsRefused(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`, `[]`)
	o.Projections["c"] = caveatDoc("FY 2024-25", `[{"id":"x","summary":"X.","theme":"left-out","text":"Other.","applies_to":[]}]`)
	if _, err := buildCaveatIndex(o, ColumnIndex{}, nav); err == nil || !strings.Contains(err.Error(), "two of them are of FY 2024-25 adopted") {
		t.Errorf("buildCaveatIndex = %v, want one year under two texts refused", err)
	}
}

// TestATextOfADocumentStatingNoYearIsHeadedByItsStem: a document stating no
// fiscal year is named by its stem rather than dropped from the heading.
//
// Mutation: skip a carrier whose label is "" and the second text's Of is
// "FY 2025-26 adopted" alone.
func TestATextOfADocumentStatingNoYearIsHeadedByItsStem(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"One.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Two.","applies_to":[]}]`, `[]`)
	o.Projections["c"] = []byte(`{"schema_version":1,"projection":"p","metadata":{"caveats":` +
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Two.","applies_to":[]}]}}`)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	var ofs []string
	for _, va := range cx.entries[0].Variants {
		ofs = append(ofs, va.Of)
	}
	if diff := cmp.Diff([]string{"FY 2024-25 adopted", "FY 2025-26 adopted, c"}, ofs); diff != "" {
		t.Errorf("variants' Of (-want +got):\n%s", diff)
	}
}

// TestACaveatNoPageCountsNamesNoNode: a data-files or checked caveat is no
// limit on any page, so a chart chipping one on a mark would warn of what
// the page does not count; the export refuses it as validateCaveats does,
// since it reads bytes.
//
// Mutation: drop the arm and both build.
func TestACaveatNoPageCountsNamesNoNode(t *testing.T) {
	for _, theme := range []string{"data-files", "checked"} {
		o, nav := caveatSite(`[{"id":"x","summary":"X.","theme":"`+theme+`","text":"T.","applies_to":["fund/100"]}]`, `[]`, `[]`)
		if _, err := buildCaveatIndex(o, ColumnIndex{}, nav); err == nil || !strings.Contains(err.Error(), "names nodes") {
			t.Errorf("%s: buildCaveatIndex = %v, want a caveat naming nodes refused", theme, err)
		}
	}
}

// TestViewsStatingOneSetInTwoOrdersShareASection: sections are matched as
// sets, so two views reaching the same documents in another order share one.
//
// Mutation: match sections with slices.Equal and b.html gets its own.
func TestViewsStatingOneSetInTwoOrdersShareASection(t *testing.T) {
	two := `[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]},{"id":"y","summary":"Y.","theme":"left-out","text":"T.","applies_to":[]}]`
	owt := `[{"id":"y","summary":"Y.","theme":"left-out","text":"T.","applies_to":[]},{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`
	o, nav := caveatSite(two, owt, `[]`)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	if cx.views[0].Section == nil || cx.views[0].Section != cx.views[1].Section {
		t.Error("a.html and b.html state x and y in two orders and do not share a section")
	}
}

// TestACaveatTwoSectionsShareIsLinkedToEachViewsOwn: x is in two sections,
// so the caveats page writes it twice; the canonical anchor is on the first,
// in nav order, the second takes its section's fragment, and each view's
// link lands on its own section's text. z is in one section and is linked
// at its canonical anchor from any view.
//
// Mutation: have href ignore the view and b.html links x at caveat-x, which
// is a.html's section. Mutation: put the canonical anchor on every section
// and the claim refuses caveat-x twice.
func TestACaveatTwoSectionsShareIsLinkedToEachViewsOwn(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]},{"id":"z","summary":"Z.","theme":"left-out","text":"T.","applies_to":[]}]`,
		`[]`)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	a, b := cx.views[0].Section, cx.views[1].Section
	if a == nil || b == nil || a == b {
		t.Fatal("a.html and b.html state different caveats and do not have a section each")
	}
	x, z := cx.byRef[caveatKey{"b", "x"}], cx.byRef[caveatKey{"b", "z"}]
	got := []string{a.At(x).Anchor, b.At(x).Anchor, b.At(z).Anchor,
		cx.href("a.html", x), cx.href("b.html", x), cx.href("a.html", z), cx.href("b.html", z)}
	want := []string{"caveat-x", "caveats-on-b--caveat-x", "caveat-z",
		"caveats.html#caveat-x", "caveats.html#caveats-on-b--caveat-x", "caveats.html#caveat-z", "caveats.html#caveat-z"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("anchors and links (-want +got):\n%s", diff)
	}
}

// TestTheFallbackLineListsTheLimitsTheLinkedLineCounts: with no caveats page a
// page's line lists the summaries of the limits the linked line would count,
// under "What these figures do not say", and not a check that passed.
//
// Mutation: list .Entries in the fallback and K. is listed as a limit.
func TestTheFallbackLineListsTheLimitsTheLinkedLineCounts(t *testing.T) {
	o, nav := caveatSite(`[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]},`+
		`{"id":"k","summary":"K.","theme":"checked","text":"T.","applies_to":[]}]`, `[]`, `[]`)
	o.Views, nav = o.Views[:2], nav[:2]
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	tmpl, err := template.New("t").ParseFS(site.FS(), ChromePartial)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "caveats-line", cx.here("a.html")); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "<li>X.</li>") || strings.Contains(got, "K.") {
		t.Errorf("a.html's fallback line is %q, want X. listed and K. not", got)
	}
}

// TestAVariantsYearsAreHeadedInOrder: a text carried by two years is headed by
// them in year order whatever order its documents sort in, so variants sort
// by their first year.
//
// Mutation: drop the sort of a variant's labels and "Both." is headed
// "FY 2025-26 adopted, FY 2024-25 adopted", after "FY 2024-25 actual".
func TestAVariantsYearsAreHeadedInOrder(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`, `[]`)
	o.Projections["a"] = caveatDoc("FY 2025-26", `[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`)
	o.Projections["b"] = caveatDoc("FY 2024-25", `[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`)
	o.Projections["c"] = []byte(`{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2023-24",` +
		`"basis":"actual","caveats":[{"id":"x","summary":"X.","theme":"left-out","text":"Other.","applies_to":[]}]}}`)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	var ofs []string
	for _, va := range cx.entries[0].Variants {
		ofs = append(ofs, va.Of)
	}
	if diff := cmp.Diff([]string{"FY 2023-24 actual", "FY 2024-25 adopted, FY 2025-26 adopted"}, ofs); diff != "" {
		t.Errorf("variants' Of (-want +got):\n%s", diff)
	}
}
