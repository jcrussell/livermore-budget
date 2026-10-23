package project

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
	"github.com/jcrussell/livermore-budget/schema"
)

// DebtServiceProjection is the file stem of Budget Book pp.80-81 as one
// document: the debt service budget by issue, and the requirements schedule
// by fund type through 2055.
const DebtServiceProjection = "debt-service"

// DebtServiceScopes is the two schedules the document draws, the issues first.
// Both are declared residue in internal/structure: no cut admits them, and
// this document selects them by scope.
func DebtServiceScopes() []string {
	return []string{structure.ScopeDebtServiceByIssue, structure.ScopeDebtServiceRequirements}
}

// DebtServiceColumns is every column the document covers, in the order its
// metadata lists them: the issues' FY2024-25 revised and two adopted years,
// and the requirements' five years and five bands. Stated and not derived,
// for [TrendsColumns]' reason.
func DebtServiceColumns() []Column {
	out := []Column{
		{FiscalYear: 2025, Basis: vocab.BasisRevised},
		{FiscalYear: 2026, Basis: vocab.BasisAdopted},
		{FiscalYear: 2027, Basis: vocab.BasisAdopted},
	}
	for y := 2026; y <= 2030; y++ {
		out = append(out, Column{FiscalYear: y, Basis: vocab.BasisProjected})
	}
	for y := 2031; y <= 2051; y += 5 {
		out = append(out, Column{FiscalYear: y, Basis: vocab.BasisProjected, Through: y + 4})
	}
	sortColumns(out)
	return out
}

// debtIssue is one printed row of the debt service budget as the page shows
// it: the label the city prints, the words this site shows for it, the
// activity block and payer heading it sits under, and the requirements fund
// type whose budget years it is part of.
type debtIssue struct {
	printed, label, activity, payer, fundType string
}

const (
	activityGovernmental = "Governmental Activities"
	activityBusinessType = "Business-Type Activities"
)

// debtIssues is every row pp.80-81 print by issue, in page order. A fact
// whose row_label is not here is refused, and so is an entry no fact names.
//
// The Lease/Purchase Loan under Internal Service Fund is Banc of America's
// equipment lease: p80 prints the lender on the line above it, with a dash in
// every column, and the business-type block prints the same lender's lease
// on one line (fisc-ovun). The label is ours; the row_label is the page's.
var debtIssues = []debtIssue{
	{"2020A Certificates of Participation", "2020A Certificates of Participation", activityGovernmental, "", fundTypeGovernmental},
	{"2020B Certificates of Participation", "2020B Certificates of Participation", activityGovernmental, "", fundTypeGovernmental},
	{"2022 Certificates of Participation", "2022 Certificates of Participation", activityGovernmental, "", fundTypeGovernmental},
	{"Housing and Urban Development Loans", "Housing and Urban Development Loans", activityGovernmental, "", fundTypeGovernmental},
	{"Interfund Loan", "Interfund Loan", activityGovernmental, "", fundTypeGovernmental},
	{"Lease/Purchase Loan", "Banc of America Equipment Lease/Purchase Loan", activityGovernmental, "Internal Service Fund", fundTypeInternalService},
	{"Business-Type Actitivies:\nAirport\n2020B Certificates of Participation", "2020B Certificates of Participation", activityBusinessType, "Airport", fundTypeAirport},
	{"Banc of America Equipment Lease/Purchase Loan", "Banc of America Equipment Lease/Purchase Loan", activityBusinessType, "Airport", fundTypeAirport},
	{"Water\n2020B Certificates of Participation", "2020B Certificates of Participation", activityBusinessType, "Water", fundTypeWater},
}

// The requirements' fund types, keyed by the fund dimension their facts carry.
const (
	fundTypeGovernmental    = "governmental"
	fundTypeInternalService = "internal-service"
	fundTypeAirport         = "fund/600"
	fundTypeWater           = "fund/640"
)

// debtFundType is one fund type the requirements print, as the page heads it.
// Governmental Funds is no single fund type of data/funds.yaml, so its facts
// carry no fund dimension at all.
type debtFundType struct {
	key, label, fundGroup string
	fund                  int
}

