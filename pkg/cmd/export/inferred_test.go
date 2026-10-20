package export

import (
	"encoding/json"
	stdhtml "html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// inferredRow is one flow the Sources page's What we inferred lists, with
// the pages it cites as "doc pN".
type inferredRow struct {
	label, dollars string
	facts          []string
	pages          []string
}

var (
	inferredSection = regexp.MustCompile(`(?s)<section class="inferred-column" data-column="([^"]+)">(.*?)</section>`)
	inferredItem    = regexp.MustCompile(`(?s)<li data-facts="([^"]*)">(?:<div class="what">)?◇ (.*?): (-?\$[0-9,.]+) <span(.*?)</li>`)
	inferredPage    = regexp.MustCompile(`href="[^"]*?([^/"]+)/pages/p0*([0-9]+)\.txt"`)
)

// TestTheSourcesPageListsEachInferredFigureOnceAtItsLeaf holds What we
// inferred, over the store as built, to four things: no flow is listed as a
// hub leg whose leaf is a different pair of nodes; no two flows with one
// label and one figure share a cell; every fact id of every derived link is
// cited by a listed flow citing every page that link cites, and every
// derived node is listed; and FY2026-27's Internal Service Funds draw and
// General Fund contribution read as the declarations settle them.
// Mutation: name a listed flow at its ribbon's own ends rather than its
// Repeats' -- the leg arm and the FY2026-27 arm go red on the Internal
// Service Funds legs; cite each listed flow at its first page alone -- the
// page arm goes red on every flow citing two.
func TestTheSourcesPageListsEachInferredFigureOnceAtItsLeaf(t *testing.T) {
	io, _, _, _ := iostreams.Test()
	opts := &Options{
		IO:        io,
		RepoRoot:  func() (string, error) { return repoRootForTest(t), nil },
		OutputDir: filepath.Join(t.TempDir(), "dist"),
		Build:     buildAll,
	}
	if err := exportRun(opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(opts.OutputDir, "provenance.html")) // #nosec G304 -- the temp site.
	if err != nil {
		t.Fatal(err)
	}
	sections := inferredSection.FindAllStringSubmatch(string(page), -1)
	if len(sections) == 0 {
		t.Fatal("provenance.html lists no column's inferences, so nothing below is checked")
	}
	legs, fy2027 := 0, false
	for _, sec := range sections {
		path, body := sec[1], stdhtml.UnescapeString(sec[2])
		var rows []inferredRow
		for _, m := range inferredItem.FindAllStringSubmatch(body, -1) {
			r := inferredRow{label: m[2], dollars: m[3], facts: strings.Fields(m[1])}
			for _, p := range inferredPage.FindAllStringSubmatch(m[4], -1) {
				r.pages = append(r.pages, p[1]+" p"+p[2])
			}
			rows = append(rows, r)
		}
		raw, err := os.ReadFile(filepath.Join(opts.OutputDir, path)) // #nosec G304 -- the temp site.
		if err != nil {
			t.Fatal(err)
		}
		var col export.ColumnDoc
		if err := json.Unmarshal(raw, &col); err != nil {
			t.Fatal(err)
		}
		label := func(id string) string {
			for _, n := range col.Nodes {
				if n.ID == id {
					return n.Label
				}
			}
			return id
		}
		for _, n := range col.Nodes {
			if n.Derived && !strings.Contains(body, "◇ "+n.Label+"</div>") {
				t.Errorf("%s does not list its derived node %s", path, n.ID)
			}
		}
		for name, sched := range col.Schedules {
			for _, l := range sched.Links {
				if !l.Derived {
					continue
				}
				var locs []project.Source
				if err := json.Unmarshal(l.Locators, &locs); err != nil {
					t.Fatal(err)
				}
				var cited []string
				for _, s := range locs {
					for _, p := range s.Pages {
						cited = append(cited, s.DocID+" p"+strconv.Itoa(p))
					}
				}
				for _, id := range l.FactIDs {
					if !slices.ContainsFunc(rows, func(r inferredRow) bool {
						return slices.Contains(r.facts, id) &&
							!slices.ContainsFunc(cited, func(p string) bool { return !slices.Contains(r.pages, p) })
					}) {
						t.Errorf("%s %s's derived link %s -> %s cites %s on %v, which no listed flow cites on all those pages",
							path, name, col.Nodes[l.From].ID, col.Nodes[l.To].ID, id, cited)
					}
				}
				if l.Repeats == "" {
					continue
				}
				src, dst := project.LeafEnds(l.Repeats)
				if src == "" || dst == "" {
					t.Errorf("%s %s's derived link %s -> %s repeats %q, which spells no leaf's ends",
						path, name, col.Nodes[l.From].ID, col.Nodes[l.To].ID, l.Repeats)
					continue
				}
				own := label(col.Nodes[l.From].ID) + " → " + label(col.Nodes[l.To].ID)
				if own == label(src)+" → "+label(dst) {
					continue
				}
				legs++
				ids := slices.Sorted(slices.Values(l.FactIDs))
				for _, r := range rows {
					if r.label == own && slices.Equal(r.facts, ids) {
						t.Errorf("%s lists the hub leg %s: %s in place of its leaf %s -> %s",
							path, own, r.dollars, src, dst)
					}
				}
			}
		}
		for i, a := range rows {
			for _, b := range rows[i+1:] {
				if a.label == b.label && a.dollars == b.dollars &&
					slices.ContainsFunc(a.facts, func(id string) bool { return slices.Contains(b.facts, id) }) {
					t.Errorf("%s lists %s: %s twice, sharing a cell", path, a.label, a.dollars)
				}
			}
		}
		if path != "fy2027-adopted.json" {
			continue
		}
		fy2027 = true
		// The spine's printed group draw and General Fund contribution are
		// cells no Completes, Repeats or Restates ties to the expansion's
		// fund rows, so each is listed beside them.
		count := map[string][]inferredRow{}
		for _, r := range rows {
			count[r.label+": "+r.dollars] = append(count[r.label+": "+r.dollars], r)
		}
		if got := count["Fund Balance Draw → Internal Service Funds: $7,160,645"]; len(got) != 1 {
			t.Errorf("%s lists the spine's Internal Service Funds draw %d times, want once", path, len(got))
		}
		if got := count["General Fund → Fund Balance Contribution: $2,351,098"]; len(got) != 2 {
			t.Errorf("%s lists the General Fund's contribution %d times, want the spine's and fund/100's", path, len(got))
		}
		for key, rs := range count {
			if strings.HasPrefix(key, "Fund Balance Draw → Internal Service Funds:") && key != "Fund Balance Draw → Internal Service Funds: $7,160,645" {
				t.Errorf("%s lists %s for %d flows, a draw at the fund group rather than its fund", path, key, len(rs))
			}
		}
	}
	if !fy2027 {
		t.Error("provenance.html lists no fy2027-adopted.json column, so its arm checks nothing")
	}
	if legs == 0 {
		t.Fatal("no derived link is a hub leg, so the leg arm covers nothing")
	}
	t.Logf("%d derived hub legs across %d columns", legs, len(sections))
}
