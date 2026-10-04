package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestTheCommittedPeersOverlapOnlyByDeclaredIdentity pins, by name, which
// pairs at one level the committed corpus compares, the one overlap and its
// figure, and which pairs are refused with which reason.
func TestTheCommittedPeersOverlapOnlyByDeclaredIdentity(t *testing.T) {
	t.Parallel()
	s := committed(t)
	res := resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	if res.Status != StatusPass {
		t.Fatalf("status = %s, findings:\n  %v", res.Status, res.Findings)
	}
	if res.Subjects != 207 {
		t.Errorf("subjects = %d, want the 22 cells pp.127-140 and p76 share and the 185 pp.186-209 share with them and p222", res.Subjects)
	}
	for _, want := range []string{
		"207 shared cell(s) over 10 pair(s)",
		`revenue-detail + fund-balance-flows at fund-by-category: 60 shared cell(s), 60 under identity "a-fund-balance-transfer-in-is-the-revenue-schedules" carrying $98,159,217.00 on each side`,
		`transfers-detail + fund-balance-flows at fund-by-category: 56 shared cell(s), 56 under identity "a-fund-balance-transfer-is-p76s" carrying $86,301,260.00 on each side`,
		`cip-transfers-out + fund-balance-flows at fund-by-category: 69 shared cell(s), 69 under identity "a-fund-balance-transfer-to-the-cip-is-p222s" carrying $181,817,740.00 on each side`,
		"cip-funds + fund-balance-flows at fund-by-category: 0 shared cell(s)",
		"7 exception(s) excuse a cell one side of an identity has no row for: pp.127-130-print-no-general-fund-transfer-in-2024, pp.127-130-print-no-general-fund-transfer-in-2025, pp.127-130-print-no-general-fund-transfer-in-p76-2026, pp.127-130-print-no-general-fund-transfer-in-p76-2027, pp.127-130-print-no-general-fund-transfer-in-pp.186-209-2026, pp.127-130-print-no-general-fund-transfer-in-pp.186-209-2027, pp.131-140-print-no-general-fund-cip-reserves-2025",
		"revenue-detail + cip-funds at fund-by-category: 0 shared cell(s)",
		"transfers-detail + cip-transfers-out at fund-by-category: 0 shared cell(s)",
		"transfers-detail + cip-funds at fund-by-category: 0 shared cell(s)",
		`revenue-detail + transfers-detail at fund-by-category: 22 shared cell(s), 22 under identity "a-transfer-in-is-printed-at-both-ends" carrying $42,183,495.00 on each side`,
		"acfr-general-fund-summary + acfr-fund-balances/general at fund-group-by-category: 0 shared cell(s)",
		"acfr-changes-in-fund-balances + acfr-fund-balances/other-governmental at category: 0 shared cell(s)",
		"16 pair(s) at one level refused: spine/acfr-general-fund-summary, spine/acfr-fund-balances/general, revenue-detail/cip-transfers-out, revenue-detail/general-fund-by-category, revenue-detail/fund-expenditures, transfers-detail/general-fund-by-category, transfers-detail/fund-expenditures, cip-transfers-out/cip-funds, cip-transfers-out/general-fund-by-category, cip-transfers-out/fund-expenditures, cip-funds/general-fund-by-category, cip-funds/fund-expenditures, general-fund-by-category/fund-expenditures, general-fund-by-category/fund-balance-flows, fund-expenditures/fund-balance-flows, fund-balance-revenues/fund-balance-expenses",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary does not say %q", want)
		}
	}
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
	res := resultFor(t, runOne(t, &mutated, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
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
	t.Parallel()
	s := mutable(t)
	run := func(t *testing.T, ids []structure.Identity) Result {
		t.Helper()
		prev := s.Identities
		s.Identities = ids
		t.Cleanup(func() { s.Identities = prev })
		return resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	}

	t.Run("an identity ValidateIdentities refuses is reported", func(t *testing.T) {
		res := run(t, append(structure.BudgetBookIdentities(), structure.Identity{
			Name: "unnamed-cut", A: "no-such-cut", B: "spine",
			Kinds: []vocab.Kind{vocab.KindRevenue}, Reason: "a plant",
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

	res := resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
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
		res := resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
		if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Detail, "pinned at their own level") {
			t.Fatalf("status %s, want the level refused:\n  %v", res.Status, res.Findings)
		}
	})
	lattice := resultFor(t, runOne(t, s, &cutsTieAlongTheLattice{}), "cuts-tie-along-the-lattice")
	for _, f := range lattice.Findings {
		if f.Subject == stale.Name {
			t.Errorf("the lattice check names an exception between peers: %s", f.Detail)
		}
	}
}
