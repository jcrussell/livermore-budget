package export

import (
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/site"
)

// fourColumns is the shape the real document has, which is what the mark is
// laid out against.
func fourColumns() []columnRef {
	return []columnRef{
		{Label: "FY 2023-24", Basis: "actual", Group: "actual"},
		{Label: "FY 2024-25", Basis: "revised", Group: "revised", New: true},
		{Label: "FY 2025-26", Basis: "adopted", Group: "adopted", New: true},
		{Label: "FY 2026-27", Basis: "adopted", Group: "adopted"},
	}
}

// cells turns amounts into the cells buildMark reads. A nil entry is a column
// the row is not printed in, which is NOT a zero.
func cells(amounts ...*int64) []cellRef {
	out := make([]cellRef, 0, len(amounts))
	for _, a := range amounts {
		if a == nil {
			out = append(out, cellRef{Missing: true, Value: "—"})
			continue
		}
		out = append(out, cellRef{Cents: *a, Value: dollars(*a), Negative: *a < 0})
	}
	return out
}

func cents(n int64) *int64 { return &n }

// TestTheMarkDrawsEveryFigureAndOnlyFigures is the mark's central claim: it is a
// rendering of the row's own published cells and of nothing else.
func TestTheMarkDrawsEveryFigureAndOnlyFigures(t *testing.T) {
	cols := fourColumns()
	m := buildMark(cells(cents(100_00), cents(200_00), cents(300_00), cents(400_00)), cols)

	if len(m.Bars) != 4 {
		t.Fatalf("bars = %d over four printed figures, want 4", len(m.Bars))
	}
	// No negative cell, so the whole box goes to the positives and zero is the
	// bottom edge.
	if m.Baseline != markHeight {
		t.Errorf("baseline = %d with no contra cell, want the bottom edge %d", m.Baseline, markHeight)
	}
	// The tallest bar is the figure furthest from zero — in this all-positive
	// row, also its largest — and fills the box. That is what "the row's own
	// scale" means, and it is the claim the page's caption makes to the reader.
	if got := m.Bars[3].Height; got != markHeight {
		t.Errorf("the largest figure's bar is %d high, want the full %d", got, markHeight)
	}
	// And the rest are proportional to it, in integer arithmetic.
	for i, want := range []int{7, 15, 22, 30} {
		if got := m.Bars[i].Height; got != want {
			t.Errorf("bar %d height = %d, want %d (%d%% of the row's largest)",
				i, got, want, (i+1)*25)
		}
	}
	// Every bar carries the column and the figure, so a bar is identifiable
	// without the mark having room for labels.
	if want := "FY 2023-24 actual: $100"; m.Bars[0].Title != want {
		t.Errorf("bar title = %q, want %q", m.Bars[0].Title, want)
	}
	if !strings.Contains(m.Label, "4 figures") {
		t.Errorf("label = %q, want it to say the row carries four figures", m.Label)
	}
}

// TestAContraRowSplitsTheBaseline: the General Fund's ERAF and RPTTF Reduction
// are printed in parentheses and published signed, and a mark that clipped them
// at zero would draw a row of nothing for a row of real money.
func TestAContraRowSplitsTheBaseline(t *testing.T) {
	cols := fourColumns()
	m := buildMark(cells(cents(-100_00), cents(-50_00), cents(100_00), cents(50_00)), cols)

	if m.Baseline != markHeight/2 {
		t.Fatalf("baseline = %d with a contra cell, want the middle %d", m.Baseline, markHeight/2)
	}
	// Negatives start AT the baseline and grow down; positives end at it.
	if got := m.Bars[0]; got.Y != m.Baseline || !got.Negative {
		t.Errorf("the contra bar is %+v, want it starting at the baseline and marked negative", got)
	}
	if got := m.Bars[2]; got.Y+got.Height != m.Baseline || got.Negative {
		t.Errorf("the positive bar is %+v, want it ending at the baseline and not marked negative", got)
	}
	// Magnitude, not value: the row scales against its own biggest number
	// whichever side of zero that is.
	if m.Bars[0].Height != markHeight/2 {
		t.Errorf("the largest MAGNITUDE (-$100) is %d high, want the half-box %d; scaling "+
			"against the largest VALUE would make a mostly-negative row draw nothing",
			m.Bars[0].Height, markHeight/2)
	}
}

