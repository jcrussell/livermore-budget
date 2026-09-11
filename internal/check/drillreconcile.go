package check

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/project"
)

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
// DECLARED RATHER THAN INFERRED, for disjointScopes' reason: "the fund-level
// document carries no link from this node" is evidence about today's corpus and
// not a rule, and a set inferred from it would absorb whatever the drill-down
// dropped next. So an endpoint is in the residual only when it is named HERE,
// and every other gap between the two documents is unaccounted.
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
// dropped fund row look like residual, which is the drift this check exists to
// see.
//
// THIS IS THE ONE DECLARATION, AND THE CLIENT CARRIES THE SAME SET. The chart's
// residual node copies these spine links verbatim onto one derived node per
// opened group; its rule for which links to copy is this map's keys and the
// whole-or-nothing rule above, read through [ResidualNodes] rather than spelled
// again. Two copies of a set that must agree drift; one declaration cannot.
//
// The keys are the five ids endpointTiers declares tiers for, and a test pins
// that the two tables name the same nodes: an endpoint is a flow that sits
// outside the hierarchy, and sitting outside the hierarchy is exactly what
// makes it undecomposable by a schedule that IS the hierarchy.
var residualNodes = map[string]string{
	"fund-balance/draw": "a negative change in working capital, inferred from pp.66-67's " +
		"Change in Working Capital row and drawn into the group. pp.127-140 print no " +
		"fund-balance row at all, so no fund receives it: measured, general 1,034,154, " +
		"capital 2,500,213 and internal-service 6,147,533 in FY2026 adopted, and every " +
		"fund-level inflow short by exactly that",

	"fund-balance/contribution": "the mirror of the draw: a positive change in working " +
		"capital, drawn out of the group to the same inferred node. No fund pays it " +
		"for the same reason no fund receives the draw. It carries nothing on FY2026's " +
		"decomposed group, where general's change is a draw, and 2,351,098 on FY2027's, " +
		"where it turns positive -- a set measured on one column alone would have " +
		"missed it",

	"fund-balance/reserve-increase": "a printed row of pp.66-67 that the city books against " +
		"the group as a whole: general's 4,699,425 in FY2026 adopted has no division " +
		"and no object category on pp.167-170, which decompose expenditure and nothing " +
		"else",

	"transfers/in": "pp.66-67 print Transfers In per fund group and pp.127-140 print it per " +
		"fund for eight funds in three groups, to the cent. The general group has no " +
		"such fund, so its 480,400 is residual and the other groups' is decomposed " +
		"whole -- which is why the rule is per group and the exception " +
		"revenue-detail-ties-to-spine declares is this one",

	"transfers/out": "pp.66-67 print Transfers Out per fund group and no fund-level " +
		"schedule prints it from any fund: pp.167-170 decompose the General Fund's " +
		"expenditure and stop there, so general's 10,037,797 in FY2026 adopted leaves " +
		"the group beside its divisions rather than through one",
}

// ResidualNodes is the declared residual set, for the seam that carries it to
// the chart. A copy, so no caller can widen the published set.
func ResidualNodes() map[string]string { return maps.Clone(residualNodes) }

