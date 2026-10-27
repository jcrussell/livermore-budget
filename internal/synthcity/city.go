// Package synthcity is a made-up city in the shape of the Budget Book: funds
// with revenue lines and spending by division, transfers between them and
// balances carried through the year, declared in round figures so every
// total is known, and Facts renders it as the facts every schedule a column
// draws would print. Funds, divisions and revenue lines are the real
// registry's (data/*.yaml), so the real labels resolve; every amount is the
// spec's. Each planted shape names the mechanism it is there for.
package synthcity

import "github.com/jcrussell/livermore-budget/internal/vocab"

// Column is one printed column and the percentage of the spec's figures it
// prints: base cents are whole dollars, so cents*Pct/100 is exact and every
// tie the spec holds in one column holds in all.
type Column struct {
	Year  int
	Basis vocab.Basis
	Pct   int64
	// Spine is whether pp.66-67 print this column, as they do the two
	// adopted years only. A fund's balance opens each column where the
	// column before closed it, so Pct scales flows and not balances.
	Spine bool
}

// Columns are the four the Budget Book prints.
var Columns = []Column{
	{Year: 2024, Basis: vocab.BasisActual, Pct: 90},
	{Year: 2025, Basis: vocab.BasisRevised, Pct: 95},
	{Year: 2026, Basis: vocab.BasisAdopted, Pct: 100, Spine: true},
	{Year: 2027, Basis: vocab.BasisAdopted, Pct: 110, Spine: true},
}

// Line is one revenue row of a fund: its spine category, the label the page
// prints it with (which data/taxonomy.yaml resolves to one line), and cents,
// negative for a row printed as a reduction of its category.
type Line struct {
	Category, Label string
	Cents           int64
}

// Spend is what a fund spends through one division in one object category.
type Spend struct {
	Division, Category string
	Cents              int64
}

// Fund is one fund of data/funds.yaml.
type Fund struct {
	Number int
	// Group is the fund's data/funds.yaml type.
	Group     string
	Revenue   []Line
	Spend     []Spend
	Beginning int64
	// Years limits the columns the fund prints figures in, its rows a dash in
	// every other; empty is every column.
	Years []int
	// Why names the mechanism a fund is planted for, when it is.
	Why string
}

// Transfer is one p76 row: payer, receiver, cents.
type Transfer struct {
	From, To int
	Cents    int64
	Why      string
}

// Division is one data/departments.yaml division and its department.
type Division struct {
	Slug, Department string
	// Page is the pp.85-125 page its department prints on.
	Page int
}

// Gap is a cell pp.66-67 print above what their funds' own rows sum to, as
// the Budget Book prints Internal Service Funds Services & Supplies in
// FY2026-27: the spine carries it into the group's change and ending
// balance, and Facts declares the exceptions that hold it apart.
type Gap struct {
	Year     int
	Group    string
	Category string
	Cents    int64
	Why      string
}

// DivisionTransfer is a Transfers Out row pp.85-125 print under one
// division, as they print Maintenance's in FY2023-24: in the department's
// spending and the fund's funding of it, and in no cut, so the department's
// two blocks are declared apart.
type DivisionTransfer struct {
	Year     int
	Division string
	Fund     int
	Cents    int64
	Why      string
}

// City is the whole spec.
type City struct {
	Divisions         []Division
	Funds             []Fund
	Transfers         []Transfer
	Gaps              []Gap
	DivisionTransfers []DivisionTransfer
}

const (
	wages    = "wages-and-benefits"
	supplies = "services-and-supplies"
	capital  = "capital-outlay"
	debt     = "debt-services"
)

// dollars is whole dollars as cents.
func dollars(d int64) int64 { return d * 100 }

