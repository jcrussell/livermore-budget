package export

import "fmt"

// The mark's coordinate system, in SVG user units (not pixels; the stylesheet
// sizes the <svg>). All geometry is integer arithmetic over int64 cents, so the
// markup is byte-identical across runs and platforms.
const (
	markBarWidth = 9
	markBarGap   = 3
	markHeight   = 30
	// markZeroRadius is the dot a PUBLISHED zero (a printed dash) gets. A dot,
	// not a short bar: one viewBox unit is sub-pixel on the shipped page, so only
	// a difference in kind keeps a zero apart from a tiny figure.
	markZeroRadius = 2
	// markMinBar is the height of a non-zero figure whose share of the row's
	// largest rounds to nothing, so a real figure never draws as "no money".
	markMinBar = 2
)

// markRef is one row's columns drawn as bars. It draws only figures the row
// already cites and computes no total, growth or share, so it publishes nothing
// needing a provenance pointer of its own.
type markRef struct {
	// Width is the viewBox width: one bar slot per document column.
	Width int
	// Height is the viewBox height, published here so the template holds no
	// geometry of its own.
	Height int
	// Rules are the boundaries between comparable groups, at the same columns
	// the table heads with a rule.
	Rules []int
	// Baseline is where zero sits: the bottom, or mid-box for a row with a
	// contra cell.
	Baseline int
	Bars     []barRef
	// Label is the mark's accessible name. The bars are aria-hidden beneath it
	// because every figure they draw is already in the row as linked text.
	Label string
}

// barRef is one column's figure as a rectangle.
type barRef struct {
	X, Y, Width, Height int
	// Zero marks a PUBLISHED zero, drawn as a dot on the baseline. Absent is
	// not zero (AGENTS.md, Provenance invariants): an absent column has no
	// barRef, a zero is a dot, the smallest real figure is markMinBar high.
	Zero bool
	// CX and R place the zero dot. They are ignored when Zero is false.
	CX, R int
	// Negative marks a contra cell for the stylesheet.
	Negative bool
	// Title is the native SVG tooltip: the column and the figure.
	Title string
}

// buildMark draws one row's cells as bars, scaled to that row's own largest
// magnitude. The scale is per row, so marks are not comparable across rows and
// the caption must say so. Heights are geometry, not money: no figure is
// computed here.
func buildMark(cells []cellRef, cols []columnRef) markRef {
	var largest int64
	for _, c := range cells {
		if c.Missing {
			continue
		}
		if m := abs64(c.Cents); m > largest {
			largest = m
		}
	}

	negative := false
	for _, c := range cells {
		if !c.Missing && c.Cents < 0 {
			negative = true
			break
		}
	}
	up, baseline := markHeight, markHeight
	if negative {
		up, baseline = markHeight/2, markHeight/2
	}

	m := markRef{
		Width:    len(cols)*markBarWidth + max(len(cols)-1, 0)*markBarGap,
		Height:   markHeight,
		Baseline: baseline,
	}
	// A rule sits mid-gap before a column that starts a new comparable group;
	// columnRef.New is never set on the first column.
	for i, c := range cols {
		if c.New && i > 0 {
			m.Rules = append(m.Rules, i*(markBarWidth+markBarGap)-markBarGap/2-1)
		}
	}
	drawn := 0
	for i, c := range cells {
		if c.Missing {
			// Absent: no bar, the slot stays empty.
			continue
		}
		drawn++
		x := i * (markBarWidth + markBarGap)
		bar := barRef{
			X: x, Width: markBarWidth, Negative: c.Cents < 0,
			Title: cols[i].Label + " " + cols[i].Basis + ": " + c.Value,
		}
		switch {
		case c.Cents == 0:
			bar.Zero = true
			bar.CX, bar.Y, bar.R = x+markBarWidth/2, baseline, markZeroRadius
		case largest == 0:
			// Unreachable while any cell is non-zero; guards the division.
			bar.Zero = true
			bar.CX, bar.Y, bar.R = x+markBarWidth/2, baseline, markZeroRadius
		default:
			h := int(abs64(c.Cents) * int64(up) / largest)
			if h < markMinBar {
				h = markMinBar
			}
			if c.Cents < 0 {
				bar.Y, bar.Height = baseline, h
			} else {
				bar.Y, bar.Height = baseline-h, h
			}
		}
		m.Bars = append(m.Bars, bar)
	}
	m.Label = markLabel(drawn, len(cols))
	return m
}

// markLabel is the mark's accessible name. It says what the mark is rather
// than repeating the row's figures, which a screen reader reads as text.
func markLabel(drawn, columns int) string {
	if drawn == columns {
		return fmt.Sprintf("This row's %d figures, drawn to the row's own scale", columns)
	}
	return fmt.Sprintf("%d of this row's %d columns carry a figure, drawn to the row's own scale",
		drawn, columns)
}

// abs64 is the magnitude of a signed cents value. It does not guard
// math.MinInt64, which internal/amount cannot produce.
func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