// drillReconcilesAcrossDocuments asserts that the two documents the drill nests
// still describe the same money: for every fiscal column both publish, each
// fund group's spine inflow and outflow equal its fund-level inflow and outflow
// plus the residual the declared endpoints carry, to the cent.
//
// TWO IDENTITIES PER GROUP, AND THE SECOND ONLY WHERE IT CAN BE STATED.
//
//	spine_in(G)  == fund_in(G)  + residual_in(G)
//	spine_out(G) == fund_out(G) + residual_out(G)
//
// where spine_in is every spine link into the group and fund_in every drill-down
// link into one of the group's funds, and the residual is the declared
// endpoints' share under residualNodes' whole-or-nothing rule. The outflow
// identity is stated only for a group the drill-down DECOMPOSES -- one with a
// link out of one of its funds -- because a group whose money ends at its funds
// publishes no fund-level outflow, and absent is not zero. Measured on the
// committed corpus that is general alone in every column, so a column has seven
// subjects: six groups' inflow and one group's outflow.
//
// THE FINDING IS THE UNACCOUNTED AMOUNT, NEVER THE RAW GAP. general's spine
// inflow exceeds its fund-level inflow by 1,514,554 in FY2026 adopted, and that
// difference is supposed to exist: it is the draw and the transfer in, which the
// residual names. Reporting it would be reporting the design. What is reported
// is what is left after the residual is set aside, and on a healthy corpus that
// is zero on every subject.
//
// IT DOES NOT REUSE project.GroupExpenditure FOR THE OUTFLOW SIDE, and the
// reason is what that function counts: the group's links to an object category
// and nothing else, so a flow to an endpoint outside the declaration would be
// invisible to it rather than unaccounted. spine_out is every link out of the
// group precisely so that a new endpoint nothing has declared goes red.
//
// WHAT IT CANNOT WITNESS. Both documents are built from one fact slice in one
// process, so it cannot witness a wrong amount -- a perturbed fact moves both
// sides together, and fact-offset-points-at-token is the check that sees it.
// What it catches is the two documents DRIFTING APART: a selector dropped from
// one schedule and not the other, a fund whose type: moved in data/funds.yaml so
// its revenue changed groups on one side only, a residual row that stopped being
// carried. One drift it cannot see: a group's decomposition of a declared
// endpoint dropped WHOLE, which the whole-or-nothing rule reads as residual.
// That drift is a fact-level one, and revenue-detail-ties-to-spine holds the
// spine's transfer_in cell against the detail's at zero tolerance, so it is
// red there while green here.
type drillReconcilesAcrossDocuments struct{}

var _ Check = (*drillReconcilesAcrossDocuments)(nil)

func (*drillReconcilesAcrossDocuments) ID() string { return "drill-reconciles-across-documents" }
func (*drillReconcilesAcrossDocuments) Tier() int  { return 3 }
func (*drillReconcilesAcrossDocuments) Full() bool { return false }
func (*drillReconcilesAcrossDocuments) Description() string {
	return "for every fiscal column both the spine and the drill-down publish, each fund " +
		"group's spine inflow and outflow equal its fund-level inflow and outflow plus the " +
		"declared residual, to the cent"
}

func (*drillReconcilesAcrossDocuments) Run(_ context.Context, s *Subject) (Result, error) {
	return runDrillReconcile(s, residualNodes)
}

// runDrillReconcile is Run with the residual set as a parameter, so a test can
// damage the declaration without touching the package's.
func runDrillReconcile(s *Subject, residual map[string]string) (Result, error) {
	spines := map[project.Column]projection{}
	for _, p := range s.graphs() {
		if p.Name != project.PublishedProjection {
			continue
		}
		if err := oneColumn(spines, p); err != nil {
			return Result{}, err
		}
	}
	drills := map[project.Column]projection{}
	for _, p := range s.fundFlowsDocuments() {
		if err := oneColumn(drills, p); err != nil {
			return Result{}, err
		}
	}

	var columns []project.Column
	for c := range spines {
		if _, ok := drills[c]; ok {
			columns = append(columns, c)
		}
	}
	sort.Slice(columns, func(i, j int) bool {
		if columns[i].FiscalYear != columns[j].FiscalYear {
			return columns[i].FiscalYear < columns[j].FiscalYear
		}
		return columns[i].Basis < columns[j].Basis
	})

	var findings []Finding
	var summaries []string
	subjects := 0
	for _, c := range columns {
		r := reconcileColumn(spines[c].Graph.Links, drills[c].FundFlows, residual)
		subjects += r.subjects
		for _, f := range r.findings {
			findings = append(findings, finding(fmt.Sprintf("%s, %s", c, f.Subject), "%s", f.Detail))
		}
		outflow := "no decomposed group's outflow"
		if n := len(r.decomposed); n > 0 {
			outflow = fmt.Sprintf("%d decomposed %s outflow (%s)", n,
				plural(n, "group's", "groups'"), joinComma(r.decomposed))
		}
		summaries = append(summaries, fmt.Sprintf("%s: %d fund groups' inflow and %s reconcile "+
			"to the cent; the residual the drill-down cannot break down is %s in and %s out",
			c, r.groups, outflow, amount.Cents(r.residualIn), amount.Cents(r.residualOut)))
	}

	return conclusion{
		subjects: subjects,
		unit:     "fund group sides",
		held:     joinSemicolon(summaries),
		nothing: "no fiscal column is published by both the spine and the drill-down, so " +
			"there is no fund group whose two grains could be compared",
		findings: findings,
	}.result(), nil
}

