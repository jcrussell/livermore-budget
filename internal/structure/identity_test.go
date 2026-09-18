package structure_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

func allCutNamed(t *testing.T, name string) structure.Cut {
	t.Helper()
	for _, c := range structure.AllCuts() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no declared cut named %q", name)
	return structure.Cut{}
}

// TestEveryScopeInTheStoreIsACut is what makes AllCuts a description of the
// store rather than a list: every scope the store carries is selected by at
// least one cut, every fact is admitted by exactly one, and every cut sits at
// the level it declares.
func TestEveryScopeInTheStoreIsACut(t *testing.T) {
	facts := committedFacts(t)
	byRule, err := structure.LevelOfRule(facts, committedFiles(t))
	if err != nil {
		t.Fatalf("derive each rule's level: %v", err)
	}
	cuts := structure.AllCuts()
	empty, err := structure.ValidateCuts(facts, byRule, cuts)
	if err != nil {
		t.Fatalf("ValidateCuts: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("cuts with no fact over the committed store: %v", empty)
	}
	scopes := map[string]bool{}
	for i := range facts {
		scopes[facts[i].Scope] = true
	}
	covered := map[string]bool{}
	for _, c := range cuts {
		covered[c.Scope] = true
	}
	for sc := range scopes {
		if !covered[sc] {
			t.Errorf("scope %q is in the store and no cut selects it", sc)
		}
	}
	for _, c := range cuts {
		if !scopes[c.Scope] {
			t.Errorf("cut %q selects scope %q, which the store does not carry", c.Name, c.Scope)
		}
	}
	// EXACTLY ONE CUT PER FACT, OR A DECLARED RESIDUE. Two cuts admitting one
	// fact would put it in two sides of a comparison, or twice in a view; no
	// cut admitting it would leave it out of both with nothing saying so.
	// Measured: pp.85-125's one Transfers Out row is the whole residue, four
	// facts over four columns.
	findings, uncovered := structure.Covered(facts, cuts, structure.BudgetBookResidue())
	if len(findings) != 0 {
		t.Errorf("coverage:\n  %s", strings.Join(findings, "\n  "))
	}
	if uncovered != 4 {
		t.Errorf("%d facts under the declared residue, want dw-maintenance's 4 Transfers Out cells", uncovered)
	}
	if v, err := structure.NewView("everything", cuts, nil, nil); err == nil {
		t.Fatalf("a view over every cut was accepted; the cuts are not an antichain and NewView should say so: %+v", v)
	}

	t.Run("a residue matching no fact is refused, and an undeclared fact is named", func(t *testing.T) {
		findings, _ := structure.Covered(facts, cuts, nil)
		if len(findings) != 4 || !strings.Contains(findings[0], "dw-maintenance") {
			t.Errorf("with no residue declared, want the 4 Transfers Out facts named:\n  %s", strings.Join(findings, "\n  "))
		}
		findings, _ = structure.Covered(facts, cuts, append(structure.BudgetBookResidue(),
			structure.Residue{Scope: "revenue-by-fund", Rule: "nothing", Kind: mapping.KindRevenue, Reason: "invented"}))
		if len(findings) != 1 || !strings.Contains(findings[0], "matches no fact") {
			t.Errorf("an invented residue was not refused:\n  %s", strings.Join(findings, "\n  "))
		}
	})
}

// TestThePeersSharingCellsAreOneMovementReadFromTwoEnds is fisc-n6yq's
// measurement reproduced by the generic comparison: pp.127-140 and p76 share
// 22 cells over the two adopted columns, every one a transfer in, 42,183,495.00
// on each side, and one declared identity covers them all.
func TestThePeersSharingCellsAreOneMovementReadFromTwoEnds(t *testing.T) {
	facts := committedFacts(t)
	o, err := structure.Peers(facts, allCutNamed(t, "revenue-detail"), allCutNamed(t, "transfers-detail"),
		structure.BudgetBookIdentities())
	if err != nil {
		t.Fatalf("peers: %v", err)
	}
	if len(o.Findings) != 0 {
		t.Errorf("findings under the declared identity:\n  %s", strings.Join(o.Findings, "\n  "))
	}
	if len(o.Shared) != 22 {
		t.Fatalf("%d shared cells, want 22", len(o.Shared))
	}
	var a, b int64
	for _, sh := range o.Shared {
		if sh.Kind != mapping.KindTransferIn || sh.Identity != "a-transfer-in-is-printed-at-both-ends" {
			t.Errorf("%s: kind %s under identity %q", sh.Key, sh.Kind, sh.Identity)
		}
		if sh.Key.Basis != "adopted" {
			t.Errorf("%s: shared outside the adopted columns", sh.Key)
		}
		a += sh.A.Cents
		b += sh.B.Cents
	}
	if diff := cmp.Diff([]int64{4218349500, 4218349500}, []int64{a, b}); diff != "" {
		t.Errorf("the two readings summed (-want +got):\n%s", diff)
	}
}

// TestAPeerOverlapGoesRed is the mutation for the identity edge, each arm run
// in memory against the committed store.
func TestAPeerOverlapGoesRed(t *testing.T) {
	facts := committedFacts(t)
	rd, td := allCutNamed(t, "revenue-detail"), allCutNamed(t, "transfers-detail")
	identities := structure.BudgetBookIdentities()

	t.Run("a second reading with a different amount names the key and both amounts", func(t *testing.T) {
		planted := make([]fact.Fact, len(facts))
		copy(planted, facts)
		moved := 0
		var key string
		for i := range planted {
			f := &planted[i]
			if f.Scope == "transfers-by-fund" && f.Kind == mapping.KindTransferIn && f.FiscalYear == 2026 &&
				f.Fund != nil && *f.Fund == 400 {
				f.AmountCents += 100
				key = structure.KeyOf(f, structure.LevelFundByCategory).String()
				moved++
				break
			}
		}
		if moved == 0 {
			t.Fatal("no p76 transfer into fund 400 to move")
		}
		o, err := structure.Peers(planted, rd, td, identities)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 1 {
			t.Fatalf("%d findings, want 1:\n  %s", len(o.Findings), strings.Join(o.Findings, "\n  "))
		}
		for _, want := range []string{key, "the pages disagree", "$1.00", "a-transfer-in-is-printed-at-both-ends"} {
			if !strings.Contains(o.Findings[0], want) {
				t.Errorf("the finding does not say %q:\n  %s", want, o.Findings[0])
			}
		}
	})

	t.Run("with no identity declared every shared cell is a finding", func(t *testing.T) {
		o, err := structure.Peers(facts, rd, td, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 22 {
			t.Fatalf("%d findings, want 22", len(o.Findings))
		}
		if !strings.Contains(o.Findings[0], "no identity says they are one figure") {
			t.Errorf("finding does not say what is missing:\n  %s", o.Findings[0])
		}
	})

	t.Run("an identity over a pair sharing no cell is refused", func(t *testing.T) {
		gf, fb := allCutNamed(t, "acfr-general-fund-summary"), allCutNamed(t, "acfr-fund-balances/general")
		id := structure.Identity{Name: "invented", A: gf.Name, B: fb.Name,
			Kinds: []mapping.Kind{mapping.KindFundBalance}, Reason: "a claim nothing bears out"}
		o, err := structure.Peers(facts, gf, fb, []structure.Identity{id})
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 1 || !strings.Contains(o.Findings[0], `identity "invented"`) ||
			!strings.Contains(o.Findings[0], "share no cell") {
			t.Fatalf("findings = %v, want the identity refused for covering nothing", o.Findings)
		}
	})

	t.Run("the spine put at the detail's level collides with p76 on the rows that name no fund", func(t *testing.T) {
		// fisc-c4ip (d), first step: with the spine at fund-by-category it is
		// a peer of p76, and the only thing separating a spine cell from a
		// detail cell is the fund coordinate. p76's two LAVWMA rows carry no
		// fund, so they land on the spine's fund-less address and the check
		// names them -- which is fisc-n6yq's re-measured pair, reproduced.
		spine := allCutNamed(t, "spine")
		spine.Level = structure.LevelFundByCategory
		o, err := structure.Peers(facts, spine, td, identities)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 2 {
			t.Fatalf("%d findings, want the 2 fund-less collisions:\n  %s", len(o.Findings), strings.Join(o.Findings, "\n  "))
		}
		for _, f := range o.Findings {
			if !strings.Contains(f, "fund=(absent)") || !strings.Contains(f, "fund_group=enterprise") {
				t.Errorf("collision is not the LAVWMA row: %s", f)
			}
		}
	})
}

// TestAViewThatWouldTraverseBothReadingsIsRefused is fisc-n6yq's acceptance
// (b): a cut set holding both readings of one figure is refused at
// construction unless it says which it takes, and once it does, the declined
// reading's cells are left out by name.
func TestAViewThatWouldTraverseBothReadingsIsRefused(t *testing.T) {
	facts := committedFacts(t)
	rd, td := allCutNamed(t, "revenue-detail"), allCutNamed(t, "transfers-detail")
	identities := structure.BudgetBookIdentities()

	_, err := structure.NewView("both", []structure.Cut{rd, td}, identities, nil)
	if err == nil || !strings.Contains(err.Error(), "which reading it takes") {
		t.Fatalf("NewView = %v, want the traversal refused", err)
	}

	_, err = structure.NewView("both", []structure.Cut{rd, td}, identities,
		map[string]string{"a-transfer-in-is-printed-at-both-ends": "spine"})
	if err == nil || !strings.Contains(err.Error(), `takes reading "spine"`) {
		t.Fatalf("NewView = %v, want a reading naming neither cut refused", err)
	}

	_, err = structure.NewView("one", []structure.Cut{rd}, identities,
		map[string]string{"a-transfer-in-is-printed-at-both-ends": "revenue-detail"})
	if err == nil || !strings.Contains(err.Error(), "joins no two of its cuts") {
		t.Fatalf("NewView = %v, want a reading over an untraversed identity refused", err)
	}

	_, err = structure.NewView("nested", []structure.Cut{rd, allCutNamed(t, "spine")}, identities, nil)
	if err == nil || !strings.Contains(err.Error(), "not an antichain") {
		t.Fatalf("NewView = %v, want a nested pair refused", err)
	}

	v, err := structure.NewView("both", []structure.Cut{rd, td}, identities,
		map[string]string{"a-transfer-in-is-printed-at-both-ends": "revenue-detail"})
	if err != nil {
		t.Fatalf("NewView with a reading: %v", err)
	}
	// COUNTED ONCE, BY NAME. The view's transfers in are pp.127-140's; p76's
	// receiving legs are declined and its paying legs stay.
	var in, out int64
	declined := 0
	for i := range facts {
		f := &facts[i]
		if f.FiscalYear != 2026 || f.Basis != mapping.BasisAdopted {
			continue
		}
		if f.Scope == "transfers-by-fund" && f.Kind == mapping.KindTransferIn {
			if v.Admits(f, identities) {
				t.Errorf("p76's receiving leg %s is counted by a view that took pp.127-140's reading", f.ID)
			}
			declined++
			continue
		}
		if !v.Admits(f, identities) {
			continue
		}
		switch f.Kind {
		case mapping.KindTransferIn:
			in += f.AmountCents
		case mapping.KindTransferOut:
			out += f.AmountCents
		default:
		}
	}
	if declined == 0 {
		t.Fatal("no p76 receiving leg was declined; the fixture no longer covers this")
	}
	if in == 0 || out == 0 {
		t.Errorf("the view counts %d cents of transfers in and %d out; both readings' other kinds should survive", in, out)
	}
}

// TestTheAntichainIsOverMoneyBothCutsPrintAndNotOverLevelsAlone pins the case
// on which a level-only antichain test and the one NewView enforces disagree.
// pp.127-140 by fund and pp.167-170 by fund, department and object are one
// above the other in the lattice, so a level-only test refuses the pair; they
// are summable because one prints revenue and the other expenditure, and no
// fact is in both.
//
// THE FIRST TWO ASSERTIONS ARE THE GUARD, not scene-setting. If the levels
// ever stop being comparable, or the kinds ever start meeting, the admission
// below would be green because the pair had become uninteresting rather than
// because NewView tests kinds -- and the two are indistinguishable by exit
// code.
func TestTheAntichainIsOverMoneyBothCutsPrintAndNotOverLevelsAlone(t *testing.T) {
	facts := committedFacts(t)
	identities := structure.BudgetBookIdentities()
	rd, gd := allCutNamed(t, "revenue-detail"), allCutNamed(t, "general-fund-departments")

	if !structure.Refines(gd.Level, rd.Level) {
		t.Fatalf("%s no longer refines %s; this pair no longer witnesses the disagreement", gd.Level, rd.Level)
	}
	for _, k := range rd.Kinds {
		if slices.Contains(gd.Kinds, k) {
			t.Fatalf("%q and %q now share kind %q; this pair no longer witnesses the disagreement", rd.Name, gd.Name, k)
		}
	}

	v, err := structure.NewView("drill", []structure.Cut{rd, gd}, identities, nil)
	if err != nil {
		t.Fatalf("NewView over revenue beside expenditure = %v, want admitted: the levels are comparable but the kinds do not meet", err)
	}
	// AND IT ADMITS BOTH SIDES. A view that took neither cut's facts would
	// satisfy the line above while meaning nothing.
	seen := map[string]int{}
	for i := range facts {
		if f := &facts[i]; v.Admits(f, identities) {
			seen[f.Scope]++
		}
	}
	if seen[rd.Scope] == 0 || seen[gd.Scope] == 0 {
		t.Errorf("the view admits %d facts from %q and %d from %q; both cuts should contribute",
			seen[rd.Scope], rd.Scope, seen[gd.Scope], gd.Scope)
	}

	// THE OTHER HALF, so the level test is not what was deleted: the same cut
	// beside one whose kinds it meets is still refused.
	spine := allCutNamed(t, "spine")
	if _, err := structure.NewView("spine-and-drill", []structure.Cut{spine, gd}, identities, nil); err == nil ||
		!strings.Contains(err.Error(), "not an antichain") {
		t.Fatalf("NewView over the spine beside the departments = %v, want refused: their kinds meet", err)
	}
}
