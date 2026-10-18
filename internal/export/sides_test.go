package export_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// sidesExpansion is an expansion schedule of the golden spine's column,
// FY2025-26 adopted: two revenue roots at tier 0, one fund group at tier 2
// that both takes in and pays out, one object category at tier 5, and a
// split of the first revenue root into two lines. `both` adds a link from the
// tier-5 root back to the second revenue root, so each flows both ways.
func sidesExpansion(both bool) []byte {
	link := func(from, to string, cents int64) string {
		return fmt.Sprintf(`{"source":%q,"target":%q,"value_cents":%d,"kind":"external","fact_ids":["fisc-f-%012x"],`+
			`"locators":[{"doc_id":"livermore-budget-fy2026-2027","pages":[66]}]}`, from, to, cents, len(from)*1000+len(to))
	}
	links := []string{
		link("revenue/a", "fund-group/general", 600_00),
		link("revenue/b", "fund-group/general", 400_00),
		link("fund-group/general", "expenditure/x", 1000_00),
		link("line/a1", "revenue/a", 350_00),
		link("line/a2", "revenue/a", 250_00),
	}
	if both {
		links = append(links, link("expenditure/x", "revenue/b", 1_00))
	}
	return []byte(`{"schema_version":1,"projection":"expansion","nodes":[` +
		`{"id":"revenue/a","label":"A","tier":0,"role":"revenue_source"},` +
		`{"id":"revenue/b","label":"B","tier":0,"role":"revenue_source"},` +
		`{"id":"line/a1","label":"A1","tier":1,"role":"revenue_line","parent":"revenue/a"},` +
		`{"id":"line/a2","label":"A2","tier":1,"role":"revenue_line","parent":"revenue/a"},` +
		`{"id":"fund-group/general","label":"General Fund","tier":2,"role":"fund_group"},` +
		`{"id":"expenditure/x","label":"X","tier":5,"role":"object_category"}],` +
		`"links":[` + strings.Join(links, ",") + `],` +
		`"splits":[{"node":"revenue/a","key":"lines","form":"extend","alternative":false,"side":"before"}],` +
		`"metadata":{"generated_by":"testdata/sankey.golden.json (hand-derived, Wave 0)","fiscal_year":2026,"fiscal_year_label":"FY 2025-26",` +
		`"basis":"adopted","scopes":["all-funds-gross"],"currency":"USD","units":"cents",` +
		`"sources":[{"doc_id":"livermore-budget-fy2026-2027","pages":[66]}],` +
		`"counts":{"facts":5,"nodes":6,"links":5},"caveats":[]}}`)
}

// sidesOptions is a site with the spine and one sides page of form `form`
// drawing sidesExpansion with the sides given.
func sidesOptions(t *testing.T, dir, form string, hints *export.SidesHints, both bool) export.Options {
	t.Helper()
	chart := export.Chart{Form: form}
	if form == export.LedgerForm {
		chart.Ledger = hints
	}
	tmpl := export.LedgerTemplate
	return export.Options{
		Dir: dir,
		Projections: map[string][]byte{
			export.PrimaryProjection:    goldenSankey(t),
			project.ExpansionProjection: sidesExpansion(both),
		},
		Views: append(spineOnly(), export.View{Path: form + ".html", Nav: "Sides",
			Template: tmpl, Projection: export.PrimaryProjection, Draws: project.ExpansionProjection,
			Overview: chart, Title: "A sides title", Lede: "A sides lede."}),
		Docs: budgetDocs(),
	}
}

func twoSides() *export.SidesHints {
	return &export.SidesHints{Sides: []export.Side{{Tier: 0, Heading: "In"}, {Tier: 5, Heading: "Out"}}}
}

// TestSidesPagesCarryTheirSidesAndConfig holds each sides page to one section
// per declared side, the config the client folds by with the form's own
// hints and no other's, and the year the page opens on.
func TestSidesPagesCarryTheirSidesAndConfig(t *testing.T) {
	for _, form := range []string{export.LedgerForm} {
		dir := t.TempDir()
		if _, err := writeSite(sidesOptions(t, dir, form, twoSides(), false)); err != nil {
			t.Fatalf("%s: Write: %v", form, err)
		}
		html := readFile(t, dir, form+".html")
		for _, want := range []string{
			"<h1>A sides title</h1>", "A sides lede.", `import { boot } from "./` + form + `.js"`,
			`<section class="sides-side" data-tier="0" aria-labelledby="side-0">`, `<h2 id="side-0">In</h2>`,
			`<section class="sides-side" data-tier="5" aria-labelledby="side-5">`, `<h2 id="side-5">Out</h2>`,
			`"primary":"expansion"`, `"form":"` + form + `"`, `"` + form + `":{"sides":[{"tier":0,"heading":"In"},{"tier":5,"heading":"Out"}]}`,
			`"path":"fy2026-adopted.json"`, `<option value="fy2026-adopted.json" selected>FY 2025-26 adopted</option>`,
			`<template id="` + form + `-`,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("%s.html does not contain %q", form, want)
			}
		}
		for _, other := range export.ChartForms() {
			if other != form && strings.Contains(html, `"`+other+`":{`) {
				t.Errorf("%s.html carries %s hints", form, other)
			}
		}
	}
}

// TestSidesPagesRefuseWhatTheyCouldNotDraw: a side with no heading, a tier
// twice, a tier with no root, a root flowing both ways, hints of another
// form, and no hints at all are each refused by name before a page is written.
//
// Mutation: drop the leaving-and-arriving arm of validateSides and the
// "both ways" case writes a page whose side has no one figure.
func TestSidesPagesRefuseWhatTheyCouldNotDraw(t *testing.T) {
	for _, c := range []struct {
		name  string
		form  string
		hints *export.SidesHints
		both  bool
		edit  func(*export.Options)
		want  string
	}{
		{"no heading", export.LedgerForm, &export.SidesHints{Sides: []export.Side{{Tier: 0}}}, false, nil, "with no heading"},
		{"a tier twice", export.LedgerForm, &export.SidesHints{Sides: []export.Side{{Tier: 0, Heading: "In"}, {Tier: 0, Heading: "Again"}}}, false, nil, "as a side twice"},
		{"a tier with no root", export.LedgerForm, &export.SidesHints{Sides: []export.Side{{Tier: 1, Heading: "Lines"}}}, false, nil, "has no root at it"},
		{"a tier the document lacks", export.LedgerForm, &export.SidesHints{Sides: []export.Side{{Tier: 7, Heading: "Nowhere"}}}, false, nil, "draws tier 7"},
		{"the middle tier", export.LedgerForm, &export.SidesHints{Sides: []export.Side{{Tier: 2, Heading: "Funds"}}}, false, nil, "both takes in and pays out"},
		{"a root flowing both ways", export.LedgerForm, twoSides(), true, nil, "revenue/b both takes in and pays out"},
		{"no hints", export.LedgerForm, nil, false, nil, "with no ledger hints"},
		{"another form's hints", export.LedgerForm, twoSides(), false, func(o *export.Options) {
			o.Views[1].Overview.Sankey = &export.SankeyHints{}
		}, "carries sankey hints and declares a \"ledger\" chart"},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := sidesOptions(t, t.TempDir(), c.form, c.hints, c.both)
			if c.edit != nil {
				c.edit(&o)
			}
			_, err := writeSite(o)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want a refusal naming %q", err, c.want)
			}
		})
	}
}
