package export

import "fmt"

// The mark's coordinate system, in SVG user units.
//
// INTEGERS, AND THE VIEWBOX IS FIXED. Every bar's geometry is computed from
// int64 cents with integer arithmetic and rendered as a whole number, so two
// runs of `fisc export` produce byte-identical markup — a float here would put
// 12.333333333333334 in the output and make the page's bytes depend on the
// platform's formatting. It is also the amounts-are-integer-cents discipline
// holding one step further than the store: nothing on the path from a fact to a
// pixel is a float either.
//
// The units are not pixels. The stylesheet gives the <svg> its rendered size
// and the viewBox scales to it, so the mark follows the table's type size
// without any of these numbers changing.
const (
	markBarWidth = 9
	markBarGap   = 3
	markHeight   = 30
	// markZeroRadius is the dot a PUBLISHED zero gets — a dash the city printed,
	// which is a figure and is therefore drawn.
	//
	// A DOT AND NOT A SHORT BAR, because a size difference is not a difference
	// at this scale. Measured on the shipped page: ten real figures
	// ($500,000, $74,748, $55,380…) rendered at exactly the height of the 165
	// published-zero ticks, and at 1.6em over 13px type one viewBox unit was
	// 0.69 CSS px — so the two were sub-pixel and told apart only by fill. A
	// circle against a rectangle is a difference in KIND, legible at any size
	// and in greyscale, and it cannot be eroded by rounding.
	markZeroRadius = 2
	// markMinBar is what a non-zero figure gets when its share of the row's
	// largest rounds to nothing. A row whose FY2023-24 actual is 0.3% of its
	// FY2026-27 adopted has a real figure that integer division puts at zero
	// height, and a bar that is not there says "no money" where the page says
	// $47,000.
	//
	// IT IS TWO AND NOT ONE, so that a tiny figure and a published zero are
	// never the same rectangle. At one unit they were, for the reason
	// markZeroRadius records. Absent is not zero, and a rounding-error figure is
	// neither.
	markMinBar = 2
)

// markRef is one row's four columns drawn as bars.
//
// IT DRAWS ONLY FIGURES THE ROW ALREADY CITES, and that is what keeps it on the
// right side of this project's central invariant. Every bar is one printed cell
// of this row, rendered beside the linked figure it is; the mark computes no
// total, no growth and no share, so it publishes nothing that could need a
// provenance pointer of its own. A chart of fund-group totals would have needed
// one for each of 28 sums, and there is no page to point at.
type markRef struct {
	// Width is the viewBox width: as many bar slots as the document has
	// columns, so a document of three columns is not a four-column mark with a
	// gap on the end.
	Width int
	// Height is the viewBox height. It is published rather than left to the
	// template because the template had it hardcoded while Width and Baseline
	// came from here, so raising markHeight clipped every bar and no test
	// noticed — a geometry split across two files where only one of them is
	// checked.
	Height int
	// Rules are the boundaries between comparable groups, at the same columns
	// the table heads with a rule. Two of these four columns are money that
	// moved and two are an intention, and a reader running an eye along a mark
	// has to meet the same warning they meet along the row.
	Rules []int
	// Baseline is where zero sits. A row with no negative figure gets the full
	// height for its positives; a row with a contra cell splits the box, so the
	// two directions are legible against each other rather than one being
	// clipped.
	Baseline int
	Bars     []barRef
	// Label is the mark's accessible name. The bars are aria-hidden beneath it
	// because every figure they draw is already in this row as linked text: a
	// screen reader that walked them would hear each number twice and reach 924
	// extra tab stops on the way, which is a worse page for the reader it was
	// added for.
	Label string
}

