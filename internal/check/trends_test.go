package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// trendsTestScope is the scope the trends document is of, spelled here for the
// same reason project.ScopeRevenueByFund is spelled in its own file.
const trendsTestScope = "revenue-by-fund"

var trendsTestColumns = []project.Column{
	{FiscalYear: 2026, Basis: mapping.BasisAdopted},
	{FiscalYear: 2027, Basis: mapping.BasisAdopted},
}

// trendsSubject is a Subject carrying one trends document over two columns of
// one printed row, built the way internal/project builds it so the check is
// exercised against the real producer rather than a hand-written document.
func trendsSubject(t *testing.T, facts []fact.Fact) *Subject {
	t.Helper()
	o := project.Options{Columns: trendsTestColumns, Scopes: []string{trendsTestScope}, Version: testVersion}
	doc, err := (&project.Trends{}).Document(facts, o)
	if err != nil {
		t.Fatalf("build the trends document: %v", err)
	}
	return &Subject{
		Facts:       facts,
		Projections: []projection{{Name: "revenue-trends", Options: o, Trends: doc}},
	}
}

// trendsTestFacts is two printed rows across the two columns: four facts.
func trendsTestFacts(t *testing.T) []fact.Fact {
	t.Helper()
	var out []fact.Fact
	for _, r := range []struct {
		rule, label string
		cents       [2]int64
	}{
		{"gf-rev-property-taxes", "Current Year - Secured", [2]int64{6_206_747_000, 6_400_000_000}},
		{"gf-rev-property-taxes", "Prior Year - Secured", [2]int64{45_839_400, 46_000_000}},
	} {
		row := mapping.Row{Category: "taxes/property"}
		rowPath := fact.RowPath(row)
		columnPath := fact.ColumnPath(mapping.Column{FundGroup: "general", Fund: 100}, trendsTestScope)
		for i, c := range trendsTestColumns {
			out = append(out, fact.Fact{
				ID: fact.MakeID(testDoc, r.rule, rowPath, r.label, columnPath,
					c.FiscalYear, c.Basis),
				DocID:       testDoc,
				Page:        127,
				Offset:      500 + 40*len(out) + i,
				Token:       "tok",
				RuleID:      r.rule,
				Kind:        mapping.KindRevenue,
				Basis:       c.Basis,
				Scope:       trendsTestScope,
				FiscalYear:  c.FiscalYear,
				RowPath:     rowPath,
				RowLabel:    r.label,
				Category:    "taxes/property",
				ColumnPath:  columnPath,
				FundGroup:   "general",
				Fund:        fact.FundNumber(100),
				Sign:        mapping.SignPositive,
				Units:       "dollars",
				AmountCents: r.cents[i],
			})
		}
	}
	return out
}

// TestTrendPointsTieToFactsPasses is the baseline: the real producer over a
// clean store reports every point examined and nothing wrong.
func TestTrendPointsTieToFactsPasses(t *testing.T) {
	t.Parallel()
	res := runTrendPoints(t, trendsSubject(t, trendsTestFacts(t)))
	if res.Status != StatusPass {
		t.Fatalf("trend-points-tie-to-facts = %s: %s (%v)", res.Status, res.Summary, res.Findings)
	}
	if res.Subjects != 4 {
		t.Errorf("examined %d points, want 4", res.Subjects)
	}
}

// TestAMutatedPointIsCaught is the check's whole reason for existing, and it is
// tested per PUBLISHED FIELD rather than on the amount alone: a point that cited
// the right figure off the wrong page would be a broken provenance link that
// every arithmetic check passes.
func TestAMutatedPointIsCaught(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		bend func(p *project.Point)
		want string
	}{
		{"amount", func(p *project.Point) { p.AmountCents++ }, "amount_cents"},
		{"page", func(p *project.Point) { p.Page = 999 }, "page"},
		{"offset", func(p *project.Point) { p.Offset++ }, "offset"},
		{"token", func(p *project.Point) { p.Token = "wrong" }, "token"},
		{"doc", func(p *project.Point) { p.DocID = "somewhere-else" }, "doc_id"},
		{"derived", func(p *project.Point) { p.Derived = true }, "derived"},
		{"column", func(p *project.Point) { p.FiscalYear = 2099 }, "column"},
		{"fact id", func(p *project.Point) { p.FactID = "fisc-f-000000000000" },
			"not in the fact store"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := trendsSubject(t, trendsTestFacts(t))
			c.bend(&s.Projections[0].Trends.Series[0].Points[0])
			res := runTrendPoints(t, s)
			if res.Status != StatusFail {
				t.Fatalf("a mutated %s reported %s: %s", c.name, res.Status, res.Summary)
			}
			if !strings.Contains(findingDetails(res), c.want) {
				t.Errorf("findings do not name %q: %s", c.want, findingDetails(res))
			}
		})
	}
}

