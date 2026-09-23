package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// uncitedFactsArePrintedZeros asserts that every fact a schedule document was
// built over and cites on no link is a figure the city printed as zero.
//
// THE COUNTS IDENTITY CANNOT SEE THIS, AND THIS IS THE ARM THAT DOES. Each of
// the four schedule documents -- fund-flows, department-spending,
// department-funding and transfers-by-fund -- publishes facts = facts_cited +
// facts_uncited, and its counts check re-derives those numbers from the links.
// But uncited is DEFINED as reached-no-link, so the identity holds by
// construction whatever an uncited fact is worth. What refuses a dropped
// non-zero fact at build time is each producer's own loop, reached through
// projections-build: the package that dropped the fact deciding that it did
// not. This is the second reading, from outside internal/project: the facts
// the document's options select, less the ids its links cite, each held to
// amount_cents 0.
//
// PER FACT AND NOT PER CELL, which is stricter than the producers. They
// classify by the cell a fact nets into, so two facts cancelling to zero pass
// there and are found here; fisc-gszi is that difference on the producers'
// side.
//
// THE SPINE IS NOT READ HERE. Its identity carries a stock term -- a fund
// balance row is a fact and is not a flow -- and counts-reconcile is the check
// of that shape.
type uncitedFactsArePrintedZeros struct{}

var _ Check = (*uncitedFactsArePrintedZeros)(nil)

func (*uncitedFactsArePrintedZeros) ID() string { return "uncited-facts-are-printed-zeros" }
func (*uncitedFactsArePrintedZeros) Tier() int  { return 1 }
func (*uncitedFactsArePrintedZeros) Full() bool { return false }
func (*uncitedFactsArePrintedZeros) Description() string {
	return "every fact a schedule document was built over and cites on no link is a printed " +
		"zero, read off the facts and the links rather than off the producer that dropped it"
}

func (*uncitedFactsArePrintedZeros) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	uncited, docs := 0, 0
	for _, p := range s.linkedDocuments() {
		if p.Graph != nil {
			continue
		}
		docs++
		cited := map[string]bool{}
		for _, l := range p.Links {
			for _, id := range l.FactIDs {
				cited[id] = true
			}
		}
		for _, f := range factsFor(s.Facts, p.Options) {
			if cited[f.ID] {
				continue
			}
			uncited++
			if f.AmountCents != 0 {
				findings = append(findings, finding(p.String(),
					"fact %s (%s %q, %s) is carried by no link and is %s, not a printed zero. "+
						"The document's identity is that every uncited fact is a cell the city "+
						"printed as nothing; this one is money the document dropped in silence, "+
						"which its own counts cannot show because uncited is counted and not valued",
					f.ID, f.Kind, f.RowLabel, f.Category, amount.Cents(f.AmountCents)))
			}
		}
	}
	return conclusion{
		subjects: uncited,
		unit:     "uncited facts",
		held: fmt.Sprintf("%d uncited facts over %d schedule document(s), every one a printed "+
			"zero", uncited, docs),
		nothing:  "no projection built a schedule document, so no uncited fact has been read",
		findings: findings,
	}.result(), nil
}
