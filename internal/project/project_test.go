package project

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

func TestOptionsValidate(t *testing.T) {
	ok := Options{
		Columns: []Column{{FiscalYear: 2026, Basis: mapping.BasisAdopted}},
		Scopes:  []string{"all-funds-gross"},
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
		{"no columns", Options{Scopes: ok.Scopes, Version: ok.Version}, "at least one column is required"},
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
		{"no scope", func() Options { o := ok; o.Scopes = nil; return o }(),
			"at least one scope is required"},
		{"an empty scope", func() Options { o := ok; o.Scopes = []string{""}; return o }(),
			"a scope may not be empty"},
		// A repeated scope selects the same facts once, so nothing doubles --
		// but every reader that COUNTS scopes would see a document claiming
		// more schedules than it has.
		{"a repeated scope", func() Options {
			o := ok
			o.Scopes = []string{PublishedScope, PublishedScope}
			return o
		}(), "is listed twice"},
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
			Scopes:  []string{"all-funds-gross"},
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
	want := []string{PublishedProjection, TrendsProjection, FundFlowsProjection}
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

// TestTheOpeningPublishedYearOpensTheYearList pins a coherence property of this
// package's own constants, and it is the last edge of fisc-rmx.
//
// Stem gives the bare name to the slice at PublishedFiscalYear, while
// PublishedDocuments walks PublishedFiscalYears(). Nothing made the constant the
// first entry of the list, and editing one without the other is a source change
// no corpus can cause and no test then noticed: with the list at {2027} and the
// constant still 2026, the declaration names a document "sankey" over FY2027
// while every derivation gives that stem to FY2026. `fisc export` does refuse
// it, clearly -- but at the far end of the pipeline, about a file, rather than
// here, about the two lines that disagree.
func TestTheOpeningPublishedYearOpensTheYearList(t *testing.T) {
	years := PublishedFiscalYears()
	if len(years) == 0 {
		t.Fatal("PublishedFiscalYears is empty")
	}
	if years[0] != PublishedFiscalYear {
		t.Errorf("PublishedFiscalYears()[0] = %d, want PublishedFiscalYear (%d): Stem gives "+
			"the bare name to the opening year, so the two must be the same year or the "+
			"site declares one stem and every derivation computes another",
			years[0], PublishedFiscalYear)
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

// TestOnlyScopeRefusesASetItCannotDescribe is the guard that replaced a field
// read.
//
// WHILE Options.Scopes WAS A STRING, "this document is of one schedule" was true
// by construction and nobody had to assert it. A set can hold two, so the three
// places that need a single scope -- Sankey.Graph, Trends.Document and
// envelope() -- each had to grow the same refusal, and the one that forgot would
// publish Scopes[0] into a singular metadata.scope: one schedule named as the
// whole of a document built over two. That is a false claim in a published file,
// and no check reads the JSON closely enough to catch it.
//
// The refusal is written once, here, and the three call it.
func TestOnlyScopeRefusesASetItCannotDescribe(t *testing.T) {
	one := Options{
		Columns: []Column{{FiscalYear: 2026, Basis: mapping.BasisAdopted}},
		Scopes:  []string{PublishedScope},
		Version: "test",
	}
	got, err := one.OnlyScope()
	if err != nil || got != PublishedScope {
		t.Fatalf("OnlyScope over one scope = %q, %v, want %q, nil", got, err, PublishedScope)
	}

	two := one
	two.Scopes = []string{PublishedScope, TrendsScope}
	if _, err := two.OnlyScope(); err == nil {
		t.Fatal("OnlyScope over two scopes = nil error, want a refusal")
	} else {
		// The message has to name BOTH, or a reader cannot tell which
		// declaration handed the wrong options over.
		for _, want := range []string{PublishedScope, TrendsScope} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("got %q, want it to name %q", err, want)
			}
		}
	}

	// Validate does not refuse the set -- a two-scope Options is legal, it is
	// only this document SHAPE that cannot describe one. Asserting that keeps
	// the two refusals from being collapsed into one by a later reader.
	if err := two.Validate(); err != nil {
		t.Errorf("Validate over two scopes = %v, want nil: the set is legal", err)
	}
}

// TestASingleGrainDocumentRefusesTwoScopes is the same guard reached through the
// two projections that publish a singular metadata.scope.
//
// HOW MANY SCHEDULES AND WHICH SCHEDULE ARE DIFFERENT MISTAKES with different
// remedies, so they are two refusals and not one: a set of two means someone
// pointed a single-grain document at a drill-down's options; a set of one that
// is the wrong schedule means they pointed it at the wrong page. Reporting
// either as the other sends the reader to the wrong declaration.
func TestASingleGrainDocumentRefusesTwoScopes(t *testing.T) {
	both := []string{PublishedScope, TrendsScope}

	so := testOptions()
	so.Scopes = both
	if _, err := (&Sankey{}).Graph(spineFacts(t, testYear), so); err == nil {
		t.Error("Sankey.Graph over two scopes = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "one schedule") {
		t.Errorf("Sankey.Graph = %q, want it to say the document is of one schedule", err)
	}

	to := trendsOptions()
	to.Scopes = both
	if _, err := (&Trends{}).Document(trendsFixture(t), to); err == nil {
		t.Error("Trends.Document over two scopes = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "one schedule") {
		t.Errorf("Trends.Document = %q, want it to say the document is of one schedule", err)
	}
}
