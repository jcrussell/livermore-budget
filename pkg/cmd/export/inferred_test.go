package export

import (
	stdhtml "html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"encoding/json"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// inferredRow is one flow the Sources page's What we inferred lists.
type inferredRow struct {
	label, dollars string
	facts          []string
}

var (
	inferredSection = regexp.MustCompile(`(?s)<section class="inferred-column" data-column="([^"]+)">(.*?)</section>`)
	inferredItem    = regexp.MustCompile(`(?s)<li data-facts="([^"]*)">(?:<div class="what">)?◇ (.*?): (-?\$[0-9,.]+) <span`)
)

// TestTheSourcesPageListsEachInferredFigureOnceAtItsLeaf holds What we
// inferred, over the store as built, to four things: no flow is listed as a
// hub leg whose leaf is a different pair of nodes; no two flows with one
// label and one figure share a cell; every derived link's cells are cited by
// some flow its column lists, and every derived node is listed; and
// FY2026-27's General Fund contribution and Internal Service Funds draw read
// once, at the fund.
// Mutation: name a listed flow at its ribbon's own ends rather than its
// Repeats' -- the leg arm goes red on the Capital Funds and Internal Service
// Funds group legs; never drop a restated total -- the FY2026-27 arm goes red
// on the second General Fund row and the $7,160,645 one.
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
	legs := 0
	for _, sec := range sections {
		path, body := sec[1], stdhtml.UnescapeString(sec[2])
		var rows []inferredRow
		listed := map[string]bool{}
		for _, m := range inferredItem.FindAllStringSubmatch(body, -1) {
			r := inferredRow{label: m[2], dollars: m[3], facts: strings.Fields(m[1])}
			rows = append(rows, r)
			for _, id := range r.facts {
				listed[id] = true
			}
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
				for _, id := range l.FactIDs {
					if !listed[id] {
						t.Errorf("%s %s's derived link %s -> %s cites %s, which no listed flow cites",
							path, name, col.Nodes[l.From].ID, col.Nodes[l.To].ID, id)
					}
				}
				if l.Repeats == "" {
					continue
				}
				src, rest, _ := strings.Cut(l.Repeats, " -> ")
				dst := rest[:strings.LastIndexByte(rest, ' ')]
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
		count := map[string]int{}
		for _, r := range rows {
			count[r.label+": "+r.dollars]++
			if r.label == "Fund Balance Draw → Internal Service Funds" {
				t.Errorf("%s lists %s: %s, a draw at the fund group rather than its fund", path, r.label, r.dollars)
			}
			if r.dollars == amount.Cents(716064500).Dollars() {
				t.Errorf("%s lists %s: %s, the total its funds' draws restate", path, r.label, r.dollars)
			}
		}
		if n := count["General Fund → Fund Balance Contribution: $2,351,098"]; n != 1 {
			t.Errorf("%s lists the General Fund's contribution %d times, want once", path, n)
		}
	}
	if legs == 0 {
		t.Fatal("no derived link is a hub leg, so the leg arm covers nothing")
	}
	t.Logf("%d derived hub legs across %d columns", legs, len(sections))
}
