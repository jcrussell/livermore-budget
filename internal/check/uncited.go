package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// isStock is whether a fact is one of the two balance rows pp.66-67 print,
// spelled here rather than imported: it is the check's own reading of what a
// stock is.
func isStock(f *fact.Fact) bool {
	return f.Kind == mapping.KindFundBalance &&
		(f.Category == project.CategoryFundBalanceBeginning || f.Category == project.CategoryFundBalanceEnding)
}

// uncitedFactsArePrintedZeros asserts that every fact a schedule document was
// built over and cites on no link is a figure the city printed as zero.
//
// facts = facts_cited + facts_uncited holds by construction whatever an uncited
// fact is worth, so counts-reconcile cannot see a dropped non-zero fact; this
// reads it from outside internal/project. Per fact, not per cell: two facts
// cancelling to zero pass the producers and fail here (fisc-gszi). The one
// non-zero fact allowed uncited is a stock row: beginning and ending working
// capital are balances the spine records and draws as no flow.
type uncitedFactsArePrintedZeros struct{}

var _ Check = (*uncitedFactsArePrintedZeros)(nil)

func (*uncitedFactsArePrintedZeros) ID() string { return "uncited-facts-are-printed-zeros" }
func (*uncitedFactsArePrintedZeros) Tier() int  { return 1 }
func (*uncitedFactsArePrintedZeros) Full() bool { return false }
func (*uncitedFactsArePrintedZeros) Description() string {
	return "every fact a graph document was built over and cites on no link is a printed zero " +
		"or a stock row"
}

func (*uncitedFactsArePrintedZeros) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	uncited, docs := 0, 0
	for _, p := range s.linkedDocuments() {
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
			if f.AmountCents != 0 && !isStock(&f) {
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
		held: fmt.Sprintf("%d uncited facts over %d graph document(s), every one a printed "+
			"zero or a stock row", uncited, docs),
		nothing:  "no projection built a graph, so no uncited fact has been read",
		findings: findings,
	}.result(), nil
}
