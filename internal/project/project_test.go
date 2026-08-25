package project

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

func TestOptionsValidate(t *testing.T) {
	ok := Options{
		Columns: []Column{{FiscalYear: 2026, Basis: mapping.BasisAdopted}},
		Scope:   "all-funds-gross",
		Version: "dev",
	}
	col := func(o Options, year int, basis mapping.Basis) Options {
		o.Columns = []Column{{FiscalYear: year, Basis: basis}}
		return o
	}

	cases := []struct {
		name string
		o    Options
		want string // substring of the expected error; "" means valid
	}{
		{"valid", ok, ""},
		{"no columns", Options{Scope: ok.Scope, Version: ok.Version}, "at least one column is required"},
		{"no fiscal year", col(ok, 0, mapping.BasisAdopted), "fiscal year is required"},
		{"negative fiscal year", col(ok, -1, mapping.BasisAdopted), "fiscal year is required"},
		{"no basis", col(ok, 2026, ""), `basis "" is not one of`},
		{"unknown basis", col(ok, 2026, "guessed"), `basis "guessed" is not one of`},
		// A repeated column is refused because anything summing the document
		// would count it twice -- the same doubling Options exists to prevent,
		// reached from inside one document instead of across two.
		{"duplicate column", func() Options {
			o := ok
			o.Columns = []Column{{2026, mapping.BasisAdopted}, {2026, mapping.BasisAdopted}}
			return o
		}(), "column FY2026 adopted is listed twice"},
		// Several DISTINCT columns are valid at this level. Options is the type
		// a trends document shares with a Sankey; refusing more than one here
		// would make the multi-column document unrepresentable, and it is
		// Sankey.Graph that refuses to be of two.
		{"several columns", func() Options {
			o := ok
			o.Columns = []Column{{2026, mapping.BasisAdopted}, {2027, mapping.BasisAdopted}}
			return o
		}(), ""},
		{"no scope", func() Options { o := ok; o.Scope = ""; return o }(), "scope is required"},
		{"no version", func() Options { o := ok; o.Version = ""; return o }(), "version is required"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.o.Validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("got %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("got no error, want one containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestOptionsValidateAcceptsEveryBasis keeps Validate honest about the enum:
// adding a basis to internal/mapping without adding it here would silently
// make that basis unprojectable.
func TestOptionsValidateAcceptsEveryBasis(t *testing.T) {
	for _, b := range []mapping.Basis{
		mapping.BasisAdopted, mapping.BasisRevised, mapping.BasisActual,
		mapping.BasisAudited, mapping.BasisProjected,
	} {
		o := Options{
			Columns: []Column{{FiscalYear: 2026, Basis: b}},
			Scope:   "all-funds-gross",
			Version: "dev",
		}
		if err := o.Validate(); err != nil {
			t.Errorf("basis %q: got %v, want no error", b, err)
		}
	}
}

func TestRegistry(t *testing.T) {
	got := Registry(nil)
	// The names are asserted rather than the count alone, and in order: the
	// first is the document the site opens on, and a registry that quietly
	// reordered would move which document `fisc export` writes to data/sankey.json.
	want := []string{PublishedProjection, TrendsProjection}
	if len(got) != len(want) {
		t.Fatalf("got %d projections, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name() != name {
			t.Errorf("projection %d is %q, want %q", i, got[i].Name(), name)
		}
	}
	// Two calls must not hand back the same instance: a caller that sets a
	// label registry on one must not be configuring everybody else's.
	if Registry(nil)[0] == got[0] {
		t.Error("got the same projection instance twice, want a fresh one per call")
	}
}

func TestFiscalYearLabel(t *testing.T) {
	cases := map[int]string{2026: "FY 2025-26", 2027: "FY 2026-27", 2000: "FY 1999-00", 2100: "FY 2099-00"}
	for year, want := range cases {
		if got := fiscalYearLabel(year); got != want {
			t.Errorf("fiscalYearLabel(%d) = %q, want %q", year, got, want)
		}
	}
}

func TestDollars(t *testing.T) {
	cases := map[int64]string{
		5961273400: "$59,612,734",
		0:          "$0",
		-100:       "-$1",
		123:        "$1.23",
	}
	for cents, want := range cases {
		if got := dollars(cents); got != want {
			t.Errorf("dollars(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestSlugLabel(t *testing.T) {
	cases := map[string]string{
		"revenue/taxes/property":         "Property",
		"expenditure/wages-and-benefits": "Wages And Benefits",
		"fund-group/internal-service":    "Internal Service",
		"":                               "",
	}
	for id, want := range cases {
		if got := slugLabel(id); got != want {
			t.Errorf("slugLabel(%q) = %q, want %q", id, got, want)
		}
	}
}
