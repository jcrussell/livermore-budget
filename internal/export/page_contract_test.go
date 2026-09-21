package export

import (
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/schema"
)

// TestTheSchemaStatesWhatThePageConfigCarries holds schema/page.schema.json to
// the structs [encodeConfig] marshals: every JSON name window.FISC_CONFIG can
// carry is a property there, and nothing is a property there it cannot carry.
//
// WHY A SHAPE CONTRACT IS NEEDED AT ALL HERE, since the page renders either
// way: these structs are rendered TWICE, by the template which reads Go FIELD
// names and as JSON which the year toggle reads. Only the second cares about
// tags, and dropping one is silent -- the page still renders, because the
// template never sees the JSON, and only the tiles a reader gets AFTER
// switching year go blank. That is exactly how it shipped once and was caught
// by eye.
//
// ONE TEST AND NOT ONE PER STRUCT, which is the part worth keeping. A
// marshal-and-compare-top-level-keys test cannot see an inner shape at all:
// "caveats" is one entry of yearView's key list whatever its elements turn out
// to be, so caveatRef's three fields needed a second test, stepView's a third,
// and clientDoc's was simply missing. [schema.Names] walks the whole tree, so
// there is nothing left for a per-struct test to add.
//
// WHAT THE SCHEMA STATES THAT THIS COMPARISON CANNOT. caveatRef is deliberately
// id, summary and href with NO `text`: a chart page shows a line and links to
// the paragraph, and the cheapest way to undo that is to put the paragraph back
// within a template's reach. additionalProperties: false in the schema is what
// fails the moment such a field exists, rather than when some template gets
// round to rendering it.
func TestTheSchemaStatesWhatThePageConfigCarries(t *testing.T) {
	stated, err := schema.Names(schema.Page)
	if err != nil {
		t.Fatalf("read %s: %v", schema.Page, err)
	}
	emitted := schema.StructNames(reflect.TypeOf(clientConfig{}), "")
	slices.Sort(stated)
	slices.Sort(emitted)
	if len(stated) == 0 {
		t.Fatalf("%s states no property, so this test compares nothing", schema.Page)
	}
	if diff := cmp.Diff(emitted, stated); diff != "" {
		t.Errorf("%s and window.FISC_CONFIG name different fields (-emitted +stated):\n%s\n"+
			"site/app.js reads these off CONFIG; a tag dropped on either side is a key "+
			"the page renders server-side and blanks after a year switch.", diff, schema.Page)
	}
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
