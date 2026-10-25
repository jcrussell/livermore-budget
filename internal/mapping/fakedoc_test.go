package mapping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/geom"
)

// scriptedDoc answers the same page differently on successive reads, so a
// test can observe whether the resolver re-reads without counting calls.
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

// Geometry has nothing to give: a part asking for the column guard fails here
// rather than resolving unguarded.
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
	f, err := parse(strings.NewReader(src), "scripted.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := NewResolver(newScriptedDoc(f.DocID, script), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0]
}
