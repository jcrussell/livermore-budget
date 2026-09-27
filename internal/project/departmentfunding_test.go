package project

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// fundingCell is one printed row of Budget Book pp.85-125's lower block: the
// department the schedule is printed for, the fund that pays it, and the figure.
//
// It carries a department paid by one fund and one paid by three, a fund
// paying two departments, a printed dash, an internal service fund and a
// department slug that is also a division slug.
var fundingCells = []struct {
	department string
	label      string
	group      string
	fund       int
	page       int
	cents      int64
}{
	// city-council names a department AND a division.
	{"city-council", "General Fund", "general", 100, 85, 14_919_800},
	{"police-department", "General Fund", "general", 100, 119, 500_000_000},
	{"police-department", "Asset Seizure", "special-revenue", 220, 119, 5_000_000},
	{"police-department", "Grant Fund", "special-revenue", 240, 119, 0},
	{"public-works", "General Fund", "general", 100, 124, 100_000_000},
	{"public-works", "Water", "enterprise", 640, 124, 30_000_000},
	{"public-works", "Information Technology", "internal-service", 720, 124, 2_000_000},
	{"library-department", "Grant Fund", "special-revenue", 240, 115, 1_362_000},
}

// fundingFacts renders the fixture at the scope this document selects, with
// mutate run over each fact.
func fundingFacts(t *testing.T, mutate func(f *fact.Fact)) []fact.Fact {
	t.Helper()
	out := make([]fact.Fact, 0, len(fundingCells))
	for _, c := range fundingCells {
		row := mapping.Row{Category: DepartmentFundingScope, Department: c.department,
			Label: c.label}
		rowPath := fact.RowPath(row)
		columnPath := fact.ColumnPath(mapping.Column{FundGroup: c.group, Fund: c.fund},
			DepartmentFundingScope)
		f := fact.Fact{
			ID: fact.MakeID(testDoc, "funding-"+c.department, rowPath, c.label, columnPath,
				testYear, testBasis),
			DocID:       testDoc,
			Page:        c.page,
			RuleID:      "funding-" + c.department,
			Kind:        mapping.KindExpenditure,
			Basis:       testBasis,
			Scope:       DepartmentFundingScope,
			FiscalYear:  testYear,
			RowPath:     rowPath,
			RowLabel:    c.label,
			Category:    DepartmentFundingScope,
			Department:  c.department,
			ColumnPath:  columnPath,
			FundGroup:   c.group,
			Fund:        fact.FundNumber(c.fund),
			AmountCents: c.cents,
		}
		if mutate != nil {
			mutate(&f)
		}
		out = append(out, f)
	}
	return out
}

// fundingLabels gives `city-council` different words at its two tiers, so a
// projection asking DivisionLabel shows it.
func fundingLabels() stubFundFlows {
	return stubFundFlows{
		names: map[int]string{100: "General Fund", 220: "Asset Seizure",
			240: "Grant Fund", 640: "Water", 720: "Information Technology"},
		types: map[int]string{100: "general", 220: "special-revenue", 240: "special-revenue",
			640: "enterprise", 720: "internal-service"},
		tiers: map[int]string{100: "discretionary"},
		notes: map[int]string{100: "Available for any general city service."},
		divisions: map[string]string{
			"city-council": "City Council Division", "maintenance": "Maintenance"},
		departments: map[string]string{
			"city-council": "City Council", "police-department": "Police Department",
			"public-works": "Public Works", "library-department": "Library Department"},
	}
}

func fundingOptions() Options {
	return Options{
		Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
		Scopes:  DepartmentFundingScopes(),
		Version: "departmentfunding_test.go",
	}
}

func buildFunding(t *testing.T, facts []fact.Fact) *DepartmentFundingDocument {
	t.Helper()
	doc, err := (&departmentFunding{Labels: fundingLabels()}).Document(facts, fundingOptions())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	return doc
}

// TestTheFundingGraphDrawsOneRibbonPerPrintedRow pins which ribbons exist,
// their direction and kind. police-department's Grant Fund dash draws none.
func TestTheFundingGraphDrawsOneRibbonPerPrintedRow(t *testing.T) {
	doc := buildFunding(t, fundingFacts(t, nil))
	type ribbon struct {
		From, To string
		Cents    int64
		Kind     LinkKind
	}
	got := make([]ribbon, 0, len(doc.Links))
	for _, l := range doc.Links {
		got = append(got, ribbon{l.Source, l.Target, l.ValueCents, l.Kind})
	}
	want := []ribbon{
		{"fund/100", "department/city-council", 14_919_800, KindExternal},
		{"fund/100", "department/police-department", 500_000_000, KindExternal},
		{"fund/100", "department/public-works", 100_000_000, KindExternal},
		{"fund/220", "department/police-department", 5_000_000, KindExternal},
		{"fund/240", "department/library-department", 1_362_000, KindExternal},
		{"fund/640", "department/public-works", 30_000_000, KindExternal},
		// The one ribbon that is not external: an Internal Service Fund pays.
		{"fund/720", "department/public-works", 2_000_000, KindInternalService},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the drawn ribbons (-want +got):\n%s", diff)
	}
}

