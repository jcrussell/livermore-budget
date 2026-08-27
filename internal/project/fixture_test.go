package project

import (
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// The fixture is the citywide spine, Budget Book pp.66-67, because a
// projection can only be checked against numbers somebody has added up by
// hand. testdata/sankey.golden.json is that hand derivation; the table below
// is the same schedule as facts, and the two are compared in
// TestSankeyReproducesGoldenFile.
//
// The facts are constructed here rather than read from a build of the real
// corpus so this package's tests do not depend on the mapping rules landing
// first. Their ids are not invented: they come from fact.MakeID over the same
// tuple the builder hashes, which is what lets the golden file's fact ids be
// asserted rather than trusted.
const (
	testDoc   = "livermore-budget-fy2026-2027"
	testScope = "all-funds-gross"
	testYear  = 2026
	testBasis = mapping.BasisAdopted
)

// testOptions are the options the golden file was derived under.
func testOptions() Options {
	return Options{
		Columns: []Column{{FiscalYear: testYear, Basis: testBasis}},
		Scopes:  []string{testScope},
		Version: "testdata/sankey.golden.json (hand-derived, Wave 0)",
	}
}

// spineGroups is the fund-group column order of spineRows, left to right as
// the two pages print them.
var spineGroups = [6]string{
	"general", "enterprise", "capital", "debt-service", "special-revenue", "internal-service",
}

// spinePage says which page carries a fund group's column: p66 carries the row
// labels and the General Fund and Enterprise columns, p67 continues the same
// rows for the other four with nothing but numbers.
func spinePage(group string) int {
	if group == "general" || group == "enterprise" {
		return 66
	}
	return 67
}

// spineRow is one printed row of the schedule across all six columns.
//
// Every cell is a fact, including the zeros: pp.66-67 print a dash in each of
// them, and a dash in this corpus means "exists and is zero" rather than "not
// applicable". So the fixture yields 20 rows x 6 groups = 120 facts against
// only 58 links — a zero-valued cell earns no link, and neither do the two
// stock rows. That gap is the whole reason counts.facts and counts.facts_cited
// are both published.
type spineRow struct {
	kind     mapping.Kind
	rule     string
	category string
	label    string
	cents    [6]int64
}

var spineRows = []spineRow{
	{mapping.KindRevenue, "spine-revenues", "taxes/property", "Property Taxes", [6]int64{6414376200, 0, 0, 0, 531565200, 0}},
	{mapping.KindRevenue, "spine-revenues", "taxes/other", "Other Taxes", [6]int64{2380019600, 0, 0, 0, 17000000, 0}},
	{mapping.KindRevenue, "spine-revenues", "intergovernmental", "Intergovernmental", [6]int64{432816400, 0, 818845300, 0, 682483500, 1884483400}},
	{mapping.KindRevenue, "spine-revenues", "charges-for-services", "Charges for Services", [6]int64{613020700, 6225082400, 152942900, 0, 212929700, 0}},
	{mapping.KindRevenue, "spine-revenues", "use-of-money-and-property", "Use of Money And Property", [6]int64{848816800, 483653700, 134600000, 0, 74113200, 12500000}},
	{mapping.KindRevenue, "spine-revenues", "contributions-outsourced", "Contributions Outsourced", [6]int64{7636000, 0, 0, 0, 193400000, 0}},
	{mapping.KindRevenue, "spine-revenues", "miscellaneous-revenue", "Miscellaneous Revenue", [6]int64{218070800, 42690000, 1608200000, 0, 1135064400, 0}},
	{mapping.KindRevenue, "spine-revenues", "taxes/sales", "Sales Taxes", [6]int64{4108660600, 0, 0, 0, 0, 0}},
	{mapping.KindRevenue, "spine-revenues", "fines-and-forfeitures", "Fines & Forfeitures", [6]int64{38650000, 0, 0, 0, 0, 0}},
	{mapping.KindRevenue, "spine-revenues", "licenses-and-permits", "Licenses & Permits", [6]int64{725279900, 0, 0, 0, 0, 0}},
	{mapping.KindExpenditure, "spine-expenditures", "wages-and-benefits", "Wages & Benefits", [6]int64{8180101100, 1606653600, 18620500, 0, 371671700, 597065400}},
	{mapping.KindExpenditure, "spine-expenditures", "services-and-supplies", "Services & Supplies", [6]int64{6284979100, 3959324000, 87515000, 0, 1471493900, 1548918900}},
	{mapping.KindExpenditure, "spine-expenditures", "capital-outlay", "Capital Outlay", [6]int64{0, 69990000, 0, 0, 60000000, 254702400}},
	{mapping.KindExpenditure, "spine-expenditures", "debt-services", "Debt Services", [6]int64{0, 69405400, 0, 698459700, 23590500, 107050000}},
	{mapping.KindTransferIn, "spine-transfers-in", "transfers/in", "TRANSFER IN:", [6]int64{48040000, 1324700000, 0, 698459700, 81400000, 0}},
	{mapping.KindTransferOut, "spine-transfers-out", "transfers/out", "TRANSFER OUT:", [6]int64{1003779700, 1981314700, 2858474000, 0, 113705000, 4000000}},
	{mapping.KindFundBalance, "spine-fund-balance", "fund-balance/reserve-increase", "ADDITION TO RESERVES", [6]int64{469942500, 0, 0, 0, 0, 0}},
	// Signed as printed: the General Fund's FY2026 CHANGE IN WORKING CAPITAL
	// is "(1,034,154)". Three of the six columns are negative, which is what
	// makes the fund-balance/draw side of the decomposition testable here.
	{mapping.KindFundBalance, "spine-fund-balance", "fund-balance/change", "CHANGE IN WORKING CAPITAL", [6]int64{-103415400, 389438400, -250021300, 0, 887494900, -614753300}},
	// Stocks, not flows. They are facts the city printed and they must survive
	// into facts.jsonl, but a balance carried into the year is not money moving
	// and gets no link. Debt Service carries a literal $2.
	{mapping.KindFundBalance, "spine-fund-balance", "fund-balance/beginning", "BEGINNING WORKING CAPITAL", [6]int64{176661300, 10339445000, 11071354500, 200, 7729007300, 2183726500}},
	{mapping.KindFundBalance, "spine-fund-balance", "fund-balance/ending", "ENDING WORKING CAPITAL", [6]int64{73245900, 10728883400, 10821333200, 200, 8616502200, 1568973200}},
}

// spineFacts renders the fixture as facts for one fiscal year.
func spineFacts(t *testing.T, year int) []fact.Fact {
	t.Helper()
	var out []fact.Fact
	for _, r := range spineRows {
		for i, cents := range r.cents {
			out = append(out, cellSpec{
				kind:     r.kind,
				rule:     r.rule,
				category: r.category,
				label:    r.label,
				group:    spineGroups[i],
				cents:    cents,
				year:     year,
			}.fact(t))
		}
	}
	return out
}

// cellSpec is one cell of a schedule, with everything that does not matter to
// this package defaulted. Zero year means testYear and zero sign means
// positive, so a test states only what it is about.
type cellSpec struct {
	kind     mapping.Kind
	rule     string
	category string
	label    string
	group    string
	cents    int64
	year     int
	sign     mapping.Sign
	page     int
}

func (c cellSpec) fact(t *testing.T) fact.Fact {
	t.Helper()
	if c.year == 0 {
		c.year = testYear
	}
	if c.sign == "" {
		c.sign = mapping.SignPositive
	}
	if c.rule == "" {
		c.rule = ruleFor(t, c.kind)
	}
	if c.label == "" {
		c.label = c.category
	}
	if c.page == 0 {
		c.page = spinePage(c.group)
	}
	rowPath := fact.RowPath(mapping.Row{Category: c.category})
	columnPath := fact.ColumnPath(mapping.Column{FundGroup: c.group}, testScope)
	return fact.Fact{
		ID:          fact.MakeID(testDoc, c.rule, rowPath, c.label, columnPath, c.year, testBasis),
		DocID:       testDoc,
		Page:        c.page,
		RuleID:      c.rule,
		Kind:        c.kind,
		Basis:       testBasis,
		Scope:       testScope,
		FiscalYear:  c.year,
		RowPath:     rowPath,
		RowLabel:    c.label,
		Category:    c.category,
		ColumnPath:  columnPath,
		FundGroup:   c.group,
		Sign:        c.sign,
		Units:       "dollars",
		AmountCents: c.cents,
	}
}

// ruleFor is the rule id the spine uses for each kind. It is part of the fact
// id, so the golden file's ids only reproduce with these exact strings.
func ruleFor(t *testing.T, k mapping.Kind) string {
	t.Helper()
	switch k {
	case mapping.KindRevenue:
		return "spine-revenues"
	case mapping.KindExpenditure:
		return "spine-expenditures"
	case mapping.KindTransferIn:
		return "spine-transfers-in"
	case mapping.KindTransferOut:
		return "spine-transfers-out"
	case mapping.KindFundBalance:
		return "spine-fund-balance"
	default:
		t.Fatalf("no fixture rule for kind %q", k)
		return ""
	}
}

// facts is shorthand for a handful of cells.
func facts(t *testing.T, specs ...cellSpec) []fact.Fact {
	t.Helper()
	out := make([]fact.Fact, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.fact(t))
	}
	return out
}

