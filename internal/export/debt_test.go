package export_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// TestDebtPageRendersItsChromeAndShipsItsModule holds the debt view to what
// its template renders and to the client module it loads being written
// beside it.
//
// Mutation: drop "debt.js" from verbatimAssets and the module goes unwritten,
// so the page's script tag names a file the site does not carry.
func TestDebtPageRendersItsChromeAndShipsItsModule(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t)},
		Views: append(spineOnly(), export.View{Path: "debt.html", Nav: "Debt",
			Template: export.DebtTemplate, Title: "A debt title", Lede: "A debt lede."}),
		Docs: budgetDocs(),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "debt.html")
	for _, want := range []string{
		"<h1>A debt title</h1>",
		"A debt lede.",
		`<script type="module" src="./debt.js"></script>`,
		`<span aria-current="page">Debt</span>`,
		`<a href="index.html">`,
		`id="sources-view"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("debt.html does not carry %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "debt.js")); err != nil {
		t.Errorf("debt.html loads ./debt.js and the site does not carry it: %v", err)
	}
}
