package check

import "maps"

// residualNodes are the spine's flow endpoints, each with the reason a
// fund-level schedule cannot decompose it.
//
// THE DRILL PUTS ONE DOCUMENT INSIDE THE OTHER, AND THE TOTALS DO NOT MATCH.
// pp.66-67 print a fund group's inflow and outflow whole; pp.127-140 and
// 167-170 print the same money by fund and by division, and they print no
// fund-balance row and decompose no transfer out. What is left over when the
// second is subtracted from the first is the RESIDUAL, and it is money the
// city printed at group grain and nowhere finer -- not an error, and not a
// figure anything here computes.
//
// DECLARED RATHER THAN INFERRED. "The fund-level document carries no link from
// this node" is evidence about today's corpus and not a rule, and a set
// inferred from it would absorb whatever the drill-down dropped next. So an
// endpoint is in the residual only when it is named HERE, and every other gap
// between the two documents is unaccounted.
//
// AN ENDPOINT IS RESIDUAL PER GROUP, NOT PER NODE, and the rule deciding which
// is the documents' own: a declared endpoint's flow into (or out of) a group is
// residual where the fund-level document carries NOTHING from that endpoint
// into that group's funds, and is decomposed where it carries it WHOLE. It is
// never split. Measured on the committed corpus, transfers/in is both:
// pp.127-140 print Transfers In for funds 210/310/400/401/402/610/622/642 and
// for no general-group fund, so the spine's 480,400 into general has no
// fund-level counterpart while enterprise's 13,247,000 is decomposed to the
// cent. A rule that subtracted the fund-level figure instead would make a
// dropped fund row look like residual.
//
// THE SET CROSSES THE SEAM AND THE RULE DOES NOT. The chart's residual node
// copies these spine links verbatim onto one derived node per opened group;
// this map, read through [ResidualNodes], is the set it chooses from, and
// site/app.js decides which of them by the whole-or-nothing rule above, spelled
// there in its own terms. Two copies of a set that must agree drift; one
// declaration cannot. The rule has no seam and is spelled once on each side,
// which is fisc-8wn7.
//
// WHAT HOLDS THE SET. The keys are the five ids endpointTiers declares tiers
// for, and a test pins that the two tables name the same nodes: an endpoint is
// a flow that sits outside the hierarchy, and sitting outside the hierarchy is
// exactly what makes it undecomposable by a schedule that IS the hierarchy.
// The reasons quote no figure, because one reason is shown under every
// column; the ribbons carry the money. What cuts-tie-along-the-lattice
// compares is the claim under them -- the fund-level cuts print no
// fund-balance row and no transfer out by declaration
// (structure.BudgetBookCuts), and the general group's transfer in is a
// declared exception. Deriving this set from those declarations rather than
// restating it beside them is fisc-8wn7 too.
var residualNodes = map[string]string{
	"fund-balance/draw": "a negative change in working capital, inferred from pp.66-67's " +
		"Change in Working Capital row and drawn into the group. pp.127-140 print no " +
		"fund-balance row at all, so no fund receives it",

	"fund-balance/contribution": "the mirror of the draw: a positive change in working " +
		"capital, drawn out of the group to the same inferred node. No fund pays it " +
		"for the same reason no fund receives the draw",

	"fund-balance/reserve-increase": "a printed row of pp.66-67 that the city books against " +
		"the group as a whole: it has no division and no object category on pp.167-170, " +
		"which decompose expenditure and nothing else",

	"transfers/in": "pp.66-67 print Transfers In per fund group, and pp.127-140 print it per " +
		"fund to the cent everywhere but the General Fund, which they print no Transfers In " +
		"row for. The general group's transfer in is therefore carried here whole; p.76 " +
		"prints it per fund, and the transfers chart draws it from there",

	"transfers/out": "pp.66-67 print Transfers Out per fund group and p.76 prints it per paying " +
		"fund, which the transfers chart draws. The schedules this chart is drawn from carry " +
		"none of it: pp.167-170 decompose the General Fund's expenditure and print no " +
		"transfer out, so the group's transfer out leaves beside its divisions rather than " +
		"through one",
}

// ResidualNodes is the declared residual set, for the seam that carries it to
// the chart. A copy, so no caller can widen the published set.
func ResidualNodes() map[string]string { return maps.Clone(residualNodes) }
