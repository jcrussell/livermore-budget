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
		if !strings.Contains(strings.Join(got.Findings, "\n"), "-$28373590.00") {
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
		{"one part", func(s *structure.Split) { s.Parts = s.Parts[:1] }, "is a containment"},
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
