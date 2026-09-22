package project

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// trendColumnsFixture are the four columns Budget Book pp.127-140 print, with
// the real year-to-basis pairing: one actual, one revised, two adopted.
//
// The pairing is the reason Column carries both components. Two of these four
// share a basis and no two share a year, so neither field alone identifies a
// column.
var trendColumnsFixture = []Column{
	{FiscalYear: 2024, Basis: mapping.BasisActual},
	{FiscalYear: 2025, Basis: mapping.BasisRevised},
	{FiscalYear: 2026, Basis: mapping.BasisAdopted},
	{FiscalYear: 2027, Basis: mapping.BasisAdopted},
}

// trendFacts renders one printed row across the four columns, in a fund.
func trendFacts(t *testing.T, rule, label string, fund int, group string, cents [4]int64) []fact.Fact {
	t.Helper()
	row := mapping.Row{Category: "taxes/property"}
	rowPath := fact.RowPath(row)
	columnPath := fact.ColumnPath(mapping.Column{FundGroup: group, Fund: fund}, TrendsScope)
	out := make([]fact.Fact, 0, len(trendColumnsFixture))
	for i, c := range trendColumnsFixture {
		out = append(out, fact.Fact{
			ID: fact.MakeID(testDoc, rule, rowPath, label, columnPath,
				c.FiscalYear, c.Basis),
			DocID:       testDoc,
			Page:        127,
			Offset:      100*(fund%1000) + 10*i,
			Token:       "x",
			RuleID:      rule,
			Kind:        mapping.KindRevenue,
			Basis:       c.Basis,
			Scope:       TrendsScope,
			FiscalYear:  c.FiscalYear,
			RowPath:     rowPath,
			RowLabel:    label,
			Category:    "taxes/property",
			ColumnPath:  columnPath,
			FundGroup:   group,
			Fund:        fact.FundNumber(fund),
			Sign:        mapping.SignPositive,
			Units:       "dollars",
			AmountCents: cents[i],
		})
	}
	return out
}

// trendsFixture is two funds printing the same row label, which is the case the
// document exists to disambiguate.
func trendsFixture(t *testing.T) []fact.Fact {
	t.Helper()
	var out []fact.Fact
	out = append(out, trendFacts(t, "gf-rev", "Property Taxes", 100, "general",
		[4]int64{100, 200, 300, 400})...)
	out = append(out, trendFacts(t, "sr-rev", "Property Taxes", 310, "special-revenue",
		[4]int64{10, 20, 30, 40})...)
	return out
}

func trendsOptions() Options {
	return Options{Columns: trendColumnsFixture, Scopes: []string{TrendsScope}, Version: "test"}
}

func buildTrends(t *testing.T, facts []fact.Fact) *TrendsDocument {
	t.Helper()
	tr := &Trends{Labels: stubFunds{
		stubLabels: stubLabels{"taxes/property": "Property Taxes"},
		funds:      map[int]string{100: "General Fund", 310: "Livermore Area Rec Park"},
	}}
	d, err := tr.Document(facts, trendsOptions())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	return d
}

// TestTrendsGroupsPrintedRowsAcrossColumns is the document's central claim: four
// printed cells of one row become one series with four points, and two funds
// printing the same words stay two series.
//
// THE SECOND HALF IS WHY THE SERIES ID IS NOT THE ROW LABEL. "Property Taxes" is
// printed by four funds in the real schedule and "Use of Money & Prop" by
// thirty-eight; a document keyed on the label alone would fold them into one
// series and publish a sum the city never printed.
func TestTrendsGroupsPrintedRowsAcrossColumns(t *testing.T) {
	d := buildTrends(t, trendsFixture(t))

	if len(d.Series) != 2 {
		t.Fatalf("got %d series, want 2 -- one per (fund, printed row)", len(d.Series))
	}
	for _, s := range d.Series {
		if len(s.Points) != 4 {
			t.Errorf("series %s (fund %s) has %d points, want 4", s.SeriesID, fact.FundString(s.Fund), len(s.Points))
		}
	}
	if d.Series[0].SeriesID == d.Series[1].SeriesID {
		t.Fatal("two funds printing the same row label share a series id")
	}
	if !fact.SameFund(d.Series[0].Fund, fact.FundNumber(100)) ||
		!fact.SameFund(d.Series[1].Fund, fact.FundNumber(310)) {
		t.Errorf("funds = %s, %s; want them in fund order",
			fact.FundString(d.Series[0].Fund), fact.FundString(d.Series[1].Fund))
	}
	if got := d.Series[0].FundName; got != "General Fund" {
		t.Errorf("fund_name = %q, want the registry's name -- the row label alone is ambiguous", got)
	}
	if got := d.Series[0].CategoryLabel; got != "Property Taxes" {
		t.Errorf("category_label = %q, want the taxonomy's words", got)
	}
}

