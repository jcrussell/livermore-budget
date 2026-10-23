package export_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// glanceFunding is a department-funding document of the golden spine's
// column, FY2025-26 adopted, whose whole_department links sum to the spine's
// all-funds gross expenditure less short: two departments paid by the General
// Fund, police by an internal service fund too.
func glanceFunding(short int64) []byte {
	const whole = 25409541200
	police := int64(20000000000)
	fleet := int64(400000000)
	council := whole - police - fleet - short
	link := func(from, to string, cents int64, kind string, page int) string {
		return fmt.Sprintf(`{"source":%q,"target":%q,"value_cents":%d,"kind":%q,"fact_ids":["fisc-f-%s"],`+
			`"locators":[{"doc_id":"livermore-budget-fy2026-2027","pages":[%d]}]}`,
			from, to, cents, kind, fmt.Sprintf("%x", sha256.Sum256([]byte(from+to)))[:12], page)
	}
	return []byte(`{"schema_version":1,"projection":"department-funding","nodes":[` +
		`{"id":"fund-group/general","label":"General Fund","tier":2,"role":"fund_group"},` +
		`{"id":"fund-group/internal-service","label":"Internal Service Funds","tier":2,"role":"fund_group"},` +
		`{"id":"fund/100","label":"General Fund","tier":3,"role":"general_fund","parent":"fund-group/general"},` +
		`{"id":"fund/730","label":"Fleet","tier":3,"role":"fund","parent":"fund-group/internal-service"},` +
		`{"id":"department/police-department","label":"Police Department","tier":4,"role":"whole_department"},` +
		`{"id":"department/city-council","label":"City Council","tier":4,"role":"whole_department"}],` +
		`"links":[` + link("fund/100", "department/police-department", police, "external", 119) + `,` +
		link("fund/730", "department/police-department", fleet, "internal_service", 120) + `,` +
		link("fund/100", "department/city-council", council, "external", 85) + `],` +
		`"metadata":{"generated_by":"testdata/sankey.golden.json (hand-derived, Wave 0)","fiscal_year":2026,"fiscal_year_label":"FY 2025-26",` +
		`"basis":"adopted","scopes":["department-funding-sources"],"currency":"USD","units":"cents",` +
		`"sources":[{"doc_id":"livermore-budget-fy2026-2027","pages":[85, 119, 120]}],` +
		`"counts":{"facts":3,"nodes":6,"links":3},` +
		`"caveats":[{"id":"a-department-here-is-not-a-division","summary":"Departments, not divisions.","theme":"left-out","text":"A text.","applies_to":[]}]}}`)
}

// glanceOptions is a site with the spine and the glance page over the golden
// spine and funding.
func glanceOptions(t *testing.T, dir string, funding []byte, apart []export.ApartFromSpine) export.Options {
	t.Helper()
	return export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			export.PrimaryProjection:            goldenSankey(t),
			project.DepartmentFundingProjection: funding,
		},
		Views: append(spineOnly(), export.View{Path: "glance.html", Nav: "At a glance",
			Template: export.GlanceTemplate, Projection: export.PrimaryProjection,
			Reads: []string{project.DepartmentFundingProjection},
			Title: "A glance title", Lede: "A glance lede."}),
		Docs:                      budgetDocs(),
		DepartmentsApartFromSpine: apart,
	}
}