// stubLabels stands in for internal/registry, keyed the way registry.Label is
// keyed: by data/taxonomy.yaml category slug.
//
// Fund names live in a second map because the two vocabularies must not share a
// key space -- the Labels doc comment says why, and a fund number and a category
// slug could not collide anyway, which is exactly the property worth keeping
// visible rather than relying on.
type stubLabels map[string]string

func (s stubLabels) Label(slug string) (string, bool) {
	l, ok := s[slug]
	return l, ok
}

// FundName satisfies Labels. The spine names no fund, so every stub returns a
// miss unless a test supplies stubFunds instead.
func (stubLabels) FundName(int) (string, bool) { return "", false }

// The four methods a fund- or department-keyed document needs. The spine keys on
// neither, so these are misses here and stubFundFlows supplies them where a test
// needs a real answer.
//
// FundType returning false is the honest stub: it is the PARENT EDGE, and a stub
// that invented a type would let a test pass over a hierarchy nothing built.
func (stubLabels) FundType(int) (string, bool)         { return "", false }
func (stubLabels) ConstraintTier(int) string           { return "" }
func (stubLabels) RestrictionNote(int) string          { return "" }
func (stubLabels) DivisionLabel(string) (string, bool) { return "", false }

// stubFunds is stubLabels with fund names attached, for the documents that key
// on funds rather than on categories.
type stubFunds struct {
	stubLabels
	funds map[int]string
}

