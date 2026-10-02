package geom

import (
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// spineGrid is Budget Book p67's own layout, to the hundredth of a point: the
// eight "FY 2025-26 / FY 2026-27" headers and the gutter this project derives
// from the first of them. Using the real numbers rather than round ones keeps
// the boundary cases honest -- the boundaries below are the ones the resolver
// will actually compute.
func spineGrid(t *testing.T) *Grid {
	t.Helper()
	headers := []Span{
		{Lo: 60.72, Hi: 105.13}, {Lo: 123.55, Hi: 167.96},
		{Lo: 188.17, Hi: 232.58}, {Lo: 250.98, Hi: 295.39},
		{Lo: 314.71, Hi: 359.12}, {Lo: 379.32, Hi: 423.73},
		{Lo: 443.04, Hi: 487.45}, {Lo: 505.87, Hi: 550.28},
	}
	g, err := NewGrid(headers, headers[0].Lo)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	return g
}

func TestGridBandsEndAtTheNextHeader(t *testing.T) {
	g := spineGrid(t)

	if got, want := g.Len(), 8; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	// Band 0 starts at the gutter, the last band runs to infinity, and every
	// interior boundary is the next header's left edge.
	if got, want := g.band(0).Lo, 60.72; got != want {
		t.Errorf("Band(0).Lo = %v, want %v", got, want)
	}
	if got := g.band(7).Hi; !math.IsInf(got, 1) {
		t.Errorf("Band(7).Hi = %v, want +Inf", got)
	}
	if got, want := g.band(0).Hi, 123.55; got != want {
		t.Errorf("Band(0).Hi = %v, want %v", got, want)
	}
	for i := 1; i < g.Len(); i++ {
		if got, want := g.band(i).Lo, g.band(i-1).Hi; got != want {
			t.Errorf("Band(%d).Lo = %v, want Band(%d).Hi = %v", i, got, i-1, want)
		}
	}
}

func TestGridIndexBoundaries(t *testing.T) {
	g := spineGrid(t)
	firstBoundary := 123.55 // the second header's left edge

	tests := []struct {
		name string
		x    float64
		want int
	}{
		{"left of the gutter is the label area", 56.0, -1},
		{"exactly on the gutter is the label area", 60.72, -1},
		{"just right of the gutter is column 1", 60.73, 0},
		// Half-open on the left: a right edge landing exactly on a boundary
		// belongs to the column it is printed under, not the one after.
		{"exactly on a band boundary is the earlier column", firstBoundary, 0},
		{"just past a band boundary is the later column", math.Nextafter(firstBoundary, 1e9), 1},
		{"a figure right-aligned past its header", 114.0, 0},
		// p187's overhang: past the gap's midpoint (114.34) and short of the
		// next header, still under its own.
		{"a figure right-aligned past the gap's midpoint", 120.0, 0},
		{"the last column has no upper bound", 1e6, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.Index(tt.x); got != tt.want {
				t.Errorf("Index(%v) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}

// TestGridIndexPlacesARightAlignedColumn states the measurement the whole guard
// rests on, in miniature. Budget Book p167 prints its "FY 2025-26" header at
// 421.99-469.01 while every figure under it ends at 477.8-477.9 -- a consistent
// +8.8 to +9.0pt, because the figures are right-aligned to a grid offset RIGHT
// of the header text. A rule that filed a value by whether it OVERLAPPED its
// header would place none of these.
func TestGridIndexPlacesARightAlignedColumn(t *testing.T) {
	headers := []Span{{Lo: 343.87, Hi: 391.02}, {Lo: 421.99, Hi: 469.01}}
	g, err := NewGrid(headers, headers[0].Lo)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	for _, right := range []float64{477.8, 477.85, 477.9} {
		if got, want := g.Index(right), 1; got != want {
			t.Errorf("Index(%v) = %d, want %d (the figure is printed under header 2)",
				right, got, want)
		}
		if headers[1].contains(right) {
			t.Errorf("%v overlaps its own header, which would make this test vacuous", right)
		}
	}
}

func TestNewGridRefusals(t *testing.T) {
	tests := []struct {
		name    string
		headers []Span
		gutter  float64
		want    string
	}{
		{"no headers", nil, 0, "at least one column header"},
		{"inverted header", []Span{{Lo: 20, Hi: 10}}, 0, "which is inverted"},
		{
			// The list is a claim about column order. A page that dropped a
			// column must fail rather than shift every figure one place.
			name:    "headers out of order",
			headers: []Span{{Lo: 100, Hi: 150}, {Lo: 20, Hi: 60}},
			gutter:  100,
			want:    "not right of header 1",
		},
		{
			name:    "overlapping headers",
			headers: []Span{{Lo: 100, Hi: 150}, {Lo: 140, Hi: 200}},
			gutter:  100,
			want:    "not right of header 1",
		},
		{
			name:    "gutter right of the first header",
			headers: []Span{{Lo: 100, Hi: 150}},
			gutter:  120,
			want:    "right of the first column header",
		},
		{
			// The Go zero value. Accepting it would start band 0 at the page's
			// left edge and make Index unable to ever return -1, which silently
			// deletes the guard that keeps row labels out of column 0.
			name:    "no gutter at all",
			headers: []Span{{Lo: 265.75, Hi: 277.25}, {Lo: 343.87, Hi: 391.02}},
			gutter:  0,
			want:    "must be a positive x-coordinate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGrid(tt.headers, tt.gutter)
			if err == nil {
				t.Fatalf("NewGrid(%v, %v) = nil error, want one mentioning %q",
					tt.headers, tt.gutter, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("NewGrid error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestSingleColumnGridClaimsEverythingRightOfTheGutter(t *testing.T) {
	g, err := NewGrid([]Span{{Lo: 100, Hi: 150}}, 100)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	want := []Span{{Lo: 100, Hi: math.Inf(1)}}
	if diff := cmp.Diff(want, []Span{g.band(0)}); diff != "" {
		t.Errorf("bands (-want +got):\n%s", diff)
	}
	if got := g.Index(99); got != -1 {
		t.Errorf("Index(99) = %d, want -1", got)
	}
	if got := g.Index(101); got != 0 {
		t.Errorf("Index(101) = %d, want 0", got)
	}
}
