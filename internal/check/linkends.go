package check

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// endName is what a link end's id names, read against the facts the link cites.
type endName int

const (
	// namesFund: every cited fact carries the fund the id numbers.
	namesFund endName = iota + 1
	// namesFundGroup: every cited fact carries the fund group the id names.
	namesFundGroup
	// namesDivision and namesDepartment: every cited fact's `department` is
	// the id's slug, read from a schedule whose cut declares that tier.
	namesDivision
	namesDepartment
	// namesPartnerFund: the id numbers the fund at the OTHER leg of the same
	// transfer_id, because p76's facts carry their own end's fund only.
	namesPartnerFund
	// namesCategory: every cited fact's category is the id's slug -- in the
	// drill-down, its division and category, `<division>/<category>`, or for a
	// fact printing no division its fund, `fund/<n>/<category>`.
	namesCategory
	// namesLine: the id is a data/taxonomy.yaml line under each cited fact's
	// category, printed as that fact's row label.
	namesLine
)

// endNames is what each id form names, keyed by project's prefixes. A test
// holds its keys to project.IDForms, so a new form is declared here or refused.
var endNames = map[string]endName{
	project.PrefixRevenue:      namesCategory,
	project.PrefixRevenueLine:  namesLine,
	project.PrefixFundGroup:    namesFundGroup,
	project.PrefixTransferFrom: namesPartnerFund,
	project.PrefixFund:         namesFund,
	project.PrefixDept:         namesDivision,
	project.PrefixDepartment:   namesDepartment,
	project.PrefixExpenditure:  namesCategory,
	project.PrefixTransferTo:   namesPartnerFund,
}

// linkEndsMatchTheirFacts asserts a link's ends name the fund, fund group,
// department, category and line its facts carry, and that a fund group's link
// into a fund goes to one of its own funds.
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
	return "every link end names the fund, fund group, department or division, category and " +
		"line its facts carry, a transfer's payer and receiver ends name its other leg's fund, " +
		"and a fund group's link into a fund goes to a fund data/funds.yaml puts in that group"
}

func (*linkEndsMatchTheirFacts) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	tiers := departmentTiers(structure.AllCuts())
	links := 0
	for _, p := range s.linkedDocuments() {
		selected := factIndex(project.SelectFacts(s.Facts, p.Options))
		legs := map[string][]int{}
		for i, l := range p.Links {
			if l.TransferID != "" {
				legs[l.TransferID] = append(legs[l.TransferID], i)
			}
		}
		for i, l := range p.Links {
			subject := fmt.Sprintf("%s %s -> %s", p, l.Source, l.Target)
			var facts []fact.Fact
			for _, id := range l.FactIDs {
				// An id that resolves to nothing is link-values-tie-to-facts'.
				if f, ok := selected[id]; ok {
					facts = append(facts, f)
				}
			}
			e := linkEnd{link: l, facts: facts, vocab: s.Vocabulary, tiers: tiers,
				divisionExpenditure: p.Name == project.FundFlowsProjection}
			for _, j := range legs[l.TransferID] {
				if j != i {
					e.others = append(e.others, p.Links[j])
				}
			}
			checked := false
			for _, end := range []struct {
				id     string
				source bool
			}{{l.Source, true}, {l.Target, false}} {
				msg, ok := e.mismatch(end.id, end.source)
				checked = checked || ok
				if msg != "" {
					findings = append(findings, finding(subject, "%s", msg))
				}
			}
			if checked && len(facts) > 0 {
				links++
			}
			group, gok := strings.CutPrefix(l.Source, project.PrefixFundGroup)
			number, fok := strings.CutPrefix(l.Target, project.PrefixFund)
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
		held: fmt.Sprintf("%d links, each ending at what its facts carry",
			links),
		nothing:  "no projection carries a link whose ends name anything its facts carry",
		findings: findings,
	}.result(), nil
}

// departmentTiers is the department tier each scope's cut declares. A scope
// two cuts give different tiers maps to "", which matches neither form.
func departmentTiers(cuts []structure.Cut) map[string]string {
	out := map[string]string{}
	for _, c := range cuts {
		if c.DepartmentTier == "" {
			continue
		}
		if have, ok := out[c.Scope]; ok && have != c.DepartmentTier {
			out[c.Scope] = ""
			continue
		}
		out[c.Scope] = c.DepartmentTier
	}
	return out
}