// TestADepartmentIsNotADivision: `city-council` names a department and a
// division, so it must draw `department/city-council` with the department's
// words.
func TestADepartmentIsNotADivision(t *testing.T) {
	doc := buildFunding(t, fundingFacts(t, nil))
	var got Node
	for _, n := range doc.Nodes {
		if n.Tier == tierDepartment && strings.HasSuffix(n.ID, "city-council") {
			got = n
		}
	}
	if got.ID != "department/city-council" {
		t.Errorf("the department node is %q, want %q -- `dept/` is the DIVISION tier and five "+
			"slugs name both", got.ID, "department/city-council")
	}
	if got.Label != "City Council" {
		t.Errorf("the department node reads %q, want %q: the fixture gives the division the "+
			"other words on purpose", got.Label, "City Council")
	}
	if got.Role != roleWholeDepartment {
		t.Errorf("the department node's role is %q, want %q", got.Role, roleWholeDepartment)
	}
	// A department is parentless: it is paid for by several funds.
	if got.Parent != "" {
		t.Errorf("the department node folds into %q; a department belongs to no fund", got.Parent)
	}
}

// TestAFundFoldsIntoTheGroupTheRegistrySays pins the parent edge and the
// disclosure that rides with a constraint tier.
//
// The group comes from data/funds.yaml, never from the fact.
func TestAFundFoldsIntoTheGroupTheRegistrySays(t *testing.T) {
	doc := buildFunding(t, fundingFacts(t, func(f *fact.Fact) {
		if *f.Fund == 640 {
			f.FundGroup = "special-revenue"
		}
	}))
	byID := map[string]Node{}
	for _, n := range doc.Nodes {
		byID[n.ID] = n
	}
	if got := byID["fund/640"].Parent; got != "fund-group/enterprise" {
		t.Errorf("fund/640 folds into %q, want fund-group/enterprise: the registry says "+
			"enterprise and the fact was mutated to say special-revenue", got)
	}
	// The tier-2 node itself is built on demand, because no link touches it.
	if g, ok := byID["fund-group/enterprise"]; !ok || g.Tier != tierFundGroup {
		t.Errorf("fund-group/enterprise is %+v, want a tier-%d node the fold can reach",
			g, tierFundGroup)
	}
	// The link kind still comes from the fact's own heading.
	for _, l := range doc.Links {
		if l.Source == "fund/640" && l.Kind != KindExternal {
			t.Errorf("fund/640's ribbon is %q, want external", l.Kind)
		}
	}
	if n := byID["fund/100"]; n.ConstraintTier != "discretionary" || n.SourceNote == "" ||
		n.Rationale == "" {
		t.Errorf("fund/100 carries tier %q with source note %q and rationale %q; a published "+
			"node carrying our classification must disclose where it came from",
			n.ConstraintTier, n.SourceNote, n.Rationale)
	}
}

// TestTheCountsPublishTheIdentityTheDocumentClaims: facts = cited + uncited, and
// every uncited fact is a printed zero.
func TestTheCountsPublishTheIdentityTheDocumentClaims(t *testing.T) {
	doc := buildFunding(t, fundingFacts(t, nil))
	c := doc.Metadata.Counts
	if c.Facts != len(fundingCells) || c.FactsCited+c.FactsUncited != c.Facts {
		t.Errorf("counts %+v do not satisfy facts = cited + uncited over %d fixture rows",
			c, len(fundingCells))
	}
	if c.FactsUncited != 1 || c.Links != 7 {
		t.Errorf("counts %+v; the fixture prints one dash and seven figures", c)
	}
}