// TestAnAllNegativeRowsTallestBarIsItsSmallestNumber: the mark scales by
// magnitude, so in a row whose non-zero figures are all negative the tallest
// bar belongs to the row's SMALLEST number, and its largest — zero — draws as
// a dot on the line. That is what the pages' captions promise: the tallest bar
// is "the figure furthest from zero", not the row's largest number. The
// shipped witness is All Other Governmental Funds / Unassigned on
// balances.html, whose tallest bar is its −$454,071.
func TestAnAllNegativeRowsTallestBarIsItsSmallestNumber(t *testing.T) {
	cols := fourColumns()
	// The Unassigned row's shape in four columns: a published zero, then three
	// figures the city printed in parentheses.
	m := buildMark(cells(cents(0), cents(-157_775_00), cents(-337_703_00), cents(-454_071_00)), cols)

	if len(m.Bars) != 4 {
		t.Fatalf("bars = %d over four printed figures, want 4", len(m.Bars))
	}
	if m.Baseline != markHeight/2 {
		t.Fatalf("baseline = %d with contra cells, want the middle %d", m.Baseline, markHeight/2)
	}
	// The full-height bar is the most negative cell — the row's smallest
	// number — hanging the whole half-box below the line.
	if got := m.Bars[3]; got.Height != markHeight/2 || !got.Negative || got.Y != m.Baseline {
		t.Errorf("the figure furthest from zero (−$454,071) drew %+v, want the full "+
			"half-box %d hanging from the baseline", got, markHeight/2)
	}
	// The row's LARGEST number is the zero, and it does not get the tallest
	// bar — it gets no bar at all, only the dot a published zero always gets.
	if got := m.Bars[0]; !got.Zero || got.Height != 0 {
		t.Errorf("the row's largest number ($0) drew %+v, want a dot and no bar", got)
	}
	// And the other negatives are proportional to the largest magnitude, in
	// integer arithmetic.
	for i, want := range []int{5, 11} {
		if got := m.Bars[i+1].Height; got != want {
			t.Errorf("bar %d height = %d, want %d (its share of the row's largest magnitude)",
				i+1, got, want)
		}
	}
}

// TestAnAbsentColumnAndAPublishedZeroDoNotLookAlike is the mark's version of the
// invariant the whole project turns on: a printed dash is a figure the city
// published, and an empty cell means the line does not apply
// (AGENTS.md, Provenance invariants). A mark that drew them the same way would be
// asserting the city published a zero it did not.
func TestAnAbsentColumnAndAPublishedZeroDoNotLookAlike(t *testing.T) {
	cols := fourColumns()
	m := buildMark(cells(nil, cents(0), cents(100_00), cents(50_00)), cols)

	if len(m.Bars) != 3 {
		t.Fatalf("bars = %d, want 3: the absent column gets no rectangle at all", len(m.Bars))
	}
	// The absent column leaves its SLOT empty rather than shifting its
	// neighbours left, which is the same failure buildCells exists to prevent
	// one row up: a figure drawn under the wrong year.
	if m.Bars[0].X != 1*(markBarWidth+markBarGap) {
		t.Errorf("the first drawn bar is at x=%d, want the SECOND slot (%d): an absent "+
			"column must not slide the rest of the row left",
			m.Bars[0].X, 1*(markBarWidth+markBarGap))
	}
	// THREE STATES, THREE SHAPES. The absent column has no barRef; the published
	// zero is a DOT centred on the baseline; a figure is a rectangle. Telling
	// the last two apart by height was tried and failed at the rendered scale --
	// one viewBox unit is about a pixel, so a one-unit tick and a two-unit bar
	// were the same smudge and the only surviving difference was fill.
	zero := m.Bars[0]
	if !zero.Zero || zero.R == 0 || zero.Y != m.Baseline {
		t.Errorf("the published zero is %+v, want a dot centred on the baseline", zero)
	}
	if want := m.Bars[0].X + markBarWidth/2; zero.CX != want {
		t.Errorf("the zero dot is at cx=%d, want the centre of its slot %d", zero.CX, want)
	}
	// And it is not a rectangle: a zero that rendered as a bar of any height
	// would be a figure the city did not print.
	if zero.Height != 0 {
		t.Errorf("the published zero carries rectangle height %d; it is drawn as a dot",
			zero.Height)
	}
	if !strings.Contains(m.Label, "3 of this row's 4") {
		t.Errorf("label = %q, want it to say one column carries no figure", m.Label)
	}
}

