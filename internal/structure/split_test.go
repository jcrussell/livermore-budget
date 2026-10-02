package structure_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

func splitNamed(t *testing.T, name string) structure.Split {
	t.Helper()
	for _, s := range structure.BudgetBookSplits() {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no declared split named %q", name)
	return structure.Split{}
}

const transferOutSplit = "a-transfer-out-is-p76-or-to-the-cip"

// TestTheSpineTransferOutIsP76PlusTheCIP holds pp.66-67's TRANSFER OUT to
// p76's list plus p222's transfers to the CIP, by fund group, and shows that
// either part alone leaves the spine short by exactly the other.
func TestTheSpineTransferOutIsP76PlusTheCIP(t *testing.T) {
	facts := committedFacts(t)
	sp := splitNamed(t, transferOutSplit)

	got, err := structure.HoldSplit(facts, structure.AllCuts(), sp)
	if err != nil {
		t.Fatalf("HoldSplit: %v", err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("the split does not hold:\n  %s", strings.Join(got.Findings, "\n  "))
	}
	// Six fund groups over the spine's two adopted columns.
	if diff := cmp.Diff(12, got.Subjects); diff != "" {
		t.Errorf("cells compared (-want +got):\n%s", diff)
	}
	t.Logf("%s: %d cells, %d one-sided", got.Name(), got.Subjects, got.OneSided)

	t.Run("without p222 the spine is short by the transfers to the CIP", func(t *testing.T) {
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			if facts[i].Scope != "cip-funding-sources" {
				kept = append(kept, facts[i])
			}
		}
		got, err := structure.HoldSplit(kept, structure.AllCuts(), sp)
		if err != nil {
			t.Fatalf("HoldSplit: %v", err)
		}
		// capital, enterprise, internal service and special revenue in both
		// years: the four groups p0198-p0205 print a to-CIP figure for.
		if len(got.Findings) != 8 {
			t.Fatalf("%d findings, want 8:\n  %s", len(got.Findings), strings.Join(got.Findings, "\n  "))
		}
		if !strings.Contains(strings.Join(got.Findings, "\n"), "-$28,373,590.00") {
			t.Errorf("no finding names FY2026 capital's 28,373,590 to the CIP:\n  %s",
				strings.Join(got.Findings, "\n  "))
		}
	})

	t.Run("a transfer to the CIP moved between groups is named in both", func(t *testing.T) {
		moved := append([]fact.Fact(nil), facts...)
		n := 0
		for i := range moved {
			f := &moved[i]
			if f.Scope == "cip-funding-sources" && f.Kind == mapping.KindTransferOut &&
				f.FundGroup == "capital" && f.FiscalYear == 2026 && f.AmountCents > 0 {
				f.FundGroup = "special-revenue"
				n++
				break
			}
		}
		if n != 1 {
			t.Fatal("no capital transfer to the CIP in FY2026 to move; the fixture no longer covers this")
		}
		got, err := structure.HoldSplit(moved, structure.AllCuts(), sp)
		if err != nil {
			t.Fatalf("HoldSplit: %v", err)
		}
		joined := strings.Join(got.Findings, "\n")
		for _, want := range []string{"fund_group=capital", "fund_group=special-revenue"} {
			if !strings.Contains(joined, want) {
				t.Errorf("no finding at %s:\n  %s", want, strings.Join(got.Findings, "\n  "))
			}
		}
	})
}

// TestAPairASplitRelatesDropsItsKinds: the lattice compares p76 to the spine
// on transfers in only, and p222's out-legs against the spine not at all.
func TestAPairASplitRelatesDropsItsKinds(t *testing.T) {
	splits := structure.BudgetBookSplits()
	for _, tc := range []struct {
		a, b  string
		held  bool
		kinds []mapping.Kind
	}{
		{"transfers-detail", "spine", false, []mapping.Kind{mapping.KindTransferIn}},
		{"spine", "cip-transfers-out", true, nil},
		{"revenue-detail", "cip-transfers-out", false, []mapping.Kind{mapping.KindRevenue, mapping.KindTransferIn}},
	} {
		a, _, held := structure.SplitPair(cutNamed(t, tc.a), cutNamed(t, tc.b), splits)
		if held != tc.held {
			t.Errorf("%s/%s: held = %v, want %v", tc.a, tc.b, held, tc.held)
		}
		if tc.a == "spine" {
			continue
		}
		if diff := cmp.Diff(tc.kinds, a.Kinds); diff != "" {
			t.Errorf("%s/%s: %s's kinds (-want +got):\n%s", tc.a, tc.b, tc.a, diff)
		}
	}
}

// TestASplitIsRefusedWhenItsSidesCannotBearIt mutates the declared split, one
// arm at a time.
func TestASplitIsRefusedWhenItsSidesCannotBearIt(t *testing.T) {
	cuts := structure.AllCuts()
	if err := structure.ValidateSplits(cuts, structure.BudgetBookSplits()); err != nil {
		t.Fatalf("the declared splits do not validate: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*structure.Split)
		want   string
	}{
		{"an undeclared whole", func(s *structure.Split) { s.Whole = "spines" }, `whole "spines"`},
		{"one part at the grain the lattice already compares it", func(s *structure.Split) {
			s.Parts, s.At = s.Parts[:1], structure.LevelFundGroupByCategory
		}, "is a containment"},
		{"a part twice", func(s *structure.Split) { s.Parts = []string{s.Parts[0], s.Parts[0]} }, "twice"},
		{"no kind", func(s *structure.Split) { s.Kinds = nil }, "names no kind"},
		{"a kind a part does not print", func(s *structure.Split) {
			s.Kinds = []mapping.Kind{mapping.KindRevenue}
		}, "does not print revenue"},
		{"a level a side does not refine", func(s *structure.Split) {
			s.At = structure.LevelDepartment
		}, "does not refine"},
		{"a side outside the reference", func(s *structure.Split) {
			s.Parts = []string{s.Parts[0], "cip-funds"}
		}, "outside the reference"},
		{"a side with a fund-group footprint", func(s *structure.Split) {
			s.Parts = []string{s.Parts[0], "general-fund-departments"}
		}, "covers fund groups"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := splitNamed(t, transferOutSplit)
			s.Parts = append([]string(nil), s.Parts...)
			tc.mutate(&s)
			err := structure.ValidateSplits(cuts, []structure.Split{s})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateSplits = %v, want a refusal saying %q", err, tc.want)
			}
		})
	}
}

