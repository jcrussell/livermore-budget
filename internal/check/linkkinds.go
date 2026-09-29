package check

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// linkKindsMatchTheirFacts asserts a link's kind does not contradict the facts
// it cites.
//
// NOTHING READ Link.Kind UNTIL THIS EXISTED, and the gap published a false claim
// in a shipped document. The drill-down classified every tier-0 flow with the
// fund-group rule, which is the right question for revenue and the wrong one for
// a transfer, so 8 links carrying $21,045,597 of transfers went out as
// "external" -- money crossing the city's boundary -- while sankey.json
// published the same money as internal_transfer. Every check was green: the
// values tied to their facts, the counts reconciled, the graph was acyclic. A
// client summing external to get external revenue over-counted by the transfers.
//
// IT IS A ONE-WAY CONSTRAINT AND NOT A SECOND CLASSIFIER, which is the whole
// reason it is cheap enough to be worth having. Re-deriving each link's kind
// would mean a second copy of every projection's rules -- the spine decomposes
// fund balance by sign and singles out Internal Service Funds; a drill-down does
// neither -- and two copies of a rule agree by construction rather than by
// evidence. What this asserts instead is the thing no projection may do whatever
// its rules:
//
//   - a link whose facts are ALL transfers may not be `external`. A transfer is
//     money moving between two city funds and crosses no boundary, whatever
//     group receives it.
//   - a link whose facts are ALL fund-balance rows may not be `external` either,
//     for the same reason: nothing moves between funds and nothing crosses the
//     boundary.
//   - a link may not carry a kind outside the four the contract declares.
//   - a link's facts must agree with each other about which of those two
//     families they are in, because a link mixing a transfer with a revenue is a
//     cell key that lost an axis.
//
// The converse -- an external link that should have been a transfer -- is NOT
// asserted, and saying so is the point: a revenue fact carries no marker that
// would distinguish it from one, so the honest claim is the one that can be
// made from the facts alone.
type linkKindsMatchTheirFacts struct{}

var _ Check = (*linkKindsMatchTheirFacts)(nil)

func (*linkKindsMatchTheirFacts) ID() string { return "link-kinds-match-their-facts" }
func (*linkKindsMatchTheirFacts) Tier() int  { return 1 }
func (*linkKindsMatchTheirFacts) Full() bool { return false }
func (*linkKindsMatchTheirFacts) Description() string {
	return "every link carries one of the four declared kinds, and a link whose facts are all " +
		"transfers or all fund-balance rows is not published as money crossing the city's " +
		"boundary"
}

func (*linkKindsMatchTheirFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	links := 0

	for _, p := range s.linkedDocuments() {
		byID := factIndex(project.SelectFacts(s.Facts, p.Options))
		for _, l := range p.Links {
			links++
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)

			if !slices.Contains(project.LinkKinds(), l.Kind) {
				findings = append(findings, finding(subject,
					"kind is %q, which is not one of %s", l.Kind, describeLinkKinds()))
			}

			kinds := map[mapping.Kind]bool{}
			for _, id := range l.FactIDs {
				if f, ok := byID[id]; ok {
					kinds[f.Kind] = true
				}
			}
			if len(kinds) == 0 {
				continue
			}
			switch {
			case allOf(kinds, mapping.KindTransferIn, mapping.KindTransferOut):
				if l.Kind == project.KindExternal {
					findings = append(findings, finding(subject,
						"every fact this link cites is a transfer and it is published as %q. "+
							"A transfer moves money between two city funds and crosses no "+
							"boundary, so a reader summing the external links to get "+
							"external revenue counts it twice over", project.KindExternal))
				}
			case allOf(kinds, mapping.KindFundBalance):
				if l.Kind == project.KindExternal {
					findings = append(findings, finding(subject,
						"every fact this link cites is a fund-balance row and it is "+
							"published as %q. Nothing moves between funds and nothing "+
							"crosses the boundary", project.KindExternal))
				}
			case len(kinds) > 1:
				findings = append(findings, finding(subject,
					"cites facts of %d different kinds (%s). One link is one cell, and a "+
						"cell holding two kinds is a key that lost an axis",
					len(kinds), describeFactKinds(kinds)))
			}
		}
	}

	return conclusion{
		subjects: links,
		unit:     "links",
		held: fmt.Sprintf("%d links across %d document(s), each carrying one of %s, and none "+
			"whose facts are all transfers or all fund-balance rows published as external",
			links, len(s.linkedDocuments()), describeLinkKinds()),
		nothing:  "no projection carries a link, so no kind has been read",
		findings: findings,
	}.result(), nil
}

// allOf reports whether the set is non-empty and drawn only from want.
func allOf(got map[mapping.Kind]bool, want ...mapping.Kind) bool {
	if len(got) == 0 {
		return false
	}
	allowed := map[mapping.Kind]bool{}
	for _, w := range want {
		allowed[w] = true
	}
	for k := range got {
		if !allowed[k] {
			return false
		}
	}
	return true
}

func describeLinkKinds() string {
	out := make([]string, 0, len(project.LinkKinds()))
	for _, k := range project.LinkKinds() {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func describeFactKinds(got map[mapping.Kind]bool) string {
	out := make([]string, 0, len(got))
	for k := range got {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
