package structure_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// yearRecord is one hand-built fact: enough of a [structure.Record] to be
// told apart by id and partitioned by year.
func yearRecord(id string, year int) structure.Record {
	return structure.Record{
		ID: id, DocID: "doc", Page: 1, Offset: len(id), Token: "1", RuleID: "r",
		Kind: mapping.KindRevenue, Basis: mapping.BasisAdopted, Scope: "s",
		FiscalYear: year, FundGroup: "g", Department: "d", Category: "c",
		RowLabel: id, Sign: mapping.SignPositive, AmountCents: 100,
	}
}

// wholeByYear is a document over three years in which one view spans every
// year and the other admits nothing before 2026, so that one part omits a
// view and two carry both; and in which the two views admit different
// subsets, so that re-indexing is visible per view rather than per part.
func wholeByYear() structure.Document {
	return structure.Document{
		SchemaVersion: structure.DocumentSchemaVersion,
		Levels:        []structure.LevelDecl{{Name: "fund", Axes: []structure.Axis{"fund"}, Refines: []structure.Level{"fund-group"}}},
		Cuts:          []structure.CutDecl{{Name: "cut", Scope: "s", Level: "fund", Kinds: []mapping.Kind{mapping.KindRevenue}, Bases: []mapping.Basis{mapping.BasisAdopted}}},
		Identities:    []structure.IdentityDecl{{Name: "id", A: "cut", B: "cut", Kinds: []mapping.Kind{mapping.KindRevenue}, Reason: "hand-built"}},
		Views: []structure.ViewDecl{
			{Name: "all", Scopes: []string{"s"}, Cuts: []string{"cut"}, Readings: map[string]string{"id": "cut"}, Facts: []int{0, 1, 2, 3, 4, 5}},
			{Name: "late", Scopes: []string{"s"}, Cuts: []string{"cut"}, Facts: []int{1, 3, 4, 5, 6}},
		},
		Facts: []structure.Record{
			yearRecord("a24", 2024), yearRecord("b26", 2026), yearRecord("c24", 2024),
			yearRecord("d27", 2027), yearRecord("e26", 2026), yearRecord("f27", 2027),
			yearRecord("g26", 2026),
		},
	}
}

// idsOf is the records a view names, by id, in the view's order.
func idsOf(t *testing.T, d structure.Document, v structure.ViewDecl) []string {
	t.Helper()
	out := []string{}
	for _, at := range v.Facts {
		if at < 0 || at >= len(d.Facts) {
			t.Fatalf("view %q of %d names fact %d, outside 0..%d", v.Name, d.FiscalYear, at, len(d.Facts)-1)
		}
		out = append(out, d.Facts[at].ID)
	}
	return out
}

