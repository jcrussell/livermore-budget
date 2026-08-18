package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/geom"
)

// scriptedDoc is a hand-rolled doc, and the only one in this package: every
// other test builds a real *corpus.Doc over an fstest.MapFS (budgetDoc,
// inlineDoc), which is the right default because it exercises the manifest and
// the artifact check on the way in.
//
// This exists for the one thing a real document cannot do — answer differently
// each time it is asked. Whether the resolver reads a page or a part more than
// once is not observable by counting calls (this project asserts behaviour, not
// call counts, byob-testing.3); it is observable by making the second answer
// differ from the first and asking whether the caller can tell.
type scriptedDoc struct {
	docID string
	// script gives each page's successive reads, in order. The last entry
	// repeats once the script runs out, so a page with one entry behaves like
	// an ordinary document.
	script map[int][]pageRead
	calls  map[int]int
}

// pageRead is one scripted answer to doc.Page.
type pageRead struct {
	text string
	err  error
}

var _ doc = (*scriptedDoc)(nil)

func newScriptedDoc(docID string, script map[int][]pageRead) *scriptedDoc {
	return &scriptedDoc{docID: docID, script: script, calls: map[int]int{}}
}

func (d *scriptedDoc) DocID() string { return d.docID }

// Geometry is the third method of doc, and this double has none to give.
//
// That is not a gap: this document exists to answer the same page differently
// on successive reads, which is a question about memoization, and no part any
// of its tests resolve asks for the column guard. A part that did would fail
// here rather than resolve without a guard it believed it had, which is the
// behaviour the required method was chosen for.
func (*scriptedDoc) Geometry(n int) (*geom.Page, error) {
	return nil, fmt.Errorf("scriptedDoc carries no geometry for page %d", n)
}

func (d *scriptedDoc) Page(n int) (string, error) {
	reads, ok := d.script[n]
	if !ok || len(reads) == 0 {
		return "", fmt.Errorf("scriptedDoc has no page %d", n)
	}
	i := min(d.calls[n], len(reads)-1)
	d.calls[n]++
	return reads[i].text, reads[i].err
}

// scriptedResolver pairs a scripted document with a rule file written inline.
func scriptedResolver(t *testing.T, src string, script map[int][]pageRead) (*Resolver, *Rule) {
	t.Helper()
	f, err := Parse(strings.NewReader(src), "scripted.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(newScriptedDoc(f.DocID, script), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}