// TestAOnePartSplitTheLatticeCannotPlaceIsRefused: one part is a split only
// where At drops an axis the pair meets on, and a pair the lattice gives no
// meet has none to drop.
func TestAOnePartSplitTheLatticeCannotPlaceIsRefused(t *testing.T) {
	cuts := append(structure.AllCuts(), structure.Cut{Name: "by-department",
		Level: structure.LevelDepartment, Kinds: []mapping.Kind{mapping.KindTransferOut}})
	s := splitNamed(t, transferOutSplit)
	s.Parts = []string{"by-department"}
	err := structure.ValidateSplits(cuts, []structure.Split{s})
	if err == nil || !strings.Contains(err.Error(), "share no common coarsening") {
		t.Fatalf("ValidateSplits = %v, want the meet's refusal", err)
	}
}

// TestACutOutsideTheReferenceIsHeldToItsFunds: p222's CIP funds carry no
// fact in any other cut, and the claim goes red the moment one does.
func TestACutOutsideTheReferenceIsHeldToItsFunds(t *testing.T) {
	facts := committedFacts(t)
	cuts := structure.AllCuts()
	if got := structure.ValidateOutside(facts, cuts); len(got) != 0 {
		t.Fatalf("the committed store refutes a declared outside cut:\n  %s", strings.Join(got, "\n  "))
	}
	cipFund := func(fs []fact.Fact) *fact.Fact {
		for i := range fs {
			if fs[i].Scope == "cip-funding-sources" && fs[i].Kind == mapping.KindRevenue {
				return &fs[i]
			}
		}
		t.Fatal("no CIP grant fact; the fixture no longer covers this")
		return nil
	}

	t.Run("a fund another cut carries", func(t *testing.T) {
		moved := append([]fact.Fact(nil), facts...)
		general := 100
		cipFund(moved).Fund = &general
		got := structure.ValidateOutside(moved, cuts)
		if len(got) != 1 || !strings.Contains(got[0], "fund 100") || !strings.Contains(got[0], "revenue-detail") {
			t.Fatalf("ValidateOutside = %q, want one line naming fund 100 and a cut that carries it", got)
		}
	})

	t.Run("a fact with no fund", func(t *testing.T) {
		moved := append([]fact.Fact(nil), facts...)
		f := cipFund(moved)
		f.Fund = nil
		got := structure.ValidateOutside(moved, cuts)
		if len(got) != 1 || !strings.Contains(got[0], f.ID) || !strings.Contains(got[0], "carries no fund") {
			t.Fatalf("ValidateOutside = %q, want one line naming %s", got, f.ID)
		}
	})
}

const fundTransferOutSplit = "a-fund-transfers-out-or-to-the-cip"