func TestPartitionByYearIsOnePartPerYearWithTheViewsReindexed(t *testing.T) {
	whole := wholeByYear()
	got, err := structure.PartitionByYear(whole)
	if err != nil {
		t.Fatal(err)
	}
	scaffold := func(year int) structure.Document {
		return structure.Document{SchemaVersion: whole.SchemaVersion, FiscalYear: year, Levels: whole.Levels, Cuts: whole.Cuts, Identities: whole.Identities}
	}
	all := whole.Views[0]
	late := whole.Views[1]
	p2024, p2026, p2027 := scaffold(2024), scaffold(2026), scaffold(2027)
	p2024.Facts = []structure.Record{whole.Facts[0], whole.Facts[2]}
	p2024.Views = []structure.ViewDecl{{Name: all.Name, Scopes: all.Scopes, Cuts: all.Cuts, Readings: all.Readings, Facts: []int{0, 1}}}
	p2026.Facts = []structure.Record{whole.Facts[1], whole.Facts[4], whole.Facts[6]}
	p2026.Views = []structure.ViewDecl{
		{Name: all.Name, Scopes: all.Scopes, Cuts: all.Cuts, Readings: all.Readings, Facts: []int{0, 1}},
		{Name: late.Name, Scopes: late.Scopes, Cuts: late.Cuts, Facts: []int{0, 1, 2}},
	}
	p2027.Facts = []structure.Record{whole.Facts[3], whole.Facts[5]}
	p2027.Views = []structure.ViewDecl{
		{Name: all.Name, Scopes: all.Scopes, Cuts: all.Cuts, Readings: all.Readings, Facts: []int{0, 1}},
		{Name: late.Name, Scopes: late.Scopes, Cuts: late.Cuts, Facts: []int{0, 1}},
	}
	want := []structure.Document{p2024, p2026, p2027}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("partition (-want +got):\n%s", diff)
	}

	// THE SAME PROPERTIES AS PROPERTIES, so a change to the hand-built
	// document above fails by name rather than by a diff of the whole.
	carried := map[string]int{}
	for _, p := range got {
		if p.FiscalYear == 0 {
			t.Error("a part carries no fiscal year")
		}
		for _, f := range p.Facts {
			carried[f.ID]++
			if f.FiscalYear != p.FiscalYear {
				t.Errorf("part %d carries %s, which is of %d", p.FiscalYear, f.ID, f.FiscalYear)
			}
		}
		for _, name := range []string{"Levels", "Cuts", "Identities"} {
			var diff string
			switch name {
			case "Levels":
				diff = cmp.Diff(whole.Levels, p.Levels)
			case "Cuts":
				diff = cmp.Diff(whole.Cuts, p.Cuts)
			case "Identities":
				diff = cmp.Diff(whole.Identities, p.Identities)
			}
			if diff != "" {
				t.Errorf("part %d's %s is not the whole's (-whole +part):\n%s", p.FiscalYear, name, diff)
			}
		}
		for _, v := range p.Views {
			if len(v.Facts) == 0 {
				t.Errorf("part %d ships view %q with no fact", p.FiscalYear, v.Name)
			}
		}
	}
	for _, f := range whole.Facts {
		if carried[f.ID] != 1 {
			t.Errorf("%s is carried by %d parts", f.ID, carried[f.ID])
		}
	}
	// EACH PART'S VIEW NAMES THE WHOLE'S VIEW'S RECORDS OF THAT YEAR, by id
	// and in the whole's order; and a view with none of that year is absent
	// from the part, not present and empty.
	for _, p := range got {
		byName := map[string]structure.ViewDecl{}
		for _, v := range p.Views {
			byName[v.Name] = v
		}
		for _, v := range whole.Views {
			wantIDs := []string{}
			for _, id := range idsOf(t, whole, v) {
				for _, f := range p.Facts {
					if f.ID == id {
						wantIDs = append(wantIDs, id)
					}
				}
			}
			pv, present := byName[v.Name]
			if len(wantIDs) == 0 {
				if present {
					t.Errorf("part %d carries view %q, which admits no fact of %d", p.FiscalYear, v.Name, p.FiscalYear)
				}
				continue
			}
			if !present {
				t.Errorf("part %d omits view %q, which admits %v of %d", p.FiscalYear, v.Name, wantIDs, p.FiscalYear)
				continue
			}
			if diff := cmp.Diff(wantIDs, idsOf(t, p, pv)); diff != "" {
				t.Errorf("part %d view %q (-whole restricted +part):\n%s", p.FiscalYear, v.Name, diff)
			}
		}
	}
	// THE FIXTURE SUPPLIES BOTH SHAPES, or the omission arm above never ran.
	var omits, carries int
	for _, p := range got {
		if len(p.Views) < len(whole.Views) {
			omits++
		} else {
			carries++
		}
	}
	if omits == 0 || carries == 0 {
		t.Errorf("%d part(s) omit a view and %d carry every view; both are needed for the omission to be witnessed", omits, carries)
	}
}

func TestPartitionByYearRefusesWhatItCannotAddress(t *testing.T) {
	cases := []struct {
		name    string
		perturb func(*structure.Document)
		want    []string
	}{
		{"a fact of no year", func(d *structure.Document) { d.Facts[4].FiscalYear = 0 }, []string{"fact 4 (e26)", "no fiscal year"}},
		{"no facts at all", func(d *structure.Document) { d.Facts = nil; d.Views = nil }, []string{"no fact"}},
		// A year every view ignores: its part would be the scaffolding and
		// nothing else, which is a file that says the year exists and
		// carries no figure of it.
		{"a year no view admits", func(d *structure.Document) { d.Facts = append(d.Facts, yearRecord("h25", 2025)) }, []string{"no view admits a fact of 2025"}},
		{"a view naming a fact outside the list", func(d *structure.Document) { d.Views[1].Facts = append(d.Views[1].Facts, 99) }, []string{`view "late" names fact 99`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := wholeByYear()
			tc.perturb(&d)
			_, err := structure.PartitionByYear(d)
			if err == nil {
				t.Fatalf("%s was partitioned rather than refused", tc.name)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal does not say %q:\n%v", want, err)
				}
			}
		})
	}
}
