package structure_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
	"github.com/jcrussell/livermore-budget/internal/vocab"
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

// TestEveryScopeInTheStoreIsACut: every scope the store carries is selected by
// a cut, every fact is admitted by exactly one, and every cut sits at the
// level it declares.
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
	// A scope no cut selects is held by its declared residue instead, which
	// Covered below holds fact by fact.
	for _, r := range structure.BudgetBookResidue() {
		covered[r.Scope] = true
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
	// Exactly one cut per fact, or a declared residue.
	findings, uncovered := structure.Covered(facts, cuts, structure.BudgetBookResidue())
	if len(findings) != 0 {
		t.Errorf("coverage:\n  %s", strings.Join(findings, "\n  "))
	}
	if uncovered != 4+54+1050+1120 {
		t.Errorf("%d facts under the declared residue, want dw-maintenance's 4 Transfers Out "+
			"cells, pp.80-81's 54 debt-service cells (9 issues, principal and interest, "+
			"three years), pp.224-235's 1,050 (210 projects, five years) and pp.186-209's "+
			"Capital Improvement Program Funds block, 1,120 (35 funds, eight lines, four years)", uncovered)
	}
	if v, err := structure.NewView("everything", cuts, nil, nil, nil); err == nil {
		t.Fatalf("a view over every cut was accepted; the cuts are not an antichain and NewView should say so: %+v", v)
	}

	t.Run("a residue matching no fact is refused, and an undeclared fact is named", func(t *testing.T) {
		findings, _ := structure.Covered(facts, cuts, nil)
		joined := strings.Join(findings, "\n  ")
		if len(findings) != 4+54+1050+1120 || !strings.Contains(joined, "dw-maintenance") ||
			!strings.Contains(joined, "debt-service-principal") ||
			!strings.Contains(joined, "cip-listing-p0224") ||
			!strings.Contains(joined, "fund-balances-fy2024-p0190") ||
			strings.Contains(joined, "fund-balances-fy2024-p0190-above-cip") {
			t.Errorf("with no residue declared, want the 4 Transfers Out facts, the 54 "+
				"debt-service facts, the 1,050 CIP listing facts and the 1,120 CIP-block "+
				"fund-balance facts named, and none above the block:\n  %s", joined)
		}
		findings, _ = structure.Covered(facts, cuts, append(structure.BudgetBookResidue(),
			structure.Residue{Scope: "revenue-by-fund", Rule: "nothing", Kind: vocab.KindRevenue, Reason: "invented"}))
		if len(findings) != 1 || !strings.Contains(findings[0], "matches no fact") {
			t.Errorf("an invented residue was not refused:\n  %s", strings.Join(findings, "\n  "))
		}
	})

	t.Run("a residue on a scope the store does not carry is refused", func(t *testing.T) {
		residue := structure.BudgetBookResidue()
		i := slices.IndexFunc(residue, func(r structure.Residue) bool { return r.Rule == "dw-maintenance" })
		if i < 0 {
			t.Fatal("no dw-maintenance residue declared")
		}
		residue[i].Scope = "departmentwide-expenditures-renamed"
		findings, _ := structure.Covered(facts, cuts, residue)
		joined := strings.Join(findings, "\n  ")
		if len(findings) != 4+1 || !strings.Contains(joined,
			"residue (departmentwide-expenditures-renamed, dw-maintenance, transfer_out) matches no fact") {
			t.Errorf("with dw-maintenance's residue moved to a scope the store does not carry, want its "+
				"4 Transfers Out facts named and the residue refused:\n  %s", joined)
		}
	})
}

