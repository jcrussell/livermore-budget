package export

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestYearViewKeysAreTheOnesTheClientReads pins the shape of the year entries in
// window.FISC_CONFIG.
//
// These structs are rendered TWICE: by the template, which reads Go field names,
// and as JSON, which the year toggle reads. Only the second cares about tags,
// and dropping one is silent — the page still renders, because the template
// never sees the JSON, and only the tiles a reader gets after switching year go
// blank. That is exactly how it was shipped once and caught by eye rather than
// by a test.
func TestYearViewKeysAreTheOnesTheClientReads(t *testing.T) {
	blob, err := json.Marshal(yearView{})
	if err != nil {
		t.Fatalf("marshal yearView: %v", err)
	}
	var got map[string]any
	if err = json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("decode yearView: %v", err)
	}
	want := []string{"basis", "caveats", "chart_title", "counts", "figures", "hero", "label", "path", "stem", "title", "year"}
	if diff := cmp.Diff(want, keysOf(got)); diff != "" {
		t.Errorf("year keys (-want +got):\n%s\nsite/app.js reads these off CONFIG.years", diff)
	}

	blob, err = json.Marshal(figure{})
	if err != nil {
		t.Fatalf("marshal figure: %v", err)
	}
	got = map[string]any{}
	if err = json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("decode figure: %v", err)
	}
	if diff := cmp.Diff([]string{"kind", "label", "note", "value"}, keysOf(got)); diff != "" {
		t.Errorf("figure keys (-want +got):\n%s\nsite/app.js reads these off each tile", diff)
	}
}

// TestCaveatMetaKeysAreTheOnesTheDocumentCarries pins the inner shape that
// TestYearViewKeysAreTheOnesTheClientReads cannot see.
//
// THAT TEST DOES NOT FAIL WHEN THIS SHAPE DRIFTS, and the reason is worth
// stating rather than rediscovering: it marshals yearView and compares TOP-LEVEL
// keys, so "caveats" is one entry in its list whatever the elements turn out to
// be. When a caveat was a string that was the whole contract; now it is four
// fields, and three of them are load-bearing in different places -- id is the
// anchor a page links to, summary is the line a page shows, text is the
// paragraph a reader came for.
//
// caveatMeta is also the DECODE side: internal/export consumes projections as
// bytes and does not import internal/project, so these tags are what couples the
// two packages. A renamed tag on either side is a caveat that decodes to the
// zero value and renders as a blank line, with no error anywhere.
func TestCaveatMetaKeysAreTheOnesTheDocumentCarries(t *testing.T) {
	blob, err := json.Marshal(caveatMeta{})
	if err != nil {
		t.Fatalf("marshal caveatMeta: %v", err)
	}
	var got map[string]any
	if err = json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("decode caveatMeta: %v", err)
	}
	want := []string{"applies_to", "id", "summary", "text"}
	if diff := cmp.Diff(want, keysOf(got)); diff != "" {
		t.Errorf("caveat keys (-want +got):\n%s\ninternal/project.Caveat writes these and site/app.js reads them", diff)
	}
}

// TestCaveatRefKeysAreTheOnesTheClientReads pins what a PAGE is given about a
// caveat, and the load-bearing part is what is absent.
//
// caveatRef is deliberately id, summary and href -- no text. The whole point of
// this lane is that a chart page shows a line and links to the paragraph, and
// the cheapest way to undo it is to put the paragraph back within reach of the
// template. A field here is an invitation; this fails the moment one exists,
// rather than when some template gets round to rendering it.
//
// It is also the client contract: site/app.js reads c.summary and c.href off
// CONFIG.years[].caveats when repainting a year switch, and a dropped tag is
// silent in exactly yearView's way -- the server-rendered opening year is fine
// and only the caveats after a switch go blank.
func TestCaveatRefKeysAreTheOnesTheClientReads(t *testing.T) {
	blob, err := json.Marshal(caveatRef{})
	if err != nil {
		t.Fatalf("marshal caveatRef: %v", err)
	}
	var got map[string]any
	if err = json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("decode caveatRef: %v", err)
	}
	want := []string{"href", "id", "summary"}
	if diff := cmp.Diff(want, keysOf(got)); diff != "" {
		t.Errorf("caveat ref keys (-want +got):\n%s\n"+
			"site/app.js reads summary and href; a \"text\" key here would put the "+
			"paragraph back within a template's reach, which is the thing this lane removed", diff)
	}
}

// TestClientDocKeysAreTheOnesTheClientReads is the same pin one struct over,
// and it was missing.
//
// clientDoc is how site/app.js's citations() builds EVERY provenance URL on
// the site: pdf_url gives the PDF anchor, page_text_base the extracted-text
// one, records_base the fact-store shard. A dropped tag here is silent in
// exactly the way yearView's is -- the page renders, the chart draws, and the
// links a reader clicks to check a figure quietly stop appearing.
func TestClientDocKeysAreTheOnesTheClientReads(t *testing.T) {
	blob, err := json.Marshal(clientDoc{})
	if err != nil {
		t.Fatalf("marshal clientDoc: %v", err)
	}
	var got map[string]any
	if err = json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("decode clientDoc: %v", err)
	}
	want := []string{"pdf_url", "page_text_base", "publisher", "records_base", "title"}
	sort.Strings(want)
	if diff := cmp.Diff(want, keysOf(got)); diff != "" {
		t.Errorf("clientDoc keys (-want +got):\n%s\nsite/app.js reads these off CONFIG.docs", diff)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestBasisAuditedIsPinnedToTheEnum keeps [basisLabelFor]'s subject in step with
// the producer's basis value.
//
// This package does not import internal/project, and basisAudited is a second
// copy of what internal/mapping declares. The copy is what makes the drift
// silent: if the enum's value moved, basisLabelFor would simply stop matching
// and both ACFR ten-year pages would go back to printing the word their own
// caveat withdraws, with every other test still green.
func TestBasisAuditedIsPinnedToTheEnum(t *testing.T) {
	if basisAudited != string(mapping.BasisAudited) {
		t.Errorf("basisAudited is %q but mapping.BasisAudited is %q; basisLabelFor "+
			"matches on this string, so a mismatch silently stops relabelling the "+
			"columns the statistical-section-unaudited caveat covers",
			basisAudited, mapping.BasisAudited)
	}
}

// TestBasisLabelForRewritesOnlyTheAuditedBasisOfAnUnauditedDocument covers the
// three answers separately, because two of them are the ones a careless
// widening would break: a document without the caveat must keep its word, and a
// document with it must keep a basis that was never "audited".
func TestBasisLabelForRewritesOnlyTheAuditedBasisOfAnUnauditedDocument(t *testing.T) {
	unaudited := []caveatMeta{{ID: "some-other-caveat"}, {ID: UnauditedCaveatID}}
	other := []caveatMeta{{ID: "some-other-caveat"}}

	cases := []struct {
		name    string
		caveats []caveatMeta
		basis   string
		want    string
	}{
		{name: "audited under the caveat", caveats: unaudited, basis: basisAudited, want: "unaudited"},
		{name: "audited without the caveat", caveats: other, basis: basisAudited, want: basisAudited},
		{name: "audited with no caveats at all", caveats: nil, basis: basisAudited, want: basisAudited},
		{name: "another basis under the caveat", caveats: unaudited, basis: "adopted", want: "adopted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := basisLabelFor(tc.caveats, tc.basis); got != tc.want {
				t.Errorf("basisLabelFor(%v, %q) = %q, want %q", tc.caveats, tc.basis, got, tc.want)
			}
		})
	}
}
