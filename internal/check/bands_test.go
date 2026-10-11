package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// band returns f restated as a band from its own year through through, with
// the id the band's fields hash to.
func band(f fact.Fact, through int) fact.Fact {
	f.FiscalYearThrough = &through
	f.ID = fact.MakeIDThrough(f.DocID, f.RuleID, f.RowPath, f.RowLabel, f.ColumnPath,
		f.FiscalYear, f.FiscalYearThrough, f.Basis)
	return f
}

// TestAnEditedBandNoLongerAddressesItself holds a band's last year to its id.
// The parser ties fiscal_year_through to the printed label at build, and
// nothing at verify re-reads the label, so a facts.jsonl line edited from
// 2031-2035 to 2031-2034 would otherwise pass every fact-store check.
//
// Mutation: hash MakeIDThrough's tuple without the band end and this goes
// green over the edited line.
func TestAnEditedBandNoLongerAddressesItself(t *testing.T) {
	t.Parallel()
	facts := testFacts()
	facts[0] = band(facts[0], facts[0].FiscalYear+4)
	if res := resultFor(t, runChecks(t, testSubject(t, facts...)), "fact-ids-recompute"); res.Status != StatusPass {
		t.Fatalf("an unedited band: status = %s, want pass: %+v", res.Status, res.Findings)
	}
	edited := facts[0].FiscalYear + 3
	facts[0].FiscalYearThrough = &edited
	res := resultFor(t, runChecks(t, testSubject(t, facts...)), "fact-ids-recompute")
	if res.Status != StatusFail {
		t.Fatalf("an edited band: status = %s, want fail", res.Status)
	}
	if got := res.Findings[0].Detail; !strings.Contains(got, "FY") || !strings.Contains(got, "-FY") {
		t.Errorf("finding %q does not name the band whole", got)
	}
}

// TestABandIsNeverKeyedAsItsFirstYear feeds a band and the one-year figure
// sharing its first year to every store-wide path that keys a fact by its
// column, and holds each to telling them apart: a key of (fiscal year, basis)
// alone would file the band's five years under FY2031.
//
// Mutation: drop fact.BandOf from any one of the keys below and its arm goes
// red.
func TestABandIsNeverKeyedAsItsFirstYear(t *testing.T) {
	t.Parallel()
	year := testFacts()[0]
	b := band(year, year.FiscalYear+4)
	b.RowLabel += " (band)"

	if project.ColumnOf(&year) == project.ColumnOf(&b) {
		t.Error("project.ColumnOf files a band under its first year")
	}
	if got, want := project.ColumnOf(&b).String(), "-FY"; !strings.Contains(got, want) {
		t.Errorf("a band column prints as %q, which names one year", got)
	}
	if structure.BalanceAtOf(&year) == structure.BalanceAtOf(&b) {
		t.Error("structure.BalanceAtOf files a band under its first year")
	}

	spineYear, spineBand := year, b
	spineYear.Scope, spineBand.Scope = spineScope, spineScope
	slices := factSlices([]fact.Fact{spineYear, spineBand}, "test")
	if len(slices) != 2 {
		t.Fatalf("factSlices over a year and a band starting in it = %d slices, want 2", len(slices))
	}
	if slices[0].Columns[0].Through != 0 || slices[1].Columns[0].Through != *b.FiscalYearThrough {
		t.Errorf("factSlices columns = %v and %v, want the year then the band",
			slices[0].Columns[0], slices[1].Columns[0])
	}

	// SelectFacts asked for the band's first year as a year refuses the band
	// rather than passing over it, and selects it only when named as a band.
	o := project.Options{Columns: []project.Column{{FiscalYear: year.FiscalYear, Basis: year.Basis}},
		Scopes: []string{year.Scope}, Version: "test"}
	if _, err := project.SelectFacts([]fact.Fact{year, b}, o); err == nil ||
		!strings.Contains(err.Error(), "a band") {
		t.Errorf("SelectFacts asked for FY%d as a year over its band = %v, want the refusal",
			year.FiscalYear, err)
	}
	o.Columns = []project.Column{project.ColumnOf(&b)}
	got, err := project.SelectFacts([]fact.Fact{year, b}, o)
	if err != nil || len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("SelectFacts over the band column = %d facts, %v; want the band alone", len(got), err)
	}
}
