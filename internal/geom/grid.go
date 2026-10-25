package geom

import (
	"fmt"
	"math"
)

// Span is a half-open x-interval (Lo, Hi], in points, so adjacent bands
// partition the page: a right edge on a boundary belongs to the band on the
// left, the column the figure is printed under.
type Span struct {
	Lo, Hi float64
}

// contains reports whether x falls in the span.
func (s Span) contains(x float64) bool { return x > s.Lo && x <= s.Hi }

// Grid is a page's column geometry: the x-band each column claims, from its
// header's left edge to the next header's left edge (the first from the
// gutter, the last without bound). Figures right-align under or past their
// header's right edge; Budget Book p187 and p207 overhang past the midpoint of
// a narrow gap, so a band ending at the midpoint would misfile them. Deciding
// that a right edge identifies a column is internal/mapping's judgment.
type Grid struct {
	bands  []Span
	gutter float64
}

// NewGrid builds the band layout for one page from its column headers' x-extents
// and a gutter, the first band's lower bound: no header sits over the
// row-label area (Budget Book p167's labels run to 250.5, its first header at
// 265.75), so without it every row label files into column 0. Headers must be
// in printed order and not overlap, so a page that dropped a column fails
// rather than shifting every figure one place.
func NewGrid(headers []Span, gutter float64) (*Grid, error) {
	if len(headers) == 0 {
		return nil, fmt.Errorf("a grid needs at least one column header")
	}
	prev := Span{Lo: math.Inf(-1), Hi: math.Inf(-1)}
	for i, h := range headers {
		if h.Hi < h.Lo {
			return nil, fmt.Errorf("column header %d spans [%g %g], which is inverted",
				i+1, h.Lo, h.Hi)
		}
		if h.Lo <= prev.Hi {
			return nil, fmt.Errorf(
				"column header %d starts at %g, which is not right of header %d ending at %g",
				i+1, h.Lo, i, prev.Hi)
		}
		prev = h
	}
	// A zero gutter, the Go zero value, would mean Index never returns -1 and
	// row labels file into column 0.
	if gutter <= 0 {
		return nil, fmt.Errorf("gutter is %g; it must be a positive x-coordinate, "+
			"because it is the lower bound of the first column and the right edge "+
			"of the row-label area", gutter)
	}
	if gutter > headers[0].Lo {
		return nil, fmt.Errorf("gutter %g is right of the first column header's left edge %g",
			gutter, headers[0].Lo)
	}

	// Each band ends where the next begins: no gap a value could fall into.
	bands := make([]Span, len(headers))
	lo := gutter
	for i := range headers {
		hi := math.Inf(1)
		if i+1 < len(headers) {
			hi = headers[i+1].Lo
		}
		bands[i] = Span{Lo: lo, Hi: hi}
		lo = hi
	}
	return &Grid{bands: bands, gutter: gutter}, nil
}

// Len is how many columns the grid has.
func (g *Grid) Len() int { return len(g.bands) }

// Index is the column an x-position falls in, or -1 for a position at or left of
// the gutter (the row-label area). The LAST band is unbounded, so a footer's
// page number files into it; excluding the footer is the rule's stop_at job.
func (g *Grid) Index(x float64) int {
	if x <= g.gutter {
		return -1
	}
	for i, b := range g.bands {
		if b.contains(x) {
			return i
		}
	}
	return -1
}