// Default is the city the client fixtures are exported from.
func Default() City {
	c := City{
		// Every data/departments.yaml division, as the department page draws
		// every department and each one whole.
		Divisions: []Division{
			{"city-council", "city-council", 85},
			{"city-manager", "city-manager", 89},
			{"city-clerk", "city-manager", 89},
			{"city-attorney", "city-attorney", 93},
			{"risk-management", "city-attorney", 93},
			{"library", "library-department", 115},
			{"innovation-and-economic-devel", "innovation-and-economic-development", 111},
			{"airport", "innovation-and-economic-development", 111},
			{"general-services", "general-services", 108},
			{"administrative-services", "administrative-services", 97},
			{"finance", "administrative-services", 97},
			{"human-resources", "administrative-services", 97},
			{"information-technology", "administrative-services", 97},
			{"cybersecurity", "administrative-services", 97},
			{"police-administration", "police-department", 119},
			{"patrol", "police-department", 119},
			{"support-services", "police-department", 119},
			{"special-operations", "police-department", 119},
			{"horizons", "police-department", 119},
			{"community-development-admin", "community-development", 101},
			{"building-and-safety", "community-development", 101},
			{"engineering", "community-development", 101},
			{"housing-and-human-services", "community-development", 101},
			{"planning", "community-development", 101},
			{"public-works-administration", "public-works", 124},
			{"maintenance", "public-works", 124},
			{"environmental-services", "public-works", 124},
			{"water-resources", "public-works", 124},
			{"fire-administration", "fire-department", 105},
		},
		Funds: []Fund{
			{Number: 100, Group: "general", Beginning: dollars(30_000_000),
				Revenue: []Line{
					// Property Taxes is $30,000,000 in FY2025-26, net of two rows the
					// page prints as reductions, as pp.127-130 print ERAF.
					{"taxes/property", "Current Year - Secured", dollars(31_000_000)},
					{"taxes/property", "Current Year - Unsecured", dollars(1_000_000)},
					{"taxes/property", "ERAF", dollars(-1_500_000)},
					{"taxes/property", "RPTTF Reduction", dollars(-500_000)},
					// Two lines of one category reaching one fund, neither a reduction.
					{"taxes/sales", "Sales & Use Tax", dollars(14_000_000)},
					{"taxes/sales", "Prop 172 - Public Sfty Augmnt", dollars(1_000_000)},
					// Other Taxes and Licenses & Permits each print fourteen lines,
					// six past the line cap, so both fold "6 smaller lines" in one
					// column and the two tails are told apart by their category.
					{"taxes/other", "Business License Tax", dollars(1000_000)},
					{"taxes/other", "TOT - Short Term Rental", dollars(800_000)},
					{"taxes/other", "TOT - Hotel", dollars(500_000)},
					{"taxes/other", "Residential Construction Tax", dollars(400_000)},
					{"taxes/other", "Franchise Tax- Garbage", dollars(300_000)},
					{"taxes/other", "Franchise Tax- Gas", dollars(250_000)},
					{"taxes/other", "Business License Registration", dollars(200_000)},
					{"taxes/other", "Industrial Construction Tax", dollars(150_000)},
					{"taxes/other", "Franchise Tax- Cable TV", dollars(120_000)},
					{"taxes/other", "Real Property Transfer Tax", dollars(100_000)},
					{"taxes/other", "Franchise Tax- Electric", dollars(80_000)},
					{"taxes/other", "Business License Penalty", dollars(50_000)},
					{"taxes/other", "In-Lieu Taxes", dollars(30_000)},
					{"taxes/other", "Emt Service Area", dollars(20_000)},
					{"licenses-and-permits", "Electrical Permits", dollars(500_000)},
					{"licenses-and-permits", "Permit Technology Fee", dollars(300_000)},
					{"licenses-and-permits", "Police Permits", dollars(250_000)},
					{"licenses-and-permits", "Street And Curb Permits", dollars(200_000)},
					{"licenses-and-permits", "Fire Permits", dollars(150_000)},
					{"licenses-and-permits", "Transportation Permit", dollars(120_000)},
					{"licenses-and-permits", "Short Term Rental Permit", dollars(100_000)},
					{"licenses-and-permits", "Hazmat Permits", dollars(90_000)},
					{"licenses-and-permits", "Tobacco License- Fee", dollars(80_000)},
					{"licenses-and-permits", "Use And Variance Permits", dollars(70_000)},
					{"licenses-and-permits", "Plumbing Gas Permits", dollars(60_000)},
					{"licenses-and-permits", "Tobacco License- Registration", dollars(40_000)},
					{"licenses-and-permits", "Building Permits", dollars(25_000)},
					{"licenses-and-permits", "Mechanical Permits", dollars(15_000)},
					{"charges-for-services", "Administrative Cost Recovery", dollars(3_000_000)},
					// Two lines under the $10,000 smallest-flow stop: listed by the
					// search, and merged into the smaller flows once revealed.
					{"charges-for-services", "Animal Shelter Fees & Charges", dollars(5_000)},
					{"charges-for-services", "Library Fees", dollars(3_000)},
					{"use-of-money-and-property", "Interest Income", dollars(1_000_000)},
					// Two lines under the $1,000,000 smallest-flow stop, which the
					// filter merges into one smaller-flows mark.
					{"fines-and-forfeitures", "Vehicle Code Fines", dollars(500_000)},
					{"contributions-outsourced", "Contribution Outside Services", dollars(300_000)},
					{"intergovernmental", "Grant- State", dollars(1_500_000)},
					{"miscellaneous-revenue", "Cardroom Revenue", dollars(1_000_000)},
				},
				Spend: []Spend{
					{"city-council", wages, dollars(1_000_000)},
					{"city-attorney", wages, dollars(2_000_000)},
					{"general-services", supplies, dollars(5_000_000)},
					{"innovation-and-economic-devel", wages, dollars(1_500_000)},
					{"library", wages, dollars(3_000_000)},
					{"fire-administration", wages, dollars(9_000_000)},
					{"police-administration", wages, dollars(2_000_000)},
					{"patrol", wages, dollars(14_000_000)},
					{"patrol", supplies, dollars(2_500_000)},
					{"support-services", wages, dollars(4_000_000)},
					{"special-operations", wages, dollars(1_000_000)},
				}},
			{Number: 610, Group: "enterprise", Beginning: dollars(10_000_000),
				Revenue: []Line{{"charges-for-services", "Charges for Services", dollars(6_000_000)}},
				Spend:   []Spend{{"water-resources", wages, dollars(3_000_000)}, {"water-resources", supplies, dollars(3_500_000)}}},
			{Number: 620, Group: "enterprise", Beginning: dollars(12_000_000),
				Revenue: []Line{{"charges-for-services", "Charges for Services", dollars(9_000_000)}},
				Spend:   []Spend{{"maintenance", supplies, dollars(6_000_000)}, {"maintenance", capital, dollars(1_000_000)}}},
			{Number: 510, Group: "capital", Beginning: dollars(8_000_000),
				Revenue: []Line{{"intergovernmental", "Intergovernmental", dollars(1_000_000)}},
				Spend:   []Spend{{"engineering", capital, dollars(3_000_000)}},
				Why:     "spends past what it takes in beside 550, which saves: a group whose funds' balances move apart"},
			{Number: 550, Group: "capital", Beginning: dollars(2_000_000),
				Revenue: []Line{{"intergovernmental", "Intergovernmental", dollars(2_000_000)}},
				Spend:   []Spend{{"engineering", capital, dollars(500_000)}}},
			{Number: 401, Group: "debt-service", Beginning: dollars(500_000),
				Revenue: []Line{{"use-of-money-and-property", "Use of Money & Prop", dollars(20_000)}},
				Spend:   []Spend{{"finance", debt, dollars(2_000_000)}},
				Why:     "earns interest far under a pixel of the opening chart: a hairline on the spine"},
			{Number: 470, Group: "permanent",
				Why: "a group every row of which prints a dash, which pp.66-67 do not print: listed last and hueless"},
			{Number: 730, Group: "internal-service", Beginning: dollars(4_000_000),
				Revenue: []Line{{"charges-for-services", "Charges for Services", dollars(2_500_000)}},
				Spend:   []Spend{{"information-technology", supplies, dollars(2_700_000)}}},
		},
		Gaps: []Gap{{Year: 2027, Group: "internal-service", Category: supplies, Cents: dollars(250_000),
			Why: "a declared remainder: the root draws the spine's cell whole, and opening the group draws " +
				"what its funds fall short of as an inferred ribbon of its own"}},
		DivisionTransfers: []DivisionTransfer{{Year: 2024, Division: "maintenance", Fund: 620, Cents: dollars(300_000),
			Why: "a department's Transfers Out, drawn as no expenditure inside its spending, and its two blocks held apart"}},
		Transfers: []Transfer{
			{From: 100, To: 401, Cents: dollars(2_000_000)},
			{From: 100, To: 610, Cents: dollars(1_000_000)},
			{From: 620, To: 610, Cents: dollars(500_000), Why: "a transfer within one fund group"},
		},
	}
	c.Funds = append(c.Funds, specialRevenue()...)
	// The General Fund supports eight small special revenue funds, so
	// Transfers Out has more receivers than its cap names and folds a tail.
	for i, n := range []int{220, 221, 222, 223, 224, 225, 226, 240} {
		c.Transfers = append(c.Transfers, Transfer{From: 100, To: n, Cents: dollars(int64(40_000 + 10_000*i)),
			Why: "one of more receivers than the transfers-out cap names"})
	}
	return c
}

