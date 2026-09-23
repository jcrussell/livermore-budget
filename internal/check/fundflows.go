package check

import (
	"context"
	"fmt"
)

// fundFlowsCountsReconcile re-derives every count the drill-down publishes.
//
// IT IS counts-reconcile FOR A DIFFERENT DOCUMENT SHAPE, and a separate check
// rather than an arm of that one because the identity is not the same statement.
// The spine's is facts = facts_cited + stocks + zero-valued cells, and it assumes
// each fact is behind at most one link. Neither half survives here: neither
// schedule prints a stock row, and both sides carry a summing link above the
// cell -- the fund-to-department link over a division's object rows, and a
// line's rollup over the funds one printed row reaches -- so most facts here are
// behind two links.
//
// THE DERIVATION IS INDEPENDENT, which is the whole reason a count is worth
// checking. counts-reconcile earns that by carrying a second copy of
// internal/project's cell netting (this package's own cellKey and netCells) and
// re-netting the facts; this earns it a different and stronger way, by deriving
// every figure from the PUBLISHED LINKS rather than from any netting at all. A
// citation the projection got wrong changes the answer here; a check that
// re-netted the same facts through the same rule would agree with the projection
// whatever the document said. fisc-ix2 asked whether the duplicate netting should
// exist at all -- for this shape the answer is that it should not, because there
// is a better witness available.
//
// WHAT EACH CLAIM CATCHES:
//
//   - facts against the slice: a document that quietly narrowed what it drew.
//   - facts_cited against the distinct union of every link's fact_ids: a
//     citation the document dropped.
//   - the identity facts = cited + uncited: that the two numbers the document
//     publishes add up to the third. It does NOT witness what an uncited fact
//     is worth -- uncited is defined as reached-no-link, so the identity holds
//     whatever those facts carry -- and uncited-facts-are-printed-zeros is the
//     check that does.
//   - facts_cited_twice: the overlap between the two grains, published as a
//     number because `links` is NOT a partition of `facts_cited` in this
//     document and the spine's shape would lead a reader to assume it is.
//   - nodes and links against the arrays: a ratchet rather than a witness, and
//     labelled as one.
type fundFlowsCountsReconcile struct{}

var _ Check = (*fundFlowsCountsReconcile)(nil)

func (*fundFlowsCountsReconcile) ID() string { return "fund-flows-counts-reconcile" }
func (*fundFlowsCountsReconcile) Tier() int  { return 1 }
func (*fundFlowsCountsReconcile) Full() bool { return false }
func (*fundFlowsCountsReconcile) Description() string {
	return "the drill-down's counts.facts equals facts_cited plus facts_uncited, every one of " +
		"them re-derived from the document's own links and the facts it was built from"
}

func (*fundFlowsCountsReconcile) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	var summaries []string

	for _, p := range s.fundFlowsDocuments() {
		doc := p.FundFlows
		c := doc.Metadata.Counts

		slice := factsFor(s.Facts, p.Options)
		// Cited and twice-cited, derived from the links a reader can see.
		times := map[string]int{}
		for _, l := range doc.Links {
			for _, id := range l.FactIDs {
				times[id]++
			}
		}
		cited, twice := 0, 0
		for _, n := range times {
			cited++
			if n > 1 {
				twice++
			}
		}
		uncited := 0
		for i := range slice {
			if times[slice[i].ID] == 0 {
				uncited++
			}
		}

		for _, cmp := range []struct {
			what      string
			got, want int
			why       string
		}{
			{"counts.facts", c.Facts, len(slice),
				"the document says it drew a different slice than the one it was built over"},
			{"counts.facts_cited", c.FactsCited, cited,
				"a citation the document dropped, or one it counted twice: this is the " +
					"number of DISTINCT facts some link names"},
			{"counts.facts_uncited", c.FactsUncited, uncited,
				"a fact that reached no link, and the document disagrees about how many"},
			{"counts.facts_cited_twice", c.FactsCitedTwice, twice,
				"the overlap between the grains -- a revenue row behind both its flow into " +
					"a fund and its line's rollup into the category, or an expenditure row " +
					"behind both a department's object rows and the fund-to-department link " +
					"that totals them"},
			{"counts.nodes", c.Nodes, len(doc.Nodes), "a ratchet on the arrays as published"},
			{"counts.links", c.Links, len(doc.Links), "a ratchet on the arrays as published"},
		} {
			if cmp.got != cmp.want {
				findings = append(findings, finding(p.String(),
					"%s is %d and re-derives to %d: %s", cmp.what, cmp.got, cmp.want, cmp.why))
			}
		}

		// THE IDENTITY, asserted against the document's OWN two numbers rather
		// than against the re-derived ones. The comparisons above already say
		// whether each is right; this says whether they are consistent with
		// each other, which is what the document publishes as a claim.
		if c.FactsCited+c.FactsUncited != c.Facts {
			findings = append(findings, finding(p.String(),
				"counts.facts is %d and facts_cited + facts_uncited is %d + %d = %d. This "+
					"document's published identity is that every fact is either behind a "+
					"link or uncited, and the two numbers it publishes do not add up to the third",
				c.Facts, c.FactsCited, c.FactsUncited, c.FactsCited+c.FactsUncited))
		}

		summaries = append(summaries, fmt.Sprintf("%s (%d = %d cited + %d uncited, %d cited twice)",
			p.String(), c.Facts, c.FactsCited, c.FactsUncited, c.FactsCitedTwice))
	}

	return conclusion{
		subjects: len(summaries),
		unit:     "drill-down documents",
		held:     joinSemicolon(summaries),
		nothing:  "no projection built a drill-down, so no counts of that shape have been read",
		findings: findings,
	}.result(), nil
}

// joinSemicolon renders one line per document, the way counts-reconcile does.
func joinSemicolon(out []string) string {
	s := ""
	for i, o := range out {
		if i > 0 {
			s += "; "
		}
		s += o
	}
	return s
}
