package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// uncitedFactsArePrintedZeros asserts that every fact a schedule document was
// built over and cites on no link is a figure the city printed as zero.
//
// facts = facts_cited + facts_uncited holds by construction whatever an uncited
// fact is worth, so the counts checks cannot see a dropped non-zero fact; this
// reads it from outside internal/project. Per fact, not per cell: two facts
// cancelling to zero pass the producers and fail here (fisc-gszi). The spine
// is not read: its fund balance rows are stocks behind no link.
type uncitedFactsArePrintedZeros struct{}

var _ Check = (*uncitedFactsArePrintedZeros)(nil)

func (*uncitedFactsArePrintedZeros) ID() string { return "uncited-facts-are-printed-zeros" }
func (*uncitedFactsArePrintedZeros) Tier() int  { return 1 }
func (*uncitedFactsArePrintedZeros) Full() bool { return false }
func (*uncitedFactsArePrintedZeros) Description() string {
	return "every fact a schedule document was built over and cites on no link is a printed zero"
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
