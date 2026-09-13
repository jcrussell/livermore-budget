package project

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// spendingCell is one printed cell of Budget Book pp.85-125's upper block: a
// division, the object heading its row sits under, and what the page prints.
//
// THE FIXTURE IS THREE DIVISIONS AND NOT TWENTY-NINE, and it carries every shape
// the committed corpus does: a division with two object rows, one with a printed
// dash, and p124's Transfers Out row, which is the one row on those eleven pages
// that is not an expenditure. Copying all 73 rows would make this a second fact
// store rather than a test of the projection.
var spendingCells = []struct {
	division string
	category string
	kind     mapping.Kind
	label    string
	page     int
	cents    int64
}{
	{"city-council", "wages-and-benefits", mapping.KindExpenditure, "Wages & Benefits", 85, 7_470_100},
	{"city-council", "services-and-supplies", mapping.KindExpenditure, "Services & Supplies", 85, 7_808_000},
	{"general-services", "wages-and-benefits", mapping.KindExpenditure, "Wages & Benefits", 89, 0},
	{"general-services", "services-and-supplies", mapping.KindExpenditure, "Services & Supplies", 89, 1_200_000},
	{"maintenance", "services-and-supplies", mapping.KindExpenditure, "Services & Supplies", 124, 3_000_000},
	{"maintenance", "transfers/out", mapping.KindTransferOut, "Transfers Out", 124, 26_679_800},
}

// spendingFacts renders the fixture at the scope this document selects.
//
// mutate runs over each fact before it is appended, so a test can put one fact
// in a state the corpus is not in -- a fund number, say -- without a second
// copy of the table.
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

func spendingOptions() Options {
	return Options{
		Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
		Scopes:  DepartmentSpendingScopes(),
		Version: "departmentspending_test.go",
	}
}

func buildSpending(t *testing.T, facts []fact.Fact) *DepartmentSpendingDocument {
	t.Helper()
	doc, err := (&departmentSpending{Labels: goldenLabels}).Document(facts, spendingOptions())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	return doc
}

// TestTheCrossTabDrawsOneRibbonPerPrintedCell is the document's whole shape in
// one assertion: which ribbons exist, which way they run, and what they claim.
//
// THE DIRECTION AND THE FLAG ARE ASSERTED TOGETHER because either alone is a
// different document. A ribbon running 5 -> 4 without `partition` is a chart
// claiming money flows from an object category into a division, which is not
// something Budget Book pp.85-125 print; the flag is what makes the direction a
// drawing choice rather than a claim.
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
	// THE ZERO CELL IS NOT HERE AND ITS FACT IS. general-services prints a dash
	// under Wages & Benefits, so there is no ribbon for it -- and the fact is
	// still counted, below.
	want := []ribbon{
		{"expenditure/services-and-supplies", "dept/city-council", 7_808_000, KindExternal, true},
		{"expenditure/services-and-supplies", "dept/general-services", 1_200_000, KindExternal, true},
		{"expenditure/services-and-supplies", "dept/maintenance", 3_000_000, KindExternal, true},
		{"expenditure/wages-and-benefits", "dept/city-council", 7_470_100, KindExternal, true},
		// THE TRANSFER TAKES THE SPINE'S OWN NODE ID AND IS NOT EXTERNAL.
		// `transfers/out` is a flow endpoint the contract pins at tier 5, and a
		// link whose every fact is a transfer may not be published as money
		// crossing the city's boundary -- link-kinds-match-their-facts reports
		// exactly that.
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

// TestTheCrossTabCountsEveryFactOnceOrAsAPrintedZero pins the identity the
// document publishes, and the number that makes it worth publishing.
//
// facts = facts_cited + facts_uncited, and the uncited one is general-services'
// printed dash. A document that dropped a cell would publish a smaller `facts`
// and still satisfy the identity, so the slice size is asserted too.
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
	if doc.Metadata.Scope != DepartmentSpendingScope {
		t.Errorf("metadata.scope = %q, want %q", doc.Metadata.Scope, DepartmentSpendingScope)
	}
}

// TestTheCrossTabRefusesAFactThatCarriesAFund is the guard that inverts the
// drill-down's, and it is the one a reader is most likely to think is backwards.
//
// EVERY FACT OF THIS SCOPE CARRIES fund 0 AND fund_group "", because the upper
// block prints what a division spends whatever pays for it. A fact here with a
// fund is a row of the LOWER block, or of pp.167-170, wearing this scope -- and
// drawn without a refusal it would be published under a document whose caveat
// tells every reader that no row carries a fund. The amounts would be unchanged,
// so departmentwide-ties-to-spine would still tie and nothing downstream would
// see it.
func TestTheCrossTabRefusesAFactThatCarriesAFund(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(f *fact.Fact)
		want   string
	}{
		{"a fund number", func(f *fact.Fact) {
			if f.Department == "maintenance" {
				f.Fund = 100
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&departmentSpending{Labels: goldenLabels}).
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
//
// Two columns add every cell to its own successor and the matrix still reads as
// a matrix; the wrong scope publishes another schedule's rows under this one's
// caveats, which say those rows carry no fund.
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
	first, err := (&departmentSpending{Labels: goldenLabels}).Build(facts, spendingOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := (&departmentSpending{Labels: goldenLabels}).Build(facts, spendingOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(first) != string(second) {
		t.Error("two builds of the same facts differ; the document is not deterministic")
	}
}

// TestTheCrossTabDeclaresOneSlicePerPrintedColumn pins the declaration
// PublishedDocuments states by hand against the one the corpus produces.
//
// The two are deliberately separate -- a published set derived from the facts
// would agree with them by construction -- so this is where they are compared.
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