// barRef is one column's figure as a rectangle.
type barRef struct {
	X, Y, Width, Height int
	// Zero marks a PUBLISHED zero — a printed dash, which is a figure — drawn
	// as a DOT on the baseline rather than as a rectangle. Absent is not zero
	// (AGENTS.md, Provenance invariants) and the three states must not look alike: a
	// column this row does not carry produces no barRef at all and the slot is
	// empty, a published zero is a dot, and the smallest real figure is a
	// rectangle markMinBar high.
	Zero bool
	// CX and R place the zero dot. They are ignored when Zero is false.
	CX, R int
	// Negative marks a contra row, so the stylesheet can tell the two
	// directions apart without the reader inferring it from position.
	Negative bool
	// Title is the native SVG tooltip: the column and the figure, so a bar is
	// identifiable on hover without the mark carrying visible labels it has no
	// room for.
	Title string
}

// buildMark draws one row's cells as bars, scaled to that row's own largest
// figure.
//
// THE SCALE IS PER ROW, AND THE CAPTION HAS TO SAY SO. These 231 rows span
// $47,000 to $58,000,000, so one scale across the table renders all but a
// handful of rows as an invisible line — a mark that shows nothing is not a
// conservative choice, it is a blank column pretending to be information. Per
// row, every mark is legible and none is comparable with its neighbour, which
// is a real limit and belongs in the words beside it rather than in a comment
// only a maintainer reads.
//
// It is also the honest scale for what this document IS. The four columns are
// not one measurement — an actual, a re-forecast, and two years of one adopted
// budget — so the comparison the mark supports is the one WITHIN a row, which
// is the only one the contract permits at all.
//
// NO FIGURE IS COMPUTED HERE. The bars' heights are geometry, not money: the
// only arithmetic is a proportion of a box, and the numbers a reader takes away
// are the linked figures in the same row. internal/export may not recompute a
// figure a document publishes, and this does not.
func buildMark(cells []cellRef, cols []columnRef) markRef {
	// The row's largest magnitude, which is the only quantity the scale needs.
	// Magnitude and not value, so a contra row scales against its own biggest
	// number rather than against zero.
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
	// A row with a contra cell splits the box; one without gives its whole
	// height to the positives.
	up, baseline := markHeight, markHeight
	if negative {
		up, baseline = markHeight/2, markHeight/2
	}

	m := markRef{
		Width:    len(cols)*markBarWidth + max(len(cols)-1, 0)*markBarGap,
		Height:   markHeight,
		Baseline: baseline,
	}
	// The rule sits in the middle of the gap BEFORE a column that starts a new
	// comparable group, which is where the table's own border-left falls. The
	// first column can never carry one -- there is nothing to its left to be
	// separated from -- and columnRef.New is false there, so no guard is needed
	// beyond reading the flag the header already reads.
	for i, c := range cols {
		if c.New && i > 0 {
			m.Rules = append(m.Rules, i*(markBarWidth+markBarGap)-markBarGap/2-1)
		}
	}
	drawn := 0
	for i, c := range cells {
		if c.Missing {
			// No bar at all. The slot stays empty, which is how an absence is
			// told from a published zero further down.
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
			// A printed dash is a figure the city published, so it is drawn --
			// as a dot ON the baseline, which is what zero looks like.
			bar.Zero = true
			bar.CX, bar.Y, bar.R = x+markBarWidth/2, baseline, markZeroRadius
		case largest == 0:
			// Unreachable while any cell is non-zero, and it is here so that a
			// row of published zeros cannot divide by zero on the way to
			// drawing nothing.
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

// markLabel is the mark's accessible name.
//
// It says what the mark IS rather than what it shows, because what it shows is
// the row's own figures and a screen reader is about to read them as text. A
// label repeating four dollar amounts would be the same information twice, and
// the second time without the column headers that make it meaningful.
func markLabel(drawn, columns int) string {
	if drawn == columns {
		return fmt.Sprintf("This row's %d figures, drawn to the row's own scale", columns)
	}
	return fmt.Sprintf("%d of this row's %d columns carry a figure, drawn to the row's own scale",
		drawn, columns)
}

// abs64 is the magnitude of a signed cents value.
//
// Spelled here rather than reached for, and it does not guard math.MinInt64 --
// which cannot arise: these are cents read off a printed page, and
// internal/amount refuses anything that is not one of a closed set of shapes.
func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
