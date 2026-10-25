package mapping

import (
	"os"
	"strings"
	"testing"
)

// TestDroppingARevenueRuleFails deletes a General Fund revenue rule from the
// published file: p130's `Total General Fund` rollup names it by id, so the
// file stops being loadable at all. A rule no rollup covers is caught by
// verify's tie against the spine instead.
func TestDroppingARevenueRuleFails(t *testing.T) {
	src, err := os.ReadFile(publishedSpine)
	if err != nil {
		t.Fatalf("read published file: %v", err)
	}
	cut := removeRule(t, string(src), "gf-rev-licenses-permits")
	if _, err := parse(strings.NewReader(cut), publishedSpine); err == nil {
		t.Fatal("a file with gf-rev-licenses-permits removed parsed; gf-total-revenues " +
			"covers it and must not resolve without it")
	} else if !strings.Contains(err.Error(), "gf-rev-licenses-permits") {
		t.Errorf("error = %v, want it to name the missing rule", err)
	}
}

// removeRule cuts one rule out of the YAML source, by id, keeping the rest
// byte-identical. It is deliberately textual: the point is to delete what an
// author would delete.
func removeRule(t *testing.T, src, id string) string {
	t.Helper()
	start := strings.Index(src, "  - id: "+id+"\n")
	if start < 0 {
		t.Fatalf("no rule %q in the published file", id)
	}
	rest := src[start+1:]
	next := strings.Index(rest, "\n  - id: ")
	if next < 0 {
		t.Fatalf("rule %q is the last in the file; this helper needs a following rule", id)
	}
	return src[:start] + rest[next+1:]
}