// oneColumn indexes a projection by its single column, refusing a projection of
// any other width or a second projection of the same column: both are states
// this check cannot make sense of rather than claims about the corpus.
func oneColumn(index map[project.Column]projection, p projection) error {
	if len(p.Options.Columns) != 1 {
		return fmt.Errorf("drill-reconciles-across-documents: %s is of %d columns, and a "+
			"graph is of one", p, len(p.Options.Columns))
	}
	c := p.Options.Columns[0]
	if prev, dup := index[c]; dup {
		return fmt.Errorf("drill-reconciles-across-documents: %s and %s are both of %s", prev, p, c)
	}
	index[c] = p
	return nil
}

// columnReconciliation is what one column's pair of documents said.
type columnReconciliation struct {
	subjects   int
	groups     int
	decomposed []string
	residualIn int64
	// residualOut is over the decomposed groups only, because an undecomposed
	// group's outflow was not examined and a figure for it would say it was.
	residualOut int64
	findings    []Finding
}

// reconcileColumn runs both identities over every fund group either document
// carries.
//
// THE GROUP SET IS THE UNION. A group only the drill-down carries has
// fund-level inflow and no spine inflow, and that is a drift rather than a
// group to skip; a group only the spine carries is the same drift the other way.
func reconcileColumn(spine []project.Link, drill *project.FundFlowsDocument,
	residual map[string]string) columnReconciliation {
	groupOfFund := map[string]string{}
	groups := map[string]bool{}
	for _, n := range drill.Nodes {
		switch {
		case strings.HasPrefix(n.ID, "fund/"):
			groupOfFund[n.ID] = n.Parent
		case strings.HasPrefix(n.ID, "fund-group/"):
			groups[n.ID] = true
		}
	}
	for _, l := range spine {
		for _, end := range []string{l.Source, l.Target} {
			if strings.HasPrefix(end, "fund-group/") {
				groups[end] = true
			}
		}
	}

	var r columnReconciliation
	for _, g := range sortedStrings(groups) {
		r.groups++

		// INFLOW: spine links into the group against drill-down links into
		// its funds, each summed whole and by source.
		spineIn := flowSum{by: map[string]int64{}}
		fundIn := flowSum{by: map[string]int64{}}
		for _, l := range spine {
			if l.Target == g {
				spineIn.add(l.Source, l.ValueCents)
			}
		}
		for _, l := range drill.Links {
			if groupOfFund[l.Target] == g {
				fundIn.add(l.Source, l.ValueCents)
			}
		}
		r.subjects++
		r.findings = append(r.findings, reconcileSide(g+" inflow", "into", spineIn, fundIn,
			residual, &r.residualIn)...)

		// OUTFLOW, stated only where the drill-down decomposes the group.
		spineOut := flowSum{by: map[string]int64{}}
		fundOut := flowSum{by: map[string]int64{}}
		for _, l := range spine {
			if l.Source == g {
				spineOut.add(l.Target, l.ValueCents)
			}
		}
		for _, l := range drill.Links {
			if groupOfFund[l.Source] == g {
				fundOut.add(l.Target, l.ValueCents)
			}
		}
		if fundOut.links == 0 {
			continue
		}
		r.decomposed = append(r.decomposed, g)
		r.subjects++
		r.findings = append(r.findings, reconcileSide(g+" outflow", "out of", spineOut, fundOut,
			residual, &r.residualOut)...)
	}
	return r
}

