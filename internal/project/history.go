package project

import (
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// FundBalancesScope is the schedule [FundBalances] draws: ACFR p167, Fund
// Balances of Governmental Funds.
const FundBalancesScope = "acfr-fund-balances"

// FundBalancesProjection is the file stem, and the name every check and the
// packager refer to this document by.
const FundBalancesProjection = "fund-balances"

// ChangesScope is the schedule [FundBalanceChanges] draws: ACFR pp.168-169,
// Changes in Fund Balances of Governmental Funds.
const ChangesScope = "acfr-changes-in-fund-balances"

// ChangesProjection is the file stem for the changes document.
const ChangesProjection = "changes-in-fund-balances"

// HistoryColumns is the ten columns both ACFR ten-year schedules print:
// FY2016 through FY2025, every one audited.
//
// STATED AND NOT DERIVED, for [TrendsColumns]' reason: Slices reads its columns
// off the store exhaustively, so a corpus that silently lost FY2016 would build
// a nine-column document every check passes over. This list is what the site
// promises, and TestPublishedDocumentsAreWhatTheCorpusBuilds is the refusal.
//
// One list serves both schedules because both print the same decade; a schedule
// that diverges gets its own list in the commit that maps it.
func HistoryColumns() []Column {
	out := make([]Column, 0, 10)
	for year := 2016; year <= 2025; year++ {
		out = append(out, Column{FiscalYear: year, Basis: mapping.BasisAudited})
	}
	return out
}

// FundBalances is ACFR p167 as one series per printed row: the GASB 54
// components of the General Fund's balance and of the aggregate All Other
// Governmental Funds, across ten audited years. Every fact carries fund 0 --
// the rows are a fund's components or an aggregate, never a numbered fund.
// The contract is docs/acfr-history-contract.md.
type FundBalances struct {
	// Labels resolves category labels; see Trends.Labels for the nil rule.
	Labels labels
}

var (
	_ Projection = (*FundBalances)(nil)
	_ Sliced     = (*FundBalances)(nil)
)

// Name is the file stem: fund-balances becomes fund-balances.json.
func (*FundBalances) Name() string { return FundBalancesProjection }

// spec is this document's identity, handed to the shared series builder.
func (p *FundBalances) spec() seriesSpec {
	return seriesSpec{
		name:    FundBalancesProjection,
		scope:   FundBalancesScope,
		labels:  p.Labels,
		caveats: fundBalancesCaveats,
	}
}

// Slices selects the whole schedule as one document; see seriesSpec.slices.
func (p *FundBalances) Slices(facts []fact.Fact, version string) []Options {
	return p.spec().slices(facts, version)
}

// Build renders the document as canonical, deterministic JSON.
func (p *FundBalances) Build(facts []fact.Fact, o Options) ([]byte, error) {
	return p.spec().build(facts, o)
}

// Document builds without encoding, so `fisc verify` can check the structure
// without parsing back the JSON it is validating.
func (p *FundBalances) Document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	return p.spec().document(facts, o)
}

// FundBalanceChanges is ACFR pp.168-169 as one series per printed row: nine
// revenue rows, twelve expenditure rows on the ACFR's function axis, and the
// printed excess line, across ten audited years, all governmental funds
// combined. The contract is docs/acfr-history-contract.md.
type FundBalanceChanges struct {
	// Labels resolves category labels; see Trends.Labels for the nil rule.
	Labels labels
}

var (
	_ Projection = (*FundBalanceChanges)(nil)
	_ Sliced     = (*FundBalanceChanges)(nil)
)

// Name is the file stem: changes-in-fund-balances.json.
func (*FundBalanceChanges) Name() string { return ChangesProjection }

// spec is this document's identity, handed to the shared series builder.
func (p *FundBalanceChanges) spec() seriesSpec {
	return seriesSpec{
		name:    ChangesProjection,
		scope:   ChangesScope,
		labels:  p.Labels,
		caveats: changesCaveats,
	}
}

// Slices selects the whole schedule as one document; see seriesSpec.slices.
func (p *FundBalanceChanges) Slices(facts []fact.Fact, version string) []Options {
	return p.spec().slices(facts, version)
}

