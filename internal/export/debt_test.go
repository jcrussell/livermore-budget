package export_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// debtFig is one figure of a debt fixture document.
func debtFig(cents int64, token string, page int) map[string]any {
	return map[string]any{"amount_cents": cents, "fact_id": "fisc-f-0000000000aa",
		"doc_id": budgetDocID, "page": page, "offset": 1, "token": token}
}

// debtDoc is a minimal debt-service document. Hand-written rather than built
// by internal/project, for historyDoc's reason: four issues in one column,
// one of them a printed dash, and two requirements rows, a year and a band,
// with one fund type printing nothing in the band. figures is the counts
// block's, or -1 for the true number.
func debtDoc(t *testing.T, figures int) []byte {
	t.Helper()
	through := 2035
	if figures < 0 {
		figures = 8 + 6
	}
	doc := map[string]any{
		"schema_version": 1, "projection": "debt-service",
		"metadata": map[string]any{
			"generated_by": "fisc test", "scopes": []string{"debt-service-by-issue", "debt-service-requirements"},
			"currency": "USD", "units": "cents",
			"columns": []map[string]any{{"fiscal_year": 2026, "fiscal_year_through": nil, "basis": "adopted"},
				{"fiscal_year": 2026, "fiscal_year_through": nil, "basis": "projected"},
				{"fiscal_year": 2031, "fiscal_year_through": through, "basis": "projected"}},
			"sources": []map[string]any{{"doc_id": budgetDocID, "pages": []int{80, 81}}},
			"counts":  map[string]any{"facts": figures, "figures": figures},
			"caveats": []map[string]any{{"id": "a-band-is-five-years", "summary": "A band is not a year.", "theme": "left-out",
				"text": "Five years in one figure.", "applies_to": []string{}}},
		},
		"issue_columns": []map[string]any{{"fiscal_year": 2026, "fiscal_year_label": "FY 2025-26", "basis": "adopted"}},
		"issues": []map[string]any{
			{"label": "2022 Certificates of Participation", "row_label": "2022 Certificates of Participation",
				"activity": "Governmental Activities", "payer": "",
				"cells": []map[string]any{{"principal": debtFig(73000000, "730,000.00", 81),
					"interest": debtFig(184805000, "1,848,050.00", 81)}}},
			{"label": "Interfund Loan", "row_label": "Interfund Loan",
				"activity": "Governmental Activities", "payer": "",
				"cells": []map[string]any{{"principal": debtFig(0, "-", 81), "interest": debtFig(0, "-", 81)}}},
			{"label": "Banc of America Equipment Lease/Purchase Loan", "row_label": "Banc of America Equipment Lease/Purchase Loan",
				"activity": "Business-Type Activities", "payer": "Airport",
				"cells": []map[string]any{{"principal": debtFig(3886400, "38,864.00", 81),
					"interest": debtFig(318500, "3,185.00", 81)}}},
			{"label": "2020B Certificates of Participation", "row_label": "Water\n2020B Certificates of Participation",
				"activity": "Business-Type Activities", "payer": "Water",
				"cells": []map[string]any{{"principal": debtFig(27688400, "276,884.00", 81),
					"interest": debtFig(6803600, "68,036.00", 81)}}},
		},
		"requirements": map[string]any{
			"fund_types": []map[string]any{{"key": "governmental", "label": "Governmental Funds", "fund_group": "", "fund": nil},
				{"key": "internal-service", "label": "Internal Service Funds", "fund_group": "internal-service", "fund": nil}},
			"periods": []map[string]any{
				{"fiscal_year": 2026, "fiscal_year_through": nil, "label": "2026", "years": 1,
					"cells": []map[string]any{
						{"principal": debtFig(405449800, "4,054,498", 80), "interest": debtFig(304656900, "3,046,569", 80)},
						{"principal": debtFig(98928400, "989,284", 80), "interest": debtFig(8107300, "81,073", 80)}}},
				{"fiscal_year": 2031, "fiscal_year_through": through, "label": "2031-2035", "years": 5,
					"cells": []map[string]any{
						{"principal": debtFig(1252166100, "12,521,661", 80), "interest": debtFig(956319100, "9,563,191", 80)},
						{"principal": nil, "interest": nil}}},
			},
			"totals": []map[string]any{
				{"fund_type": "governmental", "principal_cents": 1657615900, "interest_cents": 1260976000, "fact_ids": []string{}},
				{"fund_type": "internal-service", "principal_cents": 98928400, "interest_cents": 8107300, "fact_ids": []string{}},
			},
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func debtView() export.View {
	return export.View{Path: "debt.html", Nav: "Debt", Template: export.DebtTemplate,
		Projection: "debt-service", Title: "A debt title", Lede: "A debt lede."}
}

// TestDebtPageRendersEveryFigureOfItsDocument holds the debt view to its
// document: each figure links to its page, a printed dash renders as a dash
// and is a figure, a blank cell renders as not printed and is not a zero, a
// band says it is a band.
//
// Mutation: render a nil figure as "$0" and "not printed" vanishes.
func TestDebtPageRendersEveryFigureOfItsDocument(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir: dir,
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t),
			"debt-service": debtDoc(t, -1)},
		Views: append(spineOnly(), debtView()),
		Docs:  budgetDocs(),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "debt.html")
	for _, want := range []string{
		"<h1>A debt title</h1>",
		"A debt lede.",
		`<span aria-current="page">Debt</span>`,
		`id="sources-view"`,
		"$730,000",
		"$1,848,050",
		"$12,521,661",
		`title="Printed as a dash, which is zero; read on page 81">–</a>`,
		`<span title="The page prints no figure here">not printed</span>`,
		`class="debt-band"`,
		"5 years in one figure",
		`Banc of America Equipment Lease/Purchase Loan <span class="subtle">· Airport</span>`,
		"$16,576,159",
		"A band is not a year.",
		"debt-service.json",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("debt.html does not carry %q", want)
		}
	}
	if strings.Contains(html, "publishes no figure yet") {
		t.Error("debt.html still says it publishes no figure")
	}
	if n := strings.Count(html, "not printed</span>"); n != 2 {
		t.Errorf("debt.html renders %d unprinted cells, want the band's 2", n)
	}
}