// TestTheFundingGraphRefusesWhatItCannotPlace covers every guard, each with the
// state the corpus would have to be in to reach it.
func TestTheFundingGraphRefusesWhatItCannotPlace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		facts  []fact.Fact
		opts   *Options
		labels labels
		want   string
	}{
		{
			name:  "a fact naming no department",
			facts: fundingFacts(t, func(f *fact.Fact) { f.Department = "" }),
			want:  "carries no department",
		},
		{
			name:  "a fact naming no fund",
			facts: fundingFacts(t, func(f *fact.Fact) { f.Fund = nil }),
			want:  "names fund (absent)",
		},
		{
			name:  "a fact naming no fund group",
			facts: fundingFacts(t, func(f *fact.Fact) { f.FundGroup = "" }),
			want:  "fund group \"\"",
		},
		{
			// The group decides the ribbon's kind, so it is refused, not last-wins.
			name: "one cell printed under two fund groups",
			facts: append(fundingFacts(t, nil), fundingFacts(t, func(f *fact.Fact) {
				f.FundGroup = "capital"
				f.ID += "-again"
			})...),
			want: "is printed under fund group",
		},
		{
			name:  "a fund data/funds.yaml does not list",
			facts: fundingFacts(t, nil),
			labels: stubFundFlows{types: map[int]string{100: "general"},
				departments: fundingLabels().departments},
			want: "records no type for fund",
		},
		{
			// A funding row naming a slug the department tier does not list.
			name: "a department data/departments.yaml does not list",
			facts: fundingFacts(t, func(f *fact.Fact) {
				if f.Department == "police-department" {
					f.Department = "patrol"
				}
			}),
			want: `lists no department "patrol"`,
		},
		{
			name:  "two columns in one graph",
			facts: fundingFacts(t, nil),
			opts: &Options{
				Columns: []Column{{FiscalYear: testYear, Basis: testBasis},
					{FiscalYear: 2027, Basis: testBasis}},
				Scopes: DepartmentFundingScopes(), Version: "t",
			},
			want: "a column is one budget year",
		},
		{
			name:  "the wrong scope set",
			facts: fundingFacts(t, nil),
			opts: &Options{
				Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
				Scopes:  DepartmentSpendingScopes(), Version: "t",
			},
			want: "scopes are",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := fundingOptions()
			if tc.opts != nil {
				o = *tc.opts
			}
			var l labels = fundingLabels()
			if tc.labels != nil {
				l = tc.labels
			}
			_, err := (&departmentFunding{Labels: l}).Document(tc.facts, o)
			if err == nil {
				t.Fatalf("Document accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q", err, tc.want)
			}
		})
	}
}

// TestNetDepartmentFundingRefusesAFactOfAnotherScope drives the netting
// function directly: through Document, selectFacts drops the fact first and
// the test would be green because that gate fired.
func TestNetDepartmentFundingRefusesAFactOfAnotherScope(t *testing.T) {
	_, err := netDepartmentFunding(fundingFacts(t, func(f *fact.Fact) {
		f.Scope = DepartmentSpendingScope
	}))
	if err == nil {
		t.Fatal("netDepartmentFunding accepted a fact of the upper block's scope")
	}
	if !strings.Contains(err.Error(), "which this document does not select") {
		t.Errorf("error %q does not name the scope", err)
	}
	// Document over the same facts drops them rather than refusing.
	doc, err := (&departmentFunding{Labels: fundingLabels()}).Document(
		fundingFacts(t, func(f *fact.Fact) { f.Scope = DepartmentSpendingScope }),
		fundingOptions())
	if err != nil {
		t.Fatalf("Document over facts of another scope: %v", err)
	}
	if doc.Metadata.Counts.Facts != 0 {
		t.Errorf("Document selected %d fact(s) of another scope", doc.Metadata.Counts.Facts)
	}
}

// TestSlicesDeclaresEveryPrintedColumn: all four printed columns.
func TestSlicesDeclaresEveryPrintedColumn(t *testing.T) {
	facts := fundingFacts(t, nil)
	for _, c := range []Column{{FiscalYear: 2024, Basis: mapping.BasisActual},
		{FiscalYear: 2025, Basis: mapping.BasisRevised}, {FiscalYear: 2027, Basis: testBasis}} {
		more := fundingFacts(t, func(f *fact.Fact) {
			f.FiscalYear = c.FiscalYear
			f.Basis = c.Basis
			f.ID += "-" + string(c.Basis)
		})
		facts = append(facts, more...)
	}
	got := (&departmentFunding{}).Slices(facts, "t")
	if len(got) != 4 {
		t.Fatalf("Slices declared %d columns, want the four the schedule prints", len(got))
	}
	// Ordered by fiscal year then basis.
	want := []Column{{FiscalYear: 2024, Basis: mapping.BasisActual},
		{FiscalYear: 2025, Basis: mapping.BasisRevised},
		{FiscalYear: 2026, Basis: testBasis}, {FiscalYear: 2027, Basis: testBasis}}
	for i, o := range got {
		if diff := cmp.Diff([]Column{want[i]}, o.Columns); diff != "" {
			t.Errorf("slice %d (-want +got):\n%s", i, diff)
		}
	}
}