// TestATinyFigureStillGetsAMark: a row whose FY2023-24 actual is a rounding
// error against its FY2026-27 adopted has a REAL figure, and integer division
// puts it at zero height. A bar that is not there says "no money" where the page
// says $47,000, which is the absent-is-not-zero error arriving through geometry.
func TestATinyFigureStillGetsAMark(t *testing.T) {
	m := buildMark(cells(cents(1), cents(0), cents(0), cents(100_000_00)), fourColumns())

	tiny := m.Bars[0]
	if tiny.Height < markMinBar {
		t.Errorf("a one-cent figure beside a $100,000 one is %d high, want at least %d",
			tiny.Height, markMinBar)
	}
	if tiny.Zero {
		t.Error("a one-cent figure is marked as a published zero; it is neither zero nor absent")
	}
	// THE SHAPES DIFFER, not just the sizes, and markZeroRadius carries the
	// measurement that says why. A rectangle against a dot cannot collapse the
	// way a real figure and a published-zero tick did.
	if tiny.R != 0 || tiny.Height < markMinBar {
		t.Errorf("the smallest real figure is %+v, want a rectangle at least %d high and "+
			"no dot", tiny, markMinBar)
	}
	// And it is still told apart from the zeros beside it.
	if !m.Bars[1].Zero || !m.Bars[2].Zero {
		t.Errorf("the printed zeros are %+v and %+v, want both marked zero", m.Bars[1], m.Bars[2])
	}
}

// TestAMarkFitsItsColumns: a document of three columns gets a three-slot mark,
// not a four-slot one with a gap on the end. The mark is laid out against the
// document, like everything else on this page.
func TestAMarkFitsItsColumns(t *testing.T) {
	three := fourColumns()[:3]
	m := buildMark(cells(cents(1_00), cents(2_00), cents(3_00)), three)

	want := 3*markBarWidth + 2*markBarGap
	if m.Width != want {
		t.Errorf("width = %d over three columns, want %d", m.Width, want)
	}
	if last := m.Bars[2]; last.X+last.Width != m.Width {
		t.Errorf("the last bar ends at %d and the box is %d wide", last.X+last.Width, m.Width)
	}
}

// TestAllZeroRowDrawsZerosRatherThanDividingByThem. Every figure in the row is a
// printed dash, so the row's largest magnitude is zero and the scale has no
// denominator. It must draw four published zeros, not panic and not vanish.
func TestAllZeroRowDrawsZerosRatherThanDividingByThem(t *testing.T) {
	m := buildMark(cells(cents(0), cents(0), cents(0), cents(0)), fourColumns())
	if len(m.Bars) != 4 {
		t.Fatalf("bars = %d, want 4 published zeros", len(m.Bars))
	}
	for i, b := range m.Bars {
		if !b.Zero || b.R == 0 || b.Y != m.Baseline {
			t.Errorf("bar %d = %+v, want a dot on the baseline marked zero", i, b)
		}
	}
}

// TestTheMarkIsDeterministic pins what the byte-for-byte export depends on: the
// same cells produce the same geometry, and the geometry is integers. A float
// here would render as 12.333333333333334 and make the page's bytes depend on
// the platform.
func TestTheMarkIsDeterministic(t *testing.T) {
	in := cells(cents(-3_00), cents(0), nil, cents(7_00))
	first := buildMark(in, fourColumns())
	second := buildMark(in, fourColumns())
	if diff := cmp.Diff(first, second); diff != "" {
		t.Errorf("two builds of one row differ (-first +second):\n%s", diff)
	}
}

// TestTheGroupBoundaryIsDrawnInsideTheMark. The page's caption tells the reader
// the rules inside a mark fall where the rules between the headers do, and
// fisc-4ua.3 asks for "a labelled rule" at the actual-to-budget boundary.
//
// It was claimed and not drawn: buildMark never read columnRef.New, every gap
// was uniform, and a reader looking for the boundary inside the mark found four
// evenly spaced bars and read them as one series — which is the cross-measurement
// comparison this document's whole design exists to block.
func TestTheGroupBoundaryIsDrawnInsideTheMark(t *testing.T) {
	cols := fourColumns() // New on columns 1 and 2, as the real document has it
	m := buildMark(cells(cents(1_00), cents(2_00), cents(3_00), cents(4_00)), cols)

	if len(m.Rules) != 2 {
		t.Fatalf("rules = %v over columns with two group boundaries, want 2", m.Rules)
	}
	// Each rule falls in the gap BEFORE the column that starts a new group, so
	// it separates the same pair the header's border-left separates.
	for i, at := range m.Rules {
		col := i + 1
		gapStart := col*(markBarWidth+markBarGap) - markBarGap
		gapEnd := col * (markBarWidth + markBarGap)
		if at < gapStart || at >= gapEnd {
			t.Errorf("rule %d is at x=%d, want it inside the gap [%d,%d) before column %d",
				i, at, gapStart, gapEnd, col)
		}
	}

	// A document whose columns are all one comparable group gets no rules at
	// all, rather than a decorative line between every bar.
	flat := fourColumns()
	for i := range flat {
		flat[i].New = false
		flat[i].Group = "adopted"
	}
	if got := buildMark(cells(cents(1), cents(2), cents(3), cents(4)), flat); len(got.Rules) != 0 {
		t.Errorf("rules = %v over one comparable group, want none", got.Rules)
	}
}

