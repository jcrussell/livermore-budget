package vocab

import (
	"slices"
	"strings"
	"testing"
)

// TestKindsIsTheClosedSet pins the vocabulary itself: every package that
// refuses a kind, from the rule parser to the registry, refuses by Valid, so
// the set and the method must agree on exactly these five.
func TestKindsIsTheClosedSet(t *testing.T) {
	got := Kinds()
	want := []Kind{KindRevenue, KindExpenditure, KindTransferIn, KindTransferOut, KindFundBalance}
	if !slices.Equal(got, want) {
		t.Errorf("Kinds() = %v, want %v", got, want)
	}
	for _, k := range got {
		if !k.Valid() {
			t.Errorf("Kinds() offers %q, which Valid() rejects", k)
		}
	}
	for _, k := range []Kind{"", "banana", "transfer", "Revenue"} {
		if k.Valid() {
			t.Errorf("Valid(%q) = true, want false", k)
		}
	}
}

// TestKindsIsACopy stops a caller moving the vocabulary out from under every
// other reader. Kinds() is exported precisely so another package can iterate
// it, and a shared backing array would let a sort there change what Valid()
// accepts everywhere.
func TestKindsIsACopy(t *testing.T) {
	got := Kinds()
	got[0] = "banana"
	if !KindRevenue.Valid() {
		t.Fatal("mutating the slice Kinds() returned changed what Valid() accepts")
	}
	if again := Kinds(); again[0] != KindRevenue {
		t.Errorf("Kinds()[0] = %q after a caller wrote to an earlier result, want %q",
			again[0], KindRevenue)
	}
}

// TestKindListSpellsEveryKind covers the one message that enumerates the set
// rather than counting it.
func TestKindListSpellsEveryKind(t *testing.T) {
	got := KindList()
	for _, k := range Kinds() {
		if !strings.Contains(got, string(k)) {
			t.Errorf("KindList() = %q, which does not name %q", got, k)
		}
	}
	if want := "revenue, expenditure, transfer_in, transfer_out, fund_balance"; got != want {
		t.Errorf("KindList() = %q, want %q", got, want)
	}
}

// TestBasesIsTheClosedSet pins the bases the same way as the kinds.
func TestBasesIsTheClosedSet(t *testing.T) {
	got := Bases()
	want := []Basis{BasisAdopted, BasisRevised, BasisActual, BasisAudited, BasisProjected}
	if !slices.Equal(got, want) {
		t.Errorf("Bases() = %v, want %v", got, want)
	}
	for _, b := range got {
		if !b.Valid() {
			t.Errorf("Bases() offers %q, which Valid() rejects", b)
		}
	}
	for _, b := range []Basis{"", "budget", "Adopted"} {
		if b.Valid() {
			t.Errorf("Valid(%q) = true, want false", b)
		}
	}
	got[0] = "banana"
	if !BasisAdopted.Valid() {
		t.Fatal("mutating the slice Bases() returned changed what Valid() accepts")
	}
}

// TestBasisListSpellsEveryBasis covers the basis refusal's enumeration, which
// is read off [Bases] so a sixth basis cannot be accepted and left unnamed.
func TestBasisListSpellsEveryBasis(t *testing.T) {
	got := BasisList()
	for _, b := range Bases() {
		if !strings.Contains(got, string(b)) {
			t.Errorf("BasisList() = %q, which does not name %q", got, b)
		}
	}
	if want := "adopted, revised, actual, audited, projected"; got != want {
		t.Errorf("BasisList() = %q, want %q", got, want)
	}
}

// TestSignsIsTheClosedSet pins the signs, and that the empty string is the one
// value outside the set Valid accepts: a rule file leaves it for positive.
func TestSignsIsTheClosedSet(t *testing.T) {
	got := Signs()
	want := []Sign{SignPositive, SignContra, SignNetted}
	if !slices.Equal(got, want) {
		t.Errorf("Signs() = %v, want %v", got, want)
	}
	for _, s := range append(got, "") {
		if !s.Valid() {
			t.Errorf("Valid(%q) = false, want true", s)
		}
	}
	for _, s := range []Sign{"negative", "Contra", "net"} {
		if s.Valid() {
			t.Errorf("Valid(%q) = true, want false", s)
		}
	}
	got[0] = "banana"
	if !SignPositive.Valid() {
		t.Fatal("mutating the slice Signs() returned changed what Valid() accepts")
	}
}