// specialRevenueFunds are data/funds.yaml special revenue funds, as many as
// the real group prints, so the tier-3 cap folds a tail of real length.
var specialRevenueFunds = []int{
	200, 201, 202, 203, 204, 205, 206, 207, 210, 211, 220, 221, 222, 223,
	224, 225, 226, 240, 241, 242, 260, 280, 281, 282, 283, 284, 285, 286,
}

// specialRevenue is the group the cap folds: one large fund, the rest small
// and falling in size, the last of them sub-pixel beside the General Fund.
// Every other fund draws on its balance and the rest save, so the group's
// change is the net of funds moving both ways, as the Budget Book's is.
func specialRevenue() []Fund {
	out := make([]Fund, 0, len(specialRevenueFunds))
	for i, n := range specialRevenueFunds {
		size := int64(len(specialRevenueFunds) - i)
		spend := int64(30_000)
		if i%2 == 1 {
			spend = 50_000
		}
		f := Fund{Number: n, Group: "special-revenue", Beginning: dollars(2_000_000),
			Revenue: []Line{{"intergovernmental", "Intergovernmental", dollars(40_000 * size)}},
			Spend:   []Spend{{"library", supplies, dollars(spend * size)}}}
		switch n {
		case 200:
			f.Revenue[0].Cents, f.Spend[0].Cents = dollars(3_000_000), dollars(2_000_000)
			f.Spend = append(f.Spend, Spend{"housing-and-human-services", wages, dollars(800_000)})
			f.Why = "the one large fund of the group the cap keeps named, spending in two objects"
		case 286:
			f.Revenue[0].Cents, f.Spend[0].Cents = dollars(100), dollars(100)
			f.Why = "a ribbon far under a pixel beside the General Fund's"
		case 285:
			f.Years, f.Beginning = []int{2026}, 0
			f.Spend[0].Cents = f.Revenue[0].Cents
			f.Why = "prints figures in FY2026 only and dashes elsewhere, so the two adopted years' tails differ"
		}
		out = append(out, f)
	}
	return out
}
