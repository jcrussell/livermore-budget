// Package geom reads the `pdftotext -bbox` word geometry for one extracted
// page. It knows nothing about the page text, amounts or rules, so
// internal/mapping can name its types without importing the artifact reader.
//
// Geometry answers "which column is this token in", and nothing else. It does
// NOT recover a value the PDF never put in its text layer: CIP p40's dashes
// are drawn as non-text (fisc-8ln). "Absent is not zero" is not settled here.
package geom

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

// schemaVersion is the geometry file's OWN version, not the manifest's
// corpus.SchemaVersion; the two version independently.
// TestSchemaVersionIsTheSchemasConst holds it to schema/geometry.schema.json.
const schemaVersion = 1

// Word is one word poppler placed on the page: its bounding box and its text.
// Coordinates are PDF points with the origin at the TOP-LEFT (y increases
// downward), rounded to 2dp by the extractor.
type Word struct {
	X0, Y0, X1, Y1 float64
	Text           string
}

// Right is the word's right edge, the coordinate that identifies a column:
// these schedules right-align their figures.
func (w Word) Right() float64 { return w.X1 }

// height is the word's vertical extent, the quantity Page.Lines derives its
// clustering tolerance from.
func (w Word) height() float64 { return w.Y1 - w.Y0 }

// UnmarshalJSON decodes the on-disk form, the flat array
// [x0, y0, x1, y1, "text"] rather than an object.
func (w *Word) UnmarshalJSON(b []byte) error {
	var row []json.RawMessage
	if err := json.Unmarshal(b, &row); err != nil {
		return fmt.Errorf("word is not an array: %w", err)
	}
	if len(row) != 5 {
		return fmt.Errorf("word has %d elements, want 5 ([x0,y0,x1,y1,text])", len(row))
	}
	// Decoded through pointers so a JSON null is refused: json.Unmarshal
	// leaves a float at zero on null, making a ghost word at the origin.
	for i, into := range []*float64{&w.X0, &w.Y0, &w.X1, &w.Y1} {
		var v *float64
		if err := json.Unmarshal(row[i], &v); err != nil {
			return fmt.Errorf("word coordinate %d: %w", i, err)
		}
		if v == nil {
			return fmt.Errorf("word coordinate %d is null", i)
		}
		*into = *v
	}
	var text *string
	if err := json.Unmarshal(row[4], &text); err != nil {
		return fmt.Errorf("word text: %w", err)
	}
	if text == nil {
		return fmt.Errorf("word text is null")
	}
	w.Text = *text
	return nil
}

// Page is the geometry of one extracted page.
type Page struct {
	// DocID and Number are the page's own account of which page it is; a
	// caller that asked for a specific page is expected to compare.
	DocID  string
	Number int
	// Width and Height are the page box in points. Orientation is per page,
	// not per document: the CIP is landscape on 75 of 323 pages and the ACFR
	// on 21 of 195.
	Width, Height float64
	// Words are in the extractor's order: sorted by (y0, x0, y1, x1, text).
	Words []Word
}

// wirePage is the on-disk shape. Unknown keys are tolerated; SchemaVersion
// guards the fields this package reads.
type wirePage struct {
	SchemaVersion int     `json:"schema_version"`
	DocID         string  `json:"doc_id"`
	Page          int     `json:"page"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	Words         []Word  `json:"words"`
}

// ParsePage decodes and validates one geometry artifact. The sort-order check
// matters most: Lines ASSUMES the extractor's (y0, x0, y1, x1, text) order and
// would return confident nonsense for an out-of-order page.
func ParsePage(b []byte) (*Page, error) {
	var wire wirePage
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("parse geometry: %w", err)
	}
	if wire.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("geometry schema_version %d, want %d",
			wire.SchemaVersion, schemaVersion)
	}
	if wire.DocID == "" {
		return nil, fmt.Errorf("geometry has no doc_id")
	}
	if wire.Page <= 0 {
		return nil, fmt.Errorf("geometry page is %d, want a positive page number", wire.Page)
	}
	for i, w := range wire.Words {
		if w.X1 < w.X0 || w.Y1 < w.Y0 {
			return nil, fmt.Errorf(
				"%s p%d word %d (%q) has an inverted box [%g %g %g %g]",
				wire.DocID, wire.Page, i, w.Text, w.X0, w.Y0, w.X1, w.Y1)
		}
		if i > 0 && lessWord(w, wire.Words[i-1]) {
			return nil, fmt.Errorf(
				"%s p%d word %d (%q) precedes word %d (%q) in reading order; "+
					"words must be sorted by (y0, x0, y1, x1, text)",
				wire.DocID, wire.Page, i, w.Text, i-1, wire.Words[i-1].Text)
		}
	}
	return &Page{DocID: wire.DocID, Number: wire.Page,
		Width: wire.Width, Height: wire.Height, Words: wire.Words}, nil
}

// lessWord reports whether a sorts before b in the extractor's order.
func lessWord(a, b Word) bool {
	switch {
	case a.Y0 != b.Y0:
		return a.Y0 < b.Y0
	case a.X0 != b.X0:
		return a.X0 < b.X0
	case a.Y1 != b.Y1:
		return a.Y1 < b.Y1
	case a.X1 != b.X1:
		return a.X1 < b.X1
	}
	return a.Text < b.Text
}

// MedianWordHeight is the median vertical extent of the page's words, and the
// quantity Lines halves to get its clustering tolerance. It is false for a
// page with no words, a real state: one ACFR page is blank.
func (p *Page) MedianWordHeight() (float64, bool) {
	if len(p.Words) == 0 {
		return 0, false
	}
	hs := make([]float64, len(p.Words))
	for i, w := range p.Words {
		hs[i] = w.height()
	}
	slices.Sort(hs)
	return hs[len(hs)/2], true
}

// Line is one printed line of the page: the words that share it, left to right.
type Line struct {
	Words []Word
}

// Lines groups the page's words into printed lines: a word joins the current
// line when its y0 is within half the median word height of the line's FIRST
// word. Over the mapped pages the worst intra-line spread is 0.96pt and the
// tightest inter-line gap 7.99pt, against tolerances near 4pt.
//
// This does NOT reproduce the -layout line structure corpus-wide (124 of 786
// pages disagree, mostly superscript footnote markers), so it must never
// decide alone: its consumer cross-checks it against the page text and
// refuses the page when they disagree. Recomputed on every call.
func (p *Page) Lines() []Line {
	tol, ok := p.MedianWordHeight()
	if !ok {
		return nil
	}
	tol /= 2

	var out []Line
	var cur []Word
	var top float64
	for _, w := range p.Words {
		if len(cur) > 0 && w.Y0-top > tol {
			out = append(out, newLine(cur))
			cur = nil
		}
		if len(cur) == 0 {
			top = w.Y0
		}
		cur = append(cur, w)
	}
	if len(cur) > 0 {
		out = append(out, newLine(cur))
	}
	return out
}

// newLine sorts a cluster into printed order. The re-sort is load-bearing:
// the artifact is y0-major, so within a jittered cluster file order is NOT
// left-to-right order.
func newLine(words []Word) Line {
	ws := slices.Clone(words)
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].X0 < ws[j].X0 })
	return Line{Words: ws}
}
