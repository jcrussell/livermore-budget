package export_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
)

// TestDepartmentPageRendersItsChromeAndShipsItsModule holds the department view to what
// its template renders and to the client module it loads being written
// beside it.
//
// Mutation: drop "department.js" from verbatimAssets and the module goes unwritten,
// so the page's script tag names a file the site does not carry.
func TestDepartmentPageRendersItsChromeAndShipsItsModule(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeSite(export.Options{
		Dir:         dir,
		Projections: map[string][]byte{export.PrimaryProjection: goldenSankey(t)},
		Views: append(spineOnly(), export.View{Path: "department.html", Nav: "Department",
			Template: export.DepartmentTemplate, Title: "A department title", Lede: "A department lede."}),
		Docs: budgetDocs(),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	html := readFile(t, dir, "department.html")
	for _, want := range []string{
		"<h1>A department title</h1>",
		"A department lede.",
		`<script type="module" src="./department.js"></script>`,
		`<span aria-current="page">Department</span>`,
		`<a href="index.html">`,
		`id="sources-view"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("department.html does not carry %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "department.js")); err != nil {
		t.Errorf("department.html loads ./department.js and the site does not carry it: %v", err)
	}
}
