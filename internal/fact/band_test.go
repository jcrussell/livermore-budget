package fact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestARowsYearsAreTheFactsAndABandIsHashed: a row's years file its figure, a
// band publishes its last year beside its first, and the band end is hashed.
// Mutation: drop MakeIDThrough's through arm and the band's id equals its
// one-year twin's.
func TestARowsYearsAreTheFactsAndABandIsHashed(t *testing.T) {
	f := &mapping.File{DocID: "doc"}
	rule := &mapping.Rule{ID: "rule", Basis: vocab.BasisProjected, Scope: "s"}
	col := mapping.Column{Category: "debt-services/principal"}
	year := mapping.Row{Label: "2026", FiscalYear: 2026}
	band := mapping.Row{Label: "2031-2035", FiscalYear: 2031, FiscalYearThrough: 2035}
	got, err := FromValues(f, rule, []mapping.Value{{Row: year, Column: col}, {Row: band, Column: col}})
	if err != nil {
		t.Fatalf("FromValues: %v", err)
	}
	if got[0].FiscalYear != 2026 || got[0].FiscalYearThrough != nil {
		t.Errorf("the year row published FY%d through %v, want 2026 and null",
			got[0].FiscalYear, got[0].FiscalYearThrough)
	}
	if got[1].FiscalYear != 2031 || BandOf(&got[1]) != 2035 {
		t.Errorf("the band published FY%d through %d, want 2031 through 2035",
			got[1].FiscalYear, BandOf(&got[1]))
	}
	if want := MakeID("doc", "rule", "debt-services/principal", "2026", "s", 2026,
		vocab.BasisProjected); got[0].ID != want {
		t.Errorf("a one-year id = %s, want MakeID's %s", got[0].ID, want)
	}
	if twin := MakeID("doc", "rule", "debt-services/principal", "2031-2035", "s", 2031,
		vocab.BasisProjected); got[1].ID == twin {
		t.Error("the band's id leaves its last year out of the hash")
	}
	if got := BandLabel(2031, 2035, vocab.BasisProjected); got != "FY2031-FY2035 projected" {
		t.Errorf("BandLabel = %q", got)
	}

	// Every line carries the key: null for a year, the last year for a band.
	var b bytes.Buffer
	if err := Write(&b, got); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if !strings.Contains(lines[0], `"fiscal_year":2026,"fiscal_year_through":null,`) ||
		!strings.Contains(lines[1], `"fiscal_year":2031,"fiscal_year_through":2035,`) {
		t.Errorf("records:\n%s", b.String())
	}
}
