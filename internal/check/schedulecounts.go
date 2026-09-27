package check

import (
	"context"
	"fmt"
)

// publishedCounts is the counts a schedule document publishes about itself.
type publishedCounts struct {
	facts, cited, uncited, nodes, links int
	// transfers is p76's printed movements, links/2, and hasTransfers says
	// whether the shape publishes it at all.
	transfers    int
	hasTransfers bool
}

// countedDocuments is every built department-spending, department-funding and
// transfers-by-fund document with the counts it publishes. fund-flows has its
// own counts check: its identity carries an overlap term.
func (s *Subject) countedDocuments() []struct {
	linked
	counts publishedCounts
} {
	type counted = struct {
		linked
		counts publishedCounts
	}
	var out []counted
	for _, p := range s.Projections {
		switch {
		case p.DepartmentSpending != nil:
			c := p.DepartmentSpending.Metadata.Counts
			out = append(out, counted{
				linked{projection: p, Nodes: p.DepartmentSpending.Nodes, Links: p.DepartmentSpending.Links},
				publishedCounts{facts: c.Facts, cited: c.FactsCited, uncited: c.FactsUncited,
					nodes: c.Nodes, links: c.Links},
			})
		case p.DepartmentFunding != nil:
			c := p.DepartmentFunding.Metadata.Counts
			out = append(out, counted{
				linked{projection: p, Nodes: p.DepartmentFunding.Nodes, Links: p.DepartmentFunding.Links},
				publishedCounts{facts: c.Facts, cited: c.FactsCited, uncited: c.FactsUncited,
					nodes: c.Nodes, links: c.Links},
			})
		case p.TransfersByFund != nil:
			c := p.TransfersByFund.Metadata.Counts
			out = append(out, counted{
				linked{projection: p, Nodes: p.TransfersByFund.Nodes, Links: p.TransfersByFund.Links},
				publishedCounts{facts: c.Facts, cited: c.FactsCited, uncited: c.FactsUncited,
					nodes: c.Nodes, links: c.Links, transfers: c.Transfers, hasTransfers: true},
			})
		}
	}
	return out
}

// scheduleCountsReconcile re-derives every count department-spending,
// department-funding and transfers-by-fund publish from the published links and
// the facts the options select, never from a second run of the producer, so a
// wrong citation changes the answer. That an uncited fact is worth nothing is
// uncited-facts-are-printed-zeros'.
type scheduleCountsReconcile struct{}

var _ Check = (*scheduleCountsReconcile)(nil)

func (*scheduleCountsReconcile) ID() string { return "schedule-counts-reconcile" }
func (*scheduleCountsReconcile) Tier() int  { return 1 }
func (*scheduleCountsReconcile) Full() bool { return false }
func (*scheduleCountsReconcile) Description() string {
	return "department-spending, department-funding and transfers-by-fund's counts -- facts, " +
		"facts_cited, facts_uncited, nodes, links and p76's transfers -- each re-derived from " +
		"the document's own links and the facts it was built from"
}

func (*scheduleCountsReconcile) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	documents := 0

	for _, d := range s.countedDocuments() {
		c := d.counts
		slice := factsFor(s.Facts, d.Options)
		cited := map[string]bool{}
		for _, l := range d.Links {
			for _, id := range l.FactIDs {
				cited[id] = true
			}
		}
		uncited := 0
		for i := range slice {
			if !cited[slice[i].ID] {
				uncited++
			}
		}

		cmps := []struct {
			what      string
			got, want int
			why       string
		}{
			{"counts.facts", c.facts, len(slice),
				"the document says it drew a different slice than the one it was built over"},
			{"counts.facts_cited", c.cited, len(cited),
				"a citation dropped or counted twice; this is the distinct facts some link names"},
			{"counts.facts_uncited", c.uncited, uncited,
				"a fact that reached no link, and the document disagrees about how many"},
			{"counts.nodes", c.nodes, len(d.Nodes), "a ratchet on the arrays as published"},
			{"counts.links", c.links, len(d.Links), "a ratchet on the arrays as published"},
		}
		if c.hasTransfers {
			cmps = append(cmps, struct {
				what      string
				got, want int
				why       string
			}{"counts.transfers", c.transfers, len(d.Links) / 2,
				"p76 draws each printed movement as two legs, so the movements are links/2"})
		}
		for _, cmp := range cmps {
			if cmp.got != cmp.want {
				findings = append(findings, finding(d.String(),
					"%s is %d and re-derives to %d: %s", cmp.what, cmp.got, cmp.want, cmp.why))
			}
		}
		if c.cited+c.uncited != c.facts {
			findings = append(findings, finding(d.String(),
				"counts.facts is %d and facts_cited + facts_uncited is %d + %d = %d",
				c.facts, c.cited, c.uncited, c.cited+c.uncited))
		}
		documents++
	}

	return conclusion{
		subjects: documents,
		unit:     "schedule documents",
		held: fmt.Sprintf("%d schedule document(s), every published count re-derived from its "+
			"links and facts", documents),
		nothing:  "no projection built a department-spending, department-funding or transfers-by-fund document, so no counts of those shapes have been read",
		findings: findings,
	}.result(), nil
}