// TestThePeersSharingCellsAreOneMovementReadFromTwoEnds: pp.127-140 and p76
// share only transfer-in cells, equal on each side, all under one identity.
func TestThePeersSharingCellsAreOneMovementReadFromTwoEnds(t *testing.T) {
	facts := committedFacts(t)
	o, err := structure.Peers(facts, allCutNamed(t, "revenue-detail"), allCutNamed(t, "transfers-detail"),
		structure.BudgetBookIdentities(), structure.BudgetBookExceptions())
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
		if sh.Kind != vocab.KindTransferIn || sh.Identity != "a-transfer-in-is-printed-at-both-ends" {
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

// TestAPeerOverlapGoesRed is the mutation for the identity edge.
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
			if f.Scope == "transfers-by-fund" && f.Kind == vocab.KindTransferIn && f.FiscalYear == 2026 &&
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
		o, err := structure.Peers(planted, rd, td, identities, structure.BudgetBookExceptions())
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
		o, err := structure.Peers(facts, rd, td, nil, nil)
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

	t.Run("the one-sided General Fund cells are held by the exceptions declaring them absent", func(t *testing.T) {
		o, err := structure.Peers(facts, rd, td, identities, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 2 {
			t.Fatalf("%d findings, want the 2 General Fund transfer-in cells:\n  %s", len(o.Findings), strings.Join(o.Findings, "\n  "))
		}
		for _, f := range o.Findings {
			if !strings.Contains(f, "fund=100") || !strings.Contains(f, "no exception declares the absence") {
				t.Errorf("finding is not the General Fund's transfer in: %s", f)
			}
		}
	})

	// Each plant pins the wrong side, or calls the missing side present, and
	// must not excuse.
	t.Run("an exception excuses an absence only on the side it pins absent", func(t *testing.T) {
		var gf []structure.Exception
		for _, e := range structure.BudgetBookExceptions() {
			if strings.HasPrefix(e.Name, "pp.127-130-print-no-general-fund-transfer-in-p76-") {
				gf = append(gf, e)
			}
		}
		if len(gf) != 2 {
			t.Fatalf("%d General Fund transfer-in exceptions, want 2", len(gf))
		}
		plant := func(edit func(e *structure.Exception, p *structure.Pin)) []structure.Exception {
			out := make([]structure.Exception, 0, len(gf))
			for _, e := range gf {
				cells := slices.Clone(e.Cells)
				e.Cells = cells
				edit(&e, &e.Cells[0])
				out = append(out, e)
			}
			return out
		}
		for _, tc := range []struct {
			name     string
			edit     func(e *structure.Exception, p *structure.Pin)
			findings int
		}{
			{"as declared", func(*structure.Exception, *structure.Pin) {}, 0},
			{"the missing side named as Against", func(e *structure.Exception, p *structure.Pin) {
				e.Cut, e.Against = e.Against, e.Cut
				p.Cut, p.Against = p.Against, p.Cut
			}, 0},
			{"the present side pinned absent", func(e *structure.Exception, _ *structure.Pin) {
				e.Cut = "transfers-detail"
			}, 2},
			{"the present side pinned absent as Against", func(e *structure.Exception, p *structure.Pin) {
				e.Cut, e.Against = e.Against, "transfers-detail"
				p.Cut, p.Against = p.Against, p.Cut
			}, 2},
			{"the missing side pinned present", func(_ *structure.Exception, p *structure.Pin) {
				p.Cut = structure.Sum{Cents: p.Against.Cents, Present: true}
			}, 2},
			{"the missing side pinned present as Against", func(e *structure.Exception, p *structure.Pin) {
				e.Cut, e.Against = e.Against, e.Cut
				p.Cut, p.Against = p.Against, structure.Sum{Cents: p.Against.Cents, Present: true}
			}, 2},
		} {
			o, err := structure.Peers(facts, rd, td, identities, plant(tc.edit))
			if err != nil {
				t.Fatal(err)
			}
			if len(o.Findings) != tc.findings {
				t.Errorf("%s: %d findings, want %d:\n  %s", tc.name, len(o.Findings), tc.findings, strings.Join(o.Findings, "\n  "))
			}
		}
	})

	t.Run("an identity over a pair sharing no cell is refused", func(t *testing.T) {
		gf, fb := allCutNamed(t, "acfr-general-fund-summary"), allCutNamed(t, "acfr-fund-balances/general")
		id := structure.Identity{Name: "invented", A: gf.Name, B: fb.Name,
			Kinds: []vocab.Kind{vocab.KindFundBalance}, Reason: "a claim nothing bears out"}
		o, err := structure.Peers(facts, gf, fb, []structure.Identity{id}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) == 0 {
			t.Fatal("no findings, want the identity refused for covering nothing")
		}
		last := o.Findings[len(o.Findings)-1]
		if !strings.Contains(last, `identity "invented"`) || !strings.Contains(last, "share no cell") {
			t.Fatalf("findings end %q, want the identity refused for covering nothing", last)
		}
	})

	t.Run("the spine put at the detail's level collides with p76 on the rows that name no fund", func(t *testing.T) {
		// With the spine at fund-by-category, p76's fund-less LAVWMA rows land
		// on the spine's address and are named.
		spine := allCutNamed(t, "spine")
		spine.Level = structure.LevelFundByCategory
		o, err := structure.Peers(facts, spine, td, identities, structure.BudgetBookExceptions())
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

// TestAViewThatWouldTraverseBothReadingsIsRefused: a cut set holding both
// readings of one figure is refused unless it says which it takes, and then
// the declined reading's cells are left out.
func TestAViewThatWouldTraverseBothReadingsIsRefused(t *testing.T) {
	facts := committedFacts(t)
	rd, td := allCutNamed(t, "revenue-detail"), allCutNamed(t, "transfers-detail")
	identities := structure.BudgetBookIdentities()

	_, err := structure.NewView("both", []structure.Cut{rd, td}, identities, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "which reading it takes") {
		t.Fatalf("NewView = %v, want the traversal refused", err)
	}

	_, err = structure.NewView("both", []structure.Cut{rd, td}, identities, nil,
		map[string]string{"a-transfer-in-is-printed-at-both-ends": "spine"})
	if err == nil || !strings.Contains(err.Error(), `takes reading "spine"`) {
		t.Fatalf("NewView = %v, want a reading naming neither cut refused", err)
	}

	_, err = structure.NewView("one", []structure.Cut{rd}, identities, nil,
		map[string]string{"a-transfer-in-is-printed-at-both-ends": "revenue-detail"})
	if err == nil || !strings.Contains(err.Error(), "joins no two of its cuts") {
		t.Fatalf("NewView = %v, want a reading over an untraversed identity refused", err)
	}

	_, err = structure.NewView("nested", []structure.Cut{rd, allCutNamed(t, "spine")}, identities, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not an antichain") {
		t.Fatalf("NewView = %v, want a nested pair refused", err)
	}

	v, err := structure.NewView("both", []structure.Cut{rd, td}, identities, nil,
		map[string]string{"a-transfer-in-is-printed-at-both-ends": "revenue-detail"})
	if err != nil {
		t.Fatalf("NewView with a reading: %v", err)
	}
	// The view's transfers in are pp.127-140's; p76's paying legs stay.
	var in, out int64
	declined := 0
	for i := range facts {
		f := &facts[i]
		if f.FiscalYear != 2026 || f.Basis != vocab.BasisAdopted {
			continue
		}
		if f.Scope == "transfers-by-fund" && f.Kind == vocab.KindTransferIn {
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
		case vocab.KindTransferIn:
			in += f.AmountCents
		case vocab.KindTransferOut:
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

// TestTheAntichainIsOverMoneyBothCutsPrintAndNotOverLevelsAlone: pp.127-140
// and pp.167-170 are one above the other in the lattice and summable, because
// their kinds do not meet. The first two assertions are the guard: without
// them the admission could be green because the pair became uninteresting.
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

	v, err := structure.NewView("drill", []structure.Cut{rd, gd}, identities, nil, nil)
	if err != nil {
		t.Fatalf("NewView over revenue beside expenditure = %v, want admitted: the levels are comparable but the kinds do not meet", err)
	}
	// And it admits both sides.
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

	// The same cut beside one whose kinds it meets is still refused.
	spine := allCutNamed(t, "spine")
	if _, err := structure.NewView("spine-and-drill", []structure.Cut{spine, gd}, identities, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not an antichain") {
		t.Fatalf("NewView over the spine beside the departments = %v, want refused: their kinds meet", err)
	}
}

// TestTwoLevelsNeitherRefiningTheOtherAreNotSummable reads the cuts whose
// levels share no refinement edge and whose kinds and footprints meet: a fund's
// total revenue beside the spine's revenue by fund group and category, and
// expenditure by department beside the spine. Each pair carries the General
// Fund's money twice, so a total over it is refused.
func TestTwoLevelsNeitherRefiningTheOtherAreNotSummable(t *testing.T) {
	identities := structure.BudgetBookIdentities()
	spine := allCutNamed(t, structure.CutSpine)
	for _, other := range []string{structure.CutFundBalanceRevenues, structure.CutDepartmentwide} {
		c := allCutNamed(t, other)
		if structure.Refines(c.Level, spine.Level) || structure.Refines(spine.Level, c.Level) {
			t.Fatalf("%s and %s are now comparable; this pair no longer witnesses an incomparable overlap",
				c.Level, spine.Level)
		}
		_, err := structure.NewView("twice", []structure.Cut{spine, c}, identities, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "two decompositions") {
			t.Errorf("NewView over the spine beside %s = %v, want refused: both print %s's money",
				other, err, "the General Fund")
		}
	}
	// The three cuts pp.186-209 publish are one view: their kinds are disjoint.
	if _, err := structure.ViewOf("fund-balances", []string{structure.ScopeFundBalancesByFund}, nil); err != nil {
		t.Errorf("ViewOf(fund-balances-by-fund) = %v, want admitted", err)
	}
}

// TestDisjointFootprintsAreSummableAndTheGeneralFundIsNot is what lets
// fund-flows draw pp.172-183 beside pp.167-170: fund-expenditures refines
// nothing general-fund-departments prints because their fund groups are
// disjoint, while p172's General Fund, which pp.167-170 decompose, is refused.
func TestDisjointFootprintsAreSummableAndTheGeneralFundIsNot(t *testing.T) {
	identities := structure.BudgetBookIdentities()
	gd := allCutNamed(t, "general-fund-departments")
	fe, gc := allCutNamed(t, "fund-expenditures"), allCutNamed(t, "general-fund-by-category")
	if !structure.Refines(gd.Level, fe.Level) {
		t.Fatalf("%s no longer refines %s; this pair no longer witnesses the footprint", gd.Level, fe.Level)
	}
	if slices.Contains(fe.FundGroups, "general") || len(fe.FundGroups) == 0 {
		t.Fatalf("fund-expenditures' footprint is %v; it must name the groups and leave out general", fe.FundGroups)
	}
	if _, err := structure.NewView("drill", []structure.Cut{gd, fe}, identities, nil, nil); err != nil {
		t.Errorf("NewView over the divisions beside every other fund's objects = %v, want admitted", err)
	}
	if _, err := structure.NewView("twice", []structure.Cut{gd, gc}, identities, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not an antichain") {
		t.Errorf("NewView over the divisions beside p172's General Fund = %v, want refused: "+
			"the same money at two grains", err)
	}
}

// TestAFactOutsideItsCutsFootprintIsInNoCut is what makes a declared footprint
// more than a promise: a pp.173-183 fact filed under the General Fund is
// admitted by no cut, and coverage reports it, rather than fund-flows summing
// it beside pp.167-170 while NewView trusts the declaration.
func TestAFactOutsideItsCutsFootprintIsInNoCut(t *testing.T) {
	facts := committedFacts(t)
	i := slices.IndexFunc(facts, func(f fact.Fact) bool { return f.Scope == "expenditure-by-fund" })
	if i < 0 {
		t.Fatal("no expenditure-by-fund fact in the committed store")
	}
	stray := facts[i]
	hundred := 100
	stray.FundGroup, stray.Fund = "general", &hundred
	findings, _ := structure.Covered([]fact.Fact{stray}, structure.AllCuts(), nil)
	if len(findings) == 0 || !strings.Contains(strings.Join(findings, "\n"), "admitted by no cut") {
		t.Fatalf("findings = %v, want the stray fact admitted by no cut", findings)
	}
}

// TestAnIdentityCoversOnlyTheCategoriesItNames: pp.186-209 print a fund's
// transfers out in two columns, and p76 lists the first and p222 the second.
// An identity naming its categories excuses neither the column the other
// schedule never prints nor, outside them, a shared cell.
func TestAnIdentityCoversOnlyTheCategoriesItNames(t *testing.T) {
	facts := committedFacts(t)
	flows, td := allCutNamed(t, structure.CutFundBalanceFlows), allCutNamed(t, "transfers-detail")
	var p76 structure.Identity
	for _, id := range structure.BudgetBookIdentities() {
		if id.Name == "a-fund-balance-transfer-is-p76s" {
			p76 = id
		}
	}
	in, out := structure.KindCategory{Kind: vocab.KindTransferIn, Category: "transfers/in"},
		structure.KindCategory{Kind: vocab.KindTransferOut, Category: "transfers/out"}
	if diff := cmp.Diff([]structure.KindCategory{in, out}, p76.Categories); diff != "" {
		t.Fatalf("p76's identity with pp.186-209 covers (-want +got):\n%s", diff)
	}
	toCIP := func(o structure.Overlap) int {
		n := 0
		for _, f := range o.Findings {
			if strings.Contains(f, "transfers/out-to-cip") {
				n++
			}
		}
		return n
	}

	o, err := structure.Peers(facts, flows, td, []structure.Identity{p76}, structure.BudgetBookExceptions())
	if err != nil {
		t.Fatal(err)
	}
	if n := toCIP(o); n != 0 {
		t.Errorf("%d findings on the Transfers Out to CIP column p76 never prints:\n  %s", n, strings.Join(o.Findings, "\n  "))
	}

	t.Run("without its categories every to-CIP cell is a one-sided finding", func(t *testing.T) {
		wide := p76
		wide.Categories = nil
		o, err := structure.Peers(facts, flows, td, []structure.Identity{wide}, structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		if toCIP(o) == 0 {
			t.Fatal("an identity over every category excused the to-CIP column")
		}
	})

	t.Run("a shared cell outside its categories is uncovered", func(t *testing.T) {
		// Both kinds stay covered, so only the category leaves the
		// transfers/out cells uncovered.
		narrow := p76
		narrow.Categories = []structure.KindCategory{in,
			{Kind: vocab.KindTransferOut, Category: "transfers/out-to-cip"}}
		o, err := structure.Peers(facts, flows, td, []structure.Identity{narrow}, structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(o.Findings, "\n"), "category=transfers/out") ||
			!strings.Contains(strings.Join(o.Findings, "\n"), "no identity says they are one figure") {
			t.Fatalf("a transfers/out cell both print was not named:\n  %s", strings.Join(o.Findings, "\n  "))
		}
	})

	t.Run("a category no shared cell bears is named", func(t *testing.T) {
		wide := p76
		wide.Categories = append([]structure.KindCategory{{Kind: vocab.KindTransferOut, Category: "transfers/out-to-cip"}},
			p76.Categories...)
		o, err := structure.Peers(facts, flows, td, []structure.Identity{wide}, structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(o.Findings, "\n"), `names category "transfers/out-to-cip"`) {
			t.Fatalf("a category p76 never prints was accepted:\n  %s", strings.Join(o.Findings, "\n  "))
		}
	})

	t.Run("categories on a level with no category axis are refused", func(t *testing.T) {
		twin := allCutNamed(t, structure.CutFundBalanceRevenues)
		twin.Name = "twin"
		totals := structure.Identity{Name: "totals", A: structure.CutFundBalanceRevenues, B: twin.Name,
			Kinds: []vocab.Kind{vocab.KindRevenue}, Categories: []structure.KindCategory{{Kind: vocab.KindRevenue, Category: "revenues"}},
			Reason: "a plant"}
		err := structure.ValidateIdentities(append(structure.AllCuts(), twin), []structure.Identity{totals})
		if err == nil || !strings.Contains(err.Error(), "no category axis") {
			t.Fatalf("ValidateIdentities = %v, want categories refused at a level without them", err)
		}
	})

	t.Run("a category named twice, empty, for a kind not covered or leaving a kind out is refused", func(t *testing.T) {
		for _, cats := range [][]structure.KindCategory{
			{in, out, out},
			{in, {Kind: vocab.KindTransferOut}},
			{in, out, {Kind: vocab.KindRevenue, Category: "revenues"}},
			{in},
		} {
			bad := p76
			bad.Categories = cats
			if err := structure.ValidateIdentities(structure.AllCuts(), []structure.Identity{bad}); err == nil {
				t.Errorf("categories %v were accepted", cats)
			}
		}
	})
}

// TestAnIdentityCoversAKindOnlyUnderItsOwnCategory: p76's identity with
// pp.186-209 covers transfers in under transfers/in and transfers out under
// transfers/out. A transfer out both sides map under transfers/in is a
// mis-mapped column, an overlap the identity does not cover.
func TestAnIdentityCoversAKindOnlyUnderItsOwnCategory(t *testing.T) {
	facts := committedFacts(t)
	flows, td := allCutNamed(t, structure.CutFundBalanceFlows), allCutNamed(t, "transfers-detail")
	identities := structure.BudgetBookIdentities()
	o, err := structure.Peers(facts, flows, td, identities, structure.BudgetBookExceptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Findings) != 0 {
		t.Fatalf("findings under the declared pairs:\n  %s", strings.Join(o.Findings, "\n  "))
	}
	var key structure.Key
	for _, sh := range o.Shared {
		if sh.Kind == vocab.KindTransferOut && sh.Identity == "a-fund-balance-transfer-is-p76s" {
			key = sh.Key
			break
		}
	}
	if key == (structure.Key{}) {
		t.Fatal("no transfer out both sides print under p76's identity")
	}
	// relabel files every named side's transfer out at key under transfers/in,
	// zeroing it too when zero is set. Each fact counts toward exactly one
	// side: one both sides admit would satisfy both counts alone, so it is
	// refused rather than counted.
	relabel := func(key structure.Key, zero bool, sides ...structure.Cut) ([]fact.Fact, error) {
		moved := slices.Clone(facts)
		n := make([]int, len(sides))
		for i := range moved {
			f := &moved[i]
			if f.Kind != vocab.KindTransferOut || structure.KeyOf(f, key.Level) != key {
				continue
			}
			side := -1
			for j, c := range sides {
				if !c.Admits(f) {
					continue
				}
				if side >= 0 {
					return nil, fmt.Errorf("%s is admitted by both %q and %q at %s", f.ID, sides[side].Name, c.Name, key)
				}
				side = j
			}
			if side >= 0 {
				n[side]++
				f.Category = "transfers/in"
				if zero {
					f.AmountCents = 0
				}
			}
		}
		for j, c := range sides {
			if n[j] == 0 {
				return nil, fmt.Errorf("relabelled no fact of %q at %s", c.Name, key)
			}
		}
		return moved, nil
	}
	// relabelled is relabel, or the test fails.
	relabelled := func(t *testing.T, key structure.Key, zero bool, sides ...structure.Cut) []fact.Fact {
		t.Helper()
		moved, err := relabel(key, zero, sides...)
		if err != nil {
			t.Fatal(err)
		}
		return moved
	}

	// The helper's own premise: the same cut on both sides admits every fact
	// at key on both, and is refused rather than counted for each.
	t.Run("relabel refuses a fact both sides admit", func(t *testing.T) {
		_, err := relabel(key, false, flows, flows)
		if err == nil || !strings.Contains(err.Error(), "is admitted by both") {
			t.Fatalf("relabel(flows, flows) = %v, want a fact refused as admitted by both sides", err)
		}
	})
	misfiled := func(f string) bool {
		return strings.Contains(f, "category=transfers/in") && strings.Contains(f, "] transfer_out:") &&
			strings.Contains(f, `pairs "transfers/in" with [transfer_in]`)
	}

	t.Run("both sides under another kind's category", func(t *testing.T) {
		o, err := structure.Peers(relabelled(t, key, false, flows, td), flows, td, identities, structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 1 || !strings.Contains(o.Findings[0], "category=transfers/in") ||
			!strings.Contains(o.Findings[0], "] transfer_out:") ||
			!strings.Contains(o.Findings[0], "no identity says they are one figure") {
			t.Fatalf("findings = %q, want the transfer out under transfers/in named as uncovered", o.Findings)
		}
	})

	// A cell printed on one side only, under a category the identity pairs
	// with another kind, is a misfiled column, not a figure the other side
	// may simply not print.
	t.Run("one side under another kind's category", func(t *testing.T) {
		o, err := structure.Peers(relabelled(t, key, false, flows), flows, td, identities, structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(o.Findings, misfiled) {
			t.Fatalf("findings = %q, want the one-sided transfer out under transfers/in named as misfiled", o.Findings)
		}
	})

	// A misfile is a mapping error whatever the figure, "-" included.
	t.Run("one side's zero under another kind's category", func(t *testing.T) {
		o, err := structure.Peers(relabelled(t, key, true, flows), flows, td, identities, structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(o.Findings, misfiled) {
			t.Fatalf("findings = %q, want the one-sided zero transfer out under transfers/in named as misfiled", o.Findings)
		}
	})

	// On a basis only one cut prints, the other's having no cell says nothing.
	t.Run("one side under another kind's category on a basis the other does not print is still misfiled", func(t *testing.T) {
		i := slices.IndexFunc(facts, func(f fact.Fact) bool {
			return f.Kind == vocab.KindTransferOut && flows.Admits(&f) && !slices.Contains(td.Bases, f.Basis)
		})
		if i < 0 {
			t.Fatalf("no transfer out %q prints on a basis %q does not", flows.Name, td.Name)
		}
		o, err := structure.Peers(relabelled(t, structure.KeyOf(&facts[i], key.Level), false, flows), flows, td, identities,
			structure.BudgetBookExceptions())
		if err != nil {
			t.Fatal(err)
		}
		i = slices.IndexFunc(o.Findings, misfiled)
		if i < 0 {
			t.Fatalf("findings = %q, want the misfiled cell named on a basis %q does not print", o.Findings, td.Name)
		}
		if strings.Contains(o.Findings[i], "overlap") {
			t.Fatalf("finding %q calls it an overlap on a basis %q does not print", o.Findings[i], td.Name)
		}
	})
}

// TestAnAbsenceIsExcusedOnlyAtTheFigureItPins: pp.127-130 print no General
// Fund Transfers In, and the exceptions saying so pin what pp.186-209 print
// in its place; a figure that moves is no longer the one excused.
func TestAnAbsenceIsExcusedOnlyAtTheFigureItPins(t *testing.T) {
	facts := committedFacts(t)
	rd, flows := allCutNamed(t, structure.CutRevenueDetail), allCutNamed(t, structure.CutFundBalanceFlows)
	o, err := structure.Peers(facts, rd, flows, structure.BudgetBookIdentities(), structure.BudgetBookExceptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Findings) != 0 {
		t.Fatalf("findings over the committed store:\n  %s", strings.Join(o.Findings, "\n  "))
	}
	const name = "pp.127-130-print-no-general-fund-transfer-in-2024"
	if !slices.Contains(o.Excused, name) {
		t.Fatalf("excused %v, want %s among them", o.Excused, name)
	}

	moved := slices.Clone(facts)
	n := 0
	for i := range moved {
		f := &moved[i]
		if f.Scope == structure.ScopeFundBalancesByFund && f.FiscalYear == 2024 && f.Category == "transfers/in" &&
			f.Fund != nil && *f.Fund == 100 {
			f.AmountCents += 99900
			n++
		}
	}
	if n != 1 {
		t.Fatalf("moved %d facts, want p186's one General Fund Transfers In", n)
	}
	o, err = structure.Peers(moved, rd, flows, structure.BudgetBookIdentities(), structure.BudgetBookExceptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Findings) != 1 || !strings.Contains(o.Findings[0], "fund=100") ||
		!strings.Contains(o.Findings[0], "no exception declares the absence") {
		t.Fatalf("findings = %q, want the moved General Fund cell named", o.Findings)
	}
	if slices.Contains(o.Excused, name) {
		t.Errorf("%s is reported excusing an absence whose figure it no longer pins", name)
	}
}

// TestAPeerExceptionExcusesOnlyItsOwnPairAndOnlyWhole: an exception declared
// at the pair's level names the two peers it is between, and every cell it
// pins must meet an absence for it to count as excusing one.
func TestAPeerExceptionExcusesOnlyItsOwnPairAndOnlyWhole(t *testing.T) {
	facts := committedFacts(t)
	rd, flows := allCutNamed(t, structure.CutRevenueDetail), allCutNamed(t, structure.CutFundBalanceFlows)
	const name = "pp.127-130-print-no-general-fund-transfer-in-2024"
	plant := func(edit func(*structure.Exception)) []structure.Exception {
		var out []structure.Exception
		for _, e := range structure.BudgetBookExceptions() {
			if e.Name == name {
				e.Cells = slices.Clone(e.Cells)
				edit(&e)
			}
			out = append(out, e)
		}
		return out
	}

	t.Run("naming a third peer", func(t *testing.T) {
		o, err := structure.Peers(facts, rd, flows, structure.BudgetBookIdentities(),
			plant(func(e *structure.Exception) { e.Against = "transfers-detail" }))
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 1 || !strings.Contains(o.Findings[0], "FY2024 actual") ||
			slices.Contains(o.Excused, name) {
			t.Fatalf("findings %q, excused %v; want the FY2024 absence unexcused", o.Findings, o.Excused)
		}
	})

	t.Run("with a cell no absence meets", func(t *testing.T) {
		o, err := structure.Peers(facts, rd, flows, structure.BudgetBookIdentities(),
			plant(func(e *structure.Exception) {
				stale := e.Cells[0]
				stale.Coords = map[structure.Axis]string{structure.AxisFundGroup: "general", structure.AxisFund: "999",
					structure.AxisCategory: "transfers/in"}
				e.Cells = append(e.Cells, stale)
				e.Residual *= 2
			}))
		if err != nil {
			t.Fatal(err)
		}
		if len(o.Findings) != 0 || slices.Contains(o.Excused, name) {
			t.Fatalf("findings %q, excused %v; want the absence held and the exception not counted whole", o.Findings, o.Excused)
		}
	})
}

// TestAViewDeclinesAReadingOnlyInItsIdentitysCategories: a view taking p76's
// reading of pp.186-209's transfers still counts the Transfers Out to CIP
// column, which p76 never prints.
func TestAViewDeclinesAReadingOnlyInItsIdentitysCategories(t *testing.T) {
	facts := committedFacts(t)
	flows, td := allCutNamed(t, structure.CutFundBalanceFlows), allCutNamed(t, "transfers-detail")
	identities := structure.BudgetBookIdentities()
	v, err := structure.NewView("transfers", []structure.Cut{flows, td}, identities, nil,
		map[string]string{"a-fund-balance-transfer-is-p76s": td.Name})
	if err != nil {
		t.Fatalf("NewView: %v", err)
	}
	admitted := map[string]int{}
	for i := range facts {
		f := &facts[i]
		if flows.Admits(f) && f.Kind != vocab.KindFundBalance && v.Admits(f, identities) {
			admitted[f.Category]++
		}
	}
	if admitted["transfers/in"] != 0 || admitted["transfers/out"] != 0 || admitted["transfers/out-to-cip"] == 0 {
		t.Errorf("the view admits pp.186-209's %v; want only the to-CIP column, the reading p76 does not print", admitted)
	}
}

// TestAViewTakesTheOtherReadingWhereItsOwnIsDeclaredAbsent reads a view over
// pp.186-209's flows beside pp.127-140, taking pp.127-140's reading of a
// fund's Transfers In. pp.127-130 print no General Fund Transfers In, which
// the exceptions declare, so the view counts pp.186-209's figure there and
// declines every transfer in pp.127-140 do print.
func TestAViewTakesTheOtherReadingWhereItsOwnIsDeclaredAbsent(t *testing.T) {
	facts := committedFacts(t)
	flows, rd := allCutNamed(t, structure.CutFundBalanceFlows), allCutNamed(t, structure.CutRevenueDetail)
	identities := structure.BudgetBookIdentities()
	readings := map[string]string{"a-fund-balance-transfer-in-is-the-revenue-schedules": rd.Name}
	admittedBy := func(exceptions []structure.Exception) map[string]int64 {
		t.Helper()
		v, err := structure.NewView("revenues", []structure.Cut{flows, rd}, identities, exceptions, readings)
		if err != nil {
			t.Fatalf("NewView: %v", err)
		}
		got := map[string]int64{}
		for i := range facts {
			f := &facts[i]
			if f.Kind == vocab.KindTransferIn && flows.Admits(f) && v.Admits(f, identities) {
				got[fmt.Sprintf("fund %d FY%d %s", *f.Fund, f.FiscalYear, f.Basis)] = f.AmountCents
			}
		}
		return got
	}
	want := map[string]int64{
		"fund 100 FY2024 actual":  73_745_500,
		"fund 100 FY2025 revised": 91_420_600,
		"fund 100 FY2026 adopted": 48_040_000,
		"fund 100 FY2027 adopted": 48_673_500,
	}
	// Fund 101's FY2024-25 Transfers In: pp.127-140 print no General Fund
	// CIP Reserves section that year.
	want["fund 101 FY2025 revised"] = 412_562_700
	got := admittedBy(structure.BudgetBookExceptions())
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("admitted pp.186-209 transfers in (-want +got):\n%s", diff)
	}
	// With no exception declaring the absence, the view declines every one.
	if none := admittedBy(nil); len(none) != 0 {
		t.Errorf("with no declared absence the view admits %v; it should decline every pp.186-209 transfer in", none)
	}
	// And the same pin with the reading's side printed admits nothing: that
	// is a disagreement between two figures, not an absence.
	var printedBoth []structure.Exception
	for _, e := range structure.BudgetBookExceptions() {
		if e.Name != "pp.127-130-print-no-general-fund-transfer-in-2024" || e.Against != flows.Name {
			continue
		}
		e.Cells = slices.Clone(e.Cells)
		for j := range e.Cells {
			e.Cells[j].Cut = structure.Sum{Cents: 1, Present: true}
		}
		printedBoth = append(printedBoth, e)
	}
	if len(printedBoth) != 1 {
		t.Fatalf("found %d FY2024 absence exceptions on the pair, want 1", len(printedBoth))
	}
	if got := admittedBy(printedBoth); len(got) != 0 {
		t.Errorf("a pin printing both sides admits %v; want nothing", got)
	}
	// Nor does a pin declaring both sides absent: nothing says the other
	// reading's figure is the one the view is missing.
	for j := range printedBoth[0].Cells {
		printedBoth[0].Cells[j].Cut = structure.Sum{}
		printedBoth[0].Cells[j].Against = structure.Sum{}
	}
	if got := admittedBy(printedBoth); len(got) != 0 {
		t.Errorf("a pin absent on both sides admits %v; want nothing", got)
	}
}

// TestReadingsOfOneFigureMustNameOneCut joins pp.127-140, p76 and
// pp.186-209, each pair by an identity over a fund's Transfers In. Readings
// that decline each cut in turn leave no reading of the figure, and are
// refused; readings that agree on one cut count each transfer in once.
func TestReadingsOfOneFigureMustNameOneCut(t *testing.T) {
	facts := committedFacts(t)
	identities := structure.BudgetBookIdentities()
	rd, td := allCutNamed(t, structure.CutRevenueDetail), allCutNamed(t, "transfers-detail")
	flows := allCutNamed(t, structure.CutFundBalanceFlows)
	cuts := []structure.Cut{rd, td, flows}
	const (
		rdTD    = "a-transfer-in-is-printed-at-both-ends"
		flowsRD = "a-fund-balance-transfer-in-is-the-revenue-schedules"
		flowsTD = "a-fund-balance-transfer-is-p76s"
	)

	// rd~td declines rd, td~flows declines td, flows~rd declines flows.
	cyclic := map[string]string{rdTD: td.Name, flowsTD: flows.Name, flowsRD: rd.Name}
	_, err := structure.NewView("cycle", cuts, identities, structure.BudgetBookExceptions(), cyclic)
	if err == nil || !strings.Contains(err.Error(), "declines every reading of transfer_in") {
		t.Fatalf("NewView over cyclic readings = %v, want refused: every reading of a transfer in is declined", err)
	}

	// Without the identity joining pp.127-140 and pp.186-209, readings that
	// both decline p76 keep two readings of one transfer in.
	var withoutFlowsRD []structure.Identity
	for _, id := range identities {
		if id.Name != flowsRD {
			withoutFlowsRD = append(withoutFlowsRD, id)
		}
	}
	_, err = structure.NewView("twice", cuts, withoutFlowsRD, nil,
		map[string]string{rdTD: rd.Name, flowsTD: flows.Name})
	if err == nil || !strings.Contains(err.Error(), "keeps 2 readings of transfer_in under transfers/in") {
		t.Errorf("NewView keeping pp.127-140's and pp.186-209's readings = %v, want refused", err)
	}

	agreed := map[string]string{rdTD: rd.Name, flowsRD: rd.Name, flowsTD: flows.Name}
	v, err := structure.NewView("agreed", cuts, identities, structure.BudgetBookExceptions(), agreed)
	if err != nil {
		t.Fatalf("NewView over readings naming pp.127-140 = %v, want admitted", err)
	}
	type cell struct {
		fund  int
		year  int
		basis vocab.Basis
	}
	by := map[cell][]string{}
	for i := range facts {
		f := &facts[i]
		if f.Kind != vocab.KindTransferIn || f.Category != "transfers/in" || f.Fund == nil || !v.Admits(f, identities) {
			continue
		}
		c := cell{*f.Fund, f.FiscalYear, f.Basis}
		if !slices.Contains(by[c], f.Scope) {
			by[c] = append(by[c], f.Scope)
		}
	}
	if len(by) == 0 {
		t.Fatal("the view admits no transfer in at all")
	}
	for c, scopes := range by {
		if len(scopes) > 1 {
			t.Errorf("fund %d FY%d %s's transfers in are counted from %d schedules, %v", c.fund, c.year, c.basis,
				len(scopes), scopes)
		}
	}
}

// TestOnlyAnExceptionOnThePeerPairExcusesAPeerAbsence: pp.127-130's missing
// General Fund Transfers In, declared against the spine at the fund-group
// level, is not an absence between pp.127-130 and p76; the same pin declared
// on that pair, at its level, is.
func TestOnlyAnExceptionOnThePeerPairExcusesAPeerAbsence(t *testing.T) {
	facts := committedFacts(t)
	rd, td := allCutNamed(t, structure.CutRevenueDetail), allCutNamed(t, "transfers-detail")
	var foreign, own []structure.Exception
	for _, e := range structure.BudgetBookExceptions() {
		if !strings.HasPrefix(e.Name, "pp.127-130-print-no-general-fund-transfer-in-") || e.Against != structure.CutSpine {
			continue
		}
		foreign = append(foreign, e)
		e.Against, e.At = td.Name, structure.LevelFundByCategory
		e.Cells = slices.Clone(e.Cells)
		for i := range e.Cells {
			e.Cells[i].Coords = map[structure.Axis]string{structure.AxisFundGroup: "general",
				structure.AxisFund: "100", structure.AxisCategory: "transfers/in"}
		}
		own = append(own, e)
	}
	if len(foreign) != 2 {
		t.Fatalf("%d spine exceptions for the General Fund transfer in, want 2", len(foreign))
	}

	o, err := structure.Peers(facts, rd, td, structure.BudgetBookIdentities(), foreign)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Findings) != 2 || len(o.Excused) != 0 {
		t.Fatalf("findings %q, excused %v; want both General Fund cells unexcused by a spine exception",
			o.Findings, o.Excused)
	}

	o, err = structure.Peers(facts, rd, td, structure.BudgetBookIdentities(), own)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Findings) != 0 || len(o.Excused) != 2 {
		t.Fatalf("findings %q, excused %v; want both cells excused by the pair's own exceptions",
			o.Findings, o.Excused)
	}
}
