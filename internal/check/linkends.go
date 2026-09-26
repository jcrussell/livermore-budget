package check

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// endName is what a link end's id names, read against the facts the link cites.
type endName int

const (
	// namesFund: every cited fact carries the fund the id numbers.
	namesFund endName = iota + 1
	// namesFundGroup: every cited fact carries the fund group the id names.
	namesFundGroup
	// namesDepartment: every cited fact's `department` is the id's slug, at
	// either tier -- which tier is node-tiers-are-declared's.
	namesDepartment
	// namesPartnerFund: the id numbers the fund at the OTHER leg of the same
	// transfer_id, because p76's facts carry their own end's fund only.
	namesPartnerFund
	// namesNeither: a category or line, which names no fund and no department.
	namesNeither
)

// endNames is what each hierarchy id form names. Its keys are hierarchyTiers'
// and a test holds them equal, so a new form is declared here or refused. The
// flow endpoints of endpointTiers name neither, by the same declaration.
var endNames = map[string]endName{
	"revenue":       namesNeither,
	"revenue-line":  namesNeither,
	"fund-group":    namesFundGroup,
	"transfer-from": namesPartnerFund,
	"fund":          namesFund,
	"dept":          namesDepartment,
	"department":    namesDepartment,
	"expenditure":   namesNeither,
	"transfer-to":   namesPartnerFund,
}

// linkEndsMatchTheirFacts asserts a link's ends name the fund, fund group and
// department its facts carry, and that a fund group's link into a fund goes to
// one of its own funds.
//
// link-values-tie-to-facts holds a link's VALUE to its fact_ids and nothing
// held its ENDS: swapping two departments' ids, or re-pointing a fund's
// ribbon to another fund, moved no cent and every other check stayed green.
type linkEndsMatchTheirFacts struct{}

var _ Check = (*linkEndsMatchTheirFacts)(nil)

func (*linkEndsMatchTheirFacts) ID() string { return "link-ends-match-their-facts" }
func (*linkEndsMatchTheirFacts) Tier() int  { return 1 }
func (*linkEndsMatchTheirFacts) Full() bool { return false }
func (*linkEndsMatchTheirFacts) Description() string {
	return "every link end naming a fund, fund group or department names the one its facts " +
		"carry, a transfer's payer and receiver ends name its other leg's fund, and a fund " +
		"group's link into a fund goes to a fund data/funds.yaml puts in that group"
}

func (*linkEndsMatchTheirFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	links := 0
	for _, p := range s.linkedDocuments() {
		selected := factIndex(factsFor(s.Facts, p.Options))
		legs := map[string][]project.Link{}
		for _, l := range p.Links {
			if l.TransferID != "" {
				legs[l.TransferID] = append(legs[l.TransferID], l)
			}
		}
		for _, l := range p.Links {
			links++
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)
			var facts []fact.Fact
			for _, id := range l.FactIDs {
				// An id that resolves to nothing is link-values-tie-to-facts'.
				if f, ok := selected[id]; ok {
					facts = append(facts, f)
				}
			}
			for _, end := range []struct {
				id     string
				source bool
			}{{l.Source, true}, {l.Target, false}} {
				if msg := endMismatch(end.id, end.source, l, facts, legs[l.TransferID]); msg != "" {
					findings = append(findings, finding(subject, "%s", msg))
				}
			}
			group, gok := strings.CutPrefix(l.Source, fundGroupPrefix)
			number, fok := strings.CutPrefix(l.Target, fundNodePrefix)
			if gok && fok {
				n, err := strconv.Atoi(number)
				fund, listed := s.Vocabulary.Fund(n)
				switch {
				case err != nil || !listed:
					findings = append(findings, finding(subject, "%s is no fund data/funds.yaml lists", l.Target))
				case fund.Type != group:
					findings = append(findings, finding(subject,
						"data/funds.yaml puts fund %d in %q, not %q", n, fund.Type, group))
				}
			}
		}
	}
	return conclusion{
		subjects: links,
		unit:     "links",
		held: fmt.Sprintf("%d links, each ending at the funds, fund groups and departments its facts carry",
			links),
		nothing:  "no projection carries a link",
		findings: findings,
	}.result(), nil
}

// endMismatch is why one end of l does not name what its facts carry, or "".
func endMismatch(id string, source bool, l project.Link, facts []fact.Fact, pair []project.Link) string {
	if _, ok := endpointTiers[id]; ok {
		return ""
	}
	form, value, ok := strings.Cut(id, "/")
	name, declared := endNames[form]
	if !ok || !declared {
		return fmt.Sprintf("%s is no declared id form, so what it names cannot be held to its facts", id)
	}
	for _, f := range facts {
		var carried string
		switch name {
		case namesFund:
			carried = fact.FundString(f.Fund)
		case namesFundGroup:
			carried = f.FundGroup
		case namesDepartment:
			carried = f.Department
		default:
			continue
		}
		if carried != value {
			return fmt.Sprintf("%s names %q and fact %s carries %q", id, value, f.ID, carried)
		}
	}
	if name != namesPartnerFund {
		return ""
	}
	// The payer's end of the receiving leg is the paying leg's source fund,
	// and the receiver's end of the paying leg is the receiving leg's target.
	for _, other := range pair {
		if other.Source == l.Source && other.Target == l.Target {
			continue
		}
		want := other.Source
		if !source {
			want = other.Target
		}
		if want != fundNodePrefix+value {
			return fmt.Sprintf("%s names fund %s and the other leg of transfer %s is %s -> %s",
				id, value, l.TransferID, other.Source, other.Target)
		}
	}
	return ""
}
