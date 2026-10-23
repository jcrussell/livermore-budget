package project

import (
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// FundBalancesScope is the schedule [FundBalances] draws: ACFR p167, Fund
// Balances of Governmental Funds.
const FundBalancesScope = structure.ScopeACFRFundBalances

// FundBalancesProjection is the file stem, and the name every check and the
// packager refer to this document by.
const FundBalancesProjection = "fund-balances"

// ChangesScope is the schedule [FundBalanceChanges] draws: ACFR pp.168-169,
// Changes in Fund Balances of Governmental Funds.
const ChangesScope = structure.ScopeACFRChangesInFundBalances

// ChangesProjection is the file stem for the changes document.
const ChangesProjection = "changes-in-fund-balances"

// HistoryColumns is the ten columns both ACFR ten-year schedules print:
// FY2016 through FY2025, every one carrying the audited basis.
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
		out = append(out, Column{FiscalYear: year, Basis: vocab.BasisAudited})
	}
	return out
}

// FundBalances is ACFR p167 as one series per printed row: the GASB 54
// components of the General Fund's balance and of the aggregate All Other
// Governmental Funds, across ten years. Every fact carries fund 0 --
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
// printed excess line, across ten years, all governmental funds
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

// UnauditedCaveatID is the caveat a document ships to say its figures come
// from the ACFR section headed "Statistical Section (Unaudited)", which the
// auditor's report declines to give an opinion on; the page reads it to print
// "unaudited" for the basis.
const UnauditedCaveatID = "statistical-section-unaudited"

// The caveats each document ships. Every figure in both is printed; these are
// statements about what the schedules do not say, with the amounts measured off
// the committed store. Unconditional, for trendsCaveats' reason: each is about
// something the pages do not print, which no built document can detect ending.
var (
	caveatBalancesDoNotTieToP41 = Caveat{
		ID:      "p167-does-not-tie-to-p41",
		Theme:   ThemeBooksDisagree,
		Summary: "The annual report's summary table (p41) does not match the General Fund's audited 2025 balance.",
		Text: "This schedule's five General Fund parts for 2025 add up to $87,043,576, and the " +
			"audited statement (p54) prints exactly that. The management discussion on p41 " +
			"shows the same balance in millions as $87.10, which is not where $87,043,576 " +
			"rounds. All three are published as printed.",
		AppliesTo: []string{},
	}
	caveatNoAssignedRow = Caveat{
		ID:      "all-other-funds-print-no-assigned-row",
		Theme:   ThemeLeftOut,
		Summary: "All Other Governmental Funds prints no Assigned row, so that block has four parts, not five.",
		Text: "p167's All Other Governmental Funds block prints Nonspendable, Restricted, Committed " +
			"and Unassigned, and no Assigned line in any year. The row is missing, not zero: a " +
			"zero would be a printed dash.",
		AppliesTo: []string{},
	}
	caveatGovernmentalFundsOnly = Caveat{
		ID:      "governmental-funds-only",
		Theme:   ThemeLeftOut,
		Summary: "These schedules leave out water, sewer and the city's other business-like funds.",
		Text: "These schedules cover the governmental funds: the General Fund plus special revenue, " +
			"debt service and capital project funds. Water, sewer and the other enterprise " +
			"funds are outside them, so the totals here are well below the budget pages', " +
			"which draw every fund. The gap is scope, not error.",
		AppliesTo: []string{},
	}
	caveatOtherFinancingNotPublished = Caveat{
		ID:      "other-financing-not-published",
		Theme:   ThemeLeftOut,
		Summary: "The table stops before Other financing and Net change, whose 2023 totals are misprinted.",
		Text: "pp.168-169 go on past the excess line to an Other financing sources (uses) block and " +
			"a Net change in fund balances line. Neither is published: the block's printed 2023 " +
			"total repeats the 2022 figure (its parts add up to $39,259,064 against a printed " +
			"$(1,767,367)), and Net change 2023 carries the same copy.",
		AppliesTo: []string{},
	}
	caveatStatisticalSectionIsUnaudited = Caveat{
		ID:      UnauditedCaveatID,
		Theme:   ThemeKindsOfYear,
		Summary: "These ten-year tables are unaudited; the auditor's opinion covers only the latest year's statements.",
		Text: "p161 heads this section \"Statistical Section (Unaudited)\", and the auditor's " +
			"report (p29) says its opinion does not cover it. The audited statements cover " +
			"the year just ended, so at most a table's 2025 column can be checked against " +
			"them; the other nine years rest on this section alone.",
		AppliesTo: []string{},
	}
	caveatChangesTieToP54 = Caveat{
		ID:      "pp168-169-tie-to-p54",
		Theme:   ThemeChecked,
		Summary: "This table's 2025 column matches the audited statement (p54) to the dollar on five lines.",
		Text: "Total revenues $221,212,743, Total Expenditures $186,516,649, the excess " +
			"$34,696,094, Total other financing sources (uses) $(18,458,098) and Net change in " +
			"fund balances $16,237,996 are the same in this table and in p54's Total " +
			"Governmental Funds column. That includes the two lines this table withholds: " +
			"what rules them out is their 2023 column, not their 2025 one.",
		AppliesTo: []string{},
	}
	caveatVLFReclassification = Caveat{
		ID:      "property-tax-reclassified",
		Theme:   ThemeKindsOfYear,
		Summary: "Property taxes and Intergovernmental change definition in FY2024-25, so that year's jump is partly a relabel.",
		Text: "p168's footnote on the Property Taxes row reads: \"VLF Comp revenue reclassified to " +
			"Property tax in FY 2024-25. Previously reported under Intergovernmental.\" Part " +
			"of any movement in either line that year is the relabelling, not the economy.",
		AppliesTo: []string{},
	}
)

// fundBalancesCaveats is every caveat the p167 document ships.
func fundBalancesCaveats() []Caveat {
	return []Caveat{caveatStatisticalSectionIsUnaudited, caveatBalancesDoNotTieToP41, caveatNoAssignedRow, caveatGovernmentalFundsOnly}
}

// changesCaveats is every caveat the pp.168-169 document ships.
func changesCaveats() []Caveat {
	return []Caveat{caveatStatisticalSectionIsUnaudited, caveatOtherFinancingNotPublished, caveatChangesTieToP54, caveatVLFReclassification, caveatGovernmentalFundsOnly}
}
