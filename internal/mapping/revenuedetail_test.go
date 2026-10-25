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
