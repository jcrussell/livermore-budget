package mapping

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// TestKindsIsTheClosedSet pins the vocabulary itself, because it is now
// spelled in two packages. internal/registry must refuse a data/taxonomy.yaml
// kinds: member that is not one of these, and it cannot import this package
// (every test file here is package mapping and one of them imports registry,
// so the edge closes a cycle in the TEST build). Its own list is pinned
// against Kinds() from a package that may import both.
func TestKindsIsTheClosedSet(t *testing.T) {
	got := Kinds()
	want := []Kind{KindRevenue, KindExpenditure, KindTransferIn, KindTransferOut, KindFundBalance}
	if !slices.Equal(got, want) {
		t.Errorf("Kinds() = %v, want %v", got, want)
	}
	for _, k := range got {
		if !k.valid() {
			t.Errorf("Kinds() offers %q, which valid() rejects", k)
		}
	}
	for _, k := range []Kind{"", "banana", "transfer", "Revenue"} {
		if k.valid() {
			t.Errorf("valid(%q) = true, want false", k)
		}
	}
}

// TestKindsIsACopy stops a caller moving the vocabulary out from under the
// parser. Kinds() is exported precisely so another package can iterate it, and
// a shared backing array would let a sort there change what valid() accepts
// here.
func TestKindsIsACopy(t *testing.T) {
	got := Kinds()
	got[0] = "banana"
	if !KindRevenue.valid() {
		t.Fatal("mutating the slice Kinds() returned changed what valid() accepts")
	}
	if again := Kinds(); again[0] != KindRevenue {
		t.Errorf("Kinds()[0] = %q after a caller wrote to an earlier result, want %q",
			again[0], KindRevenue)
	}
}

// TestTheMessagesThatSayFiveStillMeanFive pins the count parse errors state in
// prose. Adding a sixth kind must redden this rather than leave "is not one of
// the five" quietly wrong on a page a reader is trying to fix.
//
// THE NUMBER OF SUCH MESSAGES IS MEASURED RATHER THAN NAMED, and that is the
// second version of this test. The first said "three messages (rows,
// total_row_kinds, counterpart)"; rollups[].kinds made it four one commit after
// it was written, and nothing went red -- a count in a failure message a reader
// acts on, gone stale exactly the way the messages it is about would. Counting
// them here cannot rot, and it costs one file read.
func TestTheMessagesThatSayFiveStillMeanFive(t *testing.T) {
	src, err := os.ReadFile("parse.go")
	if err != nil {
		t.Fatalf("read parse.go: %v", err)
	}
	n := strings.Count(string(src), "not one of the five")
	if n == 0 {
		t.Fatal("no parse error says \"not one of the five\"; either they were reworded " +
			"or this test is reading the wrong file")
	}
	if len(Kinds()) != 5 {
		t.Errorf("len(Kinds()) = %d, want 5; parse.go says \"not one of the five\" in "+
			"%d message(s) and they are now wrong", len(Kinds()), n)
	}
}

// TestKindListSpellsEveryKind covers the one message that enumerates the set
// rather than counting it. It used to be a hand-typed literal.
func TestKindListSpellsEveryKind(t *testing.T) {
	got := kindList()
	for _, k := range Kinds() {
		if !strings.Contains(got, string(k)) {
			t.Errorf("kindList() = %q, which does not name %q", got, k)
		}
	}
	if want := "revenue, expenditure, transfer_in, transfer_out, fund_balance"; got != want {
		t.Errorf("kindList() = %q, want %q", got, want)
	}
}

// TestBasisListSpellsEveryBasis covers the basis refusal's enumeration, which
// is read off [Bases] so a sixth basis cannot be accepted and left unnamed.
func TestBasisListSpellsEveryBasis(t *testing.T) {
	got := basisList()
	for _, b := range Bases() {
		if !strings.Contains(got, string(b)) {
			t.Errorf("basisList() = %q, which does not name %q", got, b)
		}
	}
	if want := "adopted, revised, actual, audited, projected"; got != want {
		t.Errorf("basisList() = %q, want %q", got, want)
	}
}
