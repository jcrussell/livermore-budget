package geom

import (
	"fmt"
	"math"
)

// Span is a half-open x-interval (Lo, Hi], in points.
//
// Half-open on the left so that adjacent bands partition the page without a
// value ever belonging to two of them: a right edge landing exactly on a
// boundary belongs to the band on the left, which is the band whose column the
// figure is printed under.
type Span struct {
	Lo, Hi float64
}

// Contains reports whether x falls in the span.
func (s Span) Contains(x float64) bool { return x > s.Lo && x <= s.Hi }

// Grid is a page's column geometry: the x-band each column claims.
//
// A band runs from the midpoint of the gap before its header to the midpoint of
// the gap after it. That is a claim about the page's typesetting and not an
// arbitrary choice -- see the filing rule in internal/mapping, which is where
// the measurement lives, because deciding that a right edge identifies a column
// is a judgment about how Livermore typesets schedules and belongs in the
// judgment layer. This type only does the interval arithmetic.
type Grid struct {
	bands  []Span
	gutter float64
}

// NewGrid builds the band layout for one page from its column headers' x-extents
// and a gutter.
//
// The gutter is the lower bound of the first band, and it is not optional:
// column 0 has no natural left edge, and these pages print no header over the
// row-label area at all -- Budget Book p167's leftmost header is "FY" at
// x0=265.75 while its row labels run out to 250.5. Without an explicit lower
// bound every row label would file into column 0.
//
// Headers must be in printed order and must not overlap, because the caller's
// list is a claim about column ORDER: a page that dropped a column has to fail
// rather than shift every figure one place.
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
	// Both bounds are checked, and the lower one is the important half: a zero
	// gutter is the Go zero value, it would make band 0 start at the page's left
	// edge, and Index could then never return -1 at all -- silently disabling
	// the one guard that keeps row labels out of column 0. A caller that has no
	// label area has to say so with a real coordinate.
	if gutter <= 0 {
		return nil, fmt.Errorf("gutter is %g; it must be a positive x-coordinate, "+
			"because it is the lower bound of the first column and the right edge "+
			"of the row-label area", gutter)
	}
	if gutter > headers[0].Lo {
		return nil, fmt.Errorf("gutter %g is right of the first column header's left edge %g",
			gutter, headers[0].Lo)
	}

	// Each band ends where the next begins, so the bands partition the page
	// right of the gutter with no gap a value could fall into.
	bands := make([]Span, len(headers))
	lo := gutter
	for i, h := range headers {
		hi := math.Inf(1)
		if i+1 < len(headers) {
			hi = midpoint(h.Hi, headers[i+1].Lo)
		}
		bands[i] = Span{Lo: lo, Hi: hi}
		lo = hi
	}
	return &Grid{bands: bands, gutter: gutter}, nil
}

func midpoint(a, b float64) float64 { return (a + b) / 2 }

// Len is how many columns the grid has.
func (g *Grid) Len() int { return len(g.bands) }

// Band is column i's x-range.
func (g *Grid) Band(i int) Span { return g.bands[i] }

// Gutter is the right edge of the row-label area: the lower bound of column 0.
func (g *Grid) Gutter() float64 { return g.gutter }

// Index is the column an x-position falls in, or -1 for a position at or left of
// the gutter, which is the row-label area and belongs to no column.
//
// The LAST band has no upper bound, which is a real limit and not an oversight:
// nothing in the geometry says where a table stops, so a running footer's page
// number files into the last column exactly as a figure would. Excluding the
// footer remains the rule's stop_at anchor's job.
func (g *Grid) Index(x float64) int {
	if x <= g.gutter {
		return -1
	}
	for i, b := range g.bands {
		if b.Contains(x) {
			return i
		}
	}
	return -1
}