// TestAFactNoPointPublishesIsCaught is the direction that catches a DROPPED ROW,
// and it is the one an obvious implementation misses. A point that disagrees
// with its fact is loud; a fact the document simply left out publishes nothing
// to disagree with, and counts.facts would fall with nothing to say so.
func TestAFactNoPointPublishesIsCaught(t *testing.T) {
	t.Parallel()
	s := trendsSubject(t, trendsTestFacts(t))
	// Drop a whole series from the published document while leaving its facts in
	// the store: exactly what a resolver that stopped emitting a row looks like.
	s.Projections[0].Trends.Series = s.Projections[0].Trends.Series[:1]
	res := runTrendPoints(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a dropped series reported %s: %s", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "no point publishes it") {
		t.Errorf("findings do not name the unpublished facts: %s", findingDetails(res))
	}
}

// TestAFactPublishedTwiceIsCaught: a duplicated point is counted twice by
// anything summing the document, and the two copies need not even agree.
func TestAFactPublishedTwiceIsCaught(t *testing.T) {
	t.Parallel()
	s := trendsSubject(t, trendsTestFacts(t))
	series := &s.Projections[0].Trends.Series[0]
	series.Points = append(series.Points, series.Points[0])
	res := runTrendPoints(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a duplicated point reported %s: %s", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "more than one point") {
		t.Errorf("findings do not name the duplicate: %s", findingDetails(res))
	}
}

