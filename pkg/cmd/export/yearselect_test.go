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
	yearLabel  = regexp.MustCompile(`<option value="([^"]+)"[^>]*>([^<]*)</option>`)
)

// TestEveryPageLabelsAYearAlike holds every year select Go writes to one label
// per key.
// Mutation: label capital's options with the fiscal year label alone rather
// than through yearOptionOf -- capital.html's arms go red.
func TestEveryPageLabelsAYearAlike(t *testing.T) {
	dir := exportedSite(t)
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(pages) == 0 {
		t.Fatalf("the export wrote no pages: %v", err)
	}
	type seen struct{ label, page string }
	byKey := map[string]seen{}
	offering := map[string]int{}
	for _, p := range pages {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		m := yearSelect.FindSubmatch(raw)
		if m == nil {
			continue
		}
		page := filepath.Base(p)
		for _, o := range yearLabel.FindAllSubmatch(m[1], -1) {
			key, label := string(o[1]), string(o[2])
			offering[page]++
			first, ok := byKey[key]
			if !ok {
				byKey[key] = seen{label, page}
				continue
			}
			if first.label != label {
				t.Errorf("%s labels %s %q and %s labels it %q", first.page, key, first.label, page, label)
			}
		}
	}
	for _, page := range []string{"flows.html", "ledger.html", "department.html", "capital.html"} {
		if offering[page] == 0 {
			t.Errorf("%s offers no year, so this test does not hold it", page)
		}
	}
}

// TestEachNavLinkListsTheYearsItsViewOffers: a top-bar link's data-years is its
// page's year select's keys, in order, and none where there is no select.
// Mutation: drop data-years from "topbar" in site/chrome.html.tmpl, and every
// per-year link goes red.
func TestEachNavLinkListsTheYearsItsViewOffers(t *testing.T) {
	dir := exportedSite(t)
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
