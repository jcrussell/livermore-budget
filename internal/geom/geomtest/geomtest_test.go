package geomtest_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/geom"
	"github.com/jcrussell/livermore-budget/internal/geom/geomtest"
)

// TestMonospacedIsAPageTheParserAccepts holds Monospaced to geom.ParsePage,
// whose validation includes the extractor's word order: a page whose words
// came out of order would be refused here rather than read as nonsense by a
// test that trusted it. Mutation: emit each line's words right to left, and
// ParsePage refuses the page.
func TestMonospacedIsAPageTheParserAccepts(t *testing.T) {
	p, err := geom.ParsePage([]byte(geomtest.Monospaced("doc", 7, "Total  $ 1,234\n  Fees      12")))
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	var got []string
	for _, w := range p.Words {
		got = append(got, w.Text)
	}
	if diff := cmp.Diff([]string{"Total", "$", "1,234", "Fees", "12"}, got); diff != "" {
		t.Errorf("words (-want +got):\n%s", diff)
	}
	if p.DocID != "doc" || p.Number != 7 {
		t.Errorf("page = %s p%d, want doc p7", p.DocID, p.Number)
	}
	if w := p.Words[2]; w.X0 != 54 || w.X1 != 84 || w.Y0 != 0 || w.Y1 != 10 {
		t.Errorf("1,234 box = %v, want x 54-84, y 0-10", w)
	}
}