// TestDebtPageRefusesADocumentThatDoesNotReconcile: a document whose figure
// count is not the figures it lays out would put a page in front of a
// reader with a figure missing and nothing saying so.
//
// Mutation: drop the count comparison in buildDebtPage and this builds.
func TestDebtPageRefusesADocumentThatDoesNotReconcile(t *testing.T) {
	_, err := writeSite(export.Options{
		Dir: t.TempDir(),
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t),
			"debt-service": debtDoc(t, 15)},
		Views: append(spineOnly(), debtView()),
		Docs:  budgetDocs(),
	})
	if err == nil || !strings.Contains(err.Error(), "publishes 15 figures and the page shows 14") {
		t.Fatalf("got %v, want the count refusal", err)
	}
}

// TestDebtPageRefusesAFigureOfAnUndescribedDocument: a figure citing a
// document the registry does not describe refuses, rather than publishing a
// cell whose link names no PDF.
//
// Mutation: compose the figure's link with sourcesFor in place of
// cellCites.page, and the page writes.
func TestDebtPageRefusesAFigureOfAnUndescribedDocument(t *testing.T) {
	cited := `"doc_id":"` + budgetDocID + `","fact_id"`
	raw := strings.Replace(string(debtDoc(t, -1)), cited, `"doc_id":"gone","fact_id"`, 1)
	if raw == string(debtDoc(t, -1)) {
		t.Fatal("the fixture carries no figure to re-cite")
	}
	_, err := writeSite(export.Options{
		Dir: t.TempDir(),
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t),
			"debt-service": []byte(raw)},
		Views: append(spineOnly(), debtView()),
		Docs:  budgetDocs(),
	})
	if err == nil || !strings.Contains(err.Error(), "gone p81") {
		t.Fatalf("got %v, want a refusal naming gone p81", err)
	}
}