// TestTrendsPointsFollowTheDocumentsColumnOrder pins what makes a missing point
// detectable: the points are in the order the OPTIONS declare, so a series short
// a column is short a point in a known position rather than merely shorter.
func TestTrendsPointsFollowTheDocumentsColumnOrder(t *testing.T) {
	d := buildTrends(t, trendsFixture(t))
	for _, s := range d.Series {
		for i, p := range s.Points {
			want := trendColumnsFixture[i]
			if p.FiscalYear != want.FiscalYear || p.Basis != string(want.Basis) {
				t.Errorf("series %s point %d is FY%d %s, want %s",
					s.SeriesID, i, p.FiscalYear, p.Basis, want)
			}
		}
	}
}

// TestTrendsSeriesIDIsTheFactIDWithoutTheColumn is the contract's identity claim
// checked against the code rather than quoted: a series id is fact.MakeID's
// tuple minus the fiscal year and the basis, so every point of a series
// recomputes to the same one and no fact id can be mistaken for it.
func TestTrendsSeriesIDIsTheFactIDWithoutTheColumn(t *testing.T) {
	facts := trendsFixture(t)
	d := buildTrends(t, facts)

	byID := map[string]fact.Fact{}
	for _, f := range facts {
		byID[f.ID] = f
	}
	for _, s := range d.Series {
		for _, p := range s.Points {
			f := byID[p.FactID]
			if got := f.SeriesID(); got != s.SeriesID {
				t.Errorf("fact %s recomputes to series %s, published under %s",
					f.ID, got, s.SeriesID)
			}
			if strings.HasPrefix(s.SeriesID, fact.IDPrefix) {
				t.Errorf("series id %q carries the FACT prefix; the two must be "+
					"distinguishable on sight", s.SeriesID)
			}
		}
	}
}

