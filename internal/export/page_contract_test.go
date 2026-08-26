package export

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
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
	want := []string{"basis", "caveats", "counts", "figures", "hero", "label", "path", "stem", "title", "year"}
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

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
