package project

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// spendingCell is one printed cell of Budget Book pp.85-125's upper block: a
// division, the object heading its row sits under, and what the page prints.
//
// It carries a division with two object rows, a printed dash, and p124's
// Transfers Out row, the one row that is not an expenditure.
var spendingCells = []struct {
	division string
	category string
	kind     vocab.Kind
	label    string
	page     int
	cents    int64
}{
	{"city-council", "wages-and-benefits", vocab.KindExpenditure, "Wages & Benefits", 85, 7_470_100},
	{"city-council", "services-and-supplies", vocab.KindExpenditure, "Services & Supplies", 85, 7_808_000},
	{"general-services", "wages-and-benefits", vocab.KindExpenditure, "Wages & Benefits", 89, 0},
	{"general-services", "services-and-supplies", vocab.KindExpenditure, "Services & Supplies", 89, 1_200_000},
	{"maintenance", "services-and-supplies", vocab.KindExpenditure, "Services & Supplies", 124, 3_000_000},
	{"maintenance", "transfers/out", vocab.KindTransferOut, "Transfers Out", 124, 26_679_800},
}

// spendingFacts renders the fixture at the scope this document selects, with
// mutate run over each fact.
func spendingFacts(t *testing.T, mutate func(f *fact.Fact)) []fact.Fact {
	t.Helper()
	out := make([]fact.Fact, 0, len(spendingCells))
	for _, c := range spendingCells {
		row := mapping.Row{Category: c.category, Department: c.division, Label: c.label}
		rowPath := fact.RowPath(row)
		columnPath := fact.ColumnPath(mapping.Column{}, DepartmentSpendingScope)
		f := fact.Fact{
			ID: fact.MakeID(testDoc, "dw-"+c.division, rowPath, c.label, columnPath,
				testYear, testBasis),
			DocID:       testDoc,
			Page:        c.page,
			RuleID:      "dw-" + c.division,
			Kind:        c.kind,
			Basis:       testBasis,
			Scope:       DepartmentSpendingScope,
			FiscalYear:  testYear,
			RowPath:     rowPath,
			RowLabel:    c.label,
			Category:    c.category,
			Department:  c.division,
			ColumnPath:  columnPath,
			AmountCents: c.cents,
		}
		if mutate != nil {
			mutate(&f)
		}
		out = append(out, f)
	}
	return out
}

// spendingLabels lists the fixture's divisions, because a division the registry
// does not list is refused rather than drawn under its slug.
func spendingLabels() stubFundFlows {
	return stubFundFlows{stubLabels: goldenLabels, divisions: map[string]string{
		"city-council": "City Council", "general-services": "General Services", "maintenance": "Maintenance"}}
}

func spendingOptions() Options {
	return Options{
		Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
		Scopes:  DepartmentSpendingScopes(),
		Version: "departmentspending_test.go",
	}
}

func buildSpending(t *testing.T, facts []fact.Fact) *Document {
	t.Helper()
	doc, err := (&departmentSpending{Labels: spendingLabels()}).Document(facts, spendingOptions())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	return doc
}

// TestTheCrossTabDrawsOneRibbonPerPrintedCell pins which ribbons exist, their
// direction and kind, and `partition` with the direction: 5 -> 4 without it
// would claim money flows from an object category into a division.
func TestTheCrossTabDrawsOneRibbonPerPrintedCell(t *testing.T) {
	doc := buildSpending(t, spendingFacts(t, nil))

	type ribbon struct {
		source, target string
		cents          int64
		kind           LinkKind
		partition      bool
	}
	got := make([]ribbon, 0, len(doc.Links))
	for _, l := range doc.Links {
		got = append(got, ribbon{l.Source, l.Target, l.ValueCents, l.Kind, l.Partition})
	}
	// general-services' printed dash draws no ribbon; its fact is counted below.
	want := []ribbon{
		{"expenditure/services-and-supplies", "dept/city-council", 7_808_000, KindExternal, true},
		{"expenditure/services-and-supplies", "dept/general-services", 1_200_000, KindExternal, true},
		{"expenditure/services-and-supplies", "dept/maintenance", 3_000_000, KindExternal, true},
		{"expenditure/wages-and-benefits", "dept/city-council", 7_470_100, KindExternal, true},
		// The transfer takes the spine's node id and is not external.
		{"transfers/out", "dept/maintenance", 26_679_800, KindInternalTransfer, true},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(ribbon{})); diff != "" {
		t.Errorf("ribbons (-want +got):\n%s", diff)
	}

	tiers := map[string]int{}
	for _, n := range doc.Nodes {
		tiers[n.ID] = n.Tier
		if n.Parent != "" {
			t.Errorf("node %q is parented to %q; these pages print no fund axis, so both "+
				"ends of every ribbon are parentless", n.ID, n.Parent)
		}
	}
	for id, want := range map[string]int{
		"expenditure/wages-and-benefits":    5,
		"expenditure/services-and-supplies": 5,
		"transfers/out":                     5,
		"dept/city-council":                 4,
		"dept/maintenance":                  4,
	} {
		if tiers[id] != want {
			t.Errorf("node %q is at tier %d, want %d", id, tiers[id], want)
		}
	}
}