// TestTheMarkPublishesItsOwnHeight: the template had the viewBox height
// hardcoded while Width and Baseline came from here, so raising markHeight would
// have clipped every bar with nothing to notice. Geometry split across two files
// where only one is checked is geometry that drifts.
func TestTheMarkPublishesItsOwnHeight(t *testing.T) {
	m := buildMark(cells(cents(1_00)), fourColumns()[:1])
	if m.Height != markHeight {
		t.Errorf("height = %d, want markHeight %d", m.Height, markHeight)
	}
	// Every bar fits inside the box it publishes. That is the property the
	// template's hardcoded 30 was silently assuming.
	for i, b := range m.Bars {
		if b.Y < 0 || b.Y+b.Height > m.Height {
			t.Errorf("bar %d spans [%d,%d) in a box %d tall", i, b.Y, b.Y+b.Height, m.Height)
		}
	}
}

// TestTheShapeSentenceAndTheMarkAgreeOnWhatTallestMeans couples the captions'
// superlative to the scaling it describes, which nothing else couples.
//
// history.html.tmpl and trends.html.tmpl each promise that the tallest bar in
// every row is the figure furthest from zero. That sentence is buildMark's
// magnitude scaling said in English: two spellings of one claim, one in a
// template a copy editor owns and one in Go, with no seam between them. The
// templates are read through site.FS() so the assertion is about the shipped
// files, not a copy (AGENTS.md, "The node boundary" makes the same argument
// for app.js).
//
// A PIN RATHER THAN A GUARD, for TestTheLedesProseNamesThePagesScope's reason:
// the sentence's meaning cannot be checked mechanically, so both halves are
// pinned and either drifting alone reddens a test named for the coupling. The
// behaviour half re-derives the promise on the shipped witness row --
// balances.html's All Other Governmental Funds / Unassigned, whose largest
// number, zero, draws a dot while its most negative figure takes the tallest
// bar. TestAnAllNegativeRowsTallestBarIsItsSmallestNumber pins that row's
// exact geometry; this half asserts only the superlative the sentence states.
func TestTheShapeSentenceAndTheMarkAgreeOnWhatTallestMeans(t *testing.T) {
	const promise = "the tallest bar in every row is the figure furthest from zero"
	for _, name := range []string{"history.html.tmpl", "trends.html.tmpl"} {
		b, err := fs.ReadFile(site.FS(), name)
		if err != nil {
			t.Fatalf("read shipped %s: %v", name, err)
		}
		if !strings.Contains(string(b), promise) {
			t.Errorf("%s no longer says %q; if buildMark's scaling changed, change the "+
				"sentence with it -- if only the wording did, the page now claims a "+
				"scaling the mark does not draw", name, promise)
		}
	}

	m := buildMark(cells(cents(0), cents(-157_775_00), cents(-337_703_00), cents(-454_071_00)), fourColumns())
	tallest := 0
	for i, b := range m.Bars {
		if b.Height > m.Bars[tallest].Height {
			tallest = i
		}
	}
	if tallest != 3 {
		t.Errorf("the tallest bar belongs to cell %d, want cell 3: the figure furthest "+
			"from zero no longer takes the tallest bar, so the promise both templates "+
			"print is false as published", tallest)
	}
}

// TestTheClientComposesMarkIDsWithTheProducersPrefixes pins site/app.js's
// RESIDUAL_PREFIX and GAP_PREFIX literals to residualPrefix and gapPrefix.
//
// THE CLIENT COMPOSES THESE IDS AND NO LONGER CHECKS THEM: carryResidual and
// markGap read a mark's id off the rung answer and draw it, and the one thing
// that would leave a mark drawn under a name the page's own tests for
// isResidual and isGap do not recognise is the two prefixes drifting apart.
// Pinned as text on both sides, because nothing compiles the client.
func TestTheClientComposesMarkIDsWithTheProducersPrefixes(t *testing.T) {
	app, err := fs.ReadFile(site.FS(), "app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	src := string(app)
	for name, want := range map[string]string{
		"RESIDUAL_PREFIX": residualPrefix,
		"GAP_PREFIX":      gapPrefix,
	} {
		decl := "export const " + name + " = " + strconv.Quote(want) + ";"
		if !strings.Contains(src, decl) {
			t.Errorf("site/app.js does not declare %s; the client would compose a mark id the producer does not", decl)
		}
	}
}
