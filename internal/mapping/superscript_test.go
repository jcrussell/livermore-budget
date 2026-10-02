package mapping

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/geom"
)

// budgetFixturePair reads one Budget Book fixture page's two substrates.
func budgetFixturePair(t *testing.T, page int) (string, *geom.Page) {
	t.Helper()
	text, err := os.ReadFile(fmt.Sprintf("../../testdata/pages/budget-p%04d.txt", page))
	if err != nil {
		t.Fatalf("read page fixture: %v", err)
	}
	raw, err := os.ReadFile(fmt.Sprintf("../../testdata/geometry/budget-p%04d.json", page))
	if err != nil {
		t.Fatalf("read geometry fixture: %v", err)
	}
	g, err := geom.ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	return string(text), g
}

// p186Marker returns the byte offset of p186's lone footnote "1" in the page
// text, and the index of its word in the geometry.
func p186Marker(t *testing.T, text string, g *geom.Page) (int, int) {
	t.Helper()
	off := -1
	base := 0
	for _, body := range strings.Split(text, "\n") {
		if strings.TrimSpace(body) == "1" {
			if off >= 0 {
				t.Fatal("p186 prints a lone \"1\" on two lines")
			}
			off = base + strings.Index(body, "1")
		}
		base += len(body) + 1
	}
	word := slices.IndexFunc(g.Words, func(w geom.Word) bool { return w.Text == "1" })
	if off < 0 || word < 0 {
		t.Fatal("p186's footnote \"1\" is gone from a substrate")
	}
	return off, word
}

// TestAFootnoteSuperscriptPairs: Budget Book p186 and p190 print a footnote
// "1" that -layout puts on a line of its own and the geometry clusters into
// "Capital Improvement Program Funds" below it. Both pages pair, the marker
// pairs with the superscript word on a line of its own, and the line that
// absorbed it holds only the label line's own words.
func TestAFootnoteSuperscriptPairs(t *testing.T) {
	for _, page := range []int{186, 190} {
		t.Run(fmt.Sprint(page), func(t *testing.T) {
			text, g := budgetFixturePair(t, page)
			pr, err := buildPairing(text, g)
			if err != nil {
				t.Fatalf("buildPairing: %v", err)
			}
			printed := strings.Split(text, "\n")
			var marker, label placed
			for off, pl := range pr.words {
				switch {
				case pl.word.Text == "1" && strings.TrimSpace(printed[lineAt(text, off)]) == "1":
					marker = pl
				case pl.word.Text == "Capital" && strings.HasPrefix(
					strings.TrimSpace(printed[lineAt(text, off)]), "Capital Improvement Program Funds"):
					label = pl
				}
			}
			if marker.num == 0 || label.num == 0 {
				t.Fatal("the marker or the label it sits over is not paired")
			}
			if got, want := label.num, marker.num+1; got != want {
				t.Errorf("label on text line %d, want %d (the line under the marker)", got, want)
			}
			if got := lineTexts(pr.lines[marker.line]); !slices.Equal(got, []string{"1"}) {
				t.Errorf("the marker's line holds %q, want only the marker", got)
			}
			if slices.Contains(lineTexts(pr.lines[label.line]), "1") {
				t.Errorf("the label's line still holds the marker: %q", lineTexts(pr.lines[label.line]))
			}
		})
	}
}

// TestAStrayLineThatIsNotAMarkerStillBreaksPairing: the same page with the
// absorbed word changed so it is no footnote marker. A three-digit figure is
// not a marker, and a full-height "1" is not a superscript; both are a
// disagreement the pairing cannot name, and it refuses.
func TestAStrayLineThatIsNotAMarkerStillBreaksPairing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(text string, off int, w *geom.Word) string
	}{
		{"three digits", func(text string, off int, w *geom.Word) string {
			w.Text = "123"
			return text[:off] + "123" + text[off+1:]
		}},
		{"full height", func(text string, off int, w *geom.Word) string {
			w.Y1 = w.Y0 + 8.82 // the height of the label it sits over
			return text
		}},
		{"not raised", func(text string, off int, w *geom.Word) string {
			w.Y0, w.Y1 = 239.57, 239.57+5.88 // the label's own top
			return text
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, g := budgetFixturePair(t, 186)
			off, word := p186Marker(t, text, g)
			text = tc.mutate(text, off, &g.Words[word])
			_, err := buildPairing(text, g)
			if err == nil || !strings.Contains(err.Error(), "61 non-blank lines but the geometry has 60") {
				t.Fatalf("buildPairing: %v, want the line-count refusal", err)
			}
		})
	}
}

