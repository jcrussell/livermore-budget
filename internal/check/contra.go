package check

import (
	"context"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// contraLinksNameTheirSchedule asserts that a link's sign and the sentence
// beside it agree, in both directions, and that the sentence names this link's
// own source's parent.
//
// THE CHART DRAWS A REDUCTION FORWARD, AT ITS MAGNITUDE. Nothing in a ribbon's
// geometry can carry a minus sign, so what makes a reduction legible as one is
// the sentence -- and a negative figure that arrived without it would be drawn
// as an addition of the same size, with no surface saying otherwise. That is
// why the empty half is a finding and not a default.
//
// THE CONVERSE HALF IS THE ONE THAT CATCHES A FIX GONE WRONG: a positive link
// carrying the sentence asserts a reduction the schedule does not print, and it
// reads exactly like a correct one on the page.
//
// WHAT THIS CANNOT WITNESS, said plainly because the check would otherwise be
// read as stronger than it is. It does not assert that Property Taxes is what
// Budget Book p127 prints ERAF under: no fact carries a marker that would
// distinguish a reduction's category from its line's, so the honest claim is
// the one the document's own hierarchy can make. What it does assert is that
// the sentence was composed from THIS document's nodes -- the defect it is
// really guarding is a sentence composed downstream from a folded chart, where
// a line's parent has been blanked and the words would name a category the
// reader is not looking at, or nothing at all.
type contraLinksNameTheirSchedule struct{}

var _ Check = (*contraLinksNameTheirSchedule)(nil)

func (*contraLinksNameTheirSchedule) ID() string { return "contra-links-name-their-schedule" }
func (*contraLinksNameTheirSchedule) Tier() int  { return 1 }
func (*contraLinksNameTheirSchedule) Full() bool { return false }
func (*contraLinksNameTheirSchedule) Description() string {
	return "a link the document prints negative names the schedule it is printed as a reduction " +
		"of, in the words the page shows beside it; no other link names one"
}

// contraPrefix and contraOrphan are the two sentences a reduction can carry.
// They are spelled here rather than read from the projection for the reason
// this package spells every vocabulary it checks: a check reading its answer
// from the thing under test asserts nothing.
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