var debtFundTypes = []debtFundType{
	{fundTypeGovernmental, "Governmental Funds", "", 0},
	{fundTypeInternalService, "Internal Service Funds", "internal-service", 0},
	{fundTypeAirport, "Airport Fund", "enterprise", 600},
	{fundTypeWater, "Water Fund", "enterprise", 640},
}

// fundTypeOf is the fund type a requirements fact is a figure of, or "" for a
// fund dimension no entry names.
func fundTypeOf(f *fact.Fact) string {
	for _, t := range debtFundTypes {
		fund := 0
		if f.Fund != nil {
			fund = *f.Fund
		}
		if f.FundGroup == t.fundGroup && fund == t.fund {
			return t.key
		}
	}
	return ""
}

// The measured figures the caveats print, each declared once and held to the
// committed store by TestTheDebtCaveatsSayWhatTheStoreSays.
var (
	// debtShortfall is, per adopted year, how far pp.80-81's by-issue total
	// falls short of the funds' Debt Services the expenditure schedules print
	// (all-funds-gross on pp.66-67): 8,985,056 against 8,865,478, and
	// 8,938,954 against 8,828,028.
	debtShortfall = []yearCents{{2026, 11957800}, {2027, 11092600}}

	// debtRequirementDrift is every requirements cell of 2026 and 2027 that
	// is not the sum of its fund type's issues in the budget above it, and
	// by how much the requirement exceeds it.
	debtRequirementDrift = []debtDrift{
		{2026, fundTypeGovernmental, debtPrincipal, -100},
		{2027, fundTypeAirport, debtInterest, -100},
		{2027, fundTypeWater, debtPrincipal, 500},
		{2027, fundTypeWater, debtInterest, -100},
	}
)

type yearCents struct {
	year  int
	cents int64
}

type debtDrift struct {
	year           int
	fundType, side string
	cents          int64
}

const (
	debtPrincipal = "debt-services/principal"
	debtInterest  = "debt-services/interest"
)

