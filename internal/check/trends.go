package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// trendPointsTieToFacts asserts every published point IS the fact it cites.
//
// It is the trends document's link-values-tie-to-facts, and it is stricter,
// because the relationship is different: a link is a SUM of facts and this
// comparison is one to one. So there is no netting to reproduce and no tolerance
// to argue about — a point either equals its fact in every published field or it
// does not.
//
// IT CHECKS BOTH DIRECTIONS, and the second is the one that catches a dropped
// row. A point citing a fact the store does not carry is the obvious failure; a
// fact in the document's own slice that NO point publishes is the quiet one, and
// it is exactly what a resolver change that silently stopped emitting a row would
// look like. counts.facts would fall and nothing else would say anything.
//
// AND A THIRD THING, which neither direction reaches: that each point's COLUMN
// is one the document declares. Both arms above compare a point against the
// FACTS, and a point can agree with a real fact in every published field while
// sitting in a column the document does not publish -- comparePoint compares it
// only to its own fact, and the reverse sweep filters the facts on those same
// columns, so the fact behind it is never looked at. internal/export then drops
// such a point when it lays the table out, because there is no column to put it
// in, and the figure leaves the published page with nothing saying so
// (fisc-4j5). internal/export refuses that document too; this arm is what names
// which point, and it is reached first because `fisc verify` runs before
// `fisc export`.
//
// Nothing in graph.go covers this. Those checks read Subject.Graphs() and this
// document has no nodes and no links, so without this check and
// trend-series-are-complete the document ships unexamined — which is the state
// documents-are-checked exists to refuse.
type trendPointsTieToFacts struct{}

var _ Check = (*trendPointsTieToFacts)(nil)

func (*trendPointsTieToFacts) ID() string { return "trend-points-tie-to-facts" }
func (*trendPointsTieToFacts) Tier() int  { return 1 }
func (*trendPointsTieToFacts) Full() bool { return false }
func (*trendPointsTieToFacts) Description() string {
	return "every point a trends document publishes equals the fact it cites, sits in a column " +
		"the document declares, and every fact in the document's slice is published by exactly one point"
}

func (*trendPointsTieToFacts) Run(_ context.Context, s *Subject) (Result, error) {
	byID := make(map[string]fact.Fact, len(s.Facts))
	for _, f := range s.Facts {
		byID[f.ID] = f
	}

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
				// A POINT IN A COLUMN THE DOCUMENT DOES NOT DECLARE is invisible
				// to every other arm here, and measured rather than assumed:
				// comparePoint compares a point only against ITS OWN FACT, so a
				// point whose every field matches a real fact passes it; and the
				// reverse sweep below walks factsFor(p.Options), which filters on
				// these same columns, so the fact behind such a point is never
				// even looked at. Append a genuine FY2027 point to a one-column
				// document and both arms stay green over it.
				//
				// It matters because internal/export DROPS such a point when it
				// lays the table out -- there is no column to put it in -- so the
				// figure leaves the published page without anything saying so
				// (fisc-4j5). The packager now refuses that document too; this is
				// the half that names WHICH point, and which is reached first
				// because `fisc verify` runs before `fisc export`.
				if col := (project.Column{FiscalYear: pt.FiscalYear, Basis: mapping.Basis(pt.Basis)}); !declared[col] {
					findings = append(findings, finding(
						fmt.Sprintf("%s %s FY%d %s", p.Name, series.SeriesID, pt.FiscalYear, pt.Basis),
						"this point publishes FY%d %s, which is not one of the %d columns %q "+
							"declares; no page can draw a column the document does not publish, "+
							"so the figure would be dropped",
						pt.FiscalYear, pt.Basis, len(p.Options.Columns), p.Name))
				}
			}
		}
		// The other direction: what the slice carries that the document does
		// not. Iterating the facts the projection was BUILT over rather than the
		// whole store is what keeps this a statement about the document rather
		// than about every schedule in the corpus.
		for _, f := range factsFor(s.Facts, p.Options) {
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
			plural(len(s.trendDocuments()), "document", "documents")),
		nothing:  "no projection built a trends document, so no point has been compared",
		findings: findings,
	}.result(), nil
}

