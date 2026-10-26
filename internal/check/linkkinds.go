package check

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// linkKindsMatchTheirFacts asserts a link's kind does not contradict the facts
// it cites. A one-way constraint, not a second classifier:
//
//   - a link whose facts are all transfers, or all fund-balance rows, may not
//     be `external`: nothing crosses the city's boundary.
//   - a link may not carry a kind outside the four schema/enums.schema.json's link_kind declares.
//   - a link's facts must agree about which family they are in; mixing a
//     transfer with a revenue is a cell key that lost an axis.
//
// The converse, an external link that should have been a transfer, is not
// asserted: a revenue fact carries no marker that would show it.
type linkKindsMatchTheirFacts struct{}

func (*linkKindsMatchTheirFacts) meta() meta {
	return meta{id: "link-kinds-match-their-facts", tier: 1,
		description: "every link carries one of the four declared kinds, and a link whose facts are all " +
			"transfers or all fund-balance rows is not published as money crossing the city's " +
			"boundary"}
}

func (*linkKindsMatchTheirFacts) Run(_ context.Context, s *Subject) (Result, error) {
	kinds := project.LinkKinds()
	var findings []Finding
	links := 0

	for _, p := range s.linkedDocuments() {
		slice, err := p.slice(s.Facts)
		if err != nil {
			return Result{}, err
		}
		byID := fact.ByID(slice)
		for _, l := range p.Links {
			links++
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)

			if !slices.Contains(kinds, l.Kind) {
				findings = append(findings, finding(subject,
					"kind is %q, which is not one of %s", l.Kind, describeLinkKinds()))
			}

			kinds := map[vocab.Kind]bool{}
			for _, id := range l.FactIDs {
				if f, ok := byID[id]; ok {
					kinds[f.Kind] = true
				}
			}
			if len(kinds) == 0 {
				continue
			}
			switch {
			case allOf(kinds, vocab.KindTransferIn, vocab.KindTransferOut):
				if l.Kind == project.KindExternal {
					findings = append(findings, finding(subject,
						"every fact this link cites is a transfer and it is published as %q. "+
							"A transfer moves money between two city funds and crosses no "+
							"boundary, so a reader summing the external links to get "+
							"external revenue counts it twice over", project.KindExternal))
				}
			case allOf(kinds, vocab.KindFundBalance):
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
func allOf(got map[vocab.Kind]bool, want ...vocab.Kind) bool {
	if len(got) == 0 {
		return false
	}
	allowed := map[vocab.Kind]bool{}
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

func describeFactKinds(got map[vocab.Kind]bool) string {
	out := make([]string, 0, len(got))
	for k := range got {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