// TestTheSpineTransferOutIsAFundsTwoTransferOutColumns holds pp.66-67's
// TRANSFER OUT to pp.186-209's Transfers Out plus Transfers Out to CIP, by
// fund group: one cut, two columns the spine prints as one line, which is a
// split of one part because the category axis names the money differently.
func TestTheSpineTransferOutIsAFundsTwoTransferOutColumns(t *testing.T) {
	facts := committedFacts(t)
	sp := splitNamed(t, fundTransferOutSplit)
	if len(sp.Parts) != 1 || sp.At != structure.LevelFundGroup {
		t.Fatalf("split %q is %v at %q; want the one fund-balances cut at the fund group", sp.Name, sp.Parts, sp.At)
	}

	got, err := structure.HoldSplit(facts, structure.AllCuts(), sp)
	if err != nil {
		t.Fatalf("HoldSplit: %v", err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("the split does not hold:\n  %s", strings.Join(got.Findings, "\n  "))
	}
	t.Logf("%s: %d cells, %d one-sided", got.Name(), got.Subjects, got.OneSided)

	t.Run("without the Transfers Out to CIP column the spine is short by it", func(t *testing.T) {
		kept := make([]fact.Fact, 0, len(facts))
		for i := range facts {
			if facts[i].Scope != structure.ScopeFundBalancesByFund || facts[i].Category != "transfers/out-to-cip" {
				kept = append(kept, facts[i])
			}
		}
		got, err := structure.HoldSplit(kept, structure.AllCuts(), sp)
		if err != nil {
			t.Fatalf("HoldSplit: %v", err)
		}
		if !strings.Contains(strings.Join(got.Findings, "\n"), "-$28,373,590.00") {
			t.Errorf("no finding names FY2026 capital's 28,373,590 to the CIP:\n  %s",
				strings.Join(got.Findings, "\n  "))
		}
	})
}

// TestTheCIPFundsBlockIsP222sMoney re-measures the residue's reason: the
// pp.186-209 rows no cut selects are p222's funds, fund by fund, and a cut
// selecting them is refuted by the cut that declares their money outside.
func TestTheCIPFundsBlockIsP222sMoney(t *testing.T) {
	facts := committedFacts(t)
	residue := structure.BudgetBookResidue()
	type cell struct {
		year int
		fund string
	}
	block := map[string]map[cell]int64{}
	p222 := map[string]map[cell]int64{}
	add := func(m map[string]map[cell]int64, line string, f *fact.Fact) {
		if m[line] == nil {
			m[line] = map[cell]int64{}
		}
		m[line][cell{f.FiscalYear, fact.FundString(f.Fund)}] += f.AmountCents
	}
	funds := map[string]bool{}
	cip := allCutNamed(t, "cip-funds")
	for i := range facts {
		f := &facts[i]
		if cip.Admits(f) {
			add(p222, string(f.Kind), f)
			continue
		}
		if f.Scope != structure.ScopeFundBalancesByFund {
			continue
		}
		for _, r := range residue {
			if r.Matches(f) {
				add(block, f.Category, f)
				funds[fact.FundString(f.Fund)] = true
				break
			}
		}
	}
	if len(funds) != 35 {
		t.Errorf("the residue carries %d funds, want the block's 35", len(funds))
	}
	total := func(m map[cell]int64, year int) int64 {
		var s int64
		for c, v := range m {
			if c.year == year {
				s += v
			}
		}
		return s
	}
	for _, year := range []int{2025, 2026, 2027} {
		for c, v := range block["transfers/in"] {
			if c.year == year && p222["transfer_in"][c] != v {
				t.Errorf("FY%d fund %s: pp.186-209 Transfers In %d, p222 %d", year, c.fund, v, p222["transfer_in"][c])
			}
		}
		for c, v := range block["revenues"] {
			if c.year == year && p222["revenue"][c] != v {
				t.Errorf("FY%d fund %s: pp.186-209 Revenues %d, p222 %d", year, c.fund, v, p222["revenue"][c])
			}
		}
		drawn := total(block["fund-balance/beginning"], year) - total(block["fund-balance/ending"], year)
		t.Logf("FY%d: transfers in %d, revenues %d, balance drawn %d", year,
			total(block["transfers/in"], year), total(block["revenues"], year), drawn)
		if drawn != total(p222["fund_balance"], year) {
			t.Errorf("FY%d: the block draws %d of balance and p222 prints %d", year, drawn, total(p222["fund_balance"], year))
		}
	}
	got := [][]int64{}
	for _, year := range []int{2025, 2026, 2027} {
		got = append(got, []int64{total(block["transfers/in"], year), total(block["revenues"], year)})
	}
	want := [][]int64{{9296875200, 1133975100}, {3808673700, 781479900}, {5076225100, 822662000}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the residue's reason quotes these (-want +got):\n%s", diff)
	}
	var drawn []int64
	for _, year := range []int{2025, 2026, 2027} {
		drawn = append(drawn, total(block["fund-balance/beginning"], year)-total(block["fund-balance/ending"], year))
	}
	if diff := cmp.Diff([]int64{35691300, 118308700, 0}, drawn); diff != "" {
		t.Errorf("the balance drawn, quoted by the reason (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]int64{196364700, 314300},
		[]int64{total(block["fund-balance/beginning"], 2024), total(block["expenses"], 2024)}); diff != "" {
		t.Errorf("FY2023-24's opening balance and expenses, quoted by the reason (-want +got):\n%s", diff)
	}

	t.Run("a fund-balances cut selecting the block is refuted by cip-funds", func(t *testing.T) {
		cuts := structure.AllCuts()
		for i := range cuts {
			if cuts[i].Name == structure.CutFundBalanceFlows {
				cuts[i].Rules = nil
			}
		}
		got := structure.ValidateOutside(facts, cuts)
		if len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), structure.CutFundBalanceFlows) {
			t.Fatalf("ValidateOutside = %q, want the CIP funds named as carried by %s", got, structure.CutFundBalanceFlows)
		}
	})
}