// debtCaveats is every caveat the document ships, built from the
// declarations above so the sentence and the figure cannot disagree.
func debtCaveats() []Caveat {
	short := make([]string, len(debtShortfall))
	for i, s := range debtShortfall {
		short[i] = fmt.Sprintf("%s in %s", amount.Cents(s.cents).Dollars(), fiscalYearLabel(s.year))
	}
	drift := make([]string, len(debtRequirementDrift))
	var lo, hi int64
	for i, d := range debtRequirementDrift {
		size := max(d.cents, -d.cents)
		if i == 0 || size < lo {
			lo = size
		}
		hi = max(hi, size)
		dir := "more"
		c := d.cents
		if c < 0 {
			dir, c = "less", -c
		}
		side := "principal"
		if d.side == debtInterest {
			side = "interest"
		}
		drift[i] = fmt.Sprintf("%s %s %s, %s %s", debtFundTypeLabel(d.fundType), side,
			YearLabelOf(d.year, 0), amount.Cents(c).Dollars(), dir)
	}
	return []Caveat{
		{
			ID:      "requirements-are-projected",
			Summary: "The requirements are payments the city's debt agreements schedule through 2055. None of them is an appropriation.",
			Text: "pp.80-81 print the debt service requirements beside the budget. They are what the " +
				"city's certificates, loans and lease oblige it to pay each year, as the city " +
				"scheduled them when the budget was printed. A council adopts no year of them: the " +
				"adopted figures are the by-issue budget for FY 2025-26 and FY 2026-27, and every " +
				"requirement is published here as projected.",
			AppliesTo: []string{},
		},
		{
			ID:      "a-band-is-five-years",
			Summary: "From 2031 each row is five years in one figure. A band is not a year, and its figure is not a year's.",
			Text: "The schedule prints one row a year through the year ending June 30, 2030, and then " +
				"one row for each five years, 2031-2035 to 2051-2055. Each band's figure is the total " +
				"of its five years, as printed. The page does not say how it falls year by year, and " +
				"this site does not divide it. Comparing a band with a single year compares five " +
				"years with one.",
			AppliesTo: []string{},
		},
		{
			ID:      "the-last-band-ends-in-2052",
			Summary: "The last band is printed as 2051-2055. The annual report prints the same figures as 2051-2052.",
			Text: "The Budget Book labels its last row 2051-2055. The city's FY2025 annual financial " +
				"report prints the same principal and interest, by activity, on p86 under " +
				"2051-2052, and on p84 gives 2052 as the 2022 Certificates of Participation's " +
				"maturity. This site publishes the band the Budget Book prints.",
			AppliesTo: []string{},
		},
		{
			ID: "by-issue-short-of-the-funds-debt-services",
			Summary: fmt.Sprintf("The by-issue budget totals fall short of the funds' Debt Services by %s.",
				strings.Join(short, " and ")),
			Text: fmt.Sprintf("The issues' adopted totals are not the Debt Services the budget's "+
				"expenditure schedules print for the funds: they fall short by %s. The difference sits "+
				"chiefly in the HUD loans' fund, 224, which budgets more than the loans' printed debt "+
				"service, and in the 2022 COP fund, 402, which budgets less. The FY 2024-25 revised "+
				"column ties exactly. Neither figure is corrected to the other.",
				strings.Join(short, " and ")),
			AppliesTo: []string{},
		},
		{
			ID: "requirements-are-not-the-budget",
			Summary: fmt.Sprintf("The requirements' 2026 and 2027 rows are close to the budget above them "+
				"but not equal: %s differ by %s to %s.", counted(len(debtRequirementDrift), "cell"),
				amount.Cents(lo).Dollars(), amount.Cents(hi).Dollars()),
			Text: fmt.Sprintf("Each fund type's requirement for 2026 and 2027 can be held against the "+
				"issues it pays in the budget above it. Most agree to the dollar. These do not: %s. "+
				"Both are published as printed.", strings.Join(drift, "; ")),
			AppliesTo: []string{},
		},
		{
			ID:      "the-equipment-lease-is-one-lender",
			Summary: "Both Lease/Purchase rows are one Banc of America equipment purchase agreement, paid by the Internal Service Fund and by the Airport.",
			Text: "p80 prints the governmental lease across three lines: Internal Service Fund, then " +
				"Banc of America Equipment with a dash in every column, then Lease/Purchase Loan " +
				"with the figures. This site labels that row Banc of America Equipment " +
				"Lease/Purchase Loan, which is how the business-type block prints the Airport's " +
				"row. The city's FY2025 annual financial report, p86, describes one equipment " +
				"purchase agreement with Bank of America, entered in 2012, whose payments are made " +
				"by the Facilities Rehabilitation Internal Service Fund and the Airport Enterprise " +
				"Fund. The Internal Service Funds' requirements for 2026 and 2027 are this row's " +
				"principal and interest to the dollar.",
			AppliesTo: []string{},
		},
	}
}

// debtFundTypeLabel is a fund type's printed heading.
func debtFundTypeLabel(key string) string {
	for _, t := range debtFundTypes {
		if t.key == key {
			return t.label
		}
	}
	return key
}

// YearLabelOf is how pp.80-81 print a requirements row's years: "2026" for the
// year ending June 30, 2026, "2031-2035" for a band.
func YearLabelOf(year, through int) string {
	if through == 0 {
		return fmt.Sprintf("%d", year)
	}
	return fmt.Sprintf("%d-%d", year, through)
}

// DebtServiceDocument is pp.80-81 as one document. Its shape is
// schema/debt-service.schema.json.
type DebtServiceDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Projection    string              `json:"projection"`
	Metadata      DebtServiceMetadata `json:"metadata"`
	// IssueColumns is the by-issue table's budget columns, oldest first, one
	// per cell of every issue.
	IssueColumns []DebtColumn     `json:"issue_columns"`
	Issues       []DebtIssue      `json:"issues"`
	Requirements DebtRequirements `json:"requirements"`
}

// DebtServiceMetadata is the document's envelope.
type DebtServiceMetadata struct {
	GeneratedBy string   `json:"generated_by"`
	Scopes      []string `json:"scopes"`
	Currency    string   `json:"currency"`
	Units       string   `json:"units"`
	// Columns is every column the document covers, both schedules', a band
	// with its last year: what the site promises (DebtServiceColumns).
	Columns []DebtDocColumn `json:"columns"`
	Sources []Source        `json:"sources"`
	Counts  DebtCounts      `json:"counts"`
	Caveats []Caveat        `json:"caveats"`
}

