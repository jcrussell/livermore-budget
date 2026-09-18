package structure_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// spineScoped is the spine's view as a document names it: the scope every
// [structure.Cut] with Reference set reads.
func spineScoped(t *testing.T) structure.Scoped {
	t.Helper()
	for _, c := range structure.AllCuts() {
		if c.Reference {
			return structure.Scoped{Name: "spine-doc", Scopes: []string{c.Scope}}
		}
	}
	t.Fatal("no declared cut is the reference, so there is no spine to build a view of")
	return structure.Scoped{}
}

func TestBuildRefusesAViewThatAdmitsNothing(t *testing.T) {
	facts := committedFacts(t)
	spine := spineScoped(t)

	// The same view over the same declarations, built once over the store and
	// once over none of it: the first is the guard's own vacuity check, the
	// second is what it refuses.
	if _, err := structure.Build(facts, []structure.Scoped{spine}); err != nil {
		t.Fatalf("the spine's view over the committed store: %v", err)
	}
	_, err := structure.Build(nil, []structure.Scoped{spine})
	if err == nil {
		t.Fatal("a view admitting no fact was built rather than refused")
	}
	for _, want := range []string{`"spine-doc"`, "admits no fact"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not say %s:\n%v", want, err)
		}
	}
}

func TestBuildRefusesADocumentNamedTwice(t *testing.T) {
	facts := committedFacts(t)
	spine := spineScoped(t)
	_, err := structure.Build(facts, []structure.Scoped{spine, spine})
	if err == nil || !strings.Contains(err.Error(), `"spine-doc" is named twice`) {
		t.Fatalf("two documents of one name were not refused by name: %v", err)
	}
}

func TestBuildRefusesAScopeNoCutReads(t *testing.T) {
	facts := committedFacts(t)
	_, err := structure.Build(facts, []structure.Scoped{{Name: "orphan", Scopes: []string{"no-such-scope"}}})
	if err == nil || !strings.Contains(err.Error(), `"no-such-scope"`) {
		t.Fatalf("a scope no cut reads was not refused by name: %v", err)
	}
}

// TestEveryFactIsCarriedOnceWithItsProvenance pins the two properties the
// bead the document answers asks for: a fact appears once however many views
// admit it, and what it carries is the store's own locator, verbatim.
func TestEveryFactIsCarriedOnceWithItsProvenance(t *testing.T) {
	facts := committedFacts(t)
	doc, err := structure.Build(facts, []structure.Scoped{spineScoped(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Facts) == 0 {
		t.Fatal("the document carries no fact, so nothing below is asserted")
	}
	byID := make(map[string]fact.Fact, len(facts))
	for _, f := range facts {
		byID[f.ID] = f
	}
	seen := map[string]bool{}
	for i, r := range doc.Facts {
		if seen[r.ID] {
			t.Errorf("fact %s is carried twice", r.ID)
		}
		seen[r.ID] = true
		f, ok := byID[r.ID]
		if !ok {
			t.Errorf("record %d names %q, which the store does not carry", i, r.ID)
			continue
		}
		if r.DocID != f.DocID || r.Page != f.Page || r.Offset != f.Offset || r.Token != f.Token || r.AmountCents != f.AmountCents {
			t.Errorf("record %d does not carry its fact's locator verbatim:\n%+v\n%+v", i, r, f)
		}
		if r.Token == "" {
			t.Errorf("record %d carries an empty token, which fact-token-reparses would refuse", i)
		}
	}
	for _, v := range doc.Views {
		for _, at := range v.Facts {
			if at < 0 || at >= len(doc.Facts) {
				t.Errorf("view %q names fact %d, outside 0..%d", v.Name, at, len(doc.Facts)-1)
			}
		}
	}
}

// TestTheDocumentRoundTripsItsOwnEncoding is what makes the omitempty tags
// safe: a field dropped from the wire comes back different, and cmp says so.
func TestTheDocumentRoundTripsItsOwnEncoding(t *testing.T) {
	facts := committedFacts(t)
	doc, err := structure.Build(facts, []structure.Scoped{spineScoped(t)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var back structure.Document
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(doc, back); diff != "" {
		t.Fatalf("the document does not survive its own encoding (-built +decoded):\n%s", diff)
	}
}
