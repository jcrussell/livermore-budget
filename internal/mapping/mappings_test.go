package mapping

import (
	"testing"
)

// These tests read the PUBLISHED rule file — mappings/, not testdata/ — against
// the committed page fixtures, which are byte-identical copies of
// data/extracted/. Everything downstream (facts, the Sankey projection, the
// site) is built from that file, so a defect in it is a defect in a published
// artifact rather than in a test fixture.
const publishedSpine = "../../mappings/livermore-budget-fy2026-2027.yaml"

// publishedSpineScope is the scope pp.66-67's rules carry. Spelled here rather
// than imported: internal/project declares it and imports this package, so the
// dependency cannot run the other way.
//
// TestPublishedPartsDeclareColumnHeaders is the ratchet on an opt-in guard.
//
// A part with no column_headers is read exactly as it was before geometry
// existed: its figures are placed by position alone, and nothing anywhere says
// so. That is the same silent degradation the resolver refuses when it requires
// its document to answer for geometry rather than discovering the capability by
// type assertion, and without this assertion the next part someone adds inherits
// it by default.
func TestPublishedPartsDeclareColumnHeaders(t *testing.T) {
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	parts := 0
	for i := range f.Rules {
		ru := &f.Rules[i]
		for j := range ru.Parts {
			p := &ru.Parts[j]
			parts++
			if len(p.ColumnHeaders) == 0 {
				t.Errorf("rule %s p%d declares no column_headers, so its figures are "+
					"placed by position alone", ru.ID, p.Page)
			}
		}
	}
	if parts == 0 {
		t.Fatal("the published file has no parts, so this test checked nothing")
	}
}
