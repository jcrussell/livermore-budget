package project

import (
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// CIPProjectsScope is the schedule [CIPProjects] draws: Budget Book
// pp.224-235, the Capital Improvement Program project listing by fund.
const CIPProjectsScope = structure.ScopeCIPProjectListing

// CIPProjectsProjection is the file stem, and the name every check and the
// packager refer to this document by.
const CIPProjectsProjection = "cip-projects"

// CIPProjectsColumns is the five columns pp.224-235 publish: two adopted
// budget years and three projected plan years. Stated, for [TrendsColumns]'
// reason.
func CIPProjectsColumns() []Column {
	return []Column{
		{FiscalYear: 2026, Basis: vocab.BasisAdopted},
		{FiscalYear: 2027, Basis: vocab.BasisAdopted},
		{FiscalYear: 2028, Basis: vocab.BasisProjected},
		{FiscalYear: 2029, Basis: vocab.BasisProjected},
		{FiscalYear: 2030, Basis: vocab.BasisProjected},
	}
}

// CIPProjects is pp.224-235 as one series per printed project row: a project
// under the fund paying for it, across the five published columns.
//
// The scope stays a declared residue (structure.BudgetBookResidue): no cut
// admits it, because its budget years are not p222's appropriations, and a
// series document compares nothing along the lattice.
type CIPProjects struct {
	// Labels resolves fund names; see Trends.Labels for the nil rule.
	Labels labels
}

var (
	_ Projection = (*CIPProjects)(nil)
	_ Sliced     = (*CIPProjects)(nil)
)

// Name is the file stem: cip-projects becomes cip-projects.json.
func (*CIPProjects) Name() string { return CIPProjectsProjection }

// spec is this document's identity, handed to the shared series builder.
func (p *CIPProjects) spec() seriesSpec {
	return seriesSpec{
		name:    CIPProjectsProjection,
		scope:   CIPProjectsScope,
		labels:  p.Labels,
		caveats: cipProjectsCaveats,
	}
}

// Slices selects the whole schedule as one document; see seriesSpec.slices.
func (p *CIPProjects) Slices(facts []fact.Fact, version string) []Options {
	return p.spec().slices(facts, version)
}

// Build renders the document as canonical, deterministic JSON.
func (p *CIPProjects) Build(facts []fact.Fact, o Options) ([]byte, error) {
	return p.spec().build(facts, o)
}

// Document builds without encoding, for FundBalances.Document's reason.
func (p *CIPProjects) Document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	return p.spec().document(facts, o)
}

// cipListingColumn is what pp.222-235 print for one published column that
// this document does not carry as facts: the listing's grand total, fund
// 611's two grant-block totals, which publish nothing (fisc-ig9h), and p222's
// new appropriations where p222 prints the year.
//
// It is the one spelling of these figures: the caveats are written from it,
// and TestCIPProjectsTieToTheListingsPrintedTotals holds the document to it.
type cipListingColumn struct {
	Column Column
	// GrandTotal is p235's TOTAL line, in dollars.
	GrandTotal int64
	// FederalStorm and StateStorm are p231's TOTAL - FEDERAL GRANT - STORM and
	// TOTAL - STATE GRANT - STORM, in dollars.
	FederalStorm, StateStorm int64
	// TotalUnder is how many dollars GrandTotal prints under the rows above it,
	// the delta cip-listing-p0234's subtotal chain declares.
	TotalUnder int64
	// NewAppropriations is p222's Totals, in dollars, or 0 where p222 prints
	// no such year.
	NewAppropriations int64
}

// cipListingColumns is every published column's printed figures, in
// [CIPProjectsColumns] order.
func cipListingColumns() []cipListingColumn {
	return []cipListingColumn{
		{Column: Column{FiscalYear: 2026, Basis: vocab.BasisAdopted}, GrandTotal: 70_765_450,
			FederalStorm: 2_246_250, StateStorm: 1_486_563, TotalUnder: 1, NewAppropriations: 47_084_623},
		{Column: Column{FiscalYear: 2027, Basis: vocab.BasisAdopted}, GrandTotal: 70_559_870,
			FederalStorm: 2_912_750, StateStorm: 134_438, TotalUnder: 1, NewAppropriations: 58_988_871},
		{Column: Column{FiscalYear: 2028, Basis: vocab.BasisProjected}, GrandTotal: 69_034_500,
			FederalStorm: 2_075_000, StateStorm: 518_750},
		{Column: Column{FiscalYear: 2029, Basis: vocab.BasisProjected}, GrandTotal: 56_440_500},
		{Column: Column{FiscalYear: 2030, Basis: vocab.BasisProjected}, GrandTotal: 51_891_000},
	}
}

