package check

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestPeersRefusesAPairWithNoColumnInCommon pins the spine's refusal against
// the ACFR's audited columns, with structure.ErrNotComparable.
func TestPeersRefusesAPairWithNoColumnInCommon(t *testing.T) {
	t.Parallel()
	s := committed(t)
	cut := map[string]structure.Cut{}
	for _, c := range structure.AllCuts() {
		cut[c.Name] = c
	}
	for _, b := range []string{"acfr-general-fund-summary", "acfr-fund-balances/general"} {
		_, err := structure.Peers(s.Facts, cut["spine"], cut[b], structure.BudgetBookIdentities(), structure.BudgetBookExceptions())
		want := `"spine" prints [adopted] columns and "` + b + `" prints [audited]; there is no column both print`
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Peers(spine, %s) = %v, want %q", b, err, want)
		}
		if !errors.Is(err, structure.ErrNotComparable) {
			t.Errorf("Peers(spine, %s) = %v, which is not structure.ErrNotComparable", b, err)
		}
	}
}

// TestThePeerCheckGoesRed runs the amount mutation through the registered
// check, so what is proven is the line fisc verify prints.
func TestThePeerCheckGoesRed(t *testing.T) {
	t.Parallel()
	s := committed(t)
	planted := make([]fact.Fact, len(s.Facts))
	copy(planted, s.Facts)
	moved := 0
	for i := range planted {
		f := &planted[i]
		if f.Scope == "transfers-by-fund" && f.Kind == vocab.KindTransferIn && f.FiscalYear == 2027 &&
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
	res := runCheck(t, &peersOverlapOnlyByDeclaredIdentity{}, &mutated)
	// The moved p76 figure disagrees with both other readings of it:
	// pp.127-140's and pp.204-209's.
	if res.Status != StatusFail || len(res.Findings) != 2 {
		t.Fatalf("status %s with %d findings, want two failures:\n  %v", res.Status, len(res.Findings), res.Findings)
	}
	for i, pair := range []string{"revenue-detail + transfers-detail", "transfers-detail + fund-balance-flows"} {
		if res.Findings[i].Subject != pair {
			t.Errorf("finding %d is on %q, want %q", i, res.Findings[i].Subject, pair)
		}
		for _, want := range []string{"fund=610", "the pages disagree", "$1.00"} {
			if !strings.Contains(res.Findings[i].Detail, want) {
				t.Errorf("the finding does not say %q: %s", want, res.Findings[i].Detail)
			}
		}
	}
}

// TestADocumentSelectingBothReadingsIsAFinding: a projection whose scope set
// holds both readings of one figure, or a cut beside its decomposition, is
// refused as a view.
func TestADocumentSelectingBothReadingsIsAFinding(t *testing.T) {
	t.Parallel()
	s := committed(t)
	mutated := *s
	mutated.Projections = append([]projection{{
		Name: "both-readings",
		Options: project.Options{
			Columns: []project.Column{{FiscalYear: 2026, Basis: vocab.BasisAdopted}},
			Scopes:  []string{"revenue-by-fund", "transfers-by-fund"},
			Version: testVersion,
		},
	}, {
		Name: "spine-beside-its-decomposition",
		Options: project.Options{
			Columns: []project.Column{{FiscalYear: 2026, Basis: vocab.BasisAdopted}},
			Scopes:  []string{project.PublishedScope, "revenue-by-fund"},
			Version: testVersion,
		},
	}}, s.Projections...)
	res := runCheck(t, &peersOverlapOnlyByDeclaredIdentity{}, &mutated)
	if res.Status != StatusFail || len(res.Findings) != 2 {
		t.Fatalf("status %s with %d findings, want two failures:\n  %v", res.Status, len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Detail, "count it twice") {
		t.Errorf("the both-readings finding does not say what is missing: %s", res.Findings[0].Detail)
	}
	if !strings.Contains(res.Findings[1].Detail, "not an antichain") {
		t.Errorf("the nested finding does not name the lattice: %s", res.Findings[1].Detail)
	}
}

// TestThePeerCheckReportsItsDeclarations drives the arms that read the
// declarations rather than the store, through the seam.
func TestThePeerCheckReportsItsDeclarations(t *testing.T) {
	t.Parallel()
	s := mutable(t)
	run := func(t *testing.T, ids []structure.Identity) Result {
		t.Helper()
		prev := s.Identities
		s.Identities = ids
		t.Cleanup(func() { s.Identities = prev })
		return runCheck(t, &peersOverlapOnlyByDeclaredIdentity{}, s)
	}

	t.Run("an identity ValidateIdentities refuses is reported", func(t *testing.T) {
		res := run(t, append(structure.BudgetBookIdentities(), structure.Identity{
			Name: "unnamed-cut", A: "no-such-cut", B: "spine",
			Kinds: []vocab.Kind{vocab.KindRevenue}, Reason: "a plant",
		}))
		if !hasFinding(res, "identities: ", `names cut "no-such-cut"`) {
			t.Fatalf("status %s, want the identities arm:\n  %v", res.Status, res.Findings)
		}
	})

	t.Run("an identity between peers that were never compared covers nothing", func(t *testing.T) {
		res := run(t, append(structure.BudgetBookIdentities(), structure.Identity{
			Name: "never-compared", A: "spine", B: "acfr-general-fund-summary",
			Kinds: []vocab.Kind{vocab.KindRevenue}, Reason: "a plant",
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
	t.Parallel()
	s := committed(t)
	mutated := *s
	mutated.Facts = make([]fact.Fact, len(s.Facts))
	copy(mutated.Facts, s.Facts)
	moved := 0
	for i := range mutated.Facts {
		f := &mutated.Facts[i]
		if f.Scope == "revenue-by-fund" && f.Kind == vocab.KindTransferIn && f.Fund != nil && *f.Fund == 610 {
			f.Fund = fact.FundNumber(621)
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("no pp.127-140 transfer into fund 610 to move")
	}
	res := runCheck(t, &peersOverlapOnlyByDeclaredIdentity{}, &mutated)
	const absent = "no exception declares the absence"
	if res.Status != StatusFail || !hasFinding(res, "fund=610", absent) || !hasFinding(res, "fund=621", absent) {
		t.Fatalf("status %s, want both ends of the move named:\n  %v", res.Status, res.Findings)
	}
}

// TestAnExceptionBetweenPeersIsThePeerChecks: an absence pinned between two
// cuts at one level is read by the peer check alone, which refuses one that
// excuses no absence; the lattice check, which compares no peers, leaves it be.
func TestAnExceptionBetweenPeersIsThePeerChecks(t *testing.T) {
	t.Parallel()
	s := mutable(t)
	stale := structure.Exception{
		Name: "a-plant-between-peers", Cut: structure.CutRevenueDetail, Against: structure.CutFundBalanceFlows,
		At: structure.LevelFundByCategory,
		Cells: []structure.Pin{{Year: 2026, Basis: "adopted",
			Coords: map[structure.Axis]string{structure.AxisFundGroup: "general", structure.AxisFund: "999",
				structure.AxisCategory: "transfers/in"},
			Against: structure.Sum{Cents: 100, Present: true}}},
		Residual: 100, Printed: "a plant", Reason: "a plant", Bead: "fisc-3eh2",
	}
	withExceptions(t, s, append(structure.BudgetBookExceptions(), stale))

	res := runCheck(t, &peersOverlapOnlyByDeclaredIdentity{}, s)
	if res.Status != StatusFail || len(res.Findings) != 1 || res.Findings[0].Subject != stale.Name ||
		!strings.Contains(res.Findings[0].Detail, "meets no absence the pair produces") {
		t.Fatalf("status %s, want exactly the stale exception named:\n  %v", res.Status, res.Findings)
	}
	t.Run("declared at another level", func(t *testing.T) {
		coarse := stale
		coarse.At = structure.LevelFundGroupByCategory
		coarse.Cells = []structure.Pin{{Year: 2026, Basis: "adopted",
			Coords:  map[structure.Axis]string{structure.AxisFundGroup: "general", structure.AxisCategory: "transfers/in"},
			Against: structure.Sum{Cents: 100, Present: true}}}
		withExceptions(t, s, append(structure.BudgetBookExceptions(), coarse))
		res := runCheck(t, &peersOverlapOnlyByDeclaredIdentity{}, s)
		if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Detail, "pinned at their own level") {
			t.Fatalf("status %s, want the level refused:\n  %v", res.Status, res.Findings)
		}
	})
	lattice := runCheck(t, &cutsTieAlongTheLattice{}, s)
	for _, f := range lattice.Findings {
		if f.Subject == stale.Name {
			t.Errorf("the lattice check names an exception between peers: %s", f.Detail)
		}
	}
}

// peersOver is the registered peer check run over a lattice the test
// declares, so what is proven is still the result fisc verify prints.
type peersOver struct {
	peersOverlapOnlyByDeclaredIdentity
	cuts []structure.Cut
}

func (c *peersOver) Run(_ context.Context, s *Subject) (Result, error) {
	return c.run(s, c.cuts)
}

// A pair Peers cannot compare because of its declarations (two cuts at a level
// no lattice declares) is a finding, not a refused pair.
//
// Mutation: count every error from structure.Peers as refused; the check goes
// green with "x/y" among the refused pairs.
func TestAPeerComparisonThatFailsOnItsDeclarationsIsAFinding(t *testing.T) {
	t.Parallel()
	s := committed(t)
	var base structure.Cut
	for _, c := range structure.AllCuts() {
		if len(c.FundGroups) == 0 && c.DepartmentTier == "" && c.Scope != "" {
			base = c
			break
		}
	}
	if base.Name == "" {
		t.Fatal("no cut free of a fund-group footprint and a department tier to copy")
	}
	x, y := base, base
	x.Name, y.Name = "x", "y"
	x.Level, y.Level = "no-such-level", "no-such-level"
	x.Reference, y.Reference = false, false
	cuts := append(structure.AllCuts(), x, y)

	_, err := structure.Peers(s.Facts, x, y, s.Identities, s.Exceptions)
	if err == nil || errors.Is(err, structure.ErrNotComparable) {
		t.Fatalf("Peers(x, y) = %v, want an error that is not structure.ErrNotComparable", err)
	}

	res := runCheck(t, &peersOver{cuts: cuts}, s)
	if res.Status != StatusFail || len(res.Findings) != 1 {
		t.Fatalf("status %s with %d findings, want one failure:\n  %v", res.Status, len(res.Findings), res.Findings)
	}
	if res.Findings[0].Subject != "x + y" || !strings.Contains(res.Findings[0].Detail, `"no-such-level" is not a declared level`) {
		t.Errorf("finding = %s: %s, want the undeclared level on x + y", res.Findings[0].Subject, res.Findings[0].Detail)
	}
}
