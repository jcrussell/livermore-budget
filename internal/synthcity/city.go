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
// prints it with (which data/taxonomy.yaml resolves to one line), and cents.
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

// City is the whole spec.
type City struct {
	Divisions []Division
	Funds     []Fund
	Transfers []Transfer
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
		Divisions: []Division{
			{"city-council", "city-council", 85},
			{"patrol", "police-department", 119},
			{"support-services", "police-department", 119},
			{"maintenance", "public-works", 124},
			{"water-resources", "public-works", 124},
			{"information-technology", "administrative-services", 97},
			{"finance", "administrative-services", 97},
			{"housing-and-human-services", "community-development", 101},
		},
		Funds: []Fund{
			{Number: 100, Group: "general", Beginning: dollars(30_000_000),
				Revenue: []Line{
					{"taxes/property", "Current Year - Secured", dollars(30_000_000)},
					{"taxes/sales", "Sales & Use Tax", dollars(15_000_000)},
					{"taxes/other", "Business License Tax", dollars(4_000_000)},
					{"licenses-and-permits", "Building Permits", dollars(2_000_000)},
					{"charges-for-services", "Administrative Cost Recovery", dollars(3_000_000)},
					{"use-of-money-and-property", "Interest Income", dollars(1_000_000)},
					{"fines-and-forfeitures", "Vehicle Code Fines", dollars(500_000)},
					{"intergovernmental", "Grant- State", dollars(1_500_000)},
					{"miscellaneous-revenue", "Cardroom Revenue", dollars(1_000_000)},
				},
				Spend: []Spend{
					{"city-council", wages, dollars(1_000_000)},
					{"patrol", wages, dollars(20_000_000)},
					{"patrol", supplies, dollars(4_000_000)},
					{"support-services", wages, dollars(6_000_000)},
					{"maintenance", supplies, dollars(8_000_000)},
					{"finance", wages, dollars(3_000_000)},
					{"housing-and-human-services", supplies, dollars(2_000_000)},
				}},
			{Number: 610, Group: "enterprise", Beginning: dollars(10_000_000),
				Revenue: []Line{{"charges-for-services", "Charges for Services", dollars(6_000_000)}},
				Spend:   []Spend{{"water-resources", wages, dollars(3_000_000)}, {"water-resources", supplies, dollars(3_500_000)}}},
			{Number: 620, Group: "enterprise", Beginning: dollars(12_000_000),
				Revenue: []Line{{"charges-for-services", "Charges for Services", dollars(9_000_000)}},
				Spend:   []Spend{{"maintenance", supplies, dollars(6_000_000)}, {"maintenance", capital, dollars(1_000_000)}}},
			{Number: 510, Group: "capital", Beginning: dollars(8_000_000),
				Revenue: []Line{{"intergovernmental", "Intergovernmental", dollars(1_000_000)}},
				Spend:   []Spend{{"maintenance", capital, dollars(3_000_000)}},
				Why:     "spends past what it takes in beside 550, which saves: a group whose funds' balances move apart"},
			{Number: 550, Group: "capital", Beginning: dollars(2_000_000),
				Revenue: []Line{{"intergovernmental", "Intergovernmental", dollars(2_000_000)}},
				Spend:   []Spend{{"maintenance", capital, dollars(500_000)}}},
			{Number: 401, Group: "debt-service", Beginning: dollars(500_000),
				Spend: []Spend{{"finance", debt, dollars(2_000_000)}}},
			{Number: 730, Group: "internal-service", Beginning: dollars(4_000_000),
				Revenue: []Line{{"charges-for-services", "Charges for Services", dollars(2_500_000)}},
				Spend:   []Spend{{"information-technology", supplies, dollars(2_400_000)}}},
		},
		Transfers: []Transfer{
			{From: 100, To: 401, Cents: dollars(2_000_000)},
			{From: 100, To: 610, Cents: dollars(1_000_000)},
			{From: 620, To: 610, Cents: dollars(500_000), Why: "a transfer within one fund group"},
		},
	}
	c.Funds = append(c.Funds, specialRevenue()...)
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
func specialRevenue() []Fund {
	out := make([]Fund, 0, len(specialRevenueFunds))
	for i, n := range specialRevenueFunds {
		size := int64(len(specialRevenueFunds) - i)
		f := Fund{Number: n, Group: "special-revenue", Beginning: dollars(1_000_000),
			Revenue: []Line{{"intergovernmental", "Intergovernmental", dollars(40_000 * size)}},
			Spend:   []Spend{{"housing-and-human-services", supplies, dollars(30_000 * size)}}}
		switch n {
		case 200:
			f.Revenue[0].Cents, f.Spend[0].Cents = dollars(3_000_000), dollars(2_800_000)
			f.Why = "the one large fund of the group the cap keeps named"
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