func (s stubFunds) FundName(number int) (string, bool) {
	n, ok := s.funds[number]
	return n, ok
}

// goldenLabels is every category label data/taxonomy.yaml carries for the
// spine, transcribed, so the golden comparison exercises the same path a build
// with a real registry attached will.
//
// Two sets of labels are deliberately absent. The six fund groups have no
// label anywhere in data/ — funds.yaml binds a fund to a type and records no
// words for the type — and the two fund-balance nodes we inferred are in no
// data file at all; both come from builtinLabels.
//
// `fund-balance/reserve-increase` is absent for a different reason: the
// taxonomy labels it "Reserve Increase / (Use)", the p75 column header, while
// testdata/sankey.golden.json shows p66's "Addition to Reserves". Listing the
// taxonomy's label here would fail the golden comparison, which is the
// disagreement rather than a bug in either side.
var goldenLabels = stubLabels{
	"taxes/property":            "Property Taxes",
	"taxes/sales":               "Sales Taxes",
	"taxes/other":               "Other Taxes",
	"licenses-and-permits":      "Licenses & Permits",
	"fines-and-forfeitures":     "Fines & Forfeitures",
	"intergovernmental":         "Intergovernmental",
	"charges-for-services":      "Charges for Services",
	"use-of-money-and-property": "Use of Money And Property",
	"contributions-outsourced":  "Contributions Outsourced",
	"miscellaneous-revenue":     "Miscellaneous Revenue",
	"wages-and-benefits":        "Wages & Benefits",
	"services-and-supplies":     "Services & Supplies",
	"capital-outlay":            "Capital Outlay",
	"debt-services":             "Debt Services",
	"transfers/in":              "Transfers In",
	"transfers/out":             "Transfers Out",
}

// buildGraph builds with the golden labels and fails the test on error.
func buildGraph(t *testing.T, fs []fact.Fact, o Options) *Graph {
	t.Helper()
	g, err := (&Sankey{Labels: goldenLabels}).Graph(fs, o)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	return g
}

// linkBetween returns the one link from source to target, or fails.
func linkBetween(t *testing.T, g *Graph, source, target string) Link {
	t.Helper()
	for _, l := range g.Links {
		if l.Source == source && l.Target == target {
			return l
		}
	}
	t.Fatalf("no link %s -> %s in %d links", source, target, len(g.Links))
	return Link{}
}

// hasLink reports whether any link joins the two nodes.
func hasLink(g *Graph, source, target string) bool {
	for _, l := range g.Links {
		if l.Source == source && l.Target == target {
			return true
		}
	}
	return false
}

// nodeByID returns the node with an id, or fails.
func nodeByID(t *testing.T, g *Graph, id string) Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %q in %d nodes", id, len(g.Nodes))
	return Node{}
}
