package export

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	siteNav    = regexp.MustCompile(`(?s)<nav class="site-nav".*?</nav>`)
	navLink    = regexp.MustCompile(`<li><a href="([^"#]+)"( data-years="([^"]*)")?>`)
	yearSelect = regexp.MustCompile(`(?s)<select id="year-select" disabled>(.*?)</select>`)
	yearOption = regexp.MustCompile(`<option value="([^"]+)"`)
)

// TestEachNavLinkListsTheYearsItsViewOffers holds every top-bar link of the
// site Go writes to the page it opens: a link's data-years is that page's
// year select's keys, in order, and a link with none opens a page with no
// select. site/format.js carryYear reads data-years alone, so a link listing
// a year its view lacks carries a reader into a refusal, and one listing
// none never carries the year at all.
//
// Mutation: drop the data-years attribute from "topbar" in
// site/chrome.html.tmpl, and every per-year link goes red.
func TestEachNavLinkListsTheYearsItsViewOffers(t *testing.T) {
	dir := clientExport(t)
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(pages) == 0 {
		t.Fatalf("the export wrote no pages: %v", err)
	}
	keysOf := map[string][]string{}
	for _, p := range pages {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		if m := yearSelect.FindSubmatch(raw); m != nil {
			for _, o := range yearOption.FindAllSubmatch(m[1], -1) {
				keys = append(keys, string(o[1]))
			}
		}
		keysOf[filepath.Base(p)] = keys
	}
	carried := 0
	for _, p := range pages {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range navLink.FindAllStringSubmatch(siteNav.FindString(string(raw)), -1) {
			target, listed := m[1], strings.Fields(m[3])
			want, ok := keysOf[target]
			if !ok {
				t.Errorf("%s links %s, which the export did not write", filepath.Base(p), target)
				continue
			}
			if len(listed) > 0 {
				carried++
			}
			if diff := cmp.Diff(strings.Join(want, " "), strings.Join(listed, " ")); diff != "" {
				t.Errorf("%s's link to %s lists years that are not its select's (-select +link):\n%s",
					filepath.Base(p), target, diff)
			}
		}
	}
	if carried == 0 {
		t.Error("no top-bar link carries a year, so this test held nothing")
	}
}