// DebtDocColumn is one column the document covers.
type DebtDocColumn struct {
	FiscalYear        int    `json:"fiscal_year"`
	FiscalYearThrough *int   `json:"fiscal_year_through"`
	Basis             string `json:"basis"`
}

// DebtColumn is one budget column of the by-issue table.
type DebtColumn struct {
	FiscalYear      int    `json:"fiscal_year"`
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
}

// DebtCounts is how the document accounts for its facts: every one is a
// figure some cell carries, so facts equals figures.
type DebtCounts struct {
	Facts   int `json:"facts"`
	Figures int `json:"figures"`
}

// DebtIssue is one printed row of the by-issue budget.
type DebtIssue struct {
	Label    string `json:"label"`
	RowLabel string `json:"row_label"`
	Activity string `json:"activity"`
	Payer    string `json:"payer"`
	// Cells is one per metadata column, in its order.
	Cells []DebtPair `json:"cells"`
}

// DebtPair is the principal and interest the page prints in one column, each
// a cited figure, or null where the page prints no cell there.
type DebtPair struct {
	Principal *DebtFigure `json:"principal"`
	Interest  *DebtFigure `json:"interest"`
}

// DebtFigure is one fact, carrying its page, byte offset and token.
type DebtFigure struct {
	AmountCents int64  `json:"amount_cents"`
	FactID      string `json:"fact_id"`
	DocID       string `json:"doc_id"`
	Page        int    `json:"page"`
	Offset      int    `json:"offset"`
	Token       string `json:"token"`
}

// DebtRequirements is the requirements schedule: its fund types, its rows,
// and each fund type's total over every row.
type DebtRequirements struct {
	FundTypes []DebtFundType `json:"fund_types"`
	Periods   []DebtPeriod   `json:"periods"`
	Totals    []DebtTotal    `json:"totals"`
}

// DebtFundType is one fund type, as the page heads it, with the fund
// dimension every fact in its column carries: no group and no fund for
// Governmental Funds, which is no single fund type.
type DebtFundType struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	FundGroup string `json:"fund_group"`
	Fund      *int   `json:"fund"`
}

// DebtPeriod is one printed row: a year, or a band of years.
type DebtPeriod struct {
	FiscalYear        int    `json:"fiscal_year"`
	FiscalYearThrough *int   `json:"fiscal_year_through"`
	Label             string `json:"label"`
	Years             int    `json:"years"`
	// Cells is one per fund type, in its order.
	Cells []DebtPair `json:"cells"`
}

// DebtTotal is one fund type's printed figures summed over every row, with
// the facts summed. fisc verify's stated-total check holds each to the Total
// pp.80-81 print beneath its column.
type DebtTotal struct {
	FundType       string   `json:"fund_type"`
	PrincipalCents int64    `json:"principal_cents"`
	InterestCents  int64    `json:"interest_cents"`
	FactIDs        []string `json:"fact_ids"`
}

// DebtService is the debt-service projection.
type DebtService struct{}

var (
	_ Projection = (*DebtService)(nil)
	_ Sliced     = (*DebtService)(nil)
)

// Name is the file stem.
func (*DebtService) Name() string { return DebtServiceProjection }

