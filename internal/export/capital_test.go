package export_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// TestCapitalPageRendersItsChromeAndShipsItsModule holds the capital view to what
// its template renders and to the client module it loads being written
// beside it.
//
// Mutation: drop "capital.js" from verbatimAssets and the module goes unwritten,
// so the page's script tag names a file the site does not carry.
func TestCapitalPageRendersItsChromeAndShipsItsModule(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t)},
		Views: append(spineOnly(), export.View{Path: "capital.html", Nav: "Capital",
			Template: export.CapitalTemplate, Title: "A capital title", Lede: "A capital lede."}),
		Docs: budgetDocs(),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "capital.html")
	for _, want := range []string{
		"<h1>A capital title</h1>",
		"A capital lede.",
		`<script type="module" src="./capital.js"></script>`,
		`<span aria-current="page">Capital</span>`,
		`<a href="index.html">`,
		`id="sources-view"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("capital.html does not carry %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "capital.js")); err != nil {
		t.Errorf("capital.html loads ./capital.js and the site does not carry it: %v", err)
	}
}