// TestAPointFiledUnderTheWrongSeriesIsCaught is why the series id is recomputed
// from the fact rather than trusted. Every published field of the point still
// agrees with its fact; only the row it is grouped under is wrong, and the
// result is a row that reads as another row's history.
func TestAPointFiledUnderTheWrongSeriesIsCaught(t *testing.T) {
	t.Parallel()
	s := trendsSubject(t, trendsTestFacts(t))
	series := s.Projections[0].Trends.Series
	if len(series) != 2 {
		t.Fatalf("fixture built %d series, want 2", len(series))
	}
	series[0].Points[0], series[1].Points[0] = series[1].Points[0], series[0].Points[0]
	res := runTrendPoints(t, s)
	if res.Status != StatusFail {
		t.Fatalf("two points swapped between series reported %s: %s", res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "different printed row") {
		t.Errorf("findings do not name the misfiling: %s", findingDetails(res))
	}
}

// TestTrendChecksAreVacuousWithoutADocument: a corpus carrying no trends
// document has nothing to examine, and a pass over zero subjects is the one
// thing this package exists to prevent.
func TestTrendChecksAreVacuousWithoutADocument(t *testing.T) {
	t.Parallel()
	s := &Subject{}
	if res := runTrendPoints(t, s); res.Status != StatusVacuous {
		t.Errorf("trend-points-tie-to-facts = %s over no document, want vacuous", res.Status)
	}
	res, err := (&trendSeriesAreComplete{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusVacuous {
		t.Errorf("trend-series-are-complete = %s over no document, want vacuous", res.Status)
	}
}

// TestASeriesShortAColumnIsCaught is fisc-d5n's whole case: the city drops a
// printed row from one column, or adds a line mid-book that only the later years
// carry. Neither is visible to trend-points-tie-to-facts, because every point
// that IS published is correct.
func TestASeriesShortAColumnIsCaught(t *testing.T) {
	t.Parallel()
	s := trendsSubject(t, trendsTestFacts(t))
	series := &s.Projections[0].Trends.Series[0]
	dropped := series.Points[1]
	series.Points = series.Points[:1]

	// THE TWO CHECKS OVERLAP HERE, AND THAT IS WRITTEN DOWN RATHER THAN WISHED
	// AWAY. This block used to say the values check "must stay QUIET, or this
	// test proves nothing about the division of labour between the two", and it
	// was wrong three times over (fisc-ewt): the premise was false, the
	// condition was inverted against its own comment, and the body was t.Logf so
	// it could not fail in either direction.
	//
	// What actually happens, reproduced rather than reasoned about: dropping a
	// point reddens BOTH checks. trend-points-tie-to-facts fails through its
	// REVERSE arm, because the dropped point's fact is still in
	// project.SelectFacts(p.Options) and no point publishes it; trend-series-are-complete
	// fails because the series is short a column. That is redundancy, not a
	// division of labour, and it is very likely the right answer -- two
	// independent statements about one corruption is what makes a check set
	// hard to fool. It simply was not what the file claimed.
	points := runTrendPoints(t, s)
	if points.Status != StatusFail {
		t.Errorf("trend-points-tie-to-facts = %s on a dropped point, want fail: %s",
			points.Status, points.Summary)
	}
	if !strings.Contains(findingDetails(points), "no point publishes it") {
		t.Errorf("trend-points-tie-to-facts did not report the unpublished fact: %s",
			findingDetails(points))
	}

	res, err := (&trendSeriesAreComplete{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("a series missing FY%d reported %s: %s",
			dropped.FiscalYear, res.Status, res.Summary)
	}
	if !strings.Contains(findingDetails(res), "FY2027 adopted") {
		t.Errorf("findings do not name the missing column: %s", findingDetails(res))
	}
}

// TestAStaleIncompleteSeriesDeclarationIsCaught pins the expiry branch. A
// declaration nobody can see expiring is a declaration that outlives its reason,
// which is the same argument uncheckedDocuments makes.
func TestAStaleIncompleteSeriesDeclarationIsCaught(t *testing.T) {
	t.Parallel()
	s := trendsSubject(t, trendsTestFacts(t))
	id := s.Projections[0].Trends.Series[0].SeriesID

	t.Run("the series is complete after all", func(t *testing.T) {
		withIncompleteSeries(t, s, map[string]string{id: "declared, but nothing is missing"})
		res, err := (&trendSeriesAreComplete{}).Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail ||
			!strings.Contains(findingDetails(res), "exempting nothing") {
			t.Errorf("a declaration over a complete series reported %s: %s",
				res.Status, findingDetails(res))
		}
	})

	t.Run("no such series", func(t *testing.T) {
		withIncompleteSeries(t, s, map[string]string{"fisc-s-000000000000": "typo'd or removed"})
		res, err := (&trendSeriesAreComplete{}).Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail ||
			!strings.Contains(findingDetails(res), "no trends document publishes a series") {
			t.Errorf("a declaration naming no series reported %s: %s",
				res.Status, findingDetails(res))
		}
	})
}

// TestIncompleteSeriesIsEmpty guards the state the map should stay in. An entry
// is a promise that a gap is real; the committed corpus has none, and a future
// entry should be a deliberate act rather than something that accumulated.
func TestIncompleteSeriesIsEmpty(t *testing.T) {
	t.Parallel()
	if len(incompleteSeries) != 0 {
		t.Errorf("incompleteSeries carries %d entries: %v", len(incompleteSeries), incompleteSeries)
	}
}

// withIncompleteSeries declares series on s in place of the ones it carries,
// for one test, so a table of cases cannot leak into the next.
func withIncompleteSeries(t *testing.T, s *Subject, m map[string]string) {
	t.Helper()
	prev := s.IncompleteSeries
	s.IncompleteSeries = m
	t.Cleanup(func() { s.IncompleteSeries = prev })
}

func runTrendPoints(t *testing.T, s *Subject) Result {
	t.Helper()
	res, err := (&trendPointsTieToFacts{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// TestAPointInAnUndeclaredColumnIsCaught is fisc-4j5's check-side half, and the
// mutation is deliberately a GENUINE point: every field matches a real fact,
// the series id recomputes, the amount and all four provenance fields agree.
// Only its COLUMN is one the document does not publish.
//
// That is the shape nothing could see. comparePoint compares a point against
// its own fact, which agrees; and the reverse sweep walks project.SelectFacts(p.Options),
// which filters on the declared columns, so the fact behind the extra point is
// never looked at. Both arms stayed green while a figure was being published
// that no page could draw -- internal/export drops it, having no column to put
// it in.
//
// The test asserts the OTHER two conclusions as well, because a new arm that
// reddens everything is not a check, it is a broken one.
func TestAPointInAnUndeclaredColumnIsCaught(t *testing.T) {
	t.Parallel()
	facts := trendsTestFacts(t)

	// A one-column document, so FY2027's facts are outside it entirely.
	o := project.Options{
		Columns: trendsTestColumns[:1], Scopes: []string{trendsTestScope}, Version: testVersion,
	}
	doc, err := (&project.Trends{}).Document(facts, o)
	if err != nil {
		t.Fatalf("build the trends document: %v", err)
	}
	s := &Subject{
		Facts:       facts,
		Projections: []projection{{Name: "revenue-trends", Options: o, Trends: doc}},
	}

	// Before the mutation the one-column document is clean, which is what makes
	// the assertions below about the mutation rather than about the fixture.
	if got := runTrendPoints(t, s).Status; got != StatusPass {
		t.Fatalf("the unmutated one-column document reported %s, want pass", got)
	}

	// Now publish FY2027 as well, from the real FY2027 fact.
	series := &s.Projections[0].Trends.Series[0]
	var extra project.Point
	found := false
	for _, f := range facts {
		if f.FiscalYear == 2027 && f.SeriesID() == series.SeriesID {
			extra = project.Point{
				FiscalYear: f.FiscalYear, Basis: string(f.Basis), AmountCents: f.AmountCents,
				FactID: f.ID, DocID: f.DocID, Page: f.Page, Offset: f.Offset,
				Token: f.Token, Derived: f.Derived,
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no FY2027 fact for this series; the fixture no longer supports this test")
	}
	series.Points = append(series.Points, extra)

	res := runTrendPoints(t, s)
	if res.Status != StatusFail {
		t.Fatalf("a point in an undeclared column reported %s: %s", res.Status, res.Summary)
	}
	for _, want := range []string{"FY2027 adopted", "not one of the 1 columns"} {
		if !strings.Contains(findingDetails(res), want) {
			t.Errorf("findings do not name %q: %s", want, findingDetails(res))
		}
	}

	// AND THE OTHER TWO STAY GREEN, which is the measurement the bead asked for:
	// this corruption is caught HERE and by nothing else.
	complete, err := (&trendSeriesAreComplete{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if complete.Status != StatusPass {
		t.Errorf("trend-series-are-complete = %s on an EXTRA point, want pass: %s",
			complete.Status, complete.Summary)
	}
}