// flowSum is one side of one group in one document: the total, the total by
// the far endpoint, and how many links carried it.
type flowSum struct {
	total int64
	by    map[string]int64
	links int
}

func (f *flowSum) add(end string, cents int64) {
	f.total += cents
	f.by[end] += cents
	f.links++
}

// reconcileSide applies the identity to one side of one group and returns its
// findings, adding the residual it named to *named.
//
// ONE FINDING PER SUBJECT, MOST SPECIFIC FIRST. A declared endpoint the
// drill-down carries in part is reported as the split it is, and the identity
// is not then restated over the same money: two findings for one defect read
// as two defects.
//
// THE SPLIT IS NOT NECESSARILY THE WHOLE DRIFT, and saying it was printed a
// wrong figure. The early return here named sp-fd as "unaccounted", which is
// this endpoint's share; the SIDE's unaccounted amount is the identity's
// residue and the two are equal only when the split is the side's sole drift.
// Measured on a fixture with a dropped transfer and a re-parented fund, the
// finding read "$20.00 is unaccounted" where the side's identity left $220.00
// -- so a reader who fixed the $20 met a second finding on the next run. Both
// figures are now named and distinguished. Found by pass two of /code-review.
func reconcileSide(subject, preposition string, spine, fund flowSum,
	residual map[string]string, named *int64) []Finding {
	var sideResidual, splitDiff int64
	var split string
	for _, id := range sortedStrings(residual) {
		sp, fd := spine.by[id], fund.by[id]
		switch {
		case fd == 0:
			sideResidual += sp
		case fd != sp:
			// A split endpoint is neither carried nor decomposed, so it joins
			// neither total; the loop runs on so the other endpoints' residual
			// is still accumulated and the side's identity can be stated.
			if split == "" {
				splitDiff = sp - fd
				split = fmt.Sprintf(
					"%s %s the group is %s on the spine and %s at fund level. A declared "+
						"residual endpoint is carried whole or decomposed whole, never split, "+
						"so one document holds a row of it the other lacks",
					id, preposition, amount.Cents(sp), amount.Cents(fd))
			}
		}
	}
	*named += sideResidual

	unaccounted := spine.total - fund.total - sideResidual
	if split != "" {
		// THE TWO FIGURES ARE THE SAME ONLY WHEN THE SPLIT IS THE SOLE DRIFT,
		// which is the common case and gets the sentence that says so. When
		// they differ, naming only the split would send a reader to fix an
		// amount that does not close the identity.
		if unaccounted == splitDiff {
			return []Finding{finding(subject, "%s, and %s is unaccounted",
				split, amount.Cents(splitDiff))}
		}
		return []Finding{finding(subject,
			"%s. That endpoint differs by %s and this side is %s unaccounted in total, so "+
				"the split is one of several drifts here",
			split, amount.Cents(splitDiff), amount.Cents(unaccounted))}
	}
	if unaccounted == 0 {
		return nil
	}
	return []Finding{finding(subject,
		"spine flow %s the group is %s, fund-level flow is %s, named residual is %s, "+
			"unaccounted %s. The residual is the declared endpoints' share and is supposed "+
			"to exist; what is left is money one document carries and the other does not: "+
			"a selector dropped from one schedule, a fund whose type moved in "+
			"data/funds.yaml, or an endpoint flow nothing declares",
		preposition, amount.Cents(spine.total), amount.Cents(fund.total),
		amount.Cents(sideResidual), amount.Cents(unaccounted))}
}
