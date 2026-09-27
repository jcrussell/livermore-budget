package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// contraLinksNameTheirSchedule asserts that a link is negative exactly when it
// carries a reduction sentence, and that the sentence names this document's own
// parent of the link's source. The chart draws a reduction forward at its
// magnitude, so without the sentence it reads as an addition; a positive link
// with one asserts a subtraction no page makes. It cannot witness that the
// named category is the one the page prints the row under -- no fact carries
// that -- only that the sentence was composed from this document's tree and
// not a folded chart's.
type contraLinksNameTheirSchedule struct{}

var _ Check = (*contraLinksNameTheirSchedule)(nil)

func (*contraLinksNameTheirSchedule) ID() string { return "contra-links-name-their-schedule" }
func (*contraLinksNameTheirSchedule) Tier() int  { return 1 }
func (*contraLinksNameTheirSchedule) Full() bool { return false }
func (*contraLinksNameTheirSchedule) Description() string {
	return "a link the document prints negative names the schedule it is printed as a reduction " +
		"of, in the words the page shows beside it; no other link names one"
}

// contraPrefix and contraOrphan are the two sentences a reduction can carry,
// spelled here rather than read from the projection under test.
const (
	contraPrefix = "printed as a reduction of "
	contraOrphan = "printed rows netting to a reduction"
)

func (*contraLinksNameTheirSchedule) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	negatives := 0

	for _, p := range s.linkedDocuments() {
		labels := map[string]project.Node{}
		for _, n := range p.Nodes {
			labels[n.ID] = n
		}
		for _, l := range p.Links {
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)

			if l.ValueCents >= 0 {
				if l.Contra != "" {
					findings = append(findings, finding(subject,
						"is published at %d cents and carries %q. Only a figure the schedule "+
							"prints negative is a reduction, and a positive one saying so "+
							"asserts a subtraction no page makes",
						l.ValueCents, l.Contra))
				}
				continue
			}

			negatives++
			if l.Contra == "" {
				findings = append(findings, finding(subject,
					"is published at %d cents and names no schedule. The chart draws a "+
						"reduction forward at its magnitude, so with no sentence beside it "+
						"the ribbon is indistinguishable from an addition of the same size",
					l.ValueCents))
				continue
			}

			up, ok := labels[labels[l.Source].Parent]
			want := contraOrphan
			if ok {
				want = contraPrefix + up.Label
			}
			if l.Contra != want {
				findings = append(findings, finding(subject,
					"carries %q, where this document's own hierarchy puts the source under "+
						"%q. The sentence a reader meets names the category the schedule "+
						"prints the row under, so one composed from anything else names a "+
						"category they are not looking at",
					l.Contra, want))
			}
		}
	}

	return conclusion{
		subjects: negatives,
		unit:     "negative links",
		held: fmt.Sprintf("%d link(s) across %d document(s) are published negative, each naming "+
			"the schedule its own document puts it under, and no other link names one",
			negatives, len(s.linkedDocuments())),
		nothing:  "no projection publishes a negative link, so no reduction has been named",
		findings: findings,
	}.result(), nil
}
