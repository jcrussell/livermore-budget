// Package geom reads the `pdftotext -bbox` word geometry for one extracted
// page.
//
// It is the read side of the second substrate, and it knows nothing about the
// first: no page text, no tokenizer, no amounts, no rules. That boundary is
// what lets its whole test suite be arithmetic over numbers, and it is why this
// package exists at all rather than the geometry types living in
// internal/corpus — internal/mapping has to NAME them without importing the
// artifact reader, which would put the artifact reader above the judgment
// layer.
//
// What geometry answers is "which column is this token in", and nothing else.
// It does NOT recover a value the PDF never put in its text layer: CIP p40
// rows PB200654 and PB202617 print "-" in their intervening columns and those
// dashes appear in neither substrate, because they are drawn as non-text
// (fisc-8ln). "Absent is not zero" is not settled here.
package geom

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

// SchemaVersion is the geometry-file version this package understands.
//
// It is the geometry file's OWN version, stamped by tools/extract.py, and is
// deliberately not the manifest's corpus.SchemaVersion: the two artifacts
// version independently, and a reader pinned to the wrong one would report a
// perfectly good page as unreadable.
const SchemaVersion = 1

// Word is one word poppler placed on the page: its bounding box and its text.
//
// Coordinates are PDF user-space points with the origin at the TOP-LEFT, so y
// increases downward. They are rounded to 2dp by the extractor, because poppler
// prints six decimals of a font-metric computation that is not stable across
// builds.
type Word struct {
	X0, Y0, X1, Y1 float64
	Text           string
}

// Right is the word's right edge, named so that the filing rule reads as prose
// at its call site. These schedules right-align their figures, so the right
// edge is the coordinate that identifies a column; the left edge varies with
// the width of the number.
func (w Word) Right() float64 { return w.X1 }

// Height is the word's vertical extent, the quantity Page.Lines derives its
// clustering tolerance from.
func (w Word) Height() float64 { return w.Y1 - w.Y0 }

// UnmarshalJSON decodes the on-disk form, which is the flat array
// [x0, y0, x1, y1, "text"] rather than an object.
//
// The extractor made it flat deliberately: at 208k words in this corpus the key
// names would outweigh the values. That means struct tags cannot decode it and
// this method has to exist.
func (w *Word) UnmarshalJSON(b []byte) error {
	var row []json.RawMessage
	if err := json.Unmarshal(b, &row); err != nil {
		return fmt.Errorf("word is not an array: %w", err)
	}
	if len(row) != 5 {
		return fmt.Errorf("word has %d elements, want 5 ([x0,y0,x1,y1,text])", len(row))
	}
	// Decoded through pointers so that a JSON null is refused rather than
	// silently left at zero. json.Unmarshal into a *float64 treats null as a
	// no-op with a nil error, which would turn [null,null,null,null,"$"] into a
	// zero-area box at the page origin -- a ghost word that passes the
	// inverted-box check, sorts to the front of the page, anchors the first
	// line cluster, and drags the median word height down.
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
	// DocID and Number are the page's own account of which page it is. A
	// caller that asked for a specific page is expected to compare, because an
	// artifact copied between extraction directories is otherwise a
	// self-consistent lie.
	DocID  string
	Number int
	// Width and Height are the page box in points. The Budget Book is portrait
	// Letter (612x792); the CIP is landscape (792x612).
	Width, Height float64
	// Words are in the extractor's order: sorted by (y0, x0, y1, x1, text).
	Words []Word
}

// wirePage is the on-disk shape. Unknown keys are tolerated for the reason
// internal/corpus tolerates them in the manifest: the extractor may add
// reporting keys, and SchemaVersion is the guard that matters for the fields
// this package actually reads.
type wirePage struct {
	SchemaVersion int     `json:"schema_version"`
	DocID         string  `json:"doc_id"`
	Page          int     `json:"page"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	Words         []Word  `json:"words"`
}

// ParsePage decodes and validates one geometry artifact.
//
// It validates rather than merely unmarshalling, and the sort-order check is
// the one that earns its keep: Lines is a greedy sweep that ASSUMES the
// extractor's (y0, x0, y1, x1, text) ordering, and handed an out-of-order page
// it would return confident nonsense rather than an error. A hand-written test
// fixture is exactly how an out-of-order page gets in. All 786 committed
// artifacts pass every check here with zero violations.
func ParsePage(b []byte) (*Page, error) {
	var wire wirePage
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("parse geometry: %w", err)
	}
	if wire.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("geometry schema_version %d, want %d",
			wire.SchemaVersion, SchemaVersion)
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
// quantity Lines halves to get its clustering tolerance.
//
// The second result is false for a page with no words at all, which is a real
// state rather than a defensive branch: one of the ACFR's 195 pages is blank
// and still HAS both artifacts, an empty pages/pNNNN.txt beside a geometry file
// with an empty words array.
func (p *Page) MedianWordHeight() (float64, bool) {
	if len(p.Words) == 0 {
		return 0, false
	}
	hs := make([]float64, len(p.Words))
	for i, w := range p.Words {
		hs[i] = w.Height()
	}
	slices.Sort(hs)
	return hs[len(hs)/2], true
}

// Line is one printed line of the page: the words that share it, left to right.
type Line struct {
	Words []Word
}

// Lines groups the page's words into printed lines.
//
// The tolerance is half the median word height, and a word joins the current
// line when its y0 is within that of the line's FIRST word. Measured over the
// pages this project maps, the margins on both sides are wide: the worst
// intra-line y0 spread is 0.96pt against a 4.55pt tolerance (Budget Book p66,
// and that one cluster is a mixed label/header line -- excluding it the worst
// data-row spread is 0.12pt), while the tightest inter-line gap is 7.99pt
// against a 4.16pt tolerance (pp.167 and 169, a wrapped department name against
// the data row below it).
//
// This does NOT reproduce the -layout line structure corpus-wide, and the
// difference is not marginal: on 124 of 786 pages the two disagree on how many
// lines the page has, and on Budget Book pp.68/70/186/190/192/194 -- financial
// schedules one page from the mapped spine -- the cause is a superscript
// footnote marker that -layout puts on its own line and geometry clusters into
// the row. So this grouping must never decide alone. Its consumer cross-checks
// it against the page text and refuses the page when they disagree.
//
// The result is recomputed on every call; callers that need it repeatedly are
// expected to hold on to it.
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

// newLine sorts a cluster into printed order.
//
// The re-sort is load-bearing rather than tidy. The artifact is sorted y0-major,
// so within a cluster whose members differ in y0 by less than the tolerance --
// which is the whole reason a tolerance exists -- file order is NOT left-to-right
// order. A consumer comparing this line against the page text token by token
// would disagree on exactly the jittered lines the tolerance was for.
func newLine(words []Word) Line {
	ws := slices.Clone(words)
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].X0 < ws[j].X0 })
	return Line{Words: ws}
}