// RowsPublished is the dollars this document's rows sum to in the column:
// the printed grand total less fund 611's two blocks, plus what the total
// prints under its rows.
func (c cipListingColumn) RowsPublished() int64 {
	return c.GrandTotal - c.FederalStorm - c.StateStorm + c.TotalUnder
}

// dollars renders a whole-dollar figure the way the pages print it.
func dollars(d int64) string { return amount.Cents(d * 100).Dollars() }

// cipProjectsCaveats is every caveat the pp.224-235 document ships.
func cipProjectsCaveats() []Caveat {
	cols := cipListingColumns()
	var carried, fund611 string
	for _, c := range cols {
		if c.NewAppropriations != 0 {
			if carried != "" {
				carried += ", and "
			}
			carried += fmt.Sprintf("%s for %s against p222's %s", dollars(c.GrandTotal),
				fiscalYearLabel(c.Column.FiscalYear), dollars(c.NewAppropriations))
		}
		if fund611 != "" {
			fund611 += "; "
		}
		fund611 += fmt.Sprintf("%s: the listing prints %s, fund 611's blocks %s and %s",
			fiscalYearLabel(c.Column.FiscalYear), dollars(c.GrandTotal),
			dollars(c.FederalStorm), dollars(c.StateStorm))
		if c.TotalUnder != 0 {
			fund611 += fmt.Sprintf(", and the printed total is %s under its own rows", dollars(c.TotalUnder))
		}
		fund611 += fmt.Sprintf(", so the rows here sum to %s", dollars(c.RowsPublished()))
	}
	return []Caveat{{
		ID: "cip-listing-carries-forward",
		Summary: "The adopted budget years include appropriations carried forward from earlier " +
			"years, so they are larger than the new money p222 appropriates, and no page says " +
			"how much of each project's figure is carried forward.",
		Text: "The listing's FY 2025-26 and FY 2026-27 columns are the adopted budget's figures " +
			"for each project, and they include unspent appropriations carried forward from " +
			"earlier years. Its grand totals are " + carried + " new appropriations. Nothing " +
			"on pp.224-235 separates carried-forward money from new money for any project, " +
			"so no figure here is a project's new appropriation.",
		AppliesTo: []string{},
	}, {
		ID:      "cip-listing-plan-years-are-projected",
		Summary: "FY 2027-28 to FY 2029-30 are the plan's projections, not adopted budgets.",
		Text: "The city adopts a two-year budget. The listing's three later columns, FY 2027-28, " +
			"FY 2028-29 and FY 2029-30, are projected: the Capital Improvement Program's plan, " +
			"nothing has been appropriated for them, and a later budget may change any of them.",
		AppliesTo: []string{},
	}, {
		ID: "cip-listing-fund-611-unpublished",
		Summary: "Fund 611's two storm grant blocks publish nothing, so the rows here sum to " +
			"less than the listing's printed grand total.",
		Text: "p230 lists fund 611's storm projects twice, under a federal grant block and " +
			"a state grant block, on lines identical but for their figures, and neither " +
			"block is published. Both still count in the listing's grand " +
			"total. " + fund611 + ".",
		AppliesTo: []string{},
	}, {
		ID: "cip-listing-columns-not-published",
		Summary: "FY 2024-25, FY 2030-45 and each row's TOTAL are read and checked against the " +
			"printed totals, and not published.",
		Text: "pp.224-235 also print an FY 2024-25 column, whose basis nothing on these pages " +
			"states; an FY 2030-45 column, fifteen years in one figure; and each row's TOTAL. " +
			"All three are held to the printed fund totals when the store is built, and none " +
			"is published here. Neither is the program a project belongs to: pp.236-245 " +
			"print it and are not read.",
		AppliesTo: []string{},
	}}
}