// goldenLinks is the golden spine's links, decoded as the glance page reads
// them.
func goldenLinks(t *testing.T) []struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	ValueCents int64  `json:"value_cents"`
} {
	t.Helper()
	var doc struct {
		Links []struct {
			Source     string `json:"source"`
			Target     string `json:"target"`
			ValueCents int64  `json:"value_cents"`
		} `json:"links"`
	}
	if err := json.Unmarshal(goldenSankey(t), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Links
}

var glanceRow = regexp.MustCompile(`<tr data-group="([^"]+)" data-facts="[^"]*">\s*<th scope="row">[^<]+</th>\s*<td>[^<]+</td>\s*<td class="num" data-cents="(\d+)">`)

// barsOf cuts the glance page into its bars, by id.
func barsOf(t *testing.T, html string) map[string]string {
	t.Helper()
	out := map[string]string{}
	parts := strings.Split(html, `<section class="glance-bar" data-bar="`)
	for _, p := range parts[1:] {
		id, rest, _ := strings.Cut(p, `"`)
		body, _, _ := strings.Cut(rest, "</section>")
		out[id] = body
	}
	return out
}

// TestGlancePageRendersEveryCellAndTheCitedWhole holds the glance page to
// one row per link of each bar, each with the link's own cents and group,
// and to the spine's own figure printed beside each bar as the whole the
// rows sum to, with no declaration where none is needed.
//
// Mutation: pick revenue links by target instead of source in buildGlancePage
// and the revenue bar draws no row; render Whole from the client's sum
// instead of the headline and the whole test fails to find the headline's
// figure.
func TestGlancePageRendersEveryCellAndTheCitedWhole(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(glanceOptions(t, dir, glanceFunding(0), nil)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "glance.html")
	for _, want := range []string{"<h1>A glance title</h1>", "A glance lede.", `import { boot } from "./glance.js"`,
		`<h2>FY 2025-26 adopted</h2>`, `href="index.html"`} {
		if !strings.Contains(html, want) {
			t.Errorf("glance.html does not contain %q", want)
		}
	}
	bars := barsOf(t, html)
	if len(bars) != 3 {
		t.Fatalf("got %d bars, want revenue, spending and departments", len(bars))
	}

	type cell struct {
		group string
		cents int64
	}
	want := map[string][]cell{}
	for _, l := range goldenLinks(t) {
		switch {
		case strings.HasPrefix(l.Source, "revenue/"):
			want["revenue"] = append(want["revenue"], cell{l.Source, l.ValueCents})
		case strings.HasPrefix(l.Target, "expenditure/"):
			want["spending"] = append(want["spending"], cell{l.Target, l.ValueCents})
		}
	}
	want["departments"] = []cell{{"department/police-department", 20000000000},
		{"department/police-department", 400000000}, {"department/city-council", 5009541200}}
	wholes := map[string]string{"revenue": "$299,969,007", "spending": "$254,095,412", "departments": "$254,095,412"}
	for id, body := range bars {
		var got []cell
		var sum int64
		for _, m := range glanceRow.FindAllStringSubmatch(body, -1) {
			var c int64
			fmt.Sscan(m[2], &c)
			got = append(got, cell{m[1], c})
			sum += c
		}
		if fmt.Sprint(got) != fmt.Sprint(want[id]) {
			t.Errorf("%s bar rows:\n got %v\nwant %v", id, got, want[id])
		}
		if !strings.Contains(body, `data-whole-cents>`+wholes[id]+`<`) {
			t.Errorf("%s bar does not print the spine's %s beside its rows", id, wholes[id])
		}
		if !strings.Contains(body, `data-ties="true"`) || strings.Contains(body, "glance-apart") {
			t.Errorf("%s bar says its rows do not tie, and no residual is declared", id)
		}
		t.Logf("%s: %d rows summing to %d", id, len(got), sum)
	}
	// The schedule a bar reads beside the spine states its caveats on the
	// page's one line, once whatever the years.
	if got := strings.Count(html, "Departments, not divisions."); got != 1 {
		t.Errorf("the page states department-funding's caveat %d times, want once", got)
	}
	if !strings.Contains(html, `href="https://www.livermoreca.gov/home/showpublisheddocument/12813#page=119"`) {
		t.Error("a department row does not link the PDF page it was read from")
	}
}

// TestGlancePageRefusesRowsThatDoNotSumToTheCitedWhole: the whole the page
// prints beside a bar is the city's, and the rows are what the client sums,
// so rows that do not reach it are refused rather than drawn under it.
//
// Mutation: return nil from glanceTies and the first case writes a page whose
// departments bar is a dollar short of the figure beside it.
func TestGlancePageRefusesRowsThatDoNotSumToTheCitedWhole(t *testing.T) {
	_, err := writeSite(glanceOptions(t, t.TempDir(), glanceFunding(100), nil))
	if err == nil || !strings.Contains(err.Error(), "sum to $254,095,411.00 and the page prints $254,095,412.00") {
		t.Errorf("rows a dollar short: got %v, want a refusal naming both figures", err)
	}

	// A declared residual of the wrong size is refused the same way.
	apart := []export.ApartFromSpine{{FiscalYear: 2026, Basis: "adopted", ResidualCents: 200, Reason: "a reason"}}
	_, err = writeSite(glanceOptions(t, t.TempDir(), glanceFunding(100), apart))
	if err == nil || !strings.Contains(err.Error(), "1 declared residuals sum to $254,095,413.00") {
		t.Errorf("a residual of the wrong size: got %v, want a refusal", err)
	}

	// An inferred link is refused, since every row is listed as printed.
	// Mutation: drop the Derived test in glanceRows and the page writes.
	inferred := strings.Replace(string(glanceFunding(0)), `"kind":"internal_service"`, `"kind":"internal_service","derived":true`, 1)
	_, err = writeSite(glanceOptions(t, t.TempDir(), []byte(inferred), nil))
	if err == nil || !strings.Contains(err.Error(), "fund/730 -> department/police-department inferred") {
		t.Errorf("an inferred row: got %v, want a refusal naming it", err)
	}

	// A row citing a document the registry does not describe is refused.
	// Mutation: compose the row's pages with sourcesFor in place of
	// cellCites.locators and the page writes.
	gone := strings.Replace(string(glanceFunding(0)), `"doc_id":"livermore-budget-fy2026-2027","pages":[120]`, `"doc_id":"gone","pages":[120]`, 1)
	_, err = writeSite(glanceOptions(t, t.TempDir(), []byte(gone), nil))
	if err == nil || !strings.Contains(err.Error(), "gone p120") {
		t.Errorf("a row of an undescribed document: got %v, want a refusal naming gone p120", err)
	}

	// And a view reading anything but department-funding is refused before
	// a row is read.
	o := glanceOptions(t, t.TempDir(), glanceFunding(0), nil)
	o.Views[1].Reads = []string{project.DepartmentSpendingProjection}
	_, err = writeSite(o)
	if err == nil || !strings.Contains(err.Error(), "reads schedule") && !strings.Contains(err.Error(), "nothing else") {
		t.Errorf("a glance view reading department-spending: got %v, want a refusal", err)
	}
}

// TestGlancePagePrintsADeclaredResidualInsteadOfATie: where the caller
// declares the department rows apart from the spine, the page says by how
// much and why, in Go's words, and marks the bar as not tying; the client
// computes no difference.
//
// Mutation: drop the residual from glanceTies' sum and the declared column is
// refused; set Ties to true unconditionally and data-ties lies.
func TestGlancePagePrintsADeclaredResidualInsteadOfATie(t *testing.T) {
	dir := t.TempDir()
	apart := []export.ApartFromSpine{{FiscalYear: 2026, Basis: "adopted", ResidualCents: 25000000,
		Reason: "p0067 prints a figure the funds' own rows do not reach"}}
	if _, err := writeSite(glanceOptions(t, dir, glanceFunding(25000000), apart)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	bars := barsOf(t, readFile(t, dir, "glance.html"))
	body := bars["departments"]
	for _, want := range []string{`data-ties="false"`, "sum to <strong>$250,000</strong> less than that figure",
		"p0067 prints a figure the funds&#39; own rows do not reach.", `data-whole-cents>$254,095,412<`} {
		if !strings.Contains(body, want) {
			t.Errorf("the departments bar does not say %q", want)
		}
	}
	for _, id := range []string{"revenue", "spending"} {
		if !strings.Contains(bars[id], `data-ties="true"`) {
			t.Errorf("the %s bar is marked apart by a declaration about the departments", id)
		}
	}

	// A residual is signed: rows above the whole say "more", with the
	// magnitude, never "-$250,000 less".
	above := []export.ApartFromSpine{{FiscalYear: 2026, Basis: "adopted", ResidualCents: -25000000,
		Reason: "a reason"}}
	dir = t.TempDir()
	if _, err := writeSite(glanceOptions(t, dir, glanceFunding(-25000000), above)); err != nil {
		t.Fatalf("Write with rows above the whole: %v", err)
	}
	if body := barsOf(t, readFile(t, dir, "glance.html"))["departments"]; !strings.Contains(body, "sum to <strong>$250,000</strong> more than that figure") {
		t.Errorf("the departments bar does not say the rows sum to $250,000 more than the whole")
	}

	// The declaration applies to its own column only: the same page over a
	// funding document that ties is refused, because the residual would then
	// be unaccounted for.
	_, err := writeSite(glanceOptions(t, t.TempDir(), glanceFunding(0), apart))
	if err == nil || !strings.Contains(err.Error(), "1 declared residuals") {
		t.Errorf("a declaration over rows that tie: got %v, want a refusal", err)
	}
	apart[0].Reason = ""
	_, err = writeSite(glanceOptions(t, t.TempDir(), glanceFunding(25000000), apart))
	if err == nil || !strings.Contains(err.Error(), "no reason") {
		t.Errorf("a declaration with no reason: got %v, want a refusal", err)
	}
}
