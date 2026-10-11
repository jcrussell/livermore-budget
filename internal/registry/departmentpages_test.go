package registry

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestEveryPrintedDivisionIsFiledUnderItsDepartment holds the registry's
// divisions to the division blocks pp.85-125 PRINT, counted off the page text
// rather than off the registry: each department's pages, the ones its
// divisions cite in that range, print exactly as many `Division Total` rows
// above `Department Funding Sources` as the registry files divisions under it,
// and the range prints no such row on a page no division cites.
//
// The department page sums a department's spending over the divisions the
// registry files under it, so a division printed and not filed would leave a
// department's sum short with every figure on it cited. The count is the
// page's, so it moves when the page does and not when the registry does.
//
// Mutation: delete `cybersecurity` from data/departments.yaml -- a division
// p97 prints and the registry would no longer file -- and
// administrative-services counts 5 printed against 4 filed. Moving `horizons`
// under public-works fails both departments.
func TestEveryPrintedDivisionIsFiledUnderItsDepartment(t *testing.T) {
	r := realRegistry(t)
	const first, last = 85, 125

	printed := func(page int) int {
		b, err := os.ReadFile(fmt.Sprintf("%s/p%04d.txt", budgetPages, page))
		if err != nil {
			t.Fatalf("read page %d: %v", page, err)
		}
		// Below the heading is the block that names funds, not divisions.
		upper, _, _ := strings.Cut(string(b), "Department Funding Sources")
		n := 0
		for _, line := range strings.Split(upper, "\n") {
			if strings.Contains(line, "Division Total") {
				n++
			}
		}
		return n
	}

	pagesOf := map[string][]int{}
	filed := map[string]int{}
	citedBy := map[int]string{}
	for _, d := range r.Divisions() {
		filed[d.Department]++
		for _, p := range d.Pages {
			if p < first || p > last {
				continue
			}
			if was, ok := citedBy[p]; ok && was != d.Department {
				t.Errorf("p%d is cited by divisions of %s and %s; a department's page is its own", p, was, d.Department)
			}
			citedBy[p] = d.Department
			if !slices.Contains(pagesOf[d.Department], p) {
				pagesOf[d.Department] = append(pagesOf[d.Department], p)
			}
		}
	}

	counted := map[string]int{}
	total := 0
	for p := first; p <= last; p++ {
		n := printed(p)
		total += n
		if n == 0 {
			continue
		}
		dept, ok := citedBy[p]
		if !ok {
			t.Errorf("p%d prints %d Division Total row(s) and no division in the registry cites it", p, n)
			continue
		}
		counted[dept] += n
	}
	if diff := cmp.Diff(counted, filed); diff != "" {
		t.Errorf("Division Total rows printed per department (-printed +filed):\n%s", diff)
	}
	// Extrinsic: the corpus prints the axis whole on these pages.
	if total == 0 {
		t.Fatal("pp.85-125 print no Division Total row, so this test counted nothing")
	}
	t.Logf("pp.%d-%d print %d Division Total rows over %d departments", first, last, total, len(counted))
}
