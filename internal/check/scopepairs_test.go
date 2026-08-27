package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// scopeFact is one fact at an address, which is all this check reads. The
// amount is deliberately absent: the claim is about two scopes occupying one
// address, not about what they say there -- two scopes agreeing to the cent is
// the doubling case, not the safe one.
func scopeFact(scope string, kind mapping.Kind, category string, fund int) fact.Fact {
	return fact.Fact{
		Scope: scope, Kind: kind, Category: category, FundGroup: "general",
		Fund: fund, FiscalYear: 2026, Basis: mapping.BasisAdopted,
	}
}

// scopeSubject is a Subject carrying facts in the named scopes and ONE
// projection selecting the scopes given, so the check has something to examine.
func scopeSubject(facts []fact.Fact, selects ...string) *Subject {
	return &Subject{
		Facts: facts,
		Projections: []Projection{{
			Name: "test-projection",
			Options: project.Options{
				Columns: []project.Column{{FiscalYear: 2026, Basis: mapping.BasisAdopted}},
				Scopes:  selects,
				Version: testVersion,
			},
		}},
	}
}

// TestProjectionScopesAreDisjointCatchesEachRouteSeparately is the proof that
// neither half of the check is redundant.
//
// The two routes are blind in opposite directions and the test says where:
// a reconciliation-related pair shares NO key (the spine carries fund 0 and a
// detail carries fund numbers), and a shared-key pair is related by NO check
// (two details tie to the spine, never to each other). Fold them into one rule
// and one of these two cases stops being caught.
func TestProjectionScopesAreDisjointCatchesEachRouteSeparately(t *testing.T) {
	c := &projectionScopesAreDisjoint{}

	t.Run("a reconciled pair, sharing no key at all", func(t *testing.T) {
		// The spine's fund is 0 and the detail's is 100, so the addresses
		// differ and only the declaration relates them -- which is exactly the
		// committed corpus's shape for all-funds-gross against revenue-by-fund.
		facts := []fact.Fact{
			scopeFact(project.PublishedScope, mapping.KindRevenue, "taxes/property", 0),
			scopeFact(revenueDetailScope, mapping.KindRevenue, "taxes/property", 100),
		}
		s := scopeSubject(facts, project.PublishedScope, revenueDetailScope)
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("status = %s over a reconciled pair, want FAIL", res.Status)
		}
		if !containsAll(res, "revenue-detail-ties-to-spine") {
			t.Errorf("the finding does not name the certificate that relates them: %v", res.Findings)
		}
	})

	t.Run("a shared-key pair, related by no check", func(t *testing.T) {
		// Two detail scopes at one address. Nothing reconciles them to each
		// other, so the declaration cannot see this and only the measurement
		// can -- the committed corpus's revenue-by-fund against transfers-by-fund.
		facts := []fact.Fact{
			scopeFact(revenueDetailScope, mapping.KindTransferIn, "transfers/in", 100),
			scopeFact(transfersDetailScope, mapping.KindTransferIn, "transfers/in", 100),
		}
		s := scopeSubject(facts, revenueDetailScope, transfersDetailScope)
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("status = %s over a shared-key pair, want FAIL", res.Status)
		}
		if !containsAll(res, "share 1 key") {
			t.Errorf("the finding does not name the shared address: %v", res.Findings)
		}
	})

	t.Run("an undeclared pair fails closed", func(t *testing.T) {
		// Sharing no key and named by no declaration. Silence is not evidence
		// of disjointness -- the first subtest is a pair that shares no key and
		// IS the same money -- so this is a finding rather than a pass.
		facts := []fact.Fact{
			scopeFact("some-new-schedule", mapping.KindRevenue, "taxes/sales", 200),
			scopeFact(transfersDetailScope, mapping.KindTransferIn, "transfers/in", 100),
		}
		s := scopeSubject(facts, "some-new-schedule", transfersDetailScope)
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusFail {
			t.Fatalf("status = %s over an undeclared pair, want FAIL", res.Status)
		}
		if !containsAll(res, "nothing declares that pair disjoint") {
			t.Errorf("the finding does not say the pair is undeclared: %v", res.Findings)
		}
	})

	t.Run("the declared-disjoint pair passes", func(t *testing.T) {
		facts := []fact.Fact{
			scopeFact(revenueDetailScope, mapping.KindRevenue, "taxes/property", 100),
			scopeFact(expenditureDetailScope, mapping.KindExpenditure, "wages-and-benefits", 100),
		}
		s := scopeSubject(facts, revenueDetailScope, expenditureDetailScope)
		res, err := c.Run(t.Context(), s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != StatusPass {
			t.Fatalf("status = %s over the declared-disjoint pair, want PASS: %v",
				res.Status, res.Findings)
		}
	})
}

// TestADisjointDeclarationTheCorpusContradictsGoesRed is what keeps
// disjointScopes from becoming prose.
//
// The declaration says revenue-by-fund and expenditure-by-department cannot
// collide because they are disjoint by KIND. If a fact ever lands that makes
// them share an address, the reason has stopped being true and the entry must go
// red rather than quiet -- the same rule unprojectedScopes' declarations follow.
func TestADisjointDeclarationTheCorpusContradictsGoesRed(t *testing.T) {
	// One address, both scopes. Only reachable if a rule mis-declares its kind,
	// which is precisely the mistake worth catching.
	facts := []fact.Fact{
		scopeFact(revenueDetailScope, mapping.KindExpenditure, "wages-and-benefits", 100),
		scopeFact(expenditureDetailScope, mapping.KindExpenditure, "wages-and-benefits", 100),
	}
	// No projection selects the pair: the contradiction is in the DECLARATION
	// and must be reported whether or not anything relies on it yet.
	s := &Subject{Facts: facts}
	res, err := (&projectionScopesAreDisjoint{}).Run(t.Context(), s)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != StatusFail {
		t.Fatalf("status = %s, want FAIL: the declaration says these cannot collide "+
			"and the corpus shows them colliding", res.Status)
	}
	if !containsAll(res, "the corpus disagrees") {
		t.Errorf("the finding does not say the declaration is contradicted: %v", res.Findings)
	}
}

// containsAll reports whether some finding's detail or subject carries want.
func containsAll(res Result, want string) bool {
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, want) || strings.Contains(f.Subject, want) {
			return true
		}
	}
	return false
}
