package check

import "maps"

// residualNodes are the spine's flow endpoints, each with the reason a
// fund-level schedule cannot decompose it: money pp.66-67 print at group grain
// and pp.127-140 and pp.167-170 print nowhere finer. Declared rather than
// inferred, so a row the drill-down drops is unaccounted, not absorbed.
//
// An endpoint is residual per group, never split: where the fund-level
// document carries nothing from it into that group's funds, not where it
// carries it whole. transfers/in is both -- residual into general, decomposed
// into enterprise. site/app.js spells that rule again (fisc-8wn7). The keys
// are endpointTiers', and a test holds them equal. The reasons quote no
// figure: one reason is shown under every column.
var residualNodes = map[string]string{
	"fund-balance/draw": "a negative change in working capital, inferred from pp.66-67's " +
		"Change in Working Capital row and drawn into the group. pp.127-140 print no " +
		"fund-balance row at all, so no fund receives it",

	"fund-balance/contribution": "a positive change in working capital, inferred from " +
		"pp.66-67's Change in Working Capital row and drawn out of the group. pp.127-140 " +
		"print no fund-balance row at all, so no fund pays it",

	"fund-balance/reserve-increase": "a printed row of pp.66-67 that the city books against " +
		"the group as a whole: it has no division and no object category on pp.167-170, " +
		"which decompose expenditure and nothing else",

	"transfers/in": "pp.66-67 print Transfers In per fund group, and pp.127-140, which this " +
		"chart draws the funds from, print it for none of this group's funds, so the group's " +
		"transfer in is carried here whole",

	"transfers/out": "pp.66-67 print Transfers Out per fund group, and the pages this chart " +
		"is drawn from print no transfer out, so the group's transfer out leaves beside its " +
		"funds rather than through one",
}

// ResidualNodes is the declared residual set, for the seam that carries it to
// the chart. A copy, so no caller can widen the published set.
func ResidualNodes() map[string]string { return maps.Clone(residualNodes) }
