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

// TestTheCaveatIndexStatesEachCaveatOnce: one id is one entry, its texts
// told apart by year; views stating the same entries share a section; the
// unviewed list holds only what no view states.
//
// Mutation: key entries on (id, text) and "y" is two entries. Mutation: drop
// the variants' sort and y's texts are in document order. Mutation: a section
// per view and a.html and b.html each get one. Mutation: list every entry an
// unreached document carries as unviewed and x is in it.
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

// TestTheCaveatIndexRefuses: each row is a caveatSite of three documents'
// caveats, optionally edited, that buildCaveatIndex must refuse with an error
// naming want.
func TestTheCaveatIndexRefuses(t *testing.T) {
	const x = `[{"id":"x","summary":"X.","theme":"left-out","text":"T.","applies_to":[]}]`
	for _, row := range []struct {
		name    string
		a, b, c string
		edit    func(o *Options, nav []navItem)
		want    string
	}{
		// Two texts of one id carried by documents of the same years could
		// not be told apart under one heading.
		// Mutation: drop the Of refusal in buildCaveatIndex and it builds,
		// heading both texts "FY 2025-26 adopted".
		{
			name: "TwoTextsOfOneYearAreRefused",
			a:    `[{"id":"x","summary":"X.","theme":"left-out","text":"One.","applies_to":[]}]`,
			b:    `[{"id":"x","summary":"X.","theme":"left-out","text":"Two.","applies_to":[]}]`, c: `[]`,
			edit: func(o *Options, _ []navItem) {
				o.Projections["a"] = caveatDoc("FY 2025-26", `[{"id":"x","summary":"X.","theme":"left-out","text":"One.","applies_to":[]}]`)
			},
			want: "two of them are of FY 2025-26 adopted",
		},
		// Two ids under one summary would head two entries, and two fallback
		// lines, alike.
		// Mutation: drop the shared-summary refusal in buildCaveatIndex and
		// both build, headed "S.".
		{
			name: "TwoCaveatsTheIndexWouldHeadAlikeAreRefused",
			a:    `[{"id":"x","summary":"S.","theme":"left-out","text":"One.","applies_to":[]},{"id":"y","summary":"S.","theme":"left-out","text":"Two.","applies_to":[]}]`,
			b:    `[]`, c: `[]`,
			want: "head two different caveats alike",
		},
		// A summary or theme that varies by document is refused, since the
		// caveats page lists an id once.
		// Mutation: drop buildCaveatIndex's heading arm and both rows go red.
		{
			name: "OneIDUnderTwoHeadingsIsRefused two summaries",
			a:    `[{"id":"x","summary":"3 groups.","theme":"left-out","text":"Same.","applies_to":[]}]`,
			b:    `[{"id":"x","summary":"4 groups.","theme":"left-out","text":"Same.","applies_to":[]}]`, c: `[]`,
			want: "lists an id once",
		},
		{
			name: "OneIDUnderTwoHeadingsIsRefused two themes",
			a:    `[{"id":"x","summary":"3 groups.","theme":"left-out","text":"Same.","applies_to":[]}]`,
			b:    `[{"id":"x","summary":"3 groups.","theme":"our-reading","text":"Same.","applies_to":[]}]`, c: `[]`,
			want: "lists an id once",
		},
		// The caveats page lists a caveat under its theme, so one of no theme
		// would be listed nowhere.
		// Mutation: drop buildCaveatIndex's theme arm and it builds.
		{
			name: "ACaveatOfNoThemeIsRefused",
			a:    `[{"id":"x","summary":"X.","theme":"caveat","text":"T.","applies_to":[]}]`, b: `[]`, c: `[]`,
			want: "none of project.CaveatThemes",
		},
		// The caveat index refuses a view drawing a schedule its year lacks,
		// as [View.drawnStems] does.
		// Mutation: have flaggedStems skip such a view and it builds, flagging
		// nothing.
		{
			name: "ASankeyViewWhoseYearDrawsNoScheduleIsRefused",
			a:    x, b: `[]`, c: `[]`,
			edit: func(o *Options, _ []navItem) {
				o.Views[0].Template, o.Views[0].Draws = SankeyTemplate, "fund-flows"
			},
			want: "carries none",
		},
		// A theme list's anchor "caveats-on-<view>--<theme>" collides with a
		// view at "<view>--<theme>".
		// Mutation: drop the claim on a section's groups and it builds, one
		// fragment on two lists.
		{
			name: "ASectionsThemeAnchorIsClaimedLikeAnyOther",
			a:    x,
			b:    `[{"id":"y","summary":"Y.","theme":"left-out","text":"T.","applies_to":[]}]`, c: `[]`,
			edit: func(o *Options, nav []navItem) {
				o.Views[1].Path, nav[1].Path = "a--left-out.html", "a--left-out.html"
				o.Views[0], o.Views[1] = o.Views[1], o.Views[0]
				nav[0], nav[1] = nav[1], nav[0]
			},
			want: "is claimed by both",
		},
		// A text of one id carried by two years and another carried by one of
		// them would head two texts with that year.
		// Mutation: compare whole Of strings rather than each year and it
		// builds, FY 2024-25 heading both texts.
		{
			name: "OneYearInTwoTextsIsRefused",
			a:    `[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`,
			b:    `[{"id":"x","summary":"X.","theme":"left-out","text":"Both.","applies_to":[]}]`, c: `[]`,
			edit: func(o *Options, _ []navItem) {
				o.Projections["c"] = caveatDoc("FY 2024-25", `[{"id":"x","summary":"X.","theme":"left-out","text":"Other.","applies_to":[]}]`)
			},
			want: "two of them are of FY 2024-25 adopted",
		},
		// A data-files or checked caveat naming a node is refused, as
		// validateCaveats does.
		// Mutation: drop the arm and both rows build.
		{
			name: "ACaveatNoPageCountsNamesNoNode data-files",
			a:    `[{"id":"x","summary":"X.","theme":"data-files","text":"T.","applies_to":["fund/100"]}]`, b: `[]`, c: `[]`,
			want: "names nodes",
		},
		{
			name: "ACaveatNoPageCountsNamesNoNode checked",
			a:    `[{"id":"x","summary":"X.","theme":"checked","text":"T.","applies_to":["fund/100"]}]`, b: `[]`, c: `[]`,
			want: "names nodes",
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			o, nav := caveatSite(row.a, row.b, row.c)
			if row.edit != nil {
				row.edit(o, nav)
			}
			if _, err := buildCaveatIndex(o, ColumnIndex{}, nav); err == nil || !strings.Contains(err.Error(), row.want) {
				t.Errorf("buildCaveatIndex = %v, want a refusal naming %q", err, row.want)
			}
		})
	}
}