// Slices is one Options over every column the two schedules carry, read off
// the store exhaustively, bands included.
func (*DebtService) Slices(facts []fact.Fact, version string) []Options {
	seen := map[Column]bool{}
	scopes := DebtServiceScopes()
	for i := range facts {
		if slices.Contains(scopes, facts[i].Scope) {
			seen[ColumnOf(&facts[i])] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	cols := make([]Column, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sortColumns(cols)
	return []Options{{Columns: cols, Scopes: scopes, Version: version}}
}

// Build renders the document as canonical JSON, validated against its schema.
func (p *DebtService) Build(facts []fact.Fact, o Options) ([]byte, error) {
	d, err := p.Document(facts, o)
	if err != nil {
		return nil, err
	}
	return encode(d, DebtServiceProjection, schema.DebtService)
}

// Document builds without encoding, so fisc verify can read the structure.
//
// Every fact selected is one figure of one cell, and the build refuses one
// that is not: a row no issue names, a fund dimension no fund type names, a
// category that is neither principal nor interest, a cell printed twice, and
// an issue or fund type no fact names.
func (*DebtService) Document(facts []fact.Fact, o Options) (*DebtServiceDocument, error) {
	if err := o.validate(); err != nil {
		return nil, fmt.Errorf("%s options: %w", DebtServiceProjection, err)
	}
	if !sameScopes(o.Scopes, DebtServiceScopes()) {
		return nil, fmt.Errorf("%s: scopes are %s, want %s", DebtServiceProjection,
			o.ScopeList(), strings.Join(DebtServiceScopes(), ", "))
	}
	selected, err := SelectFacts(facts, o)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", DebtServiceProjection, err)
	}

	var byIssue, req []*fact.Fact
	for i := range selected {
		f := &selected[i]
		if f.Category != debtPrincipal && f.Category != debtInterest {
			return nil, fmt.Errorf("%s: fact %s is %q, neither principal nor interest",
				DebtServiceProjection, f.ID, f.Category)
		}
		if f.Scope == structure.ScopeDebtServiceByIssue {
			byIssue = append(byIssue, f)
		} else {
			req = append(req, f)
		}
	}

	issues, columns, err := debtIssueRows(byIssue)
	if err != nil {
		return nil, err
	}
	reqs, err := debtRequirementRows(req)
	if err != nil {
		return nil, err
	}

	docCols := make([]DebtDocColumn, len(o.Columns))
	for i, c := range o.Columns {
		docCols[i] = DebtDocColumn{FiscalYear: c.FiscalYear, Basis: string(c.Basis)}
		if c.Through != 0 {
			n := c.Through
			docCols[i].FiscalYearThrough = &n
		}
	}

	cavs := debtCaveats()
	if err := validateCaveats(cavs, map[string]struct{}{}); err != nil {
		return nil, err
	}
	return &DebtServiceDocument{
		SchemaVersion: SchemaVersion,
		Projection:    DebtServiceProjection,
		Metadata: DebtServiceMetadata{
			GeneratedBy: o.Version,
			Scopes:      slices.Clone(o.Scopes),
			Currency:    "USD",
			Units:       "cents",
			Columns:     docCols,
			Sources:     SourcesOf(selected),
			Counts:      DebtCounts{Facts: len(selected), Figures: len(byIssue) + len(req)},
			Caveats:     cavs,
		},
		IssueColumns: columns,
		Issues:       issues,
		Requirements: reqs,
	}, nil
}

// figureOf is a fact as a cited figure.
func figureOf(f *fact.Fact) *DebtFigure {
	return &DebtFigure{AmountCents: f.AmountCents, FactID: f.ID, DocID: f.DocID,
		Page: f.Page, Offset: f.Offset, Token: f.Token}
}

// place puts a fact in its side of a pair, refusing a side filled twice.
func place(pair *DebtPair, f *fact.Fact, where string) error {
	side := &pair.Principal
	if f.Category == debtInterest {
		side = &pair.Interest
	}
	if *side != nil {
		return fmt.Errorf("%s: %s prints two %s figures, %s and %s", DebtServiceProjection,
			where, f.Category, (*side).FactID, f.ID)
	}
	*side = figureOf(f)
	return nil
}

// debtIssueRows lays the by-issue facts out one row per issue, in the order
// debtIssues lists them, and one cell per column, oldest first.
func debtIssueRows(facts []*fact.Fact) ([]DebtIssue, []DebtColumn, error) {
	seen := map[Column]bool{}
	for _, f := range facts {
		seen[ColumnOf(f)] = true
	}
	cols := make([]Column, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sortColumns(cols)
	at := map[Column]int{}
	columns := make([]DebtColumn, len(cols))
	for i, c := range cols {
		at[c] = i
		columns[i] = DebtColumn{FiscalYear: c.FiscalYear, FiscalYearLabel: fiscalYearLabel(c.FiscalYear),
			Basis: string(c.Basis)}
	}

	rows := make([]DebtIssue, len(debtIssues))
	index := map[string]int{}
	for i, is := range debtIssues {
		index[is.printed] = i
		rows[i] = DebtIssue{Label: is.label, RowLabel: is.printed, Activity: is.activity,
			Payer: is.payer, Cells: make([]DebtPair, len(cols))}
	}
	named := make([]bool, len(debtIssues))
	for _, f := range facts {
		i, ok := index[f.RowLabel]
		if !ok {
			return nil, nil, hint.With(
				fmt.Errorf("%s: fact %s prints issue %q, which debtIssues does not name",
					DebtServiceProjection, f.ID, f.RowLabel),
				"an issue the page prints and this site does not label would be shown "+
					"under no activity; add it to debtIssues")
		}
		named[i] = true
		if err := place(&rows[i].Cells[at[ColumnOf(f)]], f,
			fmt.Sprintf("issue %q %s", f.RowLabel, ColumnOf(f))); err != nil {
			return nil, nil, err
		}
	}
	for i, ok := range named {
		if !ok {
			return nil, nil, fmt.Errorf("%s: debtIssues names %q and no fact prints it; "+
				"a label that matches nothing checks nothing", DebtServiceProjection,
				debtIssues[i].printed)
		}
	}
	return rows, columns, nil
}

// debtRequirementRows lays the requirements out one period per printed row,
// oldest first, and one cell per fund type, totalled per fund type.
func debtRequirementRows(facts []*fact.Fact) (DebtRequirements, error) {
	out := DebtRequirements{}
	at := map[string]int{}
	for i, t := range debtFundTypes {
		at[t.key] = i
		ft := DebtFundType{Key: t.key, Label: t.label, FundGroup: t.fundGroup}
		if t.fund != 0 {
			n := t.fund
			ft.Fund = &n
		}
		out.FundTypes = append(out.FundTypes, ft)
		out.Totals = append(out.Totals, DebtTotal{FundType: t.key, FactIDs: []string{}})
	}

	periods := map[Column]*DebtPeriod{}
	named := make([]bool, len(debtFundTypes))
	for _, f := range facts {
		key := fundTypeOf(f)
		i, ok := at[key]
		if !ok || key == "" {
			return out, fmt.Errorf("%s: fact %s is fund group %q fund %s, which no requirements "+
				"fund type names", DebtServiceProjection, f.ID, f.FundGroup, fact.FundString(f.Fund))
		}
		named[i] = true
		c := ColumnOf(f)
		if f.RowLabel != YearLabelOf(c.FiscalYear, c.Through) {
			return out, fmt.Errorf("%s: fact %s prints %q and is %s", DebtServiceProjection,
				f.ID, f.RowLabel, c)
		}
		per := periods[c]
		if per == nil {
			per = &DebtPeriod{FiscalYear: c.FiscalYear, FiscalYearThrough: f.FiscalYearThrough,
				Label: f.RowLabel, Years: 1, Cells: make([]DebtPair, len(debtFundTypes))}
			if c.Through != 0 {
				per.Years = c.Through - c.FiscalYear + 1
			}
			periods[c] = per
		}
		if err := place(&per.Cells[i], f, fmt.Sprintf("%s %s", debtFundTypes[i].label, f.RowLabel)); err != nil {
			return out, err
		}
		t := &out.Totals[i]
		if f.Category == debtPrincipal {
			t.PrincipalCents += f.AmountCents
		} else {
			t.InterestCents += f.AmountCents
		}
		t.FactIDs = append(t.FactIDs, f.ID)
	}
	for i, ok := range named {
		if !ok {
			return out, fmt.Errorf("%s: fund type %q is printed by no fact", DebtServiceProjection,
				debtFundTypes[i].label)
		}
	}
	cols := make([]Column, 0, len(periods))
	for c := range periods {
		cols = append(cols, c)
	}
	sortColumns(cols)
	for _, c := range cols {
		out.Periods = append(out.Periods, *periods[c])
	}
	for i := range out.Totals {
		sort.Strings(out.Totals[i].FactIDs)
	}
	return out, nil
}