// p186Guarded is p186Top read with the column guard on, the read pp.186-209's
// chained rules need on every part.
func p186Guarded(t *testing.T, unmapped bool) ([]Value, error) {
	t.Helper()
	src := strings.Replace(p186Top, "        #HEADINGS",
		`        headings: ["Special Revenue Funds"]`+"\n"+
			`        column_headers: ["7/1/23", "Revenues", "Transfers In", "Total Sources"]`, 1)
	if !unmapped {
		src = strings.Replace(src, "        unmapped_text:\n"+
			`          - {text: "1", note: "footnote marker printed alone between two rows"}`+"\n", "", 1)
	}
	return probeValues(t, src, 186)
}

// TestP186ReadsGuarded: every row of p186's top block, the skipped ones
// included, is read through the guard, so each figure was placed in the band
// of the column it files under; the two published ones are the page's.
func TestP186ReadsGuarded(t *testing.T) {
	vals, err := p186Guarded(t, true)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	got := make([]amount.Cents, len(vals))
	for i, v := range vals {
		got[i] = v.Cents
	}
	if diff := cmp.Diff([]amount.Cents{1444069000, 3988199400}, got); diff != "" {
		t.Errorf("General Fund and Low Income Housing Fund FY2024 (-want +got):\n%s", diff)
	}
}

// TestP186UndeclaredMarkerIsStillRefused: the pairing reconciling the marker
// does not take it out of the text read. Inside a block it is still a line
// between two rows that no row maps, and the read refuses it until the part
// declares it.
func TestP186UndeclaredMarkerIsStillRefused(t *testing.T) {
	_, err := p186Guarded(t, false)
	if err == nil {
		t.Fatal("read the block with its footnote marker undeclared")
	}
	if got := diagnosis(t, err); !strings.Contains(got, "not mapped") {
		t.Errorf("refused for some other reason:\n%s", got)
	}
}

// TestP186IsPlacedByTheColumnGuard measures p186's grid under the band rule:
// every row's line files one figure per column in order, the one whose label
// wraps onto "& Internal Service" among them. The superscript sits left of the
// first header, in the row-label area, and files nowhere.
func TestP186IsPlacedByTheColumnGuard(t *testing.T) {
	full, other, room := fileFigures(t, 186, []string{"7/1/23", "Revenues", "Transfers In", "Total Sources"})
	t.Logf("p186: the closest figure ends %.2fpt short of the next column's header", room)
	if full != 50 {
		t.Errorf("%d lines file four figures in order, want all 50", full)
	}
	// The running footer's page number.
	if diff := cmp.Diff([]lineFiling{{"182", []int{3}}}, other); diff != "" {
		t.Errorf("lines not filed one figure per column (-want +got):\n%s", diff)
	}
}

// TestFootnoteSuperscriptsTheCorpusReconciles pins which committed pages pair
// with a footnote superscript split back onto its own line: those whose
// pairing holds more lines than the geometry clustered.
func TestFootnoteSuperscriptsTheCorpusReconciles(t *testing.T) {
	want := map[string][]int{
		"livermore-acfr-fy2025":        {36, 96, 99, 105, 124, 126, 128, 177, 178, 186},
		"livermore-budget-fy2026-2027": {68, 186, 190, 192, 194, 196},
	}
	got := map[string][]int{}
	for _, doc := range []string{"livermore-acfr-fy2025", "livermore-budget-fy2026-2027",
		"livermore-cip-fy2026-2030"} {
		pages, err := filepath.Glob("../../data/extracted/" + doc + "/geometry/p*.json")
		if err != nil || len(pages) == 0 {
			t.Fatalf("%s: no geometry (%v)", doc, err)
		}
		for _, path := range pages {
			var n int
			if _, err := fmt.Sscanf(filepath.Base(path), "p%04d.json", &n); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			g, err := geom.ParsePage(raw)
			if err != nil {
				t.Fatal(err)
			}
			text, err := os.ReadFile(fmt.Sprintf("../../data/extracted/%s/pages/p%04d.txt", doc, n))
			if err != nil {
				t.Fatal(err)
			}
			pr, err := buildPairing(string(text), g)
			if err == nil && len(pr.lines) > len(g.Lines()) {
				got[doc] = append(got[doc], n)
			}
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("pages paired by splitting out a footnote superscript (-want +got):\n%s", diff)
	}
}

func lineTexts(l geom.Line) []string {
	out := make([]string, len(l.Words))
	for i, w := range l.Words {
		out[i] = w.Text
	}
	return out
}
