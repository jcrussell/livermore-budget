package export

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// caveatDoc is a shape-blind document carrying caveats, as caveats JSON.
func caveatDoc(caveats string) []byte {
	return []byte(`{"schema_version":1,"projection":"p","metadata":{"fiscal_year_label":"FY 2025-26",` +
		`"basis":"adopted","caveats":` + caveats + `}}`)
}

// caveatSite is three documents, a view over each of the first two and the
// caveats view; "c" is read by no view.
func caveatSite(a, b, c string) (*Options, []navItem) {
	o := &Options{
		Projections: map[string][]byte{"a": caveatDoc(a), "b": caveatDoc(b), "c": caveatDoc(c)},
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

// TestTheCaveatIndexStatesEachCaveatOnce: one (id, summary, text) carried by
// two documents is one entry both link to, one id carrying two texts is two
// entries under two anchors, and each view's line counts its own section.
// "In data no page draws" holds only what no view states: c, which no view
// reaches, carries x as well as z, and a and b state x.
//
// Mutation: key byVariant on the id alone and "y" is one entry, whose text is
// a's and which b's link lands on. Mutation: list as unviewed every entry an
// unreached document carries and x is in it beside a's and b's sections.
func TestTheCaveatIndexStatesEachCaveatOnce(t *testing.T) {
	o, nav := caveatSite(
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"y","summary":"Y.","theme":"left-out","text":"In a.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"y","summary":"Y.","theme":"left-out","text":"In b.","applies_to":[]}]`,
		`[{"id":"x","summary":"X.","theme":"left-out","text":"Same.","applies_to":[]},{"id":"z","summary":"Z.","theme":"left-out","text":"Only c.","applies_to":[]}]`,
	)
	cx, err := buildCaveatIndex(o, ColumnIndex{}, nav)
	if err != nil {
		t.Fatalf("buildCaveatIndex: %v", err)
	}
	type stated struct{ Anchor, Of, Text, Pages string }
	var got []stated
	for _, e := range cx.entries {
		var pages []string
		for _, p := range e.Pages {
			pages = append(pages, p.Path)
		}
		got = append(got, stated{e.Anchor, e.Of, e.Text, strings.Join(pages, " ")})
	}
	want := []stated{
		{"caveat-x", "", "Same.", "a.html b.html"},
		// One summary over two texts, of documents stating one year: told
		// apart by stem.
		{"caveat-y--a", "a", "In a.", "a.html"},
		{"caveat-y--b", "b", "In b.", "b.html"},
		{"caveat-z", "", "Only c.", ""},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("entries (-want +got):\n%s", diff)
	}

	for stem, wantHref := range map[string]string{"a": "caveats.html#caveat-y--a", "b": "caveats.html#caveat-y--b"} {
		refs, err := cx.refs([]caveatMeta{{ID: "x"}, {ID: "y"}}, stem)
		if err != nil {
			t.Fatalf("refs(%s): %v", stem, err)
		}
		if refs[0].Href != "caveats.html#caveat-x" || refs[1].Href != wantHref {
			t.Errorf("%s links x to %q and y to %q, want caveats.html#caveat-x and %q",
				stem, refs[0].Href, refs[1].Href, wantHref)
		}
	}
	if _, err := cx.refs([]caveatMeta{{ID: "x"}}, "c-typo"); err == nil {
		t.Error("a caveat of a document the index never read is linked rather than refused")
	}

	here := cx.here("a.html")
	if here.Count != 2 || here.Href != "caveats.html#caveats-on-a" || len(here.Entries) != here.Count {
		t.Errorf("a.html's line = %+v, want 2 caveats linking to caveats.html#caveats-on-a", here)
	}
	if got := cx.here("caveats.html"); got.Count != 0 || got.Href != "" {
		t.Errorf("the caveats page's own line = %+v, want none", got)
	}
	var unviewed []string
	for _, e := range cx.unviewed {
		unviewed = append(unviewed, e.Anchor)
		for _, cv := range cx.views {
			if slices.Contains(cv.Entries, e) {
				t.Errorf("%s is listed in data no page draws and in %s's section", e.Anchor, cv.Path)
			}
		}
	}
	if diff := cmp.Diff([]string{"caveat-z"}, unviewed); diff != "" {
		t.Errorf("unviewed (-want +got):\n%s", diff)
	}
}

// TestTwoCaveatsTheIndexWouldHeadAlikeAreRefused: two ids under one summary,
// carried by the same documents, take one Of whether it is read off the
// years or the stems, so their headings and fallback lines read alike.
//
// Mutation: drop the headings refusal in buildCaveatIndex and both build,
// headed "S. (a)".
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
	if here.Count != 1 || here.Href != "" || len(here.Entries) != 1 || here.Entries[0].Summary != "X." || cx.href(here.Entries[0]) != "" {
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
