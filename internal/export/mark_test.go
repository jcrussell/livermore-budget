package export

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/site"
)

// fourColumns is the real document's column shape.
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
		out = append(out, cellRef{Cents: *a, Value: amount.Cents(*a).Dollars(), Negative: *a < 0})
	}
	return out
}

func cents(n int64) *int64 { return &n }

// TestTheMarkDrawsEveryFigureAndOnlyFigures: the mark renders the row's own
// published cells and nothing else.
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
	// The figure furthest from zero fills the box.
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
	// Every bar carries its column and figure.
	if want := "FY 2023-24 actual: $100"; m.Bars[0].Title != want {
		t.Errorf("bar title = %q, want %q", m.Bars[0].Title, want)
	}
	if !strings.Contains(m.Label, "4 figures") {
		t.Errorf("label = %q, want it to say the row carries four figures", m.Label)
	}
}

// TestAContraRowSplitsTheBaseline: a row printed in parentheses (the General
// Fund's ERAF and RPTTF Reduction) draws below the baseline, not clipped at zero.
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
// magnitude, so in an all-negative row the smallest number takes the tallest
// bar and the zero draws a dot. Witness: balances.html's All Other
// Governmental Funds / Unassigned, tallest bar −$454,071.
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
	// The most negative cell hangs the whole half-box below the line.
	if got := m.Bars[3]; got.Height != markHeight/2 || !got.Negative || got.Y != m.Baseline {
		t.Errorf("the figure furthest from zero (−$454,071) drew %+v, want the full "+
			"half-box %d hanging from the baseline", got, markHeight/2)
	}
	// The row's largest number, the zero, gets only a dot.
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

// TestAnAbsentColumnAndAPublishedZeroDoNotLookAlike: absent is not zero
// (AGENTS.md, Provenance invariants), in the mark's geometry.
func TestAnAbsentColumnAndAPublishedZeroDoNotLookAlike(t *testing.T) {
	cols := fourColumns()
	m := buildMark(cells(nil, cents(0), cents(100_00), cents(50_00)), cols)

	if len(m.Bars) != 3 {
		t.Fatalf("bars = %d, want 3: the absent column gets no rectangle at all", len(m.Bars))
	}
	// The absent column leaves its slot empty rather than shifting its
	// neighbours under the wrong year.
	if m.Bars[0].X != 1*(markBarWidth+markBarGap) {
		t.Errorf("the first drawn bar is at x=%d, want the SECOND slot (%d): an absent "+
			"column must not slide the rest of the row left",
			m.Bars[0].X, 1*(markBarWidth+markBarGap))
	}
	// Absent has no barRef; a published zero is a dot on the baseline; a
	// figure is a rectangle. Height alone cannot tell the last two apart at
	// one viewBox unit per pixel.
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

// TestATinyFigureStillGetsAMark: a figure integer division puts at zero
// height still draws a bar, distinct from a published zero.
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
	// The shapes differ, not just the sizes; see markZeroRadius.
	if tiny.R != 0 || tiny.Height < markMinBar {
		t.Errorf("the smallest real figure is %+v, want a rectangle at least %d high and "+
			"no dot", tiny, markMinBar)
	}
	// And it is still told apart from the zeros beside it.
	if !m.Bars[1].Zero || !m.Bars[2].Zero {
		t.Errorf("the printed zeros are %+v and %+v, want both marked zero", m.Bars[1], m.Bars[2])
	}
}

// TestAMarkFitsItsColumns: a document of three columns gets a three-slot mark.
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

// TestAllZeroRowDrawsZerosRatherThanDividingByThem: an all-dash row has no
// scale denominator and draws four published zeros.
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

// TestTheMarkIsDeterministic: the same cells produce the same integer
// geometry, which the byte-for-byte export depends on.
func TestTheMarkIsDeterministic(t *testing.T) {
	in := cells(cents(-3_00), cents(0), nil, cents(7_00))
	first := buildMark(in, fourColumns())
	second := buildMark(in, fourColumns())
	if diff := cmp.Diff(first, second); diff != "" {
		t.Errorf("two builds of one row differ (-first +second):\n%s", diff)
	}
}

// TestTheGroupBoundaryIsDrawnInsideTheMark: a rule falls inside the mark
// where the header rules fall, at each columnRef.New group boundary.
func TestTheGroupBoundaryIsDrawnInsideTheMark(t *testing.T) {
	cols := fourColumns() // New on columns 1 and 2, as the real document has it
	m := buildMark(cells(cents(1_00), cents(2_00), cents(3_00), cents(4_00)), cols)

	if len(m.Rules) != 2 {
		t.Fatalf("rules = %v over columns with two group boundaries, want 2", m.Rules)
	}
	// Each rule falls in the gap before the column that starts a new group.
	for i, at := range m.Rules {
		col := i + 1
		gapStart := col*(markBarWidth+markBarGap) - markBarGap
		gapEnd := col * (markBarWidth + markBarGap)
		if at < gapStart || at >= gapEnd {
			t.Errorf("rule %d is at x=%d, want it inside the gap [%d,%d) before column %d",
				i, at, gapStart, gapEnd, col)
		}
	}

	// A document of one group gets no rules.
	flat := fourColumns()
	for i := range flat {
		flat[i].New = false
		flat[i].Group = "adopted"
	}
	if got := buildMark(cells(cents(1), cents(2), cents(3), cents(4)), flat); len(got.Rules) != 0 {
		t.Errorf("rules = %v over one comparable group, want none", got.Rules)
	}
}

// TestTheMarkPublishesItsOwnHeight: the mark publishes its viewBox height
// beside Width and Baseline, and every bar fits inside it.
func TestTheMarkPublishesItsOwnHeight(t *testing.T) {
	m := buildMark(cells(cents(1_00)), fourColumns()[:1])
	if m.Height != markHeight {
		t.Errorf("height = %d, want markHeight %d", m.Height, markHeight)
	}
	// Every bar fits inside the box it publishes.
	for i, b := range m.Bars {
		if b.Y < 0 || b.Y+b.Height > m.Height {
			t.Errorf("bar %d spans [%d,%d) in a box %d tall", i, b.Y, b.Y+b.Height, m.Height)
		}
	}
}

// TestTheShapeSentenceAndTheMarkAgreeOnWhatTallestMeans pins both halves of
// one claim: history.html.tmpl's "tallest bar is the figure furthest from
// zero", read through site.FS(), and buildMark's magnitude scaling on the
// all-negative witness row.
func TestTheShapeSentenceAndTheMarkAgreeOnWhatTallestMeans(t *testing.T) {
	const promise = "the tallest bar in every row is the figure furthest from zero"
	for _, name := range []string{"history.html.tmpl"} {
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
			"from zero no longer takes the tallest bar, so the promise the template "+
			"prints is false as published", tallest)
	}
}

// TestTheHeroNoteNamesItsColumnAsTheLedeDoes holds the hero's note to
// ledeOf, the one spelling of a column the chart title and the lede use.
// Mutation: spell the note's column by hand with a comma, and it goes red.
func TestTheHeroNoteNamesItsColumnAsTheLedeDoes(t *testing.T) {
	meta := projectionMetadata{FiscalYear: 2026, FiscalYearLabel: "FY 2025-26", Basis: "adopted"}
	hero := heroFor(meta)
	if want := ledeOf(meta.FiscalYearLabel, meta.Basis); !strings.Contains(hero.Note, want) {
		t.Errorf("hero note %q does not name its column as the lede does, %q", hero.Note, want)
	}
}