// comparePoint checks one point against the fact it names.
//
// The fields compared are exactly the ones the point PUBLISHES, which is the
// rule that keeps this from drifting into a partial check: a point carries an
// amount, an id and four provenance fields, and every one of them is a claim a
// reader can act on. Comparing the amount alone would let a point cite the right
// figure off the wrong page.
func comparePoint(p projection, series project.Series, pt project.Point,
	byID map[string]fact.Fact, published map[string]bool,
) []Finding {
	subject := fmt.Sprintf("%s %s FY%d %s", p.Name, series.SeriesID, pt.FiscalYear, pt.Basis)

	f, ok := byID[pt.FactID]
	if !ok {
		return []Finding{finding(subject,
			"this point cites fact %s, which is not in the fact store; a citation the "+
				"reader cannot follow is the defect this project exists to prevent", pt.FactID)}
	}
	// A fact published twice would be counted twice by anything summing the
	// document, and the two points need not even agree — this is the shape a
	// duplicated series takes.
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
		report("column", fmt.Sprintf("FY%d %s", pt.FiscalYear, pt.Basis), fyBasis(f))
	}
	// The series id is recomputed from the fact rather than trusted, because it
	// is what groups the four points into one printed row. A point filed under
	// the wrong series is a row that reads as another row's history, and every
	// field above would still agree.
	if got := f.SeriesID(); got != series.SeriesID {
		out = append(out, finding(subject,
			"this point is published under series %s, but fact %s recomputes to series %s; "+
				"it is filed under a different printed row than the one it came from",
			series.SeriesID, f.ID, got))
	}
	return out
}

// fyBasis names a fact's column the way a finding should.
func fyBasis(f fact.Fact) string { return fmt.Sprintf("FY%d %s", f.FiscalYear, f.Basis) }

// incompleteSeries are the series a trends document legitimately cannot fill in
// every column, each with the reason.
//
// It is the same kind of declaration as unprojectedScopes and
// uncheckedDocuments, and it carries the same danger: an entry is a human saying
// a gap is real, and an entry that outlives its reason is an exemption nobody
// can see expiring. staleSeriesDeclarations is the branch that refuses that.
//
// IT IS EMPTY TODAY AND THAT IS THE STATE TO KEEP IT IN. Measured on the
// committed store: all 231 revenue-by-fund series carry a point in all four
// columns. The check is green the day it lands, which is the point — it exists
// for the day the city drops a printed row from one column or adds a line
// mid-book that only the later years carry, neither of which any other check can
// see, because a missing point publishes nothing to disagree with.
//
// The key is the series id, because that is what identifies a printed row across
// columns and is recomputable by anyone holding facts.jsonl.
var incompleteSeries = map[string]string{}

// trendSeriesAreComplete asserts every series has a point in every column.
//
// SEPARATE FROM trend-points-tie-to-facts ON PURPOSE: one check is about VALUES
// and one about SHAPE, and a combined check would report "N of M" over two
// different units — points and series — which is a summary a reader cannot act
// on.
//
// WHAT IT CATCHES AND WHAT IT DOES NOT, because the difference is easy to
// overstate and a first draft of this comment did. The columns compared against
// are the ones the projection was BUILT over -- Options.Columns -- and
// Trends.Slices derives those from the facts that survive, so they and the
// document's published columns are the same list by construction. That means:
//
//   - A ROW dropped from ONE column is caught. The other series still carry that
//     column, so it is still in Options.Columns, and the short series is short a
//     point in a known position. This is fisc-d5n's case and the one that
//     actually arrives when a city reprints a schedule.
//   - A WHOLE COLUMN vanishing from the corpus is NOT caught here. Every series
//     loses it together, Slices returns one column fewer, and every series is
//     complete over what remains. Nothing in this check compares the corpus
//     against what the corpus used to hold, and inventing an expected column
//     list to compare against would be a constant nobody could justify off a
//     page. Filed as fisc-7dt.
type trendSeriesAreComplete struct{}

var _ Check = (*trendSeriesAreComplete)(nil)

func (*trendSeriesAreComplete) ID() string { return "trend-series-are-complete" }
func (*trendSeriesAreComplete) Tier() int  { return 1 }
func (*trendSeriesAreComplete) Full() bool { return false }
func (*trendSeriesAreComplete) Description() string {
	return "every series in a trends document has a point in every column the document was " +
		"built over, or the gap is declared with its reason"
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
			if _, ok := incompleteSeries[sr.SeriesID]; ok {
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
	findings = append(findings, staleSeriesDeclarations(seen, declared)...)

	held := fmt.Sprintf("%d series, each with a point in every column its document covers", series)
	if len(declared) > 0 {
		held = fmt.Sprintf("%d series, each with a point in every column its document covers, "+
			"or a declared gap: %s", series, describeIncomplete(declared))
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
		have[project.Column{FiscalYear: p.FiscalYear, Basis: mapping.Basis(p.Basis)}] = true
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
// Same shape and same argument as staleDeclarations over unprojectedScopes: a
// declaration nobody can see expiring is a declaration that outlives its reason.
func staleSeriesDeclarations(seen map[string]bool, declared map[string]int) []Finding {
	var out []Finding
	for _, id := range sortedStrings(incompleteSeries) {
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
func describeIncomplete(byID map[string]int) string {
	out := make([]string, 0, len(byID))
	for _, id := range sortedStrings(byID) {
		out = append(out, fmt.Sprintf("%s (%s)", id, incompleteSeries[id]))
	}
	return joinComma(out)
}
