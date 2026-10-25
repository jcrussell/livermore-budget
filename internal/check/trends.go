package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// trendPointsTieToFacts asserts every published point is the fact it cites,
// one to one, in every published field.
//
// Both directions: a point citing no stored fact, and a fact in the document's
// slice no point publishes (a dropped row). And a third arm: each point's
// column is one the document declares, which neither direction reaches since
// the reverse sweep filters on those columns; internal/export would drop such
// a point silently.
type trendPointsTieToFacts struct{}

func (*trendPointsTieToFacts) meta() meta {
	return meta{id: "trend-points-tie-to-facts", tier: 1,
		description: "every point a trends document publishes equals the fact it cites, sits in a column " +
			"the document declares, and every fact in the document's slice is published by exactly one point"}
}

func (*trendPointsTieToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	byID := fact.ByID(s.Facts)

	var findings []Finding
	points := 0
	for _, p := range s.trendDocuments() {
		published := make(map[string]bool)
		declared := make(map[project.Column]bool, len(p.Options.Columns))
		for _, c := range p.Options.Columns {
			declared[c] = true
		}
		for _, series := range p.Trends.Series {
			for _, pt := range series.Points {
				points++
				findings = append(findings, comparePoint(p, series, pt, byID, published)...)
				// Mutation: append a genuine FY2027 point to a one-column
				// document and only this arm fails.
				if col := (project.Column{FiscalYear: pt.FiscalYear, Basis: vocab.Basis(pt.Basis)}); !declared[col] {
					findings = append(findings, finding(
						fmt.Sprintf("%s %s %s", p.Name, series.SeriesID, fact.ColumnLabel(pt.FiscalYear, pt.Basis)),
						"this point publishes %s, which is not one of the %d columns %q "+
							"declares; no page can draw a column the document does not publish, "+
							"so the figure would be dropped",
						fact.ColumnLabel(pt.FiscalYear, pt.Basis), len(p.Options.Columns), p.Name))
				}
			}
		}
		// The other direction, over the facts the projection was built over.
		slice, err := p.slice(s.Facts)
		if err != nil {
			return Result{}, err
		}
		for _, f := range slice {
			if published[f.ID] {
				continue
			}
			findings = append(findings, finding(f.ID,
				"%s p%d %q is %s %s in scope %q, which %s is of, and no point publishes it; "+
					"the document accounts for less of the corpus than it was built from",
				f.DocID, f.Page, f.RowLabel, fyBasis(f), f.Scope, p.Options.ScopeList(), p.Name))
		}
	}

	return conclusion{
		subjects: points,
		unit:     "points",
		held: fmt.Sprintf("%d points across %d trends %s, each equal to the fact it cites and "+
			"each fact in the slice published exactly once",
			points, len(s.trendDocuments()),
			english.Plural(len(s.trendDocuments()), "document", "documents")),
		nothing:  "no projection built a trends document, so no point has been compared",
		findings: findings,
	}.result(), nil
}

// comparePoint checks one point against the fact it names, over every field
// the point publishes.
func comparePoint(p projection, series project.Series, pt project.Point,
	byID map[string]fact.Fact, published map[string]bool,
) []Finding {
	subject := fmt.Sprintf("%s %s %s", p.Name, series.SeriesID, fact.ColumnLabel(pt.FiscalYear, pt.Basis))

	f, ok := byID[pt.FactID]
	if !ok {
		return []Finding{finding(subject,
			"this point cites fact %s, which is not in the fact store; a citation the "+
				"reader cannot follow is the defect this project exists to prevent", pt.FactID)}
	}
	if published[f.ID] {
		return []Finding{finding(subject,
			"fact %s is published by more than one point; anything summing this document "+
				"counts it twice", f.ID)}
	}
	published[f.ID] = true

	var out []Finding
	report := func(what string, got, want any) {
		out = append(out, finding(subject,
			"this point publishes %s %v, and fact %s carries %v", what, got, f.ID, want))
	}
	if pt.AmountCents != f.AmountCents {
		report("amount_cents", pt.AmountCents, f.AmountCents)
	}
	if pt.DocID != f.DocID {
		report("doc_id", pt.DocID, f.DocID)
	}
	if pt.Page != f.Page {
		report("page", pt.Page, f.Page)
	}
	if pt.Offset != f.Offset {
		report("offset", pt.Offset, f.Offset)
	}
	if pt.Token != f.Token {
		report("token", pt.Token, f.Token)
	}
	if pt.Derived != f.Derived {
		report("derived", pt.Derived, f.Derived)
	}
	if pt.FiscalYear != f.FiscalYear || pt.Basis != string(f.Basis) {
		report("column", fact.ColumnLabel(pt.FiscalYear, pt.Basis), fyBasis(f))
	}
	// The series id is recomputed, not trusted: a point under the wrong series
	// agrees in every field above.
	if got := f.SeriesID(); got != series.SeriesID {
		out = append(out, finding(subject,
			"this point is published under series %s, but fact %s recomputes to series %s; "+
				"it is filed under a different printed row than the one it came from",
			series.SeriesID, f.ID, got))
	}
	return out
}

