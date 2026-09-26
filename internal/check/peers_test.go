package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheCommittedPeersOverlapOnlyByDeclaredIdentity pins, by name, what the
// peer check covers over the committed corpus: which pairs at one level were
// compared, the one overlap and its figure, and which pairs were refused with
// which reason.
func TestTheCommittedPeersOverlapOnlyByDeclaredIdentity(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res := resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	if res.Status != StatusPass {
		t.Fatalf("status = %s, findings:\n  %v", res.Status, res.Findings)
	}
	if res.Subjects != 22 {
		t.Errorf("subjects = %d, want the 22 cells pp.127-140 and p76 share", res.Subjects)
	}
	for _, want := range []string{
		"22 shared cell(s) over 3 pair(s)",
		`revenue-detail + transfers-detail at fund-by-category: 22 shared cell(s), 22 under identity "a-transfer-in-is-printed-at-both-ends" carrying $42183495.00 on each side`,
		"acfr-general-fund-summary + acfr-fund-balances/general at fund-group-by-category: 0 shared cell(s)",
		"acfr-changes-in-fund-balances + acfr-fund-balances/other-governmental at category: 0 shared cell(s)",
		`peers "spine" and "acfr-general-fund-summary": "spine" prints [adopted] columns and "acfr-general-fund-summary" prints [audited]; there is no column both print`,
		`peers "spine" and "acfr-fund-balances/general"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary does not say %q", want)
		}
	}
}

// TestThePeerCheckGoesRed runs the amount mutation through the registered
// check, so what is proven is the line fisc verify prints.
func TestThePeerCheckGoesRed(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	planted := make([]fact.Fact, len(s.Facts))
	copy(planted, s.Facts)
	moved := 0
	for i := range planted {
		f := &planted[i]
		if f.Scope == "transfers-by-fund" && f.Kind == mapping.KindTransferIn && f.FiscalYear == 2027 &&
			f.Fund != nil && *f.Fund == 610 {
			f.AmountCents += 100
			moved++
			break
		}
	}
	if moved == 0 {
		t.Fatal("no p76 transfer into Stormwater to move")
	}
	mutated := *s
	mutated.Facts = planted
	res := resultFor(t, runOne(t, &mutated, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	if res.Status != StatusFail || len(res.Findings) != 1 {
		t.Fatalf("status %s with %d findings, want one failure:\n  %v", res.Status, len(res.Findings), res.Findings)
	}
	for _, want := range []string{"fund=610", "the pages disagree", "$1.00"} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("the finding does not say %q: %s", want, res.Findings[0].Detail)
		}
	}
}

// TestADocumentSelectingBothReadingsIsAFinding is fisc-n6yq's acceptance (b)
// reached through the registered check: a projection whose scope set holds
// both readings of one figure is refused as a view whether or not its own
// code noticed, and one whose scopes are summable together is not.
func TestADocumentSelectingBothReadingsIsAFinding(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mutated := *s
	mutated.Projections = append([]projection{{
		Name: "both-readings",
		Options: project.Options{
			Columns: []project.Column{{FiscalYear: 2026, Basis: mapping.BasisAdopted}},
			Scopes:  []string{"revenue-by-fund", "transfers-by-fund"},
			Version: testVersion,
		},
	}, {
		Name: "spine-beside-its-decomposition",
		Options: project.Options{
			Columns: []project.Column{{FiscalYear: 2026, Basis: mapping.BasisAdopted}},
			Scopes:  []string{project.PublishedScope, "revenue-by-fund"},
			Version: testVersion,
		},
	}}, s.Projections...)
	res := resultFor(t, runOne(t, &mutated, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	if res.Status != StatusFail || len(res.Findings) != 2 {
		t.Fatalf("status %s with %d findings, want two failures:\n  %v", res.Status, len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Detail, "which reading it takes") {
		t.Errorf("the both-readings finding does not say what is missing: %s", res.Findings[0].Detail)
	}
	if !strings.Contains(res.Findings[1].Detail, "not an antichain") {
		t.Errorf("the nested finding does not name the lattice: %s", res.Findings[1].Detail)
	}
}

// TestThePeerCheckReportsItsDeclarations drives the arms that read the
// declarations rather than the store, through the seam.
func TestThePeerCheckReportsItsDeclarations(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	run := func(t *testing.T, ids []structure.Identity) Result {
		t.Helper()
		prev := budgetBookIdentities
		budgetBookIdentities = func() []structure.Identity { return ids }
		t.Cleanup(func() { budgetBookIdentities = prev })
		return resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	}

	t.Run("an identity ValidateIdentities refuses is reported", func(t *testing.T) {
		res := run(t, append(structure.BudgetBookIdentities(), structure.Identity{
			Name: "unnamed-cut", A: "no-such-cut", B: "spine",
			Kinds: []mapping.Kind{mapping.KindRevenue}, Reason: "a plant",
		}))
		var reported bool
		for _, f := range res.Findings {
			if f.Subject == "identities" && strings.Contains(f.Detail, `names cut "no-such-cut"`) {
				reported = true
			}
		}
		if !reported {
			t.Fatalf("status %s, want the identities arm:\n  %v", res.Status, res.Findings)
		}
	})

	t.Run("an identity between peers that were never compared covers nothing", func(t *testing.T) {
		res := run(t, append(structure.BudgetBookIdentities(), structure.Identity{
			Name: "never-compared", A: "spine", B: "acfr-general-fund-summary",
			Kinds: []mapping.Kind{mapping.KindRevenue}, Reason: "a plant",
		}))
		if len(res.Findings) != 1 || res.Findings[0].Subject != "never-compared" ||
			!strings.Contains(res.Findings[0].Detail, "covers nothing") {
			t.Fatalf("status %s, want exactly the covers-nothing arm:\n  %v", res.Status, res.Findings)
		}
	})
}

// TestAOneSidedCellUnderAnIdentityGoesRed moves pp.127-140's transfers into
// fund 610 to fund 621: every cell stays one-sided, none is shared, and
// without the one-sided arm the check is green.
func TestAOneSidedCellUnderAnIdentityGoesRed(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mutated := *s
	mutated.Facts = make([]fact.Fact, len(s.Facts))
	copy(mutated.Facts, s.Facts)
	moved := 0
	for i := range mutated.Facts {
		f := &mutated.Facts[i]
		if f.Scope == "revenue-by-fund" && f.Kind == mapping.KindTransferIn && f.Fund != nil && *f.Fund == 610 {
			f.Fund = fact.FundNumber(621)
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("no pp.127-140 transfer into fund 610 to move")
	}
	res := resultFor(t, runOne(t, &mutated, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	var one, other bool
	for _, f := range res.Findings {
		one = one || strings.Contains(f.Detail, "fund=610") && strings.Contains(f.Detail, "no exception declares the absence")
		other = other || strings.Contains(f.Detail, "fund=621") && strings.Contains(f.Detail, "no exception declares the absence")
	}
	if res.Status != StatusFail || !one || !other {
		t.Fatalf("status %s, want both ends of the move named:\n  %v", res.Status, res.Findings)
	}
}
