package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// peersOverlapOnlyByDeclaredIdentity asserts that where two cuts at one level
// print the same cell, a declared identity says so and the two readings agree.
// cuts-tie-along-the-lattice refuses peers by design, and a total over both
// would count every shared cell twice with every downstream check agreeing.
//
// A shared cell no identity covers, a shared cell whose readings differ, a
// non-zero cell one side of an identity omits without an exception, an
// identity or a category of one no shared cell bears out, and an exception
// between two peers that is declared at another level or that does not meet
// an absence at every cell it pins are each a finding.
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

func (*peersOverlapOnlyByDeclaredIdentity) Run(_ context.Context, s *Subject) (Result, error) {
	cuts := structure.AllCuts()
	identities := s.Identities
	exceptions := s.Exceptions

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
		excused  = map[string]bool{}
	)
	for i, a := range cuts {
		for _, b := range cuts[i+1:] {
			if a.Level != b.Level || isEmpty[a.Name] || isEmpty[b.Name] {
				continue
			}
			o, err := structure.Peers(s.Facts, a, b, identities, exceptions)
			if err != nil {
				refused = append(refused, a.Name+"/"+b.Name)
				continue
			}
			pairs++
			subjects += len(o.Shared)
			for _, name := range o.Excused {
				excused[name] = true
			}
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
					clause += fmt.Sprintf(", %d under identity %q carrying %s on each side",
						n, id.Name, amount.Cents(cents[id.Name]))
				}
			}
			clauses = append(clauses, clause)
		}
	}

	// An identity between two cuts never compared is inert, and refused here.
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

	// An exception between two peers is read here alone, since the lattice
	// compares no peers; one that does not meet an absence at every cell is
	// stale in part.
	for _, e := range exceptions {
		at, between := e.BetweenPeers(cuts)
		if !between || isEmpty[e.Cut] || isEmpty[e.Against] {
			continue
		}
		if e.At != at {
			findings = append(findings, finding(e.Name,
				"this exception is declared between %q and %q, two cuts at %s, at %s; an absence between "+
					"peers is pinned at their own level (%s)", e.Cut, e.Against, at, e.At, e.Bead))
			continue
		}
		if excused[e.Name] {
			continue
		}
		findings = append(findings, finding(e.Name,
			"this exception is declared between %q and %q, two cuts at one level, and at least one "+
				"of its %d cell(s) meets no absence the pair produces, or the pair was refused; remove "+
				"the cells that have stopped describing the corpus (%s)", e.Cut, e.Against, len(e.Cells), e.Bead))
	}

	// Every document that sums is constructed as a view from its declared
	// scope set, so one holding both readings of a figure is a finding here.
	// Series documents publish no total and are not views.
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
	if len(excused) > 0 {
		names := make([]string, 0, len(excused))
		for name := range excused {
			names = append(names, name)
		}
		sort.Strings(names)
		summary += fmt.Sprintf(". %d exception(s) excuse a cell one side of an identity has no row for: %s",
			len(names), strings.Join(names, ", "))
	}
	if len(refused) > 0 {
		summary += fmt.Sprintf(". %d pair(s) at one level refused: %s", len(refused), strings.Join(refused, ", "))
	}
	return conclusion{
		subjects: subjects,
		unit:     "shared cells",
		held:     summary,
		nothing:  "no two cuts at one level share a cell, so there is no overlap to cover",
		findings: findings,
	}.result(), nil
}
