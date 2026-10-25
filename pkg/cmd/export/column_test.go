package export

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// repoRootForTest is the checkout these tests build from.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// TestEveryFundGroupNodeHasAPlaceInItsColumnsOrder: a column's fund_groups is
// exactly its fund_group nodes. site/app.js orders by the list with no
// fallback, so an omitted group would sort at -1.
func TestEveryFundGroupNodeHasAPlaceInItsColumnsOrder(t *testing.T) {
	built, err := buildAll(repoRootForTest(t))
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	columns, _, err := export.ColumnsOf(built.Projections, "fisc test")
	if err != nil {
		t.Fatalf("columnsOf: %v", err)
	}
	held := 0
	for name, col := range columns {
		listed := map[string]bool{}
		for _, g := range col.FundGroups {
			listed[g.ID] = true
		}
		nodes := map[string]bool{}
		for _, n := range col.Nodes {
			// The role vocabulary is schema/column.schema.json's; the schema
			// test beside this one holds the string.
			if n.Role != "fund_group" {
				continue
			}
			nodes[n.ID] = true
			if !listed[n.ID] {
				t.Errorf("%s carries fund-group node %s and its fund_groups omit it; the client would place it nowhere", name, n.ID)
			}
			held++
		}
		for id := range listed {
			if !nodes[id] {
				t.Errorf("%s lists %s among its fund groups and carries no such node", name, id)
			}
		}
	}
	if held == 0 {
		t.Fatal("no column carries a fund-group node, so this test holds nothing")
	}
	if !t.Failed() {
		t.Logf("%d fund-group node(s) across %d column(s), each in its column's order", held, len(columns))
	}
}

// exportedSite runs `fisc export` over the committed store into a temp dir.
func exportedSite(t *testing.T) string {
	t.Helper()
	opts, _, _, _ := testOptions(t)
	root := repoRootForTest(t)
	opts.RepoRoot = func() (string, error) { return root, nil }
	opts.Build = buildAll
	if err := exportRun(context.Background(), opts); err != nil {
		t.Fatalf("exportRun: %v", err)
	}
	return opts.OutputDir
}

// TestEveryServedColumnIsTheScheduleItFolds holds every column file the export
// writes to facts/facts.jsonl: indices resolve, links' ends are the projection's,
// and facts match year and basis, sum to the value and are the pages cited.
func TestEveryServedColumnIsTheScheduleItFolds(t *testing.T) {
	dir := exportedSite(t)
	root := repoRootForTest(t)
	byID := storeByID(t)
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	columns, ix, err := export.ColumnsOf(built.Projections, "fisc test")
	if err != nil {
		t.Fatalf("ColumnsOf: %v", err)
	}
	servedFiles := []string{"fy2024-actual.json", "fy2025-revised.json", "fy2026-adopted.json", "fy2027-adopted.json"}
	if got := slices.Sorted(maps.Keys(columns)); !slices.Equal(got, servedFiles) {
		t.Fatalf("ColumnsOf builds %v and this test reads %v", got, servedFiles)
	}
	flipped := 0
	for _, served := range servedFiles {
		raw, err := os.ReadFile(filepath.Join(dir, served)) // #nosec G304 -- a temp dir this test wrote.
		if err != nil {
			t.Fatalf("fisc export wrote no %s: %v", served, err)
		}
		var col export.ColumnDoc
		if err := json.Unmarshal(raw, &col); err != nil {
			t.Fatalf("decode %s: %v", served, err)
		}
		cited := 0
		for schedule, sched := range col.Schedules {
			for _, n := range sched.Nodes {
				if n.Node < 0 || n.Node >= len(col.Nodes) {
					t.Errorf("%s %s draws node %d of %d", served, schedule, n.Node, len(col.Nodes))
				}
			}
			stem, ok := ix.Stem(served, schedule)
			if !ok {
				t.Errorf("%s serves schedule %s, which no built document is", served, schedule)
				continue
			}
			var doc struct {
				Links []struct{ Source, Target string } `json:"links"`
			}
			if err := json.Unmarshal(built.Projections[stem], &doc); err != nil {
				t.Fatalf("decode %s: %v", stem, err)
			}
			if len(doc.Links) != len(sched.Links) {
				t.Errorf("%s %s serves %d links and %s draws %d", served, schedule, len(sched.Links), stem, len(doc.Links))
				continue
			}
			for i, l := range sched.Links {
				at := fmt.Sprintf("%s %s link %d", served, schedule, i)
				if l.From < 0 || l.From >= len(col.Nodes) || l.To < 0 || l.To >= len(col.Nodes) {
					t.Errorf("%s joins %d -> %d of %d nodes", at, l.From, l.To, len(col.Nodes))
					continue
				}
				src, dst := col.Nodes[l.From].ID, col.Nodes[l.To].ID
				if src != doc.Links[i].Source || dst != doc.Links[i].Target {
					t.Errorf("%s is %s -> %s and %s draws %s -> %s", at, src, dst, stem, doc.Links[i].Source, doc.Links[i].Target)
				}
				for _, fault := range linkFactFaults(at, l, src, dst, col.Column, byID) {
					t.Error(fault)
				}
				if len(l.FactIDs) > 0 {
					cited++
					if src == project.NodeFundBalanceDraw {
						flipped++
					}
				}
			}
		}
		if cited == 0 {
			t.Errorf("%s serves no cited link, so nothing of it was held", served)
		}
	}
	if flipped == 0 {
		t.Error("no fund-balance draw was checked, so its negated allowance was never exercised")
	}
}
