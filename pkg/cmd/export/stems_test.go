package export

import (
	"path/filepath"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// TestEveryPublishedStemRoundTripsToItsSchedule holds the two spellings of
// the stem rule together: project.Stem appends a column suffix to a
// projection's name, and internal/export's column index strips it to recover
// the schedule a stem's document becomes in its column. The two live in
// packages that do not import each other, so this is the one place both are
// in scope. Every published graph document must fold into the column its own
// metadata names and be found there under its projection's name, and a series
// document must fold into none.
func TestEveryPublishedStemRoundTripsToItsSchedule(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildProjections(root)
	if err != nil {
		t.Fatalf("buildProjections: %v", err)
	}
	_, ix, err := export.ColumnsOf(built, "")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	graphs := 0
	for _, d := range project.PublishedDocuments() {
		column, folded := ix.Column(d.Stem)
		if len(d.Columns) != 1 {
			if folded {
				t.Errorf("%s spans %d columns and folded into %s; a series document is served as itself",
					d.Stem, len(d.Columns), column)
			}
			continue
		}
		if !folded {
			t.Errorf("%s is one column of %s and folded into none", d.Stem, d.Projection)
			continue
		}
		graphs++
		if want := export.ColumnPath(d.Columns[0].FiscalYear, string(d.Columns[0].Basis)); column != want {
			t.Errorf("%s folded into %s, and its declared column is %s", d.Stem, column, want)
		}
		stem, ok := ix.Stem(column, d.Projection)
		if !ok || stem != d.Stem {
			t.Errorf("column %s holds schedule %q as %q (%v); project.Stem named the document %q, "+
				"so the suffix one side appends is not the one the other strips",
				column, d.Projection, stem, ok, d.Stem)
		}
	}
	if graphs == 0 {
		t.Fatal("no published document is a single column, so nothing above round-tripped")
	}
}