// linkEnd is one link read against its facts and its transfer's other legs.
type linkEnd struct {
	link   project.Link
	facts  []fact.Fact
	others []project.Link
	vocab  Vocabulary
	tiers  map[string]string
	// divisionExpenditure: the drill-down's expenditure ids carry the division.
	divisionExpenditure bool
}

// mismatch is why the end id does not name what the link's facts carry, or
// "", and whether the end was held to anything.
func (e linkEnd) mismatch(id string, source bool) (string, bool) {
	if readings, ok := project.EndpointCategories(id); ok {
		if msg := outsideEveryReading(id, readings, e.facts); msg != "" {
			return msg, true
		}
		sum := project.ChangeCents(e.facts)
		// A draw and a contribution cite the same categories, told apart by
		// sign; a zero change is neither.
		if len(e.facts) > 0 && ((id == project.NodeFundBalanceDraw && sum >= 0) ||
			(id == project.NodeFundBalanceContribution && sum <= 0)) {
			return fmt.Sprintf("%s cites facts summing to %d cents; a draw is negative and a contribution positive", id, sum), true
		}
		return "", true
	}
	form, value, ok := strings.Cut(id, "/")
	name, declared := endNames[form+"/"]
	if !ok || !declared {
		return fmt.Sprintf("%s is no declared id form, so what it names cannot be held to its facts", id), false
	}
	if name == namesPartnerFund && e.link.TransferID == "" {
		return fmt.Sprintf("%s names a transfer's other leg and the link carries no transfer_id", id), true
	}
	for _, f := range e.facts {
		var carried string
		switch name {
		case namesFund:
			carried = fact.FundString(f.Fund)
		case namesFundGroup:
			carried = f.FundGroup
		case namesDivision, namesDepartment:
			carried = f.Department
			tier := map[endName]string{namesDivision: structure.TierDivision, namesDepartment: structure.TierDepartment}[name]
			if e.tiers[f.Scope] != tier {
				return fmt.Sprintf("%s names a %s and fact %s is from %s, which no cut declares at that tier",
					id, tier, f.ID, f.Scope), true
			}
		case namesCategory:
			carried = f.Category
			if e.divisionExpenditure && form+"/" == project.PrefixExpenditure {
				carried = strings.TrimPrefix(project.ExpenditureIDOf(&f), project.PrefixExpenditure)
			}
		case namesLine:
			if msg := e.lineMismatch(id, value, f); msg != "" {
				return msg, true
			}
			continue
		default:
			continue
		}
		if carried != value {
			return fmt.Sprintf("%s names %q and fact %s carries %q", id, value, f.ID, carried), true
		}
	}
	if name != namesPartnerFund {
		return "", true
	}
	// The payer's end of the receiving leg is the paying leg's source fund,
	// and the receiver's end of the paying leg is the receiving leg's target.
	for _, other := range e.others {
		want := other.Source
		if !source {
			want = other.Target
		}
		if want != project.PrefixFund+value {
			return fmt.Sprintf("%s names fund %s and the other leg of transfer %s is %s -> %s",
				id, value, e.link.TransferID, other.Source, other.Target), true
		}
	}
	return "", true
}

// outsideEveryReading is why the facts behind a flow endpoint are not all of
// one of its readings' categories, or "".
func outsideEveryReading(id string, readings [][]string, facts []fact.Fact) string {
	for _, cats := range readings {
		fits := true
		for _, f := range facts {
			fits = fits && slices.Contains(cats, f.Category)
		}
		if fits {
			return ""
		}
	}
	got := make([]string, 0, len(facts))
	for _, f := range facts {
		got = append(got, fmt.Sprintf("%s carries %q", f.ID, f.Category))
	}
	return fmt.Sprintf("%s is read from categories %v and its facts are not all of one of them: %s",
		id, readings, strings.Join(got, ", "))
}

// lineMismatch is why line is not the taxonomy line f's row was printed as.
func (e linkEnd) lineMismatch(id, line string, f fact.Fact) string {
	c, ok := e.vocab.Category(line)
	if !ok {
		return fmt.Sprintf("%s is no data/taxonomy.yaml line", id)
	}
	if c.Parent != f.Category {
		return fmt.Sprintf("%s is a line under %q and fact %s carries %q", id, c.Parent, f.ID, f.Category)
	}
	if c.DocumentTerm == f.RowLabel {
		return ""
	}
	for _, a := range c.Aliases {
		if a.Term == f.RowLabel {
			return ""
		}
	}
	return fmt.Sprintf("%s is not printed as %q, fact %s's row", id, f.RowLabel, f.ID)
}
