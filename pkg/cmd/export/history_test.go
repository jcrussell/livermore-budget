package export

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	historyBlock    = regexp.MustCompile(`(?s)<tbody>(.*?)</tbody>`)
	historyHeading  = regexp.MustCompile(`<th scope="colgroup"[^>]*>([^<]*)</th>`)
	historyRow      = regexp.MustCompile(`<th scope="row">([^<]*)</th>\s*<td class="subtle">([^<]*)</td>`)
	historyTableFor = []string{"history.html", "balances.html"}
)

// TestNoHistoryRowSaysItsSectionHeadingThreeTimes reads the ten-year tables
// the committed store exports: no row's Category cell repeats its block's
// heading, which on a row its section closes on would stack the one phrase
// three times (heading, row header, cell) for a screen reader. The anchor is
// that such a row exists, so the scan has its subject.
func TestNoHistoryRowSaysItsSectionHeadingThreeTimes(t *testing.T) {
	dir := exportedSite(t)
	closing := 0
	for _, path := range historyTableFor {
		b, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, block := range historyBlock.FindAllStringSubmatch(string(b), -1) {
			head := historyHeading.FindStringSubmatch(block[1])
			if head == nil {
				t.Fatalf("%s: a block with no heading", path)
			}
			heading := html.UnescapeString(head[1])
			for _, row := range historyRow.FindAllStringSubmatch(block[1], -1) {
				label, category := html.UnescapeString(row[1]), html.UnescapeString(row[2])
				if strings.EqualFold(label, heading) {
					closing++
					t.Logf("%s: block %q closes on row %q, Category cell %q", path, heading, label, category)
				}
				if strings.EqualFold(category, heading) {
					t.Errorf("%s: row %q's Category cell %q repeats its block's heading %q", path, label, category, heading)
				}
			}
		}
	}
	if closing == 0 {
		t.Fatal("no block closes on a row reading as its heading, so this scan holds nothing")
	}
}