// TestADataFilesCaveatIsStatedOnNoPageAndACheckIsNoLimit: a data-files
// caveat is in no section, count or unviewed list; a checked one is listed
// and not counted.
//
// Mutation: drop the data-files test from the section loop and a.html lists
// d. Mutation: drop the data-files arm of the list split and d is unviewed.
// Mutation: keep the checked group and a.html's line is 2 in two themes.
// Mutation: drop the template's Checked branch and k says a.html states it.
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

	data, err := buildCaveatsPage(pageCtx{o: o, v: o.Views[2], nav: nav, cx: cx})
	if err != nil {
		t.Fatalf("buildCaveatsPage: %v", err)
	}
	html, err := renderPage(o.assetTree(), CaveatsTemplate, data)
	if err != nil {
		t.Fatalf("render caveats.html: %v", err)
	}
	_, k, _ := strings.Cut(string(html), `id="caveat-k"`)
	k, _, _ = strings.Cut(k, "</details>")
	if !strings.Contains(k, "Checked for") || strings.Contains(k, "Stated on") {
		t.Errorf("caveats.html's entry for the checked k does not say which page it was checked for: %s", k)
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

// TestACaveatTwoSectionsShareIsLinkedToEachViewsOwn: x, in two sections,
// takes the canonical anchor on the first and each view links its own; z, in
// one, is linked at its canonical anchor.
//
// Mutation: have href ignore the view and b.html links x at caveat-x.
// Mutation: put the canonical anchor on every section and b's reads caveat-x.
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

// TestTheFallbackLineListsTheLimitsTheLinkedLineCounts: with no caveats page
// a page's line lists the limits' summaries, not a check that passed.
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

// TestAVariantsYearsAreHeadedInOrder: a text carried by two years is headed
// by them in year order whatever order its documents sort in.
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