// Build renders the document as canonical, deterministic JSON.
func (p *FundBalanceChanges) Build(facts []fact.Fact, o Options) ([]byte, error) {
	return p.spec().build(facts, o)
}

// Document builds without encoding, for FundBalances.Document's reason.
func (p *FundBalanceChanges) Document(facts []fact.Fact, o Options) (*TrendsDocument, error) {
	return p.spec().document(facts, o)
}

// The caveats each document ships. Every figure in both is printed; these are
// statements about what the schedules do not say, with the amounts measured off
// the committed store. Unconditional, for trendsCaveats' reason: each is about
// something the pages do not print, which no built document can detect ending.
var (
	caveatBalancesDoNotTieToP41 = Caveat{
		ID:      "p167-does-not-tie-to-p41",
		Summary: "The ACFR's own pages disagree about the 2025 General Fund balance, by $56,424.",
		Text: "This schedule's five General Fund components for 2025 sum to $87,043,576, " +
			"while the ACFR's Statement of Revenues, Expenditures and Changes in Fund " +
			"Balances (p41) prints an ending balance of $87.10 million — $56,424 " +
			"apart. Both figures are published as printed; neither is corrected to the " +
			"other.",
		AppliesTo: []string{},
	}
	caveatNoAssignedRow = Caveat{
		ID:      "all-other-funds-print-no-assigned-row",
		Summary: "All Other Governmental Funds prints no Assigned row, so that block has four components, not five.",
		Text: "p167's All Other Governmental Funds block prints Nonspendable, Restricted, " +
			"Committed and Unassigned, and no Assigned line in any year. The row is absent " +
			"from the schedule, not zero: a published zero is a printed dash, and the page " +
			"prints none there.",
		AppliesTo: []string{},
	}
	caveatGovernmentalFundsOnly = Caveat{
		ID:      "governmental-funds-only",
		Summary: "Governmental funds only: water, sewer and the other enterprise activities are not in this schedule.",
		Text: "This schedule covers the governmental funds — the General Fund plus " +
			"special revenue, debt service and capital projects funds — on the " +
			"modified accrual basis. Enterprise activities such as water and sewer sit " +
			"outside it, and this site's budget pages draw adopted budgets for all funds, " +
			"so figures here sit well below those pages' totals. The gap is scope, not " +
			"error.",
		AppliesTo: []string{},
	}
	caveatOtherFinancingNotPublished = Caveat{
		ID:      "other-financing-not-published",
		Summary: "The schedule's Other financing block and Net change lines are not published, so this table ends at the excess line.",
		Text: "pp.168-169 continue past the excess line with an Other financing sources " +
			"(uses) block and a Net change in fund balances line. Neither is published: " +
			"the block's printed 2023 total repeats the 2022 figure — its components " +
			"sum to $39,259,064 against a printed $(1,767,367) — and Net change 2023 " +
			"carries the same copy, so neither line has an honest printed total to stand " +
			"under.",
		AppliesTo: []string{},
	}
	caveatVLFReclassification = Caveat{
		ID:      "property-tax-reclassified",
		Summary: "Property taxes and Intergovernmental change definition at FY2024-25, by the page's own footnote.",
		Text: "p168's footnote on the Property Taxes row reads: \"VLF Comp revenue " +
			"reclassified to Property tax in FY 2024-25. Previously reported under " +
			"Intergovernmental.\" The two series change definition at that column, so " +
			"part of any movement there is the reclassification rather than the economy.",
		AppliesTo: []string{},
	}
)

// fundBalancesCaveats is every caveat the p167 document ships.
func fundBalancesCaveats() []Caveat {
	return []Caveat{caveatBalancesDoNotTieToP41, caveatNoAssignedRow, caveatGovernmentalFundsOnly}
}

// changesCaveats is every caveat the pp.168-169 document ships.
func changesCaveats() []Caveat {
	return []Caveat{caveatOtherFinancingNotPublished, caveatVLFReclassification, caveatGovernmentalFundsOnly}
}
