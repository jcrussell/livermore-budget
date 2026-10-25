package mapping

import (
	"testing"
)

// These tests read the published rule file against the committed page
// fixtures.
const publishedSpine = "../../mappings/livermore-budget-fy2026-2027.yaml"

// TestPublishedPartsDeclareColumnHeaders is the ratchet on an opt-in guard: a
// part with no column_headers places its figures by position alone.
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
