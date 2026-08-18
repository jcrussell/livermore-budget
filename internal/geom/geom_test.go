package geom

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// page renders a geometry artifact the way tools/extract.py writes one, so the
// tests decode the same shape the corpus ships rather than a convenient one.
func page(t *testing.T, words []string) []byte {
	t.Helper()
	body := "[]"
	if len(words) > 0 {
		body = "[\n  " + strings.Join(words, ",\n  ") + "\n ]"
	}
	return []byte(fmt.Sprintf(`{
 "doc_id": "livermore-budget-fy2026-2027",
 "height": 792.0,
 "page": 66,
 "schema_version": 1,
 "width": 612.0,
 "words": %s
}
`, body))
}

func TestParsePageReadsTheFlatWordArray(t *testing.T) {
	got, err := ParsePage(page(t, []string{
		`[216.84,2.88,293.18,30.27,"BUDGET"]`,
		`[297.1,2.88,373.44,30.27,"SUMMARY"]`,
	}))
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	want := &Page{
		DocID: "livermore-budget-fy2026-2027", Number: 66,
		Width: 612, Height: 792,
		Words: []Word{
			{X0: 216.84, Y0: 2.88, X1: 293.18, Y1: 30.27, Text: "BUDGET"},
			{X0: 297.1, Y0: 2.88, X1: 373.44, Y1: 30.27, Text: "SUMMARY"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParsePage (-want +got):\n%s", diff)
	}
	if got, want := got.Words[0].Right(), 293.18; got != want {
		t.Errorf("Right() = %v, want %v", got, want)
	}
}

func TestParsePageRefusals(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{
			name: "a schema version this reader does not know",
			body: strings.Replace(string(page(t, nil)),
				`"schema_version": 1`, `"schema_version": 2`, 1),
			want: "geometry schema_version 2, want 1",
		},
		{
			name: "no doc_id",
			body: strings.Replace(string(page(t, nil)),
				`"doc_id": "livermore-budget-fy2026-2027"`, `"doc_id": ""`, 1),
			want: "no doc_id",
		},
		{
			name: "no page number",
			body: strings.Replace(string(page(t, nil)), `"page": 66`, `"page": 0`, 1),
			want: "want a positive page number",
		},
		{
			name: "an inverted bounding box",
			body: string(page(t, []string{`[300.0,2.88,216.84,30.27,"BUDGET"]`})),
			want: "inverted box",
		},
		{
			// Lines is a greedy sweep that assumes the extractor's ordering.
			// Handed an out-of-order page it would return confident nonsense,
			// and a hand-written fixture is exactly how one gets in.
			name: "words out of the extractor's reading order",
			body: string(page(t, []string{
				`[216.84,30.0,293.18,40.0,"SECOND"]`,
				`[216.84,2.88,293.18,12.0,"FIRST"]`,
			})),
			want: "precedes word 0",
		},
		{
			name: "a word that is not a five-element array",
			body: string(page(t, []string{`[216.84,2.88,293.18,"BUDGET"]`})),
			want: "want 5",
		},
		{
			name: "a word that is not an array at all",
			body: string(page(t, []string{`{"x0":1}`})),
			want: "not an array",
		},
		{
			// json.Unmarshal into a *float64 treats null as a no-op with a nil
			// error, so without an explicit refusal this decodes to a zero-area
			// box at the page origin: a ghost word that passes the inverted-box
			// check and sorts to the front of the page.
			name: "a null coordinate",
			body: string(page(t, []string{`[null,2.88,293.18,30.27,"BUDGET"]`})),
			want: "coordinate 0 is null",
		},
		{
			name: "null text",
			body: string(page(t, []string{`[216.84,2.88,293.18,30.27,null]`})),
			want: "text is null",
		},
		{
			// The remaining three arms of the reading-order comparison. They are
			// easy to transpose, because the wire row is [x0,y0,x1,y1] while the
			// sort key is (y0,x0,y1,x1) -- and this check is the only thing
			// standing between a hand-written fixture and Lines() returning
			// confident nonsense.
			// x1 runs the OTHER way here on purpose, so the case fails if the
			// y1 and x1 arms are transposed rather than merely if one is absent.
			name: "words out of order on the y1 tie-break",
			body: string(page(t, []string{
				`[216.84,2.88,293.18,30.27,"TALL"]`,
				`[216.84,2.88,350.00,12.00,"SHORT"]`,
			})),
			want: "precedes word 0",
		},
		{
			name: "words out of order on the x1 tie-break",
			body: string(page(t, []string{
				`[216.84,2.88,293.18,12.00,"WIDE"]`,
				`[216.84,2.88,240.00,12.00,"NARROW"]`,
			})),
			want: "precedes word 0",
		},
		{
			name: "words out of order on the text tie-break",
			body: string(page(t, []string{
				`[216.84,2.88,293.18,12.00,"b"]`,
				`[216.84,2.88,293.18,12.00,"a"]`,
			})),
			want: "precedes word 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePage([]byte(tt.body))
			if err == nil {
				t.Fatalf("ParsePage = nil error, want one mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParsePage error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestWordlessPageIsAPageAndNotAnError pins the ACFR's one genuinely blank
// page. It still HAS both artifacts -- an empty pages/pNNNN.txt beside a
// geometry file with an empty words array -- so refusing it would refuse a page
// the extraction produced correctly.
func TestWordlessPageIsAPageAndNotAnError(t *testing.T) {
	p, err := ParsePage(page(t, nil))
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	if _, ok := p.MedianWordHeight(); ok {
		t.Error("MedianWordHeight() reported a height for a page with no words")
	}
	if got := p.Lines(); got != nil {
		t.Errorf("Lines() = %v, want nil", got)
	}
}

func TestMedianWordHeightIgnoresTheOutlyingTitle(t *testing.T) {
	// A page title is set two or three times the body height. The median is
	// what keeps one such word from widening the line tolerance for the whole
	// page.
	p, err := ParsePage(page(t, []string{
		`[100.0,2.0,200.0,29.0,"TITLE"]`,
		`[100.0,50.0,150.0,59.09,"one"]`,
		`[160.0,50.0,210.0,59.09,"two"]`,
		`[100.0,70.0,150.0,79.09,"three"]`,
	}))
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	got, ok := p.MedianWordHeight()
	if !ok {
		t.Fatal("MedianWordHeight() reported no height")
	}
	// Compared with a tolerance because the height is a subtraction of two
	// decimal coordinates, not a stored value: 59.09 - 50.0 is 9.090000000000003
	// in binary floating point, and pinning the exact bits would be pinning the
	// arithmetic rather than the median.
	if want := 9.09; math.Abs(got-want) > 1e-9 {
		t.Errorf("MedianWordHeight() = %v, want %v", got, want)
	}
}

// TestLinesGroupsByTheMeasuredMargins uses Budget Book p66's own numbers: a
// 9.09pt median word height gives a 4.545pt tolerance, the worst intra-line y0
// spread on that page is 0.96pt, and the tightest inter-line gap is 11.52pt.
// The tolerance therefore sits 4.7x above what it must tolerate and 2.5x below
// what it must separate.
func TestLinesGroupsByTheMeasuredMargins(t *testing.T) {
	p, err := ParsePage(page(t, []string{
		`[56.00,194.23,120.00,203.32,"Property"]`,
		`[125.00,194.23,160.00,203.32,"Taxes"]`,
		// 0.96pt below its line's first word, the worst spread measured on p66.
		`[330.00,195.19,390.00,204.28,"64,143,762"]`,
		// 11.52pt below, the tightest inter-line gap measured on p66.
		`[56.00,209.95,110.00,219.04,"Other"]`,
		`[115.00,209.95,150.00,219.04,"Taxes"]`,
	}))
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	got := lineTexts(p.Lines())
	want := [][]string{{"Property", "Taxes", "64,143,762"}, {"Other", "Taxes"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Lines (-want +got):\n%s", diff)
	}
}

// TestLinesSortsEachLineLeftToRight is not tidiness. The artifact is sorted
// y0-major, so within a cluster whose members differ in y0 by less than the
// tolerance -- which is the whole reason a tolerance exists -- file order is not
// left-to-right order. A consumer comparing a line against the page text token
// by token would disagree on exactly those lines.
func TestLinesSortsEachLineLeftToRight(t *testing.T) {
	p, err := ParsePage(page(t, []string{
		// Sorted by y0, this right-hand word comes FIRST in the file.
		`[330.00,194.23,390.00,203.32,"64,143,762"]`,
		`[56.00,195.19,120.00,204.28,"Property"]`,
	}))
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	got := lineTexts(p.Lines())
	want := [][]string{{"Property", "64,143,762"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Lines (-want +got):\n%s", diff)
	}
}

func lineTexts(lines []Line) [][]string {
	out := make([][]string, len(lines))
	for i, l := range lines {
		out[i] = make([]string, len(l.Words))
		for j, w := range l.Words {
			out[i][j] = w.Text
		}
	}
	return out
}
