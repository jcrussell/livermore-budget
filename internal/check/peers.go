package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/structure"
)

// peersOverlapOnlyByDeclaredIdentity asserts that where two cuts at one level
// print the same cell, a declared identity says so and the two readings agree.
//
// THIS IS THE MEASURED HALF, AND CONTAINMENT IS BLIND EXACTLY HERE. Two cuts
// at one level are peers: neither decomposes the other, so
// cuts-tie-along-the-lattice refuses the pair by design, and a total over
// both would count every shared cell twice while every check downstream
// re-summed the same facts and agreed. Measured over the committed store,
// pp.127-140 and p76 share the fund-level transfers in -- one movement printed
// at the receiving fund and at the paying end -- and nothing else in
// internal/structure sees it.
//
// FOUR ARMS. A shared cell no identity covers is a finding. A shared cell
// under an identity whose amounts differ is a finding: the pages disagree on
// a figure the identity says is one. A non-zero cell under an identity that
// one side prints and the other does not is a finding unless an exception
// declares the absence. A declared identity that no shared cell bears out is
// refused, because an exemption over a cell nobody prints exempts nothing.
//
// WHAT A VIEW DOES WITH IT is decided at construction, not here: a cut set
// holding both readings is refused by structure.NewView unless it names which
// reading it takes.
type peersOverlapOnlyByDeclaredIdentity struct{}

var _ Check = (*peersOverlapOnlyByDeclaredIdentity)(nil)

func (*peersOverlapOnlyByDeclaredIdentity) ID() string {
	return "peers-overlap-only-by-declared-identity"
}
func (*peersOverlapOnlyByDeclaredIdentity) Tier() int  { return 1 }
func (*peersOverlapOnlyByDeclaredIdentity) Full() bool { return false }
func (*peersOverlapOnlyByDeclaredIdentity) Description() string {
	return "every cell two cuts at one level both publish is covered by a declared identity " +
		"whose two readings agree to the cent, and every declared identity covers a cell"
}

// budgetBookIdentities is a seam so a test can declare an identity the tree
// does not.
var budgetBookIdentities = structure.BudgetBookIdentities

func (*peersOverlapOnlyByDeclaredIdentity) Run(_ context.Context, s *Subject) (Result, error) {
	cuts := structure.AllCuts()
	identities := budgetBookIdentities()
	exceptions := budgetBookExceptions()

	var findings []Finding
	if err := structure.ValidateIdentities(cuts, identities); err != nil {
		findings = append(findings, finding("identities", "%v", err))
	}

	// A cut no fact falls in is not a peer of anything;
	// cuts-tie-along-the-lattice is what goes red on it.
	isEmpty := map[string]bool{}
	for _, c := range cuts {
		none := true
		for i := range s.Facts {
			if s.Facts[i].Scope == c.Scope {
				none = false
				break
			}
		}
		if none {
			isEmpty[c.Name] = true
		}
	}

	var (
		subjects int
		pairs    int
		clauses  []string
		refused  []string
		borne    = map[string]bool{}
	)
	for i, a := range cuts {
		for _, b := range cuts[i+1:] {
			if a.Level != b.Level || isEmpty[a.Name] || isEmpty[b.Name] {
				continue
			}
			o, err := structure.Peers(s.Facts, a, b, identities, exceptions)
			if err != nil {
				refused = append(refused, err.Error())
				continue
			}
			pairs++
			subjects += len(o.Shared)
			for _, f := range o.Findings {
				findings = append(findings, finding(fmt.Sprintf("%s + %s", a.Name, b.Name), "%s", f))
			}
			byIdentity := map[string]int{}
			cents := map[string]int64{}
			for _, sh := range o.Shared {
				if sh.Identity != "" {
					byIdentity[sh.Identity]++
					borne[sh.Identity] = true
					cents[sh.Identity] += sh.A.Cents
				}
			}
			clause := fmt.Sprintf("%s + %s at %s: %d shared cell(s)", a.Name, b.Name, o.At, len(o.Shared))
			for _, id := range identities {
				if n := byIdentity[id.Name]; n > 0 {
					clause += fmt.Sprintf(", %d under identity %q carrying %s on each side (%s)",
						n, id.Name, structure.Cents(cents[id.Name]), id.Reason)
				}
			}
			clauses = append(clauses, clause)
		}
	}

	// AN IDENTITY BETWEEN TWO CUTS THAT WERE NEVER COMPARED is not borne out
	// and not refused by Peers, so it is refused here: a renamed cut or a
	// refused pair would otherwise leave the declaration inert.
	for _, id := range identities {
		if borne[id.Name] || isEmpty[id.A] || isEmpty[id.B] {
			continue
		}
		compared := false
		for _, f := range findings {
			if strings.Contains(f.Detail, fmt.Sprintf("identity %q", id.Name)) {
				compared = true
				break
			}
		}
		if !compared {
			findings = append(findings, finding(id.Name,
				"this identity joins %q and %q and no pair of peers was compared for it, so it "+
					"covers nothing; the pair was refused or a cut was renamed", id.A, id.B))
		}
	}

	// EVERY DOCUMENT THAT SUMS IS A VIEW, and the view is constructed here
	// from the scope set the projection declared, so a set that would
	// traverse both readings of one figure, or hold a cut beside one that
	// decomposes it, is a finding whether or not the projection's own code
	// noticed. Series documents publish no total and are not views.
	views := 0
	seen := map[string]bool{}
	for _, p := range s.Projections {
		if p.Trends != nil {
			continue
		}
		key := p.Name + " " + p.Options.ScopeList()
		if seen[key] {
			continue
		}
		seen[key] = true
		views++
		if _, err := structure.ViewOf(p.Name, p.Options.Scopes, nil); err != nil {
			findings = append(findings, finding(p.Name, "%v", err))
		}
	}

	summary := fmt.Sprintf("%d shared cell(s) over %d pair(s) of cuts at one level: %s. %d "+
		"document scope set(s) each construct as a view",
		subjects, pairs, strings.Join(clauses, "; "), views)
	if len(refused) > 0 {
		summary += fmt.Sprintf(". %d pair(s) at one level are not comparisons and were refused by "+
			"name: %s", len(refused), strings.Join(refused, "; "))
	}
	return conclusion{
		subjects: subjects,
		unit:     "shared cells",
		held:     summary,
		nothing:  "no two cuts at one level share a cell, so there is no overlap to cover",
		findings: findings,
	}.result(), nil
}