// TestTrendsIsDeterministic is what lets a rebuild-and-diff mean anything.
func TestTrendsIsDeterministic(t *testing.T) {
	facts := trendsFixture(t)
	tr := &Trends{}
	first, err := tr.Build(facts, trendsOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := tr.Build(facts, trendsOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(first) != string(second) {
		t.Error("two builds over the same facts differ")
	}
}

// TestTrendsPublishesEveryKey guards the no-omitempty rule the whole project
// runs on: a key that vanishes when it is empty makes a diff between two
// releases read as a structural change.
func TestTrendsPublishesEveryKey(t *testing.T) {
	// A fund the stub does not know and a category the stub does not label, so
	// both fall back to empty rather than being dropped.
	facts := trendFacts(t, "unknown", "Mystery Row", 999, "capital", [4]int64{0, 0, 0, 0})
	tr := &Trends{}
	raw, err := tr.Build(facts, trendsOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Error("the document contains a null; absent strings are \"\" and absent slices []")
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta := doc["metadata"].(map[string]any)
	for _, key := range []string{"generated_by", "scope", "currency", "units", "columns",
		"sources", "counts", "caveats"} {
		if _, ok := meta[key]; !ok {
			t.Errorf("metadata has no %q", key)
		}
	}
	// The four the spine carries and this document must NOT, because it spans
	// four years and three bases and could fill none of them honestly.
	for _, key := range []string{"fiscal_year", "fiscal_year_label", "basis", "headline"} {
		if _, ok := meta[key]; ok {
			t.Errorf("metadata carries %q, which is the spine's and singular; publishing it "+
				"zeroed to keep a familiar shape is absent-is-not-zero exactly", key)
		}
	}
	series := doc["series"].([]any)[0].(map[string]any)
	for _, key := range []string{"series_id", "label", "fund", "fund_name", "fund_group",
		"kind", "category", "category_label", "points"} {
		if _, ok := series[key]; !ok {
			t.Errorf("series has no %q", key)
		}
	}
	point := series["points"].([]any)[0].(map[string]any)
	for _, key := range []string{"fiscal_year", "basis", "amount_cents", "fact_id", "doc_id",
		"page", "offset", "token", "derived"} {
		if _, ok := point[key]; !ok {
			t.Errorf("point has no %q", key)
		}
	}
}

// TestTrendsKeepsContraRowsSigned is the difference from the Sankey, stated as a
// test because it is a deliberate divergence rather than an oversight: the spine
// nets a contra row into its parent category before drawing a link, and this
// document does not, because a series is a printed ROW and ERAF is printed.
func TestTrendsKeepsContraRowsSigned(t *testing.T) {
	facts := trendFacts(t, "gf-rev", "ERAF", 100, "general",
		[4]int64{-1_408_643_800, -1_466_183_600, -1_517_500_000, -1_585_787_500})
	for i := range facts {
		facts[i].Sign = mapping.SignContra
	}
	d := buildTrends(t, facts)
	if len(d.Series) != 1 {
		t.Fatalf("got %d series, want 1", len(d.Series))
	}
	if got := d.Series[0].Points[2].AmountCents; got != -1_517_500_000 {
		t.Errorf("ERAF FY2026 = %d, want the printed negative -1517500000", got)
	}
}

// TestTrendsSlicesTakeTheWholeScope is the constraint fisc-rmw imposes on this
// projection, checked rather than trusted.
//
// A document covering PART of a declared scope publishes part of a schedule and
// says nothing about the rest, which no reader of the document can tell from a
// schedule the city printed at that width. So this projection must return ONE
// slice carrying EVERY column the store has -- not the two adopted years the
// spine prints a column for, which is the obvious first cut.
func TestTrendsSlicesTakeTheWholeScope(t *testing.T) {
	facts := trendsFixture(t)
	got := (&Trends{}).Slices(facts, "test")
	if len(got) != 1 {
		t.Fatalf("got %d slices, want exactly 1 -- a trend of four columns is one document", len(got))
	}
	if got[0].ScopeList() != TrendsScope {
		t.Errorf("scope = %q, want %q", got[0].ScopeList(), TrendsScope)
	}
	if !reflect.DeepEqual(got[0].Columns, trendColumnsFixture) {
		t.Errorf("columns = %v, want all four in printed order %v", got[0].Columns, trendColumnsFixture)
	}
}

// TestTrendsSlicesAreEmptyWithoutItsSchedule: a store carrying nothing of this
// schedule is a statement about the corpus, not an error. The spine-only
// fixtures throughout this repository are exactly that store.
func TestTrendsSlicesAreEmptyWithoutItsSchedule(t *testing.T) {
	if got := (&Trends{}).Slices(spineFacts(t, testYear), "test"); len(got) != 0 {
		t.Errorf("got %d slices over a spine-only store, want none", len(got))
	}
}

// TestTrendsRefusesAnotherSchedulesScope guards the doubling the scope axis
// exists to prevent, from this document's side.
func TestTrendsRefusesAnotherSchedulesScope(t *testing.T) {
	o := trendsOptions()
	o.Scopes = []string{PublishedScope}
	if _, err := (&Trends{}).Document(trendsFixture(t), o); err == nil {
		t.Fatal("Document over the spine's scope = nil error, want a refusal")
	}
}

// TestTrendsRefusesASeriesThatIsTwoRows is the fail-closed arm on the identity.
//
// If two different printed rows ever hashed to one series id, the document would
// publish a series whose label described half of it and whose points summed
// across two rows. That must be an error, not a document.
func TestTrendsRefusesASeriesThatIsTwoRows(t *testing.T) {
	facts := trendsFixture(t)
	// Same series tuple, different fund: only reachable by corrupting the store,
	// which is the point -- the refusal is what makes the id's uniqueness a
	// checked property rather than an assumption.
	facts[1].Fund = fact.FundNumber(999)
	_, err := buildTrendsErr(t, facts)
	if err == nil {
		t.Fatal("Document = nil error over a series carrying two different rows")
	}
	if !strings.Contains(err.Error(), "two different rows") {
		t.Errorf("error %v does not say what is wrong", err)
	}
}

func buildTrendsErr(t *testing.T, facts []fact.Fact) (*TrendsDocument, error) {
	t.Helper()
	return (&Trends{}).Document(facts, trendsOptions())
}
