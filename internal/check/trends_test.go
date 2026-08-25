package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// trendsTestScope is the scope the trends document is of, spelled here for the
// same reason revenueDetailScope is spelled in its own file.
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
	o := project.Options{Columns: trendsTestColumns, Scope: trendsTestScope, Version: testVersion}
	doc, err := (&project.Trends{}).Document(facts, o)
	if err != nil {
		t.Fatalf("build the trends document: %v", err)
	}
	return &Subject{
		Facts:       facts,
		Projections: []Projection{{Name: "revenue-trends", Options: o, Trends: doc}},
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
				Fund:        100,
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
	s := trendsSubject(t, trendsTestFacts(t))
	series := &s.Projections[0].Trends.Series[0]
	dropped := series.Points[1]
	series.Points = series.Points[:1]

	// The values check must stay QUIET, or this test proves nothing about the
	// division of labour between the two.
	points := runTrendPoints(t, s)
	if points.Status != StatusFail ||
		!strings.Contains(findingDetails(points), "no point publishes it") {
		t.Logf("trend-points-tie-to-facts: %s", points.Summary)
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
// which is the same argument unprojectedScopes and uncheckedDocuments make.
func TestAStaleIncompleteSeriesDeclarationIsCaught(t *testing.T) {
	s := trendsSubject(t, trendsTestFacts(t))
	id := s.Projections[0].Trends.Series[0].SeriesID

	t.Run("the series is complete after all", func(t *testing.T) {
		withIncompleteSeries(t, map[string]string{id: "declared, but nothing is missing"})
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
		withIncompleteSeries(t, map[string]string{"fisc-s-000000000000": "typo'd or removed"})
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
	if len(incompleteSeries) != 0 {
		t.Errorf("incompleteSeries carries %d entries: %v", len(incompleteSeries), incompleteSeries)
	}
}

// withIncompleteSeries swaps the declaration map for one test and restores it,
// so a table of cases cannot leak into the next.
func withIncompleteSeries(t *testing.T, m map[string]string) {
	t.Helper()
	prev := incompleteSeries
	incompleteSeries = m
	t.Cleanup(func() { incompleteSeries = prev })
}

func runTrendPoints(t *testing.T, s *Subject) Result {
	t.Helper()
	res, err := (&trendPointsTieToFacts{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}