// fyBasis names a fact's column the way a finding should.
func fyBasis(f fact.Fact) string { return fact.BandLabel(f.FiscalYear, fact.BandOf(&f), f.Basis) }

// incompleteSeries are the series a trends document legitimately cannot fill in
// every column, each with the reason, keyed by series id. An entry that
// outlives its reason is refused by staleSeriesDeclarations. Keep it empty.
// [Load] copies it to Subject.IncompleteSeries.
var incompleteSeries = map[string]string{}

// trendSeriesAreComplete asserts every series has a point in every column of
// Options.Columns. A row dropped from one column is caught (fisc-d5n); a whole
// column vanishing is not, since Slices derives the columns from the facts;
// project.TrendsColumns, held by publishedProjectionBuilt, is that floor.
type trendSeriesAreComplete struct{}

func (*trendSeriesAreComplete) meta() meta {
	return meta{id: "trend-series-are-complete", tier: 1,
		description: "every series in a trends document has a point in every column the document was " +
			"built over, or the gap is declared with its reason"}
}

func (*trendSeriesAreComplete) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	series := 0
	declared := map[string]int{}
	seen := map[string]bool{}

	for _, p := range s.trendDocuments() {
		for _, sr := range p.Trends.Series {
			series++
			seen[sr.SeriesID] = true
			missing := missingColumns(sr, p.Options.Columns)
			if len(missing) == 0 {
				// A series that is complete AND declared incomplete is caught
				// below, by the declaration exempting nothing.
				continue
			}
			if _, ok := s.IncompleteSeries[sr.SeriesID]; ok {
				declared[sr.SeriesID]++
				continue
			}
			findings = append(findings, finding(sr.SeriesID,
				"%s series %q (fund %s) has no point in %s, and no entry in "+
					"incompleteSeries declares that gap. A missing point publishes nothing "+
					"to disagree with, so no other check can see it",
				p.Name, sr.Label, fact.FundString(sr.Fund), project.Describe(missing)))
		}
	}
	findings = append(findings, staleSeriesDeclarations(s.IncompleteSeries, seen, declared)...)

	held := fmt.Sprintf("%d series, each with a point in every column its document covers", series)
	if len(declared) > 0 {
		held = fmt.Sprintf("%d series, each with a point in every column its document covers, "+
			"or a declared gap: %s", series, describeIncomplete(s.IncompleteSeries, declared))
	}
	return conclusion{
		subjects: series,
		unit:     "series",
		held:     held,
		nothing:  "no projection built a trends document, so no series has been examined",
		findings: findings,
	}.result(), nil
}

// missingColumns is the columns a series has no point in, in the document's own
// column order.
func missingColumns(s project.Series, cols []project.Column) []project.Column {
	have := make(map[project.Column]bool, len(s.Points))
	for _, p := range s.Points {
		have[project.Column{FiscalYear: p.FiscalYear, Basis: vocab.Basis(p.Basis)}] = true
	}
	var out []project.Column
	for _, c := range cols {
		if !have[c] {
			out = append(out, c)
		}
	}
	return out
}

// staleSeriesDeclarations refuses an incompleteSeries entry that has stopped
// being true, in either of the two ways it can.
//
// Same shape and same argument as staleDocumentDeclarations over
// uncheckedDocuments: a declaration nobody can see expiring is a declaration
// that outlives its reason.
func staleSeriesDeclarations(incomplete map[string]string, seen map[string]bool, declared map[string]int) []Finding {
	var out []Finding
	for _, id := range slices.Sorted(maps.Keys(incomplete)) {
		if declared[id] > 0 {
			continue
		}
		what := "no trends document publishes a series with that id at all; the series was " +
			"removed or renamed, or this id was mistyped"
		if seen[id] {
			what = "that series now has a point in every column, so the declaration is " +
				"exempting nothing"
		}
		out = append(out, finding(id,
			"incompleteSeries declares this series legitimately short a column, but %s; the "+
				"declaration must be removed", what))
	}
	return out
}

// describeIncomplete renders the declared gaps with their reasons: a count alone
// would let a growing exemption pass unread.
func describeIncomplete(incomplete map[string]string, byID map[string]int) string {
	out := make([]string, 0, len(byID))
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		out = append(out, fmt.Sprintf("%s (%s)", id, incomplete[id]))
	}
	return strings.Join(out, ", ")
}