// TestTheCrossTabCountsEveryFactOnceOrAsAPrintedZero pins facts = facts_cited +
// facts_uncited, and `facts` against the slice size, since a dropped cell would
// still satisfy the identity.
func TestTheCrossTabCountsEveryFactOnceOrAsAPrintedZero(t *testing.T) {
	doc := buildSpending(t, spendingFacts(t, nil))
	c := doc.Metadata.Counts
	if c.Facts != len(spendingCells) {
		t.Errorf("counts.facts = %d, want %d", c.Facts, len(spendingCells))
	}
	if c.FactsCited+c.FactsUncited != c.Facts {
		t.Errorf("counts.facts = %d, cited + uncited = %d + %d", c.Facts, c.FactsCited,
			c.FactsUncited)
	}
	if c.FactsUncited != 1 {
		t.Errorf("counts.facts_uncited = %d, want 1: general-services' Wages & Benefits "+
			"dash is the only cell this fixture prints as zero", c.FactsUncited)
	}
	if c.Links != len(doc.Links) || c.Nodes != len(doc.Nodes) {
		t.Errorf("counts.nodes/links = %d/%d, arrays are %d/%d", c.Nodes, c.Links,
			len(doc.Nodes), len(doc.Links))
	}
	if !slices.Equal(doc.Metadata.Scopes, DepartmentSpendingScopes()) {
		t.Errorf("metadata.scopes = %q, want %q", doc.Metadata.Scopes, DepartmentSpendingScopes())
	}
}

// TestTheCrossTabRefusesAFactThatCarriesAFund: these rows have no fund axis,
// so a fact carrying one belongs to another schedule, and no amount check
// would see it.
func TestTheCrossTabRefusesAFactThatCarriesAFund(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(f *fact.Fact)
		want   string
	}{
		{"a fund number", func(f *fact.Fact) {
			if f.Department == "maintenance" {
				f.Fund = fact.FundNumber(100)
			}
		}, "names fund 100"},
		{"a fund group", func(f *fact.Fact) {
			if f.Department == "maintenance" {
				f.FundGroup = "general"
			}
		}, `fund group "general"`},
		{"no department", func(f *fact.Fact) {
			if f.Department == "maintenance" {
				f.Department = ""
			}
		}, "carries no department"},
		{"no category", func(f *fact.Fact) {
			if f.Department == "maintenance" {
				f.Category = ""
			}
		}, "carries no category"},
		{"a division the registry does not list", func(f *fact.Fact) {
			if f.Department == "maintenance" {
				f.Department = "police-department"
			}
		}, `lists no division "police-department"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&departmentSpending{Labels: spendingLabels()}).
				Document(spendingFacts(t, tc.mutate), spendingOptions())
			if err == nil {
				t.Fatalf("Document accepted the fact, want a refusal naming %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say %q", err, tc.want)
			}
		})
	}
}

// TestTheCrossTabRefusesTwoColumnsAndTheWrongSchedule covers the two options
// mistakes that produce a document which looks right.
func TestTheCrossTabRefusesTwoColumnsAndTheWrongSchedule(t *testing.T) {
	facts := spendingFacts(t, nil)

	o := spendingOptions()
	o.Columns = append(o.Columns, Column{FiscalYear: 2027, Basis: testBasis})
	if _, err := (&departmentSpending{}).Document(facts, o); err == nil ||
		!strings.Contains(err.Error(), "is of one column") {
		t.Errorf("two columns gave %v, want a refusal naming the column count", err)
	}

	o = spendingOptions()
	o.Scopes = []string{"all-funds-gross"}
	if _, err := (&departmentSpending{}).Document(facts, o); err == nil ||
		!strings.Contains(err.Error(), "scopes are") {
		t.Errorf("the wrong scope gave %v, want a refusal naming the scope", err)
	}
}

// TestTheCrossTabIsDeterministic is the property a rebuild-and-compare rests on.
func TestTheCrossTabIsDeterministic(t *testing.T) {
	facts := spendingFacts(t, nil)
	first, err := (&departmentSpending{Labels: spendingLabels()}).Build(facts, spendingOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := (&departmentSpending{Labels: spendingLabels()}).Build(facts, spendingOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(first) != string(second) {
		t.Error("two builds of the same facts differ; the document is not deterministic")
	}
}

// TestTheCrossTabDeclaresOneSlicePerPrintedColumn pins the declaration
// PublishedDocuments states by hand against the one the corpus produces.
func TestTheCrossTabDeclaresOneSlicePerPrintedColumn(t *testing.T) {
	facts := spendingFacts(t, nil)
	for i := range facts {
		if facts[i].Department == "maintenance" {
			facts[i].FiscalYear = 2027
		}
	}
	got := (&departmentSpending{}).Slices(facts, "v")
	want := []Options{
		{Columns: []Column{{FiscalYear: 2026, Basis: testBasis}},
			Scopes: DepartmentSpendingScopes(), Version: "v"},
		{Columns: []Column{{FiscalYear: 2027, Basis: testBasis}},
			Scopes: DepartmentSpendingScopes(), Version: "v"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("slices (-want +got):\n%s", diff)
	}
}
